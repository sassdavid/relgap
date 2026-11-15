package summarize

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/httpx"
)

// Provider names, the values of llm.provider.
const (
	ProviderOpenAI    = "openai-compatible"
	ProviderAnthropic = "anthropic"
	ProviderBedrock   = "bedrock"
)

// MaxTimeout caps llm.timeout: it is the longest single attempt httpx allows.
const MaxTimeout = 2 * time.Minute

// ErrNotConfigured means the llm config can't make a call. The caller warns and prints the
// report without a summary.
var ErrNotConfigured = errors.New("llm not configured")

// Env is what [New] takes from the outside world.
type Env struct {
	Lookup    func(string) (string, bool)
	Transport http.RoundTripper // nil means the network
	Logger    *slog.Logger
	UserAgent string
}

// New returns the provider that c names. It makes no network call, so with no provider
// configured --summarize sends nothing.
func New(ctx context.Context, c config.LLM, e Env) (Provider, error) {
	if c.Provider == "" {
		return nil, fmt.Errorf("%w: llm.provider is empty", ErrNotConfigured)
	}
	if c.Model == "" {
		return nil, fmt.Errorf("%w: llm.model is empty", ErrNotConfigured)
	}
	switch c.Provider {
	case ProviderOpenAI:
		if c.BaseURL == "" {
			return nil, fmt.Errorf(
				"%w: llm.base_url is empty; the openai-compatible provider needs it",
				ErrNotConfigured,
			)
		}
		key, err := apiKey(c.APIKeyEnv, "", e.Lookup)
		if err != nil {
			return nil, err
		}
		return NewOpenAI(newHTTPClient(e), c.BaseURL, key, c.Model), nil
	case ProviderAnthropic:
		key, err := apiKey(c.APIKeyEnv, AnthropicKeyEnv, e.Lookup)
		if err != nil {
			return nil, err
		}
		base := c.BaseURL
		if base == "" {
			base = AnthropicBaseURL
		}
		return NewAnthropic(newHTTPClient(e), base, key, c.Model), nil
	case ProviderBedrock:
		cfg, err := LoadAWSConfig(ctx, c.AWSProfile, e.Transport)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNotConfigured, err)
		}
		return NewBedrock(cfg, c.Model), nil
	default:
		return nil, fmt.Errorf("%w: unknown llm.provider %q", ErrNotConfigured, c.Provider)
	}
}

// Timeout returns llm.timeout, capped at [MaxTimeout].
func Timeout(c config.LLM) (time.Duration, error) {
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%w: llm.timeout %q is not a positive Go duration", ErrNotConfigured, c.Timeout)
	}
	return min(d, MaxTimeout), nil
}

func keyEnv(configured, def string) string {
	if configured != "" {
		return configured
	}
	return def
}

// apiKey reads the key from the env var c names, or from def when c is empty. An empty def
// means no key is needed.
func apiKey(configured, def string, lookup func(string) (string, bool)) (string, error) {
	name := keyEnv(configured, def)
	if name == "" {
		return "", nil
	}
	key, ok := lookup(name)
	if !ok || key == "" {
		return "", fmt.Errorf("%w: %s is empty", ErrNotConfigured, name)
	}
	return key, nil
}

// newHTTPClient returns an httpx client without the cache. A POST is sent again only after a
// 429 or a Retry-After, so a model call is never billed twice for one answer.
func newHTTPClient(e Env) *http.Client {
	return httpx.New(httpx.Options{Logger: e.Logger, Transport: e.Transport, UserAgent: e.UserAgent})
}
