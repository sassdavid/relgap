package cli

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/scan"
)

type pinsFlags struct {
	unresolved bool
	submodules bool
	renovate   string
}

func newPinsCmd(d *app.Deps, g *globals) *cobra.Command {
	var f pinsFlags
	cmd := &cobra.Command{
		Use:   "pins [roots...]",
		Short: "List the tool versions pinned in your repositories, with file and line",
		Long: `Scans each root for repositories and lists every pinned tool version, with the
file, line and detector that found it. It reads files only and makes no network calls.

A directory that contains .git is a repository; the scan doesn't descend into another
repository below it. A root without any .git is one repository itself. git ls-files lists
the files, so .gitignore applies; without git, node_modules, vendor, .terraform and
.terragrunt-cache are skipped.

Detectors: mise (mise.toml and mise config directories), tool-versions, aqua, idiomatic
(.terraform-version, .nvmrc, go.mod and others), terraform, tf-lock, terragrunt, dockerfile,
ci (GitHub Actions, GitLab CI, Bitbucket Pipelines), helm, generic (scan.extra_files, opt-in
with scan.detectors.generic) and renovate (--renovate-report).

A value marked ≈ allows more than one version: a constraint (~> 1.15), a prefix (26) or a
channel (latest). A pin whose name maps to no known tool has an empty tool; list those
with --unresolved and map them with aliases in the config.

Roots default to scan.roots, then the current directory.`,
		Example: `  relgap pins
  relgap pins ~/src/infra --format json
  relgap pins . --unresolved
  relgap pins . --renovate-report renovate-report.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPins(cmd, d, g, f, args)
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&f.unresolved, "unresolved", false, "list only pins that map to no known tool")
	fl.BoolVar(&f.submodules, "submodules", false, "scan checked-out git submodules as repositories")
	fl.StringVar(&f.renovate, "renovate-report", "",
		"add the pins in this Renovate JSON report (renovate --platform=local --dry-run=extract --report-type=file)")
	_ = cmd.RegisterFlagCompletionFunc("renovate-report", complete.Files("json"))
	addOutputFlags(cmd, g, pinsFormats()...)
	cmd.ValidArgsFunction = complete.Dirs()
	return cmd
}

func runPins(cmd *cobra.Command, d *app.Deps, g *globals, f pinsFlags, args []string) error {
	if err := g.checkFormats(pinsFormats()...); err != nil {
		return usageError(err)
	}
	resolver, cfg, err := g.loadResolver(cmd, d)
	if err != nil {
		return err
	}
	opts, err := pinsOptions(cfg.Config.Scan, f)
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
	pins := res.Pins
	if f.unresolved {
		pins = []scan.Pin{}
		for _, p := range res.Pins {
			if !p.Resolved() {
				pins = append(pins, p)
			}
		}
	}
	if err = writePins(cmd, d, g, res, pins, f.unresolved); err != nil {
		return err
	}
	for _, p := range res.Problems {
		d.Logger.WarnContext(cmd.Context(), "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	if len(res.Problems) > 0 {
		return fmt.Errorf("%d file(s) couldn't be parsed; the list is partial", len(res.Problems))
	}
	return nil
}

func pinsOptions(sc config.Scan, f pinsFlags) (scan.Options, error) {
	opts := scan.Options{
		Exclude:    sc.Exclude,
		ExtraFiles: sc.ExtraFiles,
		Generic:    sc.Detectors.Generic,
		Actions:    sc.Detectors.CIActions,
		Submodules: f.submodules,
	}
	if f.renovate != "" {
		data, err := os.ReadFile(f.renovate)
		if err != nil {
			return opts, usageError(fmt.Errorf("read renovate report: %w", err))
		}
		if opts.Report, err = scan.RenovateReport(data); err != nil {
			return opts, usageError(err)
		}
	}
	return opts, nil
}

// pinsData is the data of `pins --format json`, defined by schemas/pins-v1.json.
type pinsData struct {
	Repos []string   `json:"repos"`
	Pins  []scan.Pin `json:"pins"`
}

func pinsFormats() []render.Format {
	return []render.Format{render.FormatTTY, render.FormatPlain, render.FormatMD, render.FormatJSON}
}

func writePins(
	cmd *cobra.Command, d *app.Deps, g *globals, res *scan.Result, pins []scan.Pin, unresolved bool,
) error {
	err := g.sink(cmd, d).Write(func(w io.Writer, f render.Format) error {
		switch f {
		case render.FormatJSON:
			env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), pinsData{Repos: res.Repos, Pins: pins})
			for _, p := range res.Problems {
				env.Errors = append(env.Errors, render.Error{Target: path.Join(p.Repo, p.File), Message: p.Message})
			}
			return render.JSON(w, env)
		case render.FormatMD:
			return render.PinsMarkdown(w, pins, unresolved)
		case render.FormatAuto, render.FormatTTY, render.FormatPlain, render.FormatCSV, render.FormatSARIF,
			render.FormatGitHub:
		}
		return render.PinsText(w, pins, unresolved)
	})
	if err != nil {
		return err
	}
	if unresolved && len(pins) > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: map these names to tools with aliases.<raw name>: <tool> in %s\n",
			config.AppName, config.ProjectFileName)
	}
	if n := countUnresolved(res.Pins); !unresolved && n > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s no known tool; list them with --unresolved\n",
			config.AppName, n, plural(n, "pin has", "pins have"))
	}
	return nil
}

func countUnresolved(pins []scan.Pin) int {
	n := 0
	for _, p := range pins {
		if !p.Resolved() {
			n++
		}
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
