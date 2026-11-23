package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

const auditRepos = 20

// auditRepoFiles returns the pin files of generated audit repository i. Every repository pins
// terraform 1.5.0, which has an advisory; kubectl alternates between two versions; k9s 0.50.16
// has no advisory; repository 3 pins terragrunt by prefix, repository 7 pins node to the lts
// channel, repository 9 pins a tool nobody knows, and every tenth repository from 2 on builds a
// node image.
func auditRepoFiles(i int) map[string]string {
	kubectl := "1.34.2"
	if i%2 == 1 {
		kubectl = "1.33.5"
	}
	mise := "[tools]\nkubectl = \"" + kubectl + "\"\n"
	if i%5 == 0 {
		mise += "k9s = \"0.50.16\"\n"
	}
	if i%4 == 1 {
		mise += "helm = \"3.19.0\"\n"
	}
	switch i {
	case 3:
		mise += "terragrunt = \"0.93\"\n"
	case 7:
		mise += "node = \"lts\"\n"
	case 9:
		mise += "no-such-tool-relgap = \"1.0.0\"\n"
	}
	files := map[string]string{
		".terraform-version": "1.5.0\n",
		"mise.toml":          mise,
	}
	if i%10 == 2 {
		files["Dockerfile"] = "FROM node:24.11.0\n"
	}
	return files
}

// writeAuditTree writes auditRepos repositories under root, five per team directory, each with
// an empty .git directory.
func writeAuditTree(root string) error {
	for i := range auditRepos {
		repo := filepath.Join(root, fmt.Sprintf("team-%d", i/5), fmt.Sprintf("repo-%02d", i))
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
			return fmt.Errorf("create repo: %w", err)
		}
		for name, body := range auditRepoFiles(i) {
			if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o600); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
	}
	return nil
}

// cmdGenAuditTree writes the generated audit tree under a directory of the script's work dir:
//
//	genaudittree tree
func cmdGenAuditTree(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: genaudittree <dir>")
	}
	ts.Check(writeAuditTree(ts.MkAbs(args[0])))
}

var osvQueriesField = regexp.MustCompile(` osv_versioned_queries=(\d+)$`)

// cmdOSVQueries sums the versioned queries of the OSV querybatch requests relgap logged to
// $TEST_REQUESTS since the last `requests reset`:
//
//	osvqueries 8  fail unless exactly 8 queries asked about a version
func cmdOSVQueries(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: osvqueries <n>")
	}
	data, err := os.ReadFile(ts.Getenv(envRequests))
	if err != nil && !os.IsNotExist(err) {
		ts.Fatalf("read request log: %v", err)
	}
	n := 0
	for line := range strings.Lines(string(data)) {
		if m := osvQueriesField.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			k, _ := strconv.Atoi(m[1])
			n += k
		}
	}
	if got := strconv.Itoa(n); got != args[0] {
		ts.Fatalf("relgap sent %s versioned OSV queries, want %s:\n%s", got, args[0], data)
	}
}

// TestSpec_vulnerability_audit_3: audit over a generated tree of 20 repositories sends one OSV
// query per distinct tool, version and package, and exits 0, 3 and 1 as ADR-0012 says, with the
// default vuln gate, --fail-on unchecked, --tool and policy.fail_on.
func TestSpec_vulnerability_audit_3(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_vulnerability_audit_3.txtar")))
}
