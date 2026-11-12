package report

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// ViaFloor says a constraint pin counts as its own lower bound, next to [version.PickKnown].
const ViaFloor = "lower bound of the constraint"

// Drift is the data of `relgap drift --format json`.
type Drift struct {
	Repos []string    `json:"repos"`
	Tools []DriftTool `json:"tools"` // sorted by tool
}

// DriftTool is every pin of one tool across the scanned repositories.
type DriftTool struct {
	Tool      string         `json:"tool"`
	Drift     bool           `json:"drift"`               // the pins stand for more than one version
	Reference string         `json:"reference,omitempty"` // the version most repositories pin; the newest on a tie
	Latest    string         `json:"latest,omitempty"`    // with --latest: the newest stable version on Reference's line
	Note      string         `json:"note,omitempty"`      // why Latest is missing
	Versions  []DriftVersion `json:"versions"`            // oldest first
	Pins      []DriftPin     `json:"pins"`                // sorted by repo, file and line

	// Err is set when looking up Latest failed.
	Err error `json:"-"`
}

// DriftVersion is one distinct version of a tool and the repositories that pin it.
type DriftVersion struct {
	Version string   `json:"version"`
	Repos   []string `json:"repos"`
}

// DriftPin is one pin and the version it counts as.
type DriftPin struct {
	Location scan.Location `json:"location"`
	Value    string        `json:"value"`
	Match    scan.Match    `json:"match"`
	Version  string        `json:"version,omitempty"` // absent for a pin that isn't compared
	Via      string        `json:"via,omitempty"`     // how a constraint pin became Version
}

// DriftRequest says which pins to compare.
type DriftRequest struct {
	Repos             []string
	Pins              []scan.Pin // resolved pins only
	Resolve           ResolveFunc
	IgnoreConstraints bool // constraint pins aren't compared
	Latest            bool // look up the newest version of each tool
}

// Drifter compares the pins of each tool across repositories.
type Drifter struct {
	logger *slog.Logger
	recent RecentLister
}

// NewDrifter returns a drifter that looks up latest versions through recent. Without
// [DriftRequest.Latest] it makes no call.
func NewDrifter(logger *slog.Logger, recent RecentLister) *Drifter {
	return &Drifter{logger: logger, recent: recent}
}

// driftGroup is one tool's pins and how to read their versions.
type driftGroup struct {
	tool  *DriftTool
	spec  registry.ToolSpec
	err   error // from resolving the tool
	rules version.Rules
	pins  []scan.Pin
}

// Drift compares every pin. Exact pins count as their version. A constraint counts as the highest
// exact pin of the tool in the same repository that satisfies it, else its lower bound. Prefix
// and channel pins aren't compared. It returns an error only when the latest lookup fails for
// every tool, such as for a rejected token.
func (d *Drifter) Drift(ctx context.Context, req DriftRequest) (*Drift, error) {
	groups := d.group(req)
	out := &Drift{Repos: append([]string{}, req.Repos...), Tools: make([]DriftTool, 0, len(groups))}
	for _, g := range groups {
		d.compare(ctx, g, req.IgnoreConstraints)
	}
	if req.Latest {
		if err := d.latest(ctx, groups); err != nil {
			return nil, err
		}
	}
	for _, g := range groups {
		out.Tools = append(out.Tools, *g.tool)
	}
	return out, nil
}

func (d *Drifter) group(req DriftRequest) []*driftGroup {
	byTool := map[string]*driftGroup{}
	var groups []*driftGroup
	for _, p := range req.Pins {
		g, ok := byTool[p.Tool]
		if !ok {
			g = &driftGroup{tool: &DriftTool{Tool: p.Tool}, rules: version.Rules{Scheme: version.Semver()}}
			g.spec, g.err = req.Resolve(p.Tool)
			if g.err == nil {
				if rules, err := g.spec.Version.Rules(); err == nil {
					g.rules = rules
				}
			}
			byTool[p.Tool] = g
			groups = append(groups, g)
		}
		g.pins = append(g.pins, p)
	}
	slices.SortFunc(groups, func(a, b *driftGroup) int { return cmp.Compare(a.tool.Tool, b.tool.Tool) })
	return groups
}

