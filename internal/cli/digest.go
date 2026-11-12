package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

// unauthenticatedConcurrency caps GitHub requests without a token (60 requests per hour).
const unauthenticatedConcurrency = 2

type digestFlags struct {
	line               string
	includePrereleases bool
	fromMise           bool
	all                bool
	only               []string
}

const (
	flagLine     = "line"
	flagFromMise = "from-mise"
	flagAll      = "all"
	flagOnly     = "only"
)

func newDigestCmd(d *app.Deps, g *globals) *cobra.Command {
	var f digestFlags
	cmd := &cobra.Command{
		Use:   "digest <tool> <from>..[<to>] | --from-mise [path] | --all [path]",
		Short: "Merge the release notes of every version in a range into one digest",
		Long: `Lists the versions after <from> up to and including <to>, fetches their release notes and
groups the entries by kind: breaking changes, deprecations, security, features, fixes, other.

<tool> resolves as in 'relgap resolve'. Without <to> the range ends at the latest stable
version on the release line (--line).

Notes come from the tool's registry sources: GitHub release bodies and changelog files, each
file fetched and parsed once. A bullet that two sources share is shown once. Each entry has a
confidence between 0 and 1; entries below --min-confidence (default 0.5) are hidden and counted.
--format md writes PR-ready Markdown with one <details> block per version. --only lists
the entries of the named kinds; the counts still cover every kind.

--from-mise digests every tool pinned in the project's mise config files that is behind
its latest version, from the pinned version to the latest. --all does the same for every pin
'relgap outdated' compares. Both use the ranges and digests 'relgap outdated' shows, and
take the project path as their only argument (default: the current directory).

--fail-on breaking exits 3 when a digest has a breaking entry at or above --min-confidence,
and --fail-on deprecation when it has a deprecation. A single digest counts once; --from-mise
and --all count each pin, as 'relgap outdated' does, and a value exits 3 once its count
reaches --min-count. With --from-mise or --all, a pin that fails exits 1 when no value
matched. The other --fail-on values belong to 'relgap outdated' and 'relgap drift'.

GitHub token: GH_TOKEN, GITHUB_TOKEN, gh's config file, then 'gh auth token'. Without one,
relgap uses the REST API (60 requests per hour) with 2 concurrent requests.`,
		Example: `  relgap digest terraform 1.15.8..1.17.1
  relgap digest helm 3.19.0.. --line major
  relgap digest kubectl 1.36.0..1.37.2 --format md > digest.md
  relgap digest terraform 1.15.8..1.17.1 --min-confidence 0
  relgap digest argo-cd 3.2.0..3.4.1 --only breaking --only deprecation
  relgap digest --from-mise --fail-on breaking
  relgap digest --all ~/src/infra --format md > upgrade-notes.md
  relgap digest terraform 1.15.8..1.17.1 --format json | jq '.data.tools[].entries[] | select(.kind == "breaking")'`,
		Args: func(cmd *cobra.Command, args []string) error {
			if f.fromMise || f.all {
				return maxArgs(1)(cmd, args)
			}
			return exactArgs(2)(cmd, args) //nolint:mnd // tool and range
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.fromMise || f.all {
				root := "."
				if len(args) == 1 {
					root = args[0]
				}
				return runDigestProject(cmd, d, g, f, root)
			}
			return runDigest(cmd, d, g, f, args[0], args[1])
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.line, flagLine, string(version.LineNone),
		"release line the range stays on: none, major or minor")
	fl.BoolVar(&f.includePrereleases, "include-prereleases", false,
		"include prereleases (on when <from> or <to> is a prerelease)")
	fl.BoolVar(&f.fromMise, flagFromMise, false,
		"digest every outdated tool pinned in the project's mise config, pin to latest")
	fl.BoolVar(&f.all, flagAll, false, "digest every outdated pin in the project, as 'relgap outdated' lists them")
	fl.StringSliceVar(&f.only, flagOnly, nil,
		"list only entries of this kind: breaking, deprecation, security, feature, fix or other; repeatable")
	addGateFlags(cmd, digestValues())
	cmd.MarkFlagsMutuallyExclusive(flagFromMise, flagAll)
	cmd.MarkFlagsMutuallyExclusive(flagFromMise, flagLine)
	cmd.MarkFlagsMutuallyExclusive(flagAll, flagLine)
	cmd.MarkFlagsMutuallyExclusive(flagFromMise, "include-prereleases")
	cmd.MarkFlagsMutuallyExclusive(flagAll, "include-prereleases")
	cmd.MarkFlagsMutuallyExclusive(flagFromMise, flagOnly)
	cmd.MarkFlagsMutuallyExclusive(flagAll, flagOnly)
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
	if _, err = version.ParseLevel(f.line); err != nil {
		return usageError(err)
	}
	kinds := make([]classify.Kind, len(f.only))
	for i, k := range f.only {
		kinds[i] = classify.Kind(k)
	}
	if err = checkKinds(kinds); err != nil {
		return err
	}
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	gate, err := newGate(cmd, d, digestValues())
	if err != nil {
		return err
	}
	a := digestArgs{
		tool:               tool,
		from:               from,
		to:                 to,
		includePrereleases: f.includePrereleases,
		minConfidence:      d.Config.Config.Policy.MinConfidence,
		kinds:              kinds,
	}
	if cmd.Flags().Changed(flagLine) {
		a.line = f.line
	}
	digest, err := s.digest(cmd.Context(), a)
	if err != nil {
		return err
	}
	data := &report.Digest{Tools: []report.ToolDigest{*digest}}
	if err = writeDigest(d, g, data, []render.Error{}); err != nil {
		return err
	}
	return gateExit(cmd, d, gate.Matches(gate.CheckDigests(data.Tools)), nil)
}

func writeDigest(d *app.Deps, g *globals, data *report.Digest, errs []render.Error) error {
	if g.outFormat == render.FormatJSON {
		env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), data)
		env.Errors = errs
		return render.JSON(d.Stdout, env)
	}
	if g.outFormat == render.FormatMD {
		return render.DigestMarkdown(d.Stdout, data)
	}
	color := g.outFormat == render.FormatTTY &&
		render.UseColor(render.ColorMode(g.color), d.Env.Lookup, app.IsTerminal(d.Stdout))
	return render.DigestText(d.Stdout, data, color)
}

