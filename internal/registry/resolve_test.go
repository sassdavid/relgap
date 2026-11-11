package registry_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/testutil/fakebin"
)

func TestMain(m *testing.M) { fakebin.Main(m) }

const testdata = "../../testdata"

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// snapshotResolver resolves without mise, from the embedded snapshot.
func snapshotResolver(t *testing.T, cfg registry.ConfigLayer) *registry.Resolver {
	t.Helper()
	r, err := registry.Load(t.Context(), discard(), nil, cfg)
	require.NoError(t, err)
	return r
}

// liveResolver resolves with a fake mise on PATH that prints the recorded `mise registry --json`.
func liveResolver(t *testing.T) *registry.Resolver {
	t.Helper()
	out, err := filepath.Abs(filepath.Join(testdata, "mise", "registry.json"))
	require.NoError(t, err)
	bin := fakebin.Install(t, "mise", fakebin.Spec{Rules: []fakebin.Rule{
		{Args: []string{"registry", "--json"}, StdoutFile: out},
	}})
	environ := []string{"PATH=" + filepath.Dir(bin)}
	runner := execx.NewRunner(discard(), execx.WithEnviron(func() []string { return environ }))
	r, err := registry.Load(t.Context(), discard(), misex.New(runner), registry.ConfigLayer{})
	require.NoError(t, err)
	return r
}

// TestSpec_tool_resolution_1: tf, terraform, TERRAFORM_VERSION and aqua:hashicorp/terraform all
// give the same ToolSpec, and every field says which layer supplied it.
func TestSpec_tool_resolution_1(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{})
	want, err := r.Resolve("terraform")
	require.NoError(t, err)
	assert.Equal(t, "hashicorp/terraform", want.Tool.Repo)
	assert.Equal(t, "semver", want.Tool.Version.Scheme)
	for _, key := range []string{
		"name", "aliases", "repo", "version.scheme", "version.tag_prefix", "version.universe",
		"version.line", "sources", "eol", "osv",
	} {
		assert.NotEmpty(t, want.Provenance[key], "no layer recorded for %s", key)
	}
	assert.Equal(t, []registry.Layer{registry.LayerRelgap}, want.Provenance["repo"])
	assert.Equal(t, []registry.Layer{registry.LayerDefault}, want.Provenance["version.line"])

	for _, in := range []string{"tf", "TERRAFORM_VERSION", "aqua:hashicorp/terraform", "TF", "github:hashicorp/terraform"} {
		got, resolveErr := r.Resolve(in)
		require.NoError(t, resolveErr, in)
		assert.Equal(t, want.Tool, got.Tool, in)
		assert.Equal(t, want.Provenance, got.Provenance, in)
	}
}

type corpus struct {
	Tools []struct {
		Tool string `yaml:"tool"`
	} `yaml:"tools"`
}

// TestSpec_tool_resolution_2: resolution gives the same results for the 30-tool corpus with mise
// absent (embedded snapshot) and present (live `mise registry --json`).
func TestSpec_tool_resolution_2(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join(testdata, "corpus.yaml"))
	require.NoError(t, err)
	var c corpus
	require.NoError(t, yaml.Unmarshal(b, &c))
	require.Len(t, c.Tools, 30)

	absent, present := snapshotResolver(t, registry.ConfigLayer{}), liveResolver(t)
	resolved := 0
	for _, tool := range c.Tools {
		snap, snapErr := absent.Resolve(tool.Tool)
		live, liveErr := present.Resolve(tool.Tool)
		if snapErr != nil || liveErr != nil {
			require.Error(t, snapErr, tool.Tool)
			require.Error(t, liveErr, tool.Tool)
			assert.Equal(t, snapErr.Error(), liveErr.Error(), tool.Tool)
			continue
		}
		resolved++
		assert.Equal(t, registry.MiseSourceSnapshot, snap.MiseSource)
		assert.Equal(t, registry.MiseSourceLive, live.MiseSource)
		assert.Equal(t, snap.Tool, live.Tool, tool.Tool)
		assert.Equal(t, snap.Provenance, live.Provenance, tool.Tool)
		assert.Equal(t, snap.Match, live.Match, tool.Tool)
	}
	assert.GreaterOrEqual(t, resolved, 22, "most of the corpus resolves without network lookups")
}

