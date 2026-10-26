package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func TestReuseAcceptedInterruptedCheck(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		for _, stopAt := range []int{2, 3, 5, 8} {
			t.Run(fmt.Sprintf("mismatch=%t/check=%d", mismatch, stopAt), func(t *testing.T) {
				req, raw, projection := acceptedFixture(t, 12)
				projection["retained_extra"] = make([]any, 8192)
				setAcceptedEvidence(&req, raw, projection)
				if mismatch {
					req.Source = []byte("wrong")
				}
				calls := 0
				var interruption error = context.Canceled
				if stopAt == 3 {
					interruption = errors.New("checkpoint sentinel")
				}
				req.Expected.Check = func() error {
					calls++
					if calls >= stopAt {
						return interruption
					}
					return nil
				}
				proof, err := JudgeReuseAccepted(req)
				want := claimBlocked
				if mismatch {
					want = claimFail
				}
				if !errors.Is(err, interruption) || proof.Result != want || mismatch && proof.Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" {
					t.Fatalf("calls=%d proof=%+v err=%v", calls, proof, err)
				}
			})
		}
	}
}

func TestReuseAcceptedDecodeResourceLimit(t *testing.T) {
	for _, member := range []string{"raw", "projection"} {
		for _, mismatch := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mismatch=%t", member, mismatch), func(t *testing.T) {
				req, raw, projection := acceptedFixture(t, 12)
				setAcceptedEvidence(&req, raw, projection)
				deep := []byte(strings.Repeat("[", jsonMaxDepth+1) + "0" + strings.Repeat("]", jsonMaxDepth+1))
				if member == "raw" {
					req.Raw = deep
				} else {
					req.Projection = deep
				}
				if mismatch {
					req.Source = []byte("wrong")
				}
				proof, err := JudgeReuseAccepted(req)
				want := claimBlocked
				if mismatch {
					want = claimFail
				}
				if err == nil || asError(err).Kind != KindResourceLimit || asError(err).Code != "JSON_DEPTH_LIMIT" || proof.Result != want || mismatch && proof.Code != "REUSE_ACCEPTED_SOURCE_MISMATCH" {
					t.Fatalf("proof=%+v err=%v", proof, err)
				}
			})
		}
	}
}

