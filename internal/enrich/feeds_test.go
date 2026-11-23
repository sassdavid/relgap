package enrich_test

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/vex"
)

// cannedTransport answers each request with answer and records its URL.
type cannedTransport struct {
	mu     sync.Mutex
	urls   []string
	answer func(req *http.Request) (int, string)
}

func (c *cannedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.urls = append(c.urls, req.URL.String())
	c.mu.Unlock()
	code, body := c.answer(req)
	return &http.Response{
		StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: req,
	}, nil
}

const kevBody = `{"catalogVersion":"2026.11.23","vulnerabilities":[
{"cveID":"CVE-2025-31125","dateAdded":"2026-01-22","dueDate":"2026-02-12","knownRansomwareCampaignUse":"Unknown"},
{"cveID":"CVE-2025-55182","dateAdded":"2025-12-05","dueDate":"2025-12-26","knownRansomwareCampaignUse":"Known"}]}`

func TestKEVClient_catalog(t *testing.T) {
	t.Parallel()
	tr := &cannedTransport{answer: func(*http.Request) (int, string) { return http.StatusOK, kevBody }}
	c := enrich.NewKEVClient(&http.Client{Transport: tr}, "")
	for range 2 {
		got, err := c.Catalog(t.Context())
		require.NoError(t, err)
		assert.Equal(t, enrich.KEV{
			CVE: "CVE-2025-55182", DateAdded: "2025-12-05", DueDate: "2025-12-26", Ransomware: true,
		}, got["CVE-2025-55182"])
		assert.False(t, got["CVE-2025-31125"].Ransomware)
	}
	assert.Equal(t, []string{enrich.KEVURL}, tr.urls, "one request per client")

	down := &cannedTransport{answer: func(*http.Request) (int, string) { return http.StatusServiceUnavailable, "" }}
	_, err := enrich.NewKEVClient(&http.Client{Transport: down}, "").Catalog(t.Context())
	require.EqualError(t, err, "CISA KEV catalog: HTTP 503")
}

// epssAnswer scores every CVE it is asked about with its number divided by 10^6.
func epssAnswer(req *http.Request) (int, string) {
	ids := strings.Split(req.URL.Query().Get("cve"), ",")
	rows := make([]string, 0, len(ids))
	for _, id := range ids {
		n, _ := strconv.Atoi(id[strings.LastIndex(id, "-")+1:])
		score := strconv.FormatFloat(float64(n)/1e6, 'f', 9, 64)
		rows = append(rows, `{"cve":"`+id+`","epss":"`+score+`","percentile":"0.500000000","date":"2026-11-23"}`)
	}
	return http.StatusOK, `{"status":"OK","data":[` + strings.Join(rows, ",") + `]}`
}

func TestEPSSClient_scores(t *testing.T) {
	t.Parallel()
	tr := &cannedTransport{answer: epssAnswer}
	c := enrich.NewEPSSClient(&http.Client{Transport: tr}, "")
	var ids []string
	for i := range 300 {
		ids = append(ids, "CVE-2025-"+strconv.Itoa(10000+i))
	}
	got, err := c.Scores(t.Context(), append(ids, "cve-2025-10000"))
	require.NoError(t, err)
	assert.Len(t, got, 300)
	assert.Equal(t, enrich.EPSS{CVE: "CVE-2025-10299", Score: 0.010299, Percentile: 0.5, Date: "2026-11-23"},
		got["CVE-2025-10299"])

	require.Len(t, tr.urls, 3, "300 IDs of 14 characters need three requests of at most 2,000 characters")
	asked := 0
	for _, u := range tr.urls {
		q, _ := strings.CutPrefix(u, enrich.EPSSURL+"?cve=")
		cves, limit, ok := strings.Cut(q, "&limit=")
		require.True(t, ok, u)
		assert.LessOrEqual(t, len(cves), 2000)
		n := strings.Count(cves, ",") + 1
		assert.Equal(t, strconv.Itoa(n), limit, "limit asks for every row, not the API's 100")
		asked += n
	}
	assert.Equal(t, 300, asked, "each ID once, sorted into the chunks")

	_, err = c.Scores(t.Context(), ids[:5])
	require.NoError(t, err)
	assert.Len(t, tr.urls, 3, "IDs already asked about send no request")

	down := &cannedTransport{answer: func(*http.Request) (int, string) { return http.StatusBadGateway, "" }}
	_, err = enrich.NewEPSSClient(&http.Client{Transport: down}, "").Scores(t.Context(), ids[:1])
	require.EqualError(t, err, "EPSS: HTTP 502")
}

// TestBinaryVulns_suppression checks that outdated --binaries applies the vulns.ignore entries
// to an executable's advisories, and that a match counts as a use of the entry.
func TestBinaryVulns_suppression(t *testing.T) {
	t.Parallel()
	_, osv := clients(t, "outdated-eol.yaml")
	rules, err := vex.New([]vex.Ignore{
		{ID: "CVE-2025-1767", Reason: "no cluster mounts a gitRepo volume", Tools: []string{"kubectl"}},
	}, nil, time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	en := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil, osv, 1)
	en.SetSuppressor(rules)
	b := &enrich.Binary{
		Tool: "kubectl", Path: "/usr/local/bin/kubectl", GoVersion: "1.24.0",
		Module: "k8s.io/kubernetes", ModuleVersion: "1.34.13",
		Queries: []enrich.Query{{
			Package: enrich.Package{Ecosystem: enrich.EcosystemGo, Name: "k8s.io/kubernetes"},
			Version: "1.34.13",
		}},
		Vulns: []enrich.Vuln{},
	}
	problems, err := en.BinaryVulns(t.Context(), []*enrich.Binary{b})
	require.NoError(t, err)
	assert.Empty(t, problems)
	got := map[string]*enrich.Suppression{}
	for _, v := range b.Vulns {
		got[v.ID] = v.Suppressed
	}
	require.Contains(t, got, "GO-2025-3521")
	require.NotNil(t, got["GO-2025-3521"], "suppressed through its CVE alias")
	assert.Equal(t, "no cluster mounts a gitRepo volume", got["GO-2025-3521"].Reason)
	assert.Nil(t, got["GO-2025-3547"])
	assert.Empty(t, rules.Unused())
}
