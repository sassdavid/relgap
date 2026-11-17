---
description: How relgap splits release notes into entries, gives each a kind and a confidence, hides weak guesses, and gives every entry a stable ID.
---

# Classification and confidence

Release notes arrive as Markdown in many styles. relgap splits them into entries and sorts each
entry into a kind, so a digest can put breaking changes first and `--fail-on breaking` can count
them. Every kind comes with a confidence that says how strong the evidence was.

## Entries

Each list item is one entry, and so is each run of other blocks, such as a paragraph under a
heading. An upgrade guide gives one entry per heading instead, because its sections are written
as a whole. Relative links are made absolute, so an entry still works outside its document.

Entries under a heading that never holds notes are dropped: Downloads, Installation, Checksums,
Assets, Binaries, Contributors and the like. A registry source adds its own with
`ignore_sections`. A heading matches an item when it equals it or starts with it and a space, so
`Downloads` also drops "Downloads for v1.37.0", and `A › B` matches only B under A. kubectl's
entry drops its Dependencies section this way.

## Kinds

There are six kinds, in the order a digest lists them:

| Kind | Meaning |
|---|---|
| `breaking` | you must change something to upgrade |
| `deprecation` | something still works but will go away |
| `security` | a security fix |
| `feature` | something new |
| `fix` | a bug fix |
| `other` | everything else, including maintenance and dependency bumps |

`relgap digest --only breaking --only deprecation` lists only those kinds. The counts at the top
still cover every kind.

## Confidence

relgap reads the evidence below and takes the first rule that matches, from the top:

<!--@include: ../parts/confidence.md-->

The kind comes from the same evidence: a heading named "Deprecations" makes deprecations, a
`fix:` commit makes a fix. Headings are tried deepest first, so a "Bug fixes" heading inside a
"Breaking changes" section decides for its own items. Headings carry the most weight because the
project chose them. A keyword such as "removed" anywhere in the text is the weakest signal: it
finds real breaking changes in notes without structure, and it finds false ones too.

`breaking_markers` in a registry entry adds text that makes an entry breaking at 0.95. kubectl
marks "ACTION REQUIRED" this way, and `SEMVER-MAJOR` always counts. When a range starts a new
major line and no source says anything breaking about that version, relgap adds a version-jump
entry at 0.6 ([Release-note sources](/concepts/sources#how-sources-merge)).

## Hiding weak guesses

`--min-confidence` (default 0.5, config `policy.min_confidence`) hides entries below it: keyword
matches and entries no rule matched. Hidden entries aren't lost. The digest counts them, and
`--min-confidence 0` shows them all:

```sh
relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
```

The threshold applies everywhere an entry counts: the kind counts, the BREAKING and DEPRECATION
columns of `relgap outdated`, the `breaking` and `deprecation` [fail-on values](/reference/fail-on)
and the [SARIF results](/reference/sarif-rules). Raise it to 0.7 or 0.9 for a gate that should
only trip on headings and markers.

## Entry IDs

Every entry has an ID such as `e-3f9a01c2`: `e-` and 8 hex digits of a SHA-256 over the tool, the
version and the entry's normalized text (lowercased words, without link targets and issue
numbers). The same entry gets the same ID in every page, every filter and every run, because IDs
are set before `--only` and `--min-confidence` filter anything. Two entries with the same text in
one version get `-2`, `-3` and so on.

IDs let an agent or a summary cite an entry that you can find again. The MCP prompt
`upgrade_review` asks the agent to cite the ID of every entry it uses
([Use relgap from an MCP client](/guides/mcp)), and `--summarize` drops every claim that cites no
ID it sent ([Add an LLM summary](/guides/summarize)).
