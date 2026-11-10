package classify_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
	registryfiles "github.com/sassdavid/relgap/registry"
)

func newClassifier(t *testing.T, o classify.Options) *classify.Classifier {
	t.Helper()
	c, err := classify.New(o)
	require.NoError(t, err)
	return c
}

func notes(t *testing.T, md string) source.VersionNotes {
	t.Helper()
	v, err := version.Semver().Parse("1.2.0")
	require.NoError(t, err)
	return source.VersionNotes{
		Version:  version.Version{V: v, Tag: "v1.2.0"},
		Markdown: md,
		Source:   source.Ref{Adapter: source.KindGHRelease, URL: "https://example.test/v1.2.0"},
	}
}

type want struct {
	kind    classify.Kind
	conf    float64
	text    string
	heading []string
}

func check(t *testing.T, got []classify.Entry, wants ...want) {
	t.Helper()
	require.Len(t, got, len(wants))
	for i, w := range wants {
		assert.Equal(t, w.kind, got[i].Kind, "entry %d kind", i)
		assert.InDelta(t, w.conf, got[i].Confidence, 1e-9, "entry %d confidence", i)
		assert.Equal(t, w.text, got[i].Text, "entry %d text", i)
		assert.Equal(t, w.heading, got[i].Source.Heading, "entry %d heading", i)
		assert.Equal(t, "1.2.0", got[i].Version)
		assert.Equal(t, "https://example.test/v1.2.0", got[i].Source.URL)
	}
}

func TestPseudoHeadingsLikeTerraform(t *testing.T) {
	t.Parallel()
	md := `## 1.2.0 (May 1, 2026)

UPGRADE NOTES:

* ` + "`a`" + ` changed. Review it.

NEW FEATURES:

* one
* two

BUG FIXES:

* fixed it

NOTES:

* built with Go 1.26
`
	check(
		t,
		newClassifier(t, classify.Options{}).Entries(notes(t, md)),
		want{classify.KindBreaking, 0.95, "`a` changed. Review it.", []string{"1.2.0 (May 1, 2026)", "UPGRADE NOTES:"}},
		want{classify.KindFeature, 0.9, "one", []string{"1.2.0 (May 1, 2026)", "NEW FEATURES:"}},
		want{classify.KindFeature, 0.9, "two", []string{"1.2.0 (May 1, 2026)", "NEW FEATURES:"}},
		want{classify.KindFix, 0.9, "fixed it", []string{"1.2.0 (May 1, 2026)", "BUG FIXES:"}},
		want{
			classify.KindOther,
			classify.ConfidenceOther,
			"built with Go 1.26",
			[]string{"1.2.0 (May 1, 2026)", "NOTES:"},
		},
	)
}

func TestMarkdownHeadingsAndBoldLabels(t *testing.T) {
	t.Parallel()
	md := `# Release

## Breaking Changes

- removed --foo
  use --bar instead

  ` + "```sh\n  tool --bar\n  ```" + `

## Security

**Fixed**

- CVE-2026-1 patched

### Deprecations

- old API

## What's Changed

* feat: x by @someone
`
	check(
		t,
		newClassifier(t, classify.Options{}).Entries(notes(t, md)),
		want{
			classify.KindBreaking, 0.95, "removed --foo\nuse --bar instead\n\n```sh\ntool --bar\n```",
			[]string{"Release", "Breaking Changes"},
		},
		// The bold label is deeper than "Security" but the deepest match wins: "Fixed" is a fix heading.
		want{classify.KindFix, 0.9, "CVE-2026-1 patched", []string{"Release", "Security", "Fixed"}},
		want{classify.KindDeprecation, 0.9, "old API", []string{"Release", "Security", "Deprecations"}},
		want{classify.KindFeature, 0.8, "feat: x by @someone", []string{"Release", "What's Changed"}},
	)
}

func TestFencedCommentsAreNotHeadings(t *testing.T) {
	t.Parallel()
	md := "## Features\n\nNew root config:\n\n```hcl\n# root.hcl\nlocals {}\n```\n\n## Other\n\ntext\n"
	check(t, newClassifier(t, classify.Options{}).Entries(notes(t, md)),
		want{classify.KindFeature, 0.9, "New root config:\n\n```hcl\n# root.hcl\nlocals {}\n```", []string{"Features"}},
		want{classify.KindOther, 0.9, "text", []string{"Other"}},
	)
}

