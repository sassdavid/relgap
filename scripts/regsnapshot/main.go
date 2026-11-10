package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/misex"
	"github.com/sassdavid/relgap/internal/registry"
)

func main() {
	os.Exit(run())
}

func run() int {
	miseDir := flag.String("mise", "", "checkout of jdx/mise")
	miseCommit := flag.String("mise-commit", "", "commit of the mise checkout")
	miseDate := flag.String("mise-date", "", "commit date of the mise checkout (RFC 3339)")
	aquaDir := flag.String("aqua", "", "checkout of aquaproj/aqua-registry")
	aquaCommit := flag.String("aqua-commit", "", "commit of the aqua-registry checkout")
	aquaDate := flag.String("aqua-date", "", "commit date of the aqua-registry checkout (RFC 3339)")
	out := flag.String("out", filepath.Join("internal", "registry", "snapshot"), "output directory")
	flag.Parse()

	err := writeMise(*miseDir, registry.SnapshotMeta{
		Source: "https://github.com/jdx/mise", Commit: *miseCommit, CommitDate: *miseDate,
	}, *out)
	if err == nil {
		err = writeAqua(*aquaDir, registry.SnapshotMeta{
			Source: "https://github.com/aquaproj/aqua-registry", Commit: *aquaCommit, CommitDate: *aquaDate,
		}, *out)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "regsnapshot: %v\n", err)
		return 1
	}
	return 0
}

type miseTOML struct {
	Aliases  []string `toml:"aliases"`
	Backends []any    `toml:"backends"`
}

func writeMise(dir string, meta registry.SnapshotMeta, out string) error {
	if dir == "" || meta.Commit == "" || meta.CommitDate == "" {
		return errors.New("-mise, -mise-commit and -mise-date are required")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "registry", "*.toml"))
	if err != nil {
		return fmt.Errorf("list mise registry: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no registry/*.toml under %s", dir)
	}
	entries := make([]misex.RegistryEntry, 0, len(paths))
	for _, p := range paths {
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", p, readErr)
		}
		var t miseTOML
		if err = toml.Unmarshal(b, &t); err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		e := misex.RegistryEntry{Short: strings.TrimSuffix(filepath.Base(p), ".toml"), Aliases: t.Aliases}
		for _, b := range t.Backends {
			switch v := b.(type) {
			case string:
				e.Backends = append(e.Backends, v)
			case map[string]any:
				if full, ok := v["full"].(string); ok {
					e.Backends = append(e.Backends, full)
				}
			}
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b misex.RegistryEntry) int { return strings.Compare(a.Short, b.Short) })
	return write(filepath.Join(out, registry.MiseSnapshotFile), meta, entries)
}

type aquaFile struct {
	Packages []aquaPackage `yaml:"packages"`
}

type aquaPackage struct {
	Name          string `yaml:"name"`
	RepoOwner     string `yaml:"repo_owner"`
	RepoName      string `yaml:"repo_name"`
	VersionPrefix string `yaml:"version_prefix"`
	VersionFilter string `yaml:"version_filter"`
	VersionSource string `yaml:"version_source"`
	Aliases       []struct {
		Name string `yaml:"name"`
	} `yaml:"aliases"`
}

func writeAqua(dir string, meta registry.SnapshotMeta, out string) error {
	if dir == "" || meta.Commit == "" || meta.CommitDate == "" {
		return errors.New("-aqua, -aqua-commit and -aqua-date are required")
	}
	p := filepath.Join(dir, "registry.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %s: %w", p, err)
	}
	var f aquaFile
	if err = yaml.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("parse %s: %w", p, err)
	}
	entries := make([]registry.AquaEntry, 0, len(f.Packages))
	for _, pkg := range f.Packages {
		e := registry.AquaEntry{
			Name:          pkg.Name,
			VersionPrefix: pkg.VersionPrefix,
			VersionFilter: pkg.VersionFilter,
			VersionSource: pkg.VersionSource,
		}
		if pkg.RepoOwner != "" && pkg.RepoName != "" {
			e.Repo = pkg.RepoOwner + "/" + pkg.RepoName
		}
		if e.Name == "" {
			e.Name = e.Repo
		}
		if e.Name == "" {
			continue
		}
		for _, a := range pkg.Aliases {
			e.Aliases = append(e.Aliases, a.Name)
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b registry.AquaEntry) int { return strings.Compare(a.Name, b.Name) })
	return write(filepath.Join(out, registry.AquaSnapshotFile), meta, entries)
}

// write puts one entry per line so that snapshot updates review as small diffs.
func write[E any](path string, meta registry.SnapshotMeta, entries []E) error {
	head, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(head[:len(head)-1])
	buf.WriteString(`,"entries":[`)
	for i, e := range entries {
		line, encErr := json.Marshal(e)
		if encErr != nil {
			return fmt.Errorf("encode entry: %w", encErr)
		}
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString("\n")
		buf.Write(line)
	}
	buf.WriteString("\n]}\n")
	if err = os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
