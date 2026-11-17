package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	goldenInclude = regexp.MustCompile(`^<<<\s+@/\.\./testdata/golden/(\S+?)(\{[^}]*\})?\s*$`)
	commandLine   = regexp.MustCompile(`^\$?\s*(relgap\s.*)$`)
)

// skippedDirs are the docs folders that hold no hand-written pages.
func skippedDirs() []string {
	return []string{"plan", "engineering", "validation", "node_modules", ".vitepress", "public", "cli"}
}

// checkTranscripts returns a problem for each golden file a site page includes whose command,
// the last relgap line above the include, isn't the command a test script compares with that
// golden file. So every output on the site is one a test produced from cassettes.
func checkTranscripts(root string) ([]string, error) {
	scripts, err := scriptText(root)
	if err != nil {
		return nil, err
	}
	docs := filepath.Join(root, "docs")
	fsys := os.DirFS(docs)
	var problems []string
	err = fs.WalkDir(fsys, ".", func(rel string, e fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case e.IsDir() && slices.Contains(skippedDirs(), rel):
			return fs.SkipDir
		case e.IsDir() || !strings.HasSuffix(rel, ".md"):
			return nil
		}
		b, rerr := fs.ReadFile(fsys, rel)
		if rerr != nil {
			return fmt.Errorf("read %s: %w", rel, rerr)
		}
		for _, p := range transcriptProblems(b, scripts) {
			problems = append(problems, filepath.Join("docs", rel)+":"+p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk docs: %w", err)
	}
	return problems, nil
}

// transcriptProblems checks one page against the concatenated test scripts.
func transcriptProblems(page []byte, scripts string) []string {
	var problems []string
	var last string
	sc := bufio.NewScanner(bytes.NewReader(page))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if m := commandLine.FindStringSubmatch(line); m != nil {
			last = strings.Join(strings.Fields(m[1]), " ")
			continue
		}
		m := goldenInclude.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		golden := m[1]
		switch {
		case last == "":
			problems = append(problems, fmt.Sprintf("%d: %s has no relgap command above it", n, golden))
		case !strings.Contains(scripts, "\nexec "+last+"\ngolden stdout "+golden+"\n"):
			problems = append(problems, fmt.Sprintf(
				"%d: no script in testdata/script runs %q and compares its output with %s", n, last, golden))
		}
		last = ""
	}
	return problems
}

// scriptText returns every test script, each line with its leading spaces and a [cond] prefix
// removed, so a page command matches an exec line.
func scriptText(root string) (string, error) {
	files, err := filepath.Glob(filepath.Join(root, "testdata", "script", "*.txtar"))
	if err != nil {
		return "", fmt.Errorf("list test scripts: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no test scripts in %s", filepath.Join(root, "testdata", "script"))
	}
	cond := regexp.MustCompile(`^(\[[^\]]*\]\s*)+`)
	var b strings.Builder
	b.WriteString("\n")
	for _, f := range files {
		data, rerr := os.ReadFile(f)
		if rerr != nil {
			return "", fmt.Errorf("read %s: %w", f, rerr)
		}
		for l := range strings.Lines(string(data)) {
			l = cond.ReplaceAllString(strings.TrimSpace(l), "")
			b.WriteString(strings.Join(strings.Fields(l), " ") + "\n")
		}
	}
	return b.String(), nil
}
