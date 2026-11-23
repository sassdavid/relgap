package vex_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/purl"
	"github.com/sassdavid/relgap/internal/testutil/vexcheck"
	"github.com/sassdavid/relgap/internal/vex"
)

func today() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }

func subject(t *testing.T, tool string, purls ...string) enrich.Subject {
	t.Helper()
	s := enrich.Subject{Tool: tool, Version: "1.37.2"}
	for _, p := range purls {
		pu, err := purl.Parse(p)
		require.NoError(t, err)
		s.PURLs = append(s.PURLs, pu)
	}
	return s
}

func at(day int) *time.Time {
	t := time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC)
	return &t
}

const kubePURL = "pkg:golang/k8s.io/kubernetes@v1.37.2"

func kubeNames() []string { return []string{"GO-2025-3547", "CVE-2024-7598", "GHSA-r56h-j38w-hrqq"} }

func TestIgnore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ignore vex.Ignore
		tool   string
		want   bool
	}{
		{name: "by ID", ignore: vex.Ignore{ID: "GO-2025-3547"}, tool: "kubectl", want: true},
		{name: "by alias, any case", ignore: vex.Ignore{ID: "ghsa-r56h-j38w-hrqq"}, tool: "kubectl", want: true},
		{name: "other advisory", ignore: vex.Ignore{ID: "GO-2025-3521"}, tool: "kubectl"},
		{
			name:   "tool glob",
			ignore: vex.Ignore{ID: "GO-2025-3547", Tools: []string{"kube*"}},
			tool:   "kubectl",
			want:   true,
		},
		{
			name:   "glob spans slashes",
			ignore: vex.Ignore{ID: "GO-2025-3547", Tools: []string{"action:actions/*"}},
			tool:   "action:actions/download-artifact/merge", want: true,
		},
		{name: "other tool", ignore: vex.Ignore{ID: "GO-2025-3547", Tools: []string{"helm"}}, tool: "kubectl"},
		{name: "last day", ignore: vex.Ignore{ID: "GO-2025-3547", Until: "2026-11-23"}, tool: "kubectl", want: true},
		{name: "expired", ignore: vex.Ignore{ID: "GO-2025-3547", Until: "2026-11-22"}, tool: "kubectl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.ignore.Reason = "accepted"
			r, err := vex.New([]vex.Ignore{tt.ignore}, nil, today())
			require.NoError(t, err)
			s := subject(t, tt.tool, kubePURL)
			assert.Equal(t, tt.want, r.Covers(s, kubeNames()))
			inEffect := len(r.Expired()) == 0
			assert.Equal(t, inEffect, len(r.Unused()) == 1, "Covers doesn't count a use")
			got := r.Suppress(s, kubeNames())
			if !tt.want {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, &enrich.Suppression{Kind: "ignore", Reason: "accepted", Until: tt.ignore.Until}, got)
			assert.Empty(t, r.Unused())
		})
	}
}

// TestExpiredAndUnused: an entry past its until date is listed as expired and never as unused;
// an entry in effect that matched nothing is unused.
func TestExpiredAndUnused(t *testing.T) {
	t.Parallel()
	r, err := vex.New([]vex.Ignore{
		{ID: "GO-2025-3547", Reason: "a", Until: "2026-11-01"},
		{ID: "GO-2025-3521", Reason: "b"},
		{ID: "CVE-2024-7598", Reason: "c", Until: "2027-01-31"},
	}, nil, today())
	require.NoError(t, err)
	require.NotNil(t, r.Suppress(subject(t, "kubectl"), kubeNames()))
	assert.Equal(t, []vex.Ignore{{ID: "GO-2025-3547", Reason: "a", Until: "2026-11-01"}}, r.Expired())
	assert.Equal(t, []vex.Ignore{{ID: "GO-2025-3521", Reason: "b"}}, r.Unused())

	_, err = vex.New([]vex.Ignore{{ID: "X", Reason: "r", Until: "31/12/2026"}}, nil, today())
	assert.ErrorContains(t, err, "isn't a date")
}

