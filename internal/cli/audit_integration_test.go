package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/internal/testutil/mcptest"
)

// auditTreeCassettes answer an audit of the generated tree (spec_vulnerability_audit_3.txtar).
func auditTreeCassettes() []string {
	return []string{"audit/tree.yaml", "audit/tree-fix.yaml", "audit/tree-added.yaml"}
}

// auditLists are the lists an audit page splits, in the order pages take them.
func auditLists() []string { return []string{"/data/findings", "/data/unchecked", "/data/suppressed"} }

// TestSpec_vulnerability_audit_9: the MCP audit tool returns the envelope of `relgap audit
// --format json`, narrowed by vuln_severity and split into pages across findings, unchecked and
// suppressed; notify publishes a saved audit as a pull request comment of kind audit, opened by
// findings and failures but not by pins no advisory source covers. The relgap-action half of
// the criterion runs on sassdavid/action-test.
func TestSpec_vulnerability_audit_9(t *testing.T) {
	t.Parallel()
	t.Run("mcp", testAuditMCP)
	t.Run("notify", testAuditNotify)
}

func testAuditMCP(t *testing.T) {
	t.Parallel()
	f := newMCPFixture(t, "2026-11-23T12:00:00Z", auditTreeCassettes()...)
	require.NoError(t, writeAuditTree(filepath.Join(f.dir, "tree")))
	// Suppressing terraform's only advisory gives the report a suppressed finding and leaves the
	// other tool versions' requests as the cassettes recorded them.
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, ".relgap.yaml"), []byte(
		"vulns:\n  ignore:\n    - id: CVE-2023-4782\n      reason: terraform init runs only on trusted modules\n",
	), 0o600))
	args := map[string]any{"roots": []string{"tree"}}
	var whole []byte
	f.serve(t, []string{"--max-result-bytes", "0"}, func(c *mcptest.Client) {
		whole = mcptest.Text(t, c.Call(t, "audit", args))
		require.NoError(t, validateJSON("audit-v1.json", mcptest.Indent(t, whole)))
		assert.Len(t, mcptest.Items(t, whole, "/data/findings"), 8)
		assert.Len(t, mcptest.Items(t, whole, "/data/suppressed"), 1)
		assert.Len(t, mcptest.Items(t, whole, "/data/unchecked"), 3)

		// vuln_severity drops the two low findings of GO-2025-3547 from the list.
		medium := mcptest.Text(
			t,
			c.Call(t, "audit", map[string]any{"roots": []string{"tree"}, "vuln_severity": "medium"}),
		)
		findings := mcptest.Items(t, medium, "/data/findings")
		assert.Len(t, findings, 6)
		for _, item := range findings {
			assert.NotContains(t, string(item), `"GO-2025-3547"`)
		}

		msg := mcptest.ErrorText(t, c.Call(t, "audit", map[string]any{"vuln_severity": "severe"}))
		assert.Contains(t, msg, `invalid --vuln-severity "severe"`)
		msg = mcptest.ErrorText(t, c.Call(t, "audit", map[string]any{"roots": []string{filepath.Dir(f.dir)}}))
		assert.Contains(t, msg, "outside the server's roots")
	})

	// Under a small cap, the pages take findings, then unchecked, then suppressed, and together
	// hold every item of the whole result once, in order.
	const maxBytes = 6000
	f.serve(t, []string{"--max-result-bytes", "6000"}, func(c *mcptest.Client) {
		pages := c.Pages(t, "audit", args)
		require.Greater(t, len(pages), 2)
		var got []json.RawMessage
		lastList := 0
		for n, p := range pages {
			assert.LessOrEqual(t, len(p), maxBytes, "page %d", n)
			require.NoError(t, validateJSON("audit-v1.json", mcptest.Indent(t, p)), "page %d", n)
			page := mcptest.PageOf(t, p)
			require.NotNil(t, page)
			assert.Equal(t, "findings, unchecked and suppressed", page.Items)
			for list, pointer := range auditLists() {
				items := mcptest.Items(t, p, pointer)
				if len(items) > 0 {
					assert.GreaterOrEqual(t, list, lastList, "page %d goes back to %s", n, pointer)
					lastList = list
				}
			}
			for _, pointer := range auditLists() {
				got = append(got, mcptest.Items(t, p, pointer)...)
			}
		}
		var want []json.RawMessage
		for _, pointer := range auditLists() {
			want = append(want, mcptest.Items(t, whole, pointer)...)
		}
		same := func(a, b json.RawMessage) bool { return bytes.Equal(compact(t, a), compact(t, b)) }
		assert.True(t, slices.EqualFunc(got, want, same), "the pages hold %d items, want the %d of the whole result",
			len(got), len(want))
	})
}

