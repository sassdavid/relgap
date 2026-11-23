package report_test

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
)

// TestAuditVersions: an exact pin is its own version without a request, a prefix and a
// constraint resolve against the full list as outdated resolves them, the latest channel comes
// from one Recent batch, and pins of one tool version share a request.
func TestAuditVersions(t *testing.T) {
	t.Parallel()
	gh, resolve := outdatedFixture()
	pins := []scan.Pin{
		pin("a", "1.0.0", scan.MatchExact, ".", "mise.toml", 1),
		pin("a", "~> 1.0", scan.MatchConstraint, ".", "main.tf", 2),
		pin("a", "~> 1.1", scan.MatchConstraint, "other", "main.tf", 2),
		pin("a", "1.1", scan.MatchPrefix, ".", "ci.yml", 3),
		pin("b", "v3.0.0", scan.MatchExact, ".", "mise.toml", 4),
		pin("b", "v3.0.0", scan.MatchExact, "other", "mise.toml", 4),
		pin("c", "latest", scan.MatchChannel, ".", "mise.toml", 5),
		pin("c", "lts", scan.MatchChannel, ".", "mise.toml", 6),
		pin("d", "1.0.0", scan.MatchExact, ".", "mise.toml", 7),
	}
	digester := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil)
	got, err := report.NewAuditor(digester, gh).Audit(t.Context(), report.AuditRequest{
		Repos: []string{".", "other"}, Pins: pins, Resolve: resolve, Concurrency: 4,
	})
	require.NoError(t, err)

	reqs := got.EnrichRequests(resolve, nil)
	versions := make([]string, len(reqs))
	for i, r := range reqs {
		versions[i] = r.Name + "@" + r.Current.String()
		assert.True(t, r.Target.IsZero(), "audit has no target version")
	}
	assert.Equal(t, []string{"a@1.1.0", "a@1.0.0", "b@3.0.0", "c@0.5.0"}, versions)
	assert.Equal(t, 1, gh.listed["o/a"], "a's full list is read once, for its prefix and constraints")
	assert.Zero(t, gh.listed["o/b"], "exact pins need no list")
	assert.Zero(t, gh.listed["o/c"], "the latest channel needs no full list")
	require.Len(t, gh.recentQs, 1)
	assert.Len(t, gh.recentQs[0], 1, "only c has a latest channel pin")
	assert.Empty(t, got.Failed())

	got.ApplyEnrichment(map[string]enrich.Result{}, nil)
	reasons := map[string]string{}
	for _, u := range got.Unchecked {
		reasons[u.Tool+" "+u.Version] = u.Reason
	}
	assert.Equal(t, map[string]string{
		"c lts":   enrich.ReasonNotExact,
		"d 1.0.0": enrich.ReasonNoSource,
	}, reasons)
}

