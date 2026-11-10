package ghtest

import (
	"cmp"
	"os"
	"regexp"

	"github.com/sassdavid/relgap/internal/testutil/vcr"
)

// DummyToken stands in for a token when replaying; the matcher ignores Authorization headers.
const DummyToken = "test-token"

// Token returns the token from GH_TOKEN or GITHUB_TOKEN while recording, and [DummyToken] otherwise.
func Token() string {
	if vcr.Recording() {
		return cmp.Or(os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))
	}
	return DummyToken
}

// Scrubber removes GitHub credentials and token metadata from recorded interactions. extra lists
// more secret values, such as a token passed through another variable.
func Scrubber(extra ...string) vcr.Scrubber {
	return vcr.Scrubber{
		Headers: []string{
			"X-Oauth-Scopes", "X-Accepted-Oauth-Scopes", "X-Oauth-Client-Id", "X-Github-Request-Id",
			"Github-Authentication-Token-Expiration",
		},
		Secrets: append([]string{os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN")}, extra...),
		Replace: []vcr.Replacement{{
			Pattern: regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})\b`),
			With:    vcr.Redacted,
		}},
	}
}
