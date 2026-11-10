# relgap: milestones

**One milestone = one fresh Claude Code session.** Start each session with:
"Read `CLAUDE.md`, then `docs/plan/milestones.md` M<n> and the linked specs. Implement M<n> only."
Tick a box only when its check has passed, and quote the evidence (command plus result) in the commit body.

Release mapping: **v0.1.0 = M0–M8** (MVP), v0.2 = M9–M10, v0.3 = M11–M12.

---

### M0: Skeleton and pipeline
Scope: repo, `cmd/relgap`, cobra root with `version` and `completion`, koanf config + `config show|path|validate`, slog/tint, `Deps` struct, render package stub, mise tasks, golden lint config, CI, goreleaser snapshot, release-please.
- [ ] `mise run check` passes on a clean clone (lint + `go test -race` + govulncheck).
- [ ] `relgap version --format json` validates against `schemas/report-v1.json` (envelope only).
- [ ] `goreleaser release --snapshot --clean` produces 6 archives, checksums and an SBOM.
- [ ] CI is green on ubuntu, macos and windows. Actions are pinned by SHA and zizmor is clean.

### M1: Versions and resolution → [versions](specs/versions-and-ranges.md), [resolution](specs/tool-resolution.md)
Scope: `version` schemes, embedded mise and aqua snapshots + `registry:snapshot` task, override registry loader + 5 seed entries, `relgap resolve`.
- [ ] All acceptance criteria in versions-and-ranges.md, plus resolution criteria 1, 2 and 4.
- [ ] Fuzz targets run for 60 s in CI without failures.

### M2: GitHub client and `gh-release` digest → [adapters](specs/source-adapters.md)
Scope: `httpx` (retry, rate, no cache yet), go-github + githubv4 batch, token discovery, `gh-release` adapter, minimal classifier (heading rules only), `digest` with tty/plain/json.
- [ ] `relgap digest terraform 1.15.8..1.17.1 --format json` matches the golden file (cassette).
- [ ] Without a token: a single warning, concurrency drops to 2, the result is still correct.
- [ ] Ctrl-C mid-digest exits 130 within 1 s (testscript with a slow fake server).

### M3: `changelog-file`, section index, full classifier → [classification](specs/classification.md)
- [ ] mdindex parses each file once (request counter test). kubernetes, claude-code and node fixtures are golden.
- [ ] All classification.md criteria. The precision check over the labelled set passes ≥ 0.9.
- [ ] `md` renderer golden output for 3 tools.

### M4: Cache, ETag, offline
- [ ] A second identical digest makes 0 non-304 requests (counter). `--offline` works with the network disabled (testscript sets a bogus proxy).
- [ ] Age-based TTLs are unit-tested with an injected clock. `cache info|clear|prune` are covered.

### M5: Pin discovery → [pin-discovery](specs/pin-discovery.md)
- [ ] All pin-discovery.md criteria. `relgap pins` works against a dotfriedrice-shaped fixture.

### M6: `outdated` + `digest --from-mise`
- [ ] Fixture project with 10 pins: correct table in tty/md/json, with breaking counts matching the full digests.
- [ ] Uses at most 2 GraphQL queries for "latest version" of 45 tools (counter).

### M7: `drift`
- [ ] A generated tree of 60 repos gives a correct matrix, constraint pins marked `≈`, `--fail-on drift` exits 3.
- [ ] Offline by default. `--latest` adds the latest column.

### M8: Remaining adapters, enrichment, curated registry → v0.1.0
Scope: `structured-json`, `web-doc`, `upgrade-guide` adapters, EOL + OSV, all 20 curated registry entries, `registry lint|test`, live corpus job.
- [ ] Every enrichment.md criterion, plus source-adapters criteria 2, 4 and 5.
- [ ] The nightly live corpus (30 tools) passes once on main.
- [ ] First tagged release v0.1.0: Homebrew cask installs, `mise use github:sassdavid/relgap` works, `cosign verify-blob` passes.

### M9: SARIF + `--fail-on` matrix → [output-and-ci](specs/output-and-ci.md)
- [ ] output-and-ci.md criteria 1–3.

### M10: MCP server → [mcp-and-llm](specs/mcp-and-llm.md)
- [ ] MCP criteria 1–2. Registered in the author's Claude Code config and used for a real upgrade (terraform or argocd), notes recorded in `docs/dogfood.md`.

### M11: GitHub Action (Renovate PR enricher)
- [ ] The action posts or updates one sticky comment on a test repo's Renovate PR. A SARIF upload job works.

### M12: LLM summary, `--binaries`, polish
- [ ] mcp-and-llm.md criteria 3–4. enrichment `--binaries` criterion.
- [ ] Submit aqua-registry PR; mise registry PR once there are ≥ 3 external users.
