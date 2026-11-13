## relgap

Release-notes digests and toolchain drift for the tools you pin

### Synopsis

relgap merges release notes across a version range into one digest (breaking changes
first, with source links), and reports toolchain drift, EOL and known vulnerabilities across
your repositories.

stdout carries data, stderr carries logs. Use --format json for scripting.

### Options

```
      --color string            colorize output: auto, always or never (default "auto")
      --concurrency int         maximum concurrent requests per tool (config github.concurrency) (default 8)
      --config string           config file to use instead of the discovered .relgap.yaml (env RELGAP_CONFIG)
      --format string           output format: tty, plain, md, json, sarif or github (default tty when stdout is a terminal, else plain)
  -h, --help[=true|false]       help for relgap
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

* [relgap cache](relgap_cache.md)	 - Inspect or clear the HTTP cache
* [relgap completion](relgap_completion.md)	 - Generate the autocompletion script for the specified shell
* [relgap config](relgap_config.md)	 - Inspect and validate configuration
* [relgap digest](relgap_digest.md)	 - Merge the release notes of every version in a range into one digest
* [relgap drift](relgap_drift.md)	 - Show which repositories pin different versions of the same tool
* [relgap mcp](relgap_mcp.md)	 - Serve digest, breaking_changes, outdated, drift and resolve to an MCP client over stdio
* [relgap notify](relgap_notify.md)	 - Publish saved digest, outdated and drift reports as a GitHub pull request comment
* [relgap outdated](relgap_outdated.md)	 - Compare every pinned tool version in a project with the latest release
* [relgap pins](relgap_pins.md)	 - List the tool versions pinned in your repositories, with file and line
* [relgap registry](relgap_registry.md)	 - Check relgap registry entries
* [relgap resolve](relgap_resolve.md)	 - Show how a tool name resolves to a repository, version rules and changelog sources
* [relgap version](relgap_version.md)	 - Print version and build information

