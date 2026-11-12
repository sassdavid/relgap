package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/htmlmd"
	"github.com/sassdavid/relgap/internal/version"
	"github.com/sassdavid/relgap/internal/yamlschema"
	"github.com/sassdavid/relgap/schemas"
)

// ErrInvalid is wrapped by every registry entry error.
var ErrInvalid = errors.New("invalid registry entry")

const (
	schemaFile = "registry-v1.json"
	schemaURL  = "https://github.com/sassdavid/relgap/schemas/" + schemaFile
)

// LintError lists every problem found in one or more entries. It unwraps to [ErrInvalid].
type LintError struct {
	Problems []yamlschema.Problem
}

func (e *LintError) Error() string { return strings.Join(e.ProblemLines(), "\n") }

func (*LintError) Unwrap() error { return ErrInvalid }

// Header introduces the problem list in error output.
func (*LintError) Header() string { return "invalid registry entries" }

// ProblemLines returns one line per problem, with file:line:column.
func (e *LintError) ProblemLines() []string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return lines
}

// Linter validates registry entries against schemas/registry-v1.json and checks what the schema
// can't: regular expressions, scheme and extract, and the file name.
type Linter struct {
	schema *yamlschema.Schema
}

// NewLinter compiles the registry schema.
func NewLinter() (*Linter, error) {
	b, err := schemas.Read(schemaFile)
	if err != nil {
		return nil, fmt.Errorf("load registry schema: %w", err)
	}
	s, err := yamlschema.Compile(schemaURL, b)
	if err != nil {
		return nil, fmt.Errorf("load registry schema: %w", err)
	}
	return &Linter{schema: s}, nil
}

// Lint parses one entry. origin is the file path used in problems and to check the name. It
// returns the entry, or the problems found.
func (l *Linter) Lint(origin string, b []byte) (*Entry, []yamlschema.Problem, error) {
	doc, problems, err := l.schema.Parse(origin, b)
	if err != nil {
		return nil, nil, fmt.Errorf("validate %s: %w", origin, err)
	}
	if len(problems) > 0 {
		return nil, problems, nil
	}
	var e Entry
	if err = doc.Root.Decode(&e); err != nil {
		//nolint:nilerr // a document that fails to decode is a problem in the entry, not a lint failure
		return nil, []yamlschema.Problem{{Origin: origin, Message: err.Error()}}, nil
	}
	problems = checkEntry(origin, doc.Root, &e)
	if len(problems) > 0 {
		return nil, problems, nil
	}
	return &e, nil, nil
}

// templateSample replaces heading placeholders so the expression can be compiled.
func templateSample() *strings.Replacer {
	return strings.NewReplacer(
		`{version}`,
		`1\.2\.3`,
		`{major}`,
		`1`,
		`{minor}`,
		`2`,
		`{prev_minor}`,
		`1`,
		`{tag}`,
		`v1\.2\.3`,
	)
}

// Keys used in problem locations.
const (
	keyName    = "name"
	keyVersion = "version"
)

func checkEntry(origin string, root *yaml.Node, e *Entry) []yamlschema.Problem {
	var problems []yamlschema.Problem
	add := func(loc []string, keyNode bool, msg string) {
		p := yamlschema.Problem{Origin: origin, Key: strings.Join(loc, "."), Message: msg}
		if n := yamlschema.NodeAt(root, loc, keyNode); n != nil {
			p.Line, p.Column = n.Line, n.Column
		}
		problems = append(problems, p)
	}
	checkRegex := func(loc []string, expr string) {
		if msg := regexProblem(expr); msg != "" {
			add(loc, false, msg)
		}
	}

	if base := strings.TrimSuffix(path.Base(filepathToSlash(origin)), path.Ext(origin)); base != e.Name {
		add([]string{keyName}, false, fmt.Sprintf("name %q must match the file name %q", e.Name, base))
	}
	if e.Version.Exclude != "" {
		checkRegex([]string{keyVersion, "exclude"}, e.Version.Exclude)
	}
	switch {
	case e.Version.Scheme == version.SchemeRegex && e.Version.Extract == "":
		add([]string{keyVersion, "scheme"}, false, "scheme regex needs version.extract")
	case e.Version.Extract != "" && e.Version.Scheme != version.SchemeRegex:
		add([]string{keyVersion, "extract"}, true, "extract needs scheme regex")
	case e.Version.Extract != "":
		checkRegex([]string{keyVersion, "extract"}, e.Version.Extract)
	}
	for i, src := range e.Sources {
		for _, p := range checkSource(src) {
			add(append([]string{"sources", strconv.Itoa(i)}, p.key...), false, p.msg)
		}
	}
	yamlschema.SortProblems(problems)
	return problems
}

// regexProblem describes why a registry regular expression doesn't compile, or returns "".
func regexProblem(expr string) string {
	if _, err := regexp.Compile(templateSample().Replace(expr)); err != nil {
		return "bad regular expression: " + strings.TrimPrefix(err.Error(), "error parsing regexp: ")
	}
	return ""
}

// sourceProblem is a problem at a key of one source.
type sourceProblem struct {
	key []string
	msg string
}

// checkSource checks what the schema can't in one source: its heading expression, template
// placeholders, URL host and selectors.
func checkSource(s Source) []sourceProblem {
	var out []sourceProblem
	if s.Heading != "" {
		if msg := regexProblem(s.Heading); msg != "" {
			out = append(out, sourceProblem{[]string{"heading"}, msg})
		}
	}
	for key, tmpl := range map[string]string{
		"path_template": s.PathTemplate, "url_template": s.URLTemplate, "ref": s.Ref, "anchor": s.Anchor,
	} {
		if err := checkTemplate(s.Kind, tmpl); err != nil {
			out = append(out, sourceProblem{[]string{key}, err.Error()})
		}
	}
	if s.URLTemplate != "" {
		if err := CheckURL(sampleURL(s.URLTemplate)); err != nil {
			out = append(out, sourceProblem{[]string{"url_template"}, err.Error()})
		}
	}
	if s.Content != "" {
		if _, err := htmlmd.ParseSelector(s.Content); err != nil {
			out = append(out, sourceProblem{[]string{"content"}, err.Error()})
		}
	}
	for j, sel := range s.Strip {
		if _, err := htmlmd.ParseSelector(sel); err != nil {
			out = append(out, sourceProblem{[]string{"strip", strconv.Itoa(j)}, err.Error()})
		}
	}
	return out
}

func filepathToSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// LoadFS lints and loads every *.yaml entry in fsys. Problems from all files are returned together
// as a [LintError].
func (l *Linter) LoadFS(fsys fs.FS, dir string) ([]*Entry, error) {
	names, err := fs.Glob(fsys, path.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("list registry entries: %w", err)
	}
	slices.Sort(names)
	var entries []*Entry
	all := &LintError{}
	for _, name := range names {
		b, readErr := fs.ReadFile(fsys, name)
		if readErr != nil {
			return nil, fmt.Errorf("read registry entry: %w", readErr)
		}
		e, problems, lintErr := l.Lint(path.Join("registry", path.Base(name)), b)
		if lintErr != nil {
			return nil, lintErr
		}
		all.Problems = append(all.Problems, problems...)
		if e != nil {
			entries = append(entries, e)
		}
	}
	if len(all.Problems) > 0 {
		return nil, all
	}
	return entries, nil
}
