package classify

import (
	"fmt"

	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// AdapterVersionJump is the source adapter of the entries [Jumps] adds.
const AdapterVersionJump = "version-jump"

const confidenceJump = 0.6

// Jumps returns a breaking entry for each version that starts a new major line (a new minor in
// 0.x) and has no breaking entry of its own. versions are oldest first; urls maps a version to
// its release page.
func Jumps(
	s version.Scheme,
	from version.V,
	versions []version.Version,
	entries []Entry,
	urls map[string]string,
) []Entry {
	hasBreaking := map[string]bool{}
	for _, e := range entries {
		if e.Kind == KindBreaking {
			hasBreaking[e.Version] = true
		}
	}
	var out []Entry
	prev := from
	for _, v := range versions {
		oldLine, newLine := jumpLines(s, prev, v.V)
		prev = v.V
		if oldLine == "" || hasBreaking[v.V.String()] {
			continue
		}
		level := "major"
		if oldLine[0] == '0' && newLine[0] == '0' {
			level = "minor"
		}
		out = append(out, Entry{
			Version:    v.V.String(),
			Kind:       KindBreaking,
			Confidence: confidenceJump,
			Text:       fmt.Sprintf("%s version jump %s.x → %s.x: review the upgrade guide", level, oldLine, newLine),
			Source:     source.Ref{Adapter: AdapterVersionJump, URL: urls[v.V.String()]},
		})
	}
	return out
}

// jumpLines returns the old and new line when b starts a new major line after a, or a new minor
// line while the major is 0. It returns empty strings otherwise.
func jumpLines(s version.Scheme, a, b version.V) (string, string) {
	am, bm := s.Line(a, version.LineMajor), s.Line(b, version.LineMajor)
	if am != bm {
		return am, bm
	}
	if am == "0" {
		if an, bn := s.Line(a, version.LineMinor), s.Line(b, version.LineMinor); an != bn {
			return an, bn
		}
	}
	return "", ""
}
