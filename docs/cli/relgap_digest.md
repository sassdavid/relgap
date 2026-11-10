## relgap digest

Merge the release notes of every version in a range into one digest

### Synopsis

Lists the versions after <from> up to and including <to>, fetches their release notes and
groups the entries by kind: breaking changes, deprecations, security, features, fixes, other.

<tool> resolves as in 'relgap resolve'. Without <to> the range ends at the latest stable
version on the release line (--line).

GitHub token: GH_TOKEN, GITHUB_TOKEN, gh's config file, then 'gh auth token'. Without one,
relgap uses the REST API (60 requests per hour) with 2 concurrent requests.

```
relgap digest <tool> <from>..[<to>] [flags]
```

### Examples

```
  relgap digest terraform 1.15.8..1.17.1
  relgap digest helm 3.19.0.. --line major
  relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'
```

### Options

```
  -h, --help[=true|false]                  help for digest
      --include-prereleases[=true|false]   include prereleases (on when <from> or <to> is a prerelease)
      --line string                        release line the range stays on: none, major or minor (default "none")
```

### Options inherited from parent commands

```
      --color string            colorize output: auto, always or never (default "auto")
      --concurrency int         maximum concurrent requests per tool (config github.concurrency) (default 8)
      --config string           config file to use instead of the discovered .relgap.yaml (env RELGAP_CONFIG)
      --format string           output format: tty, plain, md, json or sarif (default tty when stdout is a terminal, else plain)
      --log-file string         write logs to this file instead of stderr
      --log-format string       log format: text or json (default "text")
      --min-confidence float    hide entries below this confidence, 0..1 (config policy.min_confidence) (default 0.5)
      --no-input[=true|false]   never prompt for input
  -q, --quiet[=true|false]      log errors only
  -v, --verbose count           more logging: -v info, -vv debug, -vvv trace
```

### SEE ALSO

* [relgap](relgap.md)	 - Release-notes digests and toolchain drift for the tools you pin

