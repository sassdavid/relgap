package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/render"
)

const (
	flagConcurrency   = "concurrency"
	flagMinConfidence = "min-confidence"
)

func flagKeys() map[string]string {
	return map[string]string{
		flagConcurrency:   "github.concurrency",
		flagMinConfidence: "policy.min_confidence",
		flagFailOn:        "policy.fail_on",
		flagMaxBehind:     "policy.max_behind",
		flagVulnSeverity:  "policy.vuln_severity",
	}
}

// globals holds the persistent flags and the state derived from them.
type globals struct {
	format        string
	configFile    string
	verbose       int
	quiet         bool
	logFormat     string
	logFile       string
	color         string
	noInput       bool
	concurrency   int
	minConfidence float64
	offline       bool
	refresh       bool
	noCache       bool

	outFormat render.Format
	started   bool
	closers   []io.Closer
	mcpServe  serveFunc // set by [ServeMCP]; nil serves stdio
}

func (g *globals) close() {
	for _, c := range g.closers {
		_ = c.Close()
	}
	g.closers = nil
}

// NewRootCmd returns the full command tree. scripts/gendocs uses it.
func NewRootCmd(d *app.Deps) *cobra.Command {
	root, _ := newRoot(d)
	root.InitDefaultHelpCmd()
	return root
}

func newRoot(d *app.Deps) (*cobra.Command, *globals) {
	g := &globals{}
	def := config.Default()
	root := &cobra.Command{
		Use:   config.AppName,
		Short: "Release-notes digests and toolchain drift for the tools you pin",
		Long: `relgap merges release notes across a version range into one digest (breaking changes
first, with source links), and reports toolchain drift, EOL and known vulnerabilities across
your repositories.

stdout carries data, stderr carries logs. Use --format json for scripting.`,
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return g.setup(cmd, d)
		},
	}
	root.SetIn(d.Stdin)
	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })

	pf := root.PersistentFlags()
	pf.StringVar(&g.format, "format", "", complete.Usage(
		"output format: {values} (default tty when stdout is a terminal, else plain)", formatChoices()))
	pf.StringVar(&g.configFile, "config", "",
		"config file to use instead of the discovered "+config.ProjectFileName+" (env "+config.EnvConfigFile+")")
	pf.CountVarP(&g.verbose, "verbose", "v", "more logging: -v info, -vv debug, -vvv trace")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "log errors only")
	pf.StringVar(&g.logFormat, "log-format", string(app.LogFormatText),
		complete.Usage("log format: {values}", logFormatChoices()))
	pf.StringVar(&g.logFile, "log-file", "", "write logs to this file instead of stderr")
	pf.StringVar(&g.color, "color", string(render.ColorAuto),
		complete.Usage("colorize output: {values}", colorChoices()))
	pf.BoolVar(&g.noInput, "no-input", false, "never prompt for input")
	pf.IntVar(&g.concurrency, flagConcurrency, def.GitHub.Concurrency,
		"maximum concurrent requests per tool (config github.concurrency)")
	pf.Float64Var(&g.minConfidence, flagMinConfidence, def.Policy.MinConfidence,
		"hide entries below this confidence, 0..1 (config policy.min_confidence)")
	pf.BoolVar(&g.offline, "offline", false, "answer from the HTTP cache only and fail on a miss")
	pf.BoolVar(&g.refresh, "refresh", false, "revalidate every cached response, even fresh ones")
	pf.BoolVar(&g.noCache, "no-cache", false, "neither read nor write the HTTP cache")
	root.MarkFlagsMutuallyExclusive("verbose", "quiet")
	root.MarkFlagsMutuallyExclusive("offline", "refresh", "no-cache")
	registerCompletions(root, map[string]cobra.CompletionFunc{
		"format":          complete.Enum(formatChoices()),
		"config":          complete.Files("yaml", "yml"),
		"log-format":      complete.Enum(logFormatChoices()),
		"log-file":        complete.Files(),
		"color":           complete.Enum(colorChoices()),
		flagConcurrency:   complete.NoFiles,
		flagMinConfidence: complete.Enum(minConfidenceHints()),
	})

	root.AddCommand(
		newVersionCmd(d, g),
		newConfigCmd(d, g),
		newResolveCmd(d, g),
		newDigestCmd(d, g),
		newRegistryCmd(d, g),
		newCacheCmd(d, g),
		newPinsCmd(d, g),
		newOutdatedCmd(d, g),
		newDriftCmd(d, g),
		newMCPCmd(d, g),
	)
	addCompletionShells(root)
	defaultNoFiles(root)
	return root, g
}

