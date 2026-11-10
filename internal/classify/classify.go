package classify

import (
	"bytes"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"

	"github.com/sassdavid/relgap/internal/source"
)

// Kind is the category of an entry.
type Kind string

// Entry kinds.
const (
	KindBreaking    Kind = "breaking"
	KindDeprecation Kind = "deprecation"
	KindSecurity    Kind = "security"
	KindFeature     Kind = "feature"
	KindFix         Kind = "fix"
	KindOther       Kind = "other"
)

// Kinds lists every kind in display order, most urgent first.
func Kinds() []Kind {
	return []Kind{KindBreaking, KindDeprecation, KindSecurity, KindFeature, KindFix, KindOther}
}

// Entry is one item of release notes.
type Entry struct {
	Version    string     `json:"version"`
	Kind       Kind       `json:"kind"`
	Confidence float64    `json:"confidence"`
	Text       string     `json:"text"` // Markdown
	Source     source.Ref `json:"source"`
}

const (
	pseudoLevel    = 7 // below every Markdown heading level
	maxPseudoLen   = 60
	maxPseudoWords = 6
)

var (
	anchorTag   = regexp.MustCompile(`<a (?:name|id)="[^"]*">\s*</a>`)
	compareOnly = regexp.MustCompile(`(?i)^\W*full changelog\W*:?\W*\s*\S+/compare/\S+\s*$`)
	linkDest    = regexp.MustCompile(`\]\(([^)\s]+)`)
	urlScheme   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	capsLabel   = regexp.MustCompile(`^[A-Z][A-Z0-9 &/_-]{2,40}:$`) // terraform's "UPGRADE NOTES:"
)

// Classifier turns [source.VersionNotes] into entries.
type Classifier struct {
	parser   parser.Parser
	headings []rule
	verbs    []rule
	keywords []rule
	markers  []*regexp.Regexp
	ignore   []ignoreRule
}

// New returns a classifier with the built-in rules and those in o.
func New(o Options) (*Classifier, error) {
	markers, err := compileMarkers(o.BreakingMarkers)
	if err != nil {
		return nil, err
	}
	return &Classifier{
		parser:   parser.New(),
		headings: headingRules(),
		verbs:    verbRules(),
		keywords: keywordRules(),
		markers:  markers,
		ignore:   compileIgnore(o.IgnoreSections),
	}, nil
}

// Classify returns the kind and confidence of an entry's Markdown text under headings (outermost
// first), taking the first rule that matches in the spec's priority order.
func (c *Classifier) Classify(headings []string, text string) (Kind, float64) {
	if leadIn.MatchString(strings.TrimSpace(text)) {
		return KindOther, ConfidenceOther // "Check these cases:" introduces a list and changes nothing
	}
	if internalCommit(text) {
		return KindOther, confidenceCommit
	}
	for _, m := range c.markers {
		if m.MatchString(text) || slices.ContainsFunc(headings, m.MatchString) {
			return KindBreaking, confidenceMarker
		}
	}
	if kind, conf, ok := c.byHeading(headings, text); ok {
		return kind, conf
	}
	plain := plainStart(text)
	if commitFooter.MatchString(text) {
		return KindBreaking, confidenceBang
	}
	if m := commitPrefix.FindStringSubmatch(plain); m != nil {
		return commitKind(strings.ToLower(m[1]), m[2] == "!")
	}
	if semverMinor.MatchString(text) {
		return KindFeature, confidenceCommit
	}
	if r, ok := firstMatch(c.verbs, plain); ok {
		return r.kind, r.confidence
	}
	if slices.ContainsFunc(headings, highlights.MatchString) {
		return KindOther, confidenceHighlights
	}
	if r, ok := firstMatch(c.keywords, text); ok {
		return r.kind, r.confidence
	}
	return KindOther, ConfidenceOther
}

// byHeading applies the heading rules, deepest heading first.
func (c *Classifier) byHeading(headings []string, text string) (Kind, float64, bool) {
	for _, h := range slices.Backward(headings) {
		r, ok := firstMatch(c.headings, h)
		if !ok {
			continue
		}
		if r.kind == KindBreaking && future.MatchString(text) {
			return KindDeprecation, r.confidence, true
		}
		return r.kind, r.confidence, true
	}
	return "", 0, false
}

