# relgap ADR-0021: Several tag prefixes per tool, from aqua's overrides and filters

- Status: accepted (2026-11-24). The owner chose the full fix as its own pull request.
- Specs: [versions-and-ranges.md](../specs/versions-and-ranges.md),
  [tool-resolution.md](../specs/tool-resolution.md) and
  [output-and-ci.md](../specs/output-and-ci.md).

## Context
`relgap outdated` in this repository showed `lychee 0.24.2` with latest 0.15.1 and
`oxlint 1.94.0` with latest 0.1.1, both current. A tool had one tag prefix, and the aqua index
kept only a package's top-level `version_prefix` and `version_filter`. aqua writes a prefix
switch in `version_overrides[].version_prefix`, which relgap didn't read:

- lychee tagged `v0.15.1` and older, then `lychee-v0.16.0` and later. Its library tags
  `lychee-lib-v*` in the same repository, and the filter `not (Version startsWith "lychee-lib-")`
  drops them.
- oxlint in oxc-project/oxc tagged `oxlint_v` up to 1.16, `apps_v` from 1.28 to 1.84, and
  `oxlint_v` again from 1.85. The filter `Version matches "^apps_v" || Version matches "^oxlint_v"`
  keeps out the repository's `crates_v`, `oxfmt_v` and stray `v0.0.x`/`v0.1.x` tags.
- gping (`gping-` and plain) and biome (`@biomejs/biome@`, `cli/v` and plain) switch the same
  way. 21 of the 2,351 packages in the 2026-11-09 snapshot list more than their top-level prefix.

relgap took the newest tag that parsed under the one prefix, which was an old version, and called
every newer pin current because `Compare(current, latest) >= 0`.

mise reads these fields: `package_version_candidates` and `versioned_package_from_tag` in
`src/backend/aqua.rs` try the package's prefix and each override's prefix, and apply the filter.

## Decision
- `version.Rules.TagPrefixes` is a list. A tag loses the longest prefix it starts with, then
  parses. A tag with none of the prefixes is dropped, and an unprefixed tag counts only when the
  list holds `""`, so the atlascli strays stay out (versions-and-ranges criterion 5). Versions
  that compare equal under two prefixes are kept once, as before.
- The registry key `tag_prefix` takes a string or a list, newest prefix first. A string keeps
  its meaning, and JSON and YAML write one prefix as a string, so existing entries and the
  `resolve` output of one-prefix tools don't change.
- The aqua index gets `version_prefixes` when a package's prefixes differ from its top-level
  prefix alone: the top-level prefix when the package's own `version_constraint` isn't `"false"`,
  then each override's prefix from the last override to the first, where an override without
  one inherits the top-level prefix (`""` when there is none). aqua tries overrides in order and
  the last one usually covers the newest versions, so this order puts the newest prefix first.
- relgap translates three `version_filter` forms into rules on the whole tag:
  `not (Version startsWith "x")` becomes `exclude: ^x`, and `Version startsWith "x"` or
  `Version matches "re"` terms joined by `||` become an include expression. Other forms stay
  ignored, as all filters were before. A single `startsWith` on a package without a prefix
  still becomes the prefix. `include` is a new field of the version rules; only aqua sets it, so
  the registry schema doesn't have it.
- aqua's prefixes, include and exclude apply as a unit: a higher layer that sets the tag prefix
  (a relgap entry, a lookup, `github:owner/repo@prefix`) keeps aqua's filter out, because the
  filter was written for aqua's prefixes.
- `outdated` no longer calls a pin current only because it is newer than the latest version. When
  no tag has the pin's version, in the recent releases or the full list, the status is unknown
  with the note "0.24.2 is newer than the latest version found (0.15.1) and has no tag; check the
  tag prefix". A pin with a tag above the latest stable version, such as a prerelease, stays
  current. Unknown pins don't count as compared in the summary line.

## Alternatives rejected
- Evaluating aqua's expr-lang filters and constraints: it needs an expression engine as a new
  dependency, for a handful of forms that cover the packages that switch prefix.
- Picking one prefix per version from the override constraints (`semver("< 1.85.0")`): the
  constraints are written against aqua's own versions and can name full tags, and a longest-match
  prefix list gives the same versions for the packages above.
- Treating every tag that parses as a version when the prefix finds none: that brings back the
  stray tags criterion 5 keeps out.

## Consequences
- lychee and oxlint compare with lychee-v0.24.2 and oxlint_v1.95.0 (`spec_output_and_ci_12.txtar`).
  linkerd2 now reads `edge-` and `stable-` tags as one list, as aqua's overrides name both; a
  registry entry can narrow it if that mixes its lines badly.
- An OSV GIT query for a version from before a switch uses the newest prefix
  (`enrich.Request.tag`), so it can name a tag that doesn't exist. OSV then returns nothing for
  it, as it did before.
- Known limitation, not fixed here: mise registry entries can set `version_prefix` in a
  backend's options (martin `martin-v`, flyway `flyway-`, azure `azure-cli-`, and 7 more in the
  2026-11-09 snapshot), and relgap ignores it. `mise registry --json` doesn't print backend
  options, so reading them from the embedded snapshot alone would make results depend on whether
  mise is installed (tool-resolution criterion 2). Such a tool needs a relgap entry with
  `tag_prefix` until mise exposes the options.
