package kit

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"
)

func ordinaryStoredPartial() OrdinaryQuerySemanticsResult {
	p := newOrdinarySemanticsResult()
	p.Code = "ELIGIBILITY_APPROVAL_MISSING"
	p.SelectedShardIDs = []string{"shard-one"}
	p.SelectedObligationIDs = []string{"qsem1:" + strings.Repeat("1", 64), "qsem1:" + strings.Repeat("2", 64)}
	p.CoverageSHA256 = ordinaryIDDigest(p.SelectedObligationIDs)
	p.Tuple = QueryEligibilityCounts{Required: 2, Blocked: 2}
	p.Obligations = []OrdinaryQuerySemanticsTuple{
		{ID: p.SelectedObligationIDs[0], State: ordinaryTupleBlocked, Code: p.Code},
		{ID: p.SelectedObligationIDs[1], State: ordinaryTupleBlocked, Code: p.Code},
	}
	return p
}

func ordinaryStoredJSON(t *testing.T, p OrdinaryQuerySemanticsResult) []byte {
	t.Helper()
	b, err := jsonv2.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeOrdinaryQuerySemanticsResultPreservesPartialCoverage(t *testing.T) {
	input := ordinaryStoredPartial()
	raw := ordinaryStoredJSON(t, input)
	got, err := DecodeOrdinaryQuerySemanticsResult(raw)
	if err != nil || got.Tuple != input.Tuple || got.SelectedObligationIDs[1] != input.SelectedObligationIDs[1] || got.CoverageSHA256 != input.CoverageSHA256 ||
		got.Complete || got.ProcessingComplete || got.FullScopeComplete || got.Assessment != AssessBlocked {
		t.Fatalf("partial ordinary result did not round-trip: err=%v result=%+v", err, got)
	}
	missing := strings.Replace(string(raw), `"binding_valid":false,`, "", 1)
	if missing == string(raw) {
		t.Fatal("test result did not contain required binding_valid")
	}
	if _, err := DecodeOrdinaryQuerySemanticsResult([]byte(missing)); err == nil {
		t.Fatal("missing required typed field accepted")
	}
}

func TestDecodeOrdinaryQuerySemanticsResultRejectsCoverageMutants(t *testing.T) {
	control := ordinaryStoredPartial()
	if !validOrdinaryStoredResult(control) {
		t.Fatal("valid partial result rejected")
	}
	mutants := map[string]func(*OrdinaryQuerySemanticsResult){
		"selected-id-deleted": func(p *OrdinaryQuerySemanticsResult) {
			p.SelectedObligationIDs = p.SelectedObligationIDs[1:]
			p.CoverageSHA256 = ordinaryIDDigest(p.SelectedObligationIDs)
		},
		"selected-id-duplicated": func(p *OrdinaryQuerySemanticsResult) {
			p.SelectedObligationIDs[1] = p.SelectedObligationIDs[0]
			p.CoverageSHA256 = ordinaryIDDigest(p.SelectedObligationIDs)
		},
		"coverage-digest-rehashed":   func(p *OrdinaryQuerySemanticsResult) { p.CoverageSHA256 = strings.Repeat("0", 64) },
		"obligation-duplicate":       func(p *OrdinaryQuerySemanticsResult) { p.Obligations[1].ID = p.Obligations[0].ID },
		"counter-inflated":           func(p *OrdinaryQuerySemanticsResult) { p.Tuple.Verified++ },
		"processing-complete-mutant": func(p *OrdinaryQuerySemanticsResult) { p.ProcessingComplete = true },
		"full-scope-mutant":          func(p *OrdinaryQuerySemanticsResult) { p.FullScopeComplete = true },
	}
	for name, mutate := range mutants {
		t.Run(name, func(t *testing.T) {
			p := control
			p.SelectedShardIDs = append([]string(nil), control.SelectedShardIDs...)
			p.SelectedObligationIDs = append([]string(nil), control.SelectedObligationIDs...)
			p.Obligations = append([]OrdinaryQuerySemanticsTuple(nil), control.Obligations...)
			mutate(&p)
			if _, err := DecodeOrdinaryQuerySemanticsResult(ordinaryStoredJSON(t, p)); err == nil {
				t.Fatal("mutated stored result accepted")
			}
		})
	}
}

func TestOrdinaryStoredEligibilityLeavesTupleForAdjudicator(t *testing.T) {
	p := QueryReferenceEligibilityResult{
		Report: newReport("query reference eligibility"), ResultSchema: QueryReferenceEligibilityResultSchema, Complete: true,
		Build: QueryEligibilityCounts{Required: 9, Verified: 9}, Unit: QueryEligibilityCounts{Required: 4, Verified: 4},
		Control: QueryEligibilityCounts{Required: 53, Verified: 53}, Assertion: QueryEligibilityCounts{Required: 36, Verified: 36},
		Tuple:    QueryEligibilityCounts{Required: 1, Unprocessed: 1},
		Evidence: &EvidenceResult{Report: newReport("evidence verify"), ResultSchema: EvidenceResultSchema},
	}
	p.ExecutionStatus, p.EvidenceMode, p.Assessment = StatusCompleted, ModeReplayedRaw, AssessPass
	p.Evidence.ExecutionStatus, p.Evidence.EvidenceMode, p.Evidence.Assessment = StatusCompleted, ModeNewRun, AssessPass
	if !validOrdinaryNestedEligibility(p, 1) {
		t.Fatal("ordinary eligibility's intentionally pending tuple rejected")
	}
	p.Tuple = QueryEligibilityCounts{Required: 1, Verified: 1}
	if validOrdinaryNestedEligibility(p, 1) {
		t.Fatal("ordinary eligibility accepted a tuple adjudication it does not own")
	}
}

func TestDecodeOrdinaryQuerySemanticsResultKeepsFirstFailureBeyondFindingCap(t *testing.T) {
	id := "qsem1:" + strings.Repeat("a", 64)
	p := newOrdinarySemanticsResult()
	p.SelectedShardIDs = []string{"ordinary/windows-amd64/case-one"}
	p.SelectedObligationIDs = []string{id}
	p.CoverageSHA256 = ordinaryIDDigest(p.SelectedObligationIDs)
	p.Tuple = QueryEligibilityCounts{Required: 1, Unprocessed: 1}
	p.Findings = make([]Finding, 1000)
	for i := range p.Findings {
		p.Findings[i] = Finding{Severity: "warning", Code: "EARLIER_WARNING"}
	}
	p.ordinaryRecordTuple(id, ordinaryTupleJudgment{State: ordinaryTupleFailed, Code: "FIRST_FAILURE"})
	ordinaryResultError(&p, fail(KindCancelled, "CANCELLED", "", nil), p.BytesRead, p.Work, p.LogicalBytes)
	if p.Code != "FIRST_FAILURE" || p.Tuple.Failed != 1 || p.ExecutionStatus != StatusCancelled || len(p.Findings) != 1000 {
		t.Fatalf("fixture did not preserve first failure at the cap: %+v", p)
	}
	if _, err := DecodeOrdinaryQuerySemanticsResult(ordinaryStoredJSON(t, p)); err != nil {
		t.Fatalf("valid capped partial result rejected: %v", err)
	}
	p.Code = "LATER_FAILURE"
	if _, err := DecodeOrdinaryQuerySemanticsResult(ordinaryStoredJSON(t, p)); err == nil {
		t.Fatal("capped partial result accepted a different first failure code")
	}
}

func TestDecodeOrdinaryQuerySemanticsResultBoundsReadLimitOvershoot(t *testing.T) {
	p := newOrdinarySemanticsResult()
	p.ExecutionStatus, p.Assessment = StatusResourceLimit, AssessBlocked
	p.Code = "TOTAL_BYTES_LIMIT"
	p.Findings = []Finding{{Severity: "error", Code: p.Code}}
	p.BytesRead = ReplayOperations()["evidence-replay"].TotalBytes + MaxDocumentBytes
	if _, err := DecodeOrdinaryQuerySemanticsResult(ordinaryStoredJSON(t, p)); err != nil {
		t.Fatalf("one bounded file read past the cap was rejected: %v", err)
	}
	p.BytesRead++
	if _, err := DecodeOrdinaryQuerySemanticsResult(ordinaryStoredJSON(t, p)); err == nil {
		t.Fatal("stored result exceeded the maximum one-file read overshoot")
	}
}

func ordinaryReplayPartialControl() ReplayResult {
	p := ordinaryStoredPartial()
	r := ReplayResult{Report: newReport("replay"), ResultSchema: ReplayOrdinaryResultSchema,
		Reducer: ReducerInfo{ID: ordinaryReplayReducerID, Revision: "r9"}, Operation: "evidence-replay", OrdinaryQuerySemantics: &p,
		Subject:  ReplaySubject{Run: "subject-run", Attempt: 1, Platform: "windows/amd64", EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked},
		Recorded: Verdict{StatusCompleted, AssessBlocked}, Recomputed: Verdict{p.ExecutionStatus, p.Assessment}, EvidenceValid: true,
		Consumption: Consumption{Expected: len(p.SelectedObligationIDs), Consumed: len(p.Obligations)}}
	r.ExecutionStatus, r.EvidenceMode, r.Assessment = StatusCompleted, ModeRecordedNotRecomputed, AssessBlocked
	gate := ReplayGate{ID: "ordinary-query-semantics", NotRecomputed: 1}
	gate.finish()
	r.Gates = []ReplayGate{gate}
	return r
}

func TestValidOrdinaryReplayResultRejectsCrossRevisionAndMembership(t *testing.T) {
	control := ordinaryReplayPartialControl()
	if !validOrdinaryReplayResult(control) {
		t.Fatal("valid blocked replay result rejected")
	}
	tests := map[string]func(*ReplayResult){
		"legacy-reducer":       func(r *ReplayResult) { r.Reducer.ID, r.Reducer.Revision = "oracle-set-r6", "r6" },
		"legacy-result-schema": func(r *ReplayResult) { r.ResultSchema = ReplaySemanticsResultSchema },
		"legacy-payload-inserted": func(r *ReplayResult) {
			r.QuerySemantics = &QuerySemanticsResult{ResultSchema: QuerySemanticsResultSchema}
		},
		"coverage-rehashed":     func(r *ReplayResult) { r.OrdinaryQuerySemantics.CoverageSHA256 = strings.Repeat("0", 64) },
		"wrong-consumption":     func(r *ReplayResult) { r.Consumption.Consumed-- },
		"outer-budget-mismatch": func(r *ReplayResult) { r.BytesRead++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			r := control
			p := *control.OrdinaryQuerySemantics
			p.SelectedShardIDs = append([]string(nil), p.SelectedShardIDs...)
			p.SelectedObligationIDs = append([]string(nil), p.SelectedObligationIDs...)
			p.Obligations = append([]OrdinaryQuerySemanticsTuple(nil), p.Obligations...)
			r.OrdinaryQuerySemantics = &p
			mutate(&r)
			if validOrdinaryReplayResult(r) {
				t.Fatal("cross-contract replay mutant accepted")
			}
		})
	}
}

