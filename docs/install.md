---
description: Install relgap from a release archive, with mise or with go install, verify the signature, and set up man pages.
---

# Install

## Channels

| Channel | Command | Completion and man pages |
|---|---|---|
| GitHub release | download `relgap_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) from the [releases](https://github.com/sassdavid/relgap/releases) | `completions/` and `manpages/` in the archive |
| mise | `mise use -g github:sassdavid/relgap` | the binary only: generate the script with `relgap completion <shell>` |
| go install | `go install github.com/sassdavid/relgap/cmd/relgap@latest` | the binary only, unsigned |

relgap is pre-release, and its commands and output can still change before 1.0. Each release
names the changes on the [changelog](/changelog).

## Verify a release

Each release has a `checksums.txt` with the sha256 of every archive. cosign signs it without a
key, and GitHub build-provenance attestations cover every archive. Check the archive against the
checksums, then check the signature and the attestation:

```sh
sha256sum --ignore-missing -c checksums.txt
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/sassdavid/relgap/\.github/workflows/release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
gh attestation verify --owner sassdavid relgap_<version>_<os>_<arch>.tar.gz
```

On macOS, use `shasum -a 256 --ignore-missing -c checksums.txt` for the first line. `go install`
builds from source and has no signature to check.

## Man pages

The archive holds one man page per command (`manpages/relgap.1.gz`, `manpages/relgap-digest.1.gz`
and so on). Copy them to a `man1` directory on your `MANPATH`, such as
`~/.local/share/man/man1`.

## Shell completion

[Shell completion](/guides/completion) sets up completion for bash, zsh, fish, PowerShell and other shells.
