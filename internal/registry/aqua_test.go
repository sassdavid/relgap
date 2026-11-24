package registry_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/registry"
)

func TestAquaFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		expr, include, exclude string
	}{
		{expr: `not (Version startsWith "lychee-lib-")`, exclude: `^lychee-lib-`},
		{expr: "not (Version startsWith \"pinger-\")\n", exclude: `^pinger-`},
		{expr: `not (Version matches "-rc\\.\\d+$")`, exclude: `-rc\.\d+$`},
		{expr: `Version matches "^apps_v" || Version matches "^oxlint_v"`, include: `^apps_v|^oxlint_v`},
		{expr: `Version startsWith "cli@" || Version startsWith "v1.2"`, include: `^cli@|^v1\.2`},
		{expr: `Version matches "^oxfmt_v"`, include: `^oxfmt_v`},
		{expr: `Version matches "^a|^b" || Version startsWith "c"`, include: `(?:^a|^b)|^c`},
		{expr: `Version startsWith "atlascli/"`, include: `^atlascli/`},
		{expr: "let words = [\"lsp\", \"nightly\"];\nnot any(words, {Version matches #})\n"},
		{expr: `Version matches "^v" && Version != "v1.0.0"`},
		{expr: `Version matches "^v" || Version != "v1.0.0"`},
		{expr: `not (Version startsWith "a") || Version startsWith "b"`},
		{expr: `Version matches "("`},
		{expr: `Version startsWith ""`},
		{expr: ""},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			include, exclude := registry.AquaFilter(tt.expr)
			assert.Equal(t, tt.include, include)
			assert.Equal(t, tt.exclude, exclude)
		})
	}
}

// TestSpec_tool_resolution_10: an aqua package's version_overrides prefixes and its version_filter
// become the tool's tag rules, unless a higher layer set the tag prefix.
func TestSpec_tool_resolution_10(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	tests := []struct {
		in       string
		prefixes registry.Prefixes
		include  string
		exclude  string
	}{
		{in: "lychee", prefixes: registry.Prefixes{"lychee-v", ""}, exclude: `^lychee-lib-`},
		{in: "oxlint", prefixes: registry.Prefixes{"oxlint_v", "apps_v", ""}, include: `^apps_v|^oxlint_v`},
		{in: "oxfmt", prefixes: registry.Prefixes{"oxfmt_v", "apps_v"}, include: `^oxfmt_v`},
		{in: "gping", prefixes: registry.Prefixes{"gping-", ""}, exclude: `^pinger-`},
		{in: "biome", prefixes: registry.Prefixes{"@biomejs/biome@", "cli/v", ""}},
		{in: "aqua:mongodb/mongodb-atlas-cli/atlascli", prefixes: registry.Prefixes{"atlascli/"}},
		{in: "github:lycheeverse/lychee@lychee-v", prefixes: registry.Prefixes{"lychee-v"}},
		{in: "kubectl", prefixes: registry.Prefixes{"v"}, exclude: `-(alpha|beta|rc)\.`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			res, err := r.Resolve(t.Context(), tt.in, registry.ResolveOptions{})
			require.NoError(t, err)
			v := res.Tool.Version
			assert.Equal(t, tt.prefixes, v.TagPrefixes)
			assert.Equal(t, tt.include, v.Include)
			assert.Equal(t, tt.exclude, v.Exclude)
			_, err = v.Rules()
			require.NoError(t, err)
		})
	}
}

func TestPrefixesEncoding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		json     string
		yaml     string
		prefixes registry.Prefixes
	}{
		{name: "one", json: `{"tag_prefix":"v"}`, yaml: "tag_prefix: v\n", prefixes: registry.Prefixes{"v"}},
		{
			name: "several", json: `{"tag_prefix":["lychee-v",""]}`, yaml: "tag_prefix:\n    - lychee-v\n    - \"\"\n",
			prefixes: registry.Prefixes{"lychee-v", ""},
		},
		{name: "none", json: `{}`, yaml: "{}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var fromJSON, fromYAML registry.VersionSpec
			require.NoError(t, json.Unmarshal([]byte(tt.json), &fromJSON))
			require.NoError(t, yaml.Unmarshal([]byte(tt.yaml), &fromYAML))
			assert.Equal(t, tt.prefixes, fromJSON.TagPrefixes)
			assert.Equal(t, tt.prefixes, fromYAML.TagPrefixes)

			b, err := json.Marshal(struct {
				P registry.Prefixes `json:"tag_prefix,omitempty"`
			}{tt.prefixes})
			require.NoError(t, err)
			assert.JSONEq(t, tt.json, string(b))
			y, err := yaml.Marshal(struct {
				P registry.Prefixes `yaml:"tag_prefix,omitempty"`
			}{tt.prefixes})
			require.NoError(t, err)
			assert.Equal(t, tt.yaml, string(y))
		})
	}

	var v registry.VersionSpec
	require.NoError(t, json.Unmarshal([]byte(`{"tag_prefix":""}`), &v))
	assert.Nil(t, v.TagPrefixes, `"" is no prefix`)
	require.Error(t, json.Unmarshal([]byte(`{"tag_prefix":1}`), &v))
	assert.Equal(t, `"lychee-v", ""`, registry.Prefixes{"lychee-v", ""}.String())
	assert.Empty(t, registry.Prefixes(nil).Newest())
}
