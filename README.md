# relgap

relgap merges the release notes between two versions of a tool into one digest, reports which
tool versions your repositories pin, and adds end-of-life and vulnerability data. It can also run
as an MCP server.

relgap is pre-release. Milestone M0 (skeleton and release pipeline) is in place, and there
are no digests yet. The plan is in [docs/plan](docs/plan/) and the CLI reference in
[docs/cli](docs/cli/relgap.md).

## Install

Once v0.1.0 is out:

```sh
brew install --cask sassdavid/tap/relgap
scoop bucket add sassdavid https://github.com/sassdavid/scoop-bucket && scoop install relgap
mise use github:sassdavid/relgap
go install github.com/sassdavid/relgap/cmd/relgap@latest   # unsigned
```

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

## Development

```sh
mise install                         # pinned toolchain (mise.toml)
mise run check                       # linters (Go, prose, workflows) + go test -race + govulncheck
mise run test:golden -- -update      # regenerate golden files, then review the diff
mise run docs                        # regenerate docs/cli
mise run snapshot                    # local goreleaser build into dist/
mise run sibling:diff                # compare the files shared with tfcensus
```

## License

[Apache-2.0](LICENSE)
