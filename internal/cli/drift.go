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

type driftFlags struct {
	tools             []string
	latest            bool
	ignoreConstraints bool
	submodules        bool
}

func newDriftCmd(d *app.Deps, g *globals) *cobra.Command {
	var f driftFlags
	cmd := &cobra.Command{
		Use:   "drift [roots...]",
		Short: "Show which repositories pin different versions of the same tool",
		Long: `Scans each root for repositories the way 'relgap pins' does and prints a matrix with a
row per repository and a column per tool that drifts. A tool drifts when its pins stand for
more than one version across the scanned repositories. It reads files only and makes no
network calls unless --latest is set.

An exact pin counts as its version. A value marked ≈ allows more than one version. A
constraint (~> 1.15) counts as the highest exact pin of the tool in the same repository
that satisfies it, otherwise as its lower bound (1.15.0). Prefix (26) and channel (latest)
pins aren't compared. --drift-ignore-constraints leaves constraint pins out of the
comparison too. A version marked * differs from the version most repositories pin.

Tools that agree everywhere, or that only one repository pins, are listed below the matrix.
--tool limits the report to the named tools and gives each one a column, even when it
doesn't drift. Ignore rules in the config drop matching pins.

--latest adds a latest row with the newest stable version of each tool, from one GraphQL
query per 25 tools.

--fail-on drift exits 3 when a tool drifts. A file that doesn't parse, or a tool whose latest
version can't be listed, exits 1 instead, after printing the rest.

Roots default to scan.roots, then the current directory.`,
		Example: `  relgap drift ~/src/infra
  relgap drift ~/src/infra --tool terraform --tool terragrunt
  relgap drift ~/src/infra ~/src/apps --latest --format md
  relgap drift --fail-on drift --drift-ignore-constraints`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDrift(cmd, d, g, f, args)
		},
	}
	fl := cmd.Flags()
	fl.StringSliceVar(&f.tools, "tool", nil, "report only this tool, with a column even when it agrees; repeatable")
	fl.BoolVar(
		&f.latest,
		"latest",
		false,
		"add the newest stable version of each tool (needs the network or the cache)",
	)
	fl.BoolVar(&f.ignoreConstraints, "drift-ignore-constraints", false,
		"compare exact pins only; constraint pins are shown but don't count")
	fl.BoolVar(&f.submodules, "submodules", false, "scan checked-out git submodules as repositories")
	fl.StringSlice(flagFailOn, config.Default().Policy.FailOn,
		"exit 3 when this matches: drift; repeatable (config policy.fail_on)")
	return cmd
}

func runDrift(cmd *cobra.Command, d *app.Deps, g *globals, f driftFlags, args []string) error {
	err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON)
	if err != nil {
		return usageError(err)
	}
	resolver, cfg, err := g.loadResolver(cmd, d)
	if err != nil {
		return err
	}
	gate, err := newDriftGate(cmd, d)
	if err != nil {
		return err
	}
	opts, err := pinsOptions(cfg.Config.Scan, pinsFlags{submodules: f.submodules})
	if err != nil {
		return err
	}
	roots := args
	if len(roots) == 0 {
		roots = cfg.Config.Scan.Roots
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	res, err := scan.New(d.Logger, newRunner(d), resolver, opts).Scan(cmd.Context(), roots)
	if err != nil {
		return usageError(err)
	}
	tools := toolNames(resolver, f.tools)
	ignore := ignoreRules(resolver, cfg.Config.Ignore)
	var pins []scan.Pin
	for _, p := range res.Pins {
		if p.Resolved() && !scan.Ignored(ignore, &p) && (len(tools) == 0 || slices.Contains(tools, p.Tool)) {
			pins = append(pins, p)
		}
	}
	var recent report.RecentLister
	if f.latest {
		cache, cacheErr := g.newCache(d)
		if cacheErr != nil {
			return cacheErr
		}
		defer cache.Close()
		defer cache.LogSummary(cmd.Context())
		gh, _, ghErr := newGitHubClient(cmd, d, cache)
		if ghErr != nil {
			return ghErr
		}
		recent = gh
	}
	out, err := report.NewDrifter(d.Logger, recent).Drift(cmd.Context(), report.DriftRequest{
		Repos:             res.Repos,
		Pins:              pins,
		Resolve:           resolveFunc(resolver),
		IgnoreConstraints: f.ignoreConstraints,
		Latest:            f.latest,
	})
	if err != nil {
		return digestError(err)
	}
	if err = writeDrift(d, g, res, out, len(tools) > 0); err != nil {
		return err
	}
	warnDrift(cmd, d, res, out, tools)
	return driftExit(res, out, gate)
}

