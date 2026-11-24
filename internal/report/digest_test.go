package report_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

type fakeGitHub struct {
	releases []github.Release
	tags     []string
	files    map[string]string // by ref:path
	fetched  [][]string
	files404 []string
	fileGets []string
}

func (f *fakeGitHub) File(_ context.Context, _, ref, path string) ([]byte, error) {
	f.fileGets = append(f.fileGets, ref+":"+path)
	body, ok := f.files[ref+":"+path]
	if !ok {
		f.files404 = append(f.files404, ref+":"+path)
		return nil, fmt.Errorf("fetch %s: %w", path, github.ErrNotFound)
	}
	return []byte(body), nil
}

func (f *fakeGitHub) Releases(context.Context, string) ([]github.Release, error) {
	return f.releases, nil
}

func (f *fakeGitHub) Tags(context.Context, string) ([]string, error) { return f.tags, nil }

func (f *fakeGitHub) RootFiles(context.Context, string) ([]github.File, error) { return nil, nil }

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
		Name: "tool",
		Repo: "o/r",
		Version: registry.VersionSpec{
			Scheme:      version.SchemeSemver,
			TagPrefixes: registry.Prefixes{"v"},
			Universe:    universe,
		},
		Sources: sources,
	}
}

func TestDigestReleases(t *testing.T) {
	t.Parallel()
	published := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	gh := &fakeGitHub{
		releases: []github.Release{
			{
				Tag:         "v1.2.0",
				URL:         "https://x.test/v1.2.0",
				PublishedAt: published,
				Body:        "## Breaking\n\n- gone ([#1](https://x.test/1))\n\n## Notes\n\n- hi\n",
			},
			{Tag: "v1.1.0", URL: "https://x.test/v1.1.0", PublishedAt: published, Body: "## Fixes\n\n- fixed\n"},
			{Tag: "v1.1.1-rc.1", Prerelease: true},
			{Tag: "v1.0.0"},
			{Tag: "v2.0.0", Draft: true},
		},
		files: map[string]string{
			"v1.2:CHANGELOG.md": "# Changelog\n\n## 1.2.0\n\n### Breaking\n\n- Gone [#1](https://x.test/pull/1)\n\n### Added\n\n- new thing\n",
		},
	}
	got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
		Tool: spec(
			registry.UniverseReleases,
			registry.Source{Kind: "changelog-file", PathTemplate: "CHANGELOG.md", Ref: "v{major}.{minor}"},
			registry.Source{Kind: "gh-release"},
			registry.Source{Kind: "web-doc"},
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
	type row struct {
		Version, Text, Adapter string
		Kind                   classify.Kind
	}
	var rows []row
	for _, e := range got.Entries {
		rows = append(rows, row{e.Version, e.Text, e.Source.Adapter, e.Kind})
	}
	assert.Equal(t, []row{
		{"1.1.0", "fixed", "gh-release", classify.KindFix},
		{"1.2.0", "Gone [#1](https://x.test/pull/1)", "changelog-file", classify.KindBreaking},
		{"1.2.0", "new thing", "changelog-file", classify.KindFeature},
	}, rows, "by version, then source order; the release body's copy of the breaking bullet is dropped")
	assert.Equal(t, "https://github.com/o/r/blob/v1.2/CHANGELOG.md#L7", got.Entries[1].Source.URL)
	assert.Equal(t, []string{"Changelog", "1.2.0", "Breaking"}, got.Entries[1].Source.Heading)
	assert.Equal(t, 1, got.Hidden, `"hi" under Notes is below 0.5`)
	assert.Equal(
		t,
		map[classify.Kind]int{"breaking": 1, "deprecation": 0, "security": 0, "feature": 1, "fix": 1, "other": 0},
		got.Counts,
	)
	assert.Equal(t, [][]string{{"v1.1.0", "v1.2.0"}}, gh.fetched, "web-doc is skipped, gh-release runs once")
	assert.Equal(t, []string{"v1.1:CHANGELOG.md", "v1.2:CHANGELOG.md"}, gh.fileGets, "one request per file")
	assert.Equal(t, []string{"v1.1:CHANGELOG.md"}, gh.files404, "a missing file is skipped")
}

func TestDigestMajorJump(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{
		{Tag: "v3.0.0", URL: "https://x.test/v3.0.0", Body: "## Breaking changes\n\n- y\n"},
		{Tag: "v2.0.0", URL: "https://x.test/v2.0.0", Body: "- x\n"},
		{Tag: "v1.9.0"},
	}}
	got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
		Tool: spec(registry.UniverseReleases), From: "1.9.0", MinConfidence: 0.5,
	})
	require.NoError(t, err)
	require.Len(t, got.Entries, 2)
	want := classify.Entry{
		Version: "2.0.0", Kind: classify.KindBreaking, Confidence: 0.6,
		Text:   "major version jump 1.x → 2.x: review the upgrade guide",
		Source: source.Ref{Adapter: classify.AdapterVersionJump, URL: "https://x.test/v2.0.0"},
	}
	want.ID = report.EntryID("tool", want)
	assert.Equal(t, want, got.Entries[0])
	assert.Equal(t, "y", got.Entries[1].Text, "3.0.0 has a breaking entry, so no jump entry")
	assert.Equal(t, 1, got.Hidden)
}

