package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
	"github.com/sassdavid/relgap/internal/yamlschema"
	curated "github.com/sassdavid/relgap/registry"
)

func newRegistryCmd(d *app.Deps, g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Check relgap registry entries",
		Long: `relgap's registry has one file per tool, registry/<name>.yaml, for tools whose versions or
changelogs need rules that auto-discovery gets wrong. The schema is schemas/registry-v1.json.`,
		Args: noArgs,
	}
	cmd.AddCommand(newRegistryLintCmd(d, g), newRegistryTestCmd(d, g))
	return cmd
}

func newRegistryTestCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "test <tool|file.yaml> <from>..[<to>]",
		Short: "Run each source of a registry entry live and show what it produced",
		Long: `Digests the range once per source of the tool's entry and once with all of them, and
prints for each: the versions it found notes for, its entry counts and the documents they came
from. Entries below --min-confidence count too.

<tool> resolves as in 'relgap resolve'. A path to a .yaml file runs that entry on its own, after
the checks 'relgap registry lint' makes, so a new entry can be tried before it's embedded.

A source that fails is listed with its error and the command exits 1. A source without notes in
the range isn't an error: an upgrade guide only has notes where a new release line starts.`,
		Example: `  relgap registry test kubectl 1.36.0..1.37.2
  relgap registry test registry/argocd.yaml 3.5.0..3.6.1
  relgap registry test python 3.13.5..3.14.0 --format json`,
		Args: exactArgs(2), //nolint:mnd // tool and range
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return complete.NoFiles(cmd, args, toComplete)
			}
			return completeTools(d, g)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryTest(cmd, d, g, args[0], args[1])
		},
	}
}

func runRegistryTest(cmd *cobra.Command, d *app.Deps, g *globals, tool, rangeArg string) error {
	if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatJSON); err != nil {
		return usageError(err)
	}
	from, to, err := parseRange(rangeArg)
	if err != nil {
		return usageError(err)
	}
	spec, err := testSpec(cmd, d, g, tool)
	if err != nil {
		return err
	}
	cache, err := g.newCache(d)
	if err != nil {
		return err
	}
	defer cache.Close()
	defer cache.LogSummary(cmd.Context())
	gh, hc, err := newGitHubClient(cmd.Context(), d, cache, newRunner(d))
	if err != nil {
		return err
	}
	res, err := report.NewDigester(d.Logger, gh, source.NewWeb(hc)).TestSources(cmd.Context(), report.DigestRequest{
		Tool: spec, From: from, To: to, Line: version.Level(spec.Version.Line),
	})
	if err != nil {
		return digestError(err)
	}
	if g.outFormat == render.FormatJSON {
		if err = render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), res)); err != nil {
			return err
		}
	} else if err = render.SourceTestText(d.Stdout, res); err != nil {
		return err
	}
	var failed []string
	for _, s := range res.Sources {
		if s.Error != "" {
			failed = append(failed, fmt.Sprintf("%d (%s)", s.Index, s.Source.Kind))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s %s failed", plural(len(failed), "source", "sources"), strings.Join(failed, ", "))
	}
	return nil
}

// testSpec resolves tool, or loads and lints it when it names a YAML file.
func testSpec(cmd *cobra.Command, d *app.Deps, g *globals, tool string) (registry.ToolSpec, error) {
	if !strings.HasSuffix(tool, ".yaml") {
		resolution, err := g.resolveTool(cmd, d, tool)
		if err != nil {
			return registry.ToolSpec{}, err
		}
		return resolution.Tool, nil
	}
	if _, err := g.loadConfig(cmd, d); err != nil {
		return registry.ToolSpec{}, err
	}
	l, err := registry.NewLinter()
	if err != nil {
		return registry.ToolSpec{}, err
	}
	b, err := os.ReadFile(tool)
	if err != nil {
		return registry.ToolSpec{}, usageError(fmt.Errorf("read registry entry: %w", err))
	}
	e, problems, err := l.Lint(tool, b)
	if err != nil {
		return registry.ToolSpec{}, err
	}
	if len(problems) > 0 {
		return registry.ToolSpec{}, usageError(&registry.LintError{Problems: problems})
	}
	return registry.SpecFromEntry(e), nil
}

func newRegistryLintCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:               "lint [file...]",
		Short:             "Validate registry entries against the schema",
		ValidArgsFunction: complete.Files("yaml", "yml"),
		Long: `Without arguments, checks the entries embedded in relgap. With arguments, checks those files.

Reports unknown keys, wrong types, bad regular expressions and a name that doesn't match the
file name, each with file:line:column, and exits 2 when there is any.`,
		Example: `  relgap registry lint
  relgap registry lint registry/terraform.yaml`,
		RunE: func(_ *cobra.Command, args []string) error {
			if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatJSON); err != nil {
				return usageError(err)
			}
			valid, err := lintEntries(args)
			if err != nil {
				return err
			}
			if g.outFormat == render.FormatJSON {
				data := map[string]any{"valid": valid}
				return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), data))
			}
			var b strings.Builder
			for _, p := range valid {
				b.WriteString("valid: " + p + "\n")
			}
			if _, err = io.WriteString(d.Stdout, b.String()); err != nil {
				return fmt.Errorf("write result: %w", err)
			}
			return nil
		},
	}
}

func lintEntries(paths []string) ([]string, error) {
	l, err := registry.NewLinter()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		entries, loadErr := l.LoadFS(curated.FS(), ".")
		if loadErr != nil {
			return nil, usageError(loadErr)
		}
		valid := make([]string, len(entries))
		for i, e := range entries {
			valid[i] = "registry/" + e.Name + ".yaml"
		}
		return valid, nil
	}
	all := &registry.LintError{}
	for _, p := range paths {
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			all.Problems = append(all.Problems, yamlschema.Problem{Origin: p, Message: readErr.Error()})
			continue
		}
		_, problems, lintErr := l.Lint(p, b)
		if lintErr != nil {
			return nil, lintErr
		}
		all.Problems = append(all.Problems, problems...)
	}
	if len(all.Problems) > 0 {
		return nil, usageError(all)
	}
	return paths, nil
}
