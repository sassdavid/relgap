package registry

import (
	"context"
	"errors"
)

// Lookup kinds: the explicit prefixes that need a package registry, and a GitHub search.
const (
	KindTFProvider = "tfprovider"
	KindChart      = "chart"
	KindNPM        = "npm"
	KindPyPI       = "pypi"
	KindGuess      = "guess"
)

// ErrAmbiguous is wrapped by lookup errors when a name matches more than one package.
var ErrAmbiguous = errors.New("ambiguous name")

// Found is what a lookup learned about a tool.
type Found struct {
	Repo         string   // owner/repo on GitHub
	Name         string   // tool name to use when no registry knows the repo; empty means the repo name
	TagPrefix    string   // set for a chart in a repository with several charts
	Layer        Layer    // the service that answered
	Alternatives []string // other repositories a guess could have picked
}

// Finder looks a tool up in a package registry, or guesses its repository with kind [KindGuess].
type Finder interface {
	Find(ctx context.Context, kind, name string) (*Found, error)
}
