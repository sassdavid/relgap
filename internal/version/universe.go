package version

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Ref is a tag or a release as the forge lists it.
type Ref struct {
	Tag         string
	Draft       bool
	Prerelease  bool // the forge's prerelease flag; the scheme can also mark one
	PublishedAt time.Time
}

// Rules turn tags into versions.
type Rules struct {
	Scheme Scheme
	// TagPrefixes are the prefixes the tool's tags have used. The longest one a tag starts with
	// is stripped before parsing, and tags with none of them are dropped. Empty means no prefix;
	// a list takes unprefixed tags only when it holds "".
	TagPrefixes []string
	TagSuffix   string         // stripped after the prefix, such as an image variant; tags without it are dropped
	Include     *regexp.Regexp // matched against the whole tag; other tags are dropped
	Exclude     *regexp.Regexp // matched against the whole tag; matching tags are dropped
}

// cutPrefix strips the longest tag prefix s starts with, and reports whether one did.
func (r Rules) cutPrefix(s string) (string, bool) {
	if len(r.TagPrefixes) == 0 {
		return s, true
	}
	best := -1
	for i, p := range r.TagPrefixes {
		if strings.HasPrefix(s, p) && (best < 0 || len(p) > len(r.TagPrefixes[best])) {
			best = i
		}
	}
	if best < 0 {
		return s, false
	}
	return s[len(r.TagPrefixes[best]):], true
}

// Version is one entry of a [Universe].
type Version struct {
	V           V
	Tag         string
	Prerelease  bool
	PublishedAt time.Time
}

// Universe is the set of a tool's versions, sorted ascending by its scheme.
type Universe struct {
	rules    Rules
	versions []Version
}

// NewUniverse parses refs under rules. Drafts, excluded tags and tags that don't parse are
// dropped, and versions that compare equal ("3.7.12" and "v3.7.12", or one version under two
// tag prefixes) are kept once, as the first one in refs. Order comes from the scheme, never
// from publish dates.
func NewUniverse(ctx context.Context, logger *slog.Logger, refs []Ref, rules Rules) *Universe {
	vs := make([]Version, 0, len(refs))
	for _, r := range refs {
		if r.Draft {
			continue
		}
		if rules.Include != nil && !rules.Include.MatchString(r.Tag) ||
			rules.Exclude != nil && rules.Exclude.MatchString(r.Tag) {
			logger.DebugContext(ctx, "excluded tag", "tag", r.Tag)
			continue
		}
		text, ok := rules.cutPrefix(r.Tag)
		if !ok {
			logger.DebugContext(ctx, "dropped tag without prefix", "tag", r.Tag, "prefixes", rules.TagPrefixes)
			continue
		}
		if text, ok = strings.CutSuffix(text, rules.TagSuffix); !ok || text == "" {
			logger.DebugContext(ctx, "dropped tag without suffix", "tag", r.Tag, "suffix", rules.TagSuffix)
			continue
		}
		v, err := rules.Scheme.Parse(text)
		if err != nil {
			logger.DebugContext(ctx, "dropped unparseable tag", "tag", r.Tag, "error", err)
			continue
		}
		vs = append(vs, Version{
			V:           v,
			Tag:         r.Tag,
			Prerelease:  r.Prerelease || rules.Scheme.IsPrerelease(v),
			PublishedAt: r.PublishedAt,
		})
	}
	slices.SortStableFunc(vs, func(a, b Version) int { return rules.Scheme.Compare(a.V, b.V) })
	vs = slices.CompactFunc(vs, func(a, b Version) bool { return rules.Scheme.Compare(a.V, b.V) == 0 })
	return &Universe{rules: rules, versions: vs}
}

// Versions returns every version, oldest first.
func (u *Universe) Versions() []Version { return u.versions }

// Scheme returns the scheme the universe was parsed with.
func (u *Universe) Scheme() Scheme { return u.rules.Scheme }

// Parse parses user input such as "1.30.0", "v1.30.0", the full tag "atlascli/v1.30.0" or the
// image tag "1.27.0-alpine".
func (u *Universe) Parse(s string) (V, error) {
	if text, ok := u.rules.cutPrefix(s); ok && text != "" {
		text, _ = strings.CutSuffix(text, u.rules.TagSuffix)
		if v, err := u.rules.Scheme.Parse(text); err == nil {
			return v, nil
		}
	}
	if text, ok := strings.CutSuffix(s, u.rules.TagSuffix); ok && text != "" {
		s = text
	}
	return u.rules.Scheme.Parse(s) //nolint:wrapcheck // the scheme's error names the input
}

// Has reports whether the universe holds a version equal to v.
func (u *Universe) Has(v V) bool {
	_, ok := u.find(v)
	return ok
}

// find returns the index of the version equal to v.
func (u *Universe) find(v V) (int, bool) {
	return slices.BinarySearchFunc(u.versions, v, func(e Version, t V) int {
		return u.rules.Scheme.Compare(e.V, t)
	})
}
