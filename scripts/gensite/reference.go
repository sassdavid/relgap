package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/render"
	curated "github.com/sassdavid/relgap/registry"
)

// toolsPage writes /reference/tools from the curated registry entries in the binary.
func toolsPage(_ string, files map[string][]byte) error {
	l, err := registry.NewLinter()
	if err != nil {
		return fmt.Errorf("registry linter: %w", err)
	}
	entries, err := l.LoadFS(curated.FS(), ".")
	if err != nil {
		return fmt.Errorf("load curated registry: %w", err)
	}
	t := &table{head: []string{"Tool", "Aliases", "Repository", "Sources", "Versions", "EOL", "OSV"}}
	for _, e := range entries {
		var kinds []string
		for _, s := range e.Sources {
			if !slices.Contains(kinds, s.Kind) {
				kinds = append(kinds, s.Kind)
			}
		}
		sources := codes(kinds)
		if sources == "" {
			sources = "auto-discovery"
		}
		scheme := e.Version.Scheme
		if scheme == "" {
			scheme = "loose"
		}
		universe := e.Version.Universe
		if universe == "" {
			universe = registry.UniverseReleases
		}
		eol := ""
		if e.EOL != nil {
			eol = link(e.EOL.Product, "https://endoflife.date/"+e.EOL.Product)
		}
		var osv []string
		for _, p := range e.OSV {
			osv = append(osv, p.Ecosystem+": "+code(p.Name))
		}
		t.add(code(e.Name), codes(e.Aliases), link(e.Repo, "https://github.com/"+e.Repo), sources,
			scheme+", "+universe, eol, strings.Join(osv, ", "))
	}
	body := fmt.Sprintf(`relgap resolves any tool that mise's or aqua's registry knows, or that you map to a
repository with `+"`tools.<name>.repo`"+`. For those tools it finds release notes by auto-discovery:
the GitHub release bodies when they hold more than boilerplate, else a changelog file in the
repository's top directory.

The %d tools below have a curated registry entry in the binary. An entry says where the notes are
(sources), how tags become versions, which [endoflife.date](https://endoflife.date) product holds
the release cycles, and which [OSV](https://osv.dev) packages carry the vulnerabilities. Without
an entry, a tool has no EOL or vulnerability data.

`+"`relgap registry update`"+` downloads newer entries from the repository, and `+"`relgap registry lint`"+`
checks your own against the [registry entry schema](/reference/registry-schema).

%s
Versions shows the [version scheme](/reference/registry-schema#version) and whether a range
counts GitHub releases or tags.
`, len(entries), t)
	files["docs/reference/tools.md"] = page(
		"Supported tools",
		"The tools relgap's curated registry describes: aliases, repository, release-note sources, EOL product and OSV packages.",
		body,
	)
	return nil
}

// snapshotsPage writes /reference/snapshots from the metadata of the embedded registry copies.
func snapshotsPage(_ string, files map[string][]byte) error {
	miseMeta, miseEntries, err := registry.MiseSnapshot()
	if err != nil {
		return fmt.Errorf("read mise snapshot: %w", err)
	}
	aquaMeta, aquaEntries, err := registry.AquaSnapshot()
	if err != nil {
		return fmt.Errorf("read aqua snapshot: %w", err)
	}
	t := &table{head: []string{"Registry", "Source", "Commit", "Commit date", "Entries"}}
	row := func(name string, m registry.SnapshotMeta, n int) {
		t.add(name, link(strings.TrimPrefix(m.Source, "https://github.com/"), m.Source),
			link(code(m.Commit[:12]), m.Source+"/commit/"+m.Commit), m.CommitDate, strconv.Itoa(n))
	}
	row("mise", miseMeta, len(miseEntries))
	row("aqua", aquaMeta, len(aquaEntries))
	body := `relgap maps a tool name to its GitHub repository with copies of two registries that ship inside
the binary. Both registries are MIT-licensed, and relgap keeps their license texts with the copies.

` + t.String() + `
When mise is on ` + "`PATH`" + `, relgap reads ` + "`mise registry --json`" + ` instead of the mise copy, so the
registry matches the mise you run. The aqua copy is always the embedded one. This page describes
the copies in the latest release.
`
	files["docs/reference/snapshots.md"] = page("Registry snapshots",
		"The mise and aqua registry copies embedded in relgap: source commit, date and entry count.", body)
	return nil
}

