## relgap completion

Generate the autocompletion script for the specified shell

### Synopsis

Generate the autocompletion script for relgap for the specified shell.
See each sub-command's help for details on how to use the generated script.


### Options

```
  -h, --help[=true|false]   help for completion
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
* [relgap completion bash](relgap_completion_bash.md)	 - Generate the autocompletion script for bash
* [relgap completion clink](relgap_completion_clink.md)	 - Generate the autocompletion script for clink
* [relgap completion elvish](relgap_completion_elvish.md)	 - Generate the autocompletion script for elvish
* [relgap completion fish](relgap_completion_fish.md)	 - Generate the autocompletion script for fish
* [relgap completion nushell](relgap_completion_nushell.md)	 - Generate the autocompletion script for nushell
* [relgap completion oil](relgap_completion_oil.md)	 - Generate the autocompletion script for oil
* [relgap completion powershell](relgap_completion_powershell.md)	 - Generate the autocompletion script for powershell
* [relgap completion tcsh](relgap_completion_tcsh.md)	 - Generate the autocompletion script for tcsh
* [relgap completion xonsh](relgap_completion_xonsh.md)	 - Generate the autocompletion script for xonsh
* [relgap completion zsh](relgap_completion_zsh.md)	 - Generate the autocompletion script for zsh

