package mdindex

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/v2/parser"
)

const maxLevel = 6

// Demote returns src with each ATX heading one level deeper, up to level 6, so the document can
// sit under a new top heading. Headings in code fences are left alone.
func Demote(src []byte) []byte {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	var out bytes.Buffer
	prev := 0
	for _, h := range headings(parser.New().Parse(src), src) {
		line := src[h.lineStart:]
		trimmed := bytes.TrimLeft(line, " ")
		if h.level >= maxLevel || len(trimmed) == 0 || trimmed[0] != '#' {
			continue
		}
		at := h.lineStart + len(line) - len(trimmed)
		out.Write(src[prev:at])
		out.WriteByte('#')
		prev = at
	}
	out.Write(src[prev:])
	return out.Bytes()
}

// Subsection returns the body of the first heading match accepts, up to the next heading of the
// same or a higher level, that heading's text and the 0-based line of src where the body starts.
// The last result is false when no heading matches.
func Subsection(src []byte, match func(heading string) bool) (string, string, int, bool) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	hs := headings(parser.New().Parse(src), src)
	for i, h := range hs {
		if !match(h.text) {
			continue
		}
		end := len(src)
		for _, next := range hs[i+1:] {
			if next.level <= h.level {
				end = next.lineStart
				break
			}
		}
		raw := string(src[h.bodyStart:max(end, h.bodyStart)])
		body := strings.TrimLeft(raw, "\n")
		line := bytes.Count(src[:h.bodyStart], []byte("\n")) + len(raw) - len(body)
		body = strings.TrimRight(body, "\n")
		if body != "" {
			body += "\n"
		}
		return body, h.text, line, true
	}
	return "", "", 0, false
}
