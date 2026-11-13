package render_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/source"
)

func TestDigestMarkdown(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	ref := func(u string) source.Ref { return source.Ref{Adapter: source.KindChangelogFile, URL: u} }
	d := &report.Digest{Tools: []report.ToolDigest{
		{
			Tool: "tool",
			Repo: "o/r",
			From: "1.0.0",
			To:   "2.0.0",
			Versions: []report.Version{
				{Version: "1.1.0", Tag: "v1.1.0", PublishedAt: &at, URL: "https://x.test/v1.1.0"},
				{Version: "2.0.0", Tag: "v2.0.0"},
			},
			Entries: []classify.Entry{
				{Version: "1.1.0", Kind: classify.KindFix, Text: "fixed", Source: ref("https://x.test/c#L3")},
				{Version: "1.1.0", Kind: classify.KindSecurity, Text: "CVE-2026-1", Source: ref("https://x.test/c#L5")},
				{
					Version: "2.0.0",
					Kind:    classify.KindBreaking,
					Text:    "removed x\n\n```sh\nx --old\n```",
					Source:  ref("https://x.test/c#L9"),
				},
				{Version: "2.0.0", Kind: classify.KindFeature, Text: "no source"},
			},
			Counts: map[classify.Kind]int{
				"breaking":    1,
				"deprecation": 0,
				"security":    1,
				"feature":     1,
				"fix":         1,
				"other":       0,
			},
			Hidden:   2,
			Warnings: []string{"9.9.9 is newer than every known version"},
		},
	}}
	var buf bytes.Buffer
	require.NoError(t, render.DigestMarkdown(&buf, d))
	assert.Equal(t, `## tool 1.0.0..2.0.0

Repository: [o/r](https://github.com/o/r). 2 versions: [1.1.0](https://x.test/v1.1.0), 2.0.0.

breaking 1, deprecation 0, security 1, feature 1, fix 1, other 0; 2 below --min-confidence hidden

> [!WARNING]
> 9.9.9 is newer than every known version

### Breaking changes

- **2.0.0** removed x

  `+"```sh\n  x --old\n  ```"+`

  ([source](https://x.test/c#L9))

### Security

- **1.1.0** CVE-2026-1 ([source](https://x.test/c#L5))

<details><summary>1.1.0 (2026-05-01)</summary>

#### Fixes

- fixed ([source](https://x.test/c#L3))

</details>

<details><summary>2.0.0</summary>

#### Features

- no source

</details>
`, buf.String())
}

// genDigest builds a digest of 1 to 3 tools with random versions and entries of every kind,
// some of them multi-line with code blocks and multi-byte characters.
func genDigest(r *rand.Rand) *report.Digest {
	texts := []string{"fixed a crash", "añadido `--flag`", "removed x\n\n```sh\nx --old\n```", "変更 of the API", "a | b"}
	d := &report.Digest{}
	for ti := range r.IntN(3) + 1 {
		t := report.ToolDigest{
			Tool:   fmt.Sprintf("tool%d", ti),
			Repo:   "o/r",
			From:   "1.0.0",
			Counts: make(map[classify.Kind]int),
		}
		nv := r.IntN(12) + 1
		for v := range nv {
			t.Versions = append(t.Versions, report.Version{Version: fmt.Sprintf("1.%d.0", v+1)})
		}
		t.To = t.Versions[nv-1].Version
		for range r.IntN(80) {
			k := classify.Kinds()[r.IntN(len(classify.Kinds()))]
			t.Entries = append(t.Entries, classify.Entry{
				Version: t.Versions[r.IntN(nv)].Version, Kind: k, Text: texts[r.IntN(len(texts))],
				Source: source.Ref{URL: "https://x.test/c#L1"},
			})
			t.Counts[k]++
		}
		d.Tools = append(d.Tools, t)
	}
	return d
}

// TestDigestMarkdownTruncation is a property test over generated digests: the cut Markdown never
// exceeds the limit, a digest at the limit is unchanged, one character more gets the footer, and
// the kept part is a prefix of the full digest.
func TestDigestMarkdownTruncation(t *testing.T) {
	t.Parallel()
	for seed := range uint64(300) {
		r := rand.New(rand.NewPCG(seed, 99))
		var buf bytes.Buffer
		require.NoError(t, render.DigestMarkdown(&buf, genDigest(r)))
		md := buf.String()
		n := utf8.RuneCountInString(md)
		for _, limit := range []int{n, n - 1, n / 2, r.IntN(n) + 1, 400} {
			if limit < 400 {
				continue
			}
			got := render.TruncateMarkdown(md, limit, "Run `relgap digest --format json` for the full report.")
			require.LessOrEqual(t, utf8.RuneCountInString(got), limit, "seed %d limit %d", seed, limit)
			if limit >= n {
				require.Equal(t, md, got, "seed %d", seed)
				continue
			}
			i := strings.LastIndex(got, "\n---\n\nTruncated to "+strconv.Itoa(limit)+" characters: cut ")
			require.GreaterOrEqual(t, i, 0, "seed %d limit %d: no footer", seed, limit)
			require.True(t, strings.HasPrefix(md, got[:i]), "seed %d limit %d", seed, limit)
			require.True(t, strings.HasSuffix(got, "for the full report.\n"), "seed %d limit %d", seed, limit)
		}
	}
}
