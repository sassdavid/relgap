# Spec: output formats and CI mode

## Renderers
| Format | digest | outdated | drift | pins |
|---|---|---|---|---|
| `tty` | glamour-rendered sections; breaking first; per-version collapse markers; counts header | lipgloss table, colored severity | matrix table, outliers highlighted | table |
| `plain` | same structure, no ANSI | TSV-like aligned | aligned | aligned |
| `md` | PR-ready: `<details>` per version, Breaking/Security at top, source links | Markdown table | Markdown table | table |
| `json` | envelope (`schema_version`), `data.tools[]` = `ToolReport` | envelope | envelope | envelope |
| `sarif` | — | one result per finding (eol/vuln/breaking), location = pin file:line | one result per drifted pin | — |

- Markdown output stays under 60 KB by default (GitHub comment limit 65,536). When truncated, a footer says what was cut (`--md-max-bytes`).
- JSON schemas: `schemas/report-v1.json`. Every testscript that emits JSON validates against it.

## `--fail-on` (exit 3)
| Value | Matches when |
|---|---|
| `breaking` | any entry kind=breaking with confidence ≥ `--min-confidence` in a digested range |
| `deprecation` | same, kind=deprecation |
| `eol` | a current pin's cycle is EOL |
| `vuln` | a current pin has a known vuln (`--vuln-severity high` threshold optional) |
| `drift` | a tool has more than one distinct pinned version across scanned repos (`--drift-ignore-constraints`) |
| `outdated` | a pin is behind latest by more than `--max-behind` (e.g. `minor:2`, `releases:10`, `days:90`) |

## GitHub Action (M11)
`sassdavid/relgap-action`, a composite action that installs a verified release:
- `mode: comment-on-renovate`: on a Renovate or Dependabot PR, parses the PR's upgraded tools (branch name / `renovate` JSON comment / diff of pin files) and posts or updates **one** sticky comment with the `md` digest.
- `mode: report`: runs `outdated`/`drift` with SARIF upload to code scanning.

## Acceptance criteria
- [ ] Golden files for every format × command cell above.
- [ ] SARIF validates against the 2.1.0 schema and uploads in a CI dry run.
- [ ] Each `--fail-on` value has a testscript asserting exit 3 and exit 0 counterparts.
- [ ] The Markdown truncation footer appears exactly at the limit (property test on generated digests).
