---
description: Compare every pinned tool version in a project with the latest release, read the breaking, EOL and vulnerability columns, and check installed Go binaries.
---

# Find pins behind latest or past EOL

`relgap outdated` scans a project the way `relgap pins` does and compares each pin with the
newest stable version on the tool's release line. For each pin it counts the releases in between,
the breaking changes and deprecations in their notes, whether the pinned release cycle has reached
its end of life, and the known vulnerabilities.

## Run it

```sh
relgap outdated
relgap outdated ~/src/infra --format md
```

The path defaults to the current directory. On the sample project from
[Getting started](/getting-started), the Markdown output is:

```sh
relgap outdated --format md
```

<<< @/../testdata/golden/outdated/project.md

## CURRENT and LATEST

CURRENT is the version a pin stands for. An exact pin is itself. A value marked ≈ allows more
than one version, and the notes under the table say which version relgap used:

- A constraint such as `~> 1.15` takes the highest exact pin of the tool in the same repository
  that satisfies it, such as a provider version in `.terraform.lock.hcl`. Without one, it takes
  the lowest version that satisfies it.
- A prefix such as `26` or `1.25` takes the highest stable version that starts with it, which is
  what mise installs.
- The `latest` channel is the latest version. Other channels, such as `lts`, aren't compared.

LATEST is the newest stable version on the release line that the tool's registry entry sets, or
the newest stable version when it sets none. [Pins and detectors](/concepts/pins) lists the files
relgap reads, and [Versions, ranges and release lines](/concepts/ranges) the version rules.

## The columns

BEHIND counts the releases after CURRENT up to LATEST. BREAKING and DEPRECATION count the entries
of those kinds in the digest of CURRENT..LATEST, the same digest that
`relgap digest <tool> <current>..` prints. Entries below `--min-confidence` aren't counted.

EOL says whether CURRENT's release cycle has reached its end on endoflife.date: yes with the date,
no, or ? when endoflife.date doesn't list the cycle. VULNS counts the OSV advisories that affect
CURRENT, and in parentheses how many of them upgrading to LATEST fixes. A dash in EOL means the
tool's registry entry names no endoflife.date product. A `?` in VULNS means relgap didn't ask OSV
about the pin, and the list under "Not checked for vulnerabilities" says why
([Unchecked pins](/concepts/enrichment#unchecked-pins)). `0` means OSV has no advisories for it.
[EOL and vulnerabilities](/concepts/enrichment) has the details, and
[Supported tools](/reference/tools) shows which tools have that data.

## Skipped pins

`tfprovider:` pins are looked up in the Terraform registry and `chart:` pins in Artifact Hub,
which name the source repository. `bitbucket:` and `gitlab:` pins read the tags of their
repository ([Terraform modules on Bitbucket and GitLab](/guides/unresolved#terraform-modules-on-bitbucket-and-gitlab)).

Image pins compare with the tags of their registry: Docker Hub, ghcr.io, registry.k8s.io,
quay.io, public.ecr.aws, mcr.microsoft.com and the hosts in `images.registries`. Only tags with
the pin's variant count, so `nginx:1.27.0-alpine` is `image:nginx:*-alpine` and moves to a newer
`-alpine` tag. When the image's source label (`org.opencontainers.image.source`) names a GitHub
repository, the counts come from that repository's notes; otherwise the row has versions only
and a note says why.

GitHub Action pins (`scan.detectors.ci_actions`) compare with the action's releases. A pin by
commit SHA stands for the version its line comment names, such as `# v4.1.3`.

Tools without a repository relgap can read, and images on another or a private registry, are
skipped. The last line counts them, they appear in the list of pins not checked for
vulnerabilities, and `--format json` lists each with the reason in `note`:

```sh
relgap outdated --format json | jq '.data.pins[] | select(.status == "skipped") | {tool, note}'
```

A pin whose name maps to no tool shows up in `relgap pins --unresolved`;
[Map unresolved pins and private tools](/guides/unresolved) fixes it.

## Check installed Go binaries

`--binaries` also reads the Go build info of each pinned tool's installed executables, on `PATH`
and in mise's installs directory. It lists the OSV vulnerabilities of the Go version and the main
module each one was built with:

```sh
relgap outdated --binaries --format json | jq '.data.binaries[]'
```

It works like govulncheck's binary mode, without the check that the binary calls the vulnerable
code. The executable names come from the registry entry's `bin` list, or the tool name.

## Read the notes behind the numbers

`relgap digest --all` digests every pin that is behind, with the same ranges:

```sh
relgap digest --all ~/src/infra --format md > upgrade-notes.md
```

To fail a CI job on these findings, see [Gate CI on breaking changes, EOL and
vulnerabilities](/guides/ci-gate). [Audit pinned versions for vulnerabilities](/guides/audit)
covers the vulnerabilities alone, over many repositories. Ignore rules in the config (`ignore`)
drop pins you don't want reported, as the [configuration reference](/reference/config#ignore)
shows.
