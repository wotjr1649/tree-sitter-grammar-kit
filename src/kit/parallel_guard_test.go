package kit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// parGuardFunc summarizes one function declared in a *_test.go file.
type parGuardFunc struct {
	parallel bool            // body calls <x>.Parallel()
	writes   []string        // process-global writes made directly by the body
	refs     map[string]bool // names of other test-file functions the body mentions
}

// parGuardRoot returns the identifier a write target is rooted at: x in x, x[i], x.f, *x, (x).
func parGuardRoot(e ast.Expr) *ast.Ident {
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return v
		case *ast.IndexExpr:
			e = v.X
		case *ast.SelectorExpr:
			e = v.X
		case *ast.StarExpr:
			e = v.X
		case *ast.ParenExpr:
			e = v.X
		default:
			return nil
		}
	}
}

// parGuardScan reports every process-global write that a test calling t.Parallel() can reach,
// directly or through test-file helpers, and the number of such parallel tests (roots).
// files maps a file name to its AST (parsed with mode 0, so identifiers carry Obj); only
// names ending in _test.go contribute functions, every file contributes package variables.
//
// shortcut: name-based call graph without type information; a write through a local alias
// of a global (p := &testHookOpen; *p = f), a write inside product code, and files, ports or
// timing are not seen. Upgrade to go/types if one ever appears.
func parGuardScan(fset *token.FileSet, files map[string]*ast.File) (bad []string, roots int) {
	names := slices.Sorted(maps.Keys(files))
	globals := map[string]bool{}
	funcs := map[string]*parGuardFunc{}
	for _, n := range names {
		for _, d := range files[n].Decls {
			switch v := d.(type) {
			case *ast.GenDecl:
				if v.Tok == token.VAR {
					for _, s := range v.Specs {
						for _, id := range s.(*ast.ValueSpec).Names {
							globals[id.Name] = true
						}
					}
				}
			case *ast.FuncDecl:
				if v.Body != nil && strings.HasSuffix(n, "_test.go") && funcs[v.Name.Name] == nil {
					funcs[v.Name.Name] = &parGuardFunc{refs: map[string]bool{}}
				}
			}
		}
	}
	delete(globals, "_")

	// Setenv and Chdir are matched on any receiver, so t.Setenv and t.Chdir count too.
	anyReceiver := []string{"Setenv", "Unsetenv", "Clearenv", "Chdir"}
	byPackage := map[string][]string{
		"log":     {"SetOutput", "SetFlags", "SetPrefix", "SetDefault"},
		"flag":    {"Set", "Parse"},
		"runtime": {"GOMAXPROCS", "GC", "ReadMemStats"},
		"debug":   {"SetGCPercent", "SetMemoryLimit", "SetMaxThreads", "SetMaxStack"},
	}
	for _, fname := range names {
		file := files[fname]
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || !strings.HasSuffix(fname, "_test.go") {
				continue
			}
			fn := funcs[fd.Name.Name]
			// A package-level variable: unresolved in this file (declared in another one) or
			// resolved to this file's package scope, not shadowed by a local declaration.
			global := func(e ast.Expr) string {
				id := parGuardRoot(e)
				if id != nil && globals[id.Name] && (id.Obj == nil || id.Obj == file.Scope.Lookup(id.Name)) {
					return id.Name
				}
				return ""
			}
			at := func(n ast.Node, what string) string { return what + " at " + fset.Position(n.Pos()).String() }
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.AssignStmt:
					if v.Tok != token.DEFINE {
						for _, l := range v.Lhs {
							if name := global(l); name != "" {
								fn.writes = append(fn.writes, at(v, "assigns "+name))
							}
						}
					}
				case *ast.RangeStmt:
					if v.Tok == token.ASSIGN {
						for _, l := range []ast.Expr{v.Key, v.Value} {
							if name := global(l); name != "" {
								fn.writes = append(fn.writes, at(v, "assigns "+name))
							}
						}
					}
				case *ast.IncDecStmt:
					if name := global(v.X); name != "" {
						fn.writes = append(fn.writes, at(v, "modifies "+name))
					}
				case *ast.CallExpr:
					switch f := v.Fun.(type) {
					case *ast.Ident:
						if (f.Name == "delete" || f.Name == "clear" || f.Name == "copy") && len(v.Args) > 0 {
							if name := global(v.Args[0]); name != "" {
								fn.writes = append(fn.writes, at(v, f.Name+" on "+name))
							}
						}
					case *ast.SelectorExpr:
						if f.Sel.Name == "Parallel" {
							fn.parallel = true
						}
						x, _ := f.X.(*ast.Ident)
						call := f.Sel.Name
						if x != nil {
							call = x.Name + "." + call
						}
						if slices.Contains(anyReceiver, f.Sel.Name) || x != nil && slices.Contains(byPackage[x.Name], f.Sel.Name) {
							fn.writes = append(fn.writes, at(v, "calls "+call))
						}
					}
				case *ast.Ident:
					// A local variable or parameter that shares a helper's name is not a call to it.
					if funcs[v.Name] != nil && (v.Obj == nil || v.Obj.Kind == ast.Fun) {
						fn.refs[v.Name] = true
					}
				}
				return true
			})
		}
	}

	for _, name := range slices.Sorted(maps.Keys(funcs)) {
		if !strings.HasPrefix(name, "Test") || !funcs[name].parallel {
			continue
		}
		roots++
		parent, stack := map[string]string{}, []string{name}
		seen := map[string]bool{name: true}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			path := cur
			for p := cur; p != name; {
				p = parent[p]
				path = p + " -> " + path
			}
			for _, w := range funcs[cur].writes {
				bad = append(bad, path+": "+w)
			}
			for _, r := range slices.Sorted(maps.Keys(funcs[cur].refs)) {
				if !seen[r] {
					seen[r] = true
					parent[r] = cur
					stack = append(stack, r)
				}
			}
		}
	}
	slices.Sort(bad)
	return bad, roots
}

