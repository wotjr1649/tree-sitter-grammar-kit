package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Native tests need the pinned runtime source and a C compiler: TSGK_NATIVE_RUNTIME and
// TSGK_NATIVE_CC (absolute paths). Without them the tests skip, unless
// TSGK_NATIVE_REQUIRED=1 (CI), where a missing tool is a failure, never a pass.
func nativeTools(t *testing.T) (string, string) {
	t.Helper()
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("TSGK_NATIVE_REQUIRED=1 but TSGK_NATIVE_RUNTIME/TSGK_NATIVE_CC are not set")
		}
		t.Skip("native tools not configured (TSGK_NATIVE_RUNTIME, TSGK_NATIVE_CC)")
	}
	return rt, cc
}

func digestOf(t *testing.T, p string) (string, uint64) {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:]), uint64(len(data))
}

func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "native", name))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func fixtureGrammar(t *testing.T, name string) []kit.NativeInput {
	t.Helper()
	root := fixtureRoot(t, name)
	var out []kit.NativeInput
	for _, f := range []struct{ path, role string }{
		{"src/parser.c", "parser"}, {"src/scanner.c", "scanner"}, {"src/tree_sitter/alloc.h", "header"}, {"src/tree_sitter/array.h", "header"}, {"src/tree_sitter/parser.h", "header"},
	} {
		p := filepath.Join(root, filepath.FromSlash(f.path))
		if _, err := os.Stat(p); err != nil {
			continue
		}
		sum, n := digestOf(t, p)
		out = append(out, kit.NativeInput{Path: f.path, Role: f.role, SHA256: sum, Bytes: n})
	}
	return out
}

var (
	buildMu    sync.Mutex
	buildCache = map[string]*Build{}
	buildWork  string
)

// testBuildRequest is the one place a test build request is made, so no test build can
// miss the host settings: Linux refuses a build without the delegated cgroup parent
// (MEMORY_HARD_CAP_UNSUPPORTED), and identities only compare under the same sanitizer mode.
func testBuildRequest(work, rt, root string, g []kit.NativeInput, cc string, id kit.ToolIdentity) BuildRequest {
	return BuildRequest{Work: work, Runtime: rt, GrammarRoot: root, Grammar: g, Symbol: "tree_sitter_tsgk_plain", Compiler: cc, CompilerID: id,
		Sanitize: os.Getenv("TSGK_NATIVE_SANITIZE") == "1", CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
}

// fixtureBuild builds (once per test binary) the owned fixture with the given defines.
func fixtureBuild(t *testing.T, name string, defines ...string) *Build {
	t.Helper()
	rt, cc := nativeTools(t)
	key := name + "|" + strings.Join(defines, ",")
	buildMu.Lock()
	defer buildMu.Unlock()
	if b := buildCache[key]; b != nil {
		return b
	}
	if buildWork == "" {
		dir, err := os.MkdirTemp("", "tsgk-native-test-")
		if err != nil {
			t.Fatal(err)
		}
		buildWork = dir
	}
	sum, n := digestOf(t, cc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req := testBuildRequest(buildWork, rt, fixtureRoot(t, name), fixtureGrammar(t, name), cc, kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: n})
	req.Symbol, req.Defines = "tree_sitter_tsgk_"+name, defines
	b, err := NewBuild(ctx, req)
	if err != nil {
		if b != nil { // nil when the build was refused before any step ran
			for _, s := range b.Steps {
				t.Logf("%s %v: %s %s", s.Name, s.Argv, s.Result.Status, s.Stderr)
			}
		}
		t.Fatalf("build %s with compiler %s: %v", key, cc, err)
	}
	buildCache[key] = b
	return b
}

func TestMain(m *testing.M) {
	code := m.Run()
	if buildWork != "" {
		os.RemoveAll(buildWork)
	}
	os.Exit(code)
}

func editsFor(src string, steps ...[3]any) []kit.Edit {
	cur := []byte(src)
	var out []kit.Edit
	for _, s := range steps {
		start, end, repl := s[0].(int), s[1].(int), []byte(s[2].(string))
		e := kit.Edit{StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(repl)), Old: append([]byte(nil), cur[start:end]...), New: repl}
		out = append(out, e)
		cur = append(append(append([]byte(nil), cur[:start]...), repl...), cur[end:]...)
	}
	return out
}

func testContext(op string) Context {
	o := kit.NativeOperations()[op]
	return Context{Op: o, Route: "owned", Output: kit.OutputTree, PolicyRef: PolicyRef(o, kit.OutputTree), CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
}

func runCase(t *testing.T, b *Build, id, enc, src string, edits []kit.Edit, expect ...kit.StepExpectation) CaseResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := kit.IncrementalCase{ID: id, Encoding: enc, Edits: edits, Expect: expect}
	return b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(src))
}

const plainSource = "a = f(1, [2, 3]);\n{ b = \"s\"; }\nc = (4);\n"

