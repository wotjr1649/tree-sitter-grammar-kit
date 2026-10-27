package kit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestQualifyReuseAcceptedInvalidStoredProofKeepsTargets(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "evidence-invalid", true: "replay-error"}[malformed], func(t *testing.T) {
			root, _ := acceptedReplayFixture(t, func(_ map[string][]byte, top map[string]any) {
				c := top["cases"].([]any)[0].(map[string]any)
				proof := c["reuse_accepted"].(ReuseAcceptedResult)
				if malformed {
					proof.Schema = "invalid"
				} else {
					proof.HistorySHA256 = strings.Repeat("0", 64)
				}
				c["reuse_accepted"] = proof
			})
			q, w, p := acceptedReaderQualifier(t, root, false)
			s, err := q.evalSymbolSet(w, p)
			if err != nil {
				t.Fatal(err)
			}
			a := s.ReuseAccepted
			if s.Mechanism != AssessFail || a.Originals.Failed != 1 || a.Targets.Passed != 1 || a.Targets.Failed != 0 || a.Targets.Missing != 0 {
				t.Fatalf("stored integrity failure erased recomputed target: %+v %+v", s, a)
			}
		})
	}
}

func TestReuseAcceptedProjectionOnlyZero(t *testing.T) {
	for _, name := range []string{"positive", "zero", "zero-missing-source", "zero-wrong-source", "incomplete-zero"} {
		t.Run(name, func(t *testing.T) {
			req, _, projection := acceptedFixture(t, 12)
			if name != "positive" {
				acceptedObs(projection, 1, 0)["counts"].(map[string]any)["accepted"] = 0
			}
			switch name {
			case "zero-missing-source":
				req.Source = nil
			case "zero-wrong-source":
				req.Source = []byte("wrong")
			case "incomplete-zero":
				acceptedObs(projection, 1, 0)["status"] = "TRUNCATED"
				acceptedObs(projection, 1, 0)["reason"] = "MESSAGE_LIMIT"
			}
			req.Projection, _ = json.Marshal(projection)
			proof, err := JudgeReuseAccepted(req)
			want := claimBlocked
			zero := strings.HasPrefix(name, "zero")
			if zero {
				want = claimFail
			}
			code := "REUSE_ACCEPTED_ZERO"
			if name == "zero-wrong-source" {
				code = "REUSE_ACCEPTED_SOURCE_MISMATCH"
			}
			if err != nil || proof.Result != want || proof.Steps[0].Result != want || zero && (proof.Code != code || proof.Steps[0].Code != "REUSE_ACCEPTED_ZERO") {
				t.Fatalf("projection-only %s: %+v %v", name, proof, err)
			}
		})
	}
}

func TestReplayReuseAcceptedUnsubmittedSource(t *testing.T) {
	root, reg := acceptedReplayFixture(t, func(files map[string][]byte, _ map[string]any) {
		for path := range files {
			if strings.HasPrefix(path, "sources/") {
				delete(files, path)
			}
		}
	})
	parent, e := startRun(ctxT(t), ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	defer parent.cancel()
	calls := 0
	_, err := replayShared(ctxT(t), ReplayRequest{Root: root, Profile: reg}, &replayBudget{r: parent, total: ReplayOperations()["evidence-replay"].TotalBytes,
		acceptedConsumer: func(req ReuseAcceptedRequest, proof ReuseAcceptedResult, cause error) error {
			calls++
			if cause != nil || req.Source != nil || proof.Result != claimBlocked || proof.Code != "REUSE_ACCEPTED_EVIDENCE_MISSING" || proof.Steps[0].Result != claimBlocked {
				t.Fatalf("unsubmitted source called contradiction: %+v cause=%v source=%v", proof, cause, req.Source)
			}
			return nil
		}})
	if err != nil || calls != 1 {
		t.Fatalf("proof consumer calls=%d err=%v", calls, err)
	}
}

func TestReuseAcceptedLateZeroPreservesEarlierTargets(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 0)
	h := req.Ledger.original(req.Case.ID)
	last := h.BlockedSteps[len(h.BlockedSteps)-1]
	for _, data := range []map[string]any{raw, projection} {
		acceptedObs(data, last, 0)["counts"].(map[string]any)["accepted"] = 0
	}
	setAcceptedEvidence(&req, raw, projection)
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_ZERO" || len(proof.Steps) < 2 || proof.Steps[0].Result != claimPass || proof.Steps[len(proof.Steps)-1].Result != claimFail {
		t.Fatalf("later ZERO erased earlier target: %+v %v", proof, err)
	}
}

func TestReplayReuseAcceptedDeclaredSourceIntegrity(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "hash-mismatch", true: "file-missing"}[missing], func(t *testing.T) {
			root, reg := acceptedReplayFixture(t, nil)
			var profile ReplayProfile
			if err := json.Unmarshal(reg, &profile); err != nil {
				t.Fatal(err)
			}
			for _, member := range profile.Members {
				if member.Role != "case-source" {
					continue
				}
				path := filepath.Join(root, filepath.FromSlash(member.Path))
				var err error
				if missing {
					err = os.Remove(path)
				} else {
					err = os.WriteFile(path, []byte(strings.Repeat("x", int(member.Bytes))), 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if r.EvidenceValid || r.Assessment != AssessFail {
				t.Fatalf("declared source corruption was treated as unsubmitted: %+v %v", r, err)
			}
		})
	}
}

func TestReuseAcceptedEarlyTargetFailureCode(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 12)
	for _, data := range []map[string]any{raw, projection} {
		observation := acceptedObs(data, 1, 0)
		observation["old_tree"], observation["old_source_sha256"] = false, nil
		counts := observation["counts"].(map[string]any)
		counts["new_parse"], counts["parse_after_edit"], counts["accepted"] = 1, 0, 0
	}
	steps := projection["steps"]
	acceptedEarlyProcess(projection)
	projection["steps"] = steps
	setAcceptedEvidence(&req, raw, projection)
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH" || proof.Steps[0].Result != claimFail || proof.Steps[0].Code != proof.Code {
		t.Fatalf("later ZERO replaced earlier target failure: %+v %v", proof, err)
	}
}

