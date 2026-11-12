package source

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/sassdavid/relgap/internal/htmlmd"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// PageCache fetches and parses each web page once. It is safe for concurrent use and lives as
// long as a digester.
type PageCache struct {
	web WebFetcher

	mu    sync.Mutex
	pages map[string]*page
}

type page struct {
	once sync.Once
	doc  *htmlmd.Document
	err  error
}

// NewPageCache returns a cache that fetches pages with web.
func NewPageCache(web WebFetcher) *PageCache {
	return &PageCache{web: web, pages: map[string]*page{}}
}

// Get returns the parsed page at rawURL. A failed fetch is remembered.
func (c *PageCache) Get(ctx context.Context, rawURL string) (*htmlmd.Document, error) {
	c.mu.Lock()
	p, ok := c.pages[rawURL]
	if !ok {
		p = &page{}
		c.pages[rawURL] = p
	}
	c.mu.Unlock()
	p.once.Do(func() {
		if c.web == nil {
			p.err = errNoWeb
			return
		}
		var b []byte
		if b, p.err = c.web.Get(ctx, rawURL, "text/html"); p.err == nil {
			p.doc, p.err = htmlmd.Parse(b, rawURL)
		}
	})
	return p.doc, p.err
}

// WebDoc reads release notes from allowed web pages. With an anchor, each version gets the
// element with its id. Without one, a page holds a whole release line, such as one minor, and
// goes to the first version of the range whose URL is that page.
type WebDoc struct {
	logger *slog.Logger
	pages  *PageCache
	src    registry.Source
	from   version.V
}

// NewWebDoc returns a web-doc adapter for the registry source s. from is the version before
// the range, whose page the range doesn't repeat.
func NewWebDoc(logger *slog.Logger, pages *PageCache, s registry.Source, from version.V) *WebDoc {
	return &WebDoc{logger: logger, pages: pages, src: s, from: from}
}

// Kind returns [KindWebDoc].
func (a *WebDoc) Kind() string { return KindWebDoc }

// Fetch returns the notes of each version, in the order given.
func (a *WebDoc) Fetch(ctx context.Context, t registry.ToolSpec, versions []version.Version) ([]VersionNotes, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("web-doc: %w", err)
	}
	s := rules.Scheme
	opts := htmlmd.Options{Content: a.src.Content, Strip: a.src.Strip}
	prevURL := ""
	if !a.from.IsZero() {
		prevURL = expandURL(a.src.URLTemplate, s, version.Version{V: a.from}, version.V{})
	}
	var out []VersionNotes
	for _, v := range versions {
		u := expandURL(a.src.URLTemplate, s, v, version.V{})
		if a.src.Anchor == "" && u == prevURL {
			continue
		}
		prevURL = u
		doc, getErr := a.pages.Get(ctx, u)
		if missing(getErr) {
			a.logger.DebugContext(ctx, "web page not found", "tool", t.Name, "url", u)
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("web-doc: %w", getErr)
		}
		n, ok, convErr := a.notes(ctx, doc, s, v, u, opts)
		if convErr != nil {
			return nil, fmt.Errorf("web-doc: %w", convErr)
		}
		if ok {
			out = append(out, n)
		}
	}
	return out, nil
}

func (a *WebDoc) notes(
	ctx context.Context,
	doc *htmlmd.Document,
	s version.Scheme,
	v version.Version,
	u string,
	opts htmlmd.Options,
) (VersionNotes, bool, error) {
	if a.src.Anchor != "" {
		id := Expand(a.src.Anchor, s, v)
		md, ok, err := doc.Anchor(ctx, id, opts)
		if err != nil || !ok || md == "" {
			return VersionNotes{}, false, err //nolint:wrapcheck // Anchor names the page
		}
		return VersionNotes{Version: v, Markdown: md, Source: Ref{Adapter: KindWebDoc, URL: u + "#" + id}}, true, nil
	}
	md, err := doc.Markdown(ctx, opts)
	if err != nil {
		return VersionNotes{}, false, err //nolint:wrapcheck // Markdown names the page
	}
	whole, err := wholeDocument([]byte(md), s)
	if err != nil || whole == "" {
		return VersionNotes{}, false, err
	}
	return VersionNotes{Version: v, Markdown: whole, Source: Ref{Adapter: KindWebDoc, URL: u}}, true, nil
}

// wholeDocument indexes a converted page once (ADR-0001) and returns its cleaned text.
func wholeDocument(src []byte, s version.Scheme) (string, error) {
	x, err := mdindex.Parse(src, mdindex.Options{Scheme: s})
	if err != nil {
		return "", fmt.Errorf("index document: %w", err)
	}
	return x.Whole(), nil
}
