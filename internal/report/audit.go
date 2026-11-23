package report

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// AdvisoryURL is the osv.dev page of an advisory ID.
const AdvisoryURL = "https://osv.dev/vulnerability/"

// Audit is the data of `relgap audit --format json`.
type Audit struct {
	Repos    []string       `json:"repos"`
	Checked  int            `json:"checked"` // tool versions OSV answered for
	Findings []AuditFinding `json:"findings"`
	// Suppressed are the findings a vulns.ignore entry or an OpenVEX statement suppresses. They
	// don't count for --fail-on.
	Suppressed []AuditFinding `json:"suppressed"`
	Unchecked  []Unchecked    `json:"unchecked"`
	// OldestAnswer is when the oldest OSV answer of an --offline run was stored in the cache.
	OldestAnswer *time.Time `json:"oldest_answer,omitempty"`

	rows  *Outdated // each pin with the version it stands for; Latest is never looked up
	fixes []ToolFix
	feeds Feeds
}

// Feeds says which prioritization feeds a run asked: CISA KEV and FIRST EPSS.
type Feeds struct {
	KEV, EPSS bool
}

// SetFeeds records which feeds the run asked, so the text formats show their columns.
func (a *Audit) SetFeeds(f Feeds) { a.feeds = f }

// Feeds returns which prioritization feeds the run asked.
func (a *Audit) Feeds() Feeds { return a.feeds }

// ToolFix is the fix target of one tool version with findings.
type ToolFix struct {
	Tool    string
	Version string
	Fix     *enrich.Fix
}

// Fixes returns the fix target of each tool version with findings, in the order of their first
// finding. A tool version whose target relgap couldn't work out is left out.
func (a *Audit) Fixes() []ToolFix { return a.fixes }

