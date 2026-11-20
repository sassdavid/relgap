package remote

import "github.com/sassdavid/relgap/internal/version"

// ChartTagPrefix exposes chartTagPrefix to the tests, with the newest stable version of versions.
func ChartTagPrefix(chart string, versions, tags []string) (string, bool) {
	refs := make([]version.Ref, len(versions))
	for i, v := range versions {
		refs[i] = version.Ref{Tag: v}
	}
	return chartTagPrefix(chart, versions, newestStable(refs), tags)
}
