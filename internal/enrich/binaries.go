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

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/registry"
)

// ReachabilityNote says what binary mode can't tell, unlike govulncheck's symbol analysis.
const ReachabilityNote = "matched by Go version and module version only: relgap doesn't check " +
	"whether the binary calls the vulnerable code"

// goStdlib is the OSV package of the Go standard library.
const goStdlib = "stdlib"

// Binary is an installed Go executable of a tool and what OSV knows about its Go version and
// main module.
type Binary struct {
	Tool          string `json:"tool"`
	Path          string `json:"path"`
	GoVersion     string `json:"go_version"`               // as OSV takes it, such as 1.24.3
	Module        string `json:"module,omitempty"`         // the main module path
	ModuleVersion string `json:"module_version,omitempty"` // absent for (devel) builds
	// ModuleUnchecked says why OSV wasn't asked about the main module, such as private-source.
	ModuleUnchecked string  `json:"module_unchecked,omitempty"`
	Queries         []Query `json:"queries"`
	Vulns           []Vuln  `json:"vulns"`

	spec registry.ToolSpec
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
	if runtime.GOOS == goosWindows && !strings.HasSuffix(strings.ToLower(n), ".exe") {
		return n + ".exe"
	}
	return n
}

func executable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == goosWindows || st.Mode().Perm()&0o111 != 0
}

// shim reports whether p is mise itself, which is what a mise shim links to.
func shim(p string) bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(p)), ".exe")
	return base == "mise"
}

// ReadBinary reads the Go build info of the executable of tool t at path and the OSV queries it
// implies: the standard library at the Go version, and the main module unless it was built as
// (devel). [Enricher.BinaryVulns] drops the main module when it may be private.
func ReadBinary(t registry.ToolSpec, path string) (*Binary, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go build info of %s: %w", path, err)
	}
	b := &Binary{Tool: t.Name, Path: path, GoVersion: GoSemver(bi.GoVersion), Vulns: []Vuln{}, spec: t}
	if strings.HasPrefix(bi.GoVersion, "devel") || !goTag.MatchString(strings.Fields(bi.GoVersion + " ")[0]) {
		return nil, fmt.Errorf("read go build info of %s: development Go version %q", path, bi.GoVersion)
	}
	b.Queries = append(b.Queries, Query{Package: Package{Ecosystem: EcosystemGo, Name: goStdlib}, Version: b.GoVersion})
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

// BinaryVulns queries OSV for every binary's queries in one batch and fills in their vulns,
// marking the ones the suppressor names and adding the feeds' fields. It returns the feeds
// that failed as problems, and an error when OSV failed.
func (e *Enricher) BinaryVulns(ctx context.Context, bins []*Binary) ([]Problem, error) {
	problems, err := e.screenModules(ctx, bins)
	if err != nil {
		return nil, err
	}
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
		return problems, nil
	}
	hits, err := e.osv.QueryBatch(ctx, queries)
	if err != nil {
		return nil, err
	}
	records, err := e.records(ctx, slices.Concat(hits...))
	if err != nil {
		return nil, err
	}
	for qi, q := range queries {
		b := bins[owner[qi]]
		p := registry.OSVPackage{Ecosystem: q.Package.Ecosystem, Name: q.Package.Name}
		for _, h := range hits[qi] {
			if rec := records[h.ID]; rec != nil {
				b.Vulns = append(b.Vulns, vulnOf(rec, p, q.Version, Request{}))
			}
		}
	}
	lists := make([][]Vuln, len(bins))
	for i, b := range bins {
		b.Vulns = mergeAliases(b.Vulns)
		lists[i] = b.Vulns
	}
	if err = e.rateFromAliases(ctx, lists, records); err != nil {
		return nil, err
	}
	for _, b := range bins {
		e.suppressBinary(b)
		sortVulns(b.Vulns)
	}
	return append(problems, e.prioritize(ctx, lists)...), nil
}

// screenModules drops the main module query of each binary whose module GOPRIVATE names or whose
// tool may be private: OSV hears about the module only when the tool's repository comes from a
// registry or its forge says it is public.
func (e *Enricher) screenModules(ctx context.Context, bins []*Binary) ([]Problem, error) {
	reasons := make([]string, len(bins))
	var repos []Repo
	at := map[int]Repo{}
	for i, b := range bins {
		switch {
		case b.ModuleVersion == "":
		case e.privacy.Go.Matches(b.Module):
			reasons[i] = ReasonPrivate
		case b.spec.UserRepo:
			r, ok := toolRepo(b.spec)
			if !ok {
				reasons[i] = ReasonPrivate
				continue
			}
			at[i] = r
			repos = append(repos, r)
		}
	}
	var problems []Problem
	if len(repos) > 0 {
		answers, err := e.public(ctx, repos)
		if err != nil {
			return nil, err
		}
		for i, r := range at {
			if reasons[i] = visibilityReason(answers[r]); reasons[i] == ReasonFailed {
				problems = append(problems, Problem{Source: SourceOSV, Tool: bins[i].Tool, Err: answers[r]})
			}
		}
	}
	for i, b := range bins {
		if reasons[i] == "" {
			continue
		}
		b.ModuleUnchecked = reasons[i]
		b.Queries = slices.DeleteFunc(b.Queries, func(q Query) bool { return q.Package.Name == b.Module })
	}
	slices.SortFunc(problems, func(x, y Problem) int { return strings.Compare(x.Tool, y.Tool) })
	return problems, nil
}

// toolRepo returns the repository of t on github.com or gitlab.com. The second result is false
// on any other host, which OSV never hears about.
func toolRepo(t registry.ToolSpec) (Repo, bool) {
	switch {
	case t.Repo == "":
		return Repo{}, false
	case forge.IsGitHub(t.Host):
		return Repo{Host: forge.GitHub, Path: t.Repo}, true
	case strings.EqualFold(t.Host, forge.GitLab):
		return Repo{Host: forge.GitLab, Path: t.Repo}, true
	}
	return Repo{}, false
}

// suppressBinary marks each advisory of b that the suppressor names, with b's tool and the purl
// of the advisory's package at the version b was built with.
func (e *Enricher) suppressBinary(b *Binary) {
	if e.suppressor == nil {
		return
	}
	for j := range b.Vulns {
		v := &b.Vulns[j]
		at := b.ModuleVersion
		if v.Package.Name == goStdlib {
			at = b.GoVersion
		}
		sub := Subject{Tool: b.Tool, Version: at}
		pkg := registry.OSVPackage{Ecosystem: v.Package.Ecosystem, Name: v.Package.Name}
		if p, ok := (Request{}).packagePURLAt(pkg, at, ""); ok {
			sub.PURLs = append(sub.PURLs, p)
		}
		v.Suppressed = e.suppressor.Suppress(sub, v.names())
	}
}
