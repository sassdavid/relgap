package scan

import (
	"path"
	"regexp"
	"strings"
)

// isDockerfile matches Dockerfile, Dockerfile.*, *.Dockerfile, *.dockerfile and Containerfile*.
func isDockerfile(rel string) bool {
	base := path.Base(rel)
	lower := strings.ToLower(base)
	return strings.HasPrefix(base, "Dockerfile") || strings.HasPrefix(base, "Containerfile") ||
		strings.HasSuffix(lower, ".dockerfile")
}

// instruction is one Dockerfile instruction with continuation lines joined.
type instruction struct {
	line     int // first line
	cmd      string
	args     string
	comments []string // the comment lines right above it, in order
}

var (
	parserDirective = regexp.MustCompile(`^#\s*([A-Za-z]+)\s*=\s*(\S+)\s*$`)
	heredocStart    = regexp.MustCompile(`<<(-?)["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)
	envNameVersion  = regexp.MustCompile(`(?i)^[a-z0-9_]+_VERSION$`)
	dockerVar       = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
)

type heredoc struct {
	word      string
	stripTabs bool // <<-WORD
}

// dockerParser splits a Dockerfile into instructions. It honours the escape directive,
// continuation lines and comments inside them, and skips heredoc bodies.
type dockerParser struct {
	escape     byte
	directives bool // still at the top, where parser directives may appear
	out        []instruction
	comments   []string
	cur        *instruction
	heredocs   []heredoc
}

func parseDockerfile(data []byte) []instruction {
	p := dockerParser{escape: '\\', directives: true}
	for i, raw := range lines(data) {
		p.line(i+1, raw)
	}
	if p.cur != nil {
		p.out = append(p.out, *p.cur)
	}
	return p.out
}

func (p *dockerParser) line(n int, raw string) {
	if len(p.heredocs) > 0 {
		end := raw
		if p.heredocs[0].stripTabs {
			end = strings.TrimLeft(raw, "\t")
		}
		if end == p.heredocs[0].word {
			p.heredocs = p.heredocs[1:]
		}
		return
	}
	trimmed := strings.TrimSpace(raw)
	if m := parserDirective.FindStringSubmatch(trimmed); p.directives && m != nil {
		if strings.EqualFold(m[1], "escape") && (m[2] == "\\" || m[2] == "`") {
			p.escape = m[2][0]
		}
		return
	}
	p.directives = false
	switch {
	case strings.HasPrefix(trimmed, "#"):
		if p.cur == nil {
			p.comments = append(p.comments, trimmed)
		}
	case trimmed == "":
		if p.cur == nil {
			p.comments = nil
		}
	default:
		p.instructionLine(n, trimmed)
	}
}

func (p *dockerParser) instructionLine(n int, text string) {
	continued := text[len(text)-1] == p.escape
	if continued {
		text = strings.TrimSpace(text[:len(text)-1])
	}
	if p.cur == nil {
		cmd, args, _ := strings.Cut(text, " ")
		p.cur = &instruction{line: n, cmd: strings.ToUpper(cmd), args: strings.TrimSpace(args), comments: p.comments}
		p.comments = nil
	} else {
		p.cur.args = strings.TrimSpace(p.cur.args + " " + text)
	}
	if continued {
		return
	}
	switch p.cur.cmd {
	case "RUN", "COPY", "ADD":
		for _, m := range heredocStart.FindAllStringSubmatch(p.cur.args, -1) {
			p.heredocs = append(p.heredocs, heredoc{word: m[2], stripTabs: m[1] == "-"})
		}
	}
	p.out = append(p.out, *p.cur)
	p.cur = nil
}

// detectDockerfile reports FROM image tags (with ARG defaults substituted) and ARG or ENV
// <NAME>_VERSION values. A `# renovate:` comment right above an ARG or ENV names the tool.
func detectDockerfile(f file) ([]Pin, error) {
	vars := map[string]string{}
	var pins []Pin
	for _, in := range parseDockerfile(f.data) {
		loc := Location{Line: in.line}
		switch in.cmd {
		case "FROM":
			// Stage names and scratch have no tag, so imagePin skips them.
			image := expandVars(fromImage(in.args), vars)
			if p, ok := imagePin(image, loc); ok && !strings.Contains(image, "$") {
				pins = append(pins, p)
			}
		case "ARG", "ENV":
			pins = append(pins, versionAssignments(in, vars)...)
		}
	}
	return pins, nil
}

// versionAssignments returns the <NAME>_VERSION pins of an ARG or ENV and records ARG defaults
// in vars.
func versionAssignments(in instruction, vars map[string]string) []Pin {
	hint := renovateLookup(in.comments)
	var pins []Pin
	for _, kv := range assignments(in.cmd, in.args) {
		if in.cmd == "ARG" {
			vars[kv[0]] = kv[1]
		}
		if !envNameVersion.MatchString(kv[0]) || kv[1] == "" || strings.Contains(kv[1], "$") {
			continue
		}
		pins = append(pins, Pin{
			RawName: kv[0], Kind: KindTool, Value: kv[1], Location: Location{Line: in.line},
			lookup: firstNonEmpty(hint, strings.ToUpper(kv[0])),
		})
	}
	return pins
}

// fromImage returns the image of `FROM [--flag=...] image [AS name]`.
func fromImage(args string) string {
	fields := strings.Fields(args)
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func expandVars(s string, vars map[string]string) string {
	return dockerVar.ReplaceAllStringFunc(s, func(m string) string {
		sub := dockerVar.FindStringSubmatch(m)
		if v, ok := vars[firstNonEmpty(sub[1], sub[2])]; ok && v != "" {
			return v
		}
		return m
	})
}

// assignments splits ARG or ENV arguments into name/value pairs. ARG allows names without a
// value, ENV allows the legacy `ENV NAME value` form.
func assignments(cmd, args string) [][2]string {
	fields := splitQuoted(args)
	if cmd == "ENV" && len(fields) > 0 && !strings.Contains(fields[0], "=") {
		return [][2]string{{fields[0], unquote(strings.TrimSpace(strings.TrimPrefix(args, fields[0])))}}
	}
	out := make([][2]string, 0, len(fields))
	for _, fl := range fields {
		name, value, _ := strings.Cut(fl, "=")
		out = append(out, [2]string{name, unquote(value)})
	}
	return out
}

// splitQuoted splits on spaces outside single or double quotes.
func splitQuoted(s string) []string {
	var (
		out   []string
		b     strings.Builder
		quote byte
	)
	for i := range len(s) {
		c := s[i]
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && (c == ' ' || c == '\t'):
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func unquote(s string) string {
	const quoted = 2
	if len(s) >= quoted && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
