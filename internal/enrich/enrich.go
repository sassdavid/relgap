package enrich

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Sources an [Problem] can name.
const (
	SourceEOL = "endoflife.date"
	SourceOSV = "osv"
)

// Request asks for the EOL state and vulnerabilities of one tool version, and how a target
// version compares.
type Request struct {
	Key     string // the caller's key for the result
	Tool    registry.ToolSpec
	Current version.V
	Target  version.V // zero when there is no target
}

// tag is the tag of v, which GIT queries use: the tool's tag prefix and the version.
func (r Request) tag(v version.V) string { return r.Tool.Version.TagPrefix + v.String() }

// Result is what enrichment found for one request. A nil EOL or Vulns means the tool has no
// endoflife.date product or OSV package, or the source failed (see [Problem]).
type Result struct {
	EOL   *EOL
	Vulns []Vuln
}

// Problem is a source that failed for a tool. The rest of the report stands.
type Problem struct {
	Source string
	Tool   string
	Err    error
}

// Enricher looks tools up on endoflife.date and OSV.
type Enricher struct {
	logger      *slog.Logger
	eol         *EOLClient
	osv         *OSVClient
	concurrency int
}

// NewEnricher returns an enricher that uses eol and osv with up to concurrency requests at once.
func NewEnricher(logger *slog.Logger, eol *EOLClient, osv *OSVClient, concurrency int) *Enricher {
	return &Enricher{logger: logger, eol: eol, osv: osv, concurrency: max(concurrency, 1)}
}

// Enrich answers every request. It fetches each product's cycles once, sends all OSV queries in
// batches and fetches each record once. A failed source becomes a [Problem]; only a canceled
// context is an error.
func (e *Enricher) Enrich(ctx context.Context, reqs []Request) (map[string]Result, []Problem, error) {
	out := make(map[string]Result, len(reqs))
	for _, r := range reqs {
		out[r.Key] = Result{}
	}
	var problems []Problem
	eols, eolProblems := e.eolResults(ctx, reqs)
	problems = append(problems, eolProblems...)
	vulns, osvProblems := e.osvResults(ctx, reqs)
	problems = append(problems, osvProblems...)
	if err := ctx.Err(); err != nil {
		return nil, nil, err //nolint:wrapcheck // the caller matches context errors
	}
	for _, r := range reqs {
		out[r.Key] = Result{EOL: eols[r.Key], Vulns: vulns[r.Key]}
	}
	return out, problems, nil
}

func (e *Enricher) eolResults(ctx context.Context, reqs []Request) (map[string]*EOL, []Problem) {
	products := map[string][]Cycle{}
	failed := map[string]error{}
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(e.concurrency)
	seen := map[string]bool{}
	for _, r := range reqs {
		if r.Tool.EOL == nil || seen[r.Tool.EOL.Product] {
			continue
		}
		product := r.Tool.EOL.Product
		seen[product] = true
		g.Go(func() error {
			cycles, err := e.eol.Cycles(gctx, product)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed[product] = err
			} else {
				products[product] = cycles
			}
			return nil
		})
	}
	_ = g.Wait()
	out := map[string]*EOL{}
	var problems []Problem
	reported := map[string]bool{}
	for _, r := range reqs {
		if r.Tool.EOL == nil {
			continue
		}
		p := r.Tool.EOL.Product
		if err := failed[p]; err != nil {
			if !reported[r.Tool.Name] {
				reported[r.Tool.Name] = true
				problems = append(problems, Problem{Source: SourceEOL, Tool: r.Tool.Name, Err: err})
			}
			continue
		}
		rules, err := r.Tool.Version.Rules()
		if err != nil {
			continue
		}
		res := &EOL{Product: p, Current: Status(products[p], *r.Tool.EOL, rules.Scheme, r.Current)}
		if !r.Target.IsZero() {
			res.Target = Status(products[p], *r.Tool.EOL, rules.Scheme, r.Target)
		}
		out[r.Key] = res
	}
	return out, problems
}

// queryRef ties one OSV query to the request and package it came from.
type queryRef struct {
	req int
	pkg registry.OSVPackage
}

