package registry

import (
	"context"
	"errors"
	"strings"

	"github.com/sassdavid/relgap/internal/version"
)

// Lookup kinds: the explicit prefixes that need a package registry, and a GitHub search.
const (
	KindTFProvider = "tfprovider"
	KindTFModule   = "tfmodule"
	KindChart      = "chart"
	KindNPM        = "npm"
	KindPyPI       = "pypi"
	KindImage      = "image"
	KindGuess      = "guess"
)

// Explicit prefixes of repositories on other git hosts, resolved without a lookup.
const (
	KindBitbucket = "bitbucket"
	KindGitLab    = "gitlab"
)

// ErrAmbiguous is wrapped by lookup errors when a name matches more than one package.
var ErrAmbiguous = errors.New("ambiguous name")

// ChartAt returns the name of the chart called chart in the Helm repository at registryURL:
// chart:<registryURL>/<chart>.
func ChartAt(registryURL, chart string) string {
	return KindChart + ":" + strings.TrimRight(registryURL, "/") + "/" + chart
}

// SplitChartAt splits the part of a chart:<registryURL>/<chart> name after the prefix into the
// repository URL and the chart, and reports whether rest is such a name.
func SplitChartAt(rest string) (string, string, bool) {
	if !strings.Contains(rest, "://") {
		return "", "", false
	}
	i := strings.LastIndex(rest, "/")
	registryURL, chart := rest[:i], rest[i+1:]
	if chart == "" || strings.HasSuffix(registryURL, ":/") {
		return "", "", false
	}
	return registryURL, chart, true
}

// SplitTagPrefix splits github:owner/repo@prefix, without github:, into the repository and the
// tag prefix, and reports whether rest has a prefix.
func SplitTagPrefix(rest string) (string, string, bool) {
	repo, tagPrefix, ok := strings.Cut(rest, "@")
	if !ok || tagPrefix == "" {
		return rest, "", false
	}
	return repo, tagPrefix, true
}

// ImageName returns the tool name of image with the tag pattern prefix*suffix:
// image:<image>, or image:<image>:<prefix>*<suffix> when either is set.
func ImageName(image, prefix, suffix string) string {
	if prefix == "" && suffix == "" {
		return KindImage + ":" + image
	}
	return KindImage + ":" + image + ":" + prefix + "*" + suffix
}

// SplitImage splits the part of an image:<image>[:<prefix>*<suffix>] name after the prefix into
// the image, the tag prefix and the tag suffix, and reports whether rest is such a name. A
// pattern has exactly one * and no slash, so a registry port such as localhost:5000/app stays
// part of the image.
func SplitImage(rest string) (string, string, string, bool) {
	image, pattern := rest, ""
	if i := strings.LastIndexByte(rest, ':'); i >= 0 && strings.Contains(rest[i+1:], "*") &&
		!strings.Contains(rest[i+1:], "/") {
		image, pattern = rest[:i], rest[i+1:]
	}
	if image == "" || strings.ContainsAny(image, "*@ ") {
		return "", "", "", false
	}
	if pattern == "" {
		return image, "", "", true
	}
	prefix, suffix, _ := strings.Cut(pattern, "*")
	if strings.Contains(suffix, "*") {
		return "", "", "", false
	}
	return image, prefix, suffix, true
}

// Found is what a lookup learned about a tool.
type Found struct {
	Repo         string   // owner/repo on GitHub
	Name         string   // tool name to use when no registry knows the repo; empty means the repo name
	TagPrefix    string   // set for a chart in a repository with several charts, and for a Terraform module
	Layer        Layer    // the service that answered
	Alternatives []string // other repositories a guess could have picked
	// Versions are a chart's versions when its repository has no tag per chart version; the tool
	// then gets [UniverseChart].
	Versions    []version.Ref
	VersionsURL string // where Versions come from
	TagSuffix   string // an image variant, such as -alpine
	Image       string // registry/repository of an image
}

// Finder looks a tool up in a package registry, or guesses its repository with kind [KindGuess].
type Finder interface {
	Find(ctx context.Context, kind, name string) (*Found, error)
}
