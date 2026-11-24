package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/buildinfo"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/vex"
)

const flagBinaries = "binaries"

const outdatedLong = `Scans the project at path (default: the current directory) the way 'relgap pins' does and
compares each pin with the newest stable version on the tool's release line.

CURRENT is the version a pin stands for. An exact pin is itself. A value marked ≈ allows
more than one version, and the notes say which one relgap used: a constraint (~> 1.15) takes
the highest exact pin of the tool in the same repository that satisfies it, otherwise the
lowest version that does; a prefix (26, prefix:1.2) takes the highest stable version that
starts with it; the latest channel is the latest version. Other channels (lts) aren't
compared.

BEHIND, BREAKING and DEPRECATION come from the digest of CURRENT..LATEST, the same digest
'relgap digest <tool> <current>..' prints. Entries below --min-confidence aren't counted.

relgap finds the latest versions with GraphQL queries of 10 to 25 tools, sent in parallel.
Only pins that are behind, and constraint and prefix pins, need a tool's full release list;
constraint and prefix pins start loading it at the same time.

tfprovider, tfmodule and chart pins are looked up in the Terraform registry and Artifact Hub,
which name the repository. A chart pin with a repository URL (a Chart.yaml repository or a
Renovate comment's registryUrl) uses the Artifact Hub repository at that URL, so a chart name
that several repositories share still resolves. bitbucket: and gitlab: pins read the tags of
their repository on that host. Image pins compare with the tags of their registry, and their
notes come from the GitHub repository the image's source label names. Action pins
(scan.detectors.ci_actions) compare with the releases of the action's repository; a pin by
commit SHA counts as the version its line comment names (# v4.1.3). Pins of tools without a
repository relgap can read and images on a registry relgap doesn't read are skipped;
--format json lists them with the reason.

EOL shows whether CURRENT's release cycle has reached its end on endoflife.date (yes, no, or
? when endoflife.date doesn't list the cycle), for tools whose registry entry names a
product. --format json also has the state of LATEST's cycle. VULNS counts the OSV
vulnerabilities of CURRENT, and how many of them upgrading to LATEST fixes; 0 means OSV has
none. A pin OSV wasn't asked about shows ?, and the list after the notes gives the reason:
no-advisory-source, private-source, not-exact, unresolved, offline or failed. --format json
lists them in unchecked. Both sources are cached for a day. When one is down, relgap warns
and prints the rest.

A pin with a vulnerability gets a fix target, listed under "Fix targets" and as fix in
--format json: the lowest stable release above CURRENT that every advisory with a fix has
left behind, checked against OSV, and the same within CURRENT's line. An advisory the latest
release still has is one no release fixes, and relgap warns when the latest release has any.

Config vulns.ignore and OpenVEX documents in vulns.vex or --vex suppress advisories the way
'relgap audit' says: VULNS and --fail-on vuln leave them out, and the "Suppressed" list and
suppressed in --format json give the reason.

--kev (config vulns.kev) marks the advisories whose CVE is in the CISA Known Exploited
Vulnerabilities catalog, which relgap fetches from www.cisa.gov once a day: VULNS counts them
as "1 KEV", and --format json and SARIF have kev with the date CISA added it. --epss (config
vulns.epss) adds each advisory's FIRST EPSS score, the probability of exploitation in the next
30 days, from api.first.org, to --format json and SARIF. Advisories in the catalog sort first,
then by severity and EPSS score. When a feed is down, relgap warns and prints the rest.

--binaries also reads the Go build info of each pinned tool's installed executables (on
PATH and in mise's installs directory) and lists the OSV vulnerabilities of the Go version
and main module they were built with, suppressed and marked the same way. Like govulncheck's
binary mode, minus the check that the binary calls the vulnerable code.

--format sarif writes a SARIF 2.1.0 log for GitHub code scanning, located at each pin's
file and line: a result per pin behind latest (outdated), per pin on an EOL cycle (eol), per
upgrade with breaking changes or deprecations (breaking, deprecation), per advisory that
affects a pin (one rule per OSV ID), per pin or file that failed (failed), and per pin not
checked for vulnerabilities (unchecked, a note). The results don't depend on --fail-on.

--fail-on takes breaking, deprecation, eol, vuln, outdated and kev. Each value counts findings:
outdated counts the pins further behind than --max-behind (major:N, minor:N, patch:N,
releases:N or days:N; empty means any pin behind latest), breaking and deprecation count
the pins whose digest has such an entry, eol counts the pins on an EOL cycle, and vuln
counts the advisories that affect a pin at or above --vuln-severity or of unknown severity,
and with --vuln-fixable only those with a fixed version above CURRENT. kev counts the
advisories in the KEV catalog, whatever their severity, and needs --kev. A value whose count
reaches --min-count (default 1) exits 3. Otherwise a tool or file that failed exits 1, and so
does an endoflife.date outage under --fail-on eol, an OSV outage under --fail-on vuln or a KEV
outage under --fail-on kev, since a pass on partial data proves nothing.`

