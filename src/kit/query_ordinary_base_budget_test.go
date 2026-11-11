package kit

import (
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

func TestOrdinaryBaseReplayMatchesLegacy(t *testing.T) {
	for _, mutation := range []string{"none", "tree", "claim", "status", "anchor"} {
		t.Run(mutation, func(t *testing.T) {
			f := mandatoryFixture(t, false, false, claimPass, AssessPass)
			native, err := ParseIncrementalProfile(mustJSON(t, f.profile))
			if err != nil {
				t.Fatal(err)
			}
			native.Cases = native.Cases[1:2]
			c := f.cases[1]
			c["oracle_claims"] = map[string]string{"query_equality": claimNotClaimed, "query_expectations": claimNotClaimed, "api": claimNotClaimed, "fact_reproduction": claimNotClaimed, "dynamic_sql": claimNotClaimed}
			c["padding"] = strings.Repeat("x", 4096)
			switch mutation {
			case "tree":
				c["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["digest"] = strings.Repeat("0", 64)
			case "claim":
				c["claims"].(map[string]string)["incremental_equality"] = claimFail
			case "status":
				c["execution_status"] = "FORGED"
			case "anchor":
				native.Cases[0].Expect = []StepExpectation{{Step: 0, Syntax: "ANY", Contains: []string{"identifier"}, Anchors: []ExpectAnchor{{Type: "absent", StartByte: 0, EndByte: 1}}}}
			}
			profile := OracleProfile{Native: native, OracleCases: []OracleCase{{}}}
			data := mustJSON(t, c)
			run := func(ordinary bool) (QuerySemanticsResult, []Finding, uint64) {
				b := semanticTestBudget(t)
				res := QuerySemanticsResult{Report: newReport("base parity")}
				res.Assessment = AssessBlocked
				findings := []Finding{}
				add := func(code, path, message string) {
					res.Assessment = AssessFail
					findings = append(findings, Finding{Code: code, Path: path, Message: message})
				}
				if e := b.semanticBase(profile, data, &semanticNativeCache{ordinaryBase: ordinary, ready: true}, &res, add); e != nil {
					t.Fatal(e)
				}
				return res, findings, b.work
			}
			legacy, legacyFindings, legacyWork := run(false)
			ordinary, ordinaryFindings, ordinaryWork := run(true)
			if !reflect.DeepEqual(ordinary, legacy) || !reflect.DeepEqual(ordinaryFindings, legacyFindings) || ordinaryWork >= legacyWork {
				t.Fatalf("base parity/work: legacy=%+v/%v/%d ordinary=%+v/%v/%d", legacy, legacyFindings, legacyWork, ordinary, ordinaryFindings, ordinaryWork)
			}
			if mutation != "none" && ordinary.BaseVerified.Assessment != AssessFail {
				t.Fatalf("base counterexample was not rejected: %+v", ordinary)
			}
		})
	}
}

func TestOrdinaryBaseInputBoundSelectsOnlyConsumedValues(t *testing.T) {
	b := semanticTestBudget(t)
	projection, e := b.semanticAST("projection", []byte(`{"execution_status":"COMPLETED"}`))
	if e != nil {
		t.Fatal(e)
	}
	text := strings.Repeat("\x00\\한", 1024)
	decls := []nativewire.DeclItem{{Fact: text, NodeType: text, Status: text, Name: &nativewire.NameSpan{StartByte: 1, EndByte: 4294967295}}}
	raw := nativewire.Response{Status: StatusCompleted, Code: text, Producer: nativewire.Producer{Query: text, API: &text, Predicates: &text}, Steps: []nativewire.Step{{Incremental: nativewire.WireTree{Declarations: &decls}}}}
	before, e := b.ordinaryBaseInputBytes(raw, projection)
	if e != nil {
		t.Fatal(e)
	}
	var actual uint64
	for _, value := range []any{raw.Producer, raw.Status, raw.Code, nil, raw.Steps[0].Incremental.Declarations} {
		data, err := jsonv2.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		actual += uint64(len(data))
	}
	if before < actual {
		t.Fatalf("selected input bound below escaped serialization: reserved=%d actual=%d", before, actual)
	}
	raw.Steps[0].Incremental.Types = []string{strings.Repeat("unconsumed", 100000)}
	after, e := b.ordinaryBaseInputBytes(raw, projection)
	if e != nil || before != after {
		t.Fatalf("unconsumed tree payload changed input bound: before=%d after=%d err=%v", before, after, e)
	}
}
