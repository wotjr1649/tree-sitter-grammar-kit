package kit

import "testing"

func TestOrdinaryReferenceUsesIndependentExpected(t *testing.T) {
	source := []byte("x")
	id := ordinaryTupleIdentity{Case: "case-a", Query: "q", TreeRole: "incremental", Source: OrdinaryPin{Path: "source-witness/x.utf8", Bytes: 1, SHA256: digestHex(source)}}
	metadata := QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{}, Predicates: []QueryPredicate{}}
	tree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: id.Case, TreeRole: id.TreeRole, Source: id.Source, Tree: []TreeNode{{Parent: -1, Type: "root", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}}}
	tree.Tree = append(tree.Tree, TreeNode{Parent: 0, Type: "x", Named: true, EndByte: 1, EndPoint: Point{Column: 1}})
	captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: id.Case, Query: id.Query, TreeRole: id.TreeRole, Source: id.Source, Ordered: []Capture{}}
	ref := &instrumentedReference{}
	ref.Query.Patterns, ref.Query.CaptureNames, ref.Query.Predicates = 1, []string{}, []QueryPredicate{}
	ref.Query.Structural, ref.Tree.Nodes = []Capture{}, tree.Tree
	judge := func() ordinaryTupleJudgment {
		t.Helper()
		got, e := ordinaryTestBudget(t).ordinaryReferenceExpected(id, ref, ordinaryJSON(t, metadata), ordinaryJSON(t, tree), ordinaryJSON(t, captures))
		if e != nil {
			t.Fatal(e)
		}
		return got
	}
	if got := judge(); got.State != ordinaryTupleVerified {
		t.Fatalf("independent empty stream: %+v", got)
	}
	ref.Query.Patterns = 2
	field := "different"
	ref.Tree.Nodes = append([]TreeNode(nil), tree.Tree...)
	ref.Tree.Nodes[1].Field = &field
	if got := judge(); got.State != ordinaryTupleFailed || got.Code != "ORDINARY_REFERENCE_METADATA" || len(got.Findings) != 2 {
		t.Fatalf("CST difference hid independent metadata failure: %+v", got)
	}
	ref.Query.Patterns = 1
	if got := judge(); got.State != ordinaryTupleBlocked || got.Code != "REFERENCE_CST_INELIGIBLE" {
		t.Fatalf("field-only divergence: %+v", got)
	}
}
