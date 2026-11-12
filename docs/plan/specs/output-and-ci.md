# Spec: output formats and CI mode

## Renderers
| Format | digest | outdated | drift | pins |
|---|---|---|---|---|
| `tty` | glamour-rendered sections; breaking first; per-version collapse markers; counts header | aligned table with ANSI colors: latest yellow when behind, green when current, breaking counts red | matrix table (a row per repo, a column per drifting tool), outliers yellow and marked `*` | table |
| `plain` | same structure, no ANSI | TSV-like aligned | aligned | aligned |
| `md` | PR-ready: `<details>` per version, Breaking/Security at top, source links | Markdown table and a notes list | Markdown table | table |
| `json` | envelope (`schema_version`), `data.tools[]` = `ToolReport` | envelope, `data.pins[]` (`schemas/outdated-v1.json`) | envelope, `data.tools[]` (`schemas/drift-v1.json`) | envelope |
| `sarif` | - | one result per finding (outdated/eol/vuln/breaking/deprecation/failed), location = pin file:line | one result per drifted pin, plus failed | - |

- Markdown output stays under 60 KB by default (GitHub comment limit 65,536). When truncated, a footer says what was cut (`--md-max-bytes`).
- JSON schemas: `schemas/report-v1.json`. Every testscript that emits JSON validates against it.

## `--fail-on` (exit 3)
| Value | Counts | Checked by |
|---|---|---|
| `breaking` | pins (a single digest counts once) whose digested range has an entry kind=breaking with confidence ≥ `--min-confidence` | `outdated`, `digest` |
| `deprecation` | same, kind=deprecation | `outdated`, `digest` |
| `eol` | pins whose current cycle is EOL | `outdated` |
| `vuln` | advisories that affect a current pin, at or above `--vuln-severity` (`low`, `medium`, `high`, `critical`; empty counts all) | `outdated` |
| `drift` | drifted pins: compared pins whose version differs from the tool's reference version (`--drift-ignore-constraints`) | `drift` |
| `outdated` | pins behind latest by more than `--max-behind` (e.g. `minor:2`, `releases:10`, `days:90`) | `outdated` |

A value matches when its count reaches `--min-count` (default 1). A value given as a flag to a
command that doesn't check it exits 2; the same value from `policy.fail_on` is left to the
command that checks it.

## GitHub Action (M11)
`sassdavid/relgap-action`, a composite action that installs a verified release:
- `mode: comment-on-renovate`: on a Renovate or Dependabot PR, parses the PR's upgraded tools (branch name / `renovate` JSON comment / diff of pin files) and posts or updates **one** sticky comment with the `md` digest.
- `mode: report`: runs `outdated`/`drift` with SARIF upload to code scanning.

## Decisions (M9)
Agreed before coding; [ADR-0012](../adr/0012-fail-on-gate-and-sarif.md) has the reasoning for
the exit codes.

- The gate is tfcensus's: each value counts its findings and matches at `--min-count` (tfcensus
  `drift`). A match exits 3 even on a partial report, with a warning. A partial report without a
  match exits 1, including an endoflife.date outage under `--fail-on eol` and an OSV outage
  under `--fail-on vuln`. Usage errors exit 2, auth errors 4.
