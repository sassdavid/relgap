package htmlmd

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// ErrSelector is wrapped by every selector parse error.
var ErrSelector = errors.New("invalid selector")

// Selector matches elements by tag, id and classes, such as div.body, a.headerlink or #main. It
// has no combinators: one selector names one element.
type Selector struct {
	tag     string
	id      string
	classes []string
}

var selectorSyntax = regexp.MustCompile(`^([a-z][a-z0-9]*)?((?:[#.][A-Za-z_][A-Za-z0-9_-]*)*)$`)

// ParseSelector parses tag, #id and .class parts written together without spaces.
func ParseSelector(s string) (Selector, error) {
	m := selectorSyntax.FindStringSubmatch(s)
	if s == "" || m == nil {
		return Selector{}, fmt.Errorf("%w %q: want tag, #id and .class parts without spaces, such as div.body",
			ErrSelector, s)
	}
	sel := Selector{tag: m[1]}
	rest := m[2]
	for rest != "" {
		end := strings.IndexAny(rest[1:], "#.") + 1
		if end == 0 {
			end = len(rest)
		}
		part := rest[1:end]
		if rest[0] == '#' {
			if sel.id != "" {
				return Selector{}, fmt.Errorf("%w %q: more than one #id", ErrSelector, s)
			}
			sel.id = part
		} else {
			sel.classes = append(sel.classes, part)
		}
		rest = rest[end:]
	}
	return sel, nil
}

// Match reports whether n is an element the selector names.
func (s Selector) Match(n *html.Node) bool {
	if n.Type != html.ElementNode || (s.tag != "" && n.Data != s.tag) {
		return false
	}
	if s.id != "" && attr(n, "id") != s.id {
		return false
	}
	have := strings.Fields(attr(n, "class"))
	for _, c := range s.classes {
		if !slices.Contains(have, c) {
			return false
		}
	}
	return true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

// find returns the first element under n, n included, that match accepts, in document order.
func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, match); f != nil {
			return f
		}
	}
	return nil
}

// removeAll detaches every element under n that match accepts.
func removeAll(n *html.Node, match func(*html.Node) bool) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if match(c) {
			n.RemoveChild(c)
		} else {
			removeAll(c, match)
		}
		c = next
	}
}