// schemaNode is the part of a JSON schema the registry schema page reads.
type schemaNode struct {
	Type        any                    `json:"type"`
	Description string                 `json:"description"`
	Enum        []any                  `json:"enum"`
	Const       any                    `json:"const"`
	Pattern     string                 `json:"pattern"`
	Required    []string               `json:"required"`
	Properties  map[string]*schemaNode `json:"properties"`
	Items       *schemaNode            `json:"items"`
	AllOf       []struct {
		If struct {
			Properties map[string]*schemaNode `json:"properties"`
		} `json:"if"`
		Then struct {
			Required []string `json:"required"`
			AnyOf    []struct {
				Required []string `json:"required"`
			} `json:"anyOf"`
		} `json:"then"`
	} `json:"allOf"`
	Title string `json:"title"`
	ID    string `json:"$id"`
}

func readRegistrySchema(root string) (*schemaNode, error) {
	b, err := os.ReadFile(filepath.Join(root, "schemas", "registry-v1.json"))
	if err != nil {
		return nil, fmt.Errorf("read registry schema: %w", err)
	}
	var s schemaNode
	if err = json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("decode registry schema: %w", err)
	}
	return &s, nil
}

// registrySchemaPage writes /reference/registry-schema from schemas/registry-v1.json.
func registrySchemaPage(root string, files map[string][]byte) error {
	s, err := readRegistrySchema(root)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s The schema is served at [/schemas/registry-v1.json](/schemas/registry-v1.json), and\n"+
		"editors that read the `yaml-language-server` comment check an entry as you type.\n\n",
		prose(s.Description))
	b.WriteString("```yaml\n# yaml-language-server: $schema=" + s.ID + "\n```\n\n")
	top := &table{head: []string{"Key", "Type", "Required", "Allowed", colDescription}}
	addSchemaRows(top, "", s)
	b.WriteString(top.String())
	for _, name := range sortedKeys(s.Properties) {
		n := s.Properties[name]
		obj := n
		key := name
		if n.Items != nil && n.Items.Properties != nil {
			obj, key = n.Items, name+"[]"
		}
		if obj.Properties == nil {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", name)
		if n.Description != "" {
			b.WriteString(prose(n.Description) + "\n\n")
		}
		t := &table{head: []string{"Key", "Type", "Required", "Allowed", colDescription}}
		addSchemaRows(t, key+".", obj)
		b.WriteString(t.String())
		if rules := kindRules(obj); rules != nil {
			b.WriteString("\nEach source kind needs these keys:\n\n")
			b.WriteString(rules.String())
		}
	}
	files["docs/reference/registry-schema.md"] = page("Registry entry schema",
		"Every key of a relgap registry entry, its type, allowed values and the keys each source kind needs.",
		b.String())
	return nil
}

// addSchemaRows adds one row per property of n, recursing into nested objects with prefix.
func addSchemaRows(t *table, prefix string, n *schemaNode) {
	for _, name := range sortedKeys(n.Properties) {
		p := n.Properties[name]
		key := prefix + name
		req := ""
		if slices.Contains(n.Required, name) {
			req = "yes"
		}
		if prefix == "" && (p.Properties != nil || (p.Items != nil && p.Items.Properties != nil)) {
			t.add(link(code(key), "#"+name), schemaType(p), req, "", prose(p.Description))
			continue
		}
		t.add(code(key), schemaType(p), req, allowed(p), prose(p.Description))
		if p.Properties != nil {
			addSchemaRows(t, key+".", p)
		}
	}
}

