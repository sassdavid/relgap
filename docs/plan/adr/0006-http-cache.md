# relgap ADR-0006: Own HTTP cache with release-age TTLs

- Status: accepted (2026-11-11)

## Context
Engineering ADR-0005 asks for an ETag/Last-Modified disk cache with TTLs that depend on release
age, plus `--offline` and `--refresh`. The stack table left the choice between an own layer and
`bartventer/httpcache` v0.14.0 open until M4.

GitHub answers with `Vary: Accept, Authorization, Cookie`. An RFC 9111 cache matches on the varied
request headers, so it stores the token next to the response. RFC 9111 also never caches POST,
and relgap's release lists and bodies come from GraphQL POSTs when a token is set. The terraform
digest in the M3 cassette makes 7 GraphQL POSTs and 3 contents GETs.

## Decision
- `internal/httpx` has its own cache transport, outside the retry and rate-limit layers, so
  every client built with `httpx.New` and a `Cache` gets it and a fresh entry costs nothing.
- The key is a SHA-256 of method, URL, `Accept`, the request body and whether a token was sent.
  The token value is never stored. Entries keep five response headers: `Content-Type`, `Etag`,
  `Last-Modified`, `Link` and `X-Github-Media-Type`.
- GET and GraphQL POST replies with status 200 are stored. GraphQL replies with `errors` are not.
- The TTL comes from the newest date an entry describes: the newest `publishedAt` or
  `published_at` in the body (`github.NewestRelease`), else `Last-Modified`, else 1 hour. Then
  1 hour under 7 days, 1 day under 6 months, 10 days when older. `cache.ttl_override` sets a
  fixed TTL per source (`github`, `endoflife`, `osv`) or host.
- A stale GET is revalidated with `If-None-Match` and `If-Modified-Since`. A stale POST is sent
  again. A fresh entry sends nothing, so a second identical digest makes 0 requests.
- Files are `http/v1/<2 hex>/<sha256>.zst`: one line of plain JSON metadata, then the body as zstd
  (`klauspost/compress`, the version tfcensus uses). Writes go through a temporary file and a
  rename. A new format gets a new directory.
- Parsed mdindex indexes stay in memory. Parsing the 1.19 MB claude-code changelog takes 7 ms.
- `--offline` serves any entry, stale ones too, and fails on a miss with the request and a hint.
  `--refresh` revalidates every GET and resends every POST. `--no-cache` skips the cache.
- `cache prune` deletes entries unused for 30 days (`--older-than`), unreadable entries, old
  format directories and temporary files older than an hour. Expired entries stay, since their
  ETags make the next revalidation a free 304. `cache clear` deletes `http/`.

## Consequences
- No new HTTP dependency. The cache is about 450 lines with its own tests.
- `--refresh` costs one GraphQL point per query, since POSTs can't be revalidated.
- A cached entry for a private repository is served to a later run with any token on the same
  machine, since only the presence of a token is in the key.
