package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func symbolContext(symbols ...string) Context {
	x := testContext("native-parse-edit")
	pp := &kit.Preprocess{Mode: kit.PreprocessMode, Symbols: symbols}
	x.Preprocess, x.PolicyRef = pp, kit.NativePolicy(x.Op, x.Output, pp)
	x.Protocol, x.ObservationRunID = ProtocolR3, "symbol-run"
	return x
}

// parsed reports whether the step's incremental tree has a node other than the root inside
// the bytes of word: the independent anchor of which section the parser read.
func parsed(t *testing.T, r CaseResult, step int, src, word string) bool {
	t.Helper()
	at := strings.Index(src, word)
	if at < 0 || step >= len(r.Steps) || r.Steps[step].Incremental == nil || r.Steps[step].Incremental.Tree == nil {
		t.Fatalf("step %d of %s has no full tree", step, r.ID)
	}
	for _, n := range r.Steps[step].Incremental.Tree.Nodes {
		if n.Parent >= 0 && int(n.StartByte) >= at && int(n.EndByte) <= at+len(word) && n.EndByte > n.StartByte {
			return true
		}
	}
	return false
}

// #218 (218-B): the symbol mode parses each step's active ranges only. The parsed section
// follows the caller symbols, is recomputed after an edit that changes the branch (stale
// ranges would keep the old section), and a full-input parse of the same source reads both
// sections, so the anchor check has the power to see a run without ranges.
func TestNativePreprocessRanges(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered C# closure not configured; mandatory native routes own this check")
	}
	b := registeredBuild(t, "csharp")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	src := "#if A\nalpha = 1;\n#else\nbeta = 2;\n#endif\ngamma = 3;\n"
	c := kit.IncrementalCase{ID: "branch", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{SHA256: sha([]byte(src)), Bytes: uint64(len(src))}}
	for symbol, want := range map[string][2]string{"A": {"alpha", "beta"}, "": {"beta", "alpha"}} {
		var symbols []string
		if symbol != "" {
			symbols = []string{symbol}
		}
		x := symbolContext(symbols...)
		r := b.RunCase(ctx, x, c, []byte(src))
		if r.ExecutionStatus != kit.StatusCompleted || len(r.Preprocess) != 1 || r.Preprocess[0].Status != kit.PreprocessOK || len(r.Preprocess[0].Active) != 2 {
			t.Fatalf("symbols %q: %s %s %+v", symbols, r.ExecutionStatus, r.Code, r.Preprocess)
		}
		if !parsed(t, r, 0, src, want[0]) || parsed(t, r, 0, src, want[1]) || !parsed(t, r, 0, src, "gamma") {
			t.Fatalf("symbols %q: the parser did not read %s alone", symbols, want[0])
		}
		if ids := r.Steps[0].Incremental.Tree.Identities; ids[len(ids)-1] != x.PolicyRef || x.PolicyRef.Schema != kit.NativePolicySchemaR4 {
			t.Fatalf("tree identities %+v", ids)
		}
	}
	full := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(src))
	foundDirective := false
	for _, n := range full.Steps[0].Incremental.Tree.Nodes {
		if n.Type == "#if" && n.EndByte > n.StartByte {
			foundDirective = true
		}
	}
	if !foundDirective {
		t.Fatal("a full-input parse must read directive bytes excluded from every active selection")
	}
	// the edit turns #if A into #if B: step 1 reads the #else section
	edited := kit.IncrementalCase{ID: "edit", Encoding: kit.EncodingUTF8, Input: c.Input, Edits: editsBy(src, edit{"#if A", "#if B"})}
	r := b.RunCase(ctx, symbolContext("A"), edited, []byte(src))
	if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != 2 || len(r.Preprocess) != 2 {
		t.Fatalf("edit: %s %s %+v", r.ExecutionStatus, r.Code, r.Preprocess)
	}
	if !parsed(t, r, 0, src, "alpha") || parsed(t, r, 0, src, "beta") {
		t.Fatal("step 0 did not read the #if section")
	}
	after := strings.Replace(src, "#if A", "#if B", 1)
	if !parsed(t, r, 1, after, "beta") || parsed(t, r, 1, after, "alpha") {
		t.Fatal("step 1 kept the ranges of step 0")
	}
}

