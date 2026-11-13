package kit

import (
	"encoding/json"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The separation proof of G1: the derivation of the expectation (and of the kind map and the
// observation index) is a function of the bytes it is handed. It does not call any function that reads
// native input: the executed asset, the native raw response, a native observation path or the process
// environment.
//
// The proof is a call closure over the non-test files of the package, built with go/parser, go/ast and
// go/types only. A node is a package-level declaration (by name) or a method (by "Type.method"); a node
// refers to every package-level object and every method its text resolves to. Where go/types cannot
// resolve a name (a package outside the standard library and the one pinned module package is replaced
// by an empty one), the node falls back to the name: an unresolved identifier refers to the package-level
// declaration of that name, an unresolved selector refers to every method of that name. A call through an
// interface refers to every method of that name. A type refers to all its methods, because a library
// calls them through an interface (MarshalJSON, String). So the closure over-approximates; it cannot miss
// a call, a function value, a method value or a package variable. It does not follow reflection (the
// package that offers it is not allowed) or a call that only an interface of a library makes to a method
// of a type the derivation never names.
//
// A closure fails when it reaches a node of a comparison file, when a reached node uses a package that
// is not on the allow list, or when a reached node carries a //go:embed or //go:linkname directive.

// g1SepClass classifies every non-test G1 file. A new file that is not listed fails the test, so a
// helper cannot hide in a file nobody classified.
var g1SepClass = map[string]string{
	"csharp_g1_spec.go":            "derivation",
	"csharp_g1_expected.go":        "derivation",
	"csharp_g1_expected_rules.go":  "derivation",
	"csharp_g1_expected_checks.go": "derivation",
	"csharp_g1_expected_data.go":   "derivation",
	"csharp_g1_kindmap.go":         "derivation",
	"csharp_g1_roslyn.go":          "derivation",
	"csharp_g1_ledger.go":          "derivation",
	// r2 corrections: authored literals plus code that applies them to the sealed r1 bytes. No function of the
	// file reads an executed asset or a native record (the closure proves it from the roots below); the
	// literals themselves were written after the native comparison, which their after_native flag records.
	"csharp_g1_correct.go":      "derivation",
	"csharp_g1_audit_report.go": "derivation",
	// the audit of the differential-oracle alternatives: a hand derivation from the specification compared with the r2 witnesses.
	// The closure proves it cannot reach native input; that it never reads the expected types is a syntactic check of its own
	// (TestCSharpG1Audit76NeverNamesTheExpectedTypes), which the closure cannot express.
	"csharp_g1_audit76.go": "derivation",
	// what the position audit is given and checked with: the rules of its instances, the grounds and their pins. They hold data
	// about the rows of the audits only (the expected asset, the correction records and the kind map are in other files).
	"csharp_g1_audit76_rules.go": "derivation",
	"csharp_g1_ground.go":        "derivation",
	"csharp_g1_ground_pins.go":   "derivation",
	// the view of the r2 asset for the two audits: the one place where the audits' input is read from the asset (see the test
	// TestCSharpG1AuditsReachOnlyTheirOwnInputs). It is on the expectation side, so the closure checks it too.
	"csharp_g1_audit_view.go": "derivation",
	// the type audit of the same alternatives: a hand derivation from grammar.js compared with the r2 expected types. The closure
	// proves it cannot reach native input; that it never reads the kind map is a syntactic check of its own
	// (TestCSharpG1TypeAudit76NeverReadsTheKindMap).
	"csharp_g1_typeaudit76.go": "derivation",
	// the G1-3b classification of csharp-B03 and csharp-B04: its input is a narrow value (TestCSharpG1ClassifyInputIsNarrow), it names no
	// tree-sitter node type (TestCSharpG1ClassifyNeverNamesTheExpectedTypes) and its call closure reaches no other G1 file
	// (csharp_g1_audit_independence_test.go). The view is the one place that reads the ledger, the observation and the kind map for it.
	"csharp_g1_classify.go":       "derivation",
	"csharp_g1_classify_rules.go": "derivation",
	"csharp_g1_classify_view.go":  "derivation",
	"csharp_g1_executed.go":       "comparison",
	"csharp_g1_native.go":         "comparison",
	// the approval of the bundle: it reads the tracked assets as bytes and names them by path; it derives and compares nothing.
	"csharp_g1_approval.go": "neutral",
}

// g1SepNeutral are the files outside the G1 prefix that the closure of the derivation reaches (the closure
// over-approximates: a method name or a type pulls in every file that has one). They are neither derivation nor
// comparison code of G1: shared helpers of the package. A reached file that is not listed here fails the proof, and a
// package variable of one of them that the comparison side uses and the derivation reads fails it too. The package
// allow list applies to every node of such a file the closure reaches.
var g1SepNeutral = []string{
	"capture.go",
	"grammar.go",
	"query_large_template.go",
	"replay.go",
	"csharp_pp.go",
	"edit.go",
	"encoding.go",
	"evidence.go",
	"facts.go",
	"incremental.go",
	"kit.go",
	"oracle.go",
	"preprocess.go",
	"preprocess_cst.go",
	"preprocess_route.go",
	"profile.go",
	"qualify.go",
	"qualify_accepted.go",
	"qualify_preprocess_cst.go",
	"qualify_symbols.go",
	"query_svc_inline_derive.go",
	"query_witness.go",
	"replay_native.go",
	"replay_numeric.go",
	"reproduce.go",
	"reuse_accepted_registry.go",
	"schema.go",
	"strict.go",
	"svc.go",
	"svc_context.go",
	"tree.go",
	"verify.go",
}

// g1SepClasses is the file table with the neutral files added.
func g1SepClasses() map[string]string {
	c := map[string]string{}
	for f, k := range g1SepClass {
		c[f] = k
	}
	for _, f := range g1SepNeutral {
		c[f] = "neutral"
	}
	return c
}

// g1SepRoots are the entry points of the derivation side: the expectation, the kind map, the
// observation index and the metamorphic variants.
var g1SepRoots = []string{
	"DeriveCSharpG1Expected", "ParseCSharpG1Expected", "DecodeCSharpG1Expected", "CheckCSharpG1Expected", "MarshalCSharpG1Expected",
	"ParseCSharpG1KindMap", "MarshalCSharpG1KindMap", "DeriveCSharpG1KindMapCounts",
	"ParseCSharpG1RoslynObserved", "CSharpG1RoslynSources", "MarshalCSharpG1RoslynInput",
	"G1MetaVariantSources", "CSharpG1AuditSamples", "CSharpG1StructureDigest",
	"DeriveCSharpG1ExpectedR2", "ParseCSharpG1ExpectedR2", "DeriveCSharpG1KindMapR2", "ParseCSharpG1KindMapR2",
	"CSharpG1RoslynVocabulary", "CheckG1Grounds", "G1ExpectedCorrections", "G1KindMapCorrections",
	"G1AuditStrata", "G1AuditUpper95",
	"DeriveCSharpG1Audit76", "ParseCSharpG1Audit76", "DecodeCSharpG1Audit76", "MarshalCSharpG1Audit76", "CheckG1Audit76Grounds", "CheckG1GroundList",
	"G1Audit76View", "G1TypeAudit76View", "G1GroundPin", "G1GroundPinSource", "G1Audit76GroundPins", "G1TypeAudit76GroundPins",
	"DeriveCSharpG1TypeAudit76", "ParseCSharpG1TypeAudit76", "DecodeCSharpG1TypeAudit76", "MarshalCSharpG1TypeAudit76", "CheckG1TypeAudit76Grounds", "G1AuditStrataWithTypes",
	"ParseCSharpG1RoslynObservedR3", "DeriveCSharpG1Ledger", "ParseCSharpG1Ledger", "MarshalCSharpG1Ledger",
	"ClassifyCSharpG1", "G1ClassifyView",
}

// g1SepExcluded are exported Derive*, Parse*, Check* and Marshal* functions of derivation files that no root reaches
// and that need none: a function that is not a derivation entry point of the expectation side. Add one only with the
// reason.
var g1SepExcluded = []string{}

// g1SepAllowed are the packages a derivation node may use (pure computation on bytes and strings).
//
// An entry is a whole package or one selector of a package ("time.Second"). The derivation reaches the
// shared parsers of the qualification inventory and the witness tables, so the list covers what they use;
// the packages that can reach the file system, the process or the network are not on it. A package of the
// module is listed by selector: those are declarations of data (types, constants), not readers.
var g1SepAllowed = []string{
	"bytes", "cmp", "crypto/sha256", "encoding/base64", "encoding/hex", "encoding/json", "encoding/json/jsontext", "encoding/json/v2",
	"errors", "fmt", "regexp", "slices", "sort", "strconv", "strings", "unicode", "unicode/utf16", "unicode/utf8",
	"io.EOF", "io.ErrUnexpectedEOF", "sync.OnceValue", "time.Duration", "time.Second",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire.EditPoints",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire.Point",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire.TreeNode",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy.Predicate",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire.CompileError",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore.Pin",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore.FilePin",
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore.QualificationPin",
}

// g1SepNodeAllowed lets a named node use one more package. reflect compares the shape of a Go type with the
// shape of the document it decodes (reflect.TypeFor of a static type); nothing else on the derivation
// side may use it.
var g1SepNodeAllowed = map[string][]string{
	"parseInventory":     {"reflect"},
	"witnessShape":       {"reflect"},
	"preprocessCSTShape": {"reflect"},
}

// g1SepEmbedAllowed are the files the derivation side may embed: the static encoding table of the
// source-text edits.
var g1SepEmbedAllowed = []string{"data/index-euc-kr.txt"}

// the spec deriver reads the source text and nothing else
var g1SepSpecAllowed = []string{"regexp", "strings", "unicode/utf8"}

// g1SepBannedSel are functions of an allowed package that read the process (fmt.Scan reads os.Stdin).
var g1SepBannedSel = []string{"fmt.Scan", "fmt.Scanf", "fmt.Scanln"}

type g1SepNode struct {
	file    string
	kind    string // func, method, var, const, type, init
	hasInit bool   // a var with an initializer
	refs    map[string]bool
	uses    map[string]bool // "import/path.Selector"
	directs []string
}

type g1SepGraph struct {
	nodes    map[string][]*g1SepNode // by name; methods by "Type.method" and by ".method"
	methods  map[string][]string     // type -> its method keys
	linkname map[string]string       // local name of a //go:linkname -> file
	global   []string                // problems that do not depend on reachability
}

func g1SepImportName(path string) string {
	parts := strings.Split(path, "/")
	n := parts[len(parts)-1]
	if regexp.MustCompile(`^v[0-9]+$`).MatchString(n) && len(parts) > 1 {
		n = parts[len(parts)-2]
	}
	return n
}

// g1SepImporter type-checks the standard library and the pinned module package from source and gives every
// other package an empty one, so that the package under test checks without building its dependencies.
type g1SepImporter struct {
	real  types.ImporterFrom
	cache map[string]*types.Package
}

func (m *g1SepImporter) Import(path string) (*types.Package, error) {
	if p, ok := m.cache[path]; ok {
		return p, nil
	}
	var p *types.Package
	if first, _, _ := strings.Cut(path, "/"); !strings.Contains(first, ".") || strings.HasSuffix(path, "/src/kit/queryregistrycore") {
		p, _ = m.real.ImportFrom(path, ".", 0)
	}
	if p == nil {
		p = types.NewPackage(path, g1SepImportName(path))
		p.MarkComplete()
	}
	m.cache[path] = p
	return p, nil
}

// g1SepShared is the file set and importer of every proof in the run, so the standard library is read once.
var g1SepShared = sync.OnceValues(func() (*token.FileSet, *g1SepImporter) {
	fset := token.NewFileSet()
	return fset, &g1SepImporter{real: importer.ForCompiler(fset, "source", nil).(types.ImporterFrom), cache: map[string]*types.Package{}}
})

// g1SepRecv names the type of a method's receiver.
func g1SepRecv(e ast.Expr) string {
	for {
		switch v := e.(type) {
		case *ast.StarExpr:
			e = v.X
		case *ast.ParenExpr:
			e = v.X
		case *ast.IndexExpr:
			e = v.X
		case *ast.IndexListExpr:
			e = v.X
		case *ast.Ident:
			return v.Name
		default:
			return "?"
		}
	}
}

// g1SepParse builds the graph of the non-test files given by name and source.
func g1SepParse(files map[string]string) (*g1SepGraph, error) {
	g := &g1SepGraph{nodes: map[string][]*g1SepNode{}, methods: map[string][]string{}, linkname: map[string]string{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	fset, imp := g1SepShared()
	var parsed []*ast.File
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, files[name], parser.ParseComments)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, f)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{
		Importer: imp,
		Error:    func(error) {}, // the checked package may be incomplete; unresolved names fall back to names
	}
	pkg, _ := conf.Check("kit", fset, parsed, info)
	// resolve maps a used object to the key of its node, or "" when the object is not a node
	resolve := func(obj types.Object) string {
		switch o := obj.(type) {
		case *types.Func:
			if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
				rt := sig.Recv().Type()
				if p, ok := rt.(*types.Pointer); ok {
					rt = p.Elem()
				}
				if named, ok := rt.(*types.Named); ok && !types.IsInterface(rt) {
					if named.Obj().Pkg() != pkg {
						return ""
					}
					return named.Origin().Obj().Name() + "." + o.Name()
				}
				return "." + o.Name() // a call through an interface or a type parameter
			}
		}
		if obj.Pkg() == pkg && obj.Parent() == pkg.Scope() {
			return obj.Name()
		}
		return ""
	}
	for i, f := range parsed {
		name := names[i]
		for _, im := range f.Imports {
			// a blank import of embed only enables //go:embed, which is checked on the declaration
			if path := strings.Trim(im.Path.Value, `"`); im.Name != nil && (im.Name.Name == "." || im.Name.Name == "_" && path != "embed") {
				g.global = append(g.global, name+": "+im.Name.Name+" import of "+strings.Trim(im.Path.Value, `"`))
			}
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if rest, ok := strings.CutPrefix(c.Text, "//go:linkname "); ok {
					g.linkname[strings.Fields(rest)[0]] = name
				}
			}
		}
		add := func(keys []string, kind string, n ast.Node, docs ...*ast.CommentGroup) *g1SepNode {
			node := &g1SepNode{file: name, kind: kind, refs: map[string]bool{}, uses: map[string]bool{}}
			var walk func(x ast.Node) bool
			walk = func(x ast.Node) bool {
				switch v := x.(type) {
				case *ast.SelectorExpr:
					if id, ok := v.X.(*ast.Ident); ok {
						if pn, ok := info.Uses[id].(*types.PkgName); ok {
							node.uses[pn.Imported().Path()+"."+v.Sel.Name] = true
							return false
						}
					}
					if obj := info.Uses[v.Sel]; obj == nil {
						node.refs["."+v.Sel.Name] = true
					} else if k := resolve(obj); k != "" {
						node.refs[k] = true
					}
					ast.Inspect(v.X, walk)
					return false
				case *ast.Ident:
					if obj := info.Uses[v]; obj != nil {
						if k := resolve(obj); k != "" {
							node.refs[k] = true
						}
					} else if info.Defs[v] == nil {
						node.refs[v.Name] = true
					}
				}
				return true
			}
			ast.Inspect(n, walk)
			for _, d := range docs {
				if d == nil {
					continue
				}
				for _, c := range d.List {
					if strings.HasPrefix(c.Text, "//go:embed") || strings.HasPrefix(c.Text, "//go:linkname") {
						node.directs = append(node.directs, c.Text)
					}
				}
			}
			for _, k := range keys {
				g.nodes[k] = append(g.nodes[k], node)
			}
			return node
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				switch {
				case d.Recv != nil:
					recv := g1SepRecv(d.Recv.List[0].Type)
					key := recv + "." + d.Name.Name
					g.methods[recv] = append(g.methods[recv], key)
					add([]string{key, "." + d.Name.Name}, "method", d, d.Doc)
				case d.Name.Name == "init":
					add([]string{"init"}, "init", d, d.Doc)
				default:
					add([]string{d.Name.Name}, "func", d, d.Doc)
				}
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.ValueSpec:
						kind := "var"
						if d.Tok == token.CONST {
							kind = "const"
						}
						for _, n := range s.Names {
							add([]string{n.Name}, kind, s, d.Doc, s.Doc).hasInit = len(s.Values) > 0
						}
					case *ast.TypeSpec:
						add([]string{s.Name.Name}, "type", s, d.Doc, s.Doc)
					}
				}
			}
		}
	}
	return g, nil
}

