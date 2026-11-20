package scan

import (
	"path"
	"strings"
)

// Ignore drops the pins that match every field it sets. A rule that sets none drops nothing.
type Ignore struct {
	Tool string // a tool name as Pin.Tool holds it
	Name string // glob on Pin.RawName, ignoring case; * matches any characters, / included
	Path string // glob on repo/file, ** for any directories
}

// Ignored reports whether any rule drops p.
func Ignored(rules []Ignore, p *Pin) bool {
	for _, r := range rules {
		if r.Tool == "" && r.Name == "" && r.Path == "" {
			continue
		}
		if r.Tool != "" && !strings.EqualFold(r.Tool, p.Tool) || r.Name != "" && !nameMatch(r.Name, p.RawName) {
			continue
		}
		if r.Path == "" || globMatch(r.Path, path.Join(p.Location.Repo, p.Location.File)) ||
			globMatch(r.Path, p.Location.File) {
			return true
		}
	}
	return false
}

// nameMatch matches a raw name against a glob whose * spans any characters, because raw names
// such as git::git@bitbucket.org:ws/repo.git hold slashes.
func nameMatch(pattern, name string) bool {
	parts := strings.Split(strings.ToLower(pattern), "*")
	name = strings.ToLower(name)
	if len(parts) == 1 {
		return parts[0] == name
	}
	rest, ok := strings.CutPrefix(name, parts[0])
	if !ok {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return strings.HasSuffix(rest, parts[len(parts)-1])
}
