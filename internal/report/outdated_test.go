package report_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// multiGitHub serves releases of several repositories and counts the calls that cost requests.
type multiGitHub struct {
	releases  map[string][]github.Release // by repo, newest first
	recentErr map[string]error

	mu       sync.Mutex
	listed   map[string]int // Releases calls by repo
	bodies   map[string]int // ReleaseBodies calls by repo
	recentQs [][]github.RecentQuery
}

func (f *multiGitHub) Recent(_ context.Context, qs []github.RecentQuery) (map[string]github.Recent, error) {
	f.mu.Lock()
	f.recentQs = append(f.recentQs, qs)
	f.mu.Unlock()
	out := map[string]github.Recent{}
	for _, q := range qs {
		out[q.Repo] = github.Recent{Releases: f.releases[q.Repo], Err: f.recentErr[q.Repo]}
	}
	return out, nil
}

func (f *multiGitHub) Releases(_ context.Context, repo string) ([]github.Release, error) {
	f.mu.Lock()
	f.listed[repo]++
	f.mu.Unlock()
	return f.releases[repo], nil
}

func (f *multiGitHub) Tags(context.Context, string) ([]string, error) { return nil, nil }

func (f *multiGitHub) RootFiles(context.Context, string) ([]github.File, error) { return nil, nil }

func (f *multiGitHub) ReleaseBodies(_ context.Context, repo string, tags []string) (map[string]github.Release, error) {
	f.mu.Lock()
	f.bodies[repo]++
	f.mu.Unlock()
	out := map[string]github.Release{}
	for _, r := range f.releases[repo] {
		for _, t := range tags {
			if r.Tag == t {
				out[t] = r
			}
		}
	}
	return out, nil
}

func (f *multiGitHub) File(context.Context, string, string, string) ([]byte, error) {
	return nil, github.ErrNotFound
}

func pin(tool, value string, match scan.Match, repo, file string, line int) scan.Pin {
	v := value
	if match == scan.MatchExact || match == scan.MatchPrefix {
		v = trimV(value)
	}
	return scan.Pin{
		Tool: tool, RawName: tool, Kind: scan.KindTool, Value: value, Version: v,
		Exact: match == scan.MatchExact, Match: match,
		Location: scan.Location{Repo: repo, File: file, Line: line, Detector: scan.DetectorMise},
	}
}

func trimV(s string) string {
	if len(s) > 0 && s[0] == 'v' {
		return s[1:]
	}
	return s
}

func outdatedFixture() (*multiGitHub, report.ResolveFunc) {
	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	gh := &multiGitHub{
		releases: map[string][]github.Release{
			"o/a": {
				{Tag: "v2.0.0-rc.1", Prerelease: true, PublishedAt: day(9)},
				{Tag: "v1.2.0", PublishedAt: day(5), Body: "## Deprecations\n\n- the old flag\n"},
				{Tag: "v1.1.0", PublishedAt: day(3), Body: "## Breaking changes\n\n- removed the x command\n"},
				{Tag: "v1.0.0", PublishedAt: day(1)},
			},
			"o/b": {{Tag: "v3.0.0"}, {Tag: "v2.9.0"}},
			"o/c": {{Tag: "v0.5.0"}, {Tag: "v0.4.0"}},
		},
		recentErr: map[string]error{"o/e": fmt.Errorf("list recent releases of o/e: %w", github.ErrNotFound)},
		listed:    map[string]int{},
		bodies:    map[string]int{},
	}
	specs := map[string]string{"a": "o/a", "b": "o/b", "c": "o/c", "e": "o/e"}
	resolve := func(tool string) (registry.ToolSpec, error) {
		repo, ok := specs[tool]
		if !ok {
			return registry.ToolSpec{}, fmt.Errorf("%w for %q", registry.ErrNoRepo, tool)
		}
		return registry.ToolSpec{
			Name: tool,
			Repo: repo,
			Version: registry.VersionSpec{
				Scheme:    version.SchemeSemver,
				TagPrefix: "v",
				Universe:  registry.UniverseReleases,
			},
			Sources: []registry.Source{{Kind: "gh-release"}},
		}, nil
	}
	return gh, resolve
}

