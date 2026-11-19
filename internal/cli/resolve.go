package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
)

func newResolveCmd(d *app.Deps, g *globals) *cobra.Command {
	var guess bool
	cmd := &cobra.Command{
		Use:   "resolve <tool>",
		Short: "Show how a tool name resolves to a repository, version rules and changelog sources",
		Long: `Resolves a tool name and prints the result with the layer that supplied each field.

<tool> is a name or alias (terraform, tf), a NAME_VERSION variable (TERRAFORM_VERSION),
github:owner/repo, aqua:package, tfprovider:namespace/name, tfmodule:namespace/name/provider,
chart:repo/name, chart:name, npm:package or pypi:package. When a registry knows the tool, the
result is the same as for its name.

tfprovider:, tfmodule:, chart:, npm: and pypi: names that no registry knows are looked up in the
Terraform registry, Artifact Hub, the npm registry or PyPI, which name the source repository. A
module's tag prefix comes from the tag of its latest version. chart:name without a repository
resolves only when one Artifact Hub repository has a chart of that name.

Layers, highest precedence first:

  config           tools.<name> and aliases in the config file
  relgap registry  curated entries embedded in relgap (registry/*.yaml), or the newer copies
                   from 'relgap registry update'
  mise registry    mise registry --json when mise is on PATH, otherwise an embedded snapshot
  aqua registry    an embedded index of the aqua registry
  default          built-in defaults

With --guess, a name that no layer knows is searched on GitHub. The repository whose name
matches exactly and has the most stars wins, and the result is marked low-confidence.`,
		Example: `  relgap resolve terraform
  relgap resolve TERRAFORM_VERSION
  relgap resolve aqua:hashicorp/terraform --format json
  relgap resolve tfprovider:integrations/github
  relgap resolve tfmodule:terraform-aws-modules/vpc/aws
  relgap resolve chart:argo/argo-cd
  relgap resolve some-tool --guess`,
		Args: exactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return complete.NoFiles(cmd, args, toComplete)
			}
			return completeTools(d, g)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := g.newSession(cmd, d)
			if err != nil {
				return err
			}
			defer s.close(cmd.Context())
			s.guess = guess
			resolution, err := s.resolve(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if g.outFormat == render.FormatJSON {
				return render.JSON(d.Stdout, render.NewEnvelope(buildinfo.Get().Version, d.Clock(), resolution))
			}
			return writeResolution(d, resolution)
		},
	}
	addGuessFlag(cmd, &guess)
	setFormats(cmd, textFormats()...)
	setExitCodes(cmd)
	setRelated(cmd, "concepts/resolution", "guides/unresolved")
	return cmd
}

// addGuessFlag adds --guess, which lets resolution search GitHub for an unknown name.
func addGuessFlag(cmd *cobra.Command, guess *bool) {
	cmd.Flags().BoolVar(guess, "guess", false,
		"search GitHub for a tool no registry knows and use the exact name match with the most stars")
}

// resolveName resolves name with r. Unknown and ambiguous tools are usage errors.
func resolveName(
	ctx context.Context,
	r *registry.Resolver,
	name string,
	opts registry.ResolveOptions,
) (*registry.Resolution, error) {
	resolution, err := r.Resolve(ctx, name, opts)
	if errors.Is(err, registry.ErrUnknownTool) || errors.Is(err, registry.ErrUnsupported) ||
		errors.Is(err, registry.ErrNoRepo) || errors.Is(err, registry.ErrAmbiguous) {
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
	r, err := newResolver(cmd.Context(), d, res, newRunner(d))
	if err != nil {
		return nil, nil, err
	}
	return r, res, nil
}

// newResolver loads the registries with the config's tools and aliases. runner starts
// `mise registry --json`; without mise the embedded snapshot is used.
func newResolver(
	ctx context.Context,
	d *app.Deps,
	res *config.Result,
	runner misex.Runner,
) (*registry.Resolver, error) {
	cfg := registry.ConfigLayer{Tools: map[string]string{}, Aliases: res.Config.Aliases}
	for tool, t := range res.Config.Tools {
		cfg.Tools[tool] = t.Repo
	}
	updated, err := registryDir(d)
	if err != nil {
		return nil, err
	}
	r, err := registry.LoadWith(ctx, d.Logger, misex.New(runner), cfg, registry.LoadOptions{
		UpdatedDir: updated,
		Version:    buildinfo.Get().Version,
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // registry.Load names what failed
	}
	return r, nil
}

func newRunner(d *app.Deps) *execx.Runner {
	return execx.NewRunner(d.Logger, execx.WithEnviron(func() []string { return d.Env }))
}

func writeResolution(d *app.Deps, res *registry.Resolution) error {
	t := res.Tool
	tw := tabwriter.NewWriter(d.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	_, _ = fmt.Fprintf(tw, "input\t%s (%s, found in %s)\n", res.Input, res.Match.By, res.Match.Layer)
	_, _ = fmt.Fprintf(tw, "mise registry\t%s\n", res.MiseSource)
	if res.RelgapSource != "" {
		_, _ = fmt.Fprintf(tw, "relgap registry\t%s\n", res.RelgapSource)
	}
	if res.Confidence != "" {
		_, _ = fmt.Fprintf(tw, "confidence\t%s\n", res.Confidence)
	}
	if len(res.Alternatives) > 0 {
		_, _ = fmt.Fprintf(tw, "alternatives\t%s\n", strings.Join(res.Alternatives, ", "))
	}
	_, _ = fmt.Fprintln(tw)
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

// completeTools completes a tool name from the embedded registries and the config's tools.
func completeTools(d *app.Deps, g *globals) cobra.CompletionFunc {
	return complete.Dynamic(complete.Timeout, completionTools(d, g))
}

// completeToolList is [completeTools] for a comma-separated list.
func completeToolList(d *app.Deps, g *globals) cobra.CompletionFunc {
	return complete.DynamicList(complete.Timeout, completionTools(d, g))
}

func completionTools(d *app.Deps, g *globals) complete.Source {
	return complete.Tools(
		func(_ context.Context, cmd *cobra.Command) (registry.ConfigLayer, error) {
			cwd, err := d.Getwd()
			if err != nil {
				return registry.ConfigLayer{}, fmt.Errorf("get working directory: %w", err)
			}
			res, err := config.Load(config.Options{
				Environ:      d.Env,
				Cwd:          cwd,
				ExplicitFile: g.configFile,
				Flags:        cmd.Flags(),
				FlagKeys:     flagKeys(),
			})
			if err != nil {
				return registry.ConfigLayer{}, err //nolint:wrapcheck // config errors name the file
			}
			layer := registry.ConfigLayer{Tools: map[string]string{}, Aliases: res.Config.Aliases}
			for tool, t := range res.Config.Tools {
				layer.Tools[tool] = t.Repo
			}
			return layer, nil
		})
}
