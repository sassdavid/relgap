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
| 2 | heading `features?`, `enhancements?`, `added`, `new` / `bug ?fixes`, `fixed` | feature / fix | 0.9 |
| 3 | conventional-commit prefix `feat!:`, `fix(x)!:`, a `BREAKING CHANGE:` footer | breaking | 0.9 |
| 3 | conventional-commit prefix `feat:` / `fix:` / `deps:` | feature / fix / other | 0.8 |
| 4 | leading verb (claude-code style `Removed …`, `Breaking change: …`, `Deprecated …`) | mapped | 0.7 |
| 5 | keyword anywhere (`no longer`, `removed`, `deprecated`, `must now`, `renamed`, `CVE-\d+`) | breaking / deprecation / security | 0.4 |
| — | otherwise | other | 0.3 |

Registry `breaking_markers` add tool-specific regexes at priority 2 (`ACTION REQUIRED` for kubernetes).
Registry `ignore_sections` drop false-positive contexts: kubernetes `Dependencies › Removed` lists removed Go
modules, not features.

## Version-jump weighting
A range that crosses a **major** version (or a minor in `0.x`) adds a synthetic `breaking` entry,
"major version jump 1.x → 2.x: review the upgrade guide" (confidence 0.6), when no explicit breaking entry
exists for that version.

## Output hygiene
- Strip semantic-release anchors and compare links, and demote headings (they're re-rendered under our own structure).
- Keep links absolute (relative links are resolved against the source URL).
- Each entry links back to its source: the release URL or the file URL with `#L<line>` anchor.

## Acceptance criteria
- [ ] Golden classification for the corpus subset. Precision on `breaking` ≥ 0.9 against a hand-labelled set of 300 entries from 15 tools (`testdata/labels/*.yaml`); recall is reported.
- [ ] Kubernetes `Dependencies › Removed` is never classified breaking.
- [ ] Fenced `# comment` lines never become headings (terragrunt fixture).
- [ ] `--min-confidence` filtering is covered by a testscript.
