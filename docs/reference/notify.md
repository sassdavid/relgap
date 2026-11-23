---
description: The targets relgap notify publishes saved reports to, what each needs from the environment, and how a comment is found and updated again.
---

# Notify targets

`relgap notify` reads the JSON reports that `digest`, `outdated`, `drift` and `audit` write with
`--format json` or `--out json=<path>`, and publishes them. It collects nothing itself, so the
report and the comment always match. `--notify` names the target; there is one today.

## github-pr-comment

Keeps one comment per report kind (digest, outdated, drift, audit) on a GitHub pull request.

| Needs | From |
|---|---|
| the repository | `GITHUB_REPOSITORY` |
| the pull request number | the `pull_request` event, or an `issue_comment` event on a pull request, in the file `GITHUB_EVENT_PATH` names |
| the API URL | `GITHUB_API_URL`, else `https://api.github.com` |
| a token with `pull-requests: write` | `GH_TOKEN`, `GITHUB_TOKEN` or gh, as for every command |

GitHub Actions sets the three variables. Outside Actions, set them yourself.

The comment starts with a hidden HTML marker, `<!-- relgap:v1 {...} -->`, that holds the report
kind and a hash of the comment. A re-run finds its own comment by the kind and edits it, and
writes nothing when the hash is the same. A report with nothing to report turns an existing
comment into "Nothing to report" and creates none:

| Report | Has something to report when |
|---|---|
| digest | it covers at least one tool |
| outdated | a pin is outdated, end of life, vulnerable or failed |
| drift | a tool drifts or failed |
| audit | it has a finding or an error, or a pin is unchecked because OSV failed, `--offline` had no answer or the name resolves to no tool |

An image, a chart or another pin no advisory source covers doesn't open an audit comment, since
nearly every repository has one. `audit` exits 3 on a finding by default, so the example below
passes `--fail-on=` to save the report and still reach `notify`.

The comment is cut at a table row or section once it reaches `--md-max-chars` (60,000
characters by default). `--dry-run-notify` reads the existing comments with GET requests and
prints what it would change.

```sh
relgap audit --fail-on= --format json > audit.json
relgap notify --from audit.json --notify github-pr-comment
relgap outdated --format json > outdated.json
relgap notify --from outdated.json --notify github-pr-comment
relgap notify --from digest.json --from drift.json --notify github-pr-comment --dry-run-notify
```

Exit codes: 0 when every comment was published, 2 when the repository or the pull request is
unknown, 4 when GitHub refused the token, and 1 for any other failure. A pull request from a
fork gets a read-only `GITHUB_TOKEN`, so the CI examples skip the comment step there
([Comment upgrade notes on Renovate pull requests](/guides/renovate-pr)).
