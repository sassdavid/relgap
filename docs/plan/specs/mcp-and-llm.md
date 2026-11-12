# Spec: MCP server and optional LLM summary

## MCP (`relgap mcp`, stdio, official go-sdk)
| Tool | Input | Output (JSON envelope, size-capped) |
|---|---|---|
| `digest` | `tool`, `from?`, `to?`, `only?[]`, `min_confidence?` | `ToolReport` (entries truncated to N per kind, `cursor` for more) |
| `outdated` | `path?` | table rows |
| `drift` | `roots[]`, `tools?[]` | matrix |
| `resolve` | `tool` | ToolSpec |
| `breaking_changes` | `tool`, `from`, `to` | breaking + deprecation entries only (the most common agent question) |

- Read-only. `server_info` names relgap and its version. Tool descriptions are one sentence each.
- Filesystem access is limited to paths the client passes. No implicit home-dir scanning.
- Reuses the HTTP cache, so repeated agent calls are cheap.

### Decisions (M10)
Agreed before coding, except where a point says it changed during the work.

- Inputs follow the CLI: `from` is required and `to` is optional for `digest` and
  `breaking_changes`. `digest` also takes `line` and `include_prereleases`. `breaking_changes`
  is `digest` with `only: [breaking, deprecation]`, and `relgap digest --only` gives the same
  envelope, so it has a golden file too. With `--only`, `counts` and `hidden` still cover every
  kind. `outdated` takes `path` (default: the first root). `drift` takes `roots` (required),
  `tools`, `latest` and `ignore_constraints`. `--binaries`, `--submodules` and `--fail-on` are
  not offered.
- Changed: required arguments are not in the input schema's `required` list. A call with only
  a cursor needs none of them, and the Anthropic API rejects `anyOf` at the top of a tool's
  input schema. Each such property's description starts with "Required unless cursor is set",
  and mcpkit returns `missing required argument "from"` as a tool error.
- Each result is compact JSON: the `--format json` envelope on one line. `json.Indent` with two
  spaces gives the CLI's bytes, which is how the tests compare results with the golden files.
  Compact JSON is 28% (digest) to 49% (drift) smaller.
- A result over 50,000 bytes is split (`relgap mcp --max-result-bytes`, 0 never splits; Claude
  Code saves larger text results to a file). Each tool pages one list. `digest` and
  `breaking_changes` page entries, and breaking, deprecation, security, feature, fix and other
  fill the pages in that order. `outdated` pages pins: failed pins, then outdated pins, then the
  rest. `drift` pages tools: drifting tools first. Each page lists its items in their original
  order, repeats the other fields, and holds at least one item. The envelope's optional `page`
  field (`items`, `returned`, `total`, `next_cursor`) is in `schemas/report-v1.json`, shared
  with tfcensus; CLI output never has it.
- The cursor is URL-safe base64 of a random 128-bit ID and a page number. It names a result
  kept in the server's memory for 15 minutes after its last use, at most 16 at a time (least
  recently used dropped first), and only for the tool that made it. A repeated cursor returns
  the same page. An unknown or expired cursor is a tool error that says to call again without
  one. The cursor is never part of a file name.
- Directory arguments must be inside a root, after resolving symlinks: the `--root`
  directories, else the client's roots, else the server's working directory. Relative paths
  start at the first root, and repositories are named relative to it (`scan.Options.Base`), so
  `outdated` on the root prints `.` as the CLI does there. On Windows `filepath.Rel` compares
  without case and fails across volumes. `file://` roots become local paths, including
  `file:///C:/x` and `file://host/share`.
- Changed: go-sdk v1.8.0 refuses `roots/list` while it serves a request on protocol
  2026-07-28. mcpkit asks for roots with a multi round-trip input request (SEP-2322) instead,
  and the SDK sends `roots/list` itself to clients on older versions. `mcpkit.ClientRoots`
  reads the answer from the request, so it takes no context.
- No tool starts a process. The registry uses its embedded mise snapshot, scans walk
  directories instead of running `git ls-files` (so `.gitignore` doesn't apply), and the token
  comes from `GH_TOKEN`, `GITHUB_TOKEN` or gh's config file, never `gh auth token`. The tools
  write nothing but the HTTP cache, which they share with the CLI. `--offline`, `--refresh` and
  `--no-cache` on `relgap mcp` apply to every call. `drift` makes no network call unless
  `latest` is true.
- Changed: the config file is read once at startup, from `--config`, `RELGAP_CONFIG` or the
  server's working directory up to the git root, as for every command. Stopping the search at
  the root needs an option in `internal/config/load.go`, which is shared with tfcensus and
  wasn't in scope for this milestone. Tool arguments never choose a config file.
- Errors: what the CLI exits 2 or 4 for (unknown tool, bad range or kind, a path outside the
  roots, a bad cursor, auth and rate-limit failures, offline misses) is a tool error with the
  CLI's message. A partial result is a normal result with the envelope's `errors` filled, as
  `--format json` prints on exit 1.
- `internal/mcpkit` and `internal/testutil/mcptest` are shared with tfcensus. mcpkit marks every
  tool read-only, idempotent and not destructive. The no-exec rule is relgap's, because tfcensus
  runs terragrunt. `TestMCP_staticNoExecNoWrite` reads the code of `relgap mcp` and its session.
  `TestMCP_noProcessesNoWrites` puts fake git, mise and gh on PATH and checks that none ran and
  that only the cache changed.

## LLM summary (`--summarize`)
Default **off**. When on:
1. relgap first builds the normal report. The LLM only sees **extracted entries with their source URLs**, never raw pages.
2. The prompt asks for: a 5-line upgrade-risk summary, required actions, and each claim citing an entry ID. The response is checked: claims citing unknown IDs are dropped, and a warning is printed.
3. Providers, via a small `summarize.Provider` interface:
   - `openai-compatible`: `base_url` + key env var. Covers OpenAI, LiteLLM, Ollama and vLLM.
   - `anthropic`: Messages API.
   - `bedrock`: Converse API via aws-sdk-go-v2, using a profile. Fits the author's Bedrock setup.
4. Config: `llm.provider`, `llm.model`, `llm.base_url`, `llm.api_key_env`, `llm.max_input_tokens` (default 30k; entries are trimmed lowest confidence first).
5. Output: an extra `summary` block in every format, clearly labelled "LLM-generated, verify against cited sources".

Non-goals: no chat loop, no embedding store, no LLM in the default path, no sending of local file content.

## Acceptance criteria
- [x] An in-process MCP client test calls every tool and matches the `--format json` golden output. (`TestSpec_mcp_and_llm_1`)
- [x] Response size caps are enforced (over 50 KB → truncated with a cursor). (`TestSpec_mcp_and_llm_2`)
- [ ] The summarizer is tested against a fake provider. It rejects uncited claims and respects the token budget.
- [ ] A Bedrock provider smoke test exists in the `live.yml` nightly only (skipped without credentials).
