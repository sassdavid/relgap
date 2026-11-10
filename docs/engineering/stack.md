# Go stack

**Every version below was checked on 2026-11-09:**
- Go modules: queried `proxy.golang.org/<module>/@latest`, and probed `/vN+1` to catch new major versions.
- Go toolchain: `go.dev/dl`.
- GitHub Actions: their GitHub releases.

**Re-checked on 2026-11-10 at M0** (same method): every pinned version below was still the latest.
Re-run the check right before M0 (script: [§ Refresh](#refresh)). After M0, Renovate keeps everything current,
with a 3-day minimum release age (matching your mise `minimum_release_age` habit).
Choices are justified in the [ADRs](adr/). Survey basis: go.mod files of gh, terragrunt, opentofu, trivy, syft, aqua,
goreleaser, chezmoi, k9s, golangci-lint.

## Toolchain (pinned in `mise.toml`)

| Item | Version | Note |
|---|---|---|
| Go | **1.27.3** toolchain; `go 1.27.0` directive per [version-policy.md](version-policy.md) | same as tfcensus |
| golangci-lint | **v2.15.0** | + golden config v2.15.0 ([ADR-0008](adr/0008-lint-config.md)) |
| gofumpt | **v0.12.0** | golines **v0.13.0** + goimports come with the golden config |
| govulncheck (`golang.org/x/vuln`) | **v1.9.0** | |
| go-licenses (`github.com/google/go-licenses/v2`) | **v2.0.1** | collects third-party notices for the archives and fails on forbidden, restricted or unknown licenses ([ADR-0012](adr/0012-third-party-notices.md)); gh pins the same version |
| goreleaser | **v2.20.0** | cosign **v3.1.4** |
| syft | **v1.57.1** | SBOMs for goreleaser `sboms:`, locally and in CI (installed by mise, so `anchore/sbom-action` isn't needed) |
| mise | **v2026.11.5** | pinned as `version:` of `jdx/mise-action` in workflows; `mise.lock` pins tool checksums |
| zizmor | **v1.32.0** | local workflow audit (`mise x zizmor@1.32.0 -- zizmor .github/workflows`); CI uses `zizmor-action`. `.github/zizmor.yml` turns off the `self-repository` audit until actionlint accepts `$/` |
| actionlint | **v1.7.12** | `mise run lint:actions`. Doesn't parse `uses: $/...` yet (rhysd/actionlint#711), so workflows use `./` |

## Libraries (core)

| Concern | Module | Latest | Why / alternatives rejected |
|---|---|---|---|
| CLI | `github.com/spf13/cobra` (+ `spf13/pflag`) | v1.10.2 (+ v1.1.0) | [ADR-0002](adr/0002-cli-cobra.md) |
| Config | `github.com/knadh/koanf/v2` + `providers/env/v2` v2.0.2, `providers/posflag` v1.0.2 | v2.4.1 | [ADR-0003](adr/0003-config-koanf.md). `providers/file` v1.2.1 is not used: files are decoded with yaml v4 (node positions for file:line errors) and handed to koanf as maps |
| XDG paths | `github.com/adrg/xdg` | v0.5.3 | stable; used by aqua, syft, k9s |
| Logging | `log/slog` + `github.com/lmittmann/tint` | stdlib / v1.2.1 | zap rejected ([ADR-0004](adr/0004-logging.md)) |
| Concurrency | `golang.org/x/sync` (errgroup) | **v0.25.0** | |
| HTTP retry | `github.com/hashicorp/go-retryablehttp` | v0.7.8 | latest; used by terraform, tofu, trivy |
| Rate limit | `golang.org/x/time` | v0.16.0 | |
| HTTP cache | own ETag layer (default) / `github.com/bartventer/httpcache` | — / v0.14.0 | decide at relgap M4; `gregjones/httpcache` is archived |
| Compression | `github.com/klauspost/compress` (zstd cache files) | v1.20.4 | |
| Styling | `charm.land/lipgloss/v2` (+ `/table`) | **v2.1.0** (released today) | latest at M0; gh/terragrunt/goreleaser are on v2.0.x |
| Markdown to terminal | `charm.land/glamour/v2` | v2.0.1 | |
| TTY | `github.com/mattn/go-isatty` | v0.0.24 | |
| Prompts | stdlib `bufio` → `charm.land/huh/v2` later | — / v2.0.3 | [ADR-0007](adr/0007-interactive-prompts.md) |
| JSON Schema validate | `github.com/santhosh-tekuri/jsonschema/v6` | v6.0.3 | |
| JSON Schema generate | `github.com/invopop/jsonschema` | v0.14.0 | config struct → schema; test-only (the schema sync test generates `schemas/config-v1.json`) |
| YAML | `go.yaml.in/yaml/v4` | **v4.0.0** (2026-11-04) | YAML org's maintained successor of `gopkg.in/yaml.v3`. If v4.0.0 shows early bugs at M0, fall back to `go.yaml.in/yaml/v3` v3.0.6 |
| TOML | `github.com/pelletier/go-toml/v2` | v2.4.3 | reads mise's `registry/*.toml` in `scripts/regsnapshot` |
| MCP | `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | [ADR-0011](adr/0011-mcp-go-sdk.md) |

## Testing

| Concern | Module | Latest |
|---|---|---|
| Assertions | `github.com/stretchr/testify` | v1.12.1 |
| Diffs | `github.com/google/go-cmp` | v0.7.0 |
| CLI scripts | `github.com/rogpeppe/go-internal` (testscript) | v1.16.0 |
| Mocks | `go.uber.org/mock` | v0.6.0 |
| HTTP record/replay | `gopkg.in/dnaeon/go-vcr.v4` | v4.0.7 |

## relgap domain libraries

| Concern | Module | Latest | Note |
|---|---|---|---|
| GitHub REST | `github.com/google/go-github/v93` | v93.0.0 | no v94 yet |
| GitHub GraphQL | `github.com/shurcooL/githubv4` | v0.0.0-20260209031235-2402fdf4a9ed | pseudo-versions only (normal for this module) |
| GitHub auth | `github.com/cli/go-gh/v2` | v2.17.0 | `pkg/auth` |
| Markdown AST | `github.com/yuin/goldmark/v2` | **v2.1.15** | stable since v2.0.0 (2026-08-27); GitHub "Latest" release; 29 v2 releases. Every node carries a start position, which is what mdindex needs. The README's "early stages" note concerns third-party extensions only, and relgap uses none ([relgap ADR-0001](../plan/adr/0001-goldmark-section-index.md)) |
| HTML to Markdown | `github.com/JohannesKaufmann/html-to-markdown/v2` | v2.5.2 | web-doc adapter |
| Versions | `github.com/Masterminds/semver/v3` + `github.com/hashicorp/go-version` | v3.5.0 + v1.9.0 | go-version only parses the `loose` scheme. relgap compares loose versions itself because go-version's `Compare` isn't transitive (`FuzzCompareProperties` found `"0" <= "0.0.0.0" <= "0A"` but `"0" > "0A"`) |
| Terraform pins | `github.com/hashicorp/terraform-config-inspect` | v0.0.0-20260904064934-75d64de68c31 | pseudo-versions only |
| HCL | `github.com/hashicorp/hcl/v2` | v2.25.0 | |
| Dockerfile | `github.com/moby/buildkit` (`frontend/dockerfile/parser`) | v0.34.2 | **heavy module**. At M5, measure the binary and `go.sum` impact. If it's over 3 MB or pulls in many dependencies, use a ~150-line hand-written parser for FROM/ARG/ENV |
| LLM (Bedrock) | `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` | v1.63.12 | only for `--summarize` with the bedrock provider |

**Never import:** `hashicorp/terraform` (BSL 1.1), terragrunt `internal/...`, or OpenTofu `internal/` ([ADR-0010](adr/0010-license-apache2.md); background in [tfcensus ADR-0001](https://github.com/sassdavid/tfcensus/blob/main/docs/plan/adr/0001-shell-out-to-terragrunt.md)).

## GitHub Actions (pin by commit SHA; versions as of 2026-11-09)

| Action | Latest |
|---|---|
| actions/checkout | v7.0.1 |
| actions/setup-go | v7.0.0 (or `jdx/mise-action` v5.2.7, which installs the whole mise toolchain; preferred) |
| golangci/golangci-lint-action | v9.3.0 |
| goreleaser/goreleaser-action | v7.3.0 |
| sigstore/cosign-installer | v4.1.3 |
| actions/attest | v4.2.2 |
| anchore/sbom-action | v0.24.3 |
| googleapis/release-please-action | v5.0.0 |
| github/codeql-action (SARIF upload) | v4.39.3 (bundle v2.27.4) |
| actions/dependency-review-action | v5.0.0 |
| zizmorcore/zizmor-action | v0.8.1 |

Indirect modules are whatever MVS selects, except where govulncheck flags a known (even unreachable)
vulnerability: at M0, `golang.org/x/text` went from v0.14.0 to v0.43.0 and `golang.org/x/sys` from
v0.28.0 to v0.49.0 (the newest releases older than the 3-day minimum age).

| Concern | Module | Version | Why |
|---|---|---|---|
| Windows console colors | `golang.org/x/sys` (`windows`) | v0.49.0 | turns on VT processing so tint's ANSI colors render in conhost; already required by isatty |

## Refresh
The checker used for this table is in the session scratchpad. Re-create it in the repo as `scripts/latest-deps.py`
(it queries `proxy.golang.org/<module>/@latest` and probes `/vN+1`), or simply run `go get -u ./... && go list -m -u all`
once go.mod exists.
