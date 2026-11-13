## relgap mcp

Serve digest, breaking_changes, outdated, drift and resolve to an MCP client over stdio

### Synopsis

Runs a Model Context Protocol server on stdin and stdout, for agents such as Claude Code.
Logs go to stderr. Each tool returns the JSON envelope 'relgap <command> --format json'
prints:

  digest            relgap digest <tool> <from>..[<to>], with only, min_confidence and line
  breaking_changes  the same digest with only the breaking and deprecation entries
  outdated          relgap outdated <path>
  drift             relgap drift <roots...>, with tools, latest and ignore_constraints
  resolve           relgap resolve <tool>

The tools are read-only. They never start a process: the mise registry comes from the
embedded snapshot, scans walk directories instead of asking git (so .gitignore doesn't
apply), and the GitHub token comes from GH_TOKEN, GITHUB_TOKEN or gh's config file, not
from 'gh auth token'. They write nothing except the HTTP cache, which they share with the
other commands, so repeated calls are cheap. --offline, --refresh and --no-cache apply to
every call.

outdated and drift read only directories inside the roots: the --root directories, else
the roots the client lists (Claude Code lists its launch directory and the directories
added with --add-dir), else the current directory. A relative path starts at the first
root, and repositories are named relative to it. The config file is read once at startup,
as for every command.

A result over --max-result-bytes (default 50,000, Claude Code's limit for showing a result
to the model) is split. The envelope then has a page field, and the agent passes
page.next_cursor back to the same tool for the rest. Breaking changes, failed and
outdated pins, and drifting tools come first. A cursor lasts 15 minutes after its last use.

```
relgap mcp [flags]
```

### Examples

```
  claude mcp add --transport stdio --scope user relgap -- relgap mcp
  relgap mcp --root ~/src/infra --root ~/src/apps
  relgap mcp --offline
```

### Options

```
  -h, --help[=true|false]      help for mcp
      --max-result-bytes int   split results larger than this many bytes of JSON into pages; 0 never splits (default 50000)
      --root stringArray       directory that outdated and drift may read; repeatable (default: the client's roots, else the current directory)
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

