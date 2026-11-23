---
description: Make a CI job fail when an upgrade has breaking changes, a pin is past its end of life or has a known vulnerability, or pins fall too far behind.
---

# Gate CI on breaking changes, EOL and vulnerabilities

`--fail-on` turns a report into a check. When a value matches, relgap still writes the report,
then exits 3. This guide shows the values each command takes, how to tune them and a complete
workflow.

## Values per command

| Command | `--fail-on` values |
|---|---|
| [`relgap outdated`](/cli/outdated) | `breaking`, `deprecation`, `eol`, `vuln`, `outdated`, `kev` |
| [`relgap audit`](/cli/audit) | `vuln` (the default), `unchecked`, `kev` |
| [`relgap digest`](/cli/digest) | `breaking`, `deprecation` |
| [`relgap drift`](/cli/drift) | `drift` |

Repeat the flag or separate values with commas:

```sh
relgap outdated --fail-on eol --fail-on vuln
relgap outdated --fail-on eol,vuln
```

A value the command doesn't check, such as `--fail-on drift` on `outdated`, exits 2.
[Fail-on values](/reference/fail-on) describes each value.

## What each value counts

In `relgap outdated`, each value counts findings:

- `outdated` counts the pins further behind latest than `--max-behind` allows.
- `breaking` and `deprecation` count the pins whose digest from CURRENT to LATEST has such an
  entry at or above `--min-confidence`.
- `eol` counts the pins whose release cycle has reached its end of life on endoflife.date.
- `vuln` counts the OSV advisories that affect a pinned version, at or above `--vuln-severity`.
- `kev` counts the advisories in the CISA Known Exploited Vulnerabilities catalog, whatever their
  rating. It needs `--kev` (config `vulns.kev`); without it relgap exits 2 before any request.

`relgap digest` counts a single digest once. With `--from-mise` or `--all` it counts each pin, and
with `--base` or `--pull-request` each digest. `relgap drift` counts the drifted pins.

A value exits 3 once its count reaches `--min-count` (default 1):

```sh
relgap outdated --fail-on outdated --min-count 5 --format sarif > relgap.sarif
```

## Allow some lag

`--fail-on outdated` alone fails on any pin behind latest. `--max-behind` sets how far behind a
pin may be:

```sh
relgap outdated --fail-on outdated --max-behind minor:2
```

| `--max-behind` | A pin passes when |
|---|---|
| `major:N` | it is at most N major versions behind |
| `minor:N` | it is at most N minor versions behind |
| `patch:N` | it is at most N patch versions behind |
| `releases:N` | at most N releases came out after it |
| `days:N` | the first release after it is at most N days old |

`days` needs publish dates. For a tool whose versions are tags without one, relgap warns and
the pin passes.

## Count only serious vulnerabilities

`--vuln-severity` sets the lowest CVSS rating `--fail-on vuln` counts: `low`, `medium`, `high` or
`critical`. An advisory whose severity is unknown counts at every threshold, since nothing says
it is below one. Without the flag, every advisory counts:

```sh
relgap outdated --fail-on eol --fail-on vuln --vuln-severity high
```

`--vuln-fixable` counts only the advisories with a fixed version above the pinned one, so the
gate fails on what an upgrade can fix:

```sh
relgap audit --vuln-fixable --vuln-severity high
```

[EOL and vulnerabilities](/concepts/enrichment) says where the data comes from, how the rating is
read and how the fix target is chosen.

## Set the policy once

The `policy` keys of the config file hold the defaults, so the workflow and a local run agree:

```yaml
# .relgap.yaml
schema_version: 1
policy:
  fail_on: [eol, vuln]
  max_behind: "minor:2"
  vuln_severity: high
```

`policy.fail_on` lists values for every command, and each command uses the values it checks, so
`relgap drift` ignores `eol` there. A `--fail-on` flag replaces the list. `policy.max_behind`,
`policy.vuln_severity`, `policy.vuln_fixable` and `policy.min_confidence` are the defaults of
`--max-behind`, `--vuln-severity`, `--vuln-fixable` and `--min-confidence`. Each key also has a `RELGAP_POLICY_*` environment
variable ([Configuration](/reference/config#policy)).

## Exit codes in CI

| Code | Meaning |
|---|---|
| 0 | Nothing matched. |
| 1 | A tool or file failed and no value matched, so the report is partial. |
| 2 | Wrong usage or an invalid config file. |
| 3 | A `--fail-on` value matched. |
| 4 | Credentials were missing or refused for every source the command needed. |

Exit 3 wins over exit 1: a match on partial data is still a match, and relgap logs that the report
is partial. A pass on partial data proves nothing, so it exits 1. That includes an endoflife.date
outage under `--fail-on eol`, an OSV outage under `--fail-on vuln` and a CISA KEV outage under
`--fail-on kev`.
[Exit codes](/reference/exit-codes) has the full table.

## A workflow

This workflow runs once a week and on pull requests that change a pin. One `relgap outdated` run
prints annotations, writes SARIF, Markdown and JSON with `--out`, and gates the job with
`--fail-on eol,vuln`. The later steps run even when the gate fails: the Markdown goes to the step
summary, the SARIF to code scanning, and `relgap notify` keeps one comment on the pull request.
Copy it to `.github/workflows/outdated.yml` in a repository whose `mise.toml` pins relgap:

<<< @/ci/outdated.yml

`--out` writes every file before the exit code is set, so the steps after the gate always find
them. [Upload SARIF to code scanning](/guides/sarif) explains the upload, and
[Use relgap-action](/guides/action) does the same without a `mise.toml` entry.
