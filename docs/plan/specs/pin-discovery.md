# Spec: pin discovery (scanner + mise input)

Produces `[]Pin` (tool, raw name, kind, value, version, exact, match, location) from a project or
from many repo roots. Used by `pins`, `outdated` and `drift`. Code: `internal/scan`.

## Sources of pins

| Detector | Files | Extracts | Notes |
|---|---|---|---|
| `mise` | `mise.toml`, `.mise.toml`, `mise.*.toml`, `.mise.*.toml`; `config.toml`, `config.*.toml`, `conf.d/*.toml` in a `mise`, `.mise` or `_mise` directory | tool → version | always parsed with go-toml, never `mise ls` ([ADR-0007](../adr/0007-parse-mise-config-for-pins.md)). `[tools]` values may be strings, arrays or tables `{version=…}`; every array element is a pin |
| `tool-versions` | `.tool-versions` | asdf pins | |
| `aqua` | `aqua.yaml`, `aqua/*.yaml` | `packages[].name@version` | Renovate has no aqua manager, which is a differentiator |
| `idiomatic` | `.terraform-version`, `.terragrunt-version`, `.node-version`, `.nvmrc`, `.python-version`, `.go-version`, `go.mod` `toolchain`/`go` | single version | |
| `terraform` | `*.tf`, `*.tf.json` via hcl/v2 ([ADR-0008](../adr/0008-hcl-and-dockerfile-parsers.md)) | `required_version`, `required_providers` (`tfprovider:` pins), `module` blocks (`tfmodule:` pins, `github:` pins for `?ref=` sources) | constraints are recorded as `Exact=false`; a bare version is exact. A file without `required_` reports no parse error |
| `tf-lock` | `.terraform.lock.hcl` via hcl/v2 | exact provider versions | the *real* provider version in use |
| `terragrunt` | `terragrunt.hcl`, `root.hcl`, `*.hcl` | `terraform_version_constraint`, `terragrunt_version_constraint`, `terraform { source }` (`tfr://` `?version=` as `tfmodule:`, `?ref=` as `github:`) | static attributes only, no evaluation. A file without a constraint reports no parse error |
| `dockerfile` | `Dockerfile*`, `*.Dockerfile`, `Containerfile*` (own parser, [ADR-0008](../adr/0008-hcl-and-dockerfile-parsers.md)) | `FROM image:tag` (`image:` pins, ARG defaults substituted), `ARG/ENV <NAME>_VERSION=x` | `<NAME>` maps to a tool via registry aliases (`TERRAFORM_VERSION` → terraform). Honour `# renovate: datasource=… depName=…` comments above an ARG or ENV (exact mapping) |
| `ci` | `.github/workflows/*.y*ml`, `bitbucket-pipelines.yml`, `.gitlab-ci.yml` | `image:` tags, a job's `container:`, `uses: owner/action@vX`, `*_VERSION` and `*-version` keys (`go-version`, `terraform_version`) | actions are reported as kind `action` (opt-in, `scan.detectors.ci_actions`); a commit SHA with a full version in its line comment (`# v4.1.3`, `# tag=v4.1.3`) is exact at that version (M29) |
| `helm` | `Chart.yaml` | `dependencies[].version` (`chart:` pins, with an `https://` or `oci://` `repository` as `registry_url`), `appVersion` | |
| `renovate-comment` | `*.tf`, `*.tfvars`, `*.hcl`, `*.yml`, `*.yaml`, `Makefile`, `GNUmakefile`, `*.mk`, `pom.xml`, `*.sh`, `.env`, `.env.*`, `scan.extra_files` | the one version on the line after a `# renovate:`, `// renovate:` or `<!-- renovate: -->` comment | kind from the datasource ([ADR-0015](../adr/0015-renovate-annotations-and-terraform-modules.md)); replaces another detector's pin on that line; Dockerfiles stay with `dockerfile` |
| `generic` | any text file listed in config `scan.extra_files` | regex `(?i)\b([a-z0-9_]+)_VERSION\s*(?:[:?+]?=\|:)\s*["']?(v?[0-9][^\s"']*)` | opt-in (`scan.detectors.generic`); maps names via aliases. Also accepts Make's `:=`, `?=` and `+=` |
| `renovate` (input) | `--renovate-report <file.json>` | `packageFiles[].deps[]` from `renovate --platform=local --dry-run=extract --report-type=file` | optional, adds coverage for anything Renovate understands. `packageFiles` may sit at the top level or under `repositories.<name>`. Pins have line 0 |
| `tfcensus` (input) | `drift --from-tfcensus <inventory.json>` (`-` for stdin) | `terraform_version` per unit and workspace (an `opentofu` pin when `engine` is `opentofu`), `providers[].version` with `version_source: lockfile` (`tfprovider:` pins) | tfcensus `schemas/inventory-v1.json`, `schema_version` "1" only, other versions exit 2 ([ADR-0014](../adr/0014-drift-from-tfcensus.md)). A unit is a repo named `repo/path[:workspace]`. Exact pins, line 0; the state pin's file is the state key's base name |

