## relgap config init

Write a commented config file

### Synopsis

Writes a config file that lists every key commented out at its default, so the file
changes nothing until you uncomment a key. It goes to .relgap.yaml in the working
directory, to path, or with --user to the user file. The directory is created when it's missing.

An existing file is kept, with exit code 2, unless you pass --force. The written path goes
to stderr.

```
relgap config init [path] [flags]
```

### Examples

```
  relgap config init
  relgap config init --user
  relgap config init ci/relgap.yaml --force
```

### Options

```
      --force[=true|false]   replace an existing file
  -h, --help[=true|false]    help for init
      --user[=true|false]    write the user file instead of .relgap.yaml
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

* [relgap config](relgap_config.md)	 - Inspect and validate configuration

