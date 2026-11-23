package render

import (
	"fmt"
	"io"
	"time"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/purl"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/vex"
)

// vexAuthor is the author of the documents relgap writes, the name go-vex and govulncheck give a
// document whose author isn't known.
const vexAuthor = "Unknown Author"

// AuditOpenVEX writes the findings as an OpenVEX v0.2.0 document issued at now, one statement per
// advisory and tool version: affected with the fix target as the action statement, and the
// suppressed ones with the status that suppresses them, not_affected for a vulns.ignore entry.
// Products are the purls of the advisories' packages at the tool versions.
func AuditOpenVEX(w io.Writer, a *report.Audit, now time.Time, version string) error {
	doc := vex.Document{
		Author:    vexAuthor,
		Timestamp: now.UTC(),
		Tooling:   ToolName + " " + version,
	}
	for i := range a.Findings {
		f := &a.Findings[i]
		st := vexStatement(f)
		st.Status, st.ActionStatement = vex.StatusAffected, vexAction(f)
		doc.Statements = append(doc.Statements, st)
	}
	for i := range a.Suppressed {
		f := &a.Suppressed[i]
		st := vexStatement(f)
		s := f.Suppressed
		switch {
		case s.Kind == enrich.SuppressedByVEX && s.Status == vex.StatusFixed:
			st.Status = vex.StatusFixed
		case s.Kind == enrich.SuppressedByVEX:
			st.Status, st.Justification = vex.StatusNotAffected, s.Justification
			if s.Reason != s.Justification {
				st.ImpactStatement = s.Reason
			}
		default:
			st.Status, st.ImpactStatement = vex.StatusNotAffected, s.Reason
		}
		doc.Statements = append(doc.Statements, st)
	}
	if err := vex.Write(w, doc); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

// vexStatement names a finding's advisory and product.
func vexStatement(f *report.AuditFinding) vex.Statement {
	id := f.PURL()
	if id == "" {
		// The advisory's ecosystem has no purl type, so the tool version names the product.
		id = purl.PURL{Type: "generic", Name: f.Tool, Version: f.Version}.String()
	}
	return vex.Statement{
		Vulnerability: vex.Vulnerability{
			ID: f.URL, Name: f.ID, Description: f.Summary, Aliases: f.Aliases,
		},
		Products: []vex.Component{{ID: id}},
	}
}

// vexAction is what to do about an affected finding, from its fix target.
func vexAction(f *report.AuditFinding) string {
	t := f.Fix.Target
	switch {
	case !f.Fix.Known:
		return "Upgrade past the affected versions; relgap couldn't work out the release to move to."
	case t == nil:
		return "No release fixes it yet."
	case t.Lowest == "":
		return "Upgrade past the affected versions; no release clears every advisory of " + f.Version + " yet."
	case t.LowestInLine != "" && t.LowestInLine != t.Lowest:
		return "Upgrade to " + t.Lowest + ", or to " + t.LowestInLine + " in the same release line."
	default:
		return "Upgrade to " + t.Lowest + "."
	}
}
