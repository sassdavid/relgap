package enrich

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/httpx"
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
	Name    string // the pin's tool name, such as chart:argo/argo-cd; empty means Tool.Name
	Tool    registry.ToolSpec
	Current version.V
	Target  version.V // zero when there is no target
	// Versions lists the tool's releases for the fix target; nil skips it. It runs only when OSV
	// lists an advisory for Current.
	Versions VersionLister
}

// tag is the tag of v, which GIT queries use: the tool's tag prefix and the version.
func (r Request) tag(v version.V) string { return r.Tool.Version.TagPrefix + v.String() }

// Result is what enrichment found for one request. A nil EOL means the tool has no
// endoflife.date product or the source failed (see [Problem]). Vulns is nil when OSV wasn't
// asked or failed, and Unchecked says which. Fix is nil without an advisory of the current
// version, or when its version list or OSV failed.
type Result struct {
	EOL       *EOL
	Vulns     []Vuln
	Unchecked string // a Reason* constant; empty when OSV answered
	Fix       *Fix
}

// coverage returns what OSV is asked about the request.
func (r Request) coverage() Coverage { return r.coverageAt(r.Current) }

// coverageAt returns what OSV is asked about version v of the request's tool.
func (r Request) coverageAt(v version.V) Coverage {
	return CoverageOf(cmp.Or(r.Name, r.Tool.Name), r.Tool, v)
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
// A nil eol skips endoflife.date.
func NewEnricher(logger *slog.Logger, eol *EOLClient, osv *OSVClient, concurrency int) *Enricher {
	return &Enricher{logger: logger, eol: eol, osv: osv, concurrency: max(concurrency, 1)}
}

// Enrich answers every request. It asks endoflife.date and OSV at the same time, fetches each
// product's cycles once, sends all OSV queries in batches and fetches each record once. A failed
// source becomes a [Problem]; only a canceled context is an error.
func (e *Enricher) Enrich(ctx context.Context, reqs []Request) (map[string]Result, []Problem, error) {
	out := make(map[string]Result, len(reqs))
	var (
		eols        map[string]*EOL
		eolProblems []Problem
		wg          sync.WaitGroup
	)
	if e.eol != nil {
		wg.Go(func() { eols, eolProblems = e.eolResults(ctx, reqs) })
	}
	covs := make([]Coverage, len(reqs))
	for i, r := range reqs {
		covs[i] = r.coverage()
	}
	ans := e.osvResults(ctx, reqs, covs)
	fixes := e.fixTargets(ctx, reqs, ans)
	wg.Wait()
	problems := slices.Concat(eolProblems, ans.problems)
	if err := ctx.Err(); err != nil {
		return nil, nil, err //nolint:wrapcheck // the caller matches context errors
	}
	for i, r := range reqs {
		reason := covs[i].Reason
		if err := ans.failed[i]; err != nil && reason == "" {
			reason = failureReason(err)
		}
		out[r.Key] = Result{EOL: eols[r.Key], Vulns: ans.vulns[r.Key], Unchecked: reason, Fix: fixes[r.Key]}
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

// queryRef ties a request's package to the OSV query that asks about it, and to the version its
// answer is evaluated at.
type queryRef struct {
	req     int
	pkg     registry.OSVPackage
	query   int    // index into the queries
	version string // the request's current version as the package's ranges spell it
}

// osvAnswer is what OSV said about the requests.
type osvAnswer struct {
	vulns    map[string][]Vuln  // by request key, for each checked request
	failed   map[int]error      // by request index, for each request whose queries failed
	problems []Problem          // the failed sources
	sources  map[int][]evidence // by request index, the records of advisories of Current
	local    map[int][]evidence // by request index, every record of its versionless packages
	modules  map[string]string  // the module each go: path resolved to
}

// osvResults asks OSV about every checked request.
func (e *Enricher) osvResults(ctx context.Context, reqs []Request, covs []Coverage) osvAnswer {
	modules, failed, problems := e.resolveModules(ctx, reqs, covs)
	ans := osvAnswer{
		vulns: map[string][]Vuln{}, failed: failed, problems: problems,
		sources: map[int][]evidence{}, local: map[int][]evidence{}, modules: modules,
	}
	out := ans.vulns
	for i := range reqs {
		if covs[i].Checked() && failed[i] == nil {
			out[reqs[i].Key] = []Vuln{}
		}
	}
	queries, refs := osvQueries(reqs, covs, modules, func(i int) bool { return failed[i] != nil })
	if len(queries) == 0 {
		return ans
	}
	fail := func(err error) osvAnswer {
		for _, ref := range refs {
			failed[ref.req] = err
		}
		return osvAnswer{
			vulns:    map[string][]Vuln{},
			failed:   failed,
			problems: append(problems, problemsFor(reqs, refs, err)...),
		}
	}
	hits, err := e.osv.QueryBatch(ctx, queries)
	if err != nil {
		return fail(err)
	}
	records, err := e.records(ctx, slices.Concat(hits...))
	if err != nil {
		return fail(err)
	}
	ans.collect(reqs, refs, hits, records)
	lists := make([][]Vuln, 0, len(out))
	for k, vs := range out {
		out[k] = mergeAliases(vs)
		lists = append(lists, out[k])
	}
	if err = e.rateFromAliases(ctx, lists, records); err != nil {
		return fail(err)
	}
	for _, vs := range out {
		sortVulns(vs)
	}
	return ans
}

// collect adds the vulnerability each record describes to its request, and keeps the records of
// the advisories of Current as their evidence. A versionless answer lists every advisory of the
// package, so only the records whose ranges hold Current count; one whose ranges can't place
// Current doesn't, since OSV didn't match it to the version.
func (ans osvAnswer) collect(reqs []Request, refs []queryRef, hits [][]Hit, records map[string]*Record) {
	for _, ref := range refs {
		r := reqs[ref.req]
		for _, h := range hits[ref.query] {
			rec := records[h.ID]
			if rec == nil {
				continue
			}
			if versionless(ref.pkg) {
				ans.local[ref.req] = append(ans.local[ref.req], evidence{rec: rec, pkg: ref.pkg})
				pkg := Package{Ecosystem: ref.pkg.Ecosystem, Name: ref.pkg.Name}
				if affected, known := Affects(rec, pkg, ref.version); !affected || !known {
					continue
				}
			}
			v := vulnOf(rec, ref.pkg, ref.version, r)
			ans.vulns[r.Key] = append(ans.vulns[r.Key], v)
			if v.AffectsCurrent {
				ans.sources[ref.req] = append(ans.sources[ref.req], evidence{rec: rec, pkg: ref.pkg})
			}
		}
	}
}

// resolveModules finds the module of each go: path. When that fails, every request with a go:
// path fails with it.
func (e *Enricher) resolveModules(
	ctx context.Context,
	reqs []Request,
	covs []Coverage,
) (map[string]string, map[int]error, []Problem) {
	failed := map[int]error{}
	modules, err := e.goModules(ctx, covs)
	if err == nil {
		return modules, failed, nil
	}
	for i, c := range covs {
		if c.GoPath != "" {
			failed[i] = err
		}
	}
	return nil, failed, problemsFor(reqs, goPathRefs(covs), err)
}

// failureReason is the unchecked reason of a request whose OSV queries failed with err.
func failureReason(err error) string {
	if errors.Is(err, httpx.ErrOffline) {
		return ReasonOffline
	}
	return ReasonFailed
}

// osvQueries lists the OSV queries of every request that failed doesn't exclude, with the module
// each go: path resolved to. Each distinct query is sent once: a versionless package is asked
// about once for all its versions.
func osvQueries(
	reqs []Request,
	covs []Coverage,
	modules map[string]string,
	failed func(int) bool,
) ([]Query, []queryRef) {
	var (
		queries []Query
		refs    []queryRef
	)
	index := map[Query]int{}
	for i, r := range reqs {
		if failed(i) {
			continue
		}
		pkgs := covs[i].Packages
		if m := modules[covs[i].GoPath]; m != "" {
			pkgs = append(slices.Clip(pkgs), registry.OSVPackage{Ecosystem: EcosystemGo, Name: m})
		}
		for _, p := range pkgs {
			ver := QueryVersion(p, r.tag(r.Current), r.Current)
			q := Query{Package: Package{Ecosystem: p.Ecosystem, Name: p.Name}, Version: ver}
			if versionless(p) {
				q.Version = ""
			}
			at, ok := index[q]
			if !ok {
				at = len(queries)
				index[q] = at
				queries = append(queries, q)
			}
			refs = append(refs, queryRef{req: i, pkg: p, query: at, version: ver})
		}
	}
	return queries, refs
}

// goModules finds the module of each go: package path: its longest prefix that OSV has records
// of. A path without one has no module, and its tool is checked with no records.
func (e *Enricher) goModules(ctx context.Context, covs []Coverage) (map[string]string, error) {
	var (
		pkgs  []Package
		owner []string
	)
	seen := map[string]bool{}
	for _, c := range covs {
		if c.GoPath == "" || seen[c.GoPath] {
			continue
		}
		seen[c.GoPath] = true
		for _, prefix := range GoPrefixes(c.GoPath) {
			pkgs = append(pkgs, Package{Ecosystem: EcosystemGo, Name: prefix})
			owner = append(owner, c.GoPath)
		}
	}
	out := map[string]string{}
	if len(pkgs) == 0 {
		return out, nil
	}
	known, err := e.osv.Known(ctx, pkgs)
	if err != nil {
		return nil, err
	}
	for i, ok := range known {
		if _, done := out[owner[i]]; ok && !done {
			out[owner[i]] = pkgs[i].Name // prefixes come longest first
		}
	}
	return out, nil
}

// goPathRefs ties each request with a go: path to a ref, so a failed module search names its
// tools.
func goPathRefs(covs []Coverage) []queryRef {
	var out []queryRef
	for i, c := range covs {
		if c.GoPath != "" {
			out = append(out, queryRef{req: i})
		}
	}
	return out
}

// vulnOf evaluates rec for the current version of r, spelled at, and for its target version.
func vulnOf(rec *Record, p registry.OSVPackage, at string, r Request) Vuln {
	pkg := Package{Ecosystem: p.Ecosystem, Name: p.Name}
	v := Vuln{
		ID:       rec.ID,
		Aliases:  rec.Aliases,
		Summary:  rec.Summary,
		Package:  pkg,
		Severity: rec.Score(pkg),
		FixedIn:  FixedIn(rec, pkg, at),
	}
	affected, known := Affects(rec, pkg, at)
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

// records fetches each distinct record once. A withdrawn record maps to nil, so callers skip it
// and know it was fetched.
func (e *Enricher) records(ctx context.Context, hits []Hit) (map[string]*Record, error) {
	return e.fetch(ctx, hits, false)
}

// fetch fetches each distinct record of hits once, keyed by its modified time when the hit has
// one. With missingOK, an ID OSV has no record of maps to nil instead of failing.
func (e *Enricher) fetch(ctx context.Context, hits []Hit, missingOK bool) (map[string]*Record, error) {
	slices.SortFunc(hits, func(a, b Hit) int { return cmp.Compare(a.ID, b.ID) })
	hits = slices.CompactFunc(hits, func(a, b Hit) bool { return a.ID == b.ID })
	out := make(map[string]*Record, len(hits))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(e.concurrency)
	for _, h := range hits {
		g.Go(func() error {
			rec, err := e.osv.Vuln(gctx, h.ID, h.Modified)
			switch {
			case missingOK && errors.Is(err, errNotFound):
				e.logger.DebugContext(gctx, "osv has no record of alias", "id", h.ID)
				rec = nil
			case err != nil:
				return fmt.Errorf("fetch %s: %w", h.ID, err)
			case !rec.Withdrawn.IsZero():
				e.logger.DebugContext(gctx, "dropped withdrawn osv record", "id", h.ID, "withdrawn", rec.Withdrawn)
				rec = nil
			}
			mu.Lock()
			out[h.ID] = rec
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // names the record
	}
	return out, nil
}

// ghsaPrefix starts the IDs of GitHub advisories, which carry a rating when Go vulnerability
// database records don't.
const ghsaPrefix = "GHSA-"

// rateFromAliases gives each unrated vulnerability in lists the highest score of its GHSA
// aliases, fetching each alias records doesn't hold once, and rates the rest unknown.
func (e *Enricher) rateFromAliases(ctx context.Context, lists [][]Vuln, records map[string]*Record) error {
	if missing := missingAliases(lists, records); len(missing) > 0 {
		aliases, err := e.fetch(ctx, missing, true)
		if err != nil {
			return err
		}
		records = maps.Clone(records)
		maps.Copy(records, aliases)
	}
	for _, vs := range lists {
		for i := range vs {
			if vs[i].Severity == nil {
				vs[i].Severity = aliasRating(vs[i].Aliases, records)
			}
		}
	}
	return nil
}

// missingAliases lists the GHSA aliases of the unrated vulnerabilities in lists that records
// doesn't hold.
func missingAliases(lists [][]Vuln, records map[string]*Record) []Hit {
	var out []Hit
	for _, vs := range lists {
		for _, v := range vs {
			if v.Severity != nil {
				continue
			}
			for _, a := range v.Aliases {
				if _, ok := records[a]; !ok && strings.HasPrefix(a, ghsaPrefix) {
					out = append(out, Hit{ID: a})
				}
			}
		}
	}
	return out
}

// aliasRating returns the highest score among the GHSA aliases in records, or the rating
// unknown.
func aliasRating(aliases []string, records map[string]*Record) *Score {
	var best *Score
	for _, a := range aliases {
		rec := records[a]
		if rec == nil || !strings.HasPrefix(a, ghsaPrefix) {
			continue
		}
		if s := rec.aliasScore(); s != nil && (best == nil || s.Score > best.Score) {
			best = s
		}
	}
	if best == nil {
		return &Score{Rating: RatingUnknown}
	}
	return best
}

// mergeAliases keeps one entry per vulnerability when several databases describe it (GO-2023-2055
// and GHSA-h626-pv66-hhm7). It keeps the entry with a severity, else the first ID.
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
	return out
}

// sortVulns sorts by score, with unknown ratings last, then by ID.
func sortVulns(vs []Vuln) {
	slices.SortStableFunc(vs, func(a, b Vuln) int {
		return cmp.Or(cmp.Compare(scoreOf(b), scoreOf(a)), cmp.Compare(a.ID, b.ID))
	})
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
	if v.Severity.Unknown() {
		return -1
	}
	return v.Severity.Score
}
