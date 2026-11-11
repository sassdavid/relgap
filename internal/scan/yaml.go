package scan

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"
)

func parseYAML(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return &doc, nil
}

// get returns the value of key in mapping m, or nil.
func get(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node) (string, bool) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Value == "" {
		return "", false
	}
	return n.Value, true
}

// isAquaConfig matches aqua.yaml, .aqua.yaml and YAML files in an aqua or .aqua directory.
func isAquaConfig(rel string) bool {
	base, ext := path.Base(rel), path.Ext(rel)
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(base, "."), ext)
	return name == DetectorAqua || strings.TrimPrefix(path.Base(path.Dir(rel)), ".") == DetectorAqua
}

// detectAqua reads packages[] entries written as `name: pkg@version` or `name` plus `version`.
func detectAqua(f file) ([]Pin, error) {
	root, err := parseYAML(f.data)
	if err != nil {
		return nil, err
	}
	pkgs := get(root, "packages")
	if pkgs == nil || pkgs.Kind != yaml.SequenceNode {
		return nil, nil
	}
	var pins []Pin
	for _, p := range pkgs.Content {
		nameNode := get(p, "name")
		name, ok := scalar(nameNode)
		if !ok {
			continue
		}
		line := nameNode.Line
		version := ""
		if i := strings.LastIndexByte(name, '@'); i > 0 {
			name, version = name[:i], name[i+1:]
		} else if vn := get(p, keyVersion); vn != nil {
			version, line = vn.Value, vn.Line
		}
		if version == "" {
			continue
		}
		pins = append(pins, Pin{
			RawName: name, Kind: KindTool, Value: version,
			Location: Location{Line: line}, lookup: prefixAqua + name,
		})
	}
	return pins, nil
}

// detectHelm reads Chart.yaml: dependencies[] become chart pins and appVersion pins the tool
// named like the chart.
func detectHelm(f file) ([]Pin, error) {
	root, err := parseYAML(f.data)
	if err != nil {
		return nil, err
	}
	var pins []Pin
	chart, _ := scalar(get(root, "name"))
	if vn := get(root, "appVersion"); chart != "" {
		if v, ok := scalar(vn); ok {
			pins = append(pins, Pin{RawName: chart, Kind: KindTool, Value: v, Location: Location{Line: vn.Line}})
		}
	}
	if deps := get(root, "dependencies"); deps != nil && deps.Kind == yaml.SequenceNode {
		for _, d := range deps.Content {
			name, okName := scalar(get(d, "name"))
			vn := get(d, keyVersion)
			v, okVersion := scalar(vn)
			if !okName || !okVersion {
				continue
			}
			pins = append(pins, Pin{
				Tool: "chart:" + name, RawName: name, Kind: KindChart, Value: v,
				Location: Location{Line: vn.Line},
			})
		}
	}
	return pins, nil
}

// isCI matches GitHub Actions workflows, bitbucket-pipelines.yml and .gitlab-ci.yml.
func isCI(rel string) bool {
	switch rel {
	case "bitbucket-pipelines.yml", ".gitlab-ci.yml":
		return true
	}
	ext := path.Ext(rel)
	return path.Dir(rel) == ".github/workflows" && (ext == ".yml" || ext == ".yaml")
}

var versionKey = regexp.MustCompile(`(?i)^([a-z0-9_]+?)[_-]version$`)

// detectCI walks the whole document: image keys (and a GitHub job's container) give image pins,
// keys such as TERRAFORM_VERSION or go-version give tool pins and, with o.Actions, uses gives
// action pins. Values with variables are skipped.
func (o *Options) detectCI(f file) ([]Pin, error) {
	root, err := parseYAML(f.data)
	if err != nil {
		return nil, err
	}
	var pins []Pin
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if p, ok := o.ciPin(n.Content[i].Value, n.Content[i+1]); ok {
					pins = append(pins, p)
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(root)
	return pins, nil
}

func (o *Options) ciPin(key string, val *yaml.Node) (Pin, bool) {
	// GitLab writes image: {name: ...}. A GitHub container: {image: ...} is found by the walk.
	if key == "image" {
		val = cmpNode(get(val, "name"), val)
	}
	v, ok := scalar(val)
	if !ok || strings.Contains(v, "$") {
		return Pin{}, false
	}
	loc := Location{Line: val.Line}
	switch key {
	case "image", "container":
		return imagePin(v, loc)
	case "uses":
		if ref, isDocker := strings.CutPrefix(v, "docker://"); isDocker {
			return imagePin(ref, loc)
		}
		action, version, found := strings.Cut(v, "@")
		if !o.Actions || !found || strings.HasPrefix(action, ".") {
			return Pin{}, false
		}
		return Pin{Tool: "action:" + action, RawName: action, Kind: KindAction, Value: version, Location: loc}, true
	}
	if m := versionKey.FindStringSubmatch(key); m != nil {
		return Pin{RawName: key, Kind: KindTool, Value: v, Location: loc, lookup: envName(key)}, true
	}
	return Pin{}, false
}

func cmpNode(ns ...*yaml.Node) *yaml.Node {
	for _, n := range ns {
		if n != nil {
			return n
		}
	}
	return nil
}

// envName turns go-version or terraform_version into GO_VERSION or TERRAFORM_VERSION, the form
// the registry resolver maps to a tool.
func envName(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}

// imagePin splits an image reference into name and tag. Images without a tag pin nothing.
func imagePin(ref string, loc Location) (Pin, bool) {
	ref, _, _ = strings.Cut(ref, "@")
	slash := strings.LastIndexByte(ref, '/')
	colon := strings.LastIndexByte(ref, ':')
	if colon <= slash || colon == len(ref)-1 {
		return Pin{}, false
	}
	name, tag := ref[:colon], ref[colon+1:]
	return Pin{Tool: "image:" + name, RawName: name, Kind: KindImage, Value: tag, Location: loc}, true
}