func TestStatements(t *testing.T) {
	t.Parallel()
	notAffected := vex.Statement{
		Vulnerability: vex.Vulnerability{Name: "CVE-2024-7598"},
		Products:      []vex.Component{{ID: kubePURL}},
		Status:        vex.StatusNotAffected, Justification: "vulnerable_code_not_in_execute_path",
	}
	with := func(s vex.Statement, edit func(*vex.Statement)) vex.Statement {
		edit(&s)
		return s
	}
	tests := []struct {
		name       string
		statements []vex.Statement
		want       *enrich.Suppression
	}{
		{
			name:       "not_affected by alias",
			statements: []vex.Statement{notAffected},
			want: &enrich.Suppression{
				Kind: "vex", Reason: "vulnerable_code_not_in_execute_path", Source: "k.json",
				Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path",
			},
		},
		{
			name: "impact statement is the reason",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Justification, s.ImpactStatement = "", "the API server isn't exposed"
			})},
			want: &enrich.Suppression{
				Kind: "vex", Reason: "the API server isn't exposed", Source: "k.json", Status: "not_affected",
			},
		},
		{
			name: "fixed",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Status, s.Justification = vex.StatusFixed, ""
			})},
			want: &enrich.Suppression{Kind: "vex", Reason: "fixed", Source: "k.json", Status: "fixed"},
		},
		{
			name: "versionless product in identifiers",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Products = []vex.Component{
					{ID: "urn:x", Identifiers: map[string]string{"purl": "pkg:golang/k8s.io/kubernetes"}},
				}
			})},
			want: &enrich.Suppression{
				Kind: "vex", Reason: "vulnerable_code_not_in_execute_path", Source: "k.json",
				Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path",
			},
		},
		{
			name: "subcomponent",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Products = []vex.Component{
					{ID: "pkg:github/acme/infra", Subcomponents: []vex.Component{{ID: kubePURL}}},
				}
			})},
			want: &enrich.Suppression{
				Kind: "vex", Reason: "vulnerable_code_not_in_execute_path", Source: "k.json",
				Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path",
			},
		},
		{
			name: "other version",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Products = []vex.Component{{ID: "pkg:golang/k8s.io/kubernetes@v1.37.1"}}
			})},
		},
		{
			name:       "no products",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) { s.Products = nil })},
		},
		{
			name: "under_investigation",
			statements: []vex.Statement{with(notAffected, func(s *vex.Statement) {
				s.Status, s.Justification = vex.StatusUnderInvestigation, ""
			})},
		},
		{
			name: "a later affected statement wins",
			statements: []vex.Statement{
				with(notAffected, func(s *vex.Statement) { s.Timestamp = at(20) }),
				with(notAffected, func(s *vex.Statement) {
					s.Status, s.Justification, s.ActionStatement, s.Timestamp = vex.StatusAffected, "", "upgrade", at(
						21,
					)
				}),
			},
		},
		{
			name: "a later not_affected statement wins, whatever the order",
			statements: []vex.Statement{
				with(notAffected, func(s *vex.Statement) { s.Timestamp = at(22) }),
				with(notAffected, func(s *vex.Statement) {
					s.Status, s.Justification, s.ActionStatement, s.Timestamp = vex.StatusAffected, "", "upgrade", at(
						21,
					)
				}),
			},
			want: &enrich.Suppression{
				Kind: "vex", Reason: "vulnerable_code_not_in_execute_path", Source: "k.json",
				Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := vex.Document{Timestamp: *at(19), Statements: tt.statements}
			r, err := vex.New(nil, []vex.File{{Path: "k.json", Document: doc}}, today())
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Suppress(subject(t, "kubectl", kubePURL), kubeNames()))
		})
	}
}

// TestIgnoreBeforeVEX: an ignore entry and a statement for the same advisory give the entry's
// reason.
func TestIgnoreBeforeVEX(t *testing.T) {
	t.Parallel()
	doc := vex.Document{Timestamp: *at(19), Statements: []vex.Statement{{
		Vulnerability: vex.Vulnerability{Name: "GO-2025-3547"}, Products: []vex.Component{{ID: kubePURL}},
		Status: vex.StatusNotAffected, ImpactStatement: "vex",
	}}}
	r, err := vex.New([]vex.Ignore{{ID: "GO-2025-3547", Reason: "config"}}, []vex.File{{Path: "k.json", Document: doc}},
		today())
	require.NoError(t, err)
	assert.Equal(t, "config", r.Suppress(subject(t, "kubectl", kubePURL), kubeNames()).Reason)
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, doc, err string
	}{
		{
			name: "v0.2.0",
			doc: `{"@context":"https://openvex.dev/ns/v0.2.0","@id":"urn:x","author":"a",` +
				`"timestamp":"2026-11-20T00:00:00Z","version":1,"statements":[{"vulnerability":{"name":"CVE-1"},` +
				`"products":[{"@id":"pkg:npm/x@1"}],"status":"fixed"}]}`,
		},
		{name: "v0.0.1", doc: `{"@context":"https://openvex.dev/ns","statements":[]}`, err: "@context"},
		{name: "not JSON", doc: `{`, err: "parse openvex"},
		{
			name: "no vulnerability name",
			doc:  `{"@context":"https://openvex.dev/ns/v0.2.0","statements":[{"vulnerability":{},"status":"fixed"}]}`,
			err:  "no vulnerability name",
		},
		{
			name: "unknown status",
			doc: `{"@context":"https://openvex.dev/ns/v0.2.0","statements":[{"vulnerability":{"name":"CVE-1"},` +
				`"status":"accepted"}]}`,
			err: `status "accepted"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := vex.Parse([]byte(tt.doc))
			if tt.err == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.err)
		})
	}
}

// TestWrite: a written document is valid OpenVEX v0.2.0, and its @id depends on the statements,
// not on the time it was written.
func TestWrite(t *testing.T) {
	t.Parallel()
	doc := vex.Document{Author: "Unknown Author", Timestamp: today(), Statements: []vex.Statement{{
		Vulnerability: vex.Vulnerability{Name: "GO-2025-3547"}, Products: []vex.Component{{ID: kubePURL}},
		Status: vex.StatusAffected, ActionStatement: "No release fixes it yet.",
	}}}
	var a, b bytes.Buffer
	require.NoError(t, vex.Write(&a, doc))
	require.NoError(t, vexcheck.Validate(a.Bytes()))
	doc.Timestamp = today().Add(time.Hour)
	require.NoError(t, vex.Write(&b, doc))
	var da, db vex.Document
	require.NoError(t, json.Unmarshal(a.Bytes(), &da))
	require.NoError(t, json.Unmarshal(b.Bytes(), &db))
	assert.Equal(t, da.ID, db.ID)
	assert.Regexp(t, `^https://openvex\.dev/docs/public/vex-[0-9a-f]{64}$`, da.ID)
}