## Walking repos
- `drift` and `pins` roots are directories. Every directory containing `.git` (a directory, or a file for worktrees and submodules) becomes a "repo" (no recursion into nested `.git`, except git submodules if `--submodules` is passed). A root with no `.git` at or below it is one repo itself.
- `.gitignore` is respected through git's own matcher (`git ls-files -co --exclude-standard`). If git is missing, fall back to a built-in ignore list (`node_modules`, `.terraform`, `.terragrunt-cache`, `vendor`, `.git`).
- Files are read in parallel (bounded). Binary and huge files are skipped (over 2 MB).

## Normalization
- `v1.2.3` → `1.2.3` for comparison. The raw value is kept for display.
- Constraints (`~> 1.15`, `>= 1.5, < 2`) are kept as constraints. Drift counts them as an exact pin they allow in the same repo, else as their lower bound, and marks them `≈` in the matrix ([ADR-0010](../adr/0010-drift-offline-matrix.md)).
- Every pin has a `match`: `exact` (1.15.8), `constraint` (~> 1.15, >= 1.5), `prefix` (26, 3.14, mise's `prefix:1.2`) or `channel` (latest, lts, corretto-17). `exact` is true only for `exact`. Tables mark every non-exact value with `≈`.
- A pin that resolves to no known tool is reported with `tool=""` and listed under "unresolved" (`pins --unresolved`), so users can add aliases. `--unresolved` keeps the same columns with RAW NAME first and prints a hint about `aliases` on stderr.

## Output
- `kind` is `tool`, `tfprovider`, `tfmodule`, `chart`, `image` or `action`. Non-tool pins carry their name as `kind:name` in `tool` (`tfprovider:hashicorp/aws`, `tfmodule:terraform-aws-modules/vpc/aws`, `image:golang`). A chart pin may have `registry_url`.
- `tool` is the name `relgap digest` accepts: a registry name, or `github:owner/repo` and `aqua:package` when no registry names the tool.
- JSON follows `schemas/pins-v1.json`: `data.repos[]` and `data.pins[]`, sorted by repo, file and line. Files that fail to parse go into the envelope's `errors`, and the command exits 1 after printing the rest.

## Renovate annotations and Terraform modules (M27)

Decided in [ADR-0015](../adr/0015-renovate-annotations-and-terraform-modules.md), which also
records the facts the decisions rest on.

Renovate's regex custom managers are often driven by a comment on the line above a version:
`# renovate: datasource=<ds> depName=<name> [packageName=<pkg>] [registryUrl=<url>]`. The
`renovate-comment` detector reads such a comment (also after `//` or inside `<!-- -->`) and the
first line after it that is neither blank nor a comment. When that line holds exactly one version,
the version is a pin. The datasource sets the kind through `renovateDepPin` in
`internal/scan/renovate.go`, which the Renovate report shares:

| Datasource | Pin |
|---|---|
| `helm` | `chart:<depName>`, with `registryUrl` kept as `registry_url` |
| `github-releases`, `github-tags` | `github:<packageName or depName>` |
| `terraform-provider` | `tfprovider:<name>` |
| `terraform-module` | `tfmodule:<name>` |
| `docker` | `image:<name>` |
| `aqua` | `aqua:<name>` |
| `npm`, `pypi` | the tool of that name, else `npm:<name>` or `pypi:<name>` |
| other | a tool pin looked up by `packageName`, then `depName` |

A comment's pin replaces another detector's pin on the same line with the same value.

On `github-releases` and `github-tags`, `extractVersion=^<prefix>(?<version>...)$` names the tool
`github:<owner>/<repo>@<prefix>` (a trailing `v?` is allowed, and `v` or an empty prefix adds
nothing). Any other `extractVersion` leaves the pin unresolved. `versioning` is ignored
([ADR-0017](../adr/0017-chart-versions-extract-version-and-floors.md)).

`module "x" { source = "<ns>/<name>/<provider>" version = "..." }` and a Terragrunt
`terraform { source = "tfr:///<ns>/<name>/<provider>?version=<v>" }` give a `tfmodule:` pin. It
resolves through `registry.terraform.io/v1/modules/<ns>/<name>`, whose entry for the provider has
`source` (the repository), `version` and `tag`; the part of `tag` before `version` is the tag
prefix. A git source on github.com with `?ref=<ref>` gives a `github:<owner>/<repo>` pin, whose
raw name is the source without `ref`. Other git hosts give an unresolved pin.

A chart pin with a `registry_url` (from the comment or a `Chart.yaml` dependency's `repository`)
is renamed `chart:<repo>/<name>` by `outdated`, `digest --base` and `--pull-request` when
Artifact Hub has a repository at that URL with the chart. Otherwise it becomes
`chart:<url>/<name>`, which never falls back to a search by name
([ADR-0016](../adr/0016-tool-identity-and-chart-registry-urls.md)). `drift` makes no request and
groups by `chart:<name>`.

## Acceptance criteria
- [x] Every detector above has a testscript fixture under `testdata/script/pins_*.txtar` with golden JSON.
- [x] Fixture: a dotfriedrice-like `mise.toml` (array values, `http:`/`github:`/`aqua:` backends, `[tools."http:mongodb-database-tools"]` tables) parses identically with and without the mise binary on PATH.
- [x] Dockerfile `ARG TERRAFORM_VERSION=1.15.8` and `# renovate: datasource=github-releases depName=gruntwork-io/terragrunt` + `ARG TG_VERSION=1.2.1` both map to the right tools.
- [x] `.terraform.lock.hcl` produces `tfprovider:hashicorp/aws` pins with exact versions; `required_providers` produces constraint pins.
- [x] `relgap pins` over 60 repos with no network takes under 5 s (benchmark test with a generated tree).
- [x] Unknown `*_VERSION` names appear in `--unresolved` with file:line.
- [x] `drift --from-tfcensus` over a copy of the tfcensus `tf-fleet` inventory golden gives a row per
  unit and workspace with exact `terraform`, `opentofu` and `tfprovider:` pins, alone or next to
  scanned roots and from a file or stdin. Another `tool` or `schema_version` exits 2.
- [x] A `# renovate:` comment (also `//` and `<!-- -->`) above a line with one version makes it a
  pin in the files ADR-0015 lists, with the kind its datasource gives and a helm `registryUrl`
  kept; it replaces another detector's pin on that line. An `extractVersion` with a literal prefix
  names `github:<owner>/<repo>@<prefix>`, and any other form leaves the pin unresolved.
  (`TestSpec_pindiscovery_8`, fixture `pins_renovate_comment.txtar`, `TestExtractPrefix`)
- [x] `module` blocks with a registry source and a `version`, and Terragrunt `tfr://` sources,
  give `tfmodule:` pins that resolve through the Terraform registry with the latest tag's prefix;
  GitHub git sources with `?ref=` give `github:` pins. (`TestSpec_pindiscovery_9`,
  `TestRemoteModule`, fixtures `pins_terraform.txtar` and `pins_terragrunt.txtar`)
- [x] `outdated`, `drift`, `digest --base` and `--pull-request` work on a fixture repository with
  both kinds of pins, and a chart's `registryUrl` picks its Artifact Hub repository, so the
  ambiguous `external-dns` chart resolves. (`TestSpec_pindiscovery_10`, `TestRemoteChartRepo`)
