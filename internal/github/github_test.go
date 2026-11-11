package github_test

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
)

// fakeAPI answers the GraphQL and REST calls the client makes, for a repo with releases v1..vN.
type fakeAPI struct {
	t        *testing.T
	releases int
	maxAlias int // GraphQL queries with more aliases fail with a timeout error
	maxFirst int // list queries asking for more fail
	status   int // when set, every request gets this status

	mu       sync.Mutex
	auth     []string
	gqlCalls []string
	paths    []string
	accept   []string // Accept headers of contents requests
	refs     []string // ref query values of contents requests
}

var aliasRe = regexp.MustCompile(`r(\d+): release\(tagName: \$t\d+\)`)

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if r.URL.Path == "/graphql" {
		f.graphql(w, r)
		return
	}
	f.rest(w, r)
}

func (f *fakeAPI) release(n int) map[string]any {
	return map[string]any{
		"tagName": "v" + strconv.Itoa(n), "isDraft": n == f.releases, "isPrerelease": false,
		"publishedAt": "2026-01-02T03:04:05Z", "url": "https://github.com/o/r/releases/tag/v" + strconv.Itoa(n),
	}
}

func (f *fakeAPI) graphql(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if !assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&in)) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.gqlCalls = append(f.gqlCalls, in.Query)
	f.mu.Unlock()
	if aliases := aliasRe.FindAllStringSubmatch(in.Query, -1); len(aliases) > 0 {
		if len(aliases) > f.maxAlias {
			writeJSON(w, map[string]any{"errors": []map[string]any{{"message": "timeout"}}})
			return
		}
		repo := map[string]any{}
		for _, a := range aliases {
			tag, _ := in.Variables["t"+a[1]].(string)
			n, err := strconv.Atoi(strings.TrimPrefix(tag, "v"))
			if err != nil || n > f.releases {
				repo["r"+a[1]] = nil
				continue
			}
			node := f.release(n)
			node["description"] = "body of " + tag
			repo["r"+a[1]] = node
		}
		writeJSON(w, map[string]any{"data": map[string]any{"repository": repo}})
		return
	}
	first := int(in.Variables["first"].(float64))
	if first > f.maxFirst {
		writeJSON(w, map[string]any{"errors": []map[string]any{{"message": "something went wrong"}}})
		return
	}
	start := 0
	if c, ok := in.Variables["cursor"].(string); ok {
		start, _ = strconv.Atoi(c)
	}
	var nodes []map[string]any
	for n := start + 1; n <= min(start+first, f.releases); n++ {
		nodes = append(nodes, f.release(n))
	}
	end := min(start+first, f.releases)
	writeJSON(w, map[string]any{"data": map[string]any{"repository": map[string]any{"releases": map[string]any{
		"nodes":    nodes,
		"pageInfo": map[string]any{"hasNextPage": end < f.releases, "endCursor": strconv.Itoa(end)},
	}}}})
}

func (f *fakeAPI) rest(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/repos/o/r/releases":
		page, _ := strconv.Atoi(cmp.Or(r.URL.Query().Get("page"), "1"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		var out []map[string]any
		for n := (page-1)*perPage + 1; n <= min(page*perPage, f.releases); n++ {
			out = append(out, map[string]any{
				"tag_name":   "v" + strconv.Itoa(n),
				"draft":      false,
				"prerelease": false,
				"html_url": "https://github.com/o/r/releases/tag/v" + strconv.Itoa(
					n,
				),
				"body":         "body of v" + strconv.Itoa(n),
				"published_at": "2026-01-02T03:04:05Z",
			})
		}
		if page*perPage < f.releases {
			next := *r.URL
			q := next.Query()
			q.Set("page", strconv.Itoa(page+1))
			next.RawQuery = q.Encode()
			next.Scheme, next.Host = "https", "api.github.com"
			w.Header().Set("Link", `<`+next.String()+`>; rel="next"`)
		}
		writeJSON(w, out)
	case strings.HasPrefix(r.URL.Path, "/repos/o/r/contents/"):
		f.mu.Lock()
		f.accept = append(f.accept, r.Header.Get("Accept"))
		f.refs = append(f.refs, r.URL.Query().Get("ref"))
		f.mu.Unlock()
		if r.URL.Path != "/repos/o/r/contents/doc/change logs/CHANGELOG.md" {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{"message": "Not Found"})
			return
		}
		_, _ = w.Write([]byte("## 1.0.0\n\n- a\n"))
	case r.URL.Path == "/repos/o/r/tags":
		writeJSON(w, []map[string]any{{"name": "v1"}, {"name": "v2"}})
	case strings.HasPrefix(r.URL.Path, "/repos/o/r/releases/tags/"):
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"message": "Not Found"})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// redirect sends api.github.com requests to the fake server.
type redirect struct {
	base *url.URL
	rt   http.RoundTripper
}

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host = r.base.Scheme, r.base.Host
	return r.rt.RoundTrip(out)
}