func newOutdatedCmd(d *app.Deps, g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outdated [path]",
		Short: "Compare every pinned tool version in a project with the latest release",
		Long:  outdatedLong,
		Example: `  relgap outdated
  relgap outdated ~/src/infra --format md
  relgap outdated --fail-on outdated --max-behind minor:2
  relgap outdated --fail-on eol --fail-on vuln --vuln-severity high
  relgap outdated --kev --epss --fail-on kev
  relgap outdated --fail-on outdated --min-count 5 --format sarif > relgap.sarif
  relgap outdated --binaries --format json | jq '.data.binaries[]'
  relgap outdated --format json | jq '.data.pins[] | select(.status == "outdated")'`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return runOutdated(cmd, d, g, root)
		},
	}
	addGateFlags(cmd, outdatedValues())
	addFeedFlags(cmd)
	addVEXFlag(cmd)
	addOutputFlags(cmd, g, outdatedFormats()...)
	cmd.Flags().Bool(flagBinaries, false,
		"also check the Go version and main module of the pinned tools' installed executables against OSV")
	cmd.ValidArgsFunction = complete.Files()
	setExitCodes(cmd, ExitFindings, ExitAuth)
	setRelated(cmd, "guides/outdated", "guides/ci-gate", "guides/sarif", "concepts/enrichment",
		"reference/fail-on", "reference/sarif-rules")
	return cmd
}

func runOutdated(cmd *cobra.Command, d *app.Deps, g *globals, root string) error {
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(cmd.Context())
	gate, err := newGate(cmd, d, outdatedValues())
	if err != nil {
		return err
	}
	if err = checkKEVGate(d, gate); err != nil {
		return err
	}
	binaries, err := cmd.Flags().GetBool(flagBinaries)
	if err != nil {
		return fmt.Errorf("read --%s: %w", flagBinaries, err)
	}
	run, err := s.compare(cmd.Context(), root, nil, compareOptions{enrich: true, binaries: binaries})
	if err != nil {
		return err
	}
	if err = g.sink(cmd, d).Write(func(w io.Writer, f render.Format) error {
		return writeOutdated(w, f, d, g, run)
	}); err != nil {
		return err
	}
	run.warn(cmd, d)
	return run.exit(cmd, d, gate)
}

// comparison is the outcome of scanning a project and comparing its pins.
type comparison struct {
	scan     *scan.Result
	ignore   []scan.Ignore
	out      *report.Outdated
	problems []enrich.Problem
	digester *report.Digester // lists versions for the fix targets, reusing the comparison's lists
	rules    *vex.Rules       // the vulns.ignore entries and VEX statements; nil without enrichment
}

// compareOptions says what a comparison adds to the pins.
type compareOptions struct {
	enrich   bool   // EOL and OSV for each compared pin
	binaries bool   // the Go build info of installed executables
	base     string // name repositories relative to this directory
}

// compare scans root, keeps the resolved pins that keep accepts (all when nil) and compares
// them with the latest versions.
func (s *session) compare(
	ctx context.Context,
	root string,
	keep func(scan.Pin) bool,
	with compareOptions,
) (*comparison, error) {
	cfg := s.d.Config
	res, err := s.scan(ctx, []string{root}, false, with.base)
	if err != nil {
		return nil, err
	}
	ignore := ignoreRules(s.resolver, cfg.Config.Ignore)
	var pins []scan.Pin
	for _, p := range res.Pins {
		if p.Resolved() && !scan.Ignored(ignore, &p) && (keep == nil || keep(p)) {
			pins = append(pins, p)
		}
	}
	refs := make([]*scan.Pin, len(pins))
	for i := range pins {
		refs[i] = &pins[i]
	}
	s.chartRegistries(ctx, refs)
	gh, hc, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	dg := s.digester(gh, hc)
	out, err := report.NewOutdater(dg, gh).
		Outdated(ctx, report.OutdatedRequest{
			Repos:         res.Repos,
			Pins:          pins,
			Resolve:       resolveFunc(ctx, s),
			MinConfidence: cfg.Config.Policy.MinConfidence,
			Concurrency:   cfg.Config.GitHub.Concurrency,
		})
	if err != nil {
		return nil, digestError(err)
	}
	run := &comparison{scan: res, ignore: ignore, out: out, digester: dg}
	if with.enrich {
		if err = s.enrich(ctx, run, hc, pins, with.binaries); err != nil {
			return nil, err
		}
	}
	return run, nil
}

