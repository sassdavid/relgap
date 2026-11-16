# relgap ADR-0004: The LLM is optional, off by default, and only sees cited extracted entries

- Status: accepted (2026-11-09). Amended 2026-11-15 when `--summarize` shipped: guardrails,
  providers and defaults (see Amendment).

## Context
"Ask an LLM to read the release notes" is the competing workflow. LLMs hallucinate version-specific claims,
and sending whole changelogs is slow and expensive.

## Decision
- The deterministic pipeline (fetch → index → classify) is the product. `--summarize` is an optional layer on top.
- The LLM receives only extracted entries with IDs and source URLs. Every claim in its output must cite an ID. Uncited claims are removed and reported.
- Providers: OpenAI-compatible, Anthropic, Bedrock (Converse). No provider SDK is mandatory: the openai-compatible and Anthropic clients are thin HTTP clients; Bedrock reuses aws-sdk-go-v2.
- MCP is the preferred LLM integration: the agent calls `breaking_changes` and reasons itself.

## Consequences
- relgap stays useful offline and in CI without any API keys.
- Summaries are traceable, so a reviewer can verify each claim in one click.

## Amendment (2026-11-15)
The details are in mcp-and-llm.md, "Decisions (M15, `--summarize`)". The decisions that bind
later work:

- The summary is advisory. It never changes the `--fail-on` gate, the exit code or SARIF levels.
  The gate reads only the entries.
- Off by default means no network call. Without `llm.provider`, or with `--offline`,
  `--summarize` builds no client and sends nothing. A test (`TestSummarize_noProviderNoNetwork`)
  compares the requests with and without the flag.
- Every failure is a warning with no summary: missing config, an empty key, a provider error, a
  refusal and `llm.timeout` (default 120s, capped at 2m).
- Upstream text goes to the model as untrusted data: JSON lines between `<release_notes>` tags,
  with `<`, `>` and `&` escaped so no entry can close the block, and a system prompt that says not
  to follow instructions in it.
- The model gets the tool, the version range and the listed entries (ID, version, kind, text,
  source URL). Repository paths, pin locations, file contents and config values are never sent.
- A claim is kept only when it cites at least one ID and every ID it cites was sent to the model.
  An ID that the token budget left out counts as unknown.
- relgap ships no default model ID; `llm.model` is required.
- The Bedrock provider uses the AWS SDK's own HTTP client and retries (SigV4 signing, credential
  refresh), as tfcensus does, with `app.Deps.HTTPTransport` injected for tests. The other two
  providers use `internal/httpx` without the cache, and a POST is sent again only after a 429 or
  a Retry-After.

## Amendment (2026-11-16)
A fourth provider, `bedrock-mantle`, calls the Amazon Bedrock `bedrock-mantle` endpoint. AWS
serves the Claude models that Converse doesn't support there, and open-weight models that only
it hosts. The details are in mcp-and-llm.md. The decisions:

- It is a new `llm.provider` value, not a mode of `bedrock`, because it has a different wire
  protocol and transport. `llm.api` picks `messages` or `chat-completions`.
- It reuses the Anthropic and OpenAI-compatible request code on `internal/httpx`, so its retry
  rule is the same as theirs. No new module: SigV4 signing comes from
  `aws-sdk-go-v2/aws/signer/v4`, part of the core module relgap already pins. `internal/httpx`
  is unchanged and stays shared with tfcensus; the signer is the transport relgap passes in.
- Both Bedrock providers sign with SigV4 by default. `llm.api_key_env` opts into an Amazon
  Bedrock API key, which AWS recommends only as a short-term key.
