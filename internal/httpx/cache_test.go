package httpx_test

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/httpx"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // fixed test clock

const day = 24 * time.Hour

func TestTTL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		age  time.Duration
		want time.Duration
	}{
		{-1, time.Hour},
		{0, time.Hour},
		{7*day - time.Second, time.Hour},
		{7 * day, day},
		{181 * day, day},
		{182 * day, 10 * day},
		{5 * 365 * day, 10 * day},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, httpx.TTL(tt.age), "age %s", tt.age)
	}
}

// origin serves one versioned document with an ETag and counts requests and 304s.
type origin struct {
	srv          *httptest.Server
	requests     atomic.Int32
	notModified  atomic.Int32
	lastModified time.Time
	etag         atomic.Value
	status       atomic.Int32
}

func newOrigin(t *testing.T, lastModified time.Time) *origin {
	t.Helper()
	o := &origin{lastModified: lastModified}
	o.etag.Store(`"v1"`)
	o.status.Store(http.StatusOK)
	o.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.requests.Add(1)
		etag, _ := o.etag.Load().(string)
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", o.lastModified.Format(http.TimeFormat))
		w.Header().Set("Set-Cookie", "session=cookie-secret")
		w.Header().Set("X-Ratelimit-Remaining", "4999")
		if r.Header.Get("If-None-Match") == etag {
			o.notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(int(o.status.Load()))
		_, _ = io.WriteString(w, "body "+etag)
	}))
	t.Cleanup(o.srv.Close)
	return o
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func cachedClient(t *testing.T, transport http.RoundTripper, dir string, mode httpx.Mode, clk *clock,
	override map[string]time.Duration,
) (*http.Client, *httpx.Cache) {
	t.Helper()
	c := httpx.NewCache(httpx.CacheOptions{
		Dir:         dir,
		Mode:        mode,
		Clock:       clk.Now,
		Logger:      slog.New(slog.DiscardHandler),
		TTLOverride: override,
	})
	t.Cleanup(func() { _ = c.Close() })
	hc := httpx.New(httpx.Options{
		Logger:    slog.New(slog.DiscardHandler),
		Transport: transport,
		Limits:    map[string]httpx.Limit{},
		Default:   httpx.Limit{Concurrency: 4},
		RetryMax:  1,
		Clock:     clk.Now,
		RetryPOST: true,
		Wrap:      c.Wrap,
	})
	return hc, c
}

func body(t *testing.T, c *http.Client, url string, header ...string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func TestCacheAgeTTLAndRevalidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		modified time.Duration // how long before t0 the document last changed
		ttl      time.Duration
	}{
		{"young", 2 * day, time.Hour},
		{"months", 30 * day, day},
		{"old", 400 * day, 10 * day},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := newOrigin(t, t0.Add(-tt.modified))
			clk := &clock{now: t0}
			c, cache := cachedClient(t, o.srv.Client().Transport, t.TempDir(), httpx.ModeDefault, clk, nil)

			assert.Equal(t, `body "v1"`, body(t, c, o.srv.URL))
			clk.now = t0.Add(tt.ttl - time.Second)
			assert.Equal(t, `body "v1"`, body(t, c, o.srv.URL))
			assert.Equal(t, int32(1), o.requests.Load(), "fresh entry needs no request")

			clk.now = t0.Add(tt.ttl)
			assert.Equal(t, `body "v1"`, body(t, c, o.srv.URL))
			assert.Equal(t, int32(2), o.requests.Load())
			assert.Equal(t, int32(1), o.notModified.Load(), "stale entry is revalidated")

			clk.now = t0.Add(2*tt.ttl - time.Second)
			assert.Equal(t, `body "v1"`, body(t, c, o.srv.URL))
			assert.Equal(t, int32(2), o.requests.Load(), "a 304 makes the entry fresh again")

			o.etag.Store(`"v2"`)
			clk.now = t0.Add(3 * tt.ttl)
			assert.Equal(t, `body "v2"`, body(t, c, o.srv.URL), "a changed document replaces the entry")
			assert.Equal(t, httpx.CacheStats{Requests: 3, NotModified: 1, Hits: 2, Stored: 3}, cache.Stats())
		})
	}
}

