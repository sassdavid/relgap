package remote_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/registry/remote"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

type fakeSearcher []remote.RepoHit

func (f fakeSearcher) SearchRepos(context.Context, string) ([]remote.RepoHit, error) { return f, nil }

func TestRemoteFind(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "lookups"), vcr.Scrubber{})
	remote := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil)
	tests := []struct {
		kind, name string
		want       registry.Found
		wantErr    error
	}{
		{
			kind: registry.KindTFProvider, name: "integrations/github",
			want: registry.Found{Repo: "integrations/terraform-provider-github", Layer: registry.LayerTerraform},
		},
		{kind: registry.KindTFProvider, name: "hashicorp/no-such-provider-relgap", wantErr: registry.ErrUnknownTool},
		{
			kind: registry.KindChart, name: "argo/argo-cd",
			want: registry.Found{
				Repo: "argoproj/argo-helm", Name: "argo-cd", TagPrefix: "argo-cd-", Layer: registry.LayerArtifactHub,
			},
		},
		{
			// Eleven repositories have an argo-cd chart; only argo's is official.
			kind: registry.KindChart, name: "argo-cd",
			want: registry.Found{
				Repo: "argoproj/argo-helm", Name: "argo-cd", TagPrefix: "argo-cd-", Layer: registry.LayerArtifactHub,
			},
		},
		{kind: registry.KindChart, name: "external-dns", wantErr: registry.ErrAmbiguous},
		{
			// The package has no repository field; its homepage names the repository.
			kind: registry.KindNPM, name: "@anthropic-ai/claude-code",
			want: registry.Found{Repo: "anthropics/claude-code", Layer: registry.LayerNPM},
		},
		{
			kind: registry.KindNPM, name: "prettier",
			want: registry.Found{Repo: "prettier/prettier", Layer: registry.LayerNPM},
		},
		{kind: registry.KindNPM, name: "no-such-package-relgap", wantErr: registry.ErrUnknownTool},
		{
			kind: registry.KindPyPI, name: "yamllint",
			want: registry.Found{Repo: "adrienverge/yamllint", Layer: registry.LayerPyPI},
		},
		{kind: registry.KindPyPI, name: "no-such-project-relgap", wantErr: registry.ErrUnknownTool},
	}
	for _, tt := range tests {
		got, err := remote.Find(t.Context(), tt.kind, tt.name)
		if tt.wantErr != nil {
			require.ErrorIs(t, err, tt.wantErr, "%s:%s", tt.kind, tt.name)
			continue
		}
		require.NoError(t, err, "%s:%s", tt.kind, tt.name)
		assert.Equal(t, tt.want, *got, "%s:%s", tt.kind, tt.name)
	}
}

func TestRemoteGuess(t *testing.T) {
	t.Parallel()
	hits := fakeSearcher{
		{FullName: "someone/tool-fork", Stars: 900},
		{FullName: "small/Tool", Stars: 3},
		{FullName: "acme/tool", Stars: 120},
	}
	got, err := remote.New(nil, hits).Find(t.Context(), registry.KindGuess, "tool")
	require.NoError(t, err)
	assert.Equal(t, registry.Found{
		Repo: "acme/tool", Name: "tool", Layer: registry.LayerGuess, Alternatives: []string{"small/Tool"},
	}, *got, "only exact names count, most stars first")

	_, err = remote.New(nil, fakeSearcher{{FullName: "a/tool-x"}}).Find(t.Context(), registry.KindGuess, "tool")
	require.ErrorIs(t, err, registry.ErrUnknownTool)
}

func TestParseGitHubURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, repo, sub string
		ok            bool
	}{
		{"https://github.com/psf/black", "psf/black", "", true},
		{"git+https://github.com/prettier/prettier.git", "prettier/prettier", "", true},
		{"git+ssh://git@github.com/a/b.git", "a/b", "", true},
		{"git@github.com:a/b.git", "a/b", "", true},
		{"github:a/b", "a/b", "", true},
		{"https://www.github.com/a/b#readme", "a/b", "", true},
		{
			"https://github.com/argoproj/argo-helm/tree/main/charts/argo-cd",
			"argoproj/argo-helm",
			"charts/argo-cd",
			true,
		},
		{"https://github.com/psf/black/blob/main/CHANGES.md", "psf/black", "CHANGES.md", true},
		{"https://github.com/a/b/issues", "a/b", "", true},
		{"https://gitlab.com/a/b", "", "", false},
		{"https://github.com/a", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		repo, sub, ok := remote.ParseGitHubURL(tt.in)
		assert.Equal(t, tt.ok, ok, tt.in)
		assert.Equal(t, tt.repo, repo, tt.in)
		assert.Equal(t, tt.sub, sub, tt.in)
	}
}
