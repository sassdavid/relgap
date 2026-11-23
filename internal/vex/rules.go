package vex

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/purl"
	"github.com/sassdavid/relgap/internal/scan"
)

// DateLayout is how a vulns.ignore entry writes its until date.
const DateLayout = time.DateOnly

// Ignore is a vulns.ignore entry: an advisory ID or alias, why it is accepted, the last day it
// applies and the tools it applies to.
type Ignore struct {
	ID     string
	Reason string
	Until  string   // a date in [DateLayout]; empty never expires
	Tools  []string // globs on the tool name, * spanning any characters; empty means every tool
}

// Rules suppresses advisories by vulns.ignore entries and OpenVEX statements. It is safe for
// concurrent use and counts which entries matched.
type Rules struct {
	mu         sync.Mutex
	ignores    []Ignore // the entries in effect
	used       []bool
	expired    []Ignore
	statements []statement // oldest first
}

// statement is an OpenVEX statement with the purls of its products and subcomponents.
type statement struct {
	names    []string
	products []purl.PURL
	status   string
	reason   string
	label    string // the justification
	source   string
	at       time.Time
}

// New returns the rules of the ignore entries in effect on day today and the statements of
// files. An entry whose until date is before today has expired: it matches nothing and
// [Rules.Expired] lists it.
func New(ignores []Ignore, files []File, today time.Time) (*Rules, error) {
	r := &Rules{}
	day := today.UTC().Format(DateLayout)
	for _, ig := range ignores {
		if ig.Until != "" {
			if _, err := time.Parse(DateLayout, ig.Until); err != nil {
				return nil, fmt.Errorf("vulns.ignore %s: until %q isn't a date (YYYY-MM-DD)", ig.ID, ig.Until)
			}
			if ig.Until < day {
				r.expired = append(r.expired, ig)
				continue
			}
		}
		r.ignores = append(r.ignores, ig)
	}
	r.used = make([]bool, len(r.ignores))
	for _, f := range files {
		for _, s := range f.Document.Statements {
			r.statements = append(r.statements, newStatement(f, s))
		}
	}
	// A later statement about the same vulnerability and product supersedes an earlier one.
	slices.SortStableFunc(r.statements, func(a, b statement) int { return a.at.Compare(b.at) })
	return r, nil
}

func newStatement(f File, s Statement) statement {
	at := f.Document.Timestamp
	if s.Timestamp != nil {
		at = *s.Timestamp
	}
	st := statement{
		names:  append([]string{s.Vulnerability.Name}, s.Vulnerability.Aliases...),
		status: s.Status,
		reason: cmp.Or(s.ImpactStatement, s.Justification, s.Status),
		label:  s.Justification,
		source: f.Path,
		at:     at,
	}
	var add func(cs []Component)
	add = func(cs []Component) {
		for _, c := range cs {
			for _, id := range []string{c.ID, c.Identifiers["purl"]} {
				if p, err := purl.Parse(id); err == nil {
					st.products = append(st.products, p)
				}
			}
			add(c.Subcomponents)
		}
	}
	add(s.Products)
	return st
}

// Suppress returns why the advisory named names is suppressed for s, or nil: the first ignore
// entry whose ID is one of names and whose tools match, else the latest statement about it and
// one of s's purls when its status is not_affected or fixed.
func (r *Rules) Suppress(s enrich.Subject, names []string) *enrich.Suppression {
	return r.find(s, names, true)
}

// Covers reports whether the rules suppress the advisory named names for s, without counting
// the match as a use.
func (r *Rules) Covers(s enrich.Subject, names []string) bool { return r.find(s, names, false) != nil }

func (r *Rules) find(s enrich.Subject, names []string, use bool) *enrich.Suppression {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, ig := range r.ignores {
		if namedIn(ig.ID, names) && toolsMatch(ig.Tools, s.Tool) {
			if use {
				r.used[i] = true
			}
			return &enrich.Suppression{Kind: enrich.SuppressedByIgnore, Reason: ig.Reason, Until: ig.Until}
		}
	}
	for i := len(r.statements) - 1; i >= 0; i-- {
		st := &r.statements[i]
		if !slices.ContainsFunc(st.names, func(n string) bool { return namedIn(n, names) }) || !st.covers(s.PURLs) {
			continue
		}
		if st.status != StatusNotAffected && st.status != StatusFixed {
			return nil
		}
		return &enrich.Suppression{
			Kind:          enrich.SuppressedByVEX,
			Reason:        st.reason,
			Source:        st.source,
			Status:        st.status,
			Justification: st.label,
		}
	}
	return nil
}

// covers reports whether one of the statement's products or subcomponents names one of pins.
func (st *statement) covers(pins []purl.PURL) bool {
	for _, p := range st.products {
		if slices.ContainsFunc(pins, p.Covers) {
			return true
		}
	}
	return false
}

func namedIn(id string, names []string) bool {
	return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(strings.TrimSpace(id), n) })
}

func toolsMatch(globs []string, tool string) bool {
	return len(globs) == 0 || slices.ContainsFunc(globs, func(g string) bool { return scan.MatchName(g, tool) })
}

// Expired returns the ignore entries whose until date has passed, in config order.
func (r *Rules) Expired() []Ignore { return slices.Clone(r.expired) }

// Unused returns the ignore entries in effect that suppressed nothing, in config order.
func (r *Rules) Unused() []Ignore {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Ignore
	for i, ig := range r.ignores {
		if !r.used[i] {
			out = append(out, ig)
		}
	}
	return out
}