func firstMatch(rules []rule, s string) (rule, bool) {
	for _, r := range rules {
		if r.re.MatchString(s) {
			return r, true
		}
	}
	return rule{}, false
}

// internalCommit reports whether text is a node commit line tagged SEMVER-MAJOR whose scopes are
// all internal (V8 updates, tests, build tooling): node tags those major, but users see no change.
func internalCommit(text string) bool {
	m := semverMajorScopes.FindStringSubmatch(text)
	if m == nil {
		return false
	}
	for scope := range strings.SplitSeq(m[1], ",") {
		if !slices.Contains(internalScopes(), strings.TrimSpace(scope)) {
			return false
		}
	}
	return true
}

func commitKind(typ string, bang bool) (Kind, float64) {
	if bang {
		return KindBreaking, confidenceBang
	}
	switch typ {
	case "feat", "feature":
		return KindFeature, confidenceCommit
	case "fix", "bugfix":
		return KindFix, confidenceCommit
	case "security":
		return KindSecurity, confidenceCommit
	default:
		return KindOther, confidenceCommit
	}
}

// plainStart returns the first line of text with a leading commit-link or bracket prefix
// removed, as in node's "\[[`abc`](url)] - " and helm's "[backport] ".
func plainStart(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if strings.HasPrefix(line, `\[[`) {
		if i := strings.Index(line, `)] - `); i >= 0 {
			line = line[i+len(`)] - `):]
		}
	}
	return line
}

// Entries splits n into entries: one per list item and one per run of other blocks. Entries
// under an ignored section are dropped.
func (c *Classifier) Entries(n source.VersionNotes) []Entry {
	w := &walker{c: c, n: n, src: []byte(n.Markdown)}
	w.base, _ = url.Parse(n.Source.URL)
	for b := range c.parser.Parse(w.src).Children() {
		w.block(b)
	}
	w.flush()
	return w.out
}

// walker splits one document into entries.
type walker struct {
	c        *Classifier
	n        source.VersionNotes
	src      []byte
	base     *url.URL
	headings []heading
	run      []ast.Node // consecutive non-list blocks
	out      []Entry
}

func (w *walker) block(b ast.Node) {
	switch node := b.(type) {
	case *ast.Heading:
		w.flush()
		w.headings = push(w.headings, heading{level: node.Level, text: blockText(node, w.src)})
	case *ast.List:
		w.flush()
		for item := range node.Children() {
			li, ok := item.(*ast.ListItem)
			if !ok {
				continue
			}
			text, label := splitLabel(itemText(li, w.src))
			w.add(lineStart(w.src, li.Pos()), text)
			if label != "" {
				w.headings = push(w.headings, heading{level: pseudoLevel, text: label})
			}
		}
	case *ast.ThematicBreak:
		w.flush()
	default:
		if text, ok := pseudoHeading(b, w.src); ok {
			w.flush()
			w.headings = push(w.headings, heading{level: pseudoLevel, text: text})
			return
		}
		w.run = append(w.run, b)
	}
}

func (w *walker) flush() {
	if len(w.run) > 0 {
		start := lineStart(w.src, w.run[0].Pos())
		w.add(start, string(w.src[start:end(w.run[len(w.run)-1], w.src)]))
		w.run = nil
	}
}

// add classifies the entry that starts at offset start, unless it is empty, a compare link or
// under an ignored section.
func (w *walker) add(start int, text string) {
	text = strings.TrimSpace(anchorTag.ReplaceAllString(text, ""))
	if text == "" || compareOnly.MatchString(text) {
		return
	}
	inner := make([]string, len(w.headings))
	for i, h := range w.headings {
		inner[i] = h.text
	}
	path := append(append(make([]string, 0, len(w.n.Source.Heading)+len(inner)), w.n.Source.Heading...), inner...)
	if w.c.ignored(path) {
		return
	}
	kind, conf := w.c.Classify(inner, text)
	ref := w.n.Source
	ref.Heading = path
	if w.n.Line > 0 {
		ref.URL += "#L" + strconv.Itoa(w.n.Line+bytes.Count(w.src[:start], []byte("\n")))
	}
	w.out = append(w.out, Entry{
		Version:    w.n.Version.V.String(),
		Kind:       kind,
		Confidence: conf,
		Text:       absLinks(text, w.base),
		Source:     ref,
	})
}

