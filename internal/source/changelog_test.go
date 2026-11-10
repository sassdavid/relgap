package source_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
)

// changelogFixture is a changelog-file source with the versions a test asks for.
type changelogFixture struct {
	name     string
	cassette string // under testdata/cassettes
	recorded bool   // false: the cassette belongs to a CLI script and is only replayed here
	tool     registry.ToolSpec
	tags     []string
	files    int // distinct files the versions map to
}

func semverTool(repo string, s registry.Source) registry.ToolSpec {
	return registry.ToolSpec{
		Name:    filepath.Base(repo),
		Repo:    repo,
		Version: registry.VersionSpec{Scheme: version.SchemeSemver, TagPrefix: "v"},
		Sources: []registry.Source{s},
	}
}

func changelogFixtures() []changelogFixture {
	cc := make([]string, 0, 20)
	for p := 300; p < 320; p++ {
		cc = append(cc, "v2.1."+strconv.Itoa(p))
	}
	return []changelogFixture{
		{
			name:     "claude-code",
			cassette: "source/changelog-file-claude-code",
			recorded: true,
			tool: semverTool(
				"anthropics/claude-code",
				registry.Source{Kind: source.KindChangelogFile, PathTemplate: "CHANGELOG.md"},
			),
			tags:  cc,
			files: 1,
		},
		{
			name: "node", cassette: "source/changelog-file-node", recorded: true,
			tool: semverTool("nodejs/node", registry.Source{
				Kind: source.KindChangelogFile, PathTemplate: "doc/changelogs/CHANGELOG_V{major}.md",
			}),
			tags:  []string{"v26.10.0", "v26.11.0", "v26.11.1", "v26.12.0", "v26.12.1", "v26.13.0"},
			files: 1,
		},
		{
			name: "terraform", cassette: "gh-release/terraform-1.15.8..1.17.1",
			tool: semverTool("hashicorp/terraform", registry.Source{
				Kind:         source.KindChangelogFile,
				PathTemplate: "CHANGELOG.md",
				Ref:          "v{major}.{minor}",
				Heading:      `^{version} \(`,
			}),
			tags:  []string{"v1.16.6", "v1.16.7", "v1.17.0", "v1.17.1"},
			files: 2,
		},
		{
			name: "kubernetes", cassette: "digest/kubectl-1.36.0..1.37.2",
			tool: semverTool("kubernetes/kubernetes", registry.Source{
				Kind: source.KindChangelogFile, PathTemplate: "CHANGELOG/CHANGELOG-{major}.{minor}.md",
				Ref: "master", Heading: "^v{version}$",
			}),
			tags:  []string{"v1.36.5", "v1.36.6", "v1.37.0", "v1.37.1", "v1.37.2"},
			files: 2,
		},
	}
}

// counter counts requests per URL.
type counter struct {
	next http.RoundTripper
	mu   sync.Mutex
	n    map[string]int
}

func (c *counter) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.n[req.URL.String()]++
	c.mu.Unlock()
	return c.next.RoundTrip(req)
}

// open returns a file cache over the fixture's cassette and the request counter. Fixtures whose
// cassette a CLI script records are skipped while recording.
func (f changelogFixture) open(t *testing.T) (*mdindex.Cache, *counter) {
	t.Helper()
	if vcr.Recording() && !f.recorded {
		t.Skipf("replays %s, which a CLI script records", f.cassette)
	}
	c := &counter{
		next: vcr.ForTest(t, filepath.Join(testdata, "cassettes", f.cassette), ghtest.Scrubber()),
		n:    map[string]int{},
	}
	hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: c})
	gh, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	return source.NewFileCache(gh), c
}

// TestSpec_source_adapters_1_changelog: the changelog-file adapter has cassette-backed golden
// VersionNotes for claude-code (a 1.2 MB file), node and terraform (two release branches).
func TestSpec_source_adapters_1_changelog(t *testing.T) {
	t.Parallel()
	for _, f := range changelogFixtures() {
		if f.name == "kubernetes" {
			continue // its sections are in the kubectl digest golden
		}
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			cache, _ := f.open(t)
			a := source.NewChangelogFile(slog.New(slog.DiscardHandler), cache, f.tool.Sources[0])
			assert.Equal(t, source.KindChangelogFile, a.Kind())
			notes, err := a.Fetch(t.Context(), f.tool, versions(t, f.tags...))
			require.NoError(t, err)
			require.NotEmpty(t, notes)
			type view struct {
				notesView

				Line int `json:"line"`
			}
			out := make([]view, len(notes))
			for i, n := range notes {
				out[i] = view{notesView{n.Version.V.String(), n.Version.Tag, n.Source, n.Markdown}, n.Line}
			}
			got, err := json.MarshalIndent(out, "", "  ")
			require.NoError(t, err)
			testutil.Golden(
				t,
				filepath.Join(testdata, "golden", "source", "changelog-file", f.name+".json"),
				append(got, '\n'),
			)
		})
	}
}