// g1SepCheck follows the closure of the roots and returns the problems. A node of a file whose class is
// forbidClass is a problem; allowedFiles, when set, restricts the files the closure may enter; a used
// package outside allowed is a problem.
func g1SepCheck(g *g1SepGraph, class map[string]string, roots []string, forbidClass string, allowedFiles, allowed []string) []string {
	var problems []string
	parent := map[string]string{}
	var queue []string
	for _, r := range roots {
		if _, ok := parent[r]; !ok {
			parent[r] = ""
			queue = append(queue, r)
		}
	}
	chain := func(k string) string {
		var parts []string
		for ; k != ""; k = parent[k] {
			parts = append(parts, k)
		}
		slices.Reverse(parts)
		return strings.Join(parts, " -> ")
	}
	enqueue := func(from, k string) {
		if _, seen := parent[k]; !seen && len(g.nodes[k]) > 0 {
			parent[k] = from
			queue = append(queue, k)
		}
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, n := range g.nodes[k] {
			where := "[" + chain(k) + " in " + n.file + "]"
			if forbidClass != "" && class[n.file] == forbidClass {
				problems = append(problems, where+" reaches a "+forbidClass+" file")
			}
			if allowedFiles != nil && !slices.Contains(allowedFiles, n.file) {
				problems = append(problems, where+" leaves the allowed files")
			}
			for _, d := range n.directs {
				if file, ok := strings.CutPrefix(d, "//go:embed "); !ok || !slices.Contains(g1SepEmbedAllowed, file) {
					problems = append(problems, where+" carries "+d)
				}
			}
			if f, ok := g.linkname[k]; ok && f == n.file {
				problems = append(problems, where+" has a //go:linkname")
			}
			for u := range n.uses {
				dot := strings.LastIndex(u, ".")
				ok := slices.Contains(allowed, u[:dot]) || slices.Contains(allowed, u) || slices.Contains(g1SepNodeAllowed[k], u[:dot])
				if !ok || slices.Contains(g1SepBannedSel, g1SepImportName(u[:dot])+u[dot:]) {
					problems = append(problems, where+" uses "+u)
				}
			}
			for r := range n.refs {
				enqueue(k, r)
			}
			if n.kind == "type" { // a library calls the methods of a type through an interface
				for _, m := range g.methods[k] {
					enqueue(k, m)
				}
			}
		}
	}
	sort.Strings(problems)
	return slices.Compact(problems)
}

