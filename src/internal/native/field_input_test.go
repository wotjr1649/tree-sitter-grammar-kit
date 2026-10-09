package native

import (
	"os"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestWireFieldInput(t *testing.T) {
	root := []int64{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}
	child := []int64{0, 0, 0, 1, 0, 1, 0, 0, 0, 1}
	if n, err := Nodes([][]int64{root, child}, []string{"node"}, []string{"name"}); err != nil || n[0].Field != nil || n[1].Field == nil || *n[1].Field != "name" {
		t.Fatalf("valid fields: %+v, %v", n, err)
	}
	if _, err := Nodes([][]int64{root, child}, []string{"node"}, []string{""}); err == nil {
		t.Fatal("empty field accepted")
	}
	root[2] = 0
	if _, err := Nodes([][]int64{root}, []string{"node"}, []string{"name"}); err == nil {
		t.Fatal("root field accepted")
	}
}

func TestTruncatedPartialInput(t *testing.T) {
	form := "summary"
	w := WireTree{Status: kit.StatusCompleted, Form: &form, Digest: strings.Repeat("0", 64), DescendantCount: 2,
		Types: []string{"node"}, Fields: []string{"name"}, Errors: &WireErrors{Limit: 1},
		PartialTrees: []WirePartial{{Truncated: true, Nodes: [][]int64{{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}, {0, 0, 0, 1, 0, 1, 0, 0, 0, 1}}}}}
	if _, err := checkTree(w, 1, 2, Request{Protocol: Protocol}); err != nil {
		t.Fatalf("valid truncated prefix: %v", err)
	}
	for _, mutate := range []func(){
		func() { w.PartialTrees[0].Nodes[0][0] = 0 },
		func() { w.PartialTrees[0].Nodes[0][2] = 0 },
		func() { w.PartialTrees[0].Nodes[1][5] = 2 },
	} {
		w.PartialTrees[0].Nodes = [][]int64{{-1, 0, -1, 1, 0, 1, 0, 0, 0, 1}, {0, 0, 0, 1, 0, 1, 0, 0, 0, 1}}
		mutate()
		if _, err := checkTree(w, 1, 2, Request{Protocol: Protocol}); err == nil {
			t.Fatal("malformed truncated prefix accepted")
		}
	}
}

func TestDynamicExpectationRequiresQuery(t *testing.T) {
	data, err := os.ReadFile("../../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := kit.ParseFactPack(data)
	if err != nil {
		t.Fatal(err)
	}
	var route kit.FactPackRoute
	for _, r := range pack.Routes {
		if r.Route == "tsql" {
			route = r
		}
	}
	p := kit.OracleProfile{FactPack: &kit.FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: route.Route},
		Native: kit.IncrementalProfile{Declarations: &kit.Declarations{Items: route.Declarations}}}
	var dynamic kit.OracleQuery
	for _, q := range route.Queries {
		if q.Facts == kit.FactDeclarations {
			p.Queries = append(p.Queries, kit.OracleQuery{ID: q.ID, Source: q.Source})
		}
		if q.Facts == kit.FactDynamicSQL {
			dynamic = kit.OracleQuery{ID: q.ID, Source: q.Source}
		}
	}
	if _, err := checkFactPack(p, data); err != nil {
		t.Fatalf("declarations-only control: %v", err)
	}
	p.OracleCases = []kit.OracleCase{{DynamicSQL: &kit.DynamicSQLExpectation{}}}
	if _, err := checkFactPack(p, data); err == nil || err.Code != "FACT_PACK_MISMATCH" {
		t.Fatalf("unchecked expectation: %v", err)
	}
	p.Queries = append(p.Queries, dynamic)
	if _, err := checkFactPack(p, data); err != nil {
		t.Fatalf("dynamic query selected: %v", err)
	}
}

func TestDynamicCheckedFailurePreservesClaim(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		queries := []kit.FactQuery{{ID: "mismatch", Facts: kit.FactDynamicSQL}, {ID: "invalid", Facts: kit.FactDynamicSQL}}
		if reverse {
			queries[0], queries[1] = queries[1], queries[0]
		}
		queries = append(queries, kit.FactQuery{ID: "unfinished", Facts: kit.FactDeclarations})
		pack := kit.FactPack{Routes: []kit.FactPackRoute{{Route: "tsql", Queries: queries}}}
		profile := kit.OracleProfile{FactPack: &kit.FactPackRef{Route: "tsql"}, Queries: []kit.OracleQuery{{ID: "mismatch"}, {ID: "invalid"}, {ID: "unfinished"}}}
		c := CaseResult{ExecutionStatus: kit.StatusCompleted, Oracle: &OracleClaims{DynamicSQL: ClaimNotClaimed},
			Steps: []StepResult{{Incremental: &TreeOut{Queries: []QueryOut{{ID: "mismatch", Status: kit.StatusCompleted},
				{ID: "invalid", Status: kit.StatusCompleted, Captures: []kit.Capture{{EndByte: 2}}}}}}}}
		oc := kit.OracleCase{IncrementalCase: kit.IncrementalCase{Encoding: kit.EncodingUTF8},
			DynamicSQL: &kit.DynamicSQLExpectation{Facts: []kit.DynamicSQLFact{{Construct: "EXEC_PAREN"}}}}
		judgeOracle(&c, oc, profile, &pack, Context{}, []byte("x"))
		if c.Oracle.DynamicSQL != ClaimFail || c.Facts == nil || c.Facts.Difference == "" {
			t.Fatalf("downgraded failure: %+v", c)
		}
		if reverse != strings.Contains(c.Facts.Difference, "CAPTURE_RANGE_INVALID") {
			t.Fatalf("first difference overwritten: %s", c.Facts.Difference)
		}
	}
}
