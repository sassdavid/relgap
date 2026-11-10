package version_test

import (
	"testing"

	"github.com/sassdavid/relgap/internal/version"
)

func seeds() []string {
	return []string{
		"1.17.1", "v3.6.0-rc.1", "go1.20", "go1.20.1", "go1.21rc2", "2.35.3", "21.0.9+10",
		"2026.11.5", "2026.1", "3.13.0a1", "3.12.7rc1", "3.13.0", "1.0.post1.dev2", "jdk-21.0.9+10",
		"1.2.3-alpha.1", "1.2.3-alpha.beta", "1.2.3-1", "1.2.3-01a", "0", "", "v", "1..2", "1.2.3+meta",
	}
}

func allSchemes(t testing.TB) []version.Scheme {
	t.Helper()
	ss := []version.Scheme{version.Semver(), version.Loose(), version.Calver(), version.Python()}
	r, err := version.Regex(`jdk-(\d+\.\d+\.\d+)\+(\d+)`)
	if err != nil {
		t.Fatal(err)
	}
	return append(ss, r)
}

// FuzzParse checks that Parse never panics and that a parsed version equals itself.
func FuzzParse(f *testing.F) {
	for _, s := range seeds() {
		f.Add(s)
	}
	schemes := allSchemes(f)
	f.Fuzz(func(t *testing.T, in string) {
		for _, s := range schemes {
			v, err := s.Parse(in)
			if err != nil {
				continue
			}
			if c := s.Compare(v, v); c != 0 {
				t.Fatalf("%s: Compare(%q, itself) = %d", s.Name(), in, c)
			}
			_ = s.IsPrerelease(v)
			_ = s.Line(v, version.LineMinor)
		}
	})
}

// FuzzCompareProperties checks that Compare is antisymmetric and transitive on parsed values.
func FuzzCompareProperties(f *testing.F) {
	ss := seeds()
	for i := range ss {
		f.Add(ss[i], ss[(i+1)%len(ss)], ss[(i+2)%len(ss)])
	}
	schemes := allSchemes(f)
	f.Fuzz(func(t *testing.T, a, b, c string) {
		for _, s := range schemes {
			va, errA := s.Parse(a)
			vb, errB := s.Parse(b)
			vc, errC := s.Parse(c)
			if errA != nil || errB != nil || errC != nil {
				continue
			}
			ab, ba := s.Compare(va, vb), s.Compare(vb, va)
			if ab != -ba {
				t.Fatalf("%s: not antisymmetric: Compare(%q, %q) = %d, reverse = %d", s.Name(), a, b, ab, ba)
			}
			bc, ac := s.Compare(vb, vc), s.Compare(va, vc)
			if ab <= 0 && bc <= 0 && ac > 0 {
				t.Fatalf("%s: not transitive: %q <= %q <= %q but %q > %q", s.Name(), a, b, c, a, c)
			}
			if ab == 0 && bc == 0 && ac != 0 {
				t.Fatalf(
					"%s: equality not transitive: %q = %q = %q but Compare(%q, %q) = %d",
					s.Name(),
					a,
					b,
					c,
					a,
					c,
					ac,
				)
			}
		}
	})
}
