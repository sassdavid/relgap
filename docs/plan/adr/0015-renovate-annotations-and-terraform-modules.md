# relgap ADR-0015: Renovate annotations, Terraform modules and Hugo links

- Status: accepted (2026-11-19)
- Settles the open questions of [pin-discovery.md](../specs/pin-discovery.md) for M27 and widens
  the `terraform`, `terragrunt` and `helm` detectors.

## Context
Some repositories pin versions that only a Renovate regex custom manager knows about. A Terraform
and Helm module repository wrote its chart versions as variable defaults, such as
`chart_version = optional(string, "1.23.0")`, under a
`# renovate: datasource=helm depName=karpenter registryUrl=...` comment. relgap 0.9.1 found none
of them. Today only the `dockerfile` detector reads such comments, and only above ARG and ENV.
Terraform `module` blocks and Terragrunt `terraform { source }` blocks pin module versions that
relgap doesn't read at all.

Facts checked on 2026-11-19:
- Renovate's `customManagers:*` presets (`lib/config/presets/internal/custom-managers.preset.ts`)
  read Dockerfiles, Makefiles and `*.mk`, workflow and `action.yml` files, GitLab, Azure and
  Bitbucket pipelines, `pom.xml` (with `<!-- renovate: -->`), `*.tfvars` and `Chart.yaml`. Each
  preset expects the value on the line after the comment. None reads `*.tf`.
- `GET registry.terraform.io/v1/modules/<ns>/<name>/<provider>` returns the latest version with
  `source` (the repository URL), `version` (`6.8.0`) and `tag` (`v6.8.0`).
- Artifact Hub's `GET /api/v1/repositories/search?url=<url>` matches the repository URL exactly.
  OCI repositories there usually include the chart name: `oci://public.ecr.aws/karpenter/karpenter`
  is the verified `aws-karpenter` repository, and `oci://public.ecr.aws/karpenter` is an unrelated
  mirror without that chart.
- Hugo resolves a relative `ref` path against the page's parent in the content tree, and a
  `../` path with a file extension against the content file's directory (Hugo docs, "Page
  references"). Karpenter's docs mix `concepts/disruption.md` with
  `getting-started/.../_index.md`, and its `website/hugo.yaml` sets `baseURL: /`.

## Decision
### Renovate annotations
- A new `renovate-comment` detector reads a `# renovate:`, `// renovate:` or `<!-- renovate: -->`
  comment and the first non-blank, non-comment line after it. When that line holds exactly one
  version (`1.2`, `v1.2.3`, `1.2.3-rc.1`, quoted or after `=`, `:` or `>`), the version is a pin.
  Zero or several versions add no pin.
- It reads `*.tf`, `*.tfvars`, `*.hcl`, every `*.yml` and `*.yaml`, `Makefile`, `GNUmakefile`,
  `*.mk`, `pom.xml`, `*.sh`, `.env` and `.env.*`, and the files `scan.extra_files` names. A file
  without the bytes `renovate:` is skipped after the read. Dockerfiles stay with the `dockerfile`
  detector, which keeps its own handling.
- It is on by default: the comment is the author's own statement of what the value is.
- The datasource sets the pin:

  | Datasource | Pin |
  |---|---|
  | `helm` | `chart:<depName>`, with `registryUrl` kept as the pin's `registry_url` |
  | `github-releases`, `github-tags` | `github:<packageName or depName>` |
  | `terraform-provider` | `tfprovider:<name>`, `hashicorp/` added to a bare name |
  | `terraform-module` | `tfmodule:<name>` |
  | `docker` | `image:<name>` |
  | `aqua` | `aqua:<name>` |
  | `npm`, `pypi` | `npm:<name>`, `pypi:<name>` |
  | other | a tool pin looked up by `packageName`, then `depName` |

- When another detector reports a pin on the same file, line and value, the annotation's pin
  replaces it, because the annotation names the dependency exactly. The value, line and file stay
  the same; the tool and the detector can change.