func (g *globals) setup(cmd *cobra.Command, d *app.Deps) error {
	// Completion requests get unparsed flags here and may print nothing but results.
	if complete.IsRequest(cmd) {
		d.Logger = slog.New(slog.DiscardHandler)
		return nil
	}
	// cobra checks these only after PersistentPreRunE; check them first so they exit 2.
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return usageError(err)
	}
	if err := cmd.ValidateFlagGroups(); err != nil {
		return usageError(err)
	}
	format, err := render.ParseFormat(g.format)
	if err != nil {
		return usageError(err)
	}
	colorMode, err := render.ParseColorMode(g.color)
	if err != nil {
		return usageError(err)
	}
	logFormat, err := app.ParseLogFormat(g.logFormat)
	if err != nil {
		return usageError(err)
	}

	logW, logTTY := d.Stderr, app.IsTerminal(d.Stderr)
	if g.logFile != "" {
		f, openErr := os.OpenFile(g.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if openErr != nil {
			return usageError(fmt.Errorf("open log file: %w", openErr))
		}
		g.closers = append(g.closers, f)
		logW, logTTY = f, false
	}
	d.Logger = app.NewLogger(logW, app.LogOptions{
		Format:  logFormat,
		Verbose: g.verbose,
		Quiet:   g.quiet,
		Color:   render.UseColor(colorMode, d.Env.Lookup, logTTY) && app.EnableVirtualTerminal(logW),
	})
	g.outFormat = format.Resolve(app.IsTerminal(d.Stdout))
	g.started = true
	d.Logger.DebugContext(cmd.Context(), "starting", "command", cmd.CommandPath(), "format", string(g.outFormat))
	return nil
}

func (g *globals) loadConfig(cmd *cobra.Command, d *app.Deps) (*config.Result, error) {
	cwd, err := d.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	res, err := config.Load(config.Options{
		Environ:      d.Env,
		Cwd:          cwd,
		ExplicitFile: g.configFile,
		Flags:        cmd.Flags(),
		FlagKeys:     flagKeys(),
	})
	if err != nil {
		return nil, err
	}
	for _, name := range res.UnknownEnv {
		d.Logger.WarnContext(cmd.Context(), "ignoring unknown environment variable", "name", name)
	}
	for _, f := range res.Files {
		if f.Found {
			d.Logger.DebugContext(cmd.Context(), "loaded config file", "kind", string(f.Kind), "path", f.Path)
		}
	}
	d.Config = res
	return res, nil
}

func formatChoices() []complete.Choice {
	return []complete.Choice{
		{Value: string(render.FormatTTY), Desc: "table with color for a terminal"},
		{Value: string(render.FormatPlain), Desc: "table without color"},
		{Value: string(render.FormatMD), Desc: "Markdown"},
		{Value: string(render.FormatJSON), Desc: "JSON envelope for scripts"},
		{Value: string(render.FormatSARIF), Desc: "SARIF 2.1.0 for code scanning"},
	}
}

// minConfidenceHints are example values; --min-confidence takes any number from 0 to 1.
func minConfidenceHints() []complete.Choice {
	return []complete.Choice{
		{Value: "0", Desc: "show every entry"},
		{Value: "0.5", Desc: "the default: hides keyword-only and unmatched entries"},
		{Value: "0.7", Desc: "leading verbs, commit types and headings"},
		{Value: "0.9", Desc: "headings, breaking markers and footers only"},
	}
}

// noArgs is cobra.NoArgs with exit code 2.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(fmt.Errorf("%s takes no arguments, got %q", cmd.CommandPath(), strings.Join(args, " ")))
	}
	return nil
}
