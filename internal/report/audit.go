package report

import (
	"cmp"
	"context"
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
	Repos     []string       `json:"repos"`
	Checked   int            `json:"checked"` // tool versions OSV answered for
	Findings  []AuditFinding `json:"findings"`
	Unchecked []Unchecked    `json:"unchecked"`
	// OldestAnswer is when the oldest OSV answer of an --offline run was stored in the cache.
	OldestAnswer *time.Time `json:"oldest_answer,omitempty"`

	rows *Outdated // each pin with the version it stands for; Latest is never looked up
}

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
	URL      string          `json:"url"`
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
		Repos:     append([]string{}, req.Repos...),
		Findings:  []AuditFinding{},
		Unchecked: []Unchecked{},
		rows:      &Outdated{Repos: req.Repos, Pins: rows},
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
func (a *Audit) EnrichRequests(resolve ResolveFunc) []enrich.Request {
	return a.rows.EnrichRequests(resolve)
}

// ApplyEnrichment lists the unchecked pins, adding unresolved, the pins that resolve to no tool,
// and turns each advisory that affects a checked version into a finding. Findings sort by
// severity, rated before unrated, then by ID, tool and version.
func (a *Audit) ApplyEnrichment(results map[string]enrich.Result, unresolved []scan.Pin) {
	a.rows.ApplyEnrichment(results, unresolved)
	a.Unchecked = a.rows.Unchecked
	checked := map[string]bool{}
	index := map[string]int{}
	findings := []AuditFinding{}
	for i := range a.rows.Pins {
		p := &a.rows.Pins[i]
		if p.Unchecked != "" || p.Vulns == nil {
			continue
		}
		checked[enrichKey(p)] = true
		for _, v := range p.Vulns {
			if !v.AffectsCurrent {
				continue
			}
			key := enrichKey(p) + "\x00" + v.ID
			at, ok := index[key]
			if !ok {
				at = len(findings)
				index[key] = at
				findings = append(findings, AuditFinding{
					Tool: p.Tool, Version: p.vulnVersion, ID: v.ID, Aliases: v.Aliases, Summary: v.Summary,
					Package: v.Package, Severity: v.Severity, FixedIn: v.FixedIn, URL: AdvisoryURL + v.ID,
				})
			}
			findings[at].Pins = append(findings[at].Pins, p.Pin.Location)
		}
	}
	slices.SortStableFunc(findings, compareFindings)
	a.Findings, a.Checked = findings, len(checked)
}

func compareFindings(x, y AuditFinding) int {
	return cmp.Or(
		cmp.Compare(severityKey(y.Severity), severityKey(x.Severity)),
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
