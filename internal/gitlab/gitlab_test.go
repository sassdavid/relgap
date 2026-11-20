package gitlab_test

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

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/gitlab"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

func client(t *testing.T, rt http.RoundTripper, o gitlab.Options) *gitlab.Client {
	t.Helper()
	hc := httpx.New(httpx.Options{
		Logger:    slog.New(slog.DiscardHandler),
		Transport: rt,
		Limits:    map[string]httpx.Limit{},
		Default: httpx.Limit{
			Concurrency: 4,
		},
		RetryMax:     1,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: time.Millisecond,
	})
	return gitlab.New(hc, o)
}

// TestNestedGroupProject reads a nested-group project on gitlab.com: two pages of tags, the
// releases with their notes, and nothing else.
func TestNestedGroupProject(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "testdata", "cassettes", "non-github", "gitlab"), vcr.Scrubber{})
	c := client(t, rt, gitlab.Options{Host: "gitlab.com"})
	tags, err := c.Tags(t.Context(), "example-group/platform/tf-dns")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1.3.0", "v1.2.1", "v1.2.0", "v1.1.0", "v1.0.0"}, tags)

	rels, err := c.ReleaseBodies(t.Context(), "example-group/platform/tf-dns", []string{"v1.3.0", "v1.0.0"})
	require.NoError(t, err)
	require.Len(t, rels, 1, "v1.0.0 has no release")
	assert.Equal(t, "https://gitlab.com/example-group/platform/tf-dns/-/releases/v1.3.0", rels["v1.3.0"].URL)
	assert.Contains(t, rels["v1.3.0"].Body, "`hosted_zone_id`")

	self := client(t, rt, gitlab.Options{Host: "gitlab.internal.test"})
	refs, err := self.TagRefs(t.Context(), "infra/modules/tf-vpc")
	require.NoError(t, err)
	assert.Len(t, refs, 3)
	all, err := self.Releases(t.Context(), "infra/modules/tf-vpc")
	require.NoError(t, err)
	assert.Empty(t, all)
	files, err := self.RootFiles(t.Context(), "infra/modules/tf-vpc")
	require.NoError(t, err)
	assert.Equal(t, []github.File{
		{Name: "CHANGELOG.md", Size: -1}, {Name: "modules", Size: -1, Dir: true}, {Name: "main.tf", Size: -1},
	}, files)
	b, err := self.File(t.Context(), "infra/modules/tf-vpc", "", "CHANGELOG.md")
	require.NoError(t, err)
	assert.Contains(t, string(b), "## 0.5.0")
}

// fake answers each URL with a body and records the token headers.
type fake struct {
	mu      sync.Mutex
	bodies  map[string]string
	headers map[string]string // Link header by URL
	sent    []string
}

func (f *fake) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, req.URL.String()+" private="+req.Header.Get("Private-Token")+
		" job="+req.Header.Get("Job-Token"))
	body, ok := f.bodies[req.URL.String()]
	h := http.Header{"Content-Type": {"application/json"}}
	if l := f.headers[req.URL.String()]; l != "" {
		h.Set("Link", l)
	}
	code := http.StatusOK
	if !ok {
		code = http.StatusNotFound
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestTokensAndLinks(t *testing.T) {
	t.Parallel()
	const tags = "https://gitlab.com/api/v4/projects/a%2Fb/repository/tags?per_page=100"
	f := &fake{
		bodies: map[string]string{tags: `[{"name":"v1.0.0","commit":{"created_at":"2026-01-01T00:00:00Z"}}]`},
		headers: map[string]string{
			tags: `<https://evil.test/api/v4/projects/a%2Fb/repository/tags?page=2>; rel="next"`,
		},
	}
	_, err := client(t, f, gitlab.Options{Host: "gitlab.com", Token: "pat"}).Tags(t.Context(), "a/b")
	require.Error(t, err, "a Link off the host is refused")
	assert.Equal(t, []string{tags + " private=pat job="}, f.sent)

	f = &fake{bodies: map[string]string{tags: `[]`}}
	_, err = client(t, f, gitlab.Options{Host: "gitlab.com", Token: "job", TokenHeader: gitlab.HeaderJobToken}).
		Tags(t.Context(), "a/b")
	require.NoError(t, err)
	assert.Equal(t, []string{tags + " private= job=job"}, f.sent)

	f = &fake{bodies: map[string]string{
		"https://gitlab.com/api/v4/projects/a%2Fb/repository/files/docs%2FCHANGES.md/raw?ref=v1.0.0": "x",
	}}
	b, err := client(t, f, gitlab.Options{Host: "gitlab.com"}).File(t.Context(), "a/b", "v1.0.0", "docs/CHANGES.md")
	require.NoError(t, err, "the file path is one encoded segment")
	assert.Equal(t, "x", string(b))

	_, err = client(t, f, gitlab.Options{Host: "gitlab.com"}).Tags(t.Context(), "single")
	require.ErrorIs(t, err, gitlab.ErrBadRepo)
	_, err = client(t, f, gitlab.Options{Host: "gitlab.com"}).Tags(t.Context(), "a/missing")
	require.ErrorIs(t, err, httpx.ErrNotFound)
}
