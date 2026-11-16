# relgap ADR-0013: Parallel "latest" lookup in `outdated`

- Status: accepted (2026-11-16)
- Amends the batch size in [ADR-0009](0009-outdated-current-and-latest.md) and criterion 6 of
  [source-adapters.md](../specs/source-adapters.md).

## Context
ADR-0009 reads the 30 newest releases or tags of every tool with one GraphQL query per 25
repositories, so 45 tools take 2 queries. On 2026-11-16 a cold `relgap outdated` in dotfriedrice
(50 tools) took 10.57 s against the 10 s target in [overview.md](../overview.md). The trace in
[testing.md](../testing.md#measured-performance) shows where the time went:

- The two 25-repository queries took 4.2 s and 4.7 s. Timed alone, each repository takes about
  0.5 s, so GitHub answers the aliased repositories of one query one after another.
- The full version lists of `node = "26"` and `python = "3.14"` (13 pages of 100) started only
  after those queries returned, although a prefix pin always needs the full list.
- endoflife.date and OSV ran one after the other, after the GitHub phase.

Live timings of the same 50 repositories, 8 queries in flight at most:

| Repositories per query | Queries | Median of 3 |
|---|---|---|
| 25 | 2 | 4.78 s |
| 13 | 4 | 2.72 s |
| 10 | 5 | 2.37 s |
| 7 | 8 | 1.76 s |
| 5 | 10 | 2.30 s |

Every query costs 1 point of the primary rate limit and 1 point of GitHub's secondary limit of
2,000 points per minute. GitHub's GraphQL guide says to split large queries, and it ends any
request that takes over 10 s.

## Decision
- `github.Client.Recent` sends one query per concurrent GitHub request (8 by default, ADR-0005):
  `ceil(n / concurrency)` repositories per query, at least 10 and at most 25. 45 tools take
  5 queries. Up to 10 tools still take one query.
- `outdated` starts the full version list of every tool with a prefix or constraint pin while
  the batched lookup runs. It sends the same requests as before.
- `enrich.Enricher` asks endoflife.date and OSV at the same time. OSV keeps one batch: a batch of
  13 queries took as long as its slowest query (cpython, about 1 s), and 13 single queries took
  longer.
- A prefix pin still resolves against the full list, as ADR-0009 says. The 30-release window can
  miss the newest version of a prefix such as `3` when a project ships many backports.

## Consequences
- 45 tools cost 5 points instead of 2. A run with a token stays far below 5,000 points an hour.
- The 10-repository minimum keeps one query for small projects, so the outdated and drift
  cassettes (at most 9 repositories) didn't change. `github/recent-45` was re-recorded.
- With 5 batch queries and 2 version lists, 7 of the 8 GitHub slots are busy, so the lists don't
  queue behind the batches.
