# relgap: overview

> **rel**ease **gap**: everything that changed between the version you pin and the version you're moving to,
> across every tool and every repo you own.

## Problem
1. **Reading release notes across a range is manual.** Upgrading terraform 1.15.8 → 1.17.1, ArgoCD 3.5 → 3.6,
   or Claude Code across 40 patch releases means opening many pages in different formats and places:
   GitHub releases, `CHANGELOG.md` on release branches, `CHANGELOG-1.xx.md`, `.rst`, `.changes/*.json`, upgrade guides.
2. **Pins drift.** The same tool is pinned at different versions in mise.toml, Dockerfiles, CI images,
   `required_version` and `.terraform-version`, across many repos. Nobody notices until something breaks
   (e.g. an `hk` version mismatch made CI re-download it on every run).
3. **Breaking changes hide in long notes.** terragrunt v1.2.0's release body alone is 96 KB.

## Users
- Platform and DevOps engineers who manage toolchains with mise, asdf or aqua and own many repos.
- Teams reviewing Renovate or Dependabot PRs for CLI tools, Terraform providers and Helm charts.
- LLM agents (Claude Code and others) through MCP, asking "what breaks if I upgrade X?"

## Goals
- **G1 Digest.** `relgap digest <tool> [from..to]` merges notes for every version in the range and groups them into Breaking / Deprecations / Security / Features / Fixes, citing the source of each item.
- **G2 Outdated.** `relgap outdated` covers the current project: every pinned tool with current → latest, number of releases skipped, breaking count, EOL status and known vulnerabilities.
- **G3 Drift.** `relgap drift <roots…>` gives a matrix of tool × repo → pinned version, highlighting disagreements and the oldest pins.
- **G4 Correct sources.** A per-tool adapter registry that knows where each tool's real changelog lives. It is community-extensible, with auto-discovery as the fallback.
- **G5 Machine-friendly.** Stable JSON output, Markdown for PRs, SARIF and `--fail-on` for CI, an MCP server.
- **G6 Fast and polite.** A warm-cache digest takes under 2 s. ETag caching, GraphQL batching, offline mode.
- **G7 Optional LLM summary.** `--summarize` sends the extracted, cited items to any OpenAI-compatible, Anthropic or Bedrock endpoint. It is never required and never on by default.

## Non-goals
- **Editing files or opening PRs.** Renovate and Dependabot do that; relgap complements them.
- **Library dependency graphs** (npm/pypi/go.mod deps of an application). We digest *tools*. A tool that happens to ship via npm or PyPI (claude-code, uv) is in scope; your app's `package.json` is not.
- Hosted service, accounts or telemetry.
- Scraping arbitrary websites. Web sources only come from registry entries (python whatsnew, go.dev).

## Success criteria (v0.1)
- The digest is correct (versions in range, sections attributed) for the **30-tool corpus** in [testing.md](testing.md), verified by golden files.
- `relgap outdated` in dotfriedrice (≈45 mise tools) finishes in under 10 s cold and under 2 s warm, within 5,000 GitHub requests/h authenticated.
- `relgap drift ~/src/infra` over about 60 repos finishes in under 5 s with no network (`--no-latest`).
- Zero crashes on the fuzz corpus of version strings and changelogs.

## About the "weak demand signal" risk
Research found few explicit public requests: gh issue #6508 has had 2 comments since 2022, and no Reddit threads.
That's a risk to **adoption**, not to feasibility. Two things lower it:
- **The best evidence:** jdx/mise discussion #9708 (May 2026) asks for exactly the in-between release notes on `mise upgrade`. The maintainer shipped only `release_url` (target version) and called the rest "unstarted rather than planned". So relgap **integrates with mise** (`mise outdated --json` as input) instead of competing with it.
- Renovate users repeatedly ask for richer changelog content (#35031, #45328).

Mitigations are in [risks.md](risks.md) (R1).