// These are pure checker controls, not evidence that the native runtime executed.
func acceptedFixture(t *testing.T, index int) (ReuseAcceptedRequest, map[string]any, map[string]any) {
	t.Helper()
	_, l := acceptedLedgerFixture(t)
	h := l.originals[index]
	c := acceptedOriginalCase(h)
	versions, points, err := ApplyEdits(EncodingUTF8, []byte(h.SourceUTF8), c.Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	exit := 0
	e := nativewire.Expect{Revision: nativewire.ProtocolR3, ID: c.ID, Nodes: 10000, SourceBytes: uint64(len(h.SourceUTF8)), EditPoints: points, Exit: &exit, Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: "accepted-control", ProducerSHA256: strings.Repeat("a", 64)}}
	cap := map[string]string{"policy": reuseobs.Policy, "runtime_manifest_sha256": reuseobs.RuntimeManifestSHA256}
	producer := map[string]any{"language_version": 15, "runtime_language_version": 15, "runtime_min_compatible": 13, "query": "UNSUPPORTED", "reuse_observation": cap}
	policy := NativePolicy(NativeOperations()["native-parse-edit"], OutputTree, nil)
	raw := map[string]any{"protocol": e.Revision, "id": c.ID, "status": StatusCompleted, "code": "", "producer": producer, "source_bytes": len(h.SourceUTF8), "steps_completed": len(versions), "steps": []any{}, "complete": true}
	projection := map[string]any{"id": c.ID, "input": c.Input, "encoding": EncodingUTF8, "execution_status": StatusCompleted, "response_status": StatusCompleted, "response_code": "", "producer": producer, "process": map[string]any{"status": StatusCompleted, "reason": "EXITED", "exit_code": 0, "stdout_bytes": 0, "stdout_truncated": false, "stderr_truncated": false, "cleanup": map[string]any{"verified": true}}, "steps": []any{}}
	for k, src := range versions {
		sid := nativewire.SourceID{Bytes: uint64(len(src)), SHA256: digestHex(src)}
		e.Sources = append(e.Sources, sid)
		node := TreeNode{Parent: -1, Type: "document", Named: true, HasError: true, StartPoint: Point{}, EndByte: uint32(len(src)), EndPoint: PointAt(EncodingUTF8, src, len(src))}
		digest := TreeDigest([]TreeNode{node})
		tree := map[string]any{"status": StatusCompleted, "code": "", "form": "full", "descendant_count": 1, "has_error": true, "digest": digest, "types": []string{"document"}, "fields": []string{}, "nodes": [][]int64{{-1, 0, -1, 9, 0, int64(len(src)), 0, 0, int64(node.EndPoint.Row), int64(node.EndPoint.Column)}}}
		ptree := map[string]any{"status": StatusCompleted, "code": "", "form": "full", "descendant_count": 1, "has_error": true, "digest": digest, "tree": map[string]any{"schema": TreeSchema, "status": StatusCompleted, "input": rcInput{Encoding: EncodingUTF8, Bytes: sid.Bytes, SHA256: sid.SHA256}, "nodes": []TreeNode{node}}}
		ptree["tree"].(map[string]any)["identities"] = []IdentityRef{{Role: "producer", Schema: "tsgk-native-build/r1", SHA256: e.Observation.ProducerSHA256}, {Role: "source", Schema: "tsgk-source-bytes/r1", SHA256: sid.SHA256}, policy}
		observations := []reuseobs.Observation{{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256, RequestID: c.ID, RunID: e.Observation.RunID, ProducerSHA256: e.Observation.ProducerSHA256, Step: uint64(k), Role: "initial", SourceSHA256: sid.SHA256, SourceBytes: sid.Bytes, Status: "COMPLETE", Events: 3, Bytes: 30, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}}}
		var edit any
		var route, pRoute any
		var fresh, pfresh, comparison any
		if k > 0 {
			ed := c.Edits[k-1]
			e.Edits = append(e.Edits, nativewire.EditSpan{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte})
			edit = nativewire.StepEdit{StartByte: ed.StartByte, OldEndByte: ed.OldEndByte, NewEndByte: ed.NewEndByte, StartPoint: [2]uint32{points[k-1].Start.Row, points[k-1].Start.Column}, OldEndPoint: [2]uint32{points[k-1].OldEnd.Row, points[k-1].OldEnd.Column}, NewEndPoint: [2]uint32{points[k-1].NewEnd.Row, points[k-1].NewEnd.Column}}
			o := observations[0]
			o.Role, o.OldTree, o.OldSourceSHA256, o.Counts = "incremental", true, &e.Sources[k-1].SHA256, &reuseobs.Counts{ParseAfterEdit: 1, Done: 1, Accepted: 1}
			f := observations[0]
			f.Role = "fresh"
			observations = []reuseobs.Observation{o, f}
			route = map[string]any{"edit_has_changes": true, "edited_root_end_byte": len(src), "reused_nodes": 0, "fresh_reused_nodes": 0}
			pRoute = map[string]any{"edit_has_changes": true, "edited_root_end_byte": len(src), "reused_nodes": 0, "fresh_reused_nodes": 0, "proven": false}
			fresh, pfresh, comparison = tree, ptree, map[string]any{"equal": true, "first_difference": nil}
		}
		raw["steps"] = append(raw["steps"].([]any), map[string]any{"step": k, "source_bytes": len(src), "source_sha256": sid.SHA256, "edit": edit, "route": route, "incremental": tree, "fresh": fresh, "observations": observations})
		projection["steps"] = append(projection["steps"].([]any), map[string]any{"step": k, "source_bytes": len(src), "source_sha256": sid.SHA256, "edit": edit, "route": pRoute, "incremental": ptree, "fresh": pfresh, "comparison": comparison, "observations": observations})
	}
	// JSON roundtrip removes accidental object aliases between fresh and incremental.
	clone := func(v any) map[string]any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	req := ReuseAcceptedRequest{Ledger: l, Route: h.Route, Case: c, Source: []byte(h.SourceUTF8), Expected: e, Policy: policy, Platform: "windows/amd64", RecordedPlatform: "windows/amd64"}
	return req, clone(raw), clone(projection)
}

