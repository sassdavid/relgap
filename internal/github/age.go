package github

import (
	"net/http"
	"regexp"
	"time"
)

var publishedAt = regexp.MustCompile(`"(?:publishedAt|published_at)"\s*:\s*"([^"]+)"`)

// NewestRelease returns the newest release publish date in a REST or GraphQL response body, or
// the zero time when the body has none. It is an [httpx.AgeFunc].
func NewestRelease(_ *http.Request, body []byte) time.Time {
	var newest time.Time
	for _, m := range publishedAt.FindAllSubmatch(body, -1) {
		if t, err := time.Parse(time.RFC3339, string(m[1])); err == nil && t.After(newest) {
			newest = t
		}
	}
	return newest.UTC()
}
