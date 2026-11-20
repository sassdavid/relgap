package cli_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rogpeppe/go-internal/testscript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

// sentHeaders records the credential headers of every request by host, then passes it on.
type sentHeaders struct {
	mu   sync.Mutex
	seen map[string][]string // host to "Header: value" lines
	next http.RoundTripper
}

func (s *sentHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	for _, k := range []string{"Authorization", "Private-Token", "Job-Token"} {
		if v := req.Header.Get(k); v != "" {
			s.seen[req.URL.Host] = append(s.seen[req.URL.Host], k+": "+v)
		}
	}
	if _, ok := s.seen[req.URL.Host]; !ok {
		s.seen[req.URL.Host] = nil
	}
	s.mu.Unlock()
	return s.next.RoundTrip(req) //nolint:wrapcheck // a transport passes errors through unchanged
}

const nonGitHubProject = `module "network" {
  source = "git::git@bitbucket.org:example-ws/tf-network.git//.?ref=2.1.0"
}

module "dns" {
  source = "git::https://gitlab.com/example-group/platform/tf-dns.git//modules/zone?ref=v1.2.0"
}

module "vpc" {
  source = "git::ssh://git@gitlab.internal.test/infra/modules/tf-vpc.git?ref=0.4.0"
}
`

// runNonGitHub runs `relgap outdated` over the Bitbucket and GitLab fixture with env and returns
// stderr and the headers each host got. The cache lives in the returned directory.
func runNonGitHub(t *testing.T, env app.Env) (string, map[string][]string, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(nonGitHubProject), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".relgap.yaml"), []byte(
		"schema_version: 1\ngitlab:\n  hosts:\n    - host: gitlab.internal.test\n      token_env: VPC_GITLAB_TOKEN\n"),
		0o600))
	chain, err := openCassettes([]string{
		filepath.Join(cassetteDir, "non-github", "bitbucket.yaml"),
		filepath.Join(cassetteDir, "non-github", "gitlab.yaml"),
	})
	require.NoError(t, err)
	sent := &sentHeaders{seen: map[string][]string{}, next: chain}
	cache := filepath.Join(dir, "cache")
	var stdout, stderr bytes.Buffer
	d := &app.Deps{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Env:   append(app.Env{"HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "XDG_CACHE_HOME=" + cache}, env...),
		Clock: func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) },
		Getwd: func() (string, error) { return dir, nil }, HTTPTransport: sent,
	}
	code := cli.Execute(t.Context(), d, []string{"outdated", dir, "--format", "json"})
	require.Equal(t, cli.ExitOK, code, stderr.String())
	return stderr.String(), sent.seen, cache
}

