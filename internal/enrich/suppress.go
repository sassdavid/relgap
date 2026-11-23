package enrich

import (
	"cmp"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/purl"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Suppression kinds.
const (
	SuppressedByIgnore = "ignore" // a vulns.ignore entry
	SuppressedByVEX    = "vex"    // an OpenVEX statement
)

// Suppression says why an advisory doesn't count: a vulns.ignore entry or an OpenVEX statement.
// A suppressed advisory stays in the report and is left out of counts, gates and fix targets.
type Suppression struct {
	Kind          string `json:"kind"` // ignore or vex
	Reason        string `json:"reason"`
	Until         string `json:"until,omitempty"`         // the ignore entry's last day
	Source        string `json:"source,omitempty"`        // the VEX document's path
	Status        string `json:"status,omitempty"`        // the VEX status: not_affected or fixed
	Justification string `json:"justification,omitempty"` // the VEX justification label
}

// Subject is a tool version as suppression rules name it.
type Subject struct {
	Tool    string      // as pins show it, such as kubectl or action:actions/checkout
	Version string      // the version OSV was asked about
	PURLs   []purl.PURL // the package URLs of its OSV packages and repository, at the version
}

// Suppressor decides which advisories of a tool version are suppressed. names holds the
// advisory's ID and aliases.
type Suppressor interface {
	// Suppress returns why the advisory is suppressed for s, or nil, and counts a match as a use
	// of its rule.
	Suppress(s Subject, names []string) *Suppression
	// Covers reports whether a rule suppresses the advisory for s, without counting a use.
	Covers(s Subject, names []string) bool
}

// SetSuppressor makes the enricher suppress the advisories s names before it works out the fix
// targets. A nil s suppresses nothing.
func (e *Enricher) SetSuppressor(s Suppressor) { e.suppressor = s }

// suppress marks each advisory of a request's current version that the suppressor names.
func (e *Enricher) suppress(reqs []Request, ans osvAnswer) {
	if e.suppressor == nil {
		return
	}
	for _, r := range reqs {
		vs := ans.vulns[r.Key]
		if len(vs) == 0 {
			continue
		}
		sub := r.subjectAt(r.Current, ans.modules)
		for j := range vs {
			if vs[j].AffectsCurrent {
				vs[j].Suppressed = e.suppressor.Suppress(sub, vs[j].names())
			}
		}
	}
}

// names returns the advisory's ID and aliases.
func (v Vuln) names() []string { return append([]string{v.ID}, v.Aliases...) }

// Counts reports whether the advisory affects the current version and isn't suppressed.
func (v Vuln) Counts() bool { return v.AffectsCurrent && v.Suppressed == nil }

// PURL returns the package URL of the advisory's package at the version OSV was asked about, or ""
// for an ecosystem without a purl type.
func (v Vuln) PURL() string { return v.purl }

// subjectAt returns version v of the request's tool as suppression rules name it.
func (r Request) subjectAt(v version.V, modules map[string]string) Subject {
	name := cmp.Or(r.Name, r.Tool.Name)
	sub := Subject{Tool: name, Version: v.String()}
	add := func(p purl.PURL, ok bool) {
		if ok && !slices.ContainsFunc(sub.PURLs, func(q purl.PURL) bool { return q.String() == p.String() }) {
			sub.PURLs = append(sub.PURLs, p)
		}
	}
	c := r.coverageAt(v)
	pkgs := c.Packages
	if m := modules[c.GoPath]; m != "" {
		pkgs = append(slices.Clip(pkgs), registry.OSVPackage{Ecosystem: EcosystemGo, Name: m})
	}
	for _, p := range pkgs {
		add(r.packagePURL(p, v))
	}
	if r.Tool.Repo != "" && forge.IsGitHub(r.Tool.Host) &&
		!strings.HasPrefix(name, registry.KindAction+":") {
		add(repoPURL("github", r.Tool.Repo, r.tag(v)))
	}
	for _, ref := range r.Refs {
		for _, p := range slices.Clone(sub.PURLs) {
			if p.Type == purlActions {
				p.Version = ref
				add(p, true)
			}
		}
	}
	return sub
}

const purlActions = "githubactions"

// packagePURL returns the package URL of OSV package p at version v of the request's tool: the
// version as the package's ranges spell it, a Go module version with a v, and the tag for an
// action, with the v action tags have. The second result is false for an ecosystem without a purl type.
func (r Request) packagePURL(p registry.OSVPackage, v version.V) (purl.PURL, bool) {
	return r.packagePURLAt(p, QueryVersion(p, r.tag(v), v), r.tag(v))
}

// packagePURLAt returns the package URL of OSV package p at version at, as the package's ranges
// spell it, whose tag is tag.
func (r Request) packagePURLAt(p registry.OSVPackage, at, tag string) (purl.PURL, bool) {
	switch p.Ecosystem {
	case EcosystemGo:
		return splitPURL("golang", p.Name, "v"+at), true
	case ecosystemNPM:
		return splitPURL("npm", p.Name, at), true
	case ecosystemPy:
		return purl.PURL{Type: "pypi", Name: p.Name, Version: at}, true
	case ecosystemCrates:
		return purl.PURL{Type: "cargo", Name: p.Name, Version: at}, true
	case ecosystemRubyGems:
		return purl.PURL{Type: "gem", Name: p.Name, Version: at}, true
	case ecosystemNuGet:
		return purl.PURL{Type: "nuget", Name: p.Name, Version: at}, true
	case EcosystemActions:
		if tag != "" && tag[0] >= '0' && tag[0] <= '9' {
			// Version tags lose their leading v, and action tags have one: uses: x@v4.1.2.
			tag = "v" + tag
		}
		pu, ok := repoPURL(purlActions, p.Name, tag)
		if _, action, found := strings.Cut(cmp.Or(r.Name, r.Tool.Name), registry.KindAction+":"); found {
			_, rest, _ := strings.Cut(action, "/")
			_, pu.Subpath, _ = strings.Cut(rest, "/") // the path of an action below owner/repo
		}
		return pu, ok
	case EcosystemGit:
		return gitPURL(p.Name, at)
	default:
		return purl.PURL{}, false
	}
}

// splitPURL puts a slash-separated package name's last part in the name and the rest in the
// namespace, as purls of Go modules and scoped npm packages have them.
func splitPURL(typ, name, ver string) purl.PURL {
	dir, base := path.Split(name)
	return purl.PURL{Type: typ, Namespace: strings.TrimSuffix(dir, "/"), Name: base, Version: ver}
}

// repoPURL returns the purl of a repository owner/repo on a forge type such as github.
func repoPURL(typ, repo, ver string) (purl.PURL, bool) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return purl.PURL{}, false
	}
	return purl.PURL{Type: typ, Namespace: owner, Name: name, Version: ver}, true
}

// gitPURL returns the purl of a git repository at a tag: pkg:github for github.com, and
// pkg:generic with the repository in vcs_url otherwise, since purl has no gitlab type.
func gitPURL(repoURL, tag string) (purl.PURL, bool) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host == "" {
		return purl.PURL{}, false
	}
	repo := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if forge.IsGitHub(u.Host) {
		return repoPURL("github", repo, tag)
	}
	return purl.PURL{
		Type: "generic", Name: path.Base(repo), Version: tag,
		Qualifiers: map[string]string{"vcs_url": "git+" + strings.TrimSuffix(repoURL, "/")},
	}, true
}
