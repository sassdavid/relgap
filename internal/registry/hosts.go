package registry

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// ErrHostNotAllowed is wrapped by [CheckURL] errors for hosts outside [WebHosts].
var ErrHostNotAllowed = errors.New("host not allowed")

// WebHosts returns the hosts that web-doc, structured-json and upgrade-guide URLs may point to.
// Every other page comes from GitHub through the API.
func WebHosts() []string {
	return []string{"api.adoptium.net", "docs.python.org", "go.dev"}
}

// CheckURL returns an error unless raw is an https URL on one of [WebHosts].
func CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", raw, err)
	}
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return fmt.Errorf("%w: %s: want https://<host>/... without user or port", ErrHostNotAllowed, raw)
	}
	if !slices.Contains(WebHosts(), strings.ToLower(u.Hostname())) {
		return fmt.Errorf("%w: %s: relgap fetches pages only from %s", ErrHostNotAllowed, u.Hostname(),
			strings.Join(WebHosts(), ", "))
	}
	return nil
}

// Template placeholders of paths, URLs, refs and anchors.
const (
	PlaceholderVersion   = "{version}"
	PlaceholderMajor     = "{major}"
	PlaceholderMinor     = "{minor}"
	PlaceholderTag       = "{tag}"
	PlaceholderPrevMinor = "{prev_minor}"
	PlaceholderPrevMajor = "{prev_major}"
)

var placeholder = regexp.MustCompile(`\{[^{}]*\}`)

// checkTemplate reports placeholders a template of kind can't use.
func checkTemplate(kind, tmpl string) error {
	for _, p := range placeholder.FindAllString(tmpl, -1) {
		switch p {
		case PlaceholderVersion, PlaceholderMajor, PlaceholderMinor, PlaceholderTag:
		case PlaceholderPrevMinor, PlaceholderPrevMajor:
			if kind != "upgrade-guide" {
				return fmt.Errorf("placeholder %s works only in upgrade-guide sources", p)
			}
		default:
			return fmt.Errorf("unknown placeholder %s: want {version}, {major}, {minor}, {tag}, "+
				"{prev_minor} or {prev_major}", p)
		}
	}
	return nil
}

// sampleURL fills a URL template with sample values, so its host can be checked.
func sampleURL(tmpl string) string {
	return placeholder.ReplaceAllString(tmpl, "1")
}
