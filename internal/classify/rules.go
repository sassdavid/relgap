package classify

import (
	"fmt"
	"regexp"
	"strings"
)

// Confidence of each rule priority (classification spec).
const (
	ConfidenceOther      = 0.3  // no rule matched
	confidenceMarker     = 0.95 // breaking marker or breaking/security heading
	confidenceHeading    = 0.9  // other kind headings
	confidenceBang       = 0.9  // feat!: or a BREAKING CHANGE: footer
	confidenceCommit     = 0.8  // conventional-commit type
	confidenceVerb       = 0.7  // leading verb
	confidenceHighlights = 0.6  // under a "Notable changes" heading
	confidenceKeyword    = 0.4  // keyword anywhere
)

// Signal is one kind of evidence the classifier reads and the confidence an entry gets from it.
type Signal struct {
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
}

// Signals returns the evidence the classifier reads, highest confidence first.
func Signals() []Signal {
	return []Signal{
		{confidenceMarker, "A breaking marker (the entry's breaking_markers or SEMVER-MAJOR), or a heading " +
			"about breaking changes, upgrade notes, incompatibilities or security"},
		{confidenceHeading, "A heading for another kind: deprecations, removals, features, fixes, or " +
			"dependencies and other maintenance"},
		{confidenceBang, "A conventional commit with ! (such as feat!:) or a BREAKING CHANGE: footer"},
		{confidenceCommit, "A conventional commit type (feat:, fix:, security: and the rest), a (SEMVER-MINOR) " +
			"tag, or a node SEMVER-MAJOR commit whose scopes are all internal"},
		{confidenceVerb, "The first word: Added, Fixed, Deprecated, Removed or Dropped with an interface, " +
			"or a Breaking: or Security: prefix"},
		{confidenceHighlights, "Under a Notable changes or Highlights heading, with no other rule matching"},
		{confidenceJump, "A version that starts a new major line (a new minor line in 0.x) and has no breaking " +
			"entry of its own gets one breaking entry that says so"},
		{confidenceKeyword, "A keyword anywhere in the text: CVE-, deprecat, no longer, removed, must now, " +
			"renamed or dropped support"},
		{ConfidenceOther, "No rule matched, or the entry is a lead-in line ending in a colon; the kind is other"},
	}
}

type rule struct {
	re         *regexp.Regexp
	kind       Kind
	confidence float64
}

// headingRules are tried on each heading, deepest first; the first match wins.
func headingRules() []rule {
	return []rule{
		{
			regexp.MustCompile(
				`(?i)\bbreaking\b|\bupgrade (notes|guide|instructions)\b|^upgrading\b|\bcompatibility notes\b|` +
					`\x{26A0}|\bsemver-major\b|\bincompatib`,
			),
			KindBreaking,
			confidenceMarker,
		},
		{regexp.MustCompile(`(?i)\bdeprecat(ed|ions?)\b`), KindDeprecation, confidenceHeading},
		{regexp.MustCompile(`(?i)\bremov(ed|als?)\b`), KindBreaking, confidenceHeading},
		{regexp.MustCompile(`(?i)\bsecurity\b|\bCVE-\d`), KindSecurity, confidenceMarker},
		{
			regexp.MustCompile(`(?i)\bdependenc(y|ies)\b|\bdeps\b|\bother\b|\bmisc(ellaneous)?\b|\bchores?\b|` +
				`\bdocumentation\b|\binternal\b|\bmaintenance\b|\bfailing tests?\b|\bapi[ -]changes?\b`),
			KindOther, confidenceHeading,
		},
		{
			regexp.MustCompile(`(?i)\bfeatures?\b|\benhancements?\b|\bimprovements?\b|\badded\b|\bnew\b`),
			KindFeature, confidenceHeading,
		},
		{regexp.MustCompile(`(?i)\bbug ?fix(es)?\b|\bfixe[sd]\b|\bbugs?\b`), KindFix, confidenceHeading},
	}
}

var (
	commitFooter = regexp.MustCompile(`(?m)^\s*BREAKING[ -]CHANGES?:`)
	commitPrefix = regexp.MustCompile(`(?i)^(?:\[[^\]]*\]\s*)?` +
		`(feat|feature|fix|bugfix|perf|refactor|docs?|chore|build|ci|tests?|style|revert|deps|security)` +
		`(?:\([^)]*\))?(!)?:\s`)
	semverMinor = regexp.MustCompile(`\(SEMVER-MINOR\)`)
	highlights  = regexp.MustCompile(`(?i)\bnotable\b|\bhighlights?\b`)
	// future announces a later change; under a breaking heading it is a deprecation.
	future = regexp.MustCompile(`(?i)\bwill be the last\b|\bwill be (removed|dropped)\b|` +
		`\bin a (future|later|upcoming) (release|version)\b|\bscheduled for removal\b`)
	// semverMajorScopes reads node's commit line: "**(SEMVER-MAJOR)** **deps,test**: ...".
	semverMajorScopes = regexp.MustCompile(`\(SEMVER-MAJOR\)\**\s*\*\*([\w ,/-]+)\*\*:`)
	leadIn            = regexp.MustCompile(`^[^\n]{1,120}:$`)
)

