package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/ghcomment"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/term"
)

const (
	flagFrom         = "from"
	flagNotify       = "notify"
	flagDryRunNotify = "dry-run-notify"
	notifierPR       = "github-pr-comment"
)

// Report kinds a saved envelope can hold.
const (
	kindDigest   = "digest"
	kindOutdated = "outdated"
	kindDrift    = "drift"
	kindAudit    = "audit"
)

// notifyData is the data of notify --format json.
type notifyData struct {
	DryRun  bool                `json:"dry_run"`
	Results []*ghcomment.Result `json:"results"`
}

func newNotifyCmd(d *app.Deps, g *globals) *cobra.Command {
	var (
		from    []string
		only    []string
		dryRun  bool
		choices = []complete.Choice{{Value: notifierPR, Desc: "one comment per report on a GitHub pull request"}}
	)
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Publish saved digest, outdated, drift and audit reports as a GitHub pull request comment",
		Long: `notify reads the JSON that digest, outdated, drift and audit print with --format json and
publishes it. It never collects anything itself.

  github-pr-comment  one comment per report kind on a GitHub pull request, edited in place. The
                     repository comes from GITHUB_REPOSITORY and the pull request from the
                     GITHUB_EVENT_PATH event, as GitHub Actions sets them

The comment starts with a hidden marker that names the report kind and a hash of the comment, so
a re-run edits its own comment and writes nothing when the hash is the same. A report with
nothing to report turns an existing comment into "Nothing to report" and creates none. Markdown
longer than --md-max-chars is cut at a row or section boundary.

A digest has something to report when it covers a tool, an outdated report when a pin is
outdated, end of life, vulnerable or failed, and a drift report when a tool drifts or failed. An
audit report has something to report when it has a finding, an error, or a pin left unchecked
for the reason failed, offline or unresolved. A pin no advisory source covers, such as an image
or a chart, doesn't count.

The token comes from GH_TOKEN, GITHUB_TOKEN or gh, as for digest. It needs pull-requests: write.
--dry-run-notify reads the comments with GET requests and prints what would change.

Exit codes: 0 when every comment was published, 4 when GitHub refused the token, 2 when the
repository or the pull request is unknown, and 1 for any other failure.`,
		Example: `  relgap outdated --format json > outdated.json
  relgap notify --from outdated.json --notify github-pr-comment
  relgap audit --fail-on= --format json > audit.json
  relgap notify --from audit.json --notify github-pr-comment
  relgap notify --from digest.json --from drift.json --notify github-pr-comment --dry-run-notify`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(from) == 0 {
				return usageError(fmt.Errorf("pass at least one --%s file", flagFrom))
			}
			if len(only) == 0 {
				return usageError(fmt.Errorf("pass --%s %s", flagNotify, notifierPR))
			}
			for _, n := range only {
				if n != notifierPR {
					return usageError(fmt.Errorf("unknown notifier %q: want %s", n, notifierPR))
				}
			}
			if g.mdMaxChars < 0 {
				return usageError(fmt.Errorf("--%s must be 0 or more, got %d", flagMDMaxChars, g.mdMaxChars))
			}
			if _, err := g.loadConfig(cmd, d); err != nil {
				return err
			}
			return runNotify(cmd, d, g, from, dryRun)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&from, flagFrom, nil, "digest, outdated, drift or audit JSON file (repeatable)")
	f.StringSliceVar(&only, flagNotify, nil, complete.Usage("notifiers to run: {values}", choices))
	f.BoolVar(&dryRun, flagDryRunNotify, false, "print what would change; read with GET only and write nothing")
	f.IntVar(&g.mdMaxChars, flagMDMaxChars, render.DefaultMDMaxChars,
		"cut the comment to this many characters at a row or section boundary")
	registerCompletions(cmd, map[string]cobra.CompletionFunc{
		flagFrom:       complete.Files("json"),
		flagNotify:     complete.List(choices),
		flagMDMaxChars: complete.NoFiles,
	})
	setFormats(cmd, textFormats()...)
	setExitCodes(cmd, ExitAuth)
	setRelated(cmd, "reference/notify", "guides/renovate-pr")
	return cmd
}