func (e *Enricher) osvResults(ctx context.Context, reqs []Request) (map[string][]Vuln, []Problem) {
	var (
		queries []Query
		refs    []queryRef
	)
	for i, r := range reqs {
		rules, err := r.Tool.Version.Rules()
		if err != nil || len(r.Tool.OSV) == 0 {
			continue
		}
		for _, p := range PackagesFor(r.Tool.OSV, rules.Scheme, r.Current) {
			queries = append(queries, Query{
				Package: Package{Ecosystem: p.Ecosystem, Name: p.Name},
				Version: QueryVersion(p, r.tag(r.Current), r.Current),
			})
			refs = append(refs, queryRef{req: i, pkg: p})
		}
	}
	if len(queries) == 0 {
		return map[string][]Vuln{}, nil
	}
	ids, err := e.osv.QueryBatch(ctx, queries)
	if err != nil {
		return map[string][]Vuln{}, problemsFor(reqs, refs, err)
	}
	records, err := e.records(ctx, ids)
	if err != nil {
		return map[string][]Vuln{}, problemsFor(reqs, refs, err)
	}
	out := map[string][]Vuln{}
	for i := range reqs {
		if len(reqs[i].Tool.OSV) > 0 {
			out[reqs[i].Key] = []Vuln{}
		}
	}
	for qi, ref := range refs {
		r := reqs[ref.req]
		for _, id := range ids[qi] {
			if rec := records[id]; rec != nil {
				out[r.Key] = append(out[r.Key], vulnOf(rec, ref.pkg, queries[qi], r))
			}
		}
	}
	for k, vs := range out {
		out[k] = mergeAliases(vs)
	}
	return out, nil
}

// vulnOf evaluates rec for the current and target versions of r.
func vulnOf(rec *Record, p registry.OSVPackage, q Query, r Request) Vuln {
	pkg := Package{Ecosystem: p.Ecosystem, Name: p.Name}
	v := Vuln{
		ID:       rec.ID,
		Aliases:  rec.Aliases,
		Summary:  rec.Summary,
		Package:  pkg,
		Severity: rec.Score(pkg),
		FixedIn:  FixedIn(rec, pkg, q.Version),
	}
	affected, known := Affects(rec, pkg, q.Version)
	// OSV returned the record for this version, so an undecidable range counts as affected.
	v.AffectsCurrent = affected || !known
	if !r.Target.IsZero() && known {
		hit, targetKnown := Affects(rec, pkg, QueryVersion(p, r.tag(r.Target), r.Target))
		if targetKnown {
			fixed := v.AffectsCurrent && !hit
			v.FixedByTarget = &fixed
		}
	}
	return v
}

func problemsFor(reqs []Request, refs []queryRef, err error) []Problem {
	var out []Problem
	seen := map[string]bool{}
	for _, ref := range refs {
		name := reqs[ref.req].Tool.Name
		if !seen[name] {
			seen[name] = true
			out = append(out, Problem{Source: SourceOSV, Tool: name, Err: err})
		}
	}
	return out
}

// records fetches each distinct record once.
func (e *Enricher) records(ctx context.Context, ids [][]string) (map[string]*Record, error) {
	var unique []string
	for _, list := range ids {
		unique = append(unique, list...)
	}
	slices.Sort(unique)
	unique = slices.Compact(unique)
	out := make(map[string]*Record, len(unique))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(e.concurrency)
	for _, id := range unique {
		g.Go(func() error {
			rec, err := e.osv.Vuln(gctx, id)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", id, err)
			}
			mu.Lock()
			out[id] = rec
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // names the record
	}
	return out, nil
}

// mergeAliases keeps one entry per vulnerability when several databases describe it (GO-2023-2055
// and GHSA-h626-pv66-hhm7). It keeps the entry with a severity, else the first ID, and sorts
// by score, then ID.
func mergeAliases(vs []Vuln) []Vuln {
	out := []Vuln{}
	index := map[string]int{}
	for _, v := range vs {
		at := -1
		for _, name := range append([]string{v.ID}, v.Aliases...) {
			if i, ok := index[name]; ok {
				at = i
				break
			}
		}
		if at < 0 {
			at = len(out)
			out = append(out, v)
		} else {
			out[at] = mergeVuln(out[at], v)
		}
		for _, name := range append([]string{v.ID}, v.Aliases...) {
			index[name] = at
		}
	}
	slices.SortStableFunc(out, func(a, b Vuln) int {
		return cmp.Or(cmp.Compare(scoreOf(b), scoreOf(a)), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func mergeVuln(a, b Vuln) Vuln {
	keep, other := a, b
	if a.Severity == nil && b.Severity != nil {
		keep, other = b, a
	}
	names := map[string]bool{keep.ID: true}
	var aliases []string
	for _, n := range append(append([]string{other.ID}, keep.Aliases...), other.Aliases...) {
		if !names[n] {
			names[n] = true
			aliases = append(aliases, n)
		}
	}
	slices.Sort(aliases)
	keep.Aliases = aliases
	keep.Summary = cmp.Or(keep.Summary, other.Summary)
	keep.FixedIn = cmp.Or(keep.FixedIn, other.FixedIn)
	keep.AffectsCurrent = keep.AffectsCurrent || other.AffectsCurrent
	if keep.FixedByTarget == nil {
		keep.FixedByTarget = other.FixedByTarget
	}
	return keep
}

func scoreOf(v Vuln) float64 {
	if v.Severity == nil {
		return -1
	}
	return v.Severity.Score
}
