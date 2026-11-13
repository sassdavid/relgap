package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sassdavid/relgap/internal/execx"
)

// skippedDir reports whether a directory is never walked when git can't list a repository's files.
func skippedDir(name string) bool {
	switch name {
	case ".git", ".terraform", ".terragrunt-cache", "node_modules", "vendor":
		return true
	}
	return false
}

const (
	gitCmd     = "git"
	gitTimeout = 30 * time.Second
)

func hasGit(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// isSubmodule reports whether dir is a checked-out submodule, whose .git is a file.
func isSubmodule(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil && info.Mode().IsRegular()
}

// findRepos returns the directories under root that contain .git, without descending into
// them. A root without any is one repository itself.
func findRepos(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root %s: not a directory", root)
	}
	if hasGit(root) {
		return []string{root}, nil
	}
	var repos []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil && p == root:
			return walkErr
		case walkErr != nil:
			return nil //nolint:nilerr // an unreadable directory below the root is skipped
		case !d.IsDir() || p == root:
			return nil
		case skippedDir(d.Name()):
			return filepath.SkipDir
		case hasGit(p):
			repos = append(repos, p)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	if len(repos) == 0 {
		repos = []string{root}
	}
	return repos, nil
}

// listing is a repository's files and the submodules to scan as repositories of their own.
type listing struct {
	files      []string // slash-separated, relative to the repository
	submodules []string // directories
}

// list asks git for the repository's files, so .gitignore applies. Without git, or outside a
// work tree, it walks the directory and skips the directories [skippedDir] names.
func (s *Scanner) list(ctx context.Context, repo string) (listing, error) {
	res, err := s.runner.Run(ctx, execx.Cmd{
		Name: gitCmd, Args: []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"},
		Dir: repo, Timeout: gitTimeout,
	})
	if err != nil {
		if ctx.Err() != nil {
			return listing{}, fmt.Errorf("list %s: %w", repo, ctx.Err())
		}
		if !errors.Is(err, execx.ErrNotFound) {
			s.logger.DebugContext(ctx, "git can't list files; walking the directory", "repo", repo, "error", err)
		}
		return s.walk(repo)
	}
	var l listing
	for _, e := range splitNUL(res.Stdout) {
		if !strings.HasSuffix(e, "/") { // untracked nested repositories end in a slash
			l.files = append(l.files, e)
		}
	}
	if s.opts.Submodules {
		if l.submodules, err = s.gitSubmodules(ctx, repo); err != nil {
			return listing{}, err
		}
	}
	return l, nil
}

// gitSubmodules returns the directories of gitlink entries (mode 160000) that are checked out.
func (s *Scanner) gitSubmodules(ctx context.Context, repo string) ([]string, error) {
	res, err := s.runner.Run(ctx, execx.Cmd{
		Name: gitCmd, Args: []string{"ls-files", "-z", "--stage"}, Dir: repo, Timeout: gitTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("list submodules of %s: %w", repo, err)
	}
	var subs []string
	for _, e := range splitNUL(res.Stdout) {
		meta, p, ok := strings.Cut(e, "\t")
		if ok && strings.HasPrefix(meta, "160000 ") && hasGit(filepath.Join(repo, p)) {
			subs = append(subs, filepath.Join(repo, filepath.FromSlash(p)))
		}
	}
	return subs, nil
}

func (s *Scanner) walk(repo string) (listing, error) {
	var l listing
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil && p == repo:
			return walkErr
		case walkErr != nil:
			return nil //nolint:nilerr // an unreadable directory below the repository is skipped
		case d.IsDir():
			return s.enterDir(&l, repo, p, d.Name())
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(repo, p)
			if err != nil {
				return fmt.Errorf("relative path: %w", err)
			}
			l.files = append(l.files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return listing{}, fmt.Errorf("walk %s: %w", repo, err)
	}
	return l, nil
}

// enterDir decides whether the walk descends into dir. Another repository below this one is
// skipped, or listed as a submodule with Options.Submodules.
func (s *Scanner) enterDir(l *listing, repo, dir, name string) error {
	switch {
	case dir == repo:
	case skippedDir(name):
		return filepath.SkipDir
	case hasGit(dir):
		if s.opts.Submodules && isSubmodule(dir) {
			l.submodules = append(l.submodules, dir)
		}
		return filepath.SkipDir
	}
	return nil
}

func splitNUL(b []byte) []string {
	var out []string
	for e := range bytes.SplitSeq(bytes.TrimRight(b, "\x00"), []byte{0}) {
		if len(e) > 0 {
			out = append(out, string(e))
		}
	}
	return out
}

// globMatch matches a slash-separated path against a pattern where ** stands for any number of
// directories and other segments follow [path.Match].
func globMatch(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			pat = pat[1:]
			if len(pat) == 0 {
				return true
			}
			for i := range len(name) + 1 {
				if matchSegments(pat, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], name[0]); !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
