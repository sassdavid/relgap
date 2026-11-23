package report

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/sassdavid/relgap/internal/enrich"
)

// UnmarshalJSON reads a target, or null for an advisory no release fixes. An absent field never
// calls it, which leaves the target unknown.
func (f *FindingFix) UnmarshalJSON(b []byte) error {
	f.Known = true
	f.Target = nil
	if string(b) == "null" {
		return nil
	}
	return json.Unmarshal(b, &f.Target) //nolint:wrapcheck // the caller names the file
}

// ParseAudit reads the data of a saved `relgap audit --format json` report. The report has the
// fix targets and feed columns its findings carry. A tool version whose every finding has no
// fix keeps only the advisories no release fixes, since JSON holds no target for it, and pins
// whose version lookup failed are only in the envelope's errors.
func ParseAudit(raw []byte) (*Audit, error) {
	var a Audit
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("parse audit: %w", err)
	}
	a.rows = &Outdated{}
	all := slices.Concat(a.Findings, a.Suppressed)
	for i := range all {
		a.feeds.KEV = a.feeds.KEV || all[i].KEV != nil
		a.feeds.EPSS = a.feeds.EPSS || all[i].EPSS != nil
	}
	a.fixes = savedFixes(a.Findings)
	return &a, nil
}

// savedFixes rebuilds the fix target of each tool version, in the order of its first finding.
func savedFixes(findings []AuditFinding) []ToolFix {
	var (
		out   []ToolFix
		index = map[string]int{}
	)
	for _, f := range findings {
		if !f.Fix.Known {
			continue
		}
		key := f.Tool + "\x00" + f.Version
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, ToolFix{Tool: f.Tool, Version: f.Version, Fix: &enrich.Fix{}})
		}
		switch {
		case f.Fix.Target != nil:
			out[i].Fix = f.Fix.Target
		case !slices.Contains(out[i].Fix.Unfixed, f.ID):
			out[i].Fix.Unfixed = append(out[i].Fix.Unfixed, f.ID)
		}
	}
	return out
}

// AtSeverity returns the report with only the findings and suppressed findings rated rating or
// higher, and those of unknown severity, which reach every threshold. An empty rating keeps all.
func (a *Audit) AtSeverity(rating string) *Audit {
	if rating == "" {
		return a
	}
	g := Gate{VulnSeverity: rating}
	keep := func(fs []AuditFinding) []AuditFinding {
		out := []AuditFinding{}
		for _, f := range fs {
			if g.atSeverity(f.Severity) {
				out = append(out, f)
			}
		}
		return out
	}
	out := *a
	out.Findings, out.Suppressed = keep(a.Findings), keep(a.Suppressed)
	return &out
}

// Open reports whether a saved audit has something to report: a finding, or a pin that a failure,
// --offline or a name that resolves to no tool left unchecked. A pin no advisory source covers,
// such as an image or a chart, doesn't count, since nearly every repository has one.
func (a *Audit) Open() bool {
	return len(a.Findings) > 0 || slices.ContainsFunc(a.Unchecked, func(u Unchecked) bool {
		switch u.Reason {
		case enrich.ReasonFailed, enrich.ReasonOffline, enrich.ReasonUnresolved:
			return true
		}
		return false
	})
}
