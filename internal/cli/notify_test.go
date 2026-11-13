package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/schemas"
)

// fakePR is the part of the GitHub REST API the pull request comment uses.
type fakePR struct {
	mu       sync.Mutex
	srv      *httptest.Server
	comments []map[string]any
	writes   []string
}

const prComments = "/repos/acme/tools/issues/7/comments"

func newFakePR(t *testing.T) *fakePR {
	t.Helper()
	f := &fakePR{comments: []map[string]any{{"id": 1, "body": "Looks good to me.", "html_url": "u1"}}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePR) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer gh-test-token" {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	b, _ := io.ReadAll(r.Body)
	var in struct {
		Body string `json:"body"`
	}
	_ = json.Unmarshal(b, &in)
	if r.Method != http.MethodGet {
		f.writes = append(f.writes, r.Method+" "+r.URL.Path)
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == prComments:
		_ = json.NewEncoder(w).Encode(f.comments)
	case r.Method == http.MethodPost && r.URL.Path == prComments:
		c := map[string]any{
			"id":       len(f.comments) + 1,
			"body":     in.Body,
			"html_url": "u" + strconv.Itoa(len(f.comments)+1),
		}
		f.comments = append(f.comments, c)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(c)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/repos/acme/tools/issues/comments/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/repos/acme/tools/issues/comments/"))
		f.comments[id-1]["body"] = in.Body
		_ = json.NewEncoder(w).Encode(f.comments[id-1])
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

// takeWrites returns the writes since the last call.
func (f *fakePR) takeWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.writes
	f.writes = nil
	return out
}

func (f *fakePR) body(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, _ := f.comments[i]["body"].(string)
	return s
}

type notifyEnv struct {
	dir   string
	env   app.Env
	fake  *fakePR
	clock time.Time
}

func newNotifyEnv(t *testing.T) *notifyEnv {
	t.Helper()
	dir := t.TempDir()
	event := filepath.Join(dir, "event.json")
	require.NoError(t, os.WriteFile(event, []byte(`{"pull_request":{"number":7}}`), 0o600))
	f := newFakePR(t)
	return &notifyEnv{
		dir:  dir,
		fake: f,
		env: app.Env{
			"HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
			"GITHUB_REPOSITORY=acme/tools", "GITHUB_EVENT_PATH=" + event, "GITHUB_API_URL=" + f.srv.URL,
			"GH_TOKEN=gh-test-token",
		},
		clock: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC),
	}
}

func (n *notifyEnv) run(t *testing.T, env app.Env, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	d := &app.Deps{
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Env: env,
		Clock: func() time.Time { return n.clock }, Getwd: func() (string, error) { return n.dir, nil },
	}
	code := cli.Execute(t.Context(), d, args)
	return code, stdout.String(), stderr.String()
}

// driftJSON saves `relgap drift --format json` over two repositories: one pins kubectl 1.34.2,
// the other other.
func (n *notifyEnv) driftJSON(t *testing.T, name, other string) string {
	t.Helper()
	root := filepath.Join(n.dir, name)
	for i, v := range []string{"1.34.2", other} {
		repo := filepath.Join(root, fmt.Sprintf("repo-%d", i))
		require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(repo, "mise.toml"),
			[]byte("[tools]\nkubectl = \""+v+"\"\n"), 0o600))
	}
	code, stdout, stderr := n.run(t, n.env, "drift", root, "--format", "json")
	require.Equal(t, cli.ExitOK, code, stderr)
	out := filepath.Join(n.dir, name+".json")
	require.NoError(t, os.WriteFile(out, []byte(stdout), 0o600))
	return out
}