// The primary must see full source, without inheriting the active NO_ERROR expectation.
func TestNativePreprocessCST(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered C# closure not configured; mandatory native routes own this check")
	}
	b := registeredBuild(t, "csharp")
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "csharp-pp", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			ID      string   `json:"id"`
			Source  []byte   `json:"source"`
			Symbols []string `json:"symbols"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"dollars-two-quotes-raw": "ERROR", "nested-string-open-no-runaway": "ERROR", "directive-string-no-escape": "ERROR", "razor-comment-lines": "ERROR", "issue198-inactive-broken": "NO_ERROR", "feff-after-hash": "NO_ERROR"}
	anchors := map[string][]uint32{"dollars-two-quotes-raw": {}, "nested-string-open-no-runaway": {}, "directive-string-no-escape": {0}, "razor-comment-lines": {18, 35}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, source := range corpus.Cases {
		syntax, selected := want[source.ID]
		if !selected {
			continue
		}
		delete(want, source.ID)
		t.Run(source.ID, func(t *testing.T) {
			c := kit.IncrementalCase{ID: source.ID, Input: kit.NativeInput{SHA256: sha(source.Source), Bytes: uint64(len(source.Source))}, Encoding: kit.EncodingUTF8,
				Expect: []kit.StepExpectation{{Step: 0, Syntax: syntax}}}
			r := b.RunCase(ctx, symbolContext(source.Symbols...), c, source.Source)
			if r.PreprocessCST == nil {
				t.Fatal("missing CST receipt")
			}
			if r.PreprocessCST.Result != "PASS" || r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || len(r.PreprocessCST.Steps) != 1 || len(r.Steps) != 1 || len(r.PrimaryRaw) == 0 || len(r.Raw) == 0 {
				t.Fatalf("producer correction unverified: %+v / %s %s", r.PreprocessCST, r.Assessment, r.Code)
			}
			if positions, fixed := anchors[source.ID]; fixed && !slices.Equal(r.PreprocessCST.Steps[0].CSTHashes, positions) {
				t.Fatalf("primary positions %v; independent anchors %v", r.PreprocessCST.Steps[0].CSTHashes, positions)
			}
			if syntax == "ERROR" && !slices.ContainsFunc(r.Steps[0].Incremental.Tree.Nodes, func(n kit.TreeNode) bool { return n.IsError }) {
				t.Fatal("malformed literal lost its genuine ERROR node")
			}
		})
	}
	if len(want) != 0 {
		t.Fatalf("missing corpus controls: %v", want)
	}
	t.Run("lexical-boundaries", func(t *testing.T) { testCSharpLexicalBoundaries(t, b) })
	t.Run("lexical-edit-revert", func(t *testing.T) { testCSharpLexicalEditRevert(t, b) })
}

// The independent Roslyn controls distinguish doubled-quote content from malformed prefixes.
func testCSharpLexicalBoundaries(t *testing.T, b *Build) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, tc := range []struct{ id, source, syntax string }{
		{"verbatim-content-254", "class C { string s = $@\"" + strings.Repeat("\"", 254) + "x\"; }\n", "NO_ERROR"},
		{"verbatim-content-256", "class C { string s = $@\"" + strings.Repeat("\"", 256) + "x\"; }\n", "NO_ERROR"},
		{"prefix-dollar-at", "class C { string s = $@\"x\"; }\n", "NO_ERROR"},
		{"prefix-at-dollar", "class C { string s = @$\"x\"; }\n", "NO_ERROR"},
		{"prefix-at-dollar-at", "class C { string s = @$@\"x\"; }\n", "ERROR"},
		{"line-filename-lf-before", "#line 10 /* c\n*/ \"file.cs\"\nclass C {}", "ERROR"},
		{"line-filename-crlf-before", "#line 10 /* c\r\n*/ \"file.cs\"\r\nclass C {}", "ERROR"},
		{"line-filename-cr-before", "#line 10 /* c\r*/ \"file.cs\"\rclass C {}", "ERROR"},
		{"line-filename-nel-before", "#line 10 /* c\u0085*/ \"file.cs\"\u0085class C {}", "ERROR"},
		{"line-filename-ls-before", "#line 10 /* c\u2028*/ \"file.cs\"\u2028class C {}", "ERROR"},
		{"line-filename-ps-before", "#line 10 /* c\u2029*/ \"file.cs\"\u2029class C {}", "ERROR"},
		{"line-filename-inline", "#line 10 /* c */ \"file.cs\"\nclass C {}", "ERROR"},
		{"line-filename-unterminated", "#line 10 /* c\n\"file.cs\"\nclass C {}", "ERROR"},
		{"line-filename-follow-directive", "#line 10 /* c\n*/ \"file.cs\"\n#define A\nclass C {}", "ERROR"},
		{"line-filename-normal-backslash", "#line 10 \"a\\b.cs\"\nclass C {}", "NO_ERROR"},
		{"line-filename-normal-empty", "#line 10 \"\"\nclass C {}", "NO_ERROR"},
		{"line-filename-enhanced-normal", "#line (1, 1) - (2, 1) \"file.cs\"\nclass C {}", "NO_ERROR"},
		{"line-filename-enhanced-comment", "#line (1, 1) - (2, 1) /* c */ \"file.cs\"\nclass C {}", "ERROR"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			c := kit.IncrementalCase{ID: tc.id, Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: tc.syntax}}}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(tc.source))
			if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass {
				t.Fatalf("lexical boundary %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
			}
		})
	}
}

func testCSharpLexicalEditRevert(t *testing.T, b *Build) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "csharp-pp", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	type lexicalSource struct {
		ID     string `json:"id"`
		Source []byte `json:"source"`
	}
	var corpus struct {
		Cases []lexicalSource `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	changes := map[string]edit{
		"directive-string-no-escape":    {`"""`, `""""`},
		"razor-comment-lines":           {"@*", "@**"},
		"dollars-two-quotes-raw":        {`$$""`, `$$"`},
		"nested-string-open-no-runaway": {"#else", "#elif"},
	}
	for _, tc := range []struct{ id, comment string }{
		{"line-filename-multiline", "/* comment\n*/ "},
		{"line-filename-inline", "/* comment */ "},
	} {
		corpus.Cases = append(corpus.Cases, lexicalSource{tc.id, []byte("#line 10 \"file.cs\"\nclass C {}")})
		changes[tc.id] = edit{"10 ", "10 " + tc.comment}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, source := range corpus.Cases {
		change, found := changes[source.ID]
		if !found {
			continue
		}
		delete(changes, source.ID)
		t.Run(source.ID, func(t *testing.T) {
			c := kit.IncrementalCase{ID: source.ID, Encoding: kit.EncodingUTF8,
				Edits: editsBy(string(source.Source), change, edit{change.repl, change.find})}
			lineFilename := strings.HasPrefix(source.ID, "line-filename-")
			if lineFilename {
				c.Expect = []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 1, Syntax: "ERROR"}, {Step: 2, Syntax: "NO_ERROR"}}
			}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, source.Source)
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != 3 || r.Claims.IncrementalEquality != ClaimPass {
				t.Fatalf("edit/revert CST unverified: %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
			}
			for _, step := range r.Steps {
				if step.Incremental == nil || step.Incremental.Tree == nil {
					t.Fatalf("missing incremental full CST at step %d", step.Step)
				}
				if step.Step == 0 {
					if step.Fresh != nil {
						t.Fatal("initial parse unexpectedly has a fresh counterpart")
					}
					continue
				}
				if step.Fresh == nil || step.Fresh.Tree == nil || kit.CompareTrees(step.Incremental.Tree.Nodes, step.Fresh.Tree.Nodes) != nil {
					t.Fatalf("incremental/fresh full CST differs at step %d", step.Step)
				}
			}
			if lineFilename && (r.Claims.Expectations != ClaimPass || !slices.ContainsFunc(r.Steps[1].Incremental.Tree.Nodes, func(n kit.TreeNode) bool { return n.IsError })) {
				t.Fatal("comment before filename lost its genuine ERROR")
			}
			if r.Steps[0].SourceSHA256 != r.Steps[2].SourceSHA256 || kit.CompareTrees(r.Steps[0].Incremental.Tree.Nodes, r.Steps[2].Fresh.Tree.Nodes) != nil {
				t.Fatal("reverted source did not restore the full CST")
			}
		})
	}
	if len(changes) != 0 {
		t.Fatalf("missing edit/revert cases: %v", changes)
	}
}