// counted is a pin's parsed version.
type counted struct {
	v    version.V
	repo string
}

func (d *Drifter) compare(ctx context.Context, g *driftGroup, ignoreConstraints bool) {
	s := g.rules.Scheme
	empty := version.NewUniverse(ctx, d.logger, nil, g.rules)
	known := map[string][]version.V{} // exact versions per repository
	for _, p := range g.pins {
		if p.Match == scan.MatchExact {
			if v, err := s.Parse(p.Version); err == nil {
				known[p.Location.Repo] = append(known[p.Location.Repo], v)
			}
		}
	}
	var vs []counted
	for _, p := range g.pins {
		dp := DriftPin{Location: p.Location, Value: p.Value, Match: p.Match}
		if !ignoreConstraints || p.Match != scan.MatchConstraint {
			if v, via, ok := pinVersion(s, empty, p, known[p.Location.Repo]); ok {
				dp.Version, dp.Via = v.String(), via
				vs = append(vs, counted{v: v, repo: p.Location.Repo})
			}
		}
		g.tool.Pins = append(g.tool.Pins, dp)
	}
	g.tool.Versions = distinct(s, vs)
	g.tool.Drift = len(g.tool.Versions) > 1
	g.tool.Reference = reference(g.tool.Versions)
	for i := range g.tool.Pins {
		g.tool.Pins[i].Version = canonical(s, g.tool.Versions, g.tool.Pins[i].Version)
	}
	slices.SortStableFunc(g.tool.Pins, func(a, b DriftPin) int {
		return cmp.Or(
			cmp.Compare(a.Location.Repo, b.Location.Repo),
			cmp.Compare(a.Location.File, b.Location.File),
			cmp.Compare(a.Location.Line, b.Location.Line),
		)
	})
}

// pinVersion returns the version p counts as and, for a constraint, how it was chosen. known
// holds the exact versions of the tool in p's repository.
func pinVersion(s version.Scheme, empty *version.Universe, p scan.Pin, known []version.V) (version.V, string, bool) {
	switch p.Match {
	case scan.MatchExact:
		v, err := s.Parse(p.Version)
		return v, "", err == nil
	case scan.MatchConstraint:
		c, err := version.ParseConstraint(s, p.Value)
		if err != nil {
			return version.V{}, "", false
		}
		// The empty universe makes ResolveConstraint pick from the known pins only.
		if v, pick, rerr := empty.ResolveConstraint(c, known); rerr == nil {
			return v, string(pick), true
		}
		if v, ok := c.Floor(); ok {
			return v, ViaFloor, true
		}
	case scan.MatchPrefix, scan.MatchChannel:
	}
	return version.V{}, "", false
}

// distinct groups the versions the scheme reads as equal, oldest first. A group takes its
// longest text, so 1.15 and 1.15.0 share one entry, 1.15.0.
func distinct(s version.Scheme, vs []counted) []DriftVersion {
	slices.SortStableFunc(vs, func(a, b counted) int {
		return cmp.Or(
			s.Compare(a.v, b.v),
			cmp.Compare(len(b.v.String()), len(a.v.String())),
			cmp.Compare(a.v.String(), b.v.String()),
			cmp.Compare(a.repo, b.repo),
		)
	})
	out := []DriftVersion{}
	var last version.V
	for _, c := range vs {
		if len(out) == 0 || s.Compare(c.v, last) != 0 {
			out = append(out, DriftVersion{Version: c.v.String()})
			last = c.v
		}
		cur := &out[len(out)-1]
		if !slices.Contains(cur.Repos, c.repo) {
			cur.Repos = append(cur.Repos, c.repo)
		}
	}
	for i := range out {
		slices.Sort(out[i].Repos)
	}
	return out
}

// canonical returns the text of the group v belongs to, so every pin of a group shows one text.
func canonical(s version.Scheme, groups []DriftVersion, v string) string {
	if v == "" {
		return ""
	}
	parsed, err := s.Parse(v)
	if err != nil {
		return v
	}
	for _, g := range groups {
		if gv, gerr := s.Parse(g.Version); gerr == nil && s.Compare(gv, parsed) == 0 {
			return g.Version
		}
	}
	return v
}

