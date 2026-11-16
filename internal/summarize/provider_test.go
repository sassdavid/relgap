package summarize_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/summarize"
)

func testPrompt() summarize.Prompt { return summarize.Prompt{System: "sys", User: "user"} }

// captured is the last request a fake endpoint got.
type captured struct {
	method, path string
	header       http.Header
	body         map[string]any
}

func fakeEndpoint(t *testing.T, status int, answer string) (*httptest.Server, *captured) {
	t.Helper()
	c := &captured{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.method, c.path, c.header = r.Method, r.URL.Path, r.Header.Clone()
		_ = json.Unmarshal(b, &c.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func TestOpenAI(t *testing.T) {
	t.Parallel()
	srv, c := fakeEndpoint(t, http.StatusOK,
		`{"choices":[{"message":{"role":"assistant","content":"RISKS"},"finish_reason":"stop"}]}`)
	got, err := summarize.NewOpenAI(srv.Client(), srv.URL+"/v1/", "sk-test", "gpt-test").
		Complete(t.Context(), testPrompt())
	require.NoError(t, err)
	assert.Equal(t, "RISKS", got)
	assert.Equal(t, "POST /v1/chat/completions", c.method+" "+c.path)
	assert.Equal(t, "Bearer sk-test", c.header.Get("Authorization"))
	assert.Equal(t, map[string]any{
		"model": "gpt-test",
		"messages": []any{
			map[string]any{"role": "system", "content": "sys"},
			map[string]any{"role": "user", "content": "user"},
		},
	}, c.body)
}

func TestOpenAI_noKeyNoHeader(t *testing.T) {
	t.Parallel()
	srv, c := fakeEndpoint(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	_, err := summarize.NewOpenAI(srv.Client(), srv.URL, "", "llama").Complete(t.Context(), testPrompt())
	require.NoError(t, err)
	assert.Empty(t, c.header.Get("Authorization"))
}

func TestOpenAI_errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		answer string
		want   string
	}{
		{
			http.StatusUnauthorized,
			`{"error":{"message":"Incorrect API key provided"}}`,
			"401 Unauthorized: Incorrect API key provided",
		},
		{http.StatusOK, `{"choices":[]}`, "the model returned no choices"},
		{http.StatusOK, `{"choices":[{"message":{"content":""},"finish_reason":"content_filter"}]}`, "filtered"},
	}
	for _, tt := range tests {
		srv, _ := fakeEndpoint(t, tt.status, tt.answer)
		_, err := summarize.NewOpenAI(srv.Client(), srv.URL, "sk-secret", "m").Complete(t.Context(), testPrompt())
		require.ErrorContains(t, err, tt.want)
		assert.NotContains(t, err.Error(), "sk-secret")
	}
}

func TestAnthropic(t *testing.T) {
	t.Parallel()
	srv, c := fakeEndpoint(
		t,
		http.StatusOK,
		`{"content":[{"type":"thinking","thinking":""},{"type":"text","text":"RISKS\n"},{"type":"text","text":"- none"}],"stop_reason":"end_turn"}`,
	)
	got, err := summarize.NewAnthropic(srv.Client(), srv.URL, "ak-test", "claude-sonnet-5-5").
		Complete(t.Context(), testPrompt())
	require.NoError(t, err)
	assert.Equal(t, "RISKS\n- none", got)
	assert.Equal(t, "POST /v1/messages", c.method+" "+c.path)
	assert.Equal(t, "ak-test", c.header.Get("X-Api-Key"))
	assert.Equal(t, "2026-10-21", c.header.Get("Anthropic-Version"))
	assert.Equal(t, map[string]any{
		"model":      "claude-sonnet-5-5",
		"max_tokens": float64(16000),
		"system":     "sys",
		"messages":   []any{map[string]any{"role": "user", "content": "user"}},
	}, c.body, "no temperature, top_p or top_k")
}

func TestAnthropic_errors(t *testing.T) {
	t.Parallel()
	srv, _ := fakeEndpoint(t, http.StatusOK, `{"content":[],"stop_reason":"refusal"}`)
	_, err := summarize.NewAnthropic(srv.Client(), srv.URL, "k", "m").Complete(t.Context(), testPrompt())
	require.ErrorContains(t, err, "declined")

	srv, _ = fakeEndpoint(
		t,
		http.StatusNotFound,
		`{"type":"error","error":{"type":"not_found_error","message":"model: nope"}}`,
	)
	_, err = summarize.NewAnthropic(srv.Client(), srv.URL, "k", "nope").Complete(t.Context(), testPrompt())
	require.ErrorContains(t, err, "404 Not Found: model: nope")
}

func bedrockConfig(srv *httptest.Server) aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIDTEST", "secret", ""),
		HTTPClient:  srv.Client(),
	}
}

// bedrockEndpoint points the client at srv and signs with SigV4 even when the developer's
// environment sets AWS_BEARER_TOKEN_BEDROCK, which the SDK reads by itself.
func bedrockEndpoint(srv *httptest.Server) func(*bedrockruntime.Options) {
	return func(o *bedrockruntime.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.AuthSchemePreference = []string{"sigv4"}
	}
}

