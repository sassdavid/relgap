package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sassdavid/relgap/internal/classify"
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

// Gate is the set of --fail-on checks outdated, digest and drift run.
type Gate struct {
	Breaking  bool
	Outdated  bool
	Drift     bool
	EOL       bool
	MaxBehind MaxBehind
	Now       time.Time
}

// Finding is one pin that matched a --fail-on value.
type Finding struct {
	FailOn string
	Pin    *OutdatedPin
}

// Check returns the pins that match the gate, and the pins whose lag it couldn't measure. A pin
// matches eol when its current version's cycle has reached its end, outdated or not.
func (g Gate) Check(o *Outdated) ([]Finding, []*OutdatedPin) {
	var (
		findings   []Finding
		unmeasured []*OutdatedPin
	)
	for i := range o.Pins {
		p := &o.Pins[i]
		if g.EOL && p.EOL != nil && p.EOL.Current != nil && p.EOL.Current.IsEOL != nil && *p.EOL.Current.IsEOL {
			findings = append(findings, Finding{FailOn: FailOnEOL, Pin: p})
		}
		if p.Status != StatusOutdated {
			continue
		}
		if g.Outdated {
			exceeded, ok := g.MaxBehind.Exceeded(p.Behind, g.Now)
			if !ok {
				unmeasured = append(unmeasured, p)
			}
			if exceeded {
				findings = append(findings, Finding{FailOn: FailOnOutdated, Pin: p})
			}
		}
		if g.Breaking && p.Counts[classify.KindBreaking] > 0 {
			findings = append(findings, Finding{FailOn: FailOnBreaking, Pin: p})
		}
	}
	return findings, unmeasured
}

// CheckDrift returns the tools that match --fail-on drift.
func (g Gate) CheckDrift(d *Drift) []DriftTool {
	if !g.Drift {
		return nil
	}
	return d.Drifting()
}

// BreakingIn returns the digests that have a breaking entry at or above the minimum confidence.
func BreakingIn(ds []ToolDigest) []ToolDigest {
	var out []ToolDigest
	for _, d := range ds {
		if d.Counts[classify.KindBreaking] > 0 {
			out = append(out, d)
		}
	}
	return out
}
