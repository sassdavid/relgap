package registry

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/forge"
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

	LayerTerraform   Layer = "terraform registry"
	LayerArtifactHub Layer = "artifact hub"
	LayerHelmRepo    Layer = "helm repository"
	LayerNPM         Layer = "npm registry"
	LayerPyPI        Layer = "pypi"
	LayerImage       Layer = "image registry"
	LayerGuess       Layer = "github search"
)

// Where a relgap registry entry was read from.
const (
	RelgapSourceEmbedded = "embedded"
	RelgapSourceUpdated  = "updated"
)

// ConfidenceLow marks a resolution that `--guess` made from a GitHub search.
const ConfidenceLow = "low"

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
	Input        string             `json:"input"`
	Match        Match              `json:"match"`
	Tool         ToolSpec           `json:"tool"`
	Provenance   map[string][]Layer `json:"provenance"` // field path to layers
	MiseSource   string             `json:"mise_source"`
	RelgapSource string             `json:"relgap_source,omitempty"` // set when a relgap entry took part
	Confidence   string             `json:"confidence,omitempty"`    // low for a guess
	Alternatives []string           `json:"alternatives,omitempty"`  // other repositories a guess matched
}

// ResolveOptions say what [Resolver.Resolve] may do beyond the embedded registries.
type ResolveOptions struct {
	Finder Finder // nil resolves offline: tfprovider:, tfmodule:, chart:, npm: and pypi: then resolve only from registries
	Guess  bool   // search GitHub for an unknown name; needs Finder
}

// ConfigLayer is the part of the user config that takes part in resolution.
type ConfigLayer struct {
	Tools       map[string]string // tool name to owner/repo, or the project path on its host
	Hosts       map[string]string // tool name to its host when it isn't github.com
	Aliases     map[string]string // raw name to tool name
	GitLabHosts []string          // self-managed GitLab hosts that gitlab:<host>/<path> may name
}

// Inputs are the layers a [Resolver] merges.
type Inputs struct {
	Config     ConfigLayer
	Entries    []*Entry
	Updated    []*Entry // from `relgap registry update`; each replaces the entry of the same name
	Mise       []misex.RegistryEntry
	MiseSource string
	Aqua       []AquaEntry
}

// Resolver maps tool names to [ToolSpec]s.
type Resolver struct {
	in Inputs

	relgap      map[string]*Entry // by lowercase name and alias
	updated     map[*Entry]bool
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
	return LoadWith(ctx, logger, mise, cfg, LoadOptions{})
}

// LoadOptions add the entries of `relgap registry update` to [LoadWith].
type LoadOptions struct {
	UpdatedDir string // directory of updated entries; empty or missing means none
	Version    string // relgap's version, checked against an entry's min_relgap
}

