package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/execx"
	"github.com/sassdavid/relgap/internal/mcpkit"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
)

// Name is the server_info name.
const Name = "relgap"

const instructions = `relgap answers questions about upgrading the tools a project pins: what breaks ` +
	`between two versions, which pins are behind, and where repositories disagree. Every result is ` +
	`the JSON envelope of relgap --format json. When the envelope has page.next_cursor, call the ` +
	`same tool with only that cursor to get the rest. Directory arguments must be inside the ` +
	`server's roots; relative ones start at the first root.`

// NoExec refuses to start any command, as if none were installed. With it, the registry uses
// its embedded mise snapshot and the scanner walks directories instead of asking git.
type NoExec struct{}

// Run returns [execx.ErrNotFound].
func (NoExec) Run(_ context.Context, c execx.Cmd) (*execx.Result, error) {
	return nil, fmt.Errorf("%s: %w", c.Name, execx.ErrNotFound)
}

// DigestArgs says what one digest covers.
type DigestArgs struct {
	Tool, From, To     string
	Line               string // empty means the tool's registry line
	IncludePrereleases bool
	MinConfidence      *float64        // nil means the config's policy.min_confidence
	Kinds              []classify.Kind // empty means every kind
}

// DriftArgs says what a drift report covers. Roots are checked against the server's roots.
type DriftArgs struct {
	Roots, Tools      []string
	Latest            bool
	IgnoreConstraints bool
	Base              string // name repositories relative to this directory
}

// Backend runs the pipelines of the CLI commands. Errors make the call fail; the returned
// [render.Error] lists say what a partial result misses.
type Backend struct {
	Digest   func(ctx context.Context, a DigestArgs) (*report.ToolDigest, error)
	Outdated func(ctx context.Context, root, base string) (*report.Outdated, []render.Error, error)
	Drift    func(ctx context.Context, a DriftArgs) (*report.Drift, []render.Error, error)
	Resolve  func(name string) (*registry.Resolution, error)
}

// Options configures [New].
type Options struct {
	Version  string // relgap's version, for server_info and envelopes
	Logger   *slog.Logger
	Clock    func() time.Time
	MaxBytes int      // see [mcpkit.Options]
	Roots    []string // directories tool paths must be in; empty means the client's roots
	Cwd      string   // the root when neither Roots nor the client name one
	Backend  Backend
}

// New returns the MCP server with every relgap tool.
func New(o Options) *mcpkit.Server {
	s := mcpkit.NewServer(mcpkit.Options{
		Name:         Name,
		Version:      o.Version,
		Instructions: instructions,
		Logger:       o.Logger,
		Clock:        o.Clock,
		MaxBytes:     o.MaxBytes,
	})
	t := &tools{o: o}
	askRoots := len(o.Roots) == 0
	mcpkit.AddTool(s, mcpkit.Tool{
		Name:  "digest",
		Title: "Release-notes digest",
		Description: "Merges the release notes of every version of a tool after from, up to to, into entries " +
			"grouped by kind (breaking, deprecation, security, feature, fix, other), each with its source URL.",
		OpenWorld: true,
	}, t.digest)
	mcpkit.AddTool(s, mcpkit.Tool{
		Name:        "breaking_changes",
		Title:       "Breaking changes",
		Description: "Lists only the breaking changes and deprecations between two versions of a tool, each with its source URL.",
		OpenWorld:   true,
	}, t.breakingChanges)
	mcpkit.AddTool(s, mcpkit.Tool{
		Name:  "outdated",
		Title: "Outdated pins",
		Description: "Compares every tool version pinned in a project directory with the latest release, " +
			"with breaking-change counts, EOL state and known vulnerabilities.",
		OpenWorld: true,
		Roots:     askRoots,
	}, t.outdated)
	mcpkit.AddTool(s, mcpkit.Tool{
		Name:  "drift",
		Title: "Version drift",
		Description: "Shows which repositories under the given directories pin different versions of the same tool, " +
			"from local files only unless latest is true.",
		OpenWorld: true,
		Roots:     askRoots,
	}, t.drift)
	mcpkit.AddTool(s, mcpkit.Tool{
		Name:  "resolve",
		Title: "Resolve a tool",
		Description: "Shows how a tool name resolves to a GitHub repository, version rules and changelog sources, " +
			"with the registry layer behind each field.",
	}, t.resolve)
	return s
}

type tools struct {
	o Options
}

func (t *tools) envelope(data any, errs []render.Error) render.Envelope {
	env := render.NewEnvelope(t.o.Version, t.o.Clock(), data)
	if errs != nil {
		env.Errors = errs
	}
	return env
}

type digestInput struct {
	mcpkit.Paging

	Tool               string   `json:"tool"                          jsonschema:"Tool name or alias, such as terraform, tf or argocd."`
	From               string   `json:"from"                          jsonschema:"The version you have. The range starts after it."`
	To                 string   `json:"to,omitempty"                  jsonschema:"The version you upgrade to. Default: the latest stable version on the line."`                                   //nolint:lll // struct tag
	Only               []string `json:"only,omitempty"                jsonschema:"List only these kinds: breaking, deprecation, security, feature, fix or other. Counts still cover every kind."` //nolint:lll // struct tag
	MinConfidence      *float64 `json:"min_confidence,omitempty"      jsonschema:"Hide entries below this confidence, 0 to 1. Default: policy.min_confidence of the config (0.5)."`               //nolint:lll // struct tag
	Line               string   `json:"line,omitempty"                jsonschema:"Release line the range stays on: none, major or minor. Default: the tool's registry entry, else none."`         //nolint:lll // struct tag
	IncludePrereleases bool     `json:"include_prereleases,omitempty" jsonschema:"Include prereleases. On when from or to is a prerelease."`                                                      //nolint:lll // struct tag
}

