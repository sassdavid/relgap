package enrich

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// OSV ecosystems and range types relgap handles specially.
const (
	EcosystemGo  = "Go"
	EcosystemGit = "GIT"
	ecosystemPy  = "PyPI"
	rangeSemver  = "SEMVER"
	rangeEco     = "ECOSYSTEM"
	introducedAt = "0" // an introduced event at the start of time
)

// Vuln is one known vulnerability of a tool's current version.
type Vuln struct {
	ID             string   `json:"id"`
	Aliases        []string `json:"aliases,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Package        Package  `json:"package"`
	Severity       *Score   `json:"severity,omitempty"`
	FixedIn        string   `json:"fixed_in,omitempty"` // lowest fixed version above the current one
	AffectsCurrent bool     `json:"affects_current"`
	// FixedByTarget says whether the target version is outside every affected range. It is
	// absent without a target or when the record's ranges can't be compared locally.
	FixedByTarget *bool `json:"fixed_by_target,omitempty"`
}

// compare orders two versions of a range; the second result is false when either doesn't parse.
type compare func(a, b string) (int, bool)

func schemeCompare(s version.Scheme, norm func(string) string) compare {
	return func(a, b string) (int, bool) {
		va, errA := s.Parse(norm(a))
		vb, errB := s.Parse(norm(b))
		if errA != nil || errB != nil {
			return 0, false
		}
		return s.Compare(va, vb), true
	}
}

// compareFor returns how versions of a range type compare in an ecosystem, or nil when relgap
// can't order them (GIT commits, and ecosystems without a matching scheme).
func compareFor(rangeType, ecosystem string) compare {
	switch {
	case rangeType == rangeSemver && ecosystem == EcosystemGo:
		return schemeCompare(version.Semver(), GoSemver)
	case rangeType == rangeSemver:
		return schemeCompare(version.Semver(), strings.TrimSpace)
	case rangeType == rangeEco && ecosystem == ecosystemPy:
		s, err := version.NewScheme(version.SchemePython, "")
		if err != nil {
			return nil
		}
		return schemeCompare(s, strings.TrimSpace)
	case rangeType == rangeEco && (ecosystem == "npm" || ecosystem == "crates.io"):
		return schemeCompare(version.Semver(), strings.TrimSpace)
	default:
		return nil
	}
}

var goTag = regexp.MustCompile(`^(?:go|v)?(\d+)\.(\d+)(?:\.(\d+))?(?:(rc|beta)(\d+))?$`)

// GoSemver turns a Go version or tag (go1.24, 1.24.3, go1.25rc1) into the semver form OSV uses
// for the Go ecosystem (1.24.0, 1.24.3, 1.25.0-rc.1). Other text comes back trimmed of a v.
func GoSemver(s string) string {
	s = strings.TrimSpace(strings.Fields(s + " ")[0])
	m := goTag.FindStringSubmatch(s)
	if m == nil {
		return strings.TrimPrefix(s, "v")
	}
	out := m[1] + "." + m[2] + "." + cmp.Or(m[3], "0")
	if m[4] != "" {
		out += "-" + m[4] + "." + m[5]
	}
	return out
}

// matches reports whether an affected entry describes pkg. For GIT, pkg's name is the
// repository URL, which OSV records put on their GIT ranges rather than in a package.
func (a Affected) matches(pkg Package) bool {
	if !strings.EqualFold(pkg.Ecosystem, EcosystemGit) {
		return strings.EqualFold(a.Package.Ecosystem, pkg.Ecosystem) && a.Package.Name == pkg.Name
	}
	for _, rg := range a.Ranges {
		if rg.Type == EcosystemGit && sameRepo(rg.Repo, pkg.Name) {
			return true
		}
	}
	return strings.EqualFold(a.Package.Ecosystem, EcosystemGit) && sameRepo(a.Package.Name, pkg.Name)
}

// sameRepo compares repository URLs without case, a trailing slash and a .git suffix.
func sameRepo(a, b string) bool {
	norm := func(u string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSuffix(u, "/")), ".git") }
	return a != "" && norm(a) == norm(b)
}

// Affects reports whether the record lists version v of pkg as affected, following the OSV
// schema: v is in versions, or inside a range's introduced..fixed (or ..last_affected). The
// second result is false when a range that could hold v can't be compared and no versions list
// covers it.
func Affects(r *Record, pkg Package, v string) (bool, bool) {
	known := true
	for _, a := range r.Affected {
		if !a.matches(pkg) {
			continue
		}
		if slices.Contains(a.Versions, v) {
			return true, true
		}
		for _, rg := range a.Ranges {
			in, ok := inRange(rg, a.Package.Ecosystem, v)
			if in {
				return true, true
			}
			if !ok && len(a.Versions) == 0 {
				known = false
			}
		}
	}
	return false, known
}

// inRange evaluates one range's events in version order, as the OSV schema describes: each
// event at or below v switches v in or out of the range.
func inRange(rg Range, ecosystem, v string) (bool, bool) {
	order := compareFor(rg.Type, ecosystem)
	if order == nil {
		return false, false
	}
	const (
		introduced = iota
		fixed
		lastAffected
	)
	type event struct {
		at   string
		kind int
	}
	affected := false
	var evs []event
	for _, e := range rg.Events {
		switch {
		case e.Introduced == introducedAt:
			affected = true
		case e.Introduced != "":
			evs = append(evs, event{e.Introduced, introduced})
		case e.Fixed != "":
			evs = append(evs, event{e.Fixed, fixed})
		case e.LastAffected != "":
			evs = append(evs, event{e.LastAffected, lastAffected})
		case e.Limit != "":
			evs = append(evs, event{e.Limit, fixed}) // a limit ends the range like a fix
		}
	}
	parsed := true
	slices.SortStableFunc(evs, func(a, b event) int {
		c, ok := order(a.at, b.at)
		parsed = parsed && ok
		return c
	})
	if !parsed {
		return false, false
	}
	for _, e := range evs {
		c, ok := order(v, e.at)
		if !ok {
			return false, false
		}
		switch {
		case e.kind == introduced && c >= 0:
			affected = true
		case e.kind == fixed && c >= 0, e.kind == lastAffected && c > 0:
			affected = false
		}
	}
	return affected, true
}

// FixedIn returns the lowest fixed version above v in the ranges of pkg, or "".
func FixedIn(r *Record, pkg Package, v string) string {
	best := ""
	var order compare
	for _, a := range r.Affected {
		if !a.matches(pkg) {
			continue
		}
		for _, rg := range a.Ranges {
			c := compareFor(rg.Type, a.Package.Ecosystem)
			if c == nil {
				continue
			}
			if fix := lowestFixAbove(rg, c, v); fix != "" && (best == "" || less(order, fix, best)) {
				best, order = fix, c
			}
		}
	}
	return best
}

// lowestFixAbove returns the lowest fixed event of rg above v, or "".
func lowestFixAbove(rg Range, c compare, v string) string {
	best := ""
	for _, e := range rg.Events {
		if e.Fixed == "" || !less(c, v, e.Fixed) {
			continue
		}
		if best == "" || less(c, e.Fixed, best) {
			best = e.Fixed
		}
	}
	return best
}

// less reports whether a orders before b; versions that don't parse never do.
func less(c compare, a, b string) bool {
	n, ok := c(a, b)
	return ok && n < 0
}

var goMajor = regexp.MustCompile(`/v(\d+)$`)

// goSuffixFrom is the first major version whose Go module path ends in /vN.
const goSuffixFrom = 2

// PackagesFor returns the OSV packages that describe major version line of v. A Go module path
// ending in /vN belongs to major N only; one without a suffix belongs to majors 0 and 1 when the
// list has suffixed paths too, and to every major otherwise.
func PackagesFor(pkgs []registry.OSVPackage, s version.Scheme, v version.V) []registry.OSVPackage {
	major, err := strconv.Atoi(s.Line(v, version.LineMajor))
	if err != nil {
		return pkgs
	}
	suffixed := slices.ContainsFunc(pkgs, func(p registry.OSVPackage) bool {
		return p.Ecosystem == EcosystemGo && goMajor.MatchString(p.Name)
	})
	var out []registry.OSVPackage
	for _, p := range pkgs {
		if p.Ecosystem != EcosystemGo {
			out = append(out, p)
			continue
		}
		if m := goMajor.FindStringSubmatch(p.Name); m != nil {
			if n, _ := strconv.Atoi(m[1]); n == major {
				out = append(out, p)
			}
			continue
		}
		if !suffixed || major < goSuffixFrom {
			out = append(out, p)
		}
	}
	return out
}

// QueryVersion is the version string OSV expects for v of a package: the tag for GIT, the Go
// semver form for Go, and the version itself elsewhere.
func QueryVersion(p registry.OSVPackage, tag string, v version.V) string {
	switch p.Ecosystem {
	case EcosystemGit:
		return tag
	case EcosystemGo:
		return GoSemver(v.String())
	default:
		return v.String()
	}
}
