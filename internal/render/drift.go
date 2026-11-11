package render

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
)

const (
	outlierMark  = "*"
	latestRow    = "latest"
	legendApprox = ApproxMark + " allows more than one version. A constraint counts as an exact pin it allows " +
		"in the same repo, else as its lower bound."
	legendOutlier = outlierMark + " differs from the version most repos pin."
)

// DriftOptions says how to lay out the drift matrix.
type DriftOptions struct {
	All   bool // a column for every tool, drifting or not
	Color bool // yellow outliers (the tty format)
}

// DriftText writes the matrix (a row per repo, a column per drifting tool), the tools left out
// and a summary.
func DriftText(w io.Writer, d *report.Drift, o DriftOptions) error {
	rows, footer := driftRows(d, o.All)
	var b strings.Builder
	if rows != nil {
		writeAligned(&b, rows, o.Color)
		b.WriteString("\n" + legendApprox + "\n" + legendOutlier + "\n")
	}
	for _, f := range footer {
		b.WriteString(f + "\n")
	}
	b.WriteString(driftSummary(d) + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write drift: %w", err)
	}
	return nil
}

// DriftMarkdown writes the matrix as a Markdown table, the tools left out as a list and a summary.
func DriftMarkdown(w io.Writer, d *report.Drift, o DriftOptions) error {
	rows, footer := driftRows(d, o.All)
	var b strings.Builder
	if rows != nil {
		for _, r := range rows {
			for i := range r {
				r[i].text = mdEscapeStar(r[i].text)
			}
		}
		writeMarkdownTable(&b, rows, strings.Repeat("| --- ", len(rows[0]))+"|")
		b.WriteString("\n" + legendApprox + "\n\n" + mdEscapeStar(legendOutlier) + "\n")
	}
	if len(footer) > 0 {
		b.WriteString("\n")
		for _, f := range footer {
			b.WriteString("- " + mdCell(f) + "\n")
		}
	}
	b.WriteString("\n" + driftSummary(d) + "\n")
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write drift: %w", err)
	}
	return nil
}

// mdEscapeStar keeps the outlier mark from starting a list item or emphasis in Markdown.
func mdEscapeStar(s string) string { return strings.ReplaceAll(s, outlierMark, `\`+outlierMark) }

// driftRows returns the matrix rows (nil without columns) and a line each for the tools in
// agreement and the tools with nothing to compare.
func driftRows(d *report.Drift, all bool) ([][]cell, []string) {
	var cols []*report.DriftTool
	var agree, uncompared []string
	latest := false
	for i := range d.Tools {
		t := &d.Tools[i]
		latest = latest || t.Latest != "" || t.Note != ""
		switch {
		case all || t.Drift:
			cols = append(cols, t)
		case len(t.Versions) == 1:
			agree = append(agree, agreeing(t))
		default:
			uncompared = append(uncompared, notCompared(t))
		}
	}
	var footer []string
	if len(agree) > 0 {
		footer = append(footer, "In agreement: "+strings.Join(agree, ", "))
	}
	if len(uncompared) > 0 {
		footer = append(footer, "Not compared: "+strings.Join(uncompared, ", "))
	}
	if len(cols) == 0 {
		return nil, footer
	}
	header := []cell{{text: "REPO"}}
	for _, t := range cols {
		header = append(header, cell{text: t.Tool})
	}
	rows := [][]cell{header}
	if latest {
		row := []cell{{text: latestRow}}
		for _, t := range cols {
			row = append(row, latestCell(t))
		}
		rows = append(rows, row)
	}
	for _, repo := range d.Repos {
		row := []cell{{text: repo}}
		used := false
		for _, t := range cols {
			c := matrixCell(t.Cell(repo))
			used = used || c.text != noValue
			row = append(row, c)
		}
		if used {
			rows = append(rows, row)
		}
	}
	return rows, footer
}

func latestCell(t *report.DriftTool) cell {
	switch {
	case t.Err != nil:
		return cell{text: "error", style: ansiRed}
	case t.Latest == "":
		return cell{text: noValue}
	default:
		return cell{text: t.Latest, style: ansiGreen}
	}
}

// matrixCell joins a cell's values, marking approximate ones with ≈ and outliers with *. The cell
// is yellow when any value is an outlier.
func matrixCell(c report.Cell) cell {
	if len(c.Values) == 0 {
		return cell{text: noValue}
	}
	parts := make([]string, len(c.Values))
	out := cell{}
	for i, v := range c.Values {
		text := v.Text
		if v.Approx {
			text = ApproxMark + text
		}
		if v.Outlier {
			text += outlierMark
			out.style = ansiYellow
		}
		parts[i] = text
	}
	out.text = strings.Join(parts, ", ")
	return out
}

func agreeing(t *report.DriftTool) string {
	v := t.Versions[0]
	approx := true
	for _, p := range t.Pins {
		if p.Version == v.Version && p.Match == scan.MatchExact {
			approx = false
			break
		}
	}
	text := v.Version
	if approx {
		text = ApproxMark + text
	}
	detail := fmt.Sprintf("%d %s", len(v.Repos), pinsWord(len(v.Repos), "repo", "repos"))
	if t.Latest != "" {
		detail += ", latest " + t.Latest
	}
	return fmt.Sprintf("%s %s (%s)", t.Tool, text, detail)
}

func notCompared(t *report.DriftTool) string {
	var values []string
	repos := map[string]bool{}
	for _, p := range t.Pins {
		if !slices.Contains(values, ApproxMark+p.Value) {
			values = append(values, ApproxMark+p.Value)
		}
		repos[p.Location.Repo] = true
	}
	detail := fmt.Sprintf("%d %s", len(repos), pinsWord(len(repos), "repo", "repos"))
	if t.Latest != "" {
		detail += ", latest " + t.Latest
	}
	return fmt.Sprintf("%s %s (%s)", t.Tool, strings.Join(values, " "), detail)
}

func driftSummary(d *report.Drift) string {
	n := len(d.Drifting())
	return fmt.Sprintf("%d of %d %s across %d %s.", n, len(d.Tools),
		pinsWord(len(d.Tools), "tool drifts", "tools drift"), len(d.Repos), pinsWord(len(d.Repos), "repo", "repos"))
}
