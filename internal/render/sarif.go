package render

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/sarif"
	"github.com/sassdavid/relgap/internal/scan"
)

// InfoURI is the tool's home page in SARIF logs.
const InfoURI = "https://github.com/sassdavid/relgap"

// SARIF rule IDs. A vulnerability's rule ID is its OSV ID.
const (
	RuleBreaking    = "breaking"
	RuleDeprecation = "deprecation"
	RuleEOL         = "eol"
	RuleDrift       = "drift"
	RuleOutdated    = "outdated"
	RuleFailed      = "failed"
)

// fingerprintKey names relgap's partial fingerprint. GitHub ignores it and hashes the pin's line
// itself; other SARIF consumers can match results across runs with it.
const fingerprintKey = "relgapFinding/v1"

// fixedRule returns the rule with id, one of the Rule constants.
func fixedRule(id string) sarif.Rule {
	return map[string]sarif.Rule{
		RuleBreaking: {
			ID:               RuleBreaking,
			ShortDescription: sarif.Message{Text: "The upgrade to the latest version has breaking changes"},
			FullDescription: sarif.Message{Text: "The release notes from the pinned version to the latest version " +
				"have entries classified as breaking at or above --min-confidence."},
			Help:                 sarif.Message{Text: "Read the digest the message names before you upgrade the pin."},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelNote},
		},
		RuleDeprecation: {
			ID:               RuleDeprecation,
			ShortDescription: sarif.Message{Text: "The upgrade to the latest version has deprecations"},
			FullDescription: sarif.Message{Text: "The release notes from the pinned version to the latest version " +
				"have entries classified as deprecations at or above --min-confidence."},
			Help: sarif.Message{
				Text: "Read the digest the message names and move off the deprecated features before they go away.",
			},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelNote},
		},
		RuleEOL: {
			ID:               RuleEOL,
			ShortDescription: sarif.Message{Text: "The pinned release cycle has reached its end of life"},
			FullDescription: sarif.Message{
				Text: "endoflife.date lists the release cycle of the pinned version as end of life, so it gets no more fixes.",
			},
			Help:                 sarif.Message{Text: "Upgrade the pin to a supported release cycle."},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelError},
		},
		RuleDrift: {
			ID:               RuleDrift,
			ShortDescription: sarif.Message{Text: "The pin differs from the version most repositories pin"},
			FullDescription: sarif.Message{Text: "The scanned repositories pin more than one version of the tool, " +
				"and this pin stands for a version other than the one most of them pin."},
			Help: sarif.Message{
				Text: "Pin the version most repositories use, or add an ignore rule for this path in .relgap.yaml.",
			},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelWarning},
		},
		RuleOutdated: {
			ID:               RuleOutdated,
			ShortDescription: sarif.Message{Text: "The pin is behind the latest version"},
			FullDescription: sarif.Message{
				Text: "A newer stable version is out on the pin's release line.",
			},
			Help:                 sarif.Message{Text: "Upgrade the pin; relgap digest lists what changed in between."},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelWarning},
		},
		RuleFailed: {
			ID:                   RuleFailed,
			ShortDescription:     sarif.Message{Text: "relgap couldn't check this pin or file"},
			FullDescription:      sarif.Message{Text: "A file didn't parse, or looking up the tool's versions failed."},
			Help:                 sarif.Message{Text: "Fix the file, or run relgap again with -v to see the failing request."},
			DefaultConfiguration: sarif.Configuration{Level: sarif.LevelError},
		},
	}[id]
}

// sarifRun builds one run, adding each rule on first use in a fixed order.
type sarifRun struct {
	log   *sarif.Log
	index map[string]int
}

func newSARIFRun(version string, rules ...string) *sarifRun {
	r := &sarifRun{log: sarif.New(ToolName, version, InfoURI), index: map[string]int{}}
	for _, id := range rules {
		r.addRule(fixedRule(id))
	}
	return r
}

func (r *sarifRun) addRule(rule sarif.Rule) {
	if _, ok := r.index[rule.ID]; ok {
		return
	}
	run := &r.log.Runs[0]
	r.index[rule.ID] = len(run.Tool.Driver.Rules)
	run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, rule)
}

