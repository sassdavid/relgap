package enrich

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

// Reasons a pin is unchecked: relgap didn't ask OSV about it, so it is never reported as clean.
const (
	ReasonNoSource   = "no-advisory-source" // no curated osv list and no backend or repository OSV knows
	ReasonPrivate    = "private-source"     // a host or registry relgap never sends to OSV
	ReasonNotExact   = "not-exact"          // a constraint, range, branch, channel or bare SHA
	ReasonUnresolved = "unresolved"         // the pin resolves to no tool
	ReasonOffline    = "offline"            // --offline and no cached answer
	ReasonFailed     = "failed"             // OSV failed for this pin
)

// OSV ecosystems a package backend maps to.
const (
	ecosystemNPM      = "npm"
	ecosystemCrates   = "crates.io"
	ecosystemRubyGems = "RubyGems"
	ecosystemNuGet    = "NuGet"
)

// Coverage is what relgap asks OSV about one tool version, or why it asks nothing.
type Coverage struct {
	Packages []registry.OSVPackage // queried at the version
	GoPath   string                // a go: package path; its longest prefix OSV knows is queried at the version
	Reason   string                // why nothing is queried; empty when the tool is checked
}

// Checked reports whether relgap asks OSV about the tool.
func (c Coverage) Checked() bool { return c.Reason == "" }

// CoverageOf returns what relgap asks OSV about version v of tool t, whose pins name it name (the
// name `relgap digest` accepts, such as chart:argo/argo-cd). A curated osv list wins. Otherwise a
// package backend names the package, and a github.com tool gets its Go module and a GIT query on
// its repository, the mapping mise audit uses. Hosts other than github.com and gitlab.com are
// never sent.
func CoverageOf(name string, t registry.ToolSpec, v version.V) Coverage {
	if len(t.OSV) > 0 {
		rules, err := t.Version.Rules()
		if err != nil {
			return Coverage{Packages: t.OSV}
		}
		return Coverage{Packages: PackagesFor(t.OSV, rules.Scheme, v)}
	}
	if c, ok := backendCoverage(t.Backend); ok {
		return c
	}
	if reason, ok := NameReason(name); ok {
		return Coverage{Reason: reason}
	}
	if kind, _, _ := strings.Cut(name, ":"); kind == registry.KindTFModule {
		return Coverage{Reason: ReasonNoSource}
	}
	return repoCoverage(t, v)
}

// NameReason returns the unchecked reason of a pin whose tool name alone says OSV is never asked
// about it: an image (private-source on a registry relgap never sends to, else
// no-advisory-source) or a chart. Neither takes a curated entry, so it needs no lookup. The
// second result is false for every other name.
func NameReason(name string) (string, bool) {
	kind, rest, _ := strings.Cut(name, ":")
	switch kind {
	case registry.KindImage:
		if !publicRegistry(rest) {
			return ReasonPrivate, true
		}
		return ReasonNoSource, true
	case registry.KindChart:
		return ReasonNoSource, true
	}
	return "", false
}

// SkipReason returns why a pin of the tool name isn't checked when resolving name failed with
// err: a tool relgap doesn't know is unresolved, a registry it may not read or that wants an
// account is private, and a failed lookup is offline or failed.
func SkipReason(name string, err error) string {
	kind, rest, _ := strings.Cut(name, ":")
	switch {
	case errors.Is(err, httpx.ErrOffline):
		return ReasonOffline
	case errors.Is(err, httpx.ErrUnauthorized):
		return ReasonPrivate
	case errors.Is(err, registry.ErrUnknownTool):
		return ReasonUnresolved
	case kind == registry.KindImage && !publicRegistry(rest):
		return ReasonPrivate
	case errors.Is(err, registry.ErrNoRepo), errors.Is(err, registry.ErrUnsupported):
		return ReasonNoSource
	default:
		return ReasonFailed
	}
}

