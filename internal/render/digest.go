package render

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/report"
)

const (
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

// DigestText writes d as plain text, breaking changes first and entries grouped by version.
// With color it adds ANSI bold headings and red breaking changes (the tty format).
func DigestText(w io.Writer, d *report.Digest, color bool) error {
	tw := &textWriter{w: w, color: color}
	for i, t := range d.Tools {
		if i > 0 {
			tw.line("")
		}
		tw.tool(&t)
	}
	if tw.err != nil {
		return fmt.Errorf("write digest: %w", tw.err)
	}
	return nil
}

func kindTitle(k classify.Kind) string {
	switch k {
	case classify.KindBreaking:
		return "Breaking changes"
	case classify.KindDeprecation:
		return "Deprecations"
	case classify.KindSecurity:
		return "Security"
	case classify.KindFeature:
		return "Features"
	case classify.KindFix:
		return "Fixes"
	case classify.KindOther:
		return "Other"
	default:
		return string(k)
	}
}

type textWriter struct {
	w     io.Writer
	color bool
	err   error
}

func (tw *textWriter) line(s string) {
	if tw.err == nil {
		_, tw.err = io.WriteString(tw.w, s+"\n")
	}
}

func (tw *textWriter) style(s string, codes ...string) string {
	if !tw.color {
		return s
	}
	return strings.Join(codes, "") + s + ansiReset
}

func (tw *textWriter) tool(t *report.ToolDigest) {
	tw.line(tw.style(fmt.Sprintf("%s %s..%s", t.Tool, t.From, t.To), ansiBold) + "  " + t.Repo)
	versions := make([]string, len(t.Versions))
	for i, v := range t.Versions {
		versions[i] = v.Version
	}
	tw.line(fmt.Sprintf("%d versions: %s", len(t.Versions), strings.Join(versions, ", ")))
	counts := make([]string, 0, len(classify.Kinds()))
	for _, k := range classify.Kinds() {
		counts = append(counts, fmt.Sprintf("%s %d", k, t.Counts[k]))
	}
	summary := strings.Join(counts, ", ")
	if t.Hidden > 0 {
		summary += fmt.Sprintf("; %d below --min-confidence hidden", t.Hidden)
	}
	tw.line(summary)
	for _, warn := range t.Warnings {
		tw.line("warning: " + warn)
	}
	urls := map[string]string{}
	for _, v := range t.Versions {
		urls[v.Version] = v.URL
	}
	for _, k := range classify.Kinds() {
		if !listed(t.Entries, k) {
			continue
		}
		tw.line("")
		codes := []string{ansiBold}
		if k == classify.KindBreaking {
			codes = append(codes, ansiRed)
		}
		tw.line(tw.style(kindTitle(k), codes...))
		tw.entries(t.Entries, k, urls)
	}
}

// listed reports whether entries has one of kind k; with --only a kind can have a count and none.
func listed(entries []classify.Entry, k classify.Kind) bool {
	return slices.ContainsFunc(entries, func(e classify.Entry) bool { return e.Kind == k })
}

func (tw *textWriter) entries(entries []classify.Entry, k classify.Kind, urls map[string]string) {
	last := ""
	for _, e := range entries {
		if e.Kind != k {
			continue
		}
		if e.Version != last {
			last = e.Version
			header := "  " + e.Version
			if u := urls[e.Version]; u != "" {
				header += "  " + u
			}
			tw.line(header)
		}
		for i, l := range strings.Split(e.Text, "\n") {
			prefix := "      "
			if i == 0 {
				prefix = "    - "
			}
			tw.line(strings.TrimRight(prefix+l, " "))
		}
	}
}
