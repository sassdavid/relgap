package github

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/shurcooL/githubv4"

	"github.com/sassdavid/relgap/internal/httpx"
)

// listPageSizes are tried in order when a page query fails, as Renovate does.
func listPageSizes() []int { return []int{100, 50, 25, 10} }

type pageInfo struct {
	HasNextPage bool
	EndCursor   githubv4.String
}

type releaseNode struct {
	TagName      string
	IsDraft      bool
	IsPrerelease bool
	PublishedAt  *githubv4.DateTime
	URL          githubv4.URI
}

func (n releaseNode) release(body string) Release {
	r := Release{Tag: n.TagName, Draft: n.IsDraft, Prerelease: n.IsPrerelease, Body: body}
	if n.PublishedAt != nil {
		r.PublishedAt = n.PublishedAt.UTC()
	}
	if n.URL.URL != nil {
		r.URL = n.URL.String()
	}
	return r
}

type bodyNode struct {
	releaseNode

	Description string
}

func (c *Client) gqlReleases(ctx context.Context, owner, name string) ([]Release, error) {
	var out []Release
	err := c.paginate(ctx, owner, name, func(ctx context.Context, vars map[string]any) (pageInfo, error) {
		var q struct {
			Repository *struct {
				Releases struct {
					Nodes    []releaseNode
					PageInfo pageInfo
				} `graphql:"releases(first: $first, after: $cursor, orderBy: {field: CREATED_AT, direction: DESC})"`
			} `graphql:"repository(owner: $owner, name: $name)"`
		}
		if err := c.gql.Query(ctx, &q, vars); err != nil {
			return pageInfo{}, err //nolint:wrapcheck // paginate wraps it
		}
		if q.Repository == nil {
			return pageInfo{}, ErrNotFound
		}
		for _, n := range q.Repository.Releases.Nodes {
			out = append(out, n.release(""))
		}
		return q.Repository.Releases.PageInfo, nil
	})
	if err != nil {
		return nil, fmt.Errorf("list releases of %s/%s: %w", owner, name, err)
	}
	return out, nil
}

func (c *Client) gqlTags(ctx context.Context, owner, name string) ([]string, error) {
	var out []string
	err := c.paginate(ctx, owner, name, func(ctx context.Context, vars map[string]any) (pageInfo, error) {
		var q struct {
			Repository *struct {
				Refs struct {
					Nodes    []struct{ Name string }
					PageInfo pageInfo
				} `graphql:"refs(refPrefix: \"refs/tags/\", first: $first, after: $cursor)"`
			} `graphql:"repository(owner: $owner, name: $name)"`
		}
		if err := c.gql.Query(ctx, &q, vars); err != nil {
			return pageInfo{}, err //nolint:wrapcheck // paginate wraps it
		}
		if q.Repository == nil {
			return pageInfo{}, ErrNotFound
		}
		for _, n := range q.Repository.Refs.Nodes {
			out = append(out, n.Name)
		}
		return q.Repository.Refs.PageInfo, nil
	})
	if err != nil {
		return nil, fmt.Errorf("list tags of %s/%s: %w", owner, name, err)
	}
	return out, nil
}

// paginate calls page until the last page, shrinking the page size after a failed query.
func (c *Client) paginate(
	ctx context.Context, owner, name string,
	page func(context.Context, map[string]any) (pageInfo, error),
) error {
	sizes := listPageSizes()
	var cursor *githubv4.String
	for {
		vars := map[string]any{
			"owner":  githubv4.String(owner),
			"name":   githubv4.String(name),
			"first":  githubv4.Int(sizes[0]), //nolint:gosec // page sizes are at most 100
			"cursor": cursor,
		}
		info, err := page(ctx, vars)
		if err != nil {
			if !shrinkable(ctx, err) || len(sizes) == 1 {
				return mapGraphQLError(err)
			}
			c.logger.DebugContext(ctx, "graphql page failed; retrying with a smaller page",
				"repo", owner+"/"+name, "size", sizes[1], "error", err)
			sizes = sizes[1:]
			continue
		}
		if !info.HasNextPage {
			return nil
		}
		next := info.EndCursor
		cursor = &next
	}
}

// gqlBodies fetches releases by tag with one aliased query. A failed query is split into halves
// (10, 5, 2, 1) because large bodies can make GitHub time out.
func (c *Client) gqlBodies(ctx context.Context, owner, name string, tags []string) ([]Release, error) {
	rs, err := c.gqlBodiesOnce(ctx, owner, name, tags)
	if err == nil || len(tags) == 1 || !shrinkable(ctx, err) {
		if err != nil {
			return nil, fmt.Errorf("fetch releases of %s/%s: %w", owner, name, mapGraphQLError(err))
		}
		return rs, nil
	}
	size := max(len(tags)/2, 1) //nolint:mnd // halve the batch
	c.logger.DebugContext(ctx, "graphql batch failed; retrying with smaller batches",
		"repo", owner+"/"+name, "size", size, "error", err)
	var out []Release
	for batch := range chunks(tags, size) {
		part, partErr := c.gqlBodies(ctx, owner, name, batch)
		if partErr != nil {
			return nil, partErr
		}
		out = append(out, part...)
	}
	return out, nil
}

func (c *Client) gqlBodiesOnce(ctx context.Context, owner, name string, tags []string) ([]Release, error) {
	fields := make([]reflect.StructField, len(tags))
	vars := map[string]any{"owner": githubv4.String(owner), "name": githubv4.String(name)}
	for i, tag := range tags {
		n := strconv.Itoa(i)
		fields[i] = reflect.StructField{
			Name: "R" + n,
			Type: reflect.TypeFor[*bodyNode](),
			Tag:  reflect.StructTag(`graphql:"r` + n + `: release(tagName: $t` + n + `)"`),
		}
		vars["t"+n] = githubv4.String(tag)
	}
	repoType := reflect.PointerTo(reflect.StructOf(fields))
	q := reflect.New(reflect.StructOf([]reflect.StructField{{
		Name: "Repository",
		Type: repoType,
		Tag:  `graphql:"repository(owner: $owner, name: $name)"`,
	}}))
	if err := c.gql.Query(ctx, q.Interface(), vars); err != nil {
		return nil, err //nolint:wrapcheck // gqlBodies wraps it
	}
	repo := q.Elem().Field(0)
	if repo.IsNil() {
		return nil, ErrNotFound
	}
	out := make([]Release, 0, len(tags))
	for i := range tags {
		if n, ok := reflect.TypeAssert[*bodyNode](repo.Elem().Field(i)); ok && n != nil {
			out = append(out, n.release(n.Description))
		}
	}
	return out, nil
}

// shrinkable reports whether a smaller query might succeed where err failed.
func shrinkable(ctx context.Context, err error) bool {
	return ctx.Err() == nil && !errors.Is(err, ErrAuth) && !errors.Is(err, ErrNotFound) &&
		!errors.Is(err, httpx.ErrRateLimited)
}

func mapGraphQLError(err error) error {
	if err != nil && strings.Contains(err.Error(), "Could not resolve to a Repository") {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
