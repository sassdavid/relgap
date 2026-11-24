package render

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sassdavid/relgap/internal/registry"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
)

const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	noValue    = "-"
	headTool   = "TOOL" // the first column header of the tables
	columnGap  = 2      // spaces between columns, as in the tabwriter tables
)

// cell is a table cell with an optional ANSI style.
type cell struct {
	text  string
	style string
}

// OutdatedText writes the pins as an aligned table followed by notes. With color it marks
// outdated versions yellow, current ones green and breaking counts red (the tty format).
// Skipped pins are left out; JSON lists them.
func OutdatedText(w io.Writer, o *report.Outdated, color bool) error {
	rows, notes := outdatedRows(o, plain)
	var b strings.Builder
	writeAligned(&b, rows, color)
	if len(notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range notes {
			b.WriteString("  " + n + "\n")
		}
	}
	if lines := fixLines(o.Fixes(), plain); len(lines) > 0 {
		b.WriteString("\n" + FixTitle + ":\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
	}
	if lines := outdatedSuppressedLines(o, plain); len(lines) > 0 {
		b.WriteString("\n" + SuppressedTitle + ":\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(o.Unchecked) > 0 {
		b.WriteString("\n" + UncheckedTitle + ":\n")
		for _, line := range uncheckedLines(o.Unchecked, plain) {
			b.WriteString("  " + line + "\n")
		}
	}
	if o.Binaries != nil {
		b.WriteString("\nBinaries (" + enrich.ReachabilityNote + "):\n")
		for _, line := range binaryLines(o.Binaries, plain) {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + summary(o) + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write outdated: %w", err)
	}
	return nil
}

// writeAligned writes rows as columns padded to their widest cell. With color it wraps styled
// cells in ANSI codes, which don't count toward the width.
func writeAligned(b *strings.Builder, rows [][]cell, color bool) {
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(c.text))
		}
	}
	for _, r := range rows {
		for i, c := range r {
			text := c.text
			if color && c.style != "" {
				text = c.style + text + ansiReset
			}
			b.WriteString(text)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.text)+columnGap))
			}
		}
		b.WriteString("\n")
	}
}

// OutdatedMarkdown writes the pins as a Markdown table followed by notes.
func OutdatedMarkdown(w io.Writer, o *report.Outdated) error {
	rows, notes := outdatedRows(o, mdCode)
	var b strings.Builder
	writeMarkdownTable(&b, rows, "| --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- |")
	if len(notes) > 0 {
		b.WriteString("\n")
		for _, n := range notes {
			b.WriteString("- " + mdCell(n) + "\n")
		}
	}
	if lines := fixLines(o.Fixes(), mdCode); len(lines) > 0 {
		b.WriteString("\n" + FixTitle + ":\n\n")
		for _, line := range lines {
			b.WriteString("- " + mdCell(line) + "\n")
		}
	}
	if lines := outdatedSuppressedLines(o, mdCode); len(lines) > 0 {
		b.WriteString("\n" + SuppressedTitle + ":\n\n")
		for _, line := range lines {
			b.WriteString("- " + mdCell(line) + "\n")
		}
	}
	if len(o.Unchecked) > 0 {
		b.WriteString("\n" + UncheckedTitle + ":\n\n")
		for _, line := range uncheckedLines(o.Unchecked, mdCode) {
			b.WriteString("- " + mdCell(line) + "\n")
		}
	}
	if o.Binaries != nil {
		b.WriteString("\nBinaries (" + enrich.ReachabilityNote + "):\n\n")
		for _, line := range binaryLines(o.Binaries, mdCode) {
			b.WriteString("- " + mdCell(line) + "\n")
		}
	}
	b.WriteString("\n" + summary(o) + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write outdated: %w", err)
	}
	return nil
}

// writeMarkdownTable writes rows as a Markdown table with sep as the delimiter row.
func writeMarkdownTable(b *strings.Builder, rows [][]cell, sep string) {
	for i, r := range rows {
		texts := make([]string, len(r))
		for j, c := range r {
			texts[j] = mdCell(c.text)
		}
		b.WriteString("| " + strings.Join(texts, " | ") + " |\n")
		if i == 0 {
			b.WriteString(sep + "\n")
		}
	}
}

func mdCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
}

func outdatedRows(o *report.Outdated, code func(string) string) ([][]cell, []string) {
	header := []string{headTool, "CURRENT", "LATEST", "BEHIND", "BREAKING", "DEPRECATION", "EOL", "VULNS", "PIN"}
	rows := [][]cell{make([]cell, len(header))}
	for i, h := range header {
		rows[0][i] = cell{text: h}
	}
	var notes []string
	for i := range o.Pins {
		p := &o.Pins[i]
		if p.Status == report.StatusSkipped {
			continue
		}
		rows = append(rows, outdatedRow(p, code))
		if n := outdatedNote(p, code); n != "" {
			notes = append(notes, n)
		}
		if p.EOL != nil && p.EOL.Current != nil && p.EOL.Current.Note != "" {
			notes = append(
				notes,
				fmt.Sprintf("%s %s (%s): %s", code(toolName(p.Tool)), p.Current, PinLocation(p.Pin.Location),
					p.EOL.Current.Note),
			)
		}
	}
	return rows, notes
}

func outdatedRow(p *report.OutdatedPin, code func(string) string) []cell {
	current := p.Current
	if current == "" {
		current = p.Pin.Value
	}
	if !p.Pin.Exact {
		current = ApproxMark + current
	}
	row := []cell{
		{text: code(toolName(p.Tool))},
		{text: current},
		{text: noValue},
		{text: noValue},
		{text: noValue},
		{text: noValue},
		eolCell(p.EOL),
		vulnCell(p),
		{text: PinLocation(p.Pin.Location)},
	}
	switch p.Status {
	case report.StatusCurrent:
		row[2] = cell{text: p.Latest, style: ansiGreen}
	case report.StatusOutdated:
		row[2] = cell{text: p.Latest, style: ansiYellow}
	case report.StatusError:
		row[2] = cell{text: "error", style: ansiRed}
		return row
	case report.StatusUnknown, report.StatusSkipped:
		return row
	}
	row[3] = cell{text: strconv.Itoa(p.Behind.Releases)}
	if p.Counts == nil {
		return row // no release notes to count
	}
	breaking := p.Counts[classify.KindBreaking]
	row[4] = cell{text: strconv.Itoa(breaking)}
	if breaking > 0 {
		row[4].style = ansiBold + ansiRed
	}
	row[5] = cell{text: strconv.Itoa(p.Counts[classify.KindDeprecation])}
	return row
}

// eolCell says whether the current version's cycle has reached its end: yes with the date, no,
// ? for a cycle endoflife.date doesn't list, or - without a product.
func eolCell(e *enrich.EOL) cell {
	switch {
	case e == nil || e.Current == nil:
		return cell{text: noValue}
	case e.Current.IsEOL == nil:
		return cell{text: "?"}
	case *e.Current.IsEOL:
		text := "yes"
		if e.Current.EOLFrom != "" {
			text += " (" + e.Current.EOLFrom + ")"
		}
		return cell{text: text, style: ansiBold + ansiRed}
	default:
		return cell{text: "no"}
	}
}

// vulnCell counts the unsuppressed vulnerabilities of the current version, how many the latest
// version fixes and how many are in the KEV catalog. It shows ? for a pin OSV wasn't asked about
// and - when enrichment didn't run.
func vulnCell(p *report.OutdatedPin) cell {
	switch {
	case p.Unchecked != "":
		return cell{text: "?"}
	case p.Vulns == nil:
		return cell{text: noValue}
	}
	vs := slices.DeleteFunc(slices.Clone(p.Vulns), func(v enrich.Vuln) bool { return v.Suppressed != nil })
	fixed, kev := 0, 0
	for _, v := range vs {
		if v.FixedByTarget != nil && *v.FixedByTarget {
			fixed++
		}
		if v.KEV != nil {
			kev++
		}
	}
	var notes []string
	if fixed > 0 {
		notes = append(notes, fmt.Sprintf("%d fixed", fixed))
	}
	if kev > 0 {
		notes = append(notes, fmt.Sprintf("%d KEV", kev))
	}
	text := strconv.Itoa(len(vs))
	if len(notes) > 0 {
		text += " (" + strings.Join(notes, ", ") + ")"
	}
	c := cell{text: text}
	if len(vs) > 0 {
		c.style = ansiRed
	}
	return c
}

