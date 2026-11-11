package version_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/version"
)

const fixtures = "../../testdata/versions"

// loadReleases reads a GitHub REST releases listing.
func loadReleases(t *testing.T, name string) []version.Ref {
	t.Helper()
	var rs []struct {
		TagName     string    `json:"tag_name"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
	}
	readJSON(t, name, &rs)
	refs := make([]version.Ref, len(rs))
	for i, r := range rs {
		refs[i] = version.Ref{Tag: r.TagName, Draft: r.Draft, Prerelease: r.Prerelease, PublishedAt: r.PublishedAt}
	}
	return refs
}

// loadTags reads a GitHub REST tags listing.
func loadTags(t *testing.T, name string) []version.Ref {
	t.Helper()
	var ts []struct {
		Name string `json:"name"`
	}
	readJSON(t, name, &ts)
	refs := make([]version.Ref, len(ts))
	for i, tg := range ts {
		refs[i] = version.Ref{Tag: tg.Name}
	}
	return refs
}

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtures, name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, v))
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func tags(vs []version.Version) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Tag
	}
	return out
}

func universe(t *testing.T, refs []version.Ref, rules version.Rules) *version.Universe {
	t.Helper()
	return version.NewUniverse(t.Context(), discard(), refs, rules)
}

// TestSpec_versions_and_ranges_3: the helm fixture (interleaved v3/v4 publish dates) gives the
// correct ordered range for 3.19.0..3.22.1 and for --line major.
func TestSpec_versions_and_ranges_3(t *testing.T) {
	t.Parallel()
	u := universe(t, loadReleases(t, "helm-releases.json"), version.Rules{Scheme: version.Semver(), TagPrefix: "v"})
	want := []string{
		"v3.19.1", "v3.19.2", "v3.19.3", "v3.19.4", "v3.19.5",
		"v3.20.0", "v3.20.1", "v3.20.2",
		"v3.21.0", "v3.21.1", "v3.21.2", "v3.21.3", "v3.21.4",
		"v3.22.0", "v3.22.1",
	}

	r, err := u.Select(version.RangeOptions{From: "3.19.0", To: "3.22.1"})
	require.NoError(t, err)
	assert.Equal(t, want, tags(r.Versions))
	assert.Empty(t, r.Warnings)

	r, err = u.Select(version.RangeOptions{From: "3.19.0", Line: version.LineMajor})
	require.NoError(t, err)
	assert.Equal(t, want, tags(r.Versions), "latest v3 is v3.22.1 even though v4.3.1 was published after it")
	assert.Equal(t, "v3.22.1", r.To.Tag)

	r, err = u.Select(version.RangeOptions{From: "3.19.0"})
	require.NoError(t, err)
	assert.Equal(t, "v4.3.1", r.To.Tag, "without a line the range runs to the newest version")
	assert.NotContains(t, tags(r.Versions), "v4.3.0-rc.1")

	r, err = u.Select(version.RangeOptions{From: "3.21.4", To: "3.22.1", IncludePrereleases: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"v3.22.0-rc.1", "v3.22.0", "v3.22.1"}, tags(r.Versions))

	r, err = u.Select(version.RangeOptions{From: "3.21.4", To: "v3.22.0-rc.1"})
	require.NoError(t, err)
	assert.True(t, r.Prereleases, "a prerelease end turns prereleases on")
	assert.Equal(t, []string{"v3.22.0-rc.1"}, tags(r.Versions))

	_, err = u.Select(version.RangeOptions{From: "3.19.0", To: "4.0.0", Line: version.LineMajor})
	require.ErrorIs(t, err, version.ErrLineMismatch)
}

// TestSpec_versions_and_ranges_4: the awscli fixture (no releases, tags only) yields the correct
// range from tags.
func TestSpec_versions_and_ranges_4(t *testing.T) {
	t.Parallel()
	u := universe(t, loadTags(t, "awscli-tags.json"), version.Rules{Scheme: version.Semver()})
	r, err := u.Select(version.RangeOptions{From: "2.31.0", To: "2.31.10"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2.31.1", "2.31.2", "2.31.3", "2.31.4", "2.31.5",
		"2.31.6", "2.31.7", "2.31.8", "2.31.9", "2.31.10",
	}, tags(r.Versions))

	r, err = u.Select(version.RangeOptions{From: "1.42.70", Line: version.LineMajor})
	require.NoError(t, err)
	assert.Equal(t, "1.42.76", r.To.Tag, "v1 tags in the same repo form their own major line")
}

// TestSpec_versions_and_ranges_5: atlascli/v1.30.0 tags are selected and bare v1.2.3 strays are
// ignored, using the registry prefix.
func TestSpec_versions_and_ranges_5(t *testing.T) {
	t.Parallel()
	u := universe(t, loadTags(t, "atlascli-tags.json"), version.Rules{Scheme: version.Semver(), TagPrefix: "atlascli/"})
	for _, v := range u.Versions() {
		assert.Regexp(t, `^atlascli/v`, v.Tag)
	}
	r, err := u.Select(version.RangeOptions{From: "1.29.0", To: "atlascli/v1.32.0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"atlascli/v1.30.0", "atlascli/v1.31.0", "atlascli/v1.32.0"}, tags(r.Versions))

	all := tags(u.Versions())
	assert.NotContains(t, all, "v1.20.0", "bare tags are not atlascli releases")
	assert.NotContains(t, all, "mongocli/v1.30.0")
	assert.Contains(t, all, "atlascli/v1.30.0")
}

func TestNewUniverse(t *testing.T) {
	t.Parallel()
	refs := []version.Ref{
		{Tag: "v3.7.12"},
		{Tag: "3.7.12"},
		{Tag: "v3.8.0", Draft: true},
		{Tag: "v3.9.0-beta.1"},
		{Tag: "v3.9.0", Prerelease: true},
		{Tag: "nightly"},
		{Tag: "v3.7.11-internal"},
	}
	u := universe(t, refs, version.Rules{Scheme: version.Semver(), Exclude: regexp.MustCompile(`-internal$`)})
	assert.Equal(
		t,
		[]string{"v3.7.12", "v3.9.0-beta.1", "v3.9.0"},
		tags(u.Versions()),
		"dedup keeps the first, drafts and strays go",
	)
	assert.True(t, u.Versions()[2].Prerelease, "the forge's prerelease flag is kept")
}

func TestSelect(t *testing.T) {
	t.Parallel()
	refs := []version.Ref{{Tag: "1.0.0"}, {Tag: "1.1.0"}, {Tag: "1.3.0"}, {Tag: "2.0.0-rc.1"}}
	u := universe(t, refs, version.Rules{Scheme: version.Semver()})

	r, err := u.Select(version.RangeOptions{From: "1.2.0", To: "1.3.0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"1.3.0"}, tags(r.Versions))
	require.NotNil(t, r.Base)
	assert.Equal(t, "1.1.0", r.Base.Tag)
	assert.Equal(t, []string{"1.2.0 is not a known version; using the nearest lower version 1.1.0"}, r.Warnings)

	r, err = u.Select(version.RangeOptions{From: "0.1.0", To: "1.1.0"})
	require.NoError(t, err)
	assert.Nil(t, r.Base)
	assert.Equal(t, []string{"1.0.0", "1.1.0"}, tags(r.Versions))
	assert.Equal(t, []string{"0.1.0 is older than every known version"}, r.Warnings)

	_, err = u.Select(version.RangeOptions{From: "1.3.0", To: "1.0.0"})
	require.ErrorIs(t, err, version.ErrDowngrade)
	require.ErrorContains(t, err, "swap the arguments")

	_, err = u.Select(version.RangeOptions{From: "1.0.0", To: "1.2.0"})
	require.ErrorIs(t, err, version.ErrNotFound)

	r, err = u.Select(version.RangeOptions{From: "1.3.0"})
	require.NoError(t, err)
	assert.Empty(t, r.Versions, "already at the latest stable version")

	_, err = u.Select(version.RangeOptions{From: "x"})
	require.ErrorIs(t, err, version.ErrParse)

	_, err = u.Select(version.RangeOptions{From: "2.0.0", Line: version.LineMajor})
	require.ErrorIs(t, err, version.ErrNotFound)
}

func TestResolveConstraint(t *testing.T) {
	t.Parallel()
	s := version.Semver()
	refs := []version.Ref{{Tag: "1.14.0"}, {Tag: "1.15.0-rc.1"}, {Tag: "1.15.2"}, {Tag: "1.16.0"}, {Tag: "2.0.0"}}
	u := universe(t, refs, version.Rules{Scheme: s})
	c, err := version.ParseConstraint(s, "~> 1.15")
	require.NoError(t, err)

	v, pick, err := u.ResolveConstraint(c, nil)
	require.NoError(t, err)
	assert.Equal(t, "1.15.2", v.String())
	assert.Equal(t, version.PickLowest, pick)

	installed, err := s.Parse("1.16.0")
	require.NoError(t, err)
	other, err := s.Parse("2.0.0")
	require.NoError(t, err)
	v, pick, err = u.ResolveConstraint(c, []version.V{other, installed})
	require.NoError(t, err)
	assert.Equal(t, "1.16.0", v.String())
	assert.Equal(t, version.PickKnown, pick)

	c, err = version.ParseConstraint(s, ">= 3")
	require.NoError(t, err)
	_, _, err = u.ResolveConstraint(c, nil)
	require.ErrorIs(t, err, version.ErrNoMatch)
}

func TestResolvePrefix(t *testing.T) {
	t.Parallel()
	refs := []version.Ref{
		{Tag: "go1.25.9"},
		{Tag: "go1.25.10"},
		{Tag: "go1.26rc1"},
		{Tag: "go1.26.0"},
		{Tag: "go1.26.2"},
		{Tag: "go1.27rc2"},
	}
	u := universe(t, refs, version.Rules{Scheme: version.Loose(), TagPrefix: "go"})
	tests := []struct{ prefix, want, err string }{
		{prefix: "1.25", want: "1.25.10"},
		{prefix: "1", want: "1.26.2"},
		{prefix: "1.26.0", want: "1.26.0"},
		{prefix: "go1.26", want: "1.26.2"},
		{prefix: "1.27", err: "no stable version starts with 1.27"},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			v, err := u.ResolvePrefix(tt.prefix)
			if tt.err != "" {
				require.ErrorIs(t, err, version.ErrNoMatch)
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, v.String())
		})
	}
}

func TestLatest(t *testing.T) {
	t.Parallel()
	s := version.Semver()
	refs := []version.Ref{{Tag: "3.21.0"}, {Tag: "3.22.1"}, {Tag: "4.3.1"}, {Tag: "4.4.0-rc.1"}}
	u := universe(t, refs, version.Rules{Scheme: s})
	v3, err := s.Parse("3.21.0")
	require.NoError(t, err)

	got, ok := u.Latest(v3, version.LineMajor)
	require.True(t, ok)
	assert.Equal(t, "3.22.1", got.V.String(), "stays on the major line")
	got, ok = u.Latest(v3, version.LineNone)
	require.True(t, ok)
	assert.Equal(t, "4.3.1", got.V.String(), "prereleases are never latest")
	got, ok = u.Latest(version.V{}, version.LineMajor)
	require.True(t, ok)
	assert.Equal(t, "4.3.1", got.V.String(), "a zero version means any line")
}