// TestNotifyPRComment runs the comment lifecycle on saved drift reports: create, unchanged (no
// write), dry run, update, resolve to "Nothing to report", and unchanged again.
func TestNotifyPRComment(t *testing.T) {
	t.Parallel()
	n := newNotifyEnv(t)
	drifted := n.driftJSON(t, "drifted", "1.33.5")
	more := n.driftJSON(t, "more", "1.32.0")
	agreed := n.driftJSON(t, "agreed", "1.34.2")
	notify := func(from string, extra ...string) (int, string, string) {
		return n.run(t, n.env, append([]string{"notify", "--from", from, "--notify", "github-pr-comment"}, extra...)...)
	}

	code, stdout, stderr := notify(agreed)
	require.Equal(t, cli.ExitOK, code, stderr)
	assert.Equal(t, "github-pr-comment  acme/tools#7  skipped  drift\n", stdout)
	assert.Empty(t, n.fake.takeWrites())

	code, stdout, stderr = notify(drifted)
	require.Equal(t, cli.ExitOK, code, stderr)
	assert.Equal(t, "github-pr-comment  acme/tools#7  create  drift\n", stdout)
	assert.Equal(t, []string{"POST " + prComments}, n.fake.takeWrites())
	body := n.fake.body(1)
	assert.True(t, strings.HasPrefix(body, `<!-- relgap:v1 {"kind":"drift","scope":"pull-request","hash":"sha256:`))
	assert.Contains(t, body, "## relgap drift\n\n")
	assert.Contains(t, body, "1.33.5")

	code, stdout, _ = notify(drifted)
	require.Equal(t, cli.ExitOK, code)
	assert.Equal(t, "github-pr-comment  acme/tools#7  unchanged  drift\n", stdout)
	assert.Empty(t, n.fake.takeWrites())

	code, stdout, _ = notify(more, "--dry-run-notify", "--format", "json")
	require.Equal(t, cli.ExitOK, code)
	require.NoError(t, validateJSON(schemas.ReportV1, []byte(stdout)))
	assert.Contains(t, stdout, `"op": "update"`)
	assert.Contains(t, stdout, `"dry_run": true`)
	assert.Empty(t, n.fake.takeWrites())

	code, _, _ = notify(more)
	require.Equal(t, cli.ExitOK, code)
	assert.Equal(t, []string{"PATCH /repos/acme/tools/issues/comments/2"}, n.fake.takeWrites())
	assert.Contains(t, n.fake.body(1), "1.32.0")

	code, stdout, _ = notify(agreed)
	require.Equal(t, cli.ExitOK, code)
	assert.Equal(t, "github-pr-comment  acme/tools#7  resolve  drift\n", stdout)
	assert.Contains(t, n.fake.body(1), "## relgap drift\n\nNothing to report.\n")
	n.fake.takeWrites()

	code, stdout, _ = notify(agreed)
	require.Equal(t, cli.ExitOK, code)
	assert.Equal(t, "github-pr-comment  acme/tools#7  unchanged  drift\n", stdout)
	assert.Empty(t, n.fake.takeWrites())
}

func without(env app.Env, key string) app.Env {
	var out app.Env
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

func TestNotifyErrors(t *testing.T) {
	t.Parallel()
	n := newNotifyEnv(t)
	drifted := n.driftJSON(t, "drifted", "1.33.5")
	pins := filepath.Join(n.dir, "pins.json")
	code, stdout, stderr := n.run(t, n.env, "pins", filepath.Join(n.dir, "drifted"), "--format", "json")
	require.Equal(t, cli.ExitOK, code, stderr)
	require.NoError(t, os.WriteFile(pins, []byte(stdout), 0o600))
	other := filepath.Join(n.dir, "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"tool":"tfcensus","data":{}}`), 0o600))

	tests := []struct {
		name string
		env  app.Env
		args []string
		code int
		want string
	}{
		{
			"no from",
			n.env,
			[]string{"notify", "--notify", "github-pr-comment"},
			cli.ExitUsage,
			"pass at least one --from",
		},
		{"no notifier", n.env, []string{"notify", "--from", drifted}, cli.ExitUsage, "pass --notify github-pr-comment"},
		{
			"unknown notifier", n.env,
			[]string{"notify", "--from", drifted, "--notify", "slack"},
			cli.ExitUsage,
			`unknown notifier "slack"`,
		},
		{
			"pins report", n.env,
			[]string{"notify", "--from", pins, "--notify", "github-pr-comment"},
			cli.ExitUsage,
			"a pins report has nothing to publish",
		},
		{
			"other tool", n.env,
			[]string{"notify", "--from", other, "--notify", "github-pr-comment"},
			cli.ExitUsage,
			`not a relgap report (tool "tfcensus")`,
		},
		{
			"no event", without(n.env, "GITHUB_EVENT_PATH"),
			[]string{"notify", "--from", drifted, "--notify", "github-pr-comment"},
			cli.ExitUsage, "github-pr-comment: pull request comment not configured: GITHUB_EVENT_PATH is empty",
		},
		{
			"bad token",
			append(without(n.env, "GH_TOKEN"), "GH_TOKEN=wrong"),
			[]string{"notify", "--from", drifted, "--notify", "github-pr-comment"},
			cli.ExitAuth,
			"HTTP 401",
		},
	}
	for _, tt := range tests {
		got, _, errOut := n.run(t, tt.env, tt.args...)
		assert.Equal(t, tt.code, got, tt.name)
		assert.Contains(t, errOut, tt.want, tt.name)
	}
}
