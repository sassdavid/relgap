package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/version"
)

// unauthenticatedConcurrency caps GitHub requests without a token (60 requests per hour).
const unauthenticatedConcurrency = 2

type digestFlags struct {
	line               string
	includePrereleases bool
}

func newDigestCmd(d *app.Deps, g *globals) *cobra.Command {
	var f digestFlags
	cmd := &cobra.Command{
		Use:   "digest <tool> <from>..[<to>]",
		Short: "Merge the release notes of every version in a range into one digest",
		Long: `Lists the versions after <from> up to and including <to>, fetches their release notes and
groups the entries by kind: breaking changes, deprecations, security, features, fixes, other.

<tool> resolves as in 'relgap resolve'. Without <to> the range ends at the latest stable
version on the release line (--line).

Notes come from the tool's registry sources: GitHub release bodies and changelog files, each
file fetched and parsed once. A bullet that two sources share is shown once. Each entry has a
confidence between 0 and 1; entries below --min-confidence (default 0.5) are hidden and counted.
--format md writes PR-ready Markdown with one <details> block per version.

GitHub token: GH_TOKEN, GITHUB_TOKEN, gh's config file, then 'gh auth token'. Without one,
relgap uses the REST API (60 requests per hour) with 2 concurrent requests.`,
		Example: `  relgap digest terraform 1.15.8..1.17.1
  relgap digest helm 3.19.0.. --line major
  relgap digest kubectl 1.36.0..1.37.2 --format md > digest.md
  relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
  relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'`,
		Args: exactArgs(2), //nolint:mnd // tool and range
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDigest(cmd, d, g, f, args[0], args[1])
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.line, "line", string(version.LineNone),
		"release line the range stays on: none, major or minor")
	fl.BoolVar(&f.includePrereleases, "include-prereleases", false,
		"include prereleases (on when <from> or <to> is a prerelease)")
	return cmd
}

func runDigest(cmd *cobra.Command, d *app.Deps, g *globals, f digestFlags, tool, rangeArg string) error {
	if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON); err != nil {
		return usageError(err)
	}
	from, to, err := parseRange(rangeArg)
	if err != nil {
		return usageError(err)
	}
	line, err := version.ParseLevel(f.line)
	if err != nil {
		return usageError(err)
	}
	resolution, err := g.resolveTool(cmd, d, tool)
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("line") && resolution.Tool.Version.Line != "" {
		line = version.Level(resolution.Tool.Version.Line)
	}
	cache, err := g.newCache(d)
	if err != nil {
		return err
	}
	defer cache.Close()
	defer cache.LogSummary(cmd.Context())
	gh, err := newGitHubClient(cmd, d, cache)
	if err != nil {
		return err
	}
	digest, err := report.NewDigester(d.Logger, gh).Digest(cmd.Context(), report.DigestRequest{
		Tool:               resolution.Tool,
		From:               from,
		To:                 to,
		Line:               line,
		IncludePrereleases: f.includePrereleases,
		MinConfidence:      d.Config.Config.Policy.MinConfidence,
	})
	if err != nil {
		return digestError(err)
	}
	data := &report.Digest{Tools: []report.ToolDigest{*digest}}
	if g.outFormat == render.FormatJSON {
		return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), data))
	}
	if g.outFormat == render.FormatMD {
		return render.DigestMarkdown(d.Stdout, data)
	}
	color := g.outFormat == render.FormatTTY &&
		render.UseColor(render.ColorMode(g.color), d.Env.Lookup, app.IsTerminal(d.Stdout))
	return render.DigestText(d.Stdout, data, color)
}

// parseRange splits "A..B" or "A.." into from and to.
func parseRange(s string) (string, string, error) {
	from, to, ok := strings.Cut(s, "..")
	if !ok || strings.TrimSpace(from) == "" {
		return "", "", fmt.Errorf("invalid range %q: want <from>..<to> or <from>.., such as 1.15.8..1.17.1", s)
	}
	return strings.TrimSpace(from), strings.TrimSpace(to), nil
}

// newGitHubClient finds a token and builds the client. Without a token it warns once and
// lowers the concurrency.
func newGitHubClient(cmd *cobra.Command, d *app.Deps, cache *httpx.Cache) (*github.Client, error) {
	ctx := cmd.Context()
	token, tokenSource := github.Token(ctx, github.TokenOptions{
		Lookup:      d.Env.Lookup,
		ConfigToken: github.GHConfigToken,
		Runner:      newRunner(d),
	})
	concurrency := d.Config.Config.GitHub.Concurrency
	if token == "" {
		concurrency = min(concurrency, unauthenticatedConcurrency)
		d.Logger.WarnContext(ctx, "no GitHub token found; using the REST API (60 requests per hour) "+
			"with 2 concurrent requests. Set GH_TOKEN or run `gh auth login`")
	}
	d.Logger.DebugContext(ctx, "github client",
		"authenticated", token != "", "token_source", tokenSource, "concurrency", concurrency)
	limits, def := httpx.DefaultLimits()
	limits[httpx.GitHubAPIHost] = httpx.Limit{Concurrency: concurrency, Rate: limits[httpx.GitHubAPIHost].Rate}
	hc := httpx.New(httpx.Options{
		Logger:    d.Logger,
		Transport: d.HTTPTransport,
		UserAgent: fmt.Sprintf("relgap/%s (+https://github.com/sassdavid/relgap)", buildinfo.Get().Version),
		Limits:    limits,
		Default:   def,
		Cache:     cache,
	})
	gh, err := github.New(hc, github.Options{Token: token, Logger: d.Logger, Concurrency: concurrency})
	if err != nil {
		return nil, fmt.Errorf("create github client: %w", err)
	}
	return gh, nil
}

func digestError(err error) error {
	switch {
	case errors.Is(err, github.ErrAuth):
		return &exitError{code: ExitAuth, err: err}
	case errors.Is(err, version.ErrDowngrade), errors.Is(err, version.ErrLineMismatch),
		errors.Is(err, version.ErrNotFound), errors.Is(err, version.ErrParse):
		return usageError(err)
	case errors.Is(err, httpx.ErrRateLimited):
		return fmt.Errorf("%w. Set GH_TOKEN or run `gh auth login` for 5000 requests per hour", err)
	default:
		return err
	}
}
