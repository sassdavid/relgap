# Reference repositories (local clones for implementation)

These are clones Claude Code (and you) can read during implementation, **for inspiration and verification only**.
They're never a dependency or a copy source:
- Never copy code from BSL-licensed repos (hashicorp/terraform).
- Ideas from MIT, Apache or MPL repos are fine. If code is adapted, keep its notice ([ADR-0010](../engineering/adr/0010-license-apache2.md)).
- Go module sources of actual dependencies are already in `$(go env GOMODCACHE)` after `go mod download`. Don't clone those just to read them.

**Location convention:** `~/src/github/_reference/<owner>/<repo>`. The script at the bottom does treeless partial clones of the default branch.
[CLAUDE.md](../../CLAUDE.md) points to this path (see "Reference sources").

## To clone: relgap

| Repo | License | Why | Priority |
|---|---|---|---|
| jdx/mise | MIT | `registry/*.toml` format, `ls/outdated/registry --json` shapes, github backend version prefixes | **M1, must** |
| aquaproj/aqua-registry | MIT | `pkgs/**/registry.yaml`: repo, version_prefix, version_filter (snapshot source) | **M1, must** |
| renovatebot/renovate | AGPL-3.0, **read only, never copy** | `lib/workers/repository/update/pr/changelog` (changelog discovery, heading matching, cache TTLs), `lib/modules/manager/*` (pin extraction regexes) | M2/M3/M5, must |
| dependabot/dependabot-core | Prosperity Public License, **read only** | `common/lib/dependabot/metadata_finders/base` (changelog pruner, release finder) | M3, should |
| endoflife-date/endoflife.date | MIT | product identifiers, cycle naming | M8, nice |
| golang/vuln | BSD-3 | govulncheck binary mode (buildinfo → stdlib/module queries) | M12, nice |

## Go CLI patterns and template

| Repo | Why |
|---|---|
| cli/cli (gh) | cobra command structure, `Prompter` interface + huh accessible mode, acceptance tests with testscript, goreleaser + attestations workflow |
| goreleaser/goreleaser | reference `.goreleaser.yaml` (cosign bundle, SBOM, homebrew_casks), golangci config |
| maratori/golangci-lint-config | the golden `.golangci.yml` to copy at M0 |
| twpayne/chezmoi | heavy testscript usage, a good model for CLI golden tests |
| modelcontextprotocol/go-sdk | server examples (stdio, tools, schemas) |
| aquaproj/aqua | slog + tint, per-host HTTP handling, a large Go CLI |

## Clone script

```sh
# run once; safe to re-run (it pulls if the clone already exists)
REF=~/src/github/_reference
clone() { d="$REF/$1"; if [ -d "$d/.git" ]; then git -C "$d" pull --ff-only -q; else
  git clone -q --filter=tree:0 "https://github.com/$1.git" "$d"; fi; echo "ok $1"; }

# relgap
for r in jdx/mise aquaproj/aqua-registry renovatebot/renovate dependabot/dependabot-core \
         endoflife-date/endoflife.date golang/vuln; do clone "$r"; done
# Go CLI patterns
for r in cli/cli goreleaser/goreleaser maratori/golangci-lint-config twpayne/chezmoi \
         modelcontextprotocol/go-sdk aquaproj/aqua; do clone "$r"; done

# refresh every clone in _reference, including ones not listed above
for g in "$REF"/*/*/.git; do git -C "${g%/.git}" pull --ff-only -q && echo "pulled ${g%/.git}"; done
```

`--filter=tree:0` (treeless) keeps the clones small while `git log` and `git blame` still work. renovate is the
largest. Clone only the "must" rows if disk or time matters.
