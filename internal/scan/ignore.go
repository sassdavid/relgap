package scan

import (
	"path"
	"strings"
)

// Ignore drops the pins of one tool, everywhere or only in files under a glob.
type Ignore struct {
	Tool string // a tool name as Pin.Tool holds it
	Path string // glob on repo/file, ** for any directories; empty matches every file
}

// Ignored reports whether any rule drops p.
func Ignored(rules []Ignore, p *Pin) bool {
	for _, r := range rules {
		if r.Tool == "" || !strings.EqualFold(r.Tool, p.Tool) {
			continue
		}
		if r.Path == "" || globMatch(r.Path, path.Join(p.Location.Repo, p.Location.File)) ||
			globMatch(r.Path, p.Location.File) {
			return true
		}
	}
	return false
}
