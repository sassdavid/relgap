package scan

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/sassdavid/relgap/internal/forge"
)

// renovateComment matches `# renovate: key=value ...`, the comment syntax Renovate's
// documentation uses for custom regex managers, also written after // or inside <!-- -->.
var renovateComment = regexp.MustCompile(`^\s*(?:#|//|<!--)\s*renovate:(.*?)(?:-->)?\s*$`)

// renovateFields parses the key=value fields of a renovate comment (datasource, depName,
// packageName, registryUrl, versioning).
func renovateFields(comment string) (map[string]string, bool) {
	m := renovateComment.FindStringSubmatch(comment)
	if m == nil {
		return nil, false
	}
	fields := map[string]string{}
	for tok := range strings.FieldsSeq(m[1]) {
		if k, v, found := strings.Cut(tok, "="); found {
			fields[k] = v
		}
	}
	return fields, true
}

// renovateLookup returns the name to resolve from the last renovate comment in comments. It
// reports false when the comment's extractVersion is a form relgap can't read, so the pin must
// stay unresolved.
func renovateLookup(comments []string) (string, bool) {
	for _, c := range slices.Backward(comments) {
		if fields, ok := renovateFields(c); ok {
			return datasourceLookup(fields["datasource"], firstNonEmpty(fields["packageName"], fields["lookupName"]),
				fields["depName"], fields["extractVersion"])
		}
	}
	return "", true
}

// datasourceLookup maps a Renovate datasource and package to a name the resolver understands.
// github-releases and github-tags name a repository, with the tag prefix that extractVersion
// gives as github:owner/repo@prefix; other datasources use the package name. It reports false
// for a github datasource whose extractVersion isn't a literal prefix before the version.
func datasourceLookup(datasource, pkg, dep, extract string) (string, bool) {
	name := firstNonEmpty(pkg, dep)
	switch datasource {
	case "github-releases", "github-tags":
		if strings.Count(name, "/") != 1 {
			break
		}
		prefix, ok := extractPrefix(extract)
		if !ok {
			return "", false
		}
		if prefix != "" {
			return prefixGitHub + name + "@" + prefix, true
		}
		return prefixGitHub + name, true
	case DetectorAqua:
		return prefixAqua + name, true
	default:
	}
	return name, true
}

// extractGroup matches Renovate's named version group, in JavaScript's or RE2's syntax.
var extractGroup = regexp.MustCompile(`\(\?P?<version>`)

// extractPrefix reads a Renovate extractVersion of the form ^<literal>(?<version>...)$ and
// returns the literal, the part of every tag before the version. A trailing v? and a lone v are
// dropped, because version parsing already skips a v. It reports false for any other form, such
// as a pattern after the group or a wildcard before it. An empty extract is a prefix of "".
func extractPrefix(extract string) (string, bool) {
	if extract == "" {
		return "", true
	}
	rest, anchored := strings.CutPrefix(extract, "^")
	loc := extractGroup.FindStringIndex(rest)
	if !anchored || loc == nil || !groupEndsPattern(rest[loc[1]:]) {
		return "", false
	}
	literal, ok := unescapeLiteral(strings.TrimSuffix(rest[:loc[0]], "v?"))
	if !ok || literal == "v" {
		return "", ok
	}
	return literal, true
}

// extractAffixes reads a Renovate extractVersion of the form ^<literal>(?<version>...)<literal>$
// and returns the two literals, the parts of an image tag around its version. A trailing v? and
// a lone v before the group are dropped, as in [extractPrefix]. It reports false for any other
// form.
func extractAffixes(extract string) (string, string, bool) {
	rest, anchored := strings.CutPrefix(extract, "^")
	loc := extractGroup.FindStringIndex(rest)
	if !anchored || loc == nil {
		return "", "", false
	}
	end, ok := groupEnd(rest[loc[1]:])
	if !ok {
		return "", "", false
	}
	prefix, ok := unescapeLiteral(strings.TrimSuffix(rest[:loc[0]], "v?"))
	if !ok {
		return "", "", false
	}
	if prefix == "v" {
		prefix = ""
	}
	tail, anchoredEnd := strings.CutSuffix(rest[loc[1]+end+1:], "$")
	suffix, ok := unescapeLiteral(tail)
	if !ok || (suffix != "" && !anchoredEnd) {
		return "", "", false
	}
	return prefix, suffix, true
}