func schemaType(n *schemaNode) string {
	switch {
	case n.Enum != nil:
		return "string"
	case n.Items != nil:
		if n.Items.Properties != nil {
			return "list of objects"
		}
		return "list of " + schemaType(n.Items) + "s"
	}
	if s, ok := n.Type.(string); ok {
		return s
	}
	return ""
}

func allowed(n *schemaNode) string {
	switch {
	case n.Enum != nil:
		var vals []string
		for _, v := range n.Enum {
			vals = append(vals, fmt.Sprint(v))
		}
		return codes(vals)
	case n.Pattern != "":
		return "pattern " + code(n.Pattern)
	case n.Items != nil && n.Items.Pattern != "":
		return "pattern " + code(n.Items.Pattern)
	}
	return ""
}

// kindRules returns the keys each source kind requires, from the schema's if/then rules, or nil
// when n has none.
func kindRules(n *schemaNode) *table {
	if len(n.AllOf) == 0 {
		return nil
	}
	required := map[string]string{}
	for _, r := range n.AllOf {
		kind, ok := r.If.Properties["kind"]
		if !ok || kind.Const == nil {
			continue
		}
		var parts []string
		if len(r.Then.Required) > 0 {
			parts = append(parts, codes(r.Then.Required))
		}
		if len(r.Then.AnyOf) > 0 {
			var alt []string
			for _, a := range r.Then.AnyOf {
				alt = append(alt, codes(a.Required))
			}
			parts = append(parts, "one of "+strings.Join(alt, " or "))
		}
		required[fmt.Sprint(kind.Const)] = strings.Join(parts, ", and ")
	}
	t := &table{head: []string{"Kind", "Required keys"}}
	for _, k := range n.Properties["kind"].Enum {
		name := fmt.Sprint(k)
		need := required[name]
		if need == "" {
			need = "none besides `kind`"
		}
		t.add(code(name), need)
	}
	return t
}

// sarifRulesPage writes /reference/sarif-rules from the rules render puts in SARIF logs.
func sarifRulesPage(_ string, files map[string][]byte) error {
	t := &table{head: []string{"Rule", "Level", "Commands", "Description", "Help"}}
	ids := slices.Concat(render.OutdatedRules(), render.DriftRules())
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		r := render.SARIFRule(id)
		if r.ID != id {
			return fmt.Errorf("render has no SARIF rule %q", id)
		}
		var cmds []string
		if slices.Contains(render.OutdatedRules(), id) {
			cmds = append(cmds, link("outdated", "/cli/outdated"))
		}
		if slices.Contains(render.DriftRules(), id) {
			cmds = append(cmds, link("drift", "/cli/drift"))
		}
		t.add(code(id), r.DefaultConfiguration.Level, strings.Join(cmds, ", "),
			prose(r.ShortDescription.Text)+". "+prose(r.FullDescription.Text), prose(r.Help.Text))
	}
	body := "`relgap outdated --format sarif` and `relgap drift --format sarif` write SARIF 2.1.0 for GitHub code\n" +
		"scanning. Each log lists the rules below, and every result points at the pin's file and line.\n\n" +
		t.String() + `
## Vulnerabilities

outdated adds one rule per OSV advisory that affects a pinned version, with the OSV ID as the rule
ID. Its level comes from the advisory's CVSS rating: error for high and critical, warning for
medium, and note for low or no rating. The rule carries the score as ` + "`security-severity`" + `, which
code scanning shows as the alert's severity, and its help links to the advisory on osv.dev.

## Fingerprints

Every result has the partial fingerprint ` + "`relgapFinding/v1`" + `, built from the rule, the tool, the file and
what the result is about. GitHub ignores it and hashes the pin's line itself; other SARIF readers can
match results across runs with it.
`
	files["docs/reference/sarif-rules.md"] = page("SARIF rules",
		"The SARIF rules relgap outdated and drift report to code scanning, with their levels and help text.", body)
	return nil
}
