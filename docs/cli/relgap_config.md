## relgap config

Inspect and validate configuration

### Synopsis

Configuration is layered; later layers win:

  defaults < user file < project file < env (RELGAP_*) < flags

User file:    $XDG_CONFIG_HOME/relgap/config.yaml (~/.config/relgap/config.yaml)
Project file: .relgap.yaml, found by walking up from the working directory to the git root
Env vars:     RELGAP_<KEY> with dots as underscores, e.g. RELGAP_GITHUB_CONCURRENCY=4.
              Use __ to mark nesting explicitly (RELGAP_SCAN__DETECTORS__CI_ACTIONS=true).
              Lists are comma-separated.

Unknown keys and wrong types are errors (exit 2), reported with file:line:column.

### Options

```
  -h, --help[=true|false]   help for config
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
* [relgap config path](relgap_config_path.md)	 - Print the config file locations relgap reads
* [relgap config show](relgap_config_show.md)	 - Print the effective configuration
* [relgap config validate](relgap_config_validate.md)	 - Validate config files against the schema

