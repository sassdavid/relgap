package enrich_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

func TestGoSemver(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"go1.24":                    "1.24.0",
		"go1.24.3":                  "1.24.3",
		"1.27.4":                    "1.27.4",
		"go1.25rc1":                 "1.25.0-rc.1",
		"go1.21beta2":               "1.21.0-beta.2",
		"go1.24.3 X:boringcrypto":   "1.24.3",
		"v1.2.3":                    "1.2.3",
		"devel go1.26-abcdef":       "devel",
		"v0.0.0-20261112-abc+dirty": "0.0.0-20261112-abc+dirty",
	} {
		assert.Equal(t, want, enrich.GoSemver(in), in)
	}
}

func record(eco, name string, rg ...enrich.Range) *enrich.Record {
	return &enrich.Record{
		ID:       "X",
		Affected: []enrich.Affected{{Package: enrich.Package{Ecosystem: eco, Name: name}, Ranges: rg}},
	}
}

func events(es ...enrich.Event) []enrich.Event { return es }

func TestAffects(t *testing.T) {
	t.Parallel()
	goPkg := enrich.Package{Ecosystem: "Go", Name: "stdlib"}
	stdlib := record("Go", "stdlib", enrich.Range{Type: "SEMVER", Events: events(
		enrich.Event{Introduced: "0"}, enrich.Event{Fixed: "1.23.8"},
		enrich.Event{Introduced: "1.24.0-0"}, enrich.Event{Fixed: "1.24.2"},
	)})
	py := record("PyPI", "uv", enrich.Range{Type: "ECOSYSTEM", Events: events(
		enrich.Event{Introduced: "0.4.0"}, enrich.Event{LastAffected: "0.4.9"},
	)})
	pyPkg := enrich.Package{Ecosystem: "PyPI", Name: "uv"}
	git := &enrich.Record{ID: "CVE", Affected: []enrich.Affected{
		{
			Ranges: []enrich.Range{
				{
					Type:   "GIT",
					Repo:   "https://github.com/python/cpython",
					Events: events(enrich.Event{Introduced: "0"}, enrich.Event{Fixed: "abc123"}),
				},
			},
			Versions: []string{"v3.13.5", "v3.14.0"},
		},
	}}
	gitPkg := enrich.Package{Ecosystem: "GIT", Name: "https://github.com/python/cpython.git"}
	gitOnly := &enrich.Record{ID: "CVE2", Affected: []enrich.Affected{
		{
			Ranges: []enrich.Range{
				{Type: "GIT", Repo: "https://github.com/nodejs/node", Events: events(enrich.Event{Introduced: "0"})},
			},
		},
	}}
	nodePkg := enrich.Package{Ecosystem: "GIT", Name: "https://github.com/nodejs/node"}
	tests := []struct {
		name      string
		rec       *enrich.Record
		pkg       enrich.Package
		v         string
		hit, sure bool
	}{
		{"go below first fix", stdlib, goPkg, "1.23.7", true, true},
		{"go at first fix", stdlib, goPkg, "1.23.8", false, true},
		{"go between ranges", stdlib, goPkg, "1.23.9", false, true},
		{"go prerelease of next", stdlib, goPkg, "1.24.0-rc.1", true, true},
		{"go in second range", stdlib, goPkg, "1.24.1", true, true},
		{"go fixed", stdlib, goPkg, "1.24.2", false, true},
		{"other package", stdlib, enrich.Package{Ecosystem: "Go", Name: "toolchain"}, "1.20.0", false, true},
		{"pypi last affected", py, pyPkg, "0.4.9", true, true},
		{"pypi after last affected", py, pyPkg, "0.4.10", false, true},
		{"pypi before introduced", py, pyPkg, "0.3.9", false, true},
		{"git listed tag", git, gitPkg, "v3.13.5", true, true},
		{"git unlisted tag", git, gitPkg, "v3.15.2", false, true},
		{"git without versions", gitOnly, nodePkg, "v24.0.0", false, false},
		{"unparsable version", stdlib, goPkg, "not-a-version", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hit, sure := enrich.Affects(tt.rec, tt.pkg, tt.v)
			assert.Equal(t, tt.hit, hit, "affected")
			assert.Equal(t, tt.sure, sure, "known")
		})
	}
	assert.Equal(t, "1.24.2", enrich.FixedIn(stdlib, goPkg, "1.24.1"))
	assert.Equal(t, "1.23.8", enrich.FixedIn(stdlib, goPkg, "1.22.0"))
	assert.Empty(t, enrich.FixedIn(stdlib, goPkg, "1.25.0"))
}

