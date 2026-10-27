package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func semanticReplayResultControl() ReplayResult {
	p := &QuerySemanticsResult{Report: newReport("query semantics"), ResultSchema: QuerySemanticsResultSchema, ProfileSHA256: strings.Repeat("1", 64), RegistrationSHA256: strings.Repeat("2", 64), RunBindingSHA256: strings.Repeat("3", 64), BindingValid: true, ProducerFoldValid: true, Complete: true, Semantic: AssessBlocked,
		ProducerRecorded: Verdict{StatusCompleted, AssessBlocked}, BaseVerified: Verdict{StatusCompleted, AssessPass}, Tuple: QueryEligibilityCounts{Required: 24, Blocked: 24}, Evidence: &EvidenceResult{Report: newReport("evidence verify"), ResultSchema: EvidenceResultSchema}, Eligibility: &QueryReferenceEligibilityResult{Report: newReport("query reference eligibility"), ResultSchema: QueryReferenceEligibilityResultSchema}}
	p.ExecutionStatus, p.EvidenceMode, p.Assessment = StatusCompleted, ModeReplayedRaw, AssessBlocked
	p.Evidence.ExecutionStatus, p.Evidence.EvidenceMode, p.Evidence.Assessment = StatusCompleted, ModeNewRun, AssessPass
	p.Eligibility.ExecutionStatus, p.Eligibility.EvidenceMode, p.Eligibility.Assessment = StatusCompleted, ModeReplayedRaw, AssessBlocked
	p.Gates = []ReplayGate{}
	for _, id := range []string{"case-binding", "case-status", "tree", "verdict", "incremental-equality", "incremental-route", "expectations", "query-equality", "api", "query-expectations", "fact-reproduction", "dynamic-sql"} {
		gate := ReplayGate{ID: id, Recomputed: 1}
		gate.finish()
		p.Gates = append(p.Gates, gate)
	}
	p.Evidence.Modes = map[string]int{}
	p.Evidence.Explanation = []string{}
	p.Eligibility.Build = QueryEligibilityCounts{Required: 9, Unprocessed: 9}
	p.Eligibility.Unit = QueryEligibilityCounts{Required: 4, Unprocessed: 4}
	p.Eligibility.Control = QueryEligibilityCounts{Required: 53, Unprocessed: 53}
	p.Eligibility.Assertion = QueryEligibilityCounts{Required: 36, Unprocessed: 36}
	g := ReplayGate{ID: "query-semantics", NotRecomputed: 1}
	g.finish()
	r := ReplayResult{Report: newReport("replay"), ResultSchema: ReplaySemanticsResultSchema, Reducer: reducerTable()["oracle-set-r4"].info, Operation: "evidence-replay", QuerySemantics: p, Recorded: p.ProducerRecorded, Recomputed: Verdict{StatusCompleted, AssessBlocked}, Gates: []ReplayGate{g}, Consumption: Consumption{Expected: 1, Consumed: 1}, EvidenceValid: true, Observed: map[string]string{"workload": p.ProfileSHA256, "semantics-registration": p.RegistrationSHA256, "semantics-run-binding": p.RunBindingSHA256}, Subject: ReplaySubject{Run: "oracle-parent", Attempt: 1, Platform: "windows/amd64", EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked}}
	r.Explanation = []string{}
	r.ExecutionStatus, r.EvidenceMode, r.Assessment = StatusCompleted, ModeReplayedRaw, AssessBlocked
	return r
}

func TestReplaySemanticHeaderBoundsBeforeNestedAllocation(t *testing.T) {
	data := append([]byte(`{"result_schema":"tsgk-replay-result/r3","query_reference":{"findings":[`), bytes.Repeat([]byte("{},"), 2000000)...)
	data[len(data)-1] = ']'
	data = append(data, []byte("}}")...)
	b := semanticTestBudget(t)
	b.r.eligibility = nil
	x := &replayEnv{r: b.r}
	node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": data}, x.finding)
	runtime.ReadMemStats(&after)
	if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" || b.r.eligibility != nil || b.r.storedSemantics == nil || b.r.storedSemantics.logical > eligibilityLogicalLimit {
		t.Fatalf("nested result allocated before bound: %v", e)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8*uint64(len(data))+8<<20 {
		t.Fatalf("minimal schema probe allocated nested collections: %d for %d bytes", allocated, len(data))
	}
}

func TestReplaySemanticSchemaProbeBoundsWideObject(t *testing.T) {
	data := []byte(`{"query_reference":{`)
	for i := 0; i < 450000; i++ {
		data = append(data, '"', 'k')
		data = strconv.AppendInt(data, int64(i), 10)
		data = append(data, []byte(`":null,`)...)
	}
	data[len(data)-1] = '}'
	data = append(data, []byte(`,"result_schema":"tsgk-replay-result/r3"}`)...)
	b := semanticTestBudget(t)
	b.r.eligibility = nil // standalone VerifyEvidence starts without a semantic ledger
	b.r.storedSemantics = &eligibilityBudget{r: b.r, logical: eligibilityLogicalLimit - 65536}
	x := &replayEnv{r: b.r}
	node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": data}, x.finding)
	runtime.ReadMemStats(&after)
	if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" || b.r.eligibility != nil || b.r.storedSemantics == nil || b.r.storedSemantics.logical != eligibilityLogicalLimit-65536 {
		t.Fatalf("schema probe did not reserve names before reading nested object: %v", e)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("schema probe allocated before reservation: %d", allocated)
	}
}

