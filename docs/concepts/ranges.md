---
description: How relgap reads tags as versions, orders them, picks the versions in a range, and keeps the end of a range on a release line.
---

# Versions, ranges and release lines

A digest covers the releases between two versions. To know which releases those are, relgap
needs the tool's list of versions in the right order, and a rule for where the range ends. Both
come from the `version` block of the tool's [registry entry](/reference/registry-schema#version),
or from the defaults.

## From tags to versions

relgap lists the tool's GitHub releases, or its git tags when the entry sets `universe: tags`.
Releases are the default, because a release is what a project publishes; some projects, such as
kubectl and python, only tag. Draft releases are dropped.

Each tag then goes through the entry's rules:

- `exclude` is a regular expression; a matching tag is ignored (kubectl drops its alpha, beta and
  rc tags this way).
- `tag_prefix` is cut from the front of the tag (`v`, `go`, `jdk-`). A tag without the prefix is
  ignored, which keeps the tags of other components in a monorepo out. A list of prefixes covers
  a tool whose tags changed prefix (oxlint tagged `oxlint_v`, then `apps_v`, then `oxlint_v`
  again): the longest prefix a tag starts with is cut, and `""` in the list keeps unprefixed tags.
  Tools from the aqua registry get the prefixes of its `version_overrides`.
- the scheme parses what is left. A tag that doesn't parse is ignored.

Two tags that the scheme reads as the same version, such as `3.7.12` and `v3.7.12`, are kept once.
The versions are sorted by the scheme, never by publish date: a patch release of an older line
published last week still sorts below the newest major.

## Schemes

| Scheme | Reads | Prereleases |
|---|---|---|
| `semver` | semantic versions, with or without a leading `v` | `-rc.1` and other semver prerelease parts |
| `loose` | any number of numeric segments (`2.35.3.1`), Go's `go1.20` and `go1.21rc2`, build metadata (`21.0.9+10`, ignored when comparing) | a suffix such as `rc2` |
| `calver` | numeric segments compared as integers (`2026.11.5`, `2026.1`) | a `-suffix` |
| `python` | a subset of PEP 440: release segments, `a`, `b` and `rc` prereleases, `.postN` and `.devN`; epochs and local versions are rejected | `a`, `b`, `rc` and `.dev` |
| `regex` | the capture groups of `extract` joined with dots, then read as `loose`; without groups, the whole match | as `loose` |

`loose` is the default for tools without a curated entry. A GitHub release marked as a
prerelease counts as one whatever its tag says.

## The range

`relgap digest kubectl 1.36.0..1.37.2` covers every version after 1.36.0 up to and including
1.37.2. The start is excluded because you already run it; its notes describe what you have.

- From may be a version relgap doesn't list. The range then starts after the nearest lower
  version, and the digest warns. A from older than every listed version warns too.
- To must be a listed version. A to older than from is an error that tells you to swap them.
- You can type either end with or without the tag prefix: `1.30.0`, `v1.30.0` or the full tag.

Prereleases stay out of a range unless from or to is a prerelease itself, or you pass
`--include-prereleases`. A prerelease never becomes the end of an open range.

## Open ranges and release lines

`relgap digest helm 3.19.0..` has no end. It runs to the newest stable version on the release
line of from:

| Line | The range ends at |
|---|---|
| `none` | the newest stable version, wherever it is |
| `major` | the newest stable version with the same major as from |
| `minor` | the newest stable version with the same major and minor as from |

`--line` picks the line for one digest. Without the flag, the entry's `version.line` applies, and
its default is `none`. helm and awscli set `major`, so an open range of helm 3 stays on helm 3
until you pass `--line none`. With a line set and both ends given, a range that leaves the line
is an error.

[`relgap outdated`](/cli/outdated) and `relgap drift --latest` use the entry's line the same way:
LATEST is the newest stable version on the pin's line, so a pin of helm 3 is compared with the
newest helm 3.

## Pins that allow more than one version

A digest needs a from version, so a pin marked ≈ in [`relgap pins`](/concepts/pins) has to stand
for one version first. [`relgap outdated`](/cli/outdated) reads it this way:

- a constraint (`~> 1.15`) is the highest exact pin of the tool in the same repository that
  satisfies it, otherwise the lowest version that does;
- a prefix (`26`, `prefix:1.2`) is the highest stable version that starts with it, which is what
  mise installs;
- the `latest` channel is the latest version; other channels, such as `lts`, aren't compared.

Constraints use `=`, `!=`, `>`, `>=`, `<`, `<=` and `~>`, comma-separated. `~> 1.15` allows 1.x
from 1.15 on, and `~> 1.15.3` allows 1.15.x from 1.15.3 on. [Drift](/concepts/drift) reads
constraints with no version list at all, so there a constraint without a matching exact pin
counts as its lower bound.
