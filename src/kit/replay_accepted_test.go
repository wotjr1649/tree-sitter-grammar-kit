package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayReuseAcceptedPartialProofConsumer(t *testing.T) {
	root, reg := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
		for path, data := range files {
			if strings.HasPrefix(path, "sources/") {
				files[path] = []byte(strings.Repeat("x", len(data)))
			}
		}
		top["cases"].([]any)[0].(map[string]any)["retained_extra"] = make([]any, 8192)
	})
	var callbackAt int64
	for _, interrupt := range []bool{false, true} {
		at := int64(0)
		if interrupt {
			at = callbackAt - 1
		}
		wall := acceptedWall(t, at)
		parent, e := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
		if e != nil {
			t.Fatal(e)
		}
		defer parent.cancel()
		calls := 0
		var proof ReuseAcceptedResult
		var proofErr error
		res, err := replayShared(ctxT(t), ReplayRequest{Root: root, Profile: reg}, &replayBudget{r: parent, total: ReplayOperations()["evidence-replay"].TotalBytes,
			acceptedConsumer: func(_ ReuseAcceptedRequest, p ReuseAcceptedResult, cause error) error {
				calls++
				proof, proofErr = p, cause
				if !interrupt {
					callbackAt = wall.calls.Load()
					return nil
				}
				return errors.New("callback must not replace the original proof error")
			}})
		if calls != 1 || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" || len(proof.Steps) != 1 {
			t.Fatalf("partial proof lost: %+v cause=%v replay=%v", proof, proofErr, err)
		}
		if interrupt {
			if proofErr == nil || err != proofErr || asError(err).Code != "WALL_LIMIT" || res.Assessment != AssessFail {
				t.Fatalf("original error or known FAIL lost: %+v proofErr=%v err=%v", res, proofErr, err)
			}
		} else if err != nil || proofErr != nil || callbackAt < 2 {
			t.Fatalf("baseline: callbackAt=%d proofErr=%v err=%v", callbackAt, proofErr, err)
		}
	}
}

func acceptedLegacyRouteFailure(t *testing.T, files map[string][]byte, top map[string]any) {
	t.Helper()
	stdoutBytes := 0
	for path, data := range files {
		if strings.HasPrefix(path, "responses/") {
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			raw["steps"].([]any)[1].(map[string]any)["route"] = nil
			files[path], _ = json.Marshal(raw)
			stdoutBytes = len(files[path]) + 4
		}
	}
	c := top["cases"].([]any)[0].(map[string]any)
	c["steps"].([]any)[1].(map[string]any)["route"] = nil
	c["process"].(map[string]any)["stdout_bytes"] = stdoutBytes
	const code = "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"
	c["claims"].(map[string]string)["incremental_route"] = claimFail
	c["assessment"], c["code"], top["assessment"] = AssessFail, code, AssessFail
	top["summary"].(map[string]any)["assessments"] = map[string]int{AssessFail: 1}
	top["summary"].(map[string]any)["codes"] = map[string]int{code: 1}
}

func acceptedOmitRouteControl(t *testing.T, files map[string][]byte, top map[string]any) {
	t.Helper()
	c := top["cases"].([]any)[0].(map[string]any)
	var raw map[string]any
	path := "responses/00000-" + c["id"].(string) + ".json"
	if err := json.Unmarshal(files[path], &raw); err != nil {
		t.Fatal(err)
	}
	for k := 1; k < len(raw["steps"].([]any)); k++ {
		for _, m := range []map[string]any{raw, c} {
			o := acceptedObs(m, k, 0)
			o["old_tree"], o["old_source_sha256"] = false, nil
			counts := o["counts"].(map[string]any)
			counts["new_parse"], counts["parse_after_edit"], counts["accepted"] = 1, 0, 0
		}
	}
	req, _, _ := acceptedFixture(t, 12)
	req.Case.Input.Role = "case"
	code := "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_" + fmt.Sprint(req.Ledger.original(req.Case.ID).OmitOldFirst)
	c["assessment"], c["code"], c["reuse_observation_claim"] = AssessFail, code, claimFail
	c["claims"].(map[string]string)["incremental_route"] = claimFail
	setAcceptedEvidence(&req, raw, c)
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Result != claimFail {
		t.Fatalf("omit public proof: %+v %v", proof, err)
	}
	if result, err := checkReuseOmitControl(req); err != nil || result != claimPass {
		t.Fatalf("omit private control: %s %v", result, err)
	}
	c["reuse_accepted"], files[path] = proof, req.Raw
	top["assessment"] = AssessFail
	top["summary"].(map[string]any)["assessments"] = map[string]int{AssessFail: 1}
	top["summary"].(map[string]any)["codes"] = map[string]int{code: 1}
}

