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
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/testutil"
)

const (
	terraformCassette = "gh-release/terraform-1.15.8..1.17.1.yaml"
	terraformRange    = "1.15.8..1.17.1"
	llmHost           = "llm.test"
)

// fakeLLM is an OpenAI-compatible endpoint. By default it answers with one risk citing the first
// breaking entry it was sent, one claim without an ID and one citing an unknown ID.
type fakeLLM struct {
	mu      sync.Mutex
	calls   int
	bodies  []string
	status  int
	block   bool // wait until the request is cancelled
	answer  func(user string) string
	handler http.Handler
}

var breakingIDRE = regexp.MustCompile(
	`"id":"(e-[0-9a-f]{8}(?:-[0-9]+)?)","type":"entry","tool":"terraform","version":"[^"]+","kind":"breaking"`,
)

func newFakeLLM() *fakeLLM {
	f := &fakeLLM{status: http.StatusOK}
	f.answer = func(user string) string {
		id := breakingIDRE.FindStringSubmatch(user)[1]
		return "RISKS\n- No breaking changes, upgrade freely. [" + id + "]\n" +
			"- Your state is deleted.\nACTIONS\n- Rewrite every module. [e-deadbeef]\n"
	}
	f.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.calls++
		f.bodies = append(f.bodies, string(b))
		status, block := f.status, f.block
		f.mu.Unlock()
		if block {
			<-r.Context().Done()
			return
		}
		var in struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(b, &in)
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = io.WriteString(w, `{"error":{"message":"model overloaded"}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": f.answer(in.Messages[1].Content)},
		}}})
	})
	return f
}

// llmRouter sends requests for llmHost to the fake model and the rest to next.
type llmRouter struct {
	llm  http.Handler
	next http.RoundTripper
}

func (r llmRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != llmHost {
		return r.next.RoundTrip(req) //nolint:wrapcheck // the transport's error is the result
	}
	rec := httptest.NewRecorder()
	r.llm.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req // httpx's retry policy reads the method from it, as from a real transport
	return resp, nil
}

// summaryRun runs relgap in a fixture whose config points llm at the fake model.
type summaryRun struct {
	f   *mcpFixture
	llm *fakeLLM
}

func newSummaryRun(t *testing.T, llmConfig string) *summaryRun {
	t.Helper()
	f := newMCPFixture(t, "2026-11-10T12:00:00Z", terraformCassette)
	llm := newFakeLLM()
	f.d.HTTPTransport = llmRouter{llm: llm.handler, next: f.d.HTTPTransport}
	f.d.Env = append(f.d.Env, "LLM_KEY=sk-test")
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, ".relgap.yaml"), []byte(llmConfig), 0o600))
	return &summaryRun{f: f, llm: llm}
}

const openAIConfig = `llm:
  provider: openai-compatible
  model: test-model
  base_url: http://llm.test/v1
  api_key_env: LLM_KEY
`

func (s *summaryRun) run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	s.f.d.Stdout, s.f.d.Stderr = &stdout, &stderr
	code := cli.Execute(t.Context(), s.f.d, args)
	return code, stdout.String(), stderr.String()
}

func decodeDigest(t *testing.T, out string) report.Digest {
	t.Helper()
	var env struct {
		Data report.Digest `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	return env.Data
}

// TestSummarize_advisory checks that --summarize adds a checked summary and changes nothing else:
// the entries, the --fail-on gate and the exit code are the same as without it, even when the
// model claims there are no breaking changes.
func TestSummarize_advisory(t *testing.T) {
	t.Parallel()
	s := newSummaryRun(t, openAIConfig)
	args := []string{"digest", "terraform", terraformRange, "--format", "json", "--fail-on", "breaking"}
	wantCode, plain, _ := s.run(t, args...)
	require.Equal(t, cli.ExitFindings, wantCode)
	require.Equal(t, 0, s.llm.calls, "no model call without --summarize")

	code, out, stderr := s.run(t, append(args, "--summarize")...)
	assert.Equal(t, wantCode, code, "the summary never changes the exit code")
	require.NoError(t, validateJSON("digest-v1.json", []byte(out)))
	require.Equal(t, 1, s.llm.calls)

	got, want := decodeDigest(t, out), decodeDigest(t, plain)
	assert.Equal(t, want.Tools, got.Tools, "the summary never changes the entries")
	require.NotNil(t, got.Summary)
	assert.Nil(t, want.Summary)
	id := got.Summary.Risks[0].Cites[0]
	assert.Equal(t, &report.Summary{
		Notice:   report.SummaryNotice,
		Provider: "openai-compatible",
		Model:    "test-model",
		Risks:    []report.Claim{{Text: "No breaking changes, upgrade freely.", Cites: []string{id}}},
		Actions:  []report.Claim{},
		Entries:  len(got.Tools[0].Entries),
		Dropped:  2,
	}, got.Summary)
	assert.Contains(t, stderr, "WRN --summarize: dropped 1 summary claim without an entry ID")
	assert.Contains(t, stderr, "WRN --summarize: dropped 1 summary claim citing unknown entry IDs: e-deadbeef")

	body := s.llm.bodies[0]
	assert.NotContains(t, body, s.f.dir, "no local path is sent")
	assert.Contains(t, body, `\u003crelease_notes\u003e`, "entries are delimited")
}

// TestSummarize_formats checks the labelled summary block in md and plain output.
func TestSummarize_formats(t *testing.T) {
	t.Parallel()
	s := newSummaryRun(t, openAIConfig)
	s.llm.answer = func(user string) string {
		id := breakingIDRE.FindStringSubmatch(user)[1]
		return "RISKS\n- A removed command breaks scripts that call it. [" + id + "]\n" +
			"ACTIONS\n- Search CI scripts for the removed command. [" + id + "]\n"
	}
	code, md, _ := s.run(t, "digest", "terraform", terraformRange, "--format", "md", "--summarize")
	require.Equal(t, cli.ExitOK, code)
	testutil.Golden(t, filepath.Join(goldenDir, "digest", "terraform-summary.md"), []byte(md))

	_, plain, _ := s.run(t, "digest", "terraform", terraformRange, "--format", "plain", "--summarize")
	assert.True(t, strings.HasPrefix(plain, "Upgrade risk summary\n"+
		"LLM-generated, verify against cited sources. Model: openai-compatible test-model.\n"), plain)
	assert.Regexp(t, `\n  - Search CI scripts for the removed command\. \[e-[0-9a-f]{8}\]\n`, plain)
}

// TestSummarize_warnings checks that every way the summary can fail is a warning that leaves
// the report and the exit code as they are.
func TestSummarize_warnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		config    string
		setup     func(*fakeLLM)
		args      []string
		wantWarn  string
		wantCalls int
	}{
		{
			name:     "provider error",
			config:   openAIConfig,
			setup:    func(f *fakeLLM) { f.status = http.StatusServiceUnavailable },
			wantWarn: "WRN --summarize: call http://llm.test/v1/chat/completions: 503 Service Unavailable: model overloaded; no summary",
			// httpx doesn't resend a POST after a 503, so the model is never billed twice.
			wantCalls: 1,
		},
		{
			name:      "timeout",
			config:    openAIConfig + "  timeout: 50ms\n",
			setup:     func(f *fakeLLM) { f.block = true },
			wantWarn:  "WRN --summarize: the model didn't answer within llm.timeout (50ms); no summary",
			wantCalls: 1,
		},
		{
			name:     "offline",
			config:   openAIConfig,
			args:     []string{"--offline"},
			wantWarn: "WRN --summarize: skipped with --offline",
		},
		{
			name:     "missing key",
			config:   strings.Replace(openAIConfig, "LLM_KEY", "UNSET_KEY", 1),
			wantWarn: "WRN --summarize: llm not configured: UNSET_KEY is empty; no summary",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSummaryRun(t, tt.config)
			args := []string{"digest", "terraform", terraformRange, "--format", "json", "--fail-on", "breaking"}
			if tt.name == "offline" {
				// Fill the cache, so the offline run has the digest.
				code, _, _ := s.run(t, args...)
				require.Equal(t, cli.ExitFindings, code)
			}
			if tt.setup != nil {
				tt.setup(s.llm)
			}
			code, out, stderr := s.run(t, append(append(args, "--summarize"), tt.args...)...)
			assert.Equal(t, cli.ExitFindings, code)
			assert.Contains(t, stderr, tt.wantWarn)
			assert.Nil(t, decodeDigest(t, out).Summary)
			assert.Equal(t, tt.wantCalls, s.llm.calls)
		})
	}
}

