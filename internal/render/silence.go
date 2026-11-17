package render

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

// zeroWidth goes after an @ or # that would start a GitHub mention or issue reference. It renders
// as nothing, so the reader sees the original text.
const zeroWidth = "&#8203;"

var (
	// mdURL matches a URL as GitHub's autolink extension finds it, and a link destination.
	mdURL = regexp.MustCompile("(?i)(?:https?://|www\\.)[^\\s<>\"'()\\[\\]`]+|\\]\\([^)\\s]*")
	// ghRef matches a github.com issue, pull request or discussion URL, which GitHub turns into
	// a backlink on that issue.
	ghRef = regexp.MustCompile(
		`(?i)^(?:https?://)?(?:www\.)?github\.com(/[^/\s]+/[^/\s]+/(?:issues|pull|discussions)/\d+\S*)$`,
	)
)

// silence returns md with every GitHub mention and issue reference broken by a zero-width space,
// and with issue, pull request and discussion links sent through redirect.github.com. A comment
// that quotes release notes then notifies nobody and adds no backlink to the upstream issues.
// Code spans and code blocks stay as they are.
func silence(md string) string {
	var spans []span
	for _, r := range codeRanges([]byte(md)) {
		spans = append(spans, span{r.Start, r.Stop, false})
	}
	for _, loc := range mdURL.FindAllStringIndex(md, -1) {
		spans = append(spans, span{loc[0], loc[1], true})
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })

	var b strings.Builder
	b.Grow(len(md))
	next := 0
	for i := 0; i < len(md); {
		for next < len(spans) && spans[next].stop <= i {
			next++
		}
		if s := spans[next:]; len(s) > 0 && s[0].start <= i {
			if s[0].url && s[0].start == i {
				b.WriteString(linkURL(md, s[0].start, s[0].stop))
			} else {
				b.WriteString(md[i:s[0].stop])
			}
			i = s[0].stop
			continue
		}
		b.WriteByte(md[i])
		i++
		if opensReference(md, i) {
			b.WriteString(zeroWidth)
		}
	}
	return b.String()
}

// opensReference reports whether md[:i] ends with an @, # or GH- that GitHub reads as the start
// of a mention or an issue reference of the text at i.
func opensReference(md string, i int) bool {
	const gh = "gh-"
	switch c := md[i-1]; {
	case c == '@':
		return !wordBefore(md, i-1) && alnumAt(md, i)
	case c == '#':
		return (i < 2 || md[i-2] != '&') && digitAt(md, i) // not an entity such as &#123;
	case c == '-' && i >= len(gh):
		return strings.EqualFold(md[i-len(gh):i], gh) && !wordBefore(md, i-len(gh)) && digitAt(md, i)
	}
	return false
}

// span is a part of the Markdown that silence leaves alone: code, or a URL it may redirect.
type span struct {
	start, stop int
	url         bool
}

// linkURL returns the URL or link destination md[start:stop] with a github.com issue link sent
// through redirect.github.com. A bare issue link becomes a Markdown link that keeps the URL as
// its text, as GitHub would otherwise show it.
func linkURL(md string, start, stop int) string {
	raw := md[start:stop]
	if dest, ok := strings.CutPrefix(raw, "]("); ok {
		return "](" + redirect(dest)
	}
	trimmed := strings.TrimRight(raw, ".,;:!?*_~")
	to := redirect(trimmed)
	switch {
	case to == trimmed:
		return raw
	case start == 0 || strings.ContainsRune(" \t\n(", rune(md[start-1])):
		return "[" + trimmed + "](" + to + ")" + raw[len(trimmed):]
	default: // an autolink, link text or an HTML attribute
		return to + raw[len(trimmed):]
	}
}

// redirect returns u with redirect.github.com as its host when u is a github.com issue, pull
// request or discussion URL, and u otherwise.
func redirect(u string) string {
	m := ghRef.FindStringSubmatch(u)
	if m == nil {
		return u
	}
	return "https://redirect.github.com" + m[1]
}

// codeRanges returns the byte ranges of the code spans and code block lines of src.
func codeRanges(src []byte) []text.Index {
	var out []text.Index
	_ = ast.Walk(parser.New().Parse(src), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.CodeSpan:
			if !n.Value.IsOwned() {
				out = append(out, n.Value.Indices()...)
			}
		case *ast.CodeBlock:
			for _, seg := range n.Value.Segments() {
				out = append(out, text.NewIndexFromSegment(seg))
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// wordBefore reports whether the rune before md[i] is a letter, a digit or an underscore.
func wordBefore(md string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(md[:i])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func alnumAt(md string, i int) bool {
	return i < len(md) && (digitAt(md, i) || md[i] >= 'a' && md[i] <= 'z' || md[i] >= 'A' && md[i] <= 'Z')
}

func digitAt(md string, i int) bool { return i < len(md) && md[i] >= '0' && md[i] <= '9' }