// TestAuditFindings: one finding per tool version and advisory, with every pin of that version,
// sorted by severity with unrated advisories last; records that don't affect the version and
// unchecked versions add none.
func TestAuditFindings(t *testing.T) {
	t.Parallel()
	gh, resolve := outdatedFixture()
	pins := []scan.Pin{
		pin("a", "1.0.0", scan.MatchExact, "x", "mise.toml", 1),
		pin("a", "1.0.0", scan.MatchExact, "y", "mise.toml", 1),
		pin("b", "3.0.0", scan.MatchExact, "x", "mise.toml", 2),
		pin("c", "0.5.0", scan.MatchExact, "x", "mise.toml", 3),
	}
	digester := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil)
	got, err := report.NewAuditor(digester, gh).Audit(t.Context(), report.AuditRequest{
		Repos: []string{"x", "y"}, Pins: pins, Resolve: resolve, Concurrency: 2,
	})
	require.NoError(t, err)
	reqs := got.EnrichRequests(resolve, nil)
	require.Len(t, reqs, 3)
	high := &enrich.Score{Score: 7.5, Rating: "high"}
	low := &enrich.Score{Score: 3.1, Rating: "low"}
	fix := &enrich.Fix{
		Lowest: "1.2.0", Latest: "1.3.0", LatestAffected: true, LatestAffectedBy: []string{"GO-2"},
		Unfixed: []string{"GO-2"},
	}
	got.ApplyEnrichment(map[string]enrich.Result{
		reqs[0].Key: {Fix: fix, Vulns: []enrich.Vuln{
			{ID: "GO-2", AffectsCurrent: true, Severity: &enrich.Score{Rating: enrich.RatingUnknown}},
			{ID: "GHSA-low", Severity: low, AffectsCurrent: true},
			{ID: "GHSA-gone", Severity: high, AffectsCurrent: false},
		}},
		reqs[1].Key: {Vulns: []enrich.Vuln{{ID: "GHSA-high", Severity: high, AffectsCurrent: true, FixedIn: "3.0.1"}}},
		reqs[2].Key: {Unchecked: enrich.ReasonFailed},
	}, nil)

	type row struct {
		ID, Tool, Version string
		Pins              int
	}
	rows := make([]row, len(got.Findings))
	for i, f := range got.Findings {
		rows[i] = row{f.ID, f.Tool, f.Version, len(f.Pins)}
	}
	assert.Equal(t, []row{
		{"GHSA-high", "b", "3.0.0", 1},
		{"GHSA-low", "a", "1.0.0", 2},
		{"GO-2", "a", "1.0.0", 2},
	}, rows)
	assert.Equal(t, report.AdvisoryURL+"GHSA-high", got.Findings[0].URL)
	assert.Equal(t, 2, got.Checked, "c failed, so two tool versions were checked")
	require.Len(t, got.Unchecked, 1)
	assert.Equal(t, enrich.ReasonFailed, got.Unchecked[0].Reason)

	gate := report.Gate{Values: []string{report.FailOnVuln, report.FailOnUnchecked}, VulnSeverity: "high"}
	findings := gate.CheckAudit(got)
	var (
		vulns     []string
		unchecked int
	)
	for _, f := range findings {
		switch f.FailOn {
		case report.FailOnVuln:
			vulns = append(vulns, f.Advisory.ID)
		case report.FailOnUnchecked:
			unchecked++
			assert.Equal(t, "c", f.Unchecked.Tool)
		}
	}
	assert.Equal(t, []string{"GHSA-high", "GO-2"}, vulns,
		"the high advisory and the one without a rating reach --vuln-severity high")
	assert.Equal(t, 1, unchecked)
	assert.Equal(t, []string{report.FailOnVuln, report.FailOnUnchecked},
		[]string{gate.Matches(findings)[0].FailOn, gate.Matches(findings)[1].FailOn})

	gate = report.Gate{Values: []string{report.FailOnVuln}, VulnFixable: true}
	vulns = nil
	for _, f := range gate.CheckAudit(got) {
		vulns = append(vulns, f.Advisory.ID)
	}
	assert.Equal(t, []string{"GHSA-high"}, vulns, "--vuln-fixable counts only the advisory with a fixed version")

	// a's fix target is on each of its findings, null on the advisory no release fixes; b has none.
	fixes := map[string]string{}
	for _, f := range got.Findings {
		b, marshalErr := json.Marshal(f)
		require.NoError(t, marshalErr)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &m))
		fixes[f.ID] = string(m["fix"])
	}
	assert.Equal(t, map[string]string{
		"GHSA-high": "",
		"GHSA-low":  `{"lowest":"1.2.0","latest":"1.3.0","latest_affected":true,"latest_affected_by":["GO-2"],"unfixed":["GO-2"]}`,
		"GO-2":      "null",
	}, fixes)
	assert.Equal(t, []report.ToolFix{{Tool: "a", Version: "1.0.0", Fix: fix}}, got.Fixes())
}
