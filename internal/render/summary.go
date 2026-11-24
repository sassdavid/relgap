package render

import (
	"fmt"
	"strings"

	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/term"
)

// sourceURLs maps each entry ID of d to its source URL.
func sourceURLs(d *report.Digest) map[string]string {
	urls := map[string]string{}
	for _, t := range d.Tools {
		for _, e := range t.Entries {
			urls[e.ID] = e.Source.URL
		}
	}
	return urls
}

func summaryNote(s *report.Summary) string {
	note := fmt.Sprintf("%s. Model: %s %s.", s.Notice, s.Provider, s.Model)
	if s.Trimmed > 0 {
		note += fmt.Sprintf(" %d of %d entries were left out to fit the input limit.", s.Trimmed, s.Trimmed+s.Entries)
	}
	if s.Dropped > 0 {
		note += fmt.Sprintf(" %d %s without a known entry ID removed.", s.Dropped, pluralClaims(s.Dropped))
	}
	return note
}

func pluralClaims(n int) string {
	if n == 1 {
		return "claim"
	}
	return "claims"
}

// summary writes the summary block of the tty and plain formats.
func (tw *textWriter) summary(s *report.Summary) {
	tw.line(term.Heading.Paint("Upgrade risk summary"))
	tw.line(summaryNote(s))
	for _, sec := range summarySections(s) {
		tw.line("")
		tw.line(term.Heading.Paint(sec.title))
		for _, c := range sec.claims {
			ids := make([]string, len(c.Cites))
			for i, id := range c.Cites {
				ids[i] = "[" + id + "]"
			}
			tw.line(term.Wrap(term.Sanitize(c.Text+" "+strings.Join(ids, " ")), tw.width, "  - ", "    "))
		}
	}
}

// summaryMarkdown writes the summary block of the md format, each cited ID linked to its source.
func (tw *textWriter) summaryMarkdown(s *report.Summary, urls map[string]string) {
	tw.line("## Upgrade risk summary")
	tw.line("")
	tw.line("> [!NOTE]")
	tw.line("> " + summaryNote(s))
	for _, sec := range summarySections(s) {
		tw.line("")
		tw.line("### " + sec.title)
		tw.line("")
		for _, c := range sec.claims {
			ids := make([]string, len(c.Cites))
			for i, id := range c.Cites {
				ids[i] = "`" + id + "`"
				if u := urls[id]; u != "" {
					ids[i] = "[`" + id + "`](" + u + ")"
				}
			}
			tw.line("- " + silence(c.Text) + " (" + strings.Join(ids, ", ") + ")")
		}
	}
}

type summarySection struct {
	title  string
	claims []report.Claim
}

func summarySections(s *report.Summary) []summarySection {
	var out []summarySection
	if len(s.Risks) > 0 {
		out = append(out, summarySection{"Risks", s.Risks})
	}
	if len(s.Actions) > 0 {
		out = append(out, summarySection{"Required actions", s.Actions})
	}
	return out
}
