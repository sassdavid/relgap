# Spec: pin discovery (scanner + mise input)

Produces `[]Pin` (tool, raw name, value, exact?, location) from a project or from many repo roots.
Used by `pins`, `outdated` and `drift`.

## Sources of pins

| Detector | Files | Extracts | Notes |
|---|---|---|---|
| `mise` | via `mise ls --json --current` (preferred), else parse `mise.toml`, `.mise.toml`, `mise.*.toml`, `conf.d/*.toml` | tool → version | exec mise only if it's on PATH; otherwise parse with go-toml. `[tools]` values may be strings, arrays or tables `{version=…}` |
| `tool-versions` | `.tool-versions` | asdf pins | |
| `aqua` | `aqua.yaml`, `aqua/*.yaml` | `packages[].name@version` | Renovate has no aqua manager, which is a differentiator |
| `idiomatic` | `.terraform-version`, `.terragrunt-version`, `.node-version`, `.nvmrc`, `.python-version`, `.go-version`, `go.mod` `toolchain`/`go` | single version | |
| `terraform` | `*.tf` via terraform-config-inspect | `required_version` (constraint), `required_providers` (`tfprovider:` pins) | constraints are recorded as `Exact=false` |
| `tf-lock` | `.terraform.lock.hcl` via hcl/v2 | exact provider versions | the *real* provider version in use |
| `terragrunt` | `terragrunt.hcl`, `root.hcl`, `*.hcl` | `terraform_version_constraint`, `terragrunt_version_constraint` | static attributes only, no evaluation |
| `dockerfile` | `Dockerfile*`, `*.Dockerfile`, `Containerfile` | `FROM image:tag` (`image:` pins), `ARG/ENV <NAME>_VERSION=x` | `<NAME>` maps to a tool via registry aliases (`TERRAFORM_VERSION` → terraform). Honour `# renovate: datasource=… depName=…` comments above an ARG (exact mapping) |
| `ci` | `.github/workflows/*.y*ml`, `bitbucket-pipelines.yml`, `.gitlab-ci.yml` | `image:` tags, `uses: owner/action@vX`, `env: *_VERSION` | actions are reported as kind `action` (opt-in) |
| `helm` | `Chart.yaml` | `dependencies[].version` (`chart:` pins), `appVersion` | |
| `generic` | any text file listed in config `scan.extra_files` | regex `(?i)\b([a-z0-9_]+)_VERSION\s*[:=]\s*["']?v?([0-9][^\s"']*)` | opt-in; maps names via aliases |
| `renovate` (input) | `--renovate-report <file.json>` | `packageFiles[].deps[]` from `renovate --platform=local --dry-run=extract` | optional, adds coverage for anything Renovate understands |

## Walking repos
- `drift` roots are directories. Every directory containing `.git` becomes a "repo" (no recursion into nested `.git`, except git submodules if `--submodules` is passed).
- `.gitignore` is respected through git's own matcher (`git ls-files -co --exclude-standard`). If git is missing, fall back to a built-in ignore list (`node_modules`, `.terraform`, `.terragrunt-cache`, `vendor`, `.git`).
- Files are read in parallel (bounded). Binary and huge files are skipped (over 2 MB).

## Normalization
- `v1.2.3` → `1.2.3` for comparison. The raw value is kept for display.
- Constraints (`~> 1.15`, `>= 1.5, < 2`) are kept as constraints. Drift treats them as their **lowest satisfying** version for sorting and marks them `≈` in the matrix.
- A pin that resolves to no known tool is reported with `tool=""` and listed under "unresolved" (`pins --unresolved`), so users can add aliases.

## Acceptance criteria
- [ ] Every detector above has a testscript fixture under `testdata/script/pins_*.txtar` with golden JSON.
- [ ] Fixture: a dotfriedrice-like `mise.toml` (array values, `http:`/`github:`/`aqua:` backends, `[tools."http:mongodb-database-tools"]` tables) parses identically with and without the mise binary on PATH.
- [ ] Dockerfile `ARG TERRAFORM_VERSION=1.15.8` and `# renovate: datasource=github-releases depName=gruntwork-io/terragrunt` + `ARG TG_VERSION=1.2.1` both map to the right tools.
- [ ] `.terraform.lock.hcl` produces `tfprovider:hashicorp/aws` pins with exact versions; `required_providers` produces constraint pins.
- [ ] `relgap pins` over 60 repos with no network takes under 5 s (benchmark test with a generated tree).
- [ ] Unknown `*_VERSION` names appear in `--unresolved` with file:line.
