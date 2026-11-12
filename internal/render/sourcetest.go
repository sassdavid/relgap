package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/report"
)

// SourceTestText writes what each source of a registry entry produced as an aligned table,
// followed by the documents, warnings and errors of each source.
func SourceTestText(w io.Writer, t *report.SourceTest) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s) %s..%s: %d %s\n\n", t.Tool, t.Repo, t.From, t.To, len(t.Versions),
		pinsWord(len(t.Versions), "version", "versions"))
	rows := [][]cell{{
		{text: "SOURCE"},
		{text: "VERSIONS"},
		{text: "ENTRIES"},
		{text: "BREAKING"},
		{text: "DEPRECATION"},
		{text: "SECURITY"},
	}}
	all := append(append([]report.SourceResult{}, t.Sources...), t.Merged)
	for _, s := range all {
		rows = append(rows, []cell{
			{text: sourceName(s)},
			{text: fmt.Sprintf("%d/%d", len(s.Versions), len(t.Versions))},
			{text: strconv.Itoa(s.Entries)},
			{text: strconv.Itoa(s.Counts[classify.KindBreaking])},
			{text: strconv.Itoa(s.Counts[classify.KindDeprecation])},
			{text: strconv.Itoa(s.Counts[classify.KindSecurity])},
		})
	}
	writeAligned(&b, rows, false)
	for _, s := range all {
		if len(s.URLs) == 0 && len(s.Warnings) == 0 && s.Error == "" {
			continue
		}
		b.WriteString("\n" + sourceName(s) + ":\n")
		for _, u := range s.URLs {
			b.WriteString("  " + u + "\n")
		}
		for _, warn := range s.Warnings {
			b.WriteString("  warning: " + warn + "\n")
		}
		if s.Error != "" {
			b.WriteString("  error: " + s.Error + "\n")
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write source test: %w", err)
	}
	return nil
}

func sourceName(s report.SourceResult) string {
	if s.Source == nil {
		return "merged"
	}
	return strconv.Itoa(s.Index) + " " + sourceLabel(*s.Source)
}

// sourceLabel names a source by its kind and the template that tells it apart.
func sourceLabel(s registry.Source) string {
	for _, v := range []string{s.PathTemplate, s.URLTemplate, s.Anchor, s.FromHeading} {
		if v != "" {
			return s.Kind + " " + v
		}
	}
	return s.Kind
}
