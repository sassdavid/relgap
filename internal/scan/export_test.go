package scan

// MatchingDetectors returns the names of the repository detectors that read rel.
func MatchingDetectors(rel string) []string {
	var out []string
	for _, d := range (&Options{}).detectors() {
		if d.match(rel) {
			out = append(out, d.name)
		}
	}
	return out
}