func TestCacheModes(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	clk := &clock{now: t0}
	dir := t.TempDir()
	tr := o.srv.Client().Transport
	def, _ := cachedClient(t, tr, dir, httpx.ModeDefault, clk, nil)
	body(t, def, o.srv.URL)

	refresh, _ := cachedClient(t, tr, dir, httpx.ModeRefresh, clk, nil)
	assert.Equal(t, `body "v1"`, body(t, refresh, o.srv.URL))
	assert.Equal(t, int32(1), o.notModified.Load(), "--refresh revalidates a fresh entry")

	noCache, nc := cachedClient(t, tr, dir, httpx.ModeNoCache, clk, nil)
	body(t, noCache, o.srv.URL)
	assert.Equal(t, int32(3), o.requests.Load())
	assert.Equal(t, int32(1), o.notModified.Load(), "--no-cache sends no validators")
	assert.Equal(t, httpx.CacheStats{Requests: 1}, nc.Stats())

	clk.now = t0.Add(365 * day)
	offline, _ := cachedClient(t, tr, dir, httpx.ModeOffline, clk, nil)
	assert.Equal(t, `body "v1"`, body(t, offline, o.srv.URL), "--offline serves a stale entry")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, o.srv.URL+"/other", http.NoBody)
	require.NoError(t, err)
	_, err = offline.Do(req)
	require.ErrorIs(t, err, httpx.ErrOffline)
	assert.Contains(t, err.Error(), "GET "+o.srv.URL+"/other; run once without --offline")
	assert.Equal(t, int32(3), o.requests.Load(), "--offline sends nothing")
}

func TestCacheTTLOverride(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	clk := &clock{now: t0}
	host := strings.TrimPrefix(o.srv.URL, "http://")
	c, _ := cachedClient(t, o.srv.Client().Transport, t.TempDir(), httpx.ModeDefault, clk,
		map[string]time.Duration{host: time.Minute})
	body(t, c, o.srv.URL)
	clk.now = t0.Add(time.Minute)
	body(t, c, o.srv.URL)
	assert.Equal(t, int32(1), o.notModified.Load())
}

func TestCacheSkipsErrorsAndKeysOnToken(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	clk := &clock{now: t0}
	c, cache := cachedClient(t, o.srv.Client().Transport, t.TempDir(), httpx.ModeDefault, clk, nil)

	o.status.Store(http.StatusForbidden)
	assert.Equal(t, http.StatusForbidden, status(t, c, o.srv.URL))
	assert.Equal(t, int64(0), cache.Stats().Stored, "only 200 and 404 responses are stored")

	o.status.Store(http.StatusOK)
	body(t, c, o.srv.URL)
	body(t, c, o.srv.URL, "Authorization", "Bearer x")
	body(t, c, o.srv.URL, "Authorization", "Bearer y")
	assert.Equal(t, int32(3), o.requests.Load(), "with and without a token are separate entries")
	assert.Equal(t, int64(2), cache.Stats().Stored)
}

// TestCacheRemembersMisses: a 404 is answered from the cache for an hour, then asked again, and
// --offline serves it too.
func TestCacheRemembersMisses(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	o.status.Store(http.StatusNotFound)
	clk := &clock{now: t0}
	dir := t.TempDir()
	c, cache := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeDefault, clk, nil)
	assert.Equal(t, http.StatusNotFound, status(t, c, o.srv.URL))
	assert.Equal(t, http.StatusNotFound, status(t, c, o.srv.URL))
	assert.Equal(t, int32(1), o.requests.Load(), "the second miss comes from the cache")
	assert.Equal(t, int64(1), cache.Stats().Stored)

	offline, _ := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeOffline, clk, nil)
	assert.Equal(t, http.StatusNotFound, status(t, offline, o.srv.URL))
	assert.Equal(t, int32(1), o.requests.Load(), "--offline sends nothing")

	clk.now = t0.Add(time.Hour)
	o.status.Store(http.StatusOK)
	assert.NotEmpty(t, body(t, c, o.srv.URL), "after an hour the file is asked for again")
	assert.Equal(t, int32(2), o.requests.Load())
}

