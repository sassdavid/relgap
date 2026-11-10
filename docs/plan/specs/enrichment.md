# Spec: enrichment (EOL and vulnerabilities)

## End of life: endoflife.date API v1
- Endpoints: `/api/v1/products/{product}` (cycles with `isEol`, `eolFrom`, `isMaintained`, `latest`) and `/api/v1/identifiers/purl/...` for automatic mapping.
- Product mapping comes from the registry `eol.product` + `cycle` template (`{major}.{minor}` for kubernetes/terraform, `{major}` for node/java).
- Coverage today: terraform, opentofu, kubernetes (also used for kubectl), argo-cd, nodejs, python, go, eclipse-temurin. Not covered: terragrunt, helm, awscli, claude-code, mise, gh, k9s, uv, ruff. The `eol` field is just absent for those.
- Cached for 1 day (ETag).
- Report: `EOL{cycle, isEol, eolFrom, latestInCycle}` for both the **current** pin and the target.

## Vulnerabilities: OSV
- `POST https://api.osv.dev/v1/querybatch` with `{package:{ecosystem,name}, version}` for each tool's current pin; details via `/v1/vulns/{id}` (cached 1 day).
- Registry `osv` lists packages. Go modules need the major suffix (`helm.sh/helm/v3`, `github.com/argoproj/argo-cd/v3`).
- Verified hit counts: terraform 2, helm/v3 16, kubernetes 23, argo-cd/v2 41, opentofu 16, terragrunt 4, gh 16, uv (PyPI) 9, awscli (PyPI) 5, claude-code (npm) 25.
- **Installed-binary mode** (`--binaries`):
  - For each tool found on PATH (or in mise's install dir), read `debug/buildinfo`, which gives the Go version and module versions.
  - Then query OSV for `stdlib@<go version>` plus the main module. This catches "terraform 1.15.8 was built with Go 1.24.x, which has N stdlib CVEs", the way govulncheck's binary mode does, minus symbol reachability. The output says so explicitly.
- Report: `Vuln{id, aliases, summary, severity (CVSS if present), fixedIn, affectsCurrent, fixedByTarget}`. `fixedByTarget` tells you whether upgrading to the digest target fixes it.

## Acceptance criteria
- [ ] Cassette tests: terraform cycle EOL, a tool with no EOL product (field absent, no error), and an endoflife.date outage (warning, report continues).
- [ ] `outdated --fail-on eol` exits 3 on a fixture pinned to an EOL kubernetes minor.
- [ ] `--binaries` on a fixture Go binary built in the test (`go build` of a tiny module with a known Go version) produces the expected stdlib query.
- [ ] `fixedByTarget` is computed correctly for a vuln with `fixed` events in OSV ranges (semver and Go ecosystems).