func TestNoHeadings(t *testing.T) {
	t.Parallel()
	md := "Just a paragraph.\n\nAnd another.\n\n---\n\n- an item\n"
	check(t, newClassifier(t, classify.Options{}).Entries(notes(t, md)),
		want{classify.KindOther, classify.ConfidenceOther, "Just a paragraph.\n\nAnd another.", []string{}},
		want{classify.KindOther, classify.ConfidenceOther, "an item", []string{}},
	)
}

func TestHeadingRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		heading string
		kind    classify.Kind
		conf    float64
	}{
		{"BREAKING CHANGES:", classify.KindBreaking, 0.95},
		{"Urgent Upgrade Notes", classify.KindBreaking, 0.95},
		{"Compatibility Notes", classify.KindBreaking, 0.95},
		{"\u26a0 Heads up", classify.KindBreaking, 0.95},
		{"SEMVER-MAJOR Commits", classify.KindBreaking, 0.95},
		{"Deprecated", classify.KindDeprecation, 0.9},
		{"Deprecations", classify.KindDeprecation, 0.9},
		{"Removed", classify.KindBreaking, 0.9},
		{"Removals", classify.KindBreaking, 0.9},
		{"SECURITY ADVISORIES:", classify.KindSecurity, 0.95},
		{"Fix for CVE-2026-1234", classify.KindSecurity, 0.95},
		{"Enhancements", classify.KindFeature, 0.9},
		{"Added", classify.KindFeature, 0.9},
		{"What's new", classify.KindFeature, 0.9},
		{"Bug fixes", classify.KindFix, 0.9},
		{"Bugfixes", classify.KindFix, 0.9},
		{"Fixed", classify.KindFix, 0.9},
		{"Bug or Regression", classify.KindFix, 0.9},
		{"Improvements", classify.KindFeature, 0.9},
		{"Other (Cleanup or Flake)", classify.KindOther, 0.9},
		{"API Change", classify.KindOther, 0.9},
		{"Dependency", classify.KindOther, 0.9},
		{"Notable Changes", classify.KindOther, 0.6},
		{"Changelog", classify.KindOther, classify.ConfidenceOther},
	}
	c := newClassifier(t, classify.Options{})
	for _, tt := range tests {
		t.Run(tt.heading, func(t *testing.T) {
			t.Parallel()
			got := c.Entries(notes(t, "## "+tt.heading+"\n\n- item\n"))
			require.Len(t, got, 1)
			assert.Equal(t, tt.kind, got[0].Kind)
			assert.InDelta(t, tt.conf, got[0].Confidence, 1e-9)
		})
	}
}

