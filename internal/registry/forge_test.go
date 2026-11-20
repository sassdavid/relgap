package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

func TestResolveForge(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{
		Tools:       map[string]string{"net-module": "acme-ws/tf-network", "hk": "jdx/hk"},
		Hosts:       map[string]string{"net-module": "bitbucket.org"},
		GitLabHosts: []string{"gitlab.acme.internal"},
	})
	tests := []struct {
		in, name, repo, host, prefix string
	}{
		{"bitbucket:ws/repo", "repo", "ws/repo", "bitbucket.org", ""},
		{"bitbucket:ws/repo@release-", "repo", "ws/repo", "bitbucket.org", "release-"},
		{"gitlab:group/sub/repo", "repo", "group/sub/repo", "gitlab.com", ""},
		{"gitlab:my.group/repo", "repo", "my.group/repo", "gitlab.com", ""},
		{"gitlab:gitlab.acme.internal/infra/tf-vpc", "tf-vpc", "infra/tf-vpc", "gitlab.acme.internal", ""},
		{"bitbucket:acme-ws/tf-network", "net-module", "acme-ws/tf-network", "bitbucket.org", ""},
		{"net-module", "net-module", "acme-ws/tf-network", "bitbucket.org", ""},
	}
	for _, tt := range tests {
		res, err := r.Resolve(t.Context(), tt.in, registry.ResolveOptions{})
		require.NoError(t, err, tt.in)
		assert.Equal(t, tt.name, res.Tool.Name, tt.in)
		assert.Equal(t, tt.repo, res.Tool.Repo, tt.in)
		assert.Equal(t, tt.host, res.Tool.Host, tt.in)
		assert.Equal(t, tt.prefix, res.Tool.Version.TagPrefix, tt.in)
		assert.Equal(t, registry.UniverseTags, res.Tool.Version.Universe, "%s: Bitbucket has no releases", tt.in)
	}

	_, err := r.Resolve(t.Context(), "github:acme-ws/tf-network", registry.ResolveOptions{})
	require.NoError(t, err, "a config tool on Bitbucket never answers for the GitHub repository of that name")
	for _, bad := range []string{"bitbucket:ws", "bitbucket:ws/sub/repo", "gitlab:repo", "gitlab:a//b"} {
		_, err = r.Resolve(t.Context(), bad, registry.ResolveOptions{})
		require.ErrorIs(t, err, registry.ErrUnsupported, bad)
	}

	idents := map[string]string{
		"bitbucket:ws/repo":                     "bitbucket:ws/repo",
		"gitlab:gitlab.acme.internal/infra/vpc": "gitlab:gitlab.acme.internal/infra/vpc",
		"gitlab:group/repo@v":                   "gitlab:group/repo@v",
		"bitbucket:acme-ws/tf-network":          "net-module",
	}
	for in, want := range idents {
		got, ok := r.Identify(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	_, ok := r.Identify("gitlab:single")
	assert.False(t, ok)
}

func TestSplitImage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, image, prefix, suffix string
		ok                        bool
	}{
		{"nginx", "nginx", "", "", true},
		{"nginx:*-alpine", "nginx", "", "-alpine", true},
		{"ghcr.io/org/app:v*", "ghcr.io/org/app", "v", "", true},
		{"localhost:5000/app", "localhost:5000/app", "", "", true},
		{"localhost:5000/app:*-slim", "localhost:5000/app", "", "-slim", true},
		{"nginx:*a*", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tt := range tests {
		image, prefix, suffix, ok := registry.SplitImage(tt.in)
		assert.Equal(t, tt.ok, ok, tt.in)
		assert.Equal(t, []string{tt.image, tt.prefix, tt.suffix}, []string{image, prefix, suffix}, tt.in)
		if ok {
			assert.Equal(t, "image:"+tt.in, registry.ImageName(image, prefix, suffix), "round trip")
		}
	}
}

func TestResolveImage(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	versions := []version.Ref{{Tag: "1.27.0-alpine"}, {Tag: "1.27.0"}}
	f := &fakeFinder{found: map[string]*registry.Found{
		"image:nginx:*-alpine": {
			Name: "image:nginx:*-alpine", Repo: "hashicorp/terraform", TagSuffix: "-alpine", Layer: registry.LayerImage,
			Versions: versions, VersionsURL: "https://registry-1.docker.io/v2/library/nginx/tags/list",
			Image: "docker.io/library/nginx",
		},
		"image:busybox": {
			Name:     "image:busybox",
			Layer:    registry.LayerImage,
			Versions: []version.Ref{},
			Image:    "docker.io/library/busybox",
		},
	}}
	res, err := r.Resolve(t.Context(), "image:nginx:*-alpine", registry.ResolveOptions{Finder: f})
	require.NoError(t, err)
	tool := res.Tool
	assert.Equal(t, "image:nginx:*-alpine", tool.Name)
	assert.Equal(t, registry.UniverseImage, tool.Version.Universe)
	assert.Equal(t, "-alpine", tool.Version.TagSuffix)
	assert.Equal(t, "docker.io/library/nginx", tool.Image)
	assert.Equal(t, versions, tool.Versions)
	require.NotNil(t, tool.Notes, "the source repository becomes the notes tool")
	assert.Equal(t, "terraform", tool.Notes.Name, "with its curated entry")
	assert.Empty(t, tool.Aliases, "the image takes nothing from the repository's entry")

	res, err = r.Resolve(t.Context(), "image:busybox", registry.ResolveOptions{Finder: f})
	require.NoError(t, err, "an image without a source label still resolves")
	assert.Empty(t, res.Tool.Repo)
	assert.Nil(t, res.Tool.Notes)

	_, err = r.Resolve(t.Context(), "image:nginx", registry.ResolveOptions{})
	require.ErrorIs(t, err, registry.ErrUnsupported, "an image needs the registry lookup")
}
