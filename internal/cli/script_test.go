package cli_test

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/sassdavid/relgap/internal/app"
	"github.com/sassdavid/relgap/internal/cli"
	"github.com/sassdavid/relgap/internal/testutil"
	"github.com/sassdavid/relgap/internal/testutil/ghtest"
	"github.com/sassdavid/relgap/internal/testutil/vcr"
	"github.com/sassdavid/relgap/schemas"
)

const (
	scriptDir   = "../../testdata/script"
	cassetteDir = "../../testdata/cassettes"
)

// Test-only environment read by the relgap command of the test binary.
const (
	// envCassette replays (or with VCR_RECORD=1 records) every HTTP request from this cassette.
	envCassette = "TEST_CASSETTE"
	// envRedirect sends every HTTP request to this base URL, such as a fake server.
	envRedirect = "TEST_REDIRECT"
	// envNow fixes the clock (RFC 3339), so JSON envelopes are stable.
	envNow = "TEST_NOW"
	// envToken is a real token while recording and a dummy one otherwise. Scripts that test the
	// token path set GH_TOKEN=$TEST_GH_TOKEN. No RELGAP_ prefix: config warns about unknown ones.
	envToken = "TEST_GH_TOKEN"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"relgap": func() {
			os.Exit(runRelgap(os.Args[1:]))
		},
		// expect-exit <code> relgap [args...] succeeds only when relgap exits with <code>.
		"expect-exit": func() {
			os.Exit(expectExit(os.Args[1:]))
		},
	})
}

func expectExit(args []string) int {
	const minArgs = 2
	if len(args) < minArgs || args[1] != "relgap" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: expect-exit <code> relgap [args...]")
		return 1
	}
	want, err := strconv.Atoi(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "expect-exit: bad code %q\n", args[0])
		return 1
	}
	got := runRelgap(args[2:])
	if got != want {
		_, _ = fmt.Fprintf(os.Stderr, "expect-exit: relgap exited %d, want %d\n", got, want)
		return 1
	}
	return 0
}

// runRelgap runs the CLI the way main does, with the test hooks from the environment.
func runRelgap(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	d := &app.Deps{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    app.Env(os.Environ()),
		Clock:  time.Now,
		Getwd:  os.Getwd,
	}
	if now := os.Getenv(envNow); now != "" {
		t, err := time.Parse(time.RFC3339, now)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", envNow, err)
			return 1
		}
		d.Clock = func() time.Time { return t }
	}
	if base := os.Getenv(envRedirect); base != "" {
		u, err := url.Parse(base)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s: %v\n", envRedirect, err)
			return 1
		}
		d.HTTPTransport = redirect{base: u, rt: http.DefaultTransport}
	}
	if path := os.Getenv(envCassette); path != "" {
		rec, err := vcr.New(path, vcr.Options{Record: vcr.Recording(), Scrubber: ghtest.Scrubber(os.Getenv(envToken))})
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			return 1
		}
		d.HTTPTransport = rec
		defer func() {
			if stopErr := rec.Stop(); stopErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "save cassette: %v\n", stopErr)
				os.Exit(1)
			}
		}()
	}
	return cli.Execute(ctx, d, args)
}

type redirect struct {
	base *url.URL
	rt   http.RoundTripper
}

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host, out.Host = r.base.Scheme, r.base.Host, r.base.Host
	return r.rt.RoundTrip(out)
}

