package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func referenceReplayFixture(t *testing.T, f *referenceFixture) (QueryReferenceConnectionRequest, string, map[string]any) {
	t.Helper()
	direct := f.request(t)
	files := map[string][]byte{"connection.json": direct.Registration, "query-registration.json": direct.WitnessRegistration, "policy.json": direct.Evidence.Policy}
	entries, err := os.ReadDir(direct.Evidence.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("unexpected fixture directory")
		}
		b, err := os.ReadFile(filepath.Join(direct.Evidence.Root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files["evidence/"+entry.Name()] = b
	}
	root := t.TempDir()
	members := writeSet(t, root, files, func(p string) string {
		switch p {
		case "connection.json":
			return "reference-registration"
		case "query-registration.json":
			return "query-registration"
		case "policy.json":
			return "reference-policy"
		}
		return "reference-evidence"
	})
	ids := map[string]string{"registration": sum(direct.Registration), "witness": sum(direct.WitnessRegistration), "evidence-policy": sum(direct.Evidence.Policy), "source": f.registration.Source.SHA256, "route": "owned-plain", "platform": "windows/amd64", "role": "owned", "policy": "tsgk-predicates/r1"}
	prof := registration("query-reference-connection-r1", "evidence-replay", AssessPass, ids, []string{"c1"}, members)
	prof["subject"].(map[string]any)["run"] = f.registration.Native.Identities["run"]
	return direct, root, prof
}

func TestReplayQueryReferenceConnection(t *testing.T) {
	for _, kind := range []string{"complete", "missing-anchor", "wrong-anchor", "missing-response", "mutated-response", "graph-witness-undeclared", "graph-witness-missing", "shared-context", "outer-response-pin", "outer-graph-pin", "wrong-records", "wrong-subject", "wrong-subject-mode", "wrong-subject-status", "wrong-subject-assessment", "outer-extra", "nested-extra"} {
		t.Run(kind, func(t *testing.T) {
			f := newReferenceFixture(t)
			if kind == "missing-anchor" {
				f.witness["required"].([]any)[0].(map[string]any)["expected"] = nil
				f.approve(t, "witness.json")
			}
			if kind == "wrong-anchor" {
				f.witness["required"].([]any)[0].(map[string]any)["expected"] = []Capture{}
				f.approve(t, "witness.json")
			}
			if kind == "graph-witness-undeclared" {
				f.nodes[0].Files = f.nodes[0].Files[1:]
				delete(f.files, "witness.json")
			}
			direct, root, prof := referenceReplayFixture(t, f)
			switch kind {
			case "missing-response", "graph-witness-missing":
				path := "reference-0.json"
				if kind == "graph-witness-missing" {
					path = "witness.json"
				}
				if e := os.Remove(filepath.Join(root, "evidence", path)); e != nil {
					t.Fatal(e)
				}
				if e := os.Remove(filepath.Join(direct.Evidence.Root, path)); e != nil {
					t.Fatal(e)
				}
			case "mutated-response":
				for _, dir := range []string{direct.Evidence.Root, filepath.Join(root, "evidence")} {
					if e := os.WriteFile(filepath.Join(dir, "reference-0.json"), []byte("{}"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			case "outer-response-pin", "outer-graph-pin":
				path := "evidence/reference-0.json"
				if kind == "outer-graph-pin" {
					path = "evidence/evidence.json"
				}
				for _, v := range prof["members"].([]any) {
					m := v.(map[string]any)
					if m["path"] == path {
						m["sha256"] = sum([]byte("different outer approval"))
					}
				}
			case "wrong-records":
				prof["records"] = []string{"other"}
			case "wrong-subject":
				prof["subject"].(map[string]any)["run"] = "other-run"
			case "wrong-subject-mode":
				prof["subject"].(map[string]any)["evidence_mode"] = ModeReplayedRaw
			case "wrong-subject-status":
				prof["subject"].(map[string]any)["execution_status"] = StatusCancelled
			case "wrong-subject-assessment":
				prof["subject"].(map[string]any)["assessment"] = AssessBlocked
			case "shared-context":
				// Keep approval consistent but contradict the selected node's policy identity.
				var policy map[string]any
				if e := json.Unmarshal(direct.Evidence.Policy, &policy); e != nil {
					t.Fatal(e)
				}
				policy["required"] = []any{map[string]any{"node": "native", "kind": "run", "identities": map[string]string{"policy": "other"}}}
				direct.Evidence.Policy = mustJSON(t, policy)
				f.registration.EvidencePolicySHA256 = sum(direct.Evidence.Policy)
				direct.Registration = mustJSON(t, f.registration)
				for _, v := range []struct {
					name string
					data []byte
				}{{"policy.json", direct.Evidence.Policy}, {"connection.json", direct.Registration}} {
					if e := os.WriteFile(filepath.Join(root, v.name), v.data, 0600); e != nil {
						t.Fatal(e)
					}
					for _, m := range prof["members"].([]any) {
						q := m.(map[string]any)
						if q["path"] == v.name {
							q["bytes"] = len(v.data)
							q["sha256"] = sum(v.data)
						}
					}
				}
				prof["identities"].(map[string]string)["registration"] = sum(direct.Registration)
				prof["identities"].(map[string]string)["evidence-policy"] = sum(direct.Evidence.Policy)
			case "outer-extra", "nested-extra":
				dir := root
				if kind == "nested-extra" {
					dir = filepath.Join(root, "evidence")
				}
				if e := os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra"), 0600); e != nil {
					t.Fatal(e)
				}
				if kind == "nested-extra" {
					if e := os.WriteFile(filepath.Join(direct.Evidence.Root, "extra.txt"), []byte("extra"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			want, e := VerifyQueryReferenceConnection(ctxT(t), direct)
			if e != nil {
				t.Fatal(e)
			}
			got := runReg(t, root, prof)
			if got.ResultSchema != ReplayReferenceResultSchema || got.QueryReference == nil {
				t.Fatalf("missing separate result payload: %+v", got)
			}
			if kind == "outer-response-pin" {
				if got.Assessment != AssessFail || got.QueryReference.Verified != 2 {
					t.Fatalf("outer response pin: %+v", got)
				}
			} else if kind == "outer-graph-pin" {
				if got.Assessment != AssessFail || got.QueryReference.Verified != 0 {
					t.Fatalf("outer graph pin: %+v", got)
				}
			} else {
				if !reflect.DeepEqual(*got.QueryReference, want) {
					t.Fatalf("payload differs: %+v vs %+v", *got.QueryReference, want)
				}
				expected := want.Assessment
				if kind == "wrong-records" || strings.HasPrefix(kind, "wrong-subject") || kind == "outer-extra" {
					expected = AssessFail
				}
				if got.Assessment != expected {
					t.Fatalf("assessment %s != %s: %+v", got.Assessment, expected, got)
				}
			}
			if got.Consumption.Expected != 1 || got.Consumption.Consumed != 1 {
				t.Fatalf("case denominator: %+v", got.Consumption)
			}
		})
	}
}

func TestReplayQueryReferenceConnectionSinglePass(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	opens := map[string]int{}
	testHookOpen = func(path string) { opens[path]++ }
	defer func() { testHookOpen = nil }()
	r := runReg(t, root, prof)
	if r.Assessment != AssessPass {
		t.Fatalf("single pass: %+v", r)
	}
	for path, n := range opens {
		if n != 1 {
			t.Fatalf("%s opened %d times", path, n)
		}
	}
	var total uint64
	for _, v := range prof["members"].([]any) {
		m := v.(map[string]any)
		total += uint64(m["bytes"].(int))
	}
	if r.BytesRead != total {
		t.Fatalf("read budget %d != %d", r.BytesRead, total)
	}
}

func TestReplayQueryReferenceRegistrationRoles(t *testing.T) {
	for _, kind := range []string{"missing-policy", "duplicate-policy", "policy-inside", "retained-inside", "evidence-outside"} {
		t.Run(kind, func(t *testing.T) {
			f := newReferenceFixture(t)
			_, root, prof := referenceReplayFixture(t, f)
			ms := prof["members"].([]any)
			switch kind {
			case "missing-policy":
				for i, v := range ms {
					if v.(map[string]any)["role"] == "reference-policy" {
						ms = append(ms[:i:i], ms[i+1:]...)
						break
					}
				}
			case "duplicate-policy":
				data := []byte("{}")
				if e := os.WriteFile(filepath.Join(root, "second-policy.json"), data, 0600); e != nil {
					t.Fatal(e)
				}
				ms = append(ms, map[string]any{"path": "second-policy.json", "role": "reference-policy", "bytes": len(data), "sha256": sum(data)})
			case "policy-inside":
				if e := os.Rename(filepath.Join(root, "policy.json"), filepath.Join(root, "evidence", "policy.json")); e != nil {
					t.Fatal(e)
				}
				for _, v := range ms {
					m := v.(map[string]any)
					if m["role"] == "reference-policy" {
						m["path"] = "evidence/policy.json"
					}
				}
			case "retained-inside":
				for _, v := range ms {
					m := v.(map[string]any)
					if m["path"] == "evidence/reference-0.json" {
						m["role"] = "retained"
					}
				}
			case "evidence-outside":
				for _, v := range ms {
					m := v.(map[string]any)
					if m["role"] == "reference-policy" {
						m["role"] = "reference-evidence"
					}
				}
			}
			prof["members"] = ms
			r := runReg(t, root, prof)
			if r.Assessment != AssessFail {
				t.Fatalf("role boundary %s: %+v", kind, r)
			}
		})
	}
}

func TestReplayQueryReferenceLegacyResult(t *testing.T) {
	req, w, raw, proj := witnessFixture(t)
	req = marshalWitness(t, req, w, raw, proj)
	root := t.TempDir()
	files := map[string][]byte{"registration.json": req.Registration, "source.txt": req.Source, "raw.json": req.Raw, "projection.json": req.Projection}
	roles := map[string]string{"registration.json": "query-registration", "source.txt": "case-source", "raw.json": "raw", "projection.json": "projection"}
	ms := writeSet(t, root, files, func(p string) string { return roles[p] })
	ids := map[string]string{"registration": sum(req.Registration), "source": sum(req.Source), "route": "owned-plain", "platform": "windows/amd64", "role": "owned", "policy": "tsgk-predicates/r1"}
	r := runReg(t, root, registration("query-witness-r1", "evidence-replay", AssessPass, ids, []string{"c1"}, ms))
	data := mustJSON(t, r)
	if r.ResultSchema != ReplayResultSchema || r.QueryReference != nil || strings.Contains(string(data), "query_reference") {
		t.Fatalf("legacy schema changed: %s", data)
	}
	if r.Assessment != AssessPass || !slices.Contains(r.Coverage.Observed, "stored-query-witness") {
		t.Fatalf("legacy changed: %+v", r)
	}
}

func referenceReplayBytes(t *testing.T, prof map[string]any) uint64 {
	t.Helper()
	var total uint64
	for _, v := range prof["members"].([]any) {
		total += uint64(v.(map[string]any)["bytes"].(int))
	}
	return total
}

func TestReplayQueryReferenceSharedBudgets(t *testing.T) {
	for _, kind := range []string{"total", "wall", "cancel", "fail-then-total", "fail-then-wall"} {
		t.Run(kind, func(t *testing.T) {
			f := newReferenceFixture(t)
			if strings.HasPrefix(kind, "fail-then-") {
				f.witness["required"].([]any)[0].(map[string]any)["expected"] = []Capture{}
				f.approve(t, "witness.json")
			}
			_, root, prof := referenceReplayFixture(t, f)
			if strings.HasPrefix(kind, "fail-then-") {
				tail := []byte("tail read must remain inside the shared budget")
				if err := os.WriteFile(filepath.Join(root, "tail.bin"), tail, 0600); err != nil {
					t.Fatal(err)
				}
				prof["members"] = append(prof["members"].([]any), map[string]any{"path": "tail.bin", "role": "retained", "bytes": len(tail), "sha256": sum(tail)})
			}
			calls := 0
			wall, expire := context.WithCancel(context.Background())
			defer expire()
			testHookWall = func() (context.Context, context.CancelFunc) { calls++; return wall, func() {} }
			limit := referenceReplayBytes(t, prof) - 1
			if strings.Contains(kind, "total") {
				testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = limit; return l }
			}
			ctx, cancel := context.WithCancel(ctxT(t))
			defer cancel()
			testHookOpen = func(path string) {
				if kind == "wall" && path == "evidence.json" || kind == "fail-then-wall" && path == "tail.bin" {
					expire()
				}
				if kind == "cancel" && path == "evidence.json" {
					cancel()
				}
			}
			defer func() { testHookWall = nil; testHookOpen = nil; testHookReplayLimits = nil }()
			res, err := Replay(ctx, ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
			code := "WALL_LIMIT"
			if strings.Contains(kind, "total") {
				code = "TOTAL_BYTES_LIMIT"
			}
			if kind == "cancel" {
				code = "CANCELLED"
			}
			if err == nil || asError(err).Code != code || calls != 1 || res.QueryReference == nil {
				t.Fatalf("shared %s: calls%d %+v %v", kind, calls, res, err)
			}
			if strings.Contains(kind, "total") && res.BytesRead <= limit {
				t.Fatalf("read budget was reset: %d <= %d", res.BytesRead, limit)
			}
			if strings.HasPrefix(kind, "fail-then-") && (res.Assessment != AssessFail || res.QueryReference.Assessment != AssessFail || res.QueryReference.Verified != 2 || res.Recomputed.Assessment != AssessFail) {
				t.Fatalf("known failure lost: %+v", res)
			}
			if !validReferenceReplayResult(res) {
				t.Fatalf("producer error cannot round-trip: %+v", res)
			}
			if res.EvidenceValid = true; validReferenceReplayResult(res) {
				t.Fatalf("error result claiming valid evidence accepted: %+v", res)
			}
		})
	}
}

func TestReplayQueryReferenceFileReplacement(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	replaced := false
	testHookOpen = func(path string) {
		if path != "reference-0.json" || replaced {
			return
		}
		replaced = true
		src := filepath.Join(root, "evidence", path)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(src, filepath.Join(root, "evidence", "prior-reference.json")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookOpen = nil }()
	r, e := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
	if !replaced || e == nil || asError(e).Code != "SOURCE_CHANGED" || r.Assessment == AssessPass || r.QueryReference == nil {
		t.Fatalf("replacement accepted: %+v %v", r, e)
	}
	if !validReferenceReplayResult(r) {
		t.Fatalf("replacement error round-trip: %+v", r)
	}
}

func TestReplayQueryReferenceSwapBeforeNestedWalk(t *testing.T) {
	for _, swap := range []string{"member", "directory"} {
		t.Run(swap, func(t *testing.T) {
			f := newReferenceFixture(t)
			_, root, prof := referenceReplayFixture(t, f)
			keep := t.TempDir() // the originals stay alive, so a swapped file is a distinct object
			swapped := false
			testHookOpen = func(path string) {
				if path != "connection.json" || swapped {
					return
				}
				swapped = true
				dir := filepath.Join(root, "evidence")
				if swap == "directory" {
					if err := os.Rename(dir, filepath.Join(keep, "evidence")); err != nil {
						t.Fatal(err)
					}
					if err := os.CopyFS(dir, os.DirFS(filepath.Join(keep, "evidence"))); err != nil {
						t.Fatal(err)
					}
					return
				}
				src := filepath.Join(dir, "reference-0.json")
				data, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(src, filepath.Join(keep, "reference-0.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(src, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			defer func() { testHookOpen = nil }()
			r, e := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
			if !swapped || e == nil || asError(e).Code != "SOURCE_CHANGED" || r.Assessment == AssessPass || !validReferenceReplayResult(r) {
				t.Fatalf("swap before the nested walk accepted: %+v %v", r, e)
			}
		})
	}
}

func TestReplayQueryReferenceHeldNestedRoot(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	attempted, moved := false, false
	testHookOpen = func(path string) {
		if path != "evidence.json" || attempted {
			return
		}
		attempted = true
		dir := filepath.Join(root, "evidence")
		if err := os.Rename(dir, filepath.Join(root, "original-evidence")); err != nil {
			t.Logf("held directory rename refused: %v", err)
			return
		}
		moved = true
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "evidence.json"), []byte("replacement bytes"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookOpen = nil }()
	r := runReg(t, root, prof)
	if !attempted || r.Assessment != AssessPass || r.QueryReference == nil || r.QueryReference.Verified != 3 {
		t.Fatalf("held root changed: %+v", r)
	}
	if moved {
		b, e := os.ReadFile(filepath.Join(root, "evidence", "evidence.json"))
		if e != nil || string(b) != "replacement bytes" {
			t.Fatalf("replacement namespace touched: %s %v", b, e)
		}
	}
}

func TestReplayQueryReferenceResultRevision(t *testing.T) {
	for _, kind := range []string{"complete", "blocked", "contradiction", "before-core", "core-error", "over-count", "missing-payload", "legacy-label", "wrong-reducer", "wrong-recomputed", "wrong-native-count", "native-overclaim", "gate-verdict", "case-counter", "partial-without-native", "partial-incomplete-native", "registration-mismatch", "native-registration-mismatch", "native-registration-empty", "partial-native-registration-empty", "partial-registration-empty", "partial-registration-mismatch", "partial-native-status", "partial-native-assessment", "partial-native-mode", "partial-fail-downgrade", "native-fail-downgrade", "contradiction-evidence-valid", "subject-status", "subject-assessment", "subject-mode", "before-core-recomputed", "before-core-evidence-valid", "core-error-recomputed", "core-error-evidence-valid"} {
		t.Run(kind, func(t *testing.T) {
			f := newReferenceFixture(t)
			if kind == "blocked" {
				f.witness["required"].([]any)[0].(map[string]any)["expected"] = nil
				f.approve(t, "witness.json")
			}
			if strings.HasPrefix(kind, "contradiction") {
				f.witness["required"].([]any)[0].(map[string]any)["expected"] = []Capture{}
				f.approve(t, "witness.json")
			}
			_, root, prof := referenceReplayFixture(t, f)
			if strings.HasPrefix(kind, "before-core") {
				if e := os.Remove(filepath.Join(root, "policy.json")); e != nil {
					t.Fatal(e)
				}
			}
			if strings.HasPrefix(kind, "core-error") {
				testHookOpen = func(path string) {
					if path == "evidence.json" {
						if e := os.Remove(filepath.Join(root, "evidence", path)); e != nil {
							t.Fatal(e)
						}
					}
				}
				defer func() { testHookOpen = nil }()
			}
			res, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
			if err != nil && !strings.HasPrefix(kind, "core-error") {
				t.Fatal(err)
			}
			wantValid := true
			switch kind {
			case "contradiction-evidence-valid", "before-core-evidence-valid", "core-error-evidence-valid":
				res.EvidenceValid = true
				wantValid = false
			case "before-core-recomputed", "core-error-recomputed":
				res.Recomputed = Verdict{StatusCompleted, AssessBlocked}
				wantValid = false
			case "subject-status", "subject-assessment", "subject-mode":
				*map[string]*string{"subject-status": &res.Subject.ExecutionStatus, "subject-assessment": &res.Subject.Assessment, "subject-mode": &res.Subject.EvidenceMode}[kind] = "FORGED"
				wantValid = false
			case "over-count":
				res.QueryReference.Verified = res.QueryReference.Required + 1
				wantValid = false
			case "missing-payload":
				res.QueryReference = nil
				wantValid = false
			case "legacy-label":
				res.ResultSchema = ReplayResultSchema
				wantValid = false
			case "wrong-reducer":
				res.Reducer.ID = "query-witness-r1"
				wantValid = false
			case "wrong-recomputed":
				res.Recomputed.Assessment = AssessFail
				wantValid = false
			case "wrong-native-count":
				res.QueryReference.NativeWitness.Required++
				wantValid = false
			case "native-overclaim":
				res.QueryReference.NativeWitness.Verified--
				wantValid = false
			case "gate-verdict":
				res.Gates[0].Assessment = AssessFail
				wantValid = false
			case "case-counter":
				res.Consumption.Missing = 1
				wantValid = false
			case "registration-mismatch":
				res.QueryReference.RegistrationSHA256 = sum([]byte("other registration"))
				wantValid = false
			case "native-registration-mismatch":
				res.QueryReference.NativeWitness.RegistrationSHA256 = sum([]byte("other witness"))
				wantValid = false
			case "native-registration-empty", "partial-native-registration-empty":
				res.QueryReference.NativeWitness.RegistrationSHA256 = ""
				if kind == "partial-native-registration-empty" {
					res.Gates = nil
					res.QueryReference.ExecutionStatus, res.ExecutionStatus = StatusResourceLimit, StatusResourceLimit
					res.QueryReference.Assessment, res.Assessment = AssessBlocked, AssessBlocked
					res.Recomputed, res.Consumption = Verdict{}, Consumption{}
				}
				wantValid = false
			case "partial-registration-empty":
				res.Gates = nil
				res.QueryReference.NativeWitness = nil
				res.QueryReference.RegistrationSHA256 = ""
				res.QueryReference.Verified = 0
				res.QueryReference.ExecutionStatus, res.ExecutionStatus = StatusResourceLimit, StatusResourceLimit
				res.QueryReference.Assessment, res.Assessment = AssessBlocked, AssessBlocked
				res.Recomputed, res.Consumption = Verdict{}, Consumption{}
				wantValid = false
			case "partial-native-status", "partial-native-assessment", "partial-native-mode":
				res.Gates = nil
				res.QueryReference.Verified = 0
				res.QueryReference.ExecutionStatus, res.ExecutionStatus = StatusResourceLimit, StatusResourceLimit
				res.QueryReference.Assessment, res.Assessment = AssessBlocked, AssessBlocked
				res.Recomputed, res.Consumption = Verdict{}, Consumption{}
				res.QueryReference.NativeWitness.ExecutionStatus = StatusResourceLimit
				if kind == "partial-native-status" {
					res.QueryReference.NativeWitness.ExecutionStatus = "FORGED"
				}
				if kind == "partial-native-assessment" {
					res.QueryReference.NativeWitness.Assessment = "FORGED"
				}
				if kind == "partial-native-mode" {
					res.QueryReference.NativeWitness.EvidenceMode = "FORGED"
				}
				wantValid = false
			case "native-fail-downgrade":
				res.QueryReference.NativeWitness.Assessment = AssessFail
				res.QueryReference.Assessment, res.Assessment = AssessBlocked, AssessBlocked
				res.Recomputed.Assessment = AssessBlocked
				res.Gates[0].Recomputed, res.Gates[0].NotRecomputed = 0, 1
				res.Gates[0].finish()
				wantValid = false
			case "partial-without-native", "partial-incomplete-native", "partial-registration-mismatch", "partial-fail-downgrade":
				res.Gates = nil
				if kind == "partial-incomplete-native" {
					res.QueryReference.NativeWitness.ExecutionStatus = StatusResourceLimit
				} else {
					res.QueryReference.NativeWitness = nil
				}
				res.QueryReference.ExecutionStatus = StatusResourceLimit
				res.QueryReference.Assessment = AssessBlocked
				res.ExecutionStatus, res.Assessment = StatusResourceLimit, AssessBlocked
				res.Recomputed, res.Consumption = Verdict{}, Consumption{}
				if kind == "partial-registration-mismatch" {
					res.QueryReference.Verified = 0
					res.QueryReference.RegistrationSHA256 = sum([]byte("other registration"))
				}
				if kind == "partial-fail-downgrade" {
					res.QueryReference.Verified = 0
					res.QueryReference.Assessment = AssessFail
				}
				wantValid = false
			}
			if validReferenceReplayResult(res) != wantValid {
				t.Fatalf("result state %s: %+v", kind, res)
			}
			// The graph reader must recognize this revision without silently treating it as r1.
			node := &EvidenceNode{ID: "replay", EvidenceMode: res.EvidenceMode, Assessment: res.Assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			subj := &EvidenceNode{Identities: map[string]string{"run": res.Subject.Run, "attempt": "1", "platform": res.Subject.Platform}}
			env := &replayEnv{}
			env.checkReplayFile(node, subj, map[string][]byte{"result.json": mustJSON(t, res)}, func(code, path, msg string) { env.finding(code, path, msg) })
			if (len(env.findings) == 0) != wantValid {
				t.Fatalf("reader state %s: %+v", kind, env.findings)
			}
		})
	}
}

func TestReplayQueryReferenceTrustDocumentOverLimit(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	// an output error after the recorded-only verdict still yields a producer-valid error result
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes, l.OutputBytes = 1, 1; return l }
	res, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
	if err == nil || asError(err).Code != "OUTPUT_LIMIT" || res.EvidenceValid || !validReferenceReplayResult(res) {
		t.Fatalf("recorded-only output error: %+v %v", res, err)
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 1; return l }
	defer func() { testHookReplayLimits = nil }()
	res, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
	if err != nil || res.QueryReference != nil || !res.EvidenceValid || res.Recomputed != (Verdict{StatusNotRun, AssessUnresolved}) || !validReferenceReplayResult(res) {
		t.Fatalf("recorded-only result lost: %+v %v", res, err)
	}
	for _, forged := range []Verdict{{}, {StatusCompleted, AssessUnresolved}} {
		res.Recomputed = forged
		if validReferenceReplayResult(res) {
			t.Fatalf("recorded-only result with %+v accepted", forged)
		}
	}
}

func TestReplayQueryReferenceCumulativeKnownEvidenceFailure(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	// Fail the outer graph approval before the canonical pass reaches its read limit.
	for _, v := range prof["members"].([]any) {
		m := v.(map[string]any)
		if m["path"] == "evidence/evidence.json" {
			m["sha256"] = sum([]byte("wrong"))
		}
	}
	var docs uint64
	for _, v := range prof["members"].([]any) {
		m := v.(map[string]any)
		if !strings.HasPrefix(m["path"].(string), "evidence/") || m["path"] == "evidence/evidence.json" {
			docs += uint64(m["bytes"].(int))
		}
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = docs + 1; return l }
	defer func() { testHookReplayLimits = nil }()
	r, e := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
	if e == nil || asError(e).Code != "TOTAL_BYTES_LIMIT" || r.Assessment != AssessFail || r.QueryReference == nil || r.QueryReference.Assessment != AssessFail || r.QueryReference.NativeWitness != nil {
		t.Fatalf("canonical contradiction lost: %+v %v", r, e)
	}
	if !validReferenceReplayResult(r) {
		t.Fatalf("canonical partial round-trip: %+v", r)
	}
}

func TestReplayQueryReferenceCallerDeadline(t *testing.T) {
	f := newReferenceFixture(t)
	_, root, prof := referenceReplayFixture(t, f)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	r, e := Replay(ctx, ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
	if e == nil || asError(e).Kind != KindCancelled || r.Assessment == AssessPass || r.QueryReference != nil {
		t.Fatalf("expired caller: %+v %v", r, e)
	}
}

func TestReplayQueryReferencePartialSubject(t *testing.T) {
	for _, axis := range []string{"execution_status", "assessment"} {
		t.Run(axis, func(t *testing.T) {
			f := newReferenceFixture(t)
			_, root, prof := referenceReplayFixture(t, f)
			subject := prof["subject"].(map[string]any)
			if axis == "execution_status" {
				subject[axis] = StatusCancelled
			} else {
				subject[axis] = AssessBlocked
			}
			var docs uint64
			for _, v := range prof["members"].([]any) {
				m := v.(map[string]any)
				if !strings.HasPrefix(m["path"].(string), "evidence/") || m["path"] == "evidence/evidence.json" {
					docs += uint64(m["bytes"].(int))
				}
			}
			testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = docs + 1; return l }
			defer func() { testHookReplayLimits = nil }()
			res, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: mustJSON(t, prof)})
			if err == nil || asError(err).Code != "TOTAL_BYTES_LIMIT" || res.Assessment != AssessFail || res.QueryReference == nil || res.QueryReference.NativeWitness != nil || res.Recorded != (Verdict{StatusCompleted, AssessPass}) {
				t.Fatalf("partial subject mismatch lost: %+v %v", res, err)
			}
			matches := 0
			for _, f := range res.Findings {
				if f.Code == "RECORDED_VERDICT_MISMATCH" {
					matches++
				}
			}
			if matches != 1 || !validReferenceReplayResult(res) {
				t.Fatalf("partial subject result: %+v", res)
			}
		})
	}
}

func TestReplayQueryReferencePartialNativeRegistration(t *testing.T) {
	for _, state := range []string{"verified", "verified-empty", "required-empty", "early-error"} {
		t.Run(state, func(t *testing.T) {
			f := newReferenceFixture(t)
			_, root, prof := referenceReplayFixture(t, f)
			res := runReg(t, root, prof)
			res.Gates, res.EvidenceValid = nil, false // Replay sets evidence_valid only after the gate
			res.QueryReference.Verified = 0
			res.QueryReference.ExecutionStatus, res.ExecutionStatus = StatusResourceLimit, StatusResourceLimit
			res.QueryReference.Assessment, res.Assessment = AssessBlocked, AssessBlocked
			res.Recomputed, res.Consumption = Verdict{}, Consumption{}
			n := res.QueryReference.NativeWitness
			n.ExecutionStatus, n.Assessment = StatusResourceLimit, AssessBlocked
			n.Verified = 1
			wantValid := state == "verified" || state == "early-error"
			if strings.HasSuffix(state, "-empty") {
				n.RegistrationSHA256 = ""
			}
			if state == "required-empty" {
				n.Verified = 0
			}
			if state == "early-error" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				early, err := VerifyQueryWitness(ctx, QueryWitnessRequest{})
				if err == nil || early.Required != 0 || early.Verified != 0 || early.RegistrationSHA256 != "" {
					t.Fatalf("early native failure fixture: %+v %v", early, err)
				}
				res.QueryReference.NativeWitness = &early
			}
			if validReferenceReplayResult(res) != wantValid {
				t.Fatalf("partial native registration %s: %+v", state, res)
			}
			node := &EvidenceNode{ID: "replay", EvidenceMode: res.EvidenceMode, Assessment: res.Assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
			subj := &EvidenceNode{Identities: map[string]string{"run": res.Subject.Run, "attempt": "1", "platform": res.Subject.Platform}}
			env := &replayEnv{}
			env.checkReplayFile(node, subj, map[string][]byte{"result.json": mustJSON(t, res)}, func(code, path, msg string) { env.finding(code, path, msg) })
			if (len(env.findings) == 0) != wantValid {
				t.Fatalf("reader partial native %s: %+v", state, env.findings)
			}
		})
	}
}
