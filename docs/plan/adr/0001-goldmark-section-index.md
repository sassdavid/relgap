# relgap ADR-0001: Parse each changelog once into a version-keyed section index (goldmark)

- Status: accepted (2026-11-09)

## Context
- Renovate re-parses a large CHANGELOG for **every** version. One user reported 38 of 59 minutes spent on aws-sdk-rust (discussion #47333).
- Changelogs reach 1.2–1.5 MB (claude-code, awscli).
- Headings inside code fences must not count (terragrunt bodies contain hcl `# root.hcl`).

## Decision
- `internal/mdindex` parses a document once with **goldmark** (CommonMark, AST with source segments), then builds `map[versionKey]Section{HeadingPath, StartByte, EndByte}`.
- Version headings are detected like Renovate does: pick the heading level that has ≥ 2 headings containing parseable versions, strip `[]()` link syntax, and match with registry `heading` regexes when given.
- Fallbacks:
  - setext headings and "version line" patterns (Dependabot's pruner) for plain-text changelogs;
  - `.rst` is converted to Markdown headings with a tiny underline-based converter for awscli `CHANGELOG.rst`, used only when `.changes/*.json` is unavailable.
- The index is cached on disk by content hash (ETag-revalidated source → reuse index).

## goldmark v1 or v2 (checked 2026-11-09 on GitHub releases)
- **Release history:** v2 betas from 2026-07-02, **v2.0.0 stable on 2026-08-27**, v2.1.0 on 2026-09-13, and v2.1.15 (2026-11-02) as GitHub's "Latest" release. All v2 releases are marked stable, not prerelease.
- **What v2 offers:** every node holds its start position, a more semantic AST, generics, and a parser cleanly separated from the renderer. Those are exactly the properties mdindex needs.
- **The README caveat:** "v2 is still in the early stages of release. If you are using an extension that does not support v2, please use v1." It's about **third-party extensions** not yet ported. relgap uses only core CommonMark (+ the built-in GFM bits), so the caveat doesn't apply.
- **Adoption** is still small (≈180 go.mod files on v2 vs ≈18.8k on v1, GitHub code search). v1.8.7 is still maintained in parallel.
- **Decision: use goldmark v2.** mdindex hides the parser behind its own interface anyway, so a switch would be cheap if ever needed.

## Consequences
- O(file) per digest instead of O(file × versions).
- Byte offsets enable precise `#L<line>` source links.
- goldmark becomes a core dependency. It's well maintained and used by Hugo.
