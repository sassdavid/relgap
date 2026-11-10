# relgap ADR-0003: Integrate with mise by shelling out, not by competing or linking

- Status: accepted (2026-11-09)

## Context
- jdx/mise discussion #9708 asked for in-between release notes. mise added only `release_url` (target version only) to `mise outdated --json` (v2026.8.15, #12494) and called the rest "unstarted rather than planned".
- The mise crate is not usable as a library (`doc = false`, no API stability, very heavy).

## Decision
- When `mise` is on PATH, use `mise ls --json --current`, `mise outdated --json` and `mise registry --json` as inputs. Otherwise fall back to parsing `mise.toml` and the embedded registry snapshot.
- Position relgap as a mise companion. The README shows `mise outdated` → `relgap digest --from-mise`. Optionally document a mise task alias: `[tasks.upgrade-notes] run = "relgap digest --from-mise --only breaking,deprecation"`.
- Offer upstream, without depending on it: a doc PR to mise's "Tips" page once relgap is stable.

## Versions
- `mise registry --json` exists since v2025.12.7, and `outdated --json` has `release_url` since v2026.8.15 (the key is omitted when unknown).
- The version is detected via `mise version --json`. Older or missing mise falls back to file parsing + the embedded snapshot ([engineering/version-policy.md](../../engineering/version-policy.md)).
- mise publishes no JSON stability policy, and its changes so far have been additive. So decode leniently and run a weekly CI job against mise latest.

## Consequences
- Our output tracks mise's JSON shape, so a contract test runs against a pinned mise version in CI and fails loudly on shape changes.
- If mise ever ships in-between notes, relgap still offers multi-repo drift, non-mise sources (Docker, TF, CI), classification, EOL/OSV, CI gates and MCP.