func runNotify(cmd *cobra.Command, d *app.Deps, g *globals, from []string, dryRun bool) error {
	posts := make([]ghcomment.Post, 0, len(from))
	for _, path := range from {
		p, err := readSaved(path)
		if err != nil {
			return usageError(err)
		}
		posts = append(posts, p)
	}
	c, err := newCommentClient(cmd, d, g, dryRun)
	if err != nil {
		return err
	}
	data := notifyData{DryRun: dryRun, Results: []*ghcomment.Result{}}
	env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), &data)
	var failures []error
	for _, p := range posts {
		res, perr := c.Publish(cmd.Context(), p)
		if perr != nil {
			failures = append(failures, perr)
			env.Errors = append(env.Errors, render.Error{Target: notifierPR, Message: perr.Error()})
			d.Logger.ErrorContext(
				cmd.Context(),
				"notifier failed",
				"notifier",
				notifierPR,
				"kind",
				p.Kind,
				"error",
				perr,
			)
			continue
		}
		data.Results = append(data.Results, res)
	}
	if g.outFormat == render.FormatJSON {
		err = render.JSON(d.Stdout, env)
	} else {
		err = writeNotifyText(d.Stdout, &data, env.Errors)
	}
	if err != nil {
		return err
	}
	return notifyExit(failures)
}

func newCommentClient(cmd *cobra.Command, d *app.Deps, g *globals, dryRun bool) (*ghcomment.Client, error) {
	cfg, err := ghcomment.FromEnv(ghcomment.Config{DryRun: dryRun, MaxChars: g.mdMaxChars}, d.Env.Lookup)
	if err != nil {
		return nil, usageError(fmt.Errorf("%s: %w", notifierPR, err))
	}
	cfg.Token, _ = github.Token(cmd.Context(), github.TokenOptions{
		Lookup:      d.Env.Lookup,
		ConfigToken: github.GHConfigToken,
		Runner:      newRunner(d),
	})
	hc := httpx.New(httpx.Options{
		Logger:    d.Logger,
		Transport: d.HTTPTransport,
		UserAgent: fmt.Sprintf("relgap/%s (+https://github.com/sassdavid/relgap)", buildinfo.Get().Version),
	})
	c, err := ghcomment.New(hc, cfg)
	if err != nil {
		return nil, usageError(fmt.Errorf("%s: %w", notifierPR, err))
	}
	return c, nil
}

func notifyExit(failures []error) error {
	if len(failures) == 0 {
		return nil
	}
	err := fmt.Errorf("%d notifier failure(s)", len(failures))
	if !slices.ContainsFunc(failures, func(f error) bool { return !errors.Is(f, ghcomment.ErrAuth) }) {
		return &exitError{code: ExitAuth, err: err}
	}
	return err
}

