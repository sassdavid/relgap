package classify_test

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/internal/version"
)

const testdata = "../../testdata"

// fencedComments returns the "# ..." lines inside code fences of md, without the "# ".
func fencedComments(md string) []string {
	var out []string
	inFence := false
	for line := range strings.SplitSeq(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence && strings.HasPrefix(trimmed, "# ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")))
		}
	}
	return out
}

// TestSpec_classification_3: terragrunt release bodies hold hcl fences with comment lines such as
// "# terragrunt.stack.hcl". None of them becomes a heading.
func TestSpec_classification_3(t *testing.T) {
	t.Parallel()
	rt := vcr.ForTest(t, filepath.Join(testdata, "cassettes", "classify", "terragrunt"), ghtest.Scrubber())
	hc := httpx.New(httpx.Options{Logger: slog.New(slog.DiscardHandler), Transport: rt})
	gh, err := github.New(hc, github.Options{Token: ghtest.Token(), Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	var vs []version.Version
	for _, tag := range []string{"v1.1.0", "v1.1.3"} {
		v, parseErr := version.Semver().Parse(strings.TrimPrefix(tag, "v"))
		require.NoError(t, parseErr)
		vs = append(vs, version.Version{V: v, Tag: tag})
	}
	notes, err := source.NewGHRelease(gh).Fetch(t.Context(), registry.ToolSpec{Repo: "gruntwork-io/terragrunt"}, vs)
	require.NoError(t, err)
	require.Len(t, notes, 2)

	c := newClassifier(t, classify.Options{})
	comments := 0
	for _, n := range notes {
		fenced := fencedComments(n.Markdown)
		comments += len(fenced)
		entries := c.Entries(n)
		require.NotEmpty(t, entries)
		for _, e := range entries {
			for _, h := range e.Source.Heading {
				assert.NotContains(t, fenced, h, "%s: fenced comment used as a heading", n.Version.Tag)
			}
		}
	}
	assert.Positive(t, comments, "the fixture has fenced comments such as # terragrunt.stack.hcl")
}
