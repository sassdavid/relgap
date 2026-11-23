package github

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

// ErrPrivate means GitHub answered that a repository isn't public.
var ErrPrivate = errors.New("the repository is private")

// VisibilityBatch is the most repositories one GraphQL query of [Client.Visibility] asks about.
const VisibilityBatch = 100

type repoVisibility struct {
	IsPrivate bool
}

// Visibility reports for each repository ("owner/name") whether it is public: nil for a public
// repository, [ErrPrivate] for a private or internal one, [ErrNotFound] for one the client can't
// see, else the error of the lookup. With a token it sends one aliased GraphQL query per
// [VisibilityBatch] repositories; without one it fetches each repository with REST, where any
// answer means public. The error is set only for a canceled context.
func (c *Client) Visibility(ctx context.Context, repos []string) (map[string]error, error) {
	out := make(map[string]error, len(repos))
	var valid []string
	for _, r := range repos {
		if _, done := out[r]; done {
			continue
		}
		if _, _, err := splitRepo(r); err != nil {
			out[r] = err
			continue
		}
		out[r] = nil
		valid = append(valid, r)
	}
	results := make([]error, len(valid))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)
	if c.gql != nil {
		for i := 0; i < len(valid); i += VisibilityBatch {
			end := min(i+VisibilityBatch, len(valid))
			g.Go(func() error {
				copy(results[i:end], c.gqlVisibility(gctx, valid[i:end]))
				return nil
			})
		}
	} else {
		for i, r := range valid {
			g.Go(func() error {
				results[i] = c.restVisibility(gctx, r)
				return nil
			})
		}
	}
	_ = g.Wait() // the goroutines report errors per repository
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("look up repository visibility: %w", err)
	}
	for i, r := range valid {
		out[r] = results[i]
	}
	return out, nil
}

// gqlVisibility asks about every repository in repos with one aliased query.
func (c *Client) gqlVisibility(ctx context.Context, repos []string) []error {
	fields := make([]reflect.StructField, len(repos))
	vars := map[string]any{}
	for i, r := range repos {
		owner, name, _ := splitRepo(r)
		n := strconv.Itoa(i)
		fields[i] = reflect.StructField{
			Name: "R" + n,
			Type: reflect.TypeFor[*repoVisibility](),
			Tag:  reflect.StructTag(`graphql:"r` + n + `: repository(owner: $o` + n + `, name: $n` + n + `)"`),
		}
		vars["o"+n] = githubv4.String(owner)
		vars["n"+n] = githubv4.String(name)
	}
	q := reflect.New(reflect.StructOf(fields))
	err := c.gql.Query(ctx, q.Interface(), vars)
	out := make([]error, len(repos))
	for i, r := range repos {
		f := q.Elem().Field(i)
		if f.IsNil() {
			out[i] = visibilityError(r, err)
			continue
		}
		if v, _ := reflect.TypeAssert[*repoVisibility](f); v.IsPrivate {
			out[i] = fmt.Errorf("%s: %w", r, ErrPrivate)
		}
	}
	return out
}

// visibilityError explains why a repository came back empty: GraphQL reports a repository it
// can't see as an error next to the data of the others.
func visibilityError(repo string, err error) error {
	if err == nil {
		return fmt.Errorf("look up the visibility of %s: %w", repo, ErrNotFound)
	}
	return fmt.Errorf("look up the visibility of %s: %w", repo, mapGraphQLError(err))
}

func (c *Client) restVisibility(ctx context.Context, repo string) error {
	owner, name, _ := splitRepo(repo)
	r, _, err := c.rest.Repositories.Get(ctx, owner, name)
	if err != nil {
		return fmt.Errorf("look up the visibility of %s: %w", repo, mapRESTError(err))
	}
	if r.GetPrivate() || r.GetVisibility() != "" && r.GetVisibility() != "public" {
		return fmt.Errorf("%s: %w", repo, ErrPrivate)
	}
	return nil
}
