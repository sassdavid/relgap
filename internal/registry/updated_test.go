package registry_test

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sassdavid/relgap/internal/registry"
	curated "github.com/sassdavid/relgap/registry"
)

// fakeRepo serves registry files from a map by name.
type fakeRepo map[string][]byte

func (f fakeRepo) Dir(_ context.Context, repo, ref, dir string) ([]string, error) {
	if repo != registry.UpdateRepo || ref != registry.UpdateRef || dir != registry.UpdatePath {
		return nil, fs.ErrNotExist
	}
	names := make([]string, 0, len(f))
	for n := range f {
		names = append(names, n)
	}
	return names, nil
}

func (f fakeRepo) File(_ context.Context, _, _, p string) ([]byte, error) {
	return f[path.Base(p)], nil
}

func embedded(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fs.ReadFile(curated.FS(), name)
	require.NoError(t, err)
	return b
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "registry")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stale.yaml"), []byte("name: stale\nrepo: a/b\n"), 0o600))
	gh := append(embedded(t, "gh.yaml"), []byte("bin: [gh]\n")...)
	repo := fakeRepo{
		"terraform.yaml": embedded(t, "terraform.yaml"),
		"gh.yaml":        gh,
		"newtool.yaml":   []byte("name: newtool\nrepo: acme/newtool\n"),
		"broken.yaml":    []byte("name: broken\nrepo: acme/broken\nunknown_key: 1\n"),
		"future.yaml":    []byte("name: future\nrepo: acme/future\nmin_relgap: 9.0.0\n"),
		"README.md":      []byte("not an entry"),
	}
	res, err := registry.Update(t.Context(), repo, dir, "0.5.0")
	require.NoError(t, err)
	status := map[string]string{}
	for _, e := range res.Entries {
		status[e.Name] = e.Status
		if e.Status == registry.UpdateSkipped {
			assert.NotEmpty(t, e.Reason, e.Name)
		}
	}
	assert.Equal(t, map[string]string{
		"terraform": registry.UpdateUnchanged,
		"gh":        registry.UpdateChanged,
		"newtool":   registry.UpdateAdded,
		"broken":    registry.UpdateSkipped,
		"future":    registry.UpdateSkipped,
	}, status)

	files, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{filepath.Join(dir, "gh.yaml"), filepath.Join(dir, "newtool.yaml")}, files,
		"only changed and added entries are kept, and the old directory is replaced")

	r, err := registry.LoadWith(t.Context(), discard(), nil, registry.ConfigLayer{},
		registry.LoadOptions{UpdatedDir: dir, Version: "0.5.0"})
	require.NoError(t, err)
	res2, err := r.Resolve(t.Context(), "gh", registry.ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, registry.RelgapSourceUpdated, res2.RelgapSource)
	res2, err = r.Resolve(t.Context(), "newtool", registry.ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "acme/newtool", res2.Tool.Repo)
	assert.Equal(t, registry.RelgapSourceUpdated, res2.RelgapSource)
	res2, err = r.Resolve(t.Context(), "terraform", registry.ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, registry.RelgapSourceEmbedded, res2.RelgapSource)
}

func TestLoadUpdatedSkips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("terraform.yaml", "name: terraform\nrepo: acme/terraform-fork\nmin_relgap: 0.6.0\n")
	write("gh.yaml", "name: not-gh\nrepo: acme/gh\n")
	write("helm.yaml", "name: helm\nrepo: acme/helm-fork\nmin_relgap: 0.5.0\n")
	l, err := registry.NewLinter()
	require.NoError(t, err)

	got := registry.LoadUpdated(t.Context(), discard(), l, dir, "v0.5.0")
	require.Len(t, got, 1, "a newer min_relgap and a name that doesn't match the file are skipped")
	assert.Equal(t, "helm", got[0].Name)

	got = registry.LoadUpdated(t.Context(), discard(), l, dir, "dev")
	assert.Len(t, got, 2, "a development build accepts every min_relgap")

	assert.Empty(t, registry.LoadUpdated(t.Context(), discard(), l, filepath.Join(dir, "missing"), "0.5.0"))
}
