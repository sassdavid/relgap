---
description: Write SARIF from relgap outdated or relgap drift and upload it to GitHub code scanning, so each finding shows at the pin's file and line.
---

# Upload SARIF to code scanning

`relgap outdated` and `relgap drift` write SARIF 2.1.0, the format GitHub code scanning reads.
Each result points at the file and line of a pin, so the alert opens at the line to change.

## Write the SARIF

`--format sarif` prints SARIF on stdout. `--out sarif=<path>` writes it next to another format
from the same run:

```sh
relgap outdated --format sarif > relgap.sarif
relgap outdated --format github --out sarif=outdated.sarif --out md=outdated.md
relgap drift ~/src/infra --fail-on drift --min-count 3 --format sarif > drift.sarif
```

`--format github` prints GitHub Actions annotations, which show on the workflow run and in the
pull request diff. Only `outdated` and `drift` take `sarif` and `github`.

## What each command reports

`relgap outdated` writes a result for each pin behind latest (`outdated`), each pin on an
end-of-life cycle (`eol`), each upgrade with breaking changes or deprecations (`breaking`,
`deprecation`), each OSV advisory that affects a pin, and each pin or file that failed (`failed`).
Each advisory is a rule of its own, named by its OSV ID, with its CVSS score as the
`security-severity` that code scanning shows.

`relgap drift` writes a result for each drifted pin, the cells marked `*` in its matrix, and a
`failed` result for a file that doesn't parse or a tool whose latest version can't be listed.

The results don't depend on `--fail-on`: the gate decides the exit code, and the SARIF lists every
finding either way. [SARIF rules](/reference/sarif-rules) has each rule's level, description and
help text.

## Upload it

The upload step needs `security-events: write`. Give each command its own `category`, so an
outdated run doesn't close the alerts of a drift run:

```yaml
permissions:
  contents: read
  security-events: write # SARIF upload
steps:
  # checkout, mise and the relgap step as in docs/ci/outdated.yml
  - uses: github/codeql-action/upload-sarif@33d7b85da1a04171e211befe2c739303b9a4c066 # v4.39.3
    if: ${{ !cancelled() && hashFiles('outdated.sarif') != '' }}
    with:
      sarif_file: outdated.sarif
      category: relgap-outdated
```

The `if:` runs the upload even when the relgap step failed its gate. `--out` writes every file
before the exit code is set, so the SARIF is there. The complete workflow is on [Gate CI on
breaking changes, EOL and vulnerabilities](/guides/ci-gate). [relgap-action](/guides/action) in
`report` mode does the upload for you, in the category `relgap-outdated` or `relgap-drift`.

SARIF locations are relative to the path relgap scans, the current directory by default. Run
relgap from the repository root, or code scanning can't link the alert to the file.

## Alerts across runs

Code scanning matches an alert with the next run by hashing the pin's line. relgap also writes a
partial fingerprint, `relgapFinding/v1`, built from the rule, the tool, the file and what the
result is about. GitHub ignores it; other SARIF readers can match results with it.
