package enrich_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
)

// recordingOSV records every request and pages the first querybatch answer once, so the second
// body carries a page token.
type recordingOSV struct {
	mu     sync.Mutex
	reqs   []*http.Request
	bodies [][]byte
	paged  bool
}

func (r *recordingOSV) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b []byte
	if req.Body != nil {
		var err error
		if b, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
	}
	r.reqs, r.bodies = append(r.reqs, req), append(r.bodies, b)
	body := `{"id":"GHSA-aaaa-bbbb-cccc"}`
	if req.URL.Path == "/v1/querybatch" {
		var in struct {
			Queries []json.RawMessage `json:"queries"`
		}
		if err := json.Unmarshal(b, &in); err != nil {
			return nil, err
		}
		results := make([]string, len(in.Queries))
		for i := range results {
			results[i] = `{"vulns":[]}`
		}
		if !r.paged {
			r.paged, results[0] = true, `{"vulns":[],"next_page_token":"t1"}`
		}
		body = `{"results":[` + strings.Join(results, ",") + `]}`
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}, nil
}

// jsonPaths adds the path of every leaf value of v to out, with [] for an array element.
func jsonPaths(prefix string, v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			jsonPaths(p, c, out)
		}
	case []any:
		for _, c := range x {
			jsonPaths(prefix+"[]", c, out)
		}
	default:
		out[prefix] = true
	}
}

// TestSentToOSV checks the list the network page shows against the requests the OSV client
// sends: every querybatch body field is listed and nothing else, and a record request carries
// only the ID in its path.
func TestSentToOSV(t *testing.T) {
	t.Parallel()
	rec := &recordingOSV{}
	c := enrich.NewOSVClient(&http.Client{Transport: rec}, "https://osv.test")
	_, err := c.QueryBatch(t.Context(), []enrich.Query{
		{Package: enrich.Package{Ecosystem: "npm", Name: "lodash"}, Version: "4.17.20"},
		{Package: enrich.Package{Ecosystem: enrich.EcosystemActions, Name: "actions/download-artifact"}},
	})
	require.NoError(t, err)
	_, err = c.Known(t.Context(), []enrich.Package{{Ecosystem: enrich.EcosystemGo, Name: "github.com/x/y"}})
	require.NoError(t, err)
	_, err = c.Vuln(t.Context(), "GHSA-aaaa-bbbb-cccc", "")
	require.NoError(t, err)

	want := map[string][]string{}
	for _, f := range enrich.SentToOSV() {
		assert.NotEmpty(t, f.Description, f.Name)
		want[f.Request] = append(want[f.Request], f.Name)
	}
	got := map[string]bool{}
	var vulnGets int
	for i, req := range rec.reqs {
		assert.Empty(t, req.URL.RawQuery, "query string of %s", req.URL.Path)
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v1/querybatch":
			var body any
			require.NoError(t, json.Unmarshal(rec.bodies[i], &body))
			jsonPaths("", body, got)
		case req.Method == http.MethodGet && req.URL.Path == "/v1/vulns/GHSA-aaaa-bbbb-cccc":
			assert.Empty(t, rec.bodies[i], "record request body")
			vulnGets++
		default:
			t.Errorf("unlisted request %s %s", req.Method, req.URL.Path)
		}
	}
	gotNames := make([]string, 0, len(got))
	for p := range got {
		gotNames = append(gotNames, p)
	}
	assert.ElementsMatch(t, want[enrich.SentQueryBatch], gotNames, "querybatch fields")
	assert.Equal(t, []string{"id"}, want[enrich.SentVuln])
	assert.Equal(t, 1, vulnGets)
	assert.Len(t, want, 2, "requests")
}