func TestReplaySemanticProbePreservesLegacyAndInvalidFindings(t *testing.T) {
	data := mustJSON(t, map[string]any{"result_schema": ReplayResultSchema, "evidence_mode": ModeReplayedRaw, "assessment": AssessPass, "reducer": map[string]string{"id": "oracle-set-r2"}, "subject": map[string]any{"run": "original", "attempt": 1, "platform": "windows/amd64"}, "legacy_padding": strings.Repeat("x", 5<<20)})
	node := &EvidenceNode{EvidenceMode: ModeReplayedRaw, Assessment: AssessPass, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
	subject := &EvidenceNode{Identities: map[string]string{"run": "original", "attempt": "1", "platform": "windows/amd64"}}
	b := semanticTestBudget(t)
	b.r.eligibility = nil
	x := &replayEnv{r: b.r}
	for i := 0; i < 2; i++ {
		if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": data}, x.finding); e != nil || len(x.findings) != 0 || b.r.eligibility != nil {
			t.Fatalf("legacy revision acquired semantic limits: %v %+v", e, x.findings)
		}
	}
	array := []byte(`{"result_schema":"tsgk-replay-result/r1","evidence_mode":"REPLAYED_RAW","assessment":"PASS","reducer":{"id":"oracle-set-r2"},"subject":{"run":"original","attempt":1,"platform":"windows/amd64"},"legacy_padding":[` + strings.Repeat("0,", 2000000) + "0]}")
	if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": array}, x.finding); e != nil || len(x.findings) != 0 || b.r.eligibility != nil {
		t.Fatalf("legacy result acquired a new semantic value cap: %v %+v", e, x.findings)
	}
	if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": []byte("{")}, x.finding); e != nil || len(x.findings) != 1 || x.findings[0].Code != "REPLAY_RESULT_INVALID" {
		t.Fatalf("damaged JSON became API error instead of finding: %v %+v", e, x.findings)
	}
	data = mustJSON(t, semanticReplayResultControl())
	data = append(data[:len(data)-1], []byte(`,"result_schema":"tsgk-replay-result/r1"}`)...)
	x.findings = nil
	if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": data}, x.finding); e != nil || len(x.findings) != 1 || x.findings[0].Code != "REPLAY_RESULT_INVALID" {
		t.Fatalf("duplicate schema escaped selected strict decoder: %v %+v", e, x.findings)
	}
}

func TestReplaySemanticProbeBoundsNamesDuplicatesAndDepth(t *testing.T) {
	wide := []byte(`{"result_schema":"tsgk-replay-result/r3","query_reference":{`)
	for i := 0; i < 450000; i++ {
		wide = append(wide, '"', 'k')
		wide = strconv.AppendInt(wide, int64(i), 10)
		wide = append(wide, []byte(`":null,`)...)
	}
	wide[len(wide)-1] = '}'
	wide = append(wide, []byte(`,"result_schema":"tsgk-replay-result/r1"}`)...)
	for _, tc := range []struct {
		name string
		data []byte
		code string
	}{
		{"duplicate-around-wide-object", wide, ""},
		{"escaped-long-key", []byte(`{"` + strings.Repeat(`\u006b`, 1<<20) + `":null,"result_schema":"tsgk-replay-result/r3"}`), ""},
		{"long-schema-value", []byte(`{"result_schema":"` + strings.Repeat(`\u0078`, 1<<20) + `"}`), "JSON_SHAPE"},
		{"deep-semantic", []byte(`{"result_schema":"tsgk-replay-result/r3","padding":` + strings.Repeat("[", 10001) + "0" + strings.Repeat("]", 10001) + "}"), ""},
		{"deep-schema-last", []byte(`{"padding":` + strings.Repeat("[", 40) + "0" + strings.Repeat("]", 40) + `,"result_schema":"tsgk-replay-result/r3"}`), ""},
		{"long-key-deep-schema-last", []byte(`{"` + strings.Repeat(`\u006b`, 1<<20) + `":` + strings.Repeat("[", 10001) + "0" + strings.Repeat("]", 10001) + `,"result_schema":"tsgk-replay-result/r3"}`), ""},
		{"long-key-broken-json", []byte(`{"` + strings.Repeat(`\u006b`, 1<<20) + `":["unterminated`), "JSON_SYNTAX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := semanticTestBudget(t)
			b.r.eligibility = nil
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			revision, e := replayResultRevision(tc.data, b.r)
			runtime.ReadMemStats(&after)
			if tc.code == "" {
				if e != nil || revision != ReplaySemanticsResultSchema {
					t.Fatalf("bounded name probe: %q %v", revision, e)
				}
			} else if e == nil || e.Code != tc.code {
				t.Fatalf("probe %s: %q %v", tc.name, revision, e)
			}
			if b.r.eligibility != nil || after.TotalAlloc-before.TotalAlloc > 1<<20 {
				t.Fatalf("schema probe retained unbounded names: ledger=%v allocated=%d", b.r.eligibility != nil, after.TotalAlloc-before.TotalAlloc)
			}
			if strings.Contains(tc.name, "deep-") {
				x := &replayEnv{r: b.r}
				node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
				if e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": tc.data}, x.finding); e == nil || e.Kind != KindResourceLimit {
					t.Fatalf("bounded selected decoder lost its depth/resource limit: %v", e)
				}
			}
			if tc.name == "duplicate-around-wide-object" {
				b.r.storedSemantics = &eligibilityBudget{r: b.r, logical: eligibilityLogicalLimit - 65536}
				x := &replayEnv{r: b.r}
				node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
				if e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": tc.data}, x.finding); e == nil || e.Kind != KindResourceLimit {
					t.Fatalf("r3 selection fell back to unreserved legacy decoding: %v", e)
				}
				fresh := semanticTestBudget(t)
				fresh.r.eligibility = nil
				x = &replayEnv{r: fresh.r}
				if e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": tc.data}, x.finding); e != nil || len(x.findings) != 1 || x.findings[0].Code != "REPLAY_RESULT_INVALID" {
					t.Fatalf("ample token budget lost duplicate rejection: %v %+v", e, x.findings)
				}
			}
		})
	}
}

