package cli

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/gitlab"
	"github.com/sassdavid/relgap/internal/httpx"
)

// forgeVisibility asks github.com and gitlab.com whether repositories are public, for
// [enrich.Visibility].
type forgeVisibility struct {
	gh    *github.Client
	hosts *hostClients
}

func (v forgeVisibility) Public(ctx context.Context, repos []enrich.Repo) (map[enrich.Repo]error, error) {
	out := make(map[enrich.Repo]error, len(repos))
	var onGitHub []string
	var onGitLab []enrich.Repo
	for _, r := range repos {
		switch {
		case forge.IsGitHub(r.Host):
			onGitHub = append(onGitHub, r.Path)
		case r.Host == forge.GitLab:
			onGitLab = append(onGitLab, r)
		default:
			out[r] = fmt.Errorf("%s: %w", r.ModulePath(), enrich.ErrNotPublic)
		}
	}
	if len(onGitHub) > 0 {
		answers, err := v.gh.Visibility(ctx, onGitHub)
		if err != nil {
			return nil, err //nolint:wrapcheck // only a canceled context
		}
		for _, r := range repos {
			if forge.IsGitHub(r.Host) {
				out[r] = notPublic(answers[r.Path], github.ErrPrivate, github.ErrNotFound)
			}
		}
	}
	if len(onGitLab) > 0 {
		c, err := v.hosts.client(forge.GitLab)
		if err != nil {
			return nil, err
		}
		gl, _ := c.(*gitlab.Client)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, r := range onGitLab {
			wg.Go(func() {
				answer := notPublic(gl.Visibility(ctx, r.Path),
					gitlab.ErrNotPublic, httpx.ErrNotFound, httpx.ErrUnauthorized)
				mu.Lock()
				out[r] = answer
				mu.Unlock()
			})
		}
		wg.Wait()
		if err = ctx.Err(); err != nil {
			return nil, fmt.Errorf("look up repository visibility: %w", err)
		}
	}
	return out, nil
}

// notPublic marks err as [enrich.ErrNotPublic] when the forge answered with one of private: the
// repository is private, or the client can't see it.
func notPublic(err error, private ...error) error {
	for _, p := range private {
		if errors.Is(err, p) {
			return fmt.Errorf("%w: %w", enrich.ErrNotPublic, err)
		}
	}
	return err
}

// privacy returns what the session keeps away from OSV: the GOPRIVATE patterns of the
// environment and the go command's settings, and the forges' answers about repositories.
func (s *session) privacy(ctx context.Context, gh *github.Client) enrich.Privacy {
	p := enrich.Privacy{
		Go:    enrich.ReadGoPrivate(s.d.Env.Lookup, runtime.GOOS),
		Repos: forgeVisibility{gh: gh, hosts: s.hosts},
	}
	if patterns := p.Go.Patterns(); len(patterns) > 0 {
		s.d.Logger.DebugContext(ctx, "module paths kept away from OSV", "goprivate", patterns)
	}
	return p
}