// add appends a result for rule at loc. subject tells results of the same rule, tool and file
// apart in the fingerprint.
func (r *sarifRun) add(rule, level, msg string, loc scan.Location, tool, subject string) {
	uri := sarif.FileURI(path.Join(loc.Repo, loc.File))
	pl := sarif.PhysicalLocation{ArtifactLocation: sarif.ArtifactLocation{URI: uri}}
	if loc.Line > 0 {
		pl.Region = &sarif.Region{StartLine: loc.Line}
	}
	run := &r.log.Runs[0]
	run.Results = append(run.Results, sarif.Result{
		RuleID:              rule,
		RuleIndex:           r.index[rule],
		Level:               level,
		Message:             sarif.Message{Text: msg},
		Locations:           []sarif.Location{{PhysicalLocation: pl}},
		PartialFingerprints: map[string]string{fingerprintKey: strings.Join([]string{rule, tool, uri, subject}, "|")},
	})
}

// OutdatedSARIF returns a SARIF log with a result per pin behind latest, per pin on an EOL cycle,
// per pin whose upgrade has breaking changes or deprecations, per advisory that affects a pin,
// and per pin or file that failed. Results don't depend on --fail-on. Rules are in --fail-on
// order, then failed, then one rule per OSV ID.
func OutdatedSARIF(o *report.Outdated, problems []scan.Problem, version string) *sarif.Log {
	r := newSARIFRun(version, RuleBreaking, RuleDeprecation, RuleEOL, RuleOutdated, RuleFailed)
	for _, rule := range vulnRules(o) {
		r.addRule(rule)
	}
	for i := range o.Pins {
		r.pin(&o.Pins[i])
	}
	for _, pr := range problems {
		r.add(RuleFailed, sarif.LevelError, "can't read pins: "+pr.Message,
			scan.Location{Repo: pr.Repo, File: pr.File}, "", "")
	}
	return r.log
}

// pin adds the results of one pin, in the order of the rules.
func (r *sarifRun) pin(p *report.OutdatedPin) {
	loc := p.Pin.Location
	if p.Status == report.StatusError {
		r.add(RuleFailed, sarif.LevelError, fmt.Sprintf("%s %s: %s", p.Tool, p.Pin.Value, p.Note), loc, p.Tool, "")
		return
	}
	outdated := p.Status == report.StatusOutdated
	if outdated {
		rng := p.Current + ".." + p.Latest
		for _, k := range []classify.Kind{classify.KindBreaking, classify.KindDeprecation} {
			if n := p.Counts[k]; n > 0 {
				r.add(string(k), sarif.LevelNote, fmt.Sprintf("%s %s has %d %s; see relgap digest %s %s",
					p.Tool, rng, n, kindNoun(k, n), p.Tool, rng), loc, p.Tool, rng)
			}
		}
	}
	if report.IsEOL(p) {
		r.add(RuleEOL, sarif.LevelError, eolMessage(p), loc, p.Tool, p.EOL.Current.Cycle)
	}
	for j := range p.Vulns {
		if v := &p.Vulns[j]; v.AffectsCurrent {
			r.add(v.ID, vulnLevel(v), vulnMessage(p, v), loc, p.Tool, v.ID)
		}
	}
	if outdated {
		r.add(RuleOutdated, sarif.LevelWarning, outdatedMessage(p), loc, p.Tool, p.Current)
	}
}

