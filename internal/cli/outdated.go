package cli

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/source"
)

const flagBinaries = "binaries"

func newOutdatedCmd(d *app.Deps, g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outdated [path]",
		Short: "Compare every pinned tool version in a project with the latest release",
		Long: `Scans the project at path (default: the current directory) the way 'relgap pins' does and
compares each pin with the newest stable version on the tool's release line.

CURRENT is the version a pin stands for. An exact pin is itself. A value marked ≈ allows
more than one version, and the notes say which one relgap used: a constraint (~> 1.15) takes
the highest exact pin of the tool in the same repository that satisfies it, otherwise the
lowest version that does; a prefix (26, prefix:1.2) takes the highest stable version that
starts with it; the latest channel is the latest version. Other channels (lts) aren't
compared.

BEHIND, BREAKING and DEPRECATION come from the digest of CURRENT..LATEST, the same digest
'relgap digest <tool> <current>..' prints. Entries below --min-confidence aren't counted.

relgap finds the latest versions with one GraphQL query per 25 tools. Only pins that are
behind, and constraint and prefix pins, need a tool's full release list.

Pins of tools without a GitHub repository, and tfprovider, chart and image pins, are
skipped; --format json lists them with the reason.

EOL shows whether CURRENT's release cycle has reached its end on endoflife.date (yes, no, or
? when endoflife.date doesn't list the cycle), for tools whose registry entry names a
product. --format json also has the state of LATEST's cycle. VULNS counts the OSV
vulnerabilities of CURRENT, and how many of them upgrading to LATEST fixes. Both sources
are cached for a day. When one is down, relgap warns and prints the rest.

--binaries also reads the Go build info of each pinned tool's installed executables (on
PATH and in mise's installs directory) and lists the OSV vulnerabilities of the Go version
and main module they were built with. Like govulncheck's binary mode, minus the check that
the binary calls the vulnerable code.

--format sarif writes a SARIF 2.1.0 log for GitHub code scanning, located at each pin's
file and line: a result per pin behind latest (outdated), per pin on an EOL cycle (eol), per
upgrade with breaking changes or deprecations (breaking, deprecation), per advisory that
affects a pin (one rule per OSV ID), and per pin or file that failed (failed). The results
don't depend on --fail-on.

--fail-on takes breaking, deprecation, eol, vuln and outdated. Each value counts findings:
outdated counts the pins further behind than --max-behind (major:N, minor:N, patch:N,
releases:N or days:N; empty means any pin behind latest), breaking and deprecation count
the pins whose digest has such an entry, eol counts the pins on an EOL cycle, and vuln
counts the advisories that affect a pin at or above --vuln-severity. A value whose count
reaches --min-count (default 1) exits 3. Otherwise a tool or file that failed exits 1, and
so does an endoflife.date outage under --fail-on eol or an OSV outage under --fail-on vuln,
since a pass on partial data proves nothing.`,
		Example: `  relgap outdated
  relgap outdated ~/src/infra --format md
  relgap outdated --fail-on outdated --max-behind minor:2
  relgap outdated --fail-on eol --fail-on vuln --vuln-severity high
  relgap outdated --fail-on outdated --min-count 5 --format sarif > relgap.sarif
  relgap outdated --binaries --format json | jq '.data.binaries[]'

  relgap outdated --format json | jq '.data.pins[] | select(.status == "outdated")'`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return runOutdated(cmd, d, g, root)
		},
	}
	addGateFlags(cmd, outdatedValues())
	cmd.Flags().Bool(flagBinaries, false,
		"also check the Go version and main module of the pinned tools' installed executables against OSV")
	return cmd
}

func runOutdated(cmd *cobra.Command, d *app.Deps, g *globals, root string) error {
	err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON,
		render.FormatSARIF)
	if err != nil {
		return usageError(err)
	}
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	gate, err := newGate(cmd, d, outdatedValues())
	if err != nil {
		return err
	}
	binaries, err := cmd.Flags().GetBool(flagBinaries)
	if err != nil {
		return fmt.Errorf("read --%s: %w", flagBinaries, err)
	}
	run, err := s.compare(cmd.Context(), root, nil, compareOptions{enrich: true, binaries: binaries})
	if err != nil {
		return err
	}
	if err = writeOutdated(d, g, run); err != nil {
		return err
	}
	run.warn(cmd, d)
	return run.exit(cmd, d, gate)
}

