package enrich

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/registry"
)

// ReachabilityNote says what binary mode can't tell, unlike govulncheck's symbol analysis.
const ReachabilityNote = "matched by Go version and module version only: relgap doesn't check " +
	"whether the binary calls the vulnerable code"

// Binary is an installed Go executable of a tool and what OSV knows about its Go version and
// main module.
type Binary struct {
	Tool          string  `json:"tool"`
	Path          string  `json:"path"`
	GoVersion     string  `json:"go_version"`               // as OSV takes it, such as 1.24.3
	Module        string  `json:"module,omitempty"`         // the main module path
	ModuleVersion string  `json:"module_version,omitempty"` // absent for (devel) builds
	Queries       []Query `json:"queries"`
	Vulns         []Vuln  `json:"vulns"`
}

// BinaryFinder finds a tool's executables on PATH and among mise's installed versions.
type BinaryFinder struct {
	Lookup func(key string) (string, bool) // environment
	// Which resolves a mise shim to the executable it runs, as `mise which` does; nil skips
	// shims, whose targets the installs directory has anyway.
	Which func(ctx context.Context, name string) (string, error)
}

// Find returns the executables named in t.Bin (default: the tool name), resolved paths without
// duplicates: PATH first, then $MISE_DATA_DIR/installs/<tool>/<version>/ and its bin/. Version
// aliases (symlinks such as latest or 1.17) are skipped.
func (f BinaryFinder) Find(ctx context.Context, t registry.ToolSpec) []string {
	var out []string
	for _, p := range f.candidates(t) {
		if resolved, ok := f.resolve(ctx, p); ok && !slices.Contains(out, resolved) {
			out = append(out, resolved)
		}
	}
	return out
}

// candidates lists the paths an executable of t may have, in search order.
func (f BinaryFinder) candidates(t registry.ToolSpec) []string {
	names := t.Bin
	if len(names) == 0 {
		names = []string{t.Name}
	}
	var dirs []string
	path, _ := f.Lookup("PATH")
	dirs = append(dirs, filepath.SplitList(path)...)
	installs := filepath.Join(f.miseDataDir(), "installs")
	for _, tool := range append([]string{t.Name}, t.Aliases...) {
		entries, err := os.ReadDir(filepath.Join(installs, tool))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type()&fs.ModeSymlink == 0 && e.IsDir() {
				version := filepath.Join(installs, tool, e.Name())
				dirs = append(dirs, version, filepath.Join(version, "bin"))
			}
		}
	}
	out := make([]string, 0, len(dirs)*len(names))
	for _, dir := range dirs {
		for _, n := range names {
			out = append(out, filepath.Join(dir, exeName(n)))
		}
	}
	return out
}

// resolve returns the real path of the executable at p, following a mise shim to its target.
func (f BinaryFinder) resolve(ctx context.Context, p string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil || !executable(resolved) {
		return "", false
	}
	if !shim(resolved) {
		return resolved, true
	}
	if f.Which == nil {
		return "", false
	}
	target, err := f.Which(ctx, filepath.Base(p))
	if err != nil || target == "" {
		return "", false
	}
	if resolved, err = filepath.EvalSymlinks(target); err != nil || !executable(resolved) || shim(resolved) {
		return "", false
	}
	return resolved, true
}

func (f BinaryFinder) miseDataDir() string {
	if d, ok := f.Lookup("MISE_DATA_DIR"); ok && d != "" {
		return d
	}
	if d, ok := f.Lookup("XDG_DATA_HOME"); ok && d != "" {
		return filepath.Join(d, "mise")
	}
	home, _ := f.Lookup("HOME")
	return filepath.Join(home, ".local", "share", "mise")
}

func exeName(n string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(n), ".exe") {
		return n + ".exe"
	}
	return n
}

func executable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode().Perm()&0o111 != 0
}

// shim reports whether p is mise itself, which is what a mise shim links to.
func shim(p string) bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(p)), ".exe")
	return base == "mise"
}

// ReadBinary reads the Go build info of the executable at path and the OSV queries it implies:
// the standard library at the Go version, and the main module unless it was built as (devel).
func ReadBinary(tool, path string) (*Binary, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go build info of %s: %w", path, err)
	}
	b := &Binary{Tool: tool, Path: path, GoVersion: GoSemver(bi.GoVersion), Vulns: []Vuln{}}
	if strings.HasPrefix(bi.GoVersion, "devel") || !goTag.MatchString(strings.Fields(bi.GoVersion + " ")[0]) {
		return nil, fmt.Errorf("read go build info of %s: development Go version %q", path, bi.GoVersion)
	}
	b.Queries = append(b.Queries, Query{Package: Package{Ecosystem: EcosystemGo, Name: "stdlib"}, Version: b.GoVersion})
	b.Module = bi.Main.Path
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		b.ModuleVersion = strings.TrimPrefix(v, "v")
		b.Queries = append(b.Queries, Query{
			Package: Package{Ecosystem: EcosystemGo, Name: bi.Main.Path},
			Version: b.ModuleVersion,
		})
	}
	return b, nil
}

// BinaryVulns queries OSV for every binary's queries in one batch and fills in their vulns.
func (e *Enricher) BinaryVulns(ctx context.Context, bins []*Binary) error {
	var (
		queries []Query
		owner   []int
	)
	for i, b := range bins {
		for _, q := range b.Queries {
			queries = append(queries, q)
			owner = append(owner, i)
		}
	}
	if len(queries) == 0 {
		return nil
	}
	ids, err := e.osv.QueryBatch(ctx, queries)
	if err != nil {
		return err
	}
	records, err := e.records(ctx, ids)
	if err != nil {
		return err
	}
	for qi, q := range queries {
		b := bins[owner[qi]]
		p := registry.OSVPackage{Ecosystem: q.Package.Ecosystem, Name: q.Package.Name}
		for _, id := range ids[qi] {
			if rec := records[id]; rec != nil {
				b.Vulns = append(b.Vulns, vulnOf(rec, p, q, Request{}))
			}
		}
	}
	for _, b := range bins {
		b.Vulns = mergeAliases(b.Vulns)
	}
	return nil
}