// LoadWith is [Load] with the updated entries in o.UpdatedDir on top of the embedded ones.
func LoadWith(
	ctx context.Context,
	logger *slog.Logger,
	mise MiseLister,
	cfg ConfigLayer,
	o LoadOptions,
) (*Resolver, error) {
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
	if o.UpdatedDir != "" {
		in.Updated = LoadUpdated(ctx, logger, l, o.UpdatedDir, o.Version)
	}
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
		updated:     map[*Entry]bool{},
		relgapRepo:  map[string]*Entry{},
		config:      map[string]string{},
		mise:        map[string]*misex.RegistryEntry{},
		aqua:        map[string]*AquaEntry{},
		aquaRepo:    map[string][]*AquaEntry{},
		configAlias: map[string]string{},
	}
	r.in.Entries = mergeUpdated(in.Entries, in.Updated)
	for _, e := range in.Updated {
		r.updated[e] = true
	}
	for _, e := range r.in.Entries {
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

// identity is what identifying an input found, before the layers merge.
type identity struct {
	name  string // a tool a layer knows; empty when only repo is known
	match Match
	pkg   *AquaEntry
	repo  string // owner/repo when no layer knows the tool
	host  string // the host of repo when it isn't github.com
	found *Found // set when a lookup supplied repo
}

// Resolve identifies input and merges every layer that knows the tool. input is a name, an
// alias, a NAME_VERSION variable, github:owner/repo, bitbucket:workspace/repo, gitlab:path,
// aqua:package, tfprovider:namespace/name, tfmodule:namespace/name/provider, chart:repo/name,
// chart:name, chart:url/name, npm:package, pypi:package or image:name.
func (r *Resolver) Resolve(ctx context.Context, input string, opts ResolveOptions) (*Resolution, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("%w: empty name", ErrUnknownTool)
	}
	id, tagPrefix, err := r.identify(ctx, input, opts)
	if err != nil {
		return nil, err
	}
	b := newBuilder()
	r.merge(b, id.name, id.pkg)
	repoLayer, foundName := LayerExplicit, ""
	if id.found != nil {
		repoLayer, foundName = id.found.Layer, id.found.Name
	}
	b.setString("repo", &b.spec.Repo, id.repo, repoLayer)
	b.setString("host", &b.spec.Host, id.host, repoLayer)
	if b.spec.Name == "" {
		b.setString(fieldName, &b.spec.Name, cmp.Or(foundName, path.Base(id.repo)), repoLayer)
	}
	image := id.found != nil && id.found.Image != ""
	if b.spec.Repo == "" && !image {
		return nil, fmt.Errorf("%w for %q: no registry has a GitHub repo for it%s; set tools.%s.repo in the config",
			ErrNoRepo, input, r.backendHint(id.name), b.spec.Name)
	}
	if id.found != nil {
		b.fromFound(id.found)
	}
	if !forge.IsGitHub(b.spec.Host) {
		// Bitbucket Cloud has no releases, and a GitLab project's tags exist with or without one.
		b.setString("version.universe", &b.spec.Version.Universe, UniverseTags, LayerDefault)
	}
	if tagPrefix != "" {
		b.spec.Version.TagPrefix = tagPrefix
		b.prov["version.tag_prefix"] = []Layer{LayerExplicit}
	}
	b.defaults()
	if image && b.spec.Repo != "" {
		notes, notesErr := r.Resolve(ctx, backendGitHub+":"+b.spec.Repo, ResolveOptions{})
		if notesErr != nil {
			return nil, fmt.Errorf("resolve the source repository of %s: %w", input, notesErr)
		}
		b.spec.Notes = &notes.Tool
	}
	res := &Resolution{
		Input:        input,
		Match:        id.match,
		Tool:         b.spec,
		Provenance:   b.prov,
		MiseSource:   r.in.MiseSource,
		RelgapSource: r.relgapSource(id.name),
	}
	if id.match.Layer == LayerGuess {
		res.Confidence = ConfidenceLow
		if id.found != nil {
			res.Alternatives = id.found.Alternatives
		}
	}
	return res, nil
}

// identify finds what input names, and the tag prefix of a github:, bitbucket: or gitlab: name.
func (r *Resolver) identify(ctx context.Context, input string, opts ResolveOptions) (identity, string, error) {
	prefix, rest, explicit := strings.Cut(input, ":")
	var (
		id        identity
		err       error
		tagPrefix string
	)
	switch {
	case r.isConfigAlias(input):
		id.name, id.match, err = r.identifyName(input)
	case explicit && isLookupPrefix(prefix):
		id, err = r.identifyLookup(ctx, prefix, rest, opts.Finder)
	case explicit && isForgePrefix(prefix):
		var forgeRest string
		forgeRest, tagPrefix, _ = SplitTagPrefix(rest)
		id, err = r.identifyForge(prefix, forgeRest)
	case explicit:
		explicitRest := rest
		if prefix == backendGitHub {
			explicitRest, tagPrefix, _ = SplitTagPrefix(rest)
		}
		id.name, id.match, id.pkg, id.repo, err = r.identifyExplicit(prefix, explicitRest)
	default:
		id.name, id.match, err = r.identifyName(input)
		if errors.Is(err, ErrUnknownTool) && opts.Guess && opts.Finder != nil {
			id, err = r.identifyGuess(ctx, input, opts.Finder, err)
		}
	}
	return id, tagPrefix, err
}

