package enrich

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Prioritization feeds: the CISA Known Exploited Vulnerabilities catalog and FIRST EPSS.
const (
	KEVHost  = "www.cisa.gov"
	EPSSHost = "api.first.org"
	KEVURL   = "https://" + KEVHost + "/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	EPSSURL  = "https://" + EPSSHost + "/data/v1/epss"
)

// maxCVEParam is the longest cve parameter the EPSS API accepts, in characters.
const maxCVEParam = 2000

// KEV is an advisory's entry in the CISA Known Exploited Vulnerabilities catalog.
type KEV struct {
	CVE        string `json:"cve"`
	DateAdded  string `json:"date_added"` // YYYY-MM-DD
	DueDate    string `json:"due_date"`   // YYYY-MM-DD, the remediation deadline for US federal agencies
	Ransomware bool   `json:"ransomware"` // CISA knows of its use in ransomware campaigns
}

// EPSS is an advisory's FIRST EPSS score.
type EPSS struct {
	CVE        string  `json:"cve"`
	Score      float64 `json:"score"`      // probability of exploitation in the next 30 days, 0 to 1
	Percentile float64 `json:"percentile"` // share of scored CVEs at or below this score, 0 to 1
	Date       string  `json:"date"`       // YYYY-MM-DD the score was computed
}

// KEVClient reads the CISA KEV catalog once per client.
type KEVClient struct {
	hc  *http.Client
	url string

	mu      sync.Mutex
	catalog map[string]KEV
	err     error
	done    bool
}

// NewKEVClient returns a client that sends its request through hc to url (empty means
// [KEVURL]).
func NewKEVClient(hc *http.Client, url string) *KEVClient {
	return &KEVClient{hc: hc, url: cmp.Or(url, KEVURL)}
}

// Catalog returns the catalog's entries by CVE ID. The first call fetches it; later calls
// return the same answer.
func (c *KEVClient) Catalog(ctx context.Context) (map[string]KEV, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.done {
		c.catalog, c.err = c.fetch(ctx)
		c.done = ctx.Err() == nil
	}
	return c.catalog, c.err
}

func (c *KEVClient) fetch(ctx context.Context) (map[string]KEV, error) {
	var body struct {
		Vulnerabilities []struct {
			CVE        string `json:"cveID"`
			DateAdded  string `json:"dateAdded"`
			DueDate    string `json:"dueDate"`
			Ransomware string `json:"knownRansomwareCampaignUse"` // Known or Unknown
		} `json:"vulnerabilities"`
	}
	if err := getJSON(ctx, c.hc, c.url, &body); err != nil {
		return nil, fmt.Errorf("CISA KEV catalog: %w", err)
	}
	out := make(map[string]KEV, len(body.Vulnerabilities))
	for _, v := range body.Vulnerabilities {
		id := strings.ToUpper(strings.TrimSpace(v.CVE))
		out[id] = KEV{
			CVE: id, DateAdded: v.DateAdded, DueDate: v.DueDate,
			Ransomware: strings.EqualFold(v.Ransomware, "known"),
		}
	}
	return out, nil
}

// EPSSClient reads EPSS scores, each CVE once per client.
type EPSSClient struct {
	hc  *http.Client
	url string

	mu     sync.Mutex
	scores map[string]EPSS
	asked  map[string]bool
}

// NewEPSSClient returns a client that sends its requests through hc to url (empty means
// [EPSSURL]).
func NewEPSSClient(hc *http.Client, url string) *EPSSClient {
	return &EPSSClient{hc: hc, url: cmp.Or(url, EPSSURL), scores: map[string]EPSS{}, asked: map[string]bool{}}
}

// Scores returns the scores of the CVE IDs EPSS knows among cves. It sends the IDs it hasn't
// asked about yet, sorted, in as few requests as the API's limit on the cve parameter allows. A
// failed request fails the call, and its IDs are asked again next time.
func (c *EPSSClient) Scores(ctx context.Context, cves []string) (map[string]EPSS, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var todo []string
	for _, id := range cves {
		if id = strings.ToUpper(strings.TrimSpace(id)); !c.asked[id] && !slices.Contains(todo, id) {
			todo = append(todo, id)
		}
	}
	slices.Sort(todo)
	for _, chunk := range cveChunks(todo, maxCVEParam) {
		if err := c.fetch(ctx, chunk); err != nil {
			return nil, err
		}
	}
	out := map[string]EPSS{}
	for _, id := range cves {
		if s, ok := c.scores[strings.ToUpper(strings.TrimSpace(id))]; ok {
			out[s.CVE] = s
		}
	}
	return out, nil
}