// AuditFinding is one advisory that affects one tool version, with every pin of that version.
type AuditFinding struct {
	Tool     string          `json:"tool"`
	Version  string          `json:"version"`
	Pins     []scan.Location `json:"pins"`
	ID       string          `json:"id"`
	Aliases  []string        `json:"aliases,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Package  enrich.Package  `json:"package"`
	Severity *enrich.Score   `json:"severity,omitempty"`
	FixedIn  string          `json:"fixed_in,omitempty"` // lowest fixed version above Version
	Fix      FindingFix      `json:"fix,omitzero"`
	URL      string          `json:"url"`
	// Suppressed says why the finding doesn't count; only findings in [Audit.Suppressed] have it.
	Suppressed *enrich.Suppression `json:"suppressed,omitempty"`
	KEV        *enrich.KEV         `json:"kev,omitempty"`  // with --kev, when a CVE alias is in the catalog
	EPSS       *enrich.EPSS        `json:"epss,omitempty"` // with --epss, when EPSS scores a CVE alias

	purl string
}

// PURL returns the package URL of the finding's package at its version, or "" when the
// ecosystem has no purl type.
func (f *AuditFinding) PURL() string { return f.purl }

// FindingFix is a finding's fix target: the tool version's [enrich.Fix], null when no release
// fixes the advisory, and absent when relgap couldn't work it out.
type FindingFix struct {
	Target *enrich.Fix // nil when no release fixes the advisory
	Known  bool
}

// IsZero reports whether relgap couldn't work the fix target out, which leaves it out of JSON.
func (f FindingFix) IsZero() bool { return !f.Known }

// MarshalJSON writes the target, or null.
func (f FindingFix) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Target) //nolint:wrapcheck // a plain struct always marshals
}

// Failed returns the pins whose version lookup failed, which leaves them unchecked.
func (a *Audit) Failed() []*OutdatedPin {
	var out []*OutdatedPin
	for i := range a.rows.Pins {
		if p := &a.rows.Pins[i]; p.Status == StatusError {
			out = append(out, p)
		}
	}
	return out
}

// AuditRequest says which pins to audit.
type AuditRequest struct {
	Repos       []string
	Pins        []scan.Pin // resolved pins only
	Resolve     ResolveFunc
	Concurrency int
}

// Auditor finds the version each pin stands for, the CURRENT of `relgap outdated`, without
// looking up latest versions or digests.
type Auditor struct {
	digester *Digester
	recent   RecentLister
}

// NewAuditor returns an auditor that lists a tool's versions through digester, and its newest
// releases through recent, only for pins that aren't exact.
func NewAuditor(digester *Digester, recent RecentLister) *Auditor {
	return &Auditor{digester: digester, recent: recent}
}

// Audit returns the pins with their versions, ready for [Audit.EnrichRequests]. An exact pin is
// its own version and costs no request, and an image or chart pin is unchecked without a lookup
// (see [enrich.NameReason]). It returns an error only for a failure that affects every tool,
// such as a rejected token.
func (a *Auditor) Audit(ctx context.Context, req AuditRequest) (*Audit, error) {
	rows := make([]OutdatedPin, len(req.Pins))
	for i, p := range req.Pins {
		if reason, ok := enrich.NameReason(p.Tool); ok {
			rows[i] = OutdatedPin{Tool: p.Tool, Pin: p, Status: StatusSkipped, Unchecked: reason}
		}
	}
	oreq := OutdatedRequest{Pins: req.Pins, Resolve: req.Resolve}
	tools := groupToolsOf(oreq, func(i int) bool { return rows[i].Unchecked == "" })
	recent, err := a.latestWindows(ctx, req.Pins, tools)
	if err != nil {
		return nil, err
	}
	auth := &authFailure{}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(max(req.Concurrency, 1))
	for _, t := range tools {
		g.Go(func() error {
			for _, i := range t.pins {
				rows[i] = a.version(gctx, oreq, t, recent[recentQuery(t.spec)], i)
				if rows[i].failure != nil {
					auth.record(rows[i].failure)
				}
			}
			return nil
		})
	}
	_ = g.Wait() // version records errors per pin
	if err = ctx.Err(); err != nil {
		return nil, fmt.Errorf("audit pins: %w", err)
	}
	if err = auth.get(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(x, y OutdatedPin) int {
		return cmp.Or(registry.CompareTools(x.Tool, y.Tool), compareLocations(x.Pin.Location, y.Pin.Location))
	})
	return &Audit{
		Repos:      append([]string{}, req.Repos...),
		Findings:   []AuditFinding{},
		Suppressed: []AuditFinding{},
		Unchecked:  []Unchecked{},
		rows:       &Outdated{Repos: req.Repos, Pins: rows},
	}, nil
}

// latestWindows asks GitHub, in one batch, for the newest releases of the tools with a pin on the
// latest channel.
func (a *Auditor) latestWindows(
	ctx context.Context,
	pins []scan.Pin,
	tools []*toolPins,
) (map[github.RecentQuery]github.Recent, error) {
	var queries []github.RecentQuery
	for _, t := range tools {
		if t.err != nil || !fromRecent(t.spec) {
			continue
		}
		if slices.ContainsFunc(t.pins, func(i int) bool { return latestChannel(pins[i]) }) {
			queries = append(queries, recentQuery(t.spec))
		}
	}
	if len(queries) == 0 {
		return map[github.RecentQuery]github.Recent{}, nil
	}
	recent, err := a.recent.Recent(ctx, queries)
	if err != nil {
		return nil, err //nolint:wrapcheck // Recent names what it listed
	}
	for _, r := range recent {
		if errors.Is(r.Err, github.ErrAuth) {
			return nil, r.Err
		}
	}
	return recent, nil
}

func latestChannel(p scan.Pin) bool {
	return p.Match == scan.MatchChannel && strings.EqualFold(strings.TrimSpace(p.Value), "latest")
}

// version returns pin i with the version it stands for in Current, or with why it has none.
func (a *Auditor) version(
	ctx context.Context,
	req OutdatedRequest,
	t *toolPins,
	recent github.Recent,
	i int,
) OutdatedPin {
	pin := req.Pins[i]
	row := OutdatedPin{Tool: pin.Tool, Repo: t.spec.Repo, Host: t.spec.Host, Pin: pin, Status: StatusCurrent}
	if t.err != nil {
		row.Status, row.Note, row.failure = skipStatus(t.err), t.err.Error(), t.err
		return row
	}
	fail := func(err error) OutdatedPin {
		row.Status, row.Note, row.failure = statusOf(err), err.Error(), err
		return row
	}
	rules, err := t.spec.Version.Rules()
	if err != nil {
		return fail(fmt.Errorf("%s: %w", t.spec.Name, err))
	}
	var (
		v   version.V
		via string
	)
	if pin.Match == scan.MatchExact {
		v, via, err = exactVersion(version.NewUniverse(ctx, a.digester.logger, nil, rules), pin)
	} else {
		v, via, err = approxVersion(ctx, pin, version.Level(t.spec.Version.Line), approxSource{
			window: func(ctx context.Context) (*version.Universe, error) {
				if !fromRecent(t.spec) {
					return a.digester.Universe(ctx, t.spec)
				}
				if recent.Err != nil {
					return nil, recent.Err
				}
				return version.NewUniverse(ctx, a.digester.logger, recentRefs(recent.Releases), rules), nil
			},
			full:  func(ctx context.Context) (*version.Universe, error) { return a.digester.Universe(ctx, t.spec) },
			known: func(u *version.Universe) []version.V { return knownVersions(u, req.Pins, t.pins, pin) },
		})
	}
	if err != nil {
		return fail(err)
	}
	row.Current, row.Via = v.String(), via
	return row
}

// EnrichRequests returns one request per tool and version among the pins; see
// [Outdated.EnrichRequests].
func (a *Audit) EnrichRequests(resolve ResolveFunc, versions VersionsFunc) []enrich.Request {
	return a.rows.EnrichRequests(resolve, versions)
}

// ApplyEnrichment lists the unchecked pins, adding unresolved, the pins that resolve to no tool,
// and turns each advisory that affects a checked version into a finding, or into a suppressed
// one. Both lists sort KEV entries first, then by severity, rated before unrated, then by EPSS
// score, then by ID, tool and version.
func (a *Audit) ApplyEnrichment(results map[string]enrich.Result, unresolved []scan.Pin) {
	a.rows.ApplyEnrichment(results, unresolved)
	a.Unchecked = a.rows.Unchecked
	checked := map[string]bool{}
	fixes := map[string]*enrich.Fix{}
	for i := range a.rows.Pins {
		if p := &a.rows.Pins[i]; p.Unchecked == "" && p.Vulns != nil {
			checked[enrichKey(p)] = true
			fixes[enrichKey(p)] = p.Fix
		}
	}
	findings, suppressed := []AuditFinding{}, []AuditFinding{}
	for _, f := range a.collectFindings() {
		if f.Suppressed != nil {
			suppressed = append(suppressed, *f)
		} else {
			findings = append(findings, *f)
		}
	}
	slices.SortStableFunc(findings, compareFindings)
	slices.SortStableFunc(suppressed, compareFindings)
	a.Findings, a.Suppressed, a.Checked = findings, suppressed, len(checked)
	a.fixes = nil
	seen := map[string]bool{}
	for _, f := range findings {
		key := f.Tool + "\x00" + f.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		if fix := fixes[key]; fix != nil {
			a.fixes = append(a.fixes, ToolFix{Tool: f.Tool, Version: f.Version, Fix: fix})
		}
	}
}

// collectFindings turns each advisory that affects a checked version into a finding with every
// pin of that version, in the order of the pins.
func (a *Audit) collectFindings() []*AuditFinding {
	index := map[string]*AuditFinding{}
	var all []*AuditFinding
	for i := range a.rows.Pins {
		p := &a.rows.Pins[i]
		if p.Unchecked != "" {
			continue
		}
		for _, v := range p.Vulns {
			if !v.AffectsCurrent {
				continue
			}
			key := enrichKey(p) + "\x00" + v.ID
			f, ok := index[key]
			if !ok {
				f = &AuditFinding{
					Tool: p.Tool, Version: p.vulnVersion, ID: v.ID, Aliases: v.Aliases, Summary: v.Summary,
					Package: v.Package, Severity: v.Severity, FixedIn: v.FixedIn, URL: AdvisoryURL + v.ID,
					Suppressed: v.Suppressed, KEV: v.KEV, EPSS: v.EPSS, purl: v.PURL(),
				}
				if v.Suppressed == nil {
					f.Fix = findingFix(p.Fix, v.ID)
				}
				index[key] = f
				all = append(all, f)
			}
			f.Pins = append(f.Pins, p.Pin.Location)
		}
	}
	return all
}

// findingFix is the fix target of advisory id: f, or null when f says no release fixes id.
func findingFix(f *enrich.Fix, id string) FindingFix {
	if f == nil {
		return FindingFix{}
	}
	if slices.Contains(f.Unfixed, id) {
		return FindingFix{Known: true}
	}
	return FindingFix{Target: f, Known: true}
}

func compareFindings(x, y AuditFinding) int {
	return cmp.Or(
		enrich.KEVFirst(x.KEV, y.KEV),
		cmp.Compare(severityKey(y.Severity), severityKey(x.Severity)),
		cmp.Compare(enrich.EPSSScore(y.EPSS), enrich.EPSSScore(x.EPSS)),
		cmp.Compare(x.ID, y.ID),
		registry.CompareTools(x.Tool, y.Tool),
		cmp.Compare(x.Version, y.Version),
	)
}

// severityKey orders scores: unknown below every rating, then by score, then by rating for an
// advisory that has a rating without a score.
func severityKey(s *enrich.Score) float64 {
	if s.Unknown() {
		return -1
	}
	const ratingWeight = 0.01
	return s.Score + ratingWeight*float64(severityRank(s.Rating))
}
