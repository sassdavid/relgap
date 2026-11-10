package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
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
	cmd.AddCommand(newRegistryLintCmd(d, g))
	return cmd
}

func newRegistryLintCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "lint [file...]",
		Short: "Validate registry entries against the schema",
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
