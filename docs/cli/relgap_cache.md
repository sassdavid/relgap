## relgap cache

Inspect or clear the HTTP cache

### Synopsis

relgap keeps HTTP responses under $XDG_CACHE_HOME/relgap/http (config cache.dir,
env RELGAP_CACHE_DIR). Each entry holds the response body and a few response headers (content
type, ETag, Last-Modified, Link); request headers and tokens are never stored.

An entry is fresh for a time that depends on the age of the newest release it describes:
1 hour under 7 days, 1 day under 6 months, 10 days when older. endoflife.date and OSV answers
stay fresh for 1 day, and a 404 for 1 hour. cache.ttl_override sets a fixed TTL per source
(github, endoflife, osv) or host. A stale GET is revalidated with If-None-Match, and an
authenticated 304 from GitHub doesn't count against the rate limit.

--offline answers from the cache only, --refresh revalidates everything, --no-cache skips it.

### Options

```
  -h, --help[=true|false]   help for cache
```

### Options inherited from parent commands

```
      --color string            colorize output: auto, always or never (default "auto")
      --concurrency int         maximum concurrent requests per tool (config github.concurrency) (default 8)
      --config string           config file to use instead of the discovered .relgap.yaml (env RELGAP_CONFIG)
      --format string           output format: tty, plain, md, json, sarif or github (default tty when stdout is a terminal, else plain)
      --log-file string         write logs to this file instead of stderr
      --log-format string       log format: text or json (default "text")
      --min-confidence float    hide entries below this confidence, 0..1 (config policy.min_confidence) (default 0.5)
      --no-cache[=true|false]   neither read nor write the HTTP cache
      --no-input[=true|false]   never prompt for input
      --offline[=true|false]    answer from the HTTP cache only and fail on a miss
  -q, --quiet[=true|false]      log errors only
      --refresh[=true|false]    revalidate every cached response, even fresh ones
  -v, --verbose count           more logging: -v info, -vv debug, -vvv trace
```

### SEE ALSO

* [relgap](relgap.md)	 - Release-notes digests and toolchain drift for the tools you pin
* [relgap cache clear](relgap_cache_clear.md)	 - Delete every cache entry
* [relgap cache info](relgap_cache_info.md)	 - Show the cache directory, entry count, size and freshness
* [relgap cache prune](relgap_cache_prune.md)	 - Delete entries not used for a while

