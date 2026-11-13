//go:build live

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/testutil"
)

// liveOutEnv names the directory that gets results.json and, for each drifting tool, the golden
// data it expected and the data it got.
const liveOutEnv = "RELGAP_LIVE_OUT"

const defaultTolerance = 2

type corpusTool struct {
	Tool      string   `yaml:"tool"`
	Range     string   `yaml:"range"`
	Adapters  []string `yaml:"adapters"`
	Breaking  int      `yaml:"breaking"`
	Tolerance int      `yaml:"tolerance"`
	Golden    string   `yaml:"golden"`
	Skip      string   `yaml:"skip"`
}

// liveResult is one tool's line in results.json, which the live workflow turns into issues.
type liveResult struct {
	Tool     string   `json:"tool"`
	Slug     string   `json:"slug"`
	Range    string   `json:"range,omitempty"`
	Status   string   `json:"status"` // ok, drift or skip
	Problems []string `json:"problems,omitempty"`
	Golden   string   `json:"golden,omitempty"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9.]+`)

// lineAnchor is the #L<n> suffix of a source URL. Upstream files such as a CHANGELOG.md on HEAD or
// a release branch grow above the entries, which moves the anchors without changing any entry.
var lineAnchor = regexp.MustCompile(`#L\d+(-L\d+)?"`)

// TestLiveCorpus digests every corpus range against GitHub and the web, as the nightly live
// workflow does. It reports drift and never writes a golden or corpus.yaml.
//
//nolint:paralleltest // one tool at a time keeps the run inside the GITHUB_TOKEN rate limit
func TestLiveCorpus(t *testing.T) {
	if testutil.Updating() {
		t.Fatal("the live corpus never updates goldens; review the drift, then run mise run test:golden -- -update")
	}
	out := os.Getenv(liveOutEnv)
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
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
	cache := t.TempDir()
	var results []liveResult
	for _, tool := range c.Tools {
		res := liveResult{
			Tool: tool.Tool, Slug: strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(tool.Tool), "-"), "-"),
			Range: tool.Range, Golden: tool.Golden,
		}
		t.Run(res.Slug, func(t *testing.T) { //nolint:paralleltest // see TestLiveCorpus
			if tool.Skip != "" {
				res.Status = "skip"
				res.Problems = []string{tool.Skip}
				t.Skip(tool.Skip)
			}
			res.Problems = checkLive(t, tool, cache, filepath.Join(out, res.Slug))
			res.Status = "ok"
			if len(res.Problems) > 0 {
				res.Status = "drift"
				t.Errorf("%s %s drifted:\n%s", tool.Tool, tool.Range, strings.Join(res.Problems, "\n"))
			}
		})
		results = append(results, res)
	}
	j, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "results.json"), append(j, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// checkLive digests one corpus range and returns what differs from corpus.yaml and the golden.
// The expected and actual golden data go to prefix.want.json and prefix.got.json.
func checkLive(t *testing.T, tool corpusTool, cache, prefix string) []string {
	t.Helper()
	var problems []string
	data, err := liveDigest(t, cache, "digest", tool.Tool, tool.Range, "--format", "json")
	if err != nil {
		return []string{err.Error()}
	}
	all, err := liveDigest(t, cache, "digest", tool.Tool, tool.Range, "--format", "json", "--min-confidence", "0")
	if err != nil {
		return []string{err.Error()}
	}
	var adapters []string
	for _, e := range all.Tools[0].Entries {
		if !slices.Contains(adapters, e.Source.Adapter) {
			adapters = append(adapters, e.Source.Adapter)
		}
	}
	for _, a := range tool.Adapters {
		if !slices.Contains(adapters, a) {
			problems = append(
				problems,
				fmt.Sprintf("adapter %s produced no entries (got %s)", a, strings.Join(adapters, ", ")),
			)
		}
	}
	tolerance := tool.Tolerance
	if tolerance == 0 {
		tolerance = defaultTolerance
	}
	if got := data.Tools[0].Counts["breaking"]; got < tool.Breaking-tolerance || got > tool.Breaking+tolerance {
		problems = append(
			problems,
			fmt.Sprintf("breaking entries: got %d, want %d ± %d", got, tool.Breaking, tolerance),
		)
	}
	if tool.Golden == "" {
		return problems
	}
	golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", tool.Golden))
	if err != nil {
		return append(problems, err.Error())
	}
	want, err := envelopeData(golden)
	if err != nil {
		return append(problems, fmt.Sprintf("read golden %s: %v", tool.Golden, err))
	}
	if !bytes.Equal(lineAnchor.ReplaceAll(want, []byte(`"`)), lineAnchor.ReplaceAll(data.raw, []byte(`"`))) {
		problems = append(problems, fmt.Sprintf("the digest differs from testdata/golden/%s", tool.Golden))
		_ = os.WriteFile(prefix+".want.json", want, 0o600)
		_ = os.WriteFile(prefix+".got.json", data.raw, 0o600)
	}
	return problems
}

type liveData struct {
	Tools []struct {
		Counts  map[string]int `json:"counts"`
		Entries []struct {
			Source struct {
				Adapter string `json:"adapter"`
			} `json:"source"`
		} `json:"entries"`
	} `json:"tools"`
	raw []byte // the data object, indented, for comparing with a golden
}

// liveDigest runs relgap in process against the network, with a cache shared by the corpus.
func liveDigest(t *testing.T, cache string, args ...string) (*liveData, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	d := &app.Deps{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
		Env:    app.Env(append(os.Environ(), "RELGAP_CACHE_DIR="+cache, "NO_COLOR=1")),
		Clock:  time.Now,
		Getwd:  os.Getwd,
	}
	if code := cli.Execute(ctx, d, args); code != 0 {
		return nil, fmt.Errorf(
			"relgap %s exited %d: %s",
			strings.Join(args, " "),
			code,
			strings.TrimSpace(stderr.String()),
		)
	}
	raw, err := envelopeData(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	var out liveData
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode digest: %w", err)
	}
	if len(out.Tools) != 1 {
		return nil, fmt.Errorf("digest has %d tools, want 1", len(out.Tools))
	}
	out.raw = raw
	return &out, nil
}

// envelopeData returns the data object of a JSON envelope, indented, without the envelope's
// timestamps and version.
func envelopeData(b []byte) ([]byte, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	var compact, buf bytes.Buffer
	if err := json.Compact(&compact, env.Data); err != nil {
		return nil, fmt.Errorf("compact data: %w", err)
	}
	if err := json.Indent(&buf, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("indent data: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
