package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrOffline is returned for a request that --offline can't answer from the cache.
var ErrOffline = errors.New("not in the cache")

// Mode says how the cache answers requests.
type Mode int

// Cache modes, one per flag.
const (
	ModeDefault Mode = iota // serve fresh entries, revalidate stale ones
	ModeOffline             // serve any entry, never touch the network
	ModeRefresh             // revalidate every GET and refetch every POST
	ModeNoCache             // neither read nor write the cache
)

// Release-age TTLs from engineering ADR-0005, after Renovate.
const (
	youngAge = 7 * 24 * time.Hour
	midAge   = 182 * 24 * time.Hour
	youngTTL = time.Hour
	missTTL  = time.Hour // a remembered 404
	midTTL   = 24 * time.Hour
	oldTTL   = 10 * 24 * time.Hour
)

// TTL returns how long an entry stays fresh when the newest thing it describes is age old:
// 1 hour under 7 days, 1 day under 6 months, else 10 days. An unknown (negative) age gets 1 hour.
func TTL(age time.Duration) time.Duration {
	switch {
	case age < 0 || age < youngAge:
		return youngTTL
	case age < midAge:
		return midTTL
	default:
		return oldTTL
	}
}

// AgeFunc returns the newest timestamp in a response body, such as the newest release's publish
// date, or the zero time when there is none.
type AgeFunc func(req *http.Request, body []byte) time.Time

// CacheOptions configures [NewCache].
type CacheOptions struct {
	Dir         string // entries live in Dir/http; empty turns the cache off
	Mode        Mode
	Clock       func() time.Time
	Logger      *slog.Logger
	TTLOverride map[string]time.Duration // by host; replaces the age-based TTL
	Age         AgeFunc                  // nil means only Last-Modified dates an entry
}

// CacheStats counts what the cache did.
type CacheStats struct {
	Requests    int64 // sent to the network
	NotModified int64 // of Requests, answered 304
	Hits        int64 // answered from the cache without a request
	Stored      int64 // new or replaced entries
}

// Cache is a disk cache for GET and GraphQL POST responses. It keeps an allowlist of response
// headers and never stores request headers, so tokens stay out of it.
type Cache struct {
	opts  CacheOptions
	store *store

	requests, notModified, hits, stored atomic.Int64
	staleOnce                           sync.Once
}

// NewCache returns a cache. Zero options get defaults.
func NewCache(o CacheOptions) *Cache {
	if o.Clock == nil {
		o.Clock = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Dir == "" && o.Mode != ModeOffline {
		o.Mode = ModeNoCache
	}
	return &Cache{opts: o, store: newStore(o.Dir)}
}

// Stats returns the counters so far.
func (c *Cache) Stats() CacheStats {
	return CacheStats{
		Requests:    c.requests.Load(),
		NotModified: c.notModified.Load(),
		Hits:        c.hits.Load(),
		Stored:      c.stored.Load(),
	}
}

// LogSummary logs the counters at debug level.
func (c *Cache) LogSummary(ctx context.Context) {
	s := c.Stats()
	c.opts.Logger.DebugContext(ctx, "http summary",
		"requests", s.Requests, "not_modified", s.NotModified, "cache_hits", s.Hits, "stored", s.Stored)
}

// Close releases the decompressor.
func (c *Cache) Close() error {
	c.store.close()
	return nil
}

func (c *Cache) wrap(next http.RoundTripper) http.RoundTripper {
	return &cacheTransport{c: c, next: next}
}

type cacheTransport struct {
	c    *Cache
	next http.RoundTripper
}

func (t *cacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := t.c
	if c.opts.Mode == ModeNoCache || !cacheable(req) {
		return t.send(req)
	}
	body, err := readBody(req)
	if err != nil {
		return nil, err
	}
	key := cacheKey(req, body)
	ctx := req.Context()
	e, hit := c.store.load(key)
	now := c.opts.Clock()
	switch {
	case c.opts.Mode == ModeOffline && hit:
		if !c.fresh(e.meta, now) {
			c.staleOnce.Do(func() {
				c.opts.Logger.InfoContext(ctx, "offline: serving cache entries past their TTL")
			})
		}
		return c.serve(ctx, req, key, e, now, "offline")
	case c.opts.Mode == ModeOffline:
		return nil, fmt.Errorf("%w: %s %s; run once without --offline", ErrOffline, req.Method, req.URL.Redacted())
	case hit && c.opts.Mode == ModeDefault && c.fresh(e.meta, now):
		return c.serve(ctx, req, key, e, now, "hit")
	}

	out := req.Clone(ctx)
	out.Body, out.GetBody = bodyFuncs(body)
	if hit {
		conditional(out, e)
	}
	resp, err := t.send(out)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotModified && hit && e.meta.Status == 0 {
		c.notModified.Add(1)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		e.meta.revalidated(resp.Header, now)
		c.save(ctx, key, e)
		return c.serve(ctx, req, key, e, now, "revalidated")
	}
	if resp.StatusCode == http.StatusNotFound && req.Method == http.MethodGet && !noStore(resp.Header) {
		return c.keepMiss(ctx, req, key, resp, now)
	}
	if resp.StatusCode != http.StatusOK || noStore(resp.Header) {
		return resp, nil
	}
	return c.keep(ctx, req, key, resp, now)
}

// keepMiss remembers a 404 for an hour, so a per-version file that doesn't exist (k9s's
// change_logs/release_v<version>.md) costs no request in the next run and works offline.
func (c *Cache) keepMiss(ctx context.Context, req *http.Request, key string, resp *http.Response,
	now time.Time,
) (*http.Response, error) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", req.URL.Redacted(), err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	m := newMeta(req, resp, now)
	m.Status = http.StatusNotFound
	m.Newest = time.Time{}
	c.save(ctx, key, &entry{meta: m, body: body})
	c.opts.Logger.DebugContext(ctx, "http cache", "outcome", "stored miss", "url", req.URL.Redacted())
	return resp, nil
}

