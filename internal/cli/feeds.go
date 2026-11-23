package cli

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
)

const (
	flagKEV     = "kev"
	flagEPSS    = "epss"
	keyVulnKEV  = "vulns.kev"
	keyVulnEPSS = "vulns.epss"
)

// addFeedFlags adds --kev and --epss, which set vulns.kev and vulns.epss.
func addFeedFlags(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.Bool(flagKEV, false, "mark advisories in the CISA Known Exploited Vulnerabilities catalog, fetched from "+
		"www.cisa.gov (config vulns.kev)")
	fl.Bool(flagEPSS, false, "add each advisory's FIRST EPSS score, asking api.first.org about its CVE IDs "+
		"(config vulns.epss)")
}

// feeds returns the KEV and EPSS clients the config turns on; nil for one that is off.
func (s *session) feeds(hc *http.Client) (*enrich.KEVClient, *enrich.EPSSClient) {
	v := s.d.Config.Config.Vulns
	var (
		kev  *enrich.KEVClient
		epss *enrich.EPSSClient
	)
	if v.KEV {
		kev = enrich.NewKEVClient(hc, "")
	}
	if v.EPSS {
		epss = enrich.NewEPSSClient(hc, "")
	}
	return kev, epss
}

// feedsOn returns which feeds the config turns on.
func feedsOn(d *app.Deps) report.Feeds {
	v := d.Config.Config.Vulns
	return report.Feeds{KEV: v.KEV, EPSS: v.EPSS}
}

// checkKEVGate fails a gate on kev without KEV turned on: the gate could never match, and relgap
// contacts www.cisa.gov only when asked to.
func checkKEVGate(d *app.Deps, gate report.Gate) error {
	if gate.Has(report.FailOnKEV) && !d.Config.Config.Vulns.KEV {
		return usageError(fmt.Errorf("--%s %s needs --%s (config %s)", flagFailOn, report.FailOnKEV, flagKEV,
			keyVulnKEV))
	}
	return nil
}

// feedDown returns the error for a gate on kev when the KEV catalog failed, or nil.
func feedDown(gate report.Gate, problems []enrich.Problem) error {
	if !gate.Has(report.FailOnKEV) {
		return nil
	}
	for _, p := range problems {
		if p.Source == enrich.SourceKEV {
			return fmt.Errorf("--%s %s can't decide: the CISA KEV catalog failed; the report is partial",
				flagFailOn, report.FailOnKEV)
		}
	}
	return nil
}

// problemTarget names what a problem is about in the JSON errors: the source and its tool, or
// the source alone for a feed that failed for every tool.
func problemTarget(p enrich.Problem) string {
	if p.Tool == "" {
		return p.Source
	}
	return p.Source + ": " + p.Tool
}

// warnFeed warns about a KEV or EPSS feed that failed, and reports whether p was one.
func warnFeed(cmd *cobra.Command, d *app.Deps, p enrich.Problem) bool {
	if p.Source != enrich.SourceKEV && p.Source != enrich.SourceEPSS {
		return false
	}
	d.Logger.WarnContext(cmd.Context(), "prioritization feed failed; the report continues without it",
		"source", p.Source, "error", p.Err)
	return true
}
