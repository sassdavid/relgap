package enrich

import "strings"

// SentField is one value that a request to OSV carries.
type SentField struct {
	Request     string `json:"request"` // the method and path of the request
	Name        string `json:"name"`    // the JSON path of the field in the body, or the path parameter
	Description string `json:"description"`
}

// The requests of [SentField].
const (
	SentQueryBatch = "POST /v1/querybatch"
	SentVuln       = "GET /v1/vulns/{id}"
)

// SentToOSV returns every value relgap's requests to OSV carry. A test checks it against the
// bodies and paths that [OSVClient] sends.
func SentToOSV() []SentField {
	ecosystems := strings.Join([]string{
		EcosystemGo, ecosystemNPM, ecosystemPy, ecosystemCrates, ecosystemRubyGems, ecosystemNuGet,
		EcosystemGit, EcosystemActions,
	}, ", ")
	return []SentField{
		{SentQueryBatch, "queries[].package.ecosystem", "The OSV ecosystem: " + ecosystems +
			", or the one a curated `osv` list names."},
		{SentQueryBatch, "queries[].package.name", "The package name as the pin or the registry entry " +
			"writes it (PyPI names normalized); a Go module path, every prefix of a `go:` path while " +
			"relgap looks for the module, or the main module of an executable under `outdated " +
			"--binaries`; the URL of a github.com or gitlab.com repository for `GIT`; owner/repo of an action. " +
			"A repository you named only when its forge says it is public, and never a module path " +
			"that `GOPRIVATE` names."},
		{SentQueryBatch, "queries[].version", "The version relgap checks: an exact pin, the version a " +
			"pin marked ≈ stands for, or a fix target candidate or latest release. Left out when relgap " +
			"looks for a `go:` module or asks about an action."},
		{SentQueryBatch, "queries[].page_token", "The token OSV returned for the next page of an answer."},
		{SentVuln, "id", "The ID of an advisory that OSV returned, or a GHSA alias of one."},
	}
}