// g1SepReach is the set of nodes (keys of g.nodes) the closure of the roots contains, by the same edges as g1SepCheck.
func g1SepReach(g *g1SepGraph, roots []string) map[string]bool {
	seen := map[string]bool{}
	queue := slices.Clone(roots)
	for _, r := range roots {
		seen[r] = true
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		var next []string
		for _, n := range g.nodes[k] {
			for r := range n.refs {
				next = append(next, r)
			}
			if n.kind == "type" {
				next = append(next, g.methods[k]...)
			}
		}
		for _, r := range next {
			if !seen[r] && len(g.nodes[r]) > 0 {
				seen[r] = true
				queue = append(queue, r)
			}
		}
	}
	return seen
}

// g1SepDeriveName matches the exported entry points of a derivation file that a root must reach.
var g1SepDeriveName = regexp.MustCompile(`^(Derive|Parse|Check|Marshal)[A-Z0-9]`)

// g1SepProve is the whole proof on a set of files: the file table, the closure of the derivation roots, the
// closure of the spec deriver, the initialisation of the package and the one-way use of package state.
func g1SepProve(files map[string]string, class map[string]string, roots []string) ([]string, error) {
	g, err := g1SepParse(files)
	if err != nil {
		return nil, err
	}
	problems := slices.Clone(g.global)
	for name := range files {
		if strings.HasPrefix(name, "csharp_g1_") && class[name] == "" {
			problems = append(problems, name+": a G1 file without a class")
		}
	}
	for _, r := range roots {
		if len(g.nodes[r]) == 0 {
			problems = append(problems, "root "+r+" does not exist")
		}
	}
	// initialisation runs before any derivation: every init function of the package and every initialised
	// package variable of a G1 file is a root; a comparison file has no init function
	all := slices.Clone(roots)
	var comparisonInit, specRoots []string
	for key, nodes := range g.nodes {
		if strings.HasPrefix(key, ".") { // a method listed under its bare name too
			continue
		}
		for _, n := range nodes {
			switch {
			case class[n.file] == "comparison" && n.kind == "init":
				problems = append(problems, n.file+": an init function in a comparison file")
			case class[n.file] == "comparison" && n.hasInit:
				comparisonInit = append(comparisonInit, key)
			case n.kind == "init" || n.hasInit && class[n.file] != "":
				all = append(all, key)
			}
			if n.file == "csharp_g1_spec.go" {
				specRoots = append(specRoots, key)
			}
		}
	}
	problems = append(problems, g1SepCheck(g, class, all, "comparison", nil, g1SepAllowed)...)
	// the spec deriver reads the source text only: it stays in its file and uses three packages
	for _, p := range g1SepCheck(g, class, specRoots, "", []string{"csharp_g1_spec.go"}, g1SepSpecAllowed) {
		problems = append(problems, "spec: "+p)
	}
	reach := g1SepReach(g, all)
	reachedFiles := map[string]bool{}
	for key := range reach {
		for _, n := range g.nodes[key] {
			reachedFiles[n.file] = true
		}
	}
	// a file the derivation reaches is a derivation file, or is registered as neutral (a helper that is no part of
	// either side); a file nobody classified is a channel nobody checked
	for file := range reachedFiles {
		if c := class[file]; c != "derivation" && c != "comparison" && c != "neutral" {
			problems = append(problems, file+": reached by the derivation but neither derivation, comparison nor neutral")
		}
	}
	// every exported Derive, Parse, Check and Marshal function of a derivation file is a root, is reached by one or is
	// listed as excluded: a derivation entry point outside the roots would be proved by nothing
	for key, nodes := range g.nodes {
		for _, n := range nodes {
			if n.kind == "func" && class[n.file] == "derivation" && g1SepDeriveName.MatchString(key) && !reach[key] && !slices.Contains(g1SepExcluded, key) {
				problems = append(problems, n.file+": "+key+" is not a root, not reached by one and not excluded")
			}
		}
	}
	// a package variable of a neutral file that the comparison side uses must not be reached by the derivation
	for key, nodes := range g.nodes {
		for _, n := range nodes {
			if n.kind != "var" || class[n.file] != "neutral" || !reach[key] {
				continue
			}
			for ckey, cn := range g.nodes {
				for _, c := range cn {
					if class[c.file] == "comparison" && c.refs[key] {
						problems = append(problems, c.file+": "+ckey+" uses the neutral variable "+key+" that the derivation reads")
					}
				}
			}
		}
	}
	// the initialisation of the comparison files does not read the process either
	for _, p := range g1SepCheck(g, class, comparisonInit, "", nil, g1SepAllowed) {
		problems = append(problems, "comparison init: "+p)
	}
	// a comparison file shares no package variable with a derivation file: a package variable is the one
	// way state can pass between the two sides without a call
	vars := map[string]bool{}
	for key, nodes := range g.nodes {
		for _, n := range nodes {
			if n.kind == "var" && class[n.file] == "derivation" {
				vars[key] = true
			}
		}
	}
	for key, nodes := range g.nodes {
		for _, n := range nodes {
			if class[n.file] != "comparison" {
				continue
			}
			for r := range n.refs {
				if vars[r] {
					problems = append(problems, n.file+": "+key+" uses the derivation variable "+r)
				}
			}
		}
	}
	sort.Strings(problems)
	return slices.Compact(problems), nil
}

