package render

import (
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
)

// FixTitle heads the list of fix targets.
const FixTitle = "Fix targets"

// fixLine describes the fix target of version of the tool shown as name: the release to move to,
// the advisories no release fixes, and the advisories the latest release still has.
func fixLine(name, version string, f *enrich.Fix) string {
	var parts []string
	switch {
	case f.Lowest != "" && f.LowestInLine == "":
		parts = append(parts, "upgrade to "+f.Lowest+", outside the current line")
	case f.Lowest != "":
		parts = append(parts, "upgrade to "+f.Lowest)
	case len(f.Unfixed) == 0:
		parts = append(parts, "no release without a fixable advisory found")
	}
	if len(f.Unfixed) > 0 {
		parts = append(parts, "no release fixes "+strings.Join(f.Unfixed, ", "))
	}
	if f.LatestAffected {
		parts = append(parts, LatestAffected(f))
	}
	return name + " " + version + ": " + strings.Join(parts, "; ")
}

// LatestAffected is the warning for a fix target whose latest release still has advisories,
// such as "latest 1.35.0 still has GO-2025-3521".
func LatestAffected(f *enrich.Fix) string {
	return "latest " + f.Latest + " still has " + strings.Join(f.LatestAffectedBy, ", ")
}

// fixText is what a finding's message adds about advisory id from its tool version's fix target.
func fixText(f *enrich.Fix, id string) string {
	switch {
	case f == nil:
		return ""
	case slices.Contains(f.Unfixed, id):
		return "; no release fixes it"
	}
	var s string
	if f.Lowest != "" {
		s = "; upgrade to " + f.Lowest
	}
	if f.LatestAffected {
		s += "; latest " + f.Latest + " still affected"
	}
	return s
}

// fixLines describes each fix target.
func fixLines(fixes []report.ToolFix, code func(string) string) []string {
	var out []string
	for _, t := range fixes {
		out = append(out, fixLine(code(toolName(t.Tool)), t.Version, t.Fix))
	}
	return out
}

// auditFix returns the fix target of version of tool, or nil.
func auditFix(a *report.Audit, tool, version string) *enrich.Fix {
	for _, t := range a.Fixes() {
		if t.Tool == tool && t.Version == version {
			return t.Fix
		}
	}
	return nil
}
