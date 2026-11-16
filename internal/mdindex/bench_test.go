package mdindex_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/version"
)

const benchSize = 1 << 20

// changelog1MB returns benchSize bytes of real kubernetes changelog: CHANGELOG-1.37.md and
// CHANGELOG-1.36.md from the kubectl digest cassette, repeated with a new major version in each
// copy so every version heading stays unique.
func changelog1MB(tb testing.TB) []byte {
	tb.Helper()
	c, err := cassette.Load(filepath.Join("..", "..", "testdata", "cassettes", "digest", "kubectl-1.36.0..1.37.2"))
	require.NoError(tb, err)
	var parts []string
	for _, minor := range []string{"1.37", "1.36"} {
		suffix := "/CHANGELOG-" + minor + ".md?ref=master"
		for _, i := range c.Interactions {
			if strings.HasSuffix(i.Request.URL, suffix) {
				parts = append(parts, i.Response.Body)
				break
			}
		}
	}
	require.Len(tb, parts, 2, "the kubectl cassette holds both changelog files")
	src := strings.Join(parts, "\n")

	var buf bytes.Buffer
	for major := 1; buf.Len() < benchSize; major++ {
		buf.WriteString(strings.ReplaceAll(src, "v1.3", fmt.Sprintf("v%d.3", major)))
		buf.WriteByte('\n')
	}
	out := buf.Bytes()[:benchSize]
	return out[:bytes.LastIndexByte(out, '\n')+1]
}

func BenchmarkIndex1MB(b *testing.B) {
	src := changelog1MB(b)
	heading, err := mdindex.HeadingPattern("^v{version}$")
	require.NoError(b, err)
	o := mdindex.Options{Scheme: version.Semver(), Heading: heading}

	x, err := mdindex.Parse(src, o)
	require.NoError(b, err)
	sections := len(x.Sections())
	require.Greater(b, sections, 20, "the 1 MB changelog has version sections")

	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, perr := mdindex.Parse(src, o); perr != nil {
			b.Fatal(perr)
		}
	}
	// After the loop: its first call resets custom metrics.
	b.ReportMetric(float64(sections), "sections")
}
