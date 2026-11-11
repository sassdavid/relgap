package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

const driftRepos = 60

// driftRepoFiles returns the pin files of generated repository i. Most repositories agree; a few
// pin other versions, so terraform, kubectl, node, image:golang and tfprovider:hashicorp/aws
// drift, and repositories 24 and 49 have only a terraform constraint, which counts as its lower
// bound.
func driftRepoFiles(i int) map[string]string {
	terraform, constraint := `terraform = "1.15.8"`+"\n", "~> 1.15"
	switch {
	case i%20 == 7:
		terraform, constraint = `terraform = "1.14.2"`+"\n", "~> 1.14"
	case i%25 == 24:
		terraform = ""
	}
	kubectl := "1.34.2"
	if i%15 == 4 {
		kubectl = "1.33.5"
	}
	node := "24.11.0"
	if i == 13 {
		node = "24.10.0"
	}
	golang := "1.27.3"
	if i%12 == 0 {
		golang = "1.26.1"
	}
	aws := "6.12.0"
	if i%30 == 11 {
		aws = "6.10.0"
	}
	extra := ""
	if i == 42 {
		extra = `jq = "1.7.1"` + "\n"
	}
	return map[string]string{
		"mise.toml":           "[tools]\n" + terraform + `node = ["26", "` + node + `"]` + "\n" + extra,
		".tool-versions":      "kubectl " + kubectl + "\n",
		"Dockerfile":          "FROM golang:" + golang + "\nARG HELM_VERSION=3.19.0\n",
		"versions.tf":         "terraform {\n  required_version = \"" + constraint + "\"\n}\n",
		".terraform.lock.hcl": "provider \"registry.terraform.io/hashicorp/aws\" {\n  version = \"" + aws + "\"\n}\n",
		"root.hcl":            "terragrunt_version_constraint = \">= 1.2\"\n",
		"aqua.yaml":           "packages:\n  - name: cli/cli@v2.83.0\n",
		".github/workflows/ci.yml": "jobs:\n  t:\n    container: node:24.11.0\n    steps:\n" +
			"      - uses: actions/setup-go@v6\n        with:\n          go-version: \"1.27\"\n",
	}
}

// writeDriftTree writes driftRepos repositories under root, ten per team directory. Each has an
// empty .git directory, so the scanner walks it without git.
func writeDriftTree(root string) error {
	for i := range driftRepos {
		repo := filepath.Join(root, fmt.Sprintf("team-%02d", i/10), fmt.Sprintf("repo-%02d", i))
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
			return fmt.Errorf("create repo: %w", err)
		}
		for name, body := range driftRepoFiles(i) {
			path := filepath.Join(repo, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
	}
	return nil
}

// cmdGenTree writes the generated drift tree under a directory of the script's work dir:
//
//	gentree tree
func cmdGenTree(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: gentree <dir>")
	}
	ts.Check(writeDriftTree(ts.MkAbs(args[0])))
}

// cmdRequests counts the HTTP requests relgap logged to $TEST_REQUESTS since the last reset:
//
//	requests 1      fail unless exactly 1 request was sent
//	requests reset  forget the requests logged so far
func cmdRequests(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: requests <n>|reset")
	}
	file := ts.Getenv(envRequests)
	if args[0] == "reset" {
		ts.Check(os.WriteFile(file, nil, 0o600))
		return
	}
	data, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		ts.Fatalf("read request log: %v", err)
	}
	n := 0
	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	if got := strconv.Itoa(n); got != args[0] {
		ts.Fatalf("relgap sent %s requests, want %s:\n%s", got, args[0], data)
	}
}

// TestSpec_output_and_ci_6: drift over a generated tree of 60 repositories matches the goldens
// in tty, plain, md and json, marks constraint pins ≈, makes no network call and exits 3 with
// --fail-on drift.
func TestSpec_output_and_ci_6(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_output_and_ci_6.txtar")))
}

// TestSpec_output_and_ci_7: drift --latest adds the latest versions from one GraphQL request.
func TestSpec_output_and_ci_7(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_output_and_ci_7.txtar")))
}
