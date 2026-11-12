package report

import (
	"github.com/sassdavid/relgap/internal/enrich"
)

// EnrichRequests returns one request per tool and current version among the compared pins, with
// the latest version as the target.
func (o *Outdated) EnrichRequests(resolve ResolveFunc) []enrich.Request {
	var out []enrich.Request
	seen := map[string]bool{}
	for _, p := range o.Pins {
		key := enrichKey(&p)
		if key == "" || seen[key] {
			continue
		}
		spec, err := resolve(p.Tool)
		if err != nil || (spec.EOL == nil && len(spec.OSV) == 0) {
			continue
		}
		rules, err := spec.Version.Rules()
		if err != nil {
			continue
		}
		current, err := rules.Scheme.Parse(p.Current)
		if err != nil {
			continue
		}
		req := enrich.Request{Key: key, Tool: spec, Current: current}
		if p.Latest != "" && p.Latest != p.Current {
			if latest, latestErr := rules.Scheme.Parse(p.Latest); latestErr == nil {
				req.Target = latest
			}
		}
		seen[key] = true
		out = append(out, req)
	}
	return out
}

// ApplyEnrichment sets the EOL state and vulnerabilities of each compared pin.
func (o *Outdated) ApplyEnrichment(results map[string]enrich.Result) {
	for i := range o.Pins {
		p := &o.Pins[i]
		r, ok := results[enrichKey(p)]
		if !ok {
			continue
		}
		p.EOL = r.EOL
		p.Vulns = r.Vulns
	}
}

func enrichKey(p *OutdatedPin) string {
	if p.Current == "" || (p.Status != StatusCurrent && p.Status != StatusOutdated) {
		return ""
	}
	return p.Tool + "\x00" + p.Current
}
