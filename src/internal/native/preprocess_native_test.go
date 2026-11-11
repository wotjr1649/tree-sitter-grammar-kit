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
			hasError := slices.ContainsFunc(r.Steps[0].Incremental.Tree.Nodes, func(n kit.TreeNode) bool { return n.IsError })
			if (syntax == "ERROR") != hasError {
				t.Fatalf("syntax=%s but native ERROR node present=%t", syntax, hasError)
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
		{"directive-undef-line-tail", "#undef A // comment\nclass C {}", "NO_ERROR"},
		{"directive-endif-line-tail", "#if true\n#endif // comment\nclass C {}", "NO_ERROR"},
		{"directive-if-block-tail", "#if A /* c */\nclass C {}\n#endif\n", "ERROR"},
		{"pragma-line-comment-tail", "#pragma warning disable 123 // text\nclass C {}\n", "NO_ERROR"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			c := kit.IncrementalCase{ID: tc.id, Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: tc.syntax}}}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(tc.source))
			if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass {
				t.Fatalf("lexical boundary %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
			}
			if tc.id == "pragma-line-comment-tail" {
				nodes := r.Steps[0].Incremental.Tree.Nodes
				if !slices.ContainsFunc(nodes, func(n kit.TreeNode) bool {
					return n.Type == "preproc_arg" && n.StartByte == 28 && n.EndByte == 35 && n.Named && !n.Extra
				}) || slices.ContainsFunc(nodes, func(n kit.TreeNode) bool { return n.Type == "comment" }) {
					t.Fatal("pragma line-comment tail lost its baseline preproc_arg ownership")
				}
			}
		})
	}
	t.Run("pragma-string-boundaries", func(t *testing.T) { testCSharpPragmaStringBoundaries(t, b) })
	t.Run("residual-oracle-controls", func(t *testing.T) { testCSharpResidualOracleControls(t, b) })
	t.Run("registered-define-trailing-comment", func(t *testing.T) { testCSharpRegisteredDefineTrailingComment(t, b) })
}