func TestOutdated(t *testing.T) {
	t.Parallel()
	gh, resolve := outdatedFixture()
	pins := []scan.Pin{
		pin("a", "1.0.0", scan.MatchExact, ".", "mise.toml", 1),
		pin("a", "~> 1.0", scan.MatchConstraint, ".", "main.tf", 2),
		pin("a", "~> 1.1", scan.MatchConstraint, "other", "main.tf", 2),
		pin("a", "1.1", scan.MatchPrefix, ".", "ci.yml", 3),
		pin("b", "v3.0.0", scan.MatchExact, ".", "mise.toml", 4),
		pin("c", "latest", scan.MatchChannel, ".", "mise.toml", 5),
		pin("c", "lts", scan.MatchChannel, ".", "mise.toml", 6),
		pin("d", "1.0.0", scan.MatchExact, ".", "mise.toml", 7),
		pin("e", "1.0.0", scan.MatchExact, ".", "mise.toml", 8),
	}
	digester := report.NewDigester(slog.New(slog.DiscardHandler), gh, nil)
	got, err := report.NewOutdater(digester, gh).Outdated(t.Context(), report.OutdatedRequest{
		Repos: []string{".", "other"}, Pins: pins, Resolve: resolve, MinConfidence: 0.5, Concurrency: 4,
	})
	require.NoError(t, err)

	type row struct {
		Tool, Where, Current, Via, Latest string
		Status                            report.Status
		Behind, Breaking, Deprecation     int
	}
	rows := make([]row, len(got.Pins))
	for i, p := range got.Pins {
		r := row{
			Tool: p.Tool, Where: p.Pin.Location.Repo + "/" + p.Pin.Location.File, Current: p.Current, Via: p.Via,
			Latest: p.Latest, Status: p.Status,
			Breaking: p.Counts[classify.KindBreaking], Deprecation: p.Counts[classify.KindDeprecation],
		}
		if p.Behind != nil {
			r.Behind = p.Behind.Releases
		}
		rows[i] = r
	}
	assert.Equal(t, []row{
		{"a", "./ci.yml", "1.1.0", report.ViaPrefix, "1.2.0", report.StatusOutdated, 1, 0, 1},
		{"a", "./main.tf", "1.0.0", string(version.PickKnown), "1.2.0", report.StatusOutdated, 2, 1, 1},
		{"a", "./mise.toml", "1.0.0", "", "1.2.0", report.StatusOutdated, 2, 1, 1},
		{"a", "other/main.tf", "1.1.0", string(version.PickLowest), "1.2.0", report.StatusOutdated, 1, 0, 1},
		{"b", "./mise.toml", "3.0.0", "", "3.0.0", report.StatusCurrent, 0, 0, 0},
		{"c", "./mise.toml", "0.5.0", report.ViaLatest, "0.5.0", report.StatusCurrent, 0, 0, 0},
		{"c", "./mise.toml", "", "", "", report.StatusUnknown, 0, 0, 0},
		{"d", "./mise.toml", "", "", "", report.StatusSkipped, 0, 0, 0},
		{"e", "./mise.toml", "", "", "", report.StatusError, 0, 0, 0},
	}, rows)

	assert.Len(t, gh.recentQs, 1, "one Recent call for every tool")
	assert.Equal(t, 1, gh.listed["o/a"], "the full release list of a is read once")
	assert.Zero(t, gh.listed["o/b"], "an exact pin at the latest version needs no full list")
	assert.Zero(t, gh.listed["o/c"], "a latest channel pin needs no full list")
	assert.Equal(t, 2, gh.bodies["o/a"], "one digest from 1.0.0 and one from 1.1.0")
	assert.Same(t, got.Pins[1].Digest, got.Pins[2].Digest, "pins with the same current share a digest")
	assert.Len(t, got.Digests(), 2)

	a := got.Pins[2].Behind
	assert.Equal(t, report.Behind{Releases: 2, Minor: 2, Since: a.Since}, *a)
	assert.Equal(t, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), *a.Since)
	assert.Contains(t, got.Pins[6].Note, `"lts" is a channel`)
	assert.Contains(t, got.Pins[7].Note, "no GitHub repository")
}

