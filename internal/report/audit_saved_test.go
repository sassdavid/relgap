package report_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
)

// savedAudit is the data of a saved audit: kubectl 1.33.5 has a fixable advisory and one no
// release fixes, kubectl 1.34.2 only the unfixed one, and terraform's target is unknown.
const savedAudit = `{
  "repos": ["."], "checked": 3,
  "findings": [
    {"tool": "kubectl", "version": "1.33.5", "pins": [], "id": "GO-1", "package": {"ecosystem": "Go", "name": "k"},
     "severity": {"rating": "unknown"}, "fix": null, "url": "u", "kev": {"cve": "CVE-1", "date_added": "2026-01-01",
     "due_date": "2026-02-01", "ransomware": false}},
    {"tool": "kubectl", "version": "1.33.5", "pins": [], "id": "GHSA-2", "package": {"ecosystem": "Go", "name": "k"},
     "severity": {"rating": "high", "score": 7.5}, "fix": {"lowest": "1.33.6", "latest": "1.37.2",
     "latest_affected": true, "latest_affected_by": ["GO-1"], "unfixed": ["GO-1"]}, "url": "u"},
    {"tool": "kubectl", "version": "1.34.2", "pins": [], "id": "GO-1", "package": {"ecosystem": "Go", "name": "k"},
     "severity": {"rating": "unknown"}, "fix": null, "url": "u"},
    {"tool": "terraform", "version": "1.5.0", "pins": [], "id": "GHSA-3", "package": {"ecosystem": "Go", "name": "t"},
     "severity": {"rating": "low", "score": 3.1}, "url": "u"}
  ],
  "suppressed": [
    {"tool": "helm", "version": "3.19.0", "pins": [], "id": "GHSA-4", "package": {"ecosystem": "Go", "name": "h"},
     "severity": {"rating": "medium", "score": 5}, "url": "u", "suppressed": {"kind": "ignore", "reason": "r"}}
  ],
  "unchecked": []
}`

func TestParseAudit(t *testing.T) {
	t.Parallel()
	a, err := report.ParseAudit([]byte(savedAudit))
	require.NoError(t, err)
	assert.Equal(t, report.Feeds{KEV: true}, a.Feeds())
	assert.Empty(t, a.Failed())
	assert.Equal(t, []report.ToolFix{
		{Tool: "kubectl", Version: "1.33.5", Fix: &enrich.Fix{
			Lowest: "1.33.6", Latest: "1.37.2", LatestAffected: true, LatestAffectedBy: []string{"GO-1"},
			Unfixed: []string{"GO-1"},
		}},
		{Tool: "kubectl", Version: "1.34.2", Fix: &enrich.Fix{Unfixed: []string{"GO-1"}}},
	}, a.Fixes())

	// The fix fields write back as they were read: null, an object, or absent.
	out, err := json.Marshal(a.Findings)
	require.NoError(t, err)
	var got, want []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &got))
	var in struct {
		Findings []map[string]json.RawMessage `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(savedAudit), &in))
	want = in.Findings
	for i := range want {
		w, ok := want[i]["fix"]
		g, gok := got[i]["fix"]
		assert.Equal(t, ok, gok, "finding %d", i)
		if ok {
			assert.JSONEq(t, string(w), string(g), "finding %d", i)
		}
	}

	_, err = report.ParseAudit([]byte(`{"findings": 1}`))
	assert.ErrorContains(t, err, "parse audit")
}

func TestAuditAtSeverity(t *testing.T) {
	t.Parallel()
	a, err := report.ParseAudit([]byte(savedAudit))
	require.NoError(t, err)
	ids := func(fs []report.AuditFinding) []string {
		out := []string{}
		for _, f := range fs {
			out = append(out, f.ID+" "+f.Version)
		}
		return out
	}
	tests := []struct {
		rating           string
		find, suppressed []string
	}{
		{"", []string{"GO-1 1.33.5", "GHSA-2 1.33.5", "GO-1 1.34.2", "GHSA-3 1.5.0"}, []string{"GHSA-4 3.19.0"}},
		{"medium", []string{"GO-1 1.33.5", "GHSA-2 1.33.5", "GO-1 1.34.2"}, []string{"GHSA-4 3.19.0"}},
		{"critical", []string{"GO-1 1.33.5", "GO-1 1.34.2"}, []string{}},
	}
	for _, tt := range tests {
		got := a.AtSeverity(tt.rating)
		assert.Equal(t, tt.find, ids(got.Findings), tt.rating)
		assert.Equal(t, tt.suppressed, ids(got.Suppressed), tt.rating)
		assert.Equal(t, a.Fixes(), got.Fixes(), tt.rating)
	}
	assert.Len(t, a.Findings, 4, "AtSeverity leaves the report as it was")
}

func TestAuditOpen(t *testing.T) {
	t.Parallel()
	type openCase struct {
		name string
		a    report.Audit
		want bool
	}
	tests := []openCase{
		{"nothing", report.Audit{}, false},
		{"finding", report.Audit{Findings: []report.AuditFinding{{ID: "GHSA-1"}}}, true},
		{"suppressed only", report.Audit{Suppressed: []report.AuditFinding{{ID: "GHSA-1"}}}, false},
	}
	for _, reason := range []string{
		enrich.ReasonNoSource, enrich.ReasonPrivate, enrich.ReasonNotExact,
		enrich.ReasonFailed, enrich.ReasonOffline, enrich.ReasonUnresolved,
	} {
		open := reason == enrich.ReasonFailed || reason == enrich.ReasonOffline || reason == enrich.ReasonUnresolved
		tests = append(tests, openCase{reason, report.Audit{Unchecked: []report.Unchecked{{Reason: reason}}}, open})
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.a.Open(), tt.name)
	}
}
