package scan

import (
	"cmp"
	"slices"
	"strings"
)

// Change is a pin whose value differs between two scans of the same tree. Base is nil for a pin
// only the head has, Head is nil for one only the base has.
type Change struct {
	Base *Pin `json:"base"`
	Head *Pin `json:"head"`
}

// Upgrade reports whether both sides pin one exact version of the same resolved tool and the
// versions differ. Whether the head is newer is left to the tool's version scheme.
func (c Change) Upgrade() bool {
	return c.Base != nil && c.Head != nil && c.Base.Resolved() && c.Base.Exact && c.Head.Exact &&
		c.Base.Version != c.Head.Version
}

// FloorRaise returns the lower bounds of a constraint whose bound moved while its operator and
// every other clause stayed, such as >= 6.62.0 to >= 6.72.0, ~> 1.5 to ~> 1.7, or
// >= 3.6.0, < 4.0.0 to >= 3.7.0, < 4.0.0, and reports whether c is one. Each side has exactly one
// clause with >=, ~>, ^ or ~, and both name the same resolved tool.
func (c Change) FloorRaise() (string, string, bool) {
	if c.Base == nil || c.Head == nil || !c.Base.Resolved() || c.Base.Match != MatchConstraint ||
		c.Head.Match != MatchConstraint {
		return "", "", false
	}
	base, okBase := lowerBound(c.Base.Value)
	head, okHead := lowerBound(c.Head.Value)
	if !okBase || !okHead || base.op != head.op || base.others != head.others || base.version == head.version {
		return "", "", false
	}
	return base.version, head.version, true
}

// bound is the lower-bound clause of a constraint and the clauses next to it.
type bound struct {
	op, version string
	others      string // the other clauses, normalized
}

// lowerBound finds the one clause of a comma-separated constraint with a lower-bounding operator.
// An alternative (||), a hyphen range or a wildcard in it isn't read.
func lowerBound(value string) (bound, bool) {
	if strings.Contains(value, "||") || strings.Contains(value, " - ") {
		return bound{}, false
	}
	var (
		out    bound
		found  bool
		others []string
	)
	for clause := range strings.SplitSeq(value, ",") {
		clause = strings.Join(strings.Fields(clause), "")
		op := constraintOp.FindString(clause)
		switch op {
		case ">=", "~>", "^", "~":
		default:
			others = append(others, clause)
			continue
		}
		version := strings.TrimPrefix(clause[len(op):], "v")
		if found || version == "" || wildcard.MatchString(version) {
			return bound{}, false
		}
		out.op, out.version, found = op, version, true
	}
	out.others = strings.Join(others, ",")
	return out, found
}

type diffKey struct {
	repo, file, kind, tool, raw string
}

func keyOf(p *Pin) diffKey {
	return diffKey{p.Location.Repo, p.Location.File, string(p.Kind), p.Tool, p.RawName}
}

// Diff pairs the pins of two scans and returns those that changed, sorted by the head's
// location. Pins pair when they share repository, file, kind, tool and raw name; a value both
// sides have counts as unchanged. When several pins of a key change, they pair in line order if
// both sides have the same number, and otherwise count as removed and added.
func Diff(base, head []Pin) []Change {
	baseByKey := group(base)
	headByKey := group(head)
	keys := make([]diffKey, 0, len(headByKey))
	for k := range headByKey {
		keys = append(keys, k)
	}
	for k := range baseByKey {
		if _, ok := headByKey[k]; !ok {
			keys = append(keys, k)
		}
	}
	var out []Change
	for _, k := range keys {
		b, h := dropUnchanged(baseByKey[k], headByKey[k])
		if len(b) == len(h) {
			for i := range b {
				out = append(out, Change{Base: b[i], Head: h[i]})
			}
			continue
		}
		for _, p := range b {
			out = append(out, Change{Base: p})
		}
		for _, p := range h {
			out = append(out, Change{Head: p})
		}
	}
	slices.SortFunc(out, func(a, b Change) int {
		return cmp.Or(comparePins(*a.side(), *b.side()), cmp.Compare(removed(a), removed(b)))
	})
	return out
}

func removed(c Change) int {
	if c.Head == nil {
		return 1
	}
	return 0
}

func (c Change) side() *Pin {
	if c.Head != nil {
		return c.Head
	}
	return c.Base
}

func group(pins []Pin) map[diffKey][]*Pin {
	out := map[diffKey][]*Pin{}
	for i := range pins {
		k := keyOf(&pins[i])
		out[k] = append(out[k], &pins[i])
	}
	return out
}

// dropUnchanged removes the pins whose value the other side also has, one for one, and keeps
// the rest in line order.
func dropUnchanged(base, head []*Pin) ([]*Pin, []*Pin) {
	used := make([]bool, len(head))
	var restBase []*Pin
	for _, b := range base {
		found := false
		for j, h := range head {
			if !used[j] && h.Value == b.Value {
				used[j], found = true, true
				break
			}
		}
		if !found {
			restBase = append(restBase, b)
		}
	}
	var restHead []*Pin
	for j, h := range head {
		if !used[j] {
			restHead = append(restHead, h)
		}
	}
	return restBase, restHead
}
