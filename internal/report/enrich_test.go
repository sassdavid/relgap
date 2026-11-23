package report_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// TestEnrichRequestsUnchecked: a pin is checked at Current, an exact pin whose lookup failed at
// its own version, and pins of one tool and version share a request. A channel other than
// latest is not-exact, a tool that doesn't resolve is unresolved, and the unchecked list groups
// pins by tool, version and reason, with unresolved scan pins last.
func TestEnrichRequestsUnchecked(t *testing.T) {
	t.Parallel()
	loc := func(file string, line int) scan.Location { return scan.Location{Repo: ".", File: file, Line: line} }
	exact := func(tool, v string, l scan.Location) scan.Pin {
		return scan.Pin{Tool: tool, Value: v, Version: v, Exact: true, Match: scan.MatchExact, Location: l}
	}
	o := &report.Outdated{Pins: []report.OutdatedPin{
		{
			Tool: "k9s", Pin: exact("k9s", "0.50.16", loc("mise.toml", 2)), Current: "0.50.16", Latest: "0.51.0",
			Status: report.StatusOutdated,
		},
		{Tool: "k9s", Pin: exact("k9s", "0.50.16", loc("Dockerfile", 3)), Status: report.StatusError},
		{Tool: "node", Pin: scan.Pin{
			Tool: "node", Value: "lts", Version: "lts", Match: scan.MatchChannel,
			Location: loc("mise.toml", 3),
		}, Status: report.StatusUnknown},
		{Tool: "node", Pin: scan.Pin{
			Tool: "node", Value: "lts", Version: "lts", Match: scan.MatchChannel,
			Location: loc(".nvmrc", 1),
		}, Status: report.StatusError},
		{Tool: "ghost", Pin: exact("ghost", "1.0.0", loc("mise.toml", 4)), Status: report.StatusError},
		{
			Tool: "image:alpine", Pin: exact("image:alpine", "3.22", loc("Dockerfile", 1)), Current: "3.22",
			Status: report.StatusCurrent, Latest: "3.22",
		},
	}}
	specs := map[string]registry.ToolSpec{
		"k9s": {
			Name:    "k9s",
			Repo:    "derailed/k9s",
			Version: registry.VersionSpec{Scheme: version.SchemeSemver},
		},
		"node": {
			Name:    "node",
			Repo:    "nodejs/node",
			Version: registry.VersionSpec{Scheme: version.SchemeSemver},
		},
		"image:alpine": {
			Name:    "image:alpine",
			Image:   "alpine",
			Version: registry.VersionSpec{Scheme: version.SchemeLoose},
		},
	}
	resolve := func(tool string) (registry.ToolSpec, error) {
		if s, ok := specs[tool]; ok {
			return s, nil
		}
		return registry.ToolSpec{}, fmt.Errorf("%w: %s", registry.ErrUnknownTool, tool)
	}

	reqs := o.EnrichRequests(resolve, nil)
	require.Len(t, reqs, 2, "k9s pins share one request; alpine has its own")
	assert.Equal(t, "k9s", reqs[0].Name)
	assert.Equal(t, "0.50.16", reqs[0].Current.String())
	assert.Equal(t, "0.51.0", reqs[0].Target.String())
	assert.Equal(t, "image:alpine", reqs[1].Name)

	results := map[string]enrich.Result{
		reqs[0].Key: {Vulns: []enrich.Vuln{}},
		reqs[1].Key: {Unchecked: enrich.ReasonNoSource},
	}
	frob := scan.Pin{RawName: "FROBNICATOR_VERSION", Value: "2.0.1", Location: loc(".github/workflows/ci.yml", 7)}
	o.ApplyEnrichment(results, []scan.Pin{frob})

	for _, i := range []int{0, 1} {
		assert.NotNil(t, o.Pins[i].Vulns, "k9s at %s is checked", o.Pins[i].Pin.Location.File)
		assert.Empty(t, o.Pins[i].Unchecked)
	}
	assert.Equal(t, enrich.ReasonNotExact, o.Pins[2].Unchecked)
	assert.Equal(
		t,
		enrich.ReasonNotExact,
		o.Pins[3].Unchecked,
		"lts never resolves, so a failed lookup changes nothing",
	)
	assert.Equal(t, enrich.ReasonUnresolved, o.Pins[4].Unchecked)
	assert.Equal(t, enrich.ReasonNoSource, o.Pins[5].Unchecked)
	assert.Nil(t, o.Pins[5].Vulns)

	assert.Equal(t, []report.Unchecked{
		{
			Tool: "node", Version: "lts", Reason: enrich.ReasonNotExact,
			Pins: []scan.Location{loc("mise.toml", 3), loc(".nvmrc", 1)},
		},
		{Tool: "ghost", Version: "1.0.0", Reason: enrich.ReasonUnresolved, Pins: []scan.Location{loc("mise.toml", 4)}},
		{
			Tool:    "image:alpine",
			Version: "3.22",
			Reason:  enrich.ReasonNoSource,
			Pins:    []scan.Location{loc("Dockerfile", 1)},
		},
		{
			Tool: "FROBNICATOR_VERSION", Version: "2.0.1", Reason: enrich.ReasonUnresolved,
			Pins: []scan.Location{frob.Location},
		},
	}, o.Unchecked)
}
