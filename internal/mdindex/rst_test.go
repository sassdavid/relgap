package mdindex_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/version"
)

func TestFromRST(t *testing.T) {
	t.Parallel()
	src := "=========\nCHANGELOG\n=========\n\n2.31.1\n======\n\n* api-change:``s3``: Adds a flag.\n\n" +
		"Fixes\n-----\n\n* bugfix:``cli``: Fixes a crash.\n\n2.31.0\n======\n\n* feature:``eks``: New command.\n" +
		"\nShort\n==\n"
	want := "# CHANGELOG\n\n## 2.31.1\n\n* api-change:``s3``: Adds a flag.\n\n" +
		"### Fixes\n\n* bugfix:``cli``: Fixes a crash.\n\n## 2.31.0\n\n* feature:``eks``: New command.\n" +
		"\nShort\n==\n"
	assert.Equal(t, want, string(mdindex.FromRST([]byte(src))))
}

func TestWholeAndNewest(t *testing.T) {
	t.Parallel()
	src := "# Upgrading 3.5 to 3.6\n\n## 1.2.0\n\n- b\n\n## 1.10.0\n\n- c\n\n## 1.3.0\n\n- d\n"
	x, err := mdindex.Parse([]byte(src), mdindex.Options{Scheme: version.Semver()})
	require.NoError(t, err)
	assert.Equal(t, src, x.Whole())
	newest, ok := x.Newest()
	require.True(t, ok)
	assert.Equal(t, "1.10.0", newest.String())

	empty, err := mdindex.Parse([]byte("no headings\n"), mdindex.Options{Scheme: version.Semver()})
	require.NoError(t, err)
	_, ok = empty.Newest()
	assert.False(t, ok)
	assert.Equal(t, "no headings\n", empty.Whole())
}

func TestDemote(t *testing.T) {
	t.Parallel()
	src := "# Title\n\nText\n\n## Breaking\n\n```sh\n# not a heading\n```\n\n###### Deep\n\nSetext\n------\n"
	want := "## Title\n\nText\n\n### Breaking\n\n```sh\n# not a heading\n```\n\n###### Deep\n\nSetext\n------\n"
	assert.Equal(t, want, string(mdindex.Demote([]byte(src))))
}

func TestSubsection(t *testing.T) {
	t.Parallel()
	src := "## Changes by Kind\n\n- a\n\n## Urgent Upgrade Notes\n\n### (No, really, you MUST read this)\n\n" +
		"- Remove the flag. (#1)\n\n## Changelog since v1.36.0\n\n- b\n"
	body, heading, line, ok := mdindex.Subsection(
		[]byte(src),
		func(h string) bool { return h == "Urgent Upgrade Notes" },
	)
	require.True(t, ok)
	assert.Equal(t, "Urgent Upgrade Notes", heading)
	assert.Equal(t, 6, line)
	assert.Equal(t, "### (No, really, you MUST read this)\n\n- Remove the flag. (#1)\n", body)
	_, _, _, ok = mdindex.Subsection([]byte(src), func(string) bool { return false })
	assert.False(t, ok)
}
