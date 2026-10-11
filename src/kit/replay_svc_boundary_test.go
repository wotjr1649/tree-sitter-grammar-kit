package kit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func svcObservedCase(t *testing.T, source []byte) (rcCase, IncrementalCase) {
	t.Helper()
	o := ObserveServiceHost(EncodingUTF8, source)
	c := rcCase{Schema: OracleRecordSchema, ID: "synthetic", ExecutionStatus: StatusCompleted, Assessment: AssessPass,
		Input: NativeInput{Bytes: uint64(len(source)), SHA256: digestHex(source)}, Claims: rcClaims{claimNotClaimed, claimNotClaimed, claimNotClaimed}}
	c.Steps = []rcStep{{SourceBytes: uint64(len(source)), SourceSHA256: digestHex(source), Composite: &rcComposite{
		Schema: SvcCompositeSchema, Format: SvcFormat, Input: rcInput{Bytes: uint64(len(source)), SHA256: digestHex(source), Encoding: EncodingUTF8, EncodingSource: SourceDeclaration},
		Directive: o.Directive, AdditionalDirectives: o.AdditionalDirectives, Language: o.Language, CodeBehind: o.CodeBehind, Coverage: o.Coverage,
		Identities: []IdentityRef{{"producer", "tsgk-native-build/r1", fxProducer}, {"source", "tsgk-source-bytes/r1", digestHex(source)}, {"policy", "tsgk-native-policy/r1", fxPolicy}},
	}}}
	if err := json.Unmarshal([]byte(`{"oracle_claims":{"query_equality":"NOT_CLAIMED","query_expectations":"NOT_CLAIMED","api":"NOT_CLAIMED","fact_reproduction":"NOT_CLAIMED","dynamic_sql":"NOT_CLAIMED"}}`), &c); err != nil {
		t.Fatal(err)
	}
	return c, IncrementalCase{ID: c.ID, Input: c.Input, Encoding: EncodingUTF8, SvcSource: source}
}

func TestSvcObservationReplayClaims(t *testing.T) {
	for _, variant := range []string{"valid", "legacy", "equality", "route", "expectations", "extra-expectation", "query-equality", "api", "missing-oracle", "unknown-claim", "query-pass", "query-fail", "facts", "dynamic", "fresh", "comparison", "route-proof", "query-comparison", "restart"} {
		t.Run(variant, func(t *testing.T) {
			c, want := svcObservedCase(t, []byte(`<%@ ServiceHost Service="S" %>`))
			recorded := false
			switch variant {
			case "legacy":
				c.Schema, c.Oracle = "", nil
			case "equality":
				c.Claims.IncrementalEquality = claimPass
			case "route":
				c.Claims.IncrementalRoute = claimPass
			case "expectations":
				c.Claims.Expectations = claimPass
			case "extra-expectation":
				c.Expectations = []rcExpect{{Step: 0, Syntax: "NO_ERROR", Result: claimPass}}
			case "query-equality":
				c.Oracle.QueryEquality = claimPass
			case "api":
				c.Oracle.API = claimPass
			case "missing-oracle":
				c.Oracle = nil
			case "unknown-claim":
				c.Oracle.FactReproduction = "unknown"
			case "query-pass", "query-fail":
				recorded = true
				c.Oracle.QueryExpectations = claimPass
				if variant == "query-fail" {
					c.Oracle.QueryExpectations, c.Assessment, c.Code = claimFail, AssessFail, "ORACLE_CLAIM_FAILED"
				}
			case "facts":
				recorded = true
				c.Oracle.FactReproduction, c.Assessment = claimBlocked, AssessBlocked
			case "dynamic":
				recorded = true
				c.Oracle.DynamicSQL, c.Assessment, c.Code = claimFail, AssessFail, "ORACLE_CLAIM_FAILED"
			case "restart":
				c.Steps[0].Restarted = true
			case "fresh", "comparison", "route-proof", "query-comparison":
				payload := map[string]string{"fresh": `{"fresh":{}}`, "comparison": `{"comparison":{"equal":true}}`, "route-proof": `{"route":{"proven":true}}`, "query-comparison": `{"query_comparison":{"equal":true}}`}
				if err := json.Unmarshal([]byte(payload[variant]), &c.Steps[0]); err != nil {
					t.Fatal(err)
				}
			}
			x := &replayEnv{svcFormat: SvcFormat, nativeInputBytes: NativeOperations()["native-parse-edit"].InputBytes, seen: map[string]map[string]bool{}}
			out := x.replayCase(&c, 0, &want, false)
			failed := false
			for _, g := range x.gates {
				failed = failed || g.Failed > 0
			}
			if variant == "valid" || variant == "legacy" {
				if failed || out.assess != AssessPass || !out.complete {
					t.Fatalf("valid observation rejected: %+v %+v", out, x.gates)
				}
			} else if recorded {
				if failed || out.complete || out.assess != AssessUnresolved {
					t.Fatalf("recorded-only claim promoted: %+v %+v", out, x.gates)
				}
			} else if !failed {
				t.Fatal("impossible observation claim accepted")
			}
		})
	}
}

