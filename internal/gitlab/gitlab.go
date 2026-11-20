package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/version"
)

// Token header names. A personal, project or group token goes in [HeaderPrivateToken], a CI job
// token in [HeaderJobToken].
const (
	HeaderPrivateToken = "PRIVATE-TOKEN"
	HeaderJobToken     = "JOB-TOKEN"
)

const (
	perPage  = 100
	maxPages = 100
)

// ErrBadRepo is wrapped by errors for an empty project path.
var ErrBadRepo = errors.New("not a gitlab project path")

// Options configures [New].
type Options struct {
	Host        string // gitlab.com or a self-managed host
	Token       string // empty means anonymous requests
	TokenHeader string // [HeaderPrivateToken] (the default) or [HeaderJobToken]
	Scheme      string // https unless a test sets http
}

// Client reads tags, releases and files of projects on one GitLab host.
type Client struct {
	hc     *http.Client
	host   string
	base   string
	token  string
	header string

	mu       sync.Mutex
	releases map[string]*releaseList
}

type releaseList struct {
	once  sync.Once
	byTag map[string]github.Release
	list  []github.Release
	err   error
}

// New returns a client for o.Host that sends requests through hc. The token goes only to that host.
func New(hc *http.Client, o Options) *Client {
	scheme := o.Scheme
	if scheme == "" {
		scheme = "https"
	}
	header := o.TokenHeader
	if header == "" {
		header = HeaderPrivateToken
	}
	return &Client{
		hc: hc, host: o.Host, base: scheme + "://" + o.Host + "/api/v4", token: o.Token, header: header,
		releases: map[string]*releaseList{},
	}
}

// Host returns the host the client reads from.
func (c *Client) Host() string { return c.host }

// Authenticated reports whether the client has a token.
func (c *Client) Authenticated() bool { return c.token != "" }

// projectURL returns the API URL of a project, whose full path is one URL-encoded segment.
func (c *Client) projectURL(repo string) (string, error) {
	repo = strings.Trim(repo, "/")
	if repo == "" || !strings.Contains(repo, "/") {
		return "", fmt.Errorf("%w: %q", ErrBadRepo, repo)
	}
	return c.base + "/projects/" + strings.ReplaceAll(url.PathEscape(repo), "/", "%2F"), nil
}

func (c *Client) headers(accept string) http.Header {
	h := http.Header{}
	if accept != "" {
		h.Set("Accept", accept)
	}
	if c.token != "" {
		h.Set(c.header, c.token)
	}
	return h
}

func (c *Client) get(ctx context.Context, rawURL, accept string) ([]byte, http.Header, error) {
	if !httpx.SameOrigin(c.base, rawURL) {
		return nil, nil, fmt.Errorf("gitlab: refusing to follow %s off %s", rawURL, c.host)
	}
	body, h, err := httpx.Get(ctx, c.hc, rawURL, c.headers(accept))
	if err != nil {
		return nil, nil, fmt.Errorf("gitlab: %w", err)
	}
	return body, h, nil
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// pages follows the Link rel="next" headers of a paged list from first.
func pages[T any](ctx context.Context, c *Client, first string) ([]T, error) {
	var out []T
	next := first
	for i := 0; next != "" && i < maxPages; i++ {
		body, h, err := c.get(ctx, next, "application/json")
		if err != nil {
			return nil, err
		}
		var items []T
		if err = json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("gitlab: decode %s: %w", next, err)
		}
		out = append(out, items...)
		next = ""
		if m := nextLink.FindStringSubmatch(h.Get("Link")); m != nil {
			next = m[1]
		}
	}
	return out, nil
}

type tag struct {
	Name   string `json:"name"`
	Commit struct {
		CreatedAt time.Time `json:"created_at"`
	} `json:"commit"`
}