func (r *Resolver) relgapSource(name string) string {
	e := r.relgap[strings.ToLower(name)]
	switch {
	case name == "" || e == nil:
		return ""
	case r.updated[e]:
		return RelgapSourceUpdated
	}
	return RelgapSourceEmbedded
}

func isLookupPrefix(prefix string) bool {
	switch prefix {
	case KindTFProvider, KindTFModule, KindChart, KindNPM, KindPyPI, KindImage:
		return true
	}
	return false
}

func isForgePrefix(prefix string) bool { return prefix == KindBitbucket || prefix == KindGitLab }

var projectPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// minProjectParts is the number of parts in the shortest project path: a group or workspace and
// the repository.
const minProjectParts = 2

// splitForge returns the host and repository of bitbucket:workspace/repo, gitlab:group/.../repo
// and gitlab:host/group/.../repo, where host must be in the config's GitLab hosts.
func (r *Resolver) splitForge(prefix, rest string) (string, string, error) {
	host, repo := forge.Bitbucket, rest
	if prefix == KindGitLab {
		host = forge.GitLab
		if first, tail, ok := strings.Cut(rest, "/"); ok && slices.ContainsFunc(r.in.Config.GitLabHosts,
			func(h string) bool { return strings.EqualFold(h, first) }) {
			host, repo = strings.ToLower(first), tail
		}
	}
	parts := strings.Split(repo, "/")
	ok := len(parts) >= minProjectParts &&
		!slices.ContainsFunc(parts, func(p string) bool { return !projectPart.MatchString(p) })
	if prefix == KindBitbucket && len(parts) != 2 {
		ok = false
	}
	if !ok {
		want := "bitbucket:<workspace>/<repo>"
		if prefix == KindGitLab {
			want = "gitlab:<group>/<repo>, or gitlab:<host>/<group>/<repo> for a host in gitlab.hosts"
		}
		return "", "", fmt.Errorf("%w: %s:%s is not %s", ErrUnsupported, prefix, rest, want)
	}
	return host, repo, nil
}

// identifyForge identifies bitbucket: and gitlab: names, which need no lookup.
func (r *Resolver) identifyForge(prefix, rest string) (identity, error) {
	host, repo, err := r.splitForge(prefix, rest)
	if err != nil {
		return identity{}, err
	}
	match := Match{By: "repo", Value: prefix + ":" + rest, Layer: LayerExplicit}
	if name, ok := r.configByHostRepo(host, repo); ok {
		match.Layer = LayerConfig
		return identity{name: name, match: match}, nil
	}
	return identity{match: match, repo: repo, host: host}, nil
}

// configByHostRepo finds the config tool whose repository is repo on host.
func (r *Resolver) configByHostRepo(host, repo string) (string, bool) {
	for name, rp := range r.in.Config.Tools {
		if strings.EqualFold(rp, repo) && strings.EqualFold(cmp.Or(r.in.Config.Hosts[name], forge.GitHub), host) {
			return name, true
		}
	}
	return "", false
}

// forgeName returns the bitbucket: or gitlab: name of repo on host.
func forgeName(host, repo, tagPrefix string) string {
	name := KindGitLab + ":" + repo
	switch {
	case forge.KindOf(host) == forge.KindBitbucket:
		name = KindBitbucket + ":" + repo
	case !strings.EqualFold(host, forge.GitLab):
		name = KindGitLab + ":" + host + "/" + repo
	}
	if tagPrefix != "" {
		name += "@" + tagPrefix
	}
	return name
}

// tfModuleParts is the number of parts in a registry module address: namespace, name, provider.
const tfModuleParts = 3

