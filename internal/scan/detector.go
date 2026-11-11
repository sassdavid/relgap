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
)

// Detectors returns every detector name, sorted. renovate reads --renovate-report instead of
// repository files.
func Detectors() []string {
	return []string{
		DetectorAqua, DetectorCI, DetectorDockerfile, DetectorGeneric, DetectorHelm, DetectorIdiomatic,
		DetectorMise, DetectorRenovate, DetectorTerraform, DetectorTerragrunt, DetectorTFLock,
		DetectorToolVersions,
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
		{DetectorTFLock, baseIs(".terraform.lock.hcl"), detectTFLock},
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
