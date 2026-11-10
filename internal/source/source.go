package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Adapter kinds, as named in registry entries.
const (
	KindGHRelease     = "gh-release"
	KindChangelogFile = "changelog-file"
)

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