func (c *EPSSClient) fetch(ctx context.Context, ids []string) error {
	// The API answers 100 rows unless limit asks for more.
	u := c.url + "?cve=" + strings.Join(ids, ",") + "&limit=" + strconv.Itoa(len(ids))
	var body struct {
		Data []struct {
			CVE        string `json:"cve"`
			EPSS       string `json:"epss"`
			Percentile string `json:"percentile"`
			Date       string `json:"date"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c.hc, u, &body); err != nil {
		return fmt.Errorf("EPSS: %w", err)
	}
	for _, d := range body.Data {
		score, err := strconv.ParseFloat(d.EPSS, 64)
		if err != nil {
			return fmt.Errorf("EPSS %s: score %q: %w", d.CVE, d.EPSS, err)
		}
		pct, err := strconv.ParseFloat(d.Percentile, 64)
		if err != nil {
			return fmt.Errorf("EPSS %s: percentile %q: %w", d.CVE, d.Percentile, err)
		}
		id := strings.ToUpper(d.CVE)
		c.scores[id] = EPSS{CVE: id, Score: score, Percentile: pct, Date: d.Date}
	}
	for _, id := range ids {
		c.asked[id] = true
	}
	return nil
}

// cveChunks splits sorted IDs into lists whose comma-joined length is at most limit.
func cveChunks(ids []string, limit int) [][]string {
	var (
		out  [][]string
		cur  []string
		size int
	)
	for _, id := range ids {
		add := len(id)
		if len(cur) > 0 {
			add++ // the comma
		}
		if len(cur) > 0 && size+add > limit {
			out = append(out, cur)
			cur, size, add = nil, 0, len(id)
		}
		cur = append(cur, id)
		size += add
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// getJSON sends a GET for u through hc and decodes a 200 answer into v.
func getJSON(ctx context.Context, hc *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err //nolint:wrapcheck // the caller names the feed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxJSONBytes))
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// cveIDs returns the CVE IDs among an advisory's ID and aliases.
func (v Vuln) cveIDs() []string {
	var out []string
	for _, n := range v.names() {
		if id := strings.ToUpper(n); strings.HasPrefix(id, "CVE-") && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// SetFeeds makes the enricher add the KEV entry and the EPSS score of each advisory's CVE
// aliases. A nil client skips its feed.
func (e *Enricher) SetFeeds(kev *KEVClient, epss *EPSSClient) { e.kev, e.epss = kev, epss }

// prioritize adds KEV entries and EPSS scores to the advisories in lists and sorts each list
// again. A feed that fails is a [Problem], and its field stays empty.
func (e *Enricher) prioritize(ctx context.Context, lists [][]Vuln) []Problem {
	if e.kev == nil && e.epss == nil {
		return nil
	}
	var cves []string
	for _, vs := range lists {
		for _, v := range vs {
			cves = append(cves, v.cveIDs()...)
		}
	}
	if len(cves) == 0 {
		return nil
	}
	var (
		kev      map[string]KEV
		epss     map[string]EPSS
		kevErr   error
		epssErr  error
		wg       sync.WaitGroup
		problems []Problem
	)
	if e.kev != nil {
		wg.Go(func() { kev, kevErr = e.kev.Catalog(ctx) })
	}
	if e.epss != nil {
		wg.Go(func() { epss, epssErr = e.epss.Scores(ctx, cves) })
	}
	wg.Wait()
	if kevErr != nil {
		problems = append(problems, Problem{Source: SourceKEV, Err: kevErr})
	}
	if epssErr != nil {
		problems = append(problems, Problem{Source: SourceEPSS, Err: epssErr})
	}
	for _, vs := range lists {
		for j := range vs {
			vs[j].KEV, vs[j].EPSS = pickKEV(kev, vs[j].cveIDs()), pickEPSS(epss, vs[j].cveIDs())
		}
		sortVulns(vs)
	}
	return problems
}

// pickKEV returns the catalog entry of the first of ids that CISA added, or nil.
func pickKEV(catalog map[string]KEV, ids []string) *KEV {
	var best *KEV
	for _, id := range ids {
		if k, ok := catalog[id]; ok && (best == nil || k.DateAdded < best.DateAdded) {
			best = &k
		}
	}
	return best
}

// pickEPSS returns the highest score among ids, or nil.
func pickEPSS(scores map[string]EPSS, ids []string) *EPSS {
	var best *EPSS
	for _, id := range ids {
		if s, ok := scores[id]; ok && (best == nil || s.Score > best.Score) {
			best = &s
		}
	}
	return best
}

// KEVFirst orders an advisory in the KEV catalog (a) before one that isn't (b): it returns -1
// when only a is listed, 1 when only b is, and 0 otherwise.
func KEVFirst(a, b *KEV) int {
	switch {
	case (a == nil) == (b == nil):
		return 0
	case a != nil:
		return -1
	default:
		return 1
	}
}

// EPSSScore returns the score of e, or -1 without one, so advisories without a score sort last.
func EPSSScore(e *EPSS) float64 {
	if e == nil {
		return -1
	}
	return e.Score
}
