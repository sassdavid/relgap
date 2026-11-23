package render

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
	"github.com/sassdavid/relgap/internal/scan"
)

// AuditUncheckedTitle heads audit's list of tool versions OSV wasn't asked about.
const AuditUncheckedTitle = "Not checked for vulnerabilities"

// AuditRules returns the IDs of the fixed rules in audit's SARIF, in their order there.
func AuditRules() []string { return []string{RuleFailed, RuleUnchecked} }

// AuditText writes the findings as an aligned table, then the advisories, the fix targets, the
// suppressed findings, the unchecked tool versions and a summary. With color it marks high and critical ratings red (the
// tty format).
func AuditText(w io.Writer, a *report.Audit, color bool) error {
	var b strings.Builder
	if len(a.Findings) > 0 {
		writeAligned(&b, auditRows(a), color)
		b.WriteString("\nAdvisories:\n")
		for _, line := range advisoryLines(a) {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}
	if lines := fixLines(a.Fixes()); len(lines) > 0 {
		b.WriteString(FixTitle + ":\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}
	if lines := auditSuppressedLines(a); len(lines) > 0 {
		b.WriteString(SuppressedTitle + ":\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}
	if lines := auditFailedLines(a); len(lines) > 0 {
		b.WriteString("Failed:\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}
	if len(a.Unchecked) > 0 {
		b.WriteString(AuditUncheckedTitle + ":\n")
		for _, line := range uncheckedLines(a.Unchecked) {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(auditSummary(a) + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

// AuditMarkdown writes the findings as a Markdown table with the same sections as [AuditText].
// Mentions and issue references in advisory summaries are silenced.
func AuditMarkdown(w io.Writer, a *report.Audit) error {
	var b strings.Builder
	if len(a.Findings) > 0 {
		rows := auditRows(a)
		writeMarkdownTable(&b, rows, strings.Repeat("| --- ", len(rows[0]))+"|")
		b.WriteString("\nAdvisories:\n\n")
		for _, line := range advisoryLines(a) {
			b.WriteString("- " + mdCell(line) + "\n")
		}
		b.WriteString("\n")
	}
	if lines := fixLines(a.Fixes()); len(lines) > 0 {
		b.WriteString(FixTitle + ":\n\n")
		for _, line := range lines {
			b.WriteString("- " + mdCell(line) + "\n")
		}
		b.WriteString("\n")
	}
	if lines := auditSuppressedLines(a); len(lines) > 0 {
		b.WriteString(SuppressedTitle + ":\n\n")
		for _, line := range lines {
			b.WriteString("- " + mdCell(line) + "\n")
		}
		b.WriteString("\n")
	}
	if lines := auditFailedLines(a); len(lines) > 0 {
		b.WriteString("Failed:\n\n")
		for _, line := range lines {
			b.WriteString("- " + mdCell(line) + "\n")
		}
		b.WriteString("\n")
	}
	if len(a.Unchecked) > 0 {
		b.WriteString(AuditUncheckedTitle + ":\n\n")
		for _, line := range uncheckedLines(a.Unchecked) {
			b.WriteString("- " + mdCell(line) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(auditSummary(a) + "\n")
	if _, err := io.WriteString(w, silence(b.String())); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}

// auditRows has a column for the KEV entry and the EPSS score when the run asked those feeds.
func auditRows(a *report.Audit) [][]cell {
	feeds := a.Feeds()
	header := []string{headTool, "VERSION", "ADVISORY", "SEVERITY"}
	if feeds.KEV {
		header = append(header, "KEV")
	}
	if feeds.EPSS {
		header = append(header, "EPSS")
	}
	header = append(header, "FIXED IN", "PINS")
	rows := [][]cell{make([]cell, len(header))}
	for i, h := range header {
		rows[0][i] = cell{text: h}
	}
	for i := range a.Findings {
		f := &a.Findings[i]
		fixed := f.FixedIn
		if fixed == "" {
			fixed = noValue
		}
		row := []cell{{text: toolName(f.Tool)}, {text: f.Version}, {text: f.ID}, severityCell(f.Severity)}
		if feeds.KEV {
			row = append(row, kevCell(f.KEV))
		}
		if feeds.EPSS {
			row = append(row, cell{text: epssText(f.EPSS)})
		}
		rows = append(rows, append(row, cell{text: fixed}, cell{text: pinsCell(f.Pins)}))
	}
	return rows
}

// kevCell shows the date CISA added the advisory to the KEV catalog, with "ransomware" when it
// knows of its use in ransomware campaigns, or - when it isn't listed.
func kevCell(k *enrich.KEV) cell {
	if k == nil {
		return cell{text: noValue}
	}
	text := k.DateAdded
	if k.Ransomware {
		text += " ransomware"
	}
	return cell{text: text, style: ansiBold + ansiRed}
}

// epssText shows an EPSS score as a percentage with its percentile, such as "89.5% (99.8th)",
// or - without a score.
func epssText(e *enrich.EPSS) string {
	if e == nil {
		return noValue
	}
	const (
		percent  = 100
		smallest = 0.05 // the smallest percentage one decimal shows
	)
	score := strconv.FormatFloat(e.Score*percent, 'f', 1, 64) + "%"
	if e.Score > 0 && e.Score*percent < smallest {
		score = "<0.1%"
	}
	// Rounding down keeps the top scores below 100th.
	pct := math.Floor(e.Percentile*percent*10) / 10 //nolint:mnd // one decimal
	return score + " (" + strconv.FormatFloat(pct, 'f', 1, 64) + "th)"
}

// severityCell shows the rating and score, such as "high 7.5", or unknown without a rating.
func severityCell(s *enrich.Score) cell {
	if s.Unknown() || s.Rating == "" {
		return cell{text: enrich.RatingUnknown}
	}
	text := s.Rating
	if s.Score > 0 {
		text += " " + strconv.FormatFloat(s.Score, 'f', 1, 64)
	}
	c := cell{text: text}
	if s.Rating == "critical" || s.Rating == "high" {
		c.style = ansiRed
	}
	return c
}

// pinsCell names the first pin location and counts the others, which JSON lists.
func pinsCell(locs []scan.Location) string {
	if len(locs) == 0 {
		return noValue
	}
	text := PinLocation(locs[0])
	if n := len(locs) - 1; n > 0 {
		text += fmt.Sprintf(" (+%d)", n)
	}
	return text
}

// advisoryLines describes each advisory once: its summary, aliases and osv.dev page.
func advisoryLines(a *report.Audit) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range a.Findings {
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		line := f.ID
		if len(f.Aliases) > 0 {
			line += " (" + strings.Join(f.Aliases, ", ") + ")"
		}
		if f.Summary != "" {
			line += ": " + f.Summary
		}
		out = append(out, line+" "+f.URL)
	}
	return out
}

// auditFailedLines describes the pins whose version lookup failed.
func auditFailedLines(a *report.Audit) []string {
	var out []string
	for _, p := range a.Failed() {
		out = append(out, fmt.Sprintf("%s %s (%s): %s", toolName(p.Tool), p.Pin.Value, PinLocation(p.Pin.Location),
			p.Note))
	}
	return out
}

func auditSummary(a *report.Audit) string {
	versions := func(n int) string { return pinsWord(n, "tool version", "tool versions") }
	var s string
	switch {
	case a.Checked == 0:
		s = "No tool version was checked."
	case len(a.Findings) == 0:
		s = fmt.Sprintf("No known vulnerabilities in %d checked %s.", a.Checked, versions(a.Checked))
	default:
		affected := map[string]bool{}
		for _, f := range a.Findings {
			affected[f.Tool+"\x00"+f.Version] = true
		}
		s = fmt.Sprintf("%d %s in %d of %d checked %s.", len(a.Findings),
			pinsWord(len(a.Findings), "finding", "findings"), len(affected), a.Checked, versions(a.Checked))
	}
	if n := len(a.Suppressed); n > 0 {
		s += fmt.Sprintf(" %d suppressed.", n)
	}
	if n := len(a.Unchecked); n > 0 {
		s += fmt.Sprintf(" %d not checked.", n)
	}
	return s
}

// AuditSARIF returns a SARIF log with a result per finding and pin location, the suppressed ones
// marked with a suppression, per tool version OSV wasn't asked about and pin location, per pin
// whose version lookup failed and per file that didn't parse. Results don't depend on --fail-on.
// Rules are failed and unchecked, then one rule per OSV ID.
func AuditSARIF(a *report.Audit, problems []scan.Problem, version string) *sarif.Log {
	r := newSARIFRun(version, AuditRules()...)
	all := slices.Concat(a.Findings, a.Suppressed)
	ids := make([]string, 0, len(all))
	byID := map[string]*report.AuditFinding{}
	for i := range all {
		if f := &all[i]; byID[f.ID] == nil {
			byID[f.ID] = f
			ids = append(ids, f.ID)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		f := byID[id]
		r.addRule(vulnRule(id, f.Summary, f.Aliases, f.Severity))
	}
	for i := range all {
		f := &all[i]
		v := &enrich.Vuln{
			ID: f.ID, Aliases: f.Aliases, Summary: f.Summary, Severity: f.Severity, FixedIn: f.FixedIn,
			KEV: f.KEV, EPSS: f.EPSS,
		}
		var msg string
		if f.Suppressed != nil {
			msg = vulnText(f.Tool, f.Version, v, nil) + "; " + suppressionText(f.Suppressed)
		} else {
			msg = vulnText(f.Tool, f.Version, v, auditFix(a, f.Tool, f.Version))
		}
		for _, loc := range f.Pins {
			r.add(f.ID, vulnLevel(v), msg, loc, f.Tool, f.ID)
			r.feedsLast(v)
			if f.Suppressed != nil {
				r.suppressLast(f.Suppressed)
			}
		}
	}
	for _, p := range a.Failed() {
		r.add(RuleFailed, sarif.LevelError, fmt.Sprintf("%s %s: %s", p.Tool, p.Pin.Value, p.Note), p.Pin.Location,
			p.Tool, "")
	}
	for _, u := range a.Unchecked {
		for _, loc := range u.Pins {
			r.add(RuleUnchecked, sarif.LevelNote, fmt.Sprintf("%s %s: not checked for vulnerabilities: %s",
				u.Tool, u.Version, u.Reason), loc, u.Tool, u.Reason)
		}
	}
	for _, pr := range problems {
		r.add(RuleFailed, sarif.LevelError, "can't read pins: "+pr.Message,
			scan.Location{Repo: pr.Repo, File: pr.File}, "", "")
	}
	return r.log
}
