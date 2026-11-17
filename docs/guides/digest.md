---
description: Merge the release notes between two versions of a tool into one digest, filter it by kind and confidence, and digest every pin a project or a change upgrades.
---

# Read the changes between two versions

`relgap digest` lists the versions after the version you have, up to and including the version
you want, fetches their release notes and groups the entries by kind. This guide covers the
range syntax, the filters, the output formats and the modes that digest a whole project.

## Pick a range

A digest takes a tool and a range. `<from>..<to>` covers every version after `<from>` up to and
including `<to>`:

```sh
relgap digest terraform 1.15.8..1.17.1
```

The tool is any name `relgap resolve` understands: a name or alias (`terraform`, `tf`),
`github:owner/repo`, `aqua:package`, `tfprovider:`, `chart:`, `npm:` or `pypi:`.
[Tool resolution](/concepts/resolution) explains the lookup. Versions can carry the tag's
prefix or not: `1.15.8` and `v1.15.8` are the same version.

Leave out `<to>` and the range ends at the newest stable version on the release line:

```sh
relgap digest helm 3.19.0.. --line major
```

`--line` takes `none`, `major` or `minor`. `major` stays on the major version of `<from>`, so the
command above stops at the newest 3.x release even after 4.0 is out. Without the flag, the
release line comes from the tool's registry entry (`version.line`), and `none` when the entry
sets none. With both ends given, a range that leaves the line is an error.

These rules decide which versions are in the range:

- Versions are ordered by the tool's version scheme, never by publish date.
- A `<from>` that isn't a known version starts after the nearest lower version, with a warning.
- A `<to>` that isn't a known version, a backwards range and a range that leaves `--line` exit 2.
- Prereleases are left out unless you pass `--include-prereleases` or one end of the range is a
  prerelease. The default end of a range is never a prerelease.

[Versions, ranges and release lines](/concepts/ranges) has the schemes and the difference between
a tool whose versions are its releases and one whose versions are its tags.

## Read the output

The first lines name the repository, the versions in the range and the count of entries per
kind. Entries are grouped in this order: breaking changes, deprecations, security fixes,
features, fixes, other. Each entry links to the release note or changelog section it came from.
A bullet that two sources share is shown once. [Release-note sources](/concepts/sources) lists
where relgap reads notes.

Each entry has a confidence between 0 and 1. Entries below `--min-confidence` (default 0.5) are
hidden and counted on the first lines. Lower the threshold to see every entry:

```sh
relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
```

Set the default for every command with `policy.min_confidence` in the
[config file](/reference/config#policy-min-confidence). [Classification and
confidence](/concepts/classification) says how relgap picks a kind and a confidence.

## List only some kinds

`--only` lists the entries of the kinds you name. The counts still cover every kind:

```sh
relgap digest argo-cd 3.2.0..3.4.1 --only breaking --only deprecation
```

The kinds are `breaking`, `deprecation`, `security`, `feature`, `fix` and `other`.

## Choose a format

Without `--format`, relgap prints colors on a terminal and plain text otherwise.

`--format md` writes Markdown for a pull request or an issue, with one collapsible block per
version:

```sh
relgap digest kubectl 1.36.0..1.37.2 --format md > digest.md
```

The Markdown quotes release notes without notifying anyone: relgap puts a zero-width space after
each `@` and `#` that would mention a user or reference an issue, and sends github.com issue links
through redirect.github.com. Output longer than `--md-max-chars` (default 60,000 characters) is
cut at a row or section boundary.

`--format json` writes the JSON envelope. Each entry has an `id`, `version`, `kind`,
`confidence`, `text` and `source`:

```sh
relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'
```

`--out` writes more formats from the same run, such as `--out json=digest.json` next to
Markdown on stdout. [Output formats](/reference/output) lists them.

## Digest every outdated pin of a project

`--from-mise` digests each tool pinned in the project's mise config files that is behind its
latest version, from the pinned version to the latest. `--all` does the same for every pin that
`relgap outdated` compares:

```sh
relgap digest --from-mise
relgap digest --all ~/src/infra --format md > upgrade-notes.md
```

Both take the project path as their only argument and use the ranges that
[`relgap outdated`](/guides/outdated) shows. Ignore rules in the config drop pins first.

## Digest what a change upgrades

`--base` compares the pins of a git ref with the working tree and digests each tool whose exact
version changed, from the base version to the new one:

```sh
relgap digest --base origin/main --format md
```

`--head <ref>` compares with another ref instead of the working tree. Added, removed and
constraint pins are listed on stderr and get no digest. In a GitHub Actions pull request
workflow, `--pull-request` reads the base commit from the event; see
[Comment upgrade notes on Renovate pull requests](/guides/renovate-pr).

## Fail a job on a breaking change

`--fail-on breaking` exits 3 when a digest has a breaking entry at or above `--min-confidence`,
and `--fail-on deprecation` when it has a deprecation:

```sh
relgap digest --from-mise --fail-on breaking
```

[Gate CI on breaking changes, EOL and vulnerabilities](/guides/ci-gate) covers the gate and its
exit codes. [Add an LLM summary](/guides/summarize) adds a cited summary on top of a digest.
