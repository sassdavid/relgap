package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type corpusTool struct {
	Tool      string   `yaml:"tool"`
	Range     string   `yaml:"range"`
	Adapters  []string `yaml:"adapters"`
	Breaking  int      `yaml:"breaking"`
	Tolerance int      `yaml:"tolerance"`
	Golden    string   `yaml:"golden"`
	Skip      string   `yaml:"skip"`
}

// readCorpus returns the tools of testdata/corpus.yaml.
func readCorpus(t *testing.T) []corpusTool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "corpus.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Tools []corpusTool `yaml:"tools"`
	}
	if err = yaml.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c.Tools
}

// TestCorpusGoldenCounts keeps the breaking count in corpus.yaml equal to the count in the tool's
// golden. The live corpus checks the count against GitHub, so a change that moves the golden and
// leaves corpus.yaml behind would fail every nightly run.
func TestCorpusGoldenCounts(t *testing.T) {
	t.Parallel()
	for _, tool := range readCorpus(t) {
		if tool.Golden == "" {
			continue
		}
		t.Run(tool.Tool, func(t *testing.T) {
			t.Parallel()
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", tool.Golden))
			if err != nil {
				t.Fatal(err)
			}
			var golden struct {
				Data struct {
					Tools []struct {
						Counts map[string]int `json:"counts"`
					} `json:"tools"`
				} `json:"data"`
			}
			if err = json.Unmarshal(b, &golden); err != nil {
				t.Fatalf("decode %s: %v", tool.Golden, err)
			}
			if len(golden.Data.Tools) != 1 {
				t.Fatalf("%s has %d tools, want 1", tool.Golden, len(golden.Data.Tools))
			}
			if got := golden.Data.Tools[0].Counts["breaking"]; got != tool.Breaking {
				t.Errorf(
					"testdata/golden/%s has %d breaking entries, corpus.yaml says %d: update corpus.yaml",
					tool.Golden, got, tool.Breaking,
				)
			}
		})
	}
}

// newVersions returns the versions in the digest data got that the golden data want lacks, each
// with its publish date. A new patch of an older supported line sorts inside a range.
func newVersions(want, got []byte) ([]string, error) {
	type versions struct {
		Tools []struct {
			Versions []struct {
				Version     string `json:"version"`
				PublishedAt string `json:"published_at"`
			} `json:"versions"`
		} `json:"tools"`
	}
	var w, g versions
	if err := json.Unmarshal(want, &w); err != nil {
		return nil, fmt.Errorf("decode golden versions: %w", err)
	}
	if err := json.Unmarshal(got, &g); err != nil {
		return nil, fmt.Errorf("decode digest versions: %w", err)
	}
	var known []string
	for _, tool := range w.Tools {
		for _, v := range tool.Versions {
			known = append(known, v.Version)
		}
	}
	var added []string
	for _, tool := range g.Tools {
		for _, v := range tool.Versions {
			if slices.Contains(known, v.Version) {
				continue
			}
			date, _, _ := strings.Cut(v.PublishedAt, "T")
			added = append(added, fmt.Sprintf("%s (published %s)", v.Version, date))
		}
	}
	return added, nil
}

func TestNewVersions(t *testing.T) {
	t.Parallel()
	golden := `{"tools":[{"versions":[{"version":"3.5.7","published_at":"2026-11-02T20:14:51Z"},` +
		`{"version":"3.6.1","published_at":"2026-11-04T14:20:31Z"}]}]}`
	tests := []struct {
		name string
		got  string
		want []string
	}{
		{"same versions", golden, nil},
		{
			"backport inside the range",
			`{"tools":[{"versions":[{"version":"3.5.7","published_at":"2026-11-02T20:14:51Z"},` +
				`{"version":"3.5.8","published_at":"2026-11-18T19:54:30Z"},` +
				`{"version":"3.6.1","published_at":"2026-11-04T14:20:31Z"}]}]}`,
			[]string{"3.5.8 (published 2026-11-18)"},
		},
		{"version gone", `{"tools":[{"versions":[{"version":"3.6.1"}]}]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := newVersions([]byte(golden), []byte(tt.got))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("newVersions() = %q, want %q", got, tt.want)
			}
		})
	}
}