// TestSpec_non_github_sources_3: tokens go only to the host they belong to, never into the cache
// or a cassette, and a missing token gives the anonymous limits with a warning when they run low.
func TestSpec_non_github_sources_3(t *testing.T) {
	t.Parallel()
	const (
		ghToken  = "gh-secret-token"
		bbToken  = "bb-secret-token"
		glToken  = "gl-secret-token"
		vpcToken = "vpc-secret-token"
		jobToken = "job-secret-token"
	)
	secrets := []string{ghToken, bbToken, glToken, vpcToken, jobToken}

	t.Run("each token reaches its own host only", func(t *testing.T) {
		t.Parallel()
		stderr, seen, cache := runNonGitHub(t, app.Env{
			"GH_TOKEN=" + ghToken, "BITBUCKET_API_TOKEN=" + bbToken, "GITLAB_TOKEN=" + glToken,
			"VPC_GITLAB_TOKEN=" + vpcToken, "CI_JOB_TOKEN=" + jobToken, "CI_SERVER_HOST=gitlab.internal.test",
		})
		assert.Equal(t, map[string][]string{
			"api.bitbucket.org":    repeat("Authorization: Bearer "+bbToken, len(seen["api.bitbucket.org"])),
			"gitlab.com":           repeat("Private-Token: "+glToken, len(seen["gitlab.com"])),
			"gitlab.internal.test": repeat("Private-Token: "+vpcToken, len(seen["gitlab.internal.test"])),
		}, seen, "the token_env of gitlab.hosts wins over CI_JOB_TOKEN, and GH_TOKEN goes nowhere")
		assert.NotEmpty(t, seen["api.bitbucket.org"])
		assert.NotEmpty(t, seen["gitlab.com"])
		assert.NotEmpty(t, seen["gitlab.internal.test"])
		assert.NotContains(t, stderr, "no Bitbucket token")
		assertNoSecret(t, cache, secrets)
	})

	t.Run("GITLAB_HOST keeps GITLAB_TOKEN off gitlab.com, and CI_JOB_TOKEN goes to CI_SERVER_HOST", func(t *testing.T) {
		t.Parallel()
		_, seen, cache := runNonGitHub(t, app.Env{
			"GITLAB_TOKEN=" + glToken, "GITLAB_HOST=https://gitlab.other.test",
			"CI_JOB_TOKEN=" + jobToken, "CI_SERVER_HOST=gitlab.com",
		})
		assert.Empty(t, seen["api.bitbucket.org"])
		assert.Equal(t, repeat("Job-Token: "+jobToken, len(seen["gitlab.com"])), seen["gitlab.com"])
		assert.Empty(t, seen["gitlab.internal.test"], "no token_env value and GITLAB_HOST names another host")
		assertNoSecret(t, cache, secrets)
	})

	t.Run("without tokens the requests are anonymous and the warnings say so", func(t *testing.T) {
		t.Parallel()
		stderr, seen, _ := runNonGitHub(t, nil)
		for host, headers := range seen {
			assert.Empty(t, headers, host)
		}
		assert.Contains(t, stderr, "WRN no Bitbucket token found; anonymous requests are limited to 60 an hour per IP")
		assert.Contains(
			t,
			stderr,
			"WRN rate limit almost used up; slowing down host=gitlab.internal.test remaining=40 limit=500",
		)
		assert.NotContains(t, stderr, "no GitHub token found", "no request went to GitHub")
	})

	t.Run("the cassette scrubber drops every credential header", func(t *testing.T) {
		t.Parallel()
		i := &cassette.Interaction{
			Request: cassette.Request{
				Method: http.MethodGet, URL: "https://gitlab.com/api/v4/projects/a%2Fb/releases",
				Headers: http.Header{
					"Authorization": {"Bearer " + bbToken}, "Private-Token": {glToken}, "Job-Token": {jobToken},
				},
			},
			Response: cassette.Response{Headers: http.Header{"Content-Type": {"application/json"}}},
		}
		require.NoError(t, ghtest.Scrubber(secrets...).Scrub(i))
		for _, k := range vcr.DefaultHeaders() {
			assert.Empty(t, i.Request.Headers.Get(k), k)
		}
	})
}

func repeat(s string, n int) []string {
	if n == 0 {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// assertNoSecret fails when a cache entry, its metadata line or its zstd body, holds a secret.
func assertNoSecret(t *testing.T, dir string, secrets []string) {
	t.Helper()
	dec, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer dec.Close()
	entries := 0
	require.NoError(t, filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		r := bufio.NewReader(f)
		meta, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		body, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, ".zst") {
			entries++
			if body, err = dec.DecodeAll(body, nil); err != nil {
				return err
			}
		}
		for _, s := range secrets {
			assert.NotContains(t, meta+string(body), s, p)
		}
		return nil
	}))
	assert.Positive(t, entries, "the run wrote cache entries")
}

// TestSpec_non_github_sources_5: relgap pins --unresolved no longer lists Bitbucket and GitLab
// module sources or image pins, and the network page names every host and token they use.
func TestSpec_non_github_sources_5(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_non_github_sources_5.txtar")))
	var docs strings.Builder
	for _, f := range []string{"concepts/network.md", "parts/network-hosts.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "docs", filepath.FromSlash(f)))
		require.NoError(t, err)
		docs.Write(b)
	}
	for _, want := range []string{
		"`api.bitbucket.org`", "`gitlab.com`", "a host in `gitlab.hosts`", "`registry-1.docker.io`",
		"`auth.docker.io`", "`ghcr.io`", "`registry.k8s.io`", "`quay.io`", "`public.ecr.aws`",
		"`mcr.microsoft.com`", "a registry in `images.registries`",
		"`BITBUCKET_API_TOKEN`", "`GITLAB_TOKEN`", "`GITLAB_ACCESS_TOKEN`", "`GITLAB_HOST`", "`CI_JOB_TOKEN`",
		"`CI_SERVER_HOST`", "`token_env`", "`PRIVATE-TOKEN`", "`JOB-TOKEN`",
	} {
		assert.Contains(t, docs.String(), want)
	}
}
