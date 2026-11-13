## relgap pins

List the tool versions pinned in your repositories, with file and line

### Synopsis

Scans each root for repositories and lists every pinned tool version, with the
file, line and detector that found it. It reads files only and makes no network calls.

A directory that contains .git is a repository; the scan doesn't descend into another
repository below it. A root without any .git is one repository itself. git ls-files lists
the files, so .gitignore applies; without git, node_modules, vendor, .terraform and
.terragrunt-cache are skipped.

Detectors: mise (mise.toml and mise config directories), tool-versions, aqua, idiomatic
(.terraform-version, .nvmrc, go.mod and others), terraform, tf-lock, terragrunt, dockerfile,
ci (GitHub Actions, GitLab CI, Bitbucket Pipelines), helm, generic (scan.extra_files, opt-in
with scan.detectors.generic) and renovate (--renovate-report).

A value marked ≈ allows more than one version: a constraint (~> 1.15), a prefix (26) or a
channel (latest). A pin whose name maps to no known tool has an empty tool; list those
with --unresolved and map them with aliases in the config.

Roots default to scan.roots, then the current directory.

```
relgap pins [roots...] [flags]
```

### Examples

```
  relgap pins
  relgap pins ~/src/infra --format json
  relgap pins . --unresolved
  relgap pins . --renovate-report renovate-report.json
```

### Options

```
  -h, --help[=true|false]         help for pins
      --md-max-chars int          cut Markdown output to this many characters at a row or section boundary; 0 means no limit (default 60000)
      --out stringArray           also write the report to a file as format=path (repeatable); format is one of plain, md or json
      --renovate-report string    add the pins in this Renovate JSON report (renovate --platform=local --dry-run=extract --report-type=file)
      --submodules[=true|false]   scan checked-out git submodules as repositories
      --unresolved[=true|false]   list only pins that map to no known tool
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

