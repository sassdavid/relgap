package github

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	gogithub "github.com/google/go-github/v93/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

const (
	// RecentWindow is how many of the newest releases or tags [Client.Recent] reads per repo.
	RecentWindow = 30
	// RecentBatch is how many repositories one GraphQL query of [Client.Recent] asks about.
	RecentBatch = 25
)

// RecentQuery asks for the newest releases of Repo, or its newest tags when Tags is set.
type RecentQuery struct {
	Repo string // owner/name
	Tags bool
}

// Recent is the answer for one repository. Tag-only answers have only Release.Tag set.
type Recent struct {
	Releases []Release // newest first, at most [RecentWindow]
	Err      error
}

// Recent returns the newest releases or tags of every repository, keyed by repo. With a token it
// sends one GraphQL query per [RecentBatch] repositories; without one it makes REST calls per repo.
// The error is set only when nothing could be asked; per-repo failures are in [Recent.Err].
func (c *Client) Recent(ctx context.Context, qs []RecentQuery) (map[string]Recent, error) {
	out := make(map[string]Recent, len(qs))
	var mu sync.Mutex
	set := func(repo string, r Recent) {
		mu.Lock()
		defer mu.Unlock()
		out[repo] = r
	}
	valid := make([]RecentQuery, 0, len(qs))
	seen := map[string]bool{}
	for _, q := range qs {
		if seen[q.Repo] {
			continue
		}
		seen[q.Repo] = true
		if _, _, err := splitRepo(q.Repo); err != nil {
			out[q.Repo] = Recent{Err: err}
			continue
		}
		valid = append(valid, q)
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)
	if c.gql != nil {
		for i := 0; i < len(valid); i += RecentBatch {
			batch := valid[i:min(i+RecentBatch, len(valid))]
			g.Go(func() error {
				for repo, r := range c.gqlRecent(gctx, batch) {
					set(repo, r)
				}
				return nil
			})
		}
	} else {
		for _, q := range valid {
			g.Go(func() error {
				rs, err := c.restRecent(gctx, q)
				set(q.Repo, Recent{Releases: rs, Err: err})
				return nil
			})
		}
	}
	_ = g.Wait() // the goroutines report errors per repo
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list recent releases: %w", err)
	}
	return out, nil
}

type recentReleases struct {
	Releases struct {
		Nodes []releaseNode
	} `graphql:"releases(first: $first, orderBy: {field: CREATED_AT, direction: DESC})"`
}

type recentTags struct {
	Refs struct {
		Nodes []struct{ Name string }
	} `graphql:"refs(refPrefix: \"refs/tags/\", first: $first, orderBy: {field: TAG_COMMIT_DATE, direction: DESC})"`
}

// gqlRecent asks about every repository in qs with one aliased query.
func (c *Client) gqlRecent(ctx context.Context, qs []RecentQuery) map[string]Recent {
	fields := make([]reflect.StructField, len(qs))
	vars := map[string]any{"first": githubv4.Int(RecentWindow)}
	for i, q := range qs {
		owner, name, _ := splitRepo(q.Repo)
		n := strconv.Itoa(i)
		typ := reflect.TypeFor[*recentReleases]()
		if q.Tags {
			typ = reflect.TypeFor[*recentTags]()
		}
		fields[i] = reflect.StructField{
			Name: "R" + n,
			Type: typ,
			Tag:  reflect.StructTag(`graphql:"r` + n + `: repository(owner: $o` + n + `, name: $n` + n + `)"`),
		}
		vars["o"+n] = githubv4.String(owner)
		vars["n"+n] = githubv4.String(name)
	}
	q := reflect.New(reflect.StructOf(fields))
	err := c.gql.Query(ctx, q.Interface(), vars)
	out := make(map[string]Recent, len(qs))
	for i, rq := range qs {
		f := q.Elem().Field(i)
		if f.IsNil() {
			out[rq.Repo] = Recent{Err: recentError(rq.Repo, err)}
			continue
		}
		var rs []Release
		if rq.Tags {
			n, _ := reflect.TypeAssert[*recentTags](f)
			for _, t := range n.Refs.Nodes {
				rs = append(rs, Release{Tag: t.Name})
			}
		} else {
			n, _ := reflect.TypeAssert[*recentReleases](f)
			for _, r := range n.Releases.Nodes {
				rs = append(rs, r.release(""))
			}
		}
		out[rq.Repo] = Recent{Releases: rs}
	}
	return out
}

// recentError explains why a repository came back empty. GraphQL reports a missing repository as
// an error next to the data of the others.
func recentError(repo string, err error) error {
	if err == nil {
		return fmt.Errorf("list recent releases of %s: %w", repo, ErrNotFound)
	}
	if strings.Contains(err.Error(), "Could not resolve to a Repository") {
		return fmt.Errorf("list recent releases of %s: %w", repo, ErrNotFound)
	}
	return fmt.Errorf("list recent releases of %s: %w", repo, err)
}

func (c *Client) restRecent(ctx context.Context, q RecentQuery) ([]Release, error) {
	owner, name, _ := splitRepo(q.Repo)
	if q.Tags {
		// The REST tag list isn't ordered by date, so only the whole list finds the newest.
		tags, err := c.restTags(ctx, owner, name)
		if err != nil {
			return nil, err
		}
		out := make([]Release, len(tags))
		for i, t := range tags {
			out[i] = Release{Tag: t}
		}
		return out, nil
	}
	page, _, err := c.rest.Repositories.ListReleases(ctx, owner, name, &gogithub.ListOptions{PerPage: RecentWindow})
	if err != nil {
		return nil, fmt.Errorf("list recent releases of %s: %w", q.Repo, mapRESTError(err))
	}
	out := make([]Release, len(page))
	for i, r := range page {
		out[i] = restRelease(r)
	}
	return out, nil
}
