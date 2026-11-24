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
from the base version to the new one.

A constraint whose lower bound moved digests from the old bound to the new one, when its operator
and its other clauses stayed the same. `>= 6.62.0` to `>= 6.72.0` in `required_providers`,
`~> 1.5` to `~> 1.7` and `>= 3.6.0, < 4.0.0` to `>= 3.7.0, < 4.0.0` qualify, and stderr says that
the digest read the lower bounds. Renovate's `bump` range strategy opens such pull requests.
Added, removed and other changed pins are listed on stderr and get no digest.

In JSON, each digest has a `pins` array with the changed pins it covers: `repo`, `file`, `line`,
`raw_name`, and `base` and `head`, the values as written. Use it to match a digest to your own
records by file and line. Each digest's `id` is the tool's identity, the name `relgap outdated`
prints in `tool`, such as `chart:aws/aws-for-fluent-bit` or `github:aws/aws-for-fluent-bit`. Key
digests on `id`: `tool` is the short name, which a chart and its application can share.

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
relgap 0.4.0 and older quote the notes as they are, and 0.5.0 to 0.13.0 miss a few cases, such
as a mention right after a non-ASCII letter or one written as a character reference. Use 0.13.1
or later for comments; relgap-action refuses comment mode with an older `version`.

The token needs `pull-requests: write`. Pull requests from forks and from Dependabot get a
read-only `GITHUB_TOKEN`, so the workflow skips the comment for them and writes the step summary
only. [`relgap notify`](/cli/notify) and [notify targets](/reference/notify) have the details.

## The workflow

Copy this file to `.github/workflows/digest.yml` in a repository whose `mise.toml` pins relgap.
`--fail-on breaking` fails the job when an upgrade has a breaking change, after `--out` has
written the JSON for the comment:

<<< @/ci/digest.yml

The `paths` filter lists the files that hold pins; add the ones your repository uses, as
[Pins and detectors](/concepts/pins) lists them.

A Renovate branch that moves a Terraform module on Bitbucket or GitLab, or an image, is digested
the same way: from the module's changelog or GitLab releases, and from the GitHub repository an
image's source label names. A private module needs its host's token in the job's environment,
from a repository secret; relgap sends it to that host only
([Network, privacy and tokens](/concepts/network#tokens)):

```yaml
env:
  BITBUCKET_API_TOKEN: ${{ secrets.BITBUCKET_API_TOKEN }}
  GITLAB_TOKEN: ${{ secrets.GITLAB_TOKEN }}
``` [Use relgap-action](/guides/action) runs the
same digest and comment in comment mode without a `mise.toml` entry.
