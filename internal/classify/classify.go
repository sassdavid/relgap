package classify

import (
	"bytes"
	"regexp"
	"slices"
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

// ConfidenceOther is the confidence of an entry no rule matched.
const ConfidenceOther = 0.3

const (
	pseudoLevel      = 7 // below every Markdown heading level
	maxPseudoLen     = 60
	maxPseudoWords   = 6
	confidenceStrong = 0.95
	confidenceMedium = 0.9
)

type headingRule struct {
	re         *regexp.Regexp
	kind       Kind
	confidence float64
}

// headingRules are tried in order; the first match wins.
func headingRules() []headingRule {
	return []headingRule{
		{
			regexp.MustCompile(`(?i)\bbreaking\b|\bupgrade notes\b|\bcompatibility notes\b|\x{26A0}|\bsemver-major\b`),
			KindBreaking, confidenceStrong,
		},
		{regexp.MustCompile(`(?i)\bdeprecat(ed|ions?)\b`), KindDeprecation, confidenceMedium},
		{regexp.MustCompile(`(?i)\bremov(ed|als?)\b`), KindBreaking, confidenceMedium},
		{regexp.MustCompile(`(?i)\bsecurity\b|\bCVE-\d`), KindSecurity, confidenceStrong},
		{regexp.MustCompile(`(?i)\bfeatures?\b|\benhancements?\b|\badded\b|\bnew\b`), KindFeature, confidenceMedium},
		{regexp.MustCompile(`(?i)\bbug ?fix(es)?\b|\bfixe[sd]\b`), KindFix, confidenceMedium},
	}
}

// Classifier turns [source.VersionNotes] into entries.
type Classifier struct {
	parser parser.Parser
	rules  []headingRule
}

// New returns a classifier with the heading rules.
func New() *Classifier {
	return &Classifier{parser: parser.New(), rules: headingRules()}
}

// Entries splits n into entries: one per list item and one per run of other blocks, each
// classified by the deepest heading above it that matches a rule.
func (c *Classifier) Entries(n source.VersionNotes) []Entry {
	src := []byte(n.Markdown)
	doc := c.parser.Parse(src)
	var (
		out      []Entry
		headings []heading
		run      []ast.Node // consecutive non-list blocks
	)
	add := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		kind, conf := c.kind(headings)
		ref := n.Source
		ref.Heading = make([]string, len(headings))
		for i, h := range headings {
			ref.Heading[i] = h.text
		}
		out = append(out, Entry{Version: n.Version.V.String(), Kind: kind, Confidence: conf, Text: text, Source: ref})
	}
	flush := func() {
		if len(run) > 0 {
			add(string(src[run[0].Pos():end(run[len(run)-1], src)]))
			run = nil
		}
	}
	for b := range doc.Children() {
		switch node := b.(type) {
		case *ast.Heading:
			flush()
			headings = push(headings, heading{level: node.Level, text: blockText(node, src)})
		case *ast.List:
			flush()
			for item := range node.Children() {
				if li, ok := item.(*ast.ListItem); ok {
					add(itemText(li, src))
				}
			}
		case *ast.ThematicBreak:
			flush()
		default:
			if text, ok := pseudoHeading(b, src); ok {
				flush()
				headings = push(headings, heading{level: pseudoLevel, text: text})
				continue
			}
			run = append(run, b)
		}
	}
	flush()
	return out
}

func (c *Classifier) kind(headings []heading) (Kind, float64) {
	for _, h := range slices.Backward(headings) {
		for _, r := range c.rules {
			if r.re.MatchString(h.text) {
				return r.kind, r.confidence
			}
		}
	}
	return KindOther, ConfidenceOther
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

// pseudoHeading recognizes a short one-line paragraph used as a heading: "BREAKING CHANGES:"
// followed by a list, or a paragraph that is all bold ("**Bug fixes**").
func pseudoHeading(n ast.Node, src []byte) (string, bool) {
	p, ok := n.(*ast.Paragraph)
	if !ok || len(p.Source()) != 1 {
		return "", false
	}
	text := blockText(p, src)
	if len(text) > maxPseudoLen || len(strings.Fields(text)) > maxPseudoWords {
		return "", false
	}
	if _, bold := p.FirstChild().(*ast.Strong); bold && p.ChildCount() == 1 {
		return strings.TrimSpace(strings.Trim(text, "*_")), true
	}
	if _, nextIsList := p.NextSibling().(*ast.List); nextIsList && strings.HasSuffix(text, ":") {
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

// end returns the offset where n's source ends: the start of the next block or the end of src.
func end(n ast.Node, src []byte) int {
	for cur := n; cur != nil; cur = cur.Parent() {
		if next := cur.NextSibling(); next != nil && next.Pos() >= 0 {
			return next.Pos()
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
	lineStart := bytes.LastIndexByte(src[:li.Pos()], '\n') + 1
	indent := start - lineStart
	lines := strings.Split(string(src[start:end(li, src)]), "\n")
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		strip := min(len(lines[i])-len(trimmed), indent)
		lines[i] = lines[i][strip:]
	}
	return strings.Join(lines, "\n")
}
