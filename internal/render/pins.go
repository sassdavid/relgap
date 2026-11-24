package render

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/term"
)

// ApproxMark prefixes a pin value that allows more than one version.
const ApproxMark = "≈"

// PinsText writes pins as a table, pins with no known tool faint. unresolved swaps the TOOL
// column for RAW NAME.
func PinsText(w io.Writer, pins []scan.Pin, unresolved bool) error {
	rows := pinRows(pins, unresolved, plain)
	body := make([][]term.Cell, len(rows)-1)
	for i, r := range rows[1:] {
		body[i] = make([]term.Cell, len(r))
		for j, s := range r {
			body[i][j] = term.Cell{Text: s}
		}
		if r[0] == noValue {
			body[i][0].Tone = term.Muted
		}
	}
	if _, err := io.WriteString(w, term.Table(rows[0], body)); err != nil {
		return fmt.Errorf("write pins: %w", err)
	}
	return nil
}

// PinsMarkdown writes pins as a Markdown table.
func PinsMarkdown(w io.Writer, pins []scan.Pin, unresolved bool) error {
	var b strings.Builder
	for i, row := range pinRows(pins, unresolved, mdCode) {
		for j := range row {
			row[j] = strings.ReplaceAll(row[j], "|", `\|`)
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
		if i == 0 {
			b.WriteString(strings.Repeat("| --- ", len(row)) + "|\n")
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write pins: %w", err)
	}
	return nil
}

func pinRows(pins []scan.Pin, unresolved bool, code func(string) string) [][]string {
	first := headTool
	if unresolved {
		first = "RAW NAME"
	}
	rows := [][]string{{first, "VALUE", "DETECTOR", "LOCATION"}}
	for _, p := range pins {
		name := toolName(p.Tool)
		if unresolved {
			name = p.RawName
		} else if name == "" {
			name = "-"
		}
		if name != "-" {
			name = code(name)
		}
		value := p.Value
		if !p.Exact {
			value = ApproxMark + value
		}
		rows = append(rows, []string{name, value, p.Location.Detector, PinLocation(p.Location)})
	}
	return rows
}

// PinLocation formats a location as repo/file:line, leaving out a "." repo and a zero line.
func PinLocation(l scan.Location) string {
	s := path.Join(l.Repo, l.File)
	if l.Line > 0 {
		s += ":" + strconv.Itoa(l.Line)
	}
	return s
}