// TestRulePriorities covers each row of the classification table: the first matching rule wins.
func TestRulePriorities(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		headings []string
		text     string
		kind     classify.Kind
		conf     float64
	}{
		{
			"registry marker beats a feature heading",
			[]string{"Feature"},
			"ACTION REQUIRED: drop v1alpha2",
			classify.KindBreaking,
			0.95,
		},
		{
			"inline SEMVER-MAJOR",
			nil,
			"\\[[`abc`](u)] - **(SEMVER-MAJOR)** **fs**: remove x",
			classify.KindBreaking,
			0.95,
		},
		{"heading beats commit prefix", []string{"Bug Fixes"}, "feat: x", classify.KindFix, 0.9},
		{"deepest heading first", []string{"Breaking Changes", "Features"}, "x", classify.KindFeature, 0.9},
		{
			"outer heading when the inner names no kind",
			[]string{"Security", "kubectl"},
			"x",
			classify.KindSecurity,
			0.95,
		},
		{"feat!", nil, "feat!: drop node 18", classify.KindBreaking, 0.9},
		{"scoped bang", nil, "fix(api)!: rename field", classify.KindBreaking, 0.9},
		{"footer", nil, "refactor: x\n\nBREAKING CHANGE: config moved", classify.KindBreaking, 0.9},
		{"feat", nil, "feat(cli): add --json", classify.KindFeature, 0.8},
		{"fix", nil, "fix: crash", classify.KindFix, 0.8},
		{"chore(deps)", nil, "chore(deps): bump x from 1 to 2", classify.KindOther, 0.8},
		{"deps", nil, "deps: bump grpc", classify.KindOther, 0.8},
		{"backport prefix", nil, "[dev-v3 backport] fix: x", classify.KindFix, 0.8},
		{
			"unknown scope word is no commit type",
			nil,
			"cloud: `init` finds workspaces",
			classify.KindOther,
			classify.ConfidenceOther,
		},
		{"SEMVER-MINOR", nil, "\\[[`abc`](u)] - **(SEMVER-MINOR)** **fs**: add x", classify.KindFeature, 0.8},
		{"verb removed", nil, "Removed the `/foo` command", classify.KindBreaking, 0.7},
		{"verb with platform", nil, "Windows: Fixed paths with spaces", classify.KindFix, 0.7},
		{"verb breaking change", nil, "Breaking change: `--x` is gone", classify.KindBreaking, 0.7},
		{"verb deprecated", nil, "Deprecated `--y`", classify.KindDeprecation, 0.7},
		{"verb added", nil, "Added `--z`", classify.KindFeature, 0.7},
		{"notable", []string{"Notable Changes"}, "tls: reject bad input", classify.KindOther, 0.6},
		{"keyword removed", nil, "The flag was removed upstream", classify.KindBreaking, 0.4},
		{"keyword no longer", nil, "x no longer starts", classify.KindBreaking, 0.4},
		{"keyword deprecated", nil, "x is now deprecated", classify.KindDeprecation, 0.4},
		{"keyword CVE", nil, "patched CVE-2026-1", classify.KindSecurity, 0.4},
		{"nothing", nil, "bump version to 3.22", classify.KindOther, classify.ConfidenceOther},
	}
	c := newClassifier(t, classify.Options{BreakingMarkers: []string{"ACTION REQUIRED"}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, conf := c.Classify(tt.headings, tt.text)
			assert.Equal(t, tt.kind, kind)
			assert.InDelta(t, tt.conf, conf, 1e-9)
		})
	}
}

func TestInvalidMarker(t *testing.T) {
	t.Parallel()
	_, err := classify.New(classify.Options{BreakingMarkers: []string{"("}})
	require.ErrorContains(t, err, "breaking marker")
}

// TestSpec_classification_2: kubernetes "Dependencies › Removed" lists removed Go modules. With
// the kubectl registry entry those entries are dropped, so they are never breaking.
func TestSpec_classification_2(t *testing.T) {
	t.Parallel()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	entries, err := l.LoadFS(registryfiles.FS(), ".")
	require.NoError(t, err)
	var o classify.Options
	for _, e := range entries {
		if e.Name == "kubectl" {
			o = classify.Options{BreakingMarkers: e.BreakingMarkers, IgnoreSections: e.Sources[0].IgnoreSections}
		}
	}
	require.NotEmpty(t, o.IgnoreSections)
	md := `## Changes by Kind

### Deprecation

- Deprecated ` + "`--x`" + `

### Feature

- Added y

## Dependencies

### Added
_Nothing has changed._

### Removed
- github.com/old/module: v1.0.0
`
	n := notes(t, md)
	n.Source = source.Ref{
		Adapter: source.KindChangelogFile,
		URL:     "https://example.test/v1.2.0",
		Heading: []string{"v1.2.0"},
	}
	got := newClassifier(t, o).Entries(n)
	require.Len(t, got, 2)
	for _, e := range got {
		assert.NotContains(t, e.Source.Heading, "Dependencies")
		assert.NotEqual(t, classify.KindBreaking, e.Kind)
	}
	// Without the registry entry the same section would be breaking.
	plain := newClassifier(t, classify.Options{}).Entries(n)
	assert.Equal(t, classify.KindBreaking, plain[len(plain)-1].Kind)
}

