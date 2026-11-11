package queryregistry

import (
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
)

func TestRosterDenominatorsAndMeaning(t *testing.T) {
	got := Roster()
	wantPositive := []string{
		"eq-0", "eq-1", "not-eq-0", "not-eq-1", "any-of-0", "any-of-1", "not-any-of-0", "not-any-of-1",
		"capture-pair-0", "capture-pair-1", "multiple-0", "multiple-1", "two-captures-kept", "two-captures-rejected",
		"distinct-captures-eq-equal", "distinct-captures-eq-different", "distinct-captures-not-eq-equal", "distinct-captures-not-eq-different",
		"pattern-isolation", "evaluated-empty",
	}
	if len(got) != 67 {
		t.Fatalf("roster rows = %d, want 67", len(got))
	}
	ids := make(map[string]bool, len(got))
	var positive, control []string
	for _, p := range got {
		if p.ID == "" || ids[p.ID] {
			t.Fatalf("empty or duplicate ID %q", p.ID)
		}
		ids[p.ID] = true
		if p.Source == "" && p.ID != "evaluated-empty" && p.ID != "not-run" || p.Query == "" && p.ID != "not-run" || p.Basis == "" {
			t.Fatalf("incomplete source/query basis in %q", p.ID)
		}
		switch p.Kind {
		case predicateRequired:
			positive = append(positive, p.ID)
			if p.Status != "COMPLETED" || p.Evaluation != "EVALUATED" || p.Expected == "" || p.PolicyInput != nil {
				t.Fatalf("positive roster item lacks required evaluated expectation: %+v", p)
			}
		case expectedRejection:
			control = append(control, p.ID)
		default:
			t.Fatalf("unknown roster kind %q", p.Kind)
		}
	}
	if !slices.Equal(positive, wantPositive) {
		t.Fatalf("positive IDs differ:\n got %v\nwant %v", positive, wantPositive)
	}
	if len(control) != 47 {
		t.Fatalf("control rows = %d, want 47", len(control))
	}
	for i, p := range got {
		if i < len(wantPositive) {
			continue
		}
		if p.Kind != expectedRejection || p.Expected == "" {
			t.Fatalf("control row %q is not a fixed rejection expectation", p.ID)
		}
	}
}

func TestRosterPreservesControlStates(t *testing.T) {
	byID := make(map[string]Probe)
	for _, p := range Roster() {
		byID[p.ID] = p
	}
	for _, id := range []string{"unsupported-match", "left-eq-zero", "right-eq-zero-or-one", "encoding-eq-utf16le"} {
		p, ok := byID[id]
		if !ok || p.Kind != expectedRejection || p.Status != "UNSUPPORTED" || p.Evaluation != "UNSUPPORTED" || p.Expected != "NULL" || p.Query != "owned host-policy fixture" || p.Source != "a b b b" {
			t.Fatalf("unsupported control %q changed: %+v", id, p)
		}
	}
	if p := byID["left-eq-zero"]; p.PolicyInput == nil || p.PolicyInput.Operator != "eq?" || p.PolicyInput.LeftCapture != "x" || p.PolicyInput.LeftQuantifier != "ZERO" || !slices.Equal(p.PolicyInput.Literals, []string{"a"}) || !strings.HasSuffix(p.Basis, "/eq?/ZERO") {
		t.Fatalf("ZERO left operand was not represented as host policy input: %+v", p)
	}
	if p := byID["left-eq-zero-or-one"]; p.PolicyInput == nil || p.PolicyInput.LeftQuantifier != "ZERO_OR_ONE" || !strings.HasSuffix(p.Basis, "/eq?/ZERO_OR_ONE") {
		t.Fatalf("ZERO_OR_ONE left operand changed: %+v", p)
	}
	if p := byID["right-eq-zero-or-one"]; p.PolicyInput == nil || p.PolicyInput.LeftQuantifier != "ONE" || p.PolicyInput.RightCapture != "y" || p.PolicyInput.RightQuantifier != "ZERO_OR_ONE" || !strings.HasSuffix(p.Basis, "/eq?/right-quantifier") {
		t.Fatalf("right quantifier control changed: %+v", p)
	}
	if p := byID["encoding-eq-utf16le"]; p.PolicyInput == nil || p.Encoding != "UTF-16LE" || !strings.HasSuffix(p.Basis, "/eq?/UTF-16LE") {
		t.Fatalf("encoding control does not identify the policy fixture row: %+v", p)
	}
	if p := byID["unsupported-set"]; p.PolicyInput == nil || p.PolicyInput.Operator != "set!" || p.Code != "QUERY_PREDICATE_UNSUPPORTED:set!" {
		t.Fatalf("directive operator control changed: %+v", p)
	}
	if p := byID["absent-left-capture"]; p.Status != "FAILED" || p.Code != "QUERY_PREDICATE_CAPTURE_ABSENT" || p.Structural != "PRESERVED_4" {
		t.Fatalf("absent capture lost its failure or structural stream: %+v", p)
	}
	if p := byID["partial-predicate"]; p.Status != "RESOURCE_LIMIT" || p.Code != "CAPTURE_LIMIT" || !p.Partial || p.Expected != "NULL" {
		t.Fatalf("partial predicate state changed: %+v", p)
	}
	if p := byID["partial-structural"]; p.Evaluation != "STRUCTURAL" || !p.Partial || p.Expected != "PRESERVED_STRUCTURAL" {
		t.Fatalf("partial structural stream changed: %+v", p)
	}
	if p := byID["not-run"]; p.Status != "NOT_RUN" || p.Code != "TREE_NOT_COMPLETED" || p.Evaluation != "" || p.Expected != "NULL" || p.Source != "" || p.Query != "" || p.PolicyInput != nil {
		t.Fatalf("not-run control became an evaluated result: %+v", p)
	}
	if p := byID["evaluated-empty"]; p.Expected != "REGISTER_EMPTY_EVALUATED_ARRAY" || p.Source != "" || p.Evaluation != "EVALUATED" {
		t.Fatalf("completed empty expectation collapsed into missing: %+v", p)
	}
	if p := byID["pattern-isolation"]; p.Expected != "REGISTER_STRUCTURAL_X_N_EVALUATED_N" {
		t.Fatalf("pattern isolation expectation changed: %+v", p)
	}
	for _, id := range []string{"pattern-isolation", "evaluated-empty"} {
		if !strings.HasPrefix(byID[id].Basis, "registry-policy/r1#") {
			t.Fatal("new expectation proposal falsely attributed to historical check.py", id)
		}
	}
}

