---
description: The output formats relgap writes, which ones each command accepts, the JSON envelope every --format json report shares, and how --out writes several formats from one run.
---

# Output formats

The report goes to stdout and logs go to stderr, so `relgap outdated --format json > out.json`
captures the report only. Each [command page](/cli/) lists the formats it accepts; `--format`
with any other value exits 2.

| Format | What it is | Commands |
|---|---|---|
| `tty` | a table with colour, for a terminal; the default when stdout is a terminal | every command with a report |
| `plain` | the same table without colour; the default otherwise | every command with a report |
| `md` | Markdown for a pull request comment or a step summary | `digest`, `outdated`, `drift`, `pins` |
| `json` | the JSON envelope below | every command with a report |
| `sarif` | SARIF 2.1.0 for GitHub code scanning ([SARIF rules](/reference/sarif-rules)) | `outdated`, `drift` |
| `github` | GitHub Actions workflow commands, which show as annotations on the pull request | `outdated`, `drift` |

`--color always|never|auto` controls colour in `tty`. `auto` turns it off when `NO_COLOR` is set.

## The JSON envelope

Every `--format json` report has the same outer object, described by
[`report-v1.json`](/schemas/report-v1.json):

| Field | Holds |
|---|---|
| `schema_version` | `"1"`; a breaking change to the report shape gets a new version and a new schema |
| `tool` | `"relgap"` |
| `version` | the relgap version that wrote the report |
| `generated_at` | the time of the run, UTC, to the second |
| `data` | the command's report; its schema is on [JSON schemas](/reference/schemas) |
| `errors` | one object per target that failed, with `target` (a tool, file or source) and `message`; empty when nothing failed |
| `page` | only in MCP results that were split: `items`, `returned`, `total` and `next_cursor` |

A report with entries in `errors` is partial. The exit code says whether a `--fail-on` value
matched anyway ([exit codes](/reference/exit-codes)). The text and Markdown formats of `digest`
have no place for errors, so relgap prints each one on stderr as
`relgap: failed: <target>: <message>`.

```sh
relgap outdated --format json | jq '.data.pins[] | select(.status == "outdated")'
relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'
```

## Markdown

`--format md` writes Markdown that a pull request comment, an issue or a step summary renders.
A digest puts each version in a `<details>` block. Mentions and issue references in quoted
release notes are broken with a zero-width space, and issue links go through
`redirect.github.com`, so a comment notifies nobody. Output longer than `--md-max-chars`
(60,000 characters by default, under GitHub's comment limit of 65,536) is cut between table rows
or sections, and a footer says what was left out. `--md-max-chars 0` never cuts.

## Several formats from one run

`--out <format>=<path>` writes the report to a file in another format, and you can repeat it.
The run's own `--format` still goes to stdout. Every file is written before the exit code is
set, so a CI step that fails on exit 3 still has its files:

```sh
relgap outdated --format github --out sarif=outdated.sarif --out md=outdated.md --out json=outdated.json --fail-on eol,vuln
```

`relgap notify` reads the JSON that `--out json=` wrote and posts it as a pull request comment
([notify targets](/reference/notify)).