func TestQualifyExpandedSourceBudget(t *testing.T) {
	source := "S"
	prefix := bytes.Repeat([]byte(" "), (1<<20)+1)
	captures := []CaptureExpectation{}
	qc := &QualCase{ID: "synthetic", Source: &source, Edits: []Edit{{NewEndByte: uint32(len(prefix)), New: prefix}},
		QueryExpect: []QueryExpectation{{Query: "q", Step: 1, Status: StatusCompleted, Captures: &captures}}}
	c := rcCase{ID: qc.ID, ExecutionStatus: StatusCompleted, Claims: rcClaims{claimPass, claimPass, claimNotClaimed}}
	if err := json.Unmarshal([]byte(`{"oracle_claims":{"query_equality":"NOT_CLAIMED","query_expectations":"PASS","api":"NOT_CLAIMED","fact_reproduction":"NOT_CLAIMED","dynamic_sql":"NOT_CLAIMED"}}`), &c); err != nil {
		t.Fatal(err)
	}
	var extra qRecord
	if err := json.Unmarshal([]byte(`{"steps":[{}, {"incremental":{"queries":[{"id":"q","status":"COMPLETED","captures":[]}]}}]}`), &extra); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"real-world-source-r2", "native-parse-edit", "unknown"} {
		t.Run(operation, func(t *testing.T) {
			s := &qset{kinds: map[string]map[string]string{}, checks: map[string]string{}, summaries: map[string]string{}}
			q := &qualifier{hosts: map[string]*qhost{"": {x: &replayEnv{}}}}
			q.judgeCase(s, qc, nil, &c, &extra, caseOutcome{}, NativeOperations()[operation].InputBytes, func(_, _, _ string) {}, "synthetic", false, false)
			if (s.checks[qc.ID] == claimPass) != (operation == "real-world-source-r2") {
				t.Fatalf("qualifier source budget not applied: %s", s.checks[qc.ID])
			}
		})
	}
}

func TestSvcReplayRejectsFreshOnlyGap(t *testing.T) {
	c := svcSegmentCase(t, "", 0)
	if err := json.Unmarshal([]byte(`{"oracle_claims":{"query_equality":"NOT_CLAIMED","query_expectations":"NOT_CLAIMED","api":"NOT_CLAIMED","fact_reproduction":"NOT_CLAIMED","dynamic_sql":"NOT_CLAIMED"}}`), &c); err != nil {
		t.Fatal(err)
	}
	c.Steps[2].Fresh = c.Steps[1].Fresh
	x := &replayEnv{svcFormat: SvcLegacyFormat, seen: map[string]map[string]bool{}}
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("untrusted gap caused panic: %v", p)
		}
	}()
	x.replayCase(&c, 0, nil, true)
	if x.gate("case-status").Code != "STATUS_TREE_INCONSISTENT" {
		t.Fatal("fresh-only gap accepted")
	}
}

func TestSvcQualificationKeepsRecordedOnlyClaimsUnresolved(t *testing.T) {
	for _, kind := range []string{"query", "facts", "dynamic"} {
		for _, claim := range []string{claimNotClaimed, claimPass, claimFail, claimBlocked} {
			for _, registered := range []bool{false, true} {
				t.Run(kind+"/"+claim+"/"+map[bool]string{true: "registered", false: "unregistered"}[registered], func(t *testing.T) {
					c, want := svcObservedCase(t, []byte(`<%@ ServiceHost Service="S" %>`))
					switch kind {
					case "query":
						c.Oracle.QueryExpectations = claim
					case "facts":
						c.Oracle.FactReproduction = claim
					case "dynamic":
						c.Oracle.DynamicSQL = claim
					}
					c.Assessment = foldClaims([]string{claimPass, claim})
					if c.Assessment == AssessFail {
						c.Code = "ORACLE_CLAIM_FAILED"
					}
					x := &replayEnv{svcFormat: SvcFormat, nativeInputBytes: NativeOperations()["native-parse-edit"].InputBytes, seen: map[string]map[string]bool{}}
					out := x.replayCase(&c, 0, &want, false)
					for _, gate := range x.gates {
						if gate.Failed > 0 {
							t.Fatalf("valid recorded-only case rejected: %+v", gate)
						}
					}
					qc := &QualCase{ID: c.ID}
					if registered {
						qc.ExpectAssessment, qc.ExpectCode = c.Assessment, c.Code
					}
					s := &qset{kinds: map[string]map[string]string{}, checks: map[string]string{}, summaries: map[string]string{}}
					q := &qualifier{}
					q.judgeCase(s, qc, nil, &c, &qRecord{}, out, x.nativeInputBytes, func(_, _, _ string) {}, "synthetic", true, false)
					expected := claimBlocked
					if claim == claimNotClaimed {
						expected = claimPass
					}
					if s.checks[c.ID] != expected {
						t.Fatalf("recorded-only claim promoted: check=%s expected=%s replay=%+v", s.checks[c.ID], expected, out)
					}
				})
			}
		}
	}
}

