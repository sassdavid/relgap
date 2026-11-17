---
description: Every host relgap sends requests to, the programs it runs, where the GitHub token goes, what the cache keeps and what --summarize sends to a model.
---

# Network, privacy and tokens

relgap sends requests only to the hosts on this page, and only to look up the tools you ask
about. It has no telemetry and no update check. `relgap pins` and `relgap drift` without
`--latest` read files only and make no request at all.

## Hosts

<!--@include: ../parts/network-hosts.md-->

Every request goes through one HTTP client with retries, a per-host limit and the on-disk cache.
Pages of release notes come from GitHub through the API. Web pages come only from these hosts,
and `relgap registry lint` rejects an entry whose URL points anywhere else:

<!--@include: ../parts/web-hosts.md-->

## Programs relgap runs

| Program | Command | When |
|---|---|---|
| git | `git ls-files` | listing the files of a repository, so `.gitignore` applies |
| git | `git rev-parse` and `git archive` | reading the pins at another commit with `digest --base` or `--pull-request` |
| mise | `mise registry --json` | mapping tool names to repositories, when mise is on `PATH` |
| mise | `mise which` | `outdated --binaries`, finding the installed executables |
| gh | `gh auth token` | looking up the GitHub token in gh's keyring, after the other places |

Without git, relgap walks the directories itself. Without mise, it uses the embedded copy of
mise's registry ([registry snapshots](/reference/snapshots)).

## The GitHub token

relgap looks for a token the way gh does, and takes the first it finds:

1. the `GH_TOKEN` environment variable
2. the `GITHUB_TOKEN` environment variable
3. gh's config file (`hosts.yml`)
4. `gh auth token`, which reads gh's keyring

relgap adds the token only to requests to `api.github.com`. A token with no scopes reads public
repositories; a private repository needs a token that can read its contents. `relgap notify`
needs a token that can write pull request comments.

Without a token, relgap uses the REST API only, sends at most 2 requests at once, and GitHub
allows 60 requests an hour. relgap warns when the remaining requests run low. A token raises the
limit to GitHub's authenticated one and lets relgap batch release bodies into GraphQL queries.

## The cache

Answers are cached under `$XDG_CACHE_HOME/relgap/http`, or the `cache.dir` you set. The cache
keeps response bodies and the headers needed to revalidate them. It never stores the
`Authorization` header, cookies or rate-limit headers. `--offline` answers from the cache only,
`--no-cache` neither reads nor writes it, and `relgap cache clear` deletes it.

## Summaries from a model

`--summarize` is off unless you configure `llm.provider` and `llm.model`, and relgap never adds
it on its own. With it, relgap sends the digest's listed entries to the provider you configured
and nothing else. The summary is advisory: it never changes `--fail-on`, the exit code or SARIF.

<SentToModel />

The API key comes from the environment variable that `llm.api_key_env` names. relgap never reads
a key from a file and never writes one to the cache or the logs.
