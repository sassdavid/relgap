package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gogithub "github.com/google/go-github/v93/github"
)

const restPageSize = 100

func (c *Client) restReleases(ctx context.Context, owner, name string) ([]Release, error) {
	var out []Release
	opts := &gogithub.ListOptions{PerPage: restPageSize}
	for {
		page, resp, err := c.rest.Repositories.ListReleases(ctx, owner, name, opts)
		if err != nil {
			return nil, fmt.Errorf("list releases of %s/%s: %w", owner, name, mapRESTError(err))
		}
		for _, r := range page {
			out = append(out, restRelease(r))
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		opts.Page = resp.NextPage
	}
}

func (c *Client) restTags(ctx context.Context, owner, name string) ([]string, error) {
	var out []string
	opts := &gogithub.ListOptions{PerPage: restPageSize}
	for {
		page, resp, err := c.rest.Repositories.ListTags(ctx, owner, name, opts)
		if err != nil {
			return nil, fmt.Errorf("list tags of %s/%s: %w", owner, name, mapRESTError(err))
		}
		for _, t := range page {
			out = append(out, t.GetName())
		}
		if resp.NextPage == 0 {
			return out, nil
		}
		opts.Page = resp.NextPage
	}
}

func (c *Client) restRelease(ctx context.Context, owner, name, tag string) (Release, error) {
	r, _, err := c.rest.Repositories.GetReleaseByTag(ctx, owner, name, tag)
	if err != nil {
		return Release{}, fmt.Errorf("fetch release %s of %s/%s: %w", tag, owner, name, mapRESTError(err))
	}
	return restRelease(r), nil
}

func restRelease(r *gogithub.RepositoryRelease) Release {
	out := Release{
		Tag:        r.TagName,
		Draft:      r.Draft,
		Prerelease: r.Prerelease,
		URL:        r.HTMLURL,
		Body:       r.GetBody(),
	}
	if r.PublishedAt != nil {
		out.PublishedAt = r.PublishedAt.UTC()
	}
	return out
}

func mapRESTError(err error) error {
	if er, ok := errors.AsType[*gogithub.ErrorResponse](err); ok && er.Response != nil &&
		er.Response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