func TestReuseAcceptedUnsubmittedProjection(t *testing.T) {
	for _, name := range []string{"missing", "missing-multiple", "missing-source", "zero", "wrong-source", "invalid-raw", "null-process", "empty-projection", "malformed-projection", "argument", "cst"} {
		t.Run(name, func(t *testing.T) {
			index := 12
			if name == "missing-multiple" {
				index = 0
			}
			req, raw, projection := acceptedFixture(t, index)
			if name == "zero" {
				acceptedObs(raw, 1, 0)["counts"].(map[string]any)["accepted"] = 0
			}
			if name == "null-process" {
				projection["process"] = nil
			}
			if name == "argument" {
				observation := acceptedObs(raw, 1, 0)
				observation["old_tree"], observation["old_source_sha256"] = false, nil
				counts := observation["counts"].(map[string]any)
				counts["new_parse"], counts["parse_after_edit"], counts["accepted"] = 1, 0, 0
			}
			if name == "cst" {
				data, _ := json.Marshal(acceptedStep(projection, 1)["fresh"].(map[string]any)["tree"].(map[string]any)["nodes"])
				var nodes []TreeNode
				if err := json.Unmarshal(data, &nodes); err != nil {
					t.Fatal(err)
				}
				nodes[0].Type = "other"
				fresh := acceptedStep(raw, 1)["fresh"].(map[string]any)
				fresh["types"], fresh["digest"] = []string{"other"}, TreeDigest(nodes)
			}
			setAcceptedEvidence(&req, raw, projection)
			req.Projection = nil
			switch name {
			case "missing-source":
				req.Source = nil
			case "wrong-source":
				req.Source = []byte("wrong")
			case "invalid-raw":
				req.Raw = []byte("{")
			case "null-process":
				req.Projection, _ = json.Marshal(projection)
			case "empty-projection":
				req.Projection = []byte{}
			case "malformed-projection":
				req.Projection = []byte("{")
			}
			want := claimFail
			if strings.HasPrefix(name, "missing") {
				want = claimBlocked
			}
			proof, err := JudgeReuseAccepted(req)
			if err != nil || proof.Result != want || want == claimBlocked && (proof.Code != "REUSE_ACCEPTED_EVIDENCE_MISSING" || proof.Steps[0].Result != claimBlocked) || name == "zero" && (proof.Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[0].Result != claimFail) {
				t.Fatalf("projection %s: %+v %v", name, proof, err)
			}
			for _, step := range proof.Steps {
				if want == claimBlocked && step.Result != claimBlocked {
					t.Fatalf("missing projection manufactured target completion: %+v", proof)
				}
			}
			if name == "cst" && proof.Code != "REUSE_ACCEPTED_CST_MISMATCH" || name == "argument" && proof.Code != "REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH" {
				t.Fatalf("raw contradiction was lost: %+v", proof)
			}
		})
	}
}

func TestQualifyReuseAcceptedOmitMissingRoute(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive", true: "first-route"}[missing], func(t *testing.T) {
			root, _ := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
				acceptedOmitRouteControl(t, files, top)
				if !missing {
					return
				}
				c := top["cases"].([]any)[0].(map[string]any)
				acceptedStep(c, 1)["route"] = nil
				for path, data := range files {
					if strings.HasPrefix(path, "responses/") {
						var raw map[string]any
						if err := json.Unmarshal(data, &raw); err != nil {
							t.Fatal(err)
						}
						acceptedStep(raw, 1)["route"] = nil
						b, err := json.Marshal(raw)
						if err != nil {
							t.Fatal(err)
						}
						files[path] = b
						c["process"].(map[string]any)["stdout_bytes"] = len(b) + 4
					}
				}
			})
			q, w, p := acceptedReaderQualifier(t, root, false)
			w.ReuseAccepted.Control = "omit-old-tree"
			s, err := q.evalSymbolSet(w, p)
			if err != nil {
				t.Fatal(err)
			}
			want := claimPass
			if missing {
				want = claimFail
			}
			if s.ReuseAccepted.Cases[0].Result != want || missing && s.Mechanism != AssessFail {
				t.Fatalf("omit route corruption not recomputed: %+v", s)
			}
		})
	}
}

func TestQualifyReuseAcceptedOmitFailureBeforeDepthLimit(t *testing.T) {
	for _, damage := range []string{"none", "source", "fresh-count"} {
		t.Run(damage, func(t *testing.T) {
			root, _ := acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
				acceptedOmitRouteControl(t, files, top)
				c := top["cases"].([]any)[0].(map[string]any)
				if damage == "fresh-count" {
					acceptedObs(c, 1, 1)["counts"].(map[string]any)["accepted"] = 1
					c["execution_status"], c["code"] = StatusResourceLimit, "MESSAGE_LIMIT"
					process := c["process"].(map[string]any)
					process["status"], process["reason"], process["exit_code"] = StatusResourceLimit, "MESSAGE_LIMIT", 2
				}
				for path, data := range files {
					if strings.HasPrefix(path, "responses/") {
						deep := []byte(strings.Repeat("[", jsonMaxDepth+1) + "0" + strings.Repeat("]", jsonMaxDepth+1))
						files[path] = deep
						c["process"].(map[string]any)["stdout_bytes"] = len(deep) + 4
					}
					if damage == "source" && strings.HasPrefix(path, "sources/") {
						files[path] = []byte(strings.Repeat("x", len(data)))
					}
				}
			})
			q, w, p := acceptedReaderQualifier(t, root, false)
			w.ReuseAccepted.Control = "omit-old-tree"
			_, err := q.evalSymbolSet(w, p)
			if err == nil || err.Kind != KindResourceLimit || err.Code != "JSON_DEPTH_LIMIT" {
				t.Fatalf("typed depth limit lost: %v", err)
			}
			s := q.acceptedSets[p.ID+"/"+w.Set]
			a := s.ReuseAccepted
			if damage != "none" {
				if a.Controls.Failed != 1 || a.Controls.Blocked != 0 {
					t.Fatalf("independent omit failure lost: %+v", a)
				}
			} else if a.Controls.Failed != 0 || a.Controls.Blocked != 1 {
				t.Fatalf("expected public omit failure became independent FAIL: %+v", a)
			}
		})
	}
}

