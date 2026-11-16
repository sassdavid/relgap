package registry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/version"
	curated "github.com/sassdavid/relgap/registry"
)

// UpdateRepo and UpdateRef name where `relgap registry update` reads entries from.
const (
	UpdateRepo = "sassdavid/relgap"
	UpdateRef  = "main"
	UpdatePath = "registry"
)

// LoadUpdated lints the entries in dir. An entry with problems, or one that needs a newer relgap
// than v, is logged and left out, so the embedded entry of that name stays in use.
func LoadUpdated(ctx context.Context, logger *slog.Logger, l *Linter, dir, v string) []*Entry {
	names, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	var out []*Entry
	for _, name := range names {
		b, readErr := os.ReadFile(name)
		if readErr != nil {
			logger.WarnContext(ctx, "can't read updated registry entry", "file", name, "error", readErr)
			continue
		}
		e, reason := checkUpdated(l, name, b, v)
		if reason != "" {
			logger.WarnContext(ctx, "skipping updated registry entry; using the embedded one",
				"file", name, "reason", reason)
			continue
		}
		out = append(out, e)
	}
	return out
}

// checkUpdated lints one updated entry and returns why it can't be used, or "".
func checkUpdated(l *Linter, origin string, b []byte, v string) (*Entry, string) {
	e, problems, err := l.Lint(origin, b)
	switch {
	case err != nil:
		return nil, err.Error()
	case len(problems) > 0:
		return nil, (&LintError{Problems: problems}).Error()
	case e == nil:
		return nil, "empty entry"
	}
	if needsNewer(e.MinRelgap, v) {
		return nil, fmt.Sprintf("needs relgap %s or newer, this is %s", e.MinRelgap, v)
	}
	return e, ""
}

// needsNewer reports whether min is a newer version than v. Development builds accept every entry.
func needsNewer(minVersion, v string) bool {
	if minVersion == "" {
		return false
	}
	s := version.Semver()
	have, err := s.Parse(strings.TrimPrefix(v, "v"))
	if err != nil {
		return false
	}
	want, err := s.Parse(minVersion)
	if err != nil {
		return true
	}
	return s.Compare(want, have) > 0
}

// mergeUpdated replaces each embedded entry with the updated entry of the same name and adds
// updated entries with new names, sorted by name.
func mergeUpdated(embedded, updated []*Entry) []*Entry {
	if len(updated) == 0 {
		return embedded
	}
	byName := map[string]*Entry{}
	for _, e := range embedded {
		byName[strings.ToLower(e.Name)] = e
	}
	for _, e := range updated {
		byName[strings.ToLower(e.Name)] = e
	}
	out := make([]*Entry, 0, len(byName))
	for _, e := range byName {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Update statuses of an entry, compared with the embedded one.
const (
	UpdateAdded     = "added"
	UpdateChanged   = "changed"
	UpdateUnchanged = "unchanged"
	UpdateSkipped   = "skipped"
)

// UpdatedEntry is one file `relgap registry update` downloaded.
type UpdatedEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"` // why it was skipped
}

// UpdateResult is what `relgap registry update` did.
type UpdateResult struct {
	Repo    string         `json:"repo"`
	Ref     string         `json:"ref"`
	Dir     string         `json:"dir"`
	Entries []UpdatedEntry `json:"entries"`
}

// RepoFiles reads the registry directory of relgap's repository.
type RepoFiles interface {
	Dir(ctx context.Context, repo, ref, dir string) ([]string, error)
	File(ctx context.Context, repo, ref, path string) ([]byte, error)
}

// Update downloads the entries of relgap's repository and writes the ones that differ from the
// embedded entries and pass the checks of [LoadUpdated] to dir, replacing what dir had.
func Update(ctx context.Context, files RepoFiles, dir, v string) (*UpdateResult, error) {
	l, err := NewLinter()
	if err != nil {
		return nil, err
	}
	names, err := files.Dir(ctx, UpdateRepo, UpdateRef, UpdatePath)
	if err != nil {
		return nil, fmt.Errorf("list registry entries: %w", err)
	}
	res := &UpdateResult{Repo: UpdateRepo, Ref: UpdateRef, Dir: dir, Entries: []UpdatedEntry{}}
	keep := map[string][]byte{}
	for _, name := range names {
		if path.Ext(name) != ".yaml" {
			continue
		}
		b, fileErr := files.File(ctx, UpdateRepo, UpdateRef, path.Join(UpdatePath, name))
		if fileErr != nil {
			return nil, fmt.Errorf("download registry entry: %w", fileErr)
		}
		u := UpdatedEntry{Name: strings.TrimSuffix(name, ".yaml")}
		if _, reason := checkUpdated(l, path.Join(UpdatePath, name), b, v); reason != "" {
			u.Status, u.Reason = UpdateSkipped, reason
			res.Entries = append(res.Entries, u)
			continue
		}
		embedded, readErr := fs.ReadFile(curated.FS(), name)
		switch {
		case errors.Is(readErr, fs.ErrNotExist):
			u.Status = UpdateAdded
		case readErr != nil:
			return nil, fmt.Errorf("read embedded entry: %w", readErr)
		case bytes.Equal(embedded, b):
			u.Status = UpdateUnchanged
		default:
			u.Status = UpdateChanged
		}
		if u.Status != UpdateUnchanged {
			keep[name] = b
		}
		res.Entries = append(res.Entries, u)
	}
	if err = replaceDir(dir, keep); err != nil {
		return nil, err
	}
	return res, nil
}

// replaceDir writes files to a new directory and puts it in place of dir.
func replaceDir(dir string, files map[string][]byte) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(dir)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmp)
	for name, b := range files {
		if err = os.WriteFile(filepath.Join(tmp, name), b, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	// Windows can't rename over an existing directory.
	if err = os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove old entries: %w", err)
	}
	if err = os.Rename(tmp, dir); err != nil {
		return fmt.Errorf("move entries into %s: %w", dir, err)
	}
	return nil
}
