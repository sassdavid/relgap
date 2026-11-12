package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
)

func newResolveCmd(d *app.Deps, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <tool>",
		Short: "Show how a tool name resolves to a repository, version rules and changelog sources",
		Long: `Resolves a tool name and prints the result with the layer that supplied each field.

<tool> is a name or alias (terraform, tf), a NAME_VERSION variable (TERRAFORM_VERSION),
github:owner/repo or aqua:package. github: and aqua: identify a tool; when a registry knows it,
the result is the same as for its name.

Layers, highest precedence first:

  config           tools.<name> and aliases in the config file
  relgap registry  curated entries embedded in relgap (registry/*.yaml)
  mise registry    mise registry --json when mise is on PATH, otherwise an embedded snapshot
  aqua registry    an embedded index of the aqua registry
  default          built-in defaults`,
		Example: `  relgap resolve terraform
  relgap resolve TERRAFORM_VERSION
  relgap resolve aqua:hashicorp/terraform --format json`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := g.outFormat.Check(render.FormatTTY, render.FormatPlain, render.FormatJSON); err != nil {
				return usageError(err)
			}
			resolution, err := g.resolveTool(cmd, d, args[0])
			if err != nil {
				return err
			}
			if g.outFormat == render.FormatJSON {
				return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), resolution))
			}
			return writeResolution(d, resolution)
		},
	}
}

// resolveTool loads the config and the registries and resolves name. Unknown tools are usage errors.
func (g *globals) resolveTool(cmd *cobra.Command, d *app.Deps, name string) (*registry.Resolution, error) {
	r, _, err := g.loadResolver(cmd, d)
	if err != nil {
		return nil, err
	}
	resolution, err := r.Resolve(name)
	if errors.Is(err, registry.ErrUnknownTool) || errors.Is(err, registry.ErrUnsupported) ||
		errors.Is(err, registry.ErrNoRepo) {
		return nil, usageError(err)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", name, err)
	}
	return resolution, nil
}

// loadResolver loads the config and the registries.
func (g *globals) loadResolver(cmd *cobra.Command, d *app.Deps) (*registry.Resolver, *config.Result, error) {
	res, err := g.loadConfig(cmd, d)
	if err != nil {
		return nil, nil, err
	}
	cfg := registry.ConfigLayer{Tools: map[string]string{}, Aliases: res.Config.Aliases}
	for tool, t := range res.Config.Tools {
		cfg.Tools[tool] = t.Repo
	}
	r, err := registry.Load(cmd.Context(), d.Logger, misex.New(newRunner(d)), cfg)
	if err != nil {
		return nil, nil, err
	}
	return r, res, nil
}

func newRunner(d *app.Deps) *execx.Runner {
	return execx.NewRunner(d.Logger, execx.WithEnviron(func() []string { return d.Env }))
}

func writeResolution(d *app.Deps, res *registry.Resolution) error {
	t := res.Tool
	tw := tabwriter.NewWriter(d.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	_, _ = fmt.Fprintf(tw, "input\t%s (%s, found in %s)\n", res.Input, res.Match.By, res.Match.Layer)
	_, _ = fmt.Fprintf(tw, "mise registry\t%s\n\n", res.MiseSource)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE\tSOURCE")
	row := func(key, value string) {
		if value == "" {
			return
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", key, value, provenance(res, key))
	}
	row("name", t.Name)
	row("aliases", strings.Join(t.Aliases, ", "))
	row("repo", t.Repo)
	row("version.scheme", t.Version.Scheme)
	row("version.extract", t.Version.Extract)
	if t.Version.TagPrefix != "" {
		row("version.tag_prefix", strconv.Quote(t.Version.TagPrefix))
	}
	row("version.exclude", t.Version.Exclude)
	row("version.universe", t.Version.Universe)
	row("version.line", t.Version.Line)
	if len(t.Sources) == 0 {
		row("sources", "auto-discovery")
	}
	for i, s := range t.Sources {
		_, _ = fmt.Fprintf(tw, "sources.%d\t%s\t%s\n", i, sourceText(s), provenance(res, "sources"))
	}
	if t.EOL != nil {
		row("eol", strings.TrimSpace(t.EOL.Product+" "+t.EOL.Cycle))
	}
	osv := make([]string, len(t.OSV))
	for i, o := range t.OSV {
		osv[i] = o.Ecosystem + " " + o.Name
	}
	row("osv", strings.Join(osv, ", "))
	row("breaking_markers", strings.Join(t.BreakingMarkers, ", "))
	row("bin", strings.Join(t.Bin, ", "))
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write resolution: %w", err)
	}
	return nil
}

func provenance(res *registry.Resolution, key string) string {
	layers := res.Provenance[key]
	s := make([]string, len(layers))
	for i, l := range layers {
		s[i] = string(l)
	}
	return strings.Join(s, " + ")
}

func sourceText(s registry.Source) string {
	parts := []string{s.Kind}
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	add("path", s.PathTemplate)
	add("url", s.URLTemplate)
	add("ref", s.Ref)
	add("heading", s.Heading)
	add("ignore", strings.Join(s.IgnoreSections, ","))
	if s.FromHeading != "" {
		add("from_heading", strconv.Quote(s.FromHeading))
	}
	add("content", s.Content)
	add("strip", strings.Join(s.Strip, ","))
	add("anchor", s.Anchor)
	if s.JSON != nil {
		add("json.items", s.JSON.Items)
		add("json.heading", s.JSON.Heading)
		add("json.text", strconv.Quote(s.JSON.Text))
		add("json.link", s.JSON.Link)
	}
	return strings.Join(parts, " ")
}

// exactArgs is cobra.ExactArgs with exit code 2.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usageError(fmt.Errorf("%s takes %d argument(s), got %d", cmd.CommandPath(), n, len(args)))
		}
		return nil
	}
}

func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return usageError(fmt.Errorf("%s takes at most %d argument(s), got %d", cmd.CommandPath(), n, len(args)))
		}
		return nil
	}
}
