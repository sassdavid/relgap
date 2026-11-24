package registry_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
)

// fakeFinder answers lookups from a map by kind:name and records the names it was asked.
type fakeFinder struct {
	found map[string]*registry.Found
	asked []string
}

func (f *fakeFinder) Find(_ context.Context, kind, name string) (*registry.Found, error) {
	f.asked = append(f.asked, kind+":"+name)
	if found, ok := f.found[kind+":"+name]; ok {
		return found, nil
	}
	return nil, registry.ErrUnknownTool
}

func TestResolveLookups(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	tests := []struct {
		in        string
		found     *registry.Found
		repo      string
		name      string
		layer     registry.Layer // match layer
		repoLayer registry.Layer
		prefix    string
		asked     bool
	}{
		// Curated tools and mise backends resolve without a lookup.
		{
			in: "tfprovider:hashicorp/kubernetes", repo: "hashicorp/terraform-provider-kubernetes",
			name: "terraform-provider-kubernetes", layer: registry.LayerRelgap, repoLayer: registry.LayerRelgap,
		},
		{
			in: "npm:@anthropic-ai/claude-code", repo: "anthropics/claude-code", name: "claude-code",
			layer: registry.LayerRelgap, repoLayer: registry.LayerRelgap,
		},
		{in: "pypi:uv", repo: "astral-sh/uv", name: "uv", layer: registry.LayerRelgap, repoLayer: registry.LayerRelgap},
		// mise installs prettier from npm only, so its entry names no repository and npm is asked.
		{
			in: "npm:prettier", found: &registry.Found{Repo: "prettier/prettier", Layer: registry.LayerNPM},
			repo: "prettier/prettier", name: "prettier", layer: registry.LayerNPM, repoLayer: registry.LayerNPM,
			asked: true,
		},
		{
			in:        "npm:@biomejs/biome",
			repo:      "biomejs/biome",
			name:      "biome",
			layer:     registry.LayerMise,
			repoLayer: registry.LayerAqua,
		},
		{
			in:        "pypi:azure-cli",
			repo:      "Azure/azure-cli",
			name:      "azure",
			layer:     registry.LayerMise,
			repoLayer: registry.LayerMise,
		},
		// A lookup whose repository a registry knows takes that tool's entry.
		{
			in:        "npm:@hashicorp/terraform-wrapper",
			found:     &registry.Found{Repo: "hashicorp/terraform", Layer: registry.LayerNPM},
			repo:      "hashicorp/terraform",
			name:      "terraform",
			layer:     registry.LayerRelgap,
			repoLayer: registry.LayerRelgap,
			asked:     true,
		},
		{
			in:    "tfprovider:integrations/github",
			found: &registry.Found{Repo: "integrations/terraform-provider-github", Layer: registry.LayerTerraform},
			repo:  "integrations/terraform-provider-github", name: "terraform-provider-github",
			layer: registry.LayerTerraform, repoLayer: registry.LayerTerraform, asked: true,
		},
		// A chart never takes another tool's entry, even when the repository is a known tool's.
		{
			in: "chart:argo/argo-cd",
			found: &registry.Found{
				Repo: "argoproj/argo-helm", Name: "argo-cd", TagPrefix: "argo-cd-", Layer: registry.LayerArtifactHub,
			},
			repo: "argoproj/argo-helm", name: "argo-cd", layer: registry.LayerArtifactHub,
			repoLayer: registry.LayerArtifactHub, prefix: "argo-cd-", asked: true,
		},
		{
			in:    "chart:hashicorp/terraform",
			found: &registry.Found{Repo: "hashicorp/terraform", Name: "terraform", Layer: registry.LayerArtifactHub},
			repo:  "hashicorp/terraform", name: "terraform", layer: registry.LayerArtifactHub,
			repoLayer: registry.LayerArtifactHub, asked: true,
		},
	}
	for _, tt := range tests {
		f := &fakeFinder{found: map[string]*registry.Found{}}
		if tt.found != nil {
			f.found[tt.in] = tt.found
		}
		res, err := r.Resolve(t.Context(), tt.in, registry.ResolveOptions{Finder: f})
		require.NoError(t, err, tt.in)
		assert.Equal(t, tt.repo, res.Tool.Repo, tt.in)
		assert.Equal(t, tt.name, res.Tool.Name, tt.in)
		assert.Equal(t, tt.layer, res.Match.Layer, tt.in)
		assert.Equal(t, []registry.Layer{tt.repoLayer}, res.Provenance["repo"], tt.in)
		assert.Equal(t, tt.asked, len(f.asked) > 0, tt.in)
		if tt.prefix != "" {
			assert.Equal(t, tt.prefix, res.Tool.Version.TagPrefixes.Newest(), tt.in)
			assert.Equal(t, []registry.Layer{registry.LayerArtifactHub}, res.Provenance["version.tag_prefix"], tt.in)
		}
		assert.Empty(t, res.Confidence, tt.in)
	}
}