func TestReuseAcceptedOmitUnsubmittedMembers(t *testing.T) {
	for _, member := range []string{"source", "raw", "projection", "all"} {
		t.Run(member, func(t *testing.T) {
			acceptedReplayFixture(t, func(files map[string][]byte, top map[string]any) {
				acceptedOmitRouteControl(t, files, top)
				c := top["cases"].([]any)[0].(map[string]any)
				var raw map[string]any
				path := "responses/00000-" + c["id"].(string) + ".json"
				if err := json.Unmarshal(files[path], &raw); err != nil {
					t.Fatal(err)
				}
				req, _, _ := acceptedFixture(t, 12)
				req.Case.Input.Role = "case"
				setAcceptedEvidence(&req, raw, c)
				if member == "source" || member == "all" {
					req.Source = nil
				}
				if member == "raw" || member == "all" {
					req.Raw = nil
				}
				if member == "projection" || member == "all" {
					req.Projection = nil
				}
				proof, err := judgeReuseAccepted(req, true)
				if err != nil || proof.Result != claimBlocked {
					t.Fatalf("unsubmitted omit %s became contradiction: %+v %v", member, proof, err)
				}
			})
		})
	}
}

func TestReuseAcceptedOmitLaterRoute(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive", true: "later-route"}[missing], func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			for k := 1; k < len(raw["steps"].([]any)); k++ {
				for _, m := range []map[string]any{raw, projection} {
					o := acceptedObs(m, k, 0)
					o["old_tree"], o["old_source_sha256"] = false, nil
					counts := o["counts"].(map[string]any)
					counts["new_parse"], counts["parse_after_edit"], counts["accepted"] = 1, 0, 0
				}
			}
			projection["assessment"], projection["reuse_observation_claim"] = AssessFail, claimFail
			projection["claims"] = map[string]string{"incremental_equality": claimPass, "incremental_route": claimFail}
			projection["code"] = "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1"
			if missing {
				acceptedStep(raw, 2)["route"], acceptedStep(projection, 2)["route"] = nil, nil
			}
			setAcceptedEvidence(&req, raw, projection)
			proof, err := judgeReuseAccepted(req, true)
			want := claimPass
			if missing {
				want = claimFail
			}
			if err != nil || proof.Result != want {
				t.Fatalf("later route corruption: %+v %v", proof, err)
			}
		})
	}
}