// A step the evaluator stops decides the case before any driver runs: the fixed status and
// code, assessment BLOCKED, no claim, no step, process or response, and the evaluated steps
// up to that one recorded.
func TestNativePreprocessStopped(t *testing.T) {
	b := fixtureBuild(t, "plain")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	zero := "#if A\nalpha = 1;\n#endif\n"
	for name, c := range map[string]struct {
		src   string
		edits []kit.Edit
		code  string
		exec  string
		steps int
	}{
		"zero-active":   {"#if B\nalpha = 1;\n#endif\n", nil, "PREPROCESS_ZERO_ACTIVE", kit.StatusNotRun, 1},
		"zero-by-edit":  {zero, editsBy(zero, edit{"#if A", "#if B"}), "PREPROCESS_ZERO_ACTIVE", kit.StatusNotRun, 2},
		"malformed":     {"#if\nalpha = 1;\n#endif\n", nil, "PREPROCESS_MALFORMED", kit.StatusNotRun, 1},
		"range-limit":   {strings.Repeat("x = 1;\n#if A\n#endif\n", 17), nil, "PREPROCESS_RANGE_LIMIT", kit.StatusResourceLimit, 1},
		"invalid-utf-8": {"alpha = 1;\n", editsBy("alpha = 1;\n", edit{"1", "\xff"}), "PREPROCESS_INVALID_INPUT", kit.StatusNotRun, 2},
	} {
		r := b.RunCase(ctx, symbolContext("A"), kit.IncrementalCase{ID: name, Encoding: kit.EncodingUTF8, Edits: c.edits}, []byte(c.src))
		if r.ExecutionStatus != c.exec || r.Assessment != kit.AssessBlocked || r.Code != c.code || len(r.Steps) != 0 || r.Process != nil || r.Raw != nil ||
			r.Claims != (Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}) || len(r.Preprocess) != c.steps {
			t.Errorf("%s: %s %s %s steps=%d preprocess=%d", name, r.ExecutionStatus, r.Assessment, r.Code, len(r.Steps), len(r.Preprocess))
		}
	}
	// a stopped case keeps the run from passing
	if _, v := aggregate([]CaseResult{{ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass}, {ExecutionStatus: kit.StatusNotRun, Assessment: kit.AssessBlocked}}); v == kit.AssessPass {
		t.Fatal("a stopped symbol-mode case did not block the run")
	}
}
