package purl

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
)

// PURL is a package URL: pkg:type/namespace/name@version?qualifiers#subpath.
type PURL struct {
	Type       string
	Namespace  string // slash-separated segments, decoded
	Name       string
	Version    string
	Qualifiers map[string]string
	Subpath    string // slash-separated segments, decoded
}

const scheme = "pkg:"

// String returns the canonical form: each segment percent-encoded, qualifiers sorted by key.
func (p PURL) String() string {
	var b strings.Builder
	b.WriteString(scheme + strings.ToLower(p.Type) + "/")
	if p.Namespace != "" {
		b.WriteString(escapeSegments(p.Namespace) + "/")
	}
	b.WriteString(escape(p.Name))
	if p.Version != "" {
		b.WriteString("@" + escape(p.Version))
	}
	if len(p.Qualifiers) > 0 {
		keys := slices.Sorted(maps.Keys(p.Qualifiers))
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, strings.ToLower(k)+"="+escape(p.Qualifiers[k]))
		}
		b.WriteString("?" + strings.Join(parts, "&"))
	}
	if p.Subpath != "" {
		b.WriteString("#" + escapeSegments(p.Subpath))
	}
	return b.String()
}

// Parse reads a package URL. Segments are decoded, so pkg:golang/github.com%2Fa%2Fb and
// pkg:golang/github.com/a/b name the same package.
func Parse(s string) (PURL, error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(s), scheme)
	if !ok {
		return PURL{}, fmt.Errorf("parse purl %q: want the pkg: scheme", s)
	}
	rest = strings.TrimLeft(rest, "/")
	var p PURL
	var err error
	if before, sub, found := strings.Cut(rest, "#"); found {
		rest = before
		if p.Subpath, err = unescapeSegments(sub); err != nil {
			return PURL{}, fmt.Errorf("parse purl %q: %w", s, err)
		}
	}
	if before, query, found := strings.Cut(rest, "?"); found {
		rest = before
		if p.Qualifiers, err = parseQualifiers(query); err != nil {
			return PURL{}, fmt.Errorf("parse purl %q: %w", s, err)
		}
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		if p.Version, err = url.PathUnescape(rest[i+1:]); err != nil {
			return PURL{}, fmt.Errorf("parse purl %q: %w", s, err)
		}
		rest = rest[:i]
	}
	typ, path, found := strings.Cut(rest, "/")
	if !found || typ == "" {
		return PURL{}, fmt.Errorf("parse purl %q: want pkg:<type>/<name>", s)
	}
	p.Type = strings.ToLower(typ)
	segs, err := unescapeAll(strings.Split(strings.Trim(path, "/"), "/"))
	if err != nil {
		return PURL{}, fmt.Errorf("parse purl %q: %w", s, err)
	}
	p.Name = segs[len(segs)-1]
	p.Namespace = strings.Join(segs[:len(segs)-1], "/")
	if p.Name == "" {
		return PURL{}, fmt.Errorf("parse purl %q: %w", s, errNoName)
	}
	return p, nil
}

var errNoName = errors.New("no name")

// path returns the namespace and name as one slash-separated path, which a percent-encoded
// slash in either can move between them.
func (p PURL) path() string {
	if p.Namespace == "" {
		return p.Name
	}
	return p.Namespace + "/" + p.Name
}

// Covers reports whether p, as a VEX statement names a product, covers c: the same type and
// path ignoring case, the same version unless p sets none (a leading v doesn't count), the same
// subpath unless p sets none, and every qualifier of p on c with the same value.
func (p PURL) Covers(c PURL) bool {
	if !strings.EqualFold(p.Type, c.Type) || !strings.EqualFold(p.path(), c.path()) {
		return false
	}
	if p.Version != "" && trimV(p.Version) != trimV(c.Version) {
		return false
	}
	if p.Subpath != "" && !strings.EqualFold(strings.Trim(p.Subpath, "/"), strings.Trim(c.Subpath, "/")) {
		return false
	}
	for k, v := range p.Qualifiers {
		if got, ok := c.Qualifiers[strings.ToLower(k)]; !ok || got != v {
			return false
		}
	}
	return true
}

func trimV(v string) string { return strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V") }

func parseQualifiers(query string) (map[string]string, error) {
	out := map[string]string{}
	for pair := range strings.SplitSeq(query, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		val, err := url.PathUnescape(v)
		if err != nil {
			return nil, fmt.Errorf("qualifier %s: %w", k, err)
		}
		if val != "" {
			out[strings.ToLower(k)] = val
		}
	}
	return out, nil
}

// escape percent-encodes everything but the unreserved characters of RFC 3986 and the colon,
// which the purl spec leaves as is.
func escape(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
			c == '-' || c == '.' || c == '_' || c == '~' || c == ':' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func escapeSegments(s string) string {
	segs := strings.Split(strings.Trim(s, "/"), "/")
	for i, seg := range segs {
		segs[i] = escape(seg)
	}
	return strings.Join(segs, "/")
}

func unescapeSegments(s string) (string, error) {
	segs, err := unescapeAll(strings.Split(strings.Trim(s, "/"), "/"))
	if err != nil {
		return "", err
	}
	return strings.Join(segs, "/"), nil
}

func unescapeAll(segs []string) ([]string, error) {
	out := make([]string, len(segs))
	for i, seg := range segs {
		u, err := url.PathUnescape(seg)
		if err != nil {
			return nil, fmt.Errorf("segment %q: %w", seg, err)
		}
		out[i] = u
	}
	return out, nil
}
