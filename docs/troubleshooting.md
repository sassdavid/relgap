---
description: What relgap's exit codes, token errors, rate limits, cache misses and unresolved pins mean, and what to do about each.
---

# Troubleshooting

relgap logs to stderr and prints the report to stdout. Add `-v` for info logs, `-vv` for debug
logs and `-vvv` to trace every HTTP request. Trace logs redact credential headers and
cookies (`Authorization`, `X-Api-Key` and the rest), so you can paste them into an issue.

## Read the exit code first

| Code | Meaning | What to do |
|---|---|---|
| 0 | The command finished, and no `--fail-on` value matched. | Nothing. |
| 1 | Something failed: a file didn't parse, a source didn't answer, a write failed. | Read the last lines on stderr. A report with failed tools or files is still printed. |
| 2 | Wrong usage: an unknown flag, a bad value or range, an invalid config file. | The message names the flag, value or file and line. |
| 3 | A `--fail-on` value matched. The report is complete. | Read the report, or the stderr line that names the matched value and the count. |
| 4 | GitHub refused the token, or no source accepted the credentials. | See [GitHub rejected the token](#github-rejected-the-token). |
| 130 | Ctrl-C or a cancelled job. | Run again. Files under `--out` may be missing. |

When a run is partial and a `--fail-on` value matched anyway, relgap exits 3, not 1: a match is
a match. When nothing matched, a partial run exits 1, because a pass on partial data proves
nothing. [Exit codes](/reference/exit-codes) lists the codes each command can return.

## GitHub rejected the token

`github rejected the token` and exit 4 mean GitHub answered 401 to the token relgap found.
relgap takes the first token it finds in this order: `GH_TOKEN`, `GITHUB_TOKEN`, gh's
`hosts.yml`, then `gh auth token`. A stale `GITHUB_TOKEN` in your shell wins over a working gh
login. Check which one is set:

```sh
env | grep -E '^(GH|GITHUB)_TOKEN=' | cut -d= -f1
gh auth status
```

Unset the stale variable or replace it. In GitHub Actions, pass <code v-pre>${{ github.token }}</code> as
`GH_TOKEN`. A private repository needs a token that can read its contents, and
`relgap notify` needs `pull-requests: write`.

## Rate limit reached

`api.github.com rate limit reached (60 requests per hour, resets at ...)` means relgap ran
without a token. GitHub allows 60 unauthenticated requests an hour, and a digest of a tool with
many versions can use them up. Set `GH_TOKEN` or run `gh auth login` for 5,000 an hour.

With a token, relgap warns `rate limit almost used up; slowing down` when few requests are left,
and waits for GitHub's `Retry-After` on a secondary rate limit. Repeated runs answer from the
cache, and an authenticated revalidation that GitHub answers with 304 costs no request.
[Work offline and manage the cache](/guides/cache) has the cache rules.

## Not in the cache

With `--offline`, a request whose answer isn't cached fails with `not in the cache`. Run the
same command once without `--offline` to fill the cache. `relgap cache info` shows the cache
directory, its size and how many entries are fresh.

## A tool doesn't resolve

`unknown tool` means no registry knows the name. Name the repository with `github:owner/repo`
or an aqua package with `aqua:`, or map the tool in `.relgap.yaml`:

```sh
relgap resolve hk
relgap resolve github:jdx/hk
relgap resolve some-tool --guess
```

`relgap resolve` shows the repository and the layer that supplied each field.
[Map unresolved pins and private tools](/guides/unresolved) covers `aliases`,
`tools.<name>.repo` and `--guess`, and [Tool resolution](/concepts/resolution) explains the
layers.

## A pin is missing or has no tool

`relgap pins` lists every pin it found, with the file, line and detector. A pin with an empty
tool maps to no known tool: list those with `relgap pins --unresolved`. A pin that is missing
altogether is in a file no detector reads, in a path that `scan.exclude` or `.gitignore`
skips, or written with a shell variable, which detectors skip. [Pins and
detectors](/concepts/pins) lists the files each detector reads.

## The digest is empty or short

- The header says how many entries are hidden under `--min-confidence`. Run with
  `--min-confidence 0` to see them; [Classification and confidence](/concepts/classification)
  says what each level means.
- `<from> is not a known version; using the nearest lower version` means the range starts at a
  version the tool never released. Check the tag scheme with `relgap resolve`.
- A tool without a curated entry uses auto-discovery: release bodies, else a changelog file in
  the repository's top directory. When the notes live somewhere else, the digest is empty.
  [Write or fix a registry entry](/guides/registry-entry) adds a source, and
  `relgap registry test` shows what each source finds.

## outdated skips a pin

`relgap outdated` skips image and action pins, and pins of tools without a GitHub repository.
The last line of the table counts the skipped pins, and `--format json` lists each one with its
reason. A pin on a channel such as `lts` isn't compared. [Find pins behind latest or past
EOL](/guides/outdated) says how CURRENT and LATEST are chosen.

## EOL shows ? or VULNS is empty

EOL needs a curated registry entry that names an endoflife.date product
([Supported tools](/reference/tools)). `?` means endoflife.date doesn't list the pin's release
cycle. VULNS is `-` when relgap has no OSV source for the tool, such as a chart or a tool on a
private host; [EOL and vulnerabilities](/concepts/enrichment#vulnerabilities) lists the sources. When endoflife.date or OSV is down, relgap warns and prints the rest; under
`--fail-on eol` or `--fail-on vuln` the run exits 1, since the gate can't pass on missing data.

## Still stuck

Open an issue at [github.com/sassdavid/relgap/issues](https://github.com/sassdavid/relgap/issues)
with the command, the output of `relgap version` and the stderr of a run with `-vv`. Remove
repository names you don't want to share.