// TestSummarize_noProviderNoNetwork checks that --summarize without llm.provider sends exactly the
// requests the digest sends without it. Each run gets a new fixture because a cassette answers
// each request once.
func TestSummarize_noProviderNoNetwork(t *testing.T) {
	t.Parallel()
	for _, cfg := range []string{"", "llm:\n  model: test-model\n  api_key_env: LLM_KEY\n"} {
		args := []string{"digest", "terraform", terraformRange, "--format", "json"}
		runOnce := func(extra ...string) (*summaryRun, []string, int, string, string) {
			s := newSummaryRun(t, cfg)
			counter := &countingTransport{next: s.f.d.HTTPTransport}
			s.f.d.HTTPTransport = counter
			code, out, stderr := s.run(t, append(args, extra...)...)
			return s, counter.take(), code, out, stderr
		}
		_, without, _, plain, _ := runOnce()
		s, with, code, out, stderr := runOnce("--summarize")
		assert.Equal(t, cli.ExitOK, code)
		assert.Equal(t, without, with, "the same requests as without --summarize")
		assert.NotContains(t, strings.Join(with, "\n"), llmHost)
		assert.Equal(t, plain, out)
		assert.Contains(t, stderr, "WRN --summarize: llm not configured: llm.provider is empty; no summary")
		assert.Equal(t, 0, s.llm.calls)
	}
}

// countingTransport records "METHOD host/path" for every request.
type countingTransport struct {
	mu   sync.Mutex
	reqs []string
	next http.RoundTripper
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, fmt.Sprintf("%s %s%s", req.Method, req.URL.Host, req.URL.Path))
	c.mu.Unlock()
	return c.next.RoundTrip(req) //nolint:wrapcheck // the transport's error is the result
}

// take returns the requests since the last call, sorted, since they run concurrently.
func (c *countingTransport) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.reqs
	c.reqs = nil
	slices.Sort(out)
	return out
}
