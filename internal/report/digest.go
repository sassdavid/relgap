package report

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// Digest is the data of `relgap digest --format json`.
type Digest struct {
	Tools []ToolDigest `json:"tools"`
}

// ToolDigest is the digest of one tool over (From, To].
type ToolDigest struct {
	Tool     string                `json:"tool"`
	Repo     string                `json:"repo"`
	From     string                `json:"from"`
	To       string                `json:"to"`
	Versions []Version             `json:"versions"` // oldest first
	Entries  []classify.Entry      `json:"entries"`  // by version, then document order
	Counts   map[classify.Kind]int `json:"counts"`   // entries shown, per kind
	Hidden   int                   `json:"hidden"`   // entries below the minimum confidence
	Warnings []string              `json:"warnings"`
}

// Version is one version in a digested range.
type Version struct {
	Version     string     `json:"version"`
	Tag         string     `json:"tag"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	URL         string     `json:"url,omitempty"` // the GitHub release page, when there is a release
}

// GitHub lists releases and tags and fetches release bodies; [github.Client] implements it.
type GitHub interface {
	Releases(ctx context.Context, repo string) ([]github.Release, error)
	Tags(ctx context.Context, repo string) ([]string, error)
	source.ReleaseFetcher
	source.FileFetcher
}

// DigestRequest says what to digest.
type DigestRequest struct {
	Tool               registry.ToolSpec
	From, To           string // To empty means the latest stable version on the line
	Line               version.Level
	IncludePrereleases bool
	MinConfidence      float64
}

// Digester builds digests from GitHub.
type Digester struct {
	logger *slog.Logger
	gh     GitHub
	files  *mdindex.Cache

	mu   sync.Mutex
	refs map[string]*refList // by tool name
}

// refList is a tool's version universe as listed once per digester.
type refList struct {
	once     sync.Once
	refs     []version.Ref
	releases map[string]github.Release
	err      error
}

// NewDigester returns a digester that reads from gh. Changelog files are fetched and parsed once
// per digester, and so is each tool's list of releases or tags.
func NewDigester(logger *slog.Logger, gh GitHub) *Digester {
	return &Digester{logger: logger, gh: gh, files: source.NewFileCache(gh), refs: map[string]*refList{}}
}

// Universe returns every version of the tool, listed once per digester.
func (d *Digester) Universe(ctx context.Context, t registry.ToolSpec) (*version.Universe, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs, _, err := d.list(ctx, t)
	if err != nil {
		return nil, err
	}
	return version.NewUniverse(ctx, d.logger, refs, rules), nil
}

func (d *Digester) list(ctx context.Context, t registry.ToolSpec) ([]version.Ref, map[string]github.Release, error) {
	d.mu.Lock()
	l, ok := d.refs[t.Name]
	if !ok {
		l = &refList{}
		d.refs[t.Name] = l
	}
	d.mu.Unlock()
	l.once.Do(func() { l.refs, l.releases, l.err = d.fetchRefs(ctx, t) })
	return l.refs, l.releases, l.err
}

// reader is an adapter with the classifier for its source.
type reader struct {
	adapter    source.Adapter
	classifier *classify.Classifier
}

// Digest lists the tool's versions, selects the range and classifies the notes of each version.
func (d *Digester) Digest(ctx context.Context, req DigestRequest) (*ToolDigest, error) {
	t := req.Tool
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs, releases, err := d.list(ctx, t)
	if err != nil {
		return nil, err
	}
	universe := version.NewUniverse(ctx, d.logger, refs, rules)
	r, err := universe.Select(version.RangeOptions{
		From: req.From, To: req.To, Line: req.Line, IncludePrereleases: req.IncludePrereleases,
	})
	if err != nil {
		return nil, fmt.Errorf("select %s range: %w", t.Name, err)
	}
	out := &ToolDigest{
		Tool:     t.Name,
		Repo:     t.Repo,
		From:     r.From.String(),
		To:       r.To.V.String(),
		Versions: make([]Version, 0, len(r.Versions)),
		Entries:  []classify.Entry{},
		Counts: map[classify.Kind]int{
			classify.KindBreaking: 0, classify.KindDeprecation: 0, classify.KindSecurity: 0,
			classify.KindFeature: 0, classify.KindFix: 0, classify.KindOther: 0,
		},
		Warnings: append([]string{}, r.Warnings...),
	}
	for _, v := range r.Versions {
		out.Versions = append(out.Versions, versionOf(v, releases[v.Tag]))
	}
	readers, err := d.readers(ctx, t)
	if err != nil {
		return nil, err
	}
	entries, err := collect(ctx, t, readers, r.Versions)
	if err != nil {
		return nil, err
	}
	urls := map[string]string{}
	order := map[string]int{}
	for i, v := range out.Versions {
		urls[v.Version] = v.URL
		order[v.Version] = i
	}
	entries = append(classify.Jumps(rules.Scheme, r.From, r.Versions, entries, urls), entries...)
	slices.SortStableFunc(
		entries,
		func(a, b classify.Entry) int { return cmp.Compare(order[a.Version], order[b.Version]) },
	)
	for _, e := range entries {
		if e.Confidence < req.MinConfidence {
			out.Hidden++
			continue
		}
		out.Entries = append(out.Entries, e)
		out.Counts[e.Kind]++
	}
	return out, nil
}

// collect runs every reader and drops entries an earlier source already had for the version,
// since a release body often repeats the changelog.
func collect(
	ctx context.Context,
	t registry.ToolSpec,
	readers []reader,
	vs []version.Version,
) ([]classify.Entry, error) {
	var out []classify.Entry
	seen := map[string]bool{}
	for _, rd := range readers {
		notes, err := rd.adapter.Fetch(ctx, t, vs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		for _, n := range notes {
			for _, e := range rd.classifier.Entries(n) {
				key := e.Version + "\x00" + normalize(e.Text)
				if !seen[key] {
					seen[key] = true
					out = append(out, e)
				}
			}
		}
	}
	return out, nil
}

// fetchRefs lists the version universe: releases (drafts included, the universe drops them) or
// tags.
func (d *Digester) fetchRefs(
	ctx context.Context,
	t registry.ToolSpec,
) ([]version.Ref, map[string]github.Release, error) {
	byTag := map[string]github.Release{}
	if t.Version.Universe == registry.UniverseTags {
		tags, err := d.gh.Tags(ctx, t.Repo)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		refs := make([]version.Ref, len(tags))
		for i, tag := range tags {
			refs[i] = version.Ref{Tag: tag}
		}
		return refs, byTag, nil
	}
	releases, err := d.gh.Releases(ctx, t.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs := make([]version.Ref, len(releases))
	for i, rel := range releases {
		refs[i] = version.Ref{Tag: rel.Tag, Draft: rel.Draft, Prerelease: rel.Prerelease, PublishedAt: rel.PublishedAt}
		byTag[rel.Tag] = rel
	}
	return refs, byTag, nil
}

// readers returns an adapter and classifier for each of the tool's sources. Without sources it
// falls back to gh-release, the first step of auto-discovery.
func (d *Digester) readers(ctx context.Context, t registry.ToolSpec) ([]reader, error) {
	sources := t.Sources
	if len(sources) == 0 {
		sources = []registry.Source{{Kind: source.KindGHRelease}}
	}
	var out []reader
	for _, s := range sources {
		var a source.Adapter
		switch s.Kind {
		case source.KindGHRelease:
			a = source.NewGHRelease(d.gh)
		case source.KindChangelogFile:
			a = source.NewChangelogFile(d.logger, d.files, s)
		default:
			d.logger.DebugContext(ctx, "skipping a source kind relgap can't read yet", "tool", t.Name, "kind", s.Kind)
			continue
		}
		c, err := classify.New(classify.Options{BreakingMarkers: t.BreakingMarkers, IgnoreSections: s.IgnoreSections})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		out = append(out, reader{adapter: a, classifier: c})
	}
	return out, nil
}

var (
	issueRef = regexp.MustCompile(`\]\([^)]*\)|#\d+\b|https?://\S+`)
	nonWord  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// normalize reduces an entry to its words without link targets, issue numbers or URLs, so the
// same bullet from two sources compares equal.
func normalize(text string) string {
	text = issueRef.ReplaceAllString(strings.ToLower(text), " ")
	return strings.TrimSpace(nonWord.ReplaceAllString(text, " "))
}

func versionOf(v version.Version, rel github.Release) Version {
	out := Version{Version: v.V.String(), Tag: v.Tag, URL: rel.URL}
	if !v.PublishedAt.IsZero() {
		at := v.PublishedAt.UTC()
		out.PublishedAt = &at
	}
	return out
}
