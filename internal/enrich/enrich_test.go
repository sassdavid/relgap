package enrich_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
)

const testdata = "../../testdata"

// clients returns EOL and OSV clients that answer from a cassette, without retries, so a recorded
// failure comes back at once.
func clients(t *testing.T, cassette string) (*enrich.EOLClient, *enrich.OSVClient) {
	t.Helper()
	hc := &http.Client{
		Transport: vcr.ForTest(t, filepath.Join(testdata, "cassettes", "enrich", cassette), ghtest.Scrubber()),
	}
	return enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, "")
}

func semver(t *testing.T, s string) version.V {
	t.Helper()
	v, err := version.Semver().Parse(s)
	require.NoError(t, err)
	return v
}

func spec(name string, eol *registry.EOL, osv ...registry.OSVPackage) registry.ToolSpec {
	return registry.ToolSpec{
		Name:    name,
		Version: registry.VersionSpec{Scheme: version.SchemeSemver, TagPrefix: "v"},
		EOL:     eol,
		OSV:     osv,
	}
}

// meetTransport answers an endoflife.date request only once an OSV query has arrived.
type meetTransport struct {
	once    sync.Once
	osvSeen chan struct{}
}

func (m *meetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"result":{"releases":[{"name":"1.17","isEol":false}]}}`
	if req.URL.Host == "api.osv.dev" {
		m.once.Do(func() { close(m.osvSeen) })
		body = `{"results":[{}]}`
	} else {
		select {
		case <-m.osvSeen:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func TestEnrichAsksBothSourcesAtOnce(t *testing.T) {
	t.Parallel()
	hc := &http.Client{Transport: &meetTransport{osvSeen: make(chan struct{})}}
	terraform := spec("terraform", &registry.EOL{Product: "terraform", Cycle: "{major}.{minor}"},
		registry.OSVPackage{Ecosystem: "Go", Name: "github.com/hashicorp/terraform"})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler),
		enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, ""), 2).
		Enrich(ctx, []enrich.Request{{Key: "tf", Tool: terraform, Current: semver(t, "1.17.2")}})
	require.NoError(t, err, "the endoflife.date request waits for the OSV query")
	assert.Empty(t, problems)
	require.NotNil(t, res["tf"].EOL)
	assert.Equal(t, "1.17", res["tf"].EOL.Current.Cycle)
	assert.Empty(t, res["tf"].Vulns)
}

// agedTransport answers every OSV request with no records, carrying the Age header of its URL
// path, and counts endoflife.date requests.
type agedTransport struct {
	ages map[string]string
	eol  int
}

func (a *agedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "api.osv.dev" {
		a.eol++
	}
	h := http.Header{"Content-Type": {"application/json"}}
	if age := a.ages[req.URL.Path]; age != "" {
		h.Set("Age", age)
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(`{"results":[{}]}`)), Request: req,
	}, nil
}

// TestEnrichOldestAgeWithoutEOL: OldestAge is the largest Age of the OSV answers, and an enricher
// without an EOL client sends no endoflife.date request.
func TestEnrichOldestAgeWithoutEOL(t *testing.T) {
	t.Parallel()
	tr := &agedTransport{ages: map[string]string{"/v1/querybatch": "120"}}
	osv := enrich.NewOSVClient(&http.Client{Transport: tr}, "")
	_, ok := osv.OldestAge()
	assert.False(t, ok, "no answer read yet")
	terraform := spec("terraform", &registry.EOL{Product: "terraform", Cycle: "{major}.{minor}"},
		registry.OSVPackage{Ecosystem: "Go", Name: "github.com/hashicorp/terraform"})
	res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 2).
		Enrich(t.Context(), []enrich.Request{{Key: "tf", Tool: terraform, Current: semver(t, "1.17.2")}})
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Nil(t, res["tf"].EOL)
	assert.NotNil(t, res["tf"].Vulns)
	assert.Zero(t, tr.eol)
	age, ok := osv.OldestAge()
	assert.True(t, ok)
	assert.Equal(t, 2*time.Minute, age)
}

// TestSpec_enrichment_1: terraform 1.15.8 is on an EOL cycle and 1.17.2 isn't; a tool without
// an endoflife.date product gets no eol field and sends no request; an endoflife.date outage is
// a problem for that tool while the other results stand.
func TestSpec_enrichment_1(t *testing.T) {
	t.Parallel()
	terraform := spec("terraform", &registry.EOL{Product: "terraform", Cycle: "{major}.{minor}"})
	helm := spec("helm", nil)

	t.Run("terraform cycle", func(t *testing.T) {
		t.Parallel()
		eol, osv := clients(t, "eol-terraform")
		res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), eol, osv, 2).
			Enrich(t.Context(), []enrich.Request{
				{Key: "tf", Tool: terraform, Current: semver(t, "1.15.8"), Target: semver(t, "1.17.2")},
				{Key: "helm", Tool: helm, Current: semver(t, "3.19.0"), Target: semver(t, "3.22.1")},
			})
		require.NoError(t, err)
		assert.Empty(t, problems)
		got := res["tf"].EOL
		require.NotNil(t, got)
		assert.Equal(t, "terraform", got.Product)
		assert.Equal(t, "1.15", got.Current.Cycle)
		require.NotNil(t, got.Current.IsEOL)
		assert.True(t, *got.Current.IsEOL)
		assert.Equal(t, "2026-10-21", got.Current.EOLFrom)
		assert.Equal(t, "1.17", got.Target.Cycle)
		require.NotNil(t, got.Target.IsEOL)
		assert.False(t, *got.Target.IsEOL)
		assert.Nil(t, res["helm"].EOL, "no endoflife.date product, no eol field")
	})

	t.Run("outage", func(t *testing.T) {
		t.Parallel()
		if vcr.Recording() {
			t.Skip("eol-outage is written by hand: an outage can't be recorded on demand")
		}
		eol, osv := clients(t, "eol-outage")
		res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), eol, osv, 2).
			Enrich(t.Context(), []enrich.Request{
				{Key: "tf", Tool: terraform, Current: semver(t, "1.15.8")},
				{Key: "helm", Tool: helm, Current: semver(t, "3.19.0")},
			})
		require.NoError(t, err)
		require.Len(t, problems, 1)
		assert.Equal(t, enrich.SourceEOL, problems[0].Source)
		assert.Equal(t, "terraform", problems[0].Tool)
		require.ErrorContains(t, problems[0].Err, "HTTP 503")
		assert.Nil(t, res["tf"].EOL)
		assert.Contains(t, res, "helm", "the other tools still have results")
	})
}

func TestStatusMissingCycle(t *testing.T) {
	t.Parallel()
	cycles := []enrich.Cycle{{Name: "1.37", IsMaintained: true}, {Name: "1.36", IsEOL: false}}
	e := registry.EOL{Product: "kubernetes"}
	st := enrich.Status(cycles, e, version.Semver(), semver(t, "1.9.3"))
	assert.Equal(t, "1.9", st.Cycle)
	assert.Nil(t, st.IsEOL)
	assert.Equal(t, "endoflife.date doesn't list cycle 1.9 of kubernetes", st.Note)
	node := registry.EOL{Product: "nodejs", Cycle: "{major}"}
	assert.Equal(t, "24", enrich.CycleName(node, version.Semver(), semver(t, "24.11.0")))
}

// TestSpec_enrichment_4: fixedByTarget follows the fixed events of OSV ranges in the Go
// ecosystem (kubernetes 1.33.0 to 1.37.2, terraform 1.5.0 to 1.17.2) and in a semver one
// (claude-code on npm, 1.0.0 to 2.1.322). For every vuln the local answer equals OSV's own: the
// target's query no longer returns the vuln exactly when fixedByTarget is true.
func TestSpec_enrichment_4(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		tool            registry.ToolSpec
		current, target string
	}{
		{
			"kubernetes",
			spec("kubectl", nil, registry.OSVPackage{Ecosystem: "Go", Name: "k8s.io/kubernetes"}),
			"1.33.0",
			"1.37.2",
		},
		{
			"terraform",
			spec("terraform", nil, registry.OSVPackage{Ecosystem: "Go", Name: "github.com/hashicorp/terraform"}),
			"1.5.0",
			"1.17.2",
		},
		{
			"claude-code",
			spec("claude-code", nil, registry.OSVPackage{Ecosystem: "npm", Name: "@anthropic-ai/claude-code"}),
			"1.0.0",
			"2.1.322",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eol, osv := clients(t, "osv-"+tt.name)
			res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), eol, osv, 2).
				Enrich(t.Context(), []enrich.Request{
					{Key: "k", Tool: tt.tool, Current: semver(t, tt.current), Target: semver(t, tt.target)},
				})
			require.NoError(t, err)
			require.Empty(t, problems)
			vulns := res["k"].Vulns
			require.NotEmpty(t, vulns)

			pkg := enrich.Package{Ecosystem: tt.tool.OSV[0].Ecosystem, Name: tt.tool.OSV[0].Name}
			still, err := osv.QueryBatch(t.Context(), []enrich.Query{{Package: pkg, Version: tt.target}})
			require.NoError(t, err)
			for _, v := range vulns {
				require.NotNil(t, v.FixedByTarget, v.ID)
				assert.True(t, v.AffectsCurrent, v.ID)
				names := append([]string{v.ID}, v.Aliases...)
				osvSaysFixed := !slices.ContainsFunc(names, func(n string) bool { return slices.Contains(still[0], n) })
				assert.Equal(t, osvSaysFixed, *v.FixedByTarget, "%s: fixed_by_target", v.ID)
				if *v.FixedByTarget {
					assert.NotEmpty(t, v.FixedIn, "%s: a fixed vuln names its fix", v.ID)
				}
			}
		})
	}
}

// TestAliasesMerged: OSV returns GO-2023-2055 and GHSA-h626-pv66-hhm7 for terraform 1.5.0; they
// are one vulnerability, kept once with the GitHub advisory's severity.
func TestAliasesMerged(t *testing.T) {
	t.Parallel()
	if vcr.Recording() {
		t.Skip("replays osv-terraform, which TestSpec_enrichment_4 records")
	}
	eol, osv := clients(t, "osv-terraform")
	tool := spec("terraform", nil, registry.OSVPackage{Ecosystem: "Go", Name: "github.com/hashicorp/terraform"})
	res, _, err := enrich.NewEnricher(slog.New(slog.DiscardHandler), eol, osv, 2).Enrich(t.Context(), []enrich.Request{
		{Key: "k", Tool: tool, Current: semver(t, "1.5.0"), Target: semver(t, "1.17.2")},
	})
	require.NoError(t, err)
	seen := map[string]int{}
	for _, v := range res["k"].Vulns {
		for _, id := range append([]string{v.ID}, v.Aliases...) {
			seen[id]++
		}
	}
	for id, n := range seen {
		assert.Equal(t, 1, n, "%s appears once", id)
	}
	assert.Contains(t, seen, "GO-2023-2055")
	assert.Contains(t, seen, "GHSA-h626-pv66-hhm7")
	merged := res["k"].Vulns[0]
	require.NotNil(t, merged.Severity)
	assert.Positive(t, merged.Severity.Score)
}

// TestSpec_enrichment_3: --binaries finds a Go executable built in the test on PATH and asks OSV
// about the standard library at the Go version it was built with. The build runs offline with
// the local toolchain.
func TestSpec_enrichment_3(t *testing.T) {
	t.Parallel()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("needs the go command")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o750))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/fixture\n\ngo 1.22\n"), 0o600),
	)
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o750))
	out := filepath.Join(bin, exe("fixture"))

	build := exec.CommandContext(t.Context(), goBin, "build", "-o", out, ".")
	build.Dir = src
	build.Env = append(os.Environ(),
		"GOTOOLCHAIN=local", "GOPROXY=off", "GOFLAGS=-mod=mod -buildvcs=false", "GOWORK=off", "CGO_ENABLED=0")
	b, err := build.CombinedOutput()
	require.NoError(t, err, string(b))
	goVersion, err := exec.CommandContext(t.Context(), goBin, "env", "GOVERSION").Output()
	require.NoError(t, err)

	env := map[string]string{"PATH": bin, "HOME": dir}
	finder := enrich.BinaryFinder{Lookup: func(k string) (string, bool) { v, ok := env[k]; return v, ok }}
	paths := finder.Find(t.Context(), registry.ToolSpec{Name: "fixture"})
	require.Len(t, paths, 1)

	got, err := enrich.ReadBinary("fixture", paths[0])
	require.NoError(t, err)
	want := enrich.GoSemver(strings.TrimSpace(string(goVersion)))
	assert.Equal(t, want, got.GoVersion)
	assert.Equal(t, "example.com/fixture", got.Module)
	assert.Empty(t, got.ModuleVersion, "a -buildvcs=false build is (devel), which OSV can't match")
	assert.Equal(
		t,
		[]enrich.Query{{Package: enrich.Package{Ecosystem: "Go", Name: "stdlib"}, Version: want}},
		got.Queries,
	)

	_, err = enrich.ReadBinary("fixture", filepath.Join(src, "go.mod"))
	require.Error(t, err, "not an executable")
}

func exe(name string) string {
	if filepath.Separator == '\\' {
		return name + ".exe"
	}
	return name
}

// failTransport fails every OSV request with err and answers endoflife.date with no cycles.
type failTransport struct{ err error }

func (f failTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.osv.dev" {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"result":{"releases":[]}}`)),
		Request:    req,
	}, nil
}

