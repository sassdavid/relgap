# relgap: risks

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | **Weak public demand:** few explicit requests, so it may stay a personal tool | medium | medium | Dogfood first (it pays off for the author alone). Integrate with mise and Renovate instead of competing. MCP angle (agents need cited, structured notes). Announce in mise discussion #9708, r/devops, r/Terraform, HN Show with a concrete demo (terraform 1.15 → 1.17 breaking digest) |
| R2 | Upstream changelog formats change silently | high | medium | Registry entries can be fixed without a release ([ADR-0002](adr/0002-override-registry-in-repo.md)). The nightly live corpus opens issues on drift. Classification degrades gracefully (confidence scores) |
| R3 | GitHub rate limits (60/h unauthenticated, secondary limits) | medium | medium | ETag cache, GraphQL batching, adaptive concurrency, clear token guidance, offline mode |
| R4 | mise ships in-between release notes | low–medium | medium | relgap's scope is wider (drift, Docker/TF/CI pins, classification, EOL/OSV, CI gates, MCP). Integrate with whatever mise ships |
| R5 | Classification false positives or negatives erode trust | medium | high | Confidence scores, `--min-confidence`, a labelled precision gate ≥ 0.9, a source link on every entry |
| R6 | Scope creep into "Renovate 2" (editing pins, opening PRs) | medium | high | Explicit non-goal in [overview.md](overview.md). Stable JSON lets others build on it |
| R7 | Registry maintenance burden | medium | medium | Only curate what auto-discovery gets wrong. Contributor tooling (`registry test`). Snapshots of mise and aqua give repo mapping for free |
| R8 | LLM summary hallucinations | medium | medium | Off by default, cite-or-drop validation ([ADR-0004](adr/0004-llm-optional-cited.md)) |
| R9 | Maintainer bandwidth (two tools, one person) | high | medium | relgap ships v0.1 first (smaller). Shared stack and templates. Milestones sized for single sessions |
