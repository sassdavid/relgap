package registry

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// tagPrefixes returns the package's tag prefixes, newest first, or nil for unprefixed tags only.
func (a *AquaEntry) tagPrefixes() Prefixes {
	p := Prefixes(a.VersionPrefixes)
	if len(p) == 0 {
		p = newPrefixes(a.VersionPrefix)
	}
	if len(p) == 1 && p[0] == "" {
		return nil
	}
	return slices.Clone(p)
}

const filterLiteral = `("(?:[^"\\]|\\.)*")`

var (
	filterTerm = regexp.MustCompile(`Version\s+(startsWith|matches)\s+` + filterLiteral)
	filterNot  = regexp.MustCompile(`^not\s*\(\s*Version\s+(startsWith|matches)\s+` + filterLiteral + `\s*\)$`)
	filterOr   = regexp.MustCompile(`^\s*\|\|\s*$`)
)

// aquaFilter translates an aqua version_filter into regular expressions on the whole tag: tags
// must match include and must not match exclude. It knows three forms, `not (Version startsWith
// "x")`, and `Version startsWith "x"` or `Version matches "re"` joined by ||, and gives "", ""
// for any other expression.
func aquaFilter(expr string) (string, string) {
	expr = strings.TrimSpace(expr)
	if m := filterNot.FindStringSubmatch(expr); m != nil {
		re, ok := filterRegexp(m[1], m[2])
		if !ok {
			return "", ""
		}
		return "", re
	}
	locs := filterTerm.FindAllStringSubmatchIndex(expr, -1)
	if locs == nil {
		return "", ""
	}
	parts := make([]string, 0, len(locs))
	end := 0
	for i, l := range locs {
		if gap := expr[end:l[0]]; i == 0 && gap != "" || i > 0 && !filterOr.MatchString(gap) {
			return "", ""
		}
		re, ok := filterRegexp(expr[l[2]:l[3]], expr[l[4]:l[5]])
		if !ok {
			return "", ""
		}
		if len(locs) > 1 && strings.Contains(re, "|") {
			re = "(?:" + re + ")"
		}
		parts = append(parts, re)
		end = l[1]
	}
	if end != len(expr) {
		return "", ""
	}
	return strings.Join(parts, "|"), ""
}

// filterRegexp returns the regular expression of one startsWith or matches term.
func filterRegexp(op, literal string) (string, bool) {
	s, err := strconv.Unquote(literal)
	if err != nil || s == "" {
		return "", false
	}
	if op == "startsWith" {
		return "^" + regexp.QuoteMeta(s), true
	}
	if _, err = regexp.Compile(s); err != nil {
		return "", false
	}
	return s, true
}
