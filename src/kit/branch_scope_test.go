package kit

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// #218: a C# conditional's condition naming a symbol, anywhere in any recorded tree, or a
// tree without recorded nodes, puts the case's branch selection outside the claim; a
// literal condition, a symbol outside any condition and a tree the case does not have do not.
func TestBranchSelectionUnclaimed(t *testing.T) {
	cond := "condition"
	node := func(parent int64, typ string, condition bool) TreeNode {
		n := TreeNode{Parent: parent, Type: typ, Named: true}
		if condition {
			n.Field = &cond
		}
		return n
	}
	tree := func(nodes ...TreeNode) *rcTree { return &rcTree{Form: "full", Tree: &rcFullTree{Nodes: nodes}} }
	root := node(-1, "compilation_unit", false)
	symbolIf := tree(root, node(0, "preproc_if", false), node(1, "identifier", true))
	nestedElif := tree(root, node(0, "preproc_if", false), node(1, "boolean_literal", true), node(1, "preproc_elif", false),
		node(3, "binary_expression", true), node(4, "boolean_literal", false), node(4, "identifier", false))
	attribute := tree(root, node(0, "preproc_if_in_attribute_list", false), node(1, "unary_expression", true), node(2, "identifier", false))
	literal := tree(root, node(0, "preproc_if", false), node(1, "unary_expression", true), node(2, "boolean_literal", false),
		node(1, "class_declaration", false), node(4, "identifier", false))
	symbolOutside := tree(root, node(0, "class_declaration", false), node(1, "identifier", false))
	notCondition := tree(root, node(0, "preproc_if", false), node(1, "identifier", false)) // a body identifier, no condition field
	other := tree(root, node(0, "if_statement", false), node(1, "identifier", true))       // a condition field of another node
	summary := &rcTree{Form: "summary"}
	step := func(k int, inc, fresh *rcTree) rcStep { return rcStep{Step: k, Incremental: inc, Fresh: fresh} }
	for name, c := range map[string]struct {
		steps []rcStep
		want  bool
	}{
		"symbol-if":             {[]rcStep{step(0, symbolIf, nil)}, true},
		"symbol-in-nested-elif": {[]rcStep{step(0, nestedElif, nil)}, true},
		"attribute-list":        {[]rcStep{step(0, attribute, nil)}, true},
		"literal":               {[]rcStep{step(0, literal, nil)}, false},
		"symbol-outside":        {[]rcStep{step(0, symbolOutside, nil)}, false},
		"body-identifier":       {[]rcStep{step(0, notCondition, nil)}, false},
		"other-condition":       {[]rcStep{step(0, other, nil)}, false},
		"after-edit":            {[]rcStep{step(0, literal, nil), step(1, literal, symbolIf)}, true},
		"fresh-only":            {[]rcStep{step(0, literal, nil), step(1, symbolOutside, symbolIf)}, true},
		"no-fresh-at-step-0":    {[]rcStep{step(0, literal, nil)}, false},
		"summary-form":          {[]rcStep{step(0, summary, nil)}, true},
		"svc-inline-symbol": {[]rcStep{{Composite: &rcComposite{Inline: &struct {
			IncludedRanges []Span      `json:"included_ranges"`
			Tree           *rcFullTree `json:"tree"`
		}{Tree: symbolIf.Tree}}}}, true},
		"svc-without-inline": {[]rcStep{{Composite: &rcComposite{}}}, false},
	} {
		if got := branchSelectionUnclaimed(&rcCase{Steps: c.steps}); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

// #218: the mark is the set's list in set order and changes no verdict; a route outside the
// C# grammar lists nothing.
func TestQualifyBranchSelectionMark(t *testing.T) {
	marked := ""
	summaryCase := qfxAll(qfxMut{record: func(s string, recs []map[string]any) {
		if s == "s06-fxa" {
			marked = recs[1]["id"].(string)
			fxSummaryOut(recs[1]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any))
			recs[1]["oracle_claims"].(map[string]string)["api"] = claimBlocked
		}
	}})
	f := newQfx(t)
	plain := f.run(t, f.all(t, summaryCase))
	prev := branchSelectionRoutes
	branchSelectionRoutes = map[string]bool{"fxa": true}
	defer func() { branchSelectionRoutes = prev }()
	f = newQfx(t)
	r := f.run(t, f.all(t, summaryCase))
	for _, c := range r.Cells {
		want := []string{}
		if c.Route == "fxa" {
			want = []string{marked}
		}
		if !slices.Equal(c.Set.BranchSelectionNotClaimed, want) || c.Set.BranchSelectionNotClaimed == nil {
			t.Fatalf("%s %s: %v, want %v", c.Route, c.Platform, c.Set.BranchSelectionNotClaimed, want)
		}
	}
	// apart from the list the result is the unmarked one
	strip := func(q QualificationResult) []byte {
		for i := range q.Cells {
			q.Cells[i].Set.BranchSelectionNotClaimed = nil
		}
		b, _ := json.Marshal(struct {
			Cells   []QualCell
			Support string
			Claims  map[string]string
			Totals  QualTotals
		}{q.Cells, q.SupportClaim, q.PlatformClaims, q.Totals})
		return b
	}
	if !reflect.DeepEqual(strip(plain), strip(r)) {
		t.Fatal("the mark changed a verdict")
	}
}
