package source_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
)

const testdata = "../../testdata"

// notesView is the golden form of a VersionNotes.
type notesView struct {
	Version  string     `json:"version"`
	Tag      string     `json:"tag"`
	Source   source.Ref `json:"source"`
	Markdown string     `json:"markdown"`
}

func versions(t *testing.T, tags ...string) []version.Version {
	t.Helper()
	out := make([]version.Version, len(tags))
	for i, tag := range tags {
		v, err := version.Semver().Parse(strings.TrimPrefix(tag, "v"))
		require.NoError(t, err)
		out[i] = version.Version{V: v, Tag: tag}
	}
	return out
}

// TestSpec_source_adapters_1: the gh-release adapter has cassette-backed golden VersionNotes for
// 2 tools. A tag without a release (helm v3.99.0) is left out.
func TestSpec_source_adapters_1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tool string
		repo string
		tags []string
	}{
		{"terraform", "hashicorp/terraform", []string{
			"v1.15.9", "v1.16.0", "v1.16.1", "v1.16.2", "v1.16.3", "v1.16.4",
			"v1.16.5", "v1.16.6", "v1.16.7", "v1.17.0", "v1.17.1",
		}},
		{"helm", "helm/helm", []string{"v3.22.0", "v3.22.1", "v3.99.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			t.Parallel()
			rt := vcr.ForTest(
				t,
				filepath.Join(testdata, "cassettes", "source", "gh-release-"+tt.tool),
				ghtest.Scrubber(),
			)
			hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
			gh, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: slog.New(slog.DiscardHandler)})
			require.NoError(t, err)

			a := source.NewGHRelease(gh)
			assert.Equal(t, source.KindGHRelease, a.Kind())
			notes, err := a.Fetch(t.Context(), registry.ToolSpec{Name: tt.tool, Repo: tt.repo}, versions(t, tt.tags...))
			require.NoError(t, err)

			view := make([]notesView, len(notes))
			for i, n := range notes {
				view[i] = notesView{
					Version:  n.Version.V.String(),
					Tag:      n.Version.Tag,
					Source:   n.Source,
					Markdown: n.Markdown,
				}
			}
			got, err := json.MarshalIndent(view, "", "  ")
			require.NoError(t, err)
			testutil.Golden(
				t,
				filepath.Join(testdata, "golden", "source", "gh-release", tt.tool+".json"),
				append(got, '\n'),
			)
		})
	}
}

type fakeFetcher map[string]github.Release

func (f fakeFetcher) ReleaseBodies(_ context.Context, _ string, tags []string) (map[string]github.Release, error) {
	out := map[string]github.Release{}
	for _, tag := range tags {
		if r, ok := f[tag]; ok {
			out[tag] = r
		}
	}
	return maps.Clone(out), nil
}

func TestGHReleaseSkipsEmptyBodiesAndNormalizesNewlines(t *testing.T) {
	t.Parallel()
	gh := fakeFetcher{
		"v1.0.0": {Tag: "v1.0.0", URL: "https://example.test/r/1", Body: "## Fixes\r\n\r\n- one\r\n"},
		"v1.1.0": {Tag: "v1.1.0", Body: "  \r\n"},
	}
	notes, err := source.NewGHRelease(gh).Fetch(t.Context(), registry.ToolSpec{Repo: "o/r"},
		versions(t, "v1.0.0", "v1.1.0", "v1.2.0"))
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "## Fixes\n\n- one\n", notes[0].Markdown)
	assert.Equal(t, source.Ref{Adapter: source.KindGHRelease, URL: "https://example.test/r/1"}, notes[0].Source)
}