func TestReplaySemanticStoredReaderDoesNotChangeLegacyByOrder(t *testing.T) {
	r3 := semanticReplayResultControl()
	r3Data := mustJSON(t, r3)
	legacyData := []byte(`{"result_schema":"tsgk-replay-result/r1","evidence_mode":"REPLAYED_RAW","assessment":"PASS","reducer":{"id":"oracle-set-r2"},"subject":{"run":"original","attempt":1,"platform":"windows/amd64"},"padding":[` + strings.Repeat("0,", 2000000) + "0]}")
	for _, r3First := range []bool{false, true} {
		b := semanticTestBudget(t)
		b.r.eligibility = nil
		x := &replayEnv{r: b.r}
		for _, semantic := range []bool{r3First, !r3First} {
			data, assessment, run := legacyData, AssessPass, "original"
			if semantic {
				data, assessment, run = r3Data, r3.Assessment, r3.Subject.Run
			}
			node := &EvidenceNode{EvidenceMode: ModeReplayedRaw, Assessment: assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			subjectAssessment := AssessPass
			if semantic {
				subjectAssessment = r3.Subject.Assessment
			}
			subject := &EvidenceNode{ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: subjectAssessment, Identities: map[string]string{"run": run, "attempt": "1", "platform": "windows/amd64"}}
			if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": data}, x.finding); e != nil || len(x.findings) != 0 {
				t.Fatalf("stored r3 changed legacy acceptance: r3first=%v semantic=%v %v %+v", r3First, semantic, e, x.findings)
			}
		}
		if b.r.eligibility != nil || b.r.storedSemantics == nil || b.r.storedSemantics.logical == 0 || b.r.storedSemantics.work == 0 {
			t.Fatal("standalone stored r3 reservation disappeared")
		}
	}
}

func TestReplaySemanticStoredGraphPreservesReferenceAPI(t *testing.T) {
	for _, damaged := range []bool{false, true} {
		f := newReferenceFixture(t)
		stored := semanticReplayResultControl()
		stored.Subject.Run = "semantic-original"
		subject := EvidenceNode{ID: "semantic-subject", Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessBlocked, Identities: ident("semantic-policy", stored.Subject.Run), Files: []ReplayMember{}, Refs: []EvidenceRef{}}
		data := mustJSON(t, stored)
		f.files["semantics-result.json"] = data
		replayed := EvidenceNode{ID: "semantic-replay", Kind: "replay", ExecutionStatus: StatusCompleted, EvidenceMode: ModeReplayedRaw, Assessment: stored.Assessment, Identities: ident("semantic-policy", "semantic-replayed"), Files: []ReplayMember{{Path: "semantics-result.json", Role: "replay-result", Bytes: uint64(len(data)), SHA256: digestHex(data)}}, Refs: []EvidenceRef{{Relation: "subject", Node: subject.ID}}}
		f.nodes = append(f.nodes, subject, replayed)
		if damaged {
			delete(f.responses, "reference-0.json")
			f.files["reference-0.json"] = []byte("{")
			f.approve(t, "reference-0.json")
		}
		result, err := VerifyQueryReferenceConnection(ctxT(t), f.request(t))
		if err != nil || result.NativeWitness == nil || result.NativeWitness.Assessment != AssessPass || result.NativeWitness.Verified != 3 {
			t.Fatalf("stored r3 activated another witness decoder: damaged=%v %v %+v native=%+v", damaged, err, result, result.NativeWitness)
		}
		if !damaged && (result.Assessment != AssessPass || result.Verified != 3) {
			t.Fatalf("stored r3 changed normal reference API: %+v", result)
		}
		if damaged {
			found := false
			for _, finding := range result.Findings {
				found = found || finding.Code == "REFERENCE_RESPONSE_INVALID"
			}
			if result.Assessment != AssessFail || !found {
				t.Fatalf("stored r3 changed malformed reference into an API error: %+v", result)
			}
		}
	}
}

