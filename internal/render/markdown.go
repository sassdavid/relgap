package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/report"
)

// DigestMarkdown writes d as PR-ready Markdown: counts, then breaking changes and security
// entries of every version, then one <details> block per version with the other kinds.
func DigestMarkdown(w io.Writer, d *report.Digest) error {
	mw := &textWriter{w: w}
	for i, t := range d.Tools {
		if i > 0 {
			mw.line("")
		}
		markdownTool(mw, &t)
	}
	if mw.err != nil {
		return fmt.Errorf("write digest: %w", mw.err)
	}
	return nil
}

func markdownTool(mw *textWriter, t *report.ToolDigest) {
	markdownSummary(mw, t)
	for _, k := range []classify.Kind{classify.KindBreaking, classify.KindSecurity} {
		if t.Counts[k] == 0 {
			continue
		}
		mw.line("")
		mw.line("### " + kindTitle(k))
		mw.line("")
		for _, e := range t.Entries {
			if e.Kind == k {
				mw.entry(e, "**"+e.Version+"** ")
			}
		}
	}
	for _, v := range t.Versions {
		markdownVersion(mw, v, t.Entries)
	}
}

func markdownSummary(mw *textWriter, t *report.ToolDigest) {
	mw.line(fmt.Sprintf("## %s %s..%s", t.Tool, t.From, t.To))
	mw.line("")
	versions := make([]string, len(t.Versions))
	for i, v := range t.Versions {
		versions[i] = v.Version
		if v.URL != "" {
			versions[i] = fmt.Sprintf("[%s](%s)", v.Version, v.URL)
		}
	}
	mw.line(fmt.Sprintf("Repository: [%s](https://github.com/%s). %d versions: %s.",
		t.Repo, t.Repo, len(t.Versions), strings.Join(versions, ", ")))
	mw.line("")
	counts := make([]string, 0, len(classify.Kinds()))
	for _, k := range classify.Kinds() {
		counts = append(counts, fmt.Sprintf("%s %d", k, t.Counts[k]))
	}
	summary := strings.Join(counts, ", ")
	if t.Hidden > 0 {
		summary += fmt.Sprintf("; %d below --min-confidence hidden", t.Hidden)
	}
	mw.line(summary)
	for _, warn := range t.Warnings {
		mw.line("")
		mw.line("> [!WARNING]")
		mw.line("> " + warn)
	}
}

// markdownVersion writes a <details> block with the version's entries other than breaking
// changes and security, which are listed at the top. A version without such entries is skipped.
func markdownVersion(mw *textWriter, v report.Version, entries []classify.Entry) {
	var rest []classify.Entry
	for _, e := range entries {
		if e.Version == v.Version && e.Kind != classify.KindBreaking && e.Kind != classify.KindSecurity {
			rest = append(rest, e)
		}
	}
	if len(rest) == 0 {
		return
	}
	title := v.Version
	if v.PublishedAt != nil {
		title += " (" + v.PublishedAt.Format("2006-01-02") + ")"
	}
	mw.line("")
	mw.line("<details><summary>" + title + "</summary>")
	for _, k := range classify.Kinds() {
		first := true
		for _, e := range rest {
			if e.Kind != k {
				continue
			}
			if first {
				mw.line("")
				mw.line("#### " + kindTitle(k))
				mw.line("")
				first = false
			}
			mw.entry(e, "")
		}
	}
	mw.line("")
	mw.line("</details>")
}

// entry writes e as a list item with a link to its source. A multi-line entry gets the link on
// its own line, so it can't land inside a code block.
func (tw *textWriter) entry(e classify.Entry, prefix string) {
	link := ""
	if e.Source.URL != "" {
		link = "([source](" + e.Source.URL + "))"
	}
	lines := strings.Split(e.Text, "\n")
	if len(lines) == 1 {
		tw.line(strings.TrimRight("- "+prefix+lines[0]+" "+link, " "))
		return
	}
	tw.line("- " + prefix + lines[0])
	for _, l := range lines[1:] {
		tw.line(strings.TrimRight("  "+l, " "))
	}
	if link != "" {
		tw.line("")
		tw.line("  " + link)
	}
}
