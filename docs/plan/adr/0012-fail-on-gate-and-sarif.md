# relgap ADR-0012: The `--fail-on` gate, its exit codes and SARIF output

- Status: accepted (2026-11-12)
- Supersedes the exit-code precedence in [ADR-0009](0009-outdated-current-and-latest.md) and
  [ADR-0010](0010-drift-offline-matrix.md). The decisions are recorded in
  [output-and-ci.md](../specs/output-and-ci.md#decisions-m9).

## Context
M9 adds every `--fail-on` value, `--min-count` and SARIF output. tfcensus, which shares relgap's
exit codes, already has a gate: `scan`, `whois` and `drift` exit 3 when a `--fail-on` value
matches, even when part of the run failed, and exit 1 only when nothing matched. ADR-0009 and
ADR-0010 chose the opposite for relgap: a failed tool or file exits 1 and hides a match.

The two rules differ only when a run is partial and a value matched anyway. In relgap a failure
removes data: a tool whose versions can't be listed has no findings, a file that doesn't parse
has no pins, and an endoflife.date or OSV outage leaves pins without `eol` or `vulns`. Every count
the gate compares with `--min-count` can only shrink. A match on partial data is therefore a
match on the full data too. Only a pass is uncertain.

Semgrep's CLI reference documents the same split: by default, unparseable files don't change the
exit code, and only `--strict` gives them their own code.

## Decision
- A matched `--fail-on` value exits 3 whether or not the report is partial, as in tfcensus. A
  partial report with a match logs the reason as a warning.
- A partial report without a match exits 1. That includes an endoflife.date outage under
  `--fail-on eol` and an OSV outage under `--fail-on vuln`.
- Each value counts its own findings and matches when the count reaches `--min-count` (default
  1), as tfcensus `drift` does with its gates.
- SARIF results don't depend on `--fail-on`: the log lists every finding, and the gate decides
  only the exit code.

## Consequences
- `outdated_gate.txtar` and `drift.txtar` assert exit 3 with a warning for a matched partial
  report, and exit 1 for one without a match.
- A CI job that branches on exit 3 (for example to post a comment) runs even when a tool failed.
  The exit-1 message and the warning still name the failure.