func TestOutdatedAuth(t *testing.T) {
	t.Parallel()
	gh, resolve := outdatedFixture()
	gh.recentErr["o/a"] = fmt.Errorf("list recent releases of o/a: %w", github.ErrAuth)
	_, err := report.NewOutdater(report.NewDigester(slog.New(slog.DiscardHandler), gh, nil), gh).Outdated(
		t.Context(),
		report.OutdatedRequest{Pins: []scan.Pin{pin("a", "1.0.0", scan.MatchExact, ".", "f", 1)}, Resolve: resolve},
	)
	require.ErrorIs(t, err, github.ErrAuth, "a rejected token ends the command")
}

func TestParseMaxBehind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want, err string
	}{
		{in: "", want: ""},
		{in: "minor:2", want: "minor:2"},
		{in: " days:90 ", want: "days:90"},
		{in: "releases:0", want: "releases:0"},
		{in: "minor", err: "want <unit>:<n>"},
		{in: "minor:-1", err: "want <unit>:<n>"},
		{in: "weeks:2", err: "want major, minor, patch, releases or days"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := report.ParseMaxBehind(tt.in)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestMaxBehindExceeded(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	since := now.Add(-100 * 24 * time.Hour)
	b := &report.Behind{Releases: 5, Major: 1, Minor: 3, Patch: 2, Since: &since}
	tests := []struct {
		max      string
		b        *report.Behind
		exceeded bool
		ok       bool
	}{
		{max: "", b: b, exceeded: true, ok: true},
		{max: "", b: &report.Behind{}, exceeded: false, ok: true},
		{max: "major:0", b: b, exceeded: true, ok: true},
		{max: "major:1", b: b, exceeded: false, ok: true},
		{max: "minor:2", b: b, exceeded: true, ok: true},
		{max: "minor:3", b: b, exceeded: false, ok: true},
		{max: "patch:2", b: b, exceeded: false, ok: true},
		{max: "releases:4", b: b, exceeded: true, ok: true},
		{max: "days:90", b: b, exceeded: true, ok: true},
		{max: "days:120", b: b, exceeded: false, ok: true},
		{max: "days:1", b: &report.Behind{Releases: 3}, exceeded: false, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.max, func(t *testing.T) {
			t.Parallel()
			m, err := report.ParseMaxBehind(tt.max)
			require.NoError(t, err)
			exceeded, ok := m.Exceeded(tt.b, now)
			assert.Equal(t, tt.exceeded, exceeded)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestGateCheck(t *testing.T) {
	t.Parallel()
	yes := true
	o := &report.Outdated{Pins: []report.OutdatedPin{
		{
			Tool: "a", Status: report.StatusOutdated, Behind: &report.Behind{Releases: 1, Patch: 1},
			Counts: map[classify.Kind]int{
				classify.KindBreaking: 1, classify.KindDeprecation: 2, classify.KindSecurity: 0,
				classify.KindFeature: 0, classify.KindFix: 0, classify.KindOther: 0,
			},
		},
		{Tool: "b", Status: report.StatusOutdated, Behind: &report.Behind{Releases: 4, Minor: 3}},
		{Tool: "c", Status: report.StatusOutdated, Behind: &report.Behind{Releases: 2}},
		{
			Tool: "d", Status: report.StatusCurrent, Behind: &report.Behind{},
			EOL: &enrich.EOL{Current: &enrich.EOLStatus{Cycle: "1.0", IsEOL: &yes}},
			Vulns: []enrich.Vuln{
				{ID: "V-1", AffectsCurrent: true, Severity: &enrich.Score{Score: 9.8, Rating: "critical"}},
				{ID: "V-2", AffectsCurrent: true, Severity: &enrich.Score{Score: 5, Rating: "medium"}},
				{ID: "V-3", AffectsCurrent: true},
				{ID: "V-4", AffectsCurrent: false, Severity: &enrich.Score{Score: 9.8, Rating: "critical"}},
			},
		},
	}}
	ids := func(fs []report.Finding) []string {
		var got []string
		for _, f := range fs {
			s := f.FailOn + ":" + f.Pin.Tool
			if f.Vuln != nil {
				s += ":" + f.Vuln.ID
			}
			got = append(got, s)
		}
		return got
	}
	gate := report.Gate{Values: report.Values(), MaxBehind: report.MaxBehind{Unit: "minor", N: 2}}
	findings, unmeasured := gate.Check(o)
	assert.Equal(t, []string{
		"breaking:a", "deprecation:a", "outdated:b", "eol:d", "vuln:d:V-1", "vuln:d:V-2", "vuln:d:V-3",
	}, ids(findings))
	assert.Empty(t, unmeasured)

	gate.VulnSeverity = "high"
	findings, _ = gate.Check(o)
	assert.Contains(t, ids(findings), "vuln:d:V-1")
	assert.NotContains(t, ids(findings), "vuln:d:V-2")
	assert.NotContains(t, ids(findings), "vuln:d:V-3", "an unrated advisory counts only without a threshold")

	findings, unmeasured = report.Gate{
		Values: []string{report.FailOnOutdated}, MaxBehind: report.MaxBehind{Unit: "days", N: 1},
	}.Check(o)
	assert.Empty(t, findings)
	assert.Len(t, unmeasured, 3, "tags without publish dates can't be measured in days")
}

func TestGateMatches(t *testing.T) {
	t.Parallel()
	fs := []report.Finding{
		{FailOn: report.FailOnVuln},
		{FailOn: report.FailOnOutdated},
		{FailOn: report.FailOnVuln},
		{FailOn: report.FailOnBreaking},
	}
	values := func(ms []report.Match) []string {
		var out []string
		for _, m := range ms {
			out = append(out, fmt.Sprintf("%s:%d", m.FailOn, len(m.Findings)))
		}
		return out
	}
	assert.Equal(t, []string{"breaking:1", "vuln:2", "outdated:1"}, values(report.Gate{}.Matches(fs)),
		"values in schema order; MinCount 0 counts as 1")
	assert.Equal(t, []string{"vuln:2"}, values(report.Gate{MinCount: 2}.Matches(fs)), "each value counts alone")
	assert.Empty(t, report.Gate{MinCount: 3}.Matches(fs))
}

func TestGateCheckDigests(t *testing.T) {
	t.Parallel()
	counts := func(breaking, deprecation int) map[classify.Kind]int {
		return map[classify.Kind]int{
			classify.KindBreaking: breaking, classify.KindDeprecation: deprecation, classify.KindSecurity: 0,
			classify.KindFeature: 0, classify.KindFix: 0, classify.KindOther: 0,
		}
	}
	ds := []report.ToolDigest{
		{Tool: "a", Counts: counts(2, 0)},
		{Tool: "b", Counts: counts(0, 1)},
		{Tool: "c", Counts: counts(0, 0)},
	}
	var got []string
	for _, f := range (report.Gate{Values: report.Values()}).CheckDigests(ds) {
		got = append(got, f.FailOn+":"+f.Digest.Tool)
	}
	assert.Equal(t, []string{"breaking:a", "deprecation:b"}, got)
	assert.Empty(t, report.Gate{Values: []string{report.FailOnEOL}}.CheckDigests(ds))
}

func TestParseVulnSeverity(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"", "low", "medium", "high", "critical"} {
		got, err := report.ParseVulnSeverity(ok)
		require.NoError(t, err)
		assert.Equal(t, ok, got)
	}
	for _, bad := range []string{"none", "HIGH", "severe"} {
		_, err := report.ParseVulnSeverity(bad)
		require.Error(t, err, bad)
	}
}
