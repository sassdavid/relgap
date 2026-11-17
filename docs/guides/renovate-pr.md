---
description: Post the release notes of every tool a pull request upgrades as one comment that updates itself, with Renovate pull requests limited to the dependency they bump.
---

# Comment upgrade notes on Renovate pull requests

When Renovate bumps `terraform` from 1.15.8 to 1.17.1, the reviewer wants the release notes in
between. `relgap digest --pull-request` digests every tool a pull request upgrades, and
`relgap notify` keeps one comment with the result on the pull request.

## How the digest finds the upgrades

In a GitHub Actions `pull_request` run, `relgap digest --pull-request` reads the pull request
from the event file in `GITHUB_EVENT_PATH`. It scans the pins of the base commit and of the
checkout, pairs them by file, tool and name, and digests each tool whose exact version changed,
from the base version to the new one. Added, removed and constraint pins are listed on stderr and
get no digest.

The base commit must be in the clone, so the checkout needs `fetch-depth: 0`. When the base commit
is missing, relgap tries the target branch from Renovate's comment, then the pull request's base
branch. The base files come from `git archive`, so paths marked `export-ignore` aren't compared.

Outside Actions, `--base <ref>` does the same against any git ref:

```sh
relgap digest --base origin/main --format md
```

## Renovate pull requests

A pull request counts as Renovate's when its branch starts with `renovate/` or its body has the
hidden `renovate-debug` comment that Renovate adds. A self-hosted Renovate with another branch
prefix is recognized by that comment.

On such a pull request, relgap digests only the dependency the branch names, such as `terraform`
in `renovate/terraform-1.x`. A group branch digests every upgraded pin.

## The comment

`relgap notify --from digest.json --notify github-pr-comment` publishes the saved JSON as one
comment per report kind. The comment starts with a hidden marker that names the report kind and
a hash, so a re-run edits its own comment and writes nothing when the hash is the same. A digest
that covers no tool turns an existing comment into "Nothing to report" and creates none.
`--dry-run-notify` reads the comments and prints what would change.

Release notes are full of `@user` mentions and `#123` references. The Markdown relgap writes
puts a zero-width space after each `@` and `#` that would mention a user or reference an issue,
and sends github.com issue links through redirect.github.com, so the comment notifies nobody.
relgap 0.4.0 and older quote the notes as they are, so use a later release for comments.

The token needs `pull-requests: write`. Pull requests from forks and from Dependabot get a
read-only `GITHUB_TOKEN`, so the workflow skips the comment for them and writes the step summary
only. [`relgap notify`](/cli/notify) and [notify targets](/reference/notify) have the details.

## The workflow

Copy this file to `.github/workflows/digest.yml` in a repository whose `mise.toml` pins relgap.
`--fail-on breaking` fails the job when an upgrade has a breaking change, after `--out` has
written the JSON for the comment:

<<< @/ci/digest.yml

The `paths` filter lists the files that hold pins; add the ones your repository uses, as
[Pins and detectors](/concepts/pins) lists them. [Use relgap-action](/guides/action) runs the
same digest and comment in comment mode without a `mise.toml` entry.