func newClient(t *testing.T, f *fakeAPI, token string) *github.Client {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	require.NoError(t, err)
	hc := httpx.New(httpx.Options{
		Logger:       slog.New(slog.DiscardHandler),
		Transport:    redirect{base: base, rt: srv.Client().Transport},
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: time.Millisecond,
	})
	c, err := github.New(hc, github.Options{Token: token, Logger: slog.New(slog.DiscardHandler), Concurrency: 2})
	require.NoError(t, err)
	return c
}

func TestGraphQLReleasesShrinkPageSize(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{releases: 120, maxFirst: 50, maxAlias: 10}
	c := newClient(t, f, "tok")
	require.True(t, c.Authenticated())

	rs, err := c.Releases(t.Context(), "o/r")
	require.NoError(t, err)
	require.Len(t, rs, 120)
	assert.Equal(t, "v1", rs[0].Tag)
	assert.True(t, rs[119].Draft)
	assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), rs[0].PublishedAt)
	assert.Equal(t, "https://github.com/o/r/releases/tag/v1", rs[0].URL)
	assert.Empty(t, rs[0].Body, "listing does not fetch bodies")
	// 100 fails, then 50, 50, 20.
	assert.Len(t, f.gqlCalls, 4)
	for _, a := range f.auth {
		assert.Equal(t, "Bearer tok", a)
	}
}

func TestGraphQLBodiesSplitFailedBatches(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{releases: 30, maxFirst: 100, maxAlias: 2}
	c := newClient(t, f, "tok")
	tags := []string{"v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8", "v9", "v10", "v99"}

	got, err := c.ReleaseBodies(t.Context(), "o/r", tags)
	require.NoError(t, err)
	require.Len(t, got, 10, "v99 has no release")
	assert.Equal(t, "body of v7", got["v7"].Body)
	assert.NotContains(t, got, "v99")
}

func TestRESTWithoutToken(t *testing.T) {
	t.Parallel()
	f := &fakeAPI{releases: 150}
	c := newClient(t, f, "")
	require.False(t, c.Authenticated())

	rs, err := c.Releases(t.Context(), "o/r")
	require.NoError(t, err)
	require.Len(t, rs, 150)
	assert.Equal(t, "body of v150", rs[149].Body)

	calls := len(f.paths)
	got, err := c.ReleaseBodies(t.Context(), "o/r", []string{"v3", "v200"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"v3": "body of v3"}, map[string]string{"v3": got["v3"].Body})
	assert.Len(t, f.paths, calls, "bodies come from the listed releases")

	tags, err := c.Tags(t.Context(), "o/r")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1", "v2"}, tags)
	for _, a := range f.auth {
		assert.Empty(t, a)
	}
}

func TestRESTBodiesWithoutListing(t *testing.T) {
	t.Parallel()
	c := newClient(t, &fakeAPI{releases: 3}, "")
	got, err := c.ReleaseBodies(t.Context(), "o/r", []string{"v1"})
	require.NoError(t, err)
	assert.Empty(t, got, "a 404 means no release")
}

