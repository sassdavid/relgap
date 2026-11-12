package source_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// fakeRepo serves a top-level listing and files, and counts what was read.
type fakeRepo struct {
	files   map[string]string
	extra   []github.File // listed without content
	fetched []string
}

func (r *fakeRepo) RootFiles(context.Context, string) ([]github.File, error) {
	out := append([]github.File{}, r.extra...)
	for name, body := range r.files {
		out = append(out, github.File{Name: name, Size: len(body)})
	}
	return out, nil
}

func (r *fakeRepo) File(_ context.Context, _, _, path string) ([]byte, error) {
	r.fetched = append(r.fetched, path)
	body, ok := r.files[path]
	if !ok {
		return nil, fmt.Errorf("fetch %s: %w", path, github.ErrNotFound)
	}
	return []byte(body), nil
}

func boilerplateReleases(tags ...string) fakeFetcher {
	out := fakeFetcher{}
	for _, tag := range tags {
		out[tag] = github.Release{
			Tag: tag, URL: "https://github.com/o/r/releases/tag/" + tag,
			Body: "Binaries: https://example.org/" + tag + ".tar.gz\nsha256 " + strings.Repeat("a", 64),
		}
	}
	return out
}

// TestSpec_source_adapters_5: auto-discovery picks HISTORY.md, CHANGES.rst and RELEASE_NOTES.md,
// prefers .md over .rst over .txt, and never picks changelog.json or a file under 100 bytes.
func TestSpec_source_adapters_5(t *testing.T) {
	t.Parallel()
	big := 4096
	tests := []struct {
		name  string
		files []github.File
		want  string
	}{
		{"HISTORY.md", []github.File{{Name: "README.md", Size: big}, {Name: "HISTORY.md", Size: big}}, "HISTORY.md"},
		{"CHANGES.rst", []github.File{{Name: "CHANGES.rst", Size: big}, {Name: "LICENSE", Size: big}}, "CHANGES.rst"},
		{"RELEASE_NOTES.md", []github.File{{Name: "RELEASE_NOTES.md", Size: big}}, "RELEASE_NOTES.md"},
		{"md over rst over txt", []github.File{
			{Name: "CHANGELOG.txt", Size: big}, {Name: "CHANGES.rst", Size: big}, {Name: "History.md", Size: big},
		}, "History.md"},
		{"rst over txt", []github.File{
			{Name: "changelog.txt", Size: big},
			{Name: "NEWS.rst", Size: big},
			{Name: "Changes.rst", Size: big},
		}, "Changes.rst"},
		{"changelog.json rejected", []github.File{{Name: "changelog.json", Size: big}}, ""},
		{"tiny file rejected", []github.File{{Name: "CHANGELOG.md", Size: 40}}, ""},
		{"directory rejected", []github.File{{Name: "changelogs", Size: 0, Dir: true}}, ""},
		{"not a changelog", []github.File{{Name: "CHANGELOG-v1.md.bak", Size: big}, {Name: "NOTES.md", Size: big}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := source.PickChangelog(tt.files)
			assert.Equal(t, tt.want != "", ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBoilerplate(t *testing.T) {
	t.Parallel()
	prose := strings.Repeat("Fixed a crash when the config file is missing. ", 12)
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"short", "See the CHANGELOG for details.", true},
		{
			"links and checksums only",
			strings.Repeat("- https://example.org/a.tar.gz "+strings.Repeat("f", 64)+"\n", 8),
			true,
		},
		{"headings only", strings.Repeat("## Downloads\n\n| file | sha256 |\n", 20), true},
		{"real notes", "## Fixes\n\n- " + prose, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, source.Boilerplate(tt.body))
		})
	}
}

// TestSpec_source_adapters_4: argocd's CHANGELOG.md stops at 2.4.8 while releases go on, so
// auto-discovery ignores the file, says why, and keeps the release bodies there are.
func TestSpec_source_adapters_4(t *testing.T) {
	t.Parallel()
	stale, err := os.ReadFile(filepath.Join(testdata, "autodiscovery", "argocd", "CHANGELOG.md"))
	require.NoError(t, err)
	repo := &fakeRepo{files: map[string]string{"CHANGELOG.md": string(stale)}}
	tool := registry.ToolSpec{
		Name: "argocd", Repo: "argoproj/argo-cd",
		Version: registry.VersionSpec{Scheme: version.SchemeSemver, TagPrefix: "v"},
	}
	vs := versions(t, "v3.6.0", "v3.6.1")
	newest := vs[1].V
	a := source.NewAuto(slog.New(slog.DiscardHandler), boilerplateReleases("v3.6.0", "v3.6.1"), repo,
		source.NewFileCache(repo), newest)
	notes, err := a.Fetch(t.Context(), tool, vs)
	require.NoError(t, err)
	assert.Equal(t, []string{"CHANGELOG.md"}, repo.fetched)
	require.Len(t, notes, 2, "falls back to the release bodies")
	assert.Equal(t, source.KindGHRelease, notes[0].Source.Adapter)
	assert.Equal(t, []string{
		"ignored CHANGELOG.md: its newest version is 2.4.8, older than the newest release 3.6.1",
	}, a.Warnings())
}

// TestAutoUsesFreshChangelog: boilerplate release bodies send auto-discovery to the changelog,
// which reStructuredText-style HISTORY files also qualify for.
func TestAutoUsesFreshChangelog(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{files: map[string]string{
		"CHANGES.rst": "Changes\n=======\n\n1.1.0\n-----\n\n* Removed the --old flag.\n\n" +
			"1.0.0\n-----\n\n* First release with a much longer description of what it does.\n",
	}}
	tool := registry.ToolSpec{Name: "x", Repo: "o/x", Version: registry.VersionSpec{Scheme: version.SchemeSemver}}
	vs := versions(t, "1.1.0")
	a := source.NewAuto(slog.New(slog.DiscardHandler), boilerplateReleases("1.1.0"), repo,
		source.NewFileCache(repo), vs[0].V)
	notes, err := a.Fetch(t.Context(), tool, vs)
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, source.KindChangelogFile, notes[0].Source.Adapter)
	assert.Equal(t, "* Removed the --old flag.\n", notes[0].Markdown)
	assert.Empty(t, a.Warnings())
}