func TestPackagesFor(t *testing.T) {
	t.Parallel()
	argo := []registry.OSVPackage{
		{Ecosystem: "Go", Name: "github.com/argoproj/argo-cd/v3"},
		{Ecosystem: "Go", Name: "github.com/argoproj/argo-cd/v2"},
		{Ecosystem: "Go", Name: "github.com/argoproj/argo-cd"},
	}
	names := func(ps []registry.OSVPackage) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return out
	}
	v := func(s string) version.V {
		x, err := version.Semver().Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	assert.Equal(
		t,
		[]string{"github.com/argoproj/argo-cd/v3"},
		names(enrich.PackagesFor(argo, version.Semver(), v("3.5.0"))),
	)
	assert.Equal(
		t,
		[]string{"github.com/argoproj/argo-cd/v2"},
		names(enrich.PackagesFor(argo, version.Semver(), v("2.14.4"))),
	)
	assert.Equal(
		t,
		[]string{"github.com/argoproj/argo-cd"},
		names(enrich.PackagesFor(argo, version.Semver(), v("1.8.7"))),
	)
	stdlib := []registry.OSVPackage{{Ecosystem: "Go", Name: "stdlib"}, {Ecosystem: "npm", Name: "x"}}
	assert.Equal(t, []string{"stdlib", "x"}, names(enrich.PackagesFor(stdlib, version.Semver(), v("1.27.4"))))
}

func TestQueryVersion(t *testing.T) {
	t.Parallel()
	v, err := version.Loose().Parse("1.27")
	require.NoError(t, err)
	assert.Equal(t, "1.27.0", enrich.QueryVersion(registry.OSVPackage{Ecosystem: "Go", Name: "stdlib"}, "go1.27", v))
	assert.Equal(
		t,
		"go1.27",
		enrich.QueryVersion(registry.OSVPackage{Ecosystem: "GIT", Name: "https://github.com/golang/go"}, "go1.27", v),
	)
	assert.Equal(t, "1.27", enrich.QueryVersion(registry.OSVPackage{Ecosystem: "PyPI", Name: "x"}, "v1.27", v))
}

func TestRating(t *testing.T) {
	t.Parallel()
	for score, want := range map[float64]string{0: "none", 0.1: "low", 3.9: "low", 4: "medium", 6.9: "medium", 7: "high", 8.9: "high", 9: "critical", 10: "critical"} {
		assert.Equal(t, want, enrich.Rating(score), score)
	}
}

func TestScore(t *testing.T) {
	t.Parallel()
	pkg := enrich.Package{Ecosystem: "npm", Name: "x"}
	rec := func(sev []enrich.Severity, label string) *enrich.Record {
		r := &enrich.Record{Severity: sev, Affected: []enrich.Affected{{Package: pkg}}}
		r.DatabaseSpecific.Severity = label
		return r
	}
	tests := []struct {
		name string
		rec  *enrich.Record
		want *enrich.Score
	}{
		{
			"v3.1",
			rec([]enrich.Severity{{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}}, ""),
			&enrich.Score{Score: 9.8, Rating: "critical", Vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
		},
		{"highest of v3.1 and v4.0", rec([]enrich.Severity{
			{Type: "CVSS_V3", Score: "CVSS:3.1/AV:L/AC:H/PR:H/UI:R/S:U/C:L/I:N/A:N"},
			{Type: "CVSS_V4", Score: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"},
		}, "LOW"), &enrich.Score{Score: 9.3, Rating: "critical", Vector: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"}},
		{
			"v2", rec([]enrich.Severity{{Type: "CVSS_V2", Score: "AV:N/AC:L/Au:N/C:P/I:P/A:P"}}, ""),
			&enrich.Score{Score: 7.5, Rating: "high", Vector: "AV:N/AC:L/Au:N/C:P/I:P/A:P"},
		},
		{"label only", rec(nil, "MODERATE"), &enrich.Score{Rating: "medium"}},
		{
			"bad vector falls back to the label",
			rec([]enrich.Severity{{Type: "CVSS_V3", Score: "CVSS:3.1/nope"}}, "HIGH"),
			&enrich.Score{Rating: "high"},
		},
		{"nothing", rec(nil, ""), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.rec.Score(pkg))
		})
	}
}
