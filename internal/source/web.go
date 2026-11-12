package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/sassdavid/relgap/internal/registry"
)

// maxPageBytes caps a fetched page. The biggest page a curated entry reads is about 0.6 MB.
const maxPageBytes = 8 << 20

// maxRedirects is how many redirects [Web] follows, each to an allowed host.
const maxRedirects = 5

// WebFetcher returns the body of an allowed web page.
type WebFetcher interface {
	Get(ctx context.Context, rawURL, accept string) ([]byte, error)
}

// Web fetches pages from the hosts [registry.WebHosts] allows, redirects included.
type Web struct {
	hc *http.Client
}

// NewWeb returns a fetcher that sends requests through hc's transport, which brings retries,
// rate limits and the cache.
func NewWeb(hc *http.Client) *Web {
	return &Web{hc: &http.Client{
		Transport: hc.Transport,
		Timeout:   hc.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return registry.CheckURL(req.URL.String()) //nolint:wrapcheck // CheckURL names the URL
		},
	}}
}

// Get returns the body of rawURL. It wraps [ErrNotFound] for 404 and 410, and fails for hosts
// outside the allow-list and bodies over 8 MB.
func (w *Web) Get(ctx context.Context, rawURL, accept string) ([]byte, error) {
	if err := registry.CheckURL(rawURL); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := w.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, fmt.Errorf("fetch %s: %w (HTTP %d)", rawURL, ErrNotFound, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	if len(b) > maxPageBytes {
		return nil, fmt.Errorf("fetch %s: %w", rawURL, errTooLarge)
	}
	return b, nil
}

var (
	errTooLarge = errors.New("page is larger than 8 MB")
	errNoWeb    = errors.New("this run has no web client")
)