// TestSpec_source_adapters_3: a changelog file is fetched once per digest however many versions
// it holds, and a second adapter on the same cache fetches nothing and reuses the parsed index.
func TestSpec_source_adapters_3(t *testing.T) {
	t.Parallel()
	for _, f := range changelogFixtures() {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			if vcr.Recording() {
				t.Skip("replay only, so a cassette is never recorded twice")
			}
			cache, c := f.open(t)
			vs := versions(t, f.tags...)
			for range 2 {
				a := source.NewChangelogFile(slog.New(slog.DiscardHandler), cache, f.tool.Sources[0])
				_, err := a.Fetch(t.Context(), f.tool, vs)
				require.NoError(t, err)
			}
			assert.Len(t, c.n, f.files, "distinct files fetched for %d versions", len(vs))
			for u, n := range c.n {
				assert.Equal(t, 1, n, "requests for %s", u)
			}
			rules, err := f.tool.Version.Rules()
			require.NoError(t, err)
			k := mdindex.Key{
				Repo: f.tool.Repo,
				Ref:  source.Expand(f.tool.Sources[0].Ref, rules.Scheme, vs[0]),
				Path: source.Expand(f.tool.Sources[0].PathTemplate, rules.Scheme, vs[0]),
			}
			opts := mdindex.Options{Scheme: rules.Scheme}
			if h := f.tool.Sources[0].Heading; h != "" {
				opts.Heading, err = mdindex.HeadingPattern(h)
				require.NoError(t, err)
			}
			first, err := cache.Get(t.Context(), k, opts)
			require.NoError(t, err)
			again, err := cache.Get(t.Context(), k, opts)
			require.NoError(t, err)
			assert.Same(t, first, again, "the index is parsed once")
		})
	}
}

// TestChangelogIndexGolden: the section index of the kubernetes, claude-code and node changelogs.
func TestChangelogIndexGolden(t *testing.T) {
	t.Parallel()
	for _, f := range changelogFixtures() {
		if f.name == "terraform" {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			if vcr.Recording() {
				t.Skip("replay only, so a cassette is never recorded twice")
			}
			cache, _ := f.open(t)
			rules, err := f.tool.Version.Rules()
			require.NoError(t, err)
			s := f.tool.Sources[0]
			opts := mdindex.Options{Scheme: rules.Scheme}
			if s.Heading != "" {
				opts.Heading, err = mdindex.HeadingPattern(s.Heading)
				require.NoError(t, err)
			}
			type sectionView struct {
				Version     string   `json:"version"`
				Heading     []string `json:"heading"`
				HeadingLine int      `json:"heading_line"`
				Line        int      `json:"line"`
				Start       int      `json:"start"`
				End         int      `json:"end"`
			}
			type fileView struct {
				Path     string        `json:"path"`
				Sections []sectionView `json:"sections"`
			}
			var out []fileView
			seen := map[string]bool{}
			for _, v := range versions(t, f.tags...) {
				path := source.Expand(s.PathTemplate, rules.Scheme, v)
				if seen[path] {
					continue
				}
				seen[path] = true
				x, getErr := cache.Get(
					t.Context(),
					mdindex.Key{Repo: f.tool.Repo, Ref: source.Expand(s.Ref, rules.Scheme, v), Path: path},
					opts,
				)
				require.NoError(t, getErr)
				fv := fileView{Path: path}
				for _, sec := range x.Sections() {
					fv.Sections = append(fv.Sections, sectionView{
						sec.Version.String(), sec.Heading, sec.HeadingLine, sec.Line, sec.Start, sec.End,
					})
				}
				out = append(out, fv)
			}
			got, err := json.MarshalIndent(out, "", "  ")
			require.NoError(t, err)
			testutil.Golden(t, filepath.Join(testdata, "golden", "mdindex", f.name+".json"), append(got, '\n'))
		})
	}
}
