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
- [ ] An in-process MCP client test calls every tool and matches the `--format json` golden output.
- [ ] Response size caps are enforced (over 50 KB → truncated with a cursor).
- [ ] The summarizer is tested against a fake provider. It rejects uncited claims and respects the token budget.
- [ ] A Bedrock provider smoke test exists in the `live.yml` nightly only (skipped without credentials).
