package bitbucket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/version"
)

// APIHost is the host of the Bitbucket Cloud REST API.
const APIHost = "api.bitbucket.org"

// TokenEnv is the environment variable whose token the client sends as Bearer.
const TokenEnv = "BITBUCKET_API_TOKEN"

const (
	defaultBase = "https://" + APIHost + "/2.0"
	pageLen     = 100
	maxPages    = 100
)

// ErrBadRepo is wrapped by errors for a repository that isn't workspace/repo.
var ErrBadRepo = errors.New("not a bitbucket workspace/repo")

// Options configures [New].
type Options struct {
	Token string // empty means anonymous requests (60 an hour per IP)
	Base  string // API base URL; empty means https://api.bitbucket.org/2.0
}

// Client reads tags and files of Bitbucket Cloud repositories. Bitbucket Cloud has no releases.
type Client struct {
	hc    *http.Client
	base  string
	token string

	mu     sync.Mutex
	hashes map[string]map[string]string // tag commit hashes by repo, then tag
}

// New returns a client that sends requests through hc. The token goes only to the API host.
func New(hc *http.Client, o Options) *Client {
	base := o.Base
	if base == "" {
		base = defaultBase
	}
	return &Client{hc: hc, base: strings.TrimRight(base, "/"), token: o.Token, hashes: map[string]map[string]string{}}
}

// Authenticated reports whether the client has a token.
func (c *Client) Authenticated() bool { return c.token != "" }

func splitRepo(repo string) (string, string, error) {
	ws, slug, ok := strings.Cut(repo, "/")
	if !ok || ws == "" || slug == "" || strings.Contains(slug, "/") {
		return "", "", fmt.Errorf("%w: %q", ErrBadRepo, repo)
	}
	return ws, slug, nil
}

func (c *Client) repoURL(repo string) (string, error) {
	ws, slug, err := splitRepo(repo)
	if err != nil {
		return "", err
	}
	return c.base + "/repositories/" + url.PathEscape(ws) + "/" + url.PathEscape(slug), nil
}

func (c *Client) header() http.Header {
	h := http.Header{"Accept": {"application/json"}}
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	return h
}

// get fetches rawURL, sending the token only when rawURL is on the API base.
func (c *Client) get(ctx context.Context, rawURL string, h http.Header) ([]byte, error) {
	if !httpx.SameOrigin(c.base, rawURL) {
		return nil, fmt.Errorf("bitbucket: refusing to follow %s off %s", rawURL, APIHost)
	}
	body, _, err := httpx.Get(ctx, c.hc, rawURL, h)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: %w", err)
	}
	return body, nil
}

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

// pages follows the next links of a paged list from first.
func pages[T any](ctx context.Context, c *Client, first string) ([]T, error) {
	var out []T
	next := first
	for i := 0; next != "" && i < maxPages; i++ {
		body, err := c.get(ctx, next, c.header())
		if err != nil {
			return nil, err
		}
		var p page[T]
		if err = json.Unmarshal(body, &p); err != nil {
			return nil, fmt.Errorf("bitbucket: decode %s: %w", next, err)
		}
		out = append(out, p.Values...)
		next = p.Next
	}
	return out, nil
}

type tag struct {
	Name   string `json:"name"`
	Target struct {
		Hash string    `json:"hash"`
		Date time.Time `json:"date"`
	} `json:"target"`
}

// TagRefs lists every tag of repo with its commit date.
func (c *Client) TagRefs(ctx context.Context, repo string) ([]version.Ref, error) {
	base, err := c.repoURL(repo)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"pagelen": {strconv.Itoa(pageLen)},
		"fields":  {"values.name,values.target.hash,values.target.date,next"},
	}
	tags, err := pages[tag](ctx, c, base+"/refs/tags?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("list tags of %s: %w", repo, err)
	}
	refs := make([]version.Ref, len(tags))
	hashes := make(map[string]string, len(tags))
	for i, t := range tags {
		refs[i] = version.Ref{Tag: t.Name, PublishedAt: t.Target.Date}
		hashes[t.Name] = t.Target.Hash
	}
	c.mu.Lock()
	c.hashes[repo] = hashes
	c.mu.Unlock()
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

// Releases lists the tags of repo as releases without bodies, because Bitbucket Cloud has none.
func (c *Client) Releases(ctx context.Context, repo string) ([]github.Release, error) {
	refs, err := c.TagRefs(ctx, repo)
	if err != nil {
		return nil, err
	}
	out := make([]github.Release, len(refs))
	for i, r := range refs {
		out[i] = github.Release{Tag: r.Tag, PublishedAt: r.PublishedAt, URL: forge.TagURL(forge.Bitbucket, repo, r.Tag)}
	}
	return out, nil
}

// ReleaseBodies returns no releases: Bitbucket Cloud has none, so notes come from files.
func (c *Client) ReleaseBodies(context.Context, string, []string) (map[string]github.Release, error) {
	return map[string]github.Release{}, nil
}

// File returns path in repo at ref; an empty ref means the default branch. A ref with a slash is
// looked up as a tag, because the src endpoint can't take one.
func (c *Client) File(ctx context.Context, repo, ref, path string) ([]byte, error) {
	base, err := c.repoURL(repo)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		ref = "HEAD"
	}
	if strings.Contains(ref, "/") {
		if ref, err = c.tagHash(ctx, repo, ref); err != nil {
			return nil, err
		}
	}
	h := c.header()
	h.Del("Accept")
	body, err := c.get(ctx, base+"/src/"+url.PathEscape(ref)+"/"+escapePath(path), h)
	if err != nil {
		return nil, fmt.Errorf("fetch %s of %s at %q: %w", path, repo, ref, err)
	}
	return body, nil
}

func (c *Client) tagHash(ctx context.Context, repo, ref string) (string, error) {
	c.mu.Lock()
	hashes, listed := c.hashes[repo]
	c.mu.Unlock()
	if !listed {
		if _, err := c.TagRefs(ctx, repo); err != nil {
			return "", err
		}
		c.mu.Lock()
		hashes = c.hashes[repo]
		c.mu.Unlock()
	}
	if h, ok := hashes[ref]; ok && h != "" {
		return h, nil
	}
	return "", fmt.Errorf("bitbucket: ref %q of %s: %w", ref, repo, httpx.ErrNotFound)
}

type entry struct {
	Path string `json:"path"`
	Type string `json:"type"` // commit_file or commit_directory
	Size int    `json:"size"`
}

// RootFiles lists the top directory of repo on its default branch.
func (c *Client) RootFiles(ctx context.Context, repo string) ([]github.File, error) {
	base, err := c.repoURL(repo)
	if err != nil {
		return nil, err
	}
	entries, err := pages[entry](ctx, c, base+"/src/HEAD/?pagelen="+strconv.Itoa(pageLen))
	if err != nil {
		return nil, fmt.Errorf("list files of %s: %w", repo, err)
	}
	out := make([]github.File, len(entries))
	for i, e := range entries {
		out[i] = github.File{Name: e.Path, Size: e.Size, Dir: e.Type == "commit_directory"}
	}
	return out, nil
}

func escapePath(p string) string {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
