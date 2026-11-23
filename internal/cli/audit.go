package cli

import (
	"context"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
	"github.com/sassdavid/relgap/internal/scan"
)

type auditFlags struct {
	tools      []string
	submodules bool
}

func newAuditCmd(d *app.Deps, g *globals) *cobra.Command {
	var f auditFlags
	cmd := &cobra.Command{
		Use:   "audit [roots...]",
		Short: "List the known vulnerabilities of every pinned tool version, and the pins OSV can't check",
		Long: `Scans each root for repositories the way 'relgap drift' does and asks OSV about the version
of every pin: one query per tool, version and OSV package, however many repositories pin it.
--tool limits the run to the named tools. Ignore rules in the config drop matching pins.

An exact pin is checked at its own version and needs no lookup. A value marked ≈ is checked at
the version it stands for, the CURRENT of 'relgap outdated': a constraint (~> 1.15) takes the
highest exact pin of the tool in the same repository that satisfies it, otherwise the lowest
version that does; a prefix (26) takes the highest stable version that starts with it; the
latest channel takes the latest version. Only these pins need the tool's versions from GitHub
or another host.

A pin with a curated osv list in its registry entry queries those packages. Other pins query
the package their backend names (npm:, pypi:, cargo:, gem:, go:, dotnet:), or the Go module
and git repository of a github.com tool. A finding is one advisory that affects one tool
version, with every pin of that version, its severity, the lowest fixed version and its
osv.dev page. A record without a rating takes the rating of its GHSA alias, else its severity
is unknown. Findings sort by severity, unknown last, then by ID.

A pin OSV wasn't asked about is listed with the reason, and never counts as clean:
no-advisory-source, private-source, not-exact, unresolved, offline or failed.

--format sarif writes a SARIF 2.1.0 log for GitHub code scanning: a result per finding at each
pin's file and line (one rule per OSV ID), a note per unchecked pin (unchecked), and a result
per file or version lookup that failed (failed). The results don't depend on --fail-on.

--fail-on takes vuln and unchecked. vuln counts the findings at or above --vuln-severity and
those of unknown severity, and unchecked counts the unchecked tool versions. audit gates vuln
by default. Values in config policy.fail_on that audit checks add to vuln, and --fail-on
replaces the whole set: --fail-on unchecked gates only unchecked, and --fail-on= gates
nothing. A value whose count reaches --min-count (default 1) exits 3. Otherwise a file or
lookup that failed exits 1, and so does an OSV outage under --fail-on vuln, since a pass on
partial data proves nothing.

--offline answers from the HTTP cache. Pins without a cached answer are unchecked with reason
offline, and --format json has oldest_answer, when the oldest OSV answer was stored.

Roots default to scan.roots, then the current directory.`,
		Example: `  relgap audit
  relgap audit ~/src/infra ~/src/apps --format md
  relgap audit --tool terraform --tool node
  relgap audit --vuln-severity high --format sarif > audit.sarif
  relgap audit --fail-on vuln --fail-on unchecked
  relgap audit --format json | jq '.data.findings[] | {tool, version, id}'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAudit(cmd, d, g, f, args)
		},
	}
	fl := cmd.Flags()
	fl.StringSliceVar(&f.tools, "tool", nil, "audit only this tool; repeatable")
	fl.BoolVar(&f.submodules, "submodules", false, "scan checked-out git submodules as repositories")
	addGateFlags(cmd, auditValues())
	addOutputFlags(cmd, g, auditFormats()...)
	_ = cmd.RegisterFlagCompletionFunc("tool", completeToolList(d, g))
	cmd.ValidArgsFunction = complete.Dirs()
	setExitCodes(cmd, ExitFindings, ExitAuth)
	setRelated(cmd, "concepts/enrichment", "guides/ci-gate", "guides/sarif", "reference/fail-on",
		"reference/sarif-rules")
	return cmd
}

func runAudit(cmd *cobra.Command, d *app.Deps, g *globals, f auditFlags, args []string) error {
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	gate, err := newAuditGate(cmd, d)
	if err != nil {
		return err
	}
	roots := args
	if len(roots) == 0 {
		roots = d.Config.Config.Scan.Roots
		if len(roots) == 0 {
			roots = []string{"."}
		}
	}
	run, err := s.audit(cmd.Context(), auditArgs{
		roots: roots, tools: f.tools, submodules: f.submodules,
		offline: g.offline,
	})
	if err != nil {
		return err
	}
	if err = g.sink(cmd, d).Write(func(w io.Writer, f render.Format) error {
		return writeAudit(w, f, d, g, run)
	}); err != nil {
		return err
	}
	run.warn(cmd, d)
	return gateExit(cmd, d, gate.Matches(gate.CheckAudit(run.out)), run.partial(gate))
}

// auditArgs says what an audit covers.
type auditArgs struct {
	roots, tools []string
	submodules   bool
	offline      bool   // report when the oldest OSV answer was stored
	base         string // name repositories relative to this directory
}

// auditRun is the outcome of scanning the roots and asking OSV about their pins.
type auditRun struct {
	scan     *scan.Result
	ignore   []scan.Ignore
	tools    []string
	out      *report.Audit
	problems []enrich.Problem
}

// audit scans the roots, finds the version of each pin and asks OSV about every distinct tool
// version. Failed sources become problems, not errors.
func (s *session) audit(ctx context.Context, a auditArgs) (*auditRun, error) {
	d := s.d
	res, err := s.scan(ctx, a.roots, a.submodules, a.base)
	if err != nil {
		return nil, err
	}
	run := &auditRun{
		scan: res, ignore: ignoreRules(s.resolver, d.Config.Config.Ignore),
		tools: toolNames(s.resolver, a.tools),
	}
	selected := func(p *scan.Pin) bool {
		return !scan.Ignored(run.ignore, p) && (len(run.tools) == 0 || slices.Contains(run.tools, p.Tool))
	}
	var pins []scan.Pin
	var unresolved []scan.Pin
	for _, p := range res.Pins {
		switch {
		case !selected(&p):
		case p.Resolved():
			pins = append(pins, p)
		default:
			unresolved = append(unresolved, p)
		}
	}
	gh, hc, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	out, err := report.NewAuditor(s.digester(gh, hc), gh).Audit(ctx, report.AuditRequest{
		Repos:       res.Repos,
		Pins:        pins,
		Resolve:     resolveFunc(ctx, s),
		Concurrency: d.Config.Config.GitHub.Concurrency,
	})
	if err != nil {
		return nil, digestError(err)
	}
	osv := enrich.NewOSVClient(hc, "")
	en := enrich.NewEnricher(d.Logger, nil, osv, d.Config.Config.GitHub.Concurrency)
	results, problems, err := en.Enrich(ctx, out.EnrichRequests(resolveFunc(ctx, s)))
	if err != nil {
		return nil, err //nolint:wrapcheck // only a canceled context
	}
	out.ApplyEnrichment(results, unresolved)
	if age, ok := osv.OldestAge(); a.offline && ok {
		at := d.Clock().Add(-age).UTC().Truncate(time.Second)
		out.OldestAnswer = &at
	}
	run.out, run.problems = out, problems
	return run, nil
}

// auditValues lists the --fail-on values audit checks.
func auditValues() []string { return []string{report.FailOnVuln, report.FailOnUnchecked} }

// newAuditGate builds audit's --fail-on checks. Without the flag, vuln is always on and the
// config's audit values add to it, so a shared policy.fail_on can't turn the vulnerability gate
// off by naming another value.
func newAuditGate(cmd *cobra.Command, d *app.Deps) (report.Gate, error) {
	gate, err := newGate(cmd, d, auditValues())
	if err != nil {
		return gate, err
	}
	if !cmd.Flags().Changed(flagFailOn) && !gate.Has(report.FailOnVuln) {
		gate.Values = append([]string{report.FailOnVuln}, gate.Values...)
	}
	return gate, nil
}

func auditFormats() []render.Format {
	return []render.Format{
		render.FormatTTY,
		render.FormatPlain,
		render.FormatMD,
		render.FormatJSON,
		render.FormatSARIF,
		render.FormatGitHub,
	}
}

func writeAudit(w io.Writer, f render.Format, d *app.Deps, g *globals, r *auditRun) error {
	return reportWriter{
		data:   r.out,
		errors: append(r.errors(), r.sourceErrors()...),
		md:     func(w io.Writer) error { return render.AuditMarkdown(w, r.out) },
		sarif: func() *sarif.Log {
			return render.AuditSARIF(r.out, r.scan.Problems, buildinfo.Get().Version)
		},
		text: func(w io.Writer, color bool) error { return render.AuditText(w, r.out, color) },
	}.write(w, f, d, g)
}

// errors lists the files that didn't parse and the pins whose version lookup failed.
func (r *auditRun) errors() []render.Error {
	out := []render.Error{}
	for _, p := range r.scan.Problems {
		out = append(out, render.Error{Target: path.Join(p.Repo, p.File), Message: p.Message})
	}
	for _, p := range r.out.Failed() {
		out = append(out, render.Error{Target: p.Tool + "@" + render.PinLocation(p.Pin.Location), Message: p.Note})
	}
	return out
}

// sourceErrors lists the OSV requests that failed, as JSON envelope errors.
func (r *auditRun) sourceErrors() []render.Error {
	out := make([]render.Error, 0, len(r.problems))
	for _, p := range r.problems {
		out = append(out, render.Error{Target: p.Source + ": " + p.Tool, Message: p.Err.Error()})
	}
	return out
}

func (r *auditRun) warn(cmd *cobra.Command, d *app.Deps) {
	ctx := cmd.Context()
	for _, p := range r.scan.Problems {
		d.Logger.WarnContext(ctx, "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	for _, p := range r.problems {
		d.Logger.WarnContext(ctx, "OSV failed; those pins are unchecked", "tool", p.Tool, "error", p.Err)
	}
	for _, t := range r.tools {
		if !slices.ContainsFunc(r.scan.Pins, func(p scan.Pin) bool { return p.Tool == t }) {
			d.Logger.WarnContext(ctx, "no repository pins this tool", "tool", t)
		}
	}
	if len(r.tools) > 0 {
		return
	}
	if n := countUnresolved(r.scan.Pins, r.ignore); n > 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: %d %s no known tool; list them with 'relgap pins --unresolved'\n",
			config.AppName, n, plural(n, "pin has", "pins have"))
	}
}

// partial returns why the gate can't pass: a file or version lookup failed, or OSV was down
// under --fail-on vuln.
func (r *auditRun) partial(gate report.Gate) error {
	if errs := r.errors(); len(errs) > 0 {
		return fmt.Errorf("%d %s failed; the report is partial",
			len(errs), plural(len(errs), "pin or file", "pins or files"))
	}
	if !gate.Has(report.FailOnVuln) || len(r.problems) == 0 {
		return nil
	}
	var down []string
	for _, p := range r.problems {
		if !slices.Contains(down, p.Tool) {
			down = append(down, p.Tool)
		}
	}
	return fmt.Errorf("--%s %s can't decide: %s failed for %s; the report is partial",
		flagFailOn, report.FailOnVuln, enrich.SourceOSV, strings.Join(down, ", "))
}
