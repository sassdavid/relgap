---
description: Register relgap as an MCP server in Claude Code or another client, choose the directories it may read, and use its tools, pages and upgrade_review prompt.
---

# Use relgap from Claude Code or another MCP client

`relgap mcp` runs a Model Context Protocol server on stdin and stdout. An agent can then ask what
breaks between two versions, which pins are behind and where repositories disagree, and gets the
JSON that `--format json` prints. [MCP tools and prompts](/reference/mcp) has the input schema of
every tool.

## Register the server

In Claude Code, add relgap for every project:

```sh
claude mcp add --transport stdio --scope user relgap -- relgap mcp
```

`--scope local` registers it for the current project only. Claude Code starts MCP servers when a
session starts, so open a new session after adding it. Other clients that start stdio servers
need the command `relgap` with the argument `mcp`, and any flags after it.

The server reads the GitHub token from `GH_TOKEN`, `GITHUB_TOKEN` or gh's config file. It never
runs `gh auth token`, so when gh keeps your token in the system keyring, export `GH_TOKEN` in the
environment that starts the client. Logs go to stderr; add `-v` after `mcp` for more.

## Choose what it may read

`outdated` and `drift` read only directories inside the server's roots:

1. the `--root` directories, when you pass any
2. else the roots the client lists: Claude Code lists its launch directory and the directories
   added with `--add-dir`
3. else the current directory

```sh
claude mcp add --transport stdio --scope user relgap -- relgap mcp --root ~/src/infra --root ~/src/apps
```

A relative path starts at the first root, and repositories are named relative to it. The server
reads the config file once at startup, and the search for `.relgap.yaml` stops at the first
`--root`.

## The tools

| Tool | Same as |
|---|---|
| `digest` | `relgap digest <tool> <from>..[<to>]`, with `only`, `min_confidence`, `line` and `include_prereleases` |
| `breaking_changes` | the same digest with only the breaking and deprecation entries |
| `outdated` | `relgap outdated <path>` |
| `drift` | `relgap drift <roots...>`, with `tools`, `latest` and `ignore_constraints` |
| `resolve` | `relgap resolve <tool>` |

The tools are read-only. They write nothing except the HTTP cache, which they share with the other
commands, so a repeated question is answered from the cache. They never start a process: the mise
registry comes from the embedded snapshot, and scans walk the directories instead of asking git,
so `.gitignore` doesn't apply. `--offline`, `--refresh` and `--no-cache` after `mcp` apply to every
call ([Work offline and manage the cache](/guides/cache)):

```sh
relgap mcp --offline
```

## Pages

A result over `--max-result-bytes` (default 50,000, Claude Code's limit for showing a result to
the model) is split into pages. The envelope then has a `page` field, and the agent calls the
same tool with only `page.next_cursor` to get the rest. Breaking changes, failed and outdated
pins, and drifting tools come first. A digest page lists only the versions its entries cite. A
cursor lasts 15 minutes after its last use. `--max-result-bytes 0` never splits.

## The upgrade_review prompt

The prompt `upgrade_review` takes a tool, the version you have and, optionally, the version you
upgrade to. It tells the agent to call `breaking_changes`, list the required actions and cite the
ID of every entry it uses, such as `[e-3f9a01c2]`. Claude Code offers it as
`/mcp__relgap__upgrade_review`.

An entry ID is `e-` and 8 hex digits from a hash of the tool, the version and the entry's text.
It stays the same in every page, filter and run, so you can find a cited entry in
`relgap digest --format json` output. [Classification and confidence](/concepts/classification)
explains the entries.
