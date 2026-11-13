package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/complete"
	"github.com/sassdavid/relgap/internal/config"
	"github.com/sassdavid/relgap/internal/render"
	"github.com/sassdavid/relgap/internal/report"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/version"
)

const (
	flagBase        = "base"
	flagHead        = "head"
	flagPullRequest = "pull-request"
	shortSHA        = 12
	sha1Hex         = 40 // length of a full SHA-1 commit ID
)

// prEvent is the part of a GitHub pull_request or pull_request_target event that digest reads.
type prEvent struct {
	PullRequest *struct {
		Body *string `json:"body"`
		Base struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	} `json:"pull_request"`
}

// prInput is what digest knows about the pull request: the refs to try as the base, in order,
// and the head branch and body when a GitHub event gave them.
type prInput struct {
	bases  []string
	branch string
	body   string
}

// addPRFlags adds --base, --head and --pull-request, which exclude the other ways to pick ranges.
func addPRFlags(cmd *cobra.Command, f *digestFlags) {
	fl := cmd.Flags()
	fl.StringVar(&f.base, flagBase, "",
		"digest the pins upgraded since this git ref (branch, tag or commit), against the working tree")
	fl.StringVar(&f.head, flagHead, "",
		"compare --base or --pull-request with this git ref instead of the working tree")
	fl.BoolVar(&f.pullRequest, flagPullRequest, false,
		"digest the pins the pull request in GITHUB_EVENT_PATH upgrades, from its base commit")
	registerCompletions(cmd, map[string]cobra.CompletionFunc{
		flagBase: complete.NoFiles,
		flagHead: complete.NoFiles,
	})
	for _, pr := range []string{flagBase, flagHead, flagPullRequest} {
		for _, other := range []string{flagFromMise, flagAll, flagLine} {
			cmd.MarkFlagsMutuallyExclusive(pr, other)
		}
	}
}

// readPREvent reads the pull request from the GITHUB_EVENT_PATH file.
func readPREvent(d *app.Deps) (prInput, error) {
	p, _ := d.Env.Lookup("GITHUB_EVENT_PATH")
	if p == "" {
		return prInput{}, usageError(errors.New("--pull-request needs GITHUB_EVENT_PATH, " +
			"which GitHub Actions sets; pass --base outside GitHub Actions"))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return prInput{}, usageError(fmt.Errorf("read GITHUB_EVENT_PATH: %w", err))
	}
	var ev prEvent
	if err = json.Unmarshal(data, &ev); err != nil {
		return prInput{}, usageError(fmt.Errorf("parse GITHUB_EVENT_PATH: %w", err))
	}
	pr := ev.PullRequest
	if pr == nil || pr.Base.SHA == "" {
		return prInput{}, usageError(errors.New("the GITHUB_EVENT_PATH event has no pull request base; " +
			"run on pull_request or pull_request_target, or pass --base"))
	}
	in := prInput{branch: pr.Head.Ref, bases: []string{pr.Base.SHA}}
	if pr.Body != nil {
		in.body = *pr.Body
	}
	if target, ok := scan.RenovateTargetBranch(in.body); ok && target != "" {
		in.bases = append(in.bases, "origin/"+target, target)
	}
	if pr.Base.Ref != "" {
		in.bases = append(in.bases, "origin/"+pr.Base.Ref, pr.Base.Ref)
	}
	return in, nil
}

// prDigest is one range to digest and the changed pins that ask for it.
type prDigest struct {
	tool, from, to string
	changes        []scan.Change
}

// runDigestPR digests the pins that a pull request or a pair of refs upgrades: the pins at
// the base ref against the head ref, or the working tree when --head is empty.
func runDigestPR(cmd *cobra.Command, d *app.Deps, g *globals, f digestFlags, root string) error {
	if err := g.checkFormats(digestFormats()...); err != nil {
		return usageError(err)
	}
	if f.base == "" && !f.pullRequest {
		return usageError(fmt.Errorf("--%s needs --%s or --%s", flagHead, flagBase, flagPullRequest))
	}
	kinds := make([]classify.Kind, len(f.only))
	for i, k := range f.only {
		kinds[i] = classify.Kind(k)
	}
	if err := checkKinds(kinds); err != nil {
		return err
	}
	in := prInput{}
	if f.pullRequest {
		var err error
		if in, err = readPREvent(d); err != nil {
			return err
		}
	}
	if f.base != "" {
		in.bases = []string{f.base}
	}
	ctx := cmd.Context()
	s, err := g.newSession(cmd, d)
	if err != nil {
		return err
	}
	defer s.close(ctx)
	gate, err := newGate(cmd, d, digestValues())
	if err != nil {
		return err
	}
	diff, err := diffRefs(ctx, s, root, in.bases, f.head)
	if err != nil {
		return err
	}
	changes := renovateFilter(d, in, diff.changes)
	jobs, skipped := planPRDigests(changes)
	tools, errs, notes, err := s.runPRDigests(ctx, jobs, digestArgs{
		line:               string(version.LineNone),
		includePrereleases: f.includePrereleases,
		minConfidence:      d.Config.Config.Policy.MinConfidence,
		kinds:              kinds,
	})
	if err != nil {
		return err
	}
	if err = writeDigest(cmd, d, g, &report.Digest{Tools: tools}, errs); err != nil {
		return err
	}
	prNotes(d, diff.base, diff.head, len(changes), append(skipped, notes...))
	var partial error
	if len(errs) > 0 {
		partial = fmt.Errorf("%d %s failed; the report is partial", len(errs), plural(len(errs), "digest", "digests"))
	}
	return gateExit(cmd, d, gate.Matches(gate.CheckDigests(tools)), partial)
}

