package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// EndOfLifeURL is the endoflife.date API v1 base URL.
const EndOfLifeURL = "https://endoflife.date/api/v1"

// ErrNoProduct is returned when endoflife.date has no such product.
var ErrNoProduct = errors.New("not an endoflife.date product")

// Cycle is one release cycle of an endoflife.date product.
type Cycle struct {
	Name         string `json:"name"`
	IsEOL        bool   `json:"isEol"`
	EOLFrom      string `json:"eolFrom"` // date; empty when not announced
	IsMaintained bool   `json:"isMaintained"`
	Latest       struct {
		Name string `json:"name"`
	} `json:"latest"`
}

// EOLClient reads products from the endoflife.date API v1.
type EOLClient struct {
	hc   *http.Client
	base string
}

// NewEOLClient returns a client that sends requests through hc to base (empty means
// [EndOfLifeURL]).
func NewEOLClient(hc *http.Client, base string) *EOLClient {
	if base == "" {
		base = EndOfLifeURL
	}
	return &EOLClient{hc: hc, base: strings.TrimSuffix(base, "/")}
}

// Cycles returns the release cycles of product. It wraps [ErrNoProduct] for 404.
func (c *EOLClient) Cycles(ctx context.Context, product string) ([]Cycle, error) {
	u := c.base + "/products/" + url.PathEscape(product)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("endoflife.date %s: %w", product, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("endoflife.date %s: %w", product, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("endoflife.date %s: %w", product, ErrNoProduct)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("endoflife.date %s: HTTP %d", product, resp.StatusCode)
	}
	var body struct {
		Result struct {
			Releases []Cycle `json:"releases"`
		} `json:"result"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("endoflife.date %s: decode: %w", product, err)
	}
	return body.Result.Releases, nil
}

const maxJSONBytes = 16 << 20

// EOLStatus is the end-of-life state of one version's cycle.
type EOLStatus struct {
	Version       string `json:"version"`
	Cycle         string `json:"cycle"`
	IsEOL         *bool  `json:"is_eol,omitempty"`          // absent when endoflife.date doesn't list the cycle
	EOLFrom       string `json:"eol_from,omitempty"`        // date the cycle reaches or reached its end
	LatestInCycle string `json:"latest_in_cycle,omitempty"` // newest release of the cycle
	Note          string `json:"note,omitempty"`
}

// EOL is the end-of-life state of a tool's current and target versions.
type EOL struct {
	Product string     `json:"product"`
	Current *EOLStatus `json:"current"`
	Target  *EOLStatus `json:"target,omitempty"`
}

// CycleName fills the registry cycle template for v; an empty template means {major}.{minor}.
func CycleName(e registry.EOL, s version.Scheme, v version.V) string {
	tmpl := e.Cycle
	if tmpl == "" {
		tmpl = registry.PlaceholderMajor + "." + registry.PlaceholderMinor
	}
	return source.Expand(tmpl, s, version.Version{V: v})
}

// Status looks up the cycle of v among cycles.
func Status(cycles []Cycle, e registry.EOL, s version.Scheme, v version.V) *EOLStatus {
	name := CycleName(e, s, v)
	st := &EOLStatus{Version: v.String(), Cycle: name}
	for _, c := range cycles {
		if c.Name != name {
			continue
		}
		eol := c.IsEOL
		st.IsEOL = &eol
		st.EOLFrom = c.EOLFrom
		st.LatestInCycle = c.Latest.Name
		return st
	}
	st.Note = fmt.Sprintf("endoflife.date doesn't list cycle %s of %s", name, e.Product)
	return st
}
