package report

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// Status says how a pin compares with the latest version.
type Status string

// Pin statuses.
const (
	StatusCurrent  Status = "current"  // at the latest version on its line
	StatusOutdated Status = "outdated" // behind it
	StatusUnknown  Status = "unknown"  // a channel other than latest, or a value that matches no version
	StatusSkipped  Status = "skipped"  // the tool has no repository or registry relgap can read
	StatusError    Status = "error"    // the host failed for this tool
)

// Ways a non-exact pin becomes its current version, next to [version.PickKnown] and
// [version.PickLowest].
const (
	ViaPrefix = "highest stable version with this prefix"
	ViaLatest = "latest channel"
)

// Outdated is the data of `relgap outdated --format json`.
type Outdated struct {
	Repos     []string         `json:"repos"`
	Pins      []OutdatedPin    `json:"pins"`
	Unchecked []Unchecked      `json:"unchecked,omitzero"` // after enrichment; empty when OSV checked every pin
	Binaries  []*enrich.Binary `json:"binaries,omitzero"`  // with --binaries; empty when none found
}

// OutdatedPin compares one pin with the latest version of its tool.
type OutdatedPin struct {
	Tool    string                `json:"tool"`
	Repo    string                `json:"repo,omitempty"` // GitHub owner/name, or the project path on Host
	Host    string                `json:"host,omitempty"` // the host of Repo when it isn't github.com
	Pin     scan.Pin              `json:"pin"`
	Current string                `json:"current,omitempty"` // the version the pin stands for
	Via     string                `json:"via,omitempty"`     // how a non-exact pin became Current
	Latest  string                `json:"latest,omitempty"`  // newest stable version on the tool's line
	Behind  *Behind               `json:"behind,omitempty"`
	Counts  map[classify.Kind]int `json:"counts,omitempty"` // entries of the digest Current..Latest
	Hidden  int                   `json:"hidden,omitempty"` // entries below the minimum confidence
	Status  Status                `json:"status"`
	Note    string                `json:"note,omitempty"`
	EOL     *enrich.EOL           `json:"eol,omitempty"` // absent without an endoflife.date product
	// Vulns are the known vulnerabilities of Current, empty when OSV has none. Absent when OSV
	// wasn't asked (see Unchecked) or enrichment didn't run.
	Vulns     []enrich.Vuln `json:"vulns,omitzero"`
	Unchecked string        `json:"unchecked,omitempty"` // an enrich.Reason* constant saying why OSV wasn't asked
	// Fix is the release Current can move to. Absent without a vulnerability of Current, or when
	// the version list or OSV failed.
	Fix *enrich.Fix `json:"fix,omitempty"`

	// Digest is the digest Counts come from; pins with the same tool and Current share it.
	Digest *ToolDigest `json:"-"`

	failure     error  // why the lookup failed, for a skipped or failed pin
	vulnVersion string // the version OSV is asked about
}

// Behind measures how far Current is from Latest. Every field comes from the digest's versions.
type Behind struct {
	Releases int        `json:"releases"`        // stable versions after Current up to Latest
	Major    int        `json:"major"`           // newer major lines among them
	Minor    int        `json:"minor"`           // newer minor lines among them
	Patch    int        `json:"patch"`           // those on Current's own minor line
	Since    *time.Time `json:"since,omitempty"` // when the first of them was published
}

// RecentLister lists the newest releases of many repositories at once; [github.Client]
// implements it.
type RecentLister interface {
	Recent(ctx context.Context, qs []github.RecentQuery) (map[github.RecentQuery]github.Recent, error)
}

// ResolveFunc turns a pin's tool name into a tool spec.
type ResolveFunc func(tool string) (registry.ToolSpec, error)

// OutdatedRequest says which pins to compare.
type OutdatedRequest struct {
	Repos         []string
	Pins          []scan.Pin // resolved pins only
	Resolve       ResolveFunc
	MinConfidence float64
	Concurrency   int
}

// Outdater compares pins with the latest versions of their tools.
type Outdater struct {
	digester *Digester
	recent   RecentLister
}

