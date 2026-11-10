# Spec: entry extraction and classification

Turns `VersionNotes` Markdown into `[]Entry{Kind, Text, Confidence, Source}`.

## Steps
1. **Parse** with goldmark. Code fences and HTML blocks are opaque: headings inside fences are never headings (terragrunt bodies contain `# root.hcl` in hcl fences).
2. **Segment** into sections by heading path (e.g. `v1.37.0 › Changes by Kind › Deprecation`).
3. **Itemize:** each list item (with nested children) is one entry. A paragraph-only section is one entry.
4. **Classify** each entry, taking the first rule that matches.

| Priority | Signal | Kind | Confidence |
|---|---|---|---|
| 1 | structured field (awscli JSON `type`, Adoptium `type`) | mapped | 1.0 |
| 2 | section heading matches `breaking( changes)?`, `BREAKING CHANGES:`, `UPGRADE NOTES:`, `Urgent Upgrade Notes`, `Compatibility Notes`, `⚠`, `SEMVER-MAJOR` | breaking | 0.95 |
| 2 | heading `deprecat(ed\|ion)s?` / `Removed` / `Removals` | deprecation (removed → breaking) | 0.9 |
| 2 | heading `security`, `SECURITY ADVISORIES:`, `CVE-` in heading | security | 0.95 |
| 2 | heading `other`, `misc`, `chore`, `dependency`, `documentation`, `failing test`, `API change` (kubernetes kinds that name no relgap kind) | other | 0.9 |
| 2 | heading `features?`, `enhancements?`, `improvements?`, `added`, `new` / `bug ?fixes`, `fixed`, `bug` | feature / fix | 0.9 |
| 3 | conventional-commit prefix `feat!:`, `fix(x)!:`, a `BREAKING CHANGE:` footer | breaking | 0.9 |
| 3 | conventional-commit prefix `feat:` / `fix:` / `deps:`, other known types (`chore`, `docs`, `ci`, ...), node's `(SEMVER-MINOR)` | feature / fix / other | 0.8 |
| 4 | leading verb (claude-code style `Removed …`, `Breaking change: …`, `Deprecated …`), optionally after a short scope such as `Windows: ` | mapped | 0.7 |
| 4 | under a `Notable changes` or `Highlights` heading | other | 0.6 |
| 5 | keyword anywhere (`no longer`, `removed`, `deprecated`, `must now`, `renamed`, `CVE-\d+`) | breaking / deprecation / security | 0.4 |
| — | otherwise | other | 0.3 |

Registry `breaking_markers` add tool-specific regexes at priority 2 (`ACTION REQUIRED` for kubernetes), matched
against the entry text and its headings. `SEMVER-MAJOR` (node's commit tag) is a built-in marker.
Registry `ignore_sections` drop false-positive contexts: kubernetes `Dependencies › Removed` lists removed Go
modules, not features. An item is a heading, or a path joined with ` › `; a heading matches when it equals the
item or starts with it and a space (`Downloads` matches `Downloads for v1.37.0`). Release-page boilerplate
(`Downloads`, `Installation`, `Checksums`, `Assets`, `Binaries`, `New Contributors`, `Contributors`) is always
dropped.

Pseudo-headings count as headings: a short line ending in `:` before a list, an all-caps label such as
`UPGRADE NOTES:`, and an all-bold paragraph. An all-caps label written on the line right after a bullet
(a lazy continuation in CommonMark, as in terraform's changelog) is split off and starts a new section.

Duplicate entries across a tool's sources (normalized text without link targets, issue numbers and URLs) are
dropped; the first source in the registry entry wins.

## Version-jump weighting
A range that crosses a **major** version (or a minor in `0.x`) adds a synthetic `breaking` entry,
"major version jump 1.x → 2.x: review the upgrade guide" (confidence 0.6), when no explicit breaking entry
exists for that version. Its source adapter is `version-jump` and its URL is the release page, when there is one.

## Output hygiene
- Strip semantic-release anchors and compare links, and demote headings (they're re-rendered under our own structure).
- Keep links absolute (relative links are resolved against the source URL).
- Each entry links back to its source: the release URL or the file URL with `#L<line>` anchor.

## Acceptance criteria
- [ ] Golden classification for the corpus subset. Precision on `breaking` ≥ 0.9 against a hand-labelled set of 300 entries from 15 tools (`testdata/labels/*.yaml`); recall is reported.
- [x] Kubernetes `Dependencies › Removed` is never classified breaking.
- [x] Fenced `# comment` lines never become headings (terragrunt fixture).
- [x] `--min-confidence` filtering is covered by a testscript.
