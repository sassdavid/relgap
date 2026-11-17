---
description: Install a verified relgap release in GitHub Actions with relgap-action, and run outdated, drift or the pull request digest with one step.
---

# Use relgap-action

[relgap-action](https://github.com/sassdavid/relgap-action) is a composite GitHub Action. It
installs a relgap release, checks it before it runs, and can run relgap for you. The repository
then needs no `mise.toml` entry for relgap. It runs on Linux, macOS and Windows runners.

## Modes

The `mode` input picks what the step does:

- `install` (the default) puts relgap on `PATH`. Later steps call it as they would any tool.
- `report` runs `relgap outdated` (or `relgap drift` with `command: drift`) once. It writes JSON,
  Markdown, SARIF and annotations, uploads the SARIF to code scanning and appends the Markdown to
  the step summary.
- `comment` runs `relgap digest --pull-request` on a `pull_request` event and keeps one comment
  with the digest on the pull request, as [Comment upgrade notes on Renovate pull
  requests](/guides/renovate-pr) describes.

In `report` and `comment` mode, `fail-on` passes `--fail-on` values, separated by commas or
newlines. A match fails the step with exit 3 after the files, the upload and the comment are
done. `args` adds more arguments, one per line.

## A workflow

This workflow runs `relgap outdated` on pushes to `main` and every Monday, and the digest on pull
requests that change a pin. Copy it to `.github/workflows/relgap.yml`:

<<< @/ci/relgap-action.yml

The `outdated` job needs `security-events: write` for the SARIF upload, and the `digest` job
needs `pull-requests: write` for the comment and `fetch-depth: 0` so relgap can read the pins at
the base commit. Pull requests from forks and from Dependabot get a read-only `GITHUB_TOKEN`, so
they get the step summary only.

## Verification

The action checks every archive against the release's `checksums.txt` (sha256), then runs the
check that `verify` names:

- `attestation` (the default) runs `gh attestation verify`, pinned to relgap's release workflow.
- `cosign` runs `cosign verify-blob` on `checksums.txt` and needs cosign or mise on `PATH`.
- `checksum` stops after sha256. It catches a broken download, not a replaced release.

[Security and verification](/security) shows the same checks for a manual install.

## Inputs and outputs

<!--@include: ../parts/relgap-action.md-->

The `exit-code` output lets a later step act on the result, for example exit 3 for a matched
gate. [Exit codes](/reference/exit-codes) lists every code.
