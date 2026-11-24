package render

import (
	"html"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

// zeroWidth goes after an @, # or GH- that would start a GitHub mention or issue reference. It
// renders as nothing, so the reader sees the original text.
const zeroWidth = "&#8203;"

// domain matches a domain as GitHub's autolink extension accepts it.
const domain = `(?:[a-z0-9_-]+\.)*[a-z0-9-]+\.[a-z0-9-]+`

var (
	// mdURL matches a URL that GitHub's autolink extension turns into a link, and such a URL as a
	// link destination. Like GitHub, it needs a valid domain: one with a period and no underscore
	// in its last two segments. https://localhost/@x stays text on GitHub, so silence scans it.
	mdURL = regexp.MustCompile("(?i)(?:https?://|www\\.)" + domain + "[^\\s<>\"'()\\[\\]`]*|" +
		"\\]\\((?:https?://|www\\.)" + domain + "[^)\\s]*")
	// ghRef matches a github.com issue, pull request or discussion URL, which GitHub turns into
	// a backlink on that issue.
	ghRef = regexp.MustCompile(
		`(?i)^(?:https?://)?(?:www\.)?github\.com(/[^/\s]+/[^/\s]+/(?:issues|pull|discussions)/\d+\S*)$`,
	)
	// charRef matches a character reference that ends with a semicolon, which Markdown and HTML
	// both decode.
	charRef = regexp.MustCompile(`^&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});`)
)

// silence returns md with every GitHub mention and issue reference broken by a zero-width space,
// and with issue, pull request and discussion links sent through redirect.github.com. A comment
// that quotes release notes then notifies nobody and adds no backlink to the upstream issues.
// Code spans and code blocks stay as they are; they're found as GitHub finds them, after it
// splits table rows into cells.
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
	var last history
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
			last = history{}
			continue
		}
		u := unitAt(md, i)
		b.WriteString(md[i : i+u.size])
		i += u.size
		copy(last[:], last[1:])
		last[len(last)-1] = u
		if opensReference(&last, unitAt(md, i)) {
			b.WriteString(zeroWidth)
		}
	}
	return b.String()
}

// unit is one character of the Markdown: a byte, a UTF-8 rune or a character reference.
type unit struct {
	r    rune // the character, decoded when it's a character reference
	size int  // its length in the Markdown
	word bool // an ASCII letter or digit written as itself, which no Markdown syntax uses
}

// unitAt returns the unit that starts at md[i], or a zero unit at the end of md.
func unitAt(md string, i int) unit {
	if i >= len(md) {
		return unit{}
	}
	if md[i] == '&' {
		if ref := charRef.FindString(md[i:]); ref != "" {
			if dec := html.UnescapeString(ref); dec != ref {
				r, _ := utf8.DecodeRuneInString(dec)
				return unit{r: r, size: len(ref)}
			}
		}
	}
	r, size := utf8.DecodeRuneInString(md[i:])
	return unit{r: r, size: size, word: asciiAlnum(r)}
}

// history holds the units before the current position, the latest last. A zero unit stands for
// the start of the text or the end of a span silence skipped.
type history [len("/discussions/")]unit

// opensReference reports whether last, the units before next, end with an @, #, GH- or issue
// path that GitHub reads as the start of a mention or an issue reference. GitHub skips an @ or GH-
// only after an ASCII letter, digit or underscore, reads o/r/issues/1 as a reference after any
// host or none, and decodes character references first. An underscore can be emphasis, after
// which GitHub does link, so only a letter or digit counts.
func opensReference(last *history, next unit) bool {
	n := len(last)
	switch last[n-1].r {
	case '@':
		return !last[n-2].word && asciiAlnum(next.r)
	case '#':
		// &#x40 without a semicolon is still @ in HTML, so break any hex reference.
		return asciiDigit(next.r) || last[n-2].r == '&' && (next.r == 'x' || next.r == 'X')
	case '-':
		return last.endsWith("gh-") && !last[n-4].word && asciiDigit(next.r)
	case '/':
		return asciiDigit(next.r) &&
			(last.endsWith("/issues/") || last.endsWith("/pull/") || last.endsWith("/discussions/"))
	}
	return false
}

// endsWith reports whether the latest units spell s, ignoring ASCII case.
func (h *history) endsWith(s string) bool {
	off := len(h) - len(s)
	for i := range len(s) {
		r := h[off+i].r
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if r != rune(s[i]) {
			return false
		}
	}
	return true
}

// mdCode returns s as a Markdown code span when silence would change it, such as the name
// npm:@types/node: GitHub reads no mention or reference in code, and the name copies as written.
// An s with a backtick can't be a simple code span, so it comes back silenced.
func mdCode(s string) string {
	silenced := silence(s)
	switch {
	case silenced == s:
		return s
	case strings.Contains(s, "`"):
		return silenced
	}
	return "`" + s + "`"
}

// plain returns s; it's the text formats' counterpart of [mdCode].
func plain(s string) string { return s }

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
	p := parser.New(parser.WithExtensions(extension.NewTableParser()))
	_ = ast.Walk(p.Parse(src), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
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

func asciiAlnum(r rune) bool { return asciiDigit(r) || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

func asciiDigit(r rune) bool { return r >= '0' && r <= '9' }