### Terraform modules
- `module` blocks in `*.tf` with a registry source (`<ns>/<name>/<provider>`, an optional host,
  an optional `//subdir`) and a literal `version` give a `tfmodule:<ns>/<name>/<provider>` pin of
  the new kind `tfmodule`. `registry.terraform.io/` and `registry.opentofu.org/` are dropped from
  the name, as for providers; other hosts stay. Versions follow Terraform's rules: a bare version
  is exact.
- A Terragrunt `terraform { source = "tfr:///<ns>/<name>/<provider>?version=<v>" }` gives the
  same pin, exact.
- A git source on github.com with `?ref=<ref>` (`github.com/o/r`, `git::https://github.com/o/r.git`,
  `git@github.com:o/r.git`, `git::ssh://git@github.com/o/r.git`, each with an optional `//subdir`),
  in a `module` block or a Terragrunt `terraform { source }`, gives a tool pin of
  `github:<owner>/<repo>`. Other git hosts give an unresolved pin, listed by `--unresolved`.
- A ref counts as a version only when it is `x.y.z` or `vx.y.z`. A prefixed ref (`vpc-v1.2.3`,
  `modules/vpc/v1.2.3`), a branch or a commit SHA is a `channel` pin: listed, never compared.
- `tfmodule:` resolves through the Terraform registry's module endpoint. `source` names the
  repository, and the part of `tag` before `version` is the tag prefix. Without a GitHub source
  the lookup fails like a provider's does.
- `tfmodule:` and the github: module pins take part in `outdated`, `drift`, `digest --base` and
  `--pull-request` from the start. `drift` resolves offline, so a `tfmodule:` column uses the
  semver rules.

### Helm repository URLs
- The scan stays offline. A chart pin from an annotation's `registryUrl`, a `Chart.yaml`
  dependency's `repository` or the first `registryUrls` entry of a Renovate report dependency
  (`https://`, `http://` and `oci://` only) keeps the URL in `registry_url`, and its tool stays
  `chart:<name>`.
- `outdated`, `digest --base` and `--pull-request` look the URL up on Artifact Hub before they
  resolve: for `oci://`, `<url>/<name>` first, then `<url>`. The first repository that has the
  chart wins, and the pin's tool becomes `chart:<repo>/<name>`. When none has it, or the lookup
  fails, the tool stays `chart:<name>` and resolves as before.
- `drift` makes no requests, so it groups by `chart:<name>`.

### Hugo links in changelog files
- A changelog-file source can set `site_url`, the base URL of the project's Hugo site, and
  `content_dir`, the site's content directory in the repository. Without `content_dir` it is the
  path up to the last `content/` segment, plus the next segment when it looks like a language
  code (`en`, `pt-br`).
- In each version section, a link to `{{< ref "path#frag" >}}` or `{{< relref >}}` (also with
  `{{% %}}` and `path="..."`) becomes `<site_url>/<page>/#frag`. The page path follows Hugo's
  rules above, without `.md`, `_index` or `index`. The URL is the same whether the page is
  `x.md` or `x/_index.md`. A same-page ref (`#frag`) becomes a plain anchor, which points into
  the source file.
- Without `site_url` such a link keeps its text and loses its target, so the digest has no
  broken URL.
- Lines that hold only a block shortcode such as `{{% alert title="Warning" %}}` or
  `{{% /alert %}}` are dropped, and the content between them stays.

## Consequences
- `relgap pins` finds more pins in repositories that use Renovate comments. A CI or `extra_files`
  pin under such a comment changes its tool and detector in the output.
- The pins schema gets the kind `tfmodule` and an optional `registry_url`. The registry schema
  gets `site_url` and `content_dir`.
- `outdated` asks the Terraform registry for each `tfmodule:` name and Artifact Hub for each chart
  registry URL, through `httpx` and its cache.
- Monorepo modules with prefixed tags aren't compared until pins carry a tag prefix.
- A section named like a language code directly under `content/` needs `content_dir`.
