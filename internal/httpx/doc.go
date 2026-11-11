// Package httpx builds the HTTP client every network call goes through: a disk cache with
// ETag revalidation, retries with backoff, a per-host concurrency and rate limit, and a guard that
// stops at an exhausted GitHub rate limit.
package httpx