// runDigestProject digests the outdated pins of the project at root, using the comparison
// 'relgap outdated' makes, so both commands report the same ranges and counts.
func runDigestProject(cmd *cobra.Command, d *app.Deps, g *globals, f digestFlags, root string) error {
	if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON); err != nil {
		return usageError(err)
	}
	var keep func(scan.Pin) bool
	if f.fromMise {
		keep = func(p scan.Pin) bool { return p.Location.Detector == scan.DetectorMise }
	}
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	gate, err := newGate(cmd, d, digestValues())
	if err != nil {
		return err
	}
	run, err := s.compare(cmd.Context(), root, keep, compareOptions{})
	if err != nil {
		return err
	}
	if err = writeDigest(d, g, &report.Digest{Tools: run.out.Digests()}, run.errors()); err != nil {
		return err
	}
	run.warn(cmd, d)
	run.notDigested(d)
	return run.exit(cmd, d, gate)
}

// notDigested tells on stderr which compared pins have no digest and why.
func (c *comparison) notDigested(d *app.Deps) {
	var latest, other []string
	skipped := 0
	for _, p := range c.out.Pins {
		switch p.Status {
		case report.StatusCurrent:
			latest = append(latest, p.Tool+" "+p.Current)
		case report.StatusUnknown:
			other = append(
				other,
				fmt.Sprintf("%s %s (%s): %s", p.Tool, p.Pin.Value, render.PinLocation(p.Pin.Location), p.Note),
			)
		case report.StatusSkipped:
			skipped++
		case report.StatusOutdated, report.StatusError:
		}
	}
	if len(latest) > 0 {
		_, _ = fmt.Fprintf(
			d.Stderr,
			"%s: at the latest version, no digest: %s\n",
			config.AppName,
			strings.Join(latest, ", "),
		)
	}
	for _, o := range other {
		_, _ = fmt.Fprintf(d.Stderr, "%s: not digested: %s\n", config.AppName, o)
	}
	if skipped > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s skipped; 'relgap outdated --format json' says why\n",
			config.AppName, skipped, plural(skipped, "pin", "pins"))
	}
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
// lowers the concurrency. The plain HTTP client it returns has the same retries, limits and
// cache, never sends the token, and serves web pages, endoflife.date and OSV. tokenRunner runs
// `gh auth token`; nil skips it.
func newGitHubClient(
	ctx context.Context,
	d *app.Deps,
	cache *httpx.Cache,
	tokenRunner github.Runner,
) (*github.Client, *http.Client, error) {
	token, tokenSource := github.Token(ctx, github.TokenOptions{
		Lookup:      d.Env.Lookup,
		ConfigToken: github.GHConfigToken,
		Runner:      tokenRunner,
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
		return nil, nil, fmt.Errorf("create github client: %w", err)
	}
	return gh, hc, nil
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