func TestReuseAcceptedProjectionOnlyCST(t *testing.T) {
	for _, damage := range []string{"none", "incremental-type", "fresh-type", "rehashed-mismatch", "matching-rehashed", "points", "parent", "both-parent", "input", "identity", "empty", "missing-tree", "summary", "missing-source-mismatch"} {
		t.Run(damage, func(t *testing.T) {
			req, _, projection := acceptedFixture(t, 12)
			step := acceptedStep(projection, 1)
			slot := "incremental"
			if damage == "fresh-type" {
				slot = "fresh"
			}
			tr := step[slot].(map[string]any)
			tree := tr["tree"].(map[string]any)
			switch damage {
			case "incremental-type", "fresh-type", "rehashed-mismatch", "matching-rehashed", "missing-source-mismatch":
				tree["nodes"].([]any)[0].(map[string]any)["type"] = "changed"
			case "points":
				tree["nodes"].([]any)[0].(map[string]any)["end_point"].(map[string]any)["column"] = 999
			case "parent", "both-parent":
				tree["nodes"].([]any)[0].(map[string]any)["parent"] = 0
			case "input":
				tree["input"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "identity":
				tree["identities"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "empty":
				tree["nodes"] = []any{}
			case "missing-tree":
				tr["tree"] = nil
			case "summary":
				tr["form"], tr["tree"] = "record", nil
			}
			if damage == "rehashed-mismatch" || damage == "matching-rehashed" || damage == "points" || damage == "parent" || damage == "both-parent" {
				b, _ := json.Marshal(tree["nodes"])
				var nodes []TreeNode
				if err := json.Unmarshal(b, &nodes); err != nil {
					t.Fatal(err)
				}
				tr["digest"] = TreeDigest(nodes)
				if damage == "matching-rehashed" || damage == "both-parent" {
					fresh := step["fresh"].(map[string]any)
					fresh["tree"].(map[string]any)["nodes"] = tree["nodes"]
					fresh["digest"] = tr["digest"]
				}
			}
			if damage == "missing-source-mismatch" {
				req.Source = nil
			}
			req.Projection, _ = json.Marshal(projection)
			proof, err := JudgeReuseAccepted(req)
			want := claimFail
			if damage == "none" || damage == "matching-rehashed" || damage == "summary" {
				want = claimBlocked
			}
			if err != nil || proof.Result != want || proof.Steps[0].Result != want {
				t.Fatalf("projection CST %s: %+v %v", damage, proof, err)
			}
		})
	}
}

func TestReuseAcceptedProjectionOnlyBindings(t *testing.T) {
	for _, damage := range []string{"step-index", "step-source", "edit", "restarted", "encoding", "capability", "process", "route-proven", "route-end", "comparison"} {
		t.Run(damage, func(t *testing.T) {
			req, _, projection := acceptedFixture(t, 12)
			step := acceptedStep(projection, 1)
			switch damage {
			case "step-index":
				step["step"] = 99
			case "step-source":
				step["source_sha256"] = strings.Repeat("0", 64)
			case "edit":
				step["edit"].(map[string]any)["start_byte"] = 999
			case "restarted":
				step["restarted"] = true
			case "encoding":
				projection["encoding"] = EncodingUTF16LE
			case "capability":
				projection["producer"].(map[string]any)["reuse_observation"].(map[string]any)["policy"] = "wrong"
			case "process":
				projection["process"].(map[string]any)["exit_code"] = 1
			case "route-proven":
				step["route"].(map[string]any)["proven"] = true
			case "route-end":
				step["route"].(map[string]any)["edited_root_end_byte"] = 0
			case "comparison":
				step["comparison"].(map[string]any)["equal"] = false
			}
			req.Projection, _ = json.Marshal(projection)
			proof, err := JudgeReuseAccepted(req)
			if err != nil || proof.Result != claimFail {
				t.Fatalf("projection binding %s hidden: %+v %v", damage, proof, err)
			}
		})
	}
}

func TestReuseAcceptedProjectionOnlyEnvelope(t *testing.T) {
	for _, damage := range []string{"schema", "status", "count", "has-error", "source-identity", "policy-identity", "null-schema", "missing-bytes", "null-identities"} {
		t.Run(damage, func(t *testing.T) {
			req, _, projection := acceptedFixture(t, 12)
			tr := acceptedStep(projection, 1)["incremental"].(map[string]any)
			tree := tr["tree"].(map[string]any)
			switch damage {
			case "schema":
				tree["schema"] = "wrong"
			case "status":
				tree["status"] = StatusFailed
			case "count":
				tr["descendant_count"] = 2
			case "has-error":
				tr["has_error"] = false
			case "source-identity", "policy-identity":
				index := 1
				if damage == "policy-identity" {
					index = 2
				}
				tree["identities"].([]any)[index].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			case "null-schema":
				tree["schema"] = nil
			case "missing-bytes":
				delete(tree["input"].(map[string]any), "bytes")
			case "null-identities":
				tree["identities"] = nil
			}
			req.Projection, _ = json.Marshal(projection)
			proof, err := JudgeReuseAccepted(req)
			if err != nil || proof.Result != claimFail {
				t.Fatalf("envelope %s: %+v %v", damage, proof, err)
			}
		})
	}
}

func TestReuseAcceptedProjectionOnlyHonestMismatch(t *testing.T) {
	req, _, projection := acceptedFixture(t, 12)
	step := acceptedStep(projection, 1)
	incremental := step["incremental"].(map[string]any)
	tree := incremental["tree"].(map[string]any)
	tree["nodes"].([]any)[0].(map[string]any)["type"] = "changed"
	var a, b []TreeNode
	v, _ := json.Marshal(tree["nodes"])
	if err := json.Unmarshal(v, &a); err != nil {
		t.Fatal(err)
	}
	v, _ = json.Marshal(step["fresh"].(map[string]any)["tree"].(map[string]any)["nodes"])
	if err := json.Unmarshal(v, &b); err != nil {
		t.Fatal(err)
	}
	incremental["digest"] = TreeDigest(a)
	step["comparison"] = map[string]any{"equal": false, "first_difference": CompareTrees(a, b)}
	req.Projection, _ = json.Marshal(projection)
	calls := 0
	req.Expected.Check = func() error { calls++; return nil }
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Code != "REUSE_ACCEPTED_CST_MISMATCH" || proof.Steps[0].Result != claimFail {
		t.Fatalf("honest mismatch: %+v %v", proof, err)
	}
	stopAt := calls - 1
	calls = 0
	sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
	req.Expected.Check = func() error {
		calls++
		if calls >= stopAt {
			return sentinel
		}
		return nil
	}
	proof, err = JudgeReuseAccepted(req)
	if !errors.Is(err, sentinel) || proof.Code != "REUSE_ACCEPTED_CST_MISMATCH" || proof.Steps[0].Result != claimFail {
		t.Fatalf("interrupted mismatch: %+v %v", proof, err)
	}
	req.Source = []byte("wrong")
	req.Expected.Check = nil
	proof, err = JudgeReuseAccepted(req)
	if err != nil || proof.Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" || proof.Steps[0].Result != claimFail {
		t.Fatalf("first failure: %+v %v", proof, err)
	}
}

func TestReuseAcceptedLateProjectionCST(t *testing.T) {
	for _, damage := range []string{"type", "null-schema", "string-input", "missing-bytes", "null-node", "string-edit", "early-null-schema", "early-fresh-count", "early-positive"} {
		t.Run(damage, func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			h := req.Ledger.original(req.Case.ID)
			last := h.BlockedSteps[len(h.BlockedSteps)-1]
			tree := acceptedStep(projection, last)["incremental"].(map[string]any)["tree"].(map[string]any)
			switch damage {
			case "type":
				tree["nodes"].([]any)[0].(map[string]any)["type"] = "changed"
			case "null-schema", "early-null-schema":
				tree["schema"] = nil
			case "string-input":
				tree["input"] = "wrong"
			case "null-node":
				tree["nodes"].([]any)[0].(map[string]any)["extra"] = nil
			case "string-edit":
				acceptedStep(projection, last)["edit"] = "wrong"
			case "missing-bytes":
				delete(tree["input"].(map[string]any), "bytes")
			}
			if strings.HasPrefix(damage, "early-") {
				projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
				projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
				if damage == "early-fresh-count" {
					acceptedObs(projection, last, 1)["counts"].(map[string]any)["accepted"] = 1
				}
			}
			setAcceptedEvidence(&req, raw, projection)
			calls := 0
			req.Expected.Check = func() error { calls++; return nil }
			proof, err := JudgeReuseAccepted(req)
			want, lastWant := claimFail, claimFail
			if damage == "early-positive" {
				want, lastWant = claimBlocked, claimPass
			}
			if err != nil || proof.Result != want || proof.Steps[0].Result != claimPass || proof.Steps[len(proof.Steps)-1].Result != lastWant {
				t.Fatalf("late CST erased earlier target: %+v %v", proof, err)
			}
			stopAt := calls
			calls = 0
			sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
			req.Expected.Check = func() error {
				calls++
				if calls >= stopAt {
					return sentinel
				}
				return nil
			}
			interrupted, err := JudgeReuseAccepted(req)
			if !errors.Is(err, sentinel) || interrupted.Code != proof.Code || interrupted.Steps[0].Result != claimPass || interrupted.Steps[len(interrupted.Steps)-1].Result != lastWant {
				t.Fatalf("late shape interrupted: %+v %v", interrupted, err)
			}

		})
	}
}

