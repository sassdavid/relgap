## relgap config validate

Validate config files against the schema

### Synopsis

Without arguments, validates the user and project config files plus RELGAP_* env vars and
flags, exactly as other commands would load them. With arguments, validates only those files.

Exits 2 and prints every problem with file:line:column when something is invalid.

```
relgap config validate [file...] [flags]
```

### Examples

```
  relgap config validate
  relgap config validate .relgap.yaml ~/.config/relgap/config.yaml
```

### Options

```
  -h, --help[=true|false]   help for validate
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

* [relgap config](relgap_config.md)	 - Inspect and validate configuration

