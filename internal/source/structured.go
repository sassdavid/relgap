package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// StructuredJSON reads one JSON document per version, from a repository file or an allowed URL,
// and turns its entries into a Markdown list grouped by a heading field.
type StructuredJSON struct {
	logger *slog.Logger
	files  FileFetcher
	web    WebFetcher
	src    registry.Source
}

// NewStructuredJSON returns a structured-json adapter for the registry source s.
func NewStructuredJSON(logger *slog.Logger, files FileFetcher, web WebFetcher, s registry.Source) *StructuredJSON {
	return &StructuredJSON{logger: logger, files: files, web: web, src: s}
}

// Kind returns [KindStructuredJSON].
func (a *StructuredJSON) Kind() string { return KindStructuredJSON }

// Fetch returns the entries of each version, in the order given. A missing document or one
// without entries is left out.
func (a *StructuredJSON) Fetch(
	ctx context.Context,
	t registry.ToolSpec,
	versions []version.Version,
) ([]VersionNotes, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("structured-json: %w", err)
	}
	if a.src.JSON == nil {
		return nil, fmt.Errorf("structured-json: %s has no json mapping", t.Name)
	}
	var out []VersionNotes
	for _, v := range versions {
		b, where, getErr := a.get(ctx, t, rules.Scheme, v)
		if missing(getErr) {
			a.logger.DebugContext(ctx, "structured-json document not found", "tool", t.Name, "url", where)
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("structured-json: %w", getErr)
		}
		md, convErr := jsonMarkdown(b, *a.src.JSON)
		if convErr != nil {
			return nil, fmt.Errorf("structured-json: %s: %w", where, convErr)
		}
		if md == "" {
			continue
		}
		out = append(out, VersionNotes{Version: v, Markdown: md, Source: Ref{Adapter: KindStructuredJSON, URL: where}})
	}
	return out, nil
}

func (a *StructuredJSON) get(
	ctx context.Context,
	t registry.ToolSpec,
	s version.Scheme,
	v version.Version,
) ([]byte, string, error) {
	if a.src.PathTemplate != "" {
		k := mdindex.Key{Repo: t.Repo, Ref: Expand(a.src.Ref, s, v), Path: Expand(a.src.PathTemplate, s, v)}
		b, err := a.files.File(ctx, k.Repo, k.Ref, k.Path)
		return b, FileURL(k), err //nolint:wrapcheck // File names the file in its errors
	}
	u := expandURL(a.src.URLTemplate, s, v, version.V{})
	if a.web == nil {
		return nil, u, errNoWeb
	}
	b, err := a.web.Get(ctx, u, "application/json")
	return b, u, err //nolint:wrapcheck // Get names the URL in its errors
}

// expandURL fills a URL template, escaping the tag as a path segment. A "+" is escaped too,
// since some servers read it as a space (Adoptium release names such as jdk-21.0.9+10).
func expandURL(tmpl string, s version.Scheme, v version.Version, prev version.V) string {
	ev := version.Version{V: v.V, Tag: strings.ReplaceAll(url.PathEscape(v.Tag), "+", "%2B")}
	return ExpandPrev(tmpl, s, ev, prev)
}

var (
	jsonField = regexp.MustCompile(`\{([A-Za-z0-9_.-]+)\}`)
	rstCode   = regexp.MustCompile("``([^`]+)``")
)

// jsonMarkdown turns the entries of a JSON document into Markdown: a heading per value of the
// heading field, in order of first appearance, and a bullet per entry.
func jsonMarkdown(b []byte, m registry.JSONMap) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	items, ok := lookup(doc, m.Items).([]any)
	if !ok {
		return "", fmt.Errorf("json path %q is not an array", m.Items)
	}
	var order []string
	groups := map[string][]string{}
	for _, it := range items {
		h, text, isEntry := jsonEntry(it, m)
		if !isEntry {
			continue
		}
		if _, seen := groups[h]; !seen {
			order = append(order, h)
		}
		groups[h] = append(groups[h], text)
	}
	var sb strings.Builder
	for _, h := range order {
		if h != "" {
			sb.WriteString("### " + h + "\n\n")
		}
		for _, text := range groups[h] {
			sb.WriteString("- " + text + "\n")
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n") + newlineIf(sb.Len() > 0), nil
}

// jsonEntry returns the heading and Markdown text of one JSON entry; the last result is false for
// an entry that isn't an object or has no text.
func jsonEntry(item any, m registry.JSONMap) (string, string, bool) {
	obj, ok := item.(map[string]any)
	if !ok {
		return "", "", false
	}
	text := jsonField.ReplaceAllStringFunc(m.Text, func(p string) string {
		return scalar(lookup(obj, p[1:len(p)-1]))
	})
	text = strings.Join(strings.Fields(rstCode.ReplaceAllString(text, "`$1`")), " ")
	if strings.Trim(text, " :()") == "" {
		return "", "", false
	}
	if m.Link != "" {
		if link := scalar(lookup(obj, m.Link)); strings.HasPrefix(link, "https://") {
			text += " ([link](" + link + "))"
		}
	}
	h := ""
	if m.Heading != "" {
		h = strings.TrimSpace(scalar(lookup(obj, m.Heading)))
	}
	return h, text, true
}

func newlineIf(ok bool) string {
	if ok {
		return "\n"
	}
	return ""
}

// lookup follows a dotted path of object keys; an empty path returns v.
func lookup(v any, path string) any {
	if path == "" {
		return v
	}
	for key := range strings.SplitSeq(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[key]
	}
	return v
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}
