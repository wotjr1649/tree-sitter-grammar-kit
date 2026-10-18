package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestEvidenceSnapshotPreservesVerification(t *testing.T) {
	for _, mutation := range []string{"intact", "empty", "replay-result", "unretained-tamper", "retained-tamper", "rehashed-graph", "axis", "record-limit", "replay-record-limit", "file-limit", "total-limit", "cancel"} {
		t.Run(mutation, func(t *testing.T) {
			g := newFxGraph()
			g.files["raw/other.json"] = []byte(`{"other":true}`)
			g.nodes[1]["files"] = []ReplayMember{{Path: "raw/other.json", Role: "raw", Bytes: uint64(len(g.files["raw/other.json"])), SHA256: sum(g.files["raw/other.json"])}}
			if mutation == "empty" {
				g.files["raw/old.json"] = []byte{}
				g.nodes[0]["files"] = []ReplayMember{{Path: "raw/old.json", Role: "raw", SHA256: sum([]byte{})}}
			}
			if mutation == "axis" {
				g.nodes[1]["evidence_mode"] = ModeReplayedRaw
			}
			if mutation == "replay-result" || mutation == "replay-record-limit" {
				b, err := json.Marshal(map[string]any{"result_schema": ReplayResultSchema, "evidence_mode": ModeReplayedRaw, "assessment": AssessFail, "subject": map[string]any{"run": "10", "attempt": 1, "platform": "windows/amd64"}})
				if err != nil {
					t.Fatal(err)
				}
				g.files["replay/result.json"] = b
				g.nodes[2]["files"] = []ReplayMember{{Path: "replay/result.json", Role: "replay-result", Bytes: uint64(len(b)), SHA256: sum(b)}}
			}
			doc, err := json.Marshal(map[string]any{"schema": EvidenceSchema, "nodes": g.nodes})
			if err != nil {
				t.Fatal(err)
			}
			g.policy["evidence_sha256"] = sum(doc)
			root := t.TempDir()
			files := map[string][]byte{"evidence.json": doc}
			for path, data := range g.files {
				files[path] = data
			}
			if mutation == "rehashed-graph" {
				g.files["raw/old.json"] = []byte(`{"old":false}`)
				files["raw/old.json"] = g.files["raw/old.json"]
				g.nodes[0]["files"] = []ReplayMember{{Path: "raw/old.json", Role: "raw", Bytes: uint64(len(g.files["raw/old.json"])), SHA256: sum(g.files["raw/old.json"])}}
				files["evidence.json"], err = json.Marshal(map[string]any{"schema": EvidenceSchema, "nodes": g.nodes})
				if err != nil {
					t.Fatal(err)
				}
			}
			writeSet(t, root, files, func(string) string { return "" })
			if mutation == "unretained-tamper" || mutation == "retained-tamper" {
				path := "raw/other.json"
				if mutation == "retained-tamper" {
					path = "raw/old.json"
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(`{"bad":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			policy, err := json.Marshal(g.policy)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(ctxT(t))
			defer cancel()
			if mutation == "cancel" {
				cancel()
			}
			testHookReplayLimits = func(l ReplayLimits) ReplayLimits {
				switch mutation {
				case "record-limit", "replay-record-limit":
					l.RecordBytes = 1
				case "file-limit":
					l.FileBytes = 1
				case "total-limit":
					l.TotalBytes = uint64(len(doc)) + 1
				}
				return l
			}
			defer func() { testHookReplayLimits = nil; testHookOpen = nil }()
			req := EvidenceRequest{Root: root, Policy: policy}
			plain, plainErr := VerifyEvidence(ctx, req)
			opens := map[string]int{}
			testHookOpen = func(path string) { opens[path]++ }
			snapshot := &evidenceSnapshot{wanted: map[string]bool{"raw/old.json": true, "replay/result.json": true, "missing.json": true}, files: map[string][]byte{"stale": []byte("stale")}, nodes: []EvidenceNode{{ID: "stale"}}}
			got, gotErr := verifyEvidence(ctx, req, snapshot)
			if !reflect.DeepEqual(got, plain) || !reflect.DeepEqual(gotErr, plainErr) {
				t.Fatalf("snapshot changed result: %s/%v versus %s/%v", got.Assessment, gotErr, plain.Assessment, plainErr)
			}
			if _, ok := snapshot.files["stale"]; ok {
				t.Fatal("stale bytes survived")
			}
			if _, ok := snapshot.files["raw/other.json"]; ok {
				t.Fatal("unrequested member retained")
			}
			if _, ok := snapshot.files["missing.json"]; ok {
				t.Fatal("missing member manufactured")
			}
			for path, n := range opens {
				if n != 1 {
					t.Fatalf("%s reread %d times", path, n)
				}
			}
			if mutation == "intact" || mutation == "empty" || mutation == "record-limit" || mutation == "replay-result" {
				if gotErr != nil || got.Assessment != AssessPass || !reflect.DeepEqual(snapshot.files["raw/old.json"], g.files["raw/old.json"]) || len(snapshot.nodes) != len(g.nodes) {
					t.Fatalf("verified snapshot missing: %+v %v", got, gotErr)
				}
				if mutation == "replay-result" && !reflect.DeepEqual(snapshot.files["replay/result.json"], g.files["replay/result.json"]) {
					t.Fatal("replay result bytes not retained")
				}
			} else if mutation == "unretained-tamper" || mutation == "retained-tamper" || mutation == "rehashed-graph" || mutation == "axis" {
				if gotErr != nil || got.Assessment != AssessFail {
					t.Fatalf("contradiction escaped: %+v %v", got, gotErr)
				}
				if mutation == "retained-tamper" && snapshot.files["raw/old.json"] != nil {
					t.Fatal("unverified member bytes retained")
				}
				if mutation == "rehashed-graph" && !slices.Contains(codes(got.Findings), "EVIDENCE_ANCHOR_MISMATCH") {
					t.Fatal("external policy anchor not enforced")
				}
			} else if gotErr == nil {
				t.Fatal("limit/cancellation lost")
			}
		})
	}
}
