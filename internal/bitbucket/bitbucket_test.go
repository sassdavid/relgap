package bitbucket_test

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/bitbucket"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

func client(t *testing.T, rt http.RoundTripper, token string) *bitbucket.Client {
	t.Helper()
	hc := httpx.New(httpx.Options{
		Logger:    slog.New(slog.DiscardHandler),
		Transport: rt,
		Limits:    map[string]httpx.Limit{},
		Default: httpx.Limit{
			Concurrency: 4,
		},
		RetryMax:     2,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: time.Millisecond,
	})
	return bitbucket.New(hc, bitbucket.Options{Token: token})
}

func cassette(t *testing.T) http.RoundTripper {
	t.Helper()
	return vcr.ForTest(t, filepath.Join("..", "..", "testdata", "cassettes", "non-github", "bitbucket"), vcr.Scrubber{})
}

// TestTagRefs reads two pages of tags after a 429 that succeeds on retry.
func TestTagRefs(t *testing.T) {
	t.Parallel()
	c := client(t, cassette(t), "")
	refs, err := c.TagRefs(t.Context(), "example-ws/tf-network")
	require.NoError(t, err)
	var tags []string
	for _, r := range refs {
		tags = append(tags, r.Tag)
	}
	assert.Equal(t, []string{"1.0.0", "2.0.0", "2.1.0", "2.2.0", "3.0.0", "3.0.0-rc.1"}, tags)
	assert.Equal(t, time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC), refs[4].PublishedAt.UTC())

	files, err := c.RootFiles(t.Context(), "example-ws/tf-network")
	require.NoError(t, err)
	assert.Contains(t, files, github.File{Name: "examples", Dir: true})
	b, err := c.File(t.Context(), "example-ws/tf-network", "", "CHANGELOG.md")
	require.NoError(t, err)
	assert.Contains(t, string(b), "## [3.0.0] - 2026-11-02")

	_, err = c.TagRefs(t.Context(), "example-ws/private-mod")
	require.ErrorIs(t, err, httpx.ErrNotFound, "a private repository answers 404 without a token")
}

// fake answers each URL with a body, records the Authorization header and answers 404 otherwise.
type fake struct {
	mu     sync.Mutex
	bodies map[string]string
	auth   []string
	urls   []string
}

func (f *fake) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, req.Header.Get("Authorization"))
	f.urls = append(f.urls, req.URL.String())
	body, ok := f.bodies[req.URL.String()]
	code := http.StatusOK
	if !ok {
		code = http.StatusNotFound
	}
	return &http.Response{
		StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}, nil
}

const tagsURL = "https://api.bitbucket.org/2.0/repositories/ws/repo/refs/tags?" +
	"fields=values.name%2Cvalues.target.hash%2Cvalues.target.date%2Cnext&pagelen=100"

func TestTokenAndRefs(t *testing.T) {
	t.Parallel()
	f := &fake{bodies: map[string]string{
		tagsURL: `{"values":[{"name":"release/1.0","target":{"hash":"abc123","date":"2026-01-01T00:00:00+00:00"}}],` +
			`"next":"https://evil.test/2.0/repositories/ws/repo/refs/tags?page=2"}`,
	}}
	c := client(t, f, "bb-token")
	_, err := c.TagRefs(t.Context(), "ws/repo")
	require.Error(t, err, "a next link off the API host is refused")
	assert.NotContains(t, strings.Join(f.urls, " "), "evil.test")

	f.bodies[tagsURL] = `{"values":[{"name":"release/1.0","target":{"hash":"abc123","date":"2026-01-01T00:00:00+00:00"}}]}`
	f.bodies["https://api.bitbucket.org/2.0/repositories/ws/repo/src/abc123/docs/CHANGES.md"] = "# Changes\n"
	b, err := c.File(t.Context(), "ws/repo", "release/1.0", "docs/CHANGES.md")
	require.NoError(t, err, "a ref with a slash is read by its commit hash")
	assert.Equal(t, "# Changes\n", string(b))
	for _, a := range f.auth {
		assert.Equal(t, "Bearer bb-token", a)
	}

	_, err = c.File(t.Context(), "ws/repo", "no/such", "x.md")
	require.ErrorIs(t, err, httpx.ErrNotFound)
	_, err = c.TagRefs(t.Context(), "ws")
	require.ErrorIs(t, err, bitbucket.ErrBadRepo)

	rels, err := c.ReleaseBodies(t.Context(), "ws/repo", []string{"release/1.0"})
	require.NoError(t, err)
	assert.Empty(t, rels, "Bitbucket Cloud has no releases")
	assert.NotErrorIs(t, err, httpx.ErrNotFound)
}

func TestAnonymous(t *testing.T) {
	t.Parallel()
	f := &fake{bodies: map[string]string{tagsURL: `{"values":[]}`}}
	c := client(t, f, "")
	_, err := c.Tags(t.Context(), "ws/repo")
	require.NoError(t, err)
	assert.False(t, c.Authenticated())
	assert.Equal(t, []string{""}, f.auth)
}
