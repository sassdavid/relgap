package version

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrNoMatch is returned when no version satisfies a constraint.
var ErrNoMatch = errors.New("no version satisfies the constraint")

// Constraint is a comma-separated list of terms that must all hold, such as "~> 1.15" or
// ">= 1.2, < 2". Operators: =, !=, >, >=, <, <= and ~> (pessimistic: "~> 1.15" allows 1.x from
// 1.15 on, "~> 1.15.3" allows 1.15.x from 1.15.3 on).
type Constraint struct {
	text   string
	scheme Scheme
	terms  []term
}

type term struct {
	op   string
	v    V
	size int // segments written, for ~>
}

// String returns the constraint as written.
func (c Constraint) String() string { return c.text }

// ParseConstraint parses expr with versions read under s.
func ParseConstraint(s Scheme, expr string) (Constraint, error) {
	c := Constraint{text: strings.TrimSpace(expr), scheme: s}
	for part := range strings.SplitSeq(expr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return Constraint{}, fmt.Errorf("constraint %q: empty term", expr)
		}
		op := "="
		for _, candidate := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
			if rest, ok := strings.CutPrefix(part, candidate); ok {
				op, part = candidate, strings.TrimSpace(rest)
				break
			}
		}
		v, err := s.Parse(part)
		if err != nil {
			return Constraint{}, fmt.Errorf("constraint %q: %w", expr, err)
		}
		release, _, _ := strings.Cut(trimV(part), "-")
		size := len(strings.Split(release, "."))
		c.terms = append(c.terms, term{op: op, v: v, size: size})
	}
	return c, nil
}

// Check reports whether v satisfies every term.
func (c Constraint) Check(v V) bool {
	for _, t := range c.terms {
		cmp := c.scheme.Compare(v, t.v)
		var ok bool
		switch t.op {
		case "=":
			ok = cmp == 0
		case "!=":
			ok = cmp != 0
		case ">":
			ok = cmp > 0
		case ">=":
			ok = cmp >= 0
		case "<":
			ok = cmp < 0
		case "<=":
			ok = cmp <= 0
		case "~>":
			ok = cmp >= 0 && samePrefix(v.rel, t.v.rel, max(t.size-1, 1))
		}
		if !ok {
			return false
		}
	}
	return true
}

func samePrefix(a, b []uint64, n int) bool {
	for i := range n {
		var x, y uint64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return false
		}
	}
	return true
}

// Pick says how [Universe.ResolveConstraint] chose a version.
type Pick string

// Ways a constraint pin becomes a version.
const (
	PickKnown  Pick = "highest installed or locked version"
	PickLowest Pick = "lowest satisfying version"
)

// ResolveConstraint returns the version a constraint pin stands for: the highest of known
// (installed or locked versions) that satisfies c, otherwise the lowest stable version in the
// universe that does.
func (u *Universe) ResolveConstraint(c Constraint, known []V) (V, Pick, error) {
	s := u.rules.Scheme
	var best V
	for _, k := range known {
		if c.Check(k) && (best.IsZero() || s.Compare(k, best) > 0) {
			best = k
		}
	}
	if !best.IsZero() {
		return best, PickKnown, nil
	}
	i := slices.IndexFunc(u.versions, func(v Version) bool { return !v.Prerelease && c.Check(v.V) })
	if i < 0 {
		return V{}, "", fmt.Errorf("%w: %s", ErrNoMatch, c)
	}
	return u.versions[i].V, PickLowest, nil
}

// ResolvePrefix returns the version a prefix pin such as "26" or "1.2" stands for: the highest
// stable version whose leading release segments equal the prefix, which is what mise installs.
func (u *Universe) ResolvePrefix(prefix string) (V, error) {
	p, err := u.Parse(prefix)
	if err != nil {
		return V{}, err
	}
	release, _, _ := strings.Cut(trimV(strings.TrimPrefix(prefix, u.rules.TagPrefix)), "-")
	size := len(strings.Split(release, "."))
	for _, v := range slices.Backward(u.versions) {
		if !v.Prerelease && samePrefix(v.V.rel, p.rel, size) {
			return v.V, nil
		}
	}
	return V{}, fmt.Errorf("%w: no stable version starts with %s", ErrNoMatch, prefix)
}

// Floor returns the lowest version c allows without a version list: its highest inclusive lower
// bound (=, >= or ~>), padded to three segments. The second result is false when c has no
// inclusive lower bound, when an exclusive bound (>) sits at or above it, or when the bound
// itself fails another term, as in ">= 1.5, != 1.5.0".
func (c Constraint) Floor() (V, bool) {
	s := c.scheme
	var floor V
	for _, t := range c.terms {
		if (t.op == "=" || t.op == ">=" || t.op == "~>") && (floor.IsZero() || s.Compare(t.v, floor) > 0) {
			floor = t.v
		}
	}
	if floor.IsZero() {
		return V{}, false
	}
	for _, t := range c.terms {
		if t.op == ">" && s.Compare(t.v, floor) >= 0 {
			return V{}, false
		}
	}
	floor = pad(s, floor)
	return floor, c.Check(floor)
}

// pad writes a release-only version such as 1.15 as 1.15.0, when the scheme reads both as equal.
func pad(s Scheme, v V) V {
	const segments = 3
	text := v.String()
	if strings.ContainsAny(text, "-+") || len(v.rel) == 0 {
		return v
	}
	n := len(strings.Split(text, "."))
	if n >= segments {
		return v
	}
	padded, err := s.Parse(text + strings.Repeat(".0", segments-n))
	if err != nil || s.Compare(padded, v) != 0 {
		return v
	}
	return padded
}
