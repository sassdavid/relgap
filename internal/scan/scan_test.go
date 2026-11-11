package scan_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/scan"
)

// noGit makes the scanner walk directories, as on a machine without git.
type noGit struct{}

func (noGit) Run(context.Context, execx.Cmd) (*execx.Result, error) { return nil, execx.ErrNotFound }

// names knows a fixed set of tool names and maps everything else to nothing.
type names map[string]string

func (n names) Identify(input string) (string, bool) {
	tool, ok := n[input]
	return tool, ok
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
}

func run(t *testing.T, opts scan.Options, files map[string]string) *scan.Result {
	t.Helper()
	root := t.TempDir()
	write(t, root, files)
	s := scan.New(
		slog.New(slog.DiscardHandler),
		noGit{},
		names{"terraform": "terraform", "TERRAFORM_VERSION": "terraform"},
		opts,
	)
	res, err := s.Scan(t.Context(), []string{root})
	require.NoError(t, err)
	return res
}

func TestClassify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file, value string
		match       scan.Match
		version     string
	}{
		{".tool-versions", "1.15.8", scan.MatchExact, "1.15.8"},
		{".tool-versions", "v1.15.8", scan.MatchExact, "1.15.8"},
		{".tool-versions", "1.15.8-rc.1", scan.MatchExact, "1.15.8-rc.1"},
		{".tool-versions", "1.15", scan.MatchPrefix, "1.15"},
		{".tool-versions", "26", scan.MatchPrefix, "26"},
		{".tool-versions", "prefix:1.2", scan.MatchPrefix, "1.2"},
		{".tool-versions", "1.x", scan.MatchConstraint, "1.x"},
		{".tool-versions", "latest", scan.MatchChannel, "latest"},
		{".tool-versions", "ref:main", scan.MatchChannel, "ref:main"},
		{".tool-versions", "corretto-17", scan.MatchChannel, "corretto-17"},
		{"root.hcl", "~> 1.15", scan.MatchConstraint, "~> 1.15"},
		{"root.hcl", ">= 1.5, < 2", scan.MatchConstraint, ">= 1.5, < 2"},
		{"root.hcl", "1.15", scan.MatchExact, "1.15"}, // a bare Terraform version allows only itself
	}
	for _, tt := range tests {
		t.Run(tt.file+" "+tt.value, func(t *testing.T) {
			t.Parallel()
			body := "terraform " + tt.value + "\n"
			if tt.file == "root.hcl" {
				body = "terraform_version_constraint = \"" + tt.value + "\"\n"
			}
			res := run(t, scan.Options{}, map[string]string{tt.file: body})
			require.Len(t, res.Pins, 1)
			p := res.Pins[0]
			assert.Equal(t, tt.match, p.Match)
			assert.Equal(t, tt.match == scan.MatchExact, p.Exact)
			assert.Equal(t, tt.version, p.Version)
			assert.Equal(t, "terraform", p.Tool)
		})
	}
}

