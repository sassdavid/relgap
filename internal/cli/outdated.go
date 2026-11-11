package cli

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
)

const (
	flagFailOn    = "fail-on"
	flagMaxBehind = "max-behind"
)

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

--fail-on outdated exits 3 when a pin is further behind than --max-behind (major:N,
minor:N, patch:N, releases:N or days:N; empty means any pin behind latest). --fail-on
breaking exits 3 when a digest has a breaking entry. A tool that fails exits 1 instead,
after printing the rest. The eol, vuln and deprecation values exit 2 until relgap can
check them.`,
		Example: `  relgap outdated
  relgap outdated ~/src/infra --format md
  relgap outdated --fail-on outdated --max-behind minor:2
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
	addGateFlags(cmd)
	return cmd
}

func addGateFlags(cmd *cobra.Command) {
	def := config.Default().Policy
	fl := cmd.Flags()
	fl.StringSlice(flagFailOn, def.FailOn,
		"exit 3 when this matches: outdated or breaking; repeatable (config policy.fail_on)")
	fl.String(flagMaxBehind, def.MaxBehind,
		"how far behind --fail-on outdated allows, such as minor:2, releases:10 or days:90 (config policy.max_behind)")
}

func runOutdated(cmd *cobra.Command, d *app.Deps, g *globals, root string) error {
	err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON)
	if err != nil {
		return usageError(err)
	}
	resolver, err := g.loadProject(cmd, d)
	if err != nil {
		return err
	}
	gate, err := newGate(d, true)
	if err != nil {
		return err
	}
	run, err := g.compare(cmd, d, resolver, root, nil)
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
	scan *scan.Result
	out  *report.Outdated
}

// loadProject loads the config and the tool resolver; d.Config is set afterwards.
func (g *globals) loadProject(cmd *cobra.Command, d *app.Deps) (*registry.Resolver, error) {
	resolver, _, err := g.loadResolver(cmd, d)
	return resolver, err
}

// compare scans root, keeps the resolved pins that keep accepts (all when nil) and compares
// them with the latest versions.
func (g *globals) compare(
	cmd *cobra.Command, d *app.Deps, resolver *registry.Resolver, root string, keep func(scan.Pin) bool,
) (*comparison, error) {
	cfg := d.Config
	opts, err := pinsOptions(cfg.Config.Scan, pinsFlags{})
	if err != nil {
		return nil, err
	}
	res, err := scan.New(d.Logger, newRunner(d), resolver, opts).Scan(cmd.Context(), []string{root})
	if err != nil {
		return nil, usageError(err)
	}
	var pins []scan.Pin
	for _, p := range res.Pins {
		if p.Resolved() && (keep == nil || keep(p)) {
			pins = append(pins, p)
		}
	}
	cache, err := g.newCache(d)
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	defer cache.LogSummary(cmd.Context())
	gh, err := newGitHubClient(cmd, d, cache)
	if err != nil {
		return nil, err
	}
	resolve := func(tool string) (registry.ToolSpec, error) {
		r, rerr := resolver.Resolve(tool)
		if rerr != nil {
			return registry.ToolSpec{}, rerr //nolint:wrapcheck // the resolver names the tool
		}
		return r.Tool, nil
	}
	out, err := report.NewOutdater(report.NewDigester(d.Logger, gh), gh).Outdated(cmd.Context(), report.OutdatedRequest{
		Repos:         res.Repos,
		Pins:          pins,
		Resolve:       resolve,
		MinConfidence: cfg.Config.Policy.MinConfidence,
		Concurrency:   cfg.Config.GitHub.Concurrency,
	})
	if err != nil {
		return nil, digestError(err)
	}
	return &comparison{scan: res, out: out}, nil
}

// newGate builds the --fail-on checks from the flags and config. Values that belong to another
// command are left to it; values relgap can't check yet are a usage error, since a CI gate that
// silently passes is worse than one that fails to start.
func newGate(d *app.Deps, outdated bool) (report.Gate, error) {
	p := d.Config.Config.Policy
	gate := report.Gate{Now: d.Clock()}
	var err error
	if gate.MaxBehind, err = report.ParseMaxBehind(p.MaxBehind); err != nil {
		return gate, usageError(err)
	}
	for _, v := range p.FailOn {
		switch v {
		case report.FailOnBreaking:
			gate.Breaking = true
		case report.FailOnOutdated:
			gate.Outdated = outdated
		case report.FailOnDrift:
		case report.FailOnEOL, report.FailOnVuln, report.FailOnDeprecation:
			return gate, usageError(
				fmt.Errorf("--fail-on %s isn't supported yet: relgap checks outdated and breaking", v),
			)
		default:
			return gate, usageError(fmt.Errorf("invalid --fail-on value %q", v))
		}
	}
	return gate, nil
}

func writeOutdated(d *app.Deps, g *globals, c *comparison) error {
	if g.outFormat == render.FormatJSON {
		env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), c.out)
		env.Errors = c.errors()
		return render.JSON(d.Stdout, env)
	}
	if g.outFormat == render.FormatMD {
		return render.OutdatedMarkdown(d.Stdout, c.out)
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

func (c *comparison) warn(cmd *cobra.Command, d *app.Deps) {
	for _, p := range c.scan.Problems {
		d.Logger.WarnContext(cmd.Context(), "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	if n := countUnresolved(c.scan.Pins); n > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s no known tool; list them with 'relgap pins --unresolved'\n",
			config.AppName, n, plural(n, "pin has", "pins have"))
	}
}

// exit turns errors and gate findings into the exit code: a failed tool or file wins over a
// --fail-on match, so a gate never passes or fails on partial data.
func (c *comparison) exit(cmd *cobra.Command, d *app.Deps, gate report.Gate) error {
	if errs := c.errors(); len(errs) > 0 {
		return fmt.Errorf(
			"%d %s failed; the report is partial",
			len(errs),
			plural(len(errs), "pin or file", "pins or files"),
		)
	}
	findings, unmeasured := gate.Check(c.out)
	for _, p := range unmeasured {
		d.Logger.WarnContext(cmd.Context(), "can't measure days behind: the tool's versions have no publish dates",
			"tool", p.Tool, "pin", render.PinLocation(p.Pin.Location))
	}
	if len(findings) == 0 {
		return nil
	}
	return &exitError{code: ExitFindings, err: findingsError(findings)}
}

func findingsError(fs []report.Finding) error {
	byValue := map[string][]string{}
	var values []string
	for _, f := range fs {
		if _, ok := byValue[f.FailOn]; !ok {
			values = append(values, f.FailOn)
		}
		desc := fmt.Sprintf("%s %s (%s)", f.Pin.Tool, f.Pin.Current, render.PinLocation(f.Pin.Pin.Location))
		byValue[f.FailOn] = append(byValue[f.FailOn], desc)
	}
	slices.Sort(values)
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("--fail-on %s matched %s", v, strings.Join(byValue[v], ", "))
	}
	return errors.New(strings.Join(parts, "; "))
}