func g1SepReadPackage(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil || len(paths) == 0 {
		t.Fatalf("package files: %v", err)
	}
	files := map[string]string{}
	for _, p := range paths {
		// the files of this platform: the proof covers what is compiled here, and CI runs it on each platform
		if ok, _ := build.Default.MatchFile(".", p); strings.HasSuffix(p, "_test.go") || !ok {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[p] = string(data)
	}
	return files
}

func TestCSharpG1SeparationClosure(t *testing.T) {
	files := g1SepReadPackage(t)
	for name := range g1SepClasses() {
		if _, ok := files[name]; !ok {
			t.Errorf("classified file %s does not exist", name)
		}
	}
	problems, err := g1SepProve(files, g1SepClasses(), g1SepRoots)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// The proof is not empty: it fails on the shapes of the bypasses the review found and passes on the same
// code without them. Each case is a small package in memory.
func TestCSharpG1SeparationClosureCatchesBypasses(t *testing.T) {
	base := func() map[string]string {
		return map[string]string{
			"csharp_g1_expected.go": "package kit\n\nimport \"strings\"\n\nfunc Derive() string { return helper() }\n\nfunc helper() string { return strings.ToUpper(\"a\") }\n",
			"csharp_g1_executed.go": "package kit\n\nimport \"os\"\n\nvar table = map[string]string{\"a\": \"b\"}\n\nfunc Compare() []byte { b, _ := os.ReadFile(\"x\"); return b }\n\nfunc neutral() string { return table[\"a\"] }\n",
		}
	}
	class := map[string]string{"csharp_g1_expected.go": "derivation", "csharp_g1_executed.go": "comparison", "csharp_g1_correct.go": "derivation", "memo.go": "neutral"}
	roots := []string{"Derive"}
	run := func(mutate func(f map[string]string)) []string {
		f := base()
		mutate(f)
		p, err := g1SepProve(f, class, roots)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := run(func(map[string]string) {}); len(p) != 0 {
		t.Fatalf("the clean package fails: %v", p)
	}
	cases := []struct {
		name, want string
		mutate     func(f map[string]string)
	}{
		// a package that opens any file but is not on a ban list (M22)
		{"unlisted file package", "uses golang.org/x/sys/windows.GetCurrentProcessId", func(f map[string]string) {
			f["csharp_g1_expected.go"] += "\nvar pid = windows.GetCurrentProcessId()\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "import \"strings\"", "import (\n\t\"strings\"\n\t\"golang.org/x/sys/windows\"\n)", 1)
		}},
		// a helper in a new file nobody classified that reads a file named by the environment (M23)
		{"new file with os", "uses os.ReadFile", func(f map[string]string) {
			f["csharp_g1_zzpeek.go"] = "package kit\n\nimport \"os\"\n\nfunc peek() []byte { b, _ := os.ReadFile(os.Getenv(\"PEEK\")); return b }\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "_ = peek()\n\treturn helper()", 1)
		}},
		{"new file without class", "csharp_g1_zzpeek.go: a G1 file without a class", func(f map[string]string) {
			f["csharp_g1_zzpeek.go"] = "package kit\n\nfunc peek() {}\n"
		}},
		// the derivation reads a table of the comparison under a name no pattern matches (M13)
		{"comparison table", "reaches a comparison file", func(f map[string]string) {
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "_ = table\n\treturn helper()", 1)
		}},
		// a neutral name that is a function of the comparison file, reached through a method (M32)
		{"comparison helper by method", "reaches a comparison file", func(f map[string]string) {
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "return x.neutral()", 1)
			f["csharp_g1_executed.go"] = strings.Replace(f["csharp_g1_executed.go"], "func neutral()", "func (T) neutral()", 1)
		}},
		{"embed", "carries //go:embed", func(f map[string]string) {
			f["csharp_g1_expected.go"] += "\n//go:embed peek.json\nvar peek string\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "return peek", 1)
		}},
		{"stdin through fmt", "uses fmt.Scan", func(f map[string]string) {
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "import \"strings\"", "import (\n\t\"fmt\"\n\t\"strings\"\n)", 1)
			f["csharp_g1_expected.go"] += "\nfunc scan() { var s string; fmt.Scan(&s) }\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "scan()\n\treturn helper()", 1)
		}},
		{"init of the package", "uses os.Getenv", func(f map[string]string) {
			f["csharp_g1_expected.go"] += "\nfunc init() { _ = os.Getenv(\"X\") }\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "import \"strings\"", "import (\n\t\"os\"\n\t\"strings\"\n)", 1)
		}},
		{"init in the comparison", "an init function in a comparison file", func(f map[string]string) {
			f["csharp_g1_executed.go"] += "\nfunc init() { table[\"a\"] = \"c\" }\n"
		}},
		{"shared package variable", "uses the derivation variable", func(f map[string]string) {
			f["csharp_g1_expected.go"] += "\nvar shared = 1\n"
			f["csharp_g1_executed.go"] += "\nfunc set() { shared = 2 }\n"
		}},
		{"stale root", "root Derive does not exist", func(f map[string]string) {
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "func Derive()", "func Gone()", 1)
		}},
		// F2: a derivation entry point of a classified file that no root reaches is proved by nothing, even when it
		// calls a comparison function
		{"derivation function outside the roots", "csharp_g1_correct.go: DeriveR2 is not a root, not reached by one and not excluded", func(f map[string]string) {
			f["csharp_g1_correct.go"] = "package kit\n\nfunc DeriveR2() { _ = Compare() }\n"
		}},
		{"derivation function as a root", "reaches a comparison file", func(f map[string]string) {
			f["csharp_g1_correct.go"] = "package kit\n\nfunc DeriveR2() { _ = Compare() }\n"
			roots = []string{"Derive", "DeriveR2"}
		}},
		// F3: state through a file that is neither derivation nor comparison
		{"unclassified file reached", "hook.go: reached by the derivation but neither derivation, comparison nor neutral", func(f map[string]string) {
			f["hook.go"] = "package kit\n\nvar cache = map[string]string{}\n\nfunc get() string { return cache[\"a\"] }\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "_ = get()\n\treturn helper()", 1)
		}},
		{"neutral variable shared with the comparison", "uses the neutral variable cache that the derivation reads", func(f map[string]string) {
			f["memo.go"] = "package kit\n\nvar cache = map[string]string{}\n\nfunc get() string { return cache[\"a\"] }\n"
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "return helper()", "_ = get()\n\treturn helper()", 1)
			f["csharp_g1_executed.go"] += "\nfunc put() { cache[\"a\"] = string(Compare()) }\n"
		}},
		{"dot import", "import of strings", func(f map[string]string) {
			f["csharp_g1_expected.go"] = strings.Replace(f["csharp_g1_expected.go"], "import \"strings\"", "import . \"strings\"", 1)
		}},
	}
	for _, c := range cases {
		roots = []string{"Derive"}
		p := run(c.mutate)
		if !slices.ContainsFunc(p, func(s string) bool { return strings.Contains(s, c.want) }) {
			t.Errorf("%s: no problem containing %q in %v", c.name, c.want, p)
		}
	}
}