func writeNotifyText(w io.Writer, data *notifyData, errs []render.Error) error {
	var b strings.Builder
	rows := make([][]string, len(data.Results))
	for i, r := range data.Results {
		rows[i] = []string{notifierPR, r.Target, string(r.Op), r.Kind}
	}
	b.WriteString(term.Table(nil, term.Rows(rows)))
	for _, e := range errs {
		b.WriteString(term.Bad.Paint("error: "+e.Target+": "+e.Message) + "\n")
	}
	if data.DryRun {
		fmt.Fprintf(&b, "\nno changes made (--%s)\n", flagDryRunNotify)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// savedEnvelope is the JSON envelope of a saved report, with the data left raw until its kind
// is known.
type savedEnvelope struct {
	Tool   string          `json:"tool"`
	Data   json.RawMessage `json:"data"`
	Errors []render.Error  `json:"errors"`
}

// readSaved reads a digest, outdated, drift or audit envelope and renders it as a comment.
func readSaved(path string) (ghcomment.Post, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ghcomment.Post{}, fmt.Errorf("read --%s: %w", flagFrom, err)
	}
	var env savedEnvelope
	if err = json.Unmarshal(b, &env); err != nil {
		return ghcomment.Post{}, fmt.Errorf("%s: parse JSON: %w", path, err)
	}
	if env.Tool != config.AppName {
		return ghcomment.Post{}, fmt.Errorf("%s: not a %s report (tool %q)", path, config.AppName, env.Tool)
	}
	kind, err := savedKind(env.Data)
	if err != nil {
		return ghcomment.Post{}, fmt.Errorf("%s: %w", path, err)
	}
	var (
		md   bytes.Buffer
		open bool
	)
	switch kind {
	case kindDigest:
		var data report.Digest
		if err = json.Unmarshal(env.Data, &data); err == nil {
			open = len(data.Tools) > 0 || len(env.Errors) > 0
			err = render.DigestMarkdown(&md, &data, env.Errors, nil)
		}
	case kindOutdated:
		var data report.Outdated
		if err = json.Unmarshal(env.Data, &data); err == nil {
			open = slices.ContainsFunc(data.Pins, outdatedOpen)
			err = render.OutdatedMarkdown(&md, &data)
		}
	case kindDrift:
		var data report.Drift
		if err = json.Unmarshal(env.Data, &data); err == nil {
			restoreDriftErrors(&data, env.Errors)
			open = slices.ContainsFunc(data.Tools, func(t report.DriftTool) bool { return t.Drift || t.Err != nil })
			err = render.DriftMarkdown(&md, &data, render.DriftOptions{})
		}
	case kindAudit:
		var data *report.Audit
		if data, err = report.ParseAudit(env.Data); err == nil {
			open = data.Open() || len(env.Errors) > 0
			err = render.AuditMarkdown(&md, data, env.Errors)
		}
	}
	if err != nil {
		return ghcomment.Post{}, fmt.Errorf("%s: %w", path, err)
	}
	return ghcomment.Post{
		Kind:  kind,
		Title: config.AppName + " " + kind,
		Body:  md.String(),
		Open:  open,
		Hint:  "Run `" + config.AppName + " " + kind + " --format json` for the full report.",
	}, nil
}

// savedKind tells the report kinds apart by their data: audit has findings, drift has repos and
// tools, digest only tools, outdated has pins with a status.
func savedKind(raw json.RawMessage) (string, error) {
	var keys struct {
		Repos    json.RawMessage   `json:"repos"`
		Tools    json.RawMessage   `json:"tools"`
		Pins     []json.RawMessage `json:"pins"`
		Findings json.RawMessage   `json:"findings"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return "", fmt.Errorf("parse data: %w", err)
	}
	switch {
	case keys.Findings != nil:
		return kindAudit, nil
	case keys.Tools != nil && keys.Repos != nil:
		return kindDrift, nil
	case keys.Tools != nil:
		return kindDigest, nil
	case keys.Pins != nil:
		var pin struct {
			Status string `json:"status"`
		}
		if len(keys.Pins) > 0 && (json.Unmarshal(keys.Pins[0], &pin) != nil || pin.Status == "") {
			return "", errors.New("a pins report has nothing to publish; save digest, outdated, drift or audit")
		}
		return kindOutdated, nil
	}
	return "", errors.New("not a digest, outdated, drift or audit report")
}

func outdatedOpen(p report.OutdatedPin) bool {
	return p.Status == report.StatusOutdated || p.Status == report.StatusError || report.IsEOL(&p) ||
		slices.ContainsFunc(p.Vulns, func(v enrich.Vuln) bool { return v.Suppressed == nil })
}

// restoreDriftErrors puts back the per-tool errors that JSON keeps in the envelope's errors.
func restoreDriftErrors(d *report.Drift, errs []render.Error) {
	for i := range d.Tools {
		for _, e := range errs {
			if e.Target == d.Tools[i].Tool {
				d.Tools[i].Err = errors.New(e.Message)
			}
		}
	}
}