// conditional makes a GET of a stored 200 revalidate it. A remembered 404 has nothing to
// revalidate and is asked for again.
func conditional(req *http.Request, e *entry) {
	if req.Method != http.MethodGet || e.meta.Status != 0 {
		return
	}
	if e.meta.ETag != "" {
		req.Header.Set("If-None-Match", e.meta.ETag)
	}
	if e.meta.LastModified != "" {
		req.Header.Set("If-Modified-Since", e.meta.LastModified)
	}
}

func (t *cacheTransport) send(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err == nil {
		t.c.requests.Add(1)
	}
	return resp, err //nolint:wrapcheck // a transport passes errors through unchanged
}

// keep reads a 200 response, stores it and returns a copy the caller can read.
func (c *Cache) keep(ctx context.Context, req *http.Request, key string, resp *http.Response,
	now time.Time,
) (*http.Response, error) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", req.URL.Redacted(), err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	if req.Method == http.MethodPost && graphQLErrors(body) {
		return resp, nil
	}
	m := newMeta(req, resp, now)
	if c.opts.Age != nil {
		if t := c.opts.Age(req, body); t.After(m.Newest) {
			m.Newest = t
		}
	}
	c.save(ctx, key, &entry{meta: m, body: body})
	c.opts.Logger.DebugContext(ctx, "http cache", "outcome", "stored", "url", req.URL.Redacted())
	return resp, nil
}

func (c *Cache) save(ctx context.Context, key string, e *entry) {
	if c.opts.Mode == ModeOffline {
		return
	}
	if err := c.store.save(key, e, c.opts.Clock()); err != nil {
		c.opts.Logger.WarnContext(ctx, "cannot write http cache entry", "error", err)
		return
	}
	c.stored.Add(1)
}

func (c *Cache) serve(ctx context.Context, req *http.Request, key string, e *entry, now time.Time,
	outcome string,
) (*http.Response, error) {
	if outcome != "revalidated" {
		c.hits.Add(1)
	}
	c.store.touch(key, now)
	c.opts.Logger.DebugContext(ctx, "http cache", "outcome", outcome, "url", req.URL.Redacted())
	code := http.StatusOK
	if e.meta.Status != 0 {
		code = e.meta.Status
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", code, http.StatusText(code)),
		StatusCode:    code,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        e.meta.Header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       req,
	}, nil
}

// fresh reports whether m is younger than its TTL.
func (c *Cache) fresh(m meta, now time.Time) bool {
	return now.Sub(m.StoredAt) < c.ttl(m, now)
}

func (c *Cache) ttl(m meta, now time.Time) time.Duration {
	if m.Status == http.StatusNotFound {
		return missTTL
	}
	if d, ok := c.opts.TTLOverride[m.Host]; ok {
		return d
	}
	if m.Newest.IsZero() {
		return TTL(-1)
	}
	return TTL(now.Sub(m.Newest))
}

// OSVHost is the host of the OSV API, whose batch query is a read-only POST.
const OSVHost = "api.osv.dev"

// cacheable reports whether req is a GET, a GraphQL POST or an OSV batch query, without its own
// conditional headers. The POSTs only read, and the body is part of the key.
func cacheable(req *http.Request) bool {
	if req.Header.Get("If-None-Match") != "" || req.Header.Get("If-Modified-Since") != "" ||
		req.Header.Get("Range") != "" {
		return false
	}
	switch req.Method {
	case http.MethodGet:
		return true
	case http.MethodPost:
		return strings.HasSuffix(req.URL.Path, "/graphql") ||
			(req.URL.Host == OSVHost && req.URL.Path == "/v1/querybatch")
	default:
		return false
	}
}

func readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	b, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	return b, nil
}

func bodyFuncs(b []byte) (io.ReadCloser, func() (io.ReadCloser, error)) {
	if b == nil {
		return http.NoBody, nil
	}
	get := func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
	rc, _ := get()
	return rc, get
}

// cacheKey hashes what selects a response: method, URL, Accept, the request body and whether a
// token was sent. Only the presence of a token goes into the key, never its value.
func cacheKey(req *http.Request, body []byte) string {
	h := sha256.New()
	authed := req.Header.Get("Authorization") != ""
	_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n%t\n", req.Method, req.URL.String(), req.Header.Get("Accept"), authed)
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func noStore(h http.Header) bool {
	return strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-store")
}

// graphQLErrors reports whether a GraphQL reply carries errors, which must not be cached.
func graphQLErrors(body []byte) bool {
	var r struct {
		Errors json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(body, &r) != nil {
		return true
	}
	return len(r.Errors) > 0 && string(r.Errors) != "null"
}
