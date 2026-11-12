package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rogpeppe/go-internal/txtar"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/mcpkit"
	"github.com/sassdavid/relgap/internal/testutil/fakebin"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/mcptest"
)

// mcpFixture is a directory and the deps a `relgap mcp` test server runs with.
type mcpFixture struct {
	dir string // the server's working directory
	d   *app.Deps
}

// newMCPFixture returns deps with a fixed clock, an empty home and cache, a dummy GitHub token
// and the cassettes; the working directory is a new temp dir.
func newMCPFixture(t *testing.T, now string, cassettes ...string) *mcpFixture {
	t.Helper()
	clock, err := time.Parse(time.RFC3339, now)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	home := t.TempDir()
	f := &mcpFixture{dir: dir}
	f.d = &app.Deps{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: io.Discard,
		Env: app.Env{
			"HOME=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
			"GH_TOKEN=" + ghtest.Token(),
			"PATH=" + t.TempDir(),
		},
		Clock: func() time.Time { return clock },
		Getwd: func() (string, error) { return f.dir, nil },
	}
	if len(cassettes) > 0 {
		paths := make([]string, len(cassettes))
		for i, c := range cassettes {
			paths[i] = filepath.Join(cassetteDir, filepath.FromSlash(c))
		}
		chain, openErr := openCassettes(paths)
		if openErr != nil {
			t.Fatal(openErr)
		}
		t.Cleanup(chain.stop)
		f.d.HTTPTransport = chain
	}
	return f
}

