package enrich_test

import (
	"log/slog"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

func toolOf(name, scheme, tagPrefix string) registry.ToolSpec {
	return registry.ToolSpec{Name: name, Version: registry.VersionSpec{Scheme: scheme, TagPrefix: tagPrefix}}
}

func backendTool(backend, scheme string) registry.ToolSpec {
	t := toolOf(backend, scheme, "")
	t.Backend = backend
	return t
}

func repoTool(name, host, repo, tagPrefix string) registry.ToolSpec {
	t := toolOf(name, version.SchemeSemver, tagPrefix)
	t.Host, t.Repo = host, repo
	return t
}

// TestSpec_vulnerability_audit_1: every row of the coverage table finds a real advisory through
// the OSV queries its backend or repository maps to.
func TestSpec_vulnerability_audit_1(t *testing.T) {
	t.Parallel()
	jq := repoTool("github:jqlang/jq@jq-", "", "jqlang/jq", "jq-")
	jq.Version.Scheme = version.SchemeLoose
	goTool := toolOf("go", version.SchemeSemver, "go")
	goTool.OSV = []registry.OSVPackage{{Ecosystem: "Go", Name: "stdlib"}, {Ecosystem: "Go", Name: "toolchain"}}
	tests := []struct {
		cassette string
		name     string // the pin's tool name
		tool     registry.ToolSpec
		current  string
		want     []string         // advisory IDs or aliases among the results
		packages []enrich.Package // the packages records may come from
		via      enrich.Package   // a package that must give a record; zero skips the check
	}{
		{
			cassette: "npm", name: "npm:lodash", tool: backendTool("npm:lodash", version.SchemeSemver),
			current: "4.17.20", want: []string{"GHSA-35jh-r3h4-6jhm"},
			packages: []enrich.Package{{Ecosystem: "npm", Name: "lodash"}},
		},
		{
			cassette: "pypi", name: "pipx:Jinja2", tool: backendTool("pipx:Jinja2[i18n]", version.SchemePython),
			current: "3.1.2", want: []string{"GHSA-h5c8-rqwp-cp95"},
			packages: []enrich.Package{{Ecosystem: "PyPI", Name: "jinja2"}},
		},
		{
			cassette: "crates", name: "cargo:tokio", tool: backendTool("cargo:tokio", version.SchemeSemver),
			current: "1.8.0", want: []string{"RUSTSEC-2021-0124"},
			packages: []enrich.Package{{Ecosystem: "crates.io", Name: "tokio"}},
		},
		{
			cassette: "rubygems", name: "gem:rack", tool: backendTool("gem:rack", version.SchemeLoose),
			current: "2.2.3", want: []string{"GHSA-wq4h-7r42-5hrr"},
			packages: []enrich.Package{{Ecosystem: "RubyGems", Name: "rack"}},
		},
		{
			cassette: "nuget",
			name:     "dotnet:Newtonsoft.Json",
			tool:     backendTool("dotnet:Newtonsoft.Json", version.SchemeSemver),
			current:  "12.0.3",
			want:     []string{"GHSA-5crp-9r3c-p9vr"},
			packages: []enrich.Package{{Ecosystem: "NuGet", Name: "Newtonsoft.Json"}},
		},
		{
			cassette: "go-prefix", name: "go:helm.sh/helm/v3/cmd/helm",
			tool:    backendTool("go:helm.sh/helm/v3/cmd/helm", version.SchemeSemver),
			current: "3.14.0", want: []string{"GHSA-r53h-jv2g-vpx6"},
			packages: []enrich.Package{{Ecosystem: "Go", Name: "helm.sh/helm/v3"}},
		},
		{
			cassette: "github-go", name: "github:cli/cli", tool: repoTool("github:cli/cli", "", "cli/cli", "v"),
			current: "2.40.0", want: []string{"GHSA-p2h2-3vg9-4p87"},
			packages: []enrich.Package{
				{Ecosystem: "Go", Name: "github.com/cli/cli/v2"},
				{Ecosystem: "GIT", Name: "https://github.com/cli/cli"},
			},
			via: enrich.Package{Ecosystem: "Go", Name: "github.com/cli/cli/v2"},
		},
		{
			cassette: "github-git", name: "github:jqlang/jq@jq-", tool: jq,
			current: "1.7", want: []string{"CVE-2023-50246"},
			packages: []enrich.Package{
				{Ecosystem: "Go", Name: "github.com/jqlang/jq"},
				{Ecosystem: "GIT", Name: "https://github.com/jqlang/jq"},
			},
			via: enrich.Package{Ecosystem: "GIT", Name: "https://github.com/jqlang/jq"},
		},
		{
			cassette: "gitlab",
			name:     "gitlab:libtiff/libtiff",
			tool:     repoTool("gitlab:libtiff/libtiff", "gitlab.com", "libtiff/libtiff", "v"),
			current:  "4.3.0",
			want:     []string{"CVE-2022-0908"},
			packages: []enrich.Package{{Ecosystem: "GIT", Name: "https://gitlab.com/libtiff/libtiff"}},
		},
		{
			cassette: "go", name: "go", tool: goTool,
			current: "1.22.0", want: []string{"GO-2024-2687", "GO-2024-2598"},
			packages: []enrich.Package{{Ecosystem: "Go", Name: "stdlib"}, {Ecosystem: "Go", Name: "toolchain"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.cassette, func(t *testing.T) {
			t.Parallel()
			eol, osv := clients(t, "coverage-"+tt.cassette)
			rules, err := tt.tool.Version.Rules()
			require.NoError(t, err)
			current, err := rules.Scheme.Parse(tt.current)
			require.NoError(t, err)
			res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), eol, osv, 2).
				Enrich(t.Context(), []enrich.Request{{Key: "k", Name: tt.name, Tool: tt.tool, Current: current}})
			require.NoError(t, err)
			require.Empty(t, problems)
			got := res["k"]
			assert.Empty(t, got.Unchecked)
			var names []string
			pkgs := map[enrich.Package]bool{}
			for _, v := range got.Vulns {
				names = append(names, v.ID)
				names = append(names, v.Aliases...)
				pkgs[v.Package] = true
			}
			for _, id := range tt.want {
				assert.Contains(t, names, id)
			}
			if tt.via != (enrich.Package{}) {
				assert.True(t, pkgs[tt.via], "no record came from %v", tt.via)
			}
			for p := range pkgs {
				assert.Contains(t, tt.packages, p, "a record came from a package the coverage table doesn't name")
			}
		})
	}
}

// countingTransport fails every request and counts them.
type countingTransport struct{ n atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, http.ErrUseLastResponse
}

// TestSpec_vulnerability_audit_1_private: a pin on a private host or with a package name OSV
// can't know sends no request and is unchecked with reason private-source.
func TestSpec_vulnerability_audit_1_private(t *testing.T) {
	t.Parallel()
	image := toolOf("image:registry.example.com/team/app", version.SchemeSemver, "")
	image.Image = "registry.example.com/team/app"
	tests := []struct {
		name string
		tool registry.ToolSpec
	}{
		{"bitbucket:team/tool", repoTool("bitbucket:team/tool", "bitbucket.org", "team/tool", "v")},
		{
			"gitlab:git.example.com/team/tool",
			repoTool("gitlab:git.example.com/team/tool", "git.example.com", "team/tool", "v"),
		},
		{"npm:alias", backendTool("npm:alias@npm:lodash", version.SchemeSemver)},
		{"npm:@scope/name", backendTool("npm:@scope/name@npm:other", version.SchemeSemver)},
		{"image:registry.example.com/team/app", image},
	}
	tr := &countingTransport{}
	hc := &http.Client{Transport: tr}
	reqs := make([]enrich.Request, 0, len(tests))
	for _, tt := range tests {
		reqs = append(reqs, enrich.Request{Key: tt.name, Name: tt.name, Tool: tt.tool, Current: semver(t, "1.2.3")})
	}
	res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler),
		enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, ""), 2).Enrich(t.Context(), reqs)
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Zero(t, tr.n.Load(), "no request leaves for a private source")
	for _, tt := range tests {
		assert.Equal(t, enrich.ReasonPrivate, res[tt.name].Unchecked, tt.name)
		assert.Nil(t, res[tt.name].Vulns, tt.name)
	}
}

