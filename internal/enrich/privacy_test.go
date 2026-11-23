package enrich_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
)

// osvRecorder answers every querybatch with no records and keeps the package names it was asked
// about.
type osvRecorder struct {
	mu    sync.Mutex
	names []string
}

func (o *osvRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var in struct {
		Queries []struct {
			Package struct {
				Name string `json:"name"`
			} `json:"package"`
		} `json:"queries"`
	}
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &in); err != nil {
			return nil, err
		}
	}
	o.mu.Lock()
	results := make([]string, len(in.Queries))
	for i, q := range in.Queries {
		o.names = append(o.names, q.Package.Name)
		results[i] = `{"vulns":[]}`
	}
	o.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body:    io.NopCloser(strings.NewReader(`{"results":[` + strings.Join(results, ",") + `]}`)),
		Request: req,
	}, nil
}

func (o *osvRecorder) sent() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.names)
}

// fakeVisibility answers from a fixed map, public for a repository it doesn't list, and keeps
// what it was asked.
type fakeVisibility struct {
	mu      sync.Mutex
	answers map[string]error // by host/path
	asked   []string
}

func (f *fakeVisibility) Public(_ context.Context, repos []enrich.Repo) (map[enrich.Repo]error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[enrich.Repo]error{}
	for _, r := range repos {
		f.asked = append(f.asked, r.ModulePath())
		out[r] = f.answers[r.ModulePath()]
	}
	return out, nil
}

func userTool(name, host, repo string) registry.ToolSpec {
	t := repoTool(name, host, repo, "v")
	t.UserRepo = true
	return t
}

// TestSpec_vulnerability_audit_1_private_repos: a repository the user named reaches OSV only when
// its forge says it is public, and a module GOPRIVATE names never does. The request recorder
// shows no query for a private pin; registry tools need no visibility lookup.
func TestSpec_vulnerability_audit_1_private_repos(t *testing.T) {
	t.Parallel()
	vis := &fakeVisibility{answers: map[string]error{
		"github.com/acme/private-tool": fmt.Errorf("acme/private-tool: %w", enrich.ErrNotPublic),
		"gitlab.com/acme/group/tool":   fmt.Errorf("acme/group/tool: %w", enrich.ErrNotPublic),
		"github.com/acme/deploy":       fmt.Errorf("acme/deploy: %w", enrich.ErrNotPublic),
		"github.com/acme/flaky":        errors.New("connection reset"),
		"github.com/acme/cached":       fmt.Errorf("%w: GET https://api.github.com/graphql", httpx.ErrOffline),
	}}
	ginkgo := repoTool("ginkgo", "", "onsi/ginkgo", "v")
	ginkgo.Backend = "go:github.com/onsi/ginkgo/v2/ginkgo"
	internal := repoTool("internal-cli", "", "corp/internal-cli", "v")
	internal.Backend = "go:go.corp.example/internal/cmd/cli"
	tests := []struct {
		name   string
		tool   registry.ToolSpec
		reason string
	}{
		{
			"github:acme/private-tool",
			userTool("github:acme/private-tool", "", "acme/private-tool"),
			enrich.ReasonPrivate,
		},
		{
			"gitlab:acme/group/tool",
			userTool("gitlab:acme/group/tool", "gitlab.com", "acme/group/tool"),
			enrich.ReasonPrivate,
		},
		{"action:acme/deploy/sub", userTool("action:acme/deploy/sub", "", "acme/deploy"), enrich.ReasonPrivate},
		{"github:acme/flaky", userTool("github:acme/flaky", "", "acme/flaky"), enrich.ReasonFailed},
		{"github:acme/cached", userTool("github:acme/cached", "", "acme/cached"), enrich.ReasonOffline},
		// GOPRIVATE decides before any lookup.
		{"deploy-cli", userTool("deploy-cli", "", "secretco/deploy-cli"), enrich.ReasonPrivate},
		{"internal-cli", internal, enrich.ReasonPrivate},
		{"github:acme/public-tool", userTool("github:acme/public-tool", "", "acme/public-tool"), ""},
		{"k9s", repoTool("k9s", "", "derailed/k9s", "v"), ""},
		{"ginkgo", ginkgo, ""},
	}
	osv := &osvRecorder{}
	hc := &http.Client{Transport: osv}
	reqs := make([]enrich.Request, 0, len(tests))
	for _, tt := range tests {
		reqs = append(reqs, enrich.Request{Key: tt.name, Name: tt.name, Tool: tt.tool, Current: semver(t, "1.2.3")})
	}
	en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, enrich.NewOSVClient(hc, ""), 2)
	en.SetPrivacy(enrich.Privacy{Go: enrich.NewGoPrivate("github.com/secretco", "*.corp.example"), Repos: vis})
	res, problems, err := en.Enrich(t.Context(), reqs)
	require.NoError(t, err)

	for _, tt := range tests {
		assert.Equal(t, tt.reason, res[tt.name].Unchecked, tt.name)
	}
	require.Len(t, problems, 1)
	assert.Equal(
		t,
		enrich.Problem{Source: enrich.SourceOSV, Tool: "github:acme/flaky", Err: vis.answers["github.com/acme/flaky"]},
		problems[0],
	)
	assert.ElementsMatch(t, []string{
		"github.com/acme/private-tool", "gitlab.com/acme/group/tool", "github.com/acme/deploy",
		"github.com/acme/flaky", "github.com/acme/cached", "github.com/acme/public-tool",
	}, vis.asked, "only the repositories the user named, and none GOPRIVATE names")
	sent := osv.sent()
	for _, name := range sent {
		for _, private := range []string{"private-tool", "group/tool", "acme/deploy", "flaky", "cached", "secretco", "corp.example"} {
			assert.NotContains(t, name, private, "OSV heard about a private pin")
		}
	}
	assert.Contains(t, sent, "https://github.com/acme/public-tool")
	assert.Contains(t, sent, "https://github.com/derailed/k9s")
	assert.Contains(t, sent, "github.com/onsi/ginkgo/v2/ginkgo", "the go: prefix search of a registry tool")
}

