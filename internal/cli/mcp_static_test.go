package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// writeFuncs are the os functions that create, change or delete files.
func writeFuncs() []string {
	return []string{
		"Create", "CreateTemp", "WriteFile", "OpenFile", "Mkdir", "MkdirAll", "MkdirTemp", "Remove",
		"RemoveAll", "Rename", "Symlink", "Link", "Chmod", "Chown", "Lchown", "Chtimes", "Truncate",
		"StartProcess", "CopyFS",
	}
}

// TestMCP_staticNoExecNoWrite reads the code behind `relgap mcp` and fails when a tool could
// start a process or write a file. The packages that only serve MCP, and every method of the
// session the tools run through, may not import os/exec or syscall, call an os function that
// writes, or build a command runner. `relgap mcp` must give its session and its registry the
// mcpserver.NoExec runner and no token runner. The HTTP cache is the one writer, inside
// internal/httpx.
func TestMCP_staticNoExecNoWrite(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var mcpOnly []*ast.File
	for _, pattern := range []string{"../mcpserver/*.go", "../mcpkit/*.go", "mcp.go"} {
		mcpOnly = append(mcpOnly, parseGo(t, fset, pattern)...)
	}
	for _, f := range mcpOnly {
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "os/exec" || p == "syscall" {
				t.Errorf("%s imports %s", fset.Position(imp.Pos()), p)
			}
		}
		checkNoExecNoWrite(t, fset, f)
	}
	sessionMethods := 0
	for _, f := range parseGo(t, fset, "*.go") {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && isSessionRecv(fn.Recv) {
				sessionMethods++
				checkNoExecNoWrite(t, fset, fn.Body)
			}
		}
	}
	if sessionMethods < 5 {
		t.Errorf("found %d session methods; did the session move?", sessionMethods)
	}
	checkMCPWiring(t, fset, parseGo(t, fset, "mcp.go"))
}

// parseGo parses the non-test Go files that match pattern.
func parseGo(t *testing.T, fset *token.FileSet, pattern string) []*ast.File {
	t.Helper()
	names, err := filepath.Glob(filepath.FromSlash(pattern))
	if err != nil || len(names) == 0 {
		t.Fatalf("no files match %s: %v", pattern, err)
	}
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		files = append(files, f)
	}
	return files
}

func isSessionRecv(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "session"
}

// checkNoExecNoWrite fails for a call to an os write function or a runner constructor below n.
func checkNoExecNoWrite(t *testing.T, fset *token.FileSet, n ast.Node) {
	t.Helper()
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			pkg, ok := x.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkg.Name == "os" && slices.Contains(writeFuncs(), x.Sel.Name) {
				t.Errorf("%s calls os.%s", fset.Position(x.Pos()), x.Sel.Name)
			}
			if pkg.Name == "execx" && x.Sel.Name == "NewRunner" {
				t.Errorf("%s builds an execx runner", fset.Position(x.Pos()))
			}
		case *ast.Ident:
			if x.Name == "newRunner" {
				t.Errorf("%s uses newRunner", fset.Position(x.Pos()))
			}
		}
		return true
	})
}

// checkMCPWiring fails unless mcp.go builds one session, with runner mcpserver.NoExec{} and no
// tokenRunner, and calls newResolver once, with mcpserver.NoExec{}.
func checkMCPWiring(t *testing.T, fset *token.FileSet, files []*ast.File) {
	t.Helper()
	sessions, resolvers := 0, 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok && isIdent(lit.Type, "session") {
				sessions++
				checkSessionLiteral(t, fset, lit)
			}
			if call, ok := n.(*ast.CallExpr); ok && isIdent(call.Fun, "newResolver") {
				resolvers++
				if len(call.Args) != 4 || !isNoExec(call.Args[3]) {
					t.Errorf("%s: newResolver doesn't get mcpserver.NoExec{}", fset.Position(call.Pos()))
				}
			}
			return true
		})
	}
	if sessions != 1 || resolvers != 1 {
		t.Errorf("mcp.go builds %d sessions and %d resolvers, want 1 each", sessions, resolvers)
	}
}

func checkSessionLiteral(t *testing.T, fset *token.FileSet, lit *ast.CompositeLit) {
	t.Helper()
	runner := false
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		switch {
		case !ok:
		case isIdent(kv.Key, "runner"):
			runner = isNoExec(kv.Value)
		case isIdent(kv.Key, "tokenRunner"):
			t.Errorf("%s gives the MCP session a token runner", fset.Position(kv.Pos()))
		}
	}
	if !runner {
		t.Errorf("%s: the MCP session's runner isn't mcpserver.NoExec{}", fset.Position(lit.Pos()))
	}
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isNoExec(e ast.Expr) bool {
	lit, ok := e.(*ast.CompositeLit)
	if !ok {
		return false
	}
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "mcpserver" && sel.Sel.Name == "NoExec" && len(lit.Elts) == 0
}