// enrich adds EOL states and vulnerabilities to the compared pins and, with binaries, the
// vulnerabilities of installed executables. Failed sources become problems, not errors.
func (s *session) enrich(ctx context.Context, c *comparison, hc *http.Client, pins []scan.Pin, binaries bool) error {
	d := s.d
	rules, err := s.suppression()
	if err != nil {
		return err
	}
	c.rules = rules
	gh, _, err := s.clients(ctx)
	if err != nil {
		return err
	}
	en := enrich.NewEnricher(d.Logger, enrich.NewEOLClient(hc, ""), enrich.NewOSVClient(hc, ""),
		d.Config.Config.GitHub.Concurrency)
	en.SetSuppressor(rules)
	en.SetPrivacy(s.privacy(ctx, gh))
	en.SetFeeds(s.feeds(hc))
	results, problems, err := en.Enrich(ctx, c.out.EnrichRequests(resolveFunc(ctx, s), c.digester.Universe))
	if err != nil {
		return err //nolint:wrapcheck // only a canceled context
	}
	var unresolved []scan.Pin
	for _, p := range c.scan.Pins {
		if !p.Resolved() && !scan.Ignored(c.ignore, &p) {
			unresolved = append(unresolved, p)
		}
	}
	c.out.ApplyEnrichment(results, unresolved)
	c.problems = problems
	if !binaries {
		return nil
	}
	return s.binaries(ctx, c, en, pins)
}

// binaries adds the installed Go executables of the pinned tools and their vulnerabilities.
func (s *session) binaries(ctx context.Context, c *comparison, en *enrich.Enricher, pins []scan.Pin) error {
	d := s.d
	finder := enrich.BinaryFinder{
		Lookup: d.Env.Lookup,
		Which:  misex.New(s.runner).Which,
	}
	seen := map[string]bool{}
	c.out.Binaries = []*enrich.Binary{}
	for _, p := range pins {
		if seen[p.Tool] {
			continue
		}
		seen[p.Tool] = true
		spec, resolveErr := resolveFunc(ctx, s)(p.Tool)
		if resolveErr != nil {
			continue
		}
		for _, path := range finder.Find(ctx, spec) {
			b, readErr := enrich.ReadBinary(spec, path)
			if readErr != nil {
				d.Logger.DebugContext(ctx, "skipping an executable", "tool", spec.Name, "error", readErr)
				continue
			}
			c.out.Binaries = append(c.out.Binaries, b)
		}
	}
	feedProblems, err := en.BinaryVulns(ctx, c.out.Binaries)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() //nolint:wrapcheck // the caller matches context errors
		}
		c.problems = append(c.problems, enrich.Problem{Source: enrich.SourceOSV, Tool: "binaries", Err: err})
	}
	for _, p := range feedProblems {
		// The pins' enrichment already reported a feed that failed for the whole run.
		if !slices.ContainsFunc(c.problems, func(q enrich.Problem) bool { return q.Source == p.Source }) {
			c.problems = append(c.problems, p)
		}
	}
	return nil
}

// resolveFunc turns a pin's tool name into its spec through the session, which looks up
// tfprovider:, tfmodule: and chart: pins that no registry knows.
func resolveFunc(ctx context.Context, s *session) report.ResolveFunc {
	return func(tool string) (registry.ToolSpec, error) {
		r, err := s.resolve(ctx, tool)
		if err != nil {
			return registry.ToolSpec{}, err
		}
		return r.Tool, nil
	}
}

// ignoreRules turns the config's ignore rules into scan rules, naming each tool the way pins do,
// so an alias such as tf matches terraform pins.
func ignoreRules(resolver *registry.Resolver, rules []config.IgnoreRule) []scan.Ignore {
	out := make([]scan.Ignore, 0, len(rules))
	for _, r := range rules {
		tool := r.Tool
		if name, ok := resolver.Identify(tool); ok {
			tool = name
		}
		out = append(out, scan.Ignore{Tool: tool, Name: r.Name, Path: r.Path})
	}
	return out
}

func outdatedFormats() []render.Format {
	return []render.Format{
		render.FormatTTY,
		render.FormatPlain,
		render.FormatMD,
		render.FormatJSON,
		render.FormatSARIF,
		render.FormatGitHub,
	}
}

func writeOutdated(w io.Writer, f render.Format, d *app.Deps, g *globals, c *comparison) error {
	return reportWriter{
		data:   c.out,
		errors: append(c.errors(), c.sourceErrors()...),
		md:     func(w io.Writer) error { return render.OutdatedMarkdown(w, c.out) },
		sarif: func() *sarif.Log {
			return render.OutdatedSARIF(c.out, c.scan.Problems, buildinfo.Get().Version)
		},
		text: func(w io.Writer, width int) error { return render.OutdatedText(w, c.out, width) },
	}.write(w, f, d, g)
}