func acceptedStep(m map[string]any, k int) map[string]any {
	return m["steps"].([]any)[k].(map[string]any)
}
func acceptedObs(m map[string]any, k, i int) map[string]any {
	return acceptedStep(m, k)["observations"].([]any)[i].(map[string]any)
}

func TestReuseAcceptedProof(t *testing.T) {
	for _, index := range []int{0, 12, 13, 14} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			req, r, p := acceptedFixture(t, index)
			setAcceptedEvidence(&req, r, p)
			got, err := JudgeReuseAccepted(req)
			if err != nil || got.Result != claimPass {
				t.Fatalf("%+v %v", got, err)
			}
			for _, s := range got.Steps {
				if s.Result != claimPass {
					t.Fatal(s)
				}
			}
		})
	}
	cases := []struct {
		name, want string
		mutate     func(*ReuseAcceptedRequest, map[string]any, map[string]any)
	}{
		{"empty-prior-nodes", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedStep(p, 0)["incremental"].(map[string]any)["tree"].(map[string]any)["nodes"] = []any{}
		}},
		{"prefix-fresh-resume", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 1, 1)
				o["status"], o["reason"], o["events"] = "TRUNCATED", "EVENT_LIMIT", reuseobs.MaxEvents
				c := o["counts"].(map[string]any)
				c["new_parse"], c["resume_parse"], c["done"] = 0, 1, 0
			}
		}},
		{"raw-missing-zero", claimFail, func(_ *ReuseAcceptedRequest, r, _ map[string]any) {
			delete(acceptedStep(r, 1)["route"].(map[string]any), "fresh_reused_nodes")
		}},
		{"tree-identity", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedStep(p, 1)["incremental"].(map[string]any)["tree"].(map[string]any)["identities"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("c", 64)
		}},
		{"policy", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Policy = IdentityRef{} }},
		{"zero", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				acceptedObs(m, 1, 0)["counts"].(map[string]any)["accepted"] = 0
			}
		}},
		{"prefix", claimBlocked, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 1, 0)
				o["status"], o["reason"], o["events"] = "TRUNCATED", "EVENT_LIMIT", reuseobs.MaxEvents
				o["counts"].(map[string]any)["done"] = 0
			}
		}},
		{"overflow", claimBlocked, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 1, 0)
				o["status"], o["reason"], o["counts"] = "OVERFLOW", "COUNTER_OVERFLOW", nil
			}
		}},
		{"prefix-fresh-accepted", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 1, 1)
				o["status"], o["reason"], o["events"] = "TRUNCATED", "EVENT_LIMIT", reuseobs.MaxEvents
				o["counts"].(map[string]any)["done"] = 0
				o["counts"].(map[string]any)["accepted"] = 1
			}
		}},
		{"omitted-old", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 1, 0)
				o["old_tree"], o["old_source_sha256"] = false, nil
				c := o["counts"].(map[string]any)
				c["parse_after_edit"], c["new_parse"], c["accepted"] = 0, 1, 0
			}
		}},
		{"projection-cst", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedStep(p, 1)["incremental"].(map[string]any)["tree"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["type"] = "other"
		}},
		{"projection-missing-false-field", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			delete(acceptedStep(p, 1)["fresh"].(map[string]any)["tree"].(map[string]any)["nodes"].([]any)[0].(map[string]any), "extra")
		}},
		{"projection-null-count", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedStep(p, 1)["route"].(map[string]any)["fresh_reused_nodes"] = nil
		}},
		{"projection-obs", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedObs(p, 1, 0)["counts"].(map[string]any)["accepted"] = 2
		}},
		{"wrong-step", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			for _, m := range []map[string]any{r, p} {
				acceptedObs(m, 1, 0)["step"] = 0
			}
		}},
		{"wrong-producer", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) {
			req.Expected.Observation.ProducerSHA256 = strings.Repeat("b", 64)
		}},
		{"historical-runtime", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) {
			req.Expected.Observation.Manifest = "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080"
		}},
		{"wrong-os", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.RecordedPlatform = "linux/amd64" }},
		{"mac", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) {
			req.Platform, req.RecordedPlatform = "darwin/arm64", "darwin/arm64"
		}},
		{"queries", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Expected.API = true }},
		{"missing-source", claimBlocked, func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Source = nil }},
		{"wrong-source", claimFail, func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Source = []byte("different") }},
		{"submitted-pass-ignored", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			p["reuse_accepted"] = ReuseAcceptedResult{Result: claimPass}
			for _, m := range []map[string]any{r, p} {
				acceptedObs(m, 1, 0)["counts"].(map[string]any)["accepted"] = 0
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, r, p := acceptedFixture(t, 12)
			tc.mutate(&req, r, p)
			setAcceptedEvidence(&req, r, p)
			got, err := JudgeReuseAccepted(req)
			if err != nil || got.Result != tc.want {
				t.Fatalf("want%s got%+v err%v", tc.want, got, err)
			}
		})
	}
	for _, contradiction := range []bool{false, true} {
		req, _, p := acceptedFixture(t, 12)
		if contradiction {
			acceptedObs(p, 1, 1)["counts"].(map[string]any)["accepted"] = 1
		}
		req.Projection, _ = json.Marshal(p)
		got, err := JudgeReuseAccepted(req)
		want := claimBlocked
		if contradiction {
			want = claimFail
		}
		if err != nil || got.Result != want {
			t.Fatalf("raw missing contradiction=%t: %+v %v", contradiction, got, err)
		}
	}
	for _, step := range []int{0, 1} {
		req, r, p := acceptedFixture(t, 12)
		for _, slot := range []string{"incremental", "fresh"} {
			if step == 0 && slot == "fresh" {
				continue
			}
			rt := acceptedStep(r, step)[slot].(map[string]any)
			rt["form"] = "summary"
			rt["nodes"] = nil
			rt["errors"] = map[string]any{"limit": 1000, "total": 0, "truncated": false, "items": []any{}}
			pt := acceptedStep(p, step)[slot].(map[string]any)
			pt["form"] = "summary"
			delete(pt, "tree")
		}
		if step > 0 {
			acceptedStep(p, step)["comparison"] = nil
		}
		setAcceptedEvidence(&req, r, p)
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result != claimBlocked {
			t.Fatalf("summary at%d: %+v %v", step, got, err)
		}
	}
	req, r, p := acceptedFixture(t, 0)
	// Earlier target success does not promote a history whose final tree contradicts raw.
	acceptedStep(p, 4)["incremental"].(map[string]any)["digest"] = strings.Repeat("0", 64)
	setAcceptedEvidence(&req, r, p)
	got, err := JudgeReuseAccepted(req)
	if err != nil || got.Result != claimFail || got.Steps[0].Result != claimPass || got.Steps[1].Result != claimPass {
		t.Fatalf("late failure: %+v %v", got, err)
	}
	stopped := errors.New("stop")
	req.Expected.Check = func() error { return stopped }
	_, err = JudgeReuseAccepted(req)
	if !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	if reflect.DeepEqual(req.Expected.Sources, nil) {
		t.Fatal("fixture lost independent expected source versions")
	}
}

