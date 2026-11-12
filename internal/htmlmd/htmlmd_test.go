package htmlmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/htmlmd"
)

// page mirrors the shapes of go.dev and docs.python.org: a site shell around the content,
// permalink anchors in headings, and point releases as paragraphs with ids.
const page = `<!doctype html><html><head><style>p{}</style></head><body>
<header class="Site-header"><a href="/">Go</a></header>
<nav class="menu"><a href="/doc">Docs</a></nav>
<main id="main-content"><div class="body" role="main">
<h1>Release notes<a class="headerlink" href="#top">¶</a></h1>
<!-- go.dev/issue/1 -->
<h2 id="go1.27.0">go1.27.0 (released 2026-08-19)</h2>
<p>Go 1.27 adds <a href="../ref/spec#Methods">generic methods</a>.</p>
<h3 id="go1.27.minor">Minor revisions</h3>
<p id="go1.27.3">go1.27.3 includes fixes to <code>crypto/x509</code>.</p>
<pre><code class="language-go">x := 1</code></pre>
<h2 id="go1.26.0">go1.26.0</h2>
<p>Older.</p>
<script>alert(1)</script>
</div></main>
<footer class="Site-footer">Footer</footer>
</body></html>`

const pageURL = "https://go.dev/doc/devel/release"

func TestMarkdown(t *testing.T) {
	t.Parallel()
	d, err := htmlmd.Parse([]byte(page), pageURL)
	require.NoError(t, err)
	tests := []struct {
		name string
		opts htmlmd.Options
		want string
	}{
		{
			name: "main by default, shell and scripts dropped",
			opts: htmlmd.Options{Strip: []string{"a.headerlink"}},
			want: "# Release notes\n\n## go1.27.0 (released 2026-08-19)\n\n" +
				"Go 1.27 adds [generic methods](https://go.dev/doc/ref/spec#Methods).\n\n" +
				"### Minor revisions\n\ngo1.27.3 includes fixes to `crypto/x509`.\n\n" +
				"```go\nx := 1\n```\n\n## go1.26.0\n\nOlder.\n",
		},
		{
			name: "first match of a tag",
			opts: htmlmd.Options{Content: "h3"},
			want: "### Minor revisions\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, mdErr := d.Markdown(t.Context(), tt.opts)
			require.NoError(t, mdErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMarkdownMissingContent(t *testing.T) {
	t.Parallel()
	d, err := htmlmd.Parse([]byte(page), pageURL)
	require.NoError(t, err)
	_, err = d.Markdown(t.Context(), htmlmd.Options{Content: "div.missing"})
	require.ErrorContains(t, err, `no element matches "div.missing"`)
}

func TestAnchor(t *testing.T) {
	t.Parallel()
	d, err := htmlmd.Parse([]byte(page), pageURL)
	require.NoError(t, err)
	tests := []struct {
		id   string
		want string
		ok   bool
	}{
		{id: "go1.27.3", want: "go1.27.3 includes fixes to `crypto/x509`.\n", ok: true},
		{
			id: "go1.27.0",
			want: "## go1.27.0 (released 2026-08-19)\n\n" +
				"Go 1.27 adds [generic methods](https://go.dev/doc/ref/spec#Methods).\n\n" +
				"### Minor revisions\n\ngo1.27.3 includes fixes to `crypto/x509`.\n\n```go\nx := 1\n```\n",
			ok: true,
		},
		{id: "go1.99.0"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			got, ok, anchorErr := d.Anchor(t.Context(), tt.id, htmlmd.Options{})
			require.NoError(t, anchorErr)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSelector(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "div.body"},
		{in: "a.headerlink"},
		{in: "#main-content"},
		{in: "main"},
		{in: "div.a.b#x"},
		{in: "", wantErr: true},
		{in: "div .body", wantErr: true},
		{in: "div > p", wantErr: true},
		{in: "#a#b", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			_, err := htmlmd.ParseSelector(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, htmlmd.ErrSelector)
				return
			}
			require.NoError(t, err)
		})
	}
}
