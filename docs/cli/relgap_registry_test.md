## relgap registry test

Run each source of a registry entry live and show what it produced

### Synopsis

Digests the range once per source of the tool's entry and once with all of them, and
prints for each: the versions it found notes for, its entry counts and the documents they came
from. Entries below --min-confidence count too.

<tool> resolves as in 'relgap resolve'. A path to a .yaml file runs that entry on its own, after
the checks 'relgap registry lint' makes, so a new entry can be tried before it's embedded.

A source that fails is listed with its error and the command exits 1. A source without notes in
the range isn't an error: an upgrade guide only has notes where a new release line starts.

```
relgap registry test <tool|file.yaml> <from>..[<to>] [flags]
```

### Examples

```
  relgap registry test kubectl 1.36.0..1.37.2
  relgap registry test registry/argocd.yaml 3.5.0..3.6.1
  relgap registry test python 3.13.5..3.14.0 --format json
```

### Options

```
  -h, --help[=true|false]   help for test
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

