package forge

import (
	"net/url"
	"strconv"
	"strings"
)

// Known hosts. A tool without a host is on [GitHub].
const (
	GitHub    = "github.com"
	Bitbucket = "bitbucket.org"
	GitLab    = "gitlab.com"
)

// Kind says which API a host speaks.
type Kind string

// Host kinds.
const (
	KindGitHub    Kind = "github"
	KindBitbucket Kind = "bitbucket"
	KindGitLab    Kind = "gitlab"
)

// Canonical returns host in lowercase, with a forge's SSH host name replaced by its web host:
// Bitbucket Cloud's ssh.bitbucket.org and altssh.bitbucket.org, GitHub's ssh.github.com and
// GitLab.com's altssh.gitlab.com. git reaches the same repositories through them.
func Canonical(host string) string {
	switch host = strings.ToLower(host); host {
	case "ssh.bitbucket.org", "altssh.bitbucket.org":
		return Bitbucket
	case "ssh.github.com":
		return GitHub
	case "altssh.gitlab.com":
		return GitLab
	}
	return host
}

// KindOf returns the kind of host: an empty host or github.com is GitHub, bitbucket.org is
// Bitbucket Cloud, and any other host is GitLab, because only GitLab hosts can be configured.
func KindOf(host string) Kind {
	switch strings.ToLower(host) {
	case "", GitHub:
		return KindGitHub
	case Bitbucket:
		return KindBitbucket
	default:
		return KindGitLab
	}
}

// IsGitHub reports whether host is GitHub.
func IsGitHub(host string) bool { return KindOf(host) == KindGitHub }

// RepoURL returns the web page of repo on host.
func RepoURL(host, repo string) string {
	if host == "" {
		host = GitHub
	}
	return "https://" + host + "/" + repo
}

// FileURL returns the web page of path in repo on host at ref; an empty ref means HEAD.
func FileURL(host, repo, ref, path string) string {
	if ref == "" {
		ref = "HEAD"
	}
	path = strings.TrimPrefix(path, "/")
	switch KindOf(host) {
	case KindBitbucket:
		return RepoURL(host, repo) + "/src/" + ref + "/" + path
	case KindGitLab:
		return RepoURL(host, repo) + "/-/blob/" + ref + "/" + path
	case KindGitHub:
	}
	return RepoURL(host, repo) + "/blob/" + ref + "/" + path
}

// TagURL returns the web page of tag in repo on host.
func TagURL(host, repo, tag string) string {
	switch KindOf(host) {
	case KindBitbucket:
		return RepoURL(host, repo) + "/src/" + url.PathEscape(tag)
	case KindGitLab:
		return RepoURL(host, repo) + "/-/tags/" + url.PathEscape(tag)
	case KindGitHub:
	}
	return RepoURL(host, repo) + "/releases/tag/" + tag
}

// CompareURL returns the web page comparing tag from with tag to in repo on host.
func CompareURL(host, repo, from, to string) string {
	switch KindOf(host) {
	case KindBitbucket:
		return RepoURL(host, repo) + "/branches/compare/" + url.PathEscape(to) + "%0D" + url.PathEscape(from)
	case KindGitLab:
		return RepoURL(host, repo) + "/-/compare/" + from + "..." + to
	case KindGitHub:
	}
	return RepoURL(host, repo) + "/compare/" + from + "..." + to
}

// LineURL returns fileURL with the anchor of line: #lines-N on Bitbucket Cloud, #LN elsewhere.
func LineURL(fileURL string, line int) string {
	if u, err := url.Parse(fileURL); err == nil && strings.EqualFold(u.Hostname(), Bitbucket) {
		return fileURL + "#lines-" + strconv.Itoa(line)
	}
	return fileURL + "#L" + strconv.Itoa(line)
}
