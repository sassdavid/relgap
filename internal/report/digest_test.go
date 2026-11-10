package report_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/version"
)

type fakeGitHub struct {
	releases []github.Release
	tags     []string
	fetched  [][]string
}

func (f *fakeGitHub) Releases(context.Context, string) ([]github.Release, error) {
	return f.releases, nil
}

func (f *fakeGitHub) Tags(context.Context, string) ([]string, error) { return f.tags, nil }

func (f *fakeGitHub) ReleaseBodies(_ context.Context, _ string, tags []string) (map[string]github.Release, error) {
	f.fetched = append(f.fetched, tags)
	out := map[string]github.Release{}
	for _, r := range f.releases {
		for _, t := range tags {
			if r.Tag == t {
				out[t] = r
			}
		}
	}
	return out, nil
}

func spec(universe string, sources ...registry.Source) registry.ToolSpec {
	return registry.ToolSpec{
		Name:    "tool",
		Repo:    "o/r",
		Version: registry.VersionSpec{Scheme: version.SchemeSemver, TagPrefix: "v", Universe: universe},
		Sources: sources,
	}
}

func TestDigestReleases(t *testing.T) {
	t.Parallel()
	published := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	gh := &fakeGitHub{releases: []github.Release{
		{
			Tag:         "v1.2.0",
			URL:         "https://x.test/v1.2.0",
			PublishedAt: published,
			Body:        "## Breaking\n\n- gone\n\n## Notes\n\n- hi\n",
		},
		{Tag: "v1.1.0", URL: "https://x.test/v1.1.0", PublishedAt: published, Body: "## Fixes\n\n- fixed\n"},
		{Tag: "v1.1.1-rc.1", Prerelease: true},
		{Tag: "v1.0.0"},
		{Tag: "v2.0.0", Draft: true},
	}}
	got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh).Digest(t.Context(), report.DigestRequest{
		Tool: spec(
			registry.UniverseReleases,
			registry.Source{Kind: "changelog-file"},
			registry.Source{Kind: "gh-release"},
		),
		From:          "1.0.0",
		MinConfidence: 0.5,
	})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", got.From)
	assert.Equal(t, "1.2.0", got.To, "drafts and prereleases never end a range")
	require.Len(t, got.Versions, 2)
	assert.Equal(
		t,
		report.Version{Version: "1.1.0", Tag: "v1.1.0", PublishedAt: &published, URL: "https://x.test/v1.1.0"},
		got.Versions[0],
	)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, classify.KindFix, got.Entries[0].Kind)
	assert.Equal(t, "1.1.0", got.Entries[0].Version)
	assert.Equal(t, classify.KindBreaking, got.Entries[1].Kind)
	assert.Equal(t, 1, got.Hidden, `"hi" under Notes is below 0.5`)
	assert.Equal(
		t,
		map[classify.Kind]int{"breaking": 1, "deprecation": 0, "security": 0, "feature": 0, "fix": 1, "other": 0},
		got.Counts,
	)
	assert.Equal(t, [][]string{{"v1.1.0", "v1.2.0"}}, gh.fetched, "changelog-file is skipped, gh-release runs once")
}

func TestDigestTagsUniverseWithoutSources(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{tags: []string{"v1.0.0", "v1.0.1", "v1.1.0", "junk"}, releases: []github.Release{
		{Tag: "v1.0.1", Body: "- x\n"},
	}}
	got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh).Digest(t.Context(), report.DigestRequest{
		Tool: spec(registry.UniverseTags),
		From: "0.9.0",
		To:   "1.0.1",
	})
	require.NoError(t, err)
	require.Len(t, got.Versions, 2)
	assert.Empty(t, got.Versions[0].URL, "a tags universe has no release pages")
	assert.Equal(t, []string{"0.9.0 is older than every known version"}, got.Warnings)
	require.Len(t, got.Entries, 1)
	assert.Equal(t, classify.KindOther, got.Entries[0].Kind)
	assert.Equal(t, [][]string{{"v1.0.0", "v1.0.1"}}, gh.fetched, "auto-discovery falls back to gh-release")
}

func TestDigestRangeErrors(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}}
	d := report.NewDigester(slog.New(slog.DiscardHandler), gh)
	_, err := d.Digest(
		t.Context(),
		report.DigestRequest{Tool: spec(registry.UniverseReleases), From: "1.1.0", To: "1.0.0"},
	)
	require.ErrorIs(t, err, version.ErrDowngrade)
	_, err = d.Digest(
		t.Context(),
		report.DigestRequest{Tool: spec(registry.UniverseReleases), From: "1.0.0", To: "9.0.0"},
	)
	require.ErrorIs(t, err, version.ErrNotFound)
}
