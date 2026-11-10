# ADR-0005: Resilient HTTP and API clients

- Status: accepted (2026-11-09)

## Decision
**Generic HTTP** (relgap: GitHub raw/contents, endoflife.date, OSV, web docs). One `internal/httpx` client:
- `hashicorp/go-retryablehttp` v0.7.8: retries 5xx, 429 and connection resets, with exponential backoff + jitter, and honours `Retry-After`.
- Per-host `x/time/rate` limiter and per-host concurrency semaphore. Defaults: api.github.com 8 concurrent, others 4.
- A GitHub-aware middleware that reads `x-ratelimit-remaining/reset`. It slows down below 10% remaining and fails with a clear message instead of hammering.
- An ETag/Last-Modified cache on disk (`$XDG_CACHE_HOME/relgap/http/`). Revalidation sends `If-None-Match`. An **authenticated** GitHub 304 does not count against the rate limit.
- TTLs depend on the age of what's cached, following Renovate: releases under 7 days old → 1 h, under 6 months → 1 day, older → 10 days. `--offline` serves from the cache only. `--refresh` forces revalidation.
- Proxies via `http.ProxyFromEnvironment`. `--ca-bundle` / `SSL_CERT_FILE` (also on macOS and Windows) adds corporate CAs.
- User-Agent: `relgap/<version> (+https://github.com/sassdavid/relgap)`. Never personal data.

**GitHub.**
- `go-github/v93` for REST. `githubv4` for GraphQL, batching 20–50 repos per query with aliases.
- Large files are fetched with `Accept: application/vnd.github.raw`: the contents API returns `encoding: none` above 1 MB (awscli and claude-code changelogs are 1.2–1.5 MB). Avoid raw.githubusercontent.com, which ignores tokens and has been 429-throttled since 2025-05.
- Token resolution: `go-gh/v2/pkg/auth.TokenForHost` (`GH_TOKEN` → `GITHUB_TOKEN` → gh config/keyring). Without a token: warn once, cap concurrency at 2, prefer the cache.

## Consequences
- Every client sits behind a small interface, so tests replay recorded fixtures (go-vcr cassettes / httptest at the HTTP layer, so SDK deserializers really run).
- `-vvv` dumps requests and responses with `Authorization` and `x-amz-security-token` redacted.
