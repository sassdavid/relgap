package scan_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/scan"
)

// pinLine is the part of a pin the inventory tests compare.
type pinLine struct {
	repo, file, tool, value string
}

func TestTFCensusInventory(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../testdata/tfcensus/tf-fleet.json")
	require.NoError(t, err)
	res, err := scan.TFCensusInventory(data)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"tf-fleet/app", "tf-fleet/edge", "tf-fleet/edge:staging", "tf-fleet/legacy", "tf-fleet/network",
		"tf-fleet/planned", "tf-fleet/restricted", "tf-fleet/secure", "tf-fleet/shared",
	}, res.Repos)
	got := make([]pinLine, 0, len(res.Pins))
	for _, p := range res.Pins {
		assert.Equal(t, scan.DetectorTFCensus, p.Location.Detector)
		assert.Zero(t, p.Location.Line)
		assert.True(t, p.Exact, "%s in %s", p.Tool, p.Location.Repo)
		got = append(got, pinLine{p.Location.Repo, p.Location.File, p.Tool, p.Value})
	}
	assert.Equal(t, []pinLine{
		{"tf-fleet/app", ".terraform.lock.hcl", "tfprovider:hashicorp/aws", "6.20.0"},
		{"tf-fleet/app", ".terraform.lock.hcl", "tfprovider:hashicorp/random", "3.7.2"},
		{"tf-fleet/app", "terraform.tfstate", "terraform", "1.12.2"},
		{"tf-fleet/edge", "terraform.tfstate", "opentofu", "1.10.6"},
		{"tf-fleet/edge:staging", "terraform.tfstate", "opentofu", "1.10.6"},
		{"tf-fleet/legacy", "terraform.tfstate", "terraform", "0.11.14"},
		{"tf-fleet/network", "terraform.tfstate", "terraform", "1.5.7"},
		{"tf-fleet/shared", "terraform.tfstate", "terraform", "1.11.4"},
	}, got)
	require.Len(t, res.Problems, 1)
	assert.Equal(t, "tf-fleet/restricted", res.Problems[0].Repo)
	assert.Contains(t, res.Problems[0].Message, "tfcensus: access denied: s3:GetObject")
}

func TestTFCensusInventoryUnits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, unit string
		want       []pinLine
	}{
		{
			name: "root unit, other host, no lock file version",
			unit: `{"repo":"infra","path":".","workspace":"default","key":"prod/net.tfstate",
				"engine":"terraform","terraform_version":"v1.9.0",
				"providers":[{"source":"example.com/acme/thing","version":"2.0.0","version_source":"lockfile"},
				{"source":"hashicorp/null"}]}`,
			want: []pinLine{
				{"infra", ".terraform.lock.hcl", "tfprovider:example.com/acme/thing", "2.0.0"},
				{"infra", "net.tfstate", "terraform", "v1.9.0"},
			},
		},
		{
			name: "no key and an unknown field",
			unit: `{"repo":"r","path":"a/b","workspace":"prod","terraform_version":"1.15.8","future":1}`,
			want: []pinLine{{"r/a/b:prod", "terraform.tfstate", "terraform", "1.15.8"}},
		},
		{
			name: "absent state",
			unit: `{"repo":"r","path":"a","workspace":"default","status":"absent","providers":[]}`,
			want: []pinLine{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := scan.TFCensusInventory([]byte(
				`{"schema_version":"1","tool":"tfcensus","version":"0.2.0","data":{"units":[` + tt.unit +
					`],"double_managed":[]},"errors":[]}`))
			require.NoError(t, err)
			got := []pinLine{}
			for _, p := range res.Pins {
				got = append(got, pinLine{p.Location.Repo, p.Location.File, p.Tool, p.Value})
			}
			assert.Equal(t, tt.want, got)
			assert.Len(t, res.Repos, 1)
		})
	}
}

func TestTFCensusInventoryRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, data, want string
	}{
		{"not JSON", `[`, "unexpected end of JSON input"},
		{
			"another tool",
			`{"schema_version":"1","tool":"relgap","data":{"units":[]}}`,
			`tool is "relgap", want "tfcensus"`,
		},
		{"no tool", `{"schema_version":"1","data":{"units":[]}}`, `tool is "", want "tfcensus"`},
		{
			"newer schema", `{"schema_version":"2","tool":"tfcensus","data":{"units":[]}}`,
			`schema_version "2" isn't supported; relgap reads "1"`,
		},
		{
			"number", `{"schema_version":1,"tool":"tfcensus","data":{"units":[]}}`,
			`schema_version 1 isn't supported; relgap reads "1"`,
		},
		{"no schema", `{"tool":"tfcensus","data":{"units":[]}}`, "schema_version is missing"},
		{"no units", `{"schema_version":"1","tool":"tfcensus","data":{}}`, "data.units is missing"},
		{"no data", `{"schema_version":"1","tool":"tfcensus"}`, "data.units is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := scan.TFCensusInventory([]byte(tt.data))
			require.ErrorIs(t, err, scan.ErrTFCensusInventory)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestResultMerge(t *testing.T) {
	t.Parallel()
	pin := func(repo, file string) scan.Pin {
		return scan.Pin{Tool: "terraform", Location: scan.Location{Repo: repo, File: file}}
	}
	r := &scan.Result{Repos: []string{"b", "c"}, Pins: []scan.Pin{pin("b", "x"), pin("c", "x")}}
	r.Merge(&scan.Result{
		Repos:    []string{"a", "b"},
		Pins:     []scan.Pin{pin("a", "x"), pin("b", "w")},
		Problems: []scan.Problem{{Repo: "a", Message: "m"}},
	})
	assert.Equal(t, []string{"a", "b", "c"}, r.Repos)
	assert.Equal(t, []scan.Pin{pin("a", "x"), pin("b", "w"), pin("b", "x"), pin("c", "x")}, r.Pins)
	assert.Equal(t, []scan.Problem{{Repo: "a", Message: "m"}}, r.Problems)
}
