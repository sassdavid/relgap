package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	gogithub "github.com/google/go-github/v93/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/httpx"
)

// Errors returned by [Client] methods.
var (
	ErrAuth     = errors.New("github rejected the token")
	ErrNotFound = errors.New("not found on github")
)

const (
	defaultBatch       = 10
	defaultConcurrency = 8
)

// Release is a GitHub release. Body is empty in [Client.Releases] results when the client uses
// GraphQL; [Client.ReleaseBodies] fills it.
type Release struct {
	Tag         string
	Draft       bool
	Prerelease  bool
	PublishedAt time.Time
	URL         string
	Body        string
}

// Options configures [New].
type Options struct {
	Token       string // empty means unauthenticated REST calls
	Logger      *slog.Logger
	BatchSize   int // release bodies per GraphQL query; default 10
	Concurrency int // body queries in flight; default 8
}

// Client reads releases and tags of public or token-readable repositories.
type Client struct {
	logger      *slog.Logger
	gql         *githubv4.Client
	rest        *gogithub.Client
	batch       int
	concurrency int

	mu     sync.Mutex
	bodies map[string]map[string]Release // REST release lists by repo, then tag
}

// New returns a client that sends requests through hc and authenticates with o.Token.
func New(hc *http.Client, o Options) (*Client, error) {
	authed := &http.Client{Transport: &authTransport{token: o.Token, next: hc.Transport}}
	rest, err := gogithub.NewClient(gogithub.WithHTTPClient(authed))
	if err != nil {
		return nil, fmt.Errorf("create github rest client: %w", err)
	}
	c := &Client{
		logger:      o.Logger,
		rest:        rest,
		batch:       o.BatchSize,
		concurrency: o.Concurrency,
		bodies:      map[string]map[string]Release{},
	}
	if o.Token != "" {
		c.gql = githubv4.NewClient(authed)
	}
	if c.batch <= 0 {
		c.batch = defaultBatch
	}
	if c.concurrency <= 0 {
		c.concurrency = defaultConcurrency
	}
	return c, nil
}

// Authenticated reports whether the client has a token and uses GraphQL.
func (c *Client) Authenticated() bool { return c.gql != nil }

// Releases lists every release of repo ("owner/name"), drafts included, newest first.
func (c *Client) Releases(ctx context.Context, repo string) ([]Release, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	if c.gql != nil {
		return c.gqlReleases(ctx, owner, name)
	}
	rs, err := c.restReleases(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	byTag := make(map[string]Release, len(rs))
	for _, r := range rs {
		byTag[r.Tag] = r
	}
	c.mu.Lock()
	c.bodies[repo] = byTag
	c.mu.Unlock()
	return rs, nil
}

// Tags lists every tag name of repo.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	if c.gql != nil {
		return c.gqlTags(ctx, owner, name)
	}
	return c.restTags(ctx, owner, name)
}

// ReleaseBodies returns the releases of repo for tags, keyed by tag. Tags without a release are
// left out.
func (c *Client) ReleaseBodies(ctx context.Context, repo string, tags []string) (map[string]Release, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Release, len(tags))
	var mu sync.Mutex
	add := func(rs []Release) {
		mu.Lock()
		defer mu.Unlock()
		for _, r := range rs {
			out[r.Tag] = r
		}
	}
	missing := tags
	if c.gql == nil {
		missing = c.cachedBodies(repo, tags, add)
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)
	if c.gql != nil {
		for batch := range chunks(missing, c.batch) {
			g.Go(func() error {
				rs, batchErr := c.gqlBodies(gctx, owner, name, batch)
				add(rs)
				return batchErr
			})
		}
	} else {
		for _, tag := range missing {
			g.Go(func() error {
				r, tagErr := c.restRelease(gctx, owner, name, tag)
				if errors.Is(tagErr, ErrNotFound) {
					return nil
				}
				add([]Release{r})
				return tagErr
			})
		}
	}
	if err = g.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // the goroutines wrap their errors
	}
	return out, nil
}

// cachedBodies passes the releases a REST list already returned to add and returns the tags
// still missing.
func (c *Client) cachedBodies(repo string, tags []string, add func([]Release)) []string {
	c.mu.Lock()
	cached, listed := c.bodies[repo]
	c.mu.Unlock()
	if !listed {
		return tags
	}
	found := make([]Release, 0, len(tags))
	for _, t := range tags {
		if r, ok := cached[t]; ok {
			found = append(found, r)
		}
	}
	add(found)
	// The full list was read, so a tag missing from it has no release.
	return nil
}

func chunks(s []string, n int) func(func([]string) bool) {
	return func(yield func([]string) bool) {
		for i := 0; i < len(s); i += n {
			if !yield(s[i:min(i+n, len(s))]) {
				return
			}
		}
	}
}

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("invalid repository %q: want owner/name", repo)
	}
	return owner, name, nil
}

// authTransport adds the token to GitHub API requests and turns 401 into [ErrAuth].
type authTransport struct {
	token string
	next  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req
	if t.token != "" && req.URL.Host == httpx.GitHubAPIHost {
		out = req.Clone(req.Context())
		out.Header.Set("Authorization", "Bearer "+t.token)
	}
	resp, err := t.next.RoundTrip(out)
	if err != nil {
		return nil, err //nolint:wrapcheck // a transport passes errors through unchanged
	}
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w (HTTP 401); check GH_TOKEN or GITHUB_TOKEN, or run `gh auth login`", ErrAuth)
	}
	return resp, nil
}
