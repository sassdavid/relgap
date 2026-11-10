package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// env returns an isolated environment rooted at dir.
func env(dir string, extra ...string) []string {
	return append([]string{
		"HOME=" + filepath.Join(dir, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "xdg"),
	}, extra...)
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := config.Load(config.Options{Environ: env(dir), Cwd: dir})
	require.NoError(t, err)
	assert.Equal(t, config.Default(), res.Config)
	assert.Equal(t, config.SourceDefault, res.Source("github.concurrency").Kind)
	assert.Contains(t, res.Keys(), "scan.detectors.ci_actions")
	assert.Empty(t, res.UnknownEnv)
}

func TestLoad_Precedence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "xdg", "relgap", "config.yaml"), `
github: {concurrency: 2, host: ghe.example.com}
cache: {dir: /user/cache}
policy: {min_confidence: 0.1, max_behind: "minor:1"}
tools: {a: {repo: o/a}}
`)
	writeFile(t, filepath.Join(dir, ".relgap.yaml"), `
github: {concurrency: 3}
policy: {min_confidence: 0.2}
tools: {b: {repo: o/b}}
`)
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Float64("min-confidence", 0.5, "")
	fs.Int("concurrency", 8, "") // not changed: must not override lower layers
	require.NoError(t, fs.Parse([]string{"--min-confidence=0.9"}))

	res, err := config.Load(config.Options{
		Environ: env(dir, "RELGAP_GITHUB_CONCURRENCY=4", "RELGAP_POLICY_MIN_CONFIDENCE=0.3"),
		Cwd:     dir,
		Flags:   fs,
		FlagKeys: map[string]string{
			"min-confidence": "policy.min_confidence",
			"concurrency":    "github.concurrency",
		},
	})
	require.NoError(t, err)

	c := res.Config
	assert.Equal(t, "ghe.example.com", c.GitHub.Host, "user file")
	assert.Equal(t, "/user/cache", c.Cache.Dir, "user file")
	assert.Equal(t, "minor:1", c.Policy.MaxBehind, "user file")
	assert.Equal(t, 4, c.GitHub.Concurrency, "env beats project")
	assert.InDelta(t, 0.9, c.Policy.MinConfidence, 1e-9, "flag beats env")
	assert.Equal(t, map[string]config.ToolOverride{"a": {Repo: "o/a"}, "b": {Repo: "o/b"}}, c.Tools,
		"maps merge across files")

	userFile := filepath.Join(dir, "xdg", "relgap", "config.yaml")
	projectFile := filepath.Join(dir, ".relgap.yaml")
	assert.Equal(t, config.Source{Kind: config.SourceUser, Name: userFile}, res.Source("github.host"))
	assert.Equal(t, config.Source{Kind: config.SourceUser, Name: userFile}, res.Source("tools.a.repo"))
	assert.Equal(t, config.Source{Kind: config.SourceProject, Name: projectFile}, res.Source("tools.b.repo"))
	assert.Equal(t, config.Source{Kind: config.SourceEnv, Name: "RELGAP_GITHUB_CONCURRENCY"},
		res.Source("github.concurrency"))
	assert.Equal(t, config.Source{Kind: config.SourceFlag, Name: "min-confidence"}, res.Source("policy.min_confidence"))
	assert.Equal(t, "flag --min-confidence", res.Source("policy.min_confidence").String())
}

