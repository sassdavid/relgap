package scan

import (
	"cmp"
	"regexp"
	"strings"
)

// Kind says what a pin pins.
type Kind string

// Pin kinds. Every kind but [KindTool] carries its own name in Pin.Tool as kind:name.
const (
	KindTool       Kind = "tool"
	KindTFProvider Kind = "tfprovider"
	KindChart      Kind = "chart"
	KindImage      Kind = "image"
	KindAction     Kind = "action"
)

// Tool names and keys that several detectors use.
const (
	toolTerraform = "terraform"
	toolNode      = "node"
	keyVersion    = "version"
	prefixAqua    = "aqua:"
	prefixGitHub  = "github:"
)

// Match says which versions a pin value allows.
type Match string

// Match values.
const (
	MatchExact      Match = "exact"      // 1.15.8, v1.2.3-rc.1
	MatchConstraint Match = "constraint" // ~> 1.15, >= 1.5, < 2
	MatchPrefix     Match = "prefix"     // 26, 3.14, prefix:1.2
	MatchChannel    Match = "channel"    // latest, lts, corretto-17, ref:main
)

// Pin is one tool version found in a file.
type Pin struct {
	Tool     string   `json:"tool"`     // name `relgap digest` accepts; empty when unresolved
	RawName  string   `json:"raw_name"` // as written: TERRAFORM_VERSION, hashicorp/aws, http:kafka
	Kind     Kind     `json:"kind"`
	Value    string   `json:"value"`   // as written
	Version  string   `json:"version"` // Value without a leading v or go, for comparison
	Exact    bool     `json:"exact"`
	Match    Match    `json:"match"`
	Location Location `json:"location"`

	lookup string // name to resolve when it differs from RawName
}

// Location is where a pin was found.
type Location struct {
	Repo     string `json:"repo"` // repository directory, slash-separated
	File     string `json:"file"` // relative to Repo, slash-separated
	Line     int    `json:"line"` // 0 when the source has no lines, as in a Renovate report
	Detector string `json:"detector"`
}

// Resolved reports whether the pin maps to a tool.
func (p *Pin) Resolved() bool { return p.Tool != "" }

var (
	fullVersion    = regexp.MustCompile(`^v?\d+\.\d+\.\d+([.+-][0-9A-Za-z.+_-]*)?$`)
	partialVersion = regexp.MustCompile(`^v?\d+(\.\d+)?([+-][0-9A-Za-z.+_-]*)?$`)
	constraintOp   = regexp.MustCompile(`^(~>|>=|<=|!=|==|>|<|=|\^|~)`)
	wildcard       = regexp.MustCompile(`(^|\.)[x*X](\.|$)`)
)

// classify sets Match, Exact and Version from Value. bareExact means a version without an
// operator allows only itself, as in Terraform, where "6.0" means = 6.0.0.
func (p *Pin) classify(bareExact bool) {
	v := strings.TrimSpace(p.Value)
	switch {
	case p.Match != "":
	case constraintOp.MatchString(v) || strings.Contains(v, ",") || strings.Contains(v, "||") ||
		strings.Contains(v, " - ") || wildcard.MatchString(v):
		p.Match = MatchConstraint
	case fullVersion.MatchString(v):
		p.Match = MatchExact
	case partialVersion.MatchString(v) && bareExact:
		p.Match = MatchExact
	case partialVersion.MatchString(v):
		p.Match = MatchPrefix
	case strings.HasPrefix(v, "prefix:"):
		p.Match = MatchPrefix
		v = strings.TrimPrefix(v, "prefix:")
	default:
		p.Match = MatchChannel
	}
	p.Exact = p.Match == MatchExact
	if p.Match == MatchExact || p.Match == MatchPrefix {
		v = strings.TrimPrefix(v, "v")
	}
	p.Version = cmp.Or(p.Version, v)
}

func comparePins(a, b Pin) int {
	return cmp.Or(
		cmp.Compare(a.Location.Repo, b.Location.Repo),
		cmp.Compare(a.Location.File, b.Location.File),
		cmp.Compare(a.Location.Line, b.Location.Line),
		cmp.Compare(a.RawName, b.RawName),
		cmp.Compare(a.Value, b.Value),
		cmp.Compare(a.Location.Detector, b.Location.Detector),
	)
}