// TestAutoPrefersReleaseBodies: a range with one real release body uses the bodies and never
// lists the repository.
func TestAutoPrefersReleaseBodies(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	rel := boilerplateReleases("v1.0.0")
	rel["v1.1.0"] = github.Release{
		Tag:  "v1.1.0",
		Body: "## Fixes\n\n- " + strings.Repeat("Fixed the parser for nested blocks. ", 15),
	}
	tool := registry.ToolSpec{Name: "x", Repo: "o/x", Version: registry.VersionSpec{Scheme: version.SchemeSemver}}
	a := source.NewAuto(slog.New(slog.DiscardHandler), rel, repo, source.NewFileCache(repo), version.V{})
	notes, err := a.Fetch(t.Context(), tool, versions(t, "v1.0.0", "v1.1.0"))
	require.NoError(t, err)
	assert.Len(t, notes, 2)
	assert.Empty(t, repo.fetched)
}

func TestExpandPrev(t *testing.T) {
	t.Parallel()
	vs := versions(t, "v3.0.0", "v2.14.4")
	got := source.ExpandPrev(
		"upgrading/{prev_minor}-{major}.{minor}.md v{prev_major}",
		version.Semver(),
		vs[0],
		vs[1].V,
	)
	assert.Equal(t, "upgrading/2.14-3.0.md v2", got)
	assert.Equal(t, "{prev_minor}-3.0", source.Expand("{prev_minor}-{major}.{minor}", version.Semver(), vs[0]))
}