func (c *Classifier) ignored(path []string) bool {
	for _, r := range c.ignore {
		if r.matches(path) {
			return true
		}
	}
	return false
}

// absLinks resolves relative inline link targets against base, so entries work outside the
// source document.
func absLinks(text string, base *url.URL) string {
	if base == nil || base.Host == "" {
		return text
	}
	return linkDest.ReplaceAllStringFunc(text, func(m string) string {
		dest := m[2:]
		if urlScheme.MatchString(dest) || strings.HasPrefix(dest, "//") {
			return m
		}
		ref, err := url.Parse(dest)
		if err != nil {
			return m
		}
		return "](" + base.ResolveReference(ref).String()
	})
}

type heading struct {
	level int
	text  string
}

// push adds h after dropping headings at its level or deeper.
func push(hs []heading, h heading) []heading {
	for len(hs) > 0 && hs[len(hs)-1].level >= h.level {
		hs = hs[:len(hs)-1]
	}
	return append(hs, h)
}

// splitLabel cuts a trailing all-caps label line off an item. Such a line directly under a
// list item is a lazy continuation in CommonMark, but the author meant a new section.
func splitLabel(text string) (string, string) {
	rest, last, ok := strings.CutLast(strings.TrimRight(text, " \n"), "\n")
	if !ok {
		return text, ""
	}
	if last = strings.TrimSpace(last); capsLabel.MatchString(last) {
		return rest, last
	}
	return text, ""
}

// pseudoHeading recognizes a one-line paragraph used as a heading: a line ending in ":" before a
// list, an all-caps label ("NOTES:"), or a short paragraph that is all bold.
func pseudoHeading(n ast.Node, src []byte) (string, bool) {
	p, ok := n.(*ast.Paragraph)
	if !ok || len(p.Source()) != 1 {
		return "", false
	}
	text := blockText(p, src)
	if _, nextIsList := p.NextSibling().(*ast.List); nextIsList && leadIn.MatchString(text) {
		return text, true
	}
	if len(text) > maxPseudoLen || len(strings.Fields(text)) > maxPseudoWords {
		return "", false
	}
	if _, bold := p.FirstChild().(*ast.Strong); bold && p.ChildCount() == 1 {
		return strings.TrimSpace(strings.Trim(text, "*_")), true
	}
	if capsLabel.MatchString(text) {
		return text, true
	}
	return "", false
}

func blockText(b ast.BlockNode, src []byte) string {
	var sb strings.Builder
	for _, seg := range b.Source() {
		sb.Write(seg.Bytes(src))
	}
	return strings.TrimSpace(sb.String())
}

func lineStart(src []byte, pos int) int {
	if pos < 0 {
		return 0
	}
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

// end returns the offset where n's source ends: the start of the next block or the end of src.
func end(n ast.Node, src []byte) int {
	for cur := n; cur != nil; cur = cur.Parent() {
		if next := cur.NextSibling(); next != nil && next.Pos() >= 0 {
			return lineStart(src, next.Pos())
		}
	}
	return len(src)
}

// itemText returns the item's Markdown without its marker, with continuation lines dedented.
func itemText(li *ast.ListItem, src []byte) string {
	first := li.FirstChild()
	if first == nil || first.Pos() < 0 {
		return ""
	}
	start := first.Pos()
	indent := start - lineStart(src, li.Pos())
	lines := strings.Split(string(src[start:end(li, src)]), "\n")
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		strip := min(len(lines[i])-len(trimmed), indent)
		lines[i] = lines[i][strip:]
	}
	return strings.Join(lines, "\n")
}
