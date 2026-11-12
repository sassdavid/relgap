# Spec: enrichment (EOL and vulnerabilities)

`relgap outdated` adds both to every compared pin: the EOL and VULNS columns, and `eol` and
`vulns` in `--format json` (`schemas/outdated-v1.json`). Code: `internal/enrich`. Caching and the
library choices: [ADR-0011](../adr/0011-enrichment-and-web-sources.md).

## End of life: endoflife.date API v1
- Endpoint: `GET https://endoflife.date/api/v1/products/{product}`. Each release cycle has
  `name`, `isEol`, `eolFrom` (null while no date is announced), `isMaintained` and
  `latest.name`. An unknown product returns 404 with an HTML body. The `/api/v1/identifiers/purl`
  endpoint maps `pkg:github/hashicorp/terraform` to terraform, but automatic mapping is left for
  later: only registry entries name products.
- Product mapping comes from the registry `eol.product` and `cycle` template. Templates, checked
  on 2026-11-12 against each product's cycle names:

  | Product | Tools | Cycle template | Example cycle |
  |---|---|---|---|
  | terraform | terraform | `{major}.{minor}` | 1.17 |
  | opentofu | opentofu | `{major}.{minor}` | 1.13 |
  | kubernetes | kubectl | `{major}.{minor}` | 1.37 |
  | argo-cd | argocd | `{major}.{minor}` | 3.6 |
  | go | go | `{major}.{minor}` | 1.27 |
  | python | python | `{major}.{minor}` | 3.15 |
  | nodejs | node | `{major}` | 26 |
  | eclipse-temurin | temurin | `{major}` | 21 |

- Not covered: terragrunt, helm, awscli, claude-code, mise, gh, k9s, uv, ruff, golangci-lint,
  tflint, gitleaks, atlascli. The `eol` field is absent for those, and no request is sent.
- Cached for 1 day (ETag).
- Report: `eol: {product, current, target}`, each a `{version, cycle, is_eol, eol_from,
  latest_in_cycle}` for the current pin and for the latest version.
- A cycle endoflife.date doesn't list (older or newer than its list) has no `is_eol` and a
  note, "endoflife.date doesn't list cycle 1.9 of kubernetes". The table shows `?`, the notes list
  it, and `--fail-on eol` doesn't match it.
- An outage (any failed request, such as a 503 or a 403) is a warning on stderr and an entry in
  the JSON envelope's `errors`, and the report continues without `eol` for the tools that need
  it. Under `--fail-on eol` the run exits 1, since the gate can't pass on partial data. An eol
  match on another pin still exits 3 ([ADR-0012](../adr/0012-fail-on-gate-and-sarif.md)).

## Vulnerabilities: OSV
- `POST https://api.osv.dev/v1/querybatch` with `{package:{ecosystem,name}, version}` for each
  tool's current pin, at most 1000 queries per request, following each result's
  `next_page_token`. The batch returns only IDs; records come from `GET /v1/vulns/{id}`, each
  fetched once per run. Both are cached for 1 day.
- The registry `osv` lists packages. Go modules need the major suffix (`helm.sh/helm/v3`,
  `github.com/argoproj/argo-cd/v3`). A Go path ending in `/vN` is queried only for versions of
  major N. A path without a suffix is queried for majors 0 and 1 when the list also has suffixed
  paths, and for every major otherwise.
- The query version is the version itself, in OSV's semver form for Go (`go1.27` becomes
  `1.27.0`). For `ecosystem: GIT` the name is the repository URL and the version is the tag:
  node and python have OSV data only as GIT ranges (`https://github.com/python/cpython` at
  `v3.14.0`).
- Hit counts checked on 2026-11-12: k8s.io/kubernetes 87, argo-cd/v2 86 and /v3 22, helm/v3 37
  and /v4 5, opentofu 18, cli/cli/v2 18, terraform 4, terragrunt 4, terraform-provider-aws 1,
  npm `@anthropic-ai/claude-code` 30, PyPI uv 11, crates.io mise 6, Go stdlib 182. None for k9s,
  ruff, tflint, kubeconform, golangci-lint and gitleaks. PyPI's awscli is v1 only, so awscli has
  no OSV package.
