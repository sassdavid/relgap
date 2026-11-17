package scan

import (
	"path"
	"strings"
)

// Detector names, as shown in Location.Detector.
const (
	DetectorMise         = "mise"
	DetectorToolVersions = "tool-versions"
	DetectorAqua         = "aqua"
	DetectorIdiomatic    = "idiomatic"
	DetectorTerraform    = "terraform"
	DetectorTFLock       = "tf-lock"
	DetectorTerragrunt   = "terragrunt"
	DetectorDockerfile   = "dockerfile"
	DetectorCI           = "ci"
	DetectorHelm         = "helm"
	DetectorGeneric      = "generic"
	DetectorRenovate     = "renovate"
	DetectorTFCensus     = "tfcensus"
)

// Detectors returns every detector name, sorted. renovate reads --renovate-report and tfcensus
// reads drift --from-tfcensus instead of repository files.
func Detectors() []string {
	return []string{
		DetectorAqua, DetectorCI, DetectorDockerfile, DetectorGeneric, DetectorHelm, DetectorIdiomatic,
		DetectorMise, DetectorRenovate, DetectorTerraform, DetectorTerragrunt, DetectorTFLock,
		DetectorTFCensus, DetectorToolVersions,
	}
}

// DetectorDoc says which files a detector reads and which pins it reports.
type DetectorDoc struct {
	Name     string   `json:"name"`
	Files    string   `json:"files"`
	Pins     string   `json:"pins"`
	Examples []string `json:"examples,omitempty"` // repository paths the detector reads
}

// DetectorDocs returns a [DetectorDoc] for each name in [Detectors], in the same order.
func DetectorDocs() []DetectorDoc {
	return []DetectorDoc{
		{
			DetectorAqua, "aqua.yaml and aqua.yml, with or without a leading dot, and YAML files in an aqua directory",
			"packages[] entries written as name: pkg@version or name plus version",
			[]string{"aqua.yaml", ".aqua.yml", "aqua/tools.yaml"},
		},
		{
			DetectorCI, "GitHub Actions workflows, .gitlab-ci.yml and bitbucket-pipelines.yml",
			"image keys and job containers as image pins, keys such as TERRAFORM_VERSION or go-version as tool " +
				"pins, and uses: as action pins with scan.detectors.ci_actions; values with variables are skipped",
			[]string{".github/workflows/ci.yml", ".gitlab-ci.yml", "bitbucket-pipelines.yml"},
		},
		{
			DetectorDockerfile, "Dockerfile*, Containerfile* and *.dockerfile",
			"FROM image tags with ARG defaults filled in, and ARG or ENV NAME_VERSION values; a # renovate: " +
				"comment above an ARG or ENV names the tool",
			[]string{"Dockerfile", "build/Containerfile.dev", "app.dockerfile"},
		},
		{
			DetectorGeneric, "the files scan.extra_files names, with scan.detectors.generic on",
			"NAME_VERSION = x.y.z assignments in any text file",
			nil,
		},
		{
			DetectorHelm, "Chart.yaml",
			"dependencies[] as chart pins, and appVersion as a pin of the tool named like the chart",
			[]string{"Chart.yaml", "charts/app/Chart.yaml"},
		},
		{
			DetectorIdiomatic, ".terraform-version, .terragrunt-version, .node-version, .nvmrc, .python-version, " +
				".go-version and go.mod",
			"the first version line of a version file; go.mod's go line as a constraint and its toolchain line",
			[]string{
				".terraform-version", ".terragrunt-version", ".node-version", ".nvmrc", ".python-version",
				".go-version", goMod,
			},
		},
		{
			DetectorMise, "mise.toml, .mise.toml, mise.*.toml and the conf.d files of a mise directory",
			"each version under [tools]: a string, each string of an array, or the version key of a table",
			[]string{"mise.toml", ".mise.toml", "mise.local.toml", ".config/mise/conf.d/go.toml"},
		},
		{
			DetectorRenovate, "the file pins --renovate-report names, not repository files",
			"each dependency of Renovate's extract report, resolved like the other pins",
			nil,
		},
		{
			DetectorTerraform, "*.tf and *.tf.json",
			"terraform { required_version } and required_providers constraints; a provider without a source " +
				"is hashicorp/<name>",
			[]string{"main.tf", "modules/vpc/versions.tf.json"},
		},
		{
			DetectorTerragrunt, "every .hcl file except .terraform.lock.hcl",
			"terraform_version_constraint and terragrunt_version_constraint as literal strings at the top level",
			[]string{"terragrunt.hcl", "live/prod/root.hcl"},
		},
		{
			DetectorTFLock, tfLockFile,
			"the exact version of each provider",
			[]string{tfLockFile},
		},
		{
			DetectorTFCensus, "the tfcensus inventory drift --from-tfcensus reads, not repository files",
			"exact terraform or opentofu and provider versions per unit and workspace",
			nil,
		},
		{
			DetectorToolVersions, ".tool-versions",
			"a tool name followed by one or more versions",
			[]string{".tool-versions", "services/api/.tool-versions"},
		},
	}
}

// file is a repository file handed to a detector.
type file struct {
	rel  string // slash-separated, relative to the repository
	data []byte
}

type detector struct {
	name   string
	match  func(rel string) bool
	detect func(f file) ([]Pin, error)
}

func (o *Options) detectors() []detector {
	ds := []detector{
		{DetectorMise, isMiseConfig, detectMise},
		{DetectorToolVersions, baseIs(".tool-versions"), detectToolVersions},
		{DetectorAqua, isAquaConfig, detectAqua},
		{DetectorIdiomatic, isIdiomatic, detectIdiomatic},
		{DetectorTerraform, isTerraform, detectTerraform},
		{DetectorTFLock, baseIs(tfLockFile), detectTFLock},
		{DetectorTerragrunt, isTerragrunt, detectTerragrunt},
		{DetectorDockerfile, isDockerfile, detectDockerfile},
		{DetectorCI, isCI, o.detectCI},
		{DetectorHelm, baseIs("Chart.yaml"), detectHelm},
	}
	if o.Generic && len(o.ExtraFiles) > 0 {
		ds = append(ds, detector{DetectorGeneric, o.isExtraFile, detectGeneric})
	}
	return ds
}

func baseIs(name string) func(string) bool {
	return func(rel string) bool { return path.Base(rel) == name }
}

// lines splits data into lines without their line endings.
func lines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// stripComment removes a # comment that starts the line or follows whitespace.
func stripComment(s string) string {
	for i := range len(s) {
		if s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return s[:i]
		}
	}
	return s
}

func (o *Options) isExtraFile(rel string) bool {
	for _, pat := range o.ExtraFiles {
		if globMatch(pat, rel) {
			return true
		}
	}
	return false
}
