package render

import (
	"fmt"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
)

// SuppressedTitle heads the list of advisories a vulns.ignore entry or VEX statement suppresses.
const SuppressedTitle = "Suppressed"

// suppressionText says what suppresses an advisory: "ignored until 2026-12-31: reason" or
// "VEX not_affected in vex.json: reason".
func suppressionText(s *enrich.Suppression) string {
	if s.Kind == enrich.SuppressedByVEX {
		return fmt.Sprintf("VEX %s in %s: %s", s.Status, s.Source, s.Reason)
	}
	if s.Until != "" {
		return fmt.Sprintf("ignored until %s: %s", s.Until, s.Reason)
	}
	return "ignored: " + s.Reason
}

// sarifSuppression records a suppression as SARIF does for one from configuration.
func sarifSuppression(s *enrich.Suppression) sarif.Suppression {
	return sarif.Suppression{Kind: "external", Status: "accepted", Justification: suppressionText(s)}
}

// auditSuppressedLines describes each suppressed finding: the advisory, the tool version and why.
func auditSuppressedLines(a *report.Audit) []string {
	out := make([]string, 0, len(a.Suppressed))
	for _, f := range a.Suppressed {
		out = append(out, fmt.Sprintf("%s in %s %s: %s", f.ID, toolName(f.Tool), f.Version,
			suppressionText(f.Suppressed)))
	}
	return out
}

// outdatedSuppressedLines describes each suppressed advisory of a pin's current version, once per
// tool version.
func outdatedSuppressedLines(o *report.Outdated) []string {
	var out []string
	seen := map[string]bool{}
	for i := range o.Pins {
		p := &o.Pins[i]
		for _, v := range p.Vulns {
			key := p.Tool + "\x00" + p.Current + "\x00" + v.ID
			if v.Suppressed == nil || !v.AffectsCurrent || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, fmt.Sprintf("%s in %s %s: %s", v.ID, toolName(p.Tool), p.Current,
				suppressionText(v.Suppressed)))
		}
	}
	return out
}