// refDiff is the outcome of comparing the pins of two trees.
type refDiff struct {
	base, head string // as the user named them
	changes    []scan.Change
}

// diffRefs compares the pins of the first of bases that names a commit with those of head, or
// of the working tree when head is empty. Pins the config ignores drop out. It writes temporary
// files, so it isn't a session method: `relgap mcp` tools run through the session and never write.
func diffRefs(ctx context.Context, s *session, root string, bases []string, head string) (*refDiff, error) {
	baseRef, baseCommit, err := firstRef(ctx, s.runner, root, bases)
	if err != nil {
		return nil, err
	}
	basePins, err := scanRef(ctx, s, root, baseCommit)
	if err != nil {
		return nil, err
	}
	out := &refDiff{base: baseRef, head: "the working tree"}
	var headPins []scan.Pin
	if head != "" {
		_, headCommit, refErr := firstRef(ctx, s.runner, root, []string{head})
		if refErr != nil {
			return nil, refErr
		}
		out.head = head
		if headPins, err = scanRef(ctx, s, root, headCommit); err != nil {
			return nil, err
		}
	} else {
		res, scanErr := s.scan(ctx, []string{root}, false, root)
		if scanErr != nil {
			return nil, scanErr
		}
		headPins = res.Pins
	}
	ignore := ignoreRules(s.resolver, s.d.Config.Config.Ignore)
	for _, c := range scan.Diff(basePins, headPins) {
		if c.Head != nil && scan.Ignored(ignore, c.Head) || c.Base != nil && scan.Ignored(ignore, c.Base) {
			continue
		}
		out.changes = append(out.changes, c)
	}
	s.d.Logger.DebugContext(ctx, "pin diff", "base", baseRef, "commit", baseCommit, "head", out.head,
		"changes", len(out.changes))
	return out, nil
}

// firstRef returns the first of refs that names a commit, and that commit.
func firstRef(ctx context.Context, r commandRunner, dir string, refs []string) (string, string, error) {
	for _, ref := range refs {
		commit, err := scan.ResolveRef(ctx, r, dir, ref)
		if err == nil {
			return ref, commit, nil
		}
		if !errors.Is(err, scan.ErrRefNotFound) {
			return "", "", usageError(err)
		}
	}
	return "", "", usageError(fmt.Errorf("%s: none of %s is in this clone; fetch it "+
		"(actions/checkout with fetch-depth: 0) or pass --base", scan.ErrRefNotFound, strings.Join(refs, ", ")))
}

