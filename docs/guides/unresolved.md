---
description: Find the pins relgap can't map to a tool, map them with aliases, add private tools with tools.<name>.repo, read Terraform modules on Bitbucket and GitLab, and check the result with relgap resolve.
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
internal tool, a tool that neither mise's nor aqua's registry lists, and a git source on a
self-managed GitLab host that `gitlab.hosts` doesn't list.

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

`tools.<name>.repo` adds a tool by its repository. `host` says where the repository is:
github.com (the default), bitbucket.org, gitlab.com or a host in `gitlab.hosts`:

```yaml
# .relgap.yaml
tools:
  internal-cli:
    repo: acme/internal-cli
  network-module:
    repo: acme-ws/terraform-network
    host: bitbucket.org
```

The config layer wins over every registry for the repository, so this also moves a known tool
to a fork. Versions come from the repository's releases, and release notes from auto-discovery:
the release bodies when they hold more than boilerplate, else a changelog file in the top
directory. A private repository needs a token that can read its contents
([Network, privacy and tokens](/concepts/network#tokens)). A tool on Bitbucket or GitLab takes
its versions from the tags, because Bitbucket Cloud has no releases.

A pin named `INTERNAL_CLI_VERSION` now resolves as `internal-cli` without an alias. To get EOL
data, vulnerabilities or curated release-note sources, the tool needs a registry entry:
[Write or fix a registry entry](/guides/registry-entry).

## Terraform modules on Bitbucket and GitLab

A git module source with `?ref=` pins the repository it names. On bitbucket.org it becomes a
`bitbucket:` pin, and on gitlab.com a `gitlab:` pin, in the https, `git::`, `ssh://` and
`git@host:` forms. A `//subdir` and `.git` are dropped:

```hcl
module "network" {
  source = "git::git@bitbucket.org:acme-ws/terraform-network.git//.?ref=2.1.0"  # bitbucket:acme-ws/terraform-network
}

module "dns" {
  source = "git::https://gitlab.com/acme/platform/tf-dns.git?ref=v1.2.0"  # gitlab:acme/platform/tf-dns
}
```

A self-managed GitLab host can't be told from its name, so list it. Its sources then become
`gitlab:<host>/<path>` pins. A source on a host that isn't listed stays unresolved, and relgap
sends it no request:

```yaml
# .relgap.yaml
gitlab:
  hosts:
    - host: gitlab.acme.internal
      token_env: ACME_GITLAB_TOKEN   # optional; see the tokens section of the network page
```

A `# renovate:` comment with `datasource=bitbucket-tags`, `gitlab-tags`, `gitlab-releases` or
`git-tags` gives the same pins; `registryUrl` names a self-managed GitLab host.

A GitLab tool's notes come from its releases, then from a changelog file in the top directory.
A Bitbucket tool's notes come from the changelog file only. Set `BITBUCKET_API_TOKEN` or
`GITLAB_TOKEN` for a private repository; without a token, Bitbucket answers 404 for it.

## Name a tool on the command line

`relgap digest`, `relgap resolve` and `relgap registry test` take a tool name, an alias, a
`NAME_VERSION` variable or one of these prefixes:

| Input | Resolves through |
|---|---|
| `github:owner/repo` | the repository itself |
| `bitbucket:workspace/repo` | the Bitbucket Cloud repository itself |
| `gitlab:group/repo` or `gitlab:host/group/repo` | the GitLab project itself, with nested groups; the host must be in `gitlab.hosts` |
| `aqua:package` | the embedded aqua registry index |
| `tfprovider:namespace/name` | the Terraform registry, which names the source repository |
| `tfmodule:namespace/name/provider` | the Terraform registry, which names the source repository and the tag of the latest version |
| `chart:repo/name` or `chart:name` | Artifact Hub; `chart:name` only when one repository has a chart of that name |
| `npm:package` | the npm registry |
| `pypi:package` | PyPI |
| `image:name` or `image:name:<prefix>*<suffix>` | the image registry; the tag pattern keeps a variant such as `-alpine` |

When a registry already knows the tool, the result is the same as for its name. The lookups for
`tfprovider:`, `tfmodule:`, `chart:`, `npm:`, `pypi:` and `image:` go to the hosts that
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
  - name: "git::*git.acme.internal*"   # module sources on a host relgap doesn't read
  - path: "examples/**"            # every pin in the examples
```
