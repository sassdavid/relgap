package cli

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/registry/remote"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// commandRunner starts external commands; [execx.Runner] implements it.
type commandRunner interface {
	Run(ctx context.Context, c execx.Cmd) (*execx.Result, error)
}

// session holds what the digest, outdated, drift and resolve pipelines share: the loaded config,
// the tool resolver, the command runner and the HTTP clients, built on first use. Each CLI
// command has one; `relgap mcp` has one for all its calls.
type session struct {
	d        *app.Deps
	resolver *registry.Resolver
	runner   commandRunner
	// tokenRunner runs `gh auth token`; nil skips it.
	tokenRunner github.Runner
	newCache    func() (*httpx.Cache, error)
	// guess lets resolve search GitHub for names no registry knows (--guess).
	guess bool

	once  sync.Once
	cache *httpx.Cache
	gh    *github.Client
	hc    *http.Client
	err   error
}

// newSession loads the config and the registries for a CLI command.
func (g *globals) newSession(cmd *cobra.Command, d *app.Deps) (*session, error) {
	resolver, _, err := g.loadResolver(cmd, d)
	if err != nil {
		return nil, err
	}
	runner := newRunner(d)
	return &session{
		d:           d,
		resolver:    resolver,
		runner:      runner,
		tokenRunner: runner,
		newCache:    func() (*httpx.Cache, error) { return g.newCache(d) },
	}, nil
}

// clients returns the GitHub client and the plain HTTP client, building them and the cache on
// the first call.
func (s *session) clients(ctx context.Context) (*github.Client, *http.Client, error) {
	s.once.Do(func() {
		s.cache, s.err = s.newCache()
		if s.err != nil {
			return
		}
		s.gh, s.hc, s.err = newGitHubClient(ctx, s.d, s.cache, s.tokenRunner)
	})
	return s.gh, s.hc, s.err
}

// close logs the cache summary and closes the cache.
func (s *session) close(ctx context.Context) {
	if s.cache != nil {
		s.cache.LogSummary(ctx)
		if err := s.cache.Close(); err != nil {
			s.d.Logger.WarnContext(ctx, "can't close the HTTP cache", "error", err)
		}
	}
}

// resolve resolves a tool name, asking package registries for tfprovider:, tfmodule:, chart:,
// npm: and pypi: names that the registries don't know. Unknown tools are usage errors.
func (s *session) resolve(ctx context.Context, name string) (*registry.Resolution, error) {
	res, err := resolveName(ctx, s.resolver, name, registry.ResolveOptions{Finder: sessionFinder{s}, Guess: s.guess})
	if err != nil {
		return nil, err
	}
	if res.Confidence == registry.ConfidenceLow {
		args := []any{"tool", name, "repo", res.Tool.Repo}
		if len(res.Alternatives) > 0 {
			args = append(args, "alternatives", strings.Join(res.Alternatives, ", "))
		}
		s.d.Logger.WarnContext(ctx, "guessed the repository from a GitHub search; check it, then set "+
			"tools."+res.Tool.Name+".repo in the config", args...)
	}
	return res, nil
}

// sessionFinder builds the HTTP clients only when a lookup is needed, so names the registries
// know resolve without a token or the network.
type sessionFinder struct{ s *session }

func (f sessionFinder) Find(ctx context.Context, kind, name string) (*registry.Found, error) {
	gh, hc, err := f.s.clients(ctx)
	if err != nil {
		return nil, err
	}
	found, err := remote.New(hc, repoSearcher{gh}, gh).Find(ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("look up %s:%s: %w", kind, name, err)
	}
	return found, nil
}

// repoSearcher adapts the GitHub client's search to [remote.RepoSearcher].
type repoSearcher struct{ gh *github.Client }

func (r repoSearcher) SearchRepos(ctx context.Context, name string) ([]remote.RepoHit, error) {
	repos, err := r.gh.SearchRepos(ctx, name)
	if err != nil {
		return nil, err //nolint:wrapcheck // the client names the search
	}
	out := make([]remote.RepoHit, len(repos))
	for i, rp := range repos {
		out[i] = remote.RepoHit{FullName: rp.FullName, Stars: rp.Stars}
	}
	return out, nil
}

