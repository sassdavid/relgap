package scan

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// isMiseConfig matches mise.toml, .mise.toml, mise.*.toml and .mise.*.toml anywhere, plus
// config.toml, config.*.toml and conf.d/*.toml inside a mise config directory (mise, .mise, _mise).
func isMiseConfig(rel string) bool {
	base := path.Base(rel)
	if path.Ext(base) != ".toml" {
		return false
	}
	name := strings.TrimPrefix(base, ".")
	if name == "mise.toml" || strings.HasPrefix(name, "mise.") {
		return true
	}
	dir := path.Dir(rel)
	if path.Base(dir) == "conf.d" {
		return isMiseDir(path.Base(path.Dir(dir)))
	}
	return (base == "config.toml" || strings.HasPrefix(base, "config.")) && isMiseDir(path.Base(dir))
}

func isMiseDir(name string) bool {
	return strings.TrimLeft(name, "._") == "mise"
}

func detectMise(f file) ([]Pin, error) {
	var p unstable.Parser
	p.Reset(f.data)
	var table []string
	var pins []Pin
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind == unstable.Table || e.Kind == unstable.ArrayTable {
			table = nil
			if e.Kind == unstable.Table {
				table = keyParts(e.Key())
			}
			continue
		}
		if e.Kind == unstable.KeyValue {
			key := append(slices.Clone(table), keyParts(e.Key())...)
			pins = append(pins, misePins(&p, key, e.Value())...)
		}
	}
	if err := p.Error(); err != nil {
		if pe, ok := errors.AsType[*unstable.ParserError](err); ok && len(pe.Highlight) > 0 {
			return nil, fmt.Errorf("line %d: %s", p.Shape(p.Range(pe.Highlight)).Start.Line, pe.Message)
		}
		return nil, fmt.Errorf("parse toml: %w", err)
	}
	return pins, nil
}

// misePins reads `name = value` under [tools] and `version = value` under [tools.name]. A value
// is a string, an array of strings or a table with a version key.
func misePins(p *unstable.Parser, key []string, val *unstable.Node) []Pin {
	const toolKey, versionKey = 2, 3
	if len(key) < toolKey || key[0] != "tools" {
		return nil
	}
	name := key[1]
	switch {
	case len(key) == versionKey && key[2] == keyVersion:
	case len(key) != toolKey:
		return nil
	case val.Kind == unstable.InlineTable:
		val = inlineVersion(val)
	default:
	}
	var nodes []*unstable.Node
	switch {
	case val == nil:
	case val.Kind == unstable.String:
		nodes = append(nodes, val)
	case val.Kind == unstable.Array:
		for it := val.Children(); it.Next(); {
			if n := it.Node(); n.Kind == unstable.String {
				nodes = append(nodes, n)
			}
		}
	default:
	}
	pins := make([]Pin, 0, len(nodes))
	for _, n := range nodes {
		pins = append(pins, Pin{
			RawName:  name,
			Kind:     KindTool,
			Value:    string(n.Data),
			Location: Location{Line: p.Shape(n.Raw).Start.Line},
			lookup:   miseLookup(name),
		})
	}
	return pins
}

func inlineVersion(tbl *unstable.Node) *unstable.Node {
	for it := tbl.Children(); it.Next(); {
		kv := it.Node()
		if kv.Kind == unstable.KeyValue && slices.Equal(keyParts(kv.Key()), []string{keyVersion}) {
			return kv.Value()
		}
	}
	return nil
}

func keyParts(it unstable.Iterator) []string {
	var parts []string
	for it.Next() {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

// miseLookup turns a mise tool key into a name the registry resolver understands: options in
// brackets go, core: and legacy asdf names are dropped and ubi:owner/repo becomes
// github:owner/repo.
func miseLookup(name string) string {
	if i := strings.IndexByte(name, '['); i > 0 {
		name = name[:i]
	}
	// The asdf plugin names mise still accepts for its own tools.
	switch name = strings.TrimPrefix(name, "core:"); name {
	case "dotnet-core":
		return "dotnet"
	case "golang":
		return "go"
	case "nodejs":
		return toolNode
	}
	if rest, ok := strings.CutPrefix(name, "ubi:"); ok {
		return prefixGitHub + rest
	}
	return name
}
