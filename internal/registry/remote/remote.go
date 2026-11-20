package remote

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/oci"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/version"
)

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Hosts of the package registries that [Remote] asks.
const (
	TerraformRegistryHost = "registry.terraform.io"
	ArtifactHubHost       = "artifacthub.io"
	NPMHost               = "registry.npmjs.org"
	PyPIHost              = "pypi.org"
)

// Hosts returns the hosts [Remote] sends requests to, other than the GitHub API.
func Hosts() []string {
	return []string{TerraformRegistryHost, ArtifactHubHost, NPMHost, PyPIHost}
}

// RepoHit is one repository in a GitHub search result.
type RepoHit struct {
	FullName string
	Stars    int
}

// RepoSearcher searches GitHub repositories by name.
type RepoSearcher interface {
	SearchRepos(ctx context.Context, name string) ([]RepoHit, error)
}

// GitHubReader lists the tag names of a GitHub repository and reads a file at a ref.
type GitHubReader interface {
	Tags(ctx context.Context, repo string) ([]string, error)
	File(ctx context.Context, repo, ref, path string) ([]byte, error)
}

// ImageReader lists an image's tags and reads its source label; [oci.Client] implements it.
type ImageReader interface {
	Tags(ctx context.Context, r oci.Ref) ([]string, error)
	Source(ctx context.Context, r oci.Ref, tag string) (string, error)
	URL(r oci.Ref) string
}

// Remote is a [registry.Finder] that asks the Terraform registry, Artifact Hub, npm, PyPI, image
// registries and GitHub.
type Remote struct {
	hc     *http.Client
	search RepoSearcher
	gh     GitHubReader
	images ImageReader
}

// New returns a [Remote] that sends requests through hc, searches with search, and reads a
// chart repository's tags and a GitHub Pages Helm repository's index through gh.
func New(hc *http.Client, search RepoSearcher, gh GitHubReader) *Remote {
	return &Remote{hc: hc, search: search, gh: gh}
}

// WithImages returns r with an image reader for image: lookups.
func (r *Remote) WithImages(images ImageReader) *Remote {
	r.images = images
	return r
}

const maxLookupBody = 8 << 20

// Find implements [registry.Finder].
func (r *Remote) Find(ctx context.Context, kind, name string) (*registry.Found, error) {
	switch kind {
	case registry.KindTFProvider:
		return r.provider(ctx, name)
	case registry.KindTFModule:
		return r.module(ctx, name)
	case registry.KindChart:
		return r.chart(ctx, name)
	case registry.KindNPM:
		return r.npm(ctx, name)
	case registry.KindPyPI:
		return r.pypi(ctx, name)
	case registry.KindImage:
		return r.image(ctx, name)
	case registry.KindGuess:
		return r.guess(ctx, name)
	}
	return nil, fmt.Errorf("%w: no lookup for %q", registry.ErrUnsupported, kind)
}

func (r *Remote) provider(ctx context.Context, name string) (*registry.Found, error) {
	ns, provider, ok := strings.Cut(name, "/")
	if !ok || ns == "" || provider == "" || strings.Contains(provider, "/") {
		return nil, fmt.Errorf("%w: tfprovider:%s is not tfprovider:<namespace>/<name>", registry.ErrUnsupported, name)
	}
	var doc struct {
		Source string `json:"source"`
	}
	u := "https://" + TerraformRegistryHost + "/v1/providers/" + url.PathEscape(ns) + "/" + url.PathEscape(provider)
	if err := r.getJSON(ctx, u, "the Terraform registry has no provider "+name, &doc); err != nil {
		return nil, err
	}
	repo, _, ok := ParseGitHubURL(doc.Source)
	if !ok {
		return nil, fmt.Errorf(
			"%w for tfprovider:%s: its source %q isn't on GitHub",
			registry.ErrNoRepo,
			name,
			doc.Source,
		)
	}
	return &registry.Found{Repo: repo, Layer: registry.LayerTerraform}, nil
}

