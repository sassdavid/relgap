package report

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/forge"
	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/mdindex"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/version"
)

// Digest is the data of `relgap digest --format json`.
type Digest struct {
	Tools   []ToolDigest `json:"tools"`
	Summary *Summary     `json:"summary,omitempty"` // only with --summarize
}

// SummaryNotice labels every LLM summary in every format.
const SummaryNotice = "LLM-generated, verify against cited sources"

// Summary is the optional LLM summary of a digest. It is advisory: no gate reads it.
type Summary struct {
	Notice   string  `json:"notice"` // always [SummaryNotice]
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Risks    []Claim `json:"risks"`   // at most 5
	Actions  []Claim `json:"actions"` // required actions
	Entries  int     `json:"entries"` // entries sent to the model
	Trimmed  int     `json:"trimmed"` // entries left out to fit llm.max_input_tokens
	Dropped  int     `json:"dropped"` // claims removed for citing no entry ID or an unknown one
}

// Claim is one line of a [Summary] and the entry IDs it cites.
type Claim struct {
	Text  string   `json:"text"`
	Cites []string `json:"cites"`
}

// ToolDigest is the digest of one tool over (From, To].
type ToolDigest struct {
	Tool     string                `json:"tool"`
	Repo     string                `json:"repo"`
	Host     string                `json:"host,omitempty"`  // the host of Repo when it isn't github.com
	Image    string                `json:"image,omitempty"` // registry/repository of an image tool
	From     string                `json:"from"`
	To       string                `json:"to"`
	Versions []Version             `json:"versions"` // oldest first
	Entries  []classify.Entry      `json:"entries"`  // by version, then document order
	Counts   map[classify.Kind]int `json:"counts"`   // entries at or above the minimum confidence, per kind, listed or not
	Hidden   int                   `json:"hidden"`   // entries below the minimum confidence
	Warnings []string              `json:"warnings"`
	Pins     []PinChange           `json:"pins,omitempty"` // the changed pins a --base or --pull-request digest covers
}

// PinChange is one changed pin that a digest covers: where the head has it, its raw name, and
// its value on each side as written.
type PinChange struct {
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	RawName string `json:"raw_name"`
	Base    string `json:"base"`
	Head    string `json:"head"`
}