// The derivation is a function of the bytes it is handed and reads them: another observation gives another
// expectation, and the executed asset and the sealed expectation that sit in the ledger sources change
// nothing. A hidden file or a native result that fed the derivation would show here.
func TestCSharpG1ExpectedFollowsItsInputsOnly(t *testing.T) {
	a := g1ExpLoad(t)
	in := a.in
	in.Sealed = a.x.Metamorphic.Results()
	// the native-side assets present in the ledger sources are ignored
	with := in
	with.Src.Expected, with.Src.Executed = a.asset, g1xLoad(t).asset
	got, err := DeriveCSharpG1Expected(with)
	if err != nil || !g1eSame(got, a.want) {
		t.Errorf("the expected, executed asset of the ledger sources changed the derivation: %v", err)
	}
	// a witness source that Roslyn rejects gives another expectation
	var wit string
	for _, alt := range a.want.Alternatives {
		if len(alt.Witnesses) > 0 {
			wit = alt.Witnesses[0].Source
			break
		}
	}
	var obs CSharpG1RoslynObserved
	if err := json.Unmarshal(a.obs, &obs); err != nil || wit == "" {
		t.Fatalf("observation: %v %q", err, wit)
	}
	flipped := false
	for i := range obs.Sources {
		if obs.Sources[i].ID == wit && obs.Sources[i].Accepted != nil {
			v := !*obs.Sources[i].Accepted
			obs.Sources[i].Accepted = &v
			flipped = true
		}
	}
	if !flipped {
		t.Fatalf("witness source %s not found in the observation", wit)
	}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	other := in
	other.Observation = data
	// the pin of the observation changes with any byte, so the comparison leaves the pins out
	changed, err := DeriveCSharpG1Expected(other)
	if err == nil {
		changed.Inputs = a.want.Inputs
		if g1eSame(changed, a.want) {
			t.Errorf("a different observation of %s gave the same expectation", wit)
		}
	}
}
