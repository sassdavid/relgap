package render

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/term"
)

// DigestText writes d as text, breaking changes first and entries grouped by version, with
// entries wrapped at width (0 for no wrapping). A --summarize summary comes first, labelled as
// LLM output. It styles headings bold and breaking changes red; a writer with no color profile
// drops the styles.
func DigestText(w io.Writer, d *report.Digest, width int) error {
	tw := &textWriter{width: width}
	if d.Summary != nil {
		tw.summary(d.Summary)
		tw.line("")
	}
	for i, t := range d.Tools {
		if i > 0 {
			tw.line("")
		}
		tw.tool(&t)
	}
	if _, err := io.WriteString(w, tw.b.String()); err != nil {
		return fmt.Errorf("write digest: %w", err)
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

// textWriter builds a digest in memory, so it reaches the output in one write.
type textWriter struct {
	b     strings.Builder
	width int // columns to wrap entries at in the text formats; 0 means don't wrap
}

func (tw *textWriter) line(s string) {
	tw.b.WriteString(s)
	tw.b.WriteByte('\n')
}

// kindTone is the tone of a kind's heading in the text formats.
func kindTone(k classify.Kind) term.Tone {
	switch k {
	case classify.KindBreaking:
		return term.Severe
	case classify.KindSecurity:
		return term.Bad
	case classify.KindDeprecation:
		return term.Warn
	case classify.KindFeature, classify.KindFix, classify.KindOther:
	}
	return term.Heading
}

func (tw *textWriter) tool(t *report.ToolDigest) {
	tw.line(term.Heading.Paint(fmt.Sprintf("%s %s..%s", t.Tool, t.From, t.To)) + "  " + term.Sanitize(t.Repo))
	versions := make([]string, len(t.Versions))
	for i, v := range t.Versions {
		versions[i] = v.Version
	}
	tw.line(fmt.Sprintf("%d %s: %s", len(t.Versions), pinsWord(len(t.Versions), "version", "versions"),
		strings.Join(versions, ", ")))
	counts := make([]string, 0, len(classify.Kinds()))
	for _, k := range classify.Kinds() {
		counts = append(counts, fmt.Sprintf("%s %d", k, t.Counts[k]))
	}
	summary := strings.Join(counts, ", ")
	if t.Hidden > 0 {
		summary += fmt.Sprintf("; %d below --min-confidence hidden", t.Hidden)
	}
	tw.line(term.Wrap(summary, tw.width, "", ""))
	for _, warn := range t.Warnings {
		tw.line(term.Warn.Paint("warning: " + warn))
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
		tw.line(kindTone(k).Paint(kindTitle(k)))
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
			tw.line(term.Sanitize(header))
		}
		for i, l := range strings.Split(term.Sanitize(e.Text), "\n") {
			prefix := "      "
			if i == 0 {
				prefix = "    - "
			}
			tw.line(strings.TrimRight(term.Wrap(l, tw.width, prefix, "      "), " "))
		}
	}
}
