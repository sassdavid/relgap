## relgap notify

Publish saved digest, outdated and drift reports as a GitHub pull request comment

### Synopsis

notify reads the JSON that digest, outdated and drift print with --format json and publishes it.
It never collects anything itself.

  github-pr-comment  one comment per report kind on a GitHub pull request, edited in place. The
                     repository comes from GITHUB_REPOSITORY and the pull request from the
                     GITHUB_EVENT_PATH event, as GitHub Actions sets them

The comment starts with a hidden marker that names the report kind and a hash of the comment, so
a re-run edits its own comment and writes nothing when the hash is the same. A report with
nothing to report turns an existing comment into "Nothing to report" and creates none. Markdown
longer than --md-max-chars is cut at a row or section boundary.

A digest has something to report when it covers a tool, an outdated report when a pin is
outdated, end of life, vulnerable or failed, and a drift report when a tool drifts or failed.

The token comes from GH_TOKEN, GITHUB_TOKEN or gh, as for digest. It needs pull-requests: write.
--dry-run-notify reads the comments with GET requests and prints what would change.

Exit codes: 0 when every comment was published, 4 when GitHub refused the token, 2 when the
repository or the pull request is unknown, and 1 for any other failure.

```
relgap notify [flags]
```

### Examples

```
  relgap outdated --format json > outdated.json
  relgap notify --from outdated.json --notify github-pr-comment
  relgap notify --from digest.json --from drift.json --notify github-pr-comment --dry-run-notify
```

### Options

```
      --dry-run-notify[=true|false]   print what would change; read with GET only and write nothing
      --from stringArray              digest, outdated or drift JSON file (repeatable)
  -h, --help[=true|false]             help for notify
      --md-max-chars int              cut the comment to this many characters at a row or section boundary (default 60000)
      --notify strings                notifiers to run: github-pr-comment
```

### Options inherited from parent commands

```
      --color string            colorize output: auto, always or never (default "auto")
      --concurrency int         maximum concurrent requests per tool (config github.concurrency) (default 8)
      --config string           config file to use instead of the discovered .relgap.yaml (env RELGAP_CONFIG)
      --format string           output format: tty, plain, md, json, sarif or github (default tty when stdout is a terminal, else plain)
      --log-file string         write logs to this file instead of stderr
      --log-format string       log format: text or json (default "text")
      --min-confidence float    hide entries below this confidence, 0..1 (config policy.min_confidence) (default 0.5)
      --no-cache[=true|false]   neither read nor write the HTTP cache
      --no-input[=true|false]   never prompt for input
      --offline[=true|false]    answer from the HTTP cache only and fail on a miss
  -q, --quiet[=true|false]      log errors only
      --refresh[=true|false]    revalidate every cached response, even fresh ones
  -v, --verbose count           more logging: -v info, -vv debug, -vvv trace
```

### SEE ALSO

* [relgap](relgap.md)	 - Release-notes digests and toolchain drift for the tools you pin