// TagRefs lists every tag of repo with its commit date.
func (c *Client) TagRefs(ctx context.Context, repo string) ([]version.Ref, error) {
	base, err := c.projectURL(repo)
	if err != nil {
		return nil, err
	}
	tags, err := pages[tag](ctx, c, fmt.Sprintf("%s/repository/tags?per_page=%d", base, perPage))
	if err != nil {
		return nil, fmt.Errorf("list tags of %s: %w", repo, err)
	}
	refs := make([]version.Ref, len(tags))
	for i, t := range tags {
		refs[i] = version.Ref{Tag: t.Name, PublishedAt: t.Commit.CreatedAt}
	}
	return refs, nil
}

// Tags lists every tag name of repo.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	refs, err := c.TagRefs(ctx, repo)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Tag
	}
	return out, nil
}

type release struct {
	TagName         string    `json:"tag_name"`
	Description     string    `json:"description"`
	ReleasedAt      time.Time `json:"released_at"`
	UpcomingRelease bool      `json:"upcoming_release"`
}

// Releases lists every release of repo with its notes. An upcoming release counts as a draft.
func (c *Client) Releases(ctx context.Context, repo string) ([]github.Release, error) {
	l := c.releaseList(ctx, repo)
	return l.list, l.err
}

// ReleaseBodies returns the releases of repo for tags, keyed by tag. The release list is read
// once per client.
func (c *Client) ReleaseBodies(ctx context.Context, repo string, tags []string) (map[string]github.Release, error) {
	l := c.releaseList(ctx, repo)
	if l.err != nil {
		return nil, l.err
	}
	out := make(map[string]github.Release, len(tags))
	for _, t := range tags {
		if r, ok := l.byTag[t]; ok {
			out[t] = r
		}
	}
	return out, nil
}

func (c *Client) releaseList(ctx context.Context, repo string) *releaseList {
	c.mu.Lock()
	l, ok := c.releases[repo]
	if !ok {
		l = &releaseList{}
		c.releases[repo] = l
	}
	c.mu.Unlock()
	l.once.Do(func() {
		base, err := c.projectURL(repo)
		if err != nil {
			l.err = err
			return
		}
		rs, err := pages[release](ctx, c, fmt.Sprintf("%s/releases?per_page=%d", base, perPage))
		if err != nil {
			l.err = fmt.Errorf("list releases of %s: %w", repo, err)
			return
		}
		l.byTag = make(map[string]github.Release, len(rs))
		for _, r := range rs {
			rel := github.Release{
				Tag:         r.TagName,
				Draft:       r.UpcomingRelease,
				PublishedAt: r.ReleasedAt,
				URL:         forge.RepoURL(c.host, repo) + "/-/releases/" + url.PathEscape(r.TagName),
				Body:        r.Description,
			}
			l.list = append(l.list, rel)
			l.byTag[r.TagName] = rel
		}
	})
	return l
}

// File returns path in repo at ref; an empty ref means the default branch.
func (c *Client) File(ctx context.Context, repo, ref, path string) ([]byte, error) {
	base, err := c.projectURL(repo)
	if err != nil {
		return nil, err
	}
	u := base + "/repository/files/" + strings.ReplaceAll(url.PathEscape(strings.TrimPrefix(path, "/")), "/", "%2F") +
		"/raw"
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	body, _, err := c.get(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("fetch %s of %s at %q: %w", path, repo, ref, err)
	}
	return body, nil
}

type treeEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // blob or tree
}

// RootFiles lists the top directory of repo on its default branch. The tree has no sizes, so
// each file has size -1.
func (c *Client) RootFiles(ctx context.Context, repo string) ([]github.File, error) {
	base, err := c.projectURL(repo)
	if err != nil {
		return nil, err
	}
	entries, err := pages[treeEntry](ctx, c, fmt.Sprintf("%s/repository/tree?per_page=%d", base, perPage))
	if err != nil {
		return nil, fmt.Errorf("list files of %s: %w", repo, err)
	}
	out := make([]github.File, len(entries))
	for i, e := range entries {
		out[i] = github.File{Name: e.Name, Size: -1, Dir: e.Type == "tree"}
	}
	return out, nil
}
