package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/version"
	curated "github.com/sassdavid/relgap/registry"
)

// Resolution errors.
var (
	ErrUnknownTool = errors.New("unknown tool")
	ErrNoRepo      = errors.New("no GitHub repository")
	ErrUnsupported = errors.New("unsupported tool syntax")
)

// Layer names where a field of a [ToolSpec] came from, highest precedence first.
type Layer string

// Resolution layers.
const (
	LayerExplicit Layer = "explicit"
	LayerConfig   Layer = "config"
	LayerRelgap   Layer = "relgap registry"
	LayerMise     Layer = "mise registry"
	LayerAqua     Layer = "aqua registry"
	LayerDefault  Layer = "default"
)

// Where mise's registry was read from.
const (
	MiseSourceLive     = "live"
	MiseSourceSnapshot = "snapshot"
)

const (
	backendAqua   = "aqua"
	backendGitHub = "github"
	fieldName     = "name" // provenance key
	byName        = "name" // Match.By for an exact name
)

// Match says how the input was identified.
type Match struct {
	By    string `json:"by"` // name, alias, config alias, env var name, repo or aqua package
	Value string `json:"value"`
	Layer Layer  `json:"layer"`
}

// Resolution is a resolved tool with the layer that supplied each field.
type Resolution struct {
	Input      string             `json:"input"`
	Match      Match              `json:"match"`
	Tool       ToolSpec           `json:"tool"`
	Provenance map[string][]Layer `json:"provenance"` // field path to layers
	MiseSource string             `json:"mise_source"`
}

// ConfigLayer is the part of the user config that takes part in resolution.
type ConfigLayer struct {
	Tools   map[string]string // tool name to owner/repo
	Aliases map[string]string // raw name to tool name
}

// Inputs are the layers a [Resolver] merges.
type Inputs struct {
	Config     ConfigLayer
	Entries    []*Entry
	Mise       []misex.RegistryEntry
	MiseSource string
	Aqua       []AquaEntry
}

// Resolver maps tool names to [ToolSpec]s.
type Resolver struct {
	in Inputs

	relgap      map[string]*Entry // by lowercase name and alias
	relgapRepo  map[string]*Entry // by lowercase repo
	config      map[string]string // lowercase tool name to canonical name
	mise        map[string]*misex.RegistryEntry
	aqua        map[string]*AquaEntry // by lowercase package name and alias
	aquaRepo    map[string][]*AquaEntry
	configAlias map[string]string
}

// MiseLister lists mise's registry; [misex.Client] implements it.
type MiseLister interface {
	Registry(ctx context.Context) ([]misex.RegistryEntry, error)
}

// Load builds a resolver from the embedded entries and snapshots. With a non-nil mise, the live
// `mise registry --json` replaces the mise snapshot when it works.
func Load(ctx context.Context, logger *slog.Logger, mise MiseLister, cfg ConfigLayer) (*Resolver, error) {
	l, err := NewLinter()
	if err != nil {
		return nil, err
	}
	entries, err := l.LoadFS(curated.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("load embedded registry: %w", err)
	}
	_, aqua, err := AquaSnapshot()
	if err != nil {
		return nil, err
	}
	in := Inputs{Config: cfg, Entries: entries, Aqua: aqua}
	if mise != nil {
		live, liveErr := mise.Registry(ctx)
		switch {
		case liveErr == nil:
			in.Mise, in.MiseSource = live, MiseSourceLive
		case errors.Is(liveErr, misex.ErrNotInstalled):
			logger.DebugContext(ctx, "mise not found; using the embedded mise registry snapshot")
		case ctx.Err() != nil:
			return nil, fmt.Errorf("read mise registry: %w", ctx.Err())
		default:
			logger.WarnContext(ctx, "mise registry failed; using the embedded snapshot", "error", liveErr)
		}
	}
	if in.Mise == nil {
		var meta SnapshotMeta
		if meta, in.Mise, err = MiseSnapshot(); err != nil {
			return nil, err
		}
		in.MiseSource = MiseSourceSnapshot
		logger.DebugContext(ctx, "using mise registry snapshot", "commit", meta.Commit, "date", meta.CommitDate)
	}
	return NewResolver(in), nil
}

