package scan

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// renovateFields parses `# renovate: key=value key=value`, the comment syntax Renovate's
// documentation uses for custom regex managers (datasource, depName, packageName, versioning).
func renovateFields(comment string) (map[string]string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(comment, "#")), "renovate:")
	if !ok {
		return nil, false
	}
	fields := map[string]string{}
	for tok := range strings.FieldsSeq(rest) {
		if k, v, found := strings.Cut(tok, "="); found {
			fields[k] = v
		}
	}
	return fields, true
}

// renovateLookup returns the name to resolve from the last renovate comment in comments.
func renovateLookup(comments []string) string {
	for _, c := range slices.Backward(comments) {
		if fields, ok := renovateFields(c); ok {
			return datasourceLookup(fields["datasource"], firstNonEmpty(fields["packageName"], fields["lookupName"]),
				fields["depName"])
		}
	}
	return ""
}

// datasourceLookup maps a Renovate datasource and package to a name the resolver understands.
// github-releases and github-tags name a repository; other datasources use the package name.
func datasourceLookup(datasource, pkg, dep string) string {
	name := firstNonEmpty(pkg, dep)
	switch datasource {
	case "github-releases", "github-tags":
		if strings.Count(name, "/") == 1 {
			return prefixGitHub + name
		}
	case DetectorAqua:
		return prefixAqua + name
	default:
	}
	return name
}

// renovatePackageFile is one entry of packageFiles in a Renovate report.
type renovatePackageFile struct {
	PackageFile string `json:"packageFile"`
	Deps        []struct {
		DepName      string `json:"depName"`
		PackageName  string `json:"packageName"`
		CurrentValue string `json:"currentValue"`
		Datasource   string `json:"datasource"`
		SkipReason   string `json:"skipReason"`
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
				p := renovateDepPin(manager, d.Datasource, d.DepName, d.PackageName)
				p.Value = d.CurrentValue
				p.Location = Location{Repo: repo, File: pf.PackageFile, Detector: DetectorRenovate}
				pins = append(pins, p)
			}
		}
	}
	return pins
}

// renovateDepPin sets the kind and the tool or lookup name of a Renovate dependency.
func renovateDepPin(manager, datasource, dep, pkg string) Pin {
	p := Pin{RawName: dep, Kind: KindTool}
	name := firstNonEmpty(pkg, dep)
	switch {
	case datasource == "docker":
		p.Kind, p.Tool = KindImage, "image:"+name
	case datasource == "terraform-provider":
		if !strings.Contains(name, "/") {
			name = "hashicorp/" + name
		}
		p.Kind, p.Tool = KindTFProvider, "tfprovider:"+providerName(name)
	case datasource == "helm":
		p.Kind, p.Tool = KindChart, "chart:"+dep
	case manager == "github-actions":
		p.Kind, p.Tool = KindAction, "action:"+name
	default:
		p.lookup = datasourceLookup(datasource, pkg, dep)
	}
	return p
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
