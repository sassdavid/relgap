package render

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/sassdavid/relgap/internal/scan"
)

// ApproxMark prefixes a pin value that allows more than one version.
const ApproxMark = "≈"

// PinsText writes pins as an aligned table. unresolved swaps the TOOL column for RAW NAME.
func PinsText(w io.Writer, pins []scan.Pin, unresolved bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for _, row := range pinRows(pins, unresolved) {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write pins: %w", err)
	}
	return nil
}

// PinsMarkdown writes pins as a Markdown table.
func PinsMarkdown(w io.Writer, pins []scan.Pin, unresolved bool) error {
	var b strings.Builder
	for i, row := range pinRows(pins, unresolved) {
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

func pinRows(pins []scan.Pin, unresolved bool) [][]string {
	first := "TOOL"
	if unresolved {
		first = "RAW NAME"
	}
	rows := [][]string{{first, "VALUE", "DETECTOR", "LOCATION"}}
	for _, p := range pins {
		name := p.Tool
		if unresolved {
			name = p.RawName
		} else if name == "" {
			name = "-"
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
