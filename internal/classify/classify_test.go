package classify_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

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
		classify.New().Entries(notes(t, md)),
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
		classify.New().Entries(notes(t, md)),
		want{
			classify.KindBreaking, 0.95, "removed --foo\nuse --bar instead\n\n```sh\ntool --bar\n```",
			[]string{"Release", "Breaking Changes"},
		},
		// The bold label is deeper than "Security" but the deepest match wins: "Fixed" is a fix heading.
		want{classify.KindFix, 0.9, "CVE-2026-1 patched", []string{"Release", "Security", "Fixed"}},
		want{classify.KindDeprecation, 0.9, "old API", []string{"Release", "Security", "Deprecations"}},
		want{
			classify.KindOther,
			classify.ConfidenceOther,
			"feat: x by @someone",
			[]string{"Release", "What's Changed"},
		},
	)
}

func TestFencedCommentsAreNotHeadings(t *testing.T) {
	t.Parallel()
	md := "## Features\n\nNew root config:\n\n```hcl\n# root.hcl\nlocals {}\n```\n\n## Other\n\ntext\n"
	check(t, classify.New().Entries(notes(t, md)),
		want{classify.KindFeature, 0.9, "New root config:\n\n```hcl\n# root.hcl\nlocals {}\n```", []string{"Features"}},
		want{classify.KindOther, classify.ConfidenceOther, "text", []string{"Other"}},
	)
}

func TestNoHeadings(t *testing.T) {
	t.Parallel()
	md := "Just a paragraph.\n\nAnd another.\n\n---\n\n- an item\n"
	check(t, classify.New().Entries(notes(t, md)),
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
		{"Changelog", classify.KindOther, classify.ConfidenceOther},
	}
	c := classify.New()
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