// identifyLookup identifies tfprovider:, tfmodule:, chart:, npm: and pypi: names. The registries answer
// first, so curated tools resolve offline; finder asks the package registry otherwise.
func (r *Resolver) identifyLookup(ctx context.Context, prefix, rest string, finder Finder) (identity, error) {
	var (
		by      string
		offline func() (string, Layer, bool)
		service string
	)
	switch prefix {
	case KindTFProvider:
		by, service = "terraform provider", "the Terraform registry"
		ns, provider, ok := strings.Cut(rest, "/")
		if !ok || ns == "" || provider == "" || strings.Contains(provider, "/") {
			return identity{}, fmt.Errorf(
				"%w: tfprovider:%s is not tfprovider:<namespace>/<name>",
				ErrUnsupported,
				rest,
			)
		}
		// hashicorp/aws is hashicorp/terraform-provider-aws by HashiCorp's naming rule.
		offline = func() (string, Layer, bool) { return r.byRepo(ns + "/terraform-provider-" + provider) }
	case KindTFModule:
		by, service = "terraform module", "the Terraform registry"
		if parts := strings.Split(rest, "/"); len(parts) != tfModuleParts || slices.Contains(parts, "") {
			return identity{}, fmt.Errorf(
				"%w: tfmodule:%s is not tfmodule:<namespace>/<name>/<provider>",
				ErrUnsupported,
				rest,
			)
		}
	case KindChart:
		by, service = "chart", "Artifact Hub"
	case KindNPM:
		by, service = "npm package", "the npm registry"
		offline = func() (string, Layer, bool) { return r.byPackage("npm", "npm:", rest) }
	case KindPyPI:
		by, service = "pypi package", "PyPI"
		offline = func() (string, Layer, bool) { return r.byPackage("PyPI", "pipx:", rest) }
	case KindImage:
		by, service = "image", "the image registry"
		if _, _, _, ok := SplitImage(rest); !ok {
			return identity{}, fmt.Errorf("%w: image:%s is not image:<name> or image:<name>:<prefix>*<suffix>",
				ErrUnsupported, rest)
		}
	}
	if rest == "" {
		return identity{}, fmt.Errorf("%w: %s: needs a name", ErrUnsupported, prefix)
	}
	match := Match{By: by, Value: rest}
	if offline != nil {
		if name, layer, ok := offline(); ok {
			match.Layer = layer
			return identity{name: name, match: match}, nil
		}
	}
	if finder == nil {
		return identity{}, fmt.Errorf("%w: %s:%s needs a lookup in %s, which this command doesn't make",
			ErrUnsupported, prefix, rest, service)
	}
	f, err := finder.Find(ctx, prefix, rest)
	if err != nil {
		return identity{}, err //nolint:wrapcheck // lookup errors name the package and the service
	}
	match.Layer = f.Layer
	// A chart's repository can be the application's own, whose curated entry describes app
	// versions, so a chart never takes another tool's entry. An image's versions are its tags,
	// and its source repository becomes the tool in Notes instead.
	if prefix != KindChart && prefix != KindImage {
		if name, layer, ok := r.byRepo(f.Repo); ok {
			match.Layer = layer
			return identity{name: name, match: match}, nil
		}
	}
	return identity{match: match, repo: f.Repo, found: f}, nil
}

// identifyGuess searches GitHub for a repository named input, after notFound.
func (r *Resolver) identifyGuess(ctx context.Context, input string, finder Finder, notFound error) (identity, error) {
	f, err := finder.Find(ctx, KindGuess, input)
	if err != nil {
		return identity{}, fmt.Errorf("%w; --guess: %w", notFound, err)
	}
	match := Match{By: "guess", Value: input, Layer: LayerGuess}
	if name, _, ok := r.byRepo(f.Repo); ok {
		return identity{name: name, match: match, found: f}, nil
	}
	return identity{match: match, repo: f.Repo, found: f}, nil
}

