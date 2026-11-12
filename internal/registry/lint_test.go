package registry_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/registry"
	curated "github.com/sassdavid/relgap/registry"
)

func lint(t *testing.T, path string) []string {
	t.Helper()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	_, problems, err := l.Lint(path, b)
	require.NoError(t, err)
	out := make([]string, len(problems))
	for i, p := range problems {
		out[i] = p.String()
	}
	return out
}

// TestSpec_tool_resolution_4: registry lint rejects unknown keys and bad regexes with file:line
// errors.
func TestSpec_tool_resolution_4(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(testdata, "registry")
	tests := []struct {
		file string
		want []string
	}{
		{"unknown-key.yaml", []string{
			"%s:5:3: version.tag_prefixx: unknown key",
			"%s:6:1: sourcez: unknown key",
		}},
		{"bad-regex.yaml", []string{
			"%s:5:12: version.extract: bad regular expression: missing closing ): `jdk-(\\d+`",
			"%s:6:12: version.exclude: bad regular expression: missing closing ): `-(alpha|beta`",
			"%s:10:14: sources.0.heading: bad regular expression: missing closing ]: `[$`",
		}},
		{"missing-field.yaml", []string{
			"%s:7:5: sources.0: missing property 'url_template'",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, tt.file)
			want := make([]string, len(tt.want))
			for i, w := range tt.want {
				want[i] = fmt.Sprintf(w, path)
			}
			assert.Equal(t, want, lint(t, path))
		})
	}
}

func TestLintSemantics(t *testing.T) {
	t.Parallel()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	tests := []struct {
		name, in string
		want     []string
	}{
		{
			"name mismatch",
			"name: other\nrepo: a/b\n",
			[]string{`x.yaml:1:7: name: name "other" must match the file name "x"`},
		},
		{
			"regex without extract",
			"name: x\nrepo: a/b\nversion:\n  scheme: regex\n",
			[]string{"x.yaml:4:11: version.scheme: scheme regex needs version.extract"},
		},
		{
			"extract without regex",
			"name: x\nrepo: a/b\nversion:\n  extract: \"(\\\\d+)\"\n",
			[]string{"x.yaml:4:3: version.extract: extract needs scheme regex"},
		},
		{"valid", "name: x\nrepo: a/b\nversion:\n  scheme: regex\n  extract: \"jdk-(\\\\d+)\"\n", nil},
		{
			"host outside the allow-list",
			"name: x\nrepo: a/b\nsources:\n  - kind: web-doc\n    url_template: https://raw.githubusercontent.com/a/b/{version}\n",
			[]string{"x.yaml:5:19: sources.0.url_template: host not allowed: raw.githubusercontent.com: " +
				"relgap fetches pages only from api.adoptium.net, docs.python.org, go.dev"},
		},
		{
			"unknown placeholder",
			"name: x\nrepo: a/b\nsources:\n  - kind: changelog-file\n    path_template: docs/{patch}.md\n",
			[]string{"x.yaml:5:20: sources.0.path_template: unknown placeholder {patch}: want {version}, " +
				"{major}, {minor}, {tag}, {prev_minor} or {prev_major}"},
		},
		{
			"prev_minor outside upgrade-guide",
			"name: x\nrepo: a/b\nsources:\n  - kind: changelog-file\n    path_template: \"{prev_minor}.md\"\n",
			[]string{
				"x.yaml:5:20: sources.0.path_template: placeholder {prev_minor} works only in upgrade-guide sources",
			},
		},
		{
			"bad selector",
			"name: x\nrepo: a/b\nsources:\n  - kind: web-doc\n    url_template: https://go.dev/doc\n    strip: [\"div > p\"]\n",
			[]string{`x.yaml:6:13: sources.0.strip.0: invalid selector "div > p": want tag, #id and .class ` +
				"parts without spaces, such as div.body"},
		},
		{
			"structured-json needs a mapping",
			"name: x\nrepo: a/b\nsources:\n  - kind: structured-json\n    path_template: c/{version}.json\n",
			[]string{"x.yaml:4:5: sources.0: missing property 'json'"},
		},
	}
	for _, tt := range tests {
		_, problems, lintErr := l.Lint("x.yaml", []byte(tt.in))
		require.NoError(t, lintErr)
		var got []string
		for _, p := range problems {
			got = append(got, p.String())
		}
		assert.Equal(t, tt.want, got, tt.name)
	}
}

func TestLoadFSCollectsProblems(t *testing.T) {
	t.Parallel()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	fsys := fstest.MapFS{
		"a.yaml": {Data: []byte("name: a\nrepo: x/a\nbogus: 1\n")},
		"b.yaml": {Data: []byte("name: b\nrepo: x/b\n")},
		"c.yaml": {Data: []byte("name: c\n")},
	}
	_, err = l.LoadFS(fsys, ".")
	var le *registry.LintError
	require.ErrorAs(t, err, &le)
	require.ErrorIs(t, err, registry.ErrInvalid)
	assert.Equal(t, []string{
		"registry/a.yaml:3:1: bogus: unknown key",
		"registry/c.yaml:1:1: missing property 'repo'",
	}, le.ProblemLines())
}

// TestEmbeddedEntries checks every curated entry and that the Go types carry every field the
// files use: decoding and encoding again loses nothing.
func TestEmbeddedEntries(t *testing.T) {
	t.Parallel()
	l, err := registry.NewLinter()
	require.NoError(t, err)
	entries, err := l.LoadFS(curated.FS(), ".")
	require.NoError(t, err)
	assert.Len(t, entries, 20)

	for _, e := range entries {
		raw, readErr := os.ReadFile(filepath.Join("..", "..", "registry", e.Name+".yaml"))
		require.NoError(t, readErr)
		var want, got map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &want))
		b, marshalErr := yaml.Marshal(e)
		require.NoError(t, marshalErr)
		require.NoError(t, yaml.Unmarshal(b, &got))
		assert.Equal(t, want, dropEmpty(got), e.Name)
		_, rulesErr := e.Version.Rules()
		require.NoError(t, rulesErr, e.Name)
	}
}

func dropEmpty(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		switch x := v.(type) {
		case nil:
			continue
		case string:
			if x == "" {
				continue
			}
		case []any:
			if len(x) == 0 {
				continue
			}
			for i, item := range x {
				if sub, ok := item.(map[string]any); ok {
					x[i] = dropEmpty(sub)
				}
			}
		case map[string]any:
			x = dropEmpty(x)
			if len(x) == 0 {
				continue
			}
			v = x
		}
		out[k] = v
	}
	return out
}