// internalScopes are node commit scopes users never see.
func internalScopes() []string {
	return []string{"deps", "test", "tools", "build", "src", "doc", "meta", "benchmark"}
}

// verbRules match a leading verb, optionally after a short scope such as "Windows: " or
// "**cli**: ".
func verbRules() []rule {
	const prefix = `^(?:\*{0,2}[A-Za-z][\w./-]*(?: [A-Za-z][\w./-]*)?\*{0,2}:\s+)?`
	return []rule{
		// A bold label keeps its colon inside: "**Breaking:** text".
		{regexp.MustCompile(prefix + `(?i:\*{0,2}breaking(?: changes?)?:)`), KindBreaking, confidenceVerb},
		// Only a removed interface is breaking; "Removed a flicker" or a commit title "Remove x"
		// falls through to the keyword rule.
		{
			regexp.MustCompile(prefix + `(?:Removed|Dropped) (?:the |deprecated |the deprecated )*` +
				"(?:`" + `|--|support\b|[\w./-]+ (?:flag|option|setting|command|subcommand|field|key|variable|` +
				`parameter|argument|endpoint|API|config)s?\b)`),
			KindBreaking, confidenceVerb,
		},
		{
			regexp.MustCompile(prefix + `(?:Deprecated|Deprecates|Deprecate)\s|^\*{0,2}(?i:deprecations?:)`),
			KindDeprecation,
			confidenceVerb,
		},
		{regexp.MustCompile(prefix + `(?i:\*{0,2}security:)`), KindSecurity, confidenceVerb},
		{regexp.MustCompile(prefix + `(?:Added|Adds|Add|Introduced|Introduces)\s`), KindFeature, confidenceVerb},
		{regexp.MustCompile(prefix + `(?:Fixed|Fixes|Fix)\s`), KindFix, confidenceVerb},
	}
}

// keywordRules match a keyword anywhere in the entry.
func keywordRules() []rule {
	return []rule{
		{regexp.MustCompile(`\bCVE-\d+`), KindSecurity, confidenceKeyword},
		{regexp.MustCompile(`(?i)\bdeprecat`), KindDeprecation, confidenceKeyword},
		{
			regexp.MustCompile(`(?i)\bno longer\b|\bremoved\b|\bmust now\b|\brenamed\b|\bdropped support\b`),
			KindBreaking, confidenceKeyword,
		},
	}
}

// Options adds a tool's registry rules to the built-in ones.
type Options struct {
	BreakingMarkers []string // regular expressions that make an entry breaking
	IgnoreSections  []string // headings, or "A › B" paths, whose entries are dropped
}

func compileMarkers(exprs []string) ([]*regexp.Regexp, error) {
	out := []*regexp.Regexp{regexp.MustCompile(`\bSEMVER-MAJOR\b`)}
	for _, e := range exprs {
		re, err := regexp.Compile(e)
		if err != nil {
			return nil, fmt.Errorf("compile breaking marker %q: %w", e, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// ignoreRule is one ignore_sections item split into lowercased heading parts.
type ignoreRule []string

// boilerplateSections are release-page sections that never hold release notes.
func boilerplateSections() []string {
	return []string{
		"Downloads", "Download", "Installation", "Install", "Checksums", "Assets", "Binaries",
		"New Contributors", "Contributors",
	}
}

func compileIgnore(items []string) []ignoreRule {
	items = append(boilerplateSections(), items...)
	out := make([]ignoreRule, 0, len(items))
	for _, it := range items {
		var parts ignoreRule
		for p := range strings.SplitSeq(strings.ReplaceAll(it, ">", "›"), "›") {
			if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			out = append(out, parts)
		}
	}
	return out
}

// matches reports whether the parts appear as consecutive headings of path. A heading matches a
// part when it equals it or starts with it and a space ("Downloads for v1.37.0").
func (r ignoreRule) matches(path []string) bool {
	for i := 0; i+len(r) <= len(path); i++ {
		if r.at(path[i:]) {
			return true
		}
	}
	return false
}

func (r ignoreRule) at(path []string) bool {
	for j, part := range r {
		h := strings.ToLower(strings.TrimSpace(strings.Trim(path[j], "*_:")))
		if h != part && !strings.HasPrefix(h, part+" ") {
			return false
		}
	}
	return true
}