func TestResolve(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{
		Tools:   map[string]string{"internal-cli": "acme/internal-cli", "helm": "acme/helm-fork"},
		Aliases: map[string]string{"TG_VERSION": "terragrunt"},
	})
	tests := []struct {
		in         string
		name, repo string
		prefix     string
		universe   string
		match      registry.Match
		repoLayer  registry.Layer
	}{
		{
			in:       "internal-cli",
			name:     "internal-cli",
			repo:     "acme/internal-cli",
			universe: "releases",
			match: registry.Match{
				By:    "name",
				Value: "internal-cli",
				Layer: registry.LayerConfig,
			},
			repoLayer: registry.LayerConfig,
		},
		{
			in:       "helm",
			name:     "helm",
			repo:     "acme/helm-fork",
			prefix:   "v",
			universe: "releases",
			match: registry.Match{
				By:    "name",
				Value: "helm",
				Layer: registry.LayerConfig,
			},
			repoLayer: registry.LayerConfig,
		},
		{
			in:       "TG_VERSION",
			name:     "terragrunt",
			repo:     "gruntwork-io/terragrunt",
			universe: "releases",
			match: registry.Match{
				By:    "config alias",
				Value: "TG_VERSION",
				Layer: registry.LayerConfig,
			},
			repoLayer: registry.LayerAqua,
		},
		{
			in:       "KUBECTL_VERSION",
			name:     "kubectl",
			repo:     "kubernetes/kubernetes",
			prefix:   "v",
			universe: "tags",
			match: registry.Match{
				By:    "env var name",
				Value: "KUBECTL_VERSION",
				Layer: registry.LayerRelgap,
			},
			repoLayer: registry.LayerRelgap,
		},
		{
			in:       "aws",
			name:     "awscli",
			repo:     "aws/aws-cli",
			universe: "tags",
			match: registry.Match{
				By:    "alias",
				Value: "aws",
				Layer: registry.LayerRelgap,
			},
			repoLayer: registry.LayerRelgap,
		},
		{
			in:       "aqua:mongodb/mongodb-atlas-cli/atlascli",
			name:     "atlascli",
			repo:     "mongodb/mongodb-atlas-cli",
			prefix:   "atlascli/",
			universe: "releases",
			match: registry.Match{
				By:    "aqua package",
				Value: "mongodb/mongodb-atlas-cli/atlascli",
				Layer: registry.LayerAqua,
			},
			repoLayer: registry.LayerAqua,
		},
		{
			in:       "github:acme/unknown",
			name:     "unknown",
			repo:     "acme/unknown",
			universe: "releases",
			match: registry.Match{
				By:    "repo",
				Value: "acme/unknown",
				Layer: registry.LayerExplicit,
			},
			repoLayer: registry.LayerExplicit,
		},
		{
			in:       "github:jdx/fnox",
			name:     "fnox",
			repo:     "jdx/fnox",
			universe: "releases",
			match: registry.Match{
				By:    "repo",
				Value: "jdx/fnox",
				Layer: registry.LayerMise,
			},
			repoLayer: registry.LayerMise,
		},
		{
			in: "awscli-local", name: "awscli-local", repo: "", universe: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			res, err := r.Resolve(tt.in)
			if tt.repo == "" {
				require.ErrorIs(t, err, registry.ErrNoRepo)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.name, res.Tool.Name)
			assert.Equal(t, tt.repo, res.Tool.Repo)
			assert.Equal(t, tt.prefix, res.Tool.Version.TagPrefix)
			assert.Equal(t, tt.universe, res.Tool.Version.Universe)
			assert.Equal(t, tt.match, res.Match)
			assert.Equal(t, []registry.Layer{tt.repoLayer}, res.Provenance["repo"])
		})
	}
}

