package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/mcpkit"
	"github.com/sassdavid/relgap/internal/mcpserver"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
)

// serveFunc serves an MCP server until the client is done.
type serveFunc func(ctx context.Context, s *mcpkit.Server) error

// mcpFlags holds the flags of `relgap mcp`.
type mcpFlags struct {
	roots    []string
	maxBytes int
}

func newMCPCmd(d *app.Deps, g *globals) *cobra.Command {
	var f mcpFlags
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve digest, breaking_changes, outdated, drift and resolve to an MCP client over stdio",
		Long: `Runs a Model Context Protocol server on stdin and stdout, for agents such as Claude Code.
Logs go to stderr. Each tool returns the JSON envelope 'relgap <command> --format json'
prints:

  digest            relgap digest <tool> <from>..[<to>], with only, min_confidence and line
  breaking_changes  the same digest with only the breaking and deprecation entries
  outdated          relgap outdated <path>
  drift             relgap drift <roots...>, with tools, latest and ignore_constraints
  resolve           relgap resolve <tool>

The tools are read-only. They never start a process: the mise registry comes from the
embedded snapshot, scans walk directories instead of asking git (so .gitignore doesn't
apply), and the GitHub token comes from GH_TOKEN, GITHUB_TOKEN or gh's config file, not
from 'gh auth token'. They write nothing except the HTTP cache, which they share with the
other commands, so repeated calls are cheap. --offline, --refresh and --no-cache apply to
every call.

outdated and drift read only directories inside the roots: the --root directories, else
the roots the client lists (Claude Code lists its launch directory and the directories
added with --add-dir), else the current directory. A relative path starts at the first
root, and repositories are named relative to it. The config file is read once at startup,
as for every command.

A result over --max-result-bytes (default 50,000, Claude Code's limit for showing a result
to the model) is split. The envelope then has a page field, and the agent passes
page.next_cursor back to the same tool for the rest. Breaking changes, failed and
outdated pins, and drifting tools come first. A cursor lasts 15 minutes after its last use.`,
		Example: `  claude mcp add --transport stdio --scope user relgap -- relgap mcp
  relgap mcp --root ~/src/infra --root ~/src/apps
  relgap mcp --offline`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			serve := g.mcpServe
			if serve == nil {
				serve = func(ctx context.Context, s *mcpkit.Server) error { return s.Run(ctx, d.Stdin, d.Stdout) }
			}
			return runMCP(cmd, d, g, f, serve)
		},
	}
	cmd.Flags().StringArrayVar(&f.roots, "root", nil,
		"directory that outdated and drift may read; repeatable (default: the client's roots, else the current directory)")
	cmd.Flags().IntVar(&f.maxBytes, "max-result-bytes", mcpkit.MaxBytes,
		"split results larger than this many bytes of JSON into pages; 0 never splits")
	registerCompletions(cmd, map[string]cobra.CompletionFunc{
		"root":             complete.Dirs(),
		"max-result-bytes": complete.NoFiles,
	})
	return cmd
}

// ServeMCP runs `relgap mcp args...` but hands the server to serve instead of serving stdio, so
// tests can connect an in-process client.
func ServeMCP(
	ctx context.Context,
	d *app.Deps,
	args []string,
	serve func(context.Context, *mcpkit.Server) error,
) error {
	root, g := newRoot(d)
	g.mcpServe = serve
	root.SetArgs(append([]string{"mcp"}, args...))
	err := root.ExecuteContext(ctx)
	g.close()
	if err != nil {
		return fmt.Errorf("relgap mcp: %w", err)
	}
	return nil
}

func runMCP(cmd *cobra.Command, d *app.Deps, g *globals, f mcpFlags, serve serveFunc) error {
	ctx := cmd.Context()
	maxBytes := f.maxBytes
	switch {
	case maxBytes < 0:
		return usageError(fmt.Errorf("--max-result-bytes must be 0 or more, got %d", maxBytes))
	case maxBytes == 0:
		maxBytes = -1 // mcpkit's value for no cap
	}
	cfg, err := g.loadConfig(cmd, d)
	if err != nil {
		return err
	}
	resolver, err := newResolver(ctx, d, cfg, mcpserver.NoExec{})
	if err != nil {
		return err
	}
	cwd, err := d.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	s := &session{
		d:        d,
		resolver: resolver,
		runner:   mcpserver.NoExec{},
		newCache: func() (*httpx.Cache, error) { return g.newCache(d) },
	}
	defer s.close(ctx)
	srv := mcpserver.New(mcpserver.Options{
		Version:  buildinfo.Get().Version,
		Logger:   d.Logger,
		Clock:    d.Clock,
		MaxBytes: maxBytes,
		Roots:    f.roots,
		Cwd:      cwd,
		Backend:  s.backend(),
	})
	d.Logger.InfoContext(ctx, "serving MCP on stdio", "roots", f.roots)
	return serve(ctx, srv)
}

// backend runs the MCP tools through the same pipelines as the commands.
func (s *session) backend() mcpserver.Backend {
	return mcpserver.Backend{
		Digest: func(ctx context.Context, a mcpserver.DigestArgs) (*report.ToolDigest, error) {
			minConfidence := s.d.Config.Config.Policy.MinConfidence
			if a.MinConfidence != nil {
				minConfidence = *a.MinConfidence
			}
			return s.digest(ctx, digestArgs{
				tool:               a.Tool,
				from:               a.From,
				to:                 a.To,
				line:               a.Line,
				includePrereleases: a.IncludePrereleases,
				minConfidence:      minConfidence,
				kinds:              a.Kinds,
			})
		},
		Outdated: func(ctx context.Context, root, base string) (*report.Outdated, []render.Error, error) {
			run, err := s.compare(ctx, root, nil, compareOptions{enrich: true, base: base})
			if err != nil {
				return nil, nil, err
			}
			return run.out, append(run.errors(), run.sourceErrors()...), nil
		},
		Drift: func(ctx context.Context, a mcpserver.DriftArgs) (*report.Drift, []render.Error, error) {
			res, out, _, err := s.drift(ctx, driftArgs{
				roots:             a.Roots,
				tools:             a.Tools,
				latest:            a.Latest,
				ignoreConstraints: a.IgnoreConstraints,
				base:              a.Base,
			})
			if err != nil {
				return nil, nil, err
			}
			return out, driftErrors(res, out), nil
		},
		Resolve: func(name string) (*registry.Resolution, error) { return s.resolve(name) },
	}
}
