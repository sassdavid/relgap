## relgap cache prune

Delete entries not used for a while

### Synopsis

Deletes entries that no run has used for --older-than (default 720h, 30 days), entries
relgap can't read, entries in an older format and temporary files left by an interrupted run.
Expired entries stay: their ETags let the next run revalidate them for free.

```
relgap cache prune [flags]
```

### Examples

```
  relgap cache prune
  relgap cache prune --older-than 168h
```

### Options

```
  -h, --help[=true|false]     help for prune
      --older-than duration   delete entries unused for this long (default 720h0m0s)
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

* [relgap cache](relgap_cache.md)	 - Inspect or clear the HTTP cache