func (t *tools) digest(ctx context.Context, _ *mcp.CallToolRequest, in digestInput) (*mcpkit.Result, error) {
	kinds := make([]classify.Kind, len(in.Only))
	for i, k := range in.Only {
		kinds[i] = classify.Kind(k)
	}
	return t.digestResult(ctx, DigestArgs{
		Tool:               in.Tool,
		From:               in.From,
		To:                 in.To,
		Line:               in.Line,
		IncludePrereleases: in.IncludePrereleases,
		MinConfidence:      in.MinConfidence,
		Kinds:              kinds,
	})
}

type breakingInput struct {
	mcpkit.Paging

	Tool          string   `json:"tool"                     jsonschema:"Tool name or alias, such as terraform, tf or argocd."`
	From          string   `json:"from"                     jsonschema:"The version you have. The range starts after it."`
	To            string   `json:"to,omitempty"             jsonschema:"The version you upgrade to. Default: the latest stable version on the line."`                     //nolint:lll // struct tag
	MinConfidence *float64 `json:"min_confidence,omitempty" jsonschema:"Hide entries below this confidence, 0 to 1. Default: policy.min_confidence of the config (0.5)."` //nolint:lll // struct tag
}

func (t *tools) breakingChanges(ctx context.Context, _ *mcp.CallToolRequest, in breakingInput) (*mcpkit.Result, error) {
	return t.digestResult(ctx, DigestArgs{
		Tool:          in.Tool,
		From:          in.From,
		To:            in.To,
		MinConfidence: in.MinConfidence,
		Kinds:         []classify.Kind{classify.KindBreaking, classify.KindDeprecation},
	})
}

func (t *tools) digestResult(ctx context.Context, a DigestArgs) (*mcpkit.Result, error) {
	d, err := t.o.Backend.Digest(ctx, a)
	if err != nil {
		return nil, err
	}
	data := &report.Digest{Tools: []report.ToolDigest{*d}}
	return &mcpkit.Result{Envelope: t.envelope(data, nil), Pager: digestPager(data)}, nil
}

type outdatedInput struct {
	mcpkit.Paging

	Path string `json:"path,omitempty" jsonschema:"Project directory inside the server's roots. Default: the first root."`
}

func (t *tools) outdated(ctx context.Context, req *mcp.CallToolRequest, in outdatedInput) (*mcpkit.Result, error) {
	roots, err := t.roots(req)
	if err != nil {
		return nil, err
	}
	path := in.Path
	if path == "" {
		path = roots[0]
	}
	dir, err := within(path, roots)
	if err != nil {
		return nil, err
	}
	out, errs, err := t.o.Backend.Outdated(ctx, dir, roots[0])
	if err != nil {
		return nil, err
	}
	return &mcpkit.Result{Envelope: t.envelope(out, errs), Pager: outdatedPager(out)}, nil
}

type driftInput struct {
	mcpkit.Paging

	Roots             []string `json:"roots"                        jsonschema:"Directories to scan for repositories, inside the server's roots."` //nolint:lll // struct tag
	Tools             []string `json:"tools,omitempty"              jsonschema:"Report only these tools, each with a column even when it agrees."`
	Latest            bool     `json:"latest,omitempty"             jsonschema:"Add the newest stable version of each tool. Needs GitHub or the HTTP cache."` //nolint:lll // struct tag
	IgnoreConstraints bool     `json:"ignore_constraints,omitempty" jsonschema:"Compare exact pins only. Constraint pins are listed but don't count."`
}

func (t *tools) drift(ctx context.Context, req *mcp.CallToolRequest, in driftInput) (*mcpkit.Result, error) {
	roots, err := t.roots(req)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, len(in.Roots))
	for i, p := range in.Roots {
		if dirs[i], err = within(p, roots); err != nil {
			return nil, err
		}
	}
	out, errs, err := t.o.Backend.Drift(ctx, DriftArgs{
		Roots:             dirs,
		Tools:             in.Tools,
		Latest:            in.Latest,
		IgnoreConstraints: in.IgnoreConstraints,
		Base:              roots[0],
	})
	if err != nil {
		return nil, err
	}
	return &mcpkit.Result{Envelope: t.envelope(out, errs), Pager: driftPager(out)}, nil
}

type resolveInput struct {
	Tool string `json:"tool" jsonschema:"Tool name or alias (terraform, tf), a NAME_VERSION variable, github:owner/repo or aqua:package."` //nolint:lll // struct tag
}

func (t *tools) resolve(_ context.Context, _ *mcp.CallToolRequest, in resolveInput) (*mcpkit.Result, error) {
	res, err := t.o.Backend.Resolve(in.Tool)
	if err != nil {
		return nil, err
	}
	return &mcpkit.Result{Envelope: t.envelope(res, nil)}, nil
}
