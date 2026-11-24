package enrich_test

import (
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
	"github.com/sassdavid/relgap/internal/vex"
)

// recorder suppresses nothing and keeps the subjects it was asked about.
type recorder struct {
	mu       sync.Mutex
	subjects []enrich.Subject
}

func (r *recorder) Suppress(s enrich.Subject, _ []string) *enrich.Suppression {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subjects = append(r.subjects, s)
	return nil
}

func (*recorder) Covers(enrich.Subject, []string) bool { return false }

// TestSuppressionSubjects: suppression rules see each tool version by its tool name, version and
// the purls of its OSV packages and repository, and each advisory carries its package's purl.
func TestSuppressionSubjects(t *testing.T) {
	t.Parallel()
	jq := repoTool("github:jqlang/jq@jq-", "", "jqlang/jq", "jq-")
	jq.Version.Scheme = version.SchemeLoose
	goTool := toolOf("go", version.SchemeSemver, "go")
	goTool.OSV = []registry.OSVPackage{{Ecosystem: "Go", Name: "stdlib"}, {Ecosystem: "Go", Name: "toolchain"}}
	tests := []struct {
		cassette, name string
		tool           registry.ToolSpec
		current        string
		want           []string
	}{
		{
			cassette: "npm", name: "npm:lodash", tool: backendTool("npm:lodash", version.SchemeSemver),
			current: "4.17.20", want: []string{"pkg:npm/lodash@4.17.20"},
		},
		{
			cassette: "pypi", name: "pipx:Jinja2", tool: backendTool("pipx:Jinja2[i18n]", version.SchemePython),
			current: "3.1.2", want: []string{"pkg:pypi/jinja2@3.1.2"},
		},
		{
			cassette: "crates", name: "cargo:tokio", tool: backendTool("cargo:tokio", version.SchemeSemver),
			current: "1.8.0", want: []string{"pkg:cargo/tokio@1.8.0"},
		},
		{
			cassette: "rubygems", name: "gem:rack", tool: backendTool("gem:rack", version.SchemeLoose),
			current: "2.2.3", want: []string{"pkg:gem/rack@2.2.3"},
		},
		{
			cassette: "nuget", name: "dotnet:Newtonsoft.Json",
			tool:    backendTool("dotnet:Newtonsoft.Json", version.SchemeSemver),
			current: "12.0.3", want: []string{"pkg:nuget/Newtonsoft.Json@12.0.3"},
		},
		{
			cassette: "go-prefix", name: "go:helm.sh/helm/v3/cmd/helm",
			tool:    backendTool("go:helm.sh/helm/v3/cmd/helm", version.SchemeSemver),
			current: "3.14.0", want: []string{"pkg:golang/helm.sh/helm/v3@v3.14.0"},
		},
		{
			cassette: "github-go", name: "github:cli/cli", tool: repoTool("github:cli/cli", "", "cli/cli", "v"),
			current: "2.40.0", want: []string{"pkg:golang/github.com/cli/cli/v2@v2.40.0", "pkg:github/cli/cli@v2.40.0"},
		},
		{
			cassette: "github-git", name: "github:jqlang/jq@jq-", tool: jq,
			current: "1.7", want: []string{"pkg:golang/github.com/jqlang/jq@v1.7.0", "pkg:github/jqlang/jq@jq-1.7"},
		},
		{
			cassette: "gitlab", name: "gitlab:libtiff/libtiff",
			tool:    repoTool("gitlab:libtiff/libtiff", "gitlab.com", "libtiff/libtiff", "v"),
			current: "4.3.0",
			want:    []string{"pkg:generic/libtiff@v4.3.0?vcs_url=git%2Bhttps:%2F%2Fgitlab.com%2Flibtiff%2Flibtiff"},
		},
		{
			cassette: "go", name: "go", tool: goTool,
			current: "1.22.0", want: []string{"pkg:golang/stdlib@v1.22.0", "pkg:golang/toolchain@v1.22.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cassette, func(t *testing.T) {
			t.Parallel()
			_, osv := clients(t, "coverage-"+tt.cassette)
			rules, err := tt.tool.Version.Rules()
			require.NoError(t, err)
			current, err := rules.Scheme.Parse(tt.current)
			require.NoError(t, err)
			rec := &recorder{}
			en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2)
			en.SetSuppressor(rec)
			res, problems, err := en.Enrich(t.Context(),
				[]enrich.Request{{Key: "k", Name: tt.name, Tool: tt.tool, Current: current}})
			require.NoError(t, err)
			require.Empty(t, problems)
			require.NotEmpty(t, rec.subjects)
			s := rec.subjects[0]
			assert.Equal(t, tt.name, s.Tool)
			assert.Equal(t, current.String(), s.Version)
			got := make([]string, 0, len(s.PURLs))
			for _, p := range s.PURLs {
				got = append(got, p.String())
			}
			assert.Equal(t, tt.want, got)
			for _, v := range res["k"].Vulns {
				assert.Contains(t, tt.want, v.PURL(), v.ID)
			}
		})
	}
}

