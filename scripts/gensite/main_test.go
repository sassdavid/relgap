package main

import (
	"testing"
)

func TestBuildStrata(t *testing.T) {
	t.Parallel()
	report := []byte(`{"data": {"tools": [{
		"tool": "kubectl", "from": "1.30.0", "to": "1.32.0",
		"versions": [{"version": "1.31.0"}, {"version": "1.32.0"}],
		"entries": [
			{"version": "1.32.0", "kind": "breaking"},
			{"version": "1.32.0", "kind": "fix"},
			{"version": "1.31.0", "kind": "fix"},
			{"version": "1.31.0", "kind": "feature"}
		]
	}]}}`)
	got, err := buildStrata(report)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tool != "kubectl" || got.From != "1.30.0" || got.To != "1.32.0" {
		t.Errorf("tool %q range %s..%s", got.Tool, got.From, got.To)
	}
	if len(got.Layers) != 2 || got.Layers[0].Version != "1.31.0" || got.Layers[1].Version != "1.32.0" {
		t.Fatalf("layers %+v, want 1.31.0 then 1.32.0", got.Layers)
	}
	if got.Layers[1].Total != 2 || got.Layers[1].Counts["breaking"] != 1 || got.Layers[0].Counts["security"] != 0 {
		t.Errorf("counts %+v", got.Layers)
	}
	if len(got.Breaking) != 1 || got.Breaking[0] != "1.32.0" {
		t.Errorf("breaking %v, want [1.32.0]", got.Breaking)
	}
}

func TestBuildStrataRejects(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"no tools":      `{"data": {"tools": []}}`,
		"outside range": `{"data": {"tools": [{"versions": [{"version": "1.0.0"}], "entries": [{"version": "2.0.0", "kind": "fix"}]}]}}`,
		"unknown class": `{"data": {"tools": [{"versions": [{"version": "1.0.0"}], "entries": [{"version": "1.0.0", "kind": "chore"}]}]}}`,
	}
	for name, report := range tests {
		if _, err := buildStrata([]byte(report)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
