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
	"github.com/sassdavid/relgap/internal/docsurl"
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
		Short: "Check, test and update relgap registry entries",
		Long: `relgap's registry has one file per tool, registry/<name>.yaml, for tools whose versions or
changelogs need rules that auto-discovery gets wrong. The schema is ` + docsurl.Schema("registry-v1.json") + `.`,
		Example: `  relgap registry lint registry/terraform.yaml
  relgap registry test terraform 1.9.0..1.10.0
  relgap registry update`,
		Args: noArgs,
	}
	cmd.AddCommand(newRegistryLintCmd(d, g), newRegistryTestCmd(d, g), newRegistryUpdateCmd(d, g))
	return cmd
}

func newRegistryTestCmd(d *app.Deps, g *globals) *cobra.Command {
	var guess bool
	cmd := &cobra.Command{
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
			return runRegistryTest(cmd, d, g, args[0], args[1], guess)
		},
	}
	addGuessFlag(cmd, &guess)
	setFormats(cmd, textFormats()...)
	setExitCodes(cmd, ExitAuth)
	setRelated(cmd, "guides/registry-entry", "reference/registry-schema")
	return cmd
}

func runRegistryTest(cmd *cobra.Command, d *app.Deps, g *globals, tool, rangeArg string, guess bool) error {
	from, to, err := parseRange(rangeArg)
	if err != nil {
		return usageError(err)
	}
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	s.guess = guess
	spec, err := testSpec(cmd, s, tool)
	if err != nil {
		return err
	}
	gh, hc, err := s.clients(cmd.Context())
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
func testSpec(cmd *cobra.Command, s *session, tool string) (registry.ToolSpec, error) {
	if !strings.HasSuffix(tool, ".yaml") {
		resolution, err := s.resolve(cmd.Context(), tool)
		if err != nil {
			return registry.ToolSpec{}, err
		}
		return resolution.Tool, nil
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
	cmd := &cobra.Command{
		Use:               "lint [file...]",
		Short:             "Validate registry entries against the schema",
		ValidArgsFunction: complete.Files("yaml", "yml"),
		Long: `Without arguments, checks the entries embedded in relgap. With arguments, checks those files.

Reports unknown keys and wrong types; regular expressions that don't compile (version.exclude,
version.extract and source headings); version.scheme regex without version.extract and the
reverse; unknown placeholders in path_template, url_template, ref and anchor; a url_template
host relgap doesn't fetch from; CSS selectors in content and strip that don't parse; and a name
that doesn't match the file name. Each problem has file:line:column, and lint exits 2 when there
is any.`,
		Example: `  relgap registry lint
  relgap registry lint registry/terraform.yaml`,
		RunE: func(_ *cobra.Command, args []string) error {
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
	setFormats(cmd, textFormats()...)
	setExitCodes(cmd)
	setRelated(cmd, "guides/registry-entry", "reference/registry-schema")
	return cmd
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

func newRegistryUpdateCmd(d *app.Deps, g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download the newest registry entries from relgap's repository",
		Long: `Downloads registry/*.yaml from the main branch of ` + registry.UpdateRepo + ` through the
GitHub contents API, with the HTTP cache's ETags, and keeps the entries that differ from the ones
embedded in this binary. Resolution then uses them in place of the embedded entries of the same
name, and 'relgap resolve' shows "relgap registry  updated" for them.

An entry that fails 'relgap registry lint' or whose min_relgap is newer than this binary is
skipped, and the embedded entry stays in use. Each run replaces the entries of the run before.
The entries live in the registry directory next to the HTTP cache; 'relgap cache clear' removes
them.`,
		Example: `  relgap registry update
  relgap registry update --format json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := g.newSession(cmd, d)
			if err != nil {
				return err
			}
			defer s.close(cmd.Context())
			gh, _, err := s.clients(cmd.Context())
			if err != nil {
				return err
			}
			dir, err := registryDir(d)
			if err != nil {
				return err
			}
			res, err := registry.Update(cmd.Context(), gh, dir, buildinfo.Get().Version)
			if err != nil {
				return digestError(err)
			}
			if g.outFormat == render.FormatJSON {
				return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), res))
			}
			return writeUpdate(d.Stdout, res)
		},
	}
	setFormats(cmd, textFormats()...)
	setExitCodes(cmd, ExitAuth)
	setRelated(cmd, "guides/registry-entry", "reference/registry-schema")
	return cmd
}

func writeUpdate(w io.Writer, res *registry.UpdateResult) error {
	var b strings.Builder
	counts := map[string]int{}
	for _, e := range res.Entries {
		counts[e.Status]++
		if e.Status == registry.UpdateUnchanged {
			continue
		}
		line := e.Status + ": " + e.Name
		if e.Reason != "" {
			line += " (" + e.Reason + ")"
		}
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "%d added, %d changed, %d unchanged, %d skipped from %s@%s into %s\n",
		counts[registry.UpdateAdded], counts[registry.UpdateChanged], counts[registry.UpdateUnchanged],
		counts[registry.UpdateSkipped], res.Repo, res.Ref, res.Dir)
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}
