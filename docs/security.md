---
description: How to report a vulnerability in relgap, how to verify a release, what each release archive carries, and where relgap's data and token handling is described.
---

# Security and verification

## Report a vulnerability

Report it privately through GitHub's private vulnerability reporting:
[github.com/sassdavid/relgap/security/advisories/new](https://github.com/sassdavid/relgap/security/advisories/new).
Don't open a public issue for a vulnerability. The report stays visible to you and the
maintainer only until an advisory is published.

## Verify a release

Every release on [GitHub](https://github.com/sassdavid/relgap/releases) has these files besides
the archives:

| File | What it is |
|---|---|
| `checksums.txt` | the sha256 of every archive |
| `checksums.txt.sigstore.json` | a keyless cosign signature of `checksums.txt`, with its certificate, from relgap's release workflow |
| `<archive>.sbom.json` | a software bill of materials for each archive |

GitHub build-provenance attestations cover every archive as well. Check an archive against the
checksums, the checksums against the signature, and the archive against its attestation:

```sh
sha256sum --ignore-missing -c checksums.txt
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/sassdavid/relgap/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
gh attestation verify --owner sassdavid relgap_<version>_<os>_<arch>.tar.gz
```

On macOS, use `shasum -a 256 --ignore-missing -c checksums.txt` for the first line. A binary
built with `go install` has no signature or attestation. relgap-action runs the attestation or
cosign check before it puts relgap on `PATH` ([Use relgap-action](/guides/action)).

## Third-party notices

relgap is licensed under Apache-2.0. Each archive has a `third_party_licenses/` directory with
the license of every module compiled into the binary and an index that links each module's
source. The embedded registry snapshots come from
[jdx/mise](https://github.com/jdx/mise) and
[aquaproj/aqua-registry](https://github.com/aquaproj/aqua-registry), both MIT; their licenses sit
next to the snapshots in the repository ([Registry snapshots](/reference/snapshots)).

## Tokens and data

relgap reads its GitHub token from the environment or gh, never from a config file, and sends
it to `api.github.com` only. The HTTP cache never stores request headers or tokens. relgap has
no telemetry and no update check, and it calls a model only when you pass `--summarize` with an
`llm` config. [Network, privacy and tokens](/concepts/network) lists every host, every program
relgap runs and what a summary request contains.
