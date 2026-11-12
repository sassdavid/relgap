package registry

import (
	"fmt"
	"regexp"

	"github.com/sassdavid/relgap/internal/version"
)

// Entry is one registry/<name>.yaml file, as defined by schemas/registry-v1.json.
type Entry struct {
	Name            string       `yaml:"name"`
	Aliases         []string     `yaml:"aliases"`
	Repo            string       `yaml:"repo"`
	MinRelgap       string       `yaml:"min_relgap"`
	Version         VersionSpec  `yaml:"version"`
	Sources         []Source     `yaml:"sources"`
	EOL             *EOL         `yaml:"eol"`
	OSV             []OSVPackage `yaml:"osv"`
	BreakingMarkers []string     `yaml:"breaking_markers"`
	Bin             []string     `yaml:"bin"`
}

// VersionSpec says how a tool's tags become versions.
type VersionSpec struct {
	Scheme    string `json:"scheme"               yaml:"scheme"`
	Extract   string `json:"extract,omitempty"    yaml:"extract"`
	TagPrefix string `json:"tag_prefix,omitempty" yaml:"tag_prefix"`
	Exclude   string `json:"exclude,omitempty"    yaml:"exclude"`
	Universe  string `json:"universe"             yaml:"universe"` // releases or tags
	Line      string `json:"line"                 yaml:"line"`     // none, major or minor
}

// Rules builds the version rules for this spec.
func (v VersionSpec) Rules() (version.Rules, error) {
	scheme := v.Scheme
	if scheme == "" {
		scheme = version.SchemeLoose
	}
	s, err := version.NewScheme(scheme, v.Extract)
	if err != nil {
		return version.Rules{}, fmt.Errorf("version scheme: %w", err)
	}
	r := version.Rules{Scheme: s, TagPrefix: v.TagPrefix}
	if v.Exclude != "" {
		if r.Exclude, err = regexp.Compile(v.Exclude); err != nil {
			return version.Rules{}, fmt.Errorf("version exclude: %w", err)
		}
	}
	return r, nil
}

// Universe values.
const (
	UniverseReleases = "releases"
	UniverseTags     = "tags"
)

// Source is one changelog source of a tool.
type Source struct {
	Kind           string   `json:"kind"                      yaml:"kind"`
	PathTemplate   string   `json:"path_template,omitempty"   yaml:"path_template"`
	URLTemplate    string   `json:"url_template,omitempty"    yaml:"url_template"`
	Ref            string   `json:"ref,omitempty"             yaml:"ref"`
	Heading        string   `json:"heading,omitempty"         yaml:"heading"`
	IgnoreSections []string `json:"ignore_sections,omitempty" yaml:"ignore_sections"`
	FromHeading    string   `json:"from_heading,omitempty"    yaml:"from_heading"`
	Content        string   `json:"content,omitempty"         yaml:"content"` // web-doc: selector of the element to convert
	Strip          []string `json:"strip,omitempty"           yaml:"strip"`   // web-doc: selectors of elements to drop
	Anchor         string   `json:"anchor,omitempty"          yaml:"anchor"`  // web-doc: id template of a version's element
	JSON           *JSONMap `json:"json,omitempty"            yaml:"json"`
}

// JSONMap says how a structured-json document becomes Markdown entries.
type JSONMap struct {
	Items   string `json:"items,omitempty"   yaml:"items"`   // dotted path to the array; empty means the document
	Heading string `json:"heading,omitempty" yaml:"heading"` // field whose value groups entries under a heading
	Text    string `json:"text"              yaml:"text"`    // template with {field} placeholders
	Link    string `json:"link,omitempty"    yaml:"link"`    // field holding a URL for the entry
}

// EOL names the endoflife.date product of a tool.
type EOL struct {
	Product string `json:"product"         yaml:"product"`
	Cycle   string `json:"cycle,omitempty" yaml:"cycle"` // template such as {major}.{minor}
}

// OSVPackage is a package name in an OSV ecosystem.
type OSVPackage struct {
	Ecosystem string `json:"ecosystem" yaml:"ecosystem"`
	Name      string `json:"name"      yaml:"name"`
}

// ToolSpec is a resolved tool: where it lives, how its versions are read and where its notes are.
type ToolSpec struct {
	Name            string       `json:"name"`
	Aliases         []string     `json:"aliases"`
	Repo            string       `json:"repo"`
	Version         VersionSpec  `json:"version"`
	Sources         []Source     `json:"sources"` // empty means auto-discovery
	EOL             *EOL         `json:"eol,omitempty"`
	OSV             []OSVPackage `json:"osv"`
	BreakingMarkers []string     `json:"breaking_markers"`
	Bin             []string     `json:"bin"` // executable names; empty means the tool name
}