func TestReuseAcceptedProjectionOnlyMetadata(t *testing.T) {
	for _, damage := range []string{"valid-r4", "r3-api", "r4-api", "r4-predicates", "response-status", "response-code", "tree-form", "tree-status", "partial-count", "partial-digest"} {
		t.Run(damage, func(t *testing.T) {
			req, _, projection := acceptedFixture(t, 12)
			producer := projection["producer"].(map[string]any)
			if damage == "valid-r4" || damage == "r4-api" || damage == "r4-predicates" {
				req.Expected.Revision = "tsgk-native/r4"
				producer["query"] = "tsgk-query/r1"
				producer["api"] = "tsgk-api/r1"
				producer["predicates"] = "NOT_EVALUATED"
				producer["symbol_count"] = 10
				producer["field_count"] = 0
			}
			tr := acceptedStep(projection, 1)["incremental"].(map[string]any)
			switch damage {
			case "r3-api":
				producer["api"] = "tsgk-api/r1"
			case "r4-api":
				delete(producer, "api")
			case "r4-predicates":
				producer["predicates"] = "EVALUATED"
			case "response-status":
				projection["response_status"] = "FAILED"
				projection["response_code"] = "PRODUCER_FAILED"
			case "response-code":
				projection["response_code"] = "WRONG"
			case "tree-form":
				tr["form"] = "bogus"
				tr["tree"] = nil
			case "tree-status":
				tr["status"] = "bogus"
				tr["form"] = "record"
				tr["tree"] = nil
			case "partial-count":
				tr["form"] = "record"
				tr["tree"] = nil
				tr["descendant_count"] = 0
			case "partial-digest":
				tr["form"] = "record"
				tr["tree"] = nil
				tr["digest"] = ""
			}
			req.Projection, _ = json.Marshal(projection)
			proof, err := JudgeReuseAccepted(req)
			want := claimFail
			if damage == "valid-r4" {
				want = claimBlocked
			}
			if err != nil || proof.Result != want {
				t.Fatalf("projection metadata %s: %+v %v", damage, proof, err)
			}
		})
	}
}

func TestReuseAcceptedEarlyPrefixFailures(t *testing.T) {
	for _, damage := range []string{"source", "initial", "initial-extra", "first-target"} {
		t.Run(damage, func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
			projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
			h := req.Ledger.original(req.Case.ID)
			last := h.BlockedSteps[len(h.BlockedSteps)-1]
			acceptedStep(projection, last)["incremental"].(map[string]any)["tree"].(map[string]any)["schema"] = nil
			switch damage {
			case "source":
				req.Source = []byte("wrong")
			case "initial-extra":
				acceptedObs(projection, 0, 0)["unexpected"] = true
			case "initial":
				acceptedStep(projection, 0)["incremental"].(map[string]any)["tree"].(map[string]any)["schema"] = nil
			case "first-target":
				acceptedObs(projection, 1, 1)["counts"].(map[string]any)["accepted"] = 1
			}
			setAcceptedEvidence(&req, raw, projection)
			proof, err := JudgeReuseAccepted(req)
			want := claimBlocked
			if damage == "first-target" {
				want = claimFail
			}
			if err != nil || proof.Result != claimFail || proof.Steps[0].Result != want || proof.Steps[len(proof.Steps)-1].Result != claimFail || damage == "source" && proof.Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" {
				t.Fatalf("early prefix failure %s: %+v %v", damage, proof, err)
			}
		})
	}
}

func TestReuseAcceptedProjectionObservationShape(t *testing.T) {
	for _, step := range []int{0, 1, 2} {
		req, raw, projection := acceptedFixture(t, 0)
		acceptedObs(projection, step, 0)["unexpected"] = true
		setAcceptedEvidence(&req, raw, projection)
		proof, err := JudgeReuseAccepted(req)
		first := claimBlocked
		if step == 1 {
			first = claimFail
		}
		if step == 2 {
			first = claimPass
		}
		if err != nil || proof.Result != claimFail || proof.Steps[0].Result != first {
			t.Fatalf("projection extra step%d: %+v %v", step, proof, err)
		}
	}
}

func TestReuseAcceptedRawInterruptedTarget(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 12)
	for _, m := range []map[string]any{raw, projection} {
		o := acceptedObs(m, 1, 0)
		o["old_tree"], o["old_source_sha256"] = false, nil
		c := o["counts"].(map[string]any)
		c["new_parse"], c["parse_after_edit"], c["accepted"] = 1, 0, 0
	}
	setAcceptedEvidence(&req, raw, projection)
	calls := 0
	req.Expected.Check = func() error { calls++; return nil }
	baseline, err := JudgeReuseAccepted(req)
	if err != nil || baseline.Code != "REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH" {
		t.Fatalf("baseline: %+v %v", baseline, err)
	}
	total, seen := calls, 0
	if total > 256 {
		t.Fatalf("bounded checkpoint probe exceeded256: %d", total)
	}
	for stopAt := 1; stopAt <= total; stopAt++ {
		calls = 0
		sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
		req.Expected.Check = func() error {
			calls++
			if calls >= stopAt {
				return sentinel
			}
			return nil
		}
		proof, err := JudgeReuseAccepted(req)
		if err != nil && !errors.Is(err, sentinel) {
			t.Fatalf("original interruption lost: %v", err)
		}
		if err != nil && proof.Code == "REUSE_ACCEPTED_PARSE_ARGUMENT_MISMATCH" {
			seen++
			if proof.Result != claimFail || proof.Steps[0].Result != claimFail {
				t.Fatalf("known raw failure lost target at%d/%d: %+v %v", stopAt, total, proof, err)
			}
		}
	}
	if seen == 0 {
		t.Fatal("probe did not interrupt after the known raw contradiction")
	}
}

