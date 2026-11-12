package report

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
)

// FailOn values, as listed in the config schema.
const (
	FailOnBreaking    = "breaking"
	FailOnDeprecation = "deprecation"
	FailOnEOL         = "eol"
	FailOnVuln        = "vuln"
	FailOnDrift       = "drift"
	FailOnOutdated    = "outdated"
)

// MaxBehind is a --max-behind threshold such as minor:2. The zero value allows nothing: any
// pin behind latest exceeds it.
type MaxBehind struct {
	Unit string // major, minor, patch, releases or days; empty means any lag
	N    int
}

// String returns the threshold as written.
func (m MaxBehind) String() string {
	if m.Unit == "" {
		return ""
	}
	return m.Unit + ":" + strconv.Itoa(m.N)
}

// ParseMaxBehind parses "<unit>:<n>"; the empty string is the zero threshold.
func ParseMaxBehind(s string) (MaxBehind, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return MaxBehind{}, nil
	}
	unit, num, ok := strings.Cut(s, ":")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 0 {
		return MaxBehind{}, fmt.Errorf("invalid --max-behind %q: want <unit>:<n>, such as minor:2", s)
	}
	switch unit {
	case "major", "minor", "patch", "releases", "days":
	default:
		return MaxBehind{}, fmt.Errorf("invalid --max-behind unit %q: want major, minor, patch, releases or days", unit)
	}
	return MaxBehind{Unit: unit, N: n}, nil
}

// Exceeded reports whether b is further behind than m allows. The second result is false when b
// can't tell, as with days for a tool whose versions are tags without publish dates.
func (m MaxBehind) Exceeded(b *Behind, now time.Time) (bool, bool) {
	if b == nil || b.Releases == 0 {
		return false, true
	}
	switch m.Unit {
	case "major":
		return b.Major > m.N, true
	case "minor":
		return b.Minor > m.N, true
	case "patch":
		return b.Patch > m.N, true
	case "releases":
		return b.Releases > m.N, true
	case "days":
		if b.Since == nil {
			return false, false
		}
		return now.Sub(*b.Since) > time.Duration(m.N)*24*time.Hour, true
	default:
		return true, true
	}
}

// Values returns every --fail-on value in the order of the config schema.
func Values() []string {
	return []string{FailOnBreaking, FailOnDeprecation, FailOnEOL, FailOnVuln, FailOnDrift, FailOnOutdated}
}

// ParseVulnSeverity validates a --vuln-severity value. The empty string means any severity.
func ParseVulnSeverity(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || severityRank(s) > 0 {
		return s, nil
	}
	return "", fmt.Errorf("invalid --vuln-severity %q: want low, medium, high or critical", s)
}

// severityRank orders the CVSS ratings; none and unknown ratings are 0.
func severityRank(rating string) int {
	return slices.Index([]string{"low", "medium", "high", "critical"}, rating) + 1
}

// Gate is the set of --fail-on checks a command runs, with their thresholds.
type Gate struct {
	Values       []string // the --fail-on values to check, in [Values] order
	MaxBehind    MaxBehind
	VulnSeverity string // the lowest rating --fail-on vuln counts; empty counts every advisory
	MinCount     int    // findings a value needs before it matches; below 1 counts as 1
	Now          time.Time
}

// Has reports whether the gate checks value.
func (g Gate) Has(value string) bool { return slices.Contains(g.Values, value) }

// Finding is one thing a --fail-on value counts: a pin, an advisory of a pin, a digest or a
// drifted pin.
type Finding struct {
	FailOn string
	Pin    *OutdatedPin
	Vuln   *enrich.Vuln // the advisory, for vuln
	Digest *ToolDigest  // for breaking and deprecation in a single digest
	Drift  *DriftTool   // the tool of a drifted pin
}

// Match is a --fail-on value whose findings reached the minimum count.
type Match struct {
	FailOn   string
	Findings []Finding
}