func setAcceptedEvidence(req *ReuseAcceptedRequest, raw, projection map[string]any) {
	req.Raw, _ = json.Marshal(raw)
	if p, ok := projection["process"].(map[string]any); ok {
		p["stdout_bytes"] = len(req.Raw) + 4
	}
	req.Projection, _ = json.Marshal(projection)
}

func TestReuseAcceptedProcessBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		mutate     func(*ReuseAcceptedRequest, map[string]any, map[string]any)
	}{
		{"resource-unprojected", claimBlocked, func(_ *ReuseAcceptedRequest, _, p map[string]any) { acceptedEarlyProcess(p) }},
		{"cleanup-unprojected", claimBlocked, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedEarlyProcess(p)
			p["execution_status"], p["code"] = StatusFailed, "PROCESS_CLEANUP_UNVERIFIED"
			p["process"].(map[string]any)["cleanup"].(map[string]any)["verified"] = false
		}},
		{"early-old-tree", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			acceptedEarlyProcess(p)
			acceptedObs(r, 1, 0)["old_tree"] = false
		}},
		{"early-fresh-accepted", claimFail, func(_ *ReuseAcceptedRequest, r, p map[string]any) {
			acceptedEarlyProcess(p)
			acceptedObs(r, 1, 1)["counts"].(map[string]any)["accepted"] = 1
		}},
		{"early-extra-projection", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			acceptedEarlyProcess(p)
			_, r, projection := acceptedFixture(t, 12)
			_ = r
			p["steps"] = projection["steps"]
			acceptedObs(p, 1, 1)["counts"].(map[string]any)["accepted"] = 1
		}},
		{"capability-order", claimPass, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			p["producer"].(map[string]any)["reuse_observation"] = json.RawMessage(fmt.Sprintf(`{"runtime_manifest_sha256":%q, "policy":%q}`, reuseobs.RuntimeManifestSHA256, reuseobs.Policy))
		}},
		{"capability-null", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			p["producer"].(map[string]any)["reuse_observation"] = nil
		}},
		{"stdout-truncated", claimFail, func(_ *ReuseAcceptedRequest, _, p map[string]any) {
			p["process"].(map[string]any)["stdout_truncated"] = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, r, p := acceptedFixture(t, 12)
			tc.mutate(&req, r, p)
			setAcceptedEvidence(&req, r, p)
			got, err := JudgeReuseAccepted(req)
			if err != nil || got.Result != tc.want {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
	for _, status := range []string{StatusResourceLimit, StatusFailed} {
		t.Run("natural-"+status, func(t *testing.T) {
			req, r, p := acceptedFixture(t, 12)
			code, exit := "PARSE_TIME_LIMIT", 3
			if status == StatusFailed {
				code, exit = "PARSE_NULL", 4
			}
			req.Expected.Exit = &exit
			r["status"], r["code"], r["steps_completed"] = status, code, 0
			r["steps"] = r["steps"].([]any)[:1]
			p["steps"] = p["steps"].([]any)[:1]
			for _, m := range []map[string]any{r, p} {
				o := acceptedObs(m, 0, 0)
				o["status"], o["reason"] = "UNAVAILABLE", "PARSE_INCOMPLETE"
				o["counts"].(map[string]any)["done"] = 0
			}
			acceptedStep(r, 0)["incremental"] = map[string]any{"status": status, "code": code, "form": nil}
			acceptedStep(p, 0)["incremental"] = map[string]any{"status": status, "code": code, "form": "", "descendant_count": 0, "has_error": false, "digest": "", "tree": nil}
			p["execution_status"], p["code"], p["response_status"], p["response_code"] = status, code, status, code
			process := p["process"].(map[string]any)
			process["status"], process["exit_code"] = StatusFailed, exit
			setAcceptedEvidence(&req, r, p)
			got, err := JudgeReuseAccepted(req)
			if err != nil || got.Result != claimBlocked {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
	req, r, p := acceptedFixture(t, 12)
	setAcceptedEvidence(&req, r, p)
	var wire map[string]any
	_ = json.Unmarshal(req.Projection, &wire)
	wire["process"].(map[string]any)["stdout_bytes"] = len(req.Raw) + 5
	req.Projection, _ = json.Marshal(wire)
	got, err := JudgeReuseAccepted(req)
	if err != nil || got.Result != claimFail {
		t.Fatalf("transport %+v %v", got, err)
	}
}

func acceptedEarlyProcess(p map[string]any) {
	p["execution_status"], p["code"] = StatusResourceLimit, "WALL_LIMIT"
	p["response_status"], p["response_code"], p["producer"], p["steps"] = "", "", nil, []any{}
	p["process"].(map[string]any)["status"], p["process"].(map[string]any)["reason"] = StatusResourceLimit, "WALL_LIMIT"
}

// The same immutable evidence must fail the public proof and pass only the exact private control.
func TestReuseAcceptedOmitControl(t *testing.T) {
	for _, index := range []int{0, 12, 13, 14} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			tests := []struct {
				name   string
				mutate func(*ReuseAcceptedRequest, map[string]any, map[string]any)
				pass   bool
			}{
				{"complete-omit", nil, true},
				{"wrong-legacy-step", func(_ *ReuseAcceptedRequest, _, p map[string]any) {
					p["code"] = "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_99"
				}, false},
				{"additional-producer-failure", func(_ *ReuseAcceptedRequest, _, p map[string]any) { p["producer"].(map[string]any)["query"] = "BROKEN" }, false},
				{"additional-initial-cst-failure", func(_ *ReuseAcceptedRequest, _, p map[string]any) {
					acceptedStep(p, 0)["incremental"].(map[string]any)["tree"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["type"] = "other"
				}, false},
				{"fresh-accepted", func(_ *ReuseAcceptedRequest, r, p map[string]any) {
					for _, m := range []map[string]any{r, p} {
						acceptedObs(m, 1, 1)["counts"].(map[string]any)["accepted"] = 1
					}
				}, false},
				{"missing-source", func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Source = nil }, false},
				{"wrong-source", func(req *ReuseAcceptedRequest, _, _ map[string]any) { req.Source = []byte("wrong") }, false},
				{"unfinished-collector", func(_ *ReuseAcceptedRequest, r, p map[string]any) {
					for _, m := range []map[string]any{r, p} {
						o := acceptedObs(m, 1, 0)
						o["status"], o["reason"], o["events"] = "TRUNCATED", "EVENT_LIMIT", reuseobs.MaxEvents
						o["counts"].(map[string]any)["done"] = 0
					}
				}, false},
				{"transport-truncated", func(_ *ReuseAcceptedRequest, _, p map[string]any) {
					p["process"].(map[string]any)["stdout_truncated"] = true
				}, false},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					req, r, p := acceptedFixture(t, index)
					for k := 1; k < len(r["steps"].([]any)); k++ {
						for _, m := range []map[string]any{r, p} {
							o := acceptedObs(m, k, 0)
							o["old_tree"], o["old_source_sha256"] = false, nil
							c := o["counts"].(map[string]any)
							c["new_parse"], c["parse_after_edit"], c["accepted"] = 1, 0, 0
						}
					}
					p["assessment"], p["code"] = AssessFail, "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_"+fmt.Sprint(req.Ledger.original(req.Case.ID).OmitOldFirst)
					p["claims"] = rcClaims{IncrementalEquality: claimPass, IncrementalRoute: claimFail, Expectations: "NOT_CLAIMED"}
					p["reuse_observation_claim"] = claimFail
					if tc.mutate != nil {
						tc.mutate(&req, r, p)
					}
					setAcceptedEvidence(&req, r, p)
					originalRaw, originalProjection := string(req.Raw), string(req.Projection)
					public, err := JudgeReuseAccepted(req)
					if err != nil || public.Result == claimPass {
						t.Fatalf("public=%+v err=%v", public, err)
					}
					result, err := checkReuseOmitControl(req)
					if err != nil || (result == claimPass) != tc.pass {
						t.Fatalf("control=%s err=%v wantPass=%t", result, err, tc.pass)
					}
					if string(req.Raw) != originalRaw || string(req.Projection) != originalProjection {
						t.Fatal("control mutated evidence")
					}
				})
			}
			req, r, p := acceptedFixture(t, index)
			setAcceptedEvidence(&req, r, p)
			if result, err := checkReuseOmitControl(req); err != nil || result == claimPass {
				t.Fatalf("normal evidence admitted as omit control: %s %v", result, err)
			}
		})
	}
}
