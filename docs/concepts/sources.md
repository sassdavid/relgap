---
description: Where relgap reads release notes from, how a registry entry names its sources, how notes from several sources merge, and what happens for a tool without an entry.
---

# Release-note sources

Projects publish release notes in different places: GitHub release bodies, a `CHANGELOG.md`, one
changelog file per minor line, an upgrade guide, a JSON feed or a page on their own site. A source
tells relgap where one of those is. A curated [registry entry](/reference/registry-schema#sources)
lists the sources of its tool, and `relgap resolve <tool>` shows them.

## Source kinds

<!--@include: ../parts/source-kinds.md-->

Files come from the tool's GitHub repository through the API, at the `ref` the source names or the
default branch. Web pages come only from the hosts below; `relgap registry lint` rejects a
`url_template` on any other host:

<!--@include: ../parts/web-hosts.md-->

## Templates

`path_template`, `url_template`, `ref` and `anchor` hold placeholders that relgap fills for each
version in the range:

<!--@include: ../parts/placeholders.md-->

kubectl, for example, reads `CHANGELOG/CHANGELOG-{major}.{minor}.md` at ref `master`, so a range
across three minor lines reads three files. terraform reads `CHANGELOG.md` at ref
`v{major}.{minor}`, the release branch of each line.

## How sources merge

relgap runs every source of the entry for the versions in the range and keeps one list of entries
per version:

- `upgrade-notes` runs first, so its entries win over the same text in the changelog.
- The other sources follow in the order the entry lists them.
- An entry that an earlier source already had for the same version is dropped. Two entries count
  as the same when their words match once link targets, URLs and issue numbers are removed,
  because a release body often repeats the changelog with different links.

Each changelog file and web page is fetched and parsed once per run, however many versions read
it. Every entry keeps the links of its source, and the digest links each version to its GitHub
release when it has one.

When a range starts a new major line (a new minor line in 0.x) and no source has a breaking entry
for that version, relgap adds one: "major version jump 2.x → 3.x: review the upgrade guide", at
confidence 0.6. A major release with no notes still shows up as a risk.

## Without sources

A tool without sources, which is every tool that only mise or aqua knows, gets auto-discovery,
the way Renovate and Dependabot find notes:

1. The GitHub release bodies of the range, when at least one holds real notes. A body under 400
   bytes, or one with only headings, links and checksums, is boilerplate.
2. Otherwise a changelog file in the repository's top directory, named by Renovate's changelog
   file name rule (`CHANGELOG`, `HISTORY`, `RELEASES`, `release-notes` and so on). `.md` wins over
   `.rst` over `.txt`; `.json` files and files under 100 bytes are skipped. A file whose newest
   version is older than the newest release is stale: relgap ignores it and warns.
3. Otherwise whatever release bodies there are.

A range with no notes at all warns and prints the GitHub compare link of the two versions.
[Supported tools](/reference/tools) lists the tools whose entry names their sources, and
[Write or fix a registry entry](/guides/registry-entry) shows how to add one.

## Checking a source

[`relgap registry test`](/cli/registry/test) digests a range once per source and once with all of
them, and prints the versions each source found notes for and its entry counts:

```sh
relgap registry test kubectl 1.36.0..1.37.2
```

A source without notes in a range isn't an error. An upgrade guide only has notes where a new
release line starts.
