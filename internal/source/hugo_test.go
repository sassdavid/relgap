package source_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
)

const karpenterGuide = "website/content/en/docs/upgrading/upgrade-guide.md"

func TestHugoLinks(t *testing.T) {
	t.Parallel()
	const site = "https://karpenter.sh/"
	tests := []struct {
		name, md, file, site, dir, want string
	}{
		{
			name: "relative to the page's section",
			md:   `See [Drift]({{<ref "../concepts/disruption#drift" >}}).`,
			want: `See [Drift](https://karpenter.sh/docs/concepts/disruption/#drift).`,
		},
		{
			name: "a section page with a trailing slash",
			md:   `[template]({{<ref "../../docs/reference/cloudformation/">}})`,
			want: `[template](https://karpenter.sh/docs/reference/cloudformation/)`,
		},
		{
			name: "../ with an extension starts at the file's directory",
			md:   `[Migration Guide]({{<ref "../../v1.0/upgrading/v1-migration.md#before-upgrading-to-v110">}})`,
			want: `[Migration Guide](https://karpenter.sh/v1.0/upgrading/v1-migration/#before-upgrading-to-v110)`,
		},
		{
			name: "link text with brackets and code",
			md:   "See [[`spec.kubelet`]]({{< relref \"../concepts/nodeclasses#speckubelet\" >}}).",
			want: "See [[`spec.kubelet`]](https://karpenter.sh/docs/concepts/nodeclasses/#speckubelet).",
		},
		{
			name: "an absolute path, % delimiters and a named path",
			md:   `[a]({{% ref path="/docs/FAQ.md" lang="en" %}}) [b]({{< ref "/" >}})`,
			want: `[a](https://karpenter.sh/docs/faq/) [b](https://karpenter.sh/)`,
		},
		{
			name: "a same-page ref keeps its anchor",
			md:   `[0.32.0]({{<ref "#upgrading-to-0320" >}})`,
			want: `[0.32.0](#upgrading-to-0320)`,
		},
		{
			name: "without site_url a page link keeps its text",
			md:   `See [Drift]({{<ref "../concepts/disruption#drift" >}}) and [above]({{<ref "#crd-upgrades" >}}).`,
			site: "-",
			want: `See Drift and [above](#crd-upgrades).`,
		},
		{
			name: "a leaf bundle resolves from its parent",
			md:   `[x]({{< ref "sibling" >}})`,
			file: "docs/content/guides/setup/index.md",
			want: `[x](https://karpenter.sh/guides/sibling/)`,
		},
		{
			name: "content_dir overrides the language guess",
			md:   `[x]({{< ref "../install" >}})`,
			file: "site/content/go/docs/intro.md",
			dir:  "site/content",
			want: `[x](https://karpenter.sh/go/install/)`,
		},
		{
			name: "block shortcode lines become blank lines",
			md:   "{{% alert title=\"Warning\" color=\"warning\" %}}\nDo not skip 1.1.0.\n{{% /alert %}}\n- item",
			want: "\nDo not skip 1.1.0.\n\n- item",
		},
		{
			name: "other links and inline shortcodes stay",
			md:   "[PR](https://github.com/aws/karpenter-provider-aws/pull/1) {{< figure src=\"x.png\" >}} text",
			want: "[PR](https://github.com/aws/karpenter-provider-aws/pull/1) {{< figure src=\"x.png\" >}} text",
		},
		{
			name: "a file outside a content directory keeps link text only",
			md:   `[x]({{< ref "y" >}})`,
			file: "docs/upgrade.md",
			want: `x`,
		},
	}
	for _, tt := range tests {
		file, s := tt.file, tt.site
		if file == "" {
			file = karpenterGuide
		}
		switch s {
		case "":
			s = site
		case "-":
			s = ""
		}
		assert.Equal(t, tt.want, source.HugoLinks(tt.md, file, s, tt.dir), tt.name)
	}
}

type fakeFiles map[string]string

func (f fakeFiles) File(_ context.Context, _, _, path string) ([]byte, error) {
	return []byte(f[path]), nil
}

// TestSpec_source_adapters_7: Hugo ref and relref link targets in a changelog file become links
// to the site's pages when the source sets site_url, keep their text otherwise, and lines that
// hold only a block shortcode are dropped without moving the line numbers.
func TestSpec_source_adapters_7(t *testing.T) {
	t.Parallel()
	guide := strings.Join([]string{
		"# Upgrade Guide",
		"",
		"### Upgrading to `1.12.0`+",
		"",
		`{{% alert title="Warning" color="warning" %}}`,
		"**Do not** upgrade to `1.1.0`+ without the [Migration Guide]({{<ref \"../../v1.0/upgrading/v1-migration.md#before-upgrading-to-v110\">}}).",
		"{{% /alert %}}",
		"",
		"* **Breaking:** [Node Auto Repair]({{<ref \"../concepts/disruption#node-auto-repair\" >}}) is a voluntary disruption method. See [CRD Upgrades]({{<ref \"#crd-upgrades\" >}}).",
		"",
	}, "\n")
	tool := registry.ToolSpec{
		Name: "karpenter", Repo: "aws/karpenter-provider-aws",
		Version: registry.VersionSpec{Scheme: "semver", TagPrefixes: registry.Prefixes{"v"}},
	}
	src := registry.Source{
		Kind: source.KindChangelogFile, PathTemplate: karpenterGuide, Heading: "^Upgrading to `{version}`\\+$",
	}
	fetch := func(s registry.Source) source.VersionNotes {
		cache := source.NewFileCache(fakeFiles{karpenterGuide: guide})
		notes, err := source.NewChangelogFile(slog.New(slog.DiscardHandler), cache, s).
			Fetch(t.Context(), tool, versions(t, "v1.12.0"))
		require.NoError(t, err)
		require.Len(t, notes, 1)
		return notes[0]
	}

	withSite := src
	withSite.SiteURL = "https://karpenter.sh/"
	n := fetch(withSite)
	assert.NotContains(t, n.Markdown, "{{")
	assert.Contains(t, n.Markdown,
		"[Migration Guide](https://karpenter.sh/v1.0/upgrading/v1-migration/#before-upgrading-to-v110)")
	assert.Contains(
		t,
		n.Markdown,
		"[Node Auto Repair](https://karpenter.sh/docs/concepts/disruption/#node-auto-repair)",
	)
	assert.Contains(t, n.Markdown, "[CRD Upgrades](#crd-upgrades)")

	plain := fetch(src)
	assert.NotContains(t, plain.Markdown, "{{")
	assert.Contains(t, plain.Markdown, "without the Migration Guide.")
	assert.Contains(t, plain.Markdown, "* **Breaking:** Node Auto Repair is a voluntary")
	assert.Equal(t, strings.Count(n.Markdown, "\n"), strings.Count(plain.Markdown, "\n"))
	assert.Equal(t, n.Line, plain.Line)
}