// vulnRules returns a rule per advisory that affects a pin, sorted by ID.
func vulnRules(o *report.Outdated) []sarif.Rule {
	byID := map[string]*enrich.Vuln{}
	for i := range o.Pins {
		for j := range o.Pins[i].Vulns {
			if v := &o.Pins[i].Vulns[j]; v.AffectsCurrent && byID[v.ID] == nil {
				byID[v.ID] = v
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	rules := make([]sarif.Rule, 0, len(ids))
	for _, id := range ids {
		v := byID[id]
		short := v.Summary
		if short == "" {
			short = "Known vulnerability " + id
		}
		full := short
		if len(v.Aliases) > 0 {
			full += " (" + strings.Join(v.Aliases, ", ") + ")"
		}
		props := map[string]any{"tags": []string{"security", "vulnerability"}}
		if v.Severity != nil && v.Severity.Score > 0 {
			props["security-severity"] = strconv.FormatFloat(v.Severity.Score, 'f', 1, 64)
		}
		rules = append(rules, sarif.Rule{
			ID:               id,
			ShortDescription: sarif.Message{Text: short},
			FullDescription:  sarif.Message{Text: full},
			Help: sarif.Message{
				Text: "Upgrade past the affected versions. Details: https://osv.dev/vulnerability/" + id,
			},
			DefaultConfiguration: sarif.Configuration{Level: vulnLevel(v)},
			Properties:           props,
		})
	}
	return rules
}

// vulnLevel maps an advisory's CVSS rating to a level: error for high and critical, warning for
// medium, note for low and for advisories without a rating.
func vulnLevel(v *enrich.Vuln) string {
	if v.Severity == nil {
		return sarif.LevelNote
	}
	switch v.Severity.Rating {
	case "critical", "high":
		return sarif.LevelError
	case "medium":
		return sarif.LevelWarning
	default:
		return sarif.LevelNote
	}
}

func vulnMessage(p *report.OutdatedPin, v *enrich.Vuln) string {
	msg := fmt.Sprintf("%s %s is affected by %s", p.Tool, p.Current, v.ID)
	if len(v.Aliases) > 0 {
		msg += " (" + strings.Join(v.Aliases, ", ") + ")"
	}
	if v.Severity != nil && v.Severity.Rating != "" && v.Severity.Rating != "none" {
		msg += ", " + v.Severity.Rating
	}
	if v.Summary != "" {
		msg += ": " + v.Summary
	}
	if v.FixedIn != "" {
		msg += "; fixed in " + v.FixedIn
	}
	return msg
}

func eolMessage(p *report.OutdatedPin) string {
	c := p.EOL.Current
	msg := fmt.Sprintf("%s %s is on release cycle %s, which reached its end of life", p.Tool, p.Current, c.Cycle)
	if c.EOLFrom != "" {
		msg += " on " + c.EOLFrom
	}
	if p.EOL.Target != nil && p.EOL.Target.Cycle != "" {
		msg += fmt.Sprintf("; %s %s is on cycle %s", p.Tool, p.Latest, p.EOL.Target.Cycle)
	}
	return msg
}

func outdatedMessage(p *report.OutdatedPin) string {
	msg := fmt.Sprintf("%s %s is behind %s", p.Tool, p.Current, p.Latest)
	if b := p.Behind; b != nil {
		msg += fmt.Sprintf(" by %d %s (%d major, %d minor, %d patch)", b.Releases,
			pinsWord(b.Releases, "release", "releases"), b.Major, b.Minor, b.Patch)
	}
	return msg
}

func kindNoun(k classify.Kind, n int) string {
	if k == classify.KindBreaking {
		return pinsWord(n, "breaking change", "breaking changes")
	}
	return pinsWord(n, "deprecation", "deprecations")
}

// DriftSARIF returns a SARIF log with a result per drifted pin (a compared pin whose version
// differs from its tool's reference version), per file that didn't parse and per tool whose
// latest version couldn't be listed.
func DriftSARIF(d *report.Drift, problems []scan.Problem, version string) *sarif.Log {
	r := newSARIFRun(version, RuleDrift, RuleFailed)
	for i := range d.Tools {
		t := &d.Tools[i]
		refRepos := 0
		for _, v := range t.Versions {
			if v.Version == t.Reference {
				refRepos = len(v.Repos)
			}
		}
		for _, p := range t.Outliers() {
			msg := fmt.Sprintf("%s %s differs from %s, which %d of %d %s pin", t.Tool, p.Version, t.Reference,
				refRepos, len(d.Repos), pinsWord(len(d.Repos), "repository", "repositories"))
			if p.Via != "" {
				msg += fmt.Sprintf(" (%s counts as %s: %s)", p.Value, p.Version, p.Via)
			}
			r.add(RuleDrift, sarif.LevelWarning, msg, p.Location, t.Tool, p.Version)
		}
	}
	for _, pr := range problems {
		r.add(RuleFailed, sarif.LevelError, "can't read pins: "+pr.Message,
			scan.Location{Repo: pr.Repo, File: pr.File}, "", "")
	}
	for i := range d.Tools {
		if t := &d.Tools[i]; t.Err != nil {
			run := &r.log.Runs[0]
			run.Results = append(run.Results, sarif.Result{
				RuleID:              RuleFailed,
				RuleIndex:           r.index[RuleFailed],
				Level:               sarif.LevelError,
				Message:             sarif.Message{Text: t.Tool + ": can't list the latest version: " + t.Err.Error()},
				Locations:           []sarif.Location{},
				PartialFingerprints: map[string]string{fingerprintKey: RuleFailed + "|" + t.Tool + "||latest"},
			})
		}
	}
	return r.log
}
