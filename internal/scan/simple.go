package scan

import (
	"path"
	"regexp"
	"strings"
)

// detectToolVersions reads asdf's `.tool-versions`: a tool name followed by one or more versions.
func detectToolVersions(f file) ([]Pin, error) {
	var pins []Pin
	for i, line := range lines(f.data) {
		fields := strings.Fields(stripComment(line))
		if len(fields) < 2 { //nolint:mnd // a name and at least one version
			continue
		}
		for _, v := range fields[1:] {
			pins = append(pins, Pin{
				RawName: fields[0], Kind: KindTool, Value: v, Location: Location{Line: i + 1},
				lookup: miseLookup(fields[0]),
			})
		}
	}
	return pins, nil
}

// idiomaticTool returns the tool a single-version file pins.
func idiomaticTool(base string) (string, bool) {
	switch base {
	case ".terraform-version":
		return toolTerraform, true
	case ".terragrunt-version":
		return "terragrunt", true
	case ".node-version", ".nvmrc":
		return toolNode, true
	case ".python-version":
		return "python", true
	case ".go-version":
		return "go", true
	}
	return "", false
}

func isIdiomatic(rel string) bool {
	base := path.Base(rel)
	_, ok := idiomaticTool(base)
	return ok || base == "go.mod"
}

var (
	goDirective = regexp.MustCompile(`^go\s+(\S+)`)
	goToolchain = regexp.MustCompile(`^toolchain\s+(go(\S+))`)
)

// detectIdiomatic reads the first version line of a version file, or go.mod's go and toolchain
// lines. go.mod's go line is a minimum, so it's a constraint.
func detectIdiomatic(f file) ([]Pin, error) {
	base := path.Base(f.rel)
	if base == "go.mod" {
		var pins []Pin
		for i, line := range lines(f.data) {
			line = strings.TrimSpace(line)
			loc := Location{Line: i + 1}
			if m := goDirective.FindStringSubmatch(line); m != nil {
				pins = append(pins, Pin{
					RawName: "go", Kind: KindTool, Value: m[1], Version: m[1],
					Match: MatchConstraint, Location: loc,
				})
			}
			if m := goToolchain.FindStringSubmatch(line); m != nil {
				pins = append(pins, Pin{
					RawName: "toolchain", Kind: KindTool, Value: m[1], Version: m[2],
					Location: loc, lookup: "go",
				})
			}
		}
		for i := range pins {
			if pins[i].Match == "" {
				pins[i].Match = matchOf(pins[i].Version)
			}
		}
		return pins, nil
	}
	tool, _ := idiomaticTool(base)
	for i, line := range lines(f.data) {
		v := strings.TrimSpace(stripComment(line))
		if v == "" {
			continue
		}
		return []Pin{{RawName: base, Kind: KindTool, Value: v, Location: Location{Line: i + 1}, lookup: tool}}, nil
	}
	return nil, nil
}

// matchOf classifies a bare version string.
func matchOf(v string) Match {
	p := Pin{Value: v}
	p.classify(false)
	return p.Match
}