func TestFile(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"tok", ""} {
		t.Run("token="+token, func(t *testing.T) {
			t.Parallel()
			f := &fakeAPI{}
			c := newClient(t, f, token)
			got, err := c.File(t.Context(), "o/r", "release-1.0", "doc/change logs/CHANGELOG.md")
			require.NoError(t, err)
			assert.Equal(t, "## 1.0.0\n\n- a\n", string(got))

			_, err = c.File(t.Context(), "o/r", "", "missing.md")
			require.ErrorIs(t, err, github.ErrNotFound)
			assert.Equal(t, []string{"application/vnd.github.raw", "application/vnd.github.raw"}, f.accept)
			assert.Equal(t, []string{"release-1.0", ""}, f.refs)
		})
	}
}

func TestUnauthorized(t *testing.T) {
	t.Parallel()
	c := newClient(t, &fakeAPI{status: http.StatusUnauthorized}, "bad")
	_, err := c.Releases(t.Context(), "o/r")
	require.ErrorIs(t, err, github.ErrAuth)
	assert.Contains(t, err.Error(), "gh auth login")
}

func TestInvalidRepo(t *testing.T) {
	t.Parallel()
	c := newClient(t, &fakeAPI{}, "")
	_, err := c.Releases(t.Context(), "no-slash")
	require.ErrorContains(t, err, "want owner/name")
}

type fakeRunner struct {
	out  string
	err  error
	args []string
}

func (r *fakeRunner) Run(_ context.Context, c execx.Cmd) (*execx.Result, error) {
	r.args = append([]string{c.Name}, c.Args...)
	if r.err != nil {
		return nil, r.err
	}
	return &execx.Result{Stdout: []byte(r.out)}, nil
}

func TestToken(t *testing.T) {
	t.Parallel()
	env := func(kv ...string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1], true
				}
			}
			return "", false
		}
	}
	cfg := func(token string) func(string) string { return func(string) string { return token } }
	tests := []struct {
		name       string
		lookup     func(string) (string, bool)
		config     func(string) string
		runner     *fakeRunner
		wantToken  string
		wantSource string
	}{
		{"GH_TOKEN first", env("GH_TOKEN", "a", "GITHUB_TOKEN", "b"), cfg("c"), &fakeRunner{out: "d"}, "a", "GH_TOKEN"},
		{
			"GITHUB_TOKEN",
			env("GH_TOKEN", " ", "GITHUB_TOKEN", "b"),
			cfg("c"),
			&fakeRunner{out: "d"},
			"b",
			"GITHUB_TOKEN",
		},
		{"gh config", env(), cfg("c"), &fakeRunner{out: "d"}, "c", "gh config"},
		{"gh keyring", env(), cfg(""), &fakeRunner{out: "d\n"}, "d", "gh auth token"},
		{"none", env(), cfg(""), &fakeRunner{err: errors.New("gh not found")}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, src := github.Token(t.Context(), github.TokenOptions{
				Lookup: tt.lookup, ConfigToken: tt.config, Runner: tt.runner,
			})
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, tt.wantSource, src)
			if tt.wantSource == github.SourceGHKeyring {
				assert.Equal(
					t,
					"gh auth token --secure-storage --hostname github.com",
					strings.Join(tt.runner.args, " "),
				)
			}
		})
	}
}

func ExampleToken() {
	token, source := github.Token(context.Background(), github.TokenOptions{
		Lookup: func(k string) (string, bool) {
			return map[string]string{"GITHUB_TOKEN": "t0ken"}[k], k == "GITHUB_TOKEN"
		},
	})
	fmt.Println(len(token), source)
	// Output: 5 GITHUB_TOKEN
}

func TestNewestRelease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body string
		want       time.Time
	}{
		{
			"graphql", `{"data":{"repository":{"releases":{"nodes":[` +
				`{"publishedAt":"2026-10-01T10:00:00Z"},{"publishedAt":"2026-11-02T09:30:00Z"},{"publishedAt":null}]}}}}`,
			time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC),
		},
		{
			"rest", `[{"published_at": "2025-01-05T00:00:00+02:00"},{"published_at":"bad"}]`,
			time.Date(2025, 1, 4, 22, 0, 0, 0, time.UTC),
		},
		{"tags", `{"data":{"repository":{"refs":{"nodes":[{"name":"v1"}]}}}}`, time.Time{}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, github.NewestRelease(nil, []byte(tt.body)), tt.name)
	}
}