// TestPrivacyWithoutVisibility: without a forge to ask, every repository the user named is
// private-source.
func TestPrivacyWithoutVisibility(t *testing.T) {
	t.Parallel()
	osv := &osvRecorder{}
	en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil,
		enrich.NewOSVClient(&http.Client{Transport: osv}, ""), 1)
	res, problems, err := en.Enrich(t.Context(), []enrich.Request{
		{
			Key:     "a",
			Name:    "github:acme/tool",
			Tool:    userTool("github:acme/tool", "", "acme/tool"),
			Current: semver(t, "1.0.0"),
		},
	})
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, enrich.ReasonPrivate, res["a"].Unchecked)
	assert.Empty(t, osv.sent())
}

// TestPrivacyBinaries: the main module of an executable reaches OSV only when its tool comes from
// a registry or its forge says it is public, and GOPRIVATE doesn't name it. The standard library
// is always asked about.
func TestPrivacyBinaries(t *testing.T) {
	t.Parallel()
	vis := &fakeVisibility{answers: map[string]error{
		"github.com/acme/private-tool": fmt.Errorf("acme/private-tool: %w", enrich.ErrNotPublic),
	}}
	bins := []*enrich.Binary{
		enrich.NewTestBinary(repoTool("k9s", "", "derailed/k9s", "v"), "1.24.3", "github.com/derailed/k9s", "0.50.16"),
		enrich.NewTestBinary(userTool("github:acme/private-tool", "", "acme/private-tool"), "1.24.3",
			"github.com/acme/private-tool", "1.0.0"),
		enrich.NewTestBinary(userTool("github:acme/public-tool", "", "acme/public-tool"), "1.24.3",
			"github.com/acme/public-tool", "2.0.0"),
		enrich.NewTestBinary(repoTool("vanity", "", "corp/vanity", "v"), "1.24.3", "go.corp.example/vanity", "1.0.0"),
		enrich.NewTestBinary(userTool("bitbucket:team/tool", "bitbucket.org", "team/tool"), "1.24.3",
			"bitbucket.org/team/tool", "1.0.0"),
	}
	osv := &osvRecorder{}
	en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil,
		enrich.NewOSVClient(&http.Client{Transport: osv}, ""), 1)
	en.SetPrivacy(enrich.Privacy{Go: enrich.NewGoPrivate("go.corp.example"), Repos: vis})
	problems, err := en.BinaryVulns(t.Context(), bins)
	require.NoError(t, err)
	assert.Empty(t, problems)

	unchecked := make([]string, len(bins))
	for i, b := range bins {
		unchecked[i] = b.ModuleUnchecked
	}
	assert.Equal(t, []string{"", enrich.ReasonPrivate, "", enrich.ReasonPrivate, enrich.ReasonPrivate}, unchecked)
	assert.ElementsMatch(t, []string{"github.com/acme/private-tool", "github.com/acme/public-tool"}, vis.asked)
	assert.ElementsMatch(t, []string{
		"stdlib", "stdlib", "stdlib", "stdlib", "stdlib", "github.com/derailed/k9s", "github.com/acme/public-tool",
	}, osv.sent())
}

