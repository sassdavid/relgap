package enrich

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/module"

	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
)

// ErrNotPublic means a forge answered that a repository is private, internal or not visible.
var ErrNotPublic = errors.New("the repository isn't public")

// Repo is a repository on github.com or gitlab.com.
type Repo struct {
	Host string // forge.GitHub or forge.GitLab
	Path string // owner/repo, or the project path
}

// ModulePath is the repository as a Go module path, such as github.com/acme/tool.
func (r Repo) ModulePath() string { return r.Host + "/" + r.Path }

// Visibility looks up whether repositories are public.
type Visibility interface {
	// Public returns, for each repository, nil when it is public, an error wrapping
	// [ErrNotPublic] when it isn't, and the lookup's error when the forge didn't say. The error
	// is set only for a canceled context.
	Public(ctx context.Context, repos []Repo) (map[Repo]error, error)
}

// Privacy says what relgap keeps away from OSV beyond the hosts it never sends.
type Privacy struct {
	Go GoPrivate
	// Repos answers for the repositories the user named: a github: or gitlab: pin, a
	// tools.<name>.repo entry, a --guess or an action. nil treats each of them as private.
	Repos Visibility
}

// SetPrivacy sets what the enricher keeps away from OSV.
func (e *Enricher) SetPrivacy(p Privacy) { e.privacy = p }

// GoPrivate holds the module path patterns of GOPRIVATE, GONOPROXY and GONOSUMDB, which Go keeps
// away from its proxy and checksum database.
type GoPrivate struct {
	patterns []string
}

// goosWindows is the GOOS of Windows, whose paths and settings differ.
const goosWindows = "windows"

// goToolchain is the OSV package of the go command, which the curated go entry queries.
const goToolchain = "toolchain"

// NewGoPrivate returns the patterns of a GOPRIVATE value: comma-separated globs on path prefixes.
func NewGoPrivate(values ...string) GoPrivate {
	var g GoPrivate
	for _, v := range values {
		for p := range strings.SplitSeq(v, ",") {
			if p = strings.TrimSpace(p); p != "" && !slices.Contains(g.patterns, p) {
				g.patterns = append(g.patterns, p)
			}
		}
	}
	return g
}

// ReadGoPrivate reads GOPRIVATE, GONOPROXY and GONOSUMDB as the go command does: a non-empty
// environment variable, else the file `go env -w` writes ($GOENV, or go/env in the user config
// directory of goos). GONOPROXY and GONOSUMDB default to GOPRIVATE, so the patterns add up.
func ReadGoPrivate(lookup func(string) (string, bool), goos string) GoPrivate {
	private, _ := lookup("GOPRIVATE")
	noProxy, _ := lookup("GONOPROXY")
	noSumDB, _ := lookup("GONOSUMDB")
	values := []string{private, noProxy, noSumDB}
	if slices.Contains(values, "") {
		file := readGoEnvFile(goEnvFile(lookup, goos))
		for i, k := range []string{"GOPRIVATE", "GONOPROXY", "GONOSUMDB"} {
			if values[i] == "" {
				values[i] = file[k]
			}
		}
	}
	return NewGoPrivate(values...)
}

// goEnvFile returns the path of the go command's environment file, or "" when GOENV is off or
// there is no user config directory.
func goEnvFile(lookup func(string) (string, bool), goos string) string {
	if f, _ := lookup("GOENV"); f != "" {
		if f == "off" {
			return ""
		}
		return f
	}
	if dir := userConfigDir(lookup, goos); dir != "" {
		return filepath.Join(dir, "go", "env")
	}
	return ""
}

