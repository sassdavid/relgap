package remote_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
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
	remote := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil, nil)
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

// TestSpec_tool_resolution_6: the tag prefix of a chart comes from the repository's tags. The
// fixtures in testdata/charttags hold Artifact Hub's versions and every tag of the repository,
// as they were on 2026-11-19.
func TestSpec_tool_resolution_6(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"argo-cd":            "argo-cd-",                  // chart-releaser's default
		"cert-manager":       "v",                         // the chart shares the app's tags
		"cluster-autoscaler": "cluster-autoscaler-chart-", // the app's tags are cluster-autoscaler-<version>
		"external-dns":       "external-dns-helm-chart-",  // the app's tags are v<version>
		"external-secrets":   "helm-chart-",
		"keda":               "v", // kedacore/charts has keda-add-ons-http- tags too
		"metrics-server":     "metrics-server-helm-chart-",
	}
	for name, want := range tests {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "charttags", name+".json"))
		require.NoError(t, err)
		var fx struct {
			Chart    string   `json:"chart"`
			Versions []string `json:"versions"`
			Tags     []string `json:"tags"`
		}
		require.NoError(t, json.Unmarshal(raw, &fx))
		got, ok := remote.ChartTagPrefix(fx.Chart, fx.Versions, fx.Tags)
		assert.True(t, ok, name)
		assert.Equal(t, want, got, name)
	}
	_, ok := remote.ChartTagPrefix("x", []string{"1.0.0"}, []string{"v2.0.0", "x-1.0.0.1"})
	assert.False(t, ok, "no tag ends in a known version")
	got, _ := remote.ChartTagPrefix("x", []string{"1.0.0", "1.1.0"}, []string{"v1.0.0", "x-1.0.0", "11.0.0"})
	assert.Equal(t, "x-", got, "a tie goes to the prefix with the chart's name; 11.0.0 isn't 1.0.0")
}

type fakeTags struct {
	tags []string
	err  error
}

func (f *fakeTags) Tags(context.Context, string) ([]string, error) { return f.tags, f.err }

// TestRemoteChartTags: a chart lookup reads the repository's tags and keeps chart-releaser's
// default prefix when none of them ends in a version Artifact Hub lists.
func TestRemoteChartTags(t *testing.T) {
	t.Parallel()
	find := func(tags *fakeTags) (*registry.Found, error) {
		rt := vcr.ForTest(
			t,
			filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "lookups"),
			vcr.Scrubber{},
		)
		hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
		return remote.New(hc, nil, tags).Find(t.Context(), registry.KindChart, "argo/argo-cd")
	}
	got, err := find(&fakeTags{tags: []string{"v3.0.0", "argo-cd-9.1.0", "argo-cd-9.0.0"}})
	require.NoError(t, err)
	assert.Equal(t, "argo-cd-", got.TagPrefix)

	got, err = find(&fakeTags{tags: []string{"chart-v9.1.0", "chart-v9.0.0", "argo-cd-9.0.0"}})
	require.NoError(t, err)
	assert.Equal(t, "chart-v", got.TagPrefix, "the prefix that covers more versions wins")

	got, err = find(&fakeTags{tags: []string{"v99.0.0"}})
	require.NoError(t, err)
	assert.Equal(t, "argo-cd-", got.TagPrefix, "no evidence keeps the default")

	_, err = find(&fakeTags{err: errors.New("rate limited")})
	require.ErrorContains(t, err, "chart:argo/argo-cd: rate limited")
}

func TestRemoteGuess(t *testing.T) {
	t.Parallel()
	hits := fakeSearcher{
		{FullName: "someone/tool-fork", Stars: 900},
		{FullName: "small/Tool", Stars: 3},
		{FullName: "acme/tool", Stars: 120},
	}
	got, err := remote.New(nil, hits, nil).Find(t.Context(), registry.KindGuess, "tool")
	require.NoError(t, err)
	assert.Equal(t, registry.Found{
		Repo: "acme/tool", Name: "tool", Layer: registry.LayerGuess, Alternatives: []string{"small/Tool"},
	}, *got, "only exact names count, most stars first")

	_, err = remote.New(nil, fakeSearcher{{FullName: "a/tool-x"}}, nil).Find(t.Context(), registry.KindGuess, "tool")
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

// TestRemoteModule: a module's repository comes from the Terraform registry's source field, and
// its tag prefix from the tag of the latest version.
func TestRemoteModule(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "modules"), vcr.Scrubber{})
	r := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil, nil)
	tests := []struct {
		name    string
		want    registry.Found
		wantErr error
	}{
		{
			name: "terraform-aws-modules/vpc/aws",
			want: registry.Found{
				Repo:      "terraform-aws-modules/terraform-aws-vpc",
				TagPrefix: "v",
				Layer:     registry.LayerTerraform,
			},
		},
		{
			name: "cloudposse/label/null",
			want: registry.Found{Repo: "cloudposse/terraform-null-label", Layer: registry.LayerTerraform},
		},
		{name: "terraform-aws-modules/vpc/google", wantErr: registry.ErrUnknownTool},
		{name: "hashicorp/no-such-module-relgap/aws", wantErr: registry.ErrUnknownTool},
		{name: "terraform-aws-modules/vpc", wantErr: registry.ErrUnsupported},
	}
	for _, tt := range tests {
		got, err := r.Find(t.Context(), registry.KindTFModule, tt.name)
		if tt.wantErr != nil {
			require.ErrorIs(t, err, tt.wantErr, tt.name)
			continue
		}
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, *got, tt.name)
	}
}

// TestRemoteChartRepo: a Helm repository URL maps to the Artifact Hub repository at that URL
// that has the chart. An OCI chart is usually listed under its own URL, and a bare OCI registry
// URL can belong to a mirror without the chart.
func TestRemoteChartRepo(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(
		t,
		filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "chart-registries"),
		vcr.Scrubber{},
	)
	r := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil, nil)
	tests := []struct {
		url, chart, want string
	}{
		{"oci://public.ecr.aws/karpenter", "karpenter", "aws-karpenter"},
		{"https://kubernetes-sigs.github.io/external-dns", "external-dns", "external-dns"},
		{"https://charts.jetstack.io", "cert-manager", "cert-manager"},
		{"oci://registry-1.docker.io/bitnamicharts", "redis", ""},
		{"https://charts.example.invalid/stable", "app", ""},
	}
	for _, tt := range tests {
		got, ok, err := r.ChartRepo(t.Context(), tt.url, tt.chart)
		require.NoError(t, err, tt.url)
		assert.Equal(t, tt.want != "", ok, tt.url)
		assert.Equal(t, tt.want, got, tt.url)
	}
}