// NewResolver indexes the layers in in.
func NewResolver(in Inputs) *Resolver {
	r := &Resolver{
		in:          in,
		relgap:      map[string]*Entry{},
		relgapRepo:  map[string]*Entry{},
		config:      map[string]string{},
		mise:        map[string]*misex.RegistryEntry{},
		aqua:        map[string]*AquaEntry{},
		aquaRepo:    map[string][]*AquaEntry{},
		configAlias: map[string]string{},
	}
	for _, e := range in.Entries {
		for _, n := range append([]string{e.Name}, e.Aliases...) {
			setOnce(r.relgap, strings.ToLower(n), e)
		}
		setOnce(r.relgapRepo, strings.ToLower(e.Repo), e)
	}
	for name := range in.Config.Tools {
		r.config[strings.ToLower(name)] = name
	}
	for raw, tool := range in.Config.Aliases {
		r.configAlias[strings.ToLower(raw)] = tool
	}
	r.indexMise()
	for i := range in.Aqua {
		a := &in.Aqua[i]
		for _, n := range append([]string{a.Name}, a.Aliases...) {
			setOnce(r.aqua, strings.ToLower(n), a)
		}
		if a.Repo != "" {
			repo := strings.ToLower(a.Repo)
			r.aquaRepo[repo] = append(r.aquaRepo[repo], a)
		}
	}
	return r
}

// Name is a tool name the resolver knows, with the layer that defines it.
type Name struct {
	Name  string
	Layer Layer
	Repo  string // owner/repo when the layer names one
}

