package registry

import (
	"context"
	"errors"
	"strings"
)

// Lookup kinds: the explicit prefixes that need a package registry, and a GitHub search.
const (
	KindTFProvider = "tfprovider"
	KindTFModule   = "tfmodule"
	KindChart      = "chart"
	KindNPM        = "npm"
	KindPyPI       = "pypi"
	KindGuess      = "guess"
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

// Found is what a lookup learned about a tool.
type Found struct {
	Repo         string   // owner/repo on GitHub
	Name         string   // tool name to use when no registry knows the repo; empty means the repo name
	TagPrefix    string   // set for a chart in a repository with several charts, and for a Terraform module
	Layer        Layer    // the service that answered
	Alternatives []string // other repositories a guess could have picked
}

// Finder looks a tool up in a package registry, or guesses its repository with kind [KindGuess].
type Finder interface {
	Find(ctx context.Context, kind, name string) (*Found, error)
}
