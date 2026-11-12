package source_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
)

// stubTransport answers by URL and records what was asked.
type stubTransport struct {
	responses map[string]*http.Response
	asked     []string
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.asked = append(s.asked, req.URL.String())
	if r, ok := s.responses[req.URL.String()]; ok {
		r.Request = req
		return r, nil
	}
	return response(http.StatusNotFound, "", nil), nil
}

func response(code int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func TestWebGet(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{responses: map[string]*http.Response{
		"https://go.dev/doc/go1.27": response(http.StatusOK, "<p>notes</p>", nil),
		"https://go.dev/moved": response(http.StatusFound, "", http.Header{
			"Location": {"https://evil.example.org/page"},
		}),
		"https://docs.python.org/big": response(http.StatusOK, strings.Repeat("x", 8<<20+1), nil),
	}}
	web := source.NewWeb(&http.Client{Transport: stub})

	b, err := web.Get(t.Context(), "https://go.dev/doc/go1.27", "text/html")
	require.NoError(t, err)
	assert.Equal(t, "<p>notes</p>", string(b))

	_, err = web.Get(t.Context(), "https://go.dev/doc/go1.99", "")
	require.ErrorIs(t, err, source.ErrNotFound)

	_, err = web.Get(t.Context(), "https://raw.githubusercontent.com/o/r/main/CHANGELOG.md", "")
	require.ErrorIs(t, err, registry.ErrHostNotAllowed)

	_, err = web.Get(t.Context(), "http://go.dev/doc/go1.27", "")
	require.ErrorIs(t, err, registry.ErrHostNotAllowed)

	_, err = web.Get(t.Context(), "https://go.dev/moved", "")
	require.ErrorIs(t, err, registry.ErrHostNotAllowed)
	assert.NotContains(t, stub.asked, "https://evil.example.org/page", "the redirect is refused before it's sent")

	_, err = web.Get(t.Context(), "https://docs.python.org/big", "")
	require.ErrorContains(t, err, "larger than 8 MB")
}
