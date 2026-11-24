package render_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
)

// TestAuditMarkdownSilence checks that the audit comment mentions nobody and references no issue:
// advisory summaries come from OSV, and errors can quote a host's answer. Code spans stay as
// they are.
func TestAuditMarkdownSilence(t *testing.T) {
	t.Parallel()
	a, err := report.ParseAudit([]byte(`{
  "repos": ["."], "checked": 1,
  "findings": [
    {"tool": "jq", "version": "1.6", "pins": [], "id": "GHSA-1", "package": {"ecosystem": "GitHub Actions", "name": "j"},
     "summary": "Overflow found by @itchyny in #3324, see https://github.com/jqlang/jq/issues/3324 and ` + "`@x #1`" + `",
     "severity": {"rating": "high", "score": 7.5}, "url": "https://osv.dev/vulnerability/GHSA-1"}
  ],
  "suppressed": [], "unchecked": []
}`))
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, render.AuditMarkdown(&buf, a, []render.Error{{Target: "osv", Message: "ask @octocat about #9"}}))
	got := buf.String()
	assert.Contains(t, got, "found by @&#8203;itchyny in #&#8203;3324, see "+
		"[https://github.com/jqlang/jq/issues/3324](https://redirect.github.com/jqlang/jq/issues/3324) and `@x #1`")
	assert.Contains(t, got, "ask @&#8203;octocat about #&#8203;9")
}