// module reads the latest version of a registry module from the endpoint that lists it for
// every provider, which is far smaller than the one for a single provider. Its source names the
// repository, and the part of its tag before the version is the tag prefix, so v6.8.0 for 6.8.0
// gives "v".
func (r *Remote) module(ctx context.Context, name string) (*registry.Found, error) {
	parts := strings.Split(name, "/")
	if len(parts) != moduleParts || slices.Contains(parts, "") {
		return nil, fmt.Errorf("%w: tfmodule:%s is not tfmodule:<namespace>/<name>/<provider>",
			registry.ErrUnsupported, name)
	}
	var doc struct {
		Modules []moduleVersion `json:"modules"`
	}
	notFound := "the Terraform registry has no module " + name
	u := "https://" + TerraformRegistryHost + "/v1/modules/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
	if err := r.getJSON(ctx, u, notFound, &doc); err != nil {
		return nil, err
	}
	i := slices.IndexFunc(doc.Modules, func(m moduleVersion) bool { return strings.EqualFold(m.Provider, parts[2]) })
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", registry.ErrUnknownTool, notFound)
	}
	m := doc.Modules[i]
	repo, _, ok := ParseGitHubURL(m.Source)
	if !ok {
		return nil, fmt.Errorf("%w for tfmodule:%s: its source %q isn't on GitHub", registry.ErrNoRepo, name, m.Source)
	}
	f := &registry.Found{Repo: repo, Layer: registry.LayerTerraform}
	if prefix, found := strings.CutSuffix(m.Tag, m.Version); found && m.Version != "" {
		f.TagPrefix = prefix
	}
	return f, nil
}

const moduleParts = 3

// moduleVersion is a module's latest version for one provider in the Terraform registry.
type moduleVersion struct {
	Provider string `json:"provider"`
	Source   string `json:"source"`
	Version  string `json:"version"`
	Tag      string `json:"tag"`
}

// ChartRepo returns the Artifact Hub repository at the Helm repository URL registryURL that has
// a chart called chart. For an oci:// URL it tries <url>/<chart> first, because Artifact Hub
// lists most OCI charts as a repository of their own. It reports false when none matches.
func (r *Remote) ChartRepo(ctx context.Context, registryURL, chart string) (string, bool, error) {
	candidates := []string{registryURL}
	if strings.HasPrefix(registryURL, "oci://") {
		candidates = []string{registryURL + "/" + chart, registryURL}
	}
	for _, c := range candidates {
		var repos []struct {
			Name string `json:"name"`
		}
		q := url.Values{"kind": {"0"}, "url": {c}, "limit": {"60"}}
		if err := r.getJSON(
			ctx,
			"https://"+ArtifactHubHost+"/api/v1/repositories/search?"+q.Encode(),
			"",
			&repos,
		); err != nil {
			return "", false, err
		}
		for _, repo := range repos {
			has, err := r.hasChart(ctx, repo.Name, chart)
			if err != nil {
				return "", false, err
			}
			if has {
				return repo.Name, true, nil
			}
		}
	}
	return "", false, nil
}