- Installed-binary mode (`outdated --binaries`):
  - For each pinned tool, relgap looks for executables named in the registry `bin` (default: the
    tool name; awscli is `aws`, claude-code is `claude`, opentofu is `tofu`). It searches the PATH
    first, then `$MISE_DATA_DIR/installs/<tool>/<version>/` and its `bin/`
    (`$XDG_DATA_HOME/mise`, else `~/.local/share/mise`). It skips version aliases, which are
    symlinks such as `latest` or `1.17`. A mise shim is followed with `mise which <name>`.
  - It reads `debug/buildinfo`, which gives the Go version and main module version, and queries
    OSV for `stdlib@<go version>` plus the main module. A `(devel)` main module (no VCS stamp) is
    left out. A non-Go executable is skipped.
  - This catches "terraform 1.15.8 was built with Go 1.24.x, which has N stdlib CVEs", the way
    govulncheck's binary mode does, minus symbol reachability. The output says so explicitly.
- Report: `vulns: [{id, aliases, summary, package, severity, fixed_in, affects_current,
  fixed_by_target}]`, sorted by score, then ID. Records that are aliases of each other
  (GO-2023-2055 and GHSA-h626-pv66-hhm7) are kept once, with the severity of the one that has it.
- `severity` is the highest CVSS base score among the record's vectors (v2, v3.0, v3.1 or v4.0)
  with its rating (none, low, medium, high, critical), as osv-scanner computes it. Without a
  vector it is the advisory's own rating (GitHub's MODERATE becomes medium). Go vulnerability
  database records have neither, so their `severity` is absent.
- `fixed_by_target` follows the OSV schema, evaluated locally from the record:
  - A version is affected when it is in the package's `versions` list or inside a range: the
    events in version order, each `introduced` at or below the version switching it in, each
    `fixed` (or `limit`) at or below it switching it out, and `last_affected` below it switching it
    out.
  - SEMVER ranges compare as semver; for Go both sides are in OSV's semver form. ECOSYSTEM ranges
    compare with PEP 440 for PyPI and semver for npm and crates.io. GIT ranges use the `versions`
    list.
  - `affects_current` is true when the current version is affected, or when relgap can't decide,
    since OSV returned the record for that version.
  - `fixed_by_target` is true when the current version is affected and the target isn't. It is
    absent without a target or when a range that could hold the target can't be compared and no
    `versions` list covers it.
  - `fixed_in` is the lowest `fixed` event above the current version.
- `--fail-on vuln` counts the advisories with `affects_current` at or above `--vuln-severity`
  (config `policy.vuln_severity`; empty counts every advisory, and an advisory without a
  rating counts only then). An OSV outage under `--fail-on vuln` exits 1 when nothing matched.

## Acceptance criteria
- [x] Cassette tests: terraform cycle EOL, a tool with no EOL product (field absent, no error), and an endoflife.date outage (warning, report continues). (`TestSpec_enrichment_1`: cassettes `enrich/eol-terraform.yaml` and the hand-written `enrich/eol-outage.yaml`, a 503.)
- [x] `outdated --fail-on eol` exits 3 on a fixture pinned to an EOL kubernetes minor. (`TestSpec_enrichment_2`: kubectl 1.34.13 exits 3, 1.37.2 exits 0, an endoflife.date 403 warns and exits 1 under `--fail-on eol`.)
- [x] `--binaries` on a fixture Go binary built in the test (`go build` of a tiny module with a known Go version) produces the expected stdlib query. (`TestSpec_enrichment_3`: built with `GOTOOLCHAIN=local GOPROXY=off GOWORK=off GOFLAGS='-mod=mod -buildvcs=false' CGO_ENABLED=0`; the expected version comes from `go env GOVERSION`.)
- [x] `fixedByTarget` is computed correctly for a vuln with `fixed` events in OSV ranges (semver and Go ecosystems). (`TestSpec_enrichment_4`: kubernetes 1.33.0 and terraform 1.5.0 on Go, claude-code 1.0.0 on npm; for every vuln the local answer equals OSV's own query for the target.)
