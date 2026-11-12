package kit

import "testing"

func TestOrdinaryRuntimeFoldKeepsPartialFirstFailure(t *testing.T) {
	reg := OrdinaryRegistration{Namespace: "test"}
	ids := []string{}
	for _, id := range []string{"a", "b", "c", "d"} {
		c := OrdinaryCoordinate{Kind: "STRUCTURAL_ONLY", Case: id}
		reg.Required = append(reg.Required, OrdinaryRequired{ID: c.ID(reg.Namespace), Coordinate: c})
		ids = append(ids, c.ID(reg.Namespace))
	}
	judgments := []ordinaryTupleJudgment{{State: ordinaryTupleVerified}, {State: ordinaryTupleFailed, Code: "FIRST_FAILURE"}, {State: ordinaryTupleBlocked, Code: "EXPECTED_MISSING"}}
	out, e := ordinaryTestBudget(t).foldOrdinaryRuntime(reg, ids, judgments)
	if e != nil || out.Assessment != AssessFail || out.Code != "FIRST_FAILURE" || out.Tuple != (QueryEligibilityCounts{Required: 4, Verified: 1, Failed: 1, Blocked: 1, Unprocessed: 1}) || out.ProcessingComplete || out.FullScopeComplete {
		t.Fatalf("partial fold lost a result: %+v %v", out, e)
	}
	// A later malformed result cannot erase the first confirmed failure.
	judgments = append(judgments, ordinaryTupleJudgment{State: "unknown"})
	out, e = ordinaryTestBudget(t).foldOrdinaryRuntime(reg, ids, judgments)
	if e == nil || out.Assessment != AssessFail || out.Code != "FIRST_FAILURE" || out.Tuple.Failed != 1 || out.Tuple.Unprocessed != 1 {
		t.Fatalf("later invalid state erased failure: %+v %v", out, e)
	}
	if _, e := ordinaryTestBudget(t).foldOrdinaryRuntime(reg, []string{ids[0], ids[0]}, nil); e == nil {
		t.Fatal("overlapping selection accepted")
	}
	if _, e := ordinaryTestBudget(t).foldOrdinaryRuntime(reg, []string{"borrowed-id"}, nil); e == nil {
		t.Fatal("borrowed coordinate accepted")
	}
}
