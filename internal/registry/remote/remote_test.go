package remote_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/oci"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/registry/remote"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
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
	got, _ := remote.ChartTagPrefix("x", []string{"1.0.0", "1.1.0"},
		[]string{"v1.0.0", "v1.1.0", "x-1.0.0", "x-1.1.0", "11.0.0"})
	assert.Equal(t, "x-", got, "a tie goes to the prefix with the chart's name; 11.0.0 isn't 1.0.0")
	_, ok = remote.ChartTagPrefix("x", []string{"1.0.0", "1.1.0", "2.0.0-rc.1"}, []string{"x-1.0.0"})
	assert.False(t, ok, "no tag names the newest stable version, 1.1.0")
	got, _ = remote.ChartTagPrefix("x", []string{"1.0.0", "1.1.0"}, []string{"x-1.0.0", "1.1.0"})
	assert.Empty(t, got, "a repository that moved to plain tags: only the plain prefix covers the newest")
}

type fakeTags struct {
	tags []string
	err  error
}

func (f *fakeTags) Tags(context.Context, string) ([]string, error) { return f.tags, f.err }

func (f *fakeTags) File(context.Context, string, string, string) ([]byte, error) {
	return nil, errors.New("no files")
}

// TestRemoteChartTags: a chart lookup reads the repository's tags. When none of them names the
// newest version Artifact Hub lists, the chart's versions come from Artifact Hub.
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
	got, err := find(&fakeTags{tags: []string{"v3.0.0", "argo-cd-10.18.0", "argo-cd-9.0.0"}})
	require.NoError(t, err)
	assert.Equal(t, "argo-cd-", got.TagPrefix)

	got, err = find(&fakeTags{tags: []string{"chart-v10.18.0", "chart-v9.0.0", "argo-cd-10.18.0"}})
	require.NoError(t, err)
	assert.Equal(t, "chart-v", got.TagPrefix, "the prefix that covers more versions wins")

	got, err = find(&fakeTags{tags: []string{"chart-v9.1.0", "chart-v9.0.0", "argo-cd-9.0.0"}})
	require.NoError(t, err)
	assert.Empty(t, got.TagPrefix, "tags that stop before the newest chart version say nothing about it")

	got, err = find(&fakeTags{tags: []string{"v99.0.0"}})
	require.NoError(t, err)
	assert.Empty(t, got.TagPrefix, "no tag per chart version")
	assert.NotEmpty(t, got.Versions, "the versions come from Artifact Hub")
	assert.Equal(t, "https://artifacthub.io/packages/helm/argo/argo-cd", got.VersionsURL)

	_, err = find(&fakeTags{err: errors.New("rate limited")})
	require.ErrorContains(t, err, "chart argo/argo-cd: rate limited")
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

// TestSpec_tool_resolution_7: chart:<url>/<name> resolves through the Artifact Hub repository at
// the URL and never searches by name. Two unrelated repositories have a chart called
// secrets-store-csi-driver-provider-aws, and neither is the one at the pin's URL.
func TestSpec_tool_resolution_7(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(
		t,
		filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "chart-at"),
		vcr.Scrubber{},
	)
	r := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil, nil)
	tests := []struct {
		name    string
		want    registry.Found
		wantErr string
	}{
		{
			name: "https://kubernetes-sigs.github.io/external-dns/external-dns",
			want: registry.Found{
				Repo: "kubernetes-sigs/external-dns", Name: "external-dns", TagPrefix: "external-dns-",
				Layer: registry.LayerArtifactHub,
			},
		},
		{
			name: "https://aws.github.io/secrets-store-csi-driver-provider-aws/secrets-store-csi-driver-provider-aws",
			wantErr: "registryUrl https://aws.github.io/secrets-store-csi-driver-provider-aws is not an " +
				"Artifact Hub repository with a chart named secrets-store-csi-driver-provider-aws",
		},
	}
	for _, tt := range tests {
		got, err := r.Find(t.Context(), registry.KindChart, tt.name)
		if tt.wantErr != "" {
			require.ErrorIs(t, err, registry.ErrUnknownTool, tt.name)
			require.ErrorContains(t, err, tt.wantErr, tt.name)
			continue
		}
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, *got, tt.name)
	}
}

func TestSplitChartAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rest, url, chart string
		ok               bool
	}{
		{"https://charts.jetstack.io/cert-manager", "https://charts.jetstack.io", "cert-manager", true},
		{"oci://public.ecr.aws/karpenter/karpenter", "oci://public.ecr.aws/karpenter", "karpenter", true},
		{"jetstack/cert-manager", "", "", false},
		{"https://charts.jetstack.io/", "", "", false},
		{"https://cert-manager", "", "", false},
	}
	for _, tt := range tests {
		u, c, ok := registry.SplitChartAt(tt.rest)
		assert.Equal(t, []any{tt.url, tt.chart, tt.ok}, []any{u, c, ok}, tt.rest)
	}
	assert.Equal(
		t,
		"chart:https://charts.jetstack.io/cert-manager",
		registry.ChartAt("https://charts.jetstack.io/", "cert-manager"),
	)
}

// fixtureGitHub serves tag lists and GitHub Pages indexes from testdata/helmindex, as they were
// on 2026-11-20.
type fixtureGitHub struct{ t *testing.T }

func (f fixtureGitHub) read(name string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "helmindex", name))
	require.NoError(f.t, err)
	return data
}

func (f fixtureGitHub) Tags(_ context.Context, repo string) ([]string, error) {
	var tags []string
	require.NoError(f.t, json.Unmarshal(f.read(filepath.Base(repo)+".tags.json"), &tags))
	return tags, nil
}

func (f fixtureGitHub) File(_ context.Context, repo, ref, path string) ([]byte, error) {
	if repo != "aws/secrets-store-csi-driver-provider-aws" || ref != "gh-pages" || path != "index.yaml" {
		return nil, fmt.Errorf("fetch %s of %s at %q: %w", path, repo, ref, github.ErrNotFound)
	}
	return f.read(filepath.Base(repo) + ".yaml"), nil
}

// TestSpec_tool_resolution_8: a chart whose repository has no tag per chart version takes its
// versions from Artifact Hub, and a GitHub Pages Helm repository that Artifact Hub doesn't list
// is read from its gh-pages index.yaml.
func TestSpec_tool_resolution_8(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "..", "testdata", "cassettes", "resolve", "chart-versions"),
		vcr.Scrubber{})
	r := remote.New(httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt}), nil,
		fixtureGitHub{t})

	// aws/eks-charts tags every release v0.0.N for all its charts.
	got, err := r.Find(t.Context(), registry.KindChart, "https://aws.github.io/eks-charts/aws-for-fluent-bit")
	require.NoError(t, err)
	assert.Equal(t, "aws/eks-charts", got.Repo)
	assert.Empty(t, got.TagPrefix)
	assert.Equal(t, "https://artifacthub.io/packages/helm/aws/aws-for-fluent-bit", got.VersionsURL)
	assert.Contains(t, tagsOfRefs(got.Versions), "0.1.35")
	assert.Equal(t, registry.LayerArtifactHub, got.Layer)

	// Artifact Hub lists no repository at this URL. The chart's tags moved from
	// secrets-store-csi-driver-provider-aws-<version> to plain <version> at 3.0.0.
	got, err = r.Find(t.Context(), registry.KindChart,
		"https://aws.github.io/secrets-store-csi-driver-provider-aws/secrets-store-csi-driver-provider-aws")
	require.NoError(t, err)
	assert.Equal(t, registry.Found{
		Repo: "aws/secrets-store-csi-driver-provider-aws", Name: "secrets-store-csi-driver-provider-aws",
		Layer: registry.LayerHelmRepo,
	}, *got)

	_, err = r.Find(
		t.Context(),
		registry.KindChart,
		"https://aws.github.io/secrets-store-csi-driver-provider-aws/other",
	)
	require.ErrorIs(t, err, registry.ErrUnknownTool)
	require.ErrorContains(t, err, "the Helm repository at https://aws.github.io/secrets-store-csi-driver-provider-aws "+
		"has no chart named other")
}