func TestReplaySemanticSelectedDecoderOwnsErrorPriority(t *testing.T) {
	for _, schemaFirst := range []bool{false, true} {
		body := `"padding":` + strings.Repeat("[", 32) + "0" + strings.Repeat("]", 31) + "}"
		schema := `"result_schema":"tsgk-replay-result/r3"`
		data := []byte("{" + body + "," + schema + "}")
		if schemaFirst {
			data = []byte("{" + schema + "," + body + "}")
		}
		b := semanticTestBudget(t)
		x := &replayEnv{r: b.r}
		node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
		if e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": data}, x.finding); e == nil || e.Kind != KindResourceLimit || e.Code != "JSON_DEPTH_LIMIT" {
			t.Fatalf("later bracket error hid prior depth limit: schemafirst=%v %v %+v", schemaFirst, e, x.findings)
		}
	}
}

func TestReplaySemanticProbeKeepsDeepSyntaxAsFinding(t *testing.T) {
	for _, shared := range []bool{false, true} {
		for _, bad := range []string{"?", "?["} {
			data := []byte(`{"padding":` + strings.Repeat("[", 9999) + bad + strings.Repeat("]", 9999+strings.Count(bad, "[")) + `,"result_schema":"tsgk-replay-result/r1"}`)
			b := semanticTestBudget(t)
			if !shared {
				b.r.eligibility = nil
			}
			x := &replayEnv{r: b.r}
			node := &EvidenceNode{Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			if e := x.checkReplayFile(node, &EvidenceNode{}, map[string][]byte{"result.json": data}, x.finding); e != nil || len(x.findings) != 1 || x.findings[0].Code != "REPLAY_RESULT_INVALID" {
				t.Fatalf("syntax at the legacy depth boundary became a resource error: shared=%v bad=%q %v %+v", shared, bad, e, x.findings)
			}
		}
	}
}

func TestReplaySemanticProbeDoesNotAllocatePerRootName(t *testing.T) {
	data := []byte("{")
	for i := 0; i < 450000; i++ {
		data = append(data, []byte(`"prefix\u006b`)...)
		data = strconv.AppendInt(data, int64(i), 10)
		data = append(data, []byte(`":null,`)...)
	}
	data = append(data, []byte(`"\u0072esult_schema":"tsgk-replay-result/r3"}`)...)
	b := semanticTestBudget(t)
	b.r.eligibility = nil
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	revision, e := replayResultRevision(data, b.r)
	runtime.ReadMemStats(&after)
	if e != nil || revision != ReplaySemanticsResultSchema || b.r.eligibility != nil || after.TotalAlloc-before.TotalAlloc > 1<<20 {
		t.Fatalf("root key allocation before reservation: %q %v bytes=%d", revision, e, after.TotalAlloc-before.TotalAlloc)
	}
}

func TestReplaySemanticLegacySharedBudgetPreservesShapeLimits(t *testing.T) {
	for _, content := range []string{strings.Repeat("[", 40) + "0" + strings.Repeat("]", 40), "[" + strings.Repeat("{},", 999999) + "{}]"} {
		b := semanticTestBudget(t)
		data := []byte(`{"result_schema":"tsgk-replay-result/r1","evidence_mode":"REPLAYED_RAW","assessment":"PASS","reducer":{"id":"oracle-set-r2"},"subject":{"run":"original","attempt":1,"platform":"windows/amd64"},"padding":` + content + "}")
		x := &replayEnv{r: b.r}
		node := &EvidenceNode{EvidenceMode: ModeReplayedRaw, Assessment: AssessPass, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
		subject := &EvidenceNode{Identities: map[string]string{"run": "original", "attempt": "1", "platform": "windows/amd64"}}
		if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": data}, x.finding); e != nil || len(x.findings) != 0 || b.logical == 0 {
			t.Fatalf("shared legacy shape changed or was not reserved: %v %+v logical=%d", e, x.findings, b.logical)
		}
	}
	data := []byte(`{"` + strings.Repeat(`\u006b`, 1<<20) + `":?,"result_schema":"tsgk-replay-result/r1"}`)
	b := semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit - 32*uint64(len(data))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := b.reserveLegacyReplayResult(data)
	runtime.ReadMemStats(&after)
	if e == nil || e.Kind != KindResourceLimit || e.Path != "legacy-replay-buffers" || after.TotalAlloc-before.TotalAlloc > 1<<20 {
		t.Fatalf("legacy decoder/error allocated before reservation: %v heap=%d", e, after.TotalAlloc-before.TotalAlloc)
	}
}

func TestEvidenceFindingsStayUniqueAndBoundedOnFinalError(t *testing.T) {
	for _, duplicates := range []int{1, 1000} {
		t.Run(fmt.Sprintf("duplicates-%d", duplicates), func(t *testing.T) {
			root := t.TempDir()
			identities := map[string]string{}
			for _, role := range EvidenceRoles {
				identities[role] = "fixture-" + role
			}
			node := EvidenceNode{ID: "duplicate", Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass, Identities: identities, Files: []ReplayMember{}, Refs: []EvidenceRef{}}
			nodes := make([]EvidenceNode, duplicates+1)
			for i := range nodes {
				nodes[i] = node
			}
			graph := mustJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": nodes})
			if err := os.WriteFile(filepath.Join(root, "evidence.json"), graph, 0600); err != nil {
				t.Fatal(err)
			}
			policy := mustJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "bounded-final-error", "evidence_sha256": digestHex(graph), "required": []any{}, "carry_forward": []any{}})
			testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.OutputBytes = 1; return l }
			defer func() { testHookReplayLimits = nil }()
			result, err := VerifyEvidence(ctxT(t), EvidenceRequest{Root: root, Policy: policy})
			if err == nil || asError(err).Code != "OUTPUT_LIMIT" || result.Assessment != AssessFail || len(result.Findings) != min(duplicates+1, 1000) {
				t.Fatalf("final error duplicated findings: %v count=%d assessment=%s", err, len(result.Findings), result.Assessment)
			}
			seen := 0
			for _, finding := range result.Findings {
				if finding.Code == "NODE_DUPLICATE" {
					seen++
				}
			}
			if seen != min(duplicates, 999) || result.Findings[len(result.Findings)-1].Code != "OUTPUT_LIMIT" {
				t.Fatalf("first findings or final cause lost: duplicates=%d findings=%+v", seen, result.Findings)
			}
		})
	}
}