// TestDigestEntryIDs checks that an entry's ID depends on the tool, version and words only, so
// a filter keeps it, and that a repeated ID gets a suffix.
func TestDigestEntryIDs(t *testing.T) {
	t.Parallel()
	const dropped = "Dropped [#7](https://x.test/7) the flag"
	const jump = "major version jump 1.x → 2.x: review the upgrade guide"
	gh := &fakeGitHub{releases: []github.Release{
		{
			Tag:  "v3.0.0",
			URL:  "https://x.test/v3.0.0",
			Body: "## Breaking changes\n\n- " + dropped + "\n\n## Fixes\n\n- ok\n",
		},
		{Tag: "v2.0.0", URL: "https://x.test/v2.0.0", Body: "## Fixes\n\n- " + jump + "\n"},
		{Tag: "v1.9.0"},
	}}
	digest := func(name string, kinds ...classify.Kind) []classify.Entry {
		tool := spec(registry.UniverseReleases)
		tool.Name = name
		got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
			Tool: tool, From: "1.9.0", Kinds: kinds,
		})
		require.NoError(t, err)
		return got.Entries
	}
	all := digest("tool")
	ids := map[string]string{}
	for _, e := range all {
		assert.Regexp(t, `^e-[0-9a-f]{8}(-\d+)?$`, e.ID)
		ids[e.Version+" "+e.Text+" "+e.Source.Adapter] = e.ID
	}
	jumpID := report.EntryID("tool", classify.Entry{Version: "2.0.0", Text: jump})
	assert.Equal(t, map[string]string{
		"2.0.0 " + jump + " version-jump": jumpID,
		"2.0.0 " + jump + " gh-release":   jumpID + "-2",
		"3.0.0 " + dropped + " gh-release": report.EntryID(
			"tool",
			classify.Entry{Version: "3.0.0", Text: "dropped the FLAG"},
		),
		"3.0.0 ok gh-release": report.EntryID("tool", classify.Entry{Version: "3.0.0", Text: "ok"}),
	}, ids, "links, issue numbers, case and punctuation don't count; the second copy gets -2")

	breaking := digest("tool", classify.KindBreaking)
	require.Len(t, breaking, 2)
	assert.Equal(t, jumpID, breaking[0].ID, "a kind filter keeps the IDs")
	assert.Equal(t, ids["3.0.0 "+dropped+" gh-release"], breaking[1].ID)
	assert.NotEqual(t, breaking[1].ID, digest("other", classify.KindBreaking)[1].ID, "the tool is part of the ID")
}

func TestDigestBadMarker(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}}
	tool := spec(registry.UniverseReleases)
	tool.BreakingMarkers = []string{"("}
	_, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
		Tool: tool, From: "1.0.0",
	})
	require.ErrorContains(t, err, "breaking marker")
}

func TestDigestTagsUniverseWithoutSources(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{tags: []string{"v1.0.0", "v1.0.1", "v1.1.0", "junk"}, releases: []github.Release{
		{Tag: "v1.0.1", Body: "- x\n"},
	}}
	got, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
		Tool: spec(registry.UniverseTags),
		From: "0.9.0",
		To:   "1.0.1",
	})
	require.NoError(t, err)
	require.Len(t, got.Versions, 2)
	assert.Empty(t, got.Versions[0].URL, "a tags universe has no release pages")
	assert.Equal(t, []string{"0.9.0 is older than every known version"}, got.Warnings)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, classify.AdapterVersionJump, got.Entries[0].Source.Adapter, "0.9.0 to 1.0.0 crosses a major")
	assert.Empty(t, got.Entries[0].Source.URL)
	assert.Equal(t, classify.KindOther, got.Entries[1].Kind)
	assert.Equal(t, [][]string{{"v1.0.0", "v1.0.1"}}, gh.fetched, "auto-discovery falls back to gh-release")
}

func TestDigestRangeErrors(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}}
	d := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil)
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

// TestDigestAllHiddenWarns: a range whose entries are all below the minimum confidence warns
// with the compare link, as a range without notes does.
func TestDigestAllHiddenWarns(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0", Body: "See https://docs.example.test/1.1.0 for the release notes.\n"},
	}}
	d := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil)
	got, err := d.Digest(t.Context(), report.DigestRequest{
		Tool: spec(registry.UniverseReleases), From: "1.0.0", MinConfidence: 0.99,
	})
	require.NoError(t, err)
	assert.Empty(t, got.Entries)
	assert.Equal(t, 1, got.Hidden)
	assert.Equal(t, []string{
		"all 1 entries are below the minimum confidence 0.99; lower --min-confidence to see them, " +
			"or compare the versions at https://github.com/o/r/compare/v1.0.0...v1.1.0",
	}, got.Warnings)

	got, err = d.Digest(t.Context(), report.DigestRequest{Tool: spec(registry.UniverseReleases), From: "1.0.0"})
	require.NoError(t, err)
	assert.Len(t, got.Entries, 1)
	assert.Empty(t, got.Warnings, "an entry is shown")
}

// TestDigestNotFoundNamesPrefix: a missing version names the tag prefix and the tags that hold
// the version, since a wrong prefix hides every tag.
func TestDigestNotFoundNamesPrefix(t *testing.T) {
	t.Parallel()
	gh := &fakeGitHub{releases: []github.Release{
		{Tag: "x-helm-chart-1.0.0"}, {Tag: "x-helm-chart-1.1.0"}, {Tag: "v0.1.0"}, {Tag: "x-1.0.0"},
	}}
	tool := spec(registry.UniverseReleases)
	tool.Version.TagPrefixes = registry.Prefixes{"x-"}
	_, err := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil).Digest(t.Context(), report.DigestRequest{
		Tool: tool, From: "1.0.0", To: "1.1.0",
	})
	require.ErrorIs(t, err, version.ErrNotFound)
	assert.EqualError(t, err, `select tool range: version not found: 1.1.0; `+
		`1 of 4 tags of o/r are versions after the tag prefix "x-"; tags that hold 1.1.0: x-helm-chart-1.1.0; `+
		`to read other tags, write a registry entry with version.tag_prefix and try it with `+
		`relgap registry test <file.yaml> <range>`)
}
