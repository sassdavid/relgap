package scan

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/sassdavid/relgap/internal/execx"
)

const (
	archiveTimeout = 5 * time.Minute
	dirPerm        = 0o750
	filePerm       = 0o600
)

// ErrRefNotFound is returned when a git ref doesn't name a commit in the repository.
var ErrRefNotFound = errors.New("git ref not found")

// ResolveRef returns the commit ID that ref names in the repository containing dir.
func ResolveRef(ctx context.Context, r Runner, dir, ref string) (string, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("%w: %q", ErrRefNotFound, ref)
	}
	res, err := r.Run(ctx, execx.Cmd{
		Name: gitCmd, Args: []string{"rev-parse", "--verify", "--quiet", "--end-of-options", ref + "^{commit}"},
		Dir: dir, Timeout: gitTimeout,
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("resolve %s: %w", ref, ctx.Err())
		}
		if _, ok := errors.AsType[*execx.ExitError](err); ok {
			return "", fmt.Errorf("%w: %s in %s", ErrRefNotFound, ref, dir)
		}
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// ExtractRef writes the files that commit (a full commit ID) has under dir into dst, as
// `git archive` lists them: tracked files only, without export-ignore paths. Files larger than
// the scanner reads and anything but regular files and directories are left out.
func ExtractRef(ctx context.Context, r Runner, dir, commit, dst string) error {
	f, err := os.CreateTemp("", "relgap-archive-*.tar")
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	if _, err = r.Run(ctx, execx.Cmd{
		Name: gitCmd, Args: []string{"archive", "--format=tar", commit},
		Dir: dir, Stdout: f, Timeout: archiveTimeout,
	}); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("archive %s: %w", commit, ctx.Err())
		}
		return fmt.Errorf("archive %s: %w", commit, err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	return untar(f, dst)
}

// untar extracts regular files and directories through an [os.Root], so no entry can write
// outside dst.
func untar(r io.Reader, dst string) error {
	root, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() { _ = root.Close() }()
	tr := tar.NewReader(r)
	for {
		h, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			return nil
		}
		if nextErr != nil {
			return fmt.Errorf("read archive: %w", nextErr)
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "/"))
		if name == "." {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, dirPerm)
		case tar.TypeReg:
			if h.Size > maxFileSize {
				continue
			}
			err = writeEntry(root, name, tr)
		default:
			continue
		}
		if err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	}
}

func writeEntry(root *os.Root, name string, r io.Reader) error {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, dirPerm); err != nil {
			return err //nolint:wrapcheck // the caller names the entry
		}
	}
	out, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, filePerm)
	if err != nil {
		return err //nolint:wrapcheck // the caller names the entry
	}
	if _, err = io.Copy(out, io.LimitReader(r, maxFileSize)); err != nil {
		_ = out.Close()
		return err //nolint:wrapcheck // the caller names the entry
	}
	return out.Close() //nolint:wrapcheck // the caller names the entry
}
