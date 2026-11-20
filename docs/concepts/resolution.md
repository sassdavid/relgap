---
description: How relgap turns a tool name, alias, variable or prefixed name into a repository on GitHub, Bitbucket or GitLab or an image, version rules and release-note sources, and which layer supplies each field.
---

# Tool resolution

Before relgap can list versions or read release notes, it needs to know where a tool lives and
how its tags read. Resolution turns the name you type, or the name a pin carries, into a tool
spec: the repository and its host (GitHub unless the spec says `host`), the version rules, the
release-note sources, the endoflife.date product and the OSV packages. [`relgap resolve`](/cli/resolve) prints that spec and where each
field came from.

## What you can type

| Input | Example | Resolves through |
|---|---|---|
| a name or alias | `terraform`, `tf` | the layers below |
| a `NAME_VERSION` variable | `TERRAFORM_VERSION` | the name with underscores as hyphens, `terraform` |
| `github:owner/repo` | `github:jdx/hk` | the repository itself, or the tool that has it |
| `github:owner/repo@prefix` | `github:kubernetes/autoscaler@cluster-autoscaler-` | the repository, reading only the tags with that prefix |
| `bitbucket:workspace/repo` | `bitbucket:acme-ws/tf-network` | the Bitbucket Cloud repository; its tags are the versions |
| `gitlab:group/.../repo` | `gitlab:acme/platform/tf-dns` | the gitlab.com project; its tags are the versions, its releases the notes |
| `gitlab:host/group/.../repo` | `gitlab:gitlab.acme.internal/infra/tf-vpc` | a project on a host in `gitlab.hosts` |
| `aqua:package` | `aqua:hashicorp/terraform` | the embedded aqua index |
| `tfprovider:namespace/name` | `tfprovider:integrations/github` | the registries, then the Terraform registry |
| `tfmodule:namespace/name/provider` | `tfmodule:terraform-aws-modules/vpc/aws` | the Terraform registry |
| `chart:repo/name`, `chart:name`, `chart:url/name` | `chart:argo/argo-cd` | Artifact Hub |
| `npm:package` | `npm:prettier` | the registries, then the npm registry |
| `pypi:package` | `pypi:black` | the registries, then PyPI |
| `image:name`, `image:name:<prefix>*<suffix>` | `image:nginx:*-alpine` | the image registry's tags; the source label's GitHub repository becomes the notes tool |

When a registry knows the tool, every form gives the same result as its name: `github:hashicorp/terraform`
and `TERRAFORM_VERSION` both resolve to terraform with its curated entry.

## Layers

Each field comes from the highest layer that sets it, so a lower layer fills only what the
higher ones leave empty:

1. config: `tools.<name>.repo`, `tools.<name>.host` and `aliases` in your config file.
   `tools.<name>` sets only the repository and its host, which win over every registry.
2. relgap registry: the curated entries embedded in relgap (the [supported tools](/reference/tools)),
   or the newer copies [`relgap registry update`](/cli/registry/update) downloaded.
3. mise registry: `mise registry --json` when mise is on `PATH`, otherwise the embedded snapshot
   ([registry snapshots](/reference/snapshots)). It adds aliases and the repository of the first
   `aqua:`, `github:` or `ubi:` backend.
4. aqua registry: the embedded index of the aqua registry. It adds the repository, a tag prefix,
   and the tags universe for packages whose versions come from tags.
5. default: the loose version scheme, the releases universe, no release line, and
   auto-discovery of release notes when no layer names sources.

A tool that only mise and aqua know still resolves, offline. It gets its repository and tag rules
from them, and its notes from [auto-discovery](/concepts/sources#without-sources). It has no EOL
or vulnerability data, because only a curated entry names an endoflife.date product and OSV
packages.

## Lookups outside the registries

`tfprovider:`, `npm:` and `pypi:` names are first matched against the registries, so a curated
tool resolves without the network: `tfprovider:hashicorp/aws` matches the entry whose repository
is `hashicorp/terraform-provider-aws`. A name no registry knows is looked up in the Terraform
registry, the npm registry or PyPI, which name the source repository. `chart:` names always go to
Artifact Hub. A chart takes no other tool's entry, because a chart's versions are not its
application's. `chart:name` without a repository resolves only when one Artifact Hub repository
has a chart of that name. `chart:<url>/<name>`, such as
`chart:https://charts.jetstack.io/cert-manager`, names the chart by its Helm repository URL and
resolves only through the Artifact Hub repository at that URL, or through the `gh-pages` index of
a GitHub Pages URL.

A chart's versions come from its repository's tags when a tag prefix covers the newest version the
chart lists. Some repositories tag every release once for all their charts: aws/eks-charts tags
`v0.0.252`. Then the versions come from Artifact Hub, or from the Helm repository index, and the
tool has the universe `chart`. `outdated` shows latest and behind, with `-` for the counts, and a
digest lists the versions with a warning that says why it has no notes. Pin the application's own
repository as well to read its release notes.

An `image:` name lists the tags of the image's registry, and the tool has the universe `image`.
Only tags with the name's tag pattern count: `image:nginx:*-alpine` reads `1.27.0-alpine` as
version 1.27.0 and skips `1.27.0` and `1.27.0-bookworm`. relgap reads the manifest of the newest
stable tag, then the linux/amd64 image of an index, then the image config, for the source label
(`org.opencontainers.image.source`, then `org.label-schema.vcs-url`). When it names a GitHub
repository, that repository resolves like `github:owner/repo` and becomes the tool's `notes`, and
a digest of the image is that tool's digest over the same versions. Without one, the image has
versions only, like a `chart` universe tool.

`bitbucket:` and `gitlab:` names need no lookup: they resolve to the repository on that host with
the `tags` universe.

These lookups go through the HTTP cache like every other request. [Network, privacy and
tokens](/concepts/network) lists the hosts.

## Guessing

A plain name that no layer knows is an error that suggests `github:owner/repo`, `aqua:package` or
`tools.<name>.repo`. With `--guess`, relgap searches GitHub for the name instead. The repository
whose name matches exactly and has the most stars wins, and the result is marked low-confidence
with the other candidates listed. [`relgap digest`](/cli/digest), [`relgap resolve`](/cli/resolve)
and [`relgap registry test`](/cli/registry/test) take `--guess`. A guess is a starting point:
check it with `relgap resolve some-tool --guess`, then put the repository in `tools.<name>.repo`.

## Reading the result

The JSON form shows the input, how it matched, the merged spec and the provenance of each field:

```sh
relgap resolve terraform --format json
```

<<< @/../testdata/golden/resolve/terraform.json

Here every field but `version.line` comes from the curated entry, `mise_source` says the mise
registry came from the snapshot, and `relgap_source` says the entry is the embedded one, not an
updated copy. A scan names pins through the same layers without any lookup, so `relgap pins`
stays offline; `relgap outdated` makes the Terraform registry and Artifact Hub lookups for
tfprovider, tfmodule and chart pins when it compares them. [Map unresolved pins and private tools](/guides/unresolved) shows
what to do with a pin that resolves to nothing.