func TestReuseAcceptedLateRawShape(t *testing.T) {
	for _, early := range []bool{false, true} {
		for _, damage := range []string{"null-digest", "missing-digest", "typed-digest", "typed-nodes", "unknown-step", "null-step"} {
			name := damage
			if early {
				name = "early-" + name
			}
			t.Run(name, func(t *testing.T) {
				req, raw, projection := acceptedFixture(t, 0)
				h := req.Ledger.original(req.Case.ID)
				last := h.BlockedSteps[len(h.BlockedSteps)-1]
				step := acceptedStep(raw, last)
				tree := step["incremental"].(map[string]any)
				switch damage {
				case "null-digest":
					tree["digest"] = nil
				case "missing-digest":
					delete(tree, "digest")
				case "typed-digest":
					tree["digest"] = 1
				case "typed-nodes":
					tree["nodes"] = "wrong"
				case "unknown-step":
					step["unexpected"] = true
				case "null-step":
					raw["steps"].([]any)[last] = nil
				}
				if early {
					projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
					projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
				}
				setAcceptedEvidence(&req, raw, projection)
				proof, err := JudgeReuseAccepted(req)
				if err != nil || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_RAW_INVALID" || proof.Steps[0].Result != claimPass || proof.Steps[len(proof.Steps)-1].Result != claimFail {
					t.Fatalf("late raw %s: %+v %v", name, proof, err)
				}
			})
		}
	}
}

func TestQualifyReuseAcceptedKeepsFailureCode(t *testing.T) {
	root, _ := acceptedReplayFixture(t, func(files map[string][]byte, _ map[string]any) {
		for path, data := range files {
			if strings.HasPrefix(path, "sources/") {
				files[path] = []byte(strings.Repeat("x", len(data)))
			}
		}
	})
	q, w, p := acceptedReaderQualifier(t, root, false)
	s, err := q.evalSymbolSet(w, p)
	if err != nil || s.ReuseAccepted.Cases[0].Result != claimFail || s.ReuseAccepted.Cases[0].Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" {
		t.Fatalf("qualification lost accepted failure: %+v %v", s.ReuseAccepted, err)
	}
}

func TestReuseAcceptedCompletedHeaderCount(t *testing.T) {
	for _, early := range []bool{false, true} {
		for _, damage := range []string{"zero", "one", "minus-one", "plus-one"} {
			name := damage
			if early {
				name = "early-" + name
			}
			t.Run(name, func(t *testing.T) {
				req, raw, projection := acceptedFixture(t, 0)
				n := len(raw["steps"].([]any))
				if n < 3 {
					t.Fatal("multi-target fixture required")
				}
				count := map[string]int{"zero": 0, "one": 1, "minus-one": n - 1, "plus-one": n + 1}[damage]
				raw["steps_completed"] = count
				if early {
					projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
					projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
				}
				setAcceptedEvidence(&req, raw, projection)
				proof, err := JudgeReuseAccepted(req)
				if err != nil || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_RAW_INVALID" {
					t.Fatalf("header count %d/%d: %+v %v", count, n, proof, err)
				}
				for _, step := range proof.Steps {
					if step.Result == claimPass {
						t.Fatalf("common header contradiction promoted target: %+v", proof)
					}
				}
			})
		}
	}
}

