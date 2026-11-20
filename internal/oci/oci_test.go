package oci_test

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/oci"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

func TestParseRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want oci.Ref
	}{
		{"alpine", oci.Ref{Registry: "docker.io", Repository: "library/alpine"}},
		{"bitnami/redis", oci.Ref{Registry: "docker.io", Repository: "bitnami/redis"}},
		{"docker.io/alpine", oci.Ref{Registry: "docker.io", Repository: "library/alpine"}},
		{"index.docker.io/library/node", oci.Ref{Registry: "docker.io", Repository: "library/node"}},
		{"ghcr.io/org/app", oci.Ref{Registry: "ghcr.io", Repository: "org/app"}},
		{"localhost:5000/app", oci.Ref{Registry: "localhost:5000", Repository: "app"}},
		{"localhost/app", oci.Ref{Registry: "localhost", Repository: "app"}},
		{"Quay.io/org/sub/app", oci.Ref{Registry: "quay.io", Repository: "org/sub/app"}},
	}
	for _, tt := range tests {
		got, err := oci.ParseRef(tt.in)
		require.NoError(t, err, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
	for _, bad := range []string{"", "a b", "ghcr.io/", "app@sha256:abc"} {
		_, err := oci.ParseRef(bad)
		require.ErrorIs(t, err, oci.ErrBadRef, bad)
	}
}

func client(t *testing.T, registries []string) *oci.Client {
	t.Helper()
	rt := vcr.ForTest(t, filepath.Join("..", "..", "testdata", "cassettes", "non-github", "images"), vcr.Scrubber{})
	hc := httpx.New(httpx.Options{
		Logger:    slog.New(slog.DiscardHandler),
		Transport: rt,
		Limits:    map[string]httpx.Limit{},
		Default: httpx.Limit{
			Concurrency: 1,
		},
		RetryMax:     1,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: time.Millisecond,
	})
	return oci.New(hc, oci.Options{Registries: registries})
}

func ref(t *testing.T, name string) oci.Ref {
	t.Helper()
	r, err := oci.ParseRef(name)
	require.NoError(t, err)
	return r
}

// TestRegistries lists tags and reads source labels the way each registry answers: Docker Hub
// and ghcr.io after an anonymous token, ghcr.io on two pages, registry.k8s.io without a token, and
// public.ecr.aws with its own token scope.
func TestRegistries(t *testing.T) {
	t.Parallel()
	c := client(t, nil)

	api := ref(t, "example/api")
	tags, err := c.Tags(t.Context(), api)
	require.NoError(t, err)
	assert.Contains(t, tags, "1.6.0")
	src, err := c.Source(t.Context(), api, "1.6.0")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/example-org/api", src, "from the index descriptor's annotations")

	worker := ref(t, "ghcr.io/example-org/worker")
	tags, err = c.Tags(t.Context(), worker)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.9.0", "2.0.0", "2.1.0", "2.2.0", "main"}, tags)
	src, err = c.Source(t.Context(), worker, "2.2.0")
	require.NoError(t, err)
	assert.Empty(t, src, "no annotation and no config label")

	proxy := ref(t, "registry.k8s.io/kube-proxy")
	tags, err = c.Tags(t.Context(), proxy)
	require.NoError(t, err)
	assert.Len(t, tags, 5)
	_, err = c.Source(t.Context(), proxy, "v1.34.2")
	require.NoError(t, err)

	agent := ref(t, "public.ecr.aws/example/agent")
	tags, err = c.Tags(t.Context(), agent)
	require.NoError(t, err)
	assert.Contains(t, tags, "2.5.0-al2023")
	src, err = c.Source(t.Context(), agent, "2.5.0-al2023")
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/example/agent", src, "the label-schema label in the config")
}

func TestCheck(t *testing.T) {
	t.Parallel()
	c := oci.New(http.DefaultClient, oci.Options{})
	_, err := c.Tags(t.Context(), ref(t, "registry.internal.test/team/app"))
	require.ErrorIs(t, err, oci.ErrNotAllowed, "no request goes to a registry that isn't listed")
	require.NoError(t, oci.New(http.DefaultClient, oci.Options{Registries: []string{"registry.internal.test"}}).
		Check(ref(t, "registry.internal.test/team/app")))
	assert.Equal(t, "https://registry-1.docker.io/v2/library/alpine/tags/list", c.URL(ref(t, "alpine")))
}
