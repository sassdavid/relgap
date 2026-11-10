package render_test

import (
	"bytes"
	"testing"
	"time"

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