// scanRef scans the files that commit has under root, naming the repository "." as a scan of
// root does.
func scanRef(ctx context.Context, s *session, root, commit string) ([]scan.Pin, error) {
	tmp, err := os.MkdirTemp("", "relgap-ref-*")
	if err != nil {
		return nil, fmt.Errorf("create a directory for %s: %w", commit, err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err = scan.ExtractRef(ctx, s.runner, root, commit, tmp); err != nil {
		return nil, usageError(err)
	}
	res, err := s.scan(ctx, []string{tmp}, false, tmp)
	if err != nil {
		return nil, err
	}
	return res.Pins, nil
}

// renovateFilter keeps the upgrades a Renovate branch names, when the pull request comes from
// Renovate and its branch names at least one upgraded pin. Group branches name none, so they
// keep every change.
func renovateFilter(d *app.Deps, in prInput, changes []scan.Change) []scan.Change {
	if in.branch == "" {
		return changes
	}
	topic, prefixed := scan.RenovateTopic(in.branch)
	if _, commented := scan.RenovateTargetBranch(in.body); !prefixed && !commented {
		return changes
	}
	var keep, drop []scan.Change
	for _, c := range changes {
		if c.Upgrade() && scan.RenovateTopicMatches(topic, depNames(c.Head)...) {
			keep = append(keep, c)
		} else {
			drop = append(drop, c)
		}
	}
	if len(keep) == 0 {
		return changes
	}
	for _, c := range drop {
		_, _ = fmt.Fprintf(d.Stderr, "%s: not digested: %s: the Renovate branch %s names another dependency\n",
			config.AppName, changeText(c), in.branch)
	}
	return keep
}

// depNames lists the names Renovate may have used for a pin's dependency.
func depNames(p *scan.Pin) []string {
	names := []string{p.Tool, p.RawName}
	if _, name, ok := strings.Cut(p.Tool, ":"); ok {
		names = append(names, name)
	}
	if i := strings.LastIndex(p.Tool, "/"); i >= 0 {
		names = append(names, p.Tool[i+1:])
	}
	return names
}

// planPRDigests groups the upgrades by tool and range and says why each other change has no
// digest.
func planPRDigests(changes []scan.Change) ([]*prDigest, []string) {
	var jobs []*prDigest
	byRange := map[[3]string]*prDigest{}
	var notes []string
	for _, c := range changes {
		if !c.Upgrade() {
			notes = append(notes, changeText(c)+": "+changeReason(c))
			continue
		}
		k := [3]string{c.Head.Tool, c.Base.Version, c.Head.Version}
		j := byRange[k]
		if j == nil {
			j = &prDigest{tool: k[0], from: k[1], to: k[2]}
			byRange[k] = j
			jobs = append(jobs, j)
		}
		j.changes = append(j.changes, c)
	}
	return jobs, notes
}

// runPRDigests digests each range with the options in common. An auth error or a canceled context stops the run; a
// downgrade becomes a note and any other failure an envelope error.
func (s *session) runPRDigests(
	ctx context.Context,
	jobs []*prDigest,
	common digestArgs,
) ([]report.ToolDigest, []render.Error, []string, error) {
	results := make([]*report.ToolDigest, len(jobs))
	failures := make([]error, len(jobs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(max(1, s.d.Config.Config.GitHub.Concurrency))
	for i, j := range jobs {
		g.Go(func() error {
			a := common
			a.tool, a.from, a.to = j.tool, j.from, j.to
			res, err := s.digest(gctx, a)
			if err != nil {
				if ee, ok := errors.AsType[*exitError](err); ok && ee.code == ExitAuth || gctx.Err() != nil {
					return err
				}
				failures[i] = err
				return nil
			}
			results[i] = res
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, nil, err
	}
	tools := []report.ToolDigest{}
	errs := []render.Error{}
	var notes []string
	for i, j := range jobs {
		switch err := failures[i]; {
		case err == nil:
			tools = append(tools, *results[i])
		case errors.Is(err, version.ErrDowngrade):
			for _, c := range j.changes {
				notes = append(notes, changeText(c)+": a downgrade")
			}
		default:
			for _, c := range j.changes {
				errs = append(errs, render.Error{
					Target:  j.tool + "@" + render.PinLocation(c.Head.Location),
					Message: err.Error(),
				})
			}
		}
	}
	return tools, errs, notes, nil
}

// changeText names a changed pin, its values and where the head has it.
func changeText(c scan.Change) string {
	switch {
	case c.Base == nil:
		return fmt.Sprintf("%s %s (%s)", pinName(c.Head), c.Head.Value, render.PinLocation(c.Head.Location))
	case c.Head == nil:
		return fmt.Sprintf("%s %s (%s)", pinName(c.Base), c.Base.Value, render.PinLocation(c.Base.Location))
	default:
		return fmt.Sprintf("%s %s to %s (%s)", pinName(c.Head), c.Base.Value, c.Head.Value,
			render.PinLocation(c.Head.Location))
	}
}

func pinName(p *scan.Pin) string {
	if p.Tool != "" {
		return p.Tool
	}
	return p.RawName
}

func changeReason(c scan.Change) string {
	switch {
	case c.Base == nil:
		return "added"
	case c.Head == nil:
		return "removed"
	case !c.Head.Resolved():
		return "no known tool"
	default:
		return "not an exact version on both sides"
	}
}

// prNotes tells on stderr what the diff found and which changed pins have no digest.
func prNotes(d *app.Deps, base, head string, changed int, notes []string) {
	if changed == 0 {
		_, _ = fmt.Fprintf(d.Stderr, "%s: no pin changed between %s and %s\n", config.AppName, shortRef(base), head)
		return
	}
	for _, n := range notes {
		_, _ = fmt.Fprintf(d.Stderr, "%s: not digested: %s\n", config.AppName, n)
	}
}

// shortRef shortens a full commit ID.
func shortRef(ref string) string {
	if len(ref) == sha1Hex && strings.Trim(ref, "0123456789abcdef") == "" {
		return ref[:shortSHA]
	}
	return ref
}
