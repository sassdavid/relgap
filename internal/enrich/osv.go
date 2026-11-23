package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/httpx"
)

// OSVURL is the OSV API base URL.
const OSVURL = "https://api.osv.dev"

// maxBatch is the most queries OSV accepts in one querybatch request.
const maxBatch = 1000

// recordTTL is how long a record fetched for a querybatch hit stays cached. Its modified time
// is part of the cache key, so a changed record is fetched again before then.
const recordTTL = 7 * 24 * time.Hour

// errNotFound is OSV's answer for an ID it has no record of.
var errNotFound = errors.New("not found")

// Package is a package in an OSV ecosystem.
type Package struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

// Query asks which vulnerabilities affect one version of a package.
type Query struct {
	Package Package `json:"package"`
	Version string  `json:"version,omitempty"` // empty asks about every version
}

// Record is an OSV vulnerability record, reduced to the fields relgap reads.
type Record struct {
	ID               string     `json:"id"`
	Aliases          []string   `json:"aliases"`
	Summary          string     `json:"summary"`
	Details          string     `json:"details"`
	Severity         []Severity `json:"severity"`
	Affected         []Affected `json:"affected"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	Withdrawn time.Time `json:"withdrawn,omitzero"` // set when the record no longer applies
}

// Hit is a record a query returned, with the time OSV last changed it.
type Hit struct {
	ID       string `json:"id"`
	Modified string `json:"modified"` // RFC 3339, as querybatch returns it
}

// Severity is one severity score of a record, such as a CVSS vector.
type Severity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// Affected lists the affected versions of one package.
type Affected struct {
	Package          Package    `json:"package"`
	Ranges           []Range    `json:"ranges"`
	Versions         []string   `json:"versions"`
	Severity         []Severity `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

// Range is an OSV affected range: events in a SEMVER, ECOSYSTEM or GIT ordering. GIT ranges
// name their repository instead of a package.
type Range struct {
	Type   string  `json:"type"`
	Repo   string  `json:"repo,omitempty"`
	Events []Event `json:"events"`
}

// Event is one introduced, fixed, last_affected or limit event.
type Event struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
	Limit        string `json:"limit,omitempty"`
}

// OSVClient queries the OSV API.
type OSVClient struct {
	hc   *http.Client
	base string

	mu     sync.Mutex
	oldest time.Duration // the largest Age of a response read so far
	aged   bool          // a response had an Age header
}

// NewOSVClient returns a client that sends requests through hc to base (empty means [OSVURL]).
func NewOSVClient(hc *http.Client, base string) *OSVClient {
	if base == "" {
		base = OSVURL
	}
	return &OSVClient{hc: hc, base: strings.TrimSuffix(base, "/")}
}

// QueryBatch returns the records of each query, in query order. It follows each result's
// next_page_token.
func (c *OSVClient) QueryBatch(ctx context.Context, qs []Query) ([][]Hit, error) {
	out := make([][]Hit, len(qs))
	for start := 0; start < len(qs); start += maxBatch {
		end := min(start+maxBatch, len(qs))
		pending := make([]int, 0, end-start)
		tokens := map[int]string{}
		for i := start; i < end; i++ {
			pending = append(pending, i)
		}
		for len(pending) > 0 {
			hits, next, err := c.batch(ctx, qs, pending, tokens)
			if err != nil {
				return nil, err
			}
			var again []int
			for j, i := range pending {
				out[i] = append(out[i], hits[j]...)
				if next[j] != "" {
					tokens[i] = next[j]
					again = append(again, i)
				}
			}
			pending = again
		}
	}
	return out, nil
}

type batchQuery struct {
	Query

	PageToken string `json:"page_token,omitempty"`
}

func (c *OSVClient) batch(
	ctx context.Context,
	qs []Query,
	idx []int,
	tokens map[int]string,
) ([][]Hit, []string, error) {
	req := struct {
		Queries []batchQuery `json:"queries"`
	}{Queries: make([]batchQuery, len(idx))}
	for j, i := range idx {
		req.Queries[j] = batchQuery{Query: qs[i], PageToken: tokens[i]}
	}
	var resp struct {
		Results []struct {
			Vulns         []Hit  `json:"vulns"`
			NextPageToken string `json:"next_page_token"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/querybatch", req, &resp, true); err != nil {
		return nil, nil, err
	}
	if len(resp.Results) != len(idx) {
		return nil, nil, fmt.Errorf("osv querybatch: %d results for %d queries", len(resp.Results), len(idx))
	}
	hits := make([][]Hit, len(idx))
	next := make([]string, len(idx))
	for j, r := range resp.Results {
		hits[j] = r.Vulns
		next[j] = r.NextPageToken
	}
	return hits, next, nil
}

// Known reports, for each package, whether OSV has any record of it. It reads one page per
// package, sent as versionless queries in batches.
func (c *OSVClient) Known(ctx context.Context, pkgs []Package) ([]bool, error) {
	qs := make([]Query, len(pkgs))
	for i, p := range pkgs {
		qs[i] = Query{Package: p}
	}
	out := make([]bool, len(pkgs))
	for start := 0; start < len(qs); start += maxBatch {
		end := min(start+maxBatch, len(qs))
		idx := make([]int, 0, end-start)
		for i := start; i < end; i++ {
			idx = append(idx, i)
		}
		ids, _, err := c.batch(ctx, qs, idx, nil)
		if err != nil {
			return nil, err
		}
		for j, i := range idx {
			out[i] = len(ids[j]) > 0
		}
	}
	return out, nil
}

// OldestAge returns the largest Age header of the responses read so far: how long ago the oldest
// answer was stored in a cache. The second result is false when no response had one.
func (c *OSVClient) OldestAge() (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.oldest, c.aged
}

func (c *OSVClient) recordAge(h http.Header) {
	secs, err := strconv.ParseInt(h.Get("Age"), 10, 64)
	if err != nil || secs < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.oldest, c.aged = max(c.oldest, time.Duration(secs)*time.Second), true
}

// Vuln returns the record of id. A modified time from a querybatch hit keeps the record cached
// for 7 days while it is unchanged; such an answer is current, so its Age doesn't count for
// [OSVClient.OldestAge]. Empty modified caches the record like any other OSV answer.
func (c *OSVClient) Vuln(ctx context.Context, id, modified string) (*Record, error) {
	if modified != "" {
		ctx = httpx.WithVariant(ctx, "modified "+modified, recordTTL)
	}
	var r Record
	if err := c.do(ctx, http.MethodGet, "/v1/vulns/"+url.PathEscape(id), nil, &r, modified == ""); err != nil {
		return nil, err
	}
	return &r, nil
}

// do sends one request and decodes the answer into out. aged says whether the answer's Age
// counts for [OSVClient.OldestAge].
func (c *OSVClient) do(ctx context.Context, method, path string, in, out any, aged bool) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("osv %s: %w", path, err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("osv %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("osv %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && method == http.MethodGet {
		return fmt.Errorf("osv %s: %w", path, errNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("osv %s: HTTP %d", path, resp.StatusCode)
	}
	if aged {
		c.recordAge(resp.Header)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("osv %s: decode: %w", path, err)
	}
	return nil
}
