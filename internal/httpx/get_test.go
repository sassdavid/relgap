package httpx_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/httpx"
)

func TestGetStatusErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		w.Header().Set("WWW-Authenticate", `Bearer realm="x"`) // prose:ignore
		w.WriteHeader(code)
		_, _ = io.WriteString(w, "body")
	}))
	t.Cleanup(srv.Close)
	c := newClient(t, srv, httpx.Limit{Concurrency: 1})
	tests := []struct {
		code             int
		notFound, unauth bool
	}{
		{http.StatusNotFound, true, false},
		{http.StatusGone, true, false},
		{http.StatusUnauthorized, false, true},
		{http.StatusForbidden, false, true},
		{http.StatusTeapot, false, false},
	}
	for _, tt := range tests {
		_, h, err := httpx.Get(t.Context(), c, srv.URL+"/"+strconv.Itoa(tt.code), nil)
		require.Error(t, err)
		assert.Equal(t, tt.notFound, errors.Is(err, httpx.ErrNotFound), tt.code)
		assert.Equal(t, tt.unauth, errors.Is(err, httpx.ErrUnauthorized), tt.code)
		assert.NotEmpty(t, h.Get("WWW-Authenticate"), "the headers of a failed answer come back")
	}
	b, _, err := httpx.Get(t.Context(), c, srv.URL+"/200", http.Header{"Accept": {"text/plain"}})
	require.NoError(t, err)
	assert.Equal(t, "body", string(b))
}

func TestSameOrigin(t *testing.T) {
	t.Parallel()
	base := "https://api.bitbucket.org/2.0"
	assert.True(t, httpx.SameOrigin(base, "https://api.bitbucket.org/2.0/repositories/a/b?page=2"))
	assert.False(t, httpx.SameOrigin(base, "https://evil.test/2.0/repositories/a/b"))
	assert.False(t, httpx.SameOrigin(base, "http://api.bitbucket.org/2.0"))
	assert.False(t, httpx.SameOrigin(base, "https://api.bitbucket.org:8443/2.0"))
}

// TestCacheKeysOnGitLabTokens: PRIVATE-TOKEN and JOB-TOKEN count as a token in the cache key,
// as Authorization does, so an anonymous 404 never answers an authenticated request.
func TestCacheKeysOnGitLabTokens(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	c, cache := cachedClient(t, o.srv.Client().Transport, t.TempDir(), httpx.ModeDefault, &clock{now: t0}, nil)
	body(t, c, o.srv.URL)
	body(t, c, o.srv.URL, "Private-Token", "glpat-secret")
	body(t, c, o.srv.URL, "Job-Token", "job-secret")
	body(t, c, o.srv.URL, "Private-Token", "glpat-other")
	assert.Equal(t, int32(2), o.requests.Load(), "anonymous and with a token are two entries")
	assert.Equal(t, int64(2), cache.Stats().Stored)
}

// TestCacheAnonymousTokenAndNoStore: a request marked with an anonymous registry token shares the
// entry of the same request without a token, so --offline finds it, and a request with
// Cache-Control: no-store, such as a token request, is never stored.
func TestCacheAnonymousTokenAndNoStore(t *testing.T) {
	t.Parallel()
	o := newOrigin(t, t0.Add(-400*day))
	dir := t.TempDir()
	c, cache := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeDefault, &clock{now: t0}, nil)

	req, err := http.NewRequestWithContext(
		httpx.WithAnonymousToken(t.Context()),
		http.MethodGet,
		o.srv.URL,
		http.NoBody,
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer anonymous")
	resp, err := c.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	off, _ := cachedClient(t, o.srv.Client().Transport, dir, httpx.ModeOffline, &clock{now: t0}, nil)
	assert.Equal(t, `body "v1"`, body(t, off, o.srv.URL), "offline, without a token")
	assert.Equal(t, int32(1), o.requests.Load())

	body(t, c, o.srv.URL+"/token", "Cache-Control", "no-store")
	body(t, c, o.srv.URL+"/token", "Cache-Control", "no-store")
	assert.Equal(t, int32(3), o.requests.Load(), "no-store requests always go out")
	assert.Equal(t, int64(1), cache.Stats().Stored)
}

// TestGitLabRateLimitHeaders: GitLab's RateLimit-* headers slow the host down and warn once when
// less than 10% is left, as GitHub's X-RateLimit-* headers do.
func TestGitLabRateLimitHeaders(t *testing.T) {
	t.Parallel()
	reset := time.Now().Add(time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Ratelimit-Limit", "500")
		w.Header().Set("Ratelimit-Remaining", "20")
		w.Header().Set("Ratelimit-Reset", strconv.FormatInt(reset, 10))
		_, _ = io.WriteString(w, "[]")
	}))
	t.Cleanup(srv.Close)
	var logs strings.Builder
	c := httpx.New(httpx.Options{
		Logger:    slog.New(slog.NewTextHandler(&logs, nil)),
		Transport: srv.Client().Transport,
		Limits:    map[string]httpx.Limit{},
		Default:   httpx.Limit{Concurrency: 1, Rate: 100},
	})
	for range 2 {
		_, _, err := httpx.Get(t.Context(), c, srv.URL, nil)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, strings.Count(logs.String(), "rate limit almost used up"))
	assert.Contains(t, logs.String(), "remaining=20 limit=500")
}
