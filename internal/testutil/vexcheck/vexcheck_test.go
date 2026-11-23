package vexcheck_test

import (
	"testing"

	"github.com/sassdavid/relgap/internal/testutil/vexcheck"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	const head = `{"@context":"https://openvex.dev/ns/v0.2.0","@id":"https://openvex.dev/docs/public/vex-1",` +
		`"author":"Unknown Author","timestamp":"2026-11-23T12:00:00Z","version":1,"statements":`
	tests := []struct {
		name, doc string
		valid     bool
	}{
		{
			name: "affected with an action statement",
			doc: head + `[{"vulnerability":{"name":"GHSA-cxww-7g56-2vh6"},` +
				`"products":[{"@id":"pkg:githubactions/actions/download-artifact@v4.1.2"}],` +
				`"status":"affected","action_statement":"upgrade to 4.1.3"}]}`,
			valid: true,
		},
		{
			name: "not_affected with an impact statement",
			doc: head + `[{"vulnerability":{"name":"GO-2024-3080"},"products":[{"@id":"pkg:golang/stdlib@v1.22.0"}],` +
				`"status":"not_affected","impact_statement":"not reachable"}]}`,
			valid: true,
		},
		{
			name: "affected without an action statement",
			doc:  head + `[{"vulnerability":{"name":"GO-2024-3080"},"status":"affected"}]}`,
		},
		{
			name: "not_affected without a reason",
			doc:  head + `[{"vulnerability":{"name":"GO-2024-3080"},"status":"not_affected"}]}`,
		},
		{
			name: "unknown justification",
			doc: head + `[{"vulnerability":{"name":"GO-2024-3080"},"status":"not_affected",` +
				`"justification":"accepted"}]}`,
		},
		{name: "no statements", doc: head + `[]}`},
		{name: "document id that isn't an IRI", doc: `{"@id":"not an iri"}`},
		{name: "not JSON", doc: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := vexcheck.Validate([]byte(tt.doc))
			if tt.valid && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
		})
	}
}
