package source

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Adapter kinds, as named in registry entries.
const (
	KindGHRelease      = "gh-release"
	KindChangelogFile  = "changelog-file"
	KindStructuredJSON = "structured-json"
	KindWebDoc         = "web-doc"
	KindUpgradeGuide   = "upgrade-guide"
	KindUpgradeNotes   = "upgrade-notes"
)

// KindDoc says what one adapter kind reads.
type KindDoc struct {
	Kind  string `json:"kind"`
	Reads string `json:"reads"`
}

// Kinds returns every adapter kind with what it reads, in the order of the registry schema.
func Kinds() []KindDoc {
	return []KindDoc{
		{KindGHRelease, "The GitHub release body of each version."},
		{KindChangelogFile, "The section of each version in a changelog file of the tool's repository. " +
			"Each file is fetched and parsed once per digest."},
		{KindStructuredJSON, "One JSON document per version, from a repository file or an allowed URL, " +
			"turned into a list grouped by a heading field."},
		{KindWebDoc, "Release notes on an allowed web page. With an anchor, each version gets the element " +
			"with its id; without one, a page holds a whole release line."},
		{KindUpgradeGuide, "A whole upgrade document, attached to the first version of each new release line " +
			"in the range: one per minor when the template has {minor} or {prev_minor}, else one per major."},
		{KindUpgradeNotes, "The section under a heading such as Urgent Upgrade Notes inside each version's " +
			"section of the changelog-file source, so its entries count as upgrade notes."},
	}
}

// ErrNotFound is wrapped by [Web] errors for pages that don't exist.
var ErrNotFound = errors.New("page not found")

// missing reports whether err means the file or page doesn't exist, which adapters skip.
func missing(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, github.ErrNotFound)
}

// Ref says where a piece of notes came from.
type Ref struct {
	Adapter string   `json:"adapter"`
	URL     string   `json:"url"`
	Heading []string `json:"heading,omitempty"` // heading path inside the document
}

// VersionNotes is the Markdown one source has for one version.
type VersionNotes struct {
	Version  version.Version
	Markdown string
	Line     int // 1-based line where Markdown starts in a file; 0 for release bodies
	Source   Ref
}

// Adapter fetches notes for versions. Versions without notes are left out, not errors.
type Adapter interface {
	Kind() string
	Fetch(ctx context.Context, t registry.ToolSpec, versions []version.Version) ([]VersionNotes, error)
}

// ReleaseFetcher returns releases by tag; [github.Client] implements it.
type ReleaseFetcher interface {
	ReleaseBodies(ctx context.Context, repo string, tags []string) (map[string]github.Release, error)
}

// GHRelease reads the GitHub release body of each version.
type GHRelease struct {
	gh ReleaseFetcher
}

// NewGHRelease returns a gh-release adapter that fetches through gh.
func NewGHRelease(gh ReleaseFetcher) *GHRelease { return &GHRelease{gh: gh} }

// Kind returns [KindGHRelease].
func (a *GHRelease) Kind() string { return KindGHRelease }

// Fetch returns the non-empty release bodies of versions, in the order given.
func (a *GHRelease) Fetch(
	ctx context.Context,
	t registry.ToolSpec,
	versions []version.Version,
) ([]VersionNotes, error) {
	tags := make([]string, len(versions))
	for i, v := range versions {
		tags[i] = v.Tag
	}
	releases, err := a.gh.ReleaseBodies(ctx, t.Repo, tags)
	if err != nil {
		return nil, fmt.Errorf("gh-release: %w", err)
	}
	var out []VersionNotes
	for _, v := range versions {
		r, ok := releases[v.Tag]
		if !ok {
			continue
		}
		body := strings.TrimSpace(strings.ReplaceAll(r.Body, "\r\n", "\n"))
		if body == "" {
			continue
		}
		out = append(out, VersionNotes{
			Version:  v,
			Markdown: body + "\n",
			Source:   Ref{Adapter: KindGHRelease, URL: r.URL},
		})
	}
	return out, nil
}