// comparison is the outcome of scanning a project and comparing its pins.
type comparison struct {
	scan     *scan.Result
	out      *report.Outdated
	problems []enrich.Problem
}

// compareOptions says what a comparison adds to the pins.
type compareOptions struct {
	enrich   bool   // EOL and OSV for each compared pin
	binaries bool   // the Go build info of installed executables
	base     string // name repositories relative to this directory
}

// compare scans root, keeps the resolved pins that keep accepts (all when nil) and compares
// them with the latest versions.
func (s *session) compare(
	ctx context.Context,
	root string,
	keep func(scan.Pin) bool,
	with compareOptions,
) (*comparison, error) {
	cfg, resolver := s.d.Config, s.resolver
	res, err := s.scan(ctx, []string{root}, false, with.base)
	if err != nil {
		return nil, err
	}
	ignore := ignoreRules(resolver, cfg.Config.Ignore)
	var pins []scan.Pin
	for _, p := range res.Pins {
		if p.Resolved() && !scan.Ignored(ignore, &p) && (keep == nil || keep(p)) {
			pins = append(pins, p)
		}
	}
	gh, hc, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	out, err := report.NewOutdater(report.NewDigester(s.d.Logger, gh, source.NewWeb(hc)), gh).
		Outdated(ctx, report.OutdatedRequest{
			Repos:         res.Repos,
			Pins:          pins,
			Resolve:       resolveFunc(resolver),
			MinConfidence: cfg.Config.Policy.MinConfidence,
			Concurrency:   cfg.Config.GitHub.Concurrency,
		})
	if err != nil {
		return nil, digestError(err)
	}
	run := &comparison{scan: res, out: out}
	if with.enrich {
		if err = s.enrich(ctx, run, hc, pins, with.binaries); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// enrich adds EOL states and vulnerabilities to the compared pins and, with binaries, the
// vulnerabilities of installed executables. Failed sources become problems, not errors.
func (s *session) enrich(ctx context.Context, c *comparison, hc *http.Client, pins []scan.Pin, binaries bool) error {
	d, resolver := s.d, s.resolver
	en := enrich.NewEnricher(d.Logger, enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, ""),
		d.Config.Config.GitHub.Concurrency)
	results, problems, err := en.Enrich(ctx, c.out.EnrichRequests(resolveFunc(resolver)))
	if err != nil {
		return err //nolint:wrapcheck // only a canceled context
	}
	c.out.ApplyEnrichment(results)
	c.problems = problems
	if !binaries {
		return nil
	}
	finder := enrich.BinaryFinder{
		Lookup: d.Env.Lookup,
		Which:  misex.New(s.runner).Which,
	}
	seen := map[string]bool{}
	c.out.Binaries = []*enrich.Binary{}
	for _, p := range pins {
		if seen[p.Tool] {
			continue
		}
		seen[p.Tool] = true
		spec, resolveErr := resolveFunc(resolver)(p.Tool)
		if resolveErr != nil {
			continue
		}
		for _, path := range finder.Find(ctx, spec) {
			b, readErr := enrich.ReadBinary(spec.Name, path)
			if readErr != nil {
				d.Logger.DebugContext(ctx, "skipping an executable", "tool", spec.Name, "error", readErr)
				continue
			}
			c.out.Binaries = append(c.out.Binaries, b)
		}
	}
	if err = en.BinaryVulns(ctx, c.out.Binaries); err != nil {
		if ctx.Err() != nil {
			return ctx.Err() //nolint:wrapcheck // the caller matches context errors
		}
		c.problems = append(c.problems, enrich.Problem{Source: enrich.SourceOSV, Tool: "binaries", Err: err})
	}
	return nil
}

// resolveFunc turns a pin's tool name into its spec through resolver.
func resolveFunc(resolver *registry.Resolver) report.ResolveFunc {
	return func(tool string) (registry.ToolSpec, error) {
		r, err := resolver.Resolve(tool)
		if err != nil {
			return registry.ToolSpec{}, err //nolint:wrapcheck // the resolver names the tool
		}
		return r.Tool, nil
	}
}

// ignoreRules turns the config's ignore rules into scan rules, naming each tool the way pins do,
// so an alias such as tf matches terraform pins.
func ignoreRules(resolver *registry.Resolver, rules []config.IgnoreRule) []scan.Ignore {
	out := make([]scan.Ignore, 0, len(rules))
	for _, r := range rules {
		tool := r.Tool
		if name, ok := resolver.Identify(tool); ok {
			tool = name
		}
		out = append(out, scan.Ignore{Tool: tool, Path: r.Path})
	}
	return out
}

func writeOutdated(d *app.Deps, g *globals, c *comparison) error {
	if g.outFormat == render.FormatJSON {
		env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), c.out)
		env.Errors = append(c.errors(), c.sourceErrors()...)
		return render.JSON(d.Stdout, env)
	}
	if g.outFormat == render.FormatMD {
		return render.OutdatedMarkdown(d.Stdout, c.out)
	}
	if g.outFormat == render.FormatSARIF {
		return sarif.Write(d.Stdout, render.OutdatedSARIF(c.out, c.scan.Problems, buildinfo.Get().Version))
	}
	color := g.outFormat == render.FormatTTY &&
		render.UseColor(render.ColorMode(g.color), d.Env.Lookup, app.IsTerminal(d.Stdout))
	return render.OutdatedText(d.Stdout, c.out, color)
}