func TestExclude(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		".tool-versions":         "terraform 1.0.0\n",
		"a/.tool-versions":       "terraform 1.1.0\n",
		"a/b/c/.tool-versions":   "terraform 1.2.0\n",
		"skip/me/.tool-versions": "terraform 1.3.0\n",
	}
	tests := []struct {
		exclude []string
		want    []string
	}{
		{nil, []string{".tool-versions", "a/.tool-versions", "a/b/c/.tool-versions", "skip/me/.tool-versions"}},
		{[]string{"**/c/**"}, []string{".tool-versions", "a/.tool-versions", "skip/me/.tool-versions"}},
		{[]string{"skip/**", "a/*/c/*"}, []string{".tool-versions", "a/.tool-versions"}},
		{[]string{"**/.tool-versions"}, nil},
		{[]string{".tool-versions"}, []string{"a/.tool-versions", "a/b/c/.tool-versions", "skip/me/.tool-versions"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.exclude, ","), func(t *testing.T) {
			t.Parallel()
			res := run(t, scan.Options{Exclude: tt.exclude}, files)
			var got []string
			for _, p := range res.Pins {
				got = append(got, p.Location.File)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRepos(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"loose/.tool-versions":                "terraform 0.1.0\n",
		"one/.git/HEAD":                       "",
		"one/.tool-versions":                  "terraform 1.0.0\n",
		"one/nested/.git/HEAD":                "",
		"one/nested/.tool-versions":           "terraform 1.1.0\n",
		"one/sub/.git":                        "gitdir: ../.git/modules/sub\n",
		"one/sub/.tool-versions":              "terraform 1.2.0\n",
		"one/node_modules/x/.tool-versions":   "terraform 9.0.0\n",
		"group/two/.git/HEAD":                 "",
		"group/two/.terraform/.tool-versions": "terraform 9.0.0\n",
		"group/two/deep/dir/.tool-versions":   "terraform 2.0.0\n",
		"group/node_modules/three/.git/HEAD":  "",
	}
	tests := []struct {
		name       string
		submodules bool
		repos      []string
		versions   []string
	}{
		{"default", false, []string{"group/two", "one"}, []string{"2.0.0", "1.0.0"}},
		{"submodules", true, []string{"group/two", "one", "one/sub"}, []string{"2.0.0", "1.0.0", "1.2.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			write(t, root, files)
			s := scan.New(slog.New(slog.DiscardHandler), noGit{}, names{}, scan.Options{Submodules: tt.submodules})
			res, err := s.Scan(t.Context(), []string{root})
			require.NoError(t, err)
			var repos, versions []string
			for _, r := range res.Repos {
				rel, relErr := filepath.Rel(root, r)
				require.NoError(t, relErr)
				repos = append(repos, filepath.ToSlash(rel))
			}
			for _, p := range res.Pins {
				versions = append(versions, p.Value)
			}
			assert.Equal(t, tt.repos, repos)
			assert.Equal(t, tt.versions, versions)
		})
	}
}

func TestSkippedFiles(t *testing.T) {
	t.Parallel()
	big := "terraform 1.0.0\n" + strings.Repeat("#\n", 1<<20+1)
	res := run(t, scan.Options{}, map[string]string{
		"big/.tool-versions":    big,
		"binary/.tool-versions": "terraform 1.0.0\n\x00",
		"ok/.tool-versions":     "terraform 1.1.0\n",
	})
	require.Len(t, res.Pins, 1)
	assert.Equal(t, "ok/.tool-versions", res.Pins[0].Location.File)
}

func TestScanRootErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, map[string]string{"file": ""})
	s := scan.New(slog.New(slog.DiscardHandler), noGit{}, names{}, scan.Options{})
	_, err := s.Scan(t.Context(), []string{filepath.Join(root, "missing")})
	require.ErrorContains(t, err, "scan root")
	_, err = s.Scan(t.Context(), []string{filepath.Join(root, "file")})
	require.ErrorContains(t, err, "not a directory")
}

func TestRenovateReport(t *testing.T) {
	t.Parallel()
	pins, err := scan.RenovateReport([]byte(`{"packageFiles":{"regex":[{"packageFile":"a.mk","deps":[
		{"depName":"x","currentValue":"1.0.0","datasource":"npm"}]}]},"unknown":true}`))
	require.NoError(t, err)
	require.Len(t, pins, 1)
	assert.Equal(t, scan.Location{Repo: ".", File: "a.mk", Detector: scan.DetectorRenovate}, pins[0].Location)

	_, err = scan.RenovateReport([]byte(`[`))
	require.ErrorContains(t, err, "decode renovate report")
}

func TestDockerfileEscapeDirective(t *testing.T) {
	t.Parallel()
	res := run(t, scan.Options{}, map[string]string{
		"Dockerfile": "# escape=`\nFROM base:1.0.0\nARG TERRAFORM_VERSION=1.15.8 `\n    OTHER=1\n",
	})
	require.Len(t, res.Pins, 2)
	assert.Equal(t, "image:base", res.Pins[0].Tool)
	assert.Equal(t, "terraform", res.Pins[1].Tool)
	assert.Equal(t, 3, res.Pins[1].Location.Line)
}

func TestIgnored(t *testing.T) {
	t.Parallel()
	pin := func(tool, repo, file string) *scan.Pin {
		return &scan.Pin{Tool: tool, Location: scan.Location{Repo: repo, File: file}}
	}
	rules := []scan.Ignore{{Tool: "java"}, {Tool: "terraform", Path: "**/legacy/**"}}
	tests := []struct {
		pin  *scan.Pin
		want bool
	}{
		{pin("java", "infra/app", "mise.toml"), true},
		{pin("Java", "infra/app", "mise.toml"), true},
		{pin("terraform", "infra/legacy", "versions.tf"), true},
		{pin("terraform", ".", "legacy/old/versions.tf"), true},
		{pin("terraform", "infra/app", "versions.tf"), false},
		{pin("node", "infra/legacy", "mise.toml"), false},
	}
	for _, tt := range tests {
		assert.Equal(
			t,
			tt.want,
			scan.Ignored(rules, tt.pin),
			"%s %s/%s",
			tt.pin.Tool,
			tt.pin.Location.Repo,
			tt.pin.Location.File,
		)
	}
	assert.False(t, scan.Ignored(nil, pin("java", ".", "mise.toml")))
}
