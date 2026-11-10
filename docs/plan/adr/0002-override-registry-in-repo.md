# relgap ADR-0002: Community override registry lives in the main repo, embedded and refreshable

- Status: accepted (2026-11-09)

## Context
Correct sources are relgap's moat: terraform's changelog lives on release branches, kubernetes in per-minor
files, awscli in `.changes/*.json`, and argocd's `CHANGELOG.md` is stale. mise (`registry/*.toml`, one file per
tool) and aqua (`pkgs/**/registry.yaml`) show that one-file-per-tool registries attract contributions.

## Decision
- `registry/<tool>.yaml` lives in the relgap repo, validated by `schemas/registry-v1.json` and `relgap registry lint` in CI.
- The registry is **embedded** at build time (`embed.FS`), so it works offline and is reproducible per binary version.
- `relgap registry update` fetches newer entries from the default branch (ETag-cached) into the cache dir. Newer entries win when their `min_relgap` field ≤ the running version.
- A PR template asks for `relgap registry test <tool> <range>` output as evidence.

## Alternatives rejected
- Separate registry repo: more ceremony for v0.x. Revisit at 100+ entries.
- Contributing into mise's or aqua's registry: those registries describe *installation*, not changelogs. A PR to add changelog fields there would mix concerns, and maintainers would likely reject it.

## Consequences
- Fixing a tool's sources doesn't require a binary release.
- Entries stay small: only what auto-discovery gets wrong.
