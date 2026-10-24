package native

import (
	"context"
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
	b := fixtureBuild(t, "plain")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	src := "#if A\nalpha = 1;\n#else\nbeta = 2;\n#endif\ngamma = 3;\n"
	c := kit.IncrementalCase{ID: "branch", Encoding: kit.EncodingUTF8}
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
		if ids := r.Steps[0].Incremental.Tree.Identities; ids[len(ids)-1] != x.PolicyRef || x.PolicyRef.Schema != kit.NativePolicySchemaR3 {
			t.Fatalf("tree identities %+v", ids)
		}
	}
	full := b.RunCase(ctx, testContext("native-parse-edit"), c, []byte(src))
	if !parsed(t, full, 0, src, "alpha") || !parsed(t, full, 0, src, "beta") {
		t.Fatal("a full-input parse must read both sections (the anchor check would not see a run without ranges)")
	}
	// the edit turns #if A into #if B: step 1 reads the #else section
	edited := kit.IncrementalCase{ID: "edit", Encoding: kit.EncodingUTF8, Edits: editsBy(src, edit{"#if A", "#if B"})}
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
