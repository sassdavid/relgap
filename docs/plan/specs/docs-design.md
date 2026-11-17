# Spec: docs site design (relgap)

The visual identity of `https://relgap.sasskovacs.dev`. tfcensus has its own identity
(isotype census) and shares only the build and the checks ([docs-site](docs-site.md)). Chosen by
the owner on 2026-11-17: own identity per tool, the "release strata" concept for relgap, and a
wordmark with a small glyph.

## Brief

- Subject: relgap shows what changed between the version you pin and the version you move to.
  It merges and classifies release notes across the range (breaking, deprecation, security,
  feature, fix, other), finds the same tool pinned at different versions across repos, and adds
  EOL and vulnerability status.
- Audience: engineers who review dependency upgrades, often Renovate pull requests, and platform
  teams who keep many repos on the same tool versions. They read release notes for a living and
  want the breaking changes first.
- Primary job of the site: show in one picture what relgap answers, get a reader to a first
  `relgap digest`, then serve as the reference for ranges, sources, registry entries and flags.

## Concept: release strata

Every release between your pin and latest is a layer of sediment you have to dig through to
upgrade. relgap reads those layers for you. The site draws them that way:

- A vertical stack with the pinned version at the bottom and latest at the top. Each release is
  one layer, its length set by its entry count and its segments coloured by entry class.
- A breaking change is a fault line: a dashed crimson rule across the whole stack at that
  release, because everything above it is on the other side of the break.
- The same class colours and marks label entries in prose, tables and example output.

The stack is the only illustration. Everything else is type, rules and space.

## Colour

Cool white paper with plum-black ink, and plum for the brand. The class colours are semantic and
appear only where they mean their class.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--rg-paper` | `#F7F7FA` | `#17131C` | page background |
| `--rg-surface` | `#FFFFFF` | `#211B28` | code blocks, tables, the hero stack field |
| `--rg-ink` | `#221B2B` | `#ECE6F0` | body text, headings |
| `--rg-ink-muted` | `#5E5568` | `#A99FB4` | secondary text, captions |
| `--rg-rule` | `#D6D0DC` | `#3A3142` | hairlines, table borders (decorative only) |
| `--rg-control` | `#7A7088` | `#8C8299` | input and button borders (3:1 floor) |
| `--rg-brand` | `#7A2E63` | `#E59BCB` | links, active nav, focus ring, primary button |
| `--rg-breaking` | `#C0334A` | `#FF8A9A` | class: breaking; the fault line |
| `--rg-deprecation` | `#66661A` | `#CFCB6A` | class: deprecation |
| `--rg-security` | `#975E00` | `#F2B54B` | class: security; EOL and vulnerability warnings |
| `--rg-feature` | `#2F6FB5` | `#8AB8F0` | class: feature |
| `--rg-fix` | `#2B7A5F` | `#6CC7A2` | class: fix |
| `--rg-other` | `#6F6878` | `#A79FB0` | class: other |

