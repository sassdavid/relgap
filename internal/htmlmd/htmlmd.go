package htmlmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// Options selects and cleans the part of a page to convert.
type Options struct {
	Content string   // selector of the element to convert; empty means main, else body
	Strip   []string // selectors of elements to drop before converting
}

func (o Options) compile() (*Selector, []Selector, error) {
	var content *Selector
	if o.Content != "" {
		s, err := ParseSelector(o.Content)
		if err != nil {
			return nil, nil, err
		}
		content = &s
	}
	strip := make([]Selector, 0, len(o.Strip))
	for _, raw := range o.Strip {
		s, err := ParseSelector(raw)
		if err != nil {
			return nil, nil, err
		}
		strip = append(strip, s)
	}
	return content, strip, nil
}

// dropped reports whether an element of this tag never holds release notes.
func dropped(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template", "iframe", "form", "button", "svg", "nav":
		return true
	default:
		return false
	}
}

// Document is a parsed HTML page. Its methods don't change it, so one page can be converted in
// parts.
type Document struct {
	root *html.Node
	url  string
}

// Parse parses an HTML page fetched from pageURL, which makes relative links absolute.
func Parse(src []byte, pageURL string) (*Document, error) {
	root, err := html.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse html of %s: %w", pageURL, err)
	}
	return &Document{root: root, url: pageURL}, nil
}

// Markdown converts the content element of the page.
func (d *Document) Markdown(ctx context.Context, o Options) (string, error) {
	content, strip, err := o.compile()
	if err != nil {
		return "", err
	}
	var el *html.Node
	if content != nil {
		el = find(d.root, content.Match)
		if el == nil {
			return "", fmt.Errorf("convert %s: no element matches %q", d.url, o.Content)
		}
	} else {
		el = find(d.root, isTag("main"))
		if el == nil {
			el = find(d.root, isTag("body"))
		}
		if el == nil {
			el = d.root
		}
	}
	return d.convert(ctx, clone(el), strip)
}

// Anchor converts the element whose id is id. For a heading that is the heading and the
// siblings after it up to the next heading of the same or a higher level. The second result is
// false when the page has no such element.
func (d *Document) Anchor(ctx context.Context, id string, o Options) (string, bool, error) {
	_, strip, err := o.compile()
	if err != nil {
		return "", false, err
	}
	el := find(d.root, func(n *html.Node) bool { return n.Type == html.ElementNode && attr(n, "id") == id })
	if el == nil {
		return "", false, nil
	}
	level := headingLevel(el)
	if level == 0 {
		md, convErr := d.convert(ctx, clone(el), strip)
		return md, true, convErr
	}
	wrap := &html.Node{Type: html.ElementNode, Data: "div"}
	wrap.AppendChild(clone(el))
	for s := el.NextSibling; s != nil; s = s.NextSibling {
		if l := headingLevel(s); l > 0 && l <= level {
			break
		}
		wrap.AppendChild(clone(s))
	}
	md, err := d.convert(ctx, wrap, strip)
	return md, true, err
}

func (d *Document) convert(ctx context.Context, el *html.Node, strip []Selector) (string, error) {
	removeAll(el, func(n *html.Node) bool {
		if n.Type == html.CommentNode || (n.Type == html.ElementNode && dropped(n.Data)) {
			return true
		}
		for _, s := range strip {
			if s.Match(n) {
				return true
			}
		}
		return false
	})
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
		strikethrough.NewStrikethroughPlugin(),
	))
	out, err := conv.ConvertNode(el, converter.WithContext(ctx), converter.WithDomain(d.url))
	if err != nil {
		return "", fmt.Errorf("convert %s to markdown: %w", d.url, err)
	}
	md := strings.TrimSpace(string(out))
	if md == "" {
		return "", nil
	}
	return md + "\n", nil
}

func isTag(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == tag }
}

func headingLevel(n *html.Node) int {
	if n.Type != html.ElementNode || len(n.Data) != 2 || n.Data[0] != 'h' || n.Data[1] < '1' || n.Data[1] > '6' {
		return 0
	}
	return int(n.Data[1] - '0')
}

// clone copies n and its descendants, detached from any parent.
func clone(n *html.Node) *html.Node {
	c := &html.Node{
		Type:      n.Type,
		DataAtom:  n.DataAtom,
		Data:      n.Data,
		Namespace: n.Namespace,
		Attr:      append([]html.Attribute(nil), n.Attr...),
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.AppendChild(clone(ch))
	}
	return c
}
