package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sassdavid/relgap/internal/source"
)

func TestBuildStrata(t *testing.T) {
	t.Parallel()
	report := []byte(`{"data": {"tools": [{
		"tool": "kubectl", "from": "1.30.0", "to": "1.32.0",
		"versions": [{"version": "1.31.0"}, {"version": "1.32.0"}],
		"entries": [
			{"version": "1.32.0", "kind": "breaking"},
			{"version": "1.32.0", "kind": "fix"},
			{"version": "1.31.0", "kind": "fix"},
			{"version": "1.31.0", "kind": "feature"}
		]
	}]}}`)
	got, err := buildStrata(report)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tool != "kubectl" || got.From != "1.30.0" || got.To != "1.32.0" {
		t.Errorf("tool %q range %s..%s", got.Tool, got.From, got.To)
	}
	if len(got.Layers) != 2 || got.Layers[0].Version != "1.31.0" || got.Layers[1].Version != "1.32.0" {
		t.Fatalf("layers %+v, want 1.31.0 then 1.32.0", got.Layers)
	}
	if got.Layers[1].Total != 2 || got.Layers[1].Counts["breaking"] != 1 || got.Layers[0].Counts["security"] != 0 {
		t.Errorf("counts %+v", got.Layers)
	}
	if len(got.Breaking) != 1 || got.Breaking[0] != "1.32.0" {
		t.Errorf("breaking %v, want [1.32.0]", got.Breaking)
	}
}

func TestBuildStrataRejects(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"no tools":      `{"data": {"tools": []}}`,
		"outside range": `{"data": {"tools": [{"versions": [{"version": "1.0.0"}], "entries": [{"version": "2.0.0", "kind": "fix"}]}]}}`,
		"unknown class": `{"data": {"tools": [{"versions": [{"version": "1.0.0"}], "entries": [{"version": "1.0.0", "kind": "chore"}]}]}}`,
	}
	for name, report := range tests {
		if _, err := buildStrata([]byte(report)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

const repoRoot = "../.."

// TestNetworkHosts checks the network host table against every host a cassette recorded, so a
// new host can't reach the code without reaching the docs.
func TestNetworkHosts(t *testing.T) {
	t.Parallel()
	hosts, err := networkHosts()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, h := range hosts {
		listed[h.name] = true
	}
	hostLine := regexp.MustCompile(`(?m)^\s{8}host: (\S+)$`)
	seen := map[string]bool{}
	err = filepath.WalkDir(
		filepath.Join(repoRoot, "testdata", "cassettes"),
		func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ".yaml") {
				return err
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			for _, m := range hostLine.FindAllSubmatch(b, -1) {
				seen[string(m[1])] = true
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 5 {
		t.Fatalf("found hosts %v in the cassettes, want more", seen)
	}
	for h := range seen {
		// Cassettes stand for a host in gitlab.hosts or images.registries with one under .test,
		// the TLD RFC 2606 reserves, and the table names those as config entries.
		if !listed[h] && !strings.HasSuffix(h, ".test") {
			t.Errorf("a cassette sends a request to %s, which the network host table doesn't list", h)
		}
	}
}

// TestSourceKinds checks that source.Kinds lists the kinds of the registry schema, in its order.
func TestSourceKinds(t *testing.T) {
	t.Parallel()
	s, err := readRegistrySchema(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, k := range s.Properties["sources"].Items.Properties["kind"].Enum {
		want = append(want, fmt.Sprint(k))
	}
	var got []string
	for _, k := range source.Kinds() {
		got = append(got, k.Kind)
		if k.Reads == "" {
			t.Errorf("%s: no description", k.Kind)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("source.Kinds() = %v, schema enum %v", got, want)
	}
}

// TestPlaceholders checks that every placeholder has a meaning and expands.
func TestPlaceholders(t *testing.T) {
	t.Parallel()
	out, err := placeholdersTable()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| `{minor}` | the minor number alone | `32` |", "| `{prev_minor}` |"} {
		if !strings.Contains(out, want) {
			t.Errorf("placeholder table lacks %q:\n%s", want, out)
		}
	}
}

func TestReadActionPin(t *testing.T) {
	t.Parallel()
	const a, b = "28d7a84b3070faa38338522a5e83e427eeae80b5", "4c1862331d5a9380bab6f9fcd7f65c74839d8370"
	tests := map[string]struct {
		workflow string
		want     actionPin
		wantErr  bool
	}{
		"one pin": {"- uses: sassdavid/relgap-action@" + a + " # v1.1.0\n", actionPin{a, "v1.1.0"}, false},
		"two same": {
			"uses: sassdavid/relgap-action@" + a + " # v1.1.0\nuses: sassdavid/relgap-action@" + a + " # v1.1.0\n",
			actionPin{a, "v1.1.0"},
			false,
		},
		"two pins": {
			"uses: sassdavid/relgap-action@" + a + " # v1.1.0\nuses: sassdavid/relgap-action@" + b + " # v1.0.0\n",
			actionPin{},
			true,
		},
		"tag only": {"uses: sassdavid/relgap-action@v1\n", actionPin{}, true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(actionPinFile))
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.workflow), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readActionPin(root)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("readActionPin() = %+v, %v; want %+v, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

// TestActionPart checks that the committed copy of action.yml is of the pinned commit.
func TestActionPart(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{}
	if err := actionPart(repoRoot, files); err != nil {
		t.Fatal(err)
	}
	got := string(files[partsDir+"relgap-action.md"])
	for _, want := range []string{"| `mode` | `install` |", "<code v-pre>${{ github.token }}</code>", "| `exit-code` |"} {
		if !strings.Contains(got, want) {
			t.Errorf("relgap-action table lacks %q", want)
		}
	}
}

func TestTranscriptProblems(t *testing.T) {
	t.Parallel()
	scripts := "\nexec relgap pins --format plain\ngolden stdout pins/project.txt\n"
	tests := map[string]struct {
		page string
		want int
	}{
		"match": {"```sh\nrelgap pins --format plain\n```\n\n<<< @/../testdata/golden/pins/project.txt\n", 0},
		"prompt": {
			"```sh\n$ relgap  pins --format plain\n```\n<<< @/../testdata/golden/pins/project.txt{text}\n",
			0,
		},
		"other flags": {"```sh\nrelgap pins\n```\n<<< @/../testdata/golden/pins/project.txt\n", 1},
		"no command":  {"<<< @/../testdata/golden/pins/project.txt\n", 1},
		"command reused": {
			"```sh\nrelgap pins --format plain\n```\n<<< @/../testdata/golden/pins/project.txt\n<<< @/../testdata/golden/pins/project.txt\n",
			1,
		},
	}
	for name, tc := range tests {
		if got := transcriptProblems([]byte(tc.page), scripts); len(got) != tc.want {
			t.Errorf("%s: problems %v, want %d", name, got, tc.want)
		}
	}
}

// TestTranscripts runs the transcript check on the site, as mise run docs does.
func TestTranscripts(t *testing.T) {
	t.Parallel()
	problems, err := checkTranscripts(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
