package render_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
)

// TestMarkdownNames checks that every Markdown format writes a tool name that GitHub could read as
// a mention or team, such as npm:@types/node, as a code span: it then reads and copies as written,
// and GitHub links nothing in code. Other names stay as they are.
func TestMarkdownNames(t *testing.T) {
	t.Parallel()
	audit, err := report.ParseAudit([]byte(`{
  "repos": ["."], "checked": 2,
  "findings": [
    {"tool": "npm:@types/node", "version": "22.0.0", "pins": [], "id": "GHSA-1", "package": {"ecosystem": "npm", "name": "@types/node"},
     "severity": {"rating": "high", "score": 7.5}, "fix": {"lowest": "22.1.0"}, "url": "u"},
    {"tool": "kubectl", "version": "1.37.2", "pins": [], "id": "GO-1", "package": {"ecosystem": "Go", "name": "k"},
     "severity": {"rating": "low", "score": 3.1}, "url": "u"}
  ],
  "suppressed": [
    {"tool": "npm:@a/b", "version": "1.0.0", "pins": [], "id": "GHSA-2", "package": {"ecosystem": "npm", "name": "@a/b"},
     "severity": {"rating": "low", "score": 2}, "url": "u", "suppressed": {"kind": "ignore", "reason": "r"}}
  ],
  "unchecked": [{"tool": "npm:@biomejs/biome", "version": "1.9.0", "reason": "unresolved", "pins": []}]
}`))
	require.NoError(t, err)
	unchecked := []report.Unchecked{{Tool: "npm:@biomejs/biome", Version: "1.9.0", Reason: "unresolved"}}
	drift := &report.Drift{Repos: []string{"a"}, Tools: []report.DriftTool{{
		Tool:     "npm:@types/node",
		Versions: []report.DriftVersion{{Version: "22.0.0", Repos: []string{"a"}}},
		Pins: []report.DriftPin{
			{
				Location: scan.Location{
					Repo: "a",
					File: "mise.toml",
				},
				Value:   "22.0.0",
				Match:   scan.MatchExact,
				Version: "22.0.0",
			},
		},
	}}}
	pins := []scan.Pin{
		{Tool: "npm:@types/node", Value: "22.0.0", Exact: true, Location: scan.Location{File: "mise.toml", Line: 2}},
		{Tool: "kubectl", Value: "1.37.2", Exact: true, Location: scan.Location{File: "mise.toml", Line: 3}},
	}
	digest := &report.Digest{Tools: []report.ToolDigest{{
		Tool: "npm:@types/node", Repo: "DefinitelyTyped/DefinitelyTyped", From: "22.0.0", To: "22.1.0",
	}}}
	tests := []struct {
		name  string
		write func(io.Writer) error
		want  []string
	}{
		{
			"audit",
			func(w io.Writer) error {
				return render.AuditMarkdown(w, audit, []render.Error{{Target: "npm:@c/d", Message: "failed"}})
			},
			[]string{
				"| `npm:@types/node` | 22.0.0 | GHSA-1 |", "| kubectl | 1.37.2 | GO-1 |",
				"- `npm:@types/node` 22.0.0: upgrade to 22.1.0", "- GHSA-2 in `npm:@a/b` 1.0.0: ",
				"- `npm:@c/d`: failed", "- `npm:@biomejs/biome` 1.9.0 (): unresolved",
			},
		},
		{
			"outdated",
			func(w io.Writer) error { return render.OutdatedMarkdown(w, &report.Outdated{Unchecked: unchecked}) },
			[]string{"- `npm:@biomejs/biome` 1.9.0 (): unresolved"},
		},
		{
			"drift",
			func(w io.Writer) error { return render.DriftMarkdown(w, drift, render.DriftOptions{}) },
			[]string{"In agreement: `npm:@types/node` 22.0.0 (1 repo)"},
		},
		{
			"pins",
			func(w io.Writer) error { return render.PinsMarkdown(w, pins, false) },
			[]string{"| `npm:@types/node` | 22.0.0 |", "| kubectl | 1.37.2 |"},
		},
		{
			"digest",
			func(w io.Writer) error {
				return render.DigestMarkdown(w, digest, []render.Error{{Target: "npm:@c/d", Message: "x"}}, nil)
			},
			[]string{"## `npm:@types/node` 22.0.0..22.1.0", "- `npm:@c/d`: failed: x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			require.NoError(t, tt.write(&buf))
			for _, want := range tt.want {
				assert.Contains(t, buf.String(), want)
			}
			assert.NotContains(t, buf.String(), "&#8203;", "a name copies as written")
		})
	}
}

// TestMarkdownNameWithBacktick: a name with a backtick can't be a simple code span, so it's
// silenced instead.
func TestMarkdownNameWithBacktick(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, render.PinsMarkdown(&buf, []scan.Pin{{Tool: "x`@y", Value: "1", Exact: true}}, false))
	assert.Contains(t, buf.String(), "| x`@&#8203;y | 1 |")
}