// userConfigDir is [os.UserConfigDir] for goos, read from lookup.
func userConfigDir(lookup func(string) (string, bool), goos string) string {
	if goos == goosWindows {
		dir, _ := lookup("APPDATA")
		return dir
	}
	home, _ := lookup("HOME")
	if goos == "darwin" || goos == "ios" {
		if home == "" {
			return ""
		}
		return filepath.Join(home, "Library", "Application Support")
	}
	if dir, _ := lookup("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

// readGoEnvFile reads the KEY=VALUE lines of the go command's environment file. A missing or
// unreadable file sets nothing, as for the go command.
func readGoEnvFile(path string) map[string]string {
	out := map[string]string{}
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && k != "" && k[0] >= 'A' && k[0] <= 'Z' {
			out[k] = v
		}
	}
	return out
}

// Matches reports whether a pattern names the module path or one of its prefixes, by Go's rules:
// each glob matches as many leading path elements as it has.
func (g GoPrivate) Matches(path string) bool {
	return len(g.patterns) > 0 && module.MatchPrefixPatterns(strings.Join(g.patterns, ","), path)
}

// Patterns returns the patterns, for the debug log.
func (g GoPrivate) Patterns() []string { return slices.Clone(g.patterns) }

// screen applies the patterns to c: a go: path or a repository they name is private-source, and
// a curated Go package they name isn't asked about. stdlib and toolchain aren't module paths.
func (g GoPrivate) screen(c Coverage) Coverage {
	if !c.Checked() || len(g.patterns) == 0 {
		return c
	}
	if c.GoPath != "" && g.Matches(c.GoPath) || c.Repo != (Repo{}) && g.Matches(c.Repo.ModulePath()) {
		return Coverage{Reason: ReasonPrivate}
	}
	kept := slices.DeleteFunc(slices.Clone(c.Packages), func(p registry.OSVPackage) bool {
		return p.Ecosystem == EcosystemGo && p.Name != goStdlib && p.Name != goToolchain && g.Matches(p.Name)
	})
	if len(kept) == 0 && len(c.Packages) > 0 && c.GoPath == "" {
		return Coverage{Reason: ReasonPrivate}
	}
	c.Packages = kept
	return c
}

// visibility memoizes the forge's answers for one enricher, so the binaries of a tool reuse the
// answer its pins got.
type visibility struct {
	mu      sync.Mutex
	answers map[Repo]error
}

// public returns the forge's answer for each repository, asking only about the ones it has no
// answer for. Without a [Visibility], every repository is private.
func (e *Enricher) public(ctx context.Context, repos []Repo) (map[Repo]error, error) {
	out := make(map[Repo]error, len(repos))
	if e.privacy.Repos == nil {
		for _, r := range repos {
			out[r] = ErrNotPublic
		}
		return out, nil
	}
	e.vis.mu.Lock()
	defer e.vis.mu.Unlock()
	if e.vis.answers == nil {
		e.vis.answers = map[Repo]error{}
	}
	var ask []Repo
	for _, r := range repos {
		if _, ok := e.vis.answers[r]; !ok && !slices.Contains(ask, r) {
			ask = append(ask, r)
		}
	}
	if len(ask) > 0 {
		got, err := e.privacy.Repos.Public(ctx, ask)
		if err != nil {
			return nil, err //nolint:wrapcheck // only a canceled context
		}
		for _, r := range ask {
			e.vis.answers[r] = got[r]
		}
	}
	for _, r := range repos {
		out[r] = e.vis.answers[r]
	}
	return out, nil
}

// visibilityReason is the unchecked reason of a repository whose lookup answered err.
func visibilityReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotPublic):
		return ReasonPrivate
	case errors.Is(err, httpx.ErrOffline):
		return ReasonOffline
	default:
		return ReasonFailed
	}
}

// screenRepos asks the forge about the repositories the user named and leaves out of OSV each
// one that isn't public. A failed lookup is a problem of the tool, since OSV can't be asked.
func (e *Enricher) screenRepos(ctx context.Context, reqs []Request, covs []Coverage) ([]Problem, error) {
	var repos []Repo
	for i, c := range covs {
		if needsVisibility(reqs[i].Tool, c) {
			repos = append(repos, c.Repo)
		}
	}
	if len(repos) == 0 {
		return nil, nil
	}
	answers, err := e.public(ctx, repos)
	if err != nil {
		return nil, err
	}
	var problems []Problem
	seen := map[string]bool{}
	for i, c := range covs {
		if !needsVisibility(reqs[i].Tool, c) {
			continue
		}
		answer := answers[c.Repo]
		reason := visibilityReason(answer)
		if reason == "" {
			continue
		}
		covs[i] = Coverage{Reason: reason}
		if name := reqs[i].Tool.Name; reason == ReasonFailed && !seen[name] {
			seen[name] = true
			problems = append(problems, Problem{Source: SourceOSV, Tool: name, Err: answer})
		}
	}
	return problems, nil
}

// needsVisibility reports whether OSV may hear about c only after the forge says its repository
// is public: the user named the repository, and c asks about it.
func needsVisibility(t registry.ToolSpec, c Coverage) bool {
	return c.Checked() && t.UserRepo && c.Repo != (Repo{}) &&
		(forge.IsGitHub(c.Repo.Host) || strings.EqualFold(c.Repo.Host, forge.GitLab))
}