func tagsOfRefs(refs []version.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Tag
	}
	return out
}

// TestRemoteImage looks images up through the registry: example/api's label names a GitHub
// repository, the agent's names GitLab and gives no repository, and the tag pattern picks the
// newest stable tag with the variant for the label.
func TestRemoteImage(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(
		t,
		filepath.Join("..", "..", "..", "testdata", "cassettes", "non-github", "images"),
		vcr.Scrubber{},
	)
	hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
	r := remote.New(hc, nil, nil).WithImages(oci.New(hc, oci.Options{}))

	found, err := r.Find(t.Context(), registry.KindImage, "example/api")
	require.NoError(t, err)
	assert.Equal(t, "example-org/api", found.Repo)
	assert.Equal(t, "image:example/api", found.Name)
	assert.Equal(t, "docker.io/example/api", found.Image)
	assert.Len(t, found.Versions, 7, "every tag; the universe applies the pattern")

	found, err = r.Find(t.Context(), registry.KindImage, "public.ecr.aws/example/agent:*-al2023")
	require.NoError(t, err)
	assert.Empty(t, found.Repo, "a GitLab source isn't a GitHub repository")
	assert.Equal(t, "-al2023", found.TagSuffix)

	_, err = r.Find(t.Context(), registry.KindImage, "registry.internal.test/team/app")
	require.ErrorIs(t, err, registry.ErrUnsupported)
	_, err = remote.New(hc, nil, nil).Find(t.Context(), registry.KindImage, "alpine")
	require.ErrorIs(t, err, registry.ErrUnsupported, "without an image reader")
}

// TestSpec_tool_resolution_9 reads a chart behind an oci:// registryUrl that Artifact Hub doesn't list
// from the registry's tags. demo's source annotation names a GitHub repository whose tags cover
// the newest chart version, so it gets notes; plain names none and gets versions only; a registry
// relgap doesn't read gets no request.
func TestSpec_tool_resolution_9(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(
		t,
		filepath.Join("..", "..", "..", "testdata", "cassettes", "non-github", "oci-charts"),
		vcr.Scrubber{},
	)
	hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
	gh := &fakeTags{tags: []string{"demo-1.0.0", "demo-1.1.0", "demo-1.2.1+build.5", "v9.0.0"}}
	r := remote.New(hc, nil, gh).WithImages(oci.New(hc, oci.Options{}))

	found, err := r.Find(t.Context(), registry.KindChart, "oci://quay.io/example/charts/demo")
	require.NoError(t, err)
	assert.Equal(t, registry.Found{
		Repo: "example-org/demo", Name: "demo", TagPrefix: "demo-", Layer: registry.LayerHelmRepo,
	}, *found)

	found, err = r.Find(t.Context(), registry.KindChart, "registry-1.docker.io/examplecharts/plain")
	require.NoError(t, err, "the short form of oci://registry-1.docker.io/examplecharts/plain")
	assert.Empty(t, found.Repo)
	assert.Equal(t, "plain", found.Name)
	assert.Equal(t, []string{"0.1.0", "0.2.0", "0.3.0"}, tagsOfRefs(found.Versions))
	assert.Equal(t, "https://registry-1.docker.io/v2/examplecharts/plain/tags/list", found.VersionsURL)

	_, err = r.Find(t.Context(), registry.KindChart, "oci://registry.internal.test/charts/x")
	require.ErrorIs(t, err, registry.ErrUnsupported)
	assert.ErrorContains(t, err, "images.registries")
}