// Each row is transcribed from query_policy_test.go, independently of Roster's loops.
func TestControlRosterExact(t *testing.T) {
	want := []struct{ id, input, encoding, selector string }{
		{"unsupported-match", "match?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/match?"},
		{"unsupported-not-match", "not-match?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-match?"},
		{"unsupported-any-eq", "any-eq?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-eq?"},
		{"unsupported-any-not-eq", "any-not-eq?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-not-eq?"},
		{"unsupported-any-match", "any-match?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-match?"},
		{"unsupported-any-not-match", "any-not-match?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-not-match?"},
		{"unsupported-is", "is?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/is?"},
		{"unsupported-is-not", "is-not?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/is-not?"},
		{"unsupported-set", "set!/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/set!"},
		{"unsupported-select-adjacent", "select-adjacent!/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/select-adjacent!"},
		{"unsupported-strip", "strip!/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/strip!"},
		{"unsupported-unknown", "unknown?/x:ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/unknown?"},
		{"left-eq-zero", "eq?/x:ZERO//a", "UTF-8", "TestPredicatePolicyFailClosed/eq?/ZERO"},
		{"left-eq-zero-or-one", "eq?/x:ZERO_OR_ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/eq?/ZERO_OR_ONE"},
		{"left-eq-zero-or-more", "eq?/x:ZERO_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/eq?/ZERO_OR_MORE"},
		{"left-eq-one-or-more", "eq?/x:ONE_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/eq?/ONE_OR_MORE"},
		{"left-not-eq-zero", "not-eq?/x:ZERO//a", "UTF-8", "TestPredicatePolicyFailClosed/not-eq?/ZERO"},
		{"left-not-eq-zero-or-one", "not-eq?/x:ZERO_OR_ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-eq?/ZERO_OR_ONE"},
		{"left-not-eq-zero-or-more", "not-eq?/x:ZERO_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-eq?/ZERO_OR_MORE"},
		{"left-not-eq-one-or-more", "not-eq?/x:ONE_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-eq?/ONE_OR_MORE"},
		{"left-any-of-zero", "any-of?/x:ZERO//a", "UTF-8", "TestPredicatePolicyFailClosed/any-of?/ZERO"},
		{"left-any-of-zero-or-one", "any-of?/x:ZERO_OR_ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-of?/ZERO_OR_ONE"},
		{"left-any-of-zero-or-more", "any-of?/x:ZERO_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-of?/ZERO_OR_MORE"},
		{"left-any-of-one-or-more", "any-of?/x:ONE_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/any-of?/ONE_OR_MORE"},
		{"left-not-any-of-zero", "not-any-of?/x:ZERO//a", "UTF-8", "TestPredicatePolicyFailClosed/not-any-of?/ZERO"},
		{"left-not-any-of-zero-or-one", "not-any-of?/x:ZERO_OR_ONE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-any-of?/ZERO_OR_ONE"},
		{"left-not-any-of-zero-or-more", "not-any-of?/x:ZERO_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-any-of?/ZERO_OR_MORE"},
		{"left-not-any-of-one-or-more", "not-any-of?/x:ONE_OR_MORE//a", "UTF-8", "TestPredicatePolicyFailClosed/not-any-of?/ONE_OR_MORE"},
		{"right-eq-zero-or-one", "eq?/x:ONE/y:ZERO_OR_ONE/", "UTF-8", "TestPredicatePolicyFailClosed/eq?/right-quantifier"},
		{"right-not-eq-zero-or-one", "not-eq?/x:ONE/y:ZERO_OR_ONE/", "UTF-8", "TestPredicatePolicyFailClosed/not-eq?/right-quantifier"},
		{"encoding-eq-utf16le", "eq?/x:ONE//a", "UTF-16LE", "TestPredicatePolicyFailClosed/eq?/UTF-16LE"},
		{"encoding-eq-utf16be", "eq?/x:ONE//a", "UTF-16BE", "TestPredicatePolicyFailClosed/eq?/UTF-16BE"},
		{"encoding-eq-cp949", "eq?/x:ONE//a", "CP949", "TestPredicatePolicyFailClosed/eq?/CP949"},
		{"encoding-not-eq-utf16le", "not-eq?/x:ONE//a", "UTF-16LE", "TestPredicatePolicyFailClosed/not-eq?/UTF-16LE"},
		{"encoding-not-eq-utf16be", "not-eq?/x:ONE//a", "UTF-16BE", "TestPredicatePolicyFailClosed/not-eq?/UTF-16BE"},
		{"encoding-not-eq-cp949", "not-eq?/x:ONE//a", "CP949", "TestPredicatePolicyFailClosed/not-eq?/CP949"},
		{"encoding-any-of-utf16le", "any-of?/x:ONE//a", "UTF-16LE", "TestPredicatePolicyFailClosed/any-of?/UTF-16LE"},
		{"encoding-any-of-utf16be", "any-of?/x:ONE//a", "UTF-16BE", "TestPredicatePolicyFailClosed/any-of?/UTF-16BE"},
		{"encoding-any-of-cp949", "any-of?/x:ONE//a", "CP949", "TestPredicatePolicyFailClosed/any-of?/CP949"},
		{"encoding-not-any-of-utf16le", "not-any-of?/x:ONE//a", "UTF-16LE", "TestPredicatePolicyFailClosed/not-any-of?/UTF-16LE"},
		{"encoding-not-any-of-utf16be", "not-any-of?/x:ONE//a", "UTF-16BE", "TestPredicatePolicyFailClosed/not-any-of?/UTF-16BE"},
		{"encoding-not-any-of-cp949", "not-any-of?/x:ONE//a", "CP949", "TestPredicatePolicyFailClosed/not-any-of?/CP949"},
		{"absent-left-capture", "eq?/absent:ONE//a", "UTF-8", "TestPredicatePolicyAbsentCapture"},
		{"absent-right-capture", "not-eq?/x:ONE/absent:ONE/", "UTF-8", "TestPredicatePolicyAbsentCapture"},
		{"partial-predicate", "eq?/x:ONE//a", "UTF-8", "TestPredicatePolicyIncompleteAndEmptyStreams/partial-predicate"},
		{"partial-structural", "", "UTF-8", "TestPredicatePolicyIncompleteAndEmptyStreams/partial-structural"},
		{"not-run", "", "UTF-8", "TestPredicatePolicyIncompleteAndEmptyStreams/not-run"},
	}
	controls := Roster()[20:]
	if len(controls) != len(want) {
		t.Fatal("control membership changed")
	}
	for i, row := range want {
		p := controls[i]
		input := ""
		if m := p.PolicyInput; m != nil {
			right := ""
			if m.RightCapture != "" {
				right = m.RightCapture + ":" + m.RightQuantifier
			}
			input = strings.Join([]string{m.Operator, m.LeftCapture + ":" + m.LeftQuantifier, right, strings.Join(m.Literals, ",")}, "/")
		}
		if p.ID != row.id || input != row.input || p.Encoding != row.encoding || p.Basis != "src/internal/native/query_policy_test.go#"+row.selector {
			t.Fatalf("control %d: %+v, want %+v", i, p, row)
		}
		if i < 42 {
			code := "QUERY_PREDICATE_UNSUPPORTED:" + strings.SplitN(row.input, "/", 2)[0]
			if i >= 30 {
				code = "QUERY_PREDICATE_ENCODING"
			}
			if p.Status != "UNSUPPORTED" || p.Code != code || p.Evaluation != "UNSUPPORTED" || p.Partial || p.Structural != "PRESERVED_4" || p.Expected != "NULL" {
				t.Fatal("control outcome changed", p)
			}
		}
	}
}

// Control expectations are fixed before results, so they must use the values the pinned host
// policy actually emits, not a registry-side spelling (#362 review M1).
func TestRosterControlsUseHostEvaluationValues(t *testing.T) {
	host := map[string]bool{"": true, native.EvalStructural: true, native.EvalEvaluated: true, native.EvalUnsupported: true}
	controls := 0
	for _, p := range Roster() {
		if p.Kind != Control {
			continue
		}
		controls++
		if !host[p.Evaluation] {
			t.Errorf("%s: evaluation %q is not a host policy value", p.ID, p.Evaluation)
		}
	}
	if controls == 0 {
		t.Fatal("no controls checked")
	}
}
