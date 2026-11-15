package summarize

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const (
	// AnthropicBaseURL is the Anthropic API when llm.base_url is empty.
	AnthropicBaseURL = "https://api.anthropic.com"
	// AnthropicKeyEnv holds the API key when llm.api_key_env is empty.
	AnthropicKeyEnv = "ANTHROPIC_API_KEY"

	// anthropicVersion rejects temperature, top_p and top_k, so relgap sends none of them.
	anthropicVersion = "2026-10-21"
	// anthropicMaxTokens leaves room for thinking, which some models always do and which
	// counts against max_tokens.
	anthropicMaxTokens = 16000
)

// Anthropic calls the Anthropic Messages API.
type Anthropic struct {
	hc      *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewAnthropic returns a provider that posts to baseURL + "/v1/messages".
func NewAnthropic(hc *http.Client, baseURL, apiKey, model string) *Anthropic {
	return &Anthropic{hc: hc, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model}
}

// Complete sends p and joins the text blocks of the answer.
func (a *Anthropic) Complete(ctx context.Context, p Prompt) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	in := struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system"`
		Messages  []message `json:"messages"`
	}{
		Model:     a.model,
		MaxTokens: anthropicMaxTokens,
		System:    p.System,
		Messages:  []message{{Role: "user", Content: p.User}},
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	h := http.Header{}
	h.Set("X-Api-Key", a.apiKey)
	h.Set("Anthropic-Version", anthropicVersion)
	if err := postJSON(ctx, a.hc, a.baseURL+"/v1/messages", h, in, &out); err != nil {
		return "", err
	}
	if out.StopReason == "refusal" {
		return "", errors.New("the model declined to answer (stop_reason refusal)")
	}
	var b strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), nil
}
