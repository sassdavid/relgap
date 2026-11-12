package source_test

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
)

// adapterDeps is what an adapter under test reads through: one cassette for GitHub and the web.
type adapterDeps struct {
	gh    *github.Client
	web   *source.Web
	files *source.PageCache
}

// adapterFixture is one tool's registry source over a few versions, recorded in a cassette.
type adapterFixture struct {
	name     string
	cassette string // under testdata/cassettes
	recorded bool   // false: the cassette belongs to a CLI script and is only replayed here
	tool     registry.ToolSpec
	from     string // tag before the range; empty for none
	tags     []string
}

func (f adapterFixture) deps(t *testing.T) adapterDeps {
	t.Helper()
	if vcr.Recording() && !f.recorded {
		t.Skipf("replays %s, which a CLI script records", f.cassette)
	}
	rt := vcr.ForTest(t, filepath.Join(testdata, "cassettes", f.cassette), ghtest.Scrubber())
	hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
	gh, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	web := source.NewWeb(hc)
	return adapterDeps{gh: gh, web: web, files: source.NewPageCache(web)}
}

// versionsOf parses tags with the tool's own version rules.
func versionsOf(t *testing.T, tool registry.ToolSpec, tags ...string) []version.Version {
	t.Helper()
	rules, err := tool.Version.Rules()
	require.NoError(t, err)
	out := make([]version.Version, len(tags))
	for i, tag := range tags {
		v, parseErr := rules.Scheme.Parse(strings.TrimPrefix(tag, tool.Version.TagPrefix))
		require.NoError(t, parseErr)
		out[i] = version.Version{V: v, Tag: tag}
	}
	return out
}

func (f adapterFixture) fromVersion(t *testing.T) version.V {
	t.Helper()
	if f.from == "" {
		return version.V{}
	}
	return versionsOf(t, f.tool, f.from)[0].V
}

// goldenNotes compares notes with testdata/golden/source/<kind>/<name>.json.
func goldenNotes(t *testing.T, kind, name string, notes []source.VersionNotes) {
	t.Helper()
	type view struct {
		notesView

		Line int `json:"line,omitempty"`
	}
	out := make([]view, len(notes))
	for i, n := range notes {
		out[i] = view{notesView{n.Version.V.String(), n.Version.Tag, n.Source, n.Markdown}, n.Line}
	}
	got, err := json.MarshalIndent(out, "", "  ")
	require.NoError(t, err)
	testutil.Golden(t, filepath.Join(testdata, "golden", "source", kind, name+".json"), append(got, '\n'))
}

func toolSpec(name, repo, scheme, prefix string, s registry.Source) registry.ToolSpec {
	return registry.ToolSpec{
		Name:    name,
		Repo:    repo,
		Version: registry.VersionSpec{Scheme: scheme, TagPrefix: prefix},
		Sources: []registry.Source{s},
	}
}

// TestSpec_source_adapters_1_structured_json: awscli's .changes/<version>.json on the v2 branch
// and Adoptium's release notes API for temurin 21 become golden VersionNotes.
func TestSpec_source_adapters_1_structured_json(t *testing.T) {
	t.Parallel()
	fixtures := []adapterFixture{
		{
			name: "awscli", cassette: "source/structured-json-awscli", recorded: true,
			tool: toolSpec("awscli", "aws/aws-cli", version.SchemeSemver, "", registry.Source{
				Kind: source.KindStructuredJSON, PathTemplate: ".changes/{version}.json", Ref: "v2",
				JSON: &registry.JSONMap{Heading: "type", Text: "{category}: {description}"},
			}),
			tags: []string{"2.31.1", "2.31.2", "2.31.99"},
		},
		{
			name: "temurin", cassette: "source/structured-json-temurin", recorded: true,
			tool: toolSpec("temurin", "adoptium/temurin21-binaries", version.SchemeLoose, "jdk-", registry.Source{
				Kind:        source.KindStructuredJSON,
				URLTemplate: "https://api.adoptium.net/v3/info/release_notes/{tag}",
				JSON: &registry.JSONMap{
					Items:   "release_notes",
					Heading: "type",
					Text:    "{title} ({component})",
					Link:    "link",
				},
			}),
			tags: []string{"jdk-21.0.9+10"},
		},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			deps := f.deps(t)
			a := source.NewStructuredJSON(slog.New(slog.DiscardHandler), deps.gh, deps.web, f.tool.Sources[0])
			assert.Equal(t, source.KindStructuredJSON, a.Kind())
			notes, err := a.Fetch(t.Context(), f.tool, versionsOf(t, f.tool, f.tags...))
			require.NoError(t, err)
			require.NotEmpty(t, notes)
			goldenNotes(t, source.KindStructuredJSON, f.name, notes)
		})
	}
}