// reportWriter writes one report in each format of outdated and audit.
type reportWriter struct {
	data   any            // the JSON envelope's data
	errors []render.Error // the JSON envelope's errors
	md     func(io.Writer) error
	sarif  func() *sarif.Log
	text   func(w io.Writer, width int) error // tty and plain, wrapped at width
	vex    func(io.Writer) error              // openvex; nil for a command without it
}

func (r reportWriter) write(w io.Writer, f render.Format, d *app.Deps, g *globals) error {
	switch f {
	case render.FormatJSON:
		env := render.NewEnvelope(buildinfo.Get().Version, d.Clock(), r.data)
		env.Errors = r.errors
		return render.JSON(w, env)
	case render.FormatMD:
		return r.md(w)
	case render.FormatSARIF:
		return sarif.Write(w, r.sarif())
	case render.FormatGitHub:
		return sarif.WriteGitHub(w, r.sarif())
	case render.FormatOpenVEX:
		if r.vex != nil {
			return r.vex(w)
		}
	case render.FormatAuto, render.FormatTTY, render.FormatPlain, render.FormatCSV:
	}
	return r.text(w, g.textWidth(f))
}

// errors lists the files that didn't parse and the pins GitHub failed for.
func (c *comparison) errors() []render.Error {
	out := []render.Error{}
	for _, p := range c.scan.Problems {
		out = append(out, render.Error{Target: path.Join(p.Repo, p.File), Message: p.Message})
	}
	for _, p := range c.out.Pins {
		if p.Status == report.StatusError {
			out = append(out, render.Error{Target: p.Tool + "@" + render.PinLocation(p.Pin.Location), Message: p.Note})
		}
	}
	return out
}

// sourceErrors lists the enrichment sources that failed, as JSON envelope errors.
func (c *comparison) sourceErrors() []render.Error {
	out := make([]render.Error, 0, len(c.problems))
	for _, p := range c.problems {
		out = append(out, render.Error{Target: problemTarget(p), Message: p.Err.Error()})
	}
	return out
}

func (c *comparison) warn(cmd *cobra.Command, d *app.Deps) {
	for _, p := range c.scan.Problems {
		d.Logger.WarnContext(cmd.Context(), "can't read pins", "repo", p.Repo, "file", p.File, "error", p.Message)
	}
	for _, p := range c.problems {
		if !warnFeed(cmd, d, p) {
			d.Logger.WarnContext(cmd.Context(), "enrichment source failed; the report continues without it",
				"source", p.Source, "tool", p.Tool, "error", p.Err)
		}
	}
	warnLatestAffected(cmd.Context(), d, c.out.Fixes())
	warnSuppression(cmd.Context(), d, c.rules, true)
	if n := countUnresolved(c.scan.Pins, c.ignore); n > 0 {
		d.Logger.WarnContext(
			cmd.Context(),
			"pins with no known tool; list them with 'relgap pins --unresolved'",
			"count",
			n,
		)
	}
}

// exit turns gate findings and errors into the exit code; see [gateExit].
func (c *comparison) exit(cmd *cobra.Command, d *app.Deps, gate report.Gate) error {
	findings, unmeasured := gate.Check(c.out)
	for _, p := range unmeasured {
		d.Logger.WarnContext(cmd.Context(), "can't measure days behind: the tool's versions have no publish dates",
			"tool", p.Tool, "pin", render.PinLocation(p.Pin.Location))
	}
	return gateExit(cmd, d, gate.Matches(findings), c.partial(gate))
}

// partial returns why the gate can't pass: a pin or file failed, or a source that a --fail-on
// value needs was down.
func (c *comparison) partial(gate report.Gate) error {
	if errs := c.errors(); len(errs) > 0 {
		return fmt.Errorf(
			"%d %s failed; the report is partial",
			len(errs),
			plural(len(errs), "pin or file", "pins or files"),
		)
	}
	for _, need := range []struct{ value, source string }{
		{report.FailOnEOL, enrich.SourceEOL},
		{report.FailOnVuln, enrich.SourceOSV},
	} {
		if !gate.Has(need.value) {
			continue
		}
		var down []string
		for _, p := range c.problems {
			if p.Source == need.source && !slices.Contains(down, p.Tool) {
				down = append(down, p.Tool)
			}
		}
		if len(down) > 0 {
			return fmt.Errorf("--%s %s can't decide: %s failed for %s; the report is partial",
				flagFailOn, need.value, need.source, strings.Join(down, ", "))
		}
	}
	return feedDown(gate, c.problems)
}
