package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
)

// Errors wrapped by [Get] for answers that aren't a success.
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
)

// maxGetBody caps a response body that [Get] reads, so a wrong URL can't fill memory.
const maxGetBody = 64 << 20

// StatusError is an answer [Get] doesn't treat as a success.
type StatusError struct {
	URL    string
	Status int
	Header http.Header
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status)
}

// Is makes 404 and 410 match [ErrNotFound], and 401 and 403 match [ErrUnauthorized].
func (e *StatusError) Is(target error) bool {
	switch e.Status {
	case http.StatusNotFound, http.StatusGone:
		return target == ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return target == ErrUnauthorized
	}
	return false
}

// Get sends a GET for rawURL with header through hc and returns the body and headers of a 2xx
// answer. Any other answer is a [*StatusError].
func Get(ctx context.Context, hc *http.Client, rawURL string, header http.Header) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	maps.Copy(req.Header, header)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", redacted(rawURL), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxGetBody))
		return nil, resp.Header, &StatusError{URL: redacted(rawURL), Status: resp.StatusCode, Header: resp.Header}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGetBody))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", redacted(rawURL), err)
	}
	return body, resp.Header, nil
}

func redacted(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Redacted()
}

// SameOrigin reports whether next has the scheme and host of base, so a client follows a
// pagination link only where its token belongs.
func SameOrigin(base, next string) bool {
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	n, err := url.Parse(next)
	if err != nil {
		return false
	}
	return b.Scheme == n.Scheme && b.Host == n.Host
}