func TestGoPrivateMatches(t *testing.T) {
	t.Parallel()
	g := enrich.NewGoPrivate("*.corp.example,github.com/acme", " gitlab.com/team/* ", "")
	tests := []struct {
		path string
		want bool
	}{
		{"git.corp.example/x/y", true},
		{"corp.example/x", false},
		{"github.com/acme", true},
		{"github.com/acme/tool/v2", true},
		{"github.com/acmeco/tool", false},
		{"gitlab.com/team/app/cmd", true},
		{"gitlab.com/team", false},
		{"github.com/derailed/k9s", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, g.Matches(tt.path), tt.path)
	}
	assert.False(t, enrich.NewGoPrivate().Matches("github.com/acme"))
}

// TestReadGoPrivate: a non-empty variable wins over the go command's env file, as cmd/go reads
// them, and the three variables add up.
func TestReadGoPrivate(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	file := filepath.Join(home, ".config", "go", "env")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o750))
	require.NoError(t, os.WriteFile(file,
		[]byte("GOPRIVATE=github.com/fromfile\nGONOSUMDB=gitlab.com/nosum\n# comment\nbad line\n"), 0o600))
	custom := filepath.Join(home, "custom.env")
	require.NoError(t, os.WriteFile(custom, []byte("GOPRIVATE=github.com/custom\n"), 0o600))
	tests := []struct {
		name string
		env  map[string]string
		goos string
		want []string
	}{
		{"file", map[string]string{"HOME": home}, "linux", []string{"github.com/fromfile", "gitlab.com/nosum"}},
		{
			"env wins",
			map[string]string{"HOME": home, "GOPRIVATE": "github.com/fromenv", "GONOPROXY": ""},
			"linux",
			[]string{"github.com/fromenv", "gitlab.com/nosum"},
		},
		{
			"xdg",
			map[string]string{"HOME": "/nowhere", "XDG_CONFIG_HOME": filepath.Join(home, ".config")},
			"linux",
			[]string{"github.com/fromfile", "gitlab.com/nosum"},
		},
		{"GOENV", map[string]string{"HOME": home, "GOENV": custom}, "linux", []string{"github.com/custom"}},
		{"GOENV off", map[string]string{"HOME": home, "GOENV": "off"}, "linux", nil},
		{"darwin", map[string]string{"HOME": home}, "darwin", nil},
		{
			"windows",
			map[string]string{"APPDATA": filepath.Join(home, ".config")},
			"windows",
			[]string{"github.com/fromfile", "gitlab.com/nosum"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lookup := func(k string) (string, bool) { v, ok := tt.env[k]; return v, ok }
			assert.Equal(t, tt.want, enrich.ReadGoPrivate(lookup, tt.goos).Patterns())
		})
	}
}

// TestGoPrivateCuratedPackage: GOPRIVATE drops a curated Go package it names and keeps the
// others, and stdlib and toolchain always go.
func TestGoPrivateCuratedPackage(t *testing.T) {
	t.Parallel()
	goTool := spec("go", nil, registry.OSVPackage{Ecosystem: "Go", Name: "stdlib"},
		registry.OSVPackage{Ecosystem: "Go", Name: "toolchain"})
	mixed := spec("mixed", nil, registry.OSVPackage{Ecosystem: "Go", Name: "github.com/secretco/lib"},
		registry.OSVPackage{Ecosystem: "npm", Name: "mixed"})
	osv := &osvRecorder{}
	en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil,
		enrich.NewOSVClient(&http.Client{Transport: osv}, ""), 1)
	en.SetPrivacy(enrich.Privacy{Go: enrich.NewGoPrivate("*")})
	res, _, err := en.Enrich(t.Context(), []enrich.Request{
		{Key: "go", Tool: goTool, Current: semver(t, "1.24.3")},
		{Key: "mixed", Tool: mixed, Current: semver(t, "1.0.0")},
	})
	require.NoError(t, err)
	assert.Empty(t, res["go"].Unchecked)
	assert.Empty(t, res["mixed"].Unchecked)
	assert.ElementsMatch(t, []string{"stdlib", "toolchain", "mixed"}, osv.sent())
}
