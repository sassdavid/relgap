# relgap ADR-0004: The LLM is optional, off by default, and only sees cited extracted entries

- Status: accepted (2026-11-09)

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
