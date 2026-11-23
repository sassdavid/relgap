package enrich

import "github.com/sassdavid/relgap/internal/registry"

// NewTestBinary returns a binary of tool t built with Go goVersion from module at modVersion, as
// [ReadBinary] would read it.
func NewTestBinary(t registry.ToolSpec, goVersion, module, modVersion string) *Binary {
	return &Binary{
		Tool: t.Name, Path: "/bin/" + t.Name, GoVersion: goVersion, Module: module, ModuleVersion: modVersion,
		Queries: []Query{
			{Package: Package{Ecosystem: EcosystemGo, Name: goStdlib}, Version: goVersion},
			{Package: Package{Ecosystem: EcosystemGo, Name: module}, Version: modVersion},
		},
		Vulns: []Vuln{},
		spec:  t,
	}
}
