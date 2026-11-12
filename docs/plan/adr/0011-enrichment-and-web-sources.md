# relgap ADR-0011: Enrichment sources, web pages and what the HTTP cache keeps

- Status: accepted (2026-11-12)
- Amends: [ADR-0006](0006-http-cache.md) (which responses the cache stores)

## Context
M8 adds four kinds of requests beyond the GitHub API:
- endoflife.date API v1 (`GET /api/v1/products/{product}`), with an ETag and `max-age=0`.
- OSV: `POST /v1/querybatch`, which returns only `{id, modified}`, then `GET /v1/vulns/{id}`.
  OSV sends no ETag and documents no rate limit; a batch holds at most 1000 queries
  (1001 returns `400 too many queries`, checked 2026-11-12).
- Web pages for the web-doc adapter (go.dev, docs.python.org) and Adoptium's JSON API for
  structured-json.
- Per-version files that often don't exist: k9s's `change_logs/release_v<version>.md`, uv's
  `changelogs/<major>.<minor>.x.md` for the current line, upgrade guides between minors without
  one.

ADR-0006 stores only GET and GraphQL POST replies with status 200. Under that rule every OSV
batch and every missing file costs a request on each run, and `--offline` fails on both.

## Decision
- The cache also stores `POST https://api.osv.dev/v1/querybatch`. The body is part of the key,
  as for GraphQL. The request only reads.
- A GET answered 404 is stored as a miss for 1 hour, then asked for again without conditional
  headers. `--offline` serves it as a 404. This keeps the M4 rule that a second identical digest
  makes no request, for tools with per-version files.
- endoflife.date and OSV answers stay fresh for 1 day. `cache.ttl_override.endoflife` and
  `cache.ttl_override.osv` replace that default.
- Web pages come only from hosts compiled into relgap (`registry.WebHosts`: go.dev,
  docs.python.org, api.adoptium.net), over https, through `internal/httpx`, with each redirect
  checked against the same list and an 8 MB cap. `relgap registry lint` rejects a `url_template`
  on another host. Changelog files stay on the GitHub contents API.
- HTML becomes Markdown with `JohannesKaufmann/html-to-markdown/v2` (v2.5.2, MIT), which adds only
  `JohannesKaufmann/dom` and `golang.org/x/net/html`. A source picks the element to convert and
  the elements to drop with selectors (tag, `#id`, `.class`), checked by lint. The Markdown goes
  through the mdindex section index ([ADR-0001](0001-goldmark-section-index.md)).
- OSV ranges are evaluated locally, as the OSV schema describes, so `fixed_by_target` works
  offline from the cached records: SEMVER for Go (versions turned into OSV's semver form, `go1.24`
  becomes `1.24.0`), npm and crates.io; ECOSYSTEM ranges for PyPI with the PEP 440 scheme; GIT
  ranges through the record's `versions` list. A range relgap can't compare makes the answer
  unknown, and the field is left out.
- Severity is the highest CVSS base score among a record's vectors, computed with
  `pandatix/go-cvss` (v0.6.4, MIT, no dependencies; osv-scanner v2.7.1 uses it), or the advisory's
  own rating when there is no vector.

## Alternatives rejected
- Asking OSV again for the target version to decide `fixed_by_target`: one more query per tool,
  and the answer can't be checked offline. The cassette test asks OSV once to confirm that the
  local answer matches the server's for every vulnerability of kubernetes, terraform and
  claude-code.
- An own HTML converter on `x/net/html`: about 300 lines to keep for two sites.
- Showing raw CVSS vectors only: a future `--vuln-severity` threshold needs a score. CVSS v4.0
  scoring alone is about 2,900 lines in go-cvss.

## Consequences
- A missing file is retried after an hour at most, so a file published since then shows up late
  by up to an hour.
- A cached OSV batch can hide a vulnerability published in the last day. `--refresh` sends the
  batch again.
- Adding a web host is a code change and a release, on purpose.