func TestIgnoreSections(t *testing.T) {
	t.Parallel()
	md := "## Changes\n\n- a\n\n## Dependencies\n\n### Removed\n\n- b\n\n### Added\n\n- c\n\n## Downloads for v1.2.0\n\n- d\n\n## New Contributors\n\n- @x made their first contribution\n"
	tests := []struct {
		ignore []string
		want   []string
	}{
		{nil, []string{"a", "b", "c"}}, // downloads and contributors are always dropped
		{[]string{"Dependencies"}, []string{"a"}},
		{[]string{"Dependencies › Removed"}, []string{"a", "c"}},
		{[]string{"dependencies > removed"}, []string{"a", "c"}},
		{[]string{"Removed"}, []string{"a", "c"}},
		{[]string{"Depend"}, []string{"a", "b", "c"}}, // a prefix must end at a word
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.ignore, ","), func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, e := range newClassifier(t, classify.Options{IgnoreSections: tt.ignore}).Entries(notes(t, md)) {
				got = append(got, e.Text)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOutputHygiene(t *testing.T) {
	t.Parallel()
	md := "<a name=\"1.2.0\"></a>\n### Fixes\n\n- see [docs](docs/x.md#y) and [abs](https://a.test/z) and [top](#top)\n- second\n  line\n\n**Full Changelog**: https://github.com/o/r/compare/v1.1.0...v1.2.0\n"
	n := notes(t, md)
	n.Source.URL = "https://github.com/o/r/blob/main/CHANGELOG.md"
	n.Line = 40
	got := newClassifier(t, classify.Options{}).Entries(n)
	require.Len(t, got, 2)
	assert.Equal(t, "see [docs](https://github.com/o/r/blob/main/docs/x.md#y) and [abs](https://a.test/z) and "+
		"[top](https://github.com/o/r/blob/main/CHANGELOG.md#top)", got[0].Text)
	assert.Equal(t, "https://github.com/o/r/blob/main/CHANGELOG.md#L43", got[0].Source.URL)
	assert.Equal(t, "https://github.com/o/r/blob/main/CHANGELOG.md#L44", got[1].Source.URL)
}

// TestLazyCapsLabel: terraform writes "EXPERIMENTS:" on the line after a bullet, which CommonMark
// reads as part of that bullet. The label starts a new section instead.
func TestLazyCapsLabel(t *testing.T) {
	t.Parallel()
	md := "UPGRADE NOTES:\n\n* `a` changed.\nEXPERIMENTS:\n\nExperiments are alpha only.\n\n- x\n"
	check(t, newClassifier(t, classify.Options{}).Entries(notes(t, md)),
		want{classify.KindBreaking, 0.95, "`a` changed.", []string{"UPGRADE NOTES:"}},
		want{classify.KindOther, classify.ConfidenceOther, "Experiments are alpha only.", []string{"EXPERIMENTS:"}},
		want{classify.KindOther, classify.ConfidenceOther, "x", []string{"EXPERIMENTS:"}},
	)
}

func TestJumps(t *testing.T) {
	t.Parallel()
	s := version.Semver()
	vs := func(tags ...string) []version.Version {
		out := make([]version.Version, len(tags))
		for i, tag := range tags {
			v, err := s.Parse(tag)
			require.NoError(t, err)
			out[i] = version.Version{V: v, Tag: "v" + tag}
		}
		return out
	}
	from, err := s.Parse("1.9.0")
	require.NoError(t, err)
	urls := map[string]string{"2.0.0": "https://example.test/v2.0.0"}

	got := classify.Jumps(s, from, vs("1.9.1", "2.0.0", "2.1.0", "3.0.0"), []classify.Entry{
		{Version: "3.0.0", Kind: classify.KindBreaking},
	}, urls)
	require.Len(t, got, 1, "3.0.0 has its own breaking entry")
	assert.Equal(t, classify.Entry{
		Version: "2.0.0", Kind: classify.KindBreaking, Confidence: 0.6,
		Text:   "major version jump 1.x → 2.x: review the upgrade guide",
		Source: source.Ref{Adapter: classify.AdapterVersionJump, URL: "https://example.test/v2.0.0"},
	}, got[0])

	zero, err := s.Parse("0.3.2")
	require.NoError(t, err)
	got = classify.Jumps(s, zero, vs("0.3.3", "0.4.0", "1.0.0"), nil, nil)
	require.Len(t, got, 2)
	assert.Equal(t, "minor version jump 0.3.x → 0.4.x: review the upgrade guide", got[0].Text)
	assert.Equal(t, "major version jump 0.x → 1.x: review the upgrade guide", got[1].Text)
}
