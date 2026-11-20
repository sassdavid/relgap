package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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

// File returns the content of path in repo at ref (empty means the default branch) through the
// contents API with the raw media type, which works for files over 1 MB.
func (c *Client) File(ctx context.Context, repo, ref, path string) ([]byte, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("repos/%s/%s/contents/%s", owner, name, escapePath(path))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := c.rest.NewRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch %s of %s at %q: %w", path, repo, ref, err)
	}
	req.Header.Set("Accept", rawMediaType)
	var buf bytes.Buffer
	if _, err = c.rest.Do(req, &buf); err != nil {
		return nil, fmt.Errorf("fetch %s of %s at %q: %w", path, repo, ref, mapRESTError(err))
	}
	return buf.Bytes(), nil
}

const rawMediaType = "application/vnd.github.raw"

func escapePath(p string) string {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// File is an entry of a repository directory listing.
type File struct {
	Name string `json:"name"`
	Size int    `json:"size"` // -1 when the host doesn't say
	Dir  bool   `json:"dir"`
}

// RootFiles lists the top directory of repo on its default branch through the contents API.
func (c *Client) RootFiles(ctx context.Context, repo string) ([]File, error) {
	return c.listDir(ctx, repo, "", "")
}

// Dir returns the names of the files, not directories, in dir of repo at ref.
func (c *Client) Dir(ctx context.Context, repo, ref, dir string) ([]string, error) {
	files, err := c.listDir(ctx, repo, ref, dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if !f.Dir {
			out = append(out, f.Name)
		}
	}
	return out, nil
}

func (c *Client) listDir(ctx context.Context, repo, ref, dir string) ([]File, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	var opts *gogithub.RepositoryContentGetOptions
	if ref != "" {
		opts = &gogithub.RepositoryContentGetOptions{Ref: ref}
	}
	_, list, _, err := c.rest.Repositories.GetContents(ctx, owner, name, dir, opts)
	if err != nil {
		return nil, fmt.Errorf("list files of %s: %w", repo, mapRESTError(err))
	}
	out := make([]File, 0, len(list))
	for _, f := range list {
		out = append(out, File{Name: f.GetName(), Size: f.GetSize(), Dir: f.GetType() == "dir"})
	}
	return out, nil
}

// Repo is one repository of a search result.
type Repo struct {
	FullName string
	Stars    int
}

const searchPageSize = 30

// SearchRepos returns up to 30 repositories whose name contains name, most stars first.
func (c *Client) SearchRepos(ctx context.Context, name string) ([]Repo, error) {
	res, _, err := c.rest.Search.Repositories(ctx, name+" in:name", &gogithub.SearchOptions{
		Sort: "stars", Order: "desc", PerPage: searchPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("search repositories for %s: %w", name, mapRESTError(err))
	}
	out := make([]Repo, 0, len(res.Repositories))
	for _, r := range res.Repositories {
		out = append(out, Repo{FullName: r.GetFullName(), Stars: r.GetStargazersCount()})
	}
	return out, nil
}
