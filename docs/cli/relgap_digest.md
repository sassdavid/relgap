## relgap digest

Merge the release notes of every version in a range into one digest

### Synopsis

Lists the versions after <from> up to and including <to>, fetches their release notes and
groups the entries by kind: breaking changes, deprecations, security, features, fixes, other.

<tool> resolves as in 'relgap resolve'. Without <to> the range ends at the latest stable
version on the release line (--line).

Notes come from the tool's registry sources: GitHub release bodies and changelog files, each
file fetched and parsed once. A bullet that two sources share is shown once. Each entry has a
confidence between 0 and 1; entries below --min-confidence (default 0.5) are hidden and counted.
--format md writes PR-ready Markdown with one <details> block per version. --only lists
the entries of the named kinds; the counts still cover every kind.

--from-mise digests every tool pinned in the project's mise config files that is behind
its latest version, from the pinned version to the latest. --all does the same for every pin
'relgap outdated' compares. Both use the ranges and digests 'relgap outdated' shows, and
take the project path as their only argument (default: the current directory).

--fail-on breaking exits 3 when a digest has a breaking entry at or above --min-confidence,
and --fail-on deprecation when it has a deprecation. A single digest counts once; --from-mise
and --all count each pin, as 'relgap outdated' does, and a value exits 3 once its count
reaches --min-count. With --from-mise or --all, a pin that fails exits 1 when no value
matched. The other --fail-on values belong to 'relgap outdated' and 'relgap drift'.

GitHub token: GH_TOKEN, GITHUB_TOKEN, gh's config file, then 'gh auth token'. Without one,
relgap uses the REST API (60 requests per hour) with 2 concurrent requests.

```
relgap digest <tool> <from>..[<to>] | --from-mise [path] | --all [path] [flags]
```

### Examples

```
  relgap digest terraform 1.15.8..1.17.1
  relgap digest helm 3.19.0.. --line major
  relgap digest kubectl 1.36.0..1.37.2 --format md > digest.md
  relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
  relgap digest argo-cd 3.2.0..3.4.1 --only breaking --only deprecation
  relgap digest --from-mise --fail-on breaking
  relgap digest --all ~/src/infra --format md > upgrade-notes.md
  relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'
```

### Options

```
      --all[=true|false]                   digest every outdated pin in the project, as 'relgap outdated' lists them
      --fail-on strings                    exit 3 when this matches: breaking or deprecation; repeatable (config policy.fail_on)
      --from-mise[=true|false]             digest every outdated tool pinned in the project's mise config, pin to latest
  -h, --help[=true|false]                  help for digest
      --include-prereleases[=true|false]   include prereleases (on when <from> or <to> is a prerelease)
      --line string                        release line the range stays on: none, major or minor (default "none")
      --min-count int                      findings a --fail-on value needs before it exits 3 (default 1)
      --only strings                       list only entries of this kind: breaking, deprecation, security, feature, fix or other; repeatable
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
      --no-cache[=true|false]   neither read nor write the HTTP cache
      --no-input[=true|false]   never prompt for input
      --offline[=true|false]    answer from the HTTP cache only and fail on a miss
  -q, --quiet[=true|false]      log errors only
      --refresh[=true|false]    revalidate every cached response, even fresh ones
  -v, --verbose count           more logging: -v info, -vv debug, -vvv trace
```

### SEE ALSO

* [relgap](relgap.md)	 - Release-notes digests and toolchain drift for the tools you pin