// byPackage finds the tool a package of a package registry belongs to: a curated entry that
// lists it among its OSV packages, or a mise entry with that backend.
func (r *Resolver) byPackage(ecosystem, miseBackend, pkg string) (string, Layer, bool) {
	for _, e := range r.in.Entries {
		if slices.ContainsFunc(e.OSV, func(o OSVPackage) bool {
			return strings.EqualFold(o.Ecosystem, ecosystem) && strings.EqualFold(o.Name, pkg)
		}) {
			return e.Name, LayerRelgap, true
		}
	}
	if m := r.miseByBackend(miseBackend + pkg); m != nil {
		if e, ok := r.relgap[strings.ToLower(m.Short)]; ok {
			return e.Name, LayerMise, true
		}
		// A mise tool that installs only from the package registry names no repository, so
		// the lookup has to find it.
		if repo, _ := r.miseRepo(m); repo != "" {
			return m.Short, LayerMise, true
		}
	}
	return "", "", false
}

// Identify returns the name `relgap digest` accepts for input without merging layers: the
// registry name when a registry knows the tool, otherwise the github: or aqua: form itself.
func (r *Resolver) Identify(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	prefix, rest, explicit := strings.Cut(input, ":")
	if !explicit || r.isConfigAlias(input) {
		name, _, err := r.identifyName(input)
		return name, err == nil
	}
	if isForgePrefix(prefix) {
		forgeRest, tagPrefix, _ := SplitTagPrefix(rest)
		host, repo, err := r.splitForge(prefix, forgeRest)
		if err != nil {
			return "", false
		}
		if name, ok := r.configByHostRepo(host, repo); ok && tagPrefix == "" {
			return name, true
		}
		return forgeName(host, repo, tagPrefix), true
	}
	if prefix != backendGitHub && prefix != backendAqua {
		return "", false
	}
	if repo, tagPrefix, qualified := SplitTagPrefix(rest); prefix == backendGitHub && qualified {
		// The tag prefix makes this a tool of its own, never the one a registry lists for the repo.
		return "github:" + repo + "@" + tagPrefix, repoRe.MatchString(repo)
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
	default:
		return "", Match{}, nil, "", fmt.Errorf(
			"%w: unknown prefix %q (relgap knows github, bitbucket, gitlab, aqua, tfprovider, tfmodule, chart, "+
				"npm, pypi and image)",
			ErrUnsupported,
			prefix+":",
		)
	}
}

// Alias returns the tool that the config alias input points to, and false when input isn't a
// config alias or its tool is unknown.
func (r *Resolver) Alias(input string) (string, bool) {
	if !r.isConfigAlias(strings.TrimSpace(input)) {
		return "", false
	}
	name, _, err := r.identifyName(strings.TrimSpace(input))
	return name, err == nil
}

// isConfigAlias reports whether input is a key of the config's aliases. Such a key wins over
// any prefix in it, because a raw name such as git::git@host:ws/repo.git has a colon.
func (r *Resolver) isConfigAlias(input string) bool {
	_, ok := r.configAlias[strings.ToLower(input)]
	return ok
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
		if strings.EqualFold(rp, repo) && forge.IsGitHub(r.in.Config.Hosts[name]) {
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
		if host := r.in.Config.Hosts[cfgName]; !forge.IsGitHub(host) {
			b.setString("host", &b.spec.Host, strings.ToLower(host), LayerConfig)
		}
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

// fromFound takes the tag rules and the listed versions of a lookup: a chart's versions, or an
// image's tags with its variant.
func (b *builder) fromFound(f *Found) {
	b.setString("version.tag_prefix", &b.spec.Version.TagPrefix, f.TagPrefix, f.Layer)
	if f.Versions == nil {
		return
	}
	universe := UniverseChart
	if f.Image != "" {
		universe = UniverseImage
		b.setString("image", &b.spec.Image, f.Image, f.Layer)
		b.setString("version.tag_suffix", &b.spec.Version.TagSuffix, f.TagSuffix, f.Layer)
	}
	b.setString("version.universe", &b.spec.Version.Universe, universe, f.Layer)
	b.setString("version.versions_url", &b.spec.Version.VersionsURL, f.VersionsURL, f.Layer)
	b.spec.Versions = f.Versions
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