func TestReplaySemanticDirectorySwapWithSameFiles(t *testing.T) {
	for _, relative := range []string{"evidence", "evidence/oracle", "evidence/oracle/capture"} {
		t.Run(relative, func(t *testing.T) {
			reg, profile := semanticRegistrationFixture(t)
			registrationData, profile := semanticEncodeRegistration(t, reg, profile)
			graph := mustJSON(t, map[string]any{"schema": EvidenceSchema, "nodes": []EvidenceNode{}})
			policy := mustJSON(t, map[string]any{"schema": EvidencePolicySchema, "id": "directory-boundary", "evidence_sha256": digestHex(graph), "required": []any{}, "carry_forward": []any{}})
			files := map[string][]byte{"oracle-profile.json": semanticProfileBytes(t, profile), "registration.json": registrationData, "binding.json": []byte(`{}`), "approval.json": {}, "eligibility.json": {}, "witness.json": {}, "policy.json": policy, "evidence/evidence.json": graph, "evidence/oracle/capture/marker.bin": []byte("same physical file")}
			roles := map[string]string{"oracle-profile.json": "workload-profile", "registration.json": "semantics-registration", "binding.json": "semantics-run-binding", "approval.json": "eligibility-approval", "eligibility.json": "eligibility-registration", "witness.json": "query-registration", "policy.json": "evidence-policy"}
			rootPath := t.TempDir()
			members := writeSet(t, rootPath, files, func(path string) string {
				if role, ok := roles[path]; ok {
					return role
				}
				return "semantics-evidence"
			})
			root, err := os.OpenRoot(rootPath)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			before, err := root.Lstat("evidence/oracle/capture/marker.bin")
			if err != nil {
				t.Fatal(err)
			}
			moved := false
			testHookOpen = func(path string) {
				if path != "oracle-profile.json" || moved {
					return
				}
				moved = true
				source := filepath.Join(rootPath, filepath.FromSlash(relative))
				kept := filepath.Join(t.TempDir(), "saved-directory")
				if err := os.Rename(source, kept); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(kept)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if err := os.Rename(filepath.Join(kept, entry.Name()), filepath.Join(source, entry.Name())); err != nil {
						t.Fatal(err)
					}
				}
				after, err := root.Lstat("evidence/oracle/capture/marker.bin")
				if err != nil || !os.SameFile(before, after) {
					t.Fatalf("control changed file identity: %v", err)
				}
			}
			defer func() { testHookOpen = nil }()
			result, err := Replay(ctxT(t), ReplayRequest{Root: rootPath, Profile: mustJSON(t, registration("oracle-set-r4", "evidence-replay", AssessBlocked, map[string]string{}, []string{"c1"}, members))})
			if !moved || err == nil || asError(err).Code != "SOURCE_CHANGED" || result.Assessment == AssessPass {
				t.Fatalf("directory swap kept every file identity: %v %+v", err, result)
			}
			if !validSemanticReplayResult(result) {
				t.Fatalf("directory error lost partial result: %+v", result)
			}
		})
	}
}

