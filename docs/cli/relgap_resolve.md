## relgap resolve

Show how a tool name resolves to a repository, version rules and changelog sources

### Synopsis

Resolves a tool name and prints the result with the layer that supplied each field.

<tool> is a name or alias (terraform, tf), a NAME_VERSION variable (TERRAFORM_VERSION),
github:owner/repo or aqua:package. github: and aqua: identify a tool; when a registry knows it,
the result is the same as for its name.

Layers, highest precedence first:

  config           tools.<name> and aliases in the config file
  relgap registry  curated entries embedded in relgap (registry/*.yaml)
  mise registry    mise registry --json when mise is on PATH, otherwise an embedded snapshot
  aqua registry    an embedded index of the aqua registry
  default          built-in defaults

```
relgap resolve <tool> [flags]
```

### Examples

```
  relgap resolve terraform
  relgap resolve TERRAFORM_VERSION
  relgap resolve aqua:hashicorp/terraform --format json
```

### Options

```
  -h, --help[=true|false]   help for resolve
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

