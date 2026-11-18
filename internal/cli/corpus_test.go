package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
