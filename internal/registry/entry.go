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
	TagSuffix string `json:"tag_suffix,omitempty" yaml:"-"` // an image variant such as -alpine
	Exclude   string `json:"exclude,omitempty"    yaml:"exclude"`
	Universe  string `json:"universe"             yaml:"universe"` // releases or tags; chart and image only from a lookup
	Line      string `json:"line"                 yaml:"line"`     // none, major or minor
	// VersionsURL is where a chart universe's versions come from: an Artifact Hub package page
	// or a Helm repository index.
	VersionsURL string `json:"versions_url,omitempty" yaml:"-"`
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
	r := version.Rules{Scheme: s, TagPrefix: v.TagPrefix, TagSuffix: v.TagSuffix}
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
	// UniverseChart takes the versions a chart lookup listed, for a chart whose repository has no
	// tag per chart version. They carry no release notes.
	UniverseChart = "chart"
	// UniverseImage takes the tags an image registry listed. Notes come from the GitHub repository
	// the image's source label names, as the tool in [ToolSpec.Notes].
	UniverseImage = "image"
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
	Content        string   `json:"content,omitempty"         yaml:"content"`     // web-doc: selector of the element to convert
	Strip          []string `json:"strip,omitempty"           yaml:"strip"`       // web-doc: selectors of elements to drop
	Anchor         string   `json:"anchor,omitempty"          yaml:"anchor"`      // web-doc: id template of a version's element
	SiteURL        string   `json:"site_url,omitempty"        yaml:"site_url"`    // changelog-file: base URL of the Hugo site
	ContentDir     string   `json:"content_dir,omitempty"     yaml:"content_dir"` // changelog-file: Hugo content directory
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
	Name            string        `json:"name"`
	Aliases         []string      `json:"aliases"`
	Repo            string        `json:"repo"`            // owner/repo, or the project path on Host
	Host            string        `json:"host,omitempty"`  // bitbucket.org, gitlab.com or a GitLab host; empty means github.com
	Image           string        `json:"image,omitempty"` // registry/repository of a [UniverseImage] tool
	Notes           *ToolSpec     `json:"notes,omitempty"` // the tool whose notes an image's digest reads
	Version         VersionSpec   `json:"version"`
	Sources         []Source      `json:"sources"` // empty means auto-discovery
	EOL             *EOL          `json:"eol,omitempty"`
	OSV             []OSVPackage  `json:"osv"`
	BreakingMarkers []string      `json:"breaking_markers"`
	Bin             []string      `json:"bin"` // executable names; empty means the tool name
	Backend         string        `json:"-"`   // the input's npm: or pypi: name, else the first backend of its mise entry
	Versions        []version.Ref `json:"-"`   // the versions of a [UniverseChart] or [UniverseImage] tool
	UserRepo        bool          `json:"-"`   // Repo comes from the input, the config or a GitHub search, not a registry
}

// Listed reports whether the tool's versions come from a lookup instead of its repository.
func (t ToolSpec) Listed() bool {
	return t.Version.Universe == UniverseChart || t.Version.Universe == UniverseImage
}
