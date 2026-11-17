package classify

// RuleConfidences returns the confidence of every built-in rule and of the fixed results in
// Classify, so a test can check that [Signals] lists each one.
func RuleConfidences() []float64 {
	out := []float64{
		ConfidenceOther, confidenceMarker, confidenceHeading, confidenceBang, confidenceCommit,
		confidenceHighlights,
	}
	for _, rs := range [][]rule{headingRules(), verbRules(), keywordRules()} {
		for _, r := range rs {
			out = append(out, r.confidence)
		}
	}
	return out
}
