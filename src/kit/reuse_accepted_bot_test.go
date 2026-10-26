package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
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
