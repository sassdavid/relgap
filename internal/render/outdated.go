package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
)

const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	noValue    = "-"
	columnGap  = 2 // spaces between columns, as in the tabwriter tables
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
	rows, notes := outdatedRows(o)
	var b strings.Builder
	writeAligned(&b, rows, color)
	if len(notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, n := range notes {
			b.WriteString("  " + n + "\n")
		}
	}
	if o.Binaries != nil {
		b.WriteString("\nBinaries (" + enrich.ReachabilityNote + "):\n")
		for _, line := range binaryLines(o.Binaries) {
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
	rows, notes := outdatedRows(o)
	var b strings.Builder
	writeMarkdownTable(&b, rows, "| --- | --- | --- | ---: | ---: | ---: | --- | ---: | --- |")
	if len(notes) > 0 {
		b.WriteString("\n")
		for _, n := range notes {
			b.WriteString("- " + mdCell(n) + "\n")
		}
	}
	if o.Binaries != nil {
		b.WriteString("\nBinaries (" + enrich.ReachabilityNote + "):\n\n")
		for _, line := range binaryLines(o.Binaries) {
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

func outdatedRows(o *report.Outdated) ([][]cell, []string) {
	header := []string{"TOOL", "CURRENT", "LATEST", "BEHIND", "BREAKING", "DEPRECATION", "EOL", "VULNS", "PIN"}
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
		rows = append(rows, outdatedRow(p))
		if n := outdatedNote(p); n != "" {
			notes = append(notes, n)
		}
		if p.EOL != nil && p.EOL.Current != nil && p.EOL.Current.Note != "" {
			notes = append(notes, fmt.Sprintf("%s %s (%s): %s", p.Tool, p.Current, PinLocation(p.Pin.Location),
				p.EOL.Current.Note))
		}
	}
	return rows, notes
}

func outdatedRow(p *report.OutdatedPin) []cell {
	current := p.Current
	if current == "" {
		current = p.Pin.Value
	}
	if !p.Pin.Exact {
		current = ApproxMark + current
	}
	row := []cell{
		{text: p.Tool},
		{text: current},
		{text: noValue},
		{text: noValue},
		{text: noValue},
		{text: noValue},
		eolCell(p.EOL),
		vulnCell(p.Vulns),
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

// vulnCell counts the vulnerabilities of the current version, and how many the latest version
// fixes.
func vulnCell(vs []enrich.Vuln) cell {
	if vs == nil {
		return cell{text: noValue}
	}
	fixed := 0
	for _, v := range vs {
		if v.FixedByTarget != nil && *v.FixedByTarget {
			fixed++
		}
	}
	text := strconv.Itoa(len(vs))
	if fixed > 0 {
		text += fmt.Sprintf(" (%d fixed)", fixed)
	}
	c := cell{text: text}
	if len(vs) > 0 {
		c.style = ansiRed
	}
	return c
}

// binaryLines describes each installed executable and its vulnerabilities.
func binaryLines(bins []*enrich.Binary) []string {
	if len(bins) == 0 {
		return []string{"no Go executables of the pinned tools found"}
	}
	out := make([]string, 0, len(bins))
	for _, b := range bins {
		built := "Go " + b.GoVersion
		if b.ModuleVersion != "" {
			built += ", " + b.Module + " " + b.ModuleVersion
		}
		ids := make([]string, 0, len(b.Vulns))
		for _, v := range b.Vulns {
			ids = append(ids, v.ID)
		}
		line := fmt.Sprintf("%s %s (%s): %d %s", b.Tool, b.Path, built, len(b.Vulns),
			pinsWord(len(b.Vulns), "vulnerability", "vulnerabilities"))
		if len(ids) > 0 {
			line += ": " + strings.Join(ids, ", ")
		}
		out = append(out, line)
	}
	return out
}

// outdatedNote explains a non-exact pin or a pin that wasn't compared.
func outdatedNote(p *report.OutdatedPin) string {
	where := fmt.Sprintf("%s %s (%s)", p.Tool, p.Pin.Value, PinLocation(p.Pin.Location))
	switch {
	case p.Status == report.StatusUnknown || p.Status == report.StatusError:
		return where + ": " + p.Note
	case p.Via != "":
		return where + ": " + p.Via
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
