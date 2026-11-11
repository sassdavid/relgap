## relgap registry lint

Validate registry entries against the schema

### Synopsis

Without arguments, checks the entries embedded in relgap. With arguments, checks those files.

Reports unknown keys, wrong types, bad regular expressions and a name that doesn't match the
file name, each with file:line:column, and exits 2 when there is any.

```
relgap registry lint [file...] [flags]
```

### Examples

```
  relgap registry lint
  relgap registry lint registry/terraform.yaml
```

### Options

```
  -h, --help[=true|false]   help for lint
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

* [relgap registry](relgap_registry.md)	 - Check relgap registry entries

