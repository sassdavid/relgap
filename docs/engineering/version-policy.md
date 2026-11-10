# Supported versions of external tools

This is a brand-new tool with no existing users, so **we don't chase old versions**. Features that need newer
tools are **gated** by detected version (reported as "unavailable: needs ≥X"), never crashes.
This was researched on 2026-11-09 from release notes, CHANGELOGs, upstream support policies and comparable tools'
CI matrices.

## The policy at a glance

| Tool | Hard floor | Gated features | CI test matrix | Raise the floor when |
|---|---|---|---|---|
| **Go** (building) | `go 1.26.0` directive (no patch pin, so `go install` works on both supported Go minors); release builds use toolchain **1.27.3** | — | 1.26 + 1.27 | Go 1.28 ships (~Feb 2027) → `go 1.27.0` |
| **mise** | none required. If mise is absent or older than **v2025.12.7** (`registry --json`), relgap parses `mise.toml` and uses the embedded registry snapshot | `release_url` from `outdated --json` ≥ **v2026.8.15** (the key is *omitted* when unknown, so check `mise version --json`). Below that, relgap builds the URL from registry data + the GitHub API | latest mise weekly (mise ships about 5 releases a week) | — |
| **gh** (token discovery) | none. go-gh's `auth.TokenForHost` reads env, config and keyring; `gh` isn't required | — | — | — |

## Rules for implementation
- **Detect once per run:** `mise version --json`. Cache the result in `Deps`, and have `doctor` print it with each gated feature's status.
- **Parse JSON leniently:** ignore unknown fields; mise publishes no JSON stability policy ([relgap ADR-0003](../plan/adr/0003-integrate-with-mise.md)).
- Patch numbers aren't contiguous (OpenTofu skipped 1.13.3–1.13.7). Compare versions, never count them.
