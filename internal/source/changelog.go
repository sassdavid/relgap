package source

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// FileFetcher returns a file of a repository at a ref; [github.Client] implements it.
type FileFetcher interface {
	File(ctx context.Context, repo, ref, path string) ([]byte, error)
}

// NewFileCache returns an [mdindex.Cache] that reads files through gh.
func NewFileCache(gh FileFetcher) *mdindex.Cache {
	return mdindex.NewCache(func(ctx context.Context, k mdindex.Key) ([]byte, error) {
		return gh.File(ctx, k.Repo, k.Ref, k.Path) //nolint:wrapcheck // File names the file in its errors
	})
}

// ChangelogFile reads version sections from a changelog file in the tool's repository. Each
// file is fetched and parsed once through the shared cache.
type ChangelogFile struct {
	logger *slog.Logger
	cache  *mdindex.Cache
	src    registry.Source
}

// NewChangelogFile returns a changelog-file adapter for the registry source s.
func NewChangelogFile(logger *slog.Logger, cache *mdindex.Cache, s registry.Source) *ChangelogFile {
	return &ChangelogFile{logger: logger, cache: cache, src: s}
}

// Kind returns [KindChangelogFile].
func (a *ChangelogFile) Kind() string { return KindChangelogFile }

// Fetch returns the section of each version, in the order given. A missing file or a version
// without a heading is left out.
func (a *ChangelogFile) Fetch(
	ctx context.Context,
	t registry.ToolSpec,
	versions []version.Version,
) ([]VersionNotes, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("changelog-file: %w", err)
	}
	opts := mdindex.Options{Scheme: rules.Scheme}
	if a.src.Heading != "" {
		if opts.Heading, err = mdindex.HeadingPattern(a.src.Heading); err != nil {
			return nil, fmt.Errorf("changelog-file: %w", err)
		}
	}
	var out []VersionNotes
	for _, v := range versions {
		k := mdindex.Key{
			Repo: t.Repo,
			Ref:  Expand(a.src.Ref, rules.Scheme, v),
			Path: Expand(a.src.PathTemplate, rules.Scheme, v),
		}
		idx, getErr := a.cache.Get(ctx, k, opts)
		if errors.Is(getErr, github.ErrNotFound) {
			a.logger.DebugContext(ctx, "changelog file not found", "repo", k.Repo, "ref", k.Ref, "path", k.Path)
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("changelog-file: %w", getErr)
		}
		sec, ok := idx.Lookup(v.V)
		if !ok {
			continue
		}
		md, line := idx.Markdown(sec)
		if md == "" {
			continue
		}
		out = append(out, VersionNotes{
			Version:  v,
			Markdown: md,
			Line:     line,
			Source:   Ref{Adapter: KindChangelogFile, URL: FileURL(k), Heading: sec.Heading},
		})
	}
	return out, nil
}

// FileURL returns the GitHub page of the file k names.
func FileURL(k mdindex.Key) string {
	ref := k.Ref
	if ref == "" {
		ref = "HEAD"
	}
	return fmt.Sprintf("https://github.com/%s/blob/%s/%s", k.Repo, ref, strings.TrimPrefix(k.Path, "/"))
}

// Expand fills a path or ref template: {version}, {major}, {minor} and {tag}.
func Expand(tmpl string, s version.Scheme, v version.Version) string {
	if !strings.Contains(tmpl, "{") {
		return tmpl
	}
	minor := s.Line(v.V, version.LineMinor)
	if i := strings.LastIndexByte(minor, '.'); i >= 0 {
		minor = minor[i+1:]
	}
	return strings.NewReplacer(
		"{version}", v.V.String(),
		"{major}", s.Line(v.V, version.LineMajor),
		"{minor}", minor,
		"{tag}", v.Tag,
	).Replace(tmpl)
}