// backendCoverage maps a package backend (npm:prettier, pipx:black[extras], go:path) to its OSV
// package. The second result is false when backend isn't a package backend.
func backendCoverage(backend string) (Coverage, bool) {
	kind, pkg, _ := strings.Cut(backend, ":")
	pkg, _, _ = strings.Cut(pkg, "[")
	var ecosystem string
	switch kind {
	case "npm":
		ecosystem = ecosystemNPM
	case "pypi", "pipx":
		ecosystem, pkg = ecosystemPy, pep503(pkg)
	case "cargo":
		ecosystem = ecosystemCrates
	case "gem":
		ecosystem = ecosystemRubyGems
	case "dotnet", "nuget":
		ecosystem = ecosystemNuGet
	case "go":
		ecosystem = EcosystemGo
	default:
		return Coverage{}, false
	}
	if pkg == "" {
		return Coverage{Reason: ReasonNoSource}, true
	}
	// A registry alias (npm:alias@npm:pkg) or a URL names a package OSV can't know.
	unscoped := pkg
	if strings.HasPrefix(pkg, "@") {
		_, unscoped, _ = strings.Cut(pkg, "/")
	}
	if strings.ContainsAny(unscoped, ":@") {
		return Coverage{Reason: ReasonPrivate}, true
	}
	if ecosystem == EcosystemGo {
		return Coverage{GoPath: pkg}, true
	}
	return Coverage{Packages: []registry.OSVPackage{{Ecosystem: ecosystem, Name: pkg}}}, true
}

var pep503Run = regexp.MustCompile(`[-_.]+`)

// pep503 normalizes a Python package name as PEP 503 says.
func pep503(name string) string { return strings.ToLower(pep503Run.ReplaceAllString(name, "-")) }

// repoCoverage queries the tool's repository: the Go module and GIT on github.com, GIT on
// gitlab.com.
func repoCoverage(t registry.ToolSpec, v version.V) Coverage {
	switch {
	case t.Repo == "":
		return Coverage{Reason: ReasonNoSource}
	case forge.IsGitHub(t.Host):
		return Coverage{Packages: []registry.OSVPackage{
			{Ecosystem: EcosystemGo, Name: goModule("github.com/"+t.Repo, t.Version, v)},
			{Ecosystem: EcosystemGit, Name: "https://github.com/" + t.Repo},
		}}
	case strings.EqualFold(t.Host, forge.GitLab):
		return Coverage{Packages: []registry.OSVPackage{
			{Ecosystem: EcosystemGit, Name: "https://" + forge.GitLab + "/" + t.Repo},
		}}
	default:
		return Coverage{Reason: ReasonPrivate}
	}
}

// goModule is the Go module path of version v of the repository at base: base/vN from major 2 on.
func goModule(base string, vs registry.VersionSpec, v version.V) string {
	rules, err := vs.Rules()
	if err != nil {
		return base
	}
	if major, atoiErr := strconv.Atoi(
		rules.Scheme.Line(v, version.LineMajor),
	); atoiErr == nil &&
		major >= goSuffixFrom {
		return base + "/v" + strconv.Itoa(major)
	}
	return base
}

// GoPrefixes returns the prefixes of a Go package path that could be its module, longest first.
func GoPrefixes(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var out []string
	for n := len(parts); n >= minModuleParts; n-- {
		out = append(out, strings.Join(parts[:n], "/"))
	}
	return out
}

// minModuleParts is the number of parts in the shortest module path, such as gopkg.in/yaml.v3.
const minModuleParts = 2

// publicRegistry reports whether image is on a registry anyone can pull from without an account.
func publicRegistry(image string) bool {
	host, _, ok := strings.Cut(image, "/")
	if !ok || !strings.ContainsAny(host, ".:") && host != "localhost" {
		return true // a Docker Hub name such as library/nginx
	}
	switch strings.ToLower(host) {
	case "docker.io", "index.docker.io", "registry-1.docker.io", "ghcr.io", "quay.io", "gcr.io",
		"registry.k8s.io", "public.ecr.aws", "mcr.microsoft.com", "registry.gitlab.com":
		return true
	}
	return false
}