// hasChart reports whether the Artifact Hub repository repo has a chart called chart.
func (r *Remote) hasChart(ctx context.Context, repo, chart string) (bool, error) {
	var pkg chartPackage
	err := r.getJSON(ctx, chartURL(repo, chart), "no chart", &pkg)
	switch {
	case errors.Is(err, registry.ErrUnknownTool):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

func chartURL(repo, chart string) string {
	return "https://" + ArtifactHubHost + "/api/v1/packages/helm/" + url.PathEscape(repo) + "/" + url.PathEscape(chart)
}

type chartLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type chartPackage struct {
	Name       string      `json:"name"`
	Official   bool        `json:"official"`
	HomeURL    string      `json:"home_url"`
	Links      []chartLink `json:"links"`
	Repository struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"repository"`
	AvailableVersions []struct {
		Version    string `json:"version"`
		Prerelease bool   `json:"prerelease"`
		TS         int64  `json:"ts"`
	} `json:"available_versions"`
}

func (r *Remote) chart(ctx context.Context, name string) (*registry.Found, error) {
	if registryURL, chart, ok := registry.SplitChartAt(name); ok {
		return r.chartAt(ctx, registryURL, chart)
	}
	repoName, chart, hasRepo := strings.Cut(name, "/")
	if !hasRepo {
		var err error
		if repoName, err = r.chartRepo(ctx, name); err != nil {
			return nil, err
		}
		chart = name
	}
	if repoName == "" || chart == "" || strings.Contains(chart, "/") {
		return nil, fmt.Errorf("%w: chart:%s is not chart:<repo>/<name> or chart:<name>", registry.ErrUnsupported, name)
	}
	var pkg chartPackage
	if err := r.getJSON(
		ctx,
		chartURL(repoName, chart),
		"Artifact Hub has no chart "+repoName+"/"+chart,
		&pkg,
	); err != nil {
		return nil, err
	}
	f, ok := chartSource(pkg)
	if !ok {
		return nil, fmt.Errorf("%w for chart:%s/%s: Artifact Hub lists no GitHub source for it",
			registry.ErrNoRepo, repoName, chart)
	}
	f.Name = chart
	if r.gh == nil || len(pkg.AvailableVersions) == 0 {
		return f, nil
	}
	refs := make([]version.Ref, len(pkg.AvailableVersions))
	for i, v := range pkg.AvailableVersions {
		refs[i] = version.Ref{Tag: v.Version, Prerelease: v.Prerelease}
		if v.TS > 0 {
			refs[i].PublishedAt = time.Unix(v.TS, 0).UTC()
		}
	}
	return r.chartVersions(ctx, f, repoName+"/"+chart, refs,
		"https://"+ArtifactHubHost+"/packages/helm/"+repoName+"/"+chart)
}

// chartAt resolves the chart in the Helm repository at registryURL through the Artifact Hub
// repository at that URL. It never searches by the chart's name, because the URL says which
// chart the pin means and another repository's chart of that name is a different one.
func (r *Remote) chartAt(ctx context.Context, registryURL, chart string) (*registry.Found, error) {
	repo, ok, err := r.ChartRepo(ctx, registryURL, chart)
	if err != nil {
		return nil, fmt.Errorf("look up registryUrl %s on Artifact Hub: %w", registryURL, err)
	}
	if ok {
		return r.chart(ctx, repo+"/"+chart)
	}
	if pagesRepo, dir, isPages := githubPagesDir(registryURL); isPages && r.gh != nil {
		return r.pagesChart(ctx, registryURL, pagesRepo, dir, chart)
	}
	return nil, fmt.Errorf("%w: registryUrl %s is not an Artifact Hub repository with a chart named %s",
		registry.ErrUnknownTool, registryURL, chart)
}

// pagesBranch and pagesIndex are where chart-releaser publishes a GitHub Pages Helm repository
// by default (its --pages-branch and --pages-index-path).
const (
	pagesBranch = "gh-pages"
	pagesIndex  = "index.yaml"
)

// helmIndex is the part of a Helm repository's index.yaml that a lookup reads.
type helmIndex struct {
	Entries map[string][]struct {
		Version string    `yaml:"version"`
		Created time.Time `yaml:"created"`
		URLs    []string  `yaml:"urls"`
	} `yaml:"entries"`
}

// pagesChart reads the chart's versions from the index.yaml that the GitHub Pages Helm repository
// at registryURL publishes on its gh-pages branch, through the GitHub API. The chart's
// repository is the Pages repository, unless the newest version's package is a GitHub release
// asset of another one.
func (r *Remote) pagesChart(ctx context.Context, registryURL, pagesRepo, dir, chart string) (*registry.Found, error) {
	file := path.Join(dir, pagesIndex)
	data, err := r.gh.File(ctx, pagesRepo, pagesBranch, file)
	if err != nil {
		return nil, fmt.Errorf("%w: registryUrl %s is not an Artifact Hub repository, and %s of %s at %s "+
			"can't be read: %w", registry.ErrUnknownTool, registryURL, file, pagesRepo, pagesBranch, err)
	}
	var idx helmIndex
	if err = yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode %s of %s: %w", file, pagesRepo, err)
	}
	entries := idx.Entries[chart]
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: the Helm repository at %s has no chart named %s",
			registry.ErrUnknownTool, registryURL, chart)
	}
	refs := make([]version.Ref, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, version.Ref{Tag: e.Version, PublishedAt: e.Created})
	}
	repo := pagesRepo
	if urls := entries[0].URLs; len(urls) > 0 && strings.Contains(urls[0], "/releases/download/") {
		if released, _, found := ParseGitHubURL(urls[0]); found {
			repo = released
		}
	}
	f := &registry.Found{Repo: repo, Name: chart, Layer: registry.LayerHelmRepo}
	indexURL := "https://github.com/" + pagesRepo + "/blob/" + pagesBranch + "/" + file
	return r.chartVersions(ctx, f, chart, refs, indexURL)
}

// chartVersions sets the tag prefix of the chart's repository from its tags when they cover the
// newest chart version. Otherwise the repository has no tag per chart version, such as
// aws/eks-charts with its v0.0.N tags, and the chart's own versions become the universe.
func (r *Remote) chartVersions(
	ctx context.Context,
	f *registry.Found,
	chart string,
	refs []version.Ref,
	versionsURL string,
) (*registry.Found, error) {
	tags, err := r.gh.Tags(ctx, f.Repo)
	if err != nil {
		return nil, fmt.Errorf("chart %s: %w", chart, err)
	}
	versions := make([]string, len(refs))
	for i, ref := range refs {
		versions[i] = ref.Tag
	}
	if prefix, found := chartTagPrefix(chart, versions, newestStable(refs), tags); found {
		f.TagPrefix = prefix
		return f, nil
	}
	f.TagPrefix, f.Versions, f.VersionsURL = "", refs, versionsURL
	return f, nil
}

// newestStable returns the newest version in refs that isn't a prerelease, or "".
func newestStable(refs []version.Ref) string {
	s := version.Loose()
	var best version.V
	newest := ""
	for _, ref := range refs {
		v, err := s.Parse(ref.Tag)
		if err != nil || ref.Prerelease || s.IsPrerelease(v) {
			continue
		}
		if newest == "" || s.Compare(v, best) > 0 {
			best, newest = v, ref.Tag
		}
	}
	return newest
}

// chartTagPrefix finds the prefix of the tags that name the chart's versions. Each tag that
// ends in a version Artifact Hub lists gives the rest of the tag as a candidate. A candidate must
// cover newest, the newest stable chart version, when there is one: a repository that stopped
// tagging per chart can't say what is latest. Of the rest, the one that covers the most versions
// wins; on a tie, one that holds the chart's name, then the longer one. So
// external-dns-helm-chart- beats the app's v in kubernetes-sigs/external-dns.
func chartTagPrefix(chart string, versions []string, newest string, tags []string) (string, bool) {
	known := make(map[string]bool, len(versions))
	for _, v := range versions {
		known[strings.TrimPrefix(v, "v")] = true
	}
	covered := map[string]map[string]bool{}
	for _, tag := range tags {
		for i := range len(tag) {
			if !isDigit(tag[i]) || i > 0 && (isDigit(tag[i-1]) || tag[i-1] == '.') {
				continue
			}
			if v := tag[i:]; known[v] {
				if covered[tag[:i]] == nil {
					covered[tag[:i]] = map[string]bool{}
				}
				covered[tag[:i]][v] = true
			}
		}
	}
	if newest = strings.TrimPrefix(newest, "v"); newest != "" {
		maps.DeleteFunc(covered, func(_ string, vs map[string]bool) bool { return !vs[newest] })
	}
	if len(covered) == 0 {
		return "", false
	}
	chart = strings.ToLower(chart)
	rank := func(prefix string) (int, bool, int) {
		return len(covered[prefix]), strings.Contains(strings.ToLower(prefix), chart), len(prefix)
	}
	return slices.MaxFunc(slices.Sorted(maps.Keys(covered)), func(a, b string) int {
		an, ac, al := rank(a)
		bn, bc, bl := rank(b)
		return cmp.Or(cmp.Compare(an, bn), compareBool(ac, bc), cmp.Compare(al, bl))
	}), true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// chartRepo finds the Artifact Hub repository of the chart called name: the only one with such
// a chart, or else the only one whose chart Artifact Hub marks official, which means the
// project publishes it.
func (r *Remote) chartRepo(ctx context.Context, name string) (string, error) {
	var res struct {
		Packages []chartPackage `json:"packages"`
	}
	q := url.Values{"ts_query_web": {name}, "kind": {"0"}, "limit": {"60"}}
	u := "https://" + ArtifactHubHost + "/api/v1/packages/search?" + q.Encode()
	if err := r.getJSON(ctx, u, "", &res); err != nil {
		return "", err
	}
	var repos, official []string
	for _, p := range res.Packages {
		if strings.EqualFold(p.Name, name) && !slices.Contains(repos, p.Repository.Name) {
			repos = append(repos, p.Repository.Name)
			if p.Official {
				official = append(official, p.Repository.Name)
			}
		}
	}
	switch {
	case len(repos) == 0:
		return "", fmt.Errorf("%w: Artifact Hub has no chart named %q", registry.ErrUnknownTool, name)
	case len(repos) == 1:
		return repos[0], nil
	case len(official) == 1:
		return official[0], nil
	}
	slices.Sort(repos)
	for i, rp := range repos {
		repos[i] = "chart:" + rp + "/" + name
	}
	return "", fmt.Errorf("%w: %d Artifact Hub repositories have a chart named %q; use one of %s",
		registry.ErrAmbiguous, len(repos), name, strings.Join(repos, ", "))
}

// chartSource picks the repository that holds the chart itself, not the application it
// installs: a link into a subdirectory, such as argo-helm/tree/main/charts/argo-cd, wins. Such a
// repository gets chart-releaser's default tag prefix <name>-, which the repository's tags
// replace when they show another one.
func chartSource(p chartPackage) (*registry.Found, bool) {
	var plain []string
	for _, l := range p.Links {
		repo, sub, ok := ParseGitHubURL(l.URL)
		if !ok {
			continue
		}
		if sub != "" {
			return &registry.Found{Repo: repo, TagPrefix: p.Name + "-", Layer: registry.LayerArtifactHub}, true
		}
		plain = append(plain, repo)
	}
	if repo, ok := githubPages(p.Repository.URL); ok {
		return &registry.Found{Repo: repo, TagPrefix: p.Name + "-", Layer: registry.LayerArtifactHub}, true
	}
	for _, raw := range []string{p.HomeURL, p.Repository.URL} {
		if repo, _, ok := ParseGitHubURL(raw); ok {
			plain = append(plain, repo)
		}
	}
	for _, repo := range plain {
		if strings.EqualFold(path.Base(repo), p.Name) {
			return &registry.Found{Repo: repo, Layer: registry.LayerArtifactHub}, true
		}
	}
	if len(plain) > 0 {
		return &registry.Found{Repo: plain[0], Layer: registry.LayerArtifactHub}, true
	}
	return nil, false
}

// githubPages maps a chart repository served by GitHub Pages, https://<owner>.github.io/<repo>,
// to owner/repo.
func githubPages(raw string) (string, bool) {
	repo, _, ok := githubPagesDir(raw)
	return repo, ok
}

// githubPagesDir maps https://<owner>.github.io/<repo>/<dir> to owner/repo and the directory
// inside the site.
func githubPagesDir(raw string) (string, string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" {
		return "", "", false
	}
	owner, ok := strings.CutSuffix(strings.ToLower(u.Hostname()), ".github.io")
	repo, dir, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
	if !ok || owner == "" || repo == "" || !repoRe.MatchString(owner+"/"+repo) {
		return "", "", false
	}
	return owner + "/" + repo, dir, true
}

func (r *Remote) npm(ctx context.Context, name string) (*registry.Found, error) {
	var doc struct {
		Repository json.RawMessage `json:"repository"`
		Homepage   string          `json:"homepage"`
		Bugs       json.RawMessage `json:"bugs"`
	}
	escaped := strings.Replace(name, "/", "%2F", 1)
	u := "https://" + NPMHost + "/" + escaped + "/latest"
	if err := r.getJSON(ctx, u, "npm has no package "+name, &doc); err != nil {
		return nil, err
	}
	for _, raw := range []string{npmURL(doc.Repository), doc.Homepage, npmURL(doc.Bugs)} {
		if repo, _, ok := parseRepoRef(raw); ok {
			return &registry.Found{Repo: repo, Layer: registry.LayerNPM}, nil
		}
	}
	return nil, fmt.Errorf("%w for npm:%s: its repository, homepage and bugs fields name no GitHub repo",
		registry.ErrNoRepo, name)
}

// npmURL reads a package.json field that is either a string or an object with a url.
func npmURL(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(raw, &o) // a malformed field names no repository
	return o.URL
}

// pypiURLKeys are the project_urls labels that name a source repository, best first.
func pypiURLKeys() []string {
	return []string{"source", "source code", "repository", "code", "github", "homepage", "home"}
}

func (r *Remote) pypi(ctx context.Context, name string) (*registry.Found, error) {
	var doc struct {
		Info struct {
			ProjectURLs map[string]string `json:"project_urls"`
			HomePage    string            `json:"home_page"`
		} `json:"info"`
	}
	u := "https://" + PyPIHost + "/pypi/" + url.PathEscape(name) + "/json"
	if err := r.getJSON(ctx, u, "PyPI has no project "+name, &doc); err != nil {
		return nil, err
	}
	byKey := map[string]string{}
	for k, v := range doc.Info.ProjectURLs {
		byKey[strings.ToLower(strings.TrimSpace(k))] = v
	}
	candidates := make([]string, 0, len(byKey)+1)
	for _, k := range pypiURLKeys() {
		candidates = append(candidates, byKey[k])
	}
	for _, k := range slices.Sorted(maps.Keys(byKey)) {
		candidates = append(candidates, byKey[k])
	}
	candidates = append(candidates, doc.Info.HomePage)
	for _, raw := range candidates {
		if repo, _, ok := ParseGitHubURL(raw); ok {
			return &registry.Found{Repo: repo, Layer: registry.LayerPyPI}, nil
		}
	}
	return nil, fmt.Errorf("%w for pypi:%s: its project URLs name no GitHub repo", registry.ErrNoRepo, name)
}

func (r *Remote) guess(ctx context.Context, name string) (*registry.Found, error) {
	if r.search == nil {
		return nil, fmt.Errorf("%w %q: no GitHub search available", registry.ErrUnknownTool, name)
	}
	hits, err := r.search.SearchRepos(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("search GitHub for %s: %w", name, err)
	}
	var exact []RepoHit
	for _, h := range hits {
		if strings.EqualFold(path.Base(h.FullName), name) {
			exact = append(exact, h)
		}
	}
	if len(exact) == 0 {
		return nil, fmt.Errorf("%w %q: no GitHub repository is named exactly %s", registry.ErrUnknownTool, name, name)
	}
	slices.SortStableFunc(exact, func(a, b RepoHit) int { return b.Stars - a.Stars })
	f := &registry.Found{Repo: exact[0].FullName, Name: name, Layer: registry.LayerGuess}
	for _, h := range exact[1:] {
		f.Alternatives = append(f.Alternatives, h.FullName)
	}
	return f, nil
}

// getJSON decodes the JSON at u into v. A 404 wraps [registry.ErrUnknownTool] with notFound, or is an
// ordinary error when notFound is empty.
func (r *Remote) getJSON(ctx context.Context, u, notFound string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.hc.Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && notFound != "" {
		return fmt.Errorf("%w: %s", registry.ErrUnknownTool, notFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: %s", u, resp.Status)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxLookupBody)).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", u, err)
	}
	return nil
}

// ParseGitHubURL returns the owner/repo of a GitHub URL in any of the forms package metadata
// uses (https, git+https, git+ssh, git@github.com:, github:) and the path inside the repository
// that a /tree/<ref>/ or /blob/<ref>/ URL points to.
func ParseGitHubURL(raw string) (string, string, bool) {
	s := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(s, "github:"); ok {
		return cleanRepo(rest, "")
	}
	if rest, ok := strings.CutPrefix(s, "git@github.com:"); ok {
		return cleanRepo(rest, "")
	}
	s = strings.TrimPrefix(s, "git+")
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "github.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	const ownerRepo, treeRef = 2, 4
	if len(parts) < ownerRepo {
		return "", "", false
	}
	sub := ""
	if len(parts) > treeRef && (parts[2] == "tree" || parts[2] == "blob") {
		sub = strings.Join(parts[treeRef:], "/")
	}
	return cleanRepo(parts[0]+"/"+parts[1], sub)
}

// parseRepoRef is [ParseGitHubURL] plus npm's owner/repo shorthand.
func parseRepoRef(raw string) (string, string, bool) {
	if repo, sub, ok := ParseGitHubURL(raw); ok {
		return repo, sub, true
	}
	if !strings.Contains(raw, ":") {
		return cleanRepo(raw, "")
	}
	return "", "", false
}

func cleanRepo(s, sub string) (string, string, bool) {
	s, _, _ = strings.Cut(s, "#")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if !repoRe.MatchString(s) {
		return "", "", false
	}
	return s, sub, true
}

// image lists the tags of image:<name>[:<prefix>*<suffix>] and reads the source label of its
// newest stable tag with that pattern. A label that names no GitHub repository, or a manifest
// relgap can't read, leaves the tool without notes.
func (r *Remote) image(ctx context.Context, name string) (*registry.Found, error) {
	if r.images == nil {
		return nil, fmt.Errorf("%w: image:%s needs an image registry, which this command doesn't read",
			registry.ErrUnsupported, name)
	}
	image, prefix, suffix, ok := registry.SplitImage(name)
	if !ok {
		return nil, fmt.Errorf("%w: image:%s is not image:<name> or image:<name>:<prefix>*<suffix>",
			registry.ErrUnsupported, name)
	}
	ref, err := oci.ParseRef(image)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", registry.ErrUnsupported, err)
	}
	tags, err := r.images.Tags(ctx, ref)
	switch {
	case errors.Is(err, oci.ErrNotAllowed):
		return nil, fmt.Errorf("%w: %w", registry.ErrUnsupported, err)
	case errors.Is(err, httpx.ErrNotFound), errors.Is(err, httpx.ErrUnauthorized):
		return nil, fmt.Errorf("%w: image %s: not found, or private (relgap reads public images only): %w",
			registry.ErrUnknownTool, ref, err)
	case err != nil:
		return nil, fmt.Errorf("image %s: %w", ref, err)
	}
	refs := make([]version.Ref, 0, len(tags))
	var matching []version.Ref
	for _, t := range tags {
		refs = append(refs, version.Ref{Tag: t})
		if rest, hasPrefix := strings.CutPrefix(t, prefix); hasPrefix {
			if v, hasSuffix := strings.CutSuffix(rest, suffix); hasSuffix && v != "" {
				matching = append(matching, version.Ref{Tag: v})
			}
		}
	}
	found := &registry.Found{
		Name: registry.KindImage + ":" + name, TagPrefix: prefix, TagSuffix: suffix, Layer: registry.LayerImage,
		Versions: refs, VersionsURL: r.images.URL(ref), Image: ref.String(),
	}
	newest := newestStable(matching)
	if newest == "" {
		return found, nil
	}
	src, err := r.images.Source(ctx, ref, prefix+newest+suffix)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("image %s: %w", ref, ctx.Err())
		}
		return found, nil
	}
	if repo, _, isGitHub := ParseGitHubURL(src); isGitHub {
		found.Repo = repo
	}
	return found, nil
}
