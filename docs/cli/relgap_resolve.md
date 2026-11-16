## relgap resolve

Show how a tool name resolves to a repository, version rules and changelog sources

### Synopsis

Resolves a tool name and prints the result with the layer that supplied each field.

<tool> is a name or alias (terraform, tf), a NAME_VERSION variable (TERRAFORM_VERSION),
github:owner/repo, aqua:package, tfprovider:namespace/name, chart:repo/name, chart:name,
npm:package or pypi:package. When a registry knows the tool, the result is the same as for its
name.

tfprovider:, chart:, npm: and pypi: names that no registry knows are looked up in the Terraform
registry, Artifact Hub, the npm registry or PyPI, which name the source repository. chart:name
without a repository resolves only when one Artifact Hub repository has a chart of that name.

Layers, highest precedence first:

  config           tools.<name> and aliases in the config file
  relgap registry  curated entries embedded in relgap (registry/*.yaml), or the newer copies
                   from 'relgap registry update'
  mise registry    mise registry --json when mise is on PATH, otherwise an embedded snapshot
  aqua registry    an embedded index of the aqua registry
  default          built-in defaults

With --guess, a name that no layer knows is searched on GitHub. The repository whose name
matches exactly and has the most stars wins, and the result is marked low-confidence.

```
relgap resolve <tool> [flags]
```

### Examples

```
  relgap resolve terraform
  relgap resolve TERRAFORM_VERSION
  relgap resolve aqua:hashicorp/terraform --format json
  relgap resolve tfprovider:integrations/github
  relgap resolve chart:argo/argo-cd
  relgap resolve some-tool --guess
```

### Options

```
      --guess[=true|false]   search GitHub for a tool no registry knows and use the exact name match with the most stars
  -h, --help[=true|false]    help for resolve
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

