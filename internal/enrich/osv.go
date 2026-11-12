package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OSVURL is the OSV API base URL.
const OSVURL = "https://api.osv.dev"

// maxBatch is the most queries OSV accepts in one querybatch request.
const maxBatch = 1000

// Package is a package in an OSV ecosystem.
type Package struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

// Query asks which vulnerabilities affect one version of a package.
type Query struct {
	Package Package `json:"package"`
	Version string  `json:"version"`
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
}

// NewOSVClient returns a client that sends requests through hc to base (empty means [OSVURL]).
func NewOSVClient(hc *http.Client, base string) *OSVClient {
	if base == "" {
		base = OSVURL
	}
	return &OSVClient{hc: hc, base: strings.TrimSuffix(base, "/")}
}

// QueryBatch returns the vulnerability IDs of each query, in query order. It follows each
// result's next_page_token.
func (c *OSVClient) QueryBatch(ctx context.Context, qs []Query) ([][]string, error) {
	out := make([][]string, len(qs))
	for start := 0; start < len(qs); start += maxBatch {
		end := min(start+maxBatch, len(qs))
		pending := make([]int, 0, end-start)
		tokens := map[int]string{}
		for i := start; i < end; i++ {
			pending = append(pending, i)
		}
		for len(pending) > 0 {
			ids, next, err := c.batch(ctx, qs, pending, tokens)
			if err != nil {
				return nil, err
			}
			var again []int
			for j, i := range pending {
				out[i] = append(out[i], ids[j]...)
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
) ([][]string, []string, error) {
	req := struct {
		Queries []batchQuery `json:"queries"`
	}{Queries: make([]batchQuery, len(idx))}
	for j, i := range idx {
		req.Queries[j] = batchQuery{Query: qs[i], PageToken: tokens[i]}
	}
	var resp struct {
		Results []struct {
			Vulns []struct {
				ID string `json:"id"`
			} `json:"vulns"`
			NextPageToken string `json:"next_page_token"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/querybatch", req, &resp); err != nil {
		return nil, nil, err
	}
	if len(resp.Results) != len(idx) {
		return nil, nil, fmt.Errorf("osv querybatch: %d results for %d queries", len(resp.Results), len(idx))
	}
	ids := make([][]string, len(idx))
	next := make([]string, len(idx))
	for j, r := range resp.Results {
		for _, v := range r.Vulns {
			ids[j] = append(ids[j], v.ID)
		}
		next[j] = r.NextPageToken
	}
	return ids, next, nil
}

// Vuln returns the record of id.
func (c *OSVClient) Vuln(ctx context.Context, id string) (*Record, error) {
	var r Record
	if err := c.do(ctx, http.MethodGet, "/v1/vulns/"+url.PathEscape(id), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *OSVClient) do(ctx context.Context, method, path string, in, out any) error {
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("osv %s: HTTP %d", path, resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(out); err != nil {
		return fmt.Errorf("osv %s: decode: %w", path, err)
	}
	return nil
}
