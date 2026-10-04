package native

import (
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// #76 P1: evaluate passes an anchor only on a named node of exactly its type and byte range.
// The tree is "call(x); call(y);": two calls, an identifier on the first call's start and an
// unnamed "call" token, so a same-typed node elsewhere or another type on the range would
// pass a check that is not exact. A summary tree blocks the anchor.
func TestEvaluateAnchors(t *testing.T) {
	nodes := []kit.TreeNode{
		{Parent: -1, Type: "program", Named: true, EndByte: 17},
		{Parent: 0, Type: "call", Named: true, StartByte: 0, EndByte: 7},
		{Parent: 1, Type: "identifier", Named: true, StartByte: 0, EndByte: 4},
		{Parent: 0, Type: "call", Named: true, StartByte: 9, EndByte: 16},
		{Parent: 3, Type: "call", Named: false, StartByte: 9, EndByte: 13},
	}
	steps := func(form string) []CheckedStep {
		tr := Tree{Status: kit.StatusCompleted, Form: form}
		if form == "full" {
			tr.Nodes = nodes
		}
		return []CheckedStep{{Incremental: tr}}
	}
	at := func(as ...kit.ExpectAnchor) []kit.StepExpectation {
		return []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"call"}, Anchors: as}}
	}
	for name, tc := range map[string]struct {
		expect []kit.StepExpectation
		form   string
		want   string
	}{
		"exact":            {at(kit.ExpectAnchor{Type: "call", StartByte: 9, EndByte: 16}), "full", ClaimPass},
		"both":             {at(kit.ExpectAnchor{Type: "call", EndByte: 7}, kit.ExpectAnchor{Type: "call", StartByte: 9, EndByte: 16}), "full", ClaimPass},
		"same-type-other":  {at(kit.ExpectAnchor{Type: "call", StartByte: 8, EndByte: 16}), "full", ClaimFail},
		"other-type-range": {at(kit.ExpectAnchor{Type: "identifier", StartByte: 9, EndByte: 16}), "full", ClaimFail},
		"unnamed-node":     {at(kit.ExpectAnchor{Type: "call", StartByte: 9, EndByte: 13}), "full", ClaimFail},
		"summary":          {[]kit.StepExpectation{{Step: 0, Syntax: "ANY", Anchors: []kit.ExpectAnchor{{Type: "call", StartByte: 9, EndByte: 16}}}}, "summary", ClaimBlocked},
		"no-anchors":       {at(), "full", ClaimPass},
	} {
		res, claim := evaluate(tc.expect, steps(tc.form))
		if claim != tc.want || len(res) != 1 || res[0].Result != tc.want {
			t.Errorf("%s: claim %s results %+v, want %s", name, claim, res, tc.want)
		}
		if !slices.Equal(res[0].Anchors, tc.expect[0].Anchors) {
			t.Errorf("%s: result does not echo the anchors: %+v", name, res[0])
		}
	}
}
