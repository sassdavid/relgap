package purl_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/purl"
)

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		p    purl.PURL
		want string
	}{
		{purl.PURL{Type: "golang", Name: "stdlib", Version: "v1.22.0"}, "pkg:golang/stdlib@v1.22.0"},
		{
			purl.PURL{Type: "golang", Namespace: "github.com/hashicorp", Name: "terraform", Version: "v1.5.0"},
			"pkg:golang/github.com/hashicorp/terraform@v1.5.0",
		},
		{purl.PURL{Type: "npm", Namespace: "@babel", Name: "core", Version: "7.0.0"}, "pkg:npm/%40babel/core@7.0.0"},
		{
			purl.PURL{
				Type:      "githubactions",
				Namespace: "github",
				Name:      "codeql-action",
				Version:   "v3",
				Subpath:   "init",
			},
			"pkg:githubactions/github/codeql-action@v3#init",
		},
		{
			purl.PURL{
				Type: "generic", Name: "glab", Version: "v1.50.0",
				Qualifiers: map[string]string{"vcs_url": "git+https://gitlab.com/gitlab-org/cli"},
			},
			"pkg:generic/glab@v1.50.0?vcs_url=git%2Bhttps:%2F%2Fgitlab.com%2Fgitlab-org%2Fcli",
		},
		{purl.PURL{Type: "golang", Name: "x", Version: "v1.0.0+incompatible"}, "pkg:golang/x@v1.0.0%2Bincompatible"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.p.String())
			back, err := purl.Parse(tt.want)
			require.NoError(t, err)
			assert.Equal(t, tt.want, back.String())
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "golang/stdlib", "pkg:", "pkg:golang", "pkg:golang/", "pkg:npm/%zz"} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			_, err := purl.Parse(s)
			assert.Error(t, err)
		})
	}
}

func TestCovers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, statement, pin string
		want                 bool
	}{
		{"same", "pkg:golang/stdlib@v1.22.0", "pkg:golang/stdlib@v1.22.0", true},
		{"leading v", "pkg:golang/stdlib@1.22.0", "pkg:golang/stdlib@v1.22.0", true},
		{"other version", "pkg:golang/stdlib@v1.22.1", "pkg:golang/stdlib@v1.22.0", false},
		{"no version", "pkg:golang/stdlib", "pkg:golang/stdlib@v1.22.0", true},
		{
			"escaped slashes", "pkg:golang/github.com%2Fhashicorp%2Fterraform@v1.5.0",
			"pkg:golang/github.com/hashicorp/terraform@v1.5.0", true,
		},
		{"case", "pkg:GitHub/JQLang/JQ@jq-1.7", "pkg:github/jqlang/jq@jq-1.7", true},
		{"other type", "pkg:github/jqlang/jq@jq-1.7", "pkg:githubactions/jqlang/jq@jq-1.7", false},
		{"other name", "pkg:npm/lodash@4.17.20", "pkg:npm/lodash-es@4.17.20", false},
		{
			"statement subpath",
			"pkg:githubactions/github/codeql-action#init",
			"pkg:githubactions/github/codeql-action@v3",
			false,
		},
		{
			"pin subpath",
			"pkg:githubactions/github/codeql-action",
			"pkg:githubactions/github/codeql-action@v3#init",
			true,
		},
		{"statement qualifier", "pkg:golang/stdlib@v1.22.0?goarch=amd64", "pkg:golang/stdlib@v1.22.0", false},
		{
			"pin qualifier", "pkg:generic/glab@v1.50.0",
			"pkg:generic/glab@v1.50.0?vcs_url=git%2Bhttps:%2F%2Fgitlab.com%2Fgitlab-org%2Fcli", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := purl.Parse(tt.statement)
			require.NoError(t, err)
			p, err := purl.Parse(tt.pin)
			require.NoError(t, err)
			assert.Equal(t, tt.want, s.Covers(p))
		})
	}
}
