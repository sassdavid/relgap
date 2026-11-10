package mdindex_test

import (
	"context"
	"errors"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/version"
)

type sectionView struct {
	version string
	heading []string
	line    int
	body    string
}

func parse(t *testing.T, src string, heading string) []sectionView {
	t.Helper()
	o := mdindex.Options{Scheme: version.Semver()}
	if heading != "" {
		re, err := mdindex.HeadingPattern(heading)
		require.NoError(t, err)
		o.Heading = re
	}
	x, err := mdindex.Parse([]byte(src), o)
	require.NoError(t, err)
	var out []sectionView
	for _, s := range x.Sections() {
		body, line := x.Markdown(s)
		out = append(out, sectionView{s.Version.String(), s.Heading, line, body})
	}
	return out
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		src     string
		heading string
		want    []sectionView
	}{
		{
			name: "level with two version headings wins",
			src:  "# Changelog\n\n## [1.1.0](https://x.test/compare/v1.0.0...v1.1.0) (2026-01-02)\n\n### Fixes\n\n- b\n\n## 1.0.0\n\n- a\n",
			want: []sectionView{
				{
					"1.1.0",
					[]string{"Changelog", "[1.1.0](https://x.test/compare/v1.0.0...v1.1.0) (2026-01-02)"},
					5,
					"### Fixes\n\n- b\n",
				},
				{"1.0.0", []string{"Changelog", "1.0.0"}, 11, "- a\n"},
			},
		},
		{
			name: "deeper version headings stay inside the section",
			src:  "# v1.37.1\n\n## Changelog since v1.37.0\n\n- x\n\n# v1.37.0\n\n- y\n",
			want: []sectionView{
				{"1.37.1", []string{"v1.37.1"}, 3, "## Changelog since v1.37.0\n\n- x\n"},
				{"1.37.0", []string{"v1.37.0"}, 9, "- y\n"},
			},
		},
		{
			name: "fenced and indented hashes are not headings",
			src:  "## 2.0.0\n\n```hcl\n# 1.9.0\n```\n\n    # 1.8.0\n\n## 1.0.0\n\n- a\n",
			want: []sectionView{
				{"2.0.0", []string{"2.0.0"}, 3, "```hcl\n# 1.9.0\n```\n\n    # 1.8.0\n"},
				{"1.0.0", []string{"1.0.0"}, 11, "- a\n"},
			},
		},
		{
			name: "setext headings, CRLF and node anchors",
			src:  "<a id=\"2.0.0\"></a>\r\n2026-01-02, Version 2.0.0 (Current)\r\n===\r\n\r\n- b\r\n\r\n<a id=\"1.0.0\"></a>\r\n\r\n2026-01-01, Version 1.0.0\r\n===\r\n- a\r\n",
			want: []sectionView{
				{"2.0.0", []string{"2026-01-02, Version 2.0.0 (Current)"}, 5, "- b\n"},
				{"1.0.0", []string{"2026-01-01, Version 1.0.0"}, 11, "- a\n"},
			},
		},
		{
			name: "dates and URLs are not versions",
			src:  "## 2026-01-02\n\n## see https://x.test/v1.2.3\n\n## 1.2.3 (2026-01-01)\n\n- a\n\n## 1.2.2\n\n[1.2.3]: https://x.test/compare/v1.2.2...v1.2.3\n",
			want: []sectionView{
				{"1.2.3", []string{"1.2.3 (2026-01-01)"}, 7, "- a\n"},
				{"1.2.2", []string{"1.2.2"}, 0, ""},
			},
		},
		{
			name:    "heading template",
			src:     "## 1.16.8 (Unreleased)\n\n## 1.16.7 (October 22, 2026)\n\nBUG FIXES:\n\n* x\n\n## Previous Releases\n\n- [v1.15](u)\n",
			heading: `^{version} \(`,
			want: []sectionView{
				{"1.16.8", []string{"1.16.8 (Unreleased)"}, 0, ""},
				{"1.16.7", []string{"1.16.7 (October 22, 2026)"}, 5, "BUG FIXES:\n\n* x\n"},
			},
		},
		{
			name:    "heading template rejects other headings with versions",
			src:     "# v1.37.0\n\n## Downloads for v1.37.0\n\n- d\n",
			heading: "^v{version}$",
			want:    []sectionView{{"1.37.0", []string{"v1.37.0"}, 3, "## Downloads for v1.37.0\n\n- d\n"}},
		},
		{name: "no versions", src: "# Readme\n\ntext\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parse(t, tt.src, tt.heading)
			for i := range got {
				if got[i].body == "" {
					got[i].line = 0
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	x, err := mdindex.Parse([]byte("## v1.2.0\n\n- b\n\n## 1.1.0\n\n- a\n"), mdindex.Options{Scheme: version.Semver()})
	require.NoError(t, err)
	v, err := version.Semver().Parse("1.2.0")
	require.NoError(t, err)
	s, ok := x.Lookup(v)
	require.True(t, ok)
	assert.Equal(t, 1, s.HeadingLine)
	missing, err := version.Semver().Parse("1.3.0")
	require.NoError(t, err)
	_, ok = x.Lookup(missing)
	assert.False(t, ok)
}

func TestErrors(t *testing.T) {
	t.Parallel()
	_, err := mdindex.Parse(nil, mdindex.Options{})
	require.ErrorIs(t, err, mdindex.ErrNoScheme)
	_, err = mdindex.HeadingPattern("^v1$")
	require.ErrorContains(t, err, "{version}")
	_, err = mdindex.HeadingPattern("({version}")
	require.ErrorContains(t, err, "compile heading template")
}

func TestCacheFetchesOncePerFile(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	errMissing := errors.New("missing")
	c := mdindex.NewCache(func(_ context.Context, k mdindex.Key) ([]byte, error) {
		calls.Add(1)
		if k.Path == "missing.md" {
			return nil, errMissing
		}
		return []byte("## 1.0.0\n\n- a\n"), nil
	})
	o := mdindex.Options{Scheme: version.Semver()}
	k := mdindex.Key{Repo: "o/r", Ref: "main", Path: "CHANGELOG.md"}
	results := make(chan *mdindex.Index, 8)
	for range 8 {
		go func() {
			x, err := c.Get(t.Context(), k, o)
			assert.NoError(t, err)
			results <- x
		}()
	}
	first := <-results
	for range 7 {
		assert.Same(t, first, <-results)
	}
	other, err := c.Get(
		t.Context(),
		k,
		mdindex.Options{Scheme: version.Semver(), Heading: regexp.MustCompile(`^(?P<version>1\.0\.0)$`)},
	)
	require.NoError(t, err)
	assert.NotSame(t, first, other, "another heading pattern gets its own index")
	for range 2 {
		_, err = c.Get(t.Context(), mdindex.Key{Repo: "o/r", Path: "missing.md"}, o)
		require.ErrorIs(t, err, errMissing)
	}
	assert.Equal(t, int32(2), calls.Load(), "one fetch per file, failures included")
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("# Changelog\n\n## 1.1.0\n\n```\n## 1.0.9\n```\n\n## 1.0.0\n\n- a\n"))
	f.Add([]byte("v1.0.0\n===\n\n> ## 0.9.0\n\n- ## 0.8.0\n\n<a id=\"x\"></a>\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		x, err := mdindex.Parse(src, mdindex.Options{Scheme: version.Semver()})
		if err != nil {
			t.Fatal(err)
		}
		prevEnd := 0
		for _, s := range x.Sections() {
			if s.Start < prevEnd || s.Start > s.End || s.Line < s.HeadingLine {
				t.Fatalf("bad section %+v after offset %d", s, prevEnd)
			}
			prevEnd = s.End
			x.Markdown(s)
		}
	})
}
