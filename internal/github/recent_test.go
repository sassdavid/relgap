package github_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

// graphQLCounter counts GraphQL requests on their way to the next transport.
type graphQLCounter struct {
	next  http.RoundTripper
	count atomic.Int32
}

func (c *graphQLCounter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && req.URL.Path == "/graphql" {
		c.count.Add(1)
	}
	return c.next.RoundTrip(req)
}

// recent45 is 45 tool repositories; the first four list versions as tags.
func recent45() []github.RecentQuery {
	tags := []string{"golang/go", "kubernetes/kubernetes", "aws/aws-cli", "python/cpython"}
	releases := []string{
		"hashicorp/terraform", "opentofu/opentofu", "gruntwork-io/terragrunt", "helm/helm",
		"argoproj/argo-cd", "cli/cli", "derailed/k9s", "astral-sh/uv", "astral-sh/ruff", "jdx/mise",
		"nodejs/node", "golangci/golangci-lint", "mikefarah/yq", "jqlang/jq", "goreleaser/goreleaser",
		"sigstore/cosign", "anchore/syft", "rhysd/actionlint", "mvdan/gofumpt", "kubernetes-sigs/kind",
		"kubernetes-sigs/kustomize", "fluxcd/flux2", "hashicorp/vault", "hashicorp/packer",
		"hashicorp/consul", "hashicorp/nomad", "terraform-linters/tflint", "aquasecurity/trivy",
		"junegunn/fzf", "BurntSushi/ripgrep", "sharkdp/bat", "sharkdp/fd", "dandavison/delta",
		"jesseduffield/lazygit", "starship/starship", "direnv/direnv", "koalaman/shellcheck", "mvdan/sh",
		"pulumi/pulumi", "gohugoio/hugo", "casey/just",
	}
	qs := make([]github.RecentQuery, 0, len(tags)+len(releases))
	for _, r := range tags {
		qs = append(qs, github.RecentQuery{Repo: r, Tags: true})
	}
	for _, r := range releases {
		qs = append(qs, github.RecentQuery{Repo: r})
	}
	return qs
}

// TestSpec_source_adapters_6: the latest versions of 45 tools take at most 2 GraphQL queries.
func TestSpec_source_adapters_6(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "testdata", "cassettes", "github", "recent-45"), ghtest.Scrubber())
	counter := &graphQLCounter{next: rt}
	logger := slog.New(slog.DiscardHandler)
	hc := httpx.New(httpx.Options{Logger: logger, Transport: counter})
	c, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: logger})
	require.NoError(t, err)

	qs := recent45()
	require.Len(t, qs, 45)
	got, err := c.Recent(t.Context(), qs)
	require.NoError(t, err)

	assert.LessOrEqual(t, int(counter.count.Load()), 2, "GraphQL queries for 45 repositories")
	assert.Len(t, got, len(qs))
	for _, q := range qs {
		r := got[q.Repo]
		if assert.NoError(t, r.Err, q.Repo) {
			assert.NotEmpty(t, r.Releases, q.Repo)
			assert.LessOrEqual(t, len(r.Releases), github.RecentWindow, q.Repo)
		}
	}
	assert.Contains(
		t,
		tagsOf(got["helm/helm"].Releases),
		"v3.22.1",
		"the window holds the newest v3 release next to v4",
	)
	assert.Contains(t, tagsOf(got["golang/go"].Releases), "go1.27.4")
}

func tagsOf(rs []github.Release) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Tag
	}
	return out
}

// restRecentAPI serves the REST calls Recent makes without a token.
type restRecentAPI struct {
	mu    sync.Mutex
	paths []string
}

func (a *restRecentAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.paths = append(a.paths, r.URL.Path+"?"+r.URL.RawQuery)
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/repos/o/rel/releases":
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v1.1.0", "published_at": "2026-02-01T00:00:00Z"},
			{"tag_name": "v1.0.0", "published_at": "2026-01-01T00:00:00Z"},
		})
	case "/repos/o/tags/tags":
		_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "v2.0.0"}, {"name": "v1.9.0"}})
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}
}

func TestRecent_REST(t *testing.T) {
	t.Parallel()
	api := &restRecentAPI{}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	logger := slog.New(slog.DiscardHandler)
	hc := httpx.New(httpx.Options{Logger: logger, Transport: redirectTo(t, srv.URL)})
	c, err := github.New(hc, github.Options{Logger: logger})
	require.NoError(t, err)

	got, err := c.Recent(t.Context(), []github.RecentQuery{
		{Repo: "o/rel"}, {Repo: "o/tags", Tags: true}, {Repo: "o/missing"}, {Repo: "not-a-repo"}, {Repo: "o/rel"},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"v1.1.0", "v1.0.0"}, tagsOf(got["o/rel"].Releases))
	assert.Equal(t, []string{"v2.0.0", "v1.9.0"}, tagsOf(got["o/tags"].Releases))
	require.ErrorIs(t, got["o/missing"].Err, github.ErrNotFound)
	require.Error(t, got["not-a-repo"].Err)
	assert.Contains(t, api.paths, "/repos/o/rel/releases?per_page=30", "one page of the newest releases")
}

// redirectTo sends every request to base.
func redirectTo(t *testing.T, base string) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		out := req.Clone(req.Context())
		out.URL.Scheme, out.URL.Host = "http", base[len("http://"):]
		return http.DefaultTransport.RoundTrip(out)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
