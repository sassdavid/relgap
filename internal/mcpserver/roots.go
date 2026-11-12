package mcpserver

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sassdavid/relgap/internal/mcpkit"
)

// ErrOutsideRoots is returned for a directory argument outside the server's roots.
var ErrOutsideRoots = errors.New("outside the server's roots")

// roots returns the directories tool paths must be in, with symlinks resolved: the --root
// directories, else the client's roots, else the server's working directory.
func (t *tools) roots(req *mcp.CallToolRequest) ([]string, error) {
	roots := t.o.Roots
	if len(roots) == 0 {
		client, err := mcpkit.ClientRoots(req)
		if err != nil {
			return nil, err
		}
		roots = client
	}
	if len(roots) == 0 {
		roots = []string{t.o.Cwd}
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		resolved, err := realPath(r)
		if err != nil {
			return nil, fmt.Errorf("root %s: %w", r, err)
		}
		out = append(out, resolved)
	}
	return out, nil
}

// within resolves p, relative to the first root when it isn't absolute, and returns it when it
// is one of roots or inside one.
func within(p string, roots []string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(roots[0], p)
	}
	resolved, err := realPath(p)
	if err != nil {
		return "", err
	}
	for _, r := range roots {
		if inside(r, resolved) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%s is %w (%s)", p, ErrOutsideRoots, strings.Join(roots, ", "))
}

// realPath returns the absolute path of p with symlinks resolved, so a link can't lead outside
// a root.
func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", p, err)
	}
	return resolved, nil
}

// inside reports whether p is root or below it. [filepath.Rel] compares Windows paths without
// case and fails across volumes.
func inside(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}
