---
description: Where relgap keeps its cache, how long answers stay fresh, how to work offline or force fresh data, and how to inspect, prune and clear the cache.
---

# Work offline and manage the cache

relgap caches every HTTP answer on disk. A second run over the same tools answers from the cache
or revalidates with a conditional request, so it is fast and uses little of GitHub's rate limit.
With a warm cache, `--offline` runs without the network at all.

## Where the cache lives

The cache directory is `cache.dir`, or `RELGAP_CACHE_DIR`, when set. Otherwise it is
`$XDG_CACHE_HOME/relgap` when `XDG_CACHE_HOME` is an absolute path, else `~/.cache/relgap` on
Linux and macOS and the local application data folder on Windows. It holds two directories:

| Directory | Holds |
|---|---|
| `http/` | the HTTP answers |
| `registry/` | the registry entries that `relgap registry update` downloaded |

Each HTTP entry keeps the response body and a few response headers: content type, `ETag`,
`Last-Modified` and `Link`. relgap never stores request headers, so the GitHub token and the
`Authorization` header never reach the disk.

```sh
relgap cache info
```

`relgap cache info` prints the directory, the number of entries, their size, and how many are
fresh and how many are stale.

## How long an answer stays fresh

A fresh entry is used without a request. Its lifetime depends on the age of the newest release it
describes, so a project that released last week gets checked more often than one that hasn't
released in a year:

| Newest release | Fresh for |
|---|---|
| under 7 days old | 1 hour |
| under 6 months old | 1 day |
| older | 10 days |

Answers from endoflife.date, OSV, the Terraform registry, Artifact Hub, npm and PyPI stay fresh
for 1 day, and a 404 for 1 hour. A stale entry is revalidated with `If-None-Match`. GitHub
answers an unchanged resource with 304, which doesn't count against the rate limit when the
request carries a token.

`cache.ttl_override` sets a fixed lifetime per source or host, as a Go duration. The sources are
`github`, `endoflife` and `osv`, and any key with a dot is a host name:

```yaml
cache:
  ttl_override:
    github: 6h
    endoflife: 72h
    registry.terraform.io: 12h
```

## Work offline

`--offline` answers from the cache only, fresh or stale, and never touches the network. A request
the cache can't answer fails with a hint to run once without `--offline`:

```sh
relgap outdated --offline
relgap digest terraform 1.15.8..1.17.1 --offline
```

Run the same command online once, and it works offline after.
`--summarize` is skipped with a warning under `--offline`. The MCP server takes the flag too:
`relgap mcp --offline` answers every tool call from the cache.

## Force fresh data

`--refresh` revalidates every cached answer, even fresh ones, and sends GraphQL queries again.
Use it right after a release you know about. `--no-cache` neither reads nor writes the cache.
The three flags exclude each other.

```sh
relgap outdated --refresh
```

## Prune and clear

`relgap cache prune` deletes entries no run has used for `--older-than` (default 720h, 30 days),
entries relgap can't read, entries in an older format and temporary files left by an interrupted
run. Expired entries stay, because their ETags let the next run revalidate them for free.

```sh
relgap cache prune --older-than 168h
relgap cache clear
```

`relgap cache clear` deletes every HTTP entry and the entries from `relgap registry update`, so
resolution goes back to the entries embedded in relgap.