// TestSpec_source_adapters_1_web_doc: go's release notes page goes to the first version of its
// minor, point releases get their paragraph from the release history, and python's "What's new"
// page loses its permalink anchors.
func TestSpec_source_adapters_1_web_doc(t *testing.T) {
	t.Parallel()
	goTool := func(s registry.Source) registry.ToolSpec {
		return toolSpec("go", "golang/go", version.SchemeLoose, "go", s)
	}
	fixtures := []adapterFixture{
		{
			name: "go", cassette: "source/web-doc-go", recorded: true,
			tool: goTool(registry.Source{Kind: source.KindWebDoc, URLTemplate: "https://go.dev/doc/go{major}.{minor}"}),
			from: "go1.26.9", tags: []string{"go1.27", "go1.27.1"},
		},
		{
			name: "go-release", cassette: "source/web-doc-go-release", recorded: true,
			tool: goTool(registry.Source{
				Kind: source.KindWebDoc, URLTemplate: "https://go.dev/doc/devel/release", Anchor: "go{version}",
			}),
			from: "go1.26.9", tags: []string{"go1.27", "go1.27.1", "go1.27.2"},
		},
		{
			name: "python", cassette: "source/web-doc-python", recorded: true,
			tool: toolSpec("python", "python/cpython", version.SchemePython, "v", registry.Source{
				Kind: source.KindWebDoc, URLTemplate: "https://docs.python.org/3/whatsnew/{major}.{minor}.html",
				Content: "div.body", Strip: []string{"a.headerlink"},
			}),
			from: "v3.13.9", tags: []string{"v3.14.0"},
		},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			deps := f.deps(t)
			a := source.NewWebDoc(slog.New(slog.DiscardHandler), deps.files, f.tool.Sources[0], f.fromVersion(t))
			assert.Equal(t, source.KindWebDoc, a.Kind())
			notes, err := a.Fetch(t.Context(), f.tool, versionsOf(t, f.tool, f.tags...))
			require.NoError(t, err)
			require.NotEmpty(t, notes)
			assert.NotContains(t, notes[0].Markdown, "¶")
			goldenNotes(t, source.KindWebDoc, f.name, notes)
		})
	}
}

// TestSpec_source_adapters_1_upgrade_guide: argocd's per-minor guide is named after the real
// previous minor, and the kubernetes provider's per-major guide goes to the first 3.x version.
func TestSpec_source_adapters_1_upgrade_guide(t *testing.T) {
	t.Parallel()
	fixtures := []adapterFixture{
		{
			name: "argocd", cassette: "source/upgrade-guide-argocd", recorded: true,
			tool: toolSpec("argocd", "argoproj/argo-cd", version.SchemeSemver, "v", registry.Source{
				Kind:         source.KindUpgradeGuide,
				PathTemplate: "docs/operator-manual/upgrading/{prev_minor}-{major}.{minor}.md",
				Ref:          "master",
			}),
			from: "v3.5.7", tags: []string{"v3.6.0", "v3.6.1"},
		},
		{
			name:     "terraform-provider-kubernetes",
			cassette: "source/upgrade-guide-terraform-provider-kubernetes", recorded: true,
			tool: toolSpec("terraform-provider-kubernetes", "hashicorp/terraform-provider-kubernetes",
				version.SchemeSemver, "v", registry.Source{
					Kind: source.KindUpgradeGuide, PathTemplate: "docs/guides/v{major}-upgrade-guide.md", Ref: "main",
				}),
			from: "v2.38.0", tags: []string{"v3.0.0", "v3.0.1"},
		},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			deps := f.deps(t)
			a := source.NewUpgradeGuide(slog.New(slog.DiscardHandler), source.NewFileCache(deps.gh), deps.files,
				f.tool.Sources[0], f.fromVersion(t))
			assert.Equal(t, source.KindUpgradeGuide, a.Kind())
			notes, err := a.Fetch(t.Context(), f.tool, versionsOf(t, f.tool, f.tags...))
			require.NoError(t, err)
			require.Len(t, notes, 1, "one guide for one line transition")
			assert.True(t, strings.HasPrefix(notes[0].Markdown, "# "+source.HeadingUpgradeGuide+"\n"))
			goldenNotes(t, source.KindUpgradeGuide, f.name, notes)
		})
	}
}

// TestUpgradeNotesKubectl: kubernetes' "Urgent Upgrade Notes" come out of the v1.37.0 changelog
// section with the file line they start on.
func TestUpgradeNotesKubectl(t *testing.T) {
	t.Parallel()
	file := registry.Source{
		Kind: source.KindChangelogFile, PathTemplate: "CHANGELOG/CHANGELOG-{major}.{minor}.md",
		Ref: "master", Heading: "^v{version}$",
	}
	f := adapterFixture{
		name: "kubectl", cassette: "digest/kubectl-1.36.0..1.37.2",
		tool: registry.ToolSpec{
			Name: "kubectl", Repo: "kubernetes/kubernetes",
			Version: registry.VersionSpec{Scheme: version.SchemeSemver, TagPrefix: "v"},
			Sources: []registry.Source{file, {Kind: source.KindUpgradeNotes, FromHeading: "Urgent Upgrade Notes"}},
		},
		tags: []string{"v1.36.6", "v1.37.0"},
	}
	deps := f.deps(t)
	a := source.NewUpgradeNotes(slog.New(slog.DiscardHandler), source.NewFileCache(deps.gh), f.tool.Sources[1], &file)
	assert.Equal(t, source.KindUpgradeNotes, a.Kind())
	notes, err := a.Fetch(t.Context(), f.tool, versionsOf(t, f.tool, f.tags...))
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "1.37.0", notes[0].Version.V.String())
	assert.Positive(t, notes[0].Line)
	goldenNotes(t, source.KindUpgradeNotes, f.name, notes)

	none := source.NewUpgradeNotes(slog.New(slog.DiscardHandler), source.NewFileCache(deps.gh), f.tool.Sources[1], nil)
	notes, err = none.Fetch(t.Context(), f.tool, versionsOf(t, f.tool, f.tags...))
	require.NoError(t, err)
	assert.Empty(t, notes, "no changelog-file source, no notes")
}
