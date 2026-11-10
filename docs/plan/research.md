# relgap: research record (2026-11-09)

This is the evidence behind the plan. Re-check before v0.1 if it's more than about 3 months old.

## Prior art (none combines a multi-repo drift scan with range digests for DevOps binaries)
| Tool | What | Gap vs relgap |
|---|---|---|
| whatsdiff (PHP, ★77, v2.7.4 2026-08) | `changelog pkg A...B --summary`, MCP | Composer/npm only |
| release-notes-mcp (JS, ★1, 2026-09) | `changes_between`, breaking extraction | package registries only, MCP only |
| mortenscheel/release-notes-cli (PHP) | `--from/--to` for any GitHub repo | abandoned 2023 |
| Renovate | release notes in PR bodies; `--platform=local` extract (experimental) | no cross-repo view (discussion #33219), notes only in PRs |
| dependency-management-data (Go, ★20) | Renovate output → SQLite, query it yourself | heavy, DIY SQL |
| `gh` | single release view | range request open since 2022 (cli/cli#6508) |
| `mise outdated --json` | versions + target `release_url` | no in-between notes (#9708, #12494) |
| nvchecker, newreleases.io | versions / notifications | no notes aggregation |

## Where changelogs actually live (key traps)
- terraform: release body = changelog section; main `CHANGELOG.md` has only Unreleased + links to `blob/v1.17/CHANGELOG.md`.
- kubernetes: `CHANGELOG/CHANGELOG-1.xx.md`; release bodies are boilerplate; `Dependencies › Removed` false positive.
- helm: v3/v4 interleaved publish order. argocd: `CHANGELOG.md` stale at 2.4.8; upgrade docs per minor.
- awscli v2: no releases; `.changes/<ver>.json` on branch v2 is best. claude-code: `CHANGELOG.md` 1.18 MB.
- python/go/temurin: no GitHub releases. Use docs.python.org whatsnew, go.dev, api.adoptium.net.
- GitHub contents API: `encoding: none` above 1 MB → use the raw Accept header. raw.githubusercontent.com is throttled since 2025-05-08.

## Algorithms borrowed
- Renovate `lib/workers/repository/update/pr/changelog`: changelog filename regex, heading-level selection, release-to-version matching, age-based cache TTLs, GraphQL page-size backoff.
- Dependabot `metadata_finders/base`: file priority list, `changelog_pruner` version-line detection, UPGRADE.md on major bumps.

## Enrichment coverage (checked live)
- endoflife.date v1: terraform, opentofu, kubernetes, argo-cd, nodejs, python, go, eclipse-temurin (not: terragrunt, helm, awscli, claude-code, mise, gh, k9s, uv, ruff).
- OSV: hits for terraform 2, helm/v3 16, kubernetes 23, argo-cd/v2 41, opentofu 16, terragrunt 4, gh 16, uv 9, awscli 5, claude-code 25; k9s/ruff 0.

## Language decision inputs
See [engineering ADR-0001](../engineering/adr/0001-language-go.md). Rust stack survey (jdx moved to usage-rs; nobody uses octocrab; mise not linkable) and Go stack survey (cobra, go-github v93, goreleaser) are summarized in [engineering/stack.md](../engineering/stack.md).