// Names lists the tool names of the relgap registry, the config and mise's registry, without
// aliases. A name in several layers is listed once, from the first.
func (r *Resolver) Names() []Name {
	seen := map[string]bool{}
	var out []Name
	add := func(n Name) {
		if k := strings.ToLower(n.Name); n.Name != "" && !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	for _, e := range r.in.Entries {
		add(Name{Name: e.Name, Layer: LayerRelgap, Repo: e.Repo})
	}
	for _, name := range slices.Sorted(maps.Keys(r.in.Config.Tools)) {
		add(Name{Name: name, Layer: LayerConfig, Repo: r.in.Config.Tools[name]})
	}
	for i := range r.in.Mise {
		m := &r.in.Mise[i]
		if r.isPrimary(m) {
			repo, _ := r.miseRepo(m)
			add(Name{Name: m.Short, Layer: LayerMise, Repo: repo})
		}
	}
	slices.SortFunc(out, func(a, b Name) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// indexMise skips the extra entries `mise registry --json` prints for aliases, which repeat their
// tool's aliases, so the live registry and the snapshot index the same way.
func (r *Resolver) indexMise() {
	aliased := map[string]bool{}
	for _, m := range r.in.Mise {
		if !slices.Contains(m.Aliases, m.Short) {
			for _, a := range m.Aliases {
				aliased[a] = true
			}
		}
	}
	for i := range r.in.Mise {
		m := &r.in.Mise[i]
		if slices.Contains(m.Aliases, m.Short) && aliased[m.Short] {
			continue
		}
		setOnce(r.mise, strings.ToLower(m.Short), m)
	}
	for i := range r.in.Mise {
		m := &r.in.Mise[i]
		if r.isPrimary(m) {
			for _, a := range m.Aliases {
				setOnce(r.mise, strings.ToLower(a), m)
			}
		}
	}
}

// isPrimary reports whether m is the entry indexed under its own name, not an alias entry.
func (r *Resolver) isPrimary(m *misex.RegistryEntry) bool {
	return r.mise[strings.ToLower(m.Short)] == m
}

func setOnce[V any](m map[string]V, k string, v V) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

var (
	envVarName = regexp.MustCompile(`^([A-Za-z0-9_]+)_VERSION$`)
	repoRe     = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// Resolve identifies input and merges every layer that knows the tool. input is a name, an
// alias, a NAME_VERSION variable, github:owner/repo or aqua:package.
func (r *Resolver) Resolve(input string) (*Resolution, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("%w: empty name", ErrUnknownTool)
	}
	prefix, rest, explicit := strings.Cut(input, ":")
	var (
		name  string
		match Match
		pkg   *AquaEntry
		repo  string
		err   error
	)
	if explicit {
		name, match, pkg, repo, err = r.identifyExplicit(prefix, rest)
	} else {
		name, match, err = r.identifyName(input)
	}
	if err != nil {
		return nil, err
	}
	b := newBuilder()
	r.merge(b, name, pkg)
	b.setString("repo", &b.spec.Repo, repo, LayerExplicit)
	if b.spec.Name == "" {
		b.setString(fieldName, &b.spec.Name, path.Base(repo), LayerExplicit)
	}
	if b.spec.Repo == "" {
		return nil, fmt.Errorf("%w for %q: no registry has a GitHub repo for it%s; set tools.%s.repo in the config",
			ErrNoRepo, input, r.backendHint(name), b.spec.Name)
	}
	b.defaults()
	return &Resolution{
		Input:      input,
		Match:      match,
		Tool:       b.spec,
		Provenance: b.prov,
		MiseSource: r.in.MiseSource,
	}, nil
}

// Identify returns the name `relgap digest` accepts for input without merging layers: the
// registry name when a registry knows the tool, otherwise the github: or aqua: form itself.
func (r *Resolver) Identify(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	prefix, rest, explicit := strings.Cut(input, ":")
	if !explicit {
		name, _, err := r.identifyName(input)
		return name, err == nil
	}
	if prefix != backendGitHub && prefix != backendAqua {
		return "", false
	}
	name, match, pkg, repo, err := r.identifyExplicit(prefix, rest)
	switch {
	case err != nil:
		return "", false
	case repo != "":
		return "github:" + repo, true
	case match.Layer == LayerAqua:
		return "aqua:" + pkg.Name, true
	}
	return name, true
}

func (r *Resolver) identifyExplicit(prefix, rest string) (string, Match, *AquaEntry, string, error) {
	switch prefix {
	case backendGitHub:
		if !repoRe.MatchString(rest) {
			return "", Match{}, nil, "", fmt.Errorf("%w: github:%s is not github:owner/repo", ErrUnsupported, rest)
		}
		match := Match{By: "repo", Value: rest, Layer: LayerExplicit}
		if name, layer, ok := r.byRepo(rest); ok {
			match.Layer = layer
			return name, match, nil, "", nil
		}
		return "", match, nil, rest, nil
	case backendAqua:
		pkg := r.aqua[strings.ToLower(rest)]
		if pkg == nil {
			return "", Match{}, nil, "", fmt.Errorf(
				"%w: aqua package %q is not in the aqua registry snapshot",
				ErrUnknownTool,
				rest,
			)
		}
		match := Match{By: "aqua package", Value: pkg.Name, Layer: LayerAqua}
		if name, layer, ok := r.byRepo(pkg.Repo); ok && pkg.Repo != "" {
			match.Layer = layer
			return name, match, pkg, "", nil
		}
		if m := r.miseByBackend("aqua:" + pkg.Name); m != nil {
			match.Layer = LayerMise
			return m.Short, match, pkg, "", nil
		}
		return path.Base(pkg.Name), match, pkg, "", nil
	case "tfprovider":
		// Curated providers resolve without the Terraform registry: hashicorp/aws is
		// hashicorp/terraform-provider-aws by HashiCorp's naming rule.
		if ns, provider, ok := strings.Cut(
			rest,
			"/",
		); ok && ns != "" && provider != "" &&
			!strings.Contains(provider, "/") {
			if name, layer, found := r.byRepo(ns + "/terraform-provider-" + provider); found {
				return name, Match{By: "terraform provider", Value: rest, Layer: layer}, nil, "", nil
			}
		}
		return "", Match{}, nil, "", fmt.Errorf(
			"%w: tfprovider: only curated providers resolve; other names need a lookup in the Terraform "+
				"registry, which relgap doesn't do yet (use github:<namespace>/terraform-provider-<name>)",
			ErrUnsupported,
		)
	case "chart", "npm", "pypi":
		return "", Match{}, nil, "", fmt.Errorf(
			"%w: %s: names need a lookup in the %s registry, which relgap doesn't do yet",
			ErrUnsupported,
			prefix,
			prefix,
		)
	default:
		return "", Match{}, nil, "", fmt.Errorf(
			"%w: unknown prefix %q (relgap knows github and aqua)",
			ErrUnsupported,
			prefix+":",
		)
	}
}

func (r *Resolver) identifyName(input string) (string, Match, error) {
	if tool, ok := r.configAlias[strings.ToLower(input)]; ok {
		if name, _, found := r.canonical(tool); found {
			return name, Match{By: "config alias", Value: input, Layer: LayerConfig}, nil
		}
		return "", Match{}, fmt.Errorf(
			"%w: config alias %s points to %q, which no registry knows",
			ErrUnknownTool,
			input,
			tool,
		)
	}
	if name, match, ok := r.canonical(input); ok {
		return name, match, nil
	}
	if m := envVarName.FindStringSubmatch(input); m != nil {
		guess := strings.ReplaceAll(strings.ToLower(m[1]), "_", "-")
		if name, match, ok := r.canonical(guess); ok {
			match.By, match.Value = "env var name", input
			return name, match, nil
		}
	}
	return "", Match{}, fmt.Errorf("%w %q: not in the config, the relgap registry or the mise registry; "+
		"use github:owner/repo or aqua:package, or set tools.<name>.repo in the config", ErrUnknownTool, input)
}

// canonical returns the tool name that name or alias stands for, checking layers in order.
func (r *Resolver) canonical(input string) (string, Match, bool) {
	key := strings.ToLower(input)
	by := func(name string) string {
		if strings.EqualFold(name, input) {
			return byName
		}
		return "alias"
	}
	if name, ok := r.config[key]; ok {
		return name, Match{By: byName, Value: input, Layer: LayerConfig}, true
	}
	if e, ok := r.relgap[key]; ok {
		return e.Name, Match{By: by(e.Name), Value: input, Layer: LayerRelgap}, true
	}
	if m, ok := r.mise[key]; ok {
		if e, isCurated := r.relgap[strings.ToLower(m.Short)]; isCurated {
			return e.Name, Match{By: "alias", Value: input, Layer: LayerMise}, true
		}
		return m.Short, Match{By: by(m.Short), Value: input, Layer: LayerMise}, true
	}
	return "", Match{}, false
}

// byRepo finds the tool whose repository is repo.
func (r *Resolver) byRepo(repo string) (string, Layer, bool) {
	key := strings.ToLower(repo)
	for name, rp := range r.in.Config.Tools {
		if strings.EqualFold(rp, repo) {
			return name, LayerConfig, true
		}
	}
	if e, ok := r.relgapRepo[key]; ok {
		return e.Name, LayerRelgap, true
	}
	var found []string
	for i := range r.in.Mise {
		m := &r.in.Mise[i]
		if !r.isPrimary(m) {
			continue
		}
		if rp, _ := r.miseRepo(m); strings.EqualFold(rp, repo) {
			found = append(found, m.Short)
		}
	}
	if len(found) == 1 {
		return found[0], LayerMise, true
	}
	return "", "", false
}

func (r *Resolver) miseByBackend(backend string) *misex.RegistryEntry {
	for i := range r.in.Mise {
		m := &r.in.Mise[i]
		if r.isPrimary(m) &&
			slices.ContainsFunc(m.Backends, func(b string) bool { return strings.EqualFold(b, backend) }) {
			return m
		}
	}
	return nil
}

// miseRepo returns the repository of the first aqua:, github: or ubi: backend that has one, and
// the layer that knows it: an aqua: backend is looked up in the aqua index.
func (r *Resolver) miseRepo(m *misex.RegistryEntry) (string, Layer) {
	for _, b := range m.Backends {
		kind, rest, _ := strings.Cut(b, ":")
		switch kind {
		case backendAqua:
			if a := r.aqua[strings.ToLower(rest)]; a != nil && a.Repo != "" {
				return a.Repo, LayerAqua
			}
		case backendGitHub, "ubi":
			rest, _, _ = strings.Cut(rest, "[") // ubi:owner/repo[exe=x]
			if repoRe.MatchString(rest) {
				return rest, LayerMise
			}
		}
	}
	return "", ""
}

func (r *Resolver) miseAqua(m *misex.RegistryEntry) *AquaEntry {
	for _, b := range m.Backends {
		if rest, ok := strings.CutPrefix(b, "aqua:"); ok {
			if a := r.aqua[strings.ToLower(rest)]; a != nil {
				return a
			}
		}
	}
	return nil
}

func (r *Resolver) backendHint(name string) string {
	m := r.mise[strings.ToLower(name)]
	if m == nil || len(m.Backends) == 0 {
		return ""
	}
	return " (mise backends: " + strings.Join(m.Backends, ", ") + ")"
}

// merge fills b from every layer that knows name, highest precedence first.
func (r *Resolver) merge(b *builder, name string, pkg *AquaEntry) {
	if name == "" {
		return
	}
	key := strings.ToLower(name)
	cfgName, inConfig := r.config[key]
	e, m := r.relgap[key], r.mise[key]
	switch {
	case inConfig:
		b.setString(fieldName, &b.spec.Name, name, LayerConfig)
	case e != nil:
		b.setString(fieldName, &b.spec.Name, name, LayerRelgap)
	case m != nil:
		b.setString(fieldName, &b.spec.Name, name, LayerMise)
	default: // only aqua:package reaches here without another layer
		b.setString(fieldName, &b.spec.Name, name, LayerAqua)
	}
	if inConfig {
		b.setString("repo", &b.spec.Repo, r.in.Config.Tools[cfgName], LayerConfig)
	}
	if e != nil {
		b.fromEntry(e)
	}
	if m != nil {
		b.addAliases(append([]string{m.Short}, m.Aliases...), LayerMise)
		if repo, layer := r.miseRepo(m); repo != "" {
			b.setString("repo", &b.spec.Repo, repo, layer)
		}
		if pkg == nil {
			pkg = r.miseAqua(m)
		}
	}
	if pkg == nil && b.spec.Repo != "" {
		if same := r.aquaRepo[strings.ToLower(b.spec.Repo)]; len(same) == 1 {
			pkg = same[0]
		}
	}
	if pkg != nil {
		b.fromAqua(pkg)
	}
}

// builder collects fields and records the layer that set each one.
type builder struct {
	spec ToolSpec
	prov map[string][]Layer
}

func newBuilder() *builder {
	return &builder{prov: map[string][]Layer{}}
}

func (b *builder) setString(key string, dst *string, val string, l Layer) {
	if *dst == "" && val != "" {
		*dst = val
		b.prov[key] = []Layer{l}
	}
}

func (b *builder) addAliases(names []string, l Layer) {
	added := false
	for _, n := range names {
		if strings.EqualFold(n, b.spec.Name) || slices.ContainsFunc(b.spec.Aliases, func(a string) bool {
			return strings.EqualFold(a, n)
		}) {
			continue
		}
		b.spec.Aliases = append(b.spec.Aliases, n)
		added = true
	}
	if added {
		b.prov["aliases"] = append(b.prov["aliases"], l)
	}
}

func (b *builder) fromEntry(e *Entry) {
	l := LayerRelgap
	b.addAliases(e.Aliases, l)
	b.setString("repo", &b.spec.Repo, e.Repo, l)
	v := &b.spec.Version
	b.setString("version.scheme", &v.Scheme, e.Version.Scheme, l)
	b.setString("version.extract", &v.Extract, e.Version.Extract, l)
	b.setString("version.tag_prefix", &v.TagPrefix, e.Version.TagPrefix, l)
	b.setString("version.exclude", &v.Exclude, e.Version.Exclude, l)
	b.setString("version.universe", &v.Universe, e.Version.Universe, l)
	b.setString("version.line", &v.Line, e.Version.Line, l)
	if len(b.spec.Sources) == 0 && len(e.Sources) > 0 {
		b.spec.Sources = slices.Clone(e.Sources)
		b.prov["sources"] = []Layer{l}
	}
	if b.spec.EOL == nil && e.EOL != nil {
		eol := *e.EOL
		b.spec.EOL = &eol
		b.prov["eol"] = []Layer{l}
	}
	if len(b.spec.OSV) == 0 && len(e.OSV) > 0 {
		b.spec.OSV = slices.Clone(e.OSV)
		b.prov["osv"] = []Layer{l}
	}
	if len(b.spec.BreakingMarkers) == 0 && len(e.BreakingMarkers) > 0 {
		b.spec.BreakingMarkers = slices.Clone(e.BreakingMarkers)
		b.prov["breaking_markers"] = []Layer{l}
	}
	if len(b.spec.Bin) == 0 && len(e.Bin) > 0 {
		b.spec.Bin = slices.Clone(e.Bin)
		b.prov["bin"] = []Layer{l}
	}
}

// startsWith matches the one aqua version_filter form that maps to a tag prefix.
var startsWith = regexp.MustCompile(`^Version startsWith "([^"\\]+)"$`)

func (b *builder) fromAqua(a *AquaEntry) {
	l := LayerAqua
	b.setString("repo", &b.spec.Repo, a.Repo, l)
	prefix := a.VersionPrefix
	if prefix == "" {
		if m := startsWith.FindStringSubmatch(a.VersionFilter); m != nil {
			prefix = m[1]
		}
	}
	b.setString("version.tag_prefix", &b.spec.Version.TagPrefix, prefix, l)
	if a.VersionSource == "github_tag" {
		b.setString("version.universe", &b.spec.Version.Universe, UniverseTags, l)
	}
}

func (b *builder) defaults() {
	v := &b.spec.Version
	b.setString("version.scheme", &v.Scheme, version.SchemeLoose, LayerDefault)
	b.setString("version.universe", &v.Universe, UniverseReleases, LayerDefault)
	b.setString("version.line", &v.Line, string(version.LineNone), LayerDefault)
	if b.spec.Sources == nil {
		b.spec.Sources = []Source{}
		b.prov["sources"] = []Layer{LayerDefault}
	}
	if b.spec.Aliases == nil {
		b.spec.Aliases = []string{}
	}
	if b.spec.OSV == nil {
		b.spec.OSV = []OSVPackage{}
	}
	if b.spec.BreakingMarkers == nil {
		b.spec.BreakingMarkers = []string{}
	}
	if b.spec.Bin == nil {
		b.spec.Bin = []string{}
	}
}

// SpecFromEntry returns the tool spec of one entry on its own, with defaults for the fields it
// leaves out, as `relgap registry test` runs a file that isn't embedded yet.
func SpecFromEntry(e *Entry) ToolSpec {
	b := newBuilder()
	b.spec.Name = e.Name
	b.fromEntry(e)
	b.defaults()
	return b.spec
}