// Check returns the findings of the gate's values among the pins, and the outdated pins whose lag
// it couldn't measure. A pin counts for eol when its current version's cycle has reached its end,
// and for vuln once per advisory that affects its current version at or above VulnSeverity. It
// counts for outdated, breaking and deprecation only when it is behind latest.
func (g Gate) Check(o *Outdated) ([]Finding, []*OutdatedPin) {
	var (
		findings   []Finding
		unmeasured []*OutdatedPin
	)
	for i := range o.Pins {
		p := &o.Pins[i]
		if g.Has(FailOnEOL) && IsEOL(p) {
			findings = append(findings, Finding{FailOn: FailOnEOL, Pin: p})
		}
		if g.Has(FailOnVuln) {
			for j := range p.Vulns {
				if v := &p.Vulns[j]; g.countsVuln(v) {
					findings = append(findings, Finding{FailOn: FailOnVuln, Pin: p, Vuln: v})
				}
			}
		}
		if p.Status != StatusOutdated {
			continue
		}
		more, measured := g.checkOutdated(p)
		findings = append(findings, more...)
		if !measured {
			unmeasured = append(unmeasured, p)
		}
	}
	return findings, unmeasured
}

// checkOutdated returns the findings of a pin behind latest for outdated, breaking and
// deprecation, and whether --max-behind could measure its lag.
func (g Gate) checkOutdated(p *OutdatedPin) ([]Finding, bool) {
	var findings []Finding
	measured := true
	if g.Has(FailOnOutdated) {
		var exceeded bool
		exceeded, measured = g.MaxBehind.Exceeded(p.Behind, g.Now)
		if exceeded {
			findings = append(findings, Finding{FailOn: FailOnOutdated, Pin: p})
		}
	}
	for _, k := range []classify.Kind{classify.KindBreaking, classify.KindDeprecation} {
		if g.Has(string(k)) && p.Counts[k] > 0 {
			findings = append(findings, Finding{FailOn: string(k), Pin: p})
		}
	}
	return findings, measured
}

// IsEOL reports whether the pin's current version is on a release cycle that has reached its end.
func IsEOL(p *OutdatedPin) bool {
	return p.EOL != nil && p.EOL.Current != nil && p.EOL.Current.IsEOL != nil && *p.EOL.Current.IsEOL
}

func (g Gate) countsVuln(v *enrich.Vuln) bool {
	if !v.AffectsCurrent {
		return false
	}
	if g.VulnSeverity == "" {
		return true
	}
	return v.Severity != nil && severityRank(v.Severity.Rating) >= severityRank(g.VulnSeverity)
}

// CheckDigests returns a finding for each digest with a breaking or deprecation entry, for the
// values the gate checks. Entries below the minimum confidence aren't in the counts.
func (g Gate) CheckDigests(ds []ToolDigest) []Finding {
	var findings []Finding
	for i := range ds {
		for _, k := range []classify.Kind{classify.KindBreaking, classify.KindDeprecation} {
			if g.Has(string(k)) && ds[i].Counts[k] > 0 {
				findings = append(findings, Finding{FailOn: string(k), Digest: &ds[i]})
			}
		}
	}
	return findings
}

// CheckDrift returns a finding for each drifted pin: a compared pin whose version differs from its
// tool's reference version.
func (g Gate) CheckDrift(d *Drift) []Finding {
	if !g.Has(FailOnDrift) {
		return nil
	}
	var findings []Finding
	for i := range d.Tools {
		t := &d.Tools[i]
		for range t.Outliers() {
			findings = append(findings, Finding{FailOn: FailOnDrift, Drift: t})
		}
	}
	return findings
}

// Matches groups findings by value, in [Values] order, and keeps the values with at least
// MinCount findings.
func (g Gate) Matches(findings []Finding) []Match {
	var out []Match
	for _, v := range Values() {
		var m Match
		for _, f := range findings {
			if f.FailOn == v {
				m.Findings = append(m.Findings, f)
			}
		}
		if len(m.Findings) > 0 && len(m.Findings) >= max(g.MinCount, 1) {
			m.FailOn = v
			out = append(out, m)
		}
	}
	return out
}