// extract writes the files of a testscript archive under the fixture's directory.
func (f *mcpFixture) extract(t *testing.T, script string) {
	t.Helper()
	a, err := txtar.ParseFile(filepath.Join(scriptDir, script))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range a.Files {
		name := filepath.Join(f.dir, filepath.FromSlash(file.Name))
		if err = os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(name, file.Data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// serve runs `relgap mcp args...` and calls test with a client connected to it.
func (f *mcpFixture) serve(t *testing.T, args []string, test func(c *mcptest.Client), roots ...string) {
	t.Helper()
	err := cli.ServeMCP(t.Context(), f.d, args, func(_ context.Context, s *mcpkit.Server) error {
		test(mcptest.Connect(t, s, roots...))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mcpCase is a tool call whose result is the --format json output in a golden file.
type mcpCase struct {
	name      string
	now       string
	cassettes []string
	setup     func(t *testing.T, f *mcpFixture)
	tool      string
	args      map[string]any
	golden    string
	schema    string
	items     string // JSON pointer to the list a page splits
}

func mcpCases() []mcpCase {
	kubectl := []string{"digest/kubectl-1.36.0..1.37.2.yaml"}
	return []mcpCase{
		{
			name: "digest", now: "2026-11-10T12:00:00Z", cassettes: kubectl,
			tool: "digest", args: map[string]any{"tool": "kubectl", "from": "1.36.0", "to": "1.37.2"},
			golden: "digest/kubectl.json", schema: "digest-v1.json", items: "/data/tools/0/entries",
		},
		{
			name: "breaking_changes", now: "2026-11-10T12:00:00Z", cassettes: kubectl,
			tool: "breaking_changes", args: map[string]any{"tool": "kubectl", "from": "1.36.0", "to": "1.37.2"},
			golden: "digest/kubectl-breaking.json", schema: "digest-v1.json", items: "/data/tools/0/entries",
		},
		{
			name: "outdated", now: "2026-11-11T12:00:00Z",
			cassettes: []string{"outdated/project.yaml", "outdated/project-added.yaml"},
			setup: func(t *testing.T, f *mcpFixture) {
				t.Helper()
				f.extract(t, "spec_output_and_ci_5.txtar")
				f.dir = filepath.Join(f.dir, "project")
			},
			tool: "outdated", args: map[string]any{},
			golden: "outdated/project.json", schema: "outdated-v1.json", items: "/data/pins",
		},
		{
			name: "drift", now: "2026-11-11T12:00:00Z",
			setup: func(t *testing.T, f *mcpFixture) {
				t.Helper()
				if err := writeDriftTree(filepath.Join(f.dir, "tree")); err != nil {
					t.Fatal(err)
				}
			},
			tool: "drift", args: map[string]any{"roots": []string{"tree"}},
			golden: "drift/tree.json", schema: "drift-v1.json", items: "/data/tools",
		},
		{
			name: "resolve", now: "2026-11-12T12:00:00Z",
			tool: "resolve", args: map[string]any{"tool": "terraform"},
			golden: "resolve/terraform.json", schema: "report-v1.json",
		},
	}
}

func (tc mcpCase) fixture(t *testing.T) *mcpFixture {
	t.Helper()
	f := newMCPFixture(t, tc.now, tc.cassettes...)
	if tc.setup != nil {
		tc.setup(t, f)
	}
	return f
}

// TestSpec_mcp_and_llm_1: an in-process MCP client calls every tool and gets the --format json
// golden output of the matching command.
func TestSpec_mcp_and_llm_1(t *testing.T) {
	t.Parallel()
	cases := mcpCases()
	tested := map[string]bool{}
	for _, tc := range cases {
		tested[tc.tool] = true
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := tc.fixture(t)
			f.serve(t, []string{"--max-result-bytes", "0"}, func(c *mcptest.Client) {
				c.Golden(t, tc.tool, tc.args, filepath.Join(goldenDir, filepath.FromSlash(tc.golden)))
			})
		})
	}
	f := newMCPFixture(t, "2026-11-12T12:00:00Z")
	f.serve(t, nil, func(c *mcptest.Client) {
		if info := c.ServerInfo(); info.Name != "relgap" || info.Version == "" {
			t.Errorf("server info = %+v", info)
		}
		for _, tool := range c.Tools(t) {
			if !tested[tool.Name] {
				t.Errorf("tool %s has no golden case", tool.Name)
			}
			if strings.Count(strings.TrimSuffix(tool.Description, "."), ". ") > 0 ||
				!strings.HasSuffix(tool.Description, ".") {
				t.Errorf("%s description is not one sentence: %q", tool.Name, tool.Description)
			}
		}
	})
}

// TestSpec_mcp_and_llm_2: a result over 50 KB is split into pages under the cap, each with a
// cursor to the next, and the pages together hold exactly the items of the whole result.
func TestSpec_mcp_and_llm_2(t *testing.T) {
	t.Parallel()
	for _, tc := range mcpCases() {
		golden, err := os.ReadFile(filepath.Join(goldenDir, filepath.FromSlash(tc.golden)))
		if err != nil {
			t.Fatal(err)
		}
		if tc.items == "" || len(compact(t, golden)) <= mcpkit.MaxBytes {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := tc.fixture(t)
			f.serve(t, nil, func(c *mcptest.Client) {
				checkPages(t, c.Pages(t, tc.tool, tc.args), golden, tc)
			})
		})
	}
}

func checkPages(t *testing.T, pages [][]byte, golden []byte, tc mcpCase) {
	t.Helper()
	if len(pages) < 2 {
		t.Fatalf("got %d page; want the %d-byte result split", len(pages), len(compact(t, golden)))
	}
	want := newGoldenItems(t, golden, tc.items)
	for n, p := range pages {
		checkPageFrame(t, n, p, golden, tc.schema, len(want.items), n == len(pages)-1)
		want.take(t, n, mcptest.Items(t, p, tc.items))
	}
	want.checkAllTaken(t)
	if tc.tool == "digest" {
		first := mcptest.Items(t, pages[0], tc.items)
		for _, kind := range []string{"breaking", "deprecation"} {
			if !slices.ContainsFunc(first, func(e json.RawMessage) bool {
				return bytes.Contains(e, []byte(`"kind":"`+kind+`"`))
			}) {
				t.Errorf("the first page has no %s entries; they must come first", kind)
			}
		}
	}
}

// checkPageFrame checks everything of page n but its items: size, page field, schema and the
// envelope fields every page repeats.
func checkPageFrame(t *testing.T, n int, p, golden []byte, schema string, total int, last bool) {
	t.Helper()
	page := mcptest.PageOf(t, p)
	switch {
	case len(p) > mcpkit.MaxBytes:
		t.Errorf("page %d has %d bytes, over the cap", n, len(p))
	case page == nil || page.Total != total:
		t.Fatalf("page %d: page field %+v, want a total of %d", n, page, total)
	case (page.NextCursor == "") != last:
		t.Errorf("page %d: next_cursor %q", n, page.NextCursor)
	}
	if err := validateJSON(schema, mcptest.Indent(t, p)); err != nil {
		t.Errorf("page %d: %v", n, err)
	}
	var got, want map[string]json.RawMessage
	if err := json.Unmarshal(p, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema_version", "tool", "version", "generated_at", "errors"} {
		if string(compact(t, got[k])) != string(compact(t, want[k])) {
			t.Errorf("page %d: %s = %s, want %s", n, k, got[k], want[k])
		}
	}
}

// goldenItems tracks which items of a golden list the pages have returned.
type goldenItems struct {
	items []json.RawMessage
	index map[string][]int // compact item -> its positions not yet returned
	seen  []bool
}

func newGoldenItems(t *testing.T, golden []byte, pointer string) *goldenItems {
	t.Helper()
	g := &goldenItems{items: mcptest.Items(t, golden, pointer), index: map[string][]int{}}
	g.seen = make([]bool, len(g.items))
	for i, item := range g.items {
		k := string(compact(t, item))
		g.index[k] = append(g.index[k], i)
	}
	return g
}

// take marks the items of page n returned; each must be in the golden list once, in order.
func (g *goldenItems) take(t *testing.T, n int, items []json.RawMessage) {
	t.Helper()
	last := -1
	for _, item := range items {
		k := string(item)
		if len(g.index[k]) == 0 {
			t.Fatalf("page %d has an item the golden file lacks, or has it twice: %.200s", n, k)
		}
		i := g.index[k][0]
		g.index[k] = g.index[k][1:]
		g.seen[i] = true
		if i < last {
			t.Errorf("page %d: item %d after item %d; a page keeps the original order", n, i, last)
		}
		last = i
	}
}

func (g *goldenItems) checkAllTaken(t *testing.T) {
	t.Helper()
	for i, ok := range g.seen {
		if !ok {
			t.Errorf("no page has item %d: %.200s", i, g.items[i])
		}
	}
}

func compact(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestMCP_roots: outdated and drift read only directories inside the server's roots: --root,
// else the client's roots, else the working directory. Symlinks can't lead out.
func TestMCP_roots(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	tests := []struct {
		name        string
		args        []string
		clientRoots func(f *mcpFixture) []string
		tool        string
		path        func(f *mcpFixture) any
		wantErr     string
	}{
		{"cwd root, relative path", nil, nil, "drift", func(*mcpFixture) any { return []string{"tree"} }, ""},
		{
			"cwd root, outside",
			nil,
			nil,
			"drift",
			func(*mcpFixture) any { return []string{outside} },
			"outside the server's roots",
		},
		{"cwd root, parent", nil, nil, "outdated", func(*mcpFixture) any { return ".." }, "outside the server's roots"},
		{"symlink out", nil, nil, "outdated", func(*mcpFixture) any { return "link" }, "outside the server's roots"},
		{"missing dir", nil, nil, "outdated", func(*mcpFixture) any { return "nope" }, "nope"},
		{
			"--root wins over the client",
			[]string{"--root", outside},
			func(f *mcpFixture) []string { return []string{f.dir} },
			"drift",
			func(f *mcpFixture) any { return []string{filepath.Join(f.dir, "tree")} },
			"outside the server's roots",
		},
		{
			"client roots", nil,
			func(f *mcpFixture) []string { return []string{outside, f.dir} },
			"drift", func(f *mcpFixture) any { return []string{filepath.Join(f.dir, "tree")} }, "",
		},
		{
			"client roots, relative to the first", nil,
			func(f *mcpFixture) []string { return []string{outside, f.dir} },
			"drift", func(*mcpFixture) any { return []string{"tree"} }, "tree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := rootsFixture(t, outside)
			var roots []string
			if tt.clientRoots != nil {
				roots = tt.clientRoots(f)
			}
			key := "path"
			if tt.tool == "drift" {
				key = "roots"
			}
			f.serve(t, tt.args, func(c *mcptest.Client) {
				checkResult(t, c.Call(t, tt.tool, map[string]any{key: tt.path(f)}), tt.wantErr)
			}, roots...)
		})
	}
}

// rootsFixture returns a working directory with a repository under tree and a link to outside.
func rootsFixture(t *testing.T, outside string) *mcpFixture {
	t.Helper()
	f := newMCPFixture(t, "2026-11-11T12:00:00Z")
	if err := os.MkdirAll(filepath.Join(f.dir, "tree", "repo", ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.dir, "link")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	return f
}

// checkResult fails unless res is a result (wantErr empty) or a tool error containing wantErr.
func checkResult(t *testing.T, res *mcp.CallToolResult, wantErr string) {
	t.Helper()
	if wantErr == "" {
		mcptest.Text(t, res)
		return
	}
	if msg := mcptest.ErrorText(t, res); !strings.Contains(msg, wantErr) {
		t.Errorf("message = %q, want %q", msg, wantErr)
	}
}

// TestMCP_noProcessesNoWrites: every tool runs without starting git, mise or gh, even when they
// are on PATH and no token is set, and leaves the roots, the home directory and the config
// unchanged; only the HTTP cache may change.
func TestMCP_noProcessesNoWrites(t *testing.T) {
	t.Parallel()
	f := newMCPFixture(t, "2026-11-11T12:00:00Z")
	f.d.HTTPTransport = notFound{} // a 404 isn't retried, so tokenless calls fail at once
	f.extract(t, "spec_output_and_ci_5.txtar")
	f.dir = filepath.Join(f.dir, "project")
	var fakes, path []string
	for _, name := range []string{"git", "mise", "gh"} {
		fake := fakebin.Install(t, name, fakebin.Spec{Rules: []fakebin.Rule{{Stdout: "[]"}}})
		fakes = append(fakes, fake)
		path = append(path, filepath.Dir(fake))
	}
	home := strings.TrimPrefix(f.d.Env[0], "HOME=")
	var env app.Env
	for _, kv := range f.d.Env {
		if !strings.HasPrefix(kv, "GH_TOKEN=") && !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+strings.Join(path, string(os.PathListSeparator)))
	f.d.Env = env
	cache := filepath.Join(home, ".cache")
	if err := os.MkdirAll(cache, 0o750); err != nil {
		t.Fatal(err)
	}
	before := tree(t, f.dir, home)
	f.serve(t, nil, func(c *mcptest.Client) {
		for _, call := range []struct {
			tool string
			args map[string]any
		}{
			{"resolve", map[string]any{"tool": "terraform"}},
			{"drift", map[string]any{"roots": []string{"."}, "latest": true}},
			{"outdated", map[string]any{}},
			{"digest", map[string]any{"tool": "kubectl", "from": "1.36.0", "to": "1.37.2"}},
			{"breaking_changes", map[string]any{"tool": "kubectl", "from": "1.36.0"}},
		} {
			c.Call(t, call.tool, call.args) // results don't matter, only what the calls touch
		}
	})
	for _, fake := range fakes {
		if calls := fakebin.Calls(t, fake); len(calls) > 0 {
			t.Errorf("%s ran: %+v", filepath.Base(fake), calls)
		}
	}
	after := tree(t, f.dir, home)
	for p, info := range after {
		if p == cache || strings.HasPrefix(p, cache+string(filepath.Separator)) {
			continue
		}
		if before[p] != info {
			t.Errorf("%s changed or appeared", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("%s disappeared", p)
		}
	}
}

// notFound answers every request with 404.
type notFound struct{}

func (notFound) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

// tree lists every path under dirs with its size, mode and modification time.
func tree(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			out[p] = info.Mode().String() + " " + info.ModTime().String() + " " + strconv.FormatInt(info.Size(), 10)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestMCP_errors: requests the CLI rejects with exit 2 are tool errors with the same message.
func TestMCP_errors(t *testing.T) {
	t.Parallel()
	f := newMCPFixture(t, "2026-11-12T12:00:00Z")
	f.serve(t, nil, func(c *mcptest.Client) {
		for _, tt := range []struct {
			tool string
			args map[string]any
			want string
		}{
			{"resolve", map[string]any{"tool": "no-such-tool-anywhere"}, `unknown tool "no-such-tool-anywhere"`},
			{"digest", map[string]any{"tool": "kubectl", "from": "1.36.0", "only": []string{"fixes"}}, `invalid kind "fixes"`},
			{"digest", map[string]any{"tool": "kubectl", "from": "1.36.0", "line": "patch"}, "patch"},
			{"digest", map[string]any{"tool": "kubectl"}, `missing required argument "from"`},
			{"drift", map[string]any{"roots": []string{}}, `missing required argument "roots"`},
			{"digest", map[string]any{"cursor": "AAAA"}, "unknown or expired cursor; call digest again without a cursor"},
		} {
			if msg := mcptest.ErrorText(t, c.Call(t, tt.tool, tt.args)); !strings.Contains(msg, tt.want) {
				t.Errorf("%s %v: message = %q, want %q", tt.tool, tt.args, msg, tt.want)
			}
		}
	})
}

// TestMCP_listsOnlyReadOnlyTools: every tool is annotated read-only and not destructive.
func TestMCP_listsOnlyReadOnlyTools(t *testing.T) {
	t.Parallel()
	f := newMCPFixture(t, "2026-11-12T12:00:00Z")
	f.serve(t, nil, func(c *mcptest.Client) {
		var names []string
		for _, tool := range c.Tools(t) {
			names = append(names, tool.Name)
			a := tool.Annotations
			if a == nil || !a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
				t.Errorf("%s annotations = %+v", tool.Name, a)
			}
		}
		slices.Sort(names)
		if want := []string{"breaking_changes", "digest", "drift", "outdated", "resolve"}; !slices.Equal(names, want) {
			t.Errorf("tools = %q, want %q", names, want)
		}
	})
}
