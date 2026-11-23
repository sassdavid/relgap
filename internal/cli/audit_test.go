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

var osvQueriesField = regexp.MustCompile(` osv_versioned_queries=(\d+) osv_versionless_queries=(\d+)$`)

// cmdOSVQueries counts the queries of the OSV querybatch requests relgap logged to $TEST_REQUESTS
// since the last `requests reset`. The first querybatch with a versioned query asks about the
// pins; the later ones ask about fix target candidates. Versionless queries are counted apart,
// over every querybatch: GitHub Actions packages and the go: module search send them.
//
//	osvqueries 8                 fail unless the pins took exactly 8 versioned queries and no
//	                             candidate was asked about
//	osvqueries 8 fix 3           the same, with exactly 3 candidate queries
//	osvqueries 0 versionless 1   no versioned query, and exactly 1 versionless one
func cmdOSVQueries(ts *testscript.TestScript, neg bool, args []string) {
	const usage = "usage: osvqueries <n> [fix <m>] [versionless <k>]"
	if neg || len(args)%2 != 1 {
		ts.Fatalf(usage)
	}
	want := map[string]string{"fix": "0"}
	for i := 1; i < len(args); i += 2 {
		if args[i] != "fix" && args[i] != "versionless" {
			ts.Fatalf(usage)
		}
		want[args[i]] = args[i+1]
	}
	data, err := os.ReadFile(ts.Getenv(envRequests))
	if err != nil && !os.IsNotExist(err) {
		ts.Fatalf("read request log: %v", err)
	}
	pins, candidates, versionless, batches := 0, 0, 0, 0
	for line := range strings.Lines(string(data)) {
		m := osvQueriesField.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		k, _ := strconv.Atoi(m[1])
		n, _ := strconv.Atoi(m[2])
		versionless += n
		switch {
		case k == 0:
		case batches == 0:
			pins = k
			batches++
		default:
			candidates += k
		}
	}
	if got := strconv.Itoa(pins); got != args[0] {
		ts.Fatalf("relgap sent %s versioned OSV queries for the pins, want %s:\n%s", got, args[0], data)
	}
	if got := strconv.Itoa(candidates); got != want["fix"] {
		ts.Fatalf("relgap sent %s OSV queries for fix target candidates, want %s:\n%s", got, want["fix"], data)
	}
	if w, ok := want["versionless"]; ok && strconv.Itoa(versionless) != w {
		ts.Fatalf("relgap sent %d versionless OSV queries, want %s:\n%s", versionless, w, data)
	}
}

// TestSpec_vulnerability_audit_3: audit over a generated tree of 20 repositories sends one OSV
// query per distinct tool, version and package, and exits 0, 3 and 1 as ADR-0012 says, with the
// default vuln gate, --fail-on unchecked, --tool and policy.fail_on.
func TestSpec_vulnerability_audit_3(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_vulnerability_audit_3.txtar")))
}

// TestSpec_vulnerability_audit_4: a GO- record without a rating takes its GHSA alias's rating, a
// record without any rating counts under --vuln-severity high in audit and outdated, a withdrawn
// record is dropped, and a record with an unchanged modified time isn't fetched again.
func TestSpec_vulnerability_audit_4(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_vulnerability_audit_4.txtar")))
}

// TestSpec_vulnerability_audit_6: an action pin at an affected tag gets GHSA-cxww-7g56-2vh6 from a
// versionless query, the fixed tag, pinned by SHA with a version comment, doesn't, and the fix
// target is the fixed tag.
func TestSpec_vulnerability_audit_6(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_vulnerability_audit_6.txtar")))
}

// TestSpec_vulnerability_audit_7: vulns.ignore suppresses by ID and by alias, an expired entry
// warns and stops matching, an unused entry warns, an OpenVEX not_affected statement suppresses,
// and --format openvex validates against the OpenVEX v0.2.0 JSON schema.
func TestSpec_vulnerability_audit_7(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_vulnerability_audit_7.txtar")))
}