func TestIdentify(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{Aliases: map[string]string{"TG_VERSION": "terragrunt"}})
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"terraform", "terraform", true},
		{"tf", "terraform", true},
		{"TERRAFORM_VERSION", "terraform", true},
		{"TG_VERSION", "terragrunt", true},
		{"aqua:hashicorp/terraform", "terraform", true},
		{"github:gruntwork-io/terragrunt", "terragrunt", true},
		{"github:acme/unknown-cli", "github:acme/unknown-cli", true},
		{"uv", "uv", true},
		{"FOO_VERSION", "", false},
		{"http:kafka", "", false},
		{"tfprovider:hashicorp/aws", "", false},
		{"github:not-a-repo", "", false},
		{"aqua:no/such-package", "", false},
		{" ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, ok := r.Identify(tt.in)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()
	r := snapshotResolver(t, registry.ConfigLayer{Aliases: map[string]string{"X_VERSION": "nothing-like-this"}})
	tests := []struct {
		in   string
		want error
		msg  string
	}{
		{"", registry.ErrUnknownTool, "empty name"},
		{"no-such-tool-anywhere", registry.ErrUnknownTool, "use github:owner/repo or aqua:package"},
		{"X_VERSION", registry.ErrUnknownTool, "which no registry knows"},
		{"tfprovider:hashicorp/aws", registry.ErrUnsupported, "tfprovider"},
		{"chart:argo/argo-cd", registry.ErrUnsupported, "chart"},
		{"core:node", registry.ErrUnsupported, `unknown prefix "core:" (relgap knows github and aqua)`},
		{"github:not-a-repo", registry.ErrUnsupported, "github:owner/repo"},
		{"aqua:nope/nope", registry.ErrUnknownTool, "aqua registry snapshot"},
		{"node", registry.ErrNoRepo, "core:node"},
	}
	for _, tt := range tests {
		_, err := r.Resolve(tt.in)
		require.ErrorIs(t, err, tt.want, tt.in)
		assert.ErrorContains(t, err, tt.msg, tt.in)
	}
}

// TestMiseAliasEntries checks that the extra alias entries of `mise registry --json` don't change
// a resolution: live "awscli" repeats "aws-cli".
func TestMiseAliasEntries(t *testing.T) {
	t.Parallel()
	in := registry.Inputs{
		Mise: []misex.RegistryEntry{
			{Short: "aws-cli", Backends: []string{"github:aws/aws-cli"}, Aliases: []string{"aws", "awscli"}},
			{Short: "awscli", Backends: []string{"github:aws/aws-cli"}, Aliases: []string{"aws", "awscli"}},
			{Short: "tool", Backends: []string{"ubi:acme/tool[exe=t]"}},
		},
	}
	r := registry.NewResolver(in)
	for _, name := range []string{"awscli", "aws", "aws-cli"} {
		res, err := r.Resolve(name)
		require.NoError(t, err)
		assert.Equal(t, "aws-cli", res.Tool.Name, name)
		assert.Equal(t, []string{"aws", "awscli"}, res.Tool.Aliases, name)
	}
	res, err := r.Resolve("tool")
	require.NoError(t, err)
	assert.Equal(t, "acme/tool", res.Tool.Repo)
}

func TestLoadMiseFailure(t *testing.T) {
	t.Parallel()
	bin := fakebin.Install(t, "mise", fakebin.Spec{Rules: []fakebin.Rule{{Stderr: "unknown flag --json", ExitCode: 2}}})
	environ := []string{"PATH=" + filepath.Dir(bin)}
	runner := execx.NewRunner(discard(), execx.WithEnviron(func() []string { return environ }))
	r, err := registry.Load(t.Context(), discard(), misex.New(runner), registry.ConfigLayer{})
	require.NoError(t, err)
	res, err := r.Resolve("terragrunt")
	require.NoError(t, err)
	assert.Equal(t, registry.MiseSourceSnapshot, res.MiseSource, "an old or broken mise falls back to the snapshot")
}

func TestSnapshots(t *testing.T) {
	t.Parallel()
	meta, mise, err := registry.MiseSnapshot()
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/jdx/mise", meta.Source)
	assert.Len(t, meta.Commit, 40)
	assert.Greater(t, len(mise), 1000)

	meta, aqua, err := registry.AquaSnapshot()
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/aquaproj/aqua-registry", meta.Source)
	assert.Greater(t, len(aqua), 2000)
}