func testCSharpPragmaStringBoundaries(t *testing.T, b *Build) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "csharp-pp", "pragma-string-boundaries.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema string `json:"schema"`
		Cases  []struct {
			ID         string `json:"id"`
			Source     string `json:"source"`
			UndefStart uint32 `json:"undef_start"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Schema != "tsgk-csharp-pragma-string-boundaries/r1" || len(fixture.Cases) != 30 {
		t.Fatal("pragma string boundary fixture is incomplete")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, tc := range fixture.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			if int(tc.UndefStart) != strings.Index(tc.Source, "#undef") {
				t.Fatal("independent directive anchor does not match source")
			}
			c := kit.IncrementalCase{ID: tc.ID, Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(tc.Source))
			if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || len(r.Steps) != 1 {
				t.Fatalf("pragma boundary %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
			}
			nodes := r.Steps[0].Incremental.Tree.Nodes
			for _, want := range []struct {
				kind       string
				start, end uint32
			}{
				{"preproc_pragma", 0, tc.UndefStart},
				{"preproc_arg", 8, tc.UndefStart - 1},
				{"#undef", tc.UndefStart, tc.UndefStart + 6},
			} {
				if !slices.ContainsFunc(nodes, func(n kit.TreeNode) bool {
					return n.Type == want.kind && n.StartByte == want.start && n.EndByte == want.end && !n.IsMissing
				}) {
					t.Fatalf("pragma tail lost %s [%d,%d)", want.kind, want.start, want.end)
				}
			}
		})
	}
}

func testCSharpResidualOracleControls(t *testing.T, b *Build) {
	// These source bytes and Roslyn spans are the selected all-v3 oracle records, bound by the file pins below.
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "csharp-pp", "residual-controls.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Schema          string `json:"schema"`
		Roslyn          string `json:"roslyn"`
		LanguageVersion string `json:"language_version"`
		CasesSHA256     string `json:"cases_sha256"`
		ObservedSHA256  string `json:"observed_sha256"`
		RoslynDLLSHA256 string `json:"roslyn_dll_sha256"`
		Cases           []struct {
			ID        string   `json:"id"`
			Source    []byte   `json:"source"`
			SourceSHA string   `json:"source_sha256"`
			Symbols   []string `json:"symbols"`
			Roslyn    struct {
				Hashes      []uint32 `json:"directive_hashes"`
				Errors      []string `json:"errors"`
				Diagnostics []struct {
					ID      string `json:"id"`
					Start   uint32 `json:"start"`
					End     uint32 `json:"end"`
					Context string `json:"context"`
				} `json:"diagnostics"`
			} `json:"roslyn"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Schema != "tsgk-csharp-pp-residual-controls/r1" || fixture.Roslyn != "5.9.0.0" || fixture.LanguageVersion != "Latest" || fixture.CasesSHA256 != "437b4d345fa86f5b95fce494e79af551731ed9dfa6485c89f8b33ecdc6f7c55d" || fixture.ObservedSHA256 != "14c62706f61cc8f6a79830fdbf9e7898000a3b1826754565e3e97ff96337ab13" || fixture.RoslynDLLSHA256 != "12b0428203faae6555858d2ff856cef7450d2764cf00c57e7516587a04885870" {
		t.Fatal("residual controls lost their pinned Roslyn observation identity")
	}
	if len(fixture.Cases) != 20 {
		t.Fatalf("residual oracle fixture has %d cases, want 20", len(fixture.Cases))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, tc := range fixture.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			if sha(tc.Source) != tc.SourceSHA {
				t.Fatal("residual source bytes do not match the pinned SHA256")
			}
			observedErrors := make(map[string]bool, len(tc.Roslyn.Diagnostics))
			for _, diagnostic := range tc.Roslyn.Diagnostics {
				if int(diagnostic.Start) > len(tc.Source) || int(diagnostic.End) > len(tc.Source) || diagnostic.Start > diagnostic.End {
					t.Fatalf("invalid Roslyn diagnostic span: %+v", diagnostic)
				}
				observedErrors[diagnostic.ID] = true
			}
			for _, id := range tc.Roslyn.Errors {
				if !observedErrors[id] {
					t.Fatalf("Roslyn error %s has no diagnostic span", id)
				}
			}
			syntax := "ERROR"
			if len(tc.Roslyn.Errors) == 0 {
				syntax = "NO_ERROR"
			}
			c := kit.IncrementalCase{ID: strings.ReplaceAll(tc.ID, "/", "-"), Input: kit.NativeInput{SHA256: tc.SourceSHA, Bytes: uint64(len(tc.Source))}, Encoding: kit.EncodingUTF8,
				Expect: []kit.StepExpectation{{Step: 0, Syntax: syntax}}}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, tc.Source)
			if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || r.Claims.Expectations != ClaimPass || len(r.Steps) != 1 || r.Steps[0].Incremental == nil || r.Steps[0].Incremental.Tree == nil {
				t.Fatalf("residual source/diagnostic parity failed: %s/%s/%s %+v", r.ExecutionStatus, r.Assessment, r.Code, r.PreprocessCST)
			}
			var hashes []uint32
			for _, node := range r.Steps[0].Incremental.Tree.Nodes {
				if !node.IsMissing && node.StartByte < node.EndByte && slices.Contains([]string{"#region", "#endregion", "#line", "#pragma", "#nullable", "#error", "#warning", "#define", "#undef", "#if", "#endif", "#else", "#elif"}, node.Type) {
					hashes = append(hashes, node.StartByte)
				}
			}
			if !slices.Equal(hashes, tc.Roslyn.Hashes) {
				t.Fatalf("directive hashes %v, Roslyn %v", hashes, tc.Roslyn.Hashes)
			}
			hasError := slices.ContainsFunc(r.Steps[0].Incremental.Tree.Nodes, func(n kit.TreeNode) bool { return n.IsError })
			if (syntax == "ERROR") != hasError {
				t.Fatalf("Roslyn syntax=%s but native ERROR node present=%t", syntax, hasError)
			}
		})
	}
}

