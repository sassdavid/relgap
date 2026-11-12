## relgap outdated

Compare every pinned tool version in a project with the latest release

### Synopsis

Scans the project at path (default: the current directory) the way 'relgap pins' does and
compares each pin with the newest stable version on the tool's release line.

CURRENT is the version a pin stands for. An exact pin is itself. A value marked ≈ allows
more than one version, and the notes say which one relgap used: a constraint (~> 1.15) takes
the highest exact pin of the tool in the same repository that satisfies it, otherwise the
lowest version that does; a prefix (26, prefix:1.2) takes the highest stable version that
starts with it; the latest channel is the latest version. Other channels (lts) aren't
compared.

BEHIND, BREAKING and DEPRECATION come from the digest of CURRENT..LATEST, the same digest
'relgap digest <tool> <current>..' prints. Entries below --min-confidence aren't counted.

relgap finds the latest versions with one GraphQL query per 25 tools. Only pins that are
behind, and constraint and prefix pins, need a tool's full release list.

Pins of tools without a GitHub repository, and tfprovider, chart and image pins, are
skipped; --format json lists them with the reason.

EOL shows whether CURRENT's release cycle has reached its end on endoflife.date (yes, no, or
? when endoflife.date doesn't list the cycle), for tools whose registry entry names a
product. --format json also has the state of LATEST's cycle. VULNS counts the OSV
vulnerabilities of CURRENT, and how many of them upgrading to LATEST fixes. Both sources
are cached for a day. When one is down, relgap warns and prints the rest.

--binaries also reads the Go build info of each pinned tool's installed executables (on
PATH and in mise's installs directory) and lists the OSV vulnerabilities of the Go version
and main module they were built with. Like govulncheck's binary mode, minus the check that
the binary calls the vulnerable code.

--format sarif writes a SARIF 2.1.0 log for GitHub code scanning, located at each pin's
file and line: a result per pin behind latest (outdated), per pin on an EOL cycle (eol), per
upgrade with breaking changes or deprecations (breaking, deprecation), per advisory that
affects a pin (one rule per OSV ID), and per pin or file that failed (failed). The results
don't depend on --fail-on.

--fail-on takes breaking, deprecation, eol, vuln and outdated. Each value counts findings:
outdated counts the pins further behind than --max-behind (major:N, minor:N, patch:N,
releases:N or days:N; empty means any pin behind latest), breaking and deprecation count
the pins whose digest has such an entry, eol counts the pins on an EOL cycle, and vuln
counts the advisories that affect a pin at or above --vuln-severity. A value whose count
reaches --min-count (default 1) exits 3. Otherwise a tool or file that failed exits 1, and
so does an endoflife.date outage under --fail-on eol or an OSV outage under --fail-on vuln,
since a pass on partial data proves nothing.

```
relgap outdated [path] [flags]
```

### Examples

```
  relgap outdated
  relgap outdated ~/src/infra --format md
  relgap outdated --fail-on outdated --max-behind minor:2
  relgap outdated --fail-on eol --fail-on vuln --vuln-severity high
  relgap outdated --fail-on outdated --min-count 5 --format sarif > relgap.sarif
  relgap outdated --binaries --format json | jq '.data.binaries[]'

  relgap outdated --format json | jq '.data.pins[] | select(.status == "outdated")'
```

### Options

```
      --binaries[=true|false]   also check the Go version and main module of the pinned tools' installed executables against OSV
      --fail-on strings         exit 3 when this matches: breaking, deprecation, eol, vuln or outdated; repeatable (config policy.fail_on)
  -h, --help[=true|false]       help for outdated
      --max-behind string       how far behind --fail-on outdated allows, such as minor:2, releases:10 or days:90 (config policy.max_behind)
      --min-count int           findings a --fail-on value needs before it exits 3 (default 1)
      --vuln-severity string    lowest rating --fail-on vuln counts: low, medium, high or critical; empty counts all (config policy.vuln_severity)
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

