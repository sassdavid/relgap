package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// Unchecked is a tool version that OSV wasn't asked about, with every pin of it. relgap never
// reports it as clean.
type Unchecked struct {
	Tool    string          `json:"tool"`    // the pins' tool, or the name as written for a pin that resolves to none
	Version string          `json:"version"` // the version OSV would be asked about, else the value as written
	Reason  string          `json:"reason"`  // an enrich.Reason* constant
	Pins    []scan.Location `json:"pins"`
}

// EnrichRequests returns one request per tool and version among the pins, with the latest
// version as the target. A pin is checked at Current, or at its own version when it is exact and
// its lookup failed. A pin without such a version gets its unchecked reason here.
func (o *Outdated) EnrichRequests(resolve ResolveFunc) []enrich.Request {
	var out []enrich.Request
	seen := map[string]bool{}
	for i := range o.Pins {
		p := &o.Pins[i]
		if p.Unchecked != "" {
			continue
		}
		spec, v, reason := vulnTarget(p, resolve)
		if reason != "" {
			p.Unchecked = reason
			continue
		}
		p.vulnVersion = v.String()
		key := enrichKey(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		req := enrich.Request{Key: key, Name: p.Tool, Tool: spec, Current: v}
		if p.Latest != "" && p.Latest != p.vulnVersion {
			if rules, err := spec.Version.Rules(); err == nil {
				if latest, latestErr := rules.Scheme.Parse(p.Latest); latestErr == nil {
					req.Target = latest
				}
			}
		}
		out = append(out, req)
	}
	return out
}

// vulnTarget returns the tool and version OSV is asked about for p, or the reason it can't be.
func vulnTarget(p *OutdatedPin, resolve ResolveFunc) (registry.ToolSpec, version.V, string) {
	if p.Status == StatusSkipped {
		return registry.ToolSpec{}, version.V{}, enrich.SkipReason(p.Tool, p.failure)
	}
	spec, err := resolve(p.Tool)
	if err != nil {
		return registry.ToolSpec{}, version.V{}, enrich.SkipReason(p.Tool, err)
	}
	rules, err := spec.Version.Rules()
	if err != nil {
		return registry.ToolSpec{}, version.V{}, enrich.ReasonFailed
	}
	raw := p.Current
	if raw == "" && p.Pin.Match == scan.MatchExact {
		raw = p.Pin.Version
	}
	if raw == "" && p.Status == StatusError && resolvable(p.Pin) {
		// A failed lookup left a non-exact pin without the version it would resolve to.
		return registry.ToolSpec{}, version.V{}, enrich.SkipReason(p.Tool, p.failure)
	}
	v, err := rules.Scheme.Parse(raw)
	if raw == "" || err != nil {
		return registry.ToolSpec{}, version.V{}, enrich.ReasonNotExact
	}
	return spec, v, ""
}

// resolvable reports whether a non-exact pin stands for one version once the tool's versions
// are known: a constraint, a prefix or the latest channel.
func resolvable(pin scan.Pin) bool {
	return pin.Match != scan.MatchChannel || strings.EqualFold(strings.TrimSpace(pin.Value), "latest")
}

// ApplyEnrichment sets the EOL state, vulnerabilities and unchecked reason of each pin, and
// lists the unchecked pins, adding unresolved, the pins that resolve to no tool.
func (o *Outdated) ApplyEnrichment(results map[string]enrich.Result, unresolved []scan.Pin) {
	for i := range o.Pins {
		p := &o.Pins[i]
		if p.Unchecked != "" {
			continue
		}
		if r, ok := results[enrichKey(p)]; ok {
			p.EOL, p.Vulns, p.Unchecked = r.EOL, r.Vulns, r.Unchecked
		}
	}
	o.Unchecked = uncheckedList(o.Pins, unresolved)
}

// uncheckedList groups the unchecked pins by tool, version and reason, in pin order, with the
// unresolved pins last.
func uncheckedList(pins []OutdatedPin, unresolved []scan.Pin) []Unchecked {
	out := []Unchecked{}
	index := map[string]int{}
	add := func(tool, ver, reason string, loc scan.Location) {
		key := tool + "\x00" + ver + "\x00" + reason
		at, ok := index[key]
		if !ok {
			at = len(out)
			index[key] = at
			out = append(out, Unchecked{Tool: tool, Version: ver, Reason: reason})
		}
		out[at].Pins = append(out[at].Pins, loc)
	}
	for i := range pins {
		if p := &pins[i]; p.Unchecked != "" {
			add(p.Tool, cmp.Or(p.vulnVersion, p.Pin.Value), p.Unchecked, p.Pin.Location)
		}
	}
	unresolved = slices.Clone(unresolved)
	slices.SortStableFunc(unresolved, func(a, b scan.Pin) int {
		return cmp.Or(cmp.Compare(a.RawName, b.RawName), compareLocations(a.Location, b.Location))
	})
	for _, p := range unresolved {
		add(p.RawName, p.Value, enrich.ReasonUnresolved, p.Location)
	}
	return out
}

func compareLocations(a, b scan.Location) int {
	return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
}

func enrichKey(p *OutdatedPin) string {
	if p.vulnVersion == "" {
		return ""
	}
	return p.Tool + "\x00" + p.vulnVersion
}
