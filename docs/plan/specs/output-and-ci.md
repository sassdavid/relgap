# Spec: output formats and CI mode

## Renderers
| Format | digest | outdated | drift | pins |
|---|---|---|---|---|
| `tty` | glamour-rendered sections; breaking first; per-version collapse markers; counts header | aligned table with ANSI colors: latest yellow when behind, green when current, breaking counts red | matrix table (a row per repo, a column per drifting tool), outliers yellow and marked `*` | table |
| `plain` | same structure, no ANSI | TSV-like aligned | aligned | aligned |
| `md` | PR-ready: `<details>` per version, Breaking/Security at top, source links | Markdown table and a notes list | Markdown table | table |
| `json` | envelope (`schema_version`), `data.tools[]` = `ToolReport` | envelope, `data.pins[]` (`schemas/outdated-v1.json`) | envelope, `data.tools[]` (`schemas/drift-v1.json`) | envelope |
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
- [x] `outdated` over a fixture project with 10 pins matches golden files in tty, plain, md and json, and each pin's counts equal those of the full digest `current..latest` (`TestSpec_output_and_ci_5`).
- [x] `drift` over a generated tree of 60 repos matches golden files in tty, plain, md and json, marks constraint pins `≈`, sends no request (a refusing proxy and an empty request log), and `--fail-on drift` exits 3 (`TestSpec_output_and_ci_6`).
- [x] `drift --latest` over the same tree adds the latest versions of 8 tools from 1 GraphQL request, and a second run answers from the cache with 0 requests (`TestSpec_output_and_ci_7`).
