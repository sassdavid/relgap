package enrich_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
)

// fakeOSV answers querybatch with three records of one package and serves them by ID: GO-1 has
// no rating and one GHSA alias OSV doesn't know, GO-2 has no rating and a rated GHSA alias, and
// GO-W is withdrawn, as in a cached querybatch answer from before the withdrawal. It counts the
// GETs of each record.
type fakeOSV struct {
	mu       sync.Mutex
	modified string // GO-1's modified time
	gets     map[string]int
}

const pkgRange = `"affected":[{"package":{"ecosystem":"Go","name":"example.com/tool"},` +
	`"ranges":[{"type":"SEMVER","events":[{"introduced":"0"}]}]}]`

func (f *fakeOSV) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := http.StatusOK
	var body string
	switch id := strings.TrimPrefix(req.URL.Path, "/v1/vulns/"); {
	case req.URL.Path == "/v1/querybatch":
		body = fmt.Sprintf(`{"results":[{"vulns":[{"id":"GO-1","modified":%q},`+
			`{"id":"GO-2","modified":"2026-01-02T00:00:00Z"},{"id":"GO-W","modified":"2026-01-03T00:00:00Z"}]}]}`,
			f.modified)
	case id == "GO-1":
		f.gets[id]++
		body = `{"id":"GO-1","aliases":["CVE-2026-1","GHSA-none-0000-0000"],` + pkgRange + `}`
	case id == "GO-2":
		f.gets[id]++
		body = `{"id":"GO-2","aliases":["GHSA-rate-0000-0000"],` + pkgRange + `}`
	case id == "GO-W":
		f.gets[id]++
		body = `{"id":"GO-W","withdrawn":"2026-01-04T00:00:00Z",` + pkgRange + `}`
	case id == "GHSA-rate-0000-0000":
		f.gets[id]++
		body = `{"id":"GHSA-rate-0000-0000","database_specific":{"severity":"HIGH"},` +
			`"severity":[{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"}]}`
	default:
		f.gets[id]++
		status, body = http.StatusNotFound, `{"code":5,"message":"Bug not found."}`
	}
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}, nil
}

func (f *fakeOSV) setModified(m string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modified = m
}

func (f *fakeOSV) count(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[id]
}

// TestSpec_vulnerability_audit_4_records: an unrated record takes the score of a GHSA alias and
// names it; one whose alias OSV doesn't know is unknown and sorts last; a withdrawn record is
// dropped; a record is fetched again only when its modified time changes or after 7 days.
func TestSpec_vulnerability_audit_4_records(t *testing.T) {
	t.Parallel()
	osv := &fakeOSV{modified: "2026-01-01T00:00:00Z", gets: map[string]int{}}
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	cache := httpx.NewCache(httpx.CacheOptions{
		Dir:    t.TempDir(),
		Clock:  func() time.Time { return now },
		Logger: slog.New(slog.DiscardHandler),
		TTLOverride: map[string]time.Duration{
			httpx.OSVHost: 24 * time.Hour,
		},
	})
	t.Cleanup(func() { _ = cache.Close() })
	tool := spec("tool", nil, registry.OSVPackage{Ecosystem: "Go", Name: "example.com/tool"})
	enrichOnce := func() []enrich.Vuln {
		t.Helper()
		e := enrich.NewEnricher(slog.New(slog.DiscardHandler), nil,
			enrich.NewOSVClient(&http.Client{Transport: cache.Wrap(osv)}, ""), 2)
		res, problems, err := e.Enrich(t.Context(),
			[]enrich.Request{{Key: "k", Tool: tool, Current: semver(t, "1.0.0")}})
		require.NoError(t, err)
		require.Empty(t, problems)
		return res["k"].Vulns
	}

	vulns := enrichOnce()
	require.Len(t, vulns, 2, "GO-W is withdrawn")
	assert.Equal(t, "GO-2", vulns[0].ID)
	assert.Equal(t, &enrich.Score{
		Score: 7.5, Rating: "high", Vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N", From: "GHSA-rate-0000-0000",
	}, vulns[0].Severity)
	assert.Equal(t, "GO-1", vulns[1].ID)
	assert.Equal(t, &enrich.Score{Rating: enrich.RatingUnknown}, vulns[1].Severity)
	for _, id := range []string{"GO-1", "GO-2", "GO-W", "GHSA-rate-0000-0000", "GHSA-none-0000-0000"} {
		assert.Equal(t, 1, osv.count(id), "%s is fetched once", id)
	}

	now = now.Add(2 * 24 * time.Hour)
	enrichOnce()
	assert.Equal(t, 1, osv.count("GO-1"), "an unchanged modified time keeps the record past the 1-day TTL")
	assert.Equal(t, 1, osv.count("GO-W"), "a withdrawn record stays cached too")

	osv.setModified("2026-11-24T00:00:00Z")
	now = now.Add(25 * time.Hour) // past the querybatch answer's TTL
	enrichOnce()
	assert.Equal(t, 2, osv.count("GO-1"), "a changed modified time fetches the record again")
	assert.Equal(t, 1, osv.count("GO-2"))

	now = now.Add(4 * 24 * time.Hour)
	enrichOnce()
	assert.Equal(t, 2, osv.count("GO-2"), "past 7 days an unchanged record is fetched again")
	assert.Equal(t, 2, osv.count("GO-1"), "GO-1 was fetched again 4 days ago")
}
