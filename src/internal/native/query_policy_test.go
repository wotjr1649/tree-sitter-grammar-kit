package native

import (
	jsonv2 "encoding/json/v2"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func policyCapture(name, quantifier string) PredStep {
	return PredStep{Kind: "capture", Value: name, Quantifier: &quantifier}
}

func policyString(value string) PredStep { return PredStep{Kind: "string", Value: value} }

func policyPredicate(op string, args ...PredStep) WirePredicate {
	return WirePredicate{Pattern: 0, Steps: append([]PredStep{policyString(op)}, args...)}
}

// Owned host-policy inputs, not a claim that a native CLI produced this stream.
func policyWire(predicates ...WirePredicate) WireQuery {
	if predicates == nil {
		predicates = []WirePredicate{}
	}
	return WireQuery{ID: "q", Status: kit.StatusCompleted, Patterns: 1, Matches: 2,
		CaptureNames: []string{"x", "y"}, Types: []string{"identifier"}, Predicates: predicates,
		Captures: [][]int64{
			{0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}, {0, 0, 1, 1, 0, 1, 2, 3, 0, 2, 0, 3},
			{1, 0, 0, 2, 0, 1, 4, 5, 0, 4, 0, 5}, {1, 0, 1, 3, 0, 1, 6, 7, 0, 6, 0, 7},
		},
	}
}

func policyOut(t *testing.T, q WireQuery, encoding string) QueryOut {
	t.Helper()
	source := []byte("a b b b")
	queries := []QuerySource{{ID: "q", Source: []byte("owned host-policy fixture")}}
	raw, err := jsonv2.Marshal([]WireQuery{q})
	if err != nil {
		t.Fatal(err)
	}
	// the summary-form wire validation checkTree applies to a tree's queries
	decoded, werr := querywire.DecodeSummary(raw, queries, uint64(len(source)), 4)
	if werr != nil {
		t.Fatalf("invalid owned wire fixture: %v", werr)
	}
	tree := Tree{Status: kit.StatusCompleted, Wire: WireTree{DescendantCount: 4, Queries: raw}, Queries: decoded}
	return queryOuts(tree, queries, encoding, source)[0]
}

func TestPredicatePolicyEvaluatedStreams(t *testing.T) {
	x, y := policyCapture("x", "ONE"), policyCapture("y", "ONE")
	pred := policyPredicate
	for _, tc := range []struct {
		name       string
		predicates []WirePredicate
		keep       []int
	}{
		{"eq-string", []WirePredicate{pred("eq?", x, policyString("a"))}, []int{0, 1}},
		{"eq-string-none", []WirePredicate{pred("eq?", x, policyString("c"))}, []int{}},
		{"not-eq-string", []WirePredicate{pred("not-eq?", x, policyString("a"))}, []int{2, 3}},
		{"not-eq-string-all", []WirePredicate{pred("not-eq?", x, policyString("c"))}, []int{0, 1, 2, 3}},
		{"eq-capture", []WirePredicate{pred("eq?", x, y)}, []int{2, 3}},
		{"not-eq-capture", []WirePredicate{pred("not-eq?", x, y)}, []int{0, 1}},
		{"any-of", []WirePredicate{pred("any-of?", x, policyString("a"), policyString("c"))}, []int{0, 1}},
		{"any-of-all", []WirePredicate{pred("any-of?", x, policyString("b"), policyString("a"))}, []int{0, 1, 2, 3}},
		{"any-of-none", []WirePredicate{pred("any-of?", x, policyString("c"))}, []int{}},
		{"not-any-of", []WirePredicate{pred("not-any-of?", x, policyString("a"), policyString("c"))}, []int{2, 3}},
		{"not-any-of-none", []WirePredicate{pred("not-any-of?", x, policyString("a"), policyString("b"))}, []int{}},
		{"and", []WirePredicate{pred("eq?", x, policyString("a")), pred("not-eq?", x, y)}, []int{0, 1}},
		{"and-reject", []WirePredicate{pred("eq?", x, policyString("a")), pred("eq?", x, policyString("b"))}, []int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := policyWire(tc.predicates...)
			out := policyOut(t, q, kit.EncodingUTF8)
			want := []kit.Capture{}
			for _, index := range tc.keep {
				want = append(want, captureOf(q, q.Captures[index]))
			}
			if out.Status != kit.StatusCompleted || out.Code != "" || out.Evaluation != EvalEvaluated || out.Captures == nil || kit.CompareCaptures(out.Captures, want) != nil {
				t.Fatalf("predicate stream: %+v expected %+v", out, want)
			}
			if len(out.structural) != 4 {
				t.Fatal("host evaluation discarded structural evidence")
			}
		})
	}
}

func TestPredicatePolicyPatternIsolation(t *testing.T) {
	q := policyWire(policyPredicate("eq?", policyCapture("x", "ONE"), policyString("c")))
	q.Patterns = 2
	q.Captures[2][1], q.Captures[3][1] = 1, 1
	out := policyOut(t, q, kit.EncodingUTF8)
	want := []kit.Capture{captureOf(q, q.Captures[2]), captureOf(q, q.Captures[3])}
	if out.Evaluation != EvalEvaluated || kit.CompareCaptures(out.Captures, want) != nil {
		t.Fatalf("pattern predicate leaked: %+v", out)
	}
}

