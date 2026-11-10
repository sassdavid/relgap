# CI, release & distribution

## Local tasks (mise)

`mise.toml` pins `go`, `golangci-lint`, `goreleaser`, `gofumpt`, `govulncheck`, `cosign` and `syft` (checksums in
`mise.lock`), and defines these tasks:

| Task | Runs |
|---|---|
| `mise run check` | lint + test + vuln. **This is the single verification command in CLAUDE.md** |
| `mise run lint` | `lint:go` (golangci-lint), `lint:prose` (`.claude/rules/writing.md` checks) and `lint:actions` (actionlint) |
| `mise run test` | `go test -race -count=1 ./...` |
| `mise run test:golden -- -update` | regenerates golden files (review the diff!). Sets `GOLDEN_UPDATE=1`, which `internal/testutil.Golden` and testscript's `UpdateScripts` honour |
| `mise run fuzz` | runs every Go fuzz target (`func FuzzXxx`) for 60 s each; `mise run fuzz -- 10s` sets another duration. Passes when a repo has no targets |
| `mise run docs` | regenerates `docs/cli/` via cobra/doc and fails if the checked-in copy differs |
| `mise run snapshot` | `goreleaser release --snapshot --clean --skip=sign`. Keyless cosign needs CI's OIDC token, and goreleaser OSS can't make `signs:` conditional, so local snapshots skip signing |

## GitHub Actions workflows

| Workflow | Trigger | Jobs |
|---|---|---|
| `ci.yml` | PR, push main | lint; test matrix (ubuntu, macos, windows); fuzz (`mise run fuzz`); docs-up-to-date; schema tests |
| `security.yml` | PR, weekly | govulncheck (SARIF), zizmor, dependency review |
| `live.yml` | nightly, manual | live corpus against real GitHub (token secret) |
| `release-please.yml` | push main | opens or updates the release PR (version bump + CHANGELOG.md from Conventional Commits). On merge it creates the tag and a draft release (`draft` + `force-tag-creation`) |
| `release.yml` | called by `release-please.yml` when a release is created (tags pushed with `GITHUB_TOKEN` don't trigger workflows); also tag `v*` | goreleaser: binaries, checksums, SBOM, cosign keyless signature, attestations, Homebrew cask, Scoop. Secrets: `HOMEBREW_TAP_TOKEN`, `SCOOP_BUCKET_TOKEN` |

Rules:
- Actions are pinned by commit SHA (Renovate updates them).
- `permissions:` start at `{}` and each job gets only what it needs. Release gets `id-token: write`, `attestations: write` and `contents: write`.
- Release tags are created only by release-please, never by hand.

## goreleaser essentials

- Targets: `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, `windows/{amd64,arm64}`, built with `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X github.com/sassdavid/relgap/internal/buildinfo.version=..."` (`main` only calls `cli.Run`, so the ldflags variables live in `internal/buildinfo`, per ADR-0008 #2).
- `sboms:` via syft; `signs:` cosign `sign-blob --bundle` on the checksum file.
- `release.use_existing_draft` finds release-please's draft by name (`{{ .Tag }}`), uploads the assets, then publishes it. `releases/latest` never points at a release without assets, and this works with immutable releases.
- `homebrew_casks:` (replaces `brews:`) publishes to the tap `sassdavid/homebrew-tap`. Cask and Scoop uploads are skipped when their token secret is missing, and attestation runs even if a publisher fails.
- `scoops:` for Windows. `nfpms:` (deb/rpm) is optional and comes later.

## Distribution order

1. GitHub Releases (binaries + `install.sh`, verified with checksums).
2. Homebrew tap.
3. `mise use github:sassdavid/<tool>` works on day one with no registry entry.
4. aqua-registry PR (`aqua:sassdavid/<tool>`), followed by a mise registry PR (`registry/<tool>.toml`) once there's some adoption.
5. `go install github.com/sassdavid/<tool>/cmd/<tool>@latest`, documented as unsigned.

## Versioning

- SemVer. In `0.x`, a minor release may break things, and each break is called out in the changelog under "⚠ BREAKING".
- The JSON output and config schemas carry their own `schema_version` that changes independently of the binary version.
- Support policy: the latest minor only, until 1.0.