const downloadArtifactSHA = "87c55149d96e628cc2ef7e6fc2aab372015aec85"

// TestSuppressionActionSHA: an action pinned by commit SHA matches a VEX statement that names
// the SHA as well as one that names the tag, and its purl carries the tag and the action's path.
func TestSuppressionActionSHA(t *testing.T) {
	t.Parallel()
	action := repoTool("action:actions/download-artifact/merge", "", "actions/download-artifact", "v")
	tests := []struct {
		name, product string
		refs          []string
		suppressed    bool
	}{
		{
			name: "sha", product: "pkg:githubactions/actions/download-artifact@" + downloadArtifactSHA,
			refs: []string{downloadArtifactSHA}, suppressed: true,
		},
		{name: "tag", product: "pkg:githubactions/actions/download-artifact@v4.1.2", suppressed: true},
		{name: "other path", product: "pkg:githubactions/actions/download-artifact@v4.1.2#upload"},
		{name: "sha of another pin", product: "pkg:githubactions/actions/download-artifact@" + downloadArtifactSHA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, osv := clients(t, "../audit/actions")
			doc := vex.Document{Timestamp: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Statements: []vex.Statement{{
				Vulnerability: vex.Vulnerability{Name: "GHSA-cxww-7g56-2vh6"},
				Products:      []vex.Component{{ID: tt.product}},
				Status:        vex.StatusNotAffected, Justification: "vulnerable_code_not_present",
			}}}
			rules, err := vex.New(nil, []vex.File{{Path: "a.json", Document: doc}}, time.Now())
			require.NoError(t, err)
			en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2)
			en.SetSuppressor(rules)
			res, _, err := en.Enrich(t.Context(), []enrich.Request{{
				Key: "k", Name: action.Name, Tool: action, Current: semver(t, "4.1.2"), Refs: tt.refs,
			}})
			require.NoError(t, err)
			i := slices.IndexFunc(res["k"].Vulns, func(v enrich.Vuln) bool { return v.ID == "GHSA-cxww-7g56-2vh6" })
			require.GreaterOrEqual(t, i, 0)
			v := res["k"].Vulns[i]
			assert.Equal(t, "pkg:githubactions/actions/download-artifact@v4.1.2#merge", v.PURL())
			assert.Equal(t, tt.suppressed, v.Suppressed != nil)
		})
	}
}

// TestSuppressionFixTarget: a suppressed advisory neither blocks a fix target candidate nor
// keeps the latest release affected. lodash 4.18.0 clears every advisory of 4.17.20 but
// GHSA-r5fr-rjxr-66jc, and has GHSA-f23m-r3pf-42rh of its own; with both suppressed, 4.18.0 is
// the target. request's latest release still has GHSA-p8p7-x288-28g6; suppressed, it is no
// longer unfixed.
func TestSuppressionFixTarget(t *testing.T) {
	t.Parallel()
	lodash := backendTool("npm:lodash", version.SchemeSemver)
	request := backendTool("npm:request", version.SchemeSemver)
	request.Version.TagPrefixes = registry.Prefixes{"v"}
	tests := []struct {
		cassette string
		tool     registry.ToolSpec
		current  string
		versions string
		ignore   []string
		want     enrich.Fix
	}{
		{
			cassette: "lodash", tool: lodash, current: "4.17.20", versions: "3.0.0 4.0.0 4.17.20 4.18.0 4.18.1",
			ignore: []string{"GHSA-r5fr-rjxr-66jc", "GHSA-f23m-r3pf-42rh"},
			want:   enrich.Fix{Lowest: "4.18.0", LowestInLine: "4.18.0", Latest: "4.18.1"},
		},
		{
			cassette: "request", tool: request, current: "2.67.0", versions: "2.67.0 2.68.0 2.88.1",
			ignore: []string{"GHSA-p8p7-x288-28g6"},
			want:   enrich.Fix{Lowest: "2.68.0", LowestInLine: "2.68.0", Latest: "2.88.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cassette, func(t *testing.T) {
			t.Parallel()
			_, osv := clients(t, "fix-"+tt.cassette)
			var ignores []vex.Ignore
			for _, id := range tt.ignore {
				ignores = append(ignores, vex.Ignore{ID: id, Reason: "accepted"})
			}
			rules, err := vex.New(ignores, nil, time.Now())
			require.NoError(t, err)
			en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2)
			en.SetSuppressor(rules)
			res, problems, err := en.Enrich(t.Context(), []enrich.Request{{
				Key: "k", Name: tt.tool.Name, Tool: tt.tool, Current: semver(t, tt.current),
				Versions: versionsOf(t, tt.tool, tt.versions),
			}})
			require.NoError(t, err)
			require.Empty(t, problems)
			require.NotNil(t, res["k"].Fix)
			assert.Equal(t, tt.want, *res["k"].Fix)
		})
	}
}
