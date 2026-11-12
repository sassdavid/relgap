package source

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

const (
	boilerplateBytes  = 400 // release bodies shorter than this don't count as notes
	minChangelogBytes = 100 // smaller changelog files are placeholders
	minNotesWords     = 10  // fewer words of prose, links and checksums aside, aren't notes
)

// changelogName is Renovate's changelog file name rule (the changelog-filename-regex package,
// MIT), matched case-insensitively.
var changelogName = regexp.MustCompile(
	`(?i)^(?:(?:update|change|release)(?:s|[ \-_]*(?:logs?|histor(?:y|ies)))|histor(?:y|ies)|release[ \-_]*notes?)(?:\.[\da-z]+)?$`,
)

var (
	linkOrChecksum = regexp.MustCompile(`(?i)https?://\S+|\b[0-9a-f]{40,128}\b|\bsha(?:1|256|512)\b`)
	notesWord      = regexp.MustCompile(`\p{L}{3,}`)
)

// Boilerplate reports whether a release body carries no notes of its own: it is under 400 bytes,
// or it holds only headings, links and checksums.
func Boilerplate(body string) bool {
	body = strings.TrimSpace(body)
	if len(body) < boilerplateBytes {
		return true
	}
	var words int
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
			continue
		}
		words += len(notesWord.FindAllString(linkOrChecksum.ReplaceAllString(line, " "), -1))
	}
	return words < minNotesWords
}

// PickChangelog returns the changelog file of a repository's top directory. It prefers .md over
// .rst over .txt over other names, and never picks .json files or files under 100 bytes.
func PickChangelog(files []github.File) (string, bool) {
	rank := func(name string) int {
		switch strings.ToLower(path.Ext(name)) {
		case ".md", ".markdown":
			return 0
		case ".rst":
			return 1
		case ".txt":
			return 2 //nolint:mnd // preference order
		default:
			return 3 //nolint:mnd // preference order
		}
	}
	var picks []string
	for _, f := range files {
		ext := strings.ToLower(path.Ext(f.Name))
		if f.Dir || ext == ".json" || f.Size < minChangelogBytes || !changelogName.MatchString(f.Name) {
			continue
		}
		picks = append(picks, f.Name)
	}
	if len(picks) == 0 {
		return "", false
	}
	slices.SortFunc(picks, func(a, b string) int { return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a, b)) })
	return picks[0], true
}

// RootLister lists a repository's top directory; [github.Client] implements it.
type RootLister interface {
	RootFiles(ctx context.Context, repo string) ([]github.File, error)
}

// Auto finds notes for a tool without registry sources, as Renovate and Dependabot do: GitHub
// release bodies when the range has real ones, else a changelog file in the default branch,
// unless the file stops before the newest release. Without a usable file it keeps whatever
// release bodies there are.
type Auto struct {
	logger *slog.Logger
	gh     ReleaseFetcher
	lister RootLister
	files  *mdindex.Cache
	newest version.V // newest stable version, for the stale changelog guard

	mu       sync.Mutex
	warnings []string
}

// NewAuto returns an auto-discovery adapter; newest is the tool's newest stable version.
func NewAuto(logger *slog.Logger, gh ReleaseFetcher, lister RootLister, files *mdindex.Cache, newest version.V) *Auto {
	return &Auto{logger: logger, gh: gh, lister: lister, files: files, newest: newest}
}

// Kind returns "auto".
func (a *Auto) Kind() string { return "auto" }

// Warnings says why a changelog file was ignored.
func (a *Auto) Warnings() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.warnings)
}

// Fetch returns the release bodies when one is more than boilerplate, else the notes of the
// changelog file, else the release bodies there are.
func (a *Auto) Fetch(ctx context.Context, t registry.ToolSpec, versions []version.Version) ([]VersionNotes, error) {
	bodies, err := NewGHRelease(a.gh).Fetch(ctx, t, versions)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(bodies, func(n VersionNotes) bool { return !Boilerplate(n.Markdown) }) {
		return bodies, nil
	}
	a.logger.DebugContext(ctx, "no release body has notes; looking for a changelog file", "tool", t.Name)
	notes, err := a.changelog(ctx, t, versions)
	if err != nil || len(notes) > 0 {
		return notes, err
	}
	return bodies, nil
}

// changelog returns the notes of the repository's changelog file, or none when it has no such
// file or the file is stale.
func (a *Auto) changelog(ctx context.Context, t registry.ToolSpec, versions []version.Version) ([]VersionNotes, error) {
	files, err := a.lister.RootFiles(ctx, t.Repo)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auto-discovery: %w", err)
	}
	name, ok := PickChangelog(files)
	if !ok {
		return nil, nil
	}
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("auto-discovery: %w", err)
	}
	idx, err := a.files.Get(ctx, mdindex.Key{Repo: t.Repo, Path: name}, mdindex.Options{Scheme: rules.Scheme})
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auto-discovery: %w", err)
	}
	newest, ok := idx.Newest()
	if !ok {
		return nil, nil
	}
	if !a.newest.IsZero() && rules.Scheme.Compare(newest, a.newest) < 0 {
		a.mu.Lock()
		a.warnings = append(a.warnings, fmt.Sprintf(
			"ignored %s: its newest version is %s, older than the newest release %s", name, newest, a.newest))
		a.mu.Unlock()
		return nil, nil
	}
	notes, err := NewChangelogFile(a.logger, a.files, registry.Source{Kind: KindChangelogFile, PathTemplate: name}).
		Fetch(ctx, t, versions)
	if err != nil {
		return nil, fmt.Errorf("auto-discovery: %w", err)
	}
	return notes, nil
}
