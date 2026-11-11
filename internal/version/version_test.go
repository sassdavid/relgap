package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/version"
)

func mustScheme(t *testing.T, name, extract string) version.Scheme {
	t.Helper()
	s, err := version.NewScheme(name, extract)
	require.NoError(t, err)
	return s
}

const temurinExtract = `jdk-(\d+\.\d+\.\d+)\+(\d+)`

// TestSpec_versions_and_ranges_1: table tests for every scheme row, plus Go's go1.21rc2,
// go1.20 < go1.20.1, and Python 3.13.0a1 < 3.13.0.
func TestSpec_versions_and_ranges_1(t *testing.T) {
	t.Parallel()
	t.Run("parse", testParse)
	t.Run("order", testOrder)
	t.Run("lines", testLines)
}

func testParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scheme, extract, in string
		text                string
		pre                 bool
		wantErr             bool
	}{
		{scheme: "semver", in: "1.17.1", text: "1.17.1"},
		{scheme: "semver", in: "v3.6.0-rc.1", text: "3.6.0-rc.1", pre: true},
		{scheme: "semver", in: "not-a-version", wantErr: true},
		{scheme: "semver", in: "1.2.3.4", wantErr: true},
		{scheme: "loose", in: "go1.20", text: "1.20"},
		{scheme: "loose", in: "go1.21rc2", text: "1.21rc2", pre: true},
		{scheme: "loose", in: "2.35.3", text: "2.35.3"},
		{scheme: "loose", in: "21.0.9+10", text: "21.0.9+10"},
		{scheme: "loose", in: "1.2.3.4", text: "1.2.3.4"},
		{scheme: "loose", in: "release-1", wantErr: true},
		{scheme: "calver", in: "2026.11.5", text: "2026.11.5"},
		{scheme: "calver", in: "2026.1", text: "2026.1"},
		{scheme: "calver", in: "v2026.11.5-rc.1", text: "2026.11.5-rc.1", pre: true},
		{scheme: "calver", in: "2026.x", wantErr: true},
		{scheme: "python", in: "3.13.0a1", text: "3.13.0a1", pre: true},
		{scheme: "python", in: "3.12.7rc1", text: "3.12.7rc1", pre: true},
		{scheme: "python", in: "3.12.7", text: "3.12.7"},
		{scheme: "python", in: "1.0.post1", text: "1.0.post1"},
		{scheme: "python", in: "1.0.dev3", text: "1.0.dev3", pre: true},
		{scheme: "python", in: "1!2.0", wantErr: true},
		{scheme: "regex", extract: temurinExtract, in: "jdk-21.0.9+10", text: "21.0.9.10"},
		{scheme: "regex", extract: temurinExtract, in: "jdk-21.0.9", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.scheme+"/"+tt.in, func(t *testing.T) {
			t.Parallel()
			s := mustScheme(t, tt.scheme, tt.extract)
			v, err := s.Parse(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, version.ErrParse)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.text, v.String())
			assert.Equal(t, tt.pre, s.IsPrerelease(v))
		})
	}
}

func testOrder(t *testing.T) {
	t.Parallel()
	// Each list is strictly ascending.
	tests := []struct {
		scheme, extract string
		ordered         []string
	}{
		{
			scheme:  "semver",
			ordered: []string{"1.2.3-alpha", "1.2.3-alpha.1", "1.2.3-beta", "v1.2.3", "1.10.0", "3.22.1", "4.3.0"},
		},
		{scheme: "loose", ordered: []string{"go1.20rc1", "go1.20", "go1.20.1", "go1.21rc2", "go1.21.0", "go1.21.10"}},
		{scheme: "loose", ordered: []string{"2.35.3", "2.35.3.1", "2.35.10"}},
		{
			scheme:  "calver",
			ordered: []string{"2026.1", "2026.1.1", "2026.9.0", "2026.11.5-rc.1", "2026.11.5", "2026.11.10"},
		},
		{
			scheme: "python",
			ordered: []string{
				"3.13.0.dev1",
				"3.13.0a1",
				"3.13.0a2",
				"3.13.0b1",
				"3.13.0rc1",
				"3.13.0",
				"3.13.0.post1",
				"3.13.1",
			},
		},
		{
			scheme:  "regex",
			extract: temurinExtract,
			ordered: []string{"jdk-21.0.8+9", "jdk-21.0.9+9", "jdk-21.0.9+10", "jdk-21.0.10+7"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.scheme+"/"+tt.ordered[0], func(t *testing.T) {
			t.Parallel()
			s := mustScheme(t, tt.scheme, tt.extract)
			vs := make([]version.V, len(tt.ordered))
			for i, in := range tt.ordered {
				v, err := s.Parse(in)
				require.NoError(t, err, in)
				vs[i] = v
			}
			for i := range vs {
				for j := range vs {
					want := 0
					switch {
					case i < j:
						want = -1
					case i > j:
						want = 1
					}
					assert.Equal(t, want, s.Compare(vs[i], vs[j]), "%s vs %s", tt.ordered[i], tt.ordered[j])
				}
			}
		})
	}

	t.Run("equal", func(t *testing.T) {
		t.Parallel()
		equal := []struct{ scheme, a, b string }{
			{"semver", "3.7.12", "v3.7.12"},
			{"loose", "go1.20", "1.20.0"},
			{"loose", "21.0.9+10", "21.0.9+11"},
			{"calver", "2026.1", "2026.1.0"},
			{"python", "3.13.0", "3.13"},
		}
		for _, e := range equal {
			s := mustScheme(t, e.scheme, "")
			a, err := s.Parse(e.a)
			require.NoError(t, err)
			b, err := s.Parse(e.b)
			require.NoError(t, err)
			assert.Zero(t, s.Compare(a, b), "%s: %s vs %s", e.scheme, e.a, e.b)
		}
	})
}

func testLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scheme, in   string
		major, minor string
	}{
		{"semver", "v3.22.1", "3", "3.22"},
		{"loose", "go1.20", "1", "1.20"},
		{"calver", "2026.11.5", "2026", "2026.11"},
		{"python", "3.13.0a1", "3", "3.13"},
	}
	for _, tt := range tests {
		s := mustScheme(t, tt.scheme, "")
		v, err := s.Parse(tt.in)
		require.NoError(t, err)
		assert.Equal(t, tt.major, s.Line(v, version.LineMajor))
		assert.Equal(t, tt.minor, s.Line(v, version.LineMinor))
		assert.Empty(t, s.Line(v, version.LineNone))
	}
}