func TestPredicatePolicyFailClosed(t *testing.T) {
	x := policyCapture("x", "ONE")
	for _, operator := range []string{"match?", "not-match?", "any-eq?", "any-not-eq?", "any-match?", "any-not-match?", "is?", "is-not?", "set!", "select-adjacent!", "strip!", "unknown?"} {
		t.Run(operator, func(t *testing.T) {
			out := policyOut(t, policyWire(policyPredicate(operator, x, policyString("a"))), kit.EncodingUTF8)
			if out.Status != EvalUnsupported || out.Code != "QUERY_PREDICATE_UNSUPPORTED:"+operator || out.Evaluation != EvalUnsupported || out.Captures != nil || len(out.structural) != 4 {
				t.Fatalf("unsupported operator adopted: %+v", out)
			}
		})
	}
	for _, op := range []string{"eq?", "not-eq?", "any-of?", "not-any-of?"} {
		for _, quantifier := range []string{"ZERO", "ZERO_OR_ONE", "ZERO_OR_MORE", "ONE_OR_MORE"} {
			t.Run(op+"/"+quantifier, func(t *testing.T) {
				out := policyOut(t, policyWire(policyPredicate(op, policyCapture("x", quantifier), policyString("a"))), kit.EncodingUTF8)
				if out.Status != EvalUnsupported || out.Evaluation != EvalUnsupported || out.Captures != nil || out.Code != "QUERY_PREDICATE_UNSUPPORTED:"+op {
					t.Fatalf("non-ONE evaluated: %+v", out)
				}
			})
		}
		for _, encoding := range []string{kit.EncodingUTF16LE, kit.EncodingUTF16BE, kit.EncodingCP949} {
			t.Run(op+"/"+encoding, func(t *testing.T) {
				out := policyOut(t, policyWire(policyPredicate(op, x, policyString("a"))), encoding)
				if out.Status != EvalUnsupported || out.Code != "QUERY_PREDICATE_ENCODING" || out.Evaluation != EvalUnsupported || out.Captures != nil {
					t.Fatalf("encoding evaluated: %+v", out)
				}
			})
		}
	}
	for _, op := range []string{"eq?", "not-eq?"} {
		t.Run(op+"/right-quantifier", func(t *testing.T) {
			out := policyOut(t, policyWire(policyPredicate(op, x, policyCapture("y", "ZERO_OR_ONE"))), kit.EncodingUTF8)
			if out.Status != EvalUnsupported || out.Captures != nil {
				t.Fatalf("right quantified capture evaluated: %+v", out)
			}
		})
	}
}

func TestPredicatePolicyAbsentCapture(t *testing.T) {
	for _, predicate := range []WirePredicate{
		policyPredicate("eq?", policyCapture("absent", "ONE"), policyString("a")),
		policyPredicate("not-eq?", policyCapture("x", "ONE"), policyCapture("absent", "ONE")),
	} {
		// a compiled query lists every predicate capture, though a match may lack it
		q := policyWire(predicate)
		q.CaptureNames = append(q.CaptureNames, "absent")
		out := policyOut(t, q, kit.EncodingUTF8)
		if out.Status != kit.StatusFailed || out.Code != "QUERY_PREDICATE_CAPTURE_ABSENT" || out.Evaluation != EvalUnsupported || out.Captures != nil {
			t.Fatalf("absent capture adopted: %+v", out)
		}
	}
}

func TestPredicatePolicyIncompleteAndEmptyStreams(t *testing.T) {
	predicate := policyPredicate("eq?", policyCapture("x", "ONE"), policyString("a"))
	t.Run("partial-predicate", func(t *testing.T) {
		q := policyWire(predicate)
		q.Status, q.Code, q.Partial = kit.StatusResourceLimit, "CAPTURE_LIMIT", true
		out := policyOut(t, q, kit.EncodingUTF8)
		if out.Status != kit.StatusResourceLimit || out.Code != q.Code || !out.Partial || out.Evaluation != EvalUnsupported || out.Captures != nil || len(out.structural) != 4 {
			t.Fatalf("partial predicates evaluated: %+v", out)
		}
	})
	t.Run("partial-structural", func(t *testing.T) {
		q := policyWire()
		q.Status, q.Code, q.Partial = kit.StatusResourceLimit, "CAPTURE_LIMIT", true
		out := policyOut(t, q, kit.EncodingUTF8)
		if out.Evaluation != EvalStructural || !out.Partial || len(out.Captures) != 4 || !slices.Equal(out.Captures, out.structural) {
			t.Fatalf("partial structural evidence lost: %+v", out)
		}
	})
	t.Run("evaluated-empty", func(t *testing.T) {
		q := policyWire(predicate)
		q.Matches, q.Types, q.Captures = 0, []string{}, [][]int64{}
		out := policyOut(t, q, kit.EncodingUTF8)
		if out.Status != kit.StatusCompleted || out.Evaluation != EvalEvaluated || out.Captures == nil || len(out.Captures) != 0 {
			t.Fatalf("completed empty confused with absent: %+v", out)
		}
	})
	t.Run("not-run", func(t *testing.T) {
		out := queryOuts(Tree{}, []QuerySource{{ID: "q"}}, kit.EncodingUTF8, nil)[0]
		if out.Status != kit.StatusNotRun || out.Code != "TREE_NOT_COMPLETED" || out.Evaluation != "" || out.Captures != nil {
			t.Fatalf("unexecuted query adopted: %+v", out)
		}
	})
}

func TestPredicateWireJSON(t *testing.T) {
	got, err := jsonv2.Marshal(policyPredicate("eq?", policyCapture("x", "ONE"), policyString("a")))
	want := `{"pattern":0,"steps":[{"kind":"string","value":"eq?","quantifier":null},{"kind":"capture","value":"x","quantifier":"ONE"},{"kind":"string","value":"a","quantifier":null}]}`
	if err != nil || string(got) != want {
		t.Fatalf("legacy predicate wire changed: %s %v", got, err)
	}
}
