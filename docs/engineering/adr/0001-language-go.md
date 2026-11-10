# ADR-0001: Go for both tools

- Status: accepted (2026-11-09)
- Applies to: relgap, tfcensus

## Context
The author started out preferring Rust but was open to Go. We compared both for each tool, using the
dependency manifests of about 40 maintained CLIs (jdx/mise, uv, ruff, ripgrep, ccusage on the Rust side; gh,
terragrunt, opentofu, tfmigrate, trivy, aqua, goreleaser on the Go side).

## Decision
Both tools are written in **Go**.

## Reasons
**tfcensus (clear):**
- The libraries it needs exist only in Go: `terraform-json`, `terraform-exec`, `hcl/v2` with full expression evaluation and `hclwrite`, `go-cty`.
- In Rust, `hcl-rs` parses HCL but has no Terraform function library and no tfexec or plan-JSON equivalent.
- Every tool in this space is Go: terragrunt, opentofu, tfmigrate, tfautomv, atlantis, terramate, driftive, tgsieve. So the contributor pool already knows the language.
- `aws-sdk-go-v2` has far more examples, and SSO/`sso-session` behaviour is proven at scale (steampipe, trivy).

**relgap (closer call, still Go):**
- Mise alignment, Rust's main pull, is weak. mise can't be linked as a library (`doc = false`, no stable API, a huge dependency tree), so integration is `mise ls|outdated|registry --json` in either language.
- Go has official parsers for pin discovery: buildkit's Dockerfile parser, `terraform-config-inspect` for `required_version`/`required_providers`, `hcl/v2` for `.terraform.lock.hcl`.
- Mature GitHub clients: `go-github` (v93) and `githubv4`. In Rust, nobody uses `octocrab` (pre-1.0); mise hand-rolls about 2.5k lines.
- stdlib `debug/buildinfo` reads the Go version and modules inside installed DevOps binaries (terraform, helm, kubectl, gh…), which enables OSV checks against the actual binary.
- One language means one stack, one lint/CI/release template, and shared conventions between the two tools.
- Fast compiles shorten the edit-test loop, which matters when Claude Code does most of the implementation.

## Consequences
- Binaries are larger than Rust equivalents: relgap ≈ 15–20 MB, tfcensus ≈ 40–60 MB with the AWS clients (estimate). Acceptable.
- No `toml_edit`-style format-preserving TOML edits. That's fine, because rewriting pins is a non-goal (Renovate does it).
- **We'd revisit (relgap only)** if it ever needs to run inside mise as a library or plugin. That would be a different product.