// toolNames names each --tool value the way pins do, so --tool tf selects terraform pins.
func toolNames(resolver *registry.Resolver, in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		if name, ok := resolver.Identify(t); ok {
			t = name
		}
		out = append(out, t)
	}
	return out
}

// newDriftGate builds the --fail-on checks of drift. Config values that belong to outdated or
// digest are left to them; the same values given as a flag are a usage error.
func newDriftGate(cmd *cobra.Command, d *app.Deps) (report.Gate, error) {
	gate := report.Gate{Now: d.Clock()}
	explicit := cmd.Flags().Changed(flagFailOn)
	for _, v := range d.Config.Config.Policy.FailOn {
		switch v {
		case report.FailOnDrift:
			gate.Drift = true
		case report.FailOnBreaking, report.FailOnDeprecation, report.FailOnEOL, report.FailOnVuln,
			report.FailOnOutdated:
			if explicit {
				return gate, usageError(fmt.Errorf("--fail-on %s isn't checked by drift: relgap drift checks drift", v))
			}
		default:
			return gate, usageError(fmt.Errorf("invalid --fail-on value %q", v))
		}
	}
	return gate, nil
}

func writeDrift(d *app.Deps, g *globals, res *scan.Result, out *report.Drift, all bool) error {
	if g.outFormat == render.FormatJSON {
		env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), out)
		env.Errors = driftErrors(res, out)
		return render.JSON(d.Stdout, env)
	}
	if g.outFormat == render.FormatMD {
		return render.DriftMarkdown(d.Stdout, out, render.DriftOptions{All: all})
	}
	color := g.outFormat == render.FormatTTY &&
		render.UseColor(render.ColorMode(g.color), d.Env.Lookup, app.IsTerminal(d.Stdout))
	return render.DriftText(d.Stdout, out, render.DriftOptions{All: all, Color: color})
}

// driftErrors lists the files that didn't parse and the tools whose latest version GitHub
// failed to list.
func driftErrors(res *scan.Result, out *report.Drift) []render.Error {
	errs := []render.Error{}
	for _, p := range res.Problems {
		errs = append(errs, render.Error{Target: path.Join(p.Repo, p.File), Message: p.Message})
	}
	for _, t := range out.Tools {
		if t.Err != nil {
			errs = append(errs, render.Error{Target: t.Tool, Message: t.Err.Error()})
		}
	}
	return errs
}

func warnDrift(cmd *cobra.Command, d *app.Deps, res *scan.Result, out *report.Drift, tools []string) {
	for _, p := range res.Problems {
		d.Logger.WarnContext(cmd.Context(), "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	for _, t := range tools {
		if !slices.ContainsFunc(out.Tools, func(dt report.DriftTool) bool { return dt.Tool == t }) {
			d.Logger.WarnContext(cmd.Context(), "no repository pins this tool", "tool", t)
		}
	}
	if n := countUnresolved(res.Pins); n > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s no known tool; list them with 'relgap pins --unresolved'\n",
			config.AppName, n, plural(n, "pin has", "pins have"))
	}
}

// driftExit turns errors and gate findings into the exit code. A failed file or lookup wins over
// a --fail-on match, so a gate never passes or fails on partial data.
func driftExit(res *scan.Result, out *report.Drift, gate report.Gate) error {
	if errs := driftErrors(res, out); len(errs) > 0 {
		return fmt.Errorf("%d %s failed; the report is partial",
			len(errs), plural(len(errs), "file or tool", "files or tools"))
	}
	drifting := gate.CheckDrift(out)
	if len(drifting) == 0 {
		return nil
	}
	parts := make([]string, len(drifting))
	for i, t := range drifting {
		vs := make([]string, len(t.Versions))
		for j, v := range t.Versions {
			vs[j] = v.Version
		}
		parts[i] = fmt.Sprintf("%s (%s)", t.Tool, strings.Join(vs, ", "))
	}
	return &exitError{
		code: ExitFindings,
		err:  errors.New("--fail-on drift matched " + strings.Join(parts, ", ")),
	}
}
