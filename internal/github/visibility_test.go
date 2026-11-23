package github_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

// TestVisibility_GraphQL: one aliased query answers for every repository; a repository GitHub
// can't resolve is ErrNotFound and doesn't fail the others.
func TestVisibility_GraphQL(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "testdata", "cassettes", "github", "visibility"), ghtest.Scrubber())
	counter := &graphQLCounter{next: rt}
	logger := slog.New(slog.DiscardHandler)
	hc := httpx.New(httpx.Options{Logger: logger, Transport: counter, RetryPOST: true})
	c, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: logger})
	require.NoError(t, err)

	got, err := c.Visibility(t.Context(), []string{
		"jqlang/jq", "actions/download-artifact", "example-org-relgap-test/no-such-repo", "not-a-repo", "jqlang/jq",
	})
	require.NoError(t, err)

	assert.Equal(t, int32(1), counter.count.Load(), "one GraphQL query")
	require.NoError(t, got["jqlang/jq"])
	require.NoError(t, got["actions/download-artifact"])
	require.ErrorIs(t, got["example-org-relgap-test/no-such-repo"], github.ErrNotFound)
	require.Error(t, got["not-a-repo"])
	assert.Len(t, got, 4)
}

// gqlVisibilityAPI answers aliased repository queries: private repositories have isPrivate set,
// and one it doesn't know is null with a NOT_FOUND error.
type gqlVisibilityAPI struct{ private, missing map[string]bool }

func (a gqlVisibilityAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Variables map[string]string `json:"variables"`
	}
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &in)
	data := map[string]any{}
	var errs []map[string]any
	for k, owner := range in.Variables {
		n, ok := strings.CutPrefix(k, "o")
		if !ok {
			continue
		}
		repo := owner + "/" + in.Variables["n"+n]
		if a.missing[repo] {
			data["r"+n] = nil
			errs = append(errs, map[string]any{
				"type": "NOT_FOUND", "path": []string{"r" + n},
				"message": "Could not resolve to a Repository with the name '" + repo + "'.",
			})
			continue
		}
		data["r"+n] = map[string]any{"isPrivate": a.private[repo]}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "errors": errs})
}

func TestVisibility_Private(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(gqlVisibilityAPI{
		private: map[string]bool{"acme/secret": true},
		missing: map[string]bool{"acme/gone": true},
	})
	t.Cleanup(srv.Close)
	logger := slog.New(slog.DiscardHandler)
	hc := httpx.New(httpx.Options{Logger: logger, Transport: redirectTo(t, srv.URL), RetryPOST: true})
	c, err := github.New(hc, github.Options{Token: "t", Logger: logger})
	require.NoError(t, err)

	repos := make([]string, 0, github.VisibilityBatch+2)
	for range github.VisibilityBatch {
		repos = append(repos, "acme/secret")
	}
	repos = append(repos, "acme/public", "acme/gone")
	got, err := c.Visibility(t.Context(), repos)
	require.NoError(t, err)
	require.ErrorIs(t, got["acme/secret"], github.ErrPrivate)
	require.NoError(t, got["acme/public"])
	require.ErrorIs(t, got["acme/gone"], github.ErrNotFound)
}

// TestVisibility_REST: without a token each repository is fetched; a 404 is ErrNotFound.
func TestVisibility_REST(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/public":
			_, _ = w.Write([]byte(`{"full_name":"o/public","private":false,"visibility":"public"}`))
		case "/repos/o/internal":
			_, _ = w.Write([]byte(`{"full_name":"o/internal","private":false,"visibility":"internal"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	logger := slog.New(slog.DiscardHandler)
	hc := httpx.New(httpx.Options{Logger: logger, Transport: redirectTo(t, srv.URL)})
	c, err := github.New(hc, github.Options{Logger: logger})
	require.NoError(t, err)

	got, err := c.Visibility(t.Context(), []string{"o/public", "o/internal", "o/private"})
	require.NoError(t, err)
	require.NoError(t, got["o/public"])
	require.ErrorIs(t, got["o/internal"], github.ErrPrivate)
	require.ErrorIs(t, got["o/private"], github.ErrNotFound)
}
