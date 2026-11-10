package mdindex

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/sassdavid/relgap/internal/version"
)

// VersionPlaceholder is replaced by a version pattern in heading templates.
const VersionPlaceholder = "{version}"

const (
	versionGroup = "version"
	minVersions  = 2 // version headings that make a heading level the version level
)

// ErrNoScheme is returned by [Parse] when [Options] has no version scheme.
var ErrNoScheme = errors.New("mdindex: no version scheme")

var (
	versionToken = regexp.MustCompile(`^[vV]?\d+(\.\d+)+([-+][0-9A-Za-z.+-]*)?$`)
	anchorLine   = regexp.MustCompile(`(?m)^[ \t]*<a (?:name|id)="[^"]*">[ \t]*</a>[ \t]*\n?`)
	anchorTag    = regexp.MustCompile(`<a (?:name|id)="[^"]*">\s*</a>`)
	compareRef   = regexp.MustCompile(`(?m)^[ \t]*\[[^\]]+\]:[ \t]*https?://\S+/compare/\S+[ \t]*\n?`)
)

// Options says how version headings are found.
type Options struct {
	Scheme  version.Scheme
	Heading *regexp.Regexp // from [HeadingPattern]; nil means detect the version heading level
}

// HeadingPattern compiles a registry heading template such as `^v{version}$`. The template is a
// regular expression in which {version} stands for the version.
func HeadingPattern(tmpl string) (*regexp.Regexp, error) {
	if !strings.Contains(tmpl, VersionPlaceholder) {
		return nil, fmt.Errorf("heading template %q has no %s", tmpl, VersionPlaceholder)
	}
	expr := strings.Replace(tmpl, VersionPlaceholder, `(?P<`+versionGroup+`>v?\d[0-9A-Za-z.+-]*)`, 1)
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("compile heading template %q: %w", tmpl, err)
	}
	return re, nil
}

// Section is the body of one version heading.
type Section struct {
	Version     version.V
	Heading     []string // enclosing headings, then the version heading
	HeadingLine int      // 1-based line of the version heading
	Line        int      // 1-based line where the body starts
	Start, End  int      // byte range of the body
}

// Index is a parsed changelog with its version sections in document order.
type Index struct {
	src      []byte
	scheme   version.Scheme
	sections []Section
	lines    []int // offsets of each '\n'
}

type heading struct {
	level     int
	text      string
	lineStart int // offset of the heading's first line
	bodyStart int // offset after the heading's last line
	version   version.V
}

// Parse indexes src. CRLF line endings become LF first.
func Parse(src []byte, o Options) (*Index, error) {
	if o.Scheme == nil {
		return nil, ErrNoScheme
	}
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	x := &Index{src: src, scheme: o.Scheme}
	for i, b := range src {
		if b == '\n' {
			x.lines = append(x.lines, i)
		}
	}
	hs := headings(parser.New().Parse(src), src)
	for i := range hs {
		hs[i].version = x.versionOf(hs[i].text, o.Heading)
	}
	isVersion := func(h heading) bool { return !h.version.IsZero() }
	if o.Heading == nil {
		level := versionLevel(hs)
		isVersion = func(h heading) bool { return h.level == level && !h.version.IsZero() }
	}
	x.build(hs, isVersion)
	return x, nil
}

// build turns the version headings of hs into sections. A section ends at the next heading of
// the same or a higher level; the headings above it form its path.
func (x *Index) build(hs []heading, isVersion func(heading) bool) {
	var stack []heading
	for i, h := range hs {
		for len(stack) > 0 && stack[len(stack)-1].level >= h.level {
			stack = stack[:len(stack)-1]
		}
		if !isVersion(h) {
			stack = append(stack, h)
			continue
		}
		end := len(x.src)
		for _, next := range hs[i+1:] {
			if next.level <= h.level {
				end = next.lineStart
				break
			}
		}
		path := make([]string, 0, len(stack)+1)
		for _, a := range stack {
			path = append(path, a.text)
		}
		x.sections = append(x.sections, Section{
			Version:     h.version,
			Heading:     append(path, h.text),
			HeadingLine: x.line(h.lineStart),
			Line:        x.line(h.bodyStart),
			Start:       h.bodyStart,
			End:         max(end, h.bodyStart),
		})
	}
}