func TestResolveGuess(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	f := &fakeFinder{found: map[string]*registry.Found{
		"guess:some-tool": {
			Repo:         "acme/some-tool",
			Name:         "some-tool",
			Layer:        registry.LayerGuess,
			Alternatives: []string{"b/some-tool"},
		},
	}}

	_, err := r.Resolve(t.Context(), "some-tool", registry.ResolveOptions{Finder: f})
	require.ErrorIs(t, err, registry.ErrUnknownTool, "no guess without Guess")
	assert.Empty(t, f.asked)

	res, err := r.Resolve(t.Context(), "some-tool", registry.ResolveOptions{Finder: f, Guess: true})
	require.NoError(t, err)
	assert.Equal(t, "acme/some-tool", res.Tool.Repo)
	assert.Equal(t, "some-tool", res.Tool.Name)
	assert.Equal(t, registry.ConfidenceLow, res.Confidence)
	assert.Equal(t, []string{"b/some-tool"}, res.Alternatives)
	assert.Equal(t, registry.Match{By: "guess", Value: "some-tool", Layer: registry.LayerGuess}, res.Match)

	res, err = r.Resolve(t.Context(), "terraform", registry.ResolveOptions{Finder: f, Guess: true})
	require.NoError(t, err)
	assert.Empty(t, res.Confidence, "a known name is never guessed")
	assert.Equal(t, registry.RelgapSourceEmbedded, res.RelgapSource)

	_, err = r.Resolve(t.Context(), "other-tool", registry.ResolveOptions{Finder: f, Guess: true})
	require.ErrorIs(t, err, registry.ErrUnknownTool)
	assert.Contains(t, err.Error(), "--guess")
}

// TestSpec_tool_resolution_3: tfprovider:hashicorp/aws resolves to
// hashicorp/terraform-provider-aws with CHANGELOG.md as the source, without a lookup.
func TestSpec_tool_resolution_3(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	f := &fakeFinder{}
	res, err := r.Resolve(t.Context(), "tfprovider:hashicorp/aws", registry.ResolveOptions{Finder: f})
	require.NoError(t, err)
	assert.Equal(t, "hashicorp/terraform-provider-aws", res.Tool.Repo)
	require.NotEmpty(t, res.Tool.Sources)
	assert.Equal(t, "changelog-file", res.Tool.Sources[0].Kind)
	assert.Equal(t, "CHANGELOG.md", res.Tool.Sources[0].PathTemplate)
	assert.Empty(t, f.asked)
}

// TestChartShortName shortens chart names read through an oci:// or GitHub Pages URL for tables,
// and reads the short form back as the same chart.
func TestChartShortName(t *testing.T) {
	t.Parallel()
	tests := []struct{ long, short string }{
		{
			"chart:https://aws.github.io/secrets-store-csi-driver-provider-aws/secrets-store-csi-driver-provider-aws",
			"chart:aws/secrets-store-csi-driver-provider-aws/secrets-store-csi-driver-provider-aws",
		},
		{"chart:https://o.github.io/site/charts/x", "chart:o/site/charts/x"},
		{"chart:oci://quay.io/jetstack/charts/cert-manager", "chart:quay.io/jetstack/charts/cert-manager"},
		{"chart:oci://localhost:5000/x", "chart:localhost:5000/x"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.short, registry.ChartShortName(tt.long), tt.long)
		assert.Equal(t, tt.long, registry.ChartURLName(tt.short), tt.short)
		assert.Equal(t, tt.long, registry.ChartURLName(tt.long), "a URL name is already the identity")
		longURL, longChart, ok := registry.SplitChartAt(strings.TrimPrefix(tt.long, "chart:"))
		require.True(t, ok)
		shortURL, shortChart, ok := registry.SplitChartAt(strings.TrimPrefix(tt.short, "chart:"))
		require.True(t, ok)
		assert.Equal(t, []string{longURL, longChart}, []string{shortURL, shortChart}, tt.short)
	}
	for _, same := range []string{
		"chart:https://charts.jetstack.io/cert-manager", // not a Pages site
		"chart:https://o.github.io/x",                   // no site path: the chart would be the site
		"chart:argo/argo-cd",                            // an Artifact Hub repository
		"chart:argo-cd",
		"github:o/r",
	} {
		assert.Equal(t, same, registry.ChartShortName(same), same)
		assert.Equal(t, same, registry.ChartURLName(same), same)
	}
	_, _, ok := registry.SplitChartAt("argo/argo-cd")
	assert.False(t, ok, "two parts without a dot are an Artifact Hub repository and chart")
}