func testCSharpRegisteredDefineTrailingComment(t *testing.T, b *Build) {
	// Reuse the independent disposition in the existing native gaps registry.
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "gaps", "csharp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			ID         string `json:"id"`
			SourceUTF8 string `json:"source_utf8"`
			Expect     []struct {
				Step   int    `json:"step"`
				Syntax string `json:"syntax"`
			} `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		if tc.ID != "csharp-issue157-pp-define-ok-12-r1" {
			continue
		}
		if tc.SourceUTF8 != "#define A // comment\nclass C {}" || len(tc.Expect) != 1 || tc.Expect[0].Step != 0 || tc.Expect[0].Syntax != "NO_ERROR" {
			t.Fatal("registered define trailing-comment control changed")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c := kit.IncrementalCase{ID: tc.ID, Input: kit.NativeInput{SHA256: sha([]byte(tc.SourceUTF8)), Bytes: uint64(len(tc.SourceUTF8))}, Encoding: kit.EncodingUTF8,
			Expect: []kit.StepExpectation{{Step: tc.Expect[0].Step, Syntax: tc.Expect[0].Syntax}}}
		r := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(tc.SourceUTF8))
		if r.ExecutionStatus != kit.StatusCompleted || r.Assessment != kit.AssessPass || r.Claims.Expectations != ClaimPass || len(r.Steps) != 1 || !parsed(t, r, 0, tc.SourceUTF8, "class C {}") {
			t.Fatalf("registered directive-comment control failed: %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
		}
		return
	}
	t.Fatal("registered define trailing-comment control is missing")
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
	t.Run("residual", func(t *testing.T) { testCSharpResidualEditRevert(t, b) })
}

func testCSharpResidualEditRevert(t *testing.T, b *Build) {
	type residualCase struct {
		id, source string
		edits      []edit
		syntax     []string
	}
	const rawQ2Single = "x = $$\"\"\"x\"\";\n#if A\nclass X {}\n#endif\n"
	const rawQ2Multi = "x = $$\"\"\"\nx\n\"\";\n#if A\nclass X {}\n#endif\n"
	const q2Close = "\"\";\n#if A"
	const q3Close = "\"\"\";\n#if A"
	const q4Close = "\"\"\"\";\n#if A"
	rawC1 := "\"\"\"\n#if HIDDEN\n\"\"\""
	validC1 := "\"file.cs\""
	pragma := "#pragma warning disable \"\"\"\n#if A\n\"\"\"\n#if A\nclass X {}\n#endif\n"
	line := "#line 10 \"file.cs\"\nclass C {}"
	commentedLine := " /* c\n*/\n"
	cases := []residualCase{
		{"c1-raw-filename", "#line 1 " + rawC1 + "\n#if A\nclass X {}\n#endif\n", []edit{{rawC1, validC1}, {validC1, rawC1}}, []string{"ERROR", "NO_ERROR", "ERROR"}},
		{"c2-raw-q3-close-single", rawQ2Single, []edit{{q2Close, q3Close}, {q3Close, q4Close}, {q4Close, q3Close}, {q3Close, q2Close}}, []string{"ERROR", "NO_ERROR", "ERROR", "NO_ERROR", "ERROR"}},
		{"c2-raw-q3-close-multi", rawQ2Multi, []edit{{q2Close, q3Close}, {q3Close, q4Close}, {q4Close, q3Close}, {q3Close, q2Close}}, []string{"ERROR", "NO_ERROR", "ERROR", "NO_ERROR", "ERROR"}},
		{"c3-pragma-raw-tail", pragma, []edit{{"#if A", "#if B"}, {"#if B", "#if A"}}, []string{"NO_ERROR", "NO_ERROR", "NO_ERROR"}},
		{"c4-line-filename-after-comment", line, []edit{{"\n", commentedLine}, {commentedLine, "\n"}}, []string{"NO_ERROR", "ERROR", "NO_ERROR"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			expect := make([]kit.StepExpectation, len(tc.syntax))
			for step, syntax := range tc.syntax {
				expect[step] = kit.StepExpectation{Step: step, Syntax: syntax}
			}
			c := kit.IncrementalCase{ID: tc.id, Encoding: kit.EncodingUTF8, Edits: editsBy(tc.source, tc.edits...), Expect: expect}
			r := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(tc.source))
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != len(tc.edits)+1 || r.Claims.IncrementalEquality != ClaimPass || (len(tc.syntax) > 0 && r.Claims.Expectations != ClaimPass) {
				t.Fatalf("residual edit/revert CST unverified: %s/%s/%s", r.ExecutionStatus, r.Assessment, r.Code)
			}
			for i, step := range r.Steps {
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
				if len(tc.syntax) > 0 {
					hasError := slices.ContainsFunc(step.Incremental.Tree.Nodes, func(n kit.TreeNode) bool { return n.IsError })
					if (tc.syntax[i] == "ERROR") != hasError {
						t.Fatalf("step %d: syntax=%s but native ERROR node present=%t", step.Step, tc.syntax[i], hasError)
					}
				}
			}
			last := len(r.Steps) - 1
			if r.Steps[0].SourceSHA256 != r.Steps[last].SourceSHA256 || kit.CompareTrees(r.Steps[0].Incremental.Tree.Nodes, r.Steps[last].Fresh.Tree.Nodes) != nil {
				t.Fatal("reverted residual source did not restore the full CST")
			}
		})
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
