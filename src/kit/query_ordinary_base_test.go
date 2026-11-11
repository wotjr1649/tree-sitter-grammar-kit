package kit

import (
	"encoding/json"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func TestOrdinaryBaseRecomputesStoredQueryClaim(t *testing.T) {
	b := semanticTestBudget(t)
	source := []byte("x")
	input := NativeInput{Role: "case", Path: "source.txt", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	nodes := []TreeNode{{Parent: -1, Type: "root", Named: true, EndByte: 1, EndPoint: Point{Column: 1}}}
	query := querywire.Query{ID: "q", Status: StatusCompleted, Patterns: 1, CaptureNames: []string{"x"}, Predicates: []querypolicy.Predicate{}, Types: []string{"identifier"}, Captures: [][]int64{}}
	caseProfile := IncrementalCase{ID: "c1", Input: input, Encoding: EncodingUTF8, Edits: []Edit{}, Points: []NativePoint{}, Expect: []StepExpectation{}}
	profile := OracleProfile{
		Native:      IncrementalProfile{Schema: OracleSemanticsSchema, ID: "ordinary-base-test", Route: "test-route", Operation: "native-query-batch", Encoding: EncodingUTF8, Output: OutputTree, Cases: []IncrementalCase{caseProfile}},
		Queries:     []OracleQuery{{ID: "q", Source: "(root) @root"}},
		OracleCases: []OracleCase{{IncrementalCase: caseProfile, QueryExpect: []QueryExpectation{{Query: "q", Step: 0, Status: StatusCompleted, Code: "", Captures: nil}}}},
	}
	projection := rcCase{
		Schema: OracleBatchRecordSchema, ID: "c1", Input: input, ExecutionStatus: StatusCompleted, Assessment: AssessPass,
		Claims: rcClaims{IncrementalEquality: claimNotClaimed, IncrementalRoute: claimNotClaimed, Expectations: claimNotClaimed},
		Oracle: &struct {
			QueryEquality     string `json:"query_equality"`
			QueryExpectations string `json:"query_expectations"`
			API               string `json:"api"`
			FactReproduction  string `json:"fact_reproduction"`
			DynamicSQL        string `json:"dynamic_sql"`
		}{QueryEquality: claimNotClaimed, QueryExpectations: claimFail, API: claimNotClaimed, FactReproduction: claimNotClaimed, DynamicSQL: claimNotClaimed},
		Steps:        []rcStep{{Step: 0, SourceBytes: uint64(len(source)), SourceSHA256: input.SHA256, Incremental: &rcTree{Status: StatusCompleted, Form: "full", DescendantCount: uint64(len(nodes)), Digest: TreeDigest(nodes), Tree: &rcFullTree{Schema: TreeSchema, Input: rcInput{Encoding: EncodingUTF8, EncodingSource: SourceDeclaration, Bytes: uint64(len(source)), SHA256: input.SHA256}, Status: StatusCompleted, Capabilities: map[string]string{}, Nodes: nodes, Identities: []IdentityRef{}}}}},
		Expectations: []rcExpect{},
	}
	projectionBytes, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	var projectionWire map[string]any
	if err := json.Unmarshal(projectionBytes, &projectionWire); err != nil {
		t.Fatal(err)
	}
	projectionWire["encoding"] = EncodingUTF8
	projectionWire["assessment"] = AssessFail
	projectionWire["producer"] = nativewire.Producer{}
	projectionWire["response_status"] = StatusCompleted
	projectionWire["response_code"] = ""
	projectionWire["query_expectations"] = []any{map[string]any{"query": "q", "step": 0, "status": StatusCompleted, "code": "", "captures": nil, "error": nil, "result": claimPass}}
	step := projectionWire["steps"].([]any)[0].(map[string]any)
	incremental := step["incremental"].(map[string]any)
	incremental["queries"] = []any{map[string]any{"id": "q", "sha256": digestHex([]byte(profile.Queries[0].Source)), "status": StatusCompleted, "code": "", "evaluation": "STRUCTURAL", "query_ms": 0, "error": nil, "patterns": 1, "capture_names": []string{"x"}, "predicates": []any{}, "matches": 0, "partial": false, "captures": []any{}}}
	projectionBytes, err = json.Marshal(projectionWire)
	if err != nil {
		t.Fatal(err)
	}
	cache := &ordinaryCaseCache{Case: "c1", projection: projectionBytes, raw: nativewire.Response{Status: StatusCompleted}, ResponseFrame: []byte{0, 0, 0, 0},
		checked:  []nativewire.CheckedStep{{Incremental: nativewire.Tree{Status: StatusCompleted, Form: "full", Nodes: nodes, Queries: []querywire.Query{query}, Wire: nativewire.WireTree{DescendantCount: uint64(len(nodes)), Digest: TreeDigest(nodes)}}}},
		versions: [][]byte{source}, ready: true}
	got, runErr := b.recomputeOrdinaryBase(profile, 0, cache)
	if runErr != nil {
		t.Fatal(runErr)
	}
	for _, gate := range got.Gates {
		if gate.ID == "query-expectations" && gate.Failed > 0 && got.Verdict.Assessment == AssessFail {
			return
		}
	}
	t.Fatalf("stored FAIL claim was not checked against raw recomputation: verdict=%+v gates=%+v findings=%+v", got.Verdict, got.Gates, got.Findings)
}

func TestOrdinaryBaseKeepsStoredFailWhenRecomputationStops(t *testing.T) {
	b := semanticTestBudget(t)
	source := []byte("x")
	input := NativeInput{Role: "case", Path: "source.txt", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	profile := OracleProfile{Native: IncrementalProfile{Cases: []IncrementalCase{{ID: "c1"}}}, OracleCases: []OracleCase{{IncrementalCase: IncrementalCase{ID: "c1"}}}}
	projection, err := json.Marshal(ordinaryProjectionCase{ID: "c1", Input: input, Encoding: EncodingUTF8, ExecutionStatus: StatusFailed, Assessment: AssessFail, Code: "SAVED_FAILURE"})
	if err != nil {
		t.Fatal(err)
	}
	cache := &ordinaryCaseCache{Case: "c1", projection: projection, recordedBase: Verdict{StatusFailed, AssessFail}, recordedBaseCode: "SAVED_FAILURE"}
	caller := b.r.caller
	b.r.caller = &countingCtx{Context: caller}
	b.r.caller.(*countingCtx).left.Store(0)
	got, runErr := b.recomputeOrdinaryBase(profile, 0, cache)
	if runErr == nil || got.Verdict.Assessment != AssessFail || got.Verdict.ExecutionStatus != StatusFailed {
		t.Fatalf("partial recomputation erased saved FAIL: result=%+v err=%v", got, runErr)
	}
}