func TestOrdinaryReplayStoredPayloadCannotCrossLegacyRevisions(t *testing.T) {
	_, root, profile := ordinaryPartialReplayFixture(t)
	data := ordinaryJSON(t, profile)
	replayed, err := Replay(ordinaryReplayContext(t), ReplayRequest{Root: root, Profile: data})
	if err != nil || !validOrdinaryReplayResult(replayed) {
		t.Fatalf("ordinary result control rejected: err=%v result=%+v", err, replayed)
	}
	stored := ordinaryJSON(t, replayed)
	check := func(raw []byte, wantValid bool) {
		t.Helper()
		budget := ordinaryTestBudget(t)
		if wantValid {
			if _, e := decodeOrdinaryReplayResult(raw, budget.r); e != nil {
				t.Fatalf("stored replay decoder rejected raw result: %v", e)
			}
		}
		x := &replayEnv{r: budget.r}
		node := &EvidenceNode{ID: "ordinary-replay", EvidenceMode: replayed.EvidenceMode, Assessment: replayed.Assessment,
			Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
		subject := &EvidenceNode{ExecutionStatus: replayed.Subject.ExecutionStatus, EvidenceMode: replayed.Subject.EvidenceMode, Assessment: replayed.Subject.Assessment,
			Identities: map[string]string{"run": replayed.Subject.Run, "attempt": "1", "platform": replayed.Subject.Platform}}
		if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": raw}, x.finding); e != nil {
			t.Fatalf("stored evidence check returned an execution error: %v", e)
		}
		if (len(x.findings) == 0) != wantValid {
			t.Fatalf("stored payload dispatch validity=%t, want %t: %+v", len(x.findings) == 0, wantValid, x.findings)
		}
	}
	check(stored, true)
	for _, schema := range []string{ReplayResultSchema, ReplayReferenceResultSchema, ReplaySemanticsResultSchema} {
		t.Run(schema, func(t *testing.T) {
			var mutant map[string]any
			if err := json.Unmarshal(stored, &mutant); err != nil {
				t.Fatal(err)
			}
			mutant["result_schema"] = schema
			reducer := mutant["reducer"].(map[string]any)
			switch schema {
			case ReplayResultSchema:
				reducer["id"], reducer["revision"] = "oracle-set-r2", "r2"
			case ReplayReferenceResultSchema:
				reducer["id"], reducer["revision"] = "query-reference-connection-r1", "r1"
			case ReplaySemanticsResultSchema:
				reducer["id"], reducer["revision"] = "oracle-set-r4", "r4"
			}
			check(mustJSON(t, mutant), false)
		})
	}
}