// NewOutdater returns an outdater that finds latest versions through recent and counts entries
// with digester, the same code `relgap digest` runs.
func NewOutdater(digester *Digester, recent RecentLister) *Outdater {
	return &Outdater{digester: digester, recent: recent}
}

// toolPins is one tool and its pins, in request order.
type toolPins struct {
	name string
	spec registry.ToolSpec
	err  error
	pins []int // indexes into the request's pins
}

// Outdated compares every pin. The "latest" lookup asks GitHub about all tools in batches; only
// pins behind latest and non-exact pins need a tool's full version list. It returns an error
// only for a failure that affects every tool, such as a rejected token.
func (o *Outdater) Outdated(ctx context.Context, req OutdatedRequest) (*Outdated, error) {
	out := &Outdated{Repos: append([]string{}, req.Repos...), Pins: make([]OutdatedPin, len(req.Pins))}
	tools := groupToolsOf(req, markPrivateRegistries(req.Pins, out.Pins))
	var queries []github.RecentQuery
	for _, t := range tools {
		if t.err == nil && fromRecent(t.spec) {
			queries = append(queries, recentQuery(t.spec))
		}
	}
	// Prefix and constraint pins always need the full version list, so it loads while the
	// batched lookup runs instead of after it. compare reads the same memoized list.
	var prefetch sync.WaitGroup
	defer prefetch.Wait()
	for _, t := range tools {
		if t.err == nil && t.needsFullList(req.Pins) {
			prefetch.Go(func() { _, _, _ = o.digester.list(ctx, t.spec) })
		}
	}
	recent, err := o.recent.Recent(ctx, queries)
	if err != nil {
		return nil, err //nolint:wrapcheck // Recent names what it listed
	}
	for _, r := range recent {
		if errors.Is(r.Err, github.ErrAuth) {
			return nil, r.Err
		}
	}
	digests := &digestSet{byKey: map[string]*digestOnce{}}
	auth := &authFailure{}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(max(req.Concurrency, 1))
	for _, t := range tools {
		g.Go(func() error {
			c := comparer{o: o, req: req, tool: t, recent: recent[recentQuery(t.spec)], digests: digests, auth: auth}
			for _, i := range t.pins {
				out.Pins[i] = c.compare(gctx, i)
			}
			return nil
		})
	}
	_ = g.Wait() // compare records errors per pin
	if err = ctx.Err(); err != nil {
		return nil, fmt.Errorf("compare pins: %w", err)
	}
	if err = auth.get(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(out.Pins, func(a, b OutdatedPin) int {
		return cmp.Or(registry.CompareTools(a.Tool, b.Tool), compareLocations(a.Pin.Location, b.Pin.Location))
	})
	return out, nil
}

// markPrivateRegistries fills the row of each pin whose package comes from a registry other than
// the public one, and returns the filter that leaves those pins out of every lookup.
func markPrivateRegistries(pins []scan.Pin, rows []OutdatedPin) func(int) bool {
	for i, p := range pins {
		if p.PrivateRegistry() {
			rows[i] = privateRegistryRow(p)
		}
	}
	return func(i int) bool { return !pins[i].PrivateRegistry() }
}

// privateRegistryRow is the row of a pin whose package comes from a registry other than the public
// one: no registry is asked about it, and OSV isn't either.
func privateRegistryRow(p scan.Pin) OutdatedPin {
	return OutdatedPin{
		Tool: p.Tool, Pin: p, Status: StatusSkipped, Unchecked: enrich.ReasonPrivate,
		Note: "from the registry " + p.Registry + ", which relgap doesn't read",
	}
}

// fromRecent reports whether the batched GitHub lookup knows t's newest versions: a tool on
// GitHub whose versions are its releases or tags.
func fromRecent(t registry.ToolSpec) bool {
	return t.Repo != "" && forge.IsGitHub(t.Host) && !t.Listed()
}

func recentQuery(t registry.ToolSpec) github.RecentQuery {
	return github.RecentQuery{Repo: t.Repo, Tags: t.Version.Universe == registry.UniverseTags}
}

// needsFullList reports whether a pin of t is a prefix or a constraint, which [comparer.current]
// resolves against every version of the tool.
func (t *toolPins) needsFullList(pins []scan.Pin) bool {
	for _, i := range t.pins {
		if m := pins[i].Match; m == scan.MatchPrefix || m == scan.MatchConstraint {
			return true
		}
	}
	return false
}

// groupToolsOf groups the pins that keep accepts (all when nil) by tool and resolves each tool.
func groupToolsOf(req OutdatedRequest, keep func(int) bool) []*toolPins {
	byName := map[string]*toolPins{}
	var tools []*toolPins
	for i, p := range req.Pins {
		if keep != nil && !keep(i) {
			continue
		}
		t, ok := byName[p.Tool]
		if !ok {
			t = &toolPins{name: p.Tool}
			t.spec, t.err = req.Resolve(p.Tool)
			byName[p.Tool] = t
			tools = append(tools, t)
		}
		t.pins = append(t.pins, i)
	}
	return tools
}

// authFailure keeps the first rejected-token error, which ends the whole command.
type authFailure struct {
	mu  sync.Mutex
	err error
}

func (a *authFailure) record(err error) {
	if !errors.Is(err, github.ErrAuth) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err == nil {
		a.err = err
	}
}

func (a *authFailure) get() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

// digestSet builds each (tool, from) digest once, however many pins share it. The key is the
// pins' tool, the full identity: two tools can resolve to specs with the same name.
type digestSet struct {
	mu    sync.Mutex
	byKey map[string]*digestOnce
}

type digestOnce struct {
	once sync.Once
	d    *ToolDigest
	err  error
}

func (s *digestSet) get(ctx context.Context, d *Digester, tool string, req DigestRequest) (*ToolDigest, error) {
	key := tool + "\x00" + req.From
	s.mu.Lock()
	e, ok := s.byKey[key]
	if !ok {
		e = &digestOnce{}
		s.byKey[key] = e
	}
	s.mu.Unlock()
	req.ID = tool
	e.once.Do(func() { e.d, e.err = d.Digest(ctx, req) })
	return e.d, e.err
}

// comparer compares the pins of one tool.
type comparer struct {
	o       *Outdater
	req     OutdatedRequest
	tool    *toolPins
	recent  github.Recent
	digests *digestSet
	auth    *authFailure

	window *version.Universe
}

// fail marks row as failed with err and returns it.
func (c *comparer) fail(row OutdatedPin, err error) OutdatedPin {
	c.auth.record(err)
	row.Status, row.Note, row.failure = statusOf(err), err.Error(), err
	return row
}

func (c *comparer) compare(ctx context.Context, i int) OutdatedPin {
	pin := c.req.Pins[i]
	row := OutdatedPin{Tool: pin.Tool, Repo: c.tool.spec.Repo, Host: c.tool.spec.Host, Pin: pin}
	if c.tool.err != nil {
		row.Status, row.Note, row.failure = skipStatus(c.tool.err), c.tool.err.Error(), c.tool.err
		return row
	}
	if c.recent.Err != nil {
		return c.fail(row, c.recent.Err)
	}
	t := c.tool.spec
	rules, err := t.Version.Rules()
	if err != nil {
		return c.fail(row, fmt.Errorf("%s: %w", t.Name, err))
	}
	if c.window == nil && !fromRecent(t) {
		// Every other host lists all versions in one go, so the window is the whole list.
		if c.window, err = c.o.digester.Universe(ctx, t); err != nil {
			return c.fail(row, err)
		}
	}
	if c.window == nil {
		c.window = version.NewUniverse(ctx, c.o.digester.logger, recentRefs(c.recent.Releases), rules)
	}
	line := version.Level(t.Version.Line)
	current, via, err := c.current(ctx, pin, line)
	if err != nil {
		return c.fail(row, err)
	}
	row.Current, row.Via = current.String(), via
	latest, ok := c.window.Latest(current, line)
	if !ok {
		full, uerr := c.o.digester.Universe(ctx, t)
		if uerr != nil {
			return c.fail(row, uerr)
		}
		if latest, ok = full.Latest(current, line); !ok {
			row.Status, row.Note = StatusUnknown, fmt.Sprintf("no stable version on the line of %s", current)
			return row
		}
	}
	switch order := rules.Scheme.Compare(current, latest.V); {
	case order > 0 && !c.window.Has(current):
		return c.untagged(ctx, row, current, line, latest)
	case order >= 0:
		return atLatest(row, latest)
	}
	d, err := c.digests.get(ctx, c.o.digester, c.tool.name, DigestRequest{
		Tool: t, From: current.String(), Line: line, MinConfidence: c.req.MinConfidence,
	})
	if err != nil {
		return c.fail(row, err)
	}
	row.Latest, row.Digest, row.Hidden = d.To, d, d.Hidden
	row.Counts = countsOf(d.Counts)
	switch {
	case t.Version.Universe == registry.UniverseChart:
		row.Counts, row.Note = nil, ChartVersionsNote(t)
	case t.Version.Universe == registry.UniverseImage && t.Notes == nil:
		row.Counts, row.Note = nil, ImageVersionsNote(t)
	}
	row.Behind = behind(rules.Scheme, current, d.Versions)
	row.Status = StatusOutdated
	if row.Behind.Releases == 0 {
		row.Status = StatusCurrent
	}
	return row
}

// atLatest marks row as current at latest.
func atLatest(row OutdatedPin, latest version.Version) OutdatedPin {
	row.Latest, row.Status, row.Behind, row.Counts = latest.V.String(), StatusCurrent, &Behind{}, zeroCounts()
	return row
}

// untagged compares a pin newer than the latest version when the window has no tag for it. A tag
// in the full list makes the pin current, such as a prerelease above the latest stable version.
// Without one the tool's tag rules most likely miss its newest tags, so the status is unknown.
func (c *comparer) untagged(
	ctx context.Context, row OutdatedPin, current version.V, line version.Level, latest version.Version,
) OutdatedPin {
	full := c.window
	if fromRecent(c.tool.spec) {
		var err error
		if full, err = c.o.digester.Universe(ctx, c.tool.spec); err != nil {
			return c.fail(row, err)
		}
		if l, ok := full.Latest(current, line); ok {
			latest = l
		}
	}
	if full.Has(current) {
		return atLatest(row, latest)
	}
	row.Latest, row.Status = latest.V.String(), StatusUnknown
	row.Note = fmt.Sprintf("%s is newer than the latest version found (%s) and has no tag; check the tag prefix",
		current, latest.V)
	return row
}

// current returns the version pin stands for and, for a non-exact pin, how it was chosen.
func (c *comparer) current(ctx context.Context, pin scan.Pin, line version.Level) (version.V, string, error) {
	if pin.Match == scan.MatchExact {
		return exactVersion(c.window, pin)
	}
	return approxVersion(ctx, pin, line, approxSource{
		window: func(context.Context) (*version.Universe, error) { return c.window, nil },
		full:   func(ctx context.Context) (*version.Universe, error) { return c.o.digester.Universe(ctx, c.tool.spec) },
		known:  func(u *version.Universe) []version.V { return knownVersions(u, c.req.Pins, c.tool.pins, pin) },
	})
}

// exactVersion parses an exact pin's version as u's tag rules allow.
func exactVersion(u *version.Universe, pin scan.Pin) (version.V, string, error) {
	v, err := u.Parse(pin.Version)
	if err != nil {
		return version.V{}, "", fmt.Errorf("%w: %w", errUncomparable, err)
	}
	return v, "", nil
}

// approxSource supplies the versions a non-exact pin resolves against.
type approxSource struct {
	window func(context.Context) (*version.Universe, error) // the newest releases, for the latest channel
	full   func(context.Context) (*version.Universe, error) // every version, for prefixes and constraints
	known  func(*version.Universe) []version.V              // the exact pins a constraint is locked to
}

// approxVersion returns the version a non-exact pin stands for and how it was chosen: the latest
// channel is the latest stable version, a prefix the highest stable version with it, and a
// constraint the highest known version that satisfies it, else the lowest version that does.
func approxVersion(ctx context.Context, pin scan.Pin, line version.Level, src approxSource) (version.V, string, error) {
	if pin.Match == scan.MatchChannel {
		if !strings.EqualFold(strings.TrimSpace(pin.Value), "latest") {
			return version.V{}, "", fmt.Errorf("%w: %q is a channel", errUncomparable, pin.Value)
		}
		window, err := src.window(ctx)
		if err != nil {
			return version.V{}, "", err
		}
		latest, ok := window.Latest(version.V{}, line)
		if !ok {
			return version.V{}, "", fmt.Errorf("%w: no stable version in the newest releases", errUncomparable)
		}
		return latest.V, ViaLatest, nil
	}
	full, err := src.full(ctx)
	if err != nil {
		return version.V{}, "", err
	}
	if pin.Match == scan.MatchPrefix {
		v, perr := full.ResolvePrefix(pin.Version)
		if perr != nil {
			return version.V{}, "", fmt.Errorf("%w: %w", errUncomparable, perr)
		}
		return v, ViaPrefix, nil
	}
	cons, err := version.ParseConstraint(full.Scheme(), pin.Value)
	if err != nil {
		return version.V{}, "", fmt.Errorf("%w: %w", errUncomparable, err)
	}
	v, pick, err := full.ResolveConstraint(cons, src.known(full))
	if err != nil {
		return version.V{}, "", fmt.Errorf("%w: %w", errUncomparable, err)
	}
	return v, string(pick), nil
}

// knownVersions returns the exact pins among pins[idx] in the same repository as pin, such as a
// .terraform-version next to required_version: the versions a constraint pin is locked to.
func knownVersions(u *version.Universe, pins []scan.Pin, idx []int, pin scan.Pin) []version.V {
	var out []version.V
	for _, i := range idx {
		p := pins[i]
		if p.Match != scan.MatchExact || p.Location.Repo != pin.Location.Repo {
			continue
		}
		if v, err := u.Parse(p.Version); err == nil {
			out = append(out, v)
		}
	}
	return out
}

var errUncomparable = errors.New("can't compare")

func statusOf(err error) Status {
	if errors.Is(err, errUncomparable) {
		return StatusUnknown
	}
	return StatusError
}

func skipStatus(err error) Status {
	if errors.Is(err, registry.ErrNoRepo) || errors.Is(err, registry.ErrUnsupported) ||
		errors.Is(err, registry.ErrUnknownTool) {
		return StatusSkipped
	}
	return StatusError
}

func recentRefs(rs []github.Release) []version.Ref {
	out := make([]version.Ref, len(rs))
	for i, r := range rs {
		out[i] = version.Ref{Tag: r.Tag, Draft: r.Draft, Prerelease: r.Prerelease, PublishedAt: r.PublishedAt}
	}
	return out
}

func behind(s version.Scheme, current version.V, vs []Version) *Behind {
	b := &Behind{Releases: len(vs)}
	majors, minors := map[string]bool{}, map[string]bool{}
	ownMajor, ownMinor := s.Line(current, version.LineMajor), s.Line(current, version.LineMinor)
	for _, v := range vs {
		parsed, err := s.Parse(v.Version)
		if err != nil {
			continue
		}
		if m := s.Line(parsed, version.LineMajor); m != ownMajor {
			majors[m] = true
		}
		if m := s.Line(parsed, version.LineMinor); m != ownMinor {
			minors[m] = true
		} else {
			b.Patch++
		}
		if b.Since == nil && v.PublishedAt != nil {
			at := *v.PublishedAt
			b.Since = &at
		}
	}
	b.Major, b.Minor = len(majors), len(minors)
	return b
}

func zeroCounts() map[classify.Kind]int {
	m := make(map[classify.Kind]int, len(classify.Kinds()))
	for _, k := range classify.Kinds() {
		m[k] = 0
	}
	return m
}

func countsOf(in map[classify.Kind]int) map[classify.Kind]int {
	m := zeroCounts()
	maps.Copy(m, in)
	return m
}

// Digests returns the distinct digests behind the outdated pins, in pin order.
func (o *Outdated) Digests() []ToolDigest {
	var out []ToolDigest
	seen := map[*ToolDigest]bool{}
	for _, p := range o.Pins {
		if p.Digest != nil && !seen[p.Digest] {
			seen[p.Digest] = true
			out = append(out, *p.Digest)
		}
	}
	return out
}
