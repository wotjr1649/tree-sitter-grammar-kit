package nativewire

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func TestQueryNodeLinkFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*WireQuery)
	}{
		{"type", func(q *WireQuery) { q.Types[0] = "other" }},
		{"named", func(q *WireQuery) { q.Captures[0][5] ^= 1 }},
		{"extra", func(q *WireQuery) { q.Captures[0][5] ^= 2 }},
		{"error", func(q *WireQuery) { q.Captures[0][5] ^= 4 }},
		{"has-error", func(q *WireQuery) { q.Captures[0][5] ^= 8 }},
		{"missing", func(q *WireQuery) { q.Captures[0][5] ^= 16 | 8 }}, // a MISSING node always has an error
		{"start-byte", func(q *WireQuery) { q.Captures[0][6] = 1 }},
		{"end-byte", func(q *WireQuery) { q.Captures[0][7] = 0 }},
		{"start-row", func(q *WireQuery) { q.Captures[0][8] = 1 }},
		{"start-column", func(q *WireQuery) { q.Captures[0][9] = 1 }},
		{"end-row", func(q *WireQuery) { q.Captures[0][10] = 1 }},
		{"end-column", func(q *WireQuery) { q.Captures[0][11] = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := WireQuery{ID: "q", Status: StatusCompleted, Patterns: 1, Matches: 1,
				CaptureNames: []string{"x"}, Types: []string{"identifier"}, Predicates: []querypolicy.Predicate{},
				Captures: [][]int64{{0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}}}
			raw, err := jsonv2.Marshal([]WireQuery{q})
			if err != nil {
				t.Fatal(err)
			}
			tree := Tree{Wire: WireTree{DescendantCount: 1, Queries: raw},
				Nodes: []TreeNode{{Type: "identifier", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}}}
			req := []querywire.Source{{ID: "q", Source: []byte("(identifier) @x")}}
			if err := tree.CheckQueries(req, 1, nil); err != nil || len(tree.Queries) != 1 {
				t.Fatalf("normal link failed: %+v %v", tree.Queries, err)
			}
			tc.edit(&q)
			raw, err = jsonv2.Marshal([]WireQuery{q})
			if err != nil {
				t.Fatal(err)
			}
			tree.Wire.Queries = raw
			// The mutation stays valid wire; only the full CST link rejects it.
			summary := tree
			summary.Nodes = nil
			if err := summary.CheckQueries(req, 1, nil); err != nil {
				t.Fatalf("mutation is not valid wire: %v", err)
			}
			if err := tree.CheckQueries(req, 1, nil); err == nil || err.(*Error).Code != "CAPTURE_LINK_MISMATCH" || tree.Queries != nil {
				t.Fatalf("changed full-node field adopted: %+v %v", tree.Queries, err)
			}
		})
	}
}

func TestQueryNodeLinkCancellation(t *testing.T) {
	q := WireQuery{ID: "q", Status: StatusCompleted, Patterns: 1, Matches: 1, CaptureNames: []string{"x"}, Types: []string{"identifier"}, Predicates: []querypolicy.Predicate{}, Captures: [][]int64{{0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}, {0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}}}
	raw, err := jsonv2.Marshal([]WireQuery{q})
	if err != nil {
		t.Fatal(err)
	}
	tree := Tree{Wire: WireTree{DescendantCount: 1, Queries: raw}, Nodes: []TreeNode{{Type: "identifier", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}}}
	stopped := errors.New("cancelled during capture link")
	calls := 0
	err = tree.CheckQueries([]querywire.Source{{ID: "q", Source: []byte("(identifier) @x")}}, 1, func() error {
		calls++
		if calls == 2 {
			return stopped
		}
		return nil
	})
	if err != stopped || calls != 2 || tree.Queries != nil {
		t.Fatalf("cancel swallowed: err%v calls%d queries%v", err, calls, tree.Queries)
	}
}