func TestNewScheme(t *testing.T) {
	t.Parallel()
	_, err := version.NewScheme("nope", "")
	require.ErrorContains(t, err, `unknown version scheme "nope"`)
	_, err = version.NewScheme("semver", "x")
	require.ErrorContains(t, err, "extract is only valid with scheme regex")
	_, err = version.NewScheme("regex", "")
	require.ErrorContains(t, err, "needs an extract")
	_, err = version.NewScheme("regex", "(")
	require.ErrorContains(t, err, "compile extract")
}

func TestParseLevel(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]version.Level{"": version.LineNone, "none": version.LineNone, "major": version.LineMajor, "minor": version.LineMinor} {
		got, err := version.ParseLevel(in)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := version.ParseLevel("patch")
	require.Error(t, err)
}

func TestConstraint(t *testing.T) {
	t.Parallel()
	s := version.Semver()
	tests := []struct {
		expr string
		in   map[string]bool
	}{
		{"~> 1.15", map[string]bool{"1.14.9": false, "1.15.0": true, "1.17.1": true, "2.0.0": false}},
		{"~> 1.15.3", map[string]bool{"1.15.2": false, "1.15.3": true, "1.15.9": true, "1.16.0": false}},
		{"~> 1", map[string]bool{"0.9.0": false, "1.0.0": true, "1.99.0": true, "2.0.0": false}},
		{">= 1.2, < 2", map[string]bool{"1.1.9": false, "1.2.0": true, "1.9.9": true, "2.0.0": false}},
		{"!= 1.5.0", map[string]bool{"1.5.0": false, "1.5.1": true}},
		{"1.5.0", map[string]bool{"1.5.0": true, "1.5.1": false}},
		{"<= 1.5.0, > 1.0.0", map[string]bool{"1.0.0": false, "1.5.0": true}},
	}
	for _, tt := range tests {
		c, err := version.ParseConstraint(s, tt.expr)
		require.NoError(t, err)
		for in, want := range tt.in {
			v, parseErr := s.Parse(in)
			require.NoError(t, parseErr)
			assert.Equal(t, want, c.Check(v), "%s %s", in, tt.expr)
		}
	}
	_, err := version.ParseConstraint(s, ">= 1.0,")
	require.ErrorContains(t, err, "empty term")
	_, err = version.ParseConstraint(s, "~> banana")
	require.ErrorIs(t, err, version.ErrParse)
}

func TestConstraintFloor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scheme version.Scheme
		expr   string
		want   string // empty when there is no floor
	}{
		{version.Semver(), "~> 1.15", "1.15.0"},
		{version.Semver(), "~> 1.15.3", "1.15.3"},
		{version.Semver(), ">= 1.2, < 2", "1.2.0"},
		{version.Semver(), ">= 1.2, >= 1.4.1", "1.4.1"},
		{version.Semver(), "1.5.0", "1.5.0"},
		{version.Semver(), "< 2", ""},
		{version.Semver(), "> 1.0.0", ""},
		{version.Semver(), ">= 1.5, > 1.5.0", ""},
		{version.Semver(), ">= 1.5, != 1.5.0", ""},
		{version.Semver(), ">= 1.5.0-rc.1", "1.5.0-rc.1"},
		{version.Loose(), ">= 26", "26.0.0"},
	}
	for _, tt := range tests {
		c, err := version.ParseConstraint(tt.scheme, tt.expr)
		require.NoError(t, err)
		v, ok := c.Floor()
		assert.Equal(t, tt.want != "", ok, tt.expr)
		if ok {
			assert.Equal(t, tt.want, v.String(), tt.expr)
		}
	}
}
