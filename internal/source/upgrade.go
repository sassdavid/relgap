package source

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/sassdavid/relgap/internal/htmlmd"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Headings that upgrade documents are filed under. The classifier reads both as breaking.
const (
	HeadingUpgradeGuide = "Upgrade guide"
	HeadingUpgradeNotes = "Upgrade notes"
)

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	blankLines  = regexp.MustCompile(`\n{3,}`)
)

// UpgradeGuide attaches a whole upgrade document to the first version of each new release line
// in a range. A template with {minor} or {prev_minor} names one document per minor transition,
// otherwise one per major.
type UpgradeGuide struct {
	logger *slog.Logger
	files  *mdindex.Cache
	pages  *PageCache
	src    registry.Source
	from   version.V
}

// NewUpgradeGuide returns an upgrade-guide adapter for the registry source s. from is the
// version before the range; it names the line the first transition starts from.
func NewUpgradeGuide(
	logger *slog.Logger,
	files *mdindex.Cache,
	pages *PageCache,
	s registry.Source,
	from version.V,
) *UpgradeGuide {
	return &UpgradeGuide{logger: logger, files: files, pages: pages, src: s, from: from}
}

// Kind returns [KindUpgradeGuide].
func (a *UpgradeGuide) Kind() string { return KindUpgradeGuide }

// Fetch returns the guide of each line transition in the range. Transitions without a document
// are left out.
func (a *UpgradeGuide) Fetch(
	ctx context.Context,
	t registry.ToolSpec,
	versions []version.Version,
) ([]VersionNotes, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("upgrade-guide: %w", err)
	}
	s := rules.Scheme
	tmpl := a.src.PathTemplate
	if tmpl == "" {
		tmpl = a.src.URLTemplate
	}
	level := version.LineMajor
	if strings.Contains(tmpl, registry.PlaceholderMinor) || strings.Contains(tmpl, registry.PlaceholderPrevMinor) {
		level = version.LineMinor
	}
	var out []VersionNotes
	prev := a.from
	for _, v := range versions {
		last := prev
		prev = v.V
		if last.IsZero() || s.Line(last, level) == s.Line(v.V, level) {
			continue
		}
		n, ok, getErr := a.guide(ctx, t, s, v, last)
		if getErr != nil {
			return nil, fmt.Errorf("upgrade-guide: %w", getErr)
		}
		if ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func (a *UpgradeGuide) guide(
	ctx context.Context,
	t registry.ToolSpec,
	s version.Scheme,
	v version.Version,
	prev version.V,
) (VersionNotes, bool, error) {
	md, where, err := a.document(ctx, t, s, v, prev)
	if missing(err) {
		a.logger.DebugContext(ctx, "upgrade guide not found", "tool", t.Name, "url", where)
		return VersionNotes{}, false, nil
	}
	if err != nil || strings.TrimSpace(md) == "" {
		return VersionNotes{}, false, err
	}
	// The document's own headings, title included, move under one Upgrade guide heading.
	// Comments are notes to the guide's authors.
	md = blankLines.ReplaceAllString(htmlComment.ReplaceAllString(md, ""), "\n\n")
	return VersionNotes{
		Version:  v,
		Markdown: "# " + HeadingUpgradeGuide + "\n\n" + string(mdindex.Demote([]byte(md))),
		Source:   Ref{Adapter: KindUpgradeGuide, URL: where},
	}, true, nil
}

// document returns the guide's whole text from the repository file or the web page, and where
// it is.
func (a *UpgradeGuide) document(
	ctx context.Context,
	t registry.ToolSpec,
	s version.Scheme,
	v version.Version,
	prev version.V,
) (string, string, error) {
	if a.src.PathTemplate != "" {
		k := mdindex.Key{
			Repo: t.Repo,
			Ref:  ExpandPrev(a.src.Ref, s, v, prev),
			Path: ExpandPrev(a.src.PathTemplate, s, v, prev),
		}
		idx, err := a.files.Get(ctx, k, mdindex.Options{Scheme: s})
		if err != nil {
			return "", FileURL(k), err //nolint:wrapcheck // the fetch names the file
		}
		return idx.Whole(), FileURL(k), nil
	}
	where := expandURL(a.src.URLTemplate, s, v, prev)
	doc, err := a.pages.Get(ctx, where)
	if err != nil {
		return "", where, err //nolint:wrapcheck // the fetch names the page
	}
	page, err := doc.Markdown(ctx, htmlmd.Options{Content: a.src.Content, Strip: a.src.Strip})
	if err != nil {
		return "", where, err //nolint:wrapcheck // Markdown names the page
	}
	md, err := wholeDocument([]byte(page), s)
	return md, where, err
}

// UpgradeNotes takes the section under a heading such as "Urgent Upgrade Notes" out of each
// version's changelog section, so those entries carry their own source and are classified as
// upgrade notes. It reads the file of the tool's changelog-file source.
type UpgradeNotes struct {
	logger *slog.Logger
	cache  *mdindex.Cache
	src    registry.Source
	file   *registry.Source
}

// NewUpgradeNotes returns an upgrade-notes adapter for the registry source s that reads the
// changelog file source file names; nil means the tool has none.
func NewUpgradeNotes(
	logger *slog.Logger,
	cache *mdindex.Cache,
	s registry.Source,
	file *registry.Source,
) *UpgradeNotes {
	return &UpgradeNotes{logger: logger, cache: cache, src: s, file: file}
}

// Kind returns [KindUpgradeNotes].
func (a *UpgradeNotes) Kind() string { return KindUpgradeNotes }

// Fetch returns the upgrade notes of each version that has them, in the order given.
func (a *UpgradeNotes) Fetch(
	ctx context.Context,
	t registry.ToolSpec,
	versions []version.Version,
) ([]VersionNotes, error) {
	if a.file == nil {
		a.logger.DebugContext(ctx, "upgrade-notes needs a changelog-file source", "tool", t.Name)
		return nil, nil
	}
	notes, err := NewChangelogFile(a.logger, a.cache, *a.file).Fetch(ctx, t, versions)
	if err != nil {
		return nil, fmt.Errorf("upgrade-notes: %w", err)
	}
	want := strings.ToLower(strings.TrimSpace(a.src.FromHeading))
	match := func(h string) bool {
		h = strings.ToLower(h)
		return h == want || strings.HasPrefix(h, want+" ")
	}
	var out []VersionNotes
	for _, n := range notes {
		body, heading, line, ok := mdindex.Subsection([]byte(n.Markdown), match)
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		md := "# " + HeadingUpgradeNotes + "\n\n## " + heading + "\n\n"
		// Line points at the file line of the body's first line, past the two added headings.
		start := 0
		if n.Line > 0 {
			start = max(n.Line+line-strings.Count(md, "\n"), 0)
		}
		out = append(out, VersionNotes{
			Version:  n.Version,
			Markdown: md + body,
			Line:     start,
			Source:   Ref{Adapter: KindUpgradeNotes, URL: n.Source.URL, Heading: n.Source.Heading},
		})
	}
	return out, nil
}