func TestCoverageOf(t *testing.T) {
	t.Parallel()
	chart := repoTool("argo-cd", "", "argoproj/argo-helm", "argo-cd-")
	module := repoTool("tfmodule:terraform-aws-modules/vpc/aws", "", "terraform-aws-modules/terraform-aws-vpc", "v")
	dockerHub := toolOf("image:nginx", version.SchemeSemver, "")
	dockerHub.Image = "nginx"
	aquaTool := repoTool("k9s", "", "derailed/k9s", "v")
	aquaTool.Backend = "aqua:derailed/k9s"
	ginkgo := repoTool("ginkgo", "", "onsi/ginkgo", "v")
	ginkgo.Backend = "go:github.com/onsi/ginkgo/v2/ginkgo"
	codeql := repoTool("action:github/codeql-action/init", "", "github/codeql-action", "")
	privateAction := repoTool("action:team/deploy", "git.example.com", "team/deploy", "")
	tests := []struct {
		name string
		tool registry.ToolSpec
		v    string
		want enrich.Coverage
	}{
		{"chart:argo/argo-cd", chart, "1.0.0", enrich.Coverage{Reason: enrich.ReasonNoSource}},
		{"tfmodule:terraform-aws-modules/vpc/aws", module, "5.0.0", enrich.Coverage{Reason: enrich.ReasonNoSource}},
		{"image:nginx", dockerHub, "1.27.0", enrich.Coverage{Reason: enrich.ReasonNoSource}},
		{"zig", toolOf("zig", version.SchemeSemver, ""), "0.13.0", enrich.Coverage{Reason: enrich.ReasonNoSource}},
		{"k9s", aquaTool, "0.50.16", enrich.Coverage{Packages: []registry.OSVPackage{
			{Ecosystem: "Go", Name: "github.com/derailed/k9s"},
			{Ecosystem: "GIT", Name: "https://github.com/derailed/k9s"},
		}, Repo: enrich.Repo{Host: "github.com", Path: "derailed/k9s"}}},
		{"ginkgo", ginkgo, "2.1.0", enrich.Coverage{GoPath: "github.com/onsi/ginkgo/v2/ginkgo"}},
		// Advisories name the repository, also for an action in a subdirectory.
		{"action:github/codeql-action/init", codeql, "3.29.0", enrich.Coverage{
			Packages: []registry.OSVPackage{{Ecosystem: "GitHub Actions", Name: "github/codeql-action"}},
			Repo:     enrich.Repo{Host: "github.com", Path: "github/codeql-action"},
		}},
		{"action:team/deploy", privateAction, "1.0.0", enrich.Coverage{Reason: enrich.ReasonPrivate}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, enrich.CoverageOf(tt.name, tt.tool, semver(t, tt.v)))
		})
	}
	assert.Equal(t, []string{"helm.sh/helm/v3/cmd/helm", "helm.sh/helm/v3/cmd", "helm.sh/helm/v3", "helm.sh/helm"},
		enrich.GoPrefixes("helm.sh/helm/v3/cmd/helm"))
	assert.True(t, slices.Equal([]string{"gopkg.in/yaml.v3"}, enrich.GoPrefixes("gopkg.in/yaml.v3")))
}