// errors lists the files that didn't parse and the pins GitHub failed for.
func (c *comparison) errors() []render.Error {
	out := []render.Error{}
	for _, p := range c.scan.Problems {
		out = append(out, render.Error{Target: path.Join(p.Repo, p.File), Message: p.Message})
	}
	for _, p := range c.out.Pins {
		if p.Status == report.StatusError {
			out = append(out, render.Error{Target: p.Tool + "@" + render.PinLocation(p.Pin.Location), Message: p.Note})
		}
	}
	return out
}

// sourceErrors lists the enrichment sources that failed, as JSON envelope errors.
func (c *comparison) sourceErrors() []render.Error {
	out := make([]render.Error, 0, len(c.problems))
	for _, p := range c.problems {
		out = append(out, render.Error{Target: p.Source + ": " + p.Tool, Message: p.Err.Error()})
	}
	return out
}

func (c *comparison) warn(cmd *cobra.Command, d *app.Deps) {
	for _, p := range c.scan.Problems {
		d.Logger.WarnContext(cmd.Context(), "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	for _, p := range c.problems {
		d.Logger.WarnContext(cmd.Context(), "enrichment source failed; the report continues without it",
			"source", p.Source, "tool", p.Tool, "error", p.Err)
	}
	if n := countUnresolved(c.scan.Pins); n > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s no known tool; list them with 'relgap pins --unresolved'\n",
			config.AppName, n, plural(n, "pin has", "pins have"))
	}
}

// exit turns gate findings and errors into the exit code; see [gateExit].
func (c *comparison) exit(cmd *cobra.Command, d *app.Deps, gate report.Gate) error {
	findings, unmeasured := gate.Check(c.out)
	for _, p := range unmeasured {
		d.Logger.WarnContext(cmd.Context(), "can't measure days behind: the tool's versions have no publish dates",
			"tool", p.Tool, "pin", render.PinLocation(p.Pin.Location))
	}
	return gateExit(cmd, d, gate.Matches(findings), c.partial(gate))
}

// partial returns why the gate can't pass: a pin or file failed, or a source that a --fail-on
// value needs was down.
func (c *comparison) partial(gate report.Gate) error {
	if errs := c.errors(); len(errs) > 0 {
		return fmt.Errorf(
			"%d %s failed; the report is partial",
			len(errs),
			plural(len(errs), "pin or file", "pins or files"),
		)
	}
	for _, need := range []struct{ value, source string }{
		{report.FailOnEOL, enrich.SourceEOL},
		{report.FailOnVuln, enrich.SourceOSV},
	} {
		if !gate.Has(need.value) {
			continue
		}
		var down []string
		for _, p := range c.problems {
			if p.Source == need.source && !slices.Contains(down, p.Tool) {
				down = append(down, p.Tool)
			}
		}
		if len(down) > 0 {
			return fmt.Errorf("--%s %s can't decide: %s failed for %s; the report is partial",
				flagFailOn, need.value, need.source, strings.Join(down, ", "))
		}
	}
	return nil
}