func TestReplayReuseAcceptedFailureBeforeLimit(t *testing.T) {
	for _, prior := range []string{"normal", "gate-fail", "case-fail"} {
		for _, limit := range []string{"total", "wall", "case-wall"} {
			t.Run(prior+"/"+limit, func(t *testing.T) {
				root, reg := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
					if prior == "gate-fail" {
						c := top["cases"].([]any)[0].(map[string]any)
						proof := c["reuse_accepted"].(ReuseAcceptedResult)
						proof.HistorySHA256 = strings.Repeat("0", 64)
						c["reuse_accepted"] = proof
					}
					if prior == "case-fail" {
						acceptedLegacyRouteFailure(t, files, top)
					}
				})
				var profile map[string]any
				if err := json.Unmarshal(reg, &profile); err != nil {
					t.Fatal(err)
				}
				if prior == "case-fail" {
					profile["subject"].(map[string]any)["assessment"] = AssessFail
				}
				tail := []byte("late member shares the same finite budget")
				if err := os.WriteFile(filepath.Join(root, "tail.bin"), tail, 0600); err != nil {
					t.Fatal(err)
				}
				profile["members"] = append(profile["members"].([]any), map[string]any{"path": "tail.bin", "role": "retained", "bytes": len(tail), "sha256": digestHex(tail)})
				reg, _ = json.Marshal(profile)
				baseline, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
				if err != nil || baseline.Assessment != map[bool]string{true: AssessPass, false: AssessFail}[prior == "normal"] {
					t.Fatalf("baseline: %+v %v", baseline, err)
				}
				wall, expire := context.WithCancel(context.Background())
				defer expire()
				testHookWall = func() (context.Context, context.CancelFunc) { return wall, func() {} }
				if limit == "total" {
					testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = baseline.BytesRead - 1; return l }
				} else {
					testHookOpen = func(path string) {
						if path == "tail.bin" {
							expire()
						}
					}
				}
				defer func() { testHookWall = nil; testHookOpen = nil; testHookReplayLimits = nil }()
				var res ReplayResult
				if limit == "case-wall" {
					parent, e := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
					if e != nil {
						t.Fatal(e)
					}
					defer parent.cancel()
					res, err = replayShared(ctxT(t), ReplayRequest{Root: root, Profile: reg}, &replayBudget{r: parent, total: ReplayOperations()["evidence-replay"].TotalBytes,
						acceptedConsumer: func(ReuseAcceptedRequest, ReuseAcceptedResult, error) error { expire(); return nil }})
				} else {
					res, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
				}
				code, assessment := "WALL_LIMIT", AssessBlocked
				if limit == "total" {
					code = "TOTAL_BYTES_LIMIT"
				}
				if prior != "normal" {
					assessment = AssessFail
				}
				if err == nil || asError(err).Code != code || res.Assessment != assessment || res.EvidenceValid || len(res.Gates) == 0 {
					t.Fatalf("late limit erased outcome: %+v %v", res, err)
				}
				if prior == "gate-fail" && gateOf(res, "reuse-accepted").Failed == 0 || prior == "case-fail" && res.Recomputed.Assessment != AssessFail {
					t.Fatalf("lost known failure cause: %+v", res)
				}
			})
		}
	}
}