func testAuditNotify(t *testing.T) {
	t.Parallel()
	n := newNotifyEnv(t)
	// The saved reports come from the MCP tool, whose result is the envelope of `relgap audit
	// --format json` (TestSpec_mcp_and_llm_1 compares them) with repositories named relative to
	// the root.
	save := func(name string, c *mcptest.Client, args map[string]any) string {
		t.Helper()
		out := filepath.Join(n.dir, name+".json")
		require.NoError(t, os.WriteFile(out, mcptest.Indent(t, mcptest.Text(t, c.Call(t, "audit", args))), 0o600))
		return out
	}
	var images, tree, k9s string
	f := newMCPFixture(t, "2026-11-23T12:00:00Z", auditTreeCassettes()...)
	require.NoError(t, writeAuditTree(filepath.Join(f.dir, "tree")))
	f.serve(t, []string{"--max-result-bytes", "0"}, func(c *mcptest.Client) {
		images = save("images", c, map[string]any{"roots": []string{"tree"}, "tools": []string{"image:node"}})
		tree = save("tree", c, map[string]any{"roots": []string{"tree"}})
	})
	// The offline server has its own empty cache, so k9s has no OSV answer and is unchecked.
	offline := newMCPFixture(t, "2026-11-23T12:00:00Z")
	require.NoError(t, writeAuditTree(filepath.Join(offline.dir, "tree")))
	offline.serve(t, []string{"--offline", "--max-result-bytes", "0"}, func(c *mcptest.Client) {
		k9s = save("k9s", c, map[string]any{"roots": []string{"tree"}, "tools": []string{"k9s"}})
	})
	notify := func(from string) string {
		t.Helper()
		code, stdout, stderr := n.run(t, n.env, "notify", "--from", from, "--notify", "github-pr-comment")
		require.Equal(t, cli.ExitOK, code, stderr)
		return stdout
	}

	// An image no advisory source covers is nothing to report.
	assert.Equal(t, "github-pr-comment  acme/tools#7  skipped  audit\n", notify(images))
	assert.Empty(t, n.fake.takeWrites())

	assert.Equal(t, "github-pr-comment  acme/tools#7  create  audit\n", notify(tree))
	assert.Equal(t, []string{"POST " + prComments}, n.fake.takeWrites())
	body := n.fake.body()
	assert.True(t, strings.HasPrefix(body, `<!-- relgap:v1 {"kind":"audit","scope":"pull-request","hash":"sha256:`))
	testutil.Golden(t, filepath.Join(goldenDir, "audit", "tree.comment.md"), []byte(body))

	// A pin left unchecked by --offline and the OSV error open the comment.
	assert.Equal(t, "github-pr-comment  acme/tools#7  update  audit\n", notify(k9s))
	body = n.fake.body()
	assert.Contains(t, body, "\n- osv: k9s: ")
	assert.Contains(t, body, "\n- k9s 0.50.16 (tree/team-0/repo-00/mise.toml:3, ")
	n.fake.takeWrites()

	assert.Equal(t, "github-pr-comment  acme/tools#7  resolve  audit\n", notify(images))
	assert.Contains(t, n.fake.body(), "## relgap audit\n\nNothing to report.\n")
}