func TestReplaySemanticResultRevisionAndCounters(t *testing.T) {
	for _, mutation := range []string{"complete", "partial", "overflow", "partition", "digest", "payload", "gate", "recomputed", "consumption", "false-pass", "lost-fail", "legacy-r1", "legacy-r2", "legacy-null", "legacy-empty", "outer-only-pass", "completed-unprocessed", "eligibility-count", "producer-fold-false", "producer-fold-missing", "producer-fold-null", "complete-missing", "complete-null", "outer-cancelled-pass", "subject-mode", "invalid-evidence-valid", "recorded-only", "before-core-blocked", "partial-base-status", "blocked-build-zero", "partial-build-zero"} {
		t.Run(mutation, func(t *testing.T) {
			r := semanticReplayResultControl()
			valid := mutation == "complete" || mutation == "partial" || mutation == "recorded-only"
			switch mutation {
			case "before-core-blocked":
				r.QuerySemantics = nil
				r.Gates = []ReplayGate{}
				r.Consumption = Consumption{}
				r.EvidenceValid = false
				r.Recomputed = Verdict{}
			case "partial-base-status", "partial-build-zero":
				r.QuerySemantics.Complete = false
				r.QuerySemantics.ExecutionStatus, r.QuerySemantics.Assessment = StatusCancelled, AssessNotAssessed
				r.ExecutionStatus, r.Assessment, r.EvidenceValid = StatusCancelled, AssessNotAssessed, false
				r.Gates = []ReplayGate{}
				r.Consumption = Consumption{}
				r.Recomputed = Verdict{StatusCancelled, AssessNotAssessed}
				if mutation == "partial-base-status" {
					r.QuerySemantics.BaseVerified.ExecutionStatus = "FORGED"
				} else {
					r.QuerySemantics.Eligibility.Build = QueryEligibilityCounts{}
				}
			case "blocked-build-zero":
				r.QuerySemantics.Eligibility.Build = QueryEligibilityCounts{}

			case "outer-only-pass":
				r.Assessment = AssessPass
			case "completed-unprocessed":
				r.QuerySemantics.Tuple.Blocked = 0
				r.QuerySemantics.Tuple.Unprocessed = 24
			case "eligibility-count":
				r.QuerySemantics.Eligibility.Build.Verified = 1
			case "producer-fold-false", "producer-fold-missing", "producer-fold-null":
				r.QuerySemantics.ProducerFoldValid = false
			case "complete-missing", "complete-null":
				r.QuerySemantics.Complete = false
			case "outer-cancelled-pass":
				r.ExecutionStatus = StatusCancelled
				r.Assessment = AssessPass
			case "subject-mode":
				r.Subject.EvidenceMode = "FORGED"
			case "invalid-evidence-valid":
				r.EvidenceValid = false
			case "recorded-only":
				r.EvidenceMode = ModeRecordedNotRecomputed
				r.Assessment = AssessUnresolved
				r.Recomputed = Verdict{StatusNotRun, AssessUnresolved}

			case "partial":
				r.ExecutionStatus, r.Assessment, r.EvidenceValid = StatusCancelled, AssessNotAssessed, false
				r.Gates = []ReplayGate{}
				r.Consumption = Consumption{}
				r.QuerySemantics.Complete = false
				r.QuerySemantics.ExecutionStatus, r.QuerySemantics.Assessment = StatusCancelled, AssessNotAssessed
				r.Recomputed = Verdict{StatusCancelled, AssessNotAssessed}
			case "overflow":
				r.QuerySemantics.Tuple.Verified = ^uint64(0)
			case "partition":
				r.QuerySemantics.Tuple.Unprocessed = 1
			case "digest":
				r.QuerySemantics.RegistrationSHA256 = ""
			case "payload":
				r.QuerySemantics = nil
			case "gate":
				r.Gates[0].Assessment = AssessPass
			case "recomputed":
				r.Recomputed.Assessment = AssessPass
			case "consumption":
				r.Consumption.Consumed = 0
			case "false-pass":
				r.Assessment = AssessPass
				r.QuerySemantics.Assessment = AssessPass
			case "lost-fail":
				r.QuerySemantics.ProducerRecorded.Assessment = AssessFail
			case "legacy-r1", "legacy-null", "legacy-empty":
				r.ResultSchema = ReplayResultSchema
			case "legacy-r2":
				r.ResultSchema = ReplayReferenceResultSchema
			}
			if validSemanticReplayResult(r) != valid {
				t.Fatalf("result %s validity: %+v", mutation, r)
			}
			data := mustJSON(t, r)
			if mutation == "legacy-null" || mutation == "legacy-empty" {
				var fields map[string]any
				if err := json.Unmarshal(data, &fields); err != nil {
					t.Fatal(err)
				}
				fields["query_semantics"] = nil
				if mutation == "legacy-empty" {
					fields["query_semantics"] = map[string]any{}
				}
				data = mustJSON(t, fields)
			}

			if strings.HasSuffix(mutation, "-missing") || strings.HasSuffix(mutation, "-null") && mutation != "legacy-null" {
				var fields map[string]any
				if err := json.Unmarshal(data, &fields); err != nil {
					t.Fatal(err)
				}
				key := "producer_fold_valid"
				if strings.HasPrefix(mutation, "complete-") {
					key = "complete"
				}
				payload := fields["query_semantics"].(map[string]any)
				if strings.HasSuffix(mutation, "-missing") {
					delete(payload, key)
				} else {
					payload[key] = nil
				}
				data = mustJSON(t, fields)
			}
			node := &EvidenceNode{ID: "replay", EvidenceMode: r.EvidenceMode, Assessment: r.Assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			subject := &EvidenceNode{ExecutionStatus: r.Subject.ExecutionStatus, EvidenceMode: r.Subject.EvidenceMode, Assessment: r.Subject.Assessment, Identities: map[string]string{"run": r.Subject.Run, "attempt": "1", "platform": r.Subject.Platform}}
			x := &replayEnv{}
			if e := x.checkReplayFile(node, subject, map[string][]byte{"result.json": data}, x.finding); e != nil {
				t.Fatal(e)
			}
			if (len(x.findings) == 0) != valid {
				t.Fatalf("stored reader %s: %+v", mutation, x.findings)
			}
		})
	}
}

func TestReplaySemanticBlockedBaseRequiresAllGates(t *testing.T) {
	for _, gate := range semanticReplayResultControl().QuerySemantics.Gates {
		t.Run(gate.ID, func(t *testing.T) {
			r := semanticReplayResultControl()
			if !validSemanticReplayResult(r) {
				t.Fatal("honest BLOCKED control rejected")
			}
			for i, g := range r.QuerySemantics.Gates {
				if g.ID == gate.ID {
					r.QuerySemantics.Gates = append(r.QuerySemantics.Gates[:i], r.QuerySemantics.Gates[i+1:]...)
					break
				}
			}
			if validSemanticReplayResult(r) {
				t.Fatal("base PASS accepted missing gate")
			}
		})
	}
}

func TestReplaySemanticFailureCodeIntegrity(t *testing.T) {
	r := semanticReplayResultControl()
	r.ExecutionStatus, r.Assessment, r.EvidenceValid = StatusCancelled, AssessFail, false
	r.Gates, r.Consumption = nil, Consumption{}
	p := r.QuerySemantics
	p.ExecutionStatus, p.Assessment, p.Complete, p.Code = StatusCancelled, AssessFail, false, "ORIGINAL_FAILURE"
	p.Findings = []Finding{{Severity: "error", Code: "ORIGINAL_FAILURE"}}
	r.Recomputed = Verdict{p.ExecutionStatus, p.Assessment}
	if !validSemanticReplayResult(r) {
		t.Fatal("honest partial failure rejected")
	}
	p.Findings = []Finding{{Severity: "error", Code: "ORIGINAL_FAILURE"}}
	p.Code = "FORGED_FIRST_CAUSE"
	if validSemanticReplayResult(r) {
		t.Fatal("partial failure code contradicted preserved finding")
	}
	p.Code = ""
	if validSemanticReplayResult(r) {
		t.Fatal("partial FAIL accepted without original code")
	}
	for _, field := range []string{"code", "record", "detail"} {
		t.Run(field, func(t *testing.T) {
			r := semanticReplayResultControl()
			switch field {
			case "code":
				r.Gates[0].Code = "FORGED"
			case "record":
				r.Gates[0].Record = "FORGED"
			case "detail":
				r.Gates[0].Detail = "FORGED"
			}
			if validSemanticReplayResult(r) {
				t.Fatal("nonfailed outer gate accepted failure fields")
			}
		})
	}
}

func TestReplaySemanticHiddenEligibilityFirstFailure(t *testing.T) {
	r := semanticReplayResultControl()
	r.ExecutionStatus, r.Assessment, r.EvidenceValid = StatusResourceLimit, AssessFail, false
	r.Gates, r.Consumption = nil, Consumption{}
	p := r.QuerySemantics
	p.ExecutionStatus, p.Assessment, p.Complete, p.Code = StatusResourceLimit, AssessFail, false, "EARLIER_ELIGIBILITY_FAILURE"
	r.Recomputed = Verdict{p.ExecutionStatus, p.Assessment}
	p.Eligibility.Assessment = AssessFail
	p.Eligibility.Findings = make([]Finding, 1000)
	for i := range p.Eligibility.Findings {
		p.Eligibility.Findings[i] = Finding{Severity: "warning", Code: "EARLIER_WARNING"}
	}
	p.Findings = []Finding{{Severity: "error", Code: "ELIGIBILITY_WORK_LIMIT"}}
	if !validSemanticReplayResult(r) {
		t.Fatal("hidden original failure replaced with later resource error")
	}
	p.Eligibility.Findings[999] = Finding{Severity: "error", Code: p.Code}
	if !validSemanticReplayResult(r) {
		t.Fatal("visible original eligibility failure rejected")
	}
	p.Code = "FORGED_FIRST_CAUSE"
	if validSemanticReplayResult(r) {
		t.Fatal("visible original eligibility failure contradicted")
	}
}

func TestReplaySemanticEligibilityCapWithActualInterruption(t *testing.T) {
	for _, kind := range []string{KindCancelled, KindResourceLimit} {
		t.Run(kind, func(t *testing.T) {
			r := semanticReplayResultControl()
			p := r.QuerySemantics
			session := eligibilitySession{res: p.Eligibility}
			for i := 0; i < 1000; i++ {
				session.finding("PRIOR_WARNING", "fixture", false)
			}
			session.finding("ELIGIBILITY_WITNESS_CONTEXT", "fixture", true)
			if session.firstFailure != "ELIGIBILITY_WITNESS_CONTEXT" || len(p.Eligibility.Findings) != 1000 {
				t.Fatal("fixture failed to hide confirmed first failure")
			}
			code := "CANCELLED"
			if kind == KindResourceLimit {
				code = "ELIGIBILITY_WORK_LIMIT"
			}
			interruption := fail(kind, code, "fixture", nil)
			failReport(&p.Eligibility.Report, interruption)
			p.Eligibility.Assessment = AssessFail
			if len(p.Eligibility.Findings) != 1001 || p.Eligibility.Findings[1000].Code != code {
				t.Fatal("actual failReport did not append interruption after cap")
			}
			p.ExecutionStatus, p.Assessment, p.Complete, p.Code = p.Eligibility.ExecutionStatus, AssessFail, false, session.firstFailure
			p.Findings = []Finding{p.Eligibility.Findings[1000]}
			r.ExecutionStatus, r.Assessment, r.EvidenceValid = p.ExecutionStatus, AssessFail, false
			r.Gates, r.Consumption = nil, Consumption{}
			r.Recomputed = Verdict{p.ExecutionStatus, p.Assessment}
			if !validSemanticReplayResult(r) {
				t.Fatal("later interruption replaced hidden first failure")
			}
			p.Eligibility.Findings[999] = Finding{Severity: "error", Code: session.firstFailure}
			if !validSemanticReplayResult(r) {
				t.Fatal("visible original cause before cap rejected")
			}
			p.Code = "FORGED_FIRST_CAUSE"
			if validSemanticReplayResult(r) {
				t.Fatal("visible cause before cap contradicted")
			}
		})
	}
}

func TestReplaySemanticCompleteErrorFindingContradiction(t *testing.T) {
	r := semanticReplayResultControl()
	r.QuerySemantics.Findings = []Finding{{Severity: "error", Code: "SEMANTICS_PROJECTION_BASE_INPUT"}}
	if validSemanticReplayResult(r) {
		t.Fatal("complete BLOCKED result retained confirmed semantic error")
	}
	p := r.QuerySemantics
	r.ExecutionStatus, p.ExecutionStatus = StatusCancelled, StatusCancelled
	r.Assessment, p.Assessment, r.EvidenceValid, p.Complete = AssessNotAssessed, AssessNotAssessed, false, false
	r.Gates, r.Consumption = nil, Consumption{}
	p.Code, p.Findings[0].Code = "CANCELLED", "CANCELLED"
	r.Recomputed = Verdict{p.ExecutionStatus, p.Assessment}
	if !validSemanticReplayResult(r) {
		t.Fatal("honest interrupted nonFAIL result lost its error finding")
	}
}

func TestReplaySemanticBlockedNestedErrors(t *testing.T) {
	for _, target := range []string{"outer", "outer-status", "eligibility", "evidence", "eligibility-interrupted", "eligibility-status-only"} {
		t.Run(target, func(t *testing.T) {
			r := semanticReplayResultControl()
			switch target {
			case "outer":
				r.Findings = []Finding{{Severity: "error", Code: "RECORDED_VERDICT_MISMATCH"}}
			case "outer-status":
				r.ExecutionStatus = StatusResourceLimit
			case "eligibility-status-only":
				r.QuerySemantics.Eligibility.ExecutionStatus = StatusResourceLimit
			case "eligibility", "eligibility-interrupted":
				r.QuerySemantics.Eligibility.Findings = []Finding{{Severity: "error", Code: "ELIGIBILITY_WITNESS_CONTEXT"}}
				if target == "eligibility-interrupted" {
					r.QuerySemantics.Eligibility.ExecutionStatus = StatusCancelled
					r.QuerySemantics.Eligibility.Assessment = AssessNotAssessed
				}
			case "evidence":
				r.QuerySemantics.Evidence.Assessment = AssessBlocked
				r.QuerySemantics.Evidence.Findings = []Finding{{Severity: "error", Code: "EVIDENCE_FAILED"}}
				copy := *r.QuerySemantics.Evidence
				r.QuerySemantics.Eligibility.Evidence = &copy
			}
			if validSemanticReplayResult(r) {
				t.Fatal("complete BLOCKED result hid a nested error")
			}
		})
	}
	t.Run("outer-interruption-after-complete-inner", func(t *testing.T) {
		r := semanticReplayResultControl()
		r.ExecutionStatus, r.Assessment, r.EvidenceValid = StatusResourceLimit, AssessBlocked, false
		r.Findings = []Finding{{Severity: "error", Code: "OUTPUT_LIMIT"}}
		if !validSemanticReplayResult(r) {
			t.Fatal("outer interruption erased honest complete inner result")
		}
	})
	for _, status := range []string{StatusCancelled, StatusResourceLimit} {
		t.Run("interrupted-eligibility-"+status, func(t *testing.T) {
			r := semanticReplayResultControl()
			p := r.QuerySemantics
			r.ExecutionStatus, p.ExecutionStatus, p.Eligibility.ExecutionStatus = status, status, status
			assessment := AssessNotAssessed
			if status == StatusResourceLimit {
				assessment = AssessBlocked
			}
			r.Assessment, p.Assessment, p.Eligibility.Assessment = assessment, assessment, assessment
			r.EvidenceValid, p.Complete, p.BindingValid, p.ProducerFoldValid = false, false, false, false
			r.Gates, r.Consumption = nil, Consumption{}
			p.Gates, p.BaseVerified, p.ProducerRecorded = nil, Verdict{StatusNotRun, AssessBlocked}, Verdict{}
			p.Tuple = QueryEligibilityCounts{Required: 24, Unprocessed: 24}
			p.Findings = []Finding{{Severity: "error", Code: "INTERRUPTED"}}
			p.Eligibility.Findings = []Finding{{Severity: "error", Code: "INTERRUPTED"}}
			p.Code = "INTERRUPTED"
			r.Recomputed = Verdict{p.ExecutionStatus, p.Assessment}
			if !validSemanticReplayResult(r) {
				t.Fatal("honest incomplete nested interruption rejected")
			}
		})
	}
	t.Run("recorded-only-before-payload", func(t *testing.T) {
		r := semanticReplayResultControl()
		r.QuerySemantics, r.Gates, r.Consumption = nil, nil, Consumption{}
		r.EvidenceMode, r.Assessment = ModeRecordedNotRecomputed, AssessUnresolved
		r.Recomputed = Verdict{StatusNotRun, AssessUnresolved}
		if !validSemanticReplayResult(r) {
			t.Fatal("recorded-only control rejected")
		}
		r.Findings = []Finding{{Severity: "error", Code: "RECORDED_VERDICT_MISMATCH"}}
		if validSemanticReplayResult(r) {
			t.Fatal("valid evidence with outer error bypassed missing payload")
		}
	})
}
