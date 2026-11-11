package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
)

const (
	benchRepos      = 60
	benchNoiseFiles = 200 // per repository, so the walk and the git listing have work to do
	pinsPerRepo     = 13
	pinsBudget      = 5 * time.Second
)

// repoFiles returns the pin files of one generated repository, holding pinsPerRepo pins.
func repoFiles() map[string]string {
	return map[string]string{
		"mise.toml":           "[tools]\nterraform = \"1.15.8\"\nnode = [\"26\", \"24.11.0\"]\n",
		".tool-versions":      "kubectl 1.34.2\n",
		"Dockerfile":          "FROM golang:1.27.3\nARG HELM_VERSION=3.19.0\n",
		"versions.tf":         "terraform {\n  required_version = \"~> 1.15\"\n}\n",
		".terraform.lock.hcl": "provider \"registry.terraform.io/hashicorp/aws\" {\n  version = \"6.12.0\"\n}\n",
		"root.hcl":            "terragrunt_version_constraint = \">= 1.2\"\n",
		"aqua.yaml":           "packages:\n  - name: cli/cli@v2.83.0\n",
		".github/workflows/ci.yml": "jobs:\n  t:\n    container: node:24.11.0\n    steps:\n" +
			"      - uses: actions/setup-go@v6\n        with:\n          go-version: \"1.27\"\n",
		"charts/app/Chart.yaml": "name: app\nversion: 0.1.0\nappVersion: 1.0.0\n",
	}
}

// genTree writes benchRepos repositories under a new directory. With git on PATH they are real
// repositories, so git lists the files; otherwise the scanner walks them.
func genTree(tb testing.TB, git string) string {
	tb.Helper()
	root := tb.TempDir()
	for i := range benchRepos {
		repo := filepath.Join(root, fmt.Sprintf("team-%02d", i/10), fmt.Sprintf("repo-%02d", i))
		files := repoFiles()
		for j := range benchNoiseFiles {
			files[fmt.Sprintf("src/pkg%d/file%d.go", j%10, j)] = "package pkg\n"
		}
		files["node_modules/dep/Dockerfile"] = "FROM ignored:1.0.0\n"
		files[".gitignore"] = "node_modules/\n"
		for name, body := range files {
			path := filepath.Join(repo, filepath.FromSlash(name))
			require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0o750))
			require.NoError(tb, os.WriteFile(path, []byte(body), 0o600))
		}
		if git == "" {
			require.NoError(tb, os.Mkdir(filepath.Join(repo, ".git"), 0o750))
			continue
		}
		out, err := exec.CommandContext(tb.Context(), git, "init", "-q", repo).CombinedOutput()
		require.NoError(tb, err, string(out))
	}
	return root
}

// pinsDeps runs relgap with a PATH that holds only git (when installed), so neither mise nor
// anything else on the machine takes part.
func pinsDeps(tb testing.TB, git string, stdout *bytes.Buffer) *app.Deps {
	tb.Helper()
	bin := tb.TempDir()
	if git != "" {
		require.NoError(tb, os.Symlink(git, filepath.Join(bin, filepath.Base(git))))
	}
	home := tb.TempDir()
	return &app.Deps{
		Stdin:  bytes.NewReader(nil),
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
		Env: app.Env([]string{
			"PATH=" + bin, "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"), "NO_COLOR=1",
		}),
		Clock: time.Now,
		Getwd: func() (string, error) { return home, nil },
	}
}

func lookGit() string {
	git, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	return git
}

// TestSpec_pindiscovery_5: `relgap pins` over 60 generated repositories, with no network, takes
// under 5 s, both when git lists the files and when the scanner walks them.
func TestSpec_pindiscovery_5(t *testing.T) {
	t.Parallel()
	modes := map[string]string{"walk": ""}
	if git := lookGit(); git != "" {
		modes["git"] = git
	}
	for name, git := range modes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := genTree(t, git)
			var stdout bytes.Buffer
			d := pinsDeps(t, git, &stdout)

			start := time.Now()
			code := cli.Execute(t.Context(), d, []string{"pins", root, "--format", "json"})
			took := time.Since(start)

			require.Equal(t, cli.ExitOK, code, "stderr: %s", d.Stderr)
			var out struct {
				Data struct {
					Repos []string          `json:"repos"`
					Pins  []json.RawMessage `json:"pins"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
			require.Len(t, out.Data.Repos, benchRepos)
			require.Len(t, out.Data.Pins, benchRepos*pinsPerRepo, "node_modules must be skipped")
			t.Logf("%d repositories, %d pins in %s", len(out.Data.Repos), len(out.Data.Pins), took)
			require.Less(t, took, pinsBudget)
		})
	}
}

func BenchmarkPins(b *testing.B) {
	git := lookGit()
	root := genTree(b, git)
	for b.Loop() {
		var stdout bytes.Buffer
		if code := cli.Execute(
			b.Context(),
			pinsDeps(b, git, &stdout),
			[]string{"pins", root, "--format", "json"},
		); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}
