---
description: How relgap turns a tool name, alias, variable or prefixed name into a GitHub repository, version rules and release-note sources, and which layer supplies each field.
---

# Tool resolution

Before relgap can list versions or read release notes, it needs to know where a tool lives and
how its tags read. Resolution turns the name you type, or the name a pin carries, into a tool
spec: the GitHub repository, the version rules, the release-note sources, the endoflife.date
product and the OSV packages. [`relgap resolve`](/cli/resolve) prints that spec and where each
field came from.

## What you can type

| Input | Example | Resolves through |
|---|---|---|
| a name or alias | `terraform`, `tf` | the layers below |
| a `NAME_VERSION` variable | `TERRAFORM_VERSION` | the name with underscores as hyphens, `terraform` |
| `github:owner/repo` | `github:jdx/hk` | the repository itself, or the tool that has it |
| `aqua:package` | `aqua:hashicorp/terraform` | the embedded aqua index |
| `tfprovider:namespace/name` | `tfprovider:integrations/github` | the registries, then the Terraform registry |
| `tfmodule:namespace/name/provider` | `tfmodule:terraform-aws-modules/vpc/aws` | the Terraform registry |
| `chart:repo/name`, `chart:name` | `chart:argo/argo-cd` | Artifact Hub |
| `npm:package` | `npm:prettier` | the registries, then the npm registry |
| `pypi:package` | `pypi:black` | the registries, then PyPI |

When a registry knows the tool, every form gives the same result as its name: `github:hashicorp/terraform`
and `TERRAFORM_VERSION` both resolve to terraform with its curated entry.

## Layers

Each field comes from the highest layer that sets it, so a lower layer fills only what the
higher ones leave empty:

1. config: `tools.<name>.repo` and `aliases` in your config file. `tools.<name>` sets only the
   repository, which wins over every registry.
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
has a chart of that name.

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
