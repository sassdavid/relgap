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

// ExtractPrefix exposes extractPrefix to the tests.
func ExtractPrefix(extract string) (string, bool) { return extractPrefix(extract) }

// ExtractAffixes exposes extractAffixes to the tests.
func ExtractAffixes(extract string) (string, string, bool) { return extractAffixes(extract) }

// GitRemote exposes gitRemote to the tests.
func GitRemote(addr string) (string, string, bool) { return gitRemote(addr) }

// CommentVersion exposes commentVersion to the tests.
func CommentVersion(comment string) string { return commentVersion(comment) }