// TestParallelTestsDoNotReachGlobalWrites keeps a silent data race out of the parallel phase.
// Serial top-level tests never overlap parallel ones, so a test may write a package-level
// variable (testHook*, branchSelectionRoutes, largeFileProfiles, ...), the process environment
// or other process-wide state only while no test that calls t.Parallel() can reach that write,
// directly or through test-file helpers. Fix a failure by passing the value per call or by
// dropping t.Parallel().
func TestParallelTestsDoNotReachGlobalWrites(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob *.go: %v (%d files)", err, len(paths))
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[p] = f
	}
	bad, roots := parGuardScan(fset, files)
	if roots == 0 {
		t.Fatal("found no test that calls t.Parallel(); the guard would pass vacuously")
	}
	for _, b := range bad {
		t.Error(b)
	}
}

// TestParGuardScanDetects shows the guard can fail: it runs parGuardScan on in-memory sources.
func TestParGuardScanDetects(t *testing.T) {
	const globals = "package kit\nvar testHookOpen func()\nvar roles int\n"
	for _, c := range []struct{ name, src, want string }{
		{"helper writes hook", "func TestP(t *testing.T) { t.Parallel(); helper() }\nfunc helper() { testHookOpen = nil }", "TestP -> helper: assigns testHookOpen"},
		{"local shadows global", "func TestP(t *testing.T) { t.Parallel(); roles := 1; roles = 2; _ = roles }", ""},
		{"serial test writes", "func TestP(t *testing.T) { t.Parallel() }\nfunc TestS(t *testing.T) { testHookOpen = nil }", ""},
		{"t.Setenv", "func TestP(t *testing.T) { t.Parallel(); t.Setenv(\"A\", \"1\") }", "calls t.Setenv"},
		{"range assigns global", "func TestP(t *testing.T) { t.Parallel(); for roles = range 3 {} }", "assigns roles"},
		{"local named like helper", "func TestP(t *testing.T) { t.Parallel(); helper := 1; _ = helper }\nfunc helper() { testHookOpen = nil }", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			fset := token.NewFileSet()
			files := map[string]*ast.File{}
			for name, src := range map[string]string{"a.go": globals, "x_test.go": "package kit\n" + c.src} {
				f, err := parser.ParseFile(fset, name, src, 0)
				if err != nil {
					t.Fatal(err)
				}
				files[name] = f
			}
			bad, roots := parGuardScan(fset, files)
			if roots != 1 {
				t.Fatalf("roots = %d, want 1", roots)
			}
			if c.want == "" && len(bad) != 0 || c.want != "" && (len(bad) != 1 || !strings.Contains(bad[0], c.want)) {
				t.Fatalf("bad = %q, want one containing %q", bad, c.want)
			}
		})
	}
}