func TestReplayRejectsNativeEncodingProvenance(t *testing.T) {
	for _, form := range []string{"full", "summary"} {
		for _, provenance := range []string{"", "BOM", "VALIDATION", "wrong"} {
			t.Run(form+"/"+provenance, func(t *testing.T) {
				f := newFxNative()
				if form == "summary" {
					fxSummaryOut(f.cases[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any))
				}
				data, _ := json.Marshal(f.cases[0])
				var c rcCase
				_ = json.Unmarshal(data, &c)
				tree := c.Steps[0].Incremental
				if tree.Tree != nil {
					tree.Tree.Input.EncodingSource = provenance
				} else {
					tree.Summary.Input.EncodingSource = provenance
				}
				x := &replayEnv{seen: map[string]map[string]bool{}}
				x.replayCase(&c, 0, nil, false)
				if x.gate("tree").Code != "TREE_INPUT_MISMATCH" {
					t.Fatal("native encoding provenance accepted")
				}
			})
		}
	}
}

func TestSvcReplayExpandedSourceBudget(t *testing.T) {
	source := []byte(`<%@ ServiceHost Service="S" %>`)
	c, want := svcObservedCase(t, source)
	prefix := bytes.Repeat([]byte(" "), (1<<20)+1)
	next := append(bytes.Clone(prefix), source...)
	part, _ := svcObservedCase(t, next)
	part.Steps[0].Step = 1
	c.Steps = append(c.Steps, part.Steps[0])
	want.Edits = []Edit{{NewEndByte: uint32(len(prefix)), New: prefix}}
	e := want.Edits[0]
	c.Steps[1].Edit = &rcEdit{StartByte: &e.StartByte, OldEndByte: &e.OldEndByte, NewEndByte: &e.NewEndByte,
		StartPoint: []uint32{0, 0}, OldEndPoint: []uint32{0, 0}, NewEndPoint: []uint32{0, e.NewEndByte}}
	for _, operation := range []string{"real-world-source-r2", "native-parse-edit", "unknown"} {
		t.Run(operation, func(t *testing.T) {
			x := &replayEnv{svcFormat: SvcFormat, nativeInputBytes: NativeOperations()[operation].InputBytes}
			x.checkSvcComposite(&c, &want)
			if (x.gate("case-binding").Failed == 0) != (operation == "real-world-source-r2") {
				t.Fatalf("registered source budget not applied: %+v", x.gate("case-binding"))
			}
		})
	}
}

func TestSvcReplayBindsEditMetadata(t *testing.T) {
	for _, bad := range []string{"valid", "missing", "start_byte", "old_end_byte", "new_end_byte", "start_point", "old_end_point", "new_end_point", "step-zero", "missing-start_byte", "missing-old_end_byte", "missing-new_end_byte", "missing-start_point", "missing-old_end_point", "missing-new_end_point", "empty-point"} {
		t.Run(bad, func(t *testing.T) {
			src := []byte("<%@ ServiceHost Service=\"S\" %>\r\n")
			next := []byte("<%@ ServiceHost Service=\"Longer\" %>\r\n ")
			c, want := svcObservedCase(t, src)
			part, _ := svcObservedCase(t, next)
			part.Steps[0].Step = 1
			c.Steps = append(c.Steps, part.Steps[0])
			want.Edits = []Edit{{OldEndByte: uint32(len(src)), NewEndByte: uint32(len(next)), Old: src, New: next}}
			e := map[string]any{"start_byte": 0, "old_end_byte": len(src), "new_end_byte": len(next), "start_point": []uint32{0, 0}, "old_end_point": []uint32{1, 0}, "new_end_point": []uint32{1, 1}}
			switch bad {
			case "start_byte", "old_end_byte", "new_end_byte":
				e[bad] = 999
			case "start_point", "old_end_point", "new_end_point":
				e[bad] = []uint32{9, 9}
			case "empty-point":
				e["start_point"] = []uint32{}
			default:
				if strings.HasPrefix(bad, "missing-") {
					delete(e, strings.TrimPrefix(bad, "missing-"))
				}
			}
			if bad != "missing" {
				data, _ := json.Marshal(map[string]any{"edit": e})
				if err := json.Unmarshal(data, &c.Steps[1]); err != nil {
					t.Fatal(err)
				}
				if bad == "step-zero" {
					if err := json.Unmarshal(data, &c.Steps[0]); err != nil {
						t.Fatal(err)
					}
				}
			}
			x := &replayEnv{svcFormat: SvcFormat, nativeInputBytes: NativeOperations()["native-parse-edit"].InputBytes}
			x.checkSvcComposite(&c, &want)
			if (x.gate("case-binding").Failed == 0) != (bad == "valid") {
				t.Fatalf("edit metadata binding: %s %+v", bad, x.gate("case-binding"))
			}
		})
	}
}