// TestUncheckedWhenOSVFails: a request whose OSV queries fail is unchecked with reason failed,
// or offline when --offline had no cached answer. A request coverage already left unchecked
// keeps its own reason.
func TestUncheckedWhenOSVFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"outage", errors.New("connection refused"), enrich.ReasonFailed},
		{"offline", fmt.Errorf("%w: POST https://api.osv.dev/v1/querybatch", httpx.ErrOffline), enrich.ReasonOffline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hc := &http.Client{Transport: failTransport{err: tt.err}}
			k9s := spec("k9s", nil)
			k9s.Repo = "derailed/k9s"
			alpine := spec("image:alpine", nil)
			alpine.Image = "alpine"
			res, problems, err := enrich.NewEnricher(slog.New(slog.DiscardHandler),
				enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, ""), 2).
				Enrich(t.Context(), []enrich.Request{
					{Key: "k9s", Tool: k9s, Current: semver(t, "0.50.16")},
					{Key: "alpine", Name: "image:alpine", Tool: alpine, Current: semver(t, "3.22.0")},
				})
			require.NoError(t, err)
			require.Len(t, problems, 1)
			assert.Equal(t, enrich.SourceOSV, problems[0].Source)
			assert.Nil(t, res["k9s"].Vulns)
			assert.Equal(t, tt.want, res["k9s"].Unchecked)
			assert.Equal(t, enrich.ReasonNoSource, res["alpine"].Unchecked)
		})
	}
}

func TestSkipReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ghost", fmt.Errorf("%w: ghost", registry.ErrUnknownTool), enrich.ReasonUnresolved},
		{"action:actions/checkout", fmt.Errorf("%w: unknown prefix", registry.ErrUnsupported), enrich.ReasonNoSource},
		{"asdf-only", fmt.Errorf("%w for asdf-only", registry.ErrNoRepo), enrich.ReasonNoSource},
		{
			"image:registry.example.com/team/app",
			fmt.Errorf("%w: not allowed", registry.ErrUnsupported),
			enrich.ReasonPrivate,
		},
		{
			"image:ghcr.io/team/app",
			fmt.Errorf("%w: %w", registry.ErrUnknownTool, httpx.ErrUnauthorized),
			enrich.ReasonPrivate,
		},
		{"chart:argo/argo-cd", fmt.Errorf("look up: %w", httpx.ErrOffline), enrich.ReasonOffline},
		{"tfprovider:hashicorp/aws", errors.New("HTTP 502"), enrich.ReasonFailed},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, enrich.SkipReason(tt.name, tt.err), tt.name)
	}
}