- The values keep the order of the table and the config schema enum: breaking, deprecation,
  eol, vuln, drift, outdated. Help text, error messages and SARIF rules follow it. A matched
  value's error names its count and findings: `--fail-on vuln matched 2 advisories: GO-2025-3521
  in kubectl 1.34.13 (mise.toml:2), ...`.
- `--vuln-severity` and `policy.vuln_severity` set the lowest CVSS rating `--fail-on vuln`
  counts. `--min-count` is a flag only, as in tfcensus.
- SARIF comes from tfcensus's `internal/sarif`, now shared by both repos with an optional
  `region` (start line), `fullDescription` and `FileURI`.
- Rules: `breaking`, `deprecation`, `eol`, `outdated` (outdated) or `drift` (drift), `failed`,
  then one rule per OSV ID, sorted, tagged `security` and `vulnerability`, with
  `security-severity` set to the CVSS score when the advisory has one. One rule per advisory
  keeps two advisories on the same pin from merging into one GitHub alert.
- Levels are fixed per rule: `eol` error; a vulnerability error for high and critical, warning for
  medium, note for low or no rating; `outdated` and `drift` warning; `breaking` and
  `deprecation` note; `failed` error.
- Results don't depend on `--fail-on`. `outdated` writes one per pin behind latest, one per pin
  and condition for eol, breaking and deprecation, one per pin and advisory, and a failed result
  per pin whose tool failed and per file that doesn't parse. `drift` writes one per drifted pin
  (the `*` cells) and the same failed results, plus one without a location per tool whose latest
  version can't be listed.
- Locations are the pin's file, relative to the scan root and slash-separated, with the pin's
  line as `region.startLine` (no region for line 0). An absolute root becomes a `file:///` URI
  (`file:///C:/src/repo/mise.toml` on Windows).
- Fingerprints: relgap writes `partialFingerprints["relgapFinding/v1"]` =
  `rule|tool|uri|subject` (subject: version, range, cycle or OSV ID). GitHub reads only
  `primaryLocationLineHash`, which upload-sarif computes from the pin's line; when a tool sets
  it, upload-sarif keeps it and warns on every result (`src/fingerprints.ts`). relgap doesn't
  write `runs[].automationDetails`, since upload-sarif lets it override the `category` input.
- Tests validate SARIF with `internal/testutil/sarifcheck` (shared), which embeds
  `src/sarif-schema-2.1.0.json` from github/codeql-action v4.39.3 (MIT). One fix: region's
  `anyOf` moves next to its properties, as in the OASIS original, since a draft 2020-12 compiler
  rejects it where codeql-action has it.
- CI dry run: the `sarif` job in `ci.yml` (shared) runs `github/codeql-action/upload-sarif` on
  each SARIF golden file with `CODEQL_ACTION_TEST_MODE=true`, which validates the file, adds
  fingerprints and saves the payload instead of uploading it. That variable is internal to the
  action, not documented.

## Acceptance criteria
- [x] Golden files for every format × command cell above. (`TestSpec_output_and_ci_1` checks
  that each cell has a golden file a script compares; `spec_output_and_ci_1.txtar` writes digest
  tty and plain and pins in all four formats, `spec_output_and_ci_5` and `_6` the outdated and
  drift SARIF.)
- [x] SARIF validates against the 2.1.0 schema and uploads in a CI dry run.
  (`TestSpec_output_and_ci_2`: outdated with EOL, advisories and a failed tool, and drift with a
  broken file, validated with `sarifschema`; every `testdata/golden/*/*.sarif` also validates and
  has rule indexes, fingerprints and relative or file URIs. The `sarif` job in `ci.yml` runs
  upload-sarif in test mode on each golden.)
- [x] Each `--fail-on` value has a testscript asserting exit 3 and exit 0 counterparts.
  (`TestSpec_output_and_ci_3` checks that `spec_output_and_ci_3.txtar` has both for all six
  values, then runs it: kubectl 1.34.13 and 1.37.2 for outdated, digest ranges of kubectl and
  helm, gh 2.103.3 for vuln, and two small trees for drift, plus `--min-count`,
  `--vuln-severity`, `--max-behind` and the config keys.)
- [ ] The Markdown truncation footer appears exactly at the limit (property test on generated digests).
- [x] `outdated` over a fixture project with 10 pins matches golden files in tty, plain, md and json, and each pin's counts equal those of the full digest `current..latest` (`TestSpec_output_and_ci_5`).
- [x] `drift` over a generated tree of 60 repos matches golden files in tty, plain, md and json, marks constraint pins `≈`, sends no request (a refusing proxy and an empty request log), and `--fail-on drift` exits 3 (`TestSpec_output_and_ci_6`).
- [x] `drift --latest` over the same tree adds the latest versions of 10 tools (8 until M8 added the node and terraform-provider-aws entries) from 1 GraphQL request, and a second run answers from the cache with 0 requests (`TestSpec_output_and_ci_7`).
