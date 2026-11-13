# relgap

relgap merges the release notes between two versions of a tool into one digest, reports which
tool versions your repositories pin, and adds end-of-life and vulnerability data. It can also run
as an MCP server.

relgap is pre-release, and its commands and output can still change before 1.0.

- `digest <tool> <from>..<to>` merges the release notes of every version in the range, breaking
  changes and security fixes first, each entry with a link to its source.
- `pins` lists the tool versions your repositories pin, with file and line: mise,
  `.tool-versions`, aqua, version files such as `.nvmrc`, Terraform, Terragrunt, Dockerfiles, CI
  workflows, Helm and Renovate reports.
- `outdated` compares every pin with the latest release and adds end-of-life dates from
  endoflife.date and known vulnerabilities from OSV. `digest --from-mise` and `digest --all`
  digest the outdated pins.
- `digest --base <ref>` digests the pins a change upgrades, from the old version to the new one.
  `digest --pull-request` does it for the pull request of a GitHub Actions run, and on a Renovate
  pull request only for the dependency its branch names.
- `drift` shows which repositories pin different versions of the same tool, offline by default.
- `mcp` serves `digest`, `breaking_changes`, `outdated`, `drift` and `resolve` to agents such as
  Claude Code ([docs/dogfood.md](docs/dogfood.md)).

The plan is in [docs/plan](docs/plan/) and the CLI reference in [docs/cli](docs/cli/relgap.md).

## CI

`outdated`, `digest` and `drift` exit 3 when a `--fail-on` value matches (breaking, deprecation,
eol, vuln, drift or outdated). A repeatable `--out <format>=<path>` writes JSON, SARIF, Markdown
and GitHub annotations from one run with the same exit code. Markdown is cut at a table row or
section once it reaches `--md-max-chars` (60,000 characters, under GitHub's 65,536-character
comment limit). `relgap notify --from <json> --notify github-pr-comment` keeps one comment per
report on a pull request. [docs/ci/outdated.yml](docs/ci/outdated.yml) puts these together, and
[docs/ci/digest.yml](docs/ci/digest.yml) comments the release notes of the tools a pull request
upgrades.

## Install

Releases are on [GitHub](https://github.com/sassdavid/relgap/releases): download the archive for
your platform, or let mise do it.

```sh
mise use github:sassdavid/relgap
go install github.com/sassdavid/relgap/cmd/relgap@latest   # unsigned
```

Homebrew and Scoop come later (milestone M19). Each archive carries shell completion scripts and
man pages; [docs/install.md](docs/install.md) sets them up for each shell.

Release checksums are signed with cosign (keyless) and carry GitHub build-provenance attestations:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/sassdavid/relgap/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
gh attestation verify --owner sassdavid relgap_<version>_<os>_<arch>.tar.gz
```

## Configuration

`relgap config show --sources` prints every effective setting and where it came from.
Layers, lowest to highest: defaults, `~/.config/relgap/config.yaml`, `.relgap.yaml` (nearest, up to
the git root), `RELGAP_*` env vars, flags. Unknown keys are errors. Schema:
[schemas/config-v1.json](schemas/config-v1.json).

## Tool resolution

`relgap resolve <tool>` shows the repository, version rules and changelog sources relgap uses for a
tool, and which layer supplied each field: the config file, relgap's curated entries in
[registry/](registry/), mise's registry (`mise registry --json`, or an embedded snapshot when mise
isn't installed) and an embedded index of the aqua registry. `relgap registry lint` checks entries
against [schemas/registry-v1.json](schemas/registry-v1.json).

## Development

```sh
mise install                         # pinned toolchain (mise.toml)
mise run check                       # linters (Go, prose, workflows) + go test -race + govulncheck
mise run test:golden -- -update      # regenerate golden files, then review the diff
mise run docs                        # regenerate docs/cli
mise run fuzz                        # every fuzz target, 60s each
mise run registry:snapshot           # refresh the embedded mise and aqua registry snapshots
mise run snapshot                    # local goreleaser build into dist/
mise run sibling:diff                # compare the files shared with tfcensus
```

## License

[Apache-2.0](LICENSE). The embedded registry snapshots in
[internal/registry/snapshot](internal/registry/snapshot/) are derived from
[jdx/mise](https://github.com/jdx/mise) and
[aquaproj/aqua-registry](https://github.com/aquaproj/aqua-registry), both MIT; their licenses are
next to the snapshots. Each release archive has a `third_party_licenses/` directory with the
notices of everything compiled into the binary and an index that links each module's source.
