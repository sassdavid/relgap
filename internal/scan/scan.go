package scan

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/execx"
)

const (
	maxFileSize = 2 << 20 // larger files are skipped
	binarySniff = 8000    // bytes checked for a NUL
	readsPerCPU = 4
)

// Options configures a [Scanner].
type Options struct {
	Exclude     []string // globs on repository-relative paths, ** for any directories
	ExtraFiles  []string // globs the generic detector reads
	Generic     bool     // run the generic detector on ExtraFiles
	Actions     bool     // report `uses: owner/action@ref` in CI files
	Submodules  bool     // scan checked-out submodules as repositories
	Concurrency int      // files read at once; 0 means 4 per CPU
	Report      []Pin    // pins from a Renovate report, resolved and merged with the rest
}

// Runner starts git; [execx.Runner] implements it.
type Runner interface {
	Run(ctx context.Context, c execx.Cmd) (*execx.Result, error)
}

// Namer maps a raw name to a tool name and is safe for concurrent use; [registry.Resolver]
// implements it.
type Namer interface {
	Identify(input string) (string, bool)
}

// Problem is a file that couldn't be parsed.
type Problem struct {
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Message string `json:"message"`
}

// Result is what a scan found, sorted.
type Result struct {
	Repos    []string  `json:"repos"`
	Pins     []Pin     `json:"pins"`
	Problems []Problem `json:"-"`
}

// Scanner finds pins in repositories.
type Scanner struct {
	logger *slog.Logger
	runner Runner
	namer  Namer
	opts   Options
}

// New returns a scanner that lists files with git through r and names tools with n.
func New(logger *slog.Logger, r Runner, n Namer, opts Options) *Scanner {
	if opts.Concurrency <= 0 {
		opts.Concurrency = readsPerCPU * runtime.GOMAXPROCS(0)
	}
	return &Scanner{logger: logger, runner: r, namer: n, opts: opts}
}

type task struct {
	repo      string // display form
	dir       string
	rel       string
	detectors []detector
}

type taskResult struct {
	pins     []Pin
	problems []Problem
}

// Scan finds the repositories under roots and the pins in their files.
func (s *Scanner) Scan(ctx context.Context, roots []string) (*Result, error) {
	var repos []string
	for _, root := range roots {
		found, err := findRepos(root)
		if err != nil {
			return nil, err
		}
		repos = append(repos, found...)
	}
	tasks, repos, err := s.plan(ctx, repos)
	if err != nil {
		return nil, err
	}
	results := make([]taskResult, len(tasks))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.opts.Concurrency)
	for i, t := range tasks {
		g.Go(func() error {
			if ctxErr := gctx.Err(); ctxErr != nil {
				return fmt.Errorf("scan: %w", ctxErr)
			}
			results[i] = s.run(gctx, t)
			return nil
		})
	}
	if err = g.Wait(); err != nil {
		return nil, err
	}
	res := &Result{Repos: repos, Pins: []Pin{}}
	for _, r := range results {
		res.Pins = append(res.Pins, r.pins...)
		res.Problems = append(res.Problems, r.problems...)
	}
	for _, p := range s.opts.Report {
		s.finish(&p, false)
		res.Pins = append(res.Pins, p)
	}
	slices.SortFunc(res.Pins, comparePins)
	slices.SortFunc(res.Problems, func(a, b Problem) int {
		return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.File, b.File), cmp.Compare(a.Message, b.Message))
	})
	return res, nil
}

// plan lists every repository's files, adds submodules as repositories and picks the detectors
// for each file. It returns the tasks and the repositories in display form, sorted.
func (s *Scanner) plan(ctx context.Context, pending []string) ([]task, []string, error) {
	detectors := s.opts.detectors()
	var (
		tasks []task
		repos []string
		seen  = map[string]bool{}
	)
	for len(pending) > 0 {
		listings := make([]listing, len(pending))
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(s.opts.Concurrency)
		for i, dir := range pending {
			g.Go(func() error {
				l, err := s.list(gctx, dir)
				listings[i] = l
				return err
			})
		}
		if err := g.Wait(); err != nil {
			return nil, nil, err
		}
		var next []string
		for i, dir := range pending {
			repo := filepath.ToSlash(filepath.Clean(dir))
			if seen[repo] {
				continue
			}
			seen[repo] = true
			repos = append(repos, repo)
			next = append(next, listings[i].submodules...)
			for _, rel := range listings[i].files {
				if t, ok := s.taskFor(detectors, repo, dir, rel); ok {
					tasks = append(tasks, t)
				}
			}
		}
		pending = next
	}
	slices.Sort(repos)
	return tasks, repos, nil
}

func (s *Scanner) taskFor(detectors []detector, repo, dir, rel string) (task, bool) {
	for _, pat := range s.opts.Exclude {
		if globMatch(pat, rel) {
			return task{}, false
		}
	}
	t := task{repo: repo, dir: dir, rel: rel}
	for _, d := range detectors {
		if d.match(rel) {
			t.detectors = append(t.detectors, d)
		}
	}
	return t, len(t.detectors) > 0
}

// run reads one file and runs its detectors. Huge, binary and vanished files are skipped.
func (s *Scanner) run(ctx context.Context, t task) taskResult {
	name := filepath.Join(t.dir, filepath.FromSlash(t.rel))
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return taskResult{}
	}
	if info.Size() > maxFileSize {
		s.logger.DebugContext(ctx, "skipping a file over 2 MB", "repo", t.repo, "file", t.rel)
		return taskResult{}
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return taskResult{problems: []Problem{{Repo: t.repo, File: t.rel, Message: err.Error()}}}
	}
	if bytes.IndexByte(data[:min(len(data), binarySniff)], 0) >= 0 {
		return taskResult{}
	}
	var r taskResult
	for _, d := range t.detectors {
		pins, detectErr := d.detect(file{rel: t.rel, data: data})
		if detectErr != nil {
			r.problems = append(
				r.problems,
				Problem{Repo: t.repo, File: t.rel, Message: d.name + ": " + detectErr.Error()},
			)
			continue
		}
		bareExact := d.name == DetectorTerraform || d.name == DetectorTFLock || d.name == DetectorTerragrunt
		for i := range pins {
			pins[i].Location.Repo, pins[i].Location.File, pins[i].Location.Detector = t.repo, t.rel, d.name
			s.finish(&pins[i], bareExact)
		}
		r.pins = append(r.pins, pins...)
	}
	return r
}

// finish classifies the value and names the tool. A tool pin tries its lookup name first, then
// its raw name.
func (s *Scanner) finish(p *Pin, bareExact bool) {
	p.classify(bareExact)
	if p.Tool != "" || p.Kind != KindTool {
		return
	}
	for _, name := range []string{p.lookup, p.RawName} {
		if name == "" {
			continue
		}
		if tool, ok := s.namer.Identify(name); ok {
			p.Tool = tool
			return
		}
	}
}
