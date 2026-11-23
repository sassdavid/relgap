---
description: Short answers about what relgap does and doesn't do, how it compares with Renovate, which tools it supports and what it sends over the network.
---

# FAQ

## Does relgap change files or open pull requests?

No. relgap reads files and release notes and prints reports. Besides its own cache, relgap
writes in three places only: files you name with `--out`, the config template that
`relgap config init` writes, and the pull request comment that `relgap notify` posts or
edits. [Network, privacy and tokens](/concepts/network) lists every host relgap talks to.

## How is it different from Renovate or Dependabot?

Renovate and Dependabot open the pull request that bumps a pin. relgap tells you what the bump
contains: every release note in the range, merged and classified, with breaking changes and
deprecations first. The two work together. [Comment upgrade notes on Renovate pull
requests](/guides/renovate-pr) puts relgap's digest on each Renovate pull request, and
[Use Renovate's extract report as a pin source](/guides/renovate-report) reads the pins
Renovate found.

## Which tools does it support?

Any tool whose name mise's or aqua's registry knows, any GitHub repository through
`github:owner/repo`, and Terraform providers, Helm charts, npm and PyPI packages through
`tfprovider:`, `chart:`, `npm:` and `pypi:`. For those, relgap finds the release notes on its own.
The tools on [Supported tools](/reference/tools) have a curated entry that names the exact
sources, the version rules, the endoflife.date product and the OSV packages. Without an entry,
a tool has no EOL data, and its OSV packages come from its backend or repository.

## Why is a breaking change classified as a feature, or the other way round?

relgap classifies each entry from the headings it sits under, conventional commit prefixes,
leading verbs and keywords, and gives it a confidence. Entries below `--min-confidence` (0.5 by
default) are hidden and counted in the header. [Classification and
confidence](/concepts/classification) has the rules. When a tool's notes use their own
markers, a registry entry can name them in `breaking_markers`.

## Does relgap use an LLM?

Not unless you ask. `--summarize` adds a summary from a model you configure under `llm`, and no
other code path calls a model. The summary cites entry IDs, and relgap drops claims that cite
none. It never changes the exit code, `--fail-on` or SARIF. [Add an LLM
summary](/guides/summarize) covers the setup, and [Network, privacy and
tokens](/concepts/network#summaries-from-a-model) lists what the model receives.

## Do I need a GitHub token?

For a small digest, no: GitHub allows 60 unauthenticated requests an hour. For `outdated`,
`digest --all` or CI, yes. relgap finds the token the way gh does, so a gh login is enough.
A token with no scopes reads public repositories.

## Does it work offline?

`relgap pins` and `relgap drift` without `--latest` read files only. The other commands answer
from the cache with `--offline` and fail on a miss. [Work offline and manage the
cache](/guides/cache) has the details.

## Does it work with GitLab or Bitbucket?

relgap reads release notes from GitHub only, and `relgap notify` comments on GitHub pull
requests only. The pin detectors read GitLab CI and Bitbucket Pipelines files, so `pins`,
`drift` and `outdated` work on repositories hosted anywhere.

## Can I use it from Claude Code?

Yes. `relgap mcp` serves `digest`, `breaking_changes`, `outdated`, `drift` and `resolve` over
MCP, and the `upgrade_review` prompt asks the agent to cite the entries it uses. [Use relgap
from an MCP client](/guides/mcp) has the setup.

## Is the output stable?

relgap is pre-release, and its commands and output can change before 1.0. The JSON output
carries `schema_version`, and its [schemas](/reference/schemas) are published. Each release
names its changes on the [changelog](/changelog).