// groupEnd returns the index of the parenthesis that closes the group starting at the
// beginning of s (after its opening).
func groupEnd(s string) (int, bool) {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// groupEndsPattern reports whether the version group that starts at the beginning of s (after
// its opening) closes at the end of the pattern, before an optional $.
func groupEndsPattern(s string) bool {
	end, ok := groupEnd(s)
	tail := s[end+1:]
	return ok && (tail == "" || tail == "$")
}

// unescapeLiteral returns the text a regular expression matches when it has no metacharacters,
// with escapes such as \. and \- removed, and reports false otherwise.
func unescapeLiteral(re string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(re); i++ {
		c := re[i]
		switch {
		case c == '\\' && i+1 < len(re) && strings.IndexByte(`.-_/+*?()[]{}|^$\\`, re[i+1]) >= 0:
			i++
			b.WriteByte(re[i])
		case strings.IndexByte(`\\.+*?()[]{}|^$`, c) >= 0:
			return "", false
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}

// renovatePackageFile is one entry of packageFiles in a Renovate report.
type renovatePackageFile struct {
	PackageFile string `json:"packageFile"`
	Deps        []struct {
		DepName        string   `json:"depName"`
		PackageName    string   `json:"packageName"`
		CurrentValue   string   `json:"currentValue"`
		Datasource     string   `json:"datasource"`
		SkipReason     string   `json:"skipReason"`
		RegistryURLs   []string `json:"registryUrls"`
		ExtractVersion string   `json:"extractVersion"`
	} `json:"deps"`
}

type renovatePackageFiles map[string][]renovatePackageFile // by manager

// RenovateReport reads the JSON report Renovate writes with --report-type=file. packageFiles
// may sit at the top level or under repositories.<name>. Unknown fields are ignored. Pins carry
// no line, because Renovate reports none.
func RenovateReport(data []byte) ([]Pin, error) {
	var report struct {
		PackageFiles renovatePackageFiles `json:"packageFiles"`
		Repositories map[string]struct {
			PackageFiles renovatePackageFiles `json:"packageFiles"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("decode renovate report: %w", err)
	}
	pins := renovatePins(".", report.PackageFiles)
	for _, repo := range slices.Sorted(maps.Keys(report.Repositories)) {
		pins = append(pins, renovatePins(repo, report.Repositories[repo].PackageFiles)...)
	}
	return pins, nil
}

func renovatePins(repo string, files renovatePackageFiles) []Pin {
	var pins []Pin
	for _, manager := range slices.Sorted(maps.Keys(files)) {
		for _, pf := range files[manager] {
			for _, d := range pf.Deps {
				if d.CurrentValue == "" || d.SkipReason != "" || d.DepName == "" {
					continue
				}
				registryURL := ""
				if len(d.RegistryURLs) > 0 {
					registryURL = d.RegistryURLs[0]
				}
				p := renovateDepPin(manager, d.Datasource, d.DepName, d.PackageName, d.ExtractVersion, registryURL)
				if p.Kind == KindChart {
					p.Registry = chartRegistry(registryURL)
				}
				p.Value = d.CurrentValue
				p.Location = Location{Repo: repo, File: pf.PackageFile, Detector: DetectorRenovate}
				pins = append(pins, p)
			}
		}
	}
	return pins
}

// renovateDepPin sets the kind and the tool or lookup name of a Renovate dependency. An npm or
// PyPI package that no registry names by itself becomes npm:<name> or pypi:<name>. registryURL
// is the dependency's first registry URL, which names a self-managed GitLab host.
func renovateDepPin(manager, datasource, dep, pkg, extract, registryURL string) Pin {
	p := Pin{RawName: dep, Kind: KindTool}
	name := firstNonEmpty(pkg, dep)
	switch {
	case datasource == "docker":
		p.Kind = KindImage
		p.image = &imageRef{name: name}
		if extract != "" {
			prefix, suffix, ok := extractAffixes(extract)
			p.image.prefix, p.image.suffix, p.image.pattern, p.noLookup = prefix, suffix, ok, !ok
		}
	case isForgeDatasource(datasource):
		p.git, p.noLookup = forgeSource(datasource, name, extract, registryURL)
		if g := p.git; g != nil && g.host == forge.GitHub && strings.Count(g.path, "/") == 1 {
			p.lookup, p.git = prefixGitHub+g.path, nil
			if g.tagPrefix != "" {
				p.lookup += "@" + g.tagPrefix
			}
		}
	case datasource == "terraform-provider":
		if !strings.Contains(name, "/") {
			name = "hashicorp/" + name
		}
		p.Kind, p.Tool = KindTFProvider, "tfprovider:"+providerName(name)
	case datasource == "terraform-module":
		p.Kind, p.Tool = KindTFModule, "tfmodule:"+providerName(name)
	case datasource == "helm":
		p.Kind, p.Tool = KindChart, "chart:"+dep
	case manager == "github-actions":
		p.Kind, p.Tool = KindAction, "action:"+name
	case datasource == "npm" && !publicNPM(registryURL):
		// A package from another registry isn't the npm package of that name, so it is never
		// looked up on registry.npmjs.org.
		p.Tool, p.Registry = "npm:"+name, strings.TrimRight(registryURL, "/")
	case datasource == "npm" || datasource == "pypi":
		p.lookup, p.fallback = name, datasource+":"+name
	default:
		var ok bool
		if p.lookup, ok = datasourceLookup(datasource, pkg, dep, extract); !ok {
			p.noLookup = true
		}
	}
	return p
}

// npmRegistryHost is the host of the public npm registry.
const npmRegistryHost = "registry.npmjs.org"

// publicNPM reports whether a Renovate registry URL is the public npm registry, or no URL.
func publicNPM(registryURL string) bool {
	if registryURL == "" {
		return true
	}
	u, err := url.Parse(registryURL)
	return err == nil && strings.EqualFold(u.Hostname(), npmRegistryHost)
}

func isForgeDatasource(datasource string) bool {
	switch datasource {
	case "bitbucket-tags", "gitlab-tags", "gitlab-releases", "git-tags", "git-refs":
		return true
	}
	return false
}

// forgeSource reads the repository of a bitbucket-tags, gitlab-tags, gitlab-releases, git-tags or
// git-refs dependency, and reports true when the pin must stay unresolved: its extractVersion is
// a form relgap can't read, or it names no repository.
func forgeSource(datasource, name, extract, registryURL string) (*gitSource, bool) {
	prefix, ok := extractPrefix(extract)
	if !ok {
		return nil, true
	}
	var host, repo string
	switch datasource {
	case "bitbucket-tags":
		host, repo = forge.Bitbucket, strings.Trim(name, "/")
	case "gitlab-tags", "gitlab-releases":
		host, repo = forge.GitLab, strings.Trim(name, "/")
		if u, err := url.Parse(registryURL); err == nil && u.Host != "" {
			host = strings.ToLower(u.Hostname())
		}
	default:
		if host, repo, ok = gitRemote(name); !ok {
			return nil, true
		}
	}
	if repo == "" {
		return nil, true
	}
	return &gitSource{host: host, path: repo, tagPrefix: prefix}, false
}

// isAnnotated matches the files the renovate-comment detector reads. Dockerfiles have their own
// detector, which reads the same comments.
func (o *Options) isAnnotated(rel string) bool {
	if isDockerfile(rel) {
		return false
	}
	base := path.Base(rel)
	switch path.Ext(base) {
	case ".tf", ".tfvars", extYML, extYAML, ".mk", ".sh":
		return true
	case ".hcl":
		return base != tfLockFile
	}
	switch base {
	case "Makefile", "makefile", "GNUmakefile", "GNUMakefile", "pom.xml", ".env":
		return true
	}
	return strings.HasPrefix(base, ".env.") || o.isExtraFile(rel)
}

// annotatedVersion matches a version that stands alone on a line: quoted, or after =, : or >,
// and followed by a quote, a closing bracket, a separator or the end of the line.
var annotatedVersion = regexp.MustCompile(
	`(?:^|[\s=:"'>(,\[])(v?\d+(?:\.\d+){1,3}(?:-[0-9A-Za-z.]+)?(?:\+[0-9A-Za-z.]+)?)(?:$|[\s"'<),;\]])`,
)

// detectAnnotations reads each renovate comment and the first line after it that is neither
// blank nor a comment. That line's version is a pin when it is the only one there.
func detectAnnotations(f file) ([]Pin, error) {
	if !bytes.Contains(f.data, []byte("renovate:")) {
		return nil, nil
	}
	var (
		pins    []Pin
		pending map[string]string
	)
	for i, line := range lines(f.data) {
		trimmed := strings.TrimSpace(line)
		if fields, ok := renovateFields(trimmed); ok {
			pending = fields
			continue
		}
		if pending == nil || trimmed == "" || isCommentLine(trimmed) {
			continue
		}
		fields := pending
		pending = nil
		value, ok := soleVersion(trimmed)
		if !ok {
			continue
		}
		p := annotationPin(fields)
		if p.RawName == "" {
			continue
		}
		p.Value, p.Location.Line = value, i+1
		pins = append(pins, p)
	}
	return pins, nil
}

func isCommentLine(s string) bool {
	return strings.HasPrefix(s, "#") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "<!--")
}

// soleVersion returns the version on a line without its trailing comment, when there is
// exactly one.
func soleVersion(line string) (string, bool) {
	line = stripComment(line)
	if i := strings.Index(line, " //"); i >= 0 {
		line = line[:i]
	}
	m := annotatedVersion.FindAllStringSubmatch(line, -1)
	if len(m) != 1 {
		return "", false
	}
	return m[0][1], true
}

// annotationPin names the dependency a renovate comment describes. Renovate's Chart.yaml
// preset writes `image=<name>` for a Docker image without a datasource.
func annotationPin(fields map[string]string) Pin {
	datasource, dep := fields["datasource"], fields["depName"]
	if image := fields["image"]; image != "" && dep == "" {
		datasource, dep = "docker", image
	}
	pkg := firstNonEmpty(fields["packageName"], fields["lookupName"])
	p := renovateDepPin("", datasource, firstNonEmpty(dep, pkg), pkg, fields["extractVersion"], fields["registryUrl"])
	if p.Kind == KindChart {
		p.Registry = chartRegistry(fields["registryUrl"])
	}
	return p
}

// chartRegistry returns a Helm repository URL without its trailing slash, or "" when it isn't
// an http, https or oci URL, such as a Chart.yaml alias (@stable) or a file:// path.
func chartRegistry(raw string) string {
	for _, scheme := range []string{"https://", "http://", "oci://"} {
		if strings.HasPrefix(raw, scheme) && len(raw) > len(scheme) {
			return strings.TrimRight(raw, "/")
		}
	}
	return ""
}

// genericVersion accepts =, :=, ?=, += and : after the name, so Makefile and YAML assignments
// match too.
var genericVersion = regexp.MustCompile(`(?i)\b([a-z0-9_]+)_VERSION\s*(?:[:?+]?=|:)\s*["']?(v?[0-9][^\s"']*)`)

// detectGeneric finds NAME_VERSION = x.y.z assignments in any text file.
func detectGeneric(f file) ([]Pin, error) {
	var pins []Pin
	for i, line := range lines(f.data) {
		for _, m := range genericVersion.FindAllStringSubmatch(line, -1) {
			raw := m[1] + "_VERSION"
			pins = append(pins, Pin{
				RawName: raw, Kind: KindTool, Value: m[2], Location: Location{Line: i + 1},
				lookup: strings.ToUpper(raw),
			})
		}
	}
	return pins, nil
}

// renovateBranchPrefix is Renovate's default branchPrefix.
const renovateBranchPrefix = "renovate/"

// RenovateTopic returns the branch topic of a Renovate branch name, the part after the last
// slash, and reports whether the name starts with Renovate's default "renovate/" prefix.
func RenovateTopic(branch string) (string, bool) {
	return branch[strings.LastIndex(branch, "/")+1:], strings.HasPrefix(branch, renovateBranchPrefix)
}

var (
	renovateDebug = regexp.MustCompile(`<!--\s*renovate-debug:([A-Za-z0-9+/=_-]+)\s*-->`)
	// topicSuffix is what Renovate's default branch topics put after the dependency name:
	// the new major or minor line, or the update type.
	topicSuffix = regexp.MustCompile(`^(v?\d+(\.\d+)*(\.x)?|digest|pin|rollback|replacement)$`)
)

// RenovateTargetBranch returns targetBranch from the renovate-debug comment (base64 JSON) that
// Renovate appends to its pull request bodies, and reports whether the body has one.
func RenovateTargetBranch(body string) (string, bool) {
	m := renovateDebug.FindStringSubmatch(body)
	if m == nil {
		return "", false
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		if raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(m[1], "=")); err != nil {
			return "", true
		}
	}
	var debug struct {
		TargetBranch string `json:"targetBranch"`
	}
	if json.Unmarshal(raw, &debug) != nil {
		return "", true
	}
	return debug.TargetBranch, true
}

// RenovateTopicMatches reports whether a Renovate branch topic, such as terraform-1.x or
// hashicorp-aws-6.x, names a dependency called one of names.
func RenovateTopicMatches(topic string, names ...string) bool {
	topic = strings.ToLower(topic)
	for _, n := range names {
		s := sanitizeDepName(n)
		if s == "" {
			continue
		}
		if topic == s {
			return true
		}
		if rest, ok := strings.CutPrefix(topic, s+"-"); ok && topicSuffix.MatchString(rest) {
			return true
		}
	}
	return false
}

// sanitizeDepName turns a dependency name into the form Renovate puts in branch names:
// lower case, without @, with slashes, colons and spaces as dashes.
func sanitizeDepName(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, "@", ""))
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', ':', ' ', '\t':
			return '-'
		}
		return r
	}, name)
	return strings.Trim(name, "-")
}