func TestBedrock(t *testing.T) {
	t.Parallel()
	srv, c := fakeEndpoint(
		t,
		http.StatusOK,
		`{"output":{"message":{"role":"assistant","content":[{"text":"RISKS\n- none"}]}},`+
			`"stopReason":"end_turn","usage":{"inputTokens":3,"outputTokens":4,"totalTokens":7},"metrics":{"latencyMs":5}}`,
	)
	model := "global.anthropic.claude-sonnet-4-6"
	p := summarize.NewBedrock(bedrockConfig(srv), model, bedrockEndpoint(srv))
	got, err := p.Complete(t.Context(), testPrompt())
	require.NoError(t, err)
	assert.Equal(t, "RISKS\n- none", got)
	assert.Equal(t, "POST /model/"+model+"/converse", c.method+" "+c.path)
	assert.Contains(t, c.header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIDTEST/")
	assert.Equal(t, []any{map[string]any{"text": "sys"}}, c.body["system"])
	assert.Equal(
		t,
		[]any{map[string]any{"role": "user", "content": []any{map[string]any{"text": "user"}}}},
		c.body["messages"],
	)
}

func TestBedrock_blocked(t *testing.T) {
	t.Parallel()
	srv, _ := fakeEndpoint(t, http.StatusOK, `{"output":{"message":{"role":"assistant","content":[]}},`+
		`"stopReason":"guardrail_intervened","usage":{"inputTokens":1,"outputTokens":0,"totalTokens":1},"metrics":{"latencyMs":1}}`)
	p := summarize.NewBedrock(bedrockConfig(srv), "m", bedrockEndpoint(srv))
	_, err := p.Complete(t.Context(), testPrompt())
	require.ErrorContains(t, err, "blocked (stopReason guardrail_intervened)")
}

func TestBedrock_apiKey(t *testing.T) {
	t.Parallel()
	srv, c := fakeEndpoint(t, http.StatusOK, `{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},`+
		`"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2},"metrics":{"latencyMs":1}}`)
	p := summarize.NewBedrock(bedrockConfig(srv), "m", bedrockEndpoint(srv), summarize.WithBedrockAPIKey("bk"))
	_, err := p.Complete(t.Context(), testPrompt())
	require.NoError(t, err)
	assert.Equal(t, "Bearer bk", c.header.Get("Authorization"))
}

// failTransport fails the test on any request.
type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected request %s %s", r.Method, r.URL)
	return nil, http.ErrHandlerTimeout
}

func TestNew(t *testing.T) {
	t.Parallel()
	env := map[string]string{"ANTHROPIC_API_KEY": "ak", "MY_KEY": "mk", "EMPTY_KEY": ""}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	tests := []struct {
		name    string
		c       config.LLM
		want    any
		wantErr string
	}{
		{"no provider", config.LLM{}, nil, "llm not configured: llm.provider is empty"},
		{"no model", config.LLM{Provider: "anthropic"}, nil, "llm.model is empty"},
		{"anthropic default key", config.LLM{Provider: "anthropic", Model: "m"}, &summarize.Anthropic{}, ""},
		{
			"anthropic empty key",
			config.LLM{Provider: "anthropic", Model: "m", APIKeyEnv: "EMPTY_KEY"},
			nil,
			"EMPTY_KEY is empty",
		},
		{"anthropic unset key", config.LLM{Provider: "anthropic", Model: "m", APIKeyEnv: "NOPE"}, nil, "NOPE is empty"},
		{"openai needs base_url", config.LLM{Provider: "openai-compatible", Model: "m"}, nil, "llm.base_url is empty"},
		{
			"openai without key",
			config.LLM{Provider: "openai-compatible", Model: "m", BaseURL: "http://localhost:11434/v1"},
			&summarize.OpenAI{},
			"",
		},
		{
			"openai with key",
			config.LLM{Provider: "openai-compatible", Model: "m", BaseURL: "http://x", APIKeyEnv: "MY_KEY"},
			&summarize.OpenAI{},
			"",
		},
		{
			"bedrock empty key",
			config.LLM{Provider: "bedrock", Model: "m", APIKeyEnv: "EMPTY_KEY"},
			nil,
			"EMPTY_KEY is empty",
		},
		{
			"mantle empty key",
			config.LLM{Provider: "bedrock-mantle", Model: "m", APIKeyEnv: "EMPTY_KEY"},
			nil,
			"EMPTY_KEY is empty",
		},
		{
			"mantle key and base_url load no AWS config",
			config.LLM{Provider: "bedrock-mantle", Model: "m", APIKeyEnv: "MY_KEY", BaseURL: "https://x/v1"},
			&summarize.Mantle{},
			"",
		},
		{
			"mantle unknown api",
			config.LLM{
				Provider:  "bedrock-mantle",
				Model:     "m",
				API:       "responses",
				APIKeyEnv: "MY_KEY",
				BaseURL:   "https://x/v1",
			},
			nil,
			`unknown llm.api "responses"`,
		},
		{"unknown", config.LLM{Provider: "gemini", Model: "m"}, nil, `unknown llm.provider "gemini"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := summarize.New(t.Context(), tt.c, summarize.Env{
				Lookup: lookup, Transport: failTransport{t}, Logger: slog.New(slog.DiscardHandler),
			})
			if tt.wantErr != "" {
				require.ErrorIs(t, err, summarize.ErrNotConfigured)
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, p)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, tt.want, p)
		})
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	d, err := summarize.Timeout(config.LLM{Timeout: "45s"})
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, d)
	d, err = summarize.Timeout(config.LLM{Timeout: "10m"})
	require.NoError(t, err)
	assert.Equal(t, summarize.MaxTimeout, d)
	_, err = summarize.Timeout(config.LLM{Timeout: "soon"})
	require.ErrorIs(t, err, summarize.ErrNotConfigured)
}
