package main

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// distEnv names the built site, as docs:build sets it. Without it only the sources are checked.
const distEnv = "DOCS_DIST"

// unpublished are the docs paths VitePress leaves out (srcExclude in config.mts).
func unpublished() []string {
	return []string{
		"plan", "engineering", "validation", "dogfood.md", "README.md", "node_modules", ".vitepress/dist",
		".vitepress/cache",
	}
}

// testOnly are names of test harness internals and dev-only settings that no user page mentions.
func testOnly() []string {
	return []string{
		"TEST_CASSETTE", "GOLDEN_UPDATE", "TEST_NOW", "fakebin", "_carapace", "testdata/cassettes",
		"RELGAP_LIVE_OUT", "RELGAP_BEDROCK_MODEL", "test:live",
	}
}

var (
	tokenPattern = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,}|` +
		`xox[abprs]-[A-Za-z0-9-]{10,}|hooks\.slack\.com/services/T[A-Za-z0-9]+|AKIA[0-9A-Z]{16}|` +
		`ASIA[0-9A-Z]{16}|sk-ant-[A-Za-z0-9_-]{20,}`)
	// localPath finds paths of a developer's machine.
	localPath  = regexp.MustCompile(`/Users/[A-Za-z]|/home/[a-z][a-z0-9_-]*/|[A-Za-z]:\\Users\\|~/src/github`)
	linkTarget = regexp.MustCompile(`(?:\]\(|href=")([^)"\s]+)`)
	// internalTarget matches a link target in the unpublished docs, relative or on GitHub.
	internalTarget = regexp.MustCompile(`(?:^|/)(?:plan|engineering|validation)/|dogfood\.md|CLAUDE\.md|\.claude/rules`)
	milestone      = regexp.MustCompile(`\bM[0-9]{1,2}\b`)
	// svgPath is SVG path data, whose move commands look like milestone names.
	svgPath = regexp.MustCompile(`\sd="[^"]*"`)
)

// siteSources returns the published files under docs/ and the files they include, keyed by
// their path relative to the repository root.
func siteSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	docs := filepath.Join(repoRoot, "docs")
	err := filepath.WalkDir(docs, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(docs, p)
		if slices.Contains(unpublished(), filepath.ToSlash(rel)) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".md", ".txt", ".json", ".vue", ".mts", ".yml":
			if !e.IsDir() {
				b, rerr := os.ReadFile(p)
				require.NoError(t, rerr)
				out["docs/"+filepath.ToSlash(rel)] = string(b)
			}
		}
		return nil
	})
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	require.NoError(t, err)
	out["CHANGELOG.md"] = string(b)
	return out
}

// distFiles returns the text files of the built site.
func distFiles(t *testing.T, dist string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dist, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		switch filepath.Ext(p) {
		case ".html", ".txt", ".json", ".xml":
			b, rerr := os.ReadFile(p)
			require.NoError(t, rerr)
			rel, _ := filepath.Rel(dist, p)
			out["dist/"+filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, out, "%s has no built pages", dist)
	return out
}

// TestNeverOnSite runs the checks of docs-content.md "Never on the site" over the published
// sources and, with DOCS_DIST set, over the built site: the token grep, local paths, links into
// internal docs, milestone names and test internals.
func TestNeverOnSite(t *testing.T) {
	t.Parallel()
	files := siteSources(t)
	if dist := os.Getenv(distEnv); dist != "" {
		maps.Copy(files, distFiles(t, dist))
		for _, dir := range []string{"plan", "engineering", "validation"} {
			assert.NoDirExists(t, filepath.Join(dist, dir), "the built site has an unpublished folder")
		}
		assert.NoFileExists(t, filepath.Join(dist, "dogfood.html"), "the built site has an unpublished page")
	}
	for name, text := range files {
		checkNeverOnSite(t, name, text)
	}
}

// checkNeverOnSite reports everything in one file that must not reach the site.
func checkNeverOnSite(t *testing.T, name, text string) {
	t.Helper()
	if m := tokenPattern.FindString(text); m != "" {
		t.Errorf("%s holds what looks like a token: %.12s...", name, m)
	}
	if m := localPath.FindString(text); m != "" {
		t.Errorf("%s holds a local path: %s", name, m)
	}
	if link := internalLink(text); link != "" {
		t.Errorf("%s links into the internal docs: %s", name, link)
	}
	if isChangelog(name) {
		// release-please writes the changelog from commit subjects, which name milestones.
		return
	}
	if m := milestone.FindString(
		svgPath.ReplaceAllString(text, ""),
	); m != "" &&
		!strings.HasPrefix(name, "dist/assets/") {
		t.Errorf("%s names the milestone %s", name, m)
	}
	for _, s := range testOnly() {
		assert.NotContains(t, text, s, "%s mentions a test internal", name)
	}
}

func isChangelog(name string) bool {
	return name == "CHANGELOG.md" || name == "dist/changelog.html"
}

// internalLink returns the first link target in text that points into the unpublished docs.
func internalLink(text string) string {
	for _, m := range linkTarget.FindAllStringSubmatch(text, -1) {
		if internalTarget.MatchString(m[1]) {
			return m[1]
		}
	}
	return ""
}

func TestNeverOnSitePatterns(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "../plan/adr/0007.md", internalLink("[ADR](../plan/adr/0007.md)"))
	assert.NotEmpty(t, internalLink(`<a href="https://github.com/sassdavid/relgap/blob/main/docs/engineering/x.md">`))
	assert.NotEmpty(t, internalLink("[notes](https://github.com/sassdavid/relgap/blob/main/docs/dogfood.md)"))
	assert.Empty(t, internalLink("[config](/reference/config) and [plan](/cli/drift#plan)"))
	assert.Regexp(t, tokenPattern, "export GH_TOKEN=ghp_"+strings.Repeat("a", 36))
	assert.Regexp(t, tokenPattern, "sk-ant-"+strings.Repeat("b", 40))
	assert.NotRegexp(t, tokenPattern, "export GH_TOKEN=<your token>")
	assert.Regexp(t, localPath, "cd /Users/someone/src")
	assert.Regexp(t, localPath, "~/src/github/_reference")
	assert.NotRegexp(t, localPath, "~/.local/share/man/man1 and relgap outdated ~/src/infra")
	assert.Regexp(t, milestone, "since M19")
	assert.NotRegexp(t, milestone, "M5a and BM12")
	assert.NotRegexp(t, milestone, svgPath.ReplaceAllString(`<path d="M2 11 6 5M6 11l4-6" />`, ""))
}