func acceptedReplayFixture(t *testing.T, mutate func(map[string][]byte, map[string]any)) (string, []byte) {
	t.Helper()
	req, raw, projection := acceptedFixture(t, 12)
	ledger, _ := acceptedLedgerFixture(t)
	req.Case.Input.Role = "case"
	projection["input"] = req.Case.Input
	for _, v := range projection["steps"].([]any) {
		st := v.(map[string]any)
		for _, slot := range []string{"incremental", "fresh"} {
			if tr, ok := st[slot].(map[string]any); ok {
				tr["tree"].(map[string]any)["input"].(map[string]any)["encoding_source"] = SourceDeclaration
			}
		}
	}
	req.Raw, _ = json.Marshal(raw)
	setAcceptedEvidence(&req, raw, projection)
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Result != claimPass {
		t.Fatalf("fixture: %+v %v", proof, err)
	}
	projection["reuse_accepted"], projection["reuse_observation_claim"] = proof, claimPass
	projection["assessment"], projection["code"] = AssessPass, ""
	projection["claims"] = map[string]string{"incremental_equality": claimPass, "incremental_route": claimPass, "expectations": claimNotClaimed}
	projection["expectations"] = []any{}
	profile := newFxNative().profile
	profile["schema"], profile["route"] = IncrementalSchema, req.Route
	profile["cases"] = []any{map[string]any{"id": req.Case.ID, "input": req.Case.Input, "edits": req.Case.Edits, "points": []any{}, "expect": []any{}}}
	pdata, _ := json.Marshal(profile)
	top := map[string]any{"schema": ReportSchema, "result_schema": "tsgk-incremental-result/r7", "reuse_accepted_policy": ReuseAcceptedSchema, "reuse_accepted_ledger_sha256": ReuseAcceptedLedgerSHA256,
		"reuse_observation_policy": reuseobs.Policy, "observation_run_id": req.Expected.Observation.RunID,
		"execution_status": StatusCompleted, "assessment": AssessPass, "identities": []IdentityRef{req.Policy}, "route": req.Route,
		"operation": NativeOperations()["native-parse-edit"], "platform": req.Platform, "profile_sha256": digestHex(pdata),
		"build":   map[string]any{"identity": req.Expected.Observation.ProducerSHA256, "executable_sha256": fxExe, "compiler": map[string]any{"sha256": fxCompiler}, "runtime_commit": fxRuntime},
		"summary": map[string]any{"cases": 1, "execution_statuses": map[string]int{StatusCompleted: 1}, "assessments": map[string]int{AssessPass: 1}, "codes": map[string]int{}, "has_error": 1}, "cases": []any{projection}}
	files := map[string][]byte{"profile.json": pdata, "reuse-ledger.json": ledger, fmt.Sprintf("responses/00000-%s.json", req.Case.ID): req.Raw, fmt.Sprintf("sources/00000-%s.bin", req.Case.ID): req.Source}
	if mutate != nil {
		mutate(files, top)
	}
	files["result.json"], _ = json.Marshal(top)
	root := t.TempDir()
	members := writeSet(t, root, files, func(p string) string {
		switch p {
		case "profile.json":
			return "workload-profile"
		case "result.json":
			return "result"
		case "reuse-ledger.json":
			return "reuse-ledger"
		}
		if strings.HasPrefix(p, "sources/") {
			return "case-source"
		}
		return "response"
	})
	reg := registration("native-result-r8", "evidence-replay", AssessPass, map[string]string{"workload": digestHex(pdata), "policy": req.Policy.SHA256, "operation": "native-parse-edit", "platform": req.Platform,
		"producer": req.Expected.Observation.ProducerSHA256, "executable": fxExe, "compiler": fxCompiler, "runtime": fxRuntime}, []string{req.Case.ID}, members)
	b, _ := json.Marshal(reg)
	return root, b
}

func TestReplayReuseAccepted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		valid  bool
		mutate func(map[string][]byte, map[string]any)
	}{
		{"complete", true, nil},
		{"stored-proof", false, func(_ map[string][]byte, top map[string]any) {
			top["cases"].([]any)[0].(map[string]any)["reuse_accepted"].(ReuseAcceptedResult).Steps[0].Result = claimFail
		}},
		{"source", false, func(files map[string][]byte, _ map[string]any) {
			for p := range files {
				if strings.HasPrefix(p, "sources/") {
					files[p] = []byte("wrong")
				}
			}
		}},
		{"raw", false, func(files map[string][]byte, _ map[string]any) {
			for p, b := range files {
				if strings.HasPrefix(p, "responses/") {
					var m map[string]any
					_ = json.Unmarshal(b, &m)
					acceptedObs(m, 1, 1)["old_tree"] = true
					files[p], _ = json.Marshal(m)
				}
			}
		}},
		{"wrong-policy", false, func(_ map[string][]byte, top map[string]any) { top["reuse_accepted_policy"] = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, reg := acceptedReplayFixture(t, tc.mutate)
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err != nil {
				if tc.valid {
					t.Fatal(err)
				}
				return
			}
			if r.EvidenceValid != tc.valid || tc.valid && (r.Assessment != AssessPass || r.Recomputed.Assessment != AssessPass) {
				t.Fatalf("valid=%v assessment=%s findings=%v gates=%+v", r.EvidenceValid, r.Assessment, codes(r.Findings), r.Gates)
			}
		})
	}
}

func TestReplayReuseAcceptedLegacyPresence(t *testing.T) {
	for _, value := range []any{nil, "", map[string]any{}} {
		f := mandatoryFixture(t, true, false, claimBlocked, AssessBlocked)
		f.cases[1]["reuse_accepted"] = value
		root, reg := writeMandatoryFixture(t, f, "tsgk-incremental-result/r3")
		r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
		if err == nil && r.EvidenceValid {
			t.Fatalf("legacy accepted new field presence: %#v", value)
		}
	}
}
