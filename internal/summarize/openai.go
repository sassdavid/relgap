package summarize

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// OpenAI calls the chat completions endpoint of an OpenAI-compatible API, such as OpenAI,
// LiteLLM, Ollama or vLLM.
type OpenAI struct {
	hc      *http.Client
	baseURL string
	apiKey  string // empty sends no Authorization header
	model   string
}

// NewOpenAI returns a provider that posts to baseURL + "/chat/completions".
func NewOpenAI(hc *http.Client, baseURL, apiKey, model string) *OpenAI {
	return &OpenAI{hc: hc, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model}
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Complete sends p as a system and a user message.
func (o *OpenAI) Complete(ctx context.Context, p Prompt) (string, error) {
	in := struct {
		Model    string          `json:"model"`
		Messages []openAIMessage `json:"messages"`
	}{
		Model:    o.model,
		Messages: []openAIMessage{{Role: "system", Content: p.System}, {Role: "user", Content: p.User}},
	}
	var out struct {
		Choices []struct {
			Message      openAIMessage `json:"message"`
			FinishReason string        `json:"finish_reason"`
		} `json:"choices"`
	}
	h := http.Header{}
	if o.apiKey != "" {
		h.Set("Authorization", "Bearer "+o.apiKey)
	}
	if err := postJSON(ctx, o.hc, o.baseURL+"/chat/completions", h, in, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("the model returned no choices")
	}
	if c := out.Choices[0]; c.FinishReason == "content_filter" {
		return "", errors.New("the model's answer was filtered (finish_reason content_filter)")
	}
	return out.Choices[0].Message.Content, nil
}