// S05-A01: insert, delete and replace on valid source; every intermediate incremental
// tree equals the fresh tree of the same bytes, and the route is observed.
func TestIncrementalSequence(t *testing.T) {
	b := fixtureBuild(t, "plain")
	edits := editsFor(plainSource, [3]any{14, 14, ", 9"}, [3]any{4, 5, "g"}, [3]any{34, 43, ""})
	r := runCase(t, b, "a01", kit.EncodingUTF8, plainSource, edits,
		kit.StepExpectation{Step: 0, Syntax: "NO_ERROR", Contains: []string{"assignment", "call", "list", "block"}},
		kit.StepExpectation{Step: 3, Syntax: "NO_ERROR", Contains: []string{"assignment"}})
	if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || r.Claims != (Claims{ClaimPass, ClaimPass, ClaimPass}) {
		t.Fatalf("result %s %s %s %+v", r.ExecutionStatus, r.Assessment, r.Code, r.Claims)
	}
	if len(r.Steps) != 4 {
		t.Fatalf("steps %d", len(r.Steps))
	}
	for _, s := range r.Steps[1:] {
		if !s.Comparison.Equal || !s.Route.Proven || s.Route.FreshReusedNodes != 0 {
			t.Fatalf("step %d %+v %+v", s.Step, s.Comparison, s.Route)
		}
	}
}

// S05-A02: a malformed middle state followed by repair; both states are compared and
// the malformed one is expected to carry an error.
func TestMalformedThenRepair(t *testing.T) {
	b := fixtureBuild(t, "plain")
	edits := editsFor(plainSource, [3]any{16, 17, ""}, [3]any{16, 16, ";"})
	r := runCase(t, b, "a02", kit.EncodingUTF8, plainSource, edits,
		kit.StepExpectation{Step: 1, Syntax: "ERROR"}, kit.StepExpectation{Step: 2, Syntax: "NO_ERROR"})
	if r.Assessment != kit.AssessPass || r.Steps[1].Incremental.HasError != true || r.Steps[2].Incremental.Digest != r.Steps[0].Incremental.Digest {
		t.Fatalf("result %s %s %+v", r.Assessment, r.Code, r.Claims)
	}
}

// S05-A04 and A03: owned fault-control builds lose the incremental route or corrupt only
// an intermediate step; the route and comparison checks detect each one.
func TestFaultControlsDetected(t *testing.T) {
	edits := editsFor(plainSource, [3]any{4, 5, "g"}, [3]any{4, 5, "f"})
	for _, tc := range []struct {
		define, claim string
		equality      string
	}{
		{"TSGK_FAULT_OMIT_EDIT", "route", ""},
		{"TSGK_FAULT_OMIT_OLD_TREE", "route", ""},
		{"TSGK_FAULT_FRESH_WITH_OLD", "route", ""},
		{"TSGK_FAULT_STEP1_FLAG", "equality", ""},
	} {
		t.Run(tc.define, func(t *testing.T) {
			b := fixtureBuild(t, "plain", tc.define)
			r := runCase(t, b, "fault", kit.EncodingUTF8, plainSource, edits)
			if r.Assessment != kit.AssessFail {
				t.Fatalf("fault not detected: %s %s %+v", r.Assessment, r.Code, r.Claims)
			}
			switch tc.claim {
			case "route":
				if r.Claims.IncrementalRoute != ClaimFail {
					t.Fatalf("route claim %+v", r.Claims)
				}
			case "equality":
				if r.Claims.IncrementalEquality != ClaimFail || r.Steps[1].Comparison.Equal || !r.Steps[2].Comparison.Equal || r.Code != "INCREMENTAL_FRESH_MISMATCH_STEP_1" {
					t.Fatalf("intermediate mismatch not reported at step 1: %s %+v", r.Code, r.Claims)
				}
			}
		})
	}
}

const heredoc = "abc def;\n<<EOT\none\ntwo\nEOT\nghi;\n"

// S05-A08: an owned stateful external scanner keeps its delimiter across an incremental
// reparse inside the heredoc; the deliberate serialization defect is detected.
func TestStatefulScanner(t *testing.T) {
	edits := editsFor(heredoc, [3]any{19, 22, "tw0x"}, [3]any{19, 23, "two"})
	exp := []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"statement", "heredoc", "heredoc_line", "heredoc_end"}}, {Step: 1, Syntax: "NO_ERROR"}}
	good := runCase(t, fixtureBuild(t, "stateful"), "a08", kit.EncodingUTF8, heredoc, edits, exp...)
	if good.Assessment != kit.AssessPass {
		t.Fatalf("stateful scanner: %s %s %+v", good.Assessment, good.Code, good.Claims)
	}
	bad := runCase(t, fixtureBuild(t, "stateful", "TSGK_SCANNER_FAULT"), "a08", kit.EncodingUTF8, heredoc, edits, exp...)
	if bad.Assessment != kit.AssessFail || bad.Claims.IncrementalEquality != ClaimFail {
		t.Fatalf("serialization defect not detected: %s %s %+v", bad.Assessment, bad.Code, bad.Claims)
	}
}