func status(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := c.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestCacheGraphQL(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q, _ := io.ReadAll(r.Body)
		if bytes.Contains(q, []byte("broken")) {
			_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"no"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"q":`+string(q)+`}}`)
	}))
	t.Cleanup(srv.Close)
	clk := &clock{now: t0}
	cache := httpx.NewCache(httpx.CacheOptions{
		Dir:   t.TempDir(),
		Clock: clk.Now,
		Age: func(_ *http.Request, _ []byte) time.Time {
			return t0.Add(-30 * day) // one-day TTL
		},
	})
	t.Cleanup(func() { _ = cache.Close() })
	c := httpx.New(httpx.Options{
		Logger:    slog.New(slog.DiscardHandler),
		Transport: srv.Client().Transport,
		RetryPOST: true,
		Wrap:      cache.Wrap,
	})
	post := func(q string) string {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/graphql", strings.NewReader(q))
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b)
	}

	assert.JSONEq(t, `{"data":{"q":1}}`, post("1"))
	assert.JSONEq(t, `{"data":{"q":2}}`, post("2"))
	clk.now = t0.Add(day - time.Second)
	assert.JSONEq(t, `{"data":{"q":1}}`, post("1"))
	assert.Equal(t, int32(2), requests.Load(), "the body is part of the key")
	clk.now = t0.Add(day)
	post("1")
	assert.Equal(t, int32(3), requests.Load(), "a stale POST is sent again")

	post("broken")
	post("broken")
	assert.Equal(t, int32(5), requests.Load(), "GraphQL errors are not cached")
}

func TestCacheStoresNoCredentials(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	dir := t.TempDir()
	c, _ := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeDefault, &clock{now: t0}, nil)
	body(t, c, o.srv.URL+"/x?a=1", "Authorization", "Bearer ghp_secretvalue", "Cookie", "id=cookie-secret")

	var files int
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, s := range []string{"ghp_secretvalue", "Bearer", "Authorization", "cookie-secret", "Set-Cookie", "Ratelimit"} {
			assert.NotContains(t, string(raw), s, path)
		}
		head, _, _ := bytes.Cut(raw, []byte("\n"))
		assert.Contains(t, string(head), `"Etag":["\"v1\""]`)
		return nil
	}))
	assert.Equal(t, 1, files)
}

func TestCacheMaintenance(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	dir := t.TempDir()
	clk := &clock{now: t0}
	c, cache := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeDefault, clk, nil)
	body(t, c, o.srv.URL+"/a")
	clk.now = t0.Add(20 * day)
	body(t, c, o.srv.URL+"/b") // /a is now stale, /b fresh

	root := filepath.Join(dir, "http")
	old := filepath.Join(root, "v0", "ab", "old.zst")
	require.NoError(t, os.MkdirAll(filepath.Dir(old), 0o700))
	require.NoError(t, os.WriteFile(old, []byte("x"), 0o600))
	tmp := filepath.Join(root, "v1", "123.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(tmp, clk.now.Add(-2*time.Hour), clk.now.Add(-2*time.Hour)))
	broken := filepath.Join(root, "v1", "cd", "broken.zst")
	require.NoError(t, os.MkdirAll(filepath.Dir(broken), 0o700))
	require.NoError(t, os.WriteFile(broken, []byte("not json"), 0o600))
	require.NoError(t, os.Chtimes(broken, clk.now, clk.now))

	info, err := cache.Info()
	require.NoError(t, err)
	assert.Equal(t, root, info.Dir)
	assert.Equal(t, 4, info.Entries, "both entries, the old-format file and the broken one")
	assert.Equal(t, 1, info.Fresh)
	assert.Equal(t, 3, info.Stale)

	r, err := cache.Prune(30 * day)
	require.NoError(t, err)
	assert.Equal(t, 3, r.Entries, "old format, temp file and unreadable entry")
	assert.NoDirExists(t, filepath.Join(root, "v0"))
	info, err = cache.Info()
	require.NoError(t, err)
	assert.Equal(t, 2, info.Entries, "the stale entry stays for its ETag")

	clk.now = t0.Add(40 * day)
	r, err = cache.Prune(30 * day)
	require.NoError(t, err)
	assert.Equal(t, 1, r.Entries, "/a was last used 40 days ago")

	r, err = cache.Clear()
	require.NoError(t, err)
	assert.Equal(t, 1, r.Entries)
	assert.NoDirExists(t, root)
	info, err = cache.Info()
	require.NoError(t, err)
	assert.Equal(t, 0, info.Entries, "a missing cache is empty")
}
