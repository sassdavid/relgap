---
description: Find the pins relgap can't map to a tool, map them with aliases, add private tools with tools.<name>.repo, and check the result with relgap resolve.
---

# Map unresolved pins and private tools

A pin is unresolved when its name maps to no tool that relgap knows. Unresolved pins get no
latest version, no digest and no drift column. This guide finds them and maps them.
[Tool resolution](/concepts/resolution) explains the layers relgap looks names up in.

## Find unresolved pins

`relgap pins`, `relgap outdated` and `relgap drift` print a count on stderr when some pins map
to no tool. `--unresolved` lists only
those pins, with the raw name, file and line:

```sh
relgap pins --unresolved
```

Common causes are a variable with a custom name, such as `TG_VERSION` in a CI workflow, an
internal tool, and a tool that neither mise's nor aqua's registry lists.

## Map a raw name with aliases

`aliases` maps a raw pin name to a tool relgap knows:

```yaml
# .relgap.yaml
aliases:
  TG_VERSION: terragrunt
  KUBE_VERSION: kubectl
```

The tool on the right must resolve. An alias that points to an unknown tool is an error that
names the alias. A variable named after its tool needs no alias: `TERRAFORM_VERSION` resolves as
`terraform`, and `GOLANGCI_LINT_VERSION` as `golangci-lint`.

## Add a private or unlisted tool

`tools.<name>.repo` adds a tool by its GitHub repository. `repo` is the only field:

```yaml
# .relgap.yaml
tools:
  internal-cli:
    repo: acme/internal-cli
```

The config layer wins over every registry for the repository, so this also moves a known tool
to a fork. Versions come from the repository's releases, and release notes from auto-discovery:
the release bodies when they hold more than boilerplate, else a changelog file in the top
directory. A private repository needs a GitHub token that can read its contents
([Network, privacy and tokens](/concepts/network#the-github-token)).

A pin named `INTERNAL_CLI_VERSION` now resolves as `internal-cli` without an alias. To get EOL
data, vulnerabilities or curated release-note sources, the tool needs a registry entry:
[Write or fix a registry entry](/guides/registry-entry).

## Name a tool on the command line

`relgap digest`, `relgap resolve` and `relgap registry test` take a tool name, an alias, a
`NAME_VERSION` variable or one of these prefixes:

| Input | Resolves through |
|---|---|
| `github:owner/repo` | the repository itself |
| `aqua:package` | the embedded aqua registry index |
| `tfprovider:namespace/name` | the Terraform registry, which names the source repository |
| `tfmodule:namespace/name/provider` | the Terraform registry, which names the source repository and the tag of the latest version |
| `chart:repo/name` or `chart:name` | Artifact Hub; `chart:name` only when one repository has a chart of that name |
| `npm:package` | the npm registry |
| `pypi:package` | PyPI |

When a registry already knows the tool, the result is the same as for its name. The lookups for
`tfprovider:`, `tfmodule:`, `chart:`, `npm:` and `pypi:` go to the hosts that
[Network, privacy and tokens](/concepts/network#hosts) lists, and their answers are cached for a
day.

## Guess from a GitHub search

`--guess` searches GitHub for a name that no layer knows. The repository whose name matches
exactly and has the most stars wins, and relgap marks the result as low confidence:

```sh
relgap resolve some-tool --guess
```

`digest` and `registry test` take `--guess` as well. A guess can pick the wrong repository, so
check it and then pin the answer with `tools.<name>.repo`.

## Check the result

`relgap resolve` prints the repository, version rules and sources for a tool, with the layer that
supplied each field:

```sh
relgap resolve internal-cli
relgap resolve TG_VERSION
```

Run `relgap pins --unresolved` again: a mapped pin no longer shows. To leave a pin out of the
reports instead of mapping it, add an `ignore` rule ([Configuration](/reference/config#ignore)).
An unresolved pin has no tool, so match its raw name: `name` is a glob on the name that
`relgap pins --unresolved` shows, and its `*` spans slashes. A rule with only `path` drops every
pin under that glob. Ignored pins drop out of the "no known tool" count that `outdated`, `drift`
and `pins` print, and `relgap pins` still lists them.

```yaml
ignore:
  - name: "git::*bitbucket.org*"   # module sources on Bitbucket
  - path: "examples/**"            # every pin in the examples
```
