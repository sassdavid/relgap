## relgap drift

Show which repositories pin different versions of the same tool

### Synopsis

Scans each root for repositories the way 'relgap pins' does and prints a matrix with a
row per repository and a column per tool that drifts. A tool drifts when its pins stand for
more than one version across the scanned repositories. It reads files only and makes no
network calls unless --latest is set.

An exact pin counts as its version. A value marked ≈ allows more than one version. A
constraint (~> 1.15) counts as the highest exact pin of the tool in the same repository
that satisfies it, otherwise as its lower bound (1.15.0). Prefix (26) and channel (latest)
pins aren't compared. --drift-ignore-constraints leaves constraint pins out of the
comparison too. A version marked * differs from the version most repositories pin.

Tools that agree everywhere, or that only one repository pins, are listed below the matrix.
--tool limits the report to the named tools and gives each one a column, even when it
doesn't drift. Ignore rules in the config drop matching pins.

--latest adds a latest row with the newest stable version of each tool, from one GraphQL
query per 25 tools.

--fail-on drift exits 3 when a tool drifts. A file that doesn't parse, or a tool whose latest
version can't be listed, exits 1 instead, after printing the rest.

Roots default to scan.roots, then the current directory.

```
relgap drift [roots...] [flags]
```

### Examples

```
  relgap drift ~/src/infra
  relgap drift ~/src/infra --tool terraform --tool terragrunt
  relgap drift ~/src/infra ~/src/apps --latest --format md
  relgap drift --fail-on drift --drift-ignore-constraints
```

### Options

```
      --drift-ignore-constraints[=true|false]   compare exact pins only; constraint pins are shown but don't count
      --fail-on strings                         exit 3 when this matches: drift; repeatable (config policy.fail_on)
  -h, --help[=true|false]                       help for drift
      --latest[=true|false]                     add the newest stable version of each tool (needs the network or the cache)
      --submodules[=true|false]                 scan checked-out git submodules as repositories
      --tool strings                            report only this tool, with a column even when it agrees; repeatable
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