Contrast measured on 2026-11-17 (WCAG 2.x formula). Every text-capable token is at least 4.5:1 on
both `paper` and `surface` in its theme. The lowest light values: `feature` 4.84 on paper, `fix`
4.85, `other` 5.00, `security` 5.01, `breaking` 5.15, `deprecation` 5.64. Dark values are all
above 6.5. `control` is 4.36 on light paper, above the 3:1 floor for borders. `rule` is
decorative only. The CI contrast check ([docs-site](docs-site.md#checks-shared)) keeps these
numbers true.

Changes from the owner's preview: `breaking` went from `#C8384F` to `#C0334A`, `security` from
`#B87300` to `#975E00` and `fix` from `#2E7D62` to `#2B7A5F`, each to reach 4.5:1 as text.
`deprecation` was added because relgap has six classes, not four.

`--vp-c-brand-1..3`, `--vp-c-bg`, `--vp-c-bg-soft`, `--vp-c-text-1..2` and `--vp-c-divider` map
to these tokens in `tokens.css`.

## Entry marks

Colour is never the only cue (WCAG 1.4.1). Each class has a colour and a fill pattern in the
stack, and a text label always sits next to an inline mark.

| Class | Segment in the stack | Inline mark |
|---|---|---|
| breaking | solid, plus the fault line across the stack | short bar with a break in the middle |
| deprecation | diagonal hatching | bar with hatching |
| security | dotted fill | bar with dots |
| feature | solid | solid bar |
| fix | solid at a lighter tint | light bar |
| other | outline only | outlined bar |

Marks are inline SVG at 1em with `aria-hidden="true"`; the label carries the meaning.

## Type

- Atkinson Hyperlegible Next (variable, `@fontsource-variable/atkinson-hyperlegible-next`, OFL):
  UI, headings and body. The Braille Institute designed it so similar letters (l, I, 1; O, 0)
  stay distinct, which matters on a site full of version strings and tool names.
- Atkinson Hyperlegible Mono (variable, `@fontsource-variable/atkinson-hyperlegible-mono`, OFL):
  code, commands, version ranges and transcripts.
- Version numbers in prose and tables use the mono face so `1.10.0` and `1.1.0` line up and read
  apart.

Scale (rem, base 16 px, ratio 1.2 for the denser reading pages): body 1.0625 / line height 1.6,
small 0.875, h4 1.125, h3 1.35, h2 1.6, h1 2.1, home headline 2.8 on wide screens and 2.1 on
phones. Weights: body 400, strong 650, headings 700, wordmark 800. Headings in sentence case. No
all-caps labels and no tracked-out eyebrows. Prose measure at most 70ch.

## Layout

Doc, reference and CLI pages keep VitePress's three columns. The identity shows in colour, type,
the marks and the home page.

Home, wide screen. Left-aligned. The stack is the one bold element.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ ▤ relgap     Guide  Reference  CLI   v0.4.0   GitHub          [search]   │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  See everything between       ┌ kubectl 1.30.0 to 1.33.0 ────────────┐   │
│  your pin and latest.         │ latest 1.33.0 ▓▓▓▓▓▓▒▒░░░  14        │   │
│                               │        1.32.0 ▓▓▓▒▒▒░░     9         │   │
│  relgap merges the release    │ ─ ─ ─ ─ ─ ─ ─ breaking in 1.32.0 ─ ─ │   │
│  notes of every version in    │        1.31.0 ▓▓▒░░░░      7         │   │
│  the range, puts breaking     │ pinned 1.30.0                        │   │
│  changes first, and adds EOL  │ breaking  deprecation  security ...  │   │
│  and vulnerability status.    └──────────────────────────────────────┘   │
│                                                                          │
│  $ mise use github:sassdavid/relgap        [copy]                        │
│  [Get started]  [Install]                                                │
│                                                                          │
│  What it answers                                                         │
│  What breaks if I upgrade terraform to latest?   relgap digest           │
│  Which pins in this repo are behind or EOL?      relgap outdated         │
│  Which repos pin different versions of a tool?   relgap drift            │
│  What should this Renovate PR's review say?      relgap digest --pull-request │
│                                                                          │
│  footer: Apache-2.0, source, releases                                    │
└──────────────────────────────────────────────────────────────────────────┘
```

- The numbers in the wireframe are placeholders.
- The stack reads a committed fixture (`docs/.vitepress/theme/data/home-strata.json`, written by
  `scripts/gensite` from `testdata/golden/digest/kubectl.json`), so the layers are a real relgap
  output. The caption names the tool and range. (Changed in M22 from `kubectl-breaking.json`,
  which holds only breaking and deprecation entries, so only 1.37.0 would have had a layer.)
- "What it answers" is a list of rows, question then command then link. No card grid and no
  numbers, because the rows are not a sequence.
- The getting-started page is a sequence (install, token, first digest, scan a repo), so its
  steps are numbered.

Phone (about 400 px): the stack moves under the headline and install command, the layers keep
their order, the legend wraps, and the rows stack.

CLI reference pages follow the same layout as tfcensus's (flag names in the mono face at
0.875rem, hairline rows, no zebra striping).

## Components

| Component | Where | Notes |
|---|---|---|
| `HomeHero` | home | headline, lead, install command with copy button, two buttons, `StrataStack` |
| `StrataStack` | home, digest guide, classification concept | props: layers from JSON, caption. SVG with a text list of releases and counts for screen readers |
| `EntryMark` | anywhere, `<EntryMark kind="breaking" />` | mark plus label |
| `VersionRange` | guides and reference | renders `1.30.0..1.33.0` with the excluded start shown muted, matching the range rule (A excluded, B included) |
| `AnswerRows` | home | question, command, link |
| `Transcript` | guides | static output included from goldens. No typing animation |
| `SentToModel` | the `--summarize` guide | a bordered note in `--rg-security` listing what leaves the machine |
| `SiteFooter` | `layout-bottom` | license, source, releases, sasskovacs.dev |

## Glyph and wordmark

- Glyph: two horizontal strata, the lower one shorter, with a gap between them and a short
  crimson fault mark in the gap. Drawn on a 16 px grid, with a monochrome variant for
  `mask-icon`.
- Wordmark: "relgap" in Atkinson Hyperlegible Next 800, lowercase.
- Files: `docs/public/glyph.svg`, `favicon.svg`, `favicon-32.png`, `apple-touch-icon.png`,
  `site.webmanifest` with `theme_color` set to `--rg-brand` (light).

## Social cards

1200 by 630, generated by `social.mjs` with resvg and the bundled Atkinson Hyperlegible Next file
(its OFL text kept as `docs/.vitepress/fonts/AtkinsonHyperlegibleNext.LICENSE`). Paper
background, glyph and wordmark top left, the page title at 64 px weight 700, the section name
under it in `ink-muted`, and three thin strata along the right edge in class colours.

## Code blocks

Shiki themes `github-light` and `github-dark`, with the block background set to `--rg-surface`.
The contrast check covers comment colour on that background. Copy button on, line numbers off.

## Motion

One moment: on the first home page load the layers settle from the bottom up, about 700 ms in
total, and the fault line draws last. `prefers-reduced-motion: reduce` shows the finished stack.
Nothing else animates on its own. Hover states change colour only.

## Accessibility

- WCAG 2.2 AA throughout. The focus ring is 2 px `--rg-brand` with a 2 px offset, switching to
  `--rg-ink` on brand fills.
- The stack has a visually hidden list with each release, its counts per class and the breaking
  release named.
- Tables scroll inside their own container. Targets are at least 24 by 24 px.
- The theme follows the system setting, and the VitePress switch overrides it.

## Review against generic defaults

Checked on 2026-11-17 against the defaults a generated docs site falls into.

- Card grid of features with icons: replaced by `AnswerRows`.
- A purple SaaS accent: the brand is a deep plum chosen to stand apart from the six class
  colours, and it never marks a class.
- Gradients and glows behind the hero: none. The stack is flat colour with patterns.
- Monospace for small labels: mono is used for code and version strings only, where character
  shapes matter.
- Inter: replaced by Atkinson Hyperlegible, chosen for telling similar characters apart.
- Numbered markers: only on the getting-started steps.

## Acceptance criteria

1. `tokens.css` defines every token above for both themes, and the contrast check passes.
2. The home page matches the wireframe at 1280 px and 400 px, with the stack read from the
   fixture.
3. Entry marks render with their patterns and labels in the digest guide and the classification
   concept page.
4. Glyph, favicons and the manifest are in `docs/public`, and the social cards follow the layout.
5. axe reports no violation, and the reduced-motion check shows the finished stack.
