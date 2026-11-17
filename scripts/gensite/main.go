package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const strataSource = "testdata/golden/digest/kubectl.json"

// classes returns the entry classes the home page strata show, in the order of the legend in
// docs/plan/specs/docs-design.md.
func classes() []string {
	return []string{"breaking", "deprecation", "security", "feature", "fix", "other"}
}

func main() {
	os.Exit(run())
}

func run() int {
	root := flag.String("root", ".", "repository root")
	fetch := flag.Bool("fetch-action", false,
		"copy relgap-action's action.yml at the commit "+actionPinFile+" pins, then regenerate")
	flag.Parse()
	if *fetch {
		if err := fetchAction(context.Background(), *root); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "gensite: %v\n", err)
			return 1
		}
	}
	stale, problems, err := generate(*root)
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(os.Stderr, "gensite: %v\n", err)
		return 1
	case len(problems) > 0:
		for _, p := range problems {
			_, _ = fmt.Fprintln(os.Stderr, p)
		}
		return 1
	case len(stale) > 0:
		for _, f := range stale {
			_, _ = fmt.Fprintf(os.Stderr, "%s was stale and has been regenerated; review and commit it\n", f)
		}
		return 1
	default:
		return 0
	}
}

// generate writes every file gensite owns and returns the ones it changed, and the problems the
// transcript check found.
func generate(root string) ([]string, []string, error) {
	files := map[string][]byte{}
	strata, err := strataFile(root)
	if err != nil {
		return nil, nil, err
	}
	files["docs/.vitepress/theme/data/home-strata.json"] = strata
	for _, gen := range []func(string, map[string][]byte) error{
		toolsPage, snapshotsPage, registrySchemaPage, sarifRulesPage, partsFiles, actionPart, sentToModelFile,
	} {
		if err = gen(root, files); err != nil {
			return nil, nil, err
		}
	}
	var stale []string
	for _, rel := range sortedKeys(files) {
		changed, werr := writeIfChanged(root, rel, files[rel])
		if werr != nil {
			return nil, nil, werr
		}
		stale = append(stale, changed...)
	}
	problems, err := checkTranscripts(root)
	if err != nil {
		return nil, nil, err
	}
	return stale, problems, nil
}

// strataFile returns the home page strata from the kubectl digest golden.
func strataFile(root string) ([]byte, error) {
	golden, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strataSource)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", strataSource, err)
	}
	s, err := buildStrata(golden)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strataSource, err)
	}
	return jsonFile(s)
}

func jsonFile(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return append(b, '\n'), nil
}

type strata struct {
	Source   string   `json:"source"`
	Tool     string   `json:"tool"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Layers   []layer  `json:"layers"`
	Breaking []string `json:"breaking"`
}

// layer is one release; Counts has every class, so the component needs no defaults.
type layer struct {
	Version string         `json:"version"`
	Counts  map[string]int `json:"counts"`
	Total   int            `json:"total"`
}

type digestReport struct {
	Data struct {
		Tools []struct {
			Tool     string `json:"tool"`
			From     string `json:"from"`
			To       string `json:"to"`
			Versions []struct {
				Version string `json:"version"`
			} `json:"versions"`
			Entries []struct {
				Version string `json:"version"`
				Kind    string `json:"kind"`
			} `json:"entries"`
		} `json:"tools"`
	} `json:"data"`
}

// buildStrata counts the entries of the first tool in a digest report per release and class,
// oldest release first.
func buildStrata(report []byte) (strata, error) {
	var r digestReport
	if err := json.Unmarshal(report, &r); err != nil {
		return strata{}, fmt.Errorf("decode digest report: %w", err)
	}
	if len(r.Data.Tools) == 0 {
		return strata{}, errors.New("digest report has no tools")
	}
	t := r.Data.Tools[0]
	s := strata{Source: strataSource, Tool: t.Tool, From: t.From, To: t.To, Breaking: []string{}}
	index := map[string]int{}
	for _, v := range t.Versions {
		counts := make(map[string]int, len(classes()))
		for _, c := range classes() {
			counts[c] = 0
		}
		index[v.Version] = len(s.Layers)
		s.Layers = append(s.Layers, layer{Version: v.Version, Counts: counts})
	}
	for _, e := range t.Entries {
		i, ok := index[e.Version]
		if !ok {
			return strata{}, fmt.Errorf("entry for %s, which is not in the range", e.Version)
		}
		if _, known := s.Layers[i].Counts[e.Kind]; !known {
			return strata{}, fmt.Errorf("entry of unknown class %q", e.Kind)
		}
		s.Layers[i].Counts[e.Kind]++
		s.Layers[i].Total++
	}
	for _, l := range s.Layers {
		if l.Counts["breaking"] > 0 {
			s.Breaking = append(s.Breaking, l.Version)
		}
	}
	return s, nil
}

func writeIfChanged(root, rel string, data []byte) ([]string, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	have, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	if bytes.Equal(have, data) {
		return nil, nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(rel), err)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", rel, err)
	}
	return []string{rel}, nil
}
