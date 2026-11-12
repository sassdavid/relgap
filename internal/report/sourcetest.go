package report

import (
	"context"
	"regexp"
	"slices"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/registry"
)

// SourceTest is the data of `relgap registry test`: what each source of a tool produced over a
// range, and the merged digest's counts.
type SourceTest struct {
	Tool     string         `json:"tool"`
	Repo     string         `json:"repo"`
	From     string         `json:"from"`
	To       string         `json:"to"`
	Versions []string       `json:"versions"` // the versions in the range, oldest first
	Sources  []SourceResult `json:"sources"`
	Merged   SourceResult   `json:"merged"`
}

// SourceResult is what one source produced. Error is set when it failed.
type SourceResult struct {
	Index    int                   `json:"index"` // position in the registry entry; -1 for merged
	Source   *registry.Source      `json:"source,omitempty"`
	Versions []string              `json:"versions"` // versions with at least one entry
	Entries  int                   `json:"entries"`
	Counts   map[classify.Kind]int `json:"counts"`
	URLs     []string              `json:"urls"` // distinct documents the entries came from
	Warnings []string              `json:"warnings"`
	Error    string                `json:"error,omitempty"`
}

// TestSources digests the range once per source, then with every source, ignoring the minimum
// confidence. A failing source doesn't stop the others.
func (d *Digester) TestSources(ctx context.Context, req DigestRequest) (*SourceTest, error) {
	req.MinConfidence = 0
	merged, err := d.Digest(ctx, req)
	if err != nil {
		return nil, err
	}
	out := &SourceTest{Tool: merged.Tool, Repo: merged.Repo, From: merged.From, To: merged.To, Versions: []string{}}
	for _, v := range merged.Versions {
		out.Versions = append(out.Versions, v.Version)
	}
	out.Merged = summarize(-1, nil, merged)
	for i := range req.Tool.Sources {
		one := req
		one.Only = &i
		s := req.Tool.Sources[i]
		dg, oneErr := d.Digest(ctx, one)
		if oneErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err() //nolint:wrapcheck // the caller matches context errors
			}
			out.Sources = append(out.Sources, SourceResult{
				Index: i, Source: &s, Versions: []string{}, Counts: kindCounts(),
				URLs: []string{}, Warnings: []string{}, Error: oneErr.Error(),
			})
			continue
		}
		out.Sources = append(out.Sources, summarize(i, &s, dg))
	}
	return out, nil
}

// summarize counts a digest's entries, leaving out the synthetic version-jump entries.
func summarize(i int, s *registry.Source, dg *ToolDigest) SourceResult {
	r := SourceResult{
		Index: i, Source: s, Versions: []string{}, Counts: kindCounts(), URLs: []string{},
		Warnings: append([]string{}, dg.Warnings...),
	}
	for _, e := range dg.Entries {
		if e.Source.Adapter == classify.AdapterVersionJump {
			continue
		}
		r.Entries++
		r.Counts[e.Kind]++
		if !slices.Contains(r.Versions, e.Version) {
			r.Versions = append(r.Versions, e.Version)
		}
		u := documentURL(e.Source.URL)
		if !slices.Contains(r.URLs, u) {
			r.URLs = append(r.URLs, u)
		}
	}
	return r
}

// kindCounts returns a count of zero for every kind.
func kindCounts() map[classify.Kind]int {
	out := make(map[classify.Kind]int, len(classify.Kinds()))
	for _, k := range classify.Kinds() {
		out[k] = 0
	}
	return out
}

var lineAnchor = regexp.MustCompile(`#L\d+$`)

// documentURL drops a line anchor, so entries of one file count as one document.
func documentURL(u string) string { return lineAnchor.ReplaceAllString(u, "") }
