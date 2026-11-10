package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrParse is wrapped by every parse error.
var ErrParse = errors.New("invalid version")

// V is a version parsed by a [Scheme]. Only the scheme that parsed it can compare it.
type V struct {
	text string   // input without a leading "v", e.g. "3.6.0-rc.1"
	rel  []uint64 // release segments, used for lines and constraints
	pre  bool
	impl any // scheme-specific parsed value
}

// String returns the version text without a tag prefix or leading "v".
func (v V) String() string { return v.text }

// IsZero reports whether v is the zero value.
func (v V) IsZero() bool { return v.text == "" && v.impl == nil }

// Level is a release line granularity.
type Level string

// Line levels: none allows any newer version, major keeps the major, minor keeps major.minor.
const (
	LineNone  Level = "none"
	LineMajor Level = "major"
	LineMinor Level = "minor"
)

// ParseLevel validates a line level. The empty string means [LineNone].
func ParseLevel(s string) (Level, error) {
	switch l := Level(s); l {
	case "", LineNone:
		return LineNone, nil
	case LineMajor, LineMinor:
		return l, nil
	default:
		return "", fmt.Errorf("invalid line %q: want none, major or minor", s)
	}
}

// Scheme parses and orders the versions of one tool.
type Scheme interface {
	Name() string
	Parse(s string) (V, error)
	Compare(a, b V) int
	IsPrerelease(v V) bool
	Line(v V, level Level) string
}

// Scheme names, as used in registry entries.
const (
	SchemeSemver = "semver"
	SchemeLoose  = "loose"
	SchemeCalver = "calver"
	SchemePython = "python"
	SchemeRegex  = "regex"
)

// SchemeNames lists every scheme name.
func SchemeNames() []string {
	return []string{SchemeSemver, SchemeLoose, SchemeCalver, SchemePython, SchemeRegex}
}

// NewScheme returns the named scheme. extract is the regular expression for [SchemeRegex] and
// must be empty for the others.
func NewScheme(name, extract string) (Scheme, error) {
	if extract != "" && name != SchemeRegex {
		return nil, fmt.Errorf("extract is only valid with scheme %s, not %s", SchemeRegex, name)
	}
	switch name {
	case SchemeSemver:
		return Semver(), nil
	case SchemeLoose:
		return Loose(), nil
	case SchemeCalver:
		return Calver(), nil
	case SchemePython:
		return Python(), nil
	case SchemeRegex:
		return Regex(extract)
	default:
		return nil, fmt.Errorf("unknown version scheme %q: want one of %s", name, strings.Join(SchemeNames(), ", "))
	}
}

// line renders the first one or two release segments.
func line(rel []uint64, level Level) string {
	n := 0
	switch level {
	case LineMajor:
		n = 1
	case LineMinor:
		n = 2
	case LineNone:
		return ""
	}
	parts := make([]string, n)
	for i := range n {
		var seg uint64
		if i < len(rel) {
			seg = rel[i]
		}
		parts[i] = strconv.FormatUint(seg, 10)
	}
	return strings.Join(parts, ".")
}

// compareRelease compares release segments, treating missing trailing segments as zero.
func compareRelease(a, b []uint64) int {
	for i := range max(len(a), len(b)) {
		var x, y uint64
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// compareReleasePre compares release segments, then prereleases: a release without one sorts
// after every prerelease of it.
func compareReleasePre(aRel []uint64, aPre string, bRel []uint64, bPre string) int {
	if c := compareRelease(aRel, bRel); c != 0 {
		return c
	}
	switch {
	case aPre == bPre:
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	default:
		return comparePre(aPre, bPre)
	}
}

// parseSegments parses "1.2.3" into release segments.
func parseSegments(s string) ([]uint64, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: empty release", ErrParse)
	}
	parts := strings.Split(s, ".")
	out := make([]uint64, len(parts))
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: segment %q: %w", ErrParse, p, err)
		}
		out[i] = n
	}
	return out, nil
}

// comparePre orders dot-separated prerelease identifiers by semver precedence: numeric
// identifiers compare as numbers and sort before alphanumeric ones, and a shorter list of equal
// identifiers sorts first.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(as), len(bs)) {
		if c := compareIdent(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(as), len(bs))
}

func compareIdent(a, b string) int {
	an, bn := isDigits(a), isDigits(b)
	switch {
	case an && bn:
		return compareDigits(a, b)
	case an:
		return -1
	case bn:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// compareDigits compares decimal strings of any length.
func compareDigits(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if c := cmpInt(len(a), len(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func trimV(s string) string {
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		return s[1:]
	}
	return s
}
