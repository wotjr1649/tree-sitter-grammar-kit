package kit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func semanticExpectationControl() (OracleProfile, *semanticNativeCache) {
	profile := OracleProfile{Queries: []OracleQuery{{ID: "q", Source: "(number) @x"}}, OracleCases: []OracleCase{{QueryExpect: []QueryExpectation{{Query: "q", Step: 0, Status: StatusFailed, Code: querypolicy.AbsentCapture}}}}}
	cache := &semanticNativeCache{checked: []nativewire.CheckedStep{{Incremental: nativewire.Tree{Queries: []querywire.Query{{ID: "q", Status: StatusCompleted}}}}}, tuples: []QueryWitnessTuple{{Query: "q", Step: 0, TreeRole: "incremental"}}, values: []witnessEvaluation{{status: StatusFailed, code: querypolicy.AbsentCapture, evaluation: "UNSUPPORTED"}}, evaluated: []bool{true}}
	return profile, cache
}

func TestSemanticQueryExpectationUsesEvaluatedStatus(t *testing.T) {
	profile, cache := semanticExpectationControl()
	claim, computed, e := semanticTestBudget(t).semanticQueryExpectations(profile, cache)
	if e != nil || !computed || claim != claimPass {
		t.Fatalf("raw COMPLETED replaced evaluated FAILED: %s %v", claim, e)
	}
	cache.evaluated[0] = false
	claim, computed, e = semanticTestBudget(t).semanticQueryExpectations(profile, cache)
	if e != nil || computed || claim != claimBlocked {
		t.Fatalf("missing evaluation became a mismatch: %s %v", claim, e)
	}
}

func TestSemanticMissingEvaluationRemainsBlockedThroughBaseGates(t *testing.T) {
	for _, stored := range []string{claimPass, claimNotClaimed} {
		t.Run(stored, func(t *testing.T) {
			f := mandatoryFixture(t, false, false, claimPass, AssessPass)
			native, err := ParseIncrementalProfile(mustJSON(t, f.profile))
			if err != nil {
				t.Fatal(err)
			}
			native.Cases = native.Cases[:1]
			c := f.cases[0]
			c["oracle_claims"] = map[string]string{"query_equality": claimNotClaimed, "query_expectations": stored, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}
			profile, cache := semanticExpectationControl()
			profile.Native, cache.ready = native, true
			cache.evaluated[0] = false
			res := QuerySemanticsResult{Report: newReport("query semantics")}
			res.Assessment = AssessBlocked
			add := func(code, _, detail string) {
				t.Errorf("missing evaluation became a confirmed failure: %s %s", code, detail)
			}
			if e := semanticTestBudget(t).semanticBase(profile, mustJSON(t, c), cache, &res, add); e != nil {
				t.Fatal(e)
			}
			if res.BaseVerified.Assessment != AssessBlocked {
				t.Fatalf("missing evaluation gate: %+v", res)
			}
			for _, gate := range res.Gates {
				if gate.Failed != 0 {
					t.Fatalf("missing evaluation became gate failure: %+v", gate)
				}
			}

		})
	}
}

func TestSemanticQueryExpectationFailureSurvivesCancellation(t *testing.T) {
	profile, cache := semanticExpectationControl()
	profile.OracleCases[0].QueryExpect[0].Status, profile.OracleCases[0].QueryExpect[0].Code = StatusCompleted, ""
	profile.OracleCases[0].QueryExpect = append(profile.OracleCases[0].QueryExpect, QueryExpectation{Query: "q", Step: 1, Status: StatusCompleted})
	b := semanticTestBudget(t)
	caller := &countingCtx{Context: b.r.caller}
	// The significant-byte scan and base reservation precede the two expectation checks.
	caller.left.Store(3)
	b.r.caller = caller
	data, err := json.Marshal(map[string]any{"id": "c1", "code": "ORACLE_CLAIM_FAILED", "assessment": AssessFail, "oracle_claims": map[string]string{"query_equality": claimPass, "query_expectations": claimFail, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}})
	if err != nil {
		t.Fatal(err)
	}
	res := QuerySemanticsResult{Report: newReport("query semantics")}
	res.Assessment = AssessBlocked
	add := func(code, _, _ string) {
		res.Assessment = AssessFail
		if res.Code == "" {
			res.Code = code
		}
	}
	e := b.semanticBase(profile, data, cache, &res, add)
	if e == nil || e.Kind != KindCancelled || !errors.Is(e, context.Canceled) || res.Assessment != AssessFail || res.BaseVerified.Assessment != AssessFail || res.Code != "ORACLE_CLAIM_FAILED" {
		t.Fatalf("partial FAIL/code was erased by cancellation: %v %+v", e, res)
	}
}

func TestSemanticBaseNormalizesInvalidProducerStatus(t *testing.T) {
	f := mandatoryFixture(t, false, false, claimPass, AssessPass)
	native, err := ParseIncrementalProfile(mustJSON(t, f.profile))
	if err != nil {
		t.Fatal(err)
	}
	native.Cases = native.Cases[:1]
	c := f.cases[0]
	c["execution_status"] = "FORGED"
	profile := OracleProfile{Native: native, OracleCases: []OracleCase{{}}}
	res := QuerySemanticsResult{Report: newReport("query semantics")}
	res.Assessment, res.Code = AssessFail, "EARLIER_PRODUCER_FAILURE"
	findings := 0
	add := func(code, _, _ string) {
		if code == "SEMANTICS_PROJECTION_VERDICT" {
			findings++
		}
	}
	if e := semanticTestBudget(t).semanticBase(profile, mustJSON(t, c), &semanticNativeCache{}, &res, add); e != nil {
		t.Fatal(e)
	}
	if res.BaseVerified != (Verdict{StatusFailed, AssessFail}) || res.Code != "EARLIER_PRODUCER_FAILURE" || findings != 1 {
		t.Fatalf("invalid status escaped into partial result: %+v findings=%d", res, findings)
	}
}

func TestSemanticBaseMissingOracleCannotRemainPass(t *testing.T) {
	f := mandatoryFixture(t, false, false, claimPass, AssessPass)
	native, err := ParseIncrementalProfile(mustJSON(t, f.profile))
	if err != nil {
		t.Fatal(err)
	}
	native.Cases = native.Cases[:1]
	profile, cache := semanticExpectationControl()
	profile.Native, cache.ready = native, true
	c := f.cases[0]
	delete(c, "oracle_claims")
	res := QuerySemanticsResult{Report: newReport("query semantics")}
	res.Assessment = AssessBlocked
	add := func(code, _, _ string) {
		res.Assessment = AssessFail
		if res.Code == "" {
			res.Code = code
		}
	}
	if e := semanticTestBudget(t).semanticBase(profile, mustJSON(t, c), cache, &res, add); e != nil {
		t.Fatal(e)
	}
	if res.BaseVerified.Assessment != AssessFail || res.Assessment != AssessFail || res.Code != "ORACLE_CLAIMS_MISSING" {
		t.Fatalf("missing oracle claims lost: %+v", res)
	}
}
