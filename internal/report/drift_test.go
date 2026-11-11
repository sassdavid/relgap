package report_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
)

func driftPins() []scan.Pin {
	return []scan.Pin{
		pin("a", "1.0.0", scan.MatchExact, "r1", "mise.toml", 1),
		pin("a", "~> 1.0", scan.MatchConstraint, "r1", "versions.tf", 2), // counts as r1's 1.0.0
		pin("a", "1.0", scan.MatchExact, "r2", "mise.toml", 1),           // the same version as 1.0.0
		pin("a", ">= 1.1", scan.MatchConstraint, "r3", "versions.tf", 2), // counts as its floor
		pin("a", "< 2", scan.MatchConstraint, "r4", "versions.tf", 2),    // no floor: not compared
		pin("a", "1", scan.MatchPrefix, "r4", "mise.toml", 1),
		pin("b", "0.4.0", scan.MatchExact, "r1", "mise.toml", 3),
		pin("b", "0.5.0", scan.MatchExact, "r2", "mise.toml", 3), // a tie with 0.4.0: newest wins
		pin("c", "latest", scan.MatchChannel, "r1", "mise.toml", 4),
	}
}

func TestDrift(t *testing.T) {
	t.Parallel()
	_, resolve := outdatedFixture()
	d, err := report.NewDrifter(slog.New(slog.DiscardHandler), nil).Drift(t.Context(), report.DriftRequest{
		Repos: []string{"r1", "r2", "r3", "r4"}, Pins: driftPins(), Resolve: resolve,
	})
	require.NoError(t, err)
	require.Len(t, d.Tools, 3)

	a := d.Tools[0]
	assert.True(t, a.Drift)
	assert.Equal(t, "1.0.0", a.Reference)
	assert.Equal(t, []report.DriftVersion{
		{Version: "1.0.0", Repos: []string{"r1", "r2"}},
		{Version: "1.1.0", Repos: []string{"r3"}},
	}, a.Versions)
	vias := map[string]string{}
	for _, p := range a.Pins {
		vias[p.Value] = p.Via + "|" + p.Version
	}
	assert.Equal(t, map[string]string{
		"1.0.0":  "|1.0.0",
		"~> 1.0": "highest installed or locked version|1.0.0",
		"1.0":    "|1.0.0",
		">= 1.1": "lower bound of the constraint|1.1.0",
		"< 2":    "|",
		"1":      "|",
	}, vias)
	assert.Equal(t, report.Cell{Values: []report.CellValue{{Text: "1.0.0"}}}, a.Cell("r1"))
	assert.Equal(t, report.Cell{Values: []report.CellValue{{Text: "1.1.0", Approx: true, Outlier: true}}}, a.Cell("r3"))
	assert.Equal(t, report.Cell{Values: []report.CellValue{{Text: "1", Approx: true}, {Text: "< 2", Approx: true}}},
		a.Cell("r4"))
	assert.Empty(t, a.Cell("r9").Values)

	b, c := d.Tools[1], d.Tools[2]
	assert.Equal(t, "0.5.0", b.Reference)
	assert.True(t, b.Drift)
	assert.False(t, c.Drift)
	assert.Empty(t, c.Versions)
	assert.Empty(t, c.Reference)

	assert.Equal(t, []string{"a", "b"}, toolsOf(d.Drifting()))
	assert.Equal(t, []string{"a", "b"}, toolsOf(report.Gate{Drift: true}.CheckDrift(d)))
	assert.Empty(t, report.Gate{}.CheckDrift(d))
}

func TestDriftIgnoreConstraints(t *testing.T) {
	t.Parallel()
	_, resolve := outdatedFixture()
	d, err := report.NewDrifter(slog.New(slog.DiscardHandler), nil).Drift(t.Context(), report.DriftRequest{
		Pins: driftPins(), Resolve: resolve, IgnoreConstraints: true,
	})
	require.NoError(t, err)
	a := d.Tools[0]
	assert.False(t, a.Drift, "only 1.0.0 is left")
	assert.Equal(t, report.Cell{Values: []report.CellValue{{Text: "1.0.0"}, {Text: "~> 1.0", Approx: true}}},
		a.Cell("r1"))
}

func TestDriftLatest(t *testing.T) {
	t.Parallel()
	gh, resolve := outdatedFixture()
	pins := []scan.Pin{
		pin("a", "1.0.0", scan.MatchExact, "r1", "mise.toml", 1),
		pin("b", "2.9.0", scan.MatchExact, "r1", "mise.toml", 2),
		pin("e", "1.0.0", scan.MatchExact, "r1", "mise.toml", 3),
		pin("x", "1.0.0", scan.MatchExact, "r1", "mise.toml", 4),
	}
	d, err := report.NewDrifter(slog.New(slog.DiscardHandler), gh).Drift(t.Context(), report.DriftRequest{
		Pins: pins, Resolve: resolve, Latest: true,
	})
	require.NoError(t, err)
	require.Len(t, gh.recentQs, 1, "one batched lookup")
	assert.Len(t, gh.recentQs[0], 3, "x has no repository")
	got := map[string]string{}
	for _, tool := range d.Tools {
		got[tool.Tool] = tool.Latest + "|" + tool.Note
	}
	assert.Equal(t, "1.2.0|", got["a"], "the prerelease is skipped")
	assert.Equal(t, "3.0.0|", got["b"])
	assert.Contains(t, got["e"], "list releases of o/e")
	assert.Contains(t, got["x"], "not looked up: no GitHub repository")
	require.Error(t, d.Tools[2].Err)

	gh.recentErr = map[string]error{"o/a": github.ErrAuth}
	_, err = report.NewDrifter(slog.New(slog.DiscardHandler), gh).Drift(t.Context(), report.DriftRequest{
		Pins: pins[:1], Resolve: resolve, Latest: true,
	})
	require.ErrorIs(t, err, github.ErrAuth)
}

func toolsOf(ts []report.DriftTool) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.Tool)
	}
	return out
}
