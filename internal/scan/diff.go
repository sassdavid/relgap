package scan

import (
	"cmp"
	"slices"
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
