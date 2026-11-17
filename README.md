# relgap

relgap merges the release notes between two versions of a tool into one digest: breaking
changes first, then deprecations, security fixes, features, fixes and the rest, each entry with
a link to its source. It also lists the tool versions your repositories pin, shows which pins
are behind or past their end of life, finds where repositories pin different versions, and
serves all of it to agents over MCP.

relgap is pre-release, and its commands and output can still change before 1.0.

```sh
relgap digest kubectl 1.36.0..1.37.2
relgap outdated
relgap drift ~/src/infra
```

## Install

```sh
mise use -g github:sassdavid/relgap
```

Release archives for Linux, macOS and Windows are on
[GitHub](https://github.com/sassdavid/relgap/releases), signed with cosign and covered by build
provenance attestations. [Install](https://relgap.sasskovacs.dev/install) has every channel and
the verification steps.

## Documentation

The docs are at [relgap.sasskovacs.dev](https://relgap.sasskovacs.dev):

- [Getting started](https://relgap.sasskovacs.dev/getting-started): a first digest and a scan
  of a repository
- [Gate CI on breaking changes, EOL and vulnerabilities](https://relgap.sasskovacs.dev/guides/ci-gate)
  and [relgap-action](https://relgap.sasskovacs.dev/guides/action)
- [Use relgap from an MCP client](https://relgap.sasskovacs.dev/guides/mcp)
- [CLI reference](https://relgap.sasskovacs.dev/cli/) and
  [configuration](https://relgap.sasskovacs.dev/reference/config)
- [Troubleshooting](https://relgap.sasskovacs.dev/troubleshooting) and
  [FAQ](https://relgap.sasskovacs.dev/faq)

## Contributing

[Contributing](https://relgap.sasskovacs.dev/contributing) covers the build, the tests and
registry entries. Report a vulnerability through
[private vulnerability reporting](https://github.com/sassdavid/relgap/security/advisories/new).

## License

[Apache-2.0](LICENSE). The embedded registry snapshots in
[internal/registry/snapshot](internal/registry/snapshot/) are derived from
[jdx/mise](https://github.com/jdx/mise) and
[aquaproj/aqua-registry](https://github.com/aquaproj/aqua-registry), both MIT; their licenses are
next to the snapshots. Each release archive has a `third_party_licenses/` directory with the
notices of everything compiled into the binary.