// reference returns the version the most repositories pin, the newest on a tie.
func reference(vs []DriftVersion) string {
	best := -1
	for i, v := range vs {
		if best < 0 || len(v.Repos) >= len(vs[best].Repos) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return vs[best].Version
}

// latest sets the newest stable version of every tool that has a GitHub repository, on the
// registry line of the tool's reference version, from one batched lookup of the newest releases.
func (d *Drifter) latest(ctx context.Context, groups []*driftGroup) error {
	var queries []github.RecentQuery
	for _, g := range groups {
		if g.err == nil && g.spec.Repo != "" {
			queries = append(queries, github.RecentQuery{
				Repo: g.spec.Repo, Tags: g.spec.Version.Universe == registry.UniverseTags,
			})
		}
	}
	recent, err := d.recent.Recent(ctx, queries)
	if err != nil {
		return err //nolint:wrapcheck // Recent names what it listed
	}
	for _, g := range groups {
		t := g.tool
		switch {
		case g.err != nil:
			t.Note = "not looked up: " + g.err.Error()
			continue
		case g.spec.Repo == "":
			t.Note = "not looked up: the tool has no GitHub repository"
			continue
		}
		r := recent[g.spec.Repo]
		if errors.Is(r.Err, github.ErrAuth) {
			return r.Err
		}
		if r.Err != nil {
			t.Err = fmt.Errorf("list releases of %s: %w", g.spec.Repo, r.Err)
			t.Note = t.Err.Error()
			continue
		}
		u := version.NewUniverse(ctx, d.logger, recentRefs(r.Releases), g.rules)
		var from version.V
		if t.Reference != "" {
			from, _ = u.Parse(t.Reference) // a zero from means the newest version on any line
		}
		v, ok := u.Latest(from, version.Level(g.spec.Version.Line))
		if !ok {
			t.Note = fmt.Sprintf("no stable version on the line of %s in the newest %d releases",
				cmp.Or(t.Reference, "any version"), github.RecentWindow)
			continue
		}
		t.Latest = v.V.String()
	}
	return nil
}

// Cell is what one repository pins of a tool.
type Cell struct {
	Values []CellValue
}

// CellValue is one version or uncompared value in a [Cell].
type CellValue struct {
	Text    string // a version, or the value of a pin that isn't compared
	Approx  bool   // no exact pin in the repository backs it
	Outlier bool   // differs from the tool's reference version
}

// Cell returns what repo pins of t: its distinct versions oldest first, then the values of pins
// that aren't compared. It is empty when repo doesn't pin the tool.
func (t *DriftTool) Cell(repo string) Cell {
	var c Cell
	exact := map[string]bool{}
	var approx, uncompared []string
	for _, p := range t.Pins {
		switch {
		case p.Location.Repo != repo:
		case p.Version != "" && p.Match == scan.MatchExact:
			exact[p.Version] = true
		case p.Version != "":
			approx = append(approx, p.Version)
		default:
			uncompared = append(uncompared, p.Value)
		}
	}
	for _, v := range t.Versions {
		if exact[v.Version] || slices.Contains(approx, v.Version) {
			c.Values = append(c.Values, CellValue{
				Text: v.Version, Approx: !exact[v.Version], Outlier: t.Drift && v.Version != t.Reference,
			})
		}
	}
	slices.Sort(uncompared)
	for _, v := range slices.Compact(uncompared) {
		c.Values = append(c.Values, CellValue{Text: v, Approx: true})
	}
	return c
}

// Outliers returns the compared pins of a drifting tool whose version differs from Reference.
func (t *DriftTool) Outliers() []DriftPin {
	if !t.Drift {
		return nil
	}
	var out []DriftPin
	for _, p := range t.Pins {
		if p.Version != "" && p.Version != t.Reference {
			out = append(out, p)
		}
	}
	return out
}

// Drifting returns the tools whose pins stand for more than one version.
func (d *Drift) Drifting() []DriftTool {
	var out []DriftTool
	for _, t := range d.Tools {
		if t.Drift {
			out = append(out, t)
		}
	}
	return out
}