// Version is one version in a digested range.
type Version struct {
	Version     string     `json:"version"`
	Tag         string     `json:"tag"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	URL         string     `json:"url,omitempty"` // the release page, when there is a release
}

// GitHub lists releases, tags and top-level files and fetches release bodies and files;
// [github.Client] implements it.
type GitHub interface {
	Releases(ctx context.Context, repo string) ([]github.Release, error)
	Tags(ctx context.Context, repo string) ([]string, error)
	source.ReleaseFetcher
	source.FileFetcher
	source.RootLister
}

// DigestRequest says what to digest.
type DigestRequest struct {
	Tool               registry.ToolSpec
	From, To           string // To empty means the latest stable version on the line
	Line               version.Level
	IncludePrereleases bool
	MinConfidence      float64
	Kinds              []classify.Kind // when set, Entries lists only these kinds; Counts still counts every kind
	Only               *int            // when set, only the source at this index of Tool.Sources runs
}

// HostClient returns the client of a git host other than GitHub, such as bitbucket.org.
type HostClient func(host string) (GitHub, error)

// TagLister lists tags with their dates; the Bitbucket and GitLab clients implement it.
type TagLister interface {
	TagRefs(ctx context.Context, repo string) ([]version.Ref, error)
}

// Digester builds digests from GitHub and the other git hosts.
type Digester struct {
	logger *slog.Logger
	gh     GitHub
	web    source.WebFetcher
	pages  *source.PageCache
	hosts  HostClient

	mu    sync.Mutex
	refs  map[refKey]*refList
	files map[string]*mdindex.Cache // by host; "" is GitHub
}

// refKey is what a version list is fetched by. Two tools can share a name, such as a chart and
// the application it installs, so the name isn't part of it.
type refKey struct{ host, repo, universe, versionsURL string }

// refList is a tool's version universe as listed once per digester.
type refList struct {
	once     sync.Once
	refs     []version.Ref
	releases map[string]github.Release
	err      error
}

// NewDigester returns a digester that reads from gh and, for web-doc and URL sources, from web.
// Changelog files and web pages are fetched and parsed once per digester, and so is each tool's
// list of releases or tags.
func NewDigester(logger *slog.Logger, gh GitHub, web source.WebFetcher) *Digester {
	return &Digester{
		logger: logger,
		gh:     gh,
		files:  map[string]*mdindex.Cache{"": source.NewFileCache(gh)},
		web:    web,
		pages:  source.NewPageCache(web),
		refs:   map[refKey]*refList{},
	}
}

// WithHosts returns d reading tools on other git hosts through hosts.
func (d *Digester) WithHosts(hosts HostClient) *Digester {
	d.hosts = hosts
	return d
}

// client returns the client and the file cache of the tool's host.
func (d *Digester) client(t registry.ToolSpec) (GitHub, *mdindex.Cache, error) {
	if forge.IsGitHub(t.Host) {
		return d.gh, d.files[""], nil
	}
	if d.hosts == nil {
		return nil, nil, fmt.Errorf("%s: relgap can't read %s here", t.Name, t.Host)
	}
	c, err := d.hosts(t.Host)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	key := strings.ToLower(t.Host)
	files, ok := d.files[key]
	if !ok {
		files = source.NewFileCache(c)
		d.files[key] = files
	}
	return c, files, nil
}

// Universe returns every version of the tool, listed once per digester.
func (d *Digester) Universe(ctx context.Context, t registry.ToolSpec) (*version.Universe, error) {
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs, _, err := d.list(ctx, t)
	if err != nil {
		return nil, err
	}
	return version.NewUniverse(ctx, d.logger, refs, rules), nil
}

func (d *Digester) list(ctx context.Context, t registry.ToolSpec) ([]version.Ref, map[string]github.Release, error) {
	k := refKey{
		host: strings.ToLower(t.Host), repo: strings.ToLower(t.Repo), universe: t.Version.Universe,
		versionsURL: t.Version.VersionsURL,
	}
	d.mu.Lock()
	l, ok := d.refs[k]
	if !ok {
		l = &refList{}
		d.refs[k] = l
	}
	d.mu.Unlock()
	l.once.Do(func() { l.refs, l.releases, l.err = d.fetchRefs(ctx, t) })
	return l.refs, l.releases, l.err
}

// reader is an adapter with the classifier for its source.
type reader struct {
	adapter    source.Adapter
	classifier *classify.Classifier
}

// Digest lists the tool's versions, selects the range and classifies the notes of each version.
func (d *Digester) Digest(ctx context.Context, req DigestRequest) (*ToolDigest, error) {
	t := req.Tool
	rules, err := t.Version.Rules()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs, releases, err := d.list(ctx, t)
	if err != nil {
		return nil, err
	}
	universe := version.NewUniverse(ctx, d.logger, refs, rules)
	r, err := universe.Select(version.RangeOptions{
		From: req.From, To: req.To, Line: req.Line, IncludePrereleases: req.IncludePrereleases,
	})
	if err != nil {
		if errors.Is(err, version.ErrNotFound) {
			err = fmt.Errorf("%w; %s", err, tagHint(t, refs, len(universe.Versions()), req.To))
		}
		return nil, fmt.Errorf("select %s range: %w", t.Name, err)
	}
	out := &ToolDigest{
		Tool:     t.Name,
		Repo:     t.Repo,
		Host:     t.Host,
		Image:    t.Image,
		From:     r.From.String(),
		To:       r.To.V.String(),
		Versions: make([]Version, 0, len(r.Versions)),
		Entries:  []classify.Entry{},
		Counts: map[classify.Kind]int{
			classify.KindBreaking: 0, classify.KindDeprecation: 0, classify.KindSecurity: 0,
			classify.KindFeature: 0, classify.KindFix: 0, classify.KindOther: 0,
		},
		Warnings: append([]string{}, r.Warnings...),
	}
	for _, v := range r.Versions {
		out.Versions = append(out.Versions, versionOf(v, releases[v.Tag]))
	}
	if t.Version.Universe == registry.UniverseChart {
		out.Warnings = append(out.Warnings, ChartVersionsNote(t))
		return out, nil
	}
	if t.Version.Universe == registry.UniverseImage {
		return d.imageNotes(ctx, req, r, out)
	}
	var from version.V
	if r.Base != nil {
		from = r.Base.V
	}
	readers, err := d.readers(ctx, t, from, newestStable(universe), req.Only)
	if err != nil {
		return nil, err
	}
	entries, err := collect(ctx, t, readers, r.Versions)
	if err != nil {
		return nil, err
	}
	for _, rd := range readers {
		if w, ok := rd.adapter.(interface{ Warnings() []string }); ok {
			out.Warnings = append(out.Warnings, w.Warnings()...)
		}
	}
	urls := map[string]string{}
	order := map[string]int{}
	for i, v := range out.Versions {
		urls[v.Version] = v.URL
		order[v.Version] = i
	}
	entries = append(classify.Jumps(rules.Scheme, r.From, r.Versions, entries, urls), entries...)
	slices.SortStableFunc(
		entries,
		func(a, b classify.Entry) int { return cmp.Compare(order[a.Version], order[b.Version]) },
	)
	assignIDs(t.Name, entries)
	out.add(entries, req, r)
	return out, nil
}

// add counts entries, keeps those at or above the minimum confidence and of the requested kinds,
// and warns when the range shows none.
func (out *ToolDigest) add(entries []classify.Entry, req DigestRequest, r *version.Range) {
	for _, e := range entries {
		if e.Confidence < req.MinConfidence {
			out.Hidden++
			continue
		}
		out.Counts[e.Kind]++
		out.Entries = append(out.Entries, e)
	}
	if len(r.Versions) > 0 && len(out.Entries) == 0 {
		out.Warnings = append(out.Warnings, noNotesWarning(req.Tool, r, out.Hidden, req.MinConfidence))
	}
	out.Entries = EntriesOf(out.Entries, req.Kinds)
}

// imageNotes fills out with the digest of the image's source repository over the same range. An
// image without one, or a source without the range's versions, keeps its versions without notes.
func (d *Digester) imageNotes(
	ctx context.Context,
	req DigestRequest,
	r *version.Range,
	out *ToolDigest,
) (*ToolDigest, error) {
	t := req.Tool
	if t.Notes == nil {
		out.Warnings = append(out.Warnings, ImageVersionsNote(t))
		return out, nil
	}
	from := ""
	if !r.From.IsZero() {
		from = r.From.String()
	}
	notes := req
	notes.Tool, notes.From, notes.To, notes.Only = *t.Notes, from, out.To, nil
	nd, err := d.Digest(ctx, notes)
	if errors.Is(err, version.ErrNotFound) {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"%s, the source repository of %s, has no versions %s..%s, so the digest shows the image's versions "+
				"without notes: %v", t.Notes.Repo, t.Image, cmp.Or(from, "start"), out.To, err))
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Entries, out.Counts, out.Hidden = nd.Entries, nd.Counts, nd.Hidden
	out.Warnings = append(out.Warnings, nd.Warnings...)
	return out, nil
}

// EntriesOf returns the entries of the given kinds in a new slice, or entries itself when
// kinds is empty.
func EntriesOf(entries []classify.Entry, kinds []classify.Kind) []classify.Entry {
	if len(kinds) == 0 {
		return entries
	}
	out := []classify.Entry{}
	for _, e := range entries {
		if slices.Contains(kinds, e.Kind) {
			out = append(out, e)
		}
	}
	return out
}

// collect runs every reader and drops entries an earlier source already had for the version,
// since a release body often repeats the changelog.
func collect(
	ctx context.Context,
	t registry.ToolSpec,
	readers []reader,
	vs []version.Version,
) ([]classify.Entry, error) {
	var out []classify.Entry
	seen := map[string]bool{}
	for _, rd := range readers {
		notes, err := rd.adapter.Fetch(ctx, t, vs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		for _, n := range notes {
			for _, e := range rd.classifier.Entries(n) {
				key := e.Version + "\x00" + normalize(e.Text)
				if !seen[key] {
					seen[key] = true
					out = append(out, e)
				}
			}
		}
	}
	return out, nil
}

// fetchRefs lists the version universe: releases (drafts included, the universe drops them) or
// tags.
func (d *Digester) fetchRefs(
	ctx context.Context,
	t registry.ToolSpec,
) ([]version.Ref, map[string]github.Release, error) {
	byTag := map[string]github.Release{}
	if t.Listed() {
		return slices.Clone(t.Versions), byTag, nil
	}
	gh, _, err := d.client(t)
	if err != nil {
		return nil, nil, err
	}
	if t.Version.Universe == registry.UniverseTags {
		if tl, ok := gh.(TagLister); ok {
			refs, tagErr := tl.TagRefs(ctx, t.Repo)
			if tagErr != nil {
				return nil, nil, fmt.Errorf("%s: %w", t.Name, tagErr)
			}
			return refs, byTag, nil
		}
		tags, tagsErr := gh.Tags(ctx, t.Repo)
		if tagsErr != nil {
			return nil, nil, fmt.Errorf("%s: %w", t.Name, tagsErr)
		}
		refs := make([]version.Ref, len(tags))
		for i, tag := range tags {
			refs[i] = version.Ref{Tag: tag}
		}
		return refs, byTag, nil
	}
	releases, err := gh.Releases(ctx, t.Repo)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
	}
	refs := make([]version.Ref, len(releases))
	for i, rel := range releases {
		refs[i] = version.Ref{Tag: rel.Tag, Draft: rel.Draft, Prerelease: rel.Prerelease, PublishedAt: rel.PublishedAt}
		byTag[rel.Tag] = rel
	}
	return refs, byTag, nil
}

// readers returns an adapter and classifier for each of the tool's sources, upgrade-notes first
// so its entries win over the same text in the changelog. Without sources it runs
// auto-discovery. from is the version the range starts after; newest is the newest stable one.
func (d *Digester) readers(
	ctx context.Context,
	t registry.ToolSpec,
	from, newest version.V,
	only *int,
) ([]reader, error) {
	gh, files, err := d.client(t)
	if err != nil {
		return nil, err
	}
	if len(t.Sources) == 0 {
		c, classifyErr := classify.New(classify.Options{BreakingMarkers: t.BreakingMarkers})
		if classifyErr != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, classifyErr)
		}
		a := source.NewAuto(d.logger, gh, gh, files, newest)
		return []reader{{adapter: a, classifier: c}}, nil
	}
	sources := slices.Clone(t.Sources)
	if only != nil {
		if *only < 0 || *only >= len(sources) {
			return nil, fmt.Errorf("%s has no source %d", t.Name, *only)
		}
		sources = sources[*only : *only+1]
	}
	slices.SortStableFunc(sources, func(a, b registry.Source) int {
		return cmp.Compare(boolInt(a.Kind != source.KindUpgradeNotes), boolInt(b.Kind != source.KindUpgradeNotes))
	})
	var out []reader
	for _, s := range sources {
		var a source.Adapter
		switch s.Kind {
		case source.KindGHRelease:
			a = source.NewGHRelease(gh)
		case source.KindChangelogFile:
			a = source.NewChangelogFile(d.logger, files, s)
		case source.KindStructuredJSON:
			a = source.NewStructuredJSON(d.logger, gh, d.web, s)
		case source.KindWebDoc:
			a = source.NewWebDoc(d.logger, d.pages, s, from)
		case source.KindUpgradeGuide:
			a = source.NewUpgradeGuide(d.logger, files, d.pages, s, from)
		case source.KindUpgradeNotes:
			a = source.NewUpgradeNotes(d.logger, files, s, changelogSource(t.Sources))
		default:
			d.logger.DebugContext(ctx, "skipping a source kind relgap can't read", "tool", t.Name, "kind", s.Kind)
			continue
		}
		c, classifyErr := classify.New(
			classify.Options{BreakingMarkers: t.BreakingMarkers, IgnoreSections: s.IgnoreSections},
		)
		if classifyErr != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, classifyErr)
		}
		out = append(out, reader{adapter: a, classifier: c})
	}
	return out, nil
}

// changelogSource returns the first changelog-file source, which upgrade-notes reads.
func changelogSource(sources []registry.Source) *registry.Source {
	for i := range sources {
		if sources[i].Kind == source.KindChangelogFile {
			return &sources[i]
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// newestStable returns the newest version that isn't a prerelease, or the zero version.
func newestStable(u *version.Universe) version.V {
	var newest version.V
	for _, v := range u.Versions() {
		if !v.Prerelease && (newest.IsZero() || u.Scheme().Compare(v.V, newest) > 0) {
			newest = v.V
		}
	}
	return newest
}

// ChartVersionsNote explains why a [registry.UniverseChart] tool has versions but no notes.
func ChartVersionsNote(t registry.ToolSpec) string {
	return fmt.Sprintf("%s has no tag per %s chart version, so the versions come from %s without notes; "+
		"for notes, pin the application's repository too (datasource=github-releases)",
		t.Repo, t.Name, t.Version.VersionsURL)
}

// ImageVersionsNote explains why a [registry.UniverseImage] tool has versions but no notes.
func ImageVersionsNote(t registry.ToolSpec) string {
	return fmt.Sprintf("%s has no source label that names a GitHub repository, so the versions come from %s "+
		"without notes", t.Image, t.Version.VersionsURL)
}

// noNotesWarning says that a range shows no entries, and why: it has none, or all of them are
// below the minimum confidence, as release bodies that only link to a docs site are.
func noNotesWarning(t registry.ToolSpec, r *version.Range, hidden int, minConfidence float64) string {
	if hidden == 0 {
		return "no release notes found; compare the versions at " + compareURL(t, r)
	}
	return fmt.Sprintf("all %d entries are below the minimum confidence %g; lower --min-confidence to see them, "+
		"or compare the versions at %s", hidden, minConfidence, compareURL(t, r))
}

// tagHint says which tags a missing version was looked for in. A wrong tag prefix hides every
// tag of the tool, so it names the prefix and up to 3 tags that hold the version.
func tagHint(t registry.ToolSpec, refs []version.Ref, parsed int, want string) string {
	where := cmp.Or(t.Image, t.Repo)
	hint := fmt.Sprintf("%d of %d tags of %s are versions", parsed, len(refs), where)
	switch p, sfx := t.Version.TagPrefix, t.Version.TagSuffix; {
	case sfx != "":
		hint = fmt.Sprintf("%d of %d tags of %s are versions between %q and %q", parsed, len(refs), where, p, sfx)
	case p != "":
		hint = fmt.Sprintf("%d of %d tags of %s are versions after the tag prefix %q", parsed, len(refs), where, p)
	}
	if want = strings.TrimPrefix(want, "v"); want != "" {
		var holding []string
		for _, r := range refs {
			if strings.Contains(r.Tag, want) && len(holding) < 3 {
				holding = append(holding, r.Tag)
			}
		}
		if len(holding) > 0 {
			hint += "; tags that hold " + want + ": " + strings.Join(holding, ", ")
		}
	}
	return hint + "; to read other tags, write a registry entry with version.tag_prefix and try it with " +
		"relgap registry test <file.yaml> <range>"
}

// compareURL returns the page comparing the version before the range with its end, on the
// tool's host.
func compareURL(t registry.ToolSpec, r *version.Range) string {
	if r.Base == nil {
		return forge.TagURL(t.Host, t.Repo, r.To.Tag)
	}
	return forge.CompareURL(t.Host, t.Repo, r.Base.Tag, r.To.Tag)
}

var (
	issueRef = regexp.MustCompile(`\]\([^)]*\)|#\d+\b|https?://\S+`)
	nonWord  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// normalize reduces an entry to its words without link targets, issue numbers or URLs, so the
// same bullet from two sources compares equal.
func normalize(text string) string {
	text = issueRef.ReplaceAllString(strings.ToLower(text), " ")
	return strings.TrimSpace(nonWord.ReplaceAllString(text, " "))
}

func versionOf(v version.Version, rel github.Release) Version {
	out := Version{Version: v.V.String(), Tag: v.Tag, URL: rel.URL}
	if !v.PublishedAt.IsZero() {
		at := v.PublishedAt.UTC()
		out.PublishedAt = &at
	}
	return out
}
