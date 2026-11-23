package enrich

import (
	"cmp"
	"math"
	"strings"

	cvss20 "github.com/pandatix/go-cvss/20"
	cvss30 "github.com/pandatix/go-cvss/30"
	cvss31 "github.com/pandatix/go-cvss/31"
	cvss40 "github.com/pandatix/go-cvss/40"
)

// Score is a record's severity: the highest CVSS base score among its vectors with its rating,
// only the advisory's own rating when it has no vector, or the rating unknown.
type Score struct {
	Score  float64 `json:"score,omitempty"`
	Rating string  `json:"rating"`           // none, low, medium, high, critical or unknown
	Vector string  `json:"vector,omitempty"` // the CVSS vector the score comes from
	From   string  `json:"from,omitempty"`   // the GHSA alias the score comes from
}

// RatingUnknown is the rating of an advisory that neither it nor a GHSA alias rates.
const RatingUnknown = "unknown"

// Unknown reports whether s carries no rating: nil, or the rating unknown.
func (s *Score) Unknown() bool { return s == nil || s.Rating == RatingUnknown }

// CVSS rating bounds from the CVSS v3.1 and v4.0 specifications.
const (
	ratingLow      = 0.1
	ratingMedium   = 4.0
	ratingHigh     = 7.0
	ratingCritical = 9.0
)

// Rating returns the qualitative rating of a CVSS score.
func Rating(score float64) string {
	switch {
	case score >= ratingCritical:
		return "critical"
	case score >= ratingHigh:
		return "high"
	case score >= ratingMedium:
		return "medium"
	case score >= ratingLow:
		return "low"
	default:
		return "none"
	}
}

// vectorScore returns the base score of a CVSS v2, v3.0, v3.1 or v4.0 vector.
func vectorScore(vector string) (float64, bool) {
	switch {
	case strings.HasPrefix(vector, "CVSS:4.0/"):
		c, err := cvss40.ParseVector(vector)
		if err != nil {
			return 0, false
		}
		return c.Score(), true
	case strings.HasPrefix(vector, "CVSS:3.1/"):
		c, err := cvss31.ParseVector(vector)
		if err != nil {
			return 0, false
		}
		return c.BaseScore(), true
	case strings.HasPrefix(vector, "CVSS:3.0/"):
		c, err := cvss30.ParseVector(vector)
		if err != nil {
			return 0, false
		}
		return c.BaseScore(), true
	default:
		c, err := cvss20.ParseVector(vector)
		if err != nil {
			return 0, false
		}
		return c.BaseScore(), true
	}
}

// Score returns the highest score among the record's CVSS vectors for pkg, as osv-scanner
// does, or the advisory's database-specific rating (GitHub's MODERATE, HIGH). It returns nil
// when the record has neither, as Go vulnerability database records don't.
func (r *Record) Score(pkg Package) *Score {
	return r.score(func(a Affected) bool { return a.matches(pkg) })
}

// aliasScore is the score of r as an alias of another record: its own vectors and those of
// every affected entry, since a GHSA record often names other packages than the Go record.
func (r *Record) aliasScore() *Score {
	s := r.score(func(Affected) bool { return true })
	if s != nil {
		s.From = r.ID
	}
	return s
}

func (r *Record) score(match func(Affected) bool) *Score {
	vectors := append([]Severity{}, r.Severity...)
	label := r.DatabaseSpecific.Severity
	for _, a := range r.Affected {
		if match(a) {
			vectors = append(vectors, a.Severity...)
			label = cmp.Or(label, a.DatabaseSpecific.Severity)
		}
	}
	var best *Score
	for _, v := range vectors {
		if !strings.HasPrefix(v.Type, "CVSS_") {
			continue
		}
		score, ok := vectorScore(v.Score)
		if !ok || math.IsNaN(score) {
			continue
		}
		if best == nil || score > best.Score {
			best = &Score{Score: score, Rating: Rating(score), Vector: v.Score}
		}
	}
	if best != nil {
		return best
	}
	if label == "" {
		return nil
	}
	rating := strings.ToLower(label)
	if rating == "moderate" {
		rating = "medium"
	}
	return &Score{Rating: rating}
}
