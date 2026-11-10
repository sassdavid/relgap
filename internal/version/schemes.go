package version

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	goversion "github.com/hashicorp/go-version"
)

// Semver returns the semantic versioning scheme. A leading "v" is accepted.
func Semver() Scheme { return semverScheme{} }

type semverScheme struct{}

func (semverScheme) Name() string { return SchemeSemver }

func (semverScheme) Parse(s string) (V, error) {
	sv, err := semver.NewVersion(s)
	if err != nil {
		return V{}, fmt.Errorf("%w: %q: %w", ErrParse, s, err)
	}
	return V{
		text: trimV(s),
		rel:  []uint64{sv.Major(), sv.Minor(), sv.Patch()},
		pre:  sv.Prerelease() != "",
		impl: sv,
	}, nil
}

func (semverScheme) Compare(a, b V) int {
	x, okA := a.impl.(*semver.Version)
	y, okB := b.impl.(*semver.Version)
	if !okA || !okB {
		return strings.Compare(a.text, b.text)
	}
	return x.Compare(y)
}

func (semverScheme) IsPrerelease(v V) bool { return v.pre }

func (semverScheme) Line(v V, level Level) string { return line(v.rel, level) }

// Loose returns a lenient scheme for versions that aren't semver: any number of segments
// ("2.35.3.1"), Go's "go1.20" and "go1.21rc2", and build metadata ("21.0.9+10", ignored when
// comparing).
func Loose() Scheme { return looseScheme{} }

type looseScheme struct{}

func (looseScheme) Name() string { return SchemeLoose }

var goPrefix = regexp.MustCompile(`^go[0-9]`)

func (looseScheme) Parse(s string) (V, error) {
	text := trimV(s)
	if goPrefix.MatchString(text) {
		text = text[2:]
	}
	gv, err := goversion.NewVersion(text)
	if err != nil {
		return V{}, fmt.Errorf("%w: %q: %w", ErrParse, s, err)
	}
	segs := gv.Segments64()
	rel := make([]uint64, len(segs))
	for i, n := range segs {
		rel[i] = uint64(n) //nolint:gosec // go-version only parses non-negative segments
	}
	return V{text: text, rel: rel, pre: gv.Prerelease() != "", impl: loose{rel: rel, pre: gv.Prerelease()}}, nil
}

// loose is compared here, not by go-version, whose Compare isn't transitive when segment counts
// differ and a prerelease is involved ("0" <= "0.0.0.0" <= "0A" but "0" > "0A").
type loose struct {
	rel []uint64
	pre string
}

func (looseScheme) Compare(a, b V) int {
	x, okA := a.impl.(loose)
	y, okB := b.impl.(loose)
	if !okA || !okB {
		return strings.Compare(a.text, b.text)
	}
	return compareReleasePre(x.rel, x.pre, y.rel, y.pre)
}

func (looseScheme) IsPrerelease(v V) bool { return v.pre }

func (looseScheme) Line(v V, level Level) string { return line(v.rel, level) }

// Calver returns the calendar versioning scheme: numeric segments compared as integers
// ("2026.11.5", "2026.1"), with an optional "-suffix" marking a prerelease.
func Calver() Scheme { return calverScheme{} }

type calverScheme struct{}

type calver struct {
	rel []uint64
	pre string
}

var calverRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)*)(?:-([0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*))?$`)

func (calverScheme) Name() string { return SchemeCalver }

func (calverScheme) Parse(s string) (V, error) {
	text := trimV(s)
	m := calverRe.FindStringSubmatch(text)
	if m == nil {
		return V{}, fmt.Errorf("%w: %q is not a calendar version", ErrParse, s)
	}
	rel, err := parseSegments(m[1])
	if err != nil {
		return V{}, err
	}
	return V{text: text, rel: rel, pre: m[2] != "", impl: calver{rel: rel, pre: m[2]}}, nil
}

func (calverScheme) Compare(a, b V) int {
	x, okA := a.impl.(calver)
	y, okB := b.impl.(calver)
	if !okA || !okB {
		return strings.Compare(a.text, b.text)
	}
	return compareReleasePre(x.rel, x.pre, y.rel, y.pre)
}

func (calverScheme) IsPrerelease(v V) bool { return v.pre }

func (calverScheme) Line(v V, level Level) string { return line(v.rel, level) }

// Python returns a PEP 440 subset: release segments, a/b/rc prereleases, .postN and .devN.
// Epochs and local versions are rejected.
func Python() Scheme { return pythonScheme{} }

type pythonScheme struct{}

// pep440 holds the comparison key used by the pypa packaging library.
type pep440 struct {
	rel     []uint64
	preRank int // see the rank constants
	preNum  uint64
	hasPre  bool
	post    uint64
	hasPost bool
	dev     uint64
	hasDev  bool
}

// Ranks order the prerelease part; a dev-only release sorts first and a final release last.
const (
	rankDevOnly = -1
	rankAlpha   = 0
	rankBeta    = 1
	rankRC      = 2
	rankFinal   = 3
)

var pythonRe = regexp.MustCompile(`^(?P<rel>[0-9]+(?:\.[0-9]+)*)` +
	`(?:[-_.]?(?P<pre>a|b|c|rc|alpha|beta|pre|preview)[-_.]?(?P<prenum>[0-9]*))?` +
	`(?:[-_.]?(?:post|rev|r)[-_.]?(?P<post>[0-9]*)|-(?P<implicitpost>[0-9]+))?` +
	`(?:[-_.]?(?P<dev>dev)[-_.]?(?P<devnum>[0-9]*))?$`)

func (pythonScheme) Name() string { return SchemePython }

func (pythonScheme) Parse(s string) (V, error) {
	text := trimV(strings.ToLower(s))
	p, err := parsePEP440(text)
	if err != nil {
		return V{}, fmt.Errorf("%w: %q: %w", ErrParse, s, err)
	}
	return V{text: text, rel: p.rel, pre: p.hasPre || p.hasDev, impl: p}, nil
}

func parsePEP440(text string) (pep440, error) {
	m := pythonRe.FindStringSubmatchIndex(text)
	if m == nil {
		return pep440{}, errors.New("not a PEP 440 version")
	}
	group := func(name string) (string, bool) {
		i := pythonRe.SubexpIndex(name)
		if m[2*i] < 0 {
			return "", false
		}
		return text[m[2*i]:m[2*i+1]], true
	}
	var firstErr error
	num := func(name string) (uint64, bool) {
		g, ok := group(name)
		if !ok || g == "" {
			return 0, ok
		}
		n, err := strconv.ParseUint(g, 10, 64)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		return n, true
	}
	relText, _ := group("rel")
	rel, err := parseSegments(relText)
	if err != nil {
		return pep440{}, err
	}
	p := pep440{rel: rel}
	if label, ok := group("pre"); ok {
		p.hasPre, p.preRank = true, preRank(label)
		p.preNum, _ = num("prenum")
	}
	if p.post, p.hasPost = num("post"); !p.hasPost {
		p.post, p.hasPost = num("implicitpost")
	}
	if _, ok := group("dev"); ok {
		p.dev, p.hasDev = num("devnum")
		p.hasDev = true
	}
	return p, firstErr
}

func preRank(label string) int {
	switch label {
	case "a", "alpha":
		return rankAlpha
	case "b", "beta":
		return rankBeta
	default:
		return rankRC
	}
}

func (p pep440) preKey() (int, uint64) {
	switch {
	case !p.hasPre && !p.hasPost && p.hasDev:
		return rankDevOnly, 0
	case !p.hasPre:
		return rankFinal, 0
	default:
		return p.preRank, p.preNum
	}
}

func (pythonScheme) Compare(a, b V) int {
	x, okA := a.impl.(pep440)
	y, okB := b.impl.(pep440)
	if !okA || !okB {
		return strings.Compare(a.text, b.text)
	}
	if c := compareRelease(x.rel, y.rel); c != 0 {
		return c
	}
	xr, xn := x.preKey()
	yr, yn := y.preKey()
	if c := cmpInt(xr, yr); c != 0 {
		return c
	}
	if c := cmpUint(xn, yn); c != 0 {
		return c
	}
	// No post release sorts before any post release.
	if c := cmpBoolUint(x.hasPost, x.post, y.hasPost, y.post); c != 0 {
		return c
	}
	return compareDev(x, y)
}

// compareDev sorts a version without a dev release after every dev release of it.
func compareDev(x, y pep440) int {
	switch {
	case x.hasDev && y.hasDev:
		return cmpUint(x.dev, y.dev)
	case x.hasDev:
		return -1
	case y.hasDev:
		return 1
	default:
		return 0
	}
}

func (pythonScheme) IsPrerelease(v V) bool { return v.pre }

func (pythonScheme) Line(v V, level Level) string { return line(v.rel, level) }

// Regex returns a scheme that applies extract to the tag and parses its capture groups, joined
// with ".", under [Loose]. Without capture groups the whole match is used.
func Regex(extract string) (Scheme, error) {
	if extract == "" {
		return nil, fmt.Errorf("scheme %s needs an extract expression", SchemeRegex)
	}
	re, err := regexp.Compile(extract)
	if err != nil {
		return nil, fmt.Errorf("compile extract: %w", err)
	}
	return regexScheme{re: re}, nil
}

type regexScheme struct{ re *regexp.Regexp }

func (regexScheme) Name() string { return SchemeRegex }

func (r regexScheme) Parse(s string) (V, error) {
	m := r.re.FindStringSubmatch(s)
	if m == nil {
		return V{}, fmt.Errorf("%w: %q does not match %s", ErrParse, s, r.re)
	}
	text := m[0]
	if len(m) > 1 {
		var parts []string
		for _, g := range m[1:] {
			if g != "" {
				parts = append(parts, g)
			}
		}
		text = strings.Join(parts, ".")
	}
	return Loose().Parse(text)
}

func (regexScheme) Compare(a, b V) int { return Loose().Compare(a, b) }

func (regexScheme) IsPrerelease(v V) bool { return v.pre }

func (regexScheme) Line(v V, level Level) string { return line(v.rel, level) }

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// cmpBoolUint orders (false, _) before (true, n), and (true, n) by n.
func cmpBoolUint(aSet bool, a uint64, bSet bool, b uint64) int {
	switch {
	case aSet && bSet:
		return cmpUint(a, b)
	case aSet:
		return 1
	case bSet:
		return -1
	default:
		return 0
	}
}