func scriptParams(t *testing.T, files ...string) testscript.Params {
	t.Helper()
	return testscript.Params{
		Files:               files,
		UpdateScripts:       testutil.Updating(),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Setup: func(env *testscript.Env) error {
			env.Setenv("HOME", filepath.Join(env.WorkDir, "home"))
			env.Setenv("XDG_CONFIG_HOME", filepath.Join(env.WorkDir, ".config"))
			env.Setenv("XDG_CACHE_HOME", filepath.Join(env.WorkDir, ".cache"))
			env.Setenv("NO_COLOR", "1")
			// A -race binary sleeps 1s before exiting by default, which digest_interrupt would count.
			env.Setenv("GORACE", "atexit_sleep_ms=0")
			// Keep only testscript's own bin directory, so a mise installed on the machine doesn't
			// change resolve output. Scripts that need a fake mise prepend its directory.
			bin, _, _ := strings.Cut(env.Getenv("PATH"), string(os.PathListSeparator))
			env.Setenv("PATH", bin)
			cassettes, err := filepath.Abs(cassetteDir)
			if err != nil {
				return err
			}
			env.Setenv("CASSETTES", cassettes)
			env.Setenv(envToken, ghtest.Token())
			if vcr.Recording() {
				env.Setenv(vcr.RecordEnv, "1")
			}
			env.Values[slowServerKey{}] = &slowServer{}
			return nil
		},
		// [recording] is true under VCR_RECORD=1. Scripts that only replay another script's
		// cassette skip then, so two processes never record the same file.
		Condition: func(cond string) (bool, error) {
			if cond == "recording" {
				return vcr.Recording(), nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"jsonschema": cmdJSONSchema,
			"slowgithub": cmdSlowGitHub,
		},
	}
}

// cmdJSONSchema validates stdout, stderr or a file against an embedded schema:
//
//	jsonschema report-v1.json stdout
func cmdJSONSchema(ts *testscript.TestScript, neg bool, args []string) {
	const wantArgs = 2
	if len(args) != wantArgs {
		ts.Fatalf("usage: jsonschema <schema-file> <stdout|stderr|file>")
	}
	// ReadFile understands "stdout" and "stderr" as the last command's output.
	err := validateJSON(args[0], []byte(ts.ReadFile(args[1])))
	switch {
	case neg && err == nil:
		ts.Fatalf("%s unexpectedly valid against %s", args[1], args[0])
	case !neg && err != nil:
		ts.Fatalf("%s invalid against %s: %v", args[1], args[0], err)
	}
}

// validateJSON validates data against an embedded schema. Every embedded schema is loaded, so
// digest-v1.json can refer to report-v1.json.
func validateJSON(schemaName string, data []byte) error {
	names, globErr := fs.Glob(os.DirFS("../../schemas"), "*.json")
	if globErr != nil {
		return fmt.Errorf("list schemas: %w", globErr)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	ids := map[string]string{}
	for _, name := range names {
		raw, err := schemas.Read(name)
		if err != nil {
			return err
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("parse schema %s: %w", name, err)
		}
		id, _ := doc.(map[string]any)["$id"].(string)
		ids[name] = cmp.Or(id, name)
		if err = c.AddResource(ids[name], doc); err != nil {
			return fmt.Errorf("add schema %s: %w", name, err)
		}
	}
	sch, err := c.Compile(cmp.Or(ids[schemaName], schemaName))
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse instance: %w", err)
	}
	return sch.Validate(inst) //nolint:wrapcheck // the validation error is the message
}

// TestScripts runs every testdata/script/*.txtar that isn't bound to a spec criterion.
func TestScripts(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob(filepath.Join(scriptDir, "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, m := range matches {
		if !strings.HasPrefix(filepath.Base(m), "spec_") {
			files = append(files, m)
		}
	}
	testscript.Run(t, scriptParams(t, files...))
}

// TestSpec_config_1: `config show --sources` prints key, value and source for every key (golden).
func TestSpec_config_1(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_config_1.txtar")))
}

// TestSpec_tool_resolution_1: resolve prints the same ToolSpec for tf, terraform,
// TERRAFORM_VERSION and aqua:hashicorp/terraform, with the layer of each field (golden).
func TestSpec_tool_resolution_1(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_tool_resolution_1.txtar")))
}

// TestSpec_tool_resolution_4: registry lint rejects unknown keys and bad regexes with file:line.
func TestSpec_tool_resolution_4(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_tool_resolution_4.txtar")))
}

// TestSpec_config_2: unknown keys and wrong types each produce a precise error.
func TestSpec_config_2(t *testing.T) {
	t.Parallel()
	testscript.Run(t, scriptParams(t, filepath.Join(scriptDir, "spec_config_2.txtar")))
}

type slowServerKey struct{}

// slowServer is a fake GitHub API that accepts requests and never answers them.
type slowServer struct {
	srv     *httptest.Server
	arrived chan struct{}
	done    chan struct{}
	since   time.Time
}

// cmdSlowGitHub drives a fake GitHub API that never answers:
//
//	slowgithub start     start it and point relgap at it (TEST_REDIRECT)
//	slowgithub wait      block until relgap has sent a request
//	slowgithub within 1s fail unless less than 1s passed since wait returned
func cmdSlowGitHub(ts *testscript.TestScript, neg bool, args []string) {
	const waitLimit = 30 * time.Second
	if neg || len(args) == 0 {
		ts.Fatalf("usage: slowgithub start|wait|within <duration>")
	}
	s, _ := ts.Value(slowServerKey{}).(*slowServer)
	switch args[0] {
	case "start":
		s.arrived = make(chan struct{}, 1)
		s.done = make(chan struct{})
		s.srv = httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case s.arrived <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-s.done:
			}
		}))
		ts.Defer(func() {
			close(s.done)
			s.srv.Close()
		})
		ts.Setenv(envRedirect, s.srv.URL)
	case "wait":
		select {
		case <-s.arrived:
			s.since = time.Now()
		case <-time.After(waitLimit):
			ts.Fatalf("no request reached the fake GitHub API within %s", waitLimit)
		}
	case "within":
		limit, err := time.ParseDuration(args[1])
		ts.Check(err)
		took := time.Since(s.since)
		ts.Logf("exited %s after the interrupt", took.Round(time.Millisecond))
		if took > limit {
			ts.Fatalf("relgap took %s to exit after the interrupt, want under %s", took, limit)
		}
	default:
		ts.Fatalf("unknown slowgithub command %q", args[0])
	}
}
