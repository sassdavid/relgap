# relgap docs

**Status:** draft plan, 2026-11-09. Not started. Language: Go ([ADR-0001](engineering/adr/0001-language-go.md)).

`relgap` shows everything that changed between the version you pin and the version you're moving to:
merged, classified release notes for any version range, drift of the same tool across many repos, and EOL and
vulnerability status. Output goes to the terminal, Markdown, JSON, SARIF and MCP.

Contributor and agent rules: [CLAUDE.md](../CLAUDE.md).

## Plan (`plan/`): reading order
1. [overview.md](plan/overview.md): problem, users, goals, non-goals, success criteria, demand risk
2. [architecture.md](plan/architecture.md): pipeline, packages, data model
3. [cli.md](plan/cli.md): commands, flags, exit codes
4. specs (feature contracts with acceptance criteria):
   [pin-discovery](plan/specs/pin-discovery.md) ·
   [tool-resolution](plan/specs/tool-resolution.md) ·
   [versions-and-ranges](plan/specs/versions-and-ranges.md) ·
   [source-adapters](plan/specs/source-adapters.md) ·
   [classification](plan/specs/classification.md) ·
   [enrichment](plan/specs/enrichment.md) ·
   [output-and-ci](plan/specs/output-and-ci.md) ·
   [mcp-and-llm](plan/specs/mcp-and-llm.md) ·
   [config](plan/specs/config.md)
5. [milestones.md](plan/milestones.md): M0–M12, each one session with checkable "done when" items
6. [testing.md](plan/testing.md) · [risks.md](plan/risks.md) · [open-questions.md](plan/open-questions.md) · [research.md](plan/research.md)
7. Tool ADRs: [0001 goldmark section index](plan/adr/0001-goldmark-section-index.md) ·
   [0002 override registry](plan/adr/0002-override-registry-in-repo.md) ·
   [0003 integrate with mise](plan/adr/0003-integrate-with-mise.md) ·
   [0004 optional cited LLM](plan/adr/0004-llm-optional-cited.md)
8. [reference-repos.md](plan/reference-repos.md): local clones to read (never copy) during implementation
9. [integration.md](plan/integration.md): how relgap connects to its sibling tool [tfcensus](https://github.com/sassdavid/tfcensus)

## Engineering (`engineering/`)
Stack, standards, CI/release pipeline and engineering ADRs (shared with tfcensus): [engineering/README.md](engineering/README.md).
