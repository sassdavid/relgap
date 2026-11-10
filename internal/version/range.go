package version

import (
	"errors"
	"fmt"
	"slices"
)

// Range selection errors.
var (
	ErrDowngrade    = errors.New("range goes backwards")
	ErrNotFound     = errors.New("version not found")
	ErrLineMismatch = errors.New("range leaves the release line")
)

// RangeOptions selects the versions in (From, To].
type RangeOptions struct {
	From               string
	To                 string // empty means the latest stable version on the line
	Line               Level
	IncludePrereleases bool // also on when From or To is itself a prerelease
}

// Range is the result of [Universe.Select].
type Range struct {
	From        V        // as requested
	Base        *Version // the version the range starts after: From, the nearest lower one, or nil
	To          Version
	Versions    []Version // (From, To], oldest first
	Prereleases bool      // whether prereleases were included
	Warnings    []string
}

// Select returns the versions after From up to and including To.
func (u *Universe) Select(opts RangeOptions) (*Range, error) {
	s := u.rules.Scheme
	from, to, err := u.bounds(opts)
	if err != nil {
		return nil, err
	}

	r := &Range{From: from}
	r.Prereleases = opts.IncludePrereleases || s.IsPrerelease(from) || (!to.IsZero() && s.IsPrerelease(to))
	candidates := u.filtered(r.Prereleases)

	r.Base = nearestAtOrBelow(s, candidates, from)
	switch {
	case r.Base == nil:
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s is older than every known version", from))
	case s.Compare(r.Base.V, from) != 0:
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("%s is not a known version; using the nearest lower version %s", from, r.Base.V))
	}

	if to.IsZero() {
		latest, ok := latestOnLine(s, candidates, from, opts.Line)
		if !ok {
			return nil, fmt.Errorf("%w: no stable version on the %s line of %s", ErrNotFound, lineName(opts.Line), from)
		}
		r.To = latest
	} else {
		i, ok := u.find(to)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, to)
		}
		r.To = u.versions[i]
	}

	for _, v := range candidates {
		if s.Compare(v.V, from) > 0 && s.Compare(v.V, r.To.V) <= 0 {
			r.Versions = append(r.Versions, v)
		}
	}
	return r, nil
}

// bounds parses From and To (zero when empty) and rejects backwards and cross-line ranges.
func (u *Universe) bounds(opts RangeOptions) (V, V, error) {
	s := u.rules.Scheme
	from, err := u.Parse(opts.From)
	if err != nil {
		return V{}, V{}, fmt.Errorf("parse from: %w", err)
	}
	if opts.To == "" {
		return from, V{}, nil
	}
	to, err := u.Parse(opts.To)
	if err != nil {
		return V{}, V{}, fmt.Errorf("parse to: %w", err)
	}
	if s.Compare(from, to) > 0 {
		return V{}, V{}, fmt.Errorf("%w: %s is newer than %s; swap the arguments", ErrDowngrade, from, to)
	}
	if opts.Line != LineNone && s.Line(from, opts.Line) != s.Line(to, opts.Line) {
		return V{}, V{}, fmt.Errorf("%w: %s and %s are on different %s lines", ErrLineMismatch, from, to, opts.Line)
	}
	return from, to, nil
}

func (u *Universe) filtered(prereleases bool) []Version {
	out := make([]Version, 0, len(u.versions))
	for _, v := range u.versions {
		if prereleases || !v.Prerelease {
			out = append(out, v)
		}
	}
	return out
}

func nearestAtOrBelow(s Scheme, vs []Version, v V) *Version {
	var best *Version
	for i := range vs {
		if s.Compare(vs[i].V, v) > 0 {
			break
		}
		best = &vs[i]
	}
	return best
}

// latestOnLine returns the newest stable version on from's line. Prereleases never become the
// default end of a range.
func latestOnLine(s Scheme, vs []Version, from V, level Level) (Version, bool) {
	want := s.Line(from, level)
	for _, v := range slices.Backward(vs) {
		if v.Prerelease {
			continue
		}
		if level == LineNone || s.Line(v.V, level) == want {
			return v, true
		}
	}
	return Version{}, false
}

func lineName(l Level) string {
	if l == LineNone || l == "" {
		return "any"
	}
	return string(l)
}