// UncheckedTitle heads the list of pins OSV wasn't asked about.
const UncheckedTitle = "Not checked for vulnerabilities (VULNS ?)"

// uncheckedLines describes each unchecked tool version: its reason and where it is pinned.
func uncheckedLines(us []report.Unchecked, code func(string) string) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		locs := make([]string, len(u.Pins))
		for i, l := range u.Pins {
			locs[i] = PinLocation(l)
		}
		out = append(
			out,
			fmt.Sprintf("%s %s (%s): %s", code(toolName(u.Tool)), u.Version, strings.Join(locs, ", "), u.Reason),
		)
	}
	return out
}

// binaryLines describes each installed executable, its unsuppressed vulnerabilities and the
// suppressed ones with the reason.
func binaryLines(bins []*enrich.Binary, code func(string) string) []string {
	if len(bins) == 0 {
		return []string{"no Go executables of the pinned tools found"}
	}
	out := make([]string, 0, len(bins))
	for _, b := range bins {
		built := "Go " + b.GoVersion
		if b.ModuleVersion != "" {
			built += ", " + b.Module + " " + b.ModuleVersion
		}
		var ids, suppressed []string
		for _, v := range b.Vulns {
			if v.Suppressed != nil {
				suppressed = append(suppressed, v.ID+" ("+suppressionText(v.Suppressed)+")")
				continue
			}
			id := v.ID
			if v.KEV != nil {
				id += " (KEV)"
			}
			ids = append(ids, id)
		}
		line := fmt.Sprintf("%s %s (%s): %d %s", code(b.Tool), b.Path, built, len(ids),
			pinsWord(len(ids), "vulnerability", "vulnerabilities"))
		if len(ids) > 0 {
			line += ": " + strings.Join(ids, ", ")
		}
		if len(suppressed) > 0 {
			line += fmt.Sprintf("; %d suppressed: %s", len(suppressed), strings.Join(suppressed, ", "))
		}
		out = append(out, line)
	}
	return out
}

// outdatedNote explains a non-exact pin or a pin that wasn't compared.
func outdatedNote(p *report.OutdatedPin, code func(string) string) string {
	where := fmt.Sprintf("%s %s (%s)", code(toolName(p.Tool)), p.Pin.Value, PinLocation(p.Pin.Location))
	switch {
	case p.Status == report.StatusUnknown || p.Status == report.StatusError:
		return where + ": " + p.Note
	case p.Via != "":
		return where + ": " + p.Via
	case p.Note != "":
		return where + ": " + p.Note
	default:
		return ""
	}
}

func summary(o *report.Outdated) string {
	var outdated, compared, skipped int
	for _, p := range o.Pins {
		switch p.Status {
		case report.StatusOutdated:
			outdated++
			compared++
		case report.StatusCurrent:
			compared++
		case report.StatusSkipped:
			skipped++
		case report.StatusUnknown, report.StatusError:
		}
	}
	s := fmt.Sprintf(
		"%d of %d compared %s behind latest.",
		outdated,
		compared,
		pinsWord(compared, "pin is", "pins are"),
	)
	if skipped > 0 {
		s += fmt.Sprintf(" %d %s skipped (see --format json).", skipped, pinsWord(skipped, "pin", "pins"))
	}
	return s
}

func pinsWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// toolName is the name a table shows for a tool: the short form of a chart read through a URL.
// JSON, SARIF and the CLI's inputs keep the full name.
func toolName(name string) string { return registry.ChartShortName(name) }
