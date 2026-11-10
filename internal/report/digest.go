package report

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/github"
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
	logger     *slog.Logger
	gh         GitHub
	classifier *classify.Classifier
}

// NewDigester returns a digester that reads from gh.
func NewDigester(logger *slog.Logger, gh GitHub) *Digester {
	return &Digester{logger: logger, gh: gh, classifier: classify.New()}
}

// Digest lists the tool's versions, selects the range and classifies the notes of each version.
func (d *Digester) Digest(ctx context.Context, req DigestRequest) (*ToolDigest, error) {
	t := req.Tool
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs, releases, err := d.refs(ctx, t)
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
	for _, a := range d.adapters(ctx, t) {
		notes, fetchErr := a.Fetch(ctx, t, r.Versions)
		if fetchErr != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, fetchErr)
		}
		for _, n := range notes {
			for _, e := range d.classifier.Entries(n) {
				if e.Confidence < req.MinConfidence {
					out.Hidden++
					continue
				}
				out.Entries = append(out.Entries, e)
				out.Counts[e.Kind]++
			}
		}
	}
	return out, nil
}

// refs lists the version universe: releases (drafts included, the universe drops them) or tags.
func (d *Digester) refs(ctx context.Context, t registry.ToolSpec) ([]version.Ref, map[string]github.Release, error) {
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

// adapters returns the adapters for the tool's sources. Without sources it falls back to
// gh-release, the first step of auto-discovery.
func (d *Digester) adapters(ctx context.Context, t registry.ToolSpec) []source.Adapter {
	if len(t.Sources) == 0 {
		return []source.Adapter{source.NewGHRelease(d.gh)}
	}
	var out []source.Adapter
	for _, s := range t.Sources {
		switch s.Kind {
		case source.KindGHRelease:
			out = append(out, source.NewGHRelease(d.gh))
		default:
			d.logger.DebugContext(ctx, "skipping a source kind relgap can't read yet", "tool", t.Name, "kind", s.Kind)
		}
	}
	return out
}

func versionOf(v version.Version, rel github.Release) Version {
	out := Version{Version: v.V.String(), Tag: v.Tag, URL: rel.URL}
	if !v.PublishedAt.IsZero() {
		at := v.PublishedAt.UTC()
		out.PublishedAt = &at
	}
	return out
}