// Sections returns every version section in document order.
func (x *Index) Sections() []Section { return x.sections }

// Lookup returns the first section whose version equals v.
func (x *Index) Lookup(v version.V) (Section, bool) {
	for _, s := range x.sections {
		if x.scheme.Compare(s.Version, v) == 0 {
			return s, true
		}
	}
	return Section{}, false
}

// Markdown returns the body of s without HTML anchors and compare-link definitions, trimmed of
// blank lines, and the 1-based line it starts on.
func (x *Index) Markdown(s Section) (string, int) {
	body := string(x.src[s.Start:s.End])
	line := s.Line
	for body != "" && (body[0] == '\n' || anchorLine.MatchString(firstLine(body))) {
		i := strings.IndexByte(body, '\n')
		if i < 0 {
			body = ""
			break
		}
		body = body[i+1:]
		line++
	}
	body = anchorLine.ReplaceAllString(body, "")
	body = compareRef.ReplaceAllString(body, "")
	body = strings.TrimRight(body, " \t\n")
	if body == "" {
		return "", line
	}
	return body + "\n", line
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i+1]
	}
	return s
}

// line returns the 1-based line of offset off.
func (x *Index) line(off int) int {
	return sort.SearchInts(x.lines, off) + 1
}

// versionOf returns the version a heading names, or the zero V.
func (x *Index) versionOf(text string, re *regexp.Regexp) version.V {
	if re != nil {
		m := re.FindStringSubmatch(text)
		if m == nil {
			return version.V{}
		}
		return x.parse(m[re.SubexpIndex(versionGroup)])
	}
	// Link syntax is dropped first, as Renovate does, so "[1.2.0](url)" yields 1.2.0.
	words := strings.FieldsSeq(strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ").Replace(text))
	for w := range words {
		w = strings.Trim(w, ",;:'\"")
		if versionToken.MatchString(w) {
			if v := x.parse(w); !v.IsZero() {
				return v
			}
		}
	}
	return version.V{}
}

func (x *Index) parse(s string) version.V {
	v, err := x.scheme.Parse(strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V"))
	if err != nil {
		return version.V{}
	}
	return v
}

// versionLevel returns the shallowest heading level with at least 2 version headings, or the
// shallowest with one.
func versionLevel(hs []heading) int {
	var counts [7]int
	for _, h := range hs {
		if !h.version.IsZero() {
			counts[h.level]++
		}
	}
	for l := 1; l <= 6; l++ {
		if counts[l] >= minVersions {
			return l
		}
	}
	for l := 1; l <= 6; l++ {
		if counts[l] == 1 {
			return l
		}
	}
	return 0
}

// headings lists the document-level headings; headings in fences, HTML blocks and containers
// are not headings of the changelog.
func headings(doc ast.Node, src []byte) []heading {
	var out []heading
	for n := range doc.Children() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		lineStart := bytes.LastIndexByte(src[:max(h.Pos(), 0)], '\n') + 1
		segs := h.Source()
		last := lineStart
		var sb strings.Builder
		for _, s := range segs {
			sb.Write(s.Bytes(src))
			last = s.Stop
		}
		bodyStart := lineEnd(src, max(last-1, lineStart))
		if h.HeadingKind == ast.HeadingKindSetext {
			bodyStart = lineEnd(src, bodyStart)
		}
		out = append(out, heading{
			level:     h.Level,
			text:      strings.Join(strings.Fields(anchorTag.ReplaceAllString(sb.String(), "")), " "),
			lineStart: lineStart,
			bodyStart: bodyStart,
		})
	}
	return out
}

// lineEnd returns the offset after the newline that ends the line containing off.
func lineEnd(src []byte, off int) int {
	if off >= len(src) {
		return len(src)
	}
	if i := bytes.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(src)
}
