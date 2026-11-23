---
description: Every host relgap sends requests to, the programs it runs, where each token goes, what OSV receives, what the cache keeps and what --summarize sends to a model.
---

# Network, privacy and tokens

relgap sends requests only to the hosts on this page, and only to look up the tools you ask
about or the pins it scanned. It has no telemetry and no update check. `relgap pins` and
`relgap drift` without `--latest` read files only and make no request at all.

Scanned files name git hosts and image registries, so relgap contacts only the ones it knows:
bitbucket.org, gitlab.com and the hosts in `gitlab.hosts` for git sources, and Docker Hub,
ghcr.io, registry.k8s.io, quay.io, public.ecr.aws, mcr.microsoft.com and the hosts in
`images.registries` for images and for Helm charts behind an `oci://` URL that Artifact Hub
doesn't list. A pin on any other host stays unresolved or skipped, and relgap runs no
`git ls-remote` against it.

## Hosts

<!--@include: ../parts/network-hosts.md-->

`www.cisa.gov` and `api.first.org` are off by default. With `--kev` relgap downloads the whole KEV
catalog and matches it locally, so CISA learns nothing about your pins. With `--epss` relgap sends
the CVE IDs of the advisories it found, which tells FIRST which vulnerabilities your pins have.

Every request goes through one HTTP client with retries, a per-host limit and the on-disk cache.
Pages of release notes come from GitHub through the API. Web pages come only from these hosts,
and `relgap registry lint` rejects an entry whose URL points anywhere else:

<!--@include: ../parts/web-hosts.md-->

## What OSV receives

`relgap audit` and `relgap outdated` ask `api.osv.dev` about every pin they can check, with no
token. The table lists every value those requests carry. It comes from the list in relgap's
source that a test compares with the requests the OSV client sends:

<!--@include: ../parts/osv-sent.md-->

relgap never sends OSV a file path, a pin's location, the name of the repository that pins a tool,
or a token. It never queries a tool on a host it doesn't send to: Bitbucket, a self-managed GitLab
host, a `github.host` other than github.com, an image registry, or a package name with `:` or `@`
after the npm scope. Those pins are unchecked with reason `private-source` or
`no-advisory-source` ([Unchecked pins](/concepts/enrichment#unchecked-pins)).

relgap doesn't know whether a repository on github.com or gitlab.com is private. A tool you map
to a private repository with `tools.<name>.repo`, or a `github:` backend that names one, goes to
OSV as its Go module path and repository URL. A `go:` pin of a private module sends each prefix of
its path, a scoped npm package its name, and `outdated --binaries` the main module of each
executable. To keep a tool away from OSV, add an [`ignore`](/reference/config#ignore) rule for it.
relgap drops ignored pins before any request, so the tool also leaves every other report:

```yaml
ignore:
  - tool: deploy-cli
```

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

## Tokens

Each token goes only to the host it belongs to. relgap never sends one host's token to another,
never writes a token to the cache or the logs, and names in the debug log only whether a client
has a token and which variable it came from.

### GitHub

relgap looks for a token the way gh does, and takes the first it finds:

1. the `GH_TOKEN` environment variable
2. the `GITHUB_TOKEN` environment variable
3. gh's config file (`hosts.yml`)
4. `gh auth token`, which reads gh's keyring

relgap adds the token only to requests to `api.github.com`. A token with no scopes reads public
repositories; a private repository needs a token that can read its contents. `relgap notify`
needs a token that can write pull request comments.

Without a token, relgap uses the REST API only, sends at most 2 requests at once, and GitHub
allows 60 requests an hour. relgap warns on the first GitHub request, and again when the
remaining requests run low. A token raises the limit to GitHub's authenticated one and lets
relgap batch release bodies into GraphQL queries.

### Bitbucket Cloud

relgap sends `BITBUCKET_API_TOKEN` to `api.bitbucket.org` only, as `Authorization: Bearer`. A
user API token with the `read:repository:bitbucket` scope works, and so do repository, project
and workspace access tokens. relgap reads no username or email, because a Bearer token needs
none. Without a token, relgap warns once, and Bitbucket allows 60 anonymous requests an hour per
IP; a private repository then answers 404.

### GitLab

relgap reads GitLab tokens the way glab does and sends each to one host:

1. the variable that a `gitlab.hosts` entry names in `token_env`, to that entry's host
2. `GITLAB_TOKEN`, then `GITLAB_ACCESS_TOKEN`, to the host in `GITLAB_HOST` (default gitlab.com)
3. `CI_JOB_TOKEN`, to the host in `CI_SERVER_HOST`, when that host has no other token

A personal, project or group token goes in `PRIVATE-TOKEN` and needs `read_api`. A CI job token
goes in `JOB-TOKEN`; it reads tags, releases and raw files of projects whose job token allowlist
has the calling project, but not the directory listing that finds a changelog file. Without a
token, gitlab.com allows 500 requests a minute per IP, and relgap warns when the remaining
requests run low.

### Image registries

relgap reads no Docker credentials. When a registry answers 401 with a Bearer challenge, relgap
asks the token URL in the challenge for an anonymous pull token and keeps it in memory for the run. A
private image therefore stays skipped. Reading an image's source label costs one manifest
request, which Docker Hub counts as a pull.

## The cache

Answers are cached under `$XDG_CACHE_HOME/relgap/http`, or the `cache.dir` you set. The cache
keeps response bodies and the headers needed to revalidate them. It never stores request
headers, so `Authorization`, `PRIVATE-TOKEN` and `JOB-TOKEN` stay out, and it never stores
cookies, rate-limit headers or the answers that carry an anonymous registry token. `--offline` answers from the cache only,
`--no-cache` neither reads nor writes it, and `relgap cache clear` deletes it.

## Summaries from a model

`--summarize` is off unless you configure `llm.provider` and `llm.model`, and relgap never adds
it on its own. With it, relgap sends the digest's listed entries to the provider you configured
and nothing else. The summary is advisory: it never changes `--fail-on`, the exit code or SARIF.

<SentToModel />

The API key comes from the environment variable that `llm.api_key_env` names. relgap never reads
a key from a file and never writes one to the cache or the logs.
