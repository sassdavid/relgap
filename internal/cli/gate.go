package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
)

const (
	flagFailOn       = "fail-on"
	flagMinCount     = "min-count"
	flagMaxBehind    = "max-behind"
	flagVulnSeverity = "vuln-severity"
)

// outdatedValues lists the --fail-on values outdated checks, in [report.Values] order.
func outdatedValues() []string {
	return []string{
		report.FailOnBreaking, report.FailOnDeprecation, report.FailOnEOL, report.FailOnVuln, report.FailOnOutdated,
	}
}

// digestValues lists the --fail-on values digest checks.
func digestValues() []string { return []string{report.FailOnBreaking, report.FailOnDeprecation} }

// driftValues lists the --fail-on values drift checks.
func driftValues() []string { return []string{report.FailOnDrift} }

// addGateFlags adds --fail-on and --min-count, plus --max-behind and --vuln-severity when the
// command checks outdated and vuln.
func addGateFlags(cmd *cobra.Command, values []string) {
	def := config.Default().Policy
	fl := cmd.Flags()
	fl.StringSlice(flagFailOn, def.FailOn,
		"exit 3 when this matches: "+orList(values)+"; repeatable (config policy.fail_on)")
	fl.Int(flagMinCount, 1, "findings a --fail-on value needs before it exits 3")
	if slices.Contains(values, report.FailOnOutdated) {
		fl.String(
			flagMaxBehind,
			def.MaxBehind,
			"how far behind --fail-on outdated allows, such as minor:2, releases:10 or days:90 (config policy.max_behind)",
		)
	}
	if slices.Contains(values, report.FailOnVuln) {
		fl.String(
			flagVulnSeverity,
			def.VulnSeverity,
			"lowest rating --fail-on vuln counts: low, medium, high or critical; empty counts all (config policy.vuln_severity)",
		)
	}
}

// newGate builds the --fail-on checks of a command that checks values. A value from the config
// that the command doesn't check is left to the command that does; the same value given as a
// flag is a usage error, since a CI gate that silently passes is worse than one that fails to
// start.
func newGate(cmd *cobra.Command, d *app.Deps, values []string) (report.Gate, error) {
	p := d.Config.Config.Policy
	gate := report.Gate{Now: d.Clock()}
	var err error
	if gate.MinCount, err = cmd.Flags().GetInt(flagMinCount); err != nil {
		return gate, fmt.Errorf("read --%s: %w", flagMinCount, err)
	}
	if gate.MinCount < 1 {
		return gate, usageError(fmt.Errorf("--%s must be at least 1, got %d", flagMinCount, gate.MinCount))
	}
	if gate.MaxBehind, err = report.ParseMaxBehind(p.MaxBehind); err != nil {
		return gate, usageError(err)
	}
	if gate.VulnSeverity, err = report.ParseVulnSeverity(p.VulnSeverity); err != nil {
		return gate, usageError(err)
	}
	explicit := cmd.Flags().Changed(flagFailOn)
	for _, v := range p.FailOn {
		v = strings.TrimSpace(v)
		switch {
		case !slices.Contains(report.Values(), v):
			return gate, usageError(fmt.Errorf("invalid --%s value %q: want %s", flagFailOn, v, orList(values)))
		case slices.Contains(values, v):
			if !gate.Has(v) {
				gate.Values = append(gate.Values, v)
			}
		case explicit:
			return gate, usageError(fmt.Errorf("--%s %s isn't checked by %s: relgap %s checks %s",
				flagFailOn, v, cmd.Name(), cmd.Name(), orList(values)))
		}
	}
	slices.SortFunc(gate.Values, func(a, b string) int {
		return slices.Index(report.Values(), a) - slices.Index(report.Values(), b)
	})
	return gate, nil
}

// gateExit returns exit 3 when a --fail-on value matched, else partial, which is nil or the error
// for a report that is missing something. A failure can only hide findings, never add one, so a
// match stands on partial data, while a pass on partial data exits 1.
func gateExit(cmd *cobra.Command, d *app.Deps, matches []report.Match, partial error) error {
	if len(matches) == 0 {
		return partial
	}
	if partial != nil {
		d.Logger.WarnContext(cmd.Context(), "the report is partial; --fail-on matched anyway", "error", partial)
	}
	parts := make([]string, len(matches))
	for i, m := range matches {
		parts[i] = matchText(m)
	}
	return &exitError{code: ExitFindings, err: errors.New(strings.Join(parts, "; "))}
}

// matchText says which value matched, how many findings it counted and what they are.
func matchText(m report.Match) string {
	n := len(m.Findings)
	var unit string
	var items []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			items = append(items, s)
		}
	}
	for _, f := range m.Findings {
		switch {
		case f.Vuln != nil:
			unit = plural(n, "advisory", "advisories")
			add(
				fmt.Sprintf(
					"%s in %s %s (%s)",
					f.Vuln.ID,
					f.Pin.Tool,
					f.Pin.Current,
					render.PinLocation(f.Pin.Pin.Location),
				),
			)
		case f.Pin != nil:
			unit = plural(n, "pin", "pins")
			add(fmt.Sprintf("%s %s (%s)", f.Pin.Tool, f.Pin.Current, render.PinLocation(f.Pin.Pin.Location)))
		case f.Digest != nil:
			unit = plural(n, "digest", "digests")
			add(fmt.Sprintf("%s %s..%s", f.Digest.Tool, f.Digest.From, f.Digest.To))
		case f.Drift != nil:
			unit = plural(n, "drifted pin", "drifted pins")
			vs := make([]string, len(f.Drift.Versions))
			for j, v := range f.Drift.Versions {
				vs[j] = v.Version
			}
			add(fmt.Sprintf("%s (%s)", f.Drift.Tool, strings.Join(vs, ", ")))
		}
	}
	return fmt.Sprintf(
		"--%s %s matched %s %s: %s",
		flagFailOn,
		m.FailOn,
		strconv.Itoa(n),
		unit,
		strings.Join(items, ", "),
	)
}

// orList joins values as "a, b or c".
func orList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	default:
		return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
	}
}
