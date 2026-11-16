package summarize_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/summarize"
)

const (
	messagesAnswer = `{"content":[{"type":"text","text":"RISKS"}],"stop_reason":"end_turn"}`
	chatAnswer     = `{"choices":[{"message":{"role":"assistant","content":"RISKS"},"finish_reason":"stop"}]}`
)

func mantleEnv(srv *httptest.Server) summarize.Env {
	return summarize.Env{
		Transport: srv.Client().Transport,
		Logger:    slog.New(slog.DiscardHandler),
		UserAgent: "relgap-test",
	}
}

func TestMantle(t *testing.T) {
	t.Parallel()
	creds := credentials.NewStaticCredentialsProvider("AKIDTEST", "secret", "session-token")
	now := func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }
	tests := []struct {
		name, model, api, prefix, key, answer string
		wantPath                              string
		check                                 func(t *testing.T, h http.Header)
	}{
		{
			name: "messages signed", model: "anthropic.claude-sonnet-5", prefix: "/anthropic", answer: messagesAnswer,
			wantPath: "POST /anthropic/v1/messages",
			check: func(t *testing.T, h http.Header) {
				t.Helper()
				assert.Equal(t, "2023-06-01", h.Get("Anthropic-Version"))
				assert.Empty(t, h.Get("X-Api-Key"))
			},
		},
		{
			name: "chat signed", model: "openai.gpt-oss-120b", prefix: "/v1", answer: chatAnswer,
			wantPath: "POST /v1/chat/completions",
		},
		{
			name:     "messages with key",
			model:    "anthropic.claude-sonnet-5",
			prefix:   "/anthropic",
			key:      "bk",
			answer:   messagesAnswer,
			wantPath: "POST /anthropic/v1/messages",
			check: func(t *testing.T, h http.Header) {
				t.Helper()
				assert.Equal(t, "bk", h.Get("X-Api-Key"))
				assert.Empty(t, h.Get("Authorization"))
			},
		},
		{
			name:     "chat with key",
			model:    "anthropic.claude-sonnet-5",
			api:      summarize.APIChatCompletions,
			prefix:   "/openai/v1",
			key:      "bk",
			answer:   chatAnswer,
			wantPath: "POST /openai/v1/chat/completions",
			check: func(t *testing.T, h http.Header) {
				t.Helper()
				assert.Equal(t, "Bearer bk", h.Get("Authorization"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, c := fakeEndpoint(t, http.StatusOK, tt.answer)
			p, err := summarize.NewMantle(summarize.MantleOptions{
				Model: tt.model, API: tt.api, BaseURL: srv.URL + tt.prefix, Region: "us-east-1",
				APIKey: tt.key, Credentials: creds, Now: now,
			}, mantleEnv(srv))
			require.NoError(t, err)
			got, err := p.Complete(t.Context(), testPrompt())
			require.NoError(t, err)
			assert.Equal(t, "RISKS", got)
			assert.Equal(t, tt.wantPath, c.method+" "+c.path)
			assert.Equal(t, tt.model, c.body["model"])
			if tt.key == "" {
				auth := c.header.Get("Authorization")
				assert.Contains(
					t,
					auth,
					"AWS4-HMAC-SHA256 Credential=AKIDTEST/20261116/us-east-1/bedrock-mantle/aws4_request",
				)
				assert.Contains(t, auth, "content-type;host;x-amz-date;x-amz-security-token, Signature=")
				assert.Equal(t, "20261116T120000Z", c.header.Get("X-Amz-Date"))
				assert.Equal(t, "session-token", c.header.Get("X-Amz-Security-Token"))
			}
			if tt.check != nil {
				tt.check(t, c.header)
			}
		})
	}
}

func TestMantle_error(t *testing.T) {
	t.Parallel()
	srv, _ := fakeEndpoint(
		t,
		http.StatusForbidden,
		`{"error":{"message":"not authorized to perform bedrock-mantle:CreateInference"}}`,
	)
	p, err := summarize.NewMantle(summarize.MantleOptions{
		Model: "openai.gpt-oss-120b", BaseURL: srv.URL + "/v1", Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIDTEST", "secret", ""),
	}, mantleEnv(srv))
	require.NoError(t, err)
	_, err = p.Complete(t.Context(), testPrompt())
	require.ErrorContains(t, err, "bedrock-mantle chat-completions: call ")
	require.ErrorContains(t, err, "403 Forbidden: not authorized to perform bedrock-mantle:CreateInference")
}

func TestNewMantle_errors(t *testing.T) {
	t.Parallel()
	creds := credentials.NewStaticCredentialsProvider("AKIDTEST", "secret", "")
	tests := []struct {
		name string
		o    summarize.MantleOptions
		want string
	}{
		{
			"unknown api",
			summarize.MantleOptions{Model: "m", API: "responses", Region: "us-east-1", Credentials: creds},
			`unknown llm.api "responses"`,
		},
		{"no region", summarize.MantleOptions{Model: "m", Credentials: creds}, "no region"},
		{
			"signing needs region",
			summarize.MantleOptions{Model: "m", BaseURL: "https://x", Credentials: creds},
			"no region",
		},
		{"no credentials", summarize.MantleOptions{Model: "m", Region: "us-east-1"}, "no AWS credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := summarize.NewMantle(tt.o, summarize.Env{Logger: slog.New(slog.DiscardHandler)})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestMantleAPI(t *testing.T) {
	t.Parallel()
	tests := []struct{ api, model, want string }{
		{"", "anthropic.claude-sonnet-5", summarize.APIMessages},
		{"", "openai.gpt-oss-120b", summarize.APIChatCompletions},
		{summarize.APIChatCompletions, "anthropic.claude-sonnet-5", summarize.APIChatCompletions},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, summarize.MantleAPI(tt.api, tt.model), tt.model)
	}
	assert.Equal(
		t,
		"https://bedrock-mantle.eu-west-1.api.aws/anthropic",
		summarize.MantleBaseURL("eu-west-1", summarize.APIMessages),
	)
	assert.Equal(
		t,
		"https://bedrock-mantle.eu-west-1.api.aws/v1",
		summarize.MantleBaseURL("eu-west-1", summarize.APIChatCompletions),
	)
}

func TestSDKLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := summarize.SDKLogger(
		t.Context(),
		slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	)
	l.Logf(logging.Warn, "retry %d", 2)
	l.Logf(logging.Debug, "request")
	assert.Contains(t, buf.String(), `level=WARN msg="aws sdk: retry 2"`)
	assert.Contains(t, buf.String(), `level=DEBUG msg="aws sdk: request"`)
}