// digestArgs says what one digest covers.
type digestArgs struct {
	tool, from, to     string
	line               string // empty means the tool's registry line, else none
	includePrereleases bool
	minConfidence      float64
	kinds              []classify.Kind // empty means every kind
}

// digest digests one tool over a range.
func (s *session) digest(ctx context.Context, a digestArgs) (*report.ToolDigest, error) {
	if err := checkKinds(a.kinds); err != nil {
		return nil, err
	}
	resolution, err := s.resolve(ctx, a.tool)
	if err != nil {
		return nil, err
	}
	line := version.LineNone
	switch {
	case a.line != "":
		if line, err = version.ParseLevel(a.line); err != nil {
			return nil, usageError(err)
		}
	case resolution.Tool.Version.Line != "":
		line = version.Level(resolution.Tool.Version.Line)
	}
	gh, hc, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	digest, err := report.NewDigester(s.d.Logger, gh, source.NewWeb(hc)).Digest(ctx, report.DigestRequest{
		Tool:               resolution.Tool,
		From:               a.from,
		To:                 a.to,
		Line:               line,
		IncludePrereleases: a.includePrereleases,
		MinConfidence:      a.minConfidence,
		Kinds:              a.kinds,
	})
	if err != nil {
		return nil, digestError(err)
	}
	return digest, nil
}

// checkKinds returns a usage error for a kind classify doesn't have.
func checkKinds(kinds []classify.Kind) error {
	for _, k := range kinds {
		if !slices.Contains(classify.Kinds(), k) {
			names := make([]string, len(classify.Kinds()))
			for i, kind := range classify.Kinds() {
				names[i] = string(kind)
			}
			return usageError(fmt.Errorf("invalid kind %q: want one of %s", k, strings.Join(names, ", ")))
		}
	}
	return nil
}

// chartRegistries names each chart pin that has a Helm repository URL after the Artifact Hub
// repository at that URL, chart:<repo>/<name>, so a chart name that several repositories share
// resolves. A pin whose URL Artifact Hub doesn't list, or whose lookup fails, keeps its name.
func (s *session) chartRegistries(ctx context.Context, pins []*scan.Pin) {
	type key struct{ url, chart string }
	repos := map[key]string{}
	for _, p := range pins {
		chart, ok := strings.CutPrefix(p.Tool, registry.KindChart+":")
		if p.Kind != scan.KindChart || p.Registry == "" || !ok || strings.Contains(chart, "/") {
			continue
		}
		k := key{p.Registry, chart}
		repo, seen := repos[k]
		if !seen {
			repo = s.chartRepo(ctx, k.url, k.chart)
			repos[k] = repo
		}
		if repo != "" {
			p.Tool = registry.KindChart + ":" + repo + "/" + chart
		}
	}
}

// chartRepo returns the Artifact Hub repository at registryURL with the chart, or "".
func (s *session) chartRepo(ctx context.Context, registryURL, chart string) string {
	gh, hc, err := s.clients(ctx)
	if err != nil {
		s.d.Logger.DebugContext(ctx, "can't look up a chart repository", "url", registryURL, "error", err)
		return ""
	}
	repo, ok, err := remote.New(hc, repoSearcher{gh}, gh).ChartRepo(ctx, registryURL, chart)
	switch {
	case err != nil:
		s.d.Logger.DebugContext(ctx, "can't look up a chart repository", "url", registryURL, "chart", chart,
			"error", err)
	case !ok:
		s.d.Logger.DebugContext(ctx, "Artifact Hub lists no repository with the chart at this URL",
			"url", registryURL, "chart", chart)
	}
	return repo
}

// scan scans roots for pins, naming repositories relative to base when it is set.
func (s *session) scan(ctx context.Context, roots []string, submodules bool, base string) (*scan.Result, error) {
	opts, err := pinsOptions(s.d.Config.Config.Scan, pinsFlags{submodules: submodules})
	if err != nil {
		return nil, err
	}
	opts.Base = base
	res, err := scan.New(s.d.Logger, s.runner, s.resolver, opts).Scan(ctx, roots)
	if err != nil {
		return nil, usageError(err)
	}
	return res, nil
}
