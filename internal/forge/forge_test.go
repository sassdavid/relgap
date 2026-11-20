package forge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sassdavid/relgap/internal/forge"
)

func TestLinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host, file, tag, compare, line string
	}{
		{
			"", "https://github.com/o/r/blob/HEAD/CHANGELOG.md", "https://github.com/o/r/releases/tag/v2",
			"https://github.com/o/r/compare/v1...v2", "https://github.com/o/r/blob/HEAD/CHANGELOG.md#L7",
		},
		{
			"bitbucket.org",
			"https://bitbucket.org/o/r/src/HEAD/CHANGELOG.md",
			"https://bitbucket.org/o/r/src/v2",
			"https://bitbucket.org/o/r/branches/compare/v2%0Dv1",
			"https://bitbucket.org/o/r/src/HEAD/CHANGELOG.md#lines-7",
		},
		{
			"gitlab.acme.internal", "https://gitlab.acme.internal/o/r/-/blob/HEAD/CHANGELOG.md",
			"https://gitlab.acme.internal/o/r/-/tags/v2", "https://gitlab.acme.internal/o/r/-/compare/v1...v2",
			"https://gitlab.acme.internal/o/r/-/blob/HEAD/CHANGELOG.md#L7",
		},
	}
	for _, tt := range tests {
		file := forge.FileURL(tt.host, "o/r", "", "/CHANGELOG.md")
		assert.Equal(t, tt.file, file)
		assert.Equal(t, tt.tag, forge.TagURL(tt.host, "o/r", "v2"))
		assert.Equal(t, tt.compare, forge.CompareURL(tt.host, "o/r", "v1", "v2"))
		assert.Equal(t, tt.line, forge.LineURL(file, 7))
	}
	assert.Equal(t, forge.KindGitHub, forge.KindOf("GitHub.com"))
	assert.Equal(t, forge.KindBitbucket, forge.KindOf("bitbucket.org"))
	assert.Equal(t, forge.KindGitLab, forge.KindOf("gitlab.com"))
}