func TestReuseAcceptedPartialHeader(t *testing.T) {
	for _, damage := range []string{"valid", "count", "code", "status", "late-invalid", "late-invalid-code", "zero-code", "last-target-valid", "last-target-code", "last-malformed", "last-malformed-code", "last-wire-invalid", "last-wire-invalid-code"} {
		t.Run(damage, func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			n := len(raw["steps"].([]any))
			h := req.Ledger.original(req.Case.ID)
			if strings.HasPrefix(damage, "last-target") {
				n = h.BlockedSteps[len(h.BlockedSteps)-1] + 1
				raw["steps"] = raw["steps"].([]any)[:n]
				projection["steps"] = projection["steps"].([]any)[:n]
			}
			last := n - 1
			status, code, exit := StatusResourceLimit, "PARSE_TIME_LIMIT", 3
			raw["steps_completed"] = last
			acceptedStep(raw, last)["fresh"] = map[string]any{"status": status, "code": code, "form": nil}
			acceptedStep(projection, last)["fresh"] = map[string]any{"status": status, "code": code, "form": "", "descendant_count": 0, "has_error": false, "digest": "", "tree": nil}
			acceptedStep(projection, last)["comparison"] = nil
			for _, m := range []map[string]any{raw, projection} {
				o := acceptedObs(m, last, 1)
				o["status"], o["reason"] = "UNAVAILABLE", "PARSE_INCOMPLETE"
				o["counts"].(map[string]any)["done"] = 0
			}
			switch damage {
			case "count":
				raw["steps_completed"] = n
			case "code", "late-invalid-code", "zero-code", "last-target-code", "last-malformed-code", "last-wire-invalid-code":
				code = "NODE_LIMIT"
			case "status":
				status, code, exit = StatusFailed, "PARSE_NULL", 4
			}
			if strings.HasPrefix(damage, "late-invalid") {
				delete(acceptedStep(raw, h.BlockedSteps[len(h.BlockedSteps)-1])["incremental"].(map[string]any), "digest")
			}
			if strings.HasPrefix(damage, "last-malformed") {
				raw["steps"].([]any)[last] = nil
			}
			if strings.HasPrefix(damage, "last-wire-invalid") {
				acceptedStep(raw, last)["incremental"].(map[string]any)["digest"] = strings.Repeat("f", 64)
			}
			if damage == "zero-code" {
				for _, m := range []map[string]any{raw, projection} {
					acceptedObs(m, h.BlockedSteps[0], 0)["counts"].(map[string]any)["accepted"] = 0
				}
			}
			req.Expected.Exit = &exit
			raw["status"], raw["code"] = status, code
			projection["execution_status"], projection["code"], projection["response_status"], projection["response_code"] = status, code, status, code
			process := projection["process"].(map[string]any)
			process["status"], process["exit_code"] = StatusFailed, exit
			setAcceptedEvidence(&req, raw, projection)
			calls := 0
			req.Expected.Check = func() error { calls++; return nil }
			proof, err := JudgeReuseAccepted(req)
			wantResult, wantFirst := claimFail, claimBlocked
			if damage == "valid" || damage == "last-target-valid" {
				wantResult, wantFirst = claimBlocked, claimPass
			}
			if damage == "late-invalid" || strings.HasPrefix(damage, "last-malformed") || strings.HasPrefix(damage, "last-wire-invalid") {
				wantFirst = claimPass
			}
			if damage == "zero-code" {
				wantFirst = claimFail
			}
			if err != nil || proof.Result != wantResult || proof.Steps[0].Result != wantFirst {
				t.Fatalf("partial header %s: %+v %v", damage, proof, err)
			}
			if strings.HasPrefix(damage, "last-target") && proof.Steps[len(proof.Steps)-1].Result != claimBlocked {
				t.Fatalf("common header failure blamed current target: %+v", proof)
			}
			if damage == "zero-code" && (proof.Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[0].Code != "REUSE_ACCEPTED_ZERO") {
				t.Fatalf("known first failure lost: %+v", proof)
			}
			if strings.HasPrefix(damage, "late-invalid") && proof.Steps[len(proof.Steps)-1].Result != claimFail {
				t.Fatalf("late target failure lost: %+v", proof)
			}
			if damage == "code" {
				total, seen := calls, 0
				if total > 256 {
					t.Fatalf("bounded checkpoint probe exceeded256: %d", total)
				}
				for stopAt := 1; stopAt <= total; stopAt++ {
					calls = 0
					sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
					req.Expected.Check = func() error {
						calls++
						if calls >= stopAt {
							return sentinel
						}
						return nil
					}
					got, err := JudgeReuseAccepted(req)
					if err != nil && !errors.Is(err, sentinel) {
						t.Fatalf("original interruption lost: %v", err)
					}
					if err != nil && got.Code == "REUSE_ACCEPTED_RAW_INVALID" {
						seen++
						if got.Result != claimFail || got.Steps[0].Result == claimPass {
							t.Fatalf("known common header failure lost: %+v %v", got, err)
						}
					}
				}
				if seen == 0 {
					t.Fatal("no interruption after known header contradiction")
				}
			}
		})
	}
}

func TestReuseAcceptedZeroBeforeCSTInterruption(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 12)
	if req.Ledger.original(req.Case.ID).BlockedSteps[0] != 1 || len(acceptedStep(raw, 0)["incremental"].(map[string]any)["nodes"].([]any)) != 1 {
		t.Fatal("one-node initial then target1 fixture required")
	}
	for _, m := range []map[string]any{raw, projection} {
		acceptedObs(m, 1, 0)["counts"].(map[string]any)["accepted"] = 0
	}
	setAcceptedEvidence(&req, raw, projection)
	sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
	nodeChecks := 0
	req.Expected.Check = func() error {
		var pcs [16]uintptr
		frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs[:])])
		for {
			frame, more := frames.Next()
			if strings.HasSuffix(frame.Function, "/nativewire.nodes") {
				nodeChecks++
				if nodeChecks == 2 {
					return sentinel
				}
				break
			}
			if !more {
				break
			}
		}
		return nil
	}
	proof, err := JudgeReuseAccepted(req)
	if nodeChecks != 2 || !errors.Is(err, sentinel) || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[0].Result != claimFail || proof.Steps[0].Code != "REUSE_ACCEPTED_ZERO" {
		t.Fatalf("bound zero lost before target CST: nodechecks%d %+v %v", nodeChecks, proof, err)
	}
}

func TestReuseAcceptedEarlyProjectionZeroInterruption(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 0)
	h := req.Ledger.original(req.Case.ID)
	last := h.BlockedSteps[len(h.BlockedSteps)-1]
	for _, m := range []map[string]any{raw, projection} {
		acceptedObs(m, last, 0)["counts"].(map[string]any)["accepted"] = 0
	}
	projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
	projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
	setAcceptedEvidence(&req, raw, projection)
	baseline, err := JudgeReuseAccepted(req)
	if err != nil || baseline.Result != claimFail || baseline.Code != "REUSE_ACCEPTED_ZERO" || baseline.Steps[0].Result != claimPass || baseline.Steps[len(baseline.Steps)-1].Result != claimFail {
		t.Fatalf("late zero prefix control: %+v %v", baseline, err)
	}
	sentinel := &Error{Kind: KindResourceLimit, Code: "TEST_WALL_LIMIT"}
	stopped := false
	req.Expected.Check = func() error {
		var pcs [24]uintptr
		frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs[:])])
		for {
			frame, more := frames.Next()
			if strings.HasSuffix(frame.Function, "/nativewire.nodes") {
				stopped = true
				return sentinel
			}
			if !more {
				break
			}
		}
		return nil
	}
	proof, err := JudgeReuseAccepted(req)
	if !stopped || !errors.Is(err, sentinel) || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[len(proof.Steps)-1].Result != claimFail || proof.Steps[len(proof.Steps)-1].Code != "REUSE_ACCEPTED_ZERO" {
		t.Fatalf("early projection zero lost before raw target: stopped%v %+v %v", stopped, proof, err)
	}
}