func TestLoad_EnvCoercion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := config.Load(config.Options{
		Environ: env(dir,
			"RELGAP_SCAN_EXCLUDE= a/** , b/** ",
			"RELGAP_SCAN_DETECTORS_GENERIC=1",
			"RELGAP_SCAN__DETECTORS__CI_ACTIONS=true",
			"RELGAP_LLM_MAX_INPUT_TOKENS=", // empty means unset for numbers
			"RELGAP_TYPO=1",
			"RELGAP_CONFIG=",
		),
		Cwd: dir,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a/**", "b/**"}, res.Config.Scan.Exclude)
	assert.True(t, res.Config.Scan.Detectors.Generic)
	assert.True(t, res.Config.Scan.Detectors.CIActions)
	assert.Equal(t, config.Default().LLM.MaxInputTokens, res.Config.LLM.MaxInputTokens)
	assert.Equal(t, []string{"RELGAP_TYPO"}, res.UnknownEnv)
}

func TestLoad_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		file    string
		environ []string
		want    string
	}{
		{
			name: "unknown key",
			file: "github:\n  hots: x\n",
			want: ".relgap.yaml:2:3: github.hots: unknown key",
		},
		{
			name: "wrong type",
			file: "scan:\n  roots: /src\n",
			want: ".relgap.yaml:2:10: scan.roots: wrong type: got string, want array",
		},
		{
			name: "top level not a mapping",
			file: "- a\n- b\n",
			want: ".relgap.yaml:1:1: wrong type: got array, want object",
		},
		{
			name: "non-string keys",
			file: "1: a\n",
			want: ".relgap.yaml:1:1: 1: unknown key",
		},
		{
			name:    "env bool",
			environ: []string{"RELGAP_SCAN_DETECTORS_GENERIC=yes please"},
			want:    `env RELGAP_SCAN_DETECTORS_GENERIC: scan.detectors.generic: wrong type: "yes please" is not a boolean`,
		},
		{
			name:    "env enum",
			environ: []string{"RELGAP_POLICY_FAIL_ON=eol,never"},
			want:    "env RELGAP_POLICY_FAIL_ON: policy.fail_on.1: value must be one of",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.file != "" {
				writeFile(t, filepath.Join(dir, ".relgap.yaml"), tt.file)
			}
			_, err := config.Load(config.Options{Environ: env(dir, tt.environ...), Cwd: dir})
			require.Error(t, err)
			require.ErrorIs(t, err, config.ErrInvalid)
			var ve *config.ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestLocate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	deep := filepath.Join(repo, "a", "b")
	require.NoError(t, os.MkdirAll(deep, 0o750))
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o750))
	// Above the git root: must not be found from inside the repo.
	writeFile(t, filepath.Join(dir, ".relgap.yaml"), "{}\n")

	files, err := config.Locate(config.Options{Environ: env(dir), Cwd: deep})
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, filepath.Join(dir, "xdg", "relgap", "config.yaml"), files[0].Path)
	assert.False(t, files[0].Found)
	assert.Equal(t, config.File{Kind: config.SourceProject}, files[1])

	writeFile(t, filepath.Join(repo, "a", ".relgap.yaml"), "{}\n")
	files, err = config.Locate(config.Options{Environ: env(dir), Cwd: deep})
	require.NoError(t, err)
	assert.Equal(
		t,
		config.File{Kind: config.SourceProject, Path: filepath.Join(repo, "a", ".relgap.yaml"), Found: true},
		files[1],
	)

	missing := filepath.Join(dir, "nope.yaml")
	files, err = config.Locate(config.Options{Environ: env(dir, "RELGAP_CONFIG="+missing), Cwd: deep})
	require.NoError(t, err)
	assert.Equal(t, config.File{Kind: config.SourceProject, Path: missing, Explicit: true}, files[1])

	_, err = config.Load(config.Options{Environ: env(dir), Cwd: deep, ExplicitFile: missing})
	require.ErrorIs(t, err, config.ErrInvalid)
}

func TestUserConfigPath(t *testing.T) {
	t.Parallel()
	lookup := func(m map[string]string) config.LookupFunc {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	tests := []struct {
		name string
		env  map[string]string
		goos string
		want string
	}{
		{"xdg wins", map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/h"}, "darwin", "/x/relgap/config.yaml"},
		{"macOS uses ~/.config", map[string]string{"HOME": "/h"}, "darwin", "/h/.config/relgap/config.yaml"},
		{
			"relative xdg ignored",
			map[string]string{"XDG_CONFIG_HOME": "rel", "HOME": "/h"},
			"linux",
			"/h/.config/relgap/config.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if filepath.Separator != '/' {
				t.Skip("unix paths")
			}
			got, err := config.UserConfigPath(lookup(tt.env), tt.goos)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	writeFile(t, good, "# only comments\n")
	require.NoError(t, config.ValidateFile(good))

	err := config.ValidateFile(filepath.Join(dir, "missing.yaml"))
	require.ErrorIs(t, err, config.ErrInvalid)
	assert.NotErrorAs(t, err, new(*config.ValidationError))
}

func TestEnvVarName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "RELGAP_SCAN_DETECTORS_CI_ACTIONS", config.EnvVarName("scan.detectors.ci_actions"))
}
