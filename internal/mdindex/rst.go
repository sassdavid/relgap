package mdindex

import (
	"bytes"
	"strings"
)

// overlined is how far the underline of an overlined title is from the overline.
const overlined = 2

// rstAdornment is the set of characters reStructuredText allows in section underlines.
const rstAdornment = "=-~^*+#'\"`:.,_!$%&()/;<>?@[\\]{|}"

// FromRST turns reStructuredText section titles (a line followed by an underline, with an
// optional overline) into ATX headings, so [Parse] can index CHANGES.rst-style changelogs. Levels
// follow the order in which adornment styles first appear, as in docutils. Other markup stays.
func FromRST(src []byte) []byte {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	levels := map[string]int{}
	var out bytes.Buffer
	for i := 0; i < len(lines); i++ {
		over := ""
		title := lines[i]
		next := i + 1
		if isAdornment(title) && i+2 < len(lines) && lines[i+2] == title {
			over, title, next = title, lines[i+1], i+overlined
		}
		text := strings.TrimSpace(title)
		if text == "" || next >= len(lines) || isAdornment(title) {
			out.WriteString(lines[i] + "\n")
			continue
		}
		under := lines[next]
		if !isAdornment(under) || len(strings.TrimRight(under, " ")) < len([]rune(text)) {
			out.WriteString(lines[i] + "\n")
			continue
		}
		style := string(under[0])
		if over != "" {
			style = "over" + style
		}
		level, ok := levels[style]
		if !ok {
			level = len(levels) + 1
			levels[style] = level
		}
		out.WriteString(strings.Repeat("#", min(level, maxLevel)) + " " + text + "\n")
		i = next
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

func isAdornment(line string) bool {
	line = strings.TrimRight(line, " ")
	if len(line) < 3 || !strings.ContainsRune(rstAdornment, rune(line[0])) {
		return false
	}
	return strings.Count(line, line[:1]) == len(line)
}