func TestReuseAcceptedProjectionUnknownFields(t *testing.T) {
	for _, early := range []bool{false, true} {
		for _, slot := range []string{"step", "edit", "route", "comparison", "tree-header", "full-tree", "input", "node", "point", "identity"} {
			name := slot
			if early {
				name = "early-" + name
			}
			t.Run(name, func(t *testing.T) {
				req, raw, projection := acceptedFixture(t, 0)
				h := req.Ledger.original(req.Case.ID)
				last := h.BlockedSteps[len(h.BlockedSteps)-1]
				step := acceptedStep(projection, last)
				tree := step["incremental"].(map[string]any)
				full := tree["tree"].(map[string]any)
				var target map[string]any
				switch slot {
				case "step":
					target = step
				case "edit", "route", "comparison":
					target = step[slot].(map[string]any)
				case "tree-header":
					target = tree
				case "full-tree":
					target = full
				case "input":
					target = full["input"].(map[string]any)
				case "node":
					target = full["nodes"].([]any)[0].(map[string]any)
				case "point":
					target = full["nodes"].([]any)[0].(map[string]any)["start_point"].(map[string]any)
				case "identity":
					target = full["identities"].([]any)[0].(map[string]any)
				}
				target["unexpected"] = true
				if early {
					projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
					projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
				}
				setAcceptedEvidence(&req, raw, projection)
				proof, err := JudgeReuseAccepted(req)
				if err != nil || proof.Result != claimFail || proof.Steps[0].Result != claimPass || proof.Steps[len(proof.Steps)-1].Result != claimFail {
					t.Fatalf("unknown projection %s: %+v %v", name, proof, err)
				}
			})
		}
	}
}

func TestReuseAcceptedProjectionKnownExtras(t *testing.T) {
	req, raw, projection := acceptedFixture(t, 0)
	for _, value := range projection["steps"].([]any) {
		step := value.(map[string]any)
		for _, name := range []string{"incremental", "fresh"} {
			tree, _ := step[name].(map[string]any)
			if tree == nil {
				continue
			}
			tree["parse_ms"], tree["max_depth"] = 0, 1
			full := tree["tree"].(map[string]any)
			full["capabilities"], full["captures"] = map[string]string{"full_cst": "SUPPORTED"}, []any{}
			full["input"].(map[string]any)["encoding_source"] = SourceDeclaration
		}
	}
	setAcceptedEvidence(&req, raw, projection)
	proof, err := JudgeReuseAccepted(req)
	if err != nil || proof.Result != claimPass {
		t.Fatalf("known native projection fields: %+v %v", proof, err)
	}
}

func TestReuseAcceptedProjectionProducerAndFullForm(t *testing.T) {
	for _, damage := range []string{"producer-extra", "capability-extra", "full-summary"} {
		t.Run(damage, func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			producer := projection["producer"].(map[string]any)
			switch damage {
			case "producer-extra":
				producer["unexpected"] = true
			case "capability-extra":
				producer["reuse_observation"].(map[string]any)["unexpected"] = true
			case "full-summary":
				h := req.Ledger.original(req.Case.ID)
				last := h.BlockedSteps[len(h.BlockedSteps)-1]
				acceptedStep(projection, last)["incremental"].(map[string]any)["summary"] = map[string]any{}
			}
			setAcceptedEvidence(&req, raw, projection)
			proof, err := JudgeReuseAccepted(req)
			if err != nil || proof.Result != claimFail {
				t.Fatalf("projection %s: %+v %v", damage, proof, err)
			}
			if damage == "full-summary" {
				if proof.Steps[0].Result != claimPass || proof.Steps[len(proof.Steps)-1].Result != claimFail {
					t.Fatalf("late full-form contradiction: %+v", proof)
				}
			} else {
				for _, step := range proof.Steps {
					if step.Result == claimPass {
						t.Fatalf("common producer contradiction promoted target: %+v", proof)
					}
				}
			}
		})
	}
}

func TestReuseAcceptedDistinctTargetFailureCodes(t *testing.T) {
	for _, mode := range []string{"raw", "early", "projection-only"} {
		t.Run(mode, func(t *testing.T) {
			req, raw, projection := acceptedFixture(t, 0)
			h := req.Ledger.original(req.Case.ID)
			first, last := h.BlockedSteps[0], h.BlockedSteps[len(h.BlockedSteps)-1]
			for _, m := range []map[string]any{raw, projection} {
				acceptedObs(m, first, 0)["counts"].(map[string]any)["accepted"] = 0
			}
			want := "REUSE_ACCEPTED_RAW_INVALID"
			if mode == "raw" || mode == "early" {
				delete(acceptedStep(raw, last)["incremental"].(map[string]any), "digest")
			} else {
				acceptedStep(projection, last)["unexpected"] = true
				want = "REUSE_ACCEPTED_PROJECTION_INVALID"
			}
			if mode == "early" {
				projection["execution_status"], projection["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
				projection["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
			}
			setAcceptedEvidence(&req, raw, projection)
			if mode == "projection-only" {
				req.Raw = nil
			}
			proof, err := JudgeReuseAccepted(req)
			if err != nil || proof.Result != claimFail || proof.Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[0].Code != "REUSE_ACCEPTED_ZERO" || proof.Steps[len(proof.Steps)-1].Result != claimFail || proof.Steps[len(proof.Steps)-1].Code != want {
				t.Fatalf("distinct %s: %+v %v", mode, proof, err)
			}
		})
	}
}
