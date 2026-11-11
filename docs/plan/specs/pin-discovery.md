# Spec: pin discovery (scanner + mise input)

Produces `[]Pin` (tool, raw name, kind, value, version, exact, match, location) from a project or
from many repo roots. Used by `pins`, `outdated` and `drift`. Code: `internal/scan`.

## Sources of pins

| Detector | Files | Extracts | Notes |
|---|---|---|---|
| `mise` | `mise.toml`, `.mise.toml`, `mise.*.toml`, `.mise.*.toml`; `config.toml`, `config.*.toml`, `conf.d/*.toml` in a `mise`, `.mise` or `_mise` directory | tool → version | always parsed with go-toml, never `mise ls` ([ADR-0007](../adr/0007-parse-mise-config-for-pins.md)). `[tools]` values may be strings, arrays or tables `{version=…}`; every array element is a pin |
| `tool-versions` | `.tool-versions` | asdf pins | |
| `aqua` | `aqua.yaml`, `aqua/*.yaml` | `packages[].name@version` | Renovate has no aqua manager, which is a differentiator |
| `idiomatic` | `.terraform-version`, `.terragrunt-version`, `.node-version`, `.nvmrc`, `.python-version`, `.go-version`, `go.mod` `toolchain`/`go` | single version | |
| `terraform` | `*.tf`, `*.tf.json` via hcl/v2 ([ADR-0008](../adr/0008-hcl-and-dockerfile-parsers.md)) | `required_version`, `required_providers` (`tfprovider:` pins) | constraints are recorded as `Exact=false`; a bare version is exact |
| `tf-lock` | `.terraform.lock.hcl` via hcl/v2 | exact provider versions | the *real* provider version in use |
| `terragrunt` | `terragrunt.hcl`, `root.hcl`, `*.hcl` | `terraform_version_constraint`, `terragrunt_version_constraint` | static attributes only, no evaluation |
| `dockerfile` | `Dockerfile*`, `*.Dockerfile`, `Containerfile*` (own parser, [ADR-0008](../adr/0008-hcl-and-dockerfile-parsers.md)) | `FROM image:tag` (`image:` pins, ARG defaults substituted), `ARG/ENV <NAME>_VERSION=x` | `<NAME>` maps to a tool via registry aliases (`TERRAFORM_VERSION` → terraform). Honour `# renovate: datasource=… depName=…` comments above an ARG or ENV (exact mapping) |
| `ci` | `.github/workflows/*.y*ml`, `bitbucket-pipelines.yml`, `.gitlab-ci.yml` | `image:` tags, a job's `container:`, `uses: owner/action@vX`, `*_VERSION` and `*-version` keys (`go-version`, `terraform_version`) | actions are reported as kind `action` (opt-in, `scan.detectors.ci_actions`) |
| `helm` | `Chart.yaml` | `dependencies[].version` (`chart:` pins), `appVersion` | |
| `generic` | any text file listed in config `scan.extra_files` | regex `(?i)\b([a-z0-9_]+)_VERSION\s*(?:[:?+]?=\|:)\s*["']?(v?[0-9][^\s"']*)` | opt-in (`scan.detectors.generic`); maps names via aliases. Also accepts Make's `:=`, `?=` and `+=` |
| `renovate` (input) | `--renovate-report <file.json>` | `packageFiles[].deps[]` from `renovate --platform=local --dry-run=extract --report-type=file` | optional, adds coverage for anything Renovate understands. `packageFiles` may sit at the top level or under `repositories.<name>`. Pins have line 0 |

## Walking repos
- `drift` and `pins` roots are directories. Every directory containing `.git` (a directory, or a file for worktrees and submodules) becomes a "repo" (no recursion into nested `.git`, except git submodules if `--submodules` is passed). A root with no `.git` at or below it is one repo itself.
- `.gitignore` is respected through git's own matcher (`git ls-files -co --exclude-standard`). If git is missing, fall back to a built-in ignore list (`node_modules`, `.terraform`, `.terragrunt-cache`, `vendor`, `.git`).
- Files are read in parallel (bounded). Binary and huge files are skipped (over 2 MB).

## Normalization
- `v1.2.3` → `1.2.3` for comparison. The raw value is kept for display.
- Constraints (`~> 1.15`, `>= 1.5, < 2`) are kept as constraints. Drift counts them as an exact pin they allow in the same repo, else as their lower bound, and marks them `≈` in the matrix ([ADR-0010](../adr/0010-drift-offline-matrix.md)).
- Every pin has a `match`: `exact` (1.15.8), `constraint` (~> 1.15, >= 1.5), `prefix` (26, 3.14, mise's `prefix:1.2`) or `channel` (latest, lts, corretto-17). `exact` is true only for `exact`. Tables mark every non-exact value with `≈`.
- A pin that resolves to no known tool is reported with `tool=""` and listed under "unresolved" (`pins --unresolved`), so users can add aliases. `--unresolved` keeps the same columns with RAW NAME first and prints a hint about `aliases` on stderr.

## Output
- `kind` is `tool`, `tfprovider`, `chart`, `image` or `action`. Non-tool pins carry their name as `kind:name` in `tool` (`tfprovider:hashicorp/aws`, `image:golang`).
- `tool` is the name `relgap digest` accepts: a registry name, or `github:owner/repo` and `aqua:package` when no registry names the tool.
- JSON follows `schemas/pins-v1.json`: `data.repos[]` and `data.pins[]`, sorted by repo, file and line. Files that fail to parse go into the envelope's `errors`, and the command exits 1 after printing the rest.

## Acceptance criteria
- [x] Every detector above has a testscript fixture under `testdata/script/pins_*.txtar` with golden JSON.
- [x] Fixture: a dotfriedrice-like `mise.toml` (array values, `http:`/`github:`/`aqua:` backends, `[tools."http:mongodb-database-tools"]` tables) parses identically with and without the mise binary on PATH.
- [x] Dockerfile `ARG TERRAFORM_VERSION=1.15.8` and `# renovate: datasource=github-releases depName=gruntwork-io/terragrunt` + `ARG TG_VERSION=1.2.1` both map to the right tools.
- [x] `.terraform.lock.hcl` produces `tfprovider:hashicorp/aws` pins with exact versions; `required_providers` produces constraint pins.
- [x] `relgap pins` over 60 repos with no network takes under 5 s (benchmark test with a generated tree).
- [x] Unknown `*_VERSION` names appear in `--unresolved` with file:line.
