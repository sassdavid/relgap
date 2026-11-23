package enrich

import (
	"context"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// maxFixRounds is how many candidate querybatches the fix target search sends. A candidate with
// a fixable record of its own moves the search on, and the next pick needs another round.
const maxFixRounds = 3

// Fix is the release a tool version can move to: the lowest stable release above it that every
// advisory with a fix has left behind and that has no fixable advisory of its own.
type Fix struct {
	Lowest       string `json:"lowest,omitempty"`         // absent when no release qualifies
	LowestInLine string `json:"lowest_in_line,omitempty"` // the same within the current version's line
	Latest       string `json:"latest"`                   // the newest stable release
	// LatestAffected says whether OSV lists an advisory for Latest, named in LatestAffectedBy.
	LatestAffected   bool     `json:"latest_affected"`
	LatestAffectedBy []string `json:"latest_affected_by,omitempty"`
	// Unfixed lists the advisories of the current version that Latest still has: no release fixes
	// them.
	Unfixed []string `json:"unfixed,omitempty"`
}

// VersionLister returns a tool's released versions.
type VersionLister func(ctx context.Context) (*version.Universe, error)

// evidence is one record that describes an advisory for one of the request's packages.
type evidence struct {
	rec *Record
	pkg registry.OSVPackage
}

// advisory is an advisory the fix target search knows: one of the current version's merged
// vulnerabilities, or a record a candidate has of its own.
type advisory struct {
	id      string
	names   []string // the ID and every alias
	sources []evidence
	own     bool // found at a candidate, not at the current version
}

// fixSearch is the fix target search of one tool version.
type fixSearch struct {
	req      Request
	modules  map[string]string // the module each go: path resolved to
	scheme   version.Scheme
	level    version.Level
	cands    []version.V // releases above the current version, oldest first
	latest   version.V
	advs     []*advisory
	local    []evidence       // every record of the versionless packages, matched to candidates here
	observed map[string][]int // advisories OSV lists for a version it was asked about
	settled  map[string]bool  // versions the local records were matched to
	latestOK bool             // OSV answered for latest
	failed   bool
}

// fixRef ties a candidate query to its search and version.
type fixRef struct {
	search *fixSearch
	v      version.V
}

// fixTargets returns the fix target of every request with a vulnerability of its current
// version, by key, from OSV's answer about the current versions.
func (e *Enricher) fixTargets(ctx context.Context, reqs []Request, ans osvAnswer) map[string]*Fix {
	var searches []*fixSearch
	for i, r := range reqs {
		affected := slices.ContainsFunc(ans.vulns[r.Key], func(v Vuln) bool { return v.AffectsCurrent })
		if r.Versions == nil || !affected {
			continue
		}
		searches = append(searches, &fixSearch{
			req: r, modules: ans.modules, advs: advisoriesOf(ans.vulns[r.Key], ans.sources[i]), local: ans.local[i],
		})
	}
	out := map[string]*Fix{}
	if len(searches) == 0 {
		return out
	}
	e.loadVersions(ctx, searches)
	for range maxFixRounds {
		for _, s := range searches {
			s.settle()
		}
		if !e.fixRound(ctx, searches) {
			break
		}
	}
	for _, s := range searches {
		if f := s.fix(); f != nil {
			out[s.req.Key] = f
		}
	}
	return out
}

// advisoriesOf lists the vulnerabilities of the current version with the records behind them.
func advisoriesOf(vs []Vuln, srcs []evidence) []*advisory {
	var out []*advisory
	for _, v := range vs {
		if !v.AffectsCurrent {
			continue
		}
		a := &advisory{id: v.ID, names: append([]string{v.ID}, v.Aliases...)}
		for _, s := range srcs {
			if slices.Contains(a.names, s.rec.ID) {
				a.sources = append(a.sources, s)
			}
		}
		out = append(out, a)
	}
	return out
}

// loadVersions lists each search's tool versions at the same time. A search whose list fails
// gets no fix target.
func (e *Enricher) loadVersions(ctx context.Context, searches []*fixSearch) {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(e.concurrency)
	for _, s := range searches {
		g.Go(func() error {
			u, err := s.req.Versions(gctx)
			if err != nil {
				e.logger.WarnContext(gctx, "can't list versions for the fix target", "tool", s.req.Tool.Name,
					"error", err)
				s.failed = true
				return nil
			}
			s.init(u)
			return nil
		})
	}
	_ = g.Wait() // each search records its own failure
}

// init keeps the stable releases above the current version as candidates, and pre-releases too
// when the current version is one.
func (s *fixSearch) init(u *version.Universe) {
	s.scheme = u.Scheme()
	s.level = version.Level(s.req.Tool.Version.Line)
	if s.level != version.LineMinor {
		s.level = version.LineMajor
	}
	s.observed = map[string][]int{}
	s.settled = map[string]bool{}
	pre := s.scheme.IsPrerelease(s.req.Current)
	for _, v := range u.Versions() {
		if !v.Prerelease {
			s.latest = v.V
		}
		// A release without an OSV package, such as a major the curated list doesn't name, can't
		// be checked.
		if s.scheme.Compare(v.V, s.req.Current) > 0 && (pre || !v.Prerelease) && len(s.packagesAt(v.V)) > 0 {
			s.cands = append(s.cands, v.V)
		}
	}
}

// fixBatch is one candidate querybatch: its queries, and the searches and versions behind each.
type fixBatch struct {
	queries []Query
	refs    [][]fixRef
}

// fixRound asks OSV about the versions each search still has to check, in one querybatch, and
// reports whether it asked anything.
func (e *Enricher) fixRound(ctx context.Context, searches []*fixSearch) bool {
	b := pendingBatch(searches)
	if len(b.queries) == 0 {
		return false
	}
	hits, err := e.osv.QueryBatch(ctx, b.queries)
	if err == nil {
		var records map[string]*Record
		if records, err = e.fetch(ctx, b.unknown(hits), false); err == nil {
			b.apply(hits, records)
			return true
		}
	}
	e.logger.WarnContext(ctx, "OSV failed for the fix targets", "error", err)
	for _, rs := range b.refs {
		for _, r := range rs {
			r.search.failed = true
		}
	}
	return false
}

// settle matches the local records to the versions the search still has to check whose packages
// are all versionless, until no such version is left. OSV's answer for such a version would be
// empty, since it doesn't evaluate their ranges, and the versionless answer already holds every
// record of the package.
func (s *fixSearch) settle() {
	for {
		var next []version.V
		for _, v := range s.pending() {
			if !slices.ContainsFunc(s.packagesAt(v), func(p registry.OSVPackage) bool { return !versionless(p) }) {
				next = append(next, v)
			}
		}
		if len(next) == 0 {
			return
		}
		for _, v := range next {
			s.matchLocal(v)
			s.asked(v)
		}
	}
}

// matchLocal records the local records whose ranges hold version v, once per version.
func (s *fixSearch) matchLocal(v version.V) {
	if s.settled[v.String()] {
		return
	}
	s.settled[v.String()] = true
	for _, ev := range s.local {
		pkg := Package{Ecosystem: ev.pkg.Ecosystem, Name: ev.pkg.Name}
		if affected, known := Affects(ev.rec, pkg, QueryVersion(ev.pkg, s.req.tag(v), v)); affected && known {
			s.observe(v, ev.rec, ev.pkg)
		}
	}
}

// pendingBatch lists one query per package and version the searches still have to check. A
// versionless package is matched locally instead.
func pendingBatch(searches []*fixSearch) fixBatch {
	var b fixBatch
	index := map[Query]int{}
	for _, s := range searches {
		for _, v := range s.pending() {
			for _, p := range s.packagesAt(v) {
				if versionless(p) {
					continue
				}
				q := Query{
					Package: Package{Ecosystem: p.Ecosystem, Name: p.Name},
					Version: QueryVersion(p, s.req.tag(v), v),
				}
				at, ok := index[q]
				if !ok {
					at = len(b.queries)
					index[q] = at
					b.queries = append(b.queries, q)
					b.refs = append(b.refs, nil)
				}
				b.refs[at] = append(b.refs[at], fixRef{search: s, v: v})
			}
		}
	}
	return b
}

// unknown returns the hits a search behind them doesn't know yet. Only those records are
// fetched; a known ID needs no second request.
func (b fixBatch) unknown(hits [][]Hit) []Hit {
	var out []Hit
	for qi, rs := range b.refs {
		for _, h := range hits[qi] {
			if slices.ContainsFunc(rs, func(r fixRef) bool { return r.search.known(h.ID) < 0 }) {
				out = append(out, h)
			}
		}
	}
	return out
}

// apply records OSV's answers in each search, with records holding the newly fetched ones.
func (b fixBatch) apply(hits [][]Hit, records map[string]*Record) {
	for qi, rs := range b.refs {
		for _, r := range rs {
			r.search.asked(r.v)
			r.search.matchLocal(r.v)
			for _, h := range hits[qi] {
				if at := r.search.known(h.ID); at >= 0 {
					r.search.mark(r.v, at)
				} else if rec := records[h.ID]; rec != nil {
					r.search.observe(r.v, rec, packageOf(r.search.packagesAt(r.v), b.queries[qi].Package))
				}
			}
		}
	}
}

// asked records that OSV answered for version v, with or without advisories.
func (s *fixSearch) asked(v version.V) {
	if _, ok := s.observed[v.String()]; !ok {
		s.observed[v.String()] = []int{}
	}
	if v.String() == s.latest.String() {
		s.latestOK = true
	}
}

// packagesAt returns the OSV packages of version v, which can differ from the current version's,
// such as the Go module of another major.
func (s *fixSearch) packagesAt(v version.V) []registry.OSVPackage {
	c := s.req.coverageAt(v)
	pkgs := c.Packages
	if m := s.modules[c.GoPath]; m != "" {
		pkgs = append(slices.Clip(pkgs), registry.OSVPackage{Ecosystem: EcosystemGo, Name: m})
	}
	return pkgs
}

func packageOf(pkgs []registry.OSVPackage, p Package) registry.OSVPackage {
	for _, rp := range pkgs {
		if rp.Ecosystem == p.Ecosystem && rp.Name == p.Name {
			return rp
		}
	}
	return registry.OSVPackage{Ecosystem: p.Ecosystem, Name: p.Name}
}

// known returns the index of the advisory with id among its names, or -1.
func (s *fixSearch) known(id string) int {
	return slices.IndexFunc(s.advs, func(a *advisory) bool { return slices.Contains(a.names, id) })
}

// observe records that OSV lists rec for version v, adding rec as the version's own advisory
// when no known advisory has its ID or an alias.
func (s *fixSearch) observe(v version.V, rec *Record, p registry.OSVPackage) {
	names := append([]string{rec.ID}, rec.Aliases...)
	at := slices.IndexFunc(s.advs, func(a *advisory) bool {
		return slices.ContainsFunc(names, func(n string) bool { return slices.Contains(a.names, n) })
	})
	if at < 0 {
		at = len(s.advs)
		s.advs = append(s.advs, &advisory{id: rec.ID, names: names, own: true})
	}
	a := s.advs[at]
	if !slices.ContainsFunc(a.sources, func(src evidence) bool { return src.rec.ID == rec.ID && src.pkg == p }) {
		a.sources = append(a.sources, evidence{rec: rec, pkg: p})
	}
	s.mark(v, at)
}

// mark records that OSV lists advisory at for version v.
func (s *fixSearch) mark(v version.V, at int) {
	key := v.String()
	if !slices.Contains(s.observed[key], at) {
		s.observed[key] = append(s.observed[key], at)
	}
}

// pending returns the versions OSV still has to check: latest, then the current picks.
func (s *fixSearch) pending() []version.V {
	if s.failed || len(s.cands) == 0 {
		return nil
	}
	var out []version.V
	add := func(v version.V) {
		if _, done := s.observed[v.String()]; !v.IsZero() && !done &&
			!slices.ContainsFunc(out, func(o version.V) bool { return o.String() == v.String() }) {
			out = append(out, v)
		}
	}
	add(s.latest)
	if s.fixable() {
		add(s.pick(false))
		add(s.pick(true))
	}
	return out
}

// fixable reports whether the current version has an advisory that latest doesn't keep. Without
// one, moving to another release fixes nothing.
func (s *fixSearch) fixable() bool {
	for i, a := range s.advs {
		if !a.own && !s.unfixed(i) {
			return true
		}
	}
	return false
}

// unfixed reports whether advisory a stays at latest: OSV lists it there, or, before OSV
// answered, the records say so.
func (s *fixSearch) unfixed(a int) bool {
	if s.latestOK {
		return slices.Contains(s.observed[s.latest.String()], a)
	}
	affected, known := s.affects(s.advs[a], s.latest)
	return affected && known
}

// pick returns the lowest candidate, within the current version's line when inLine is set, that
// no fixable advisory affects as far as the records and OSV's answers tell, or the zero version.
func (s *fixSearch) pick(inLine bool) version.V {
	line := s.scheme.Line(s.req.Current, s.level)
	for _, c := range s.cands {
		if inLine && s.scheme.Line(c, s.level) != line {
			continue
		}
		if s.clean(c) {
			return c
		}
	}
	return version.V{}
}

// clean reports whether no fixable advisory affects c. An advisory whose records can't place c
// counts as clean until OSV answers for c.
func (s *fixSearch) clean(c version.V) bool {
	if hit, ok := s.observed[c.String()]; ok {
		return !slices.ContainsFunc(hit, func(a int) bool { return !s.unfixed(a) })
	}
	for i, a := range s.advs {
		if s.unfixed(i) {
			continue
		}
		if affected, known := s.affects(a, c); affected && known {
			return false
		}
	}
	return true
}

// affects reports whether a's records list v as affected; the second result is false when no
// record says either way.
func (s *fixSearch) affects(a *advisory, v version.V) (bool, bool) {
	known := false
	for _, src := range a.sources {
		pkg := Package{Ecosystem: src.pkg.Ecosystem, Name: src.pkg.Name}
		affected, ok := Affects(src.rec, pkg, QueryVersion(src.pkg, s.req.tag(v), v))
		if affected {
			return true, true
		}
		known = known || ok
	}
	return false, known
}

// fix returns the search's result, or nil when its version list or OSV failed. A pick counts
// only after OSV confirmed it.
func (s *fixSearch) fix() *Fix {
	if s.failed || s.observed == nil {
		return nil
	}
	if len(s.cands) == 0 {
		// The current version is the newest release, so nothing fixes its advisories yet.
		ids := s.ids(func(a *advisory) bool { return !a.own })
		return &Fix{Latest: s.req.Current.String(), LatestAffected: true, LatestAffectedBy: ids, Unfixed: ids}
	}
	if !s.latestOK {
		return nil
	}
	f := &Fix{Latest: s.latest.String()}
	confirmed := func(v version.V) string {
		if _, ok := s.observed[v.String()]; ok && !v.IsZero() && s.clean(v) {
			return v.String()
		}
		return ""
	}
	atLatest := s.observed[s.latest.String()]
	if s.fixable() {
		f.Lowest, f.LowestInLine = confirmed(s.pick(false)), confirmed(s.pick(true))
	}
	f.LatestAffectedBy = s.ids(func(a *advisory) bool { return slices.Contains(atLatest, slices.Index(s.advs, a)) })
	f.LatestAffected = len(f.LatestAffectedBy) > 0
	f.Unfixed = s.ids(func(a *advisory) bool {
		return !a.own && slices.Contains(atLatest, slices.Index(s.advs, a))
	})
	return f
}

// ids returns the sorted IDs of the advisories keep accepts, or nil.
func (s *fixSearch) ids(keep func(*advisory) bool) []string {
	var out []string
	for _, a := range s.advs {
		if keep(a) {
			out = append(out, a.id)
		}
	}
	slices.SortFunc(out, strings.Compare)
	return out
}
