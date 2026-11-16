package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
)

// TFCensusSchemaVersion is the schema_version of the tfcensus inventory that [TFCensusInventory]
// reads (tfcensus schemas/inventory-v1.json).
const TFCensusSchemaVersion = "1"

const (
	tfcensusTool             = "tfcensus"
	tfcensusDefaultWorkspace = "default"
	tfcensusOpenTofu         = "opentofu"
	tfcensusLockfile         = "lockfile"
	tfLockFile               = ".terraform.lock.hcl"
	rawTerraformVersion      = "terraform_version"
	defaultStateFile         = "terraform.tfstate"
)

// ErrTFCensusInventory is the error of an input that isn't a tfcensus inventory relgap reads.
var ErrTFCensusInventory = errors.New("not a tfcensus inventory relgap reads")

type tfcensusUnit struct {
	Repo             string `json:"repo"`
	Path             string `json:"path"`
	Workspace        string `json:"workspace"`
	Key              string `json:"key"`
	Engine           string `json:"engine"`
	TerraformVersion string `json:"terraform_version"`
	Providers        []struct {
		Source        string `json:"source"`
		Version       string `json:"version"`
		VersionSource string `json:"version_source"`
	} `json:"providers"`
}

// TFCensusInventory reads the JSON of `tfcensus inventory --format json` into a repository per
// unit and workspace, its state and lock file versions into pins, and its errors into problems.
func TFCensusInventory(data []byte) (*Result, error) {
	var inv struct {
		SchemaVersion json.RawMessage `json:"schema_version"`
		Tool          string          `json:"tool"`
		Data          *struct {
			Units *[]tfcensusUnit `json:"units"`
		} `json:"data"`
		Errors []struct {
			Target  string `json:"target"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTFCensusInventory, err)
	}
	if inv.Tool != tfcensusTool {
		return nil, fmt.Errorf("%w: tool is %q, want %q", ErrTFCensusInventory, inv.Tool, tfcensusTool)
	}
	got, want := bytes.TrimSpace(inv.SchemaVersion), `"`+TFCensusSchemaVersion+`"`
	switch {
	case len(got) == 0:
		return nil, fmt.Errorf("%w: schema_version is missing; relgap reads %s", ErrTFCensusInventory, want)
	case string(got) != want:
		return nil, fmt.Errorf("%w: schema_version %s isn't supported; relgap reads %s",
			ErrTFCensusInventory, got, want)
	}
	if inv.Data == nil || inv.Data.Units == nil {
		return nil, fmt.Errorf("%w: data.units is missing", ErrTFCensusInventory)
	}
	res := &Result{Repos: []string{}, Pins: []Pin{}}
	for _, u := range *inv.Data.Units {
		repo := tfcensusUnitName(u.Repo, u.Path, u.Workspace)
		res.Repos = append(res.Repos, repo)
		res.Pins = append(res.Pins, tfcensusPins(repo, u)...)
	}
	for _, e := range inv.Errors {
		res.Problems = append(res.Problems, Problem{Repo: e.Target, Message: "tfcensus: " + e.Message})
	}
	slices.Sort(res.Repos)
	res.Repos = slices.Compact(res.Repos)
	slices.SortFunc(res.Pins, comparePins)
	return res, nil
}

// tfcensusUnitName returns repo/path[:workspace], the form tfcensus uses for error targets.
func tfcensusUnitName(repo, dir, workspace string) string {
	name := path.Join(repo, dir)
	if workspace != "" && workspace != tfcensusDefaultWorkspace {
		name += ":" + workspace
	}
	return name
}

// stateFile returns the name of the state object, which stands in for a file in the unit.
func stateFile(key string) string {
	if base := path.Base(key); key != "" && base != "/" && base != "." {
		return base
	}
	return defaultStateFile
}

// tfcensusPins returns the unit's terraform or opentofu version and its lock file's provider
// versions.
func tfcensusPins(repo string, u tfcensusUnit) []Pin {
	var pins []Pin
	if u.TerraformVersion != "" {
		tool := toolTerraform
		if u.Engine == tfcensusOpenTofu {
			tool = tfcensusOpenTofu
		}
		pins = append(pins, Pin{
			Tool: tool, RawName: rawTerraformVersion, Kind: KindTool, Value: u.TerraformVersion,
			Location: Location{Repo: repo, File: stateFile(u.Key), Detector: DetectorTFCensus},
		})
	}
	for _, p := range u.Providers {
		if p.Version == "" || p.Source == "" || p.VersionSource != tfcensusLockfile {
			continue
		}
		pin := tfProviderPin(p.Source, p.Version, 0)
		pin.Location = Location{Repo: repo, File: tfLockFile, Detector: DetectorTFCensus}
		pins = append(pins, pin)
	}
	for i := range pins {
		pins[i].classify(true)
	}
	return pins
}

// Merge adds the repositories, pins and problems of o to r and sorts them again.
func (r *Result) Merge(o *Result) {
	r.Repos = append(r.Repos, o.Repos...)
	slices.Sort(r.Repos)
	r.Repos = slices.Compact(r.Repos)
	r.Pins = append(r.Pins, o.Pins...)
	slices.SortFunc(r.Pins, comparePins)
	r.Problems = append(r.Problems, o.Problems...)
	sortProblems(r.Problems)
}
