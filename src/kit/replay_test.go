package kit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// ordered builds a JSON object with members in the given order (Go maps sort keys, and
// record completeness depends on member order).
func ordered(kv ...any) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(kv[i].(string))
		v, err := json.Marshal(kv[i+1])
		if err != nil {
			panic(err)
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

const (
	fxProducer = "1111111111111111111111111111111111111111111111111111111111111111"
	fxPolicy   = "2222222222222222222222222222222222222222222222222222222222222222"
	fxExe      = "3333333333333333333333333333333333333333333333333333333333333333"
	fxCompiler = "4444444444444444444444444444444444444444444444444444444444444444"
	fxRuntime  = "659cda7c7f86ebe31cc825dc5da59e9add172dc7"
)

// fxTree is a two-node full tree over src: a root and one named child.
func fxTree(src string, hasError bool) []TreeNode {
	n := uint32(len(src))
	return []TreeNode{
		{Parent: -1, Type: "doc", Named: true, HasError: hasError, EndByte: n, EndPoint: Point{Column: n}},
		{Parent: 0, Type: "word", Named: true, EndByte: n, EndPoint: Point{Column: n}},
	}
}

func fxTreeOut(src string, hasError bool) map[string]any {
	nodes := fxTree(src, hasError)
	return map[string]any{"status": "COMPLETED", "code": "", "form": "full", "parse_ms": 0, "descendant_count": len(nodes), "max_depth": 2, "has_error": hasError,
		"digest": TreeDigest(nodes), "tree": map[string]any{"schema": TreeSchema, "input": map[string]any{"bytes": len(src), "sha256": sum([]byte(src)), "encoding": "UTF-8", "encoding_source": "DECLARATION"},
			"status": "COMPLETED", "capabilities": map[string]string{"query": "UNSUPPORTED"}, "nodes": nodes,
			"identities": []IdentityRef{{"producer", "tsgk-native-build/r1", fxProducer}, {"source", "tsgk-source-bytes/r1", sum([]byte(src))}, {"policy", "tsgk-native-policy/r1", fxPolicy}}}}
}

func fxSummaryOut(tr map[string]any) {
	tree := tr["tree"].(map[string]any)
	tr["form"], tr["summary"] = "summary", map[string]any{"schema": TreeSummarySchema, "status": StatusCompleted, "input": tree["input"], "identities": tree["identities"], "descendant_count": tr["descendant_count"], "digest": map[string]any{"sha256": tr["digest"]}}
	delete(tr, "tree")
}

// fxNative is a mutable S05 fixture: a workload profile with two cases (one with an edit)
// and the matching recorded result.
type fxNative struct {
	profile map[string]any
	cases   []map[string]any
	top     map[string]any
	extra   map[string][]byte // extra files written into the root
	reducer string            // the registered reducer id
}

func newFxNative() *fxNative {
	src1, src2, src2b := "alpha", "beta", "betz"
	in := func(id, s string) map[string]any {
		return map[string]any{"path": "cases/" + id + ".txt", "role": "case", "sha256": sum([]byte(s)), "bytes": len(s)}
	}
	f := &fxNative{extra: map[string][]byte{}, reducer: "native-result-r1"}
	f.profile = map[string]any{"schema": "tsgk-incremental/r1", "id": "fx", "route": "owned-plain", "operation": "native-parse-edit", "symbol": "tree_sitter_fx",
		"encoding": "UTF-8", "output": "tree", "compiler": map[string]any{"name": "cc", "version": "1", "sha256": fxCompiler, "bytes": 1},
		"grammar":      []any{map[string]any{"path": "src/parser.c", "role": "parser", "sha256": fxCompiler, "bytes": 1}},
		"declarations": nil,
		"cases": []any{
			map[string]any{"id": "c1", "input": in("c1", src1), "edits": []any{}, "points": []any{}, "expect": []any{map[string]any{"step": 0, "syntax": "NO_ERROR", "contains": []string{"word"}, "declarations": ""}}},
			map[string]any{"id": "c2", "input": in("c2", src2), "edits": []any{map[string]any{"start_byte": 3, "old_end_byte": 4, "new_end_byte": 4, "old": "YQ==", "new": "eg=="}}, "points": []any{}, "expect": []any{}},
		}}
	step := func(i int, s string, fresh bool) map[string]any {
		m := map[string]any{"step": i, "source_bytes": len(s), "source_sha256": sum([]byte(s)), "edit": nil, "route": nil, "comparison": nil, "incremental": fxTreeOut(s, false), "fresh": nil}
		if fresh {
			m["fresh"] = fxTreeOut(s, false)
			m["route"] = map[string]any{"edit_has_changes": true, "edited_root_end_byte": len(s), "reused_nodes": 1, "fresh_reused_nodes": 0, "proven": true}
			m["comparison"] = map[string]any{"equal": true, "first_difference": nil}
		}
		return m
	}
	claims := func(eq, route, ex string) map[string]string {
		return map[string]string{"incremental_equality": eq, "incremental_route": route, "expectations": ex}
	}
	f.cases = []map[string]any{
		{"id": "c1", "input": in("c1", src1), "encoding": "UTF-8", "execution_status": "COMPLETED", "assessment": "PASS", "code": "", "claims": claims("NOT_CLAIMED", "NOT_CLAIMED", "PASS"),
			"response_status": "COMPLETED", "response_code": "", "producer": nil, "process": nil, "steps": []any{step(0, src1, false)},
			"expectations": []any{map[string]any{"step": 0, "syntax": "NO_ERROR", "contains": []string{"word"}, "declarations": "", "result": "PASS"}}},
		{"id": "c2", "input": in("c2", src2), "encoding": "UTF-8", "execution_status": "COMPLETED", "assessment": "PASS", "code": "", "claims": claims("PASS", "PASS", "NOT_CLAIMED"),
			"response_status": "COMPLETED", "response_code": "", "producer": nil, "process": nil, "steps": []any{step(0, src2, false), step(1, src2b, true)}, "expectations": []any{}},
	}
	f.top = map[string]any{"execution_status": "COMPLETED", "assessment": "PASS",
		"summary": map[string]any{"cases": 2, "execution_statuses": map[string]int{"COMPLETED": 2}, "assessments": map[string]int{"PASS": 2}, "codes": map[string]int{}, "has_error": 0, "batches": 0, "requeued": 0}}
	return f
}

// write materializes the fixture and returns the root and the replay registration.
func (f *fxNative) write(t *testing.T, edit func(root string, reg map[string]any)) (string, []byte) {
	t.Helper()
	root := t.TempDir()
	prof, _ := json.Marshal(f.profile)
	cases := make([]json.RawMessage, len(f.cases))
	for i, c := range f.cases {
		cases[i], _ = json.Marshal(c)
	}
	result := ordered("schema", ReportSchema, "command", "incremental", "execution_status", f.top["execution_status"], "evidence_mode", "NEW_RUN", "assessment", f.top["assessment"],
		"identities", []IdentityRef{{"profile", "tsgk-incremental/r1", sum(prof)}, {"policy", "tsgk-native-policy/r1", fxPolicy}, {"producer", "tsgk-native-build/r1", fxProducer}},
		"findings", []any{}, "coverage", map[string]any{"requested": []string{}, "observed": []string{}, "unsupported": []string{}},
		"result_schema", "tsgk-incremental-result/r1", "profile_id", "fx", "route", "owned-plain", "operation", map[string]any{"name": "native-parse-edit"}, "output", "tree",
		"build", map[string]any{"identity": fxProducer, "executable_sha256": fxExe, "compiler": map[string]any{"sha256": fxCompiler}, "runtime_commit": fxRuntime},
		"build_removed", "VERIFIED", "summary", f.top["summary"], "cases", cases, "batches", []any{}, "platform", "windows/amd64", "started_at", "", "wall_ms", 1, "profile_sha256", sum(prof))
	files := map[string][]byte{"workload.json": prof, "result.json": result, "responses/00000-c1.json": []byte(`{"raw":1}`)}
	for k, v := range f.extra {
		files[k] = v
	}
	roles := map[string]string{"workload.json": "workload-profile", "result.json": "result"}
	var members []any
	for _, p := range sortedKeys(files) {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, files[p], 0o644); err != nil {
			t.Fatal(err)
		}
		role := roles[p]
		switch {
		case role != "":
		case strings.HasPrefix(p, "responses/"):
			role = "response"
		default:
			role = "retained"
		}
		members = append(members, map[string]any{"path": p, "role": role, "bytes": len(files[p]), "sha256": sum(files[p])})
	}
	reg := map[string]any{"schema": ReplaySchema, "id": "fx-replay", "reducer": f.reducer, "operation": "evidence-replay",
		"subject":    map[string]any{"run": "100", "attempt": 1, "platform": "windows/amd64", "commit": "abc", "evidence_mode": "NEW_RUN", "execution_status": f.top["execution_status"], "assessment": f.top["assessment"]},
		"identities": map[string]string{"workload": sum(prof), "policy": fxPolicy, "operation": "native-parse-edit", "platform": "windows/amd64", "producer": fxProducer, "executable": fxExe, "compiler": fxCompiler, "runtime": fxRuntime},
		"records":    nil, "members": members}
	if edit != nil {
		edit(root, reg)
	}
	data, _ := json.Marshal(reg)
	return root, data
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func codes(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Code)
	}
	return out
}

func gateOf(r ReplayResult, id string) ReplayGate {
	for _, g := range r.Gates {
		if g.ID == id {
			return g
		}
	}
	return ReplayGate{}
}

// S07-A01/A10: a complete supported set with an independent registration replays from the
// recorded trees alone (no tool, no process) to the recorded verdict.
func TestReplayNativeResult(t *testing.T) {
	root, reg := newFxNative().write(t, nil)
	t.Setenv("PATH", "")
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil {
		t.Fatal(err)
	}
	if r.Assessment != AssessPass || !r.EvidenceValid || r.EvidenceMode != ModeReplayedRaw || r.ExecutionStatus != StatusCompleted {
		t.Fatalf("replay = %s/%s/%s valid=%v findings=%v", r.ExecutionStatus, r.EvidenceMode, r.Assessment, r.EvidenceValid, r.Findings)
	}
	if r.Subject.Run != "100" || r.Subject.EvidenceMode != ModeNewRun || r.Recorded != r.Recomputed {
		t.Fatalf("subject or verdicts changed: %+v %+v %+v", r.Subject, r.Recorded, r.Recomputed)
	}
	if c := r.Consumption; c.Expected != 2 || c.Consumed != 2 || c.Missing+c.Duplicate+c.Unused+c.OutOfOrder != 0 {
		t.Fatalf("consumption %+v", c)
	}
	for _, id := range []string{"case-binding", "case-status", "tree", "incremental-equality", "incremental-route", "expectations", "verdict", "summary", "responses"} {
		if g := gateOf(r, id); g.EvidenceMode != ModeReplayedRaw || g.Assessment != AssessPass {
			t.Fatalf("gate %s = %+v", id, g)
		}
	}
	if len(r.Explanation) < 3 {
		t.Fatalf("no Korean explanation: %v", r.Explanation)
	}
}

func replayFails(t *testing.T, f *fxNative, edit func(root string, reg map[string]any), want string) ReplayResult {
	t.Helper()
	root, reg := f.write(t, edit)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil {
		t.Fatalf("%s: unexpected error %v", want, err)
	}
	found := slices.Contains(codes(r.Findings), want)
	for _, g := range r.Gates {
		found = found || g.Code == want
	}
	if r.Assessment != AssessFail || r.EvidenceValid || !found {
		t.Fatalf("%s: assessment %s valid %v findings %v gates %+v", want, r.Assessment, r.EvidenceValid, r.Findings, r.Gates)
	}
	return r
}

// S07-A02: missing, duplicate, unused and reordered records and missing, extra or changed
// members are rejected.
func TestReplayConsumption(t *testing.T) {
	f := newFxNative()
	f.cases = f.cases[:1]
	r := replayFails(t, f, nil, "CONSUMPTION_INCOMPLETE")
	if r.Consumption.Missing != 1 {
		t.Fatalf("missing %+v", r.Consumption)
	}
	f = newFxNative()
	f.cases = append(f.cases, f.cases[0])
	if r := replayFails(t, f, nil, "CONSUMPTION_INCOMPLETE"); r.Consumption.Duplicate != 1 {
		t.Fatalf("duplicate %+v", r.Consumption)
	}
	f = newFxNative()
	x := map[string]any{}
	for k, v := range f.cases[0] {
		x[k] = v
	}
	x["id"] = "zz"
	f.cases = append(f.cases, x)
	if r := replayFails(t, f, nil, "CONSUMPTION_INCOMPLETE"); r.Consumption.Unused != 1 {
		t.Fatalf("unused %+v", r.Consumption)
	}
	f = newFxNative()
	f.cases[0], f.cases[1] = f.cases[1], f.cases[0]
	if r := replayFails(t, f, nil, "CONSUMPTION_INCOMPLETE"); r.Consumption.OutOfOrder != 1 {
		t.Fatalf("order %+v", r.Consumption)
	}
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		os.WriteFile(filepath.Join(root, "stray.json"), []byte("{}"), 0o644)
	}, "MEMBER_UNLISTED")
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		os.Remove(filepath.Join(root, "responses", "00000-c1.json"))
	}, "MEMBER_MISSING")
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		os.WriteFile(filepath.Join(root, "responses", "00000-c1.json"), []byte(`{"raw":2}`), 0o644)
	}, "MEMBER_MISMATCH")
	f = newFxNative()
	f.extra["responses/00009-c9.json"] = []byte("{}")
	replayFails(t, f, nil, "RESPONSE_UNBOUND")
}

// S07-A03: a changed source, tool, policy, workload or operation identity, a tree from
// another build in the same result and a summary rewritten without its records are all
// rejected; a re-hashed summary cannot bypass the recomputation.
func TestReplayStaleIdentity(t *testing.T) {
	for _, role := range []string{"workload", "policy", "operation", "platform", "producer", "executable", "compiler", "runtime"} {
		replayFails(t, newFxNative(), func(root string, reg map[string]any) {
			reg["identities"].(map[string]string)[role] = "changed"
		}, "IDENTITY_MISMATCH")
	}
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		reg["identities"].(map[string]string)["query"] = "x"
	}, "IDENTITY_UNKNOWN")
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		delete(reg["identities"].(map[string]string), "producer")
	}, "IDENTITY_UNBOUND")
	f := newFxNative()
	t0 := f.cases[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)
	t0["identities"] = []IdentityRef{{"producer", "tsgk-native-build/r1", fxExe}}
	replayFails(t, f, nil, "MIXED_IDENTITY")
	f = newFxNative()
	f.cases[1]["input"].(map[string]any)["sha256"] = fxExe // a source the workload did not register
	replayFails(t, f, nil, "CASE_BINDING_MISMATCH")
	f = newFxNative()
	f.top["summary"].(map[string]any)["has_error"] = 1 // summary-only change, re-hashed
	replayFails(t, f, nil, "SUMMARY_MISMATCH")
	f = newFxNative()
	f.top["assessment"] = "FAIL" // run verdict relabelled without its cases
	replayFails(t, f, nil, "RUN_VERDICT_MISMATCH")
	replayFails(t, newFxNative(), func(root string, reg map[string]any) {
		reg["subject"].(map[string]any)["assessment"] = "FAIL"
	}, "RECORDED_VERDICT_MISMATCH")
}

// S07-A12: targeted controls. A record that claims PASS over a failed claim, a digest that
// no longer matches its nodes, an incremental/fresh difference recorded as equal and an
// expectation recorded as met when its tree misses the node are each detected.
func TestReplayMutantControls(t *testing.T) {
	f := newFxNative()
	f.cases[1]["claims"].(map[string]string)["incremental_route"] = "FAIL"
	replayFails(t, f, nil, "CLAIM_MISMATCH") // trusting the PASS label would accept this case
	f = newFxNative()
	f.cases[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["digest"] = fxExe
	replayFails(t, f, nil, "TREE_DIGEST_MISMATCH")
	f = newFxNative()
	fr := f.cases[1]["steps"].([]any)[1].(map[string]any)["fresh"].(map[string]any)
	nodes := fxTree("betz", false)
	nodes[1].Type = "other"
	fr["tree"].(map[string]any)["nodes"] = nodes
	fr["digest"] = TreeDigest(nodes)
	replayFails(t, f, nil, "COMPARISON_MISMATCH")
	f = newFxNative()
	f.cases[0]["expectations"].([]any)[0].(map[string]any)["contains"] = []string{"missing"}
	replayFails(t, f, nil, "EXPECTATION_MISMATCH")
	f = newFxNative()
	f.cases[1]["steps"].([]any)[1].(map[string]any)["route"].(map[string]any)["fresh_reused_nodes"] = 1
	replayFails(t, f, nil, "ROUTE_PROOF_MISMATCH")
	f = newFxNative()
	f.cases[1]["execution_status"] = "RESOURCE_LIMIT" // a limited case with a PASS
	replayFails(t, f, nil, "STATUS_ASSESSMENT_INCONSISTENT")
}

// fxNoReuse makes c2's edit step reuse no node, with an error in its step 0 tree when
// errTree, and records the route claim, verdict and code as given (with the matching
// summary and run verdict).
func fxNoReuse(errTree bool, route, assess, code string) *fxNative {
	f := newFxNative()
	c := f.cases[1]
	steps := c["steps"].([]any)
	steps[0].(map[string]any)["incremental"] = fxTreeOut("beta", errTree)
	r := steps[1].(map[string]any)["route"].(map[string]any)
	r["reused_nodes"], r["proven"] = 0, false
	c["claims"].(map[string]string)["incremental_route"] = route
	c["assessment"], c["code"] = assess, code
	sum := f.top["summary"].(map[string]any)
	sum["assessments"] = map[string]int{"PASS": 1, assess: 1}
	if code != "" {
		sum["codes"] = map[string]int{code: 1}
	}
	if errTree {
		sum["has_error"] = 1
	}
	f.top["assessment"] = assess
	return f
}

// under sets the reducer a fixture registers.
func under(reducer string, f *fxNative) *fxNative {
	f.reducer = reducer
	return f
}

// An edit step that reuses no node next to a full error tree is unobservable under the r2
// reducers: incremental_route is recomputed BLOCKED as the native run records it, so a
// BLOCKED record replays valid to BLOCKED and the old FAIL record of the same observation
// is a claim mismatch. Under r1 the same bytes replay exactly as before the rule: the old
// FAIL record is valid and a BLOCKED record is a mismatch. On a clean tree the same
// observation is FAIL under both, and a BLOCKED record of it is a mismatch.
func TestReplayRouteOnErrorTree(t *testing.T) {
	const code = "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"
	const notObserved = "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"
	valid := map[string]*fxNative{
		"r2-error-tree-blocked": under("native-result-r2", fxNoReuse(true, "BLOCKED", AssessBlocked, code)),
		"r1-error-tree-fail":    under("native-result-r1", fxNoReuse(true, "FAIL", AssessFail, notObserved)),
		"r2-clean-tree-fail":    under("native-result-r2", fxNoReuse(false, "FAIL", AssessFail, notObserved)),
		"r1-clean-tree-fail":    under("native-result-r1", fxNoReuse(false, "FAIL", AssessFail, notObserved)),
	}
	for name, f := range valid {
		t.Run(name, func(t *testing.T) {
			assess := f.top["assessment"]
			root, reg := f.write(t, func(root string, reg map[string]any) {
				reg["subject"].(map[string]any)["assessment"] = assess
			})
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			g := gateOf(r, "incremental-route")
			if err != nil || r.Reducer.ID != f.reducer || !r.EvidenceValid || r.Assessment != assess || r.Recomputed.Assessment != assess || g.Failed != 0 {
				t.Fatalf("replay %v: %s %s valid %v recomputed %+v findings %v gate %+v", err, r.Reducer.ID, r.Assessment, r.EvidenceValid, r.Recomputed, r.Findings, g)
			}
		})
	}
	// the recorded claim disagrees with the one the named reducer recomputes
	mismatch := map[string]*fxNative{
		"r2-error-tree-recorded-fail":    under("native-result-r2", fxNoReuse(true, "FAIL", AssessFail, notObserved)),
		"r1-error-tree-recorded-blocked": under("native-result-r1", fxNoReuse(true, "BLOCKED", AssessBlocked, code)),
		"r2-clean-tree-recorded-blocked": under("native-result-r2", fxNoReuse(false, "BLOCKED", AssessBlocked, code)),
		"r1-clean-tree-recorded-blocked": under("native-result-r1", fxNoReuse(false, "BLOCKED", AssessBlocked, code)),
	}
	for name, f := range mismatch {
		t.Run(name, func(t *testing.T) {
			replayFails(t, f, func(root string, reg map[string]any) {
				reg["subject"].(map[string]any)["assessment"] = f.top["assessment"]
			}, "CLAIM_MISMATCH")
		})
	}
	// a fresh parse that reused nodes is a FAIL even next to an error tree
	f := under("native-result-r2", fxNoReuse(true, "BLOCKED", AssessBlocked, code))
	f.cases[1]["steps"].([]any)[1].(map[string]any)["route"].(map[string]any)["fresh_reused_nodes"] = 1
	replayFails(t, f, func(root string, reg map[string]any) {
		reg["subject"].(map[string]any)["assessment"] = AssessBlocked
	}, "CLAIM_MISMATCH")
}

// The r2 error-tree exception needs full old and new trees (only their has_error is
// recomputed from nodes): with the error tree in summary form a recorded BLOCKED is a
// claim mismatch and a recorded FAIL replays without one.
func TestReplayRouteNeedsFullTrees(t *testing.T) {
	summary := func(f *fxNative) *fxNative {
		tr := f.cases[1]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
		fxSummaryOut(tr)
		return under("native-result-r2", f)
	}
	f := summary(fxNoReuse(true, "BLOCKED", AssessBlocked, "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"))
	replayFails(t, f, func(root string, reg map[string]any) {
		reg["subject"].(map[string]any)["assessment"] = AssessBlocked
	}, "CLAIM_MISMATCH")
	f = summary(fxNoReuse(true, "FAIL", AssessFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"))
	root, reg := f.write(t, func(root string, reg map[string]any) {
		reg["subject"].(map[string]any)["assessment"] = AssessFail
	})
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if g := gateOf(r, "incremental-route"); err != nil || g.Failed != 0 || r.Recomputed.Assessment != AssessFail {
		t.Fatalf("summary error tree recorded FAIL: %v %s %+v gate %+v findings %v", err, r.Assessment, r.Recomputed, g, r.Findings)
	}
}

// S07-A04/A12: an unknown reducer, schema or operation is unsupported, never a replayed
// success; a gate the reducer cannot recompute stays recorded and unresolved.
func TestReplayUnsupported(t *testing.T) {
	for field, val := range map[string]string{"reducer": "brightscript-17-gates", "operation": "evidence-replay-fast", "schema": "tsgk-replay/r0"} {
		root, reg := newFxNative().write(t, func(root string, reg map[string]any) { reg[field] = val })
		r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
		var ke *Error
		if !errors.As(err, &ke) || ke.Kind != KindUnsupported || r.Assessment == AssessPass || r.EvidenceMode != ModeNotRun {
			t.Fatalf("%s=%s: err %v report %s/%s", field, val, err, r.EvidenceMode, r.Assessment)
		}
	}
	root, reg := newFxNative().write(t, func(root string, reg map[string]any) { reg["operation"] = "private-corpus-replay" })
	if _, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg}); err == nil || !strings.Contains(err.Error(), "OPERATION_REDUCER_MISMATCH") {
		t.Fatalf("operation mismatch: %v", err)
	}
	// a full tree expectation on declarations is not recorded for r1 trees: recorded only
	f := newFxNative()
	f.profile["declarations"] = map[string]any{"mapping": "declaration-facts-r1", "items": []any{map[string]any{"fact": "type_declaration", "node": "word", "name": "node"}}}
	f.profile["cases"].([]any)[0].(map[string]any)["expect"].([]any)[0].(map[string]any)["declarations"] = "PASS"
	f.cases[0]["expectations"].([]any)[0].(map[string]any)["declarations"] = "PASS"
	root, reg = f.write(t, nil)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Assessment != AssessUnresolved || gateOf(r, "expectations").EvidenceMode != ModeRecordedNotRecomputed || r.Recomputed.Assessment != AssessUnresolved {
		t.Fatalf("recorded-only gate: %v %s %+v", err, r.Assessment, gateOf(r, "expectations"))
	}
	names := []string{}
	for _, ri := range Reducers() {
		names = append(names, ri.ID)
		if len(ri.Gates) == 0 || len(ri.Recorded) == 0 {
			t.Fatalf("reducer %s must declare its gates and what it does not recompute", ri.ID)
		}
	}
	if !slices.Equal(names, []string{"bs-gate-compare-r1", "native-result-r1", "native-result-r2", "oracle-set-r1", "oracle-set-r2", "prepare-native-r1", "private-corpus-r1", "private-corpus-r2"}) {
		t.Fatalf("registry %v", names)
	}
	// an r2 revision differs from its r1 only in id, revision and the route rule it adds; r1
	// stays byte-for-byte the reducer that judged the evidence recorded before the rule
	table := reducerTable()
	for _, id := range []string{"native-result", "oracle-set", "private-corpus"} {
		r1, r2 := table[id+"-r1"].info, table[id+"-r2"].info
		if r2.Revision != "r2" || r2.Operation != r1.Operation || !slices.Equal(r2.Gates, r1.Gates) || !slices.Equal(r2.Recorded, r1.Recorded) ||
			!slices.Equal(r2.Inputs, r1.Inputs) || !slices.Equal(r2.Roles, r1.Roles) || !strings.HasPrefix(r2.Description, r1.Description+"; r2: ") ||
			reducerText(r1) == reducerText(r2) {
			t.Fatalf("%s revisions: %+v %+v", id, r1, r2)
		}
	}
}

// S07-A05: links, traversal names, duplicate keys and nested archives are bounded by the
// shared guards; an archive member is hashed as data, never extracted.
func TestReplayGuards(t *testing.T) {
	root, reg := newFxNative().write(t, func(root string, reg map[string]any) {
		ms := reg["members"].([]any)
		reg["members"] = append(ms, map[string]any{"path": "../escape.json", "role": "response", "bytes": 1, "sha256": fxExe})
	})
	if _, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg}); err == nil || !strings.Contains(err.Error(), "MEMBER_PATH_INVALID") {
		t.Fatalf("traversal: %v", err)
	}
	if _, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: []byte(`{"schema":"tsgk-replay/r1","schema":"x"}`)}); err == nil || !strings.Contains(err.Error(), "JSON_DUPLICATE_KEY") {
		t.Fatalf("duplicate key: %v", err)
	}
	root, reg = newFxNative().write(t, nil)
	if err := os.Symlink(filepath.Join(root, "result.json"), filepath.Join(root, "alias.json")); err == nil {
		if _, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg}); err == nil || !strings.Contains(err.Error(), "LINK_OR_SPECIAL_REJECTED") {
			t.Fatalf("link: %v", err)
		}
	} else {
		t.Logf("symlink unavailable here (%v); link rejection is covered by the shared guard tests", err)
	}
	// a nested archive in a set is an opaque registered member: hashed, never opened
	f := newFxNative()
	f.extra["responses/00001-c2.json"] = append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 64)...)
	root, reg = f.write(t, nil)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Assessment != AssessPass {
		t.Fatalf("opaque archive member: %v %s %v", err, r.Assessment, r.Findings)
	}
}

// S07-A11: cancellation, an over-limit record and an over-limit member end truthfully:
// cancelled, resource limit, or recorded-not-recomputed — never a replayed PASS.
func TestReplayLimitsAndCancel(t *testing.T) {
	root, reg := newFxNative().write(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cancel()
	r, err := Replay(ctx, ReplayRequest{Root: root, Profile: reg})
	if err == nil || r.ExecutionStatus != StatusCancelled || r.Assessment == AssessPass {
		t.Fatalf("cancel: %v %s %s", err, r.ExecutionStatus, r.Assessment)
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.RecordBytes = 1024; return l }
	r, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	testHookReplayLimits = nil
	if err == nil || r.ExecutionStatus != StatusResourceLimit || !strings.Contains(err.Error(), "LIMIT") {
		t.Fatalf("record limit: %v %s", err, r.ExecutionStatus)
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 1024; return l }
	r, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	testHookReplayLimits = nil
	if err != nil || r.EvidenceMode != ModeRecordedNotRecomputed || r.Assessment != AssessUnresolved || !slices.Contains(codes(r.Findings), "RAW_OVER_LIMIT") {
		t.Fatalf("over-limit member: %v %s %s %v", err, r.EvidenceMode, r.Assessment, r.Findings)
	}
	// streaming: a result larger than one record value is read one case at a time
	f := newFxNative()
	for i := 0; i < 200; i++ {
		f.extra[fmt.Sprintf("pad/%03d.bin", i)] = bytes.Repeat([]byte("p"), 64)
	}
	root, reg = f.write(t, nil)
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.RecordBytes = 64 * 1024; return l }
	r, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	testHookReplayLimits = nil
	if err != nil || r.Assessment != AssessPass {
		t.Fatalf("streamed: %v %s %v", err, r.Assessment, r.Findings)
	}
}

// S07-A11 (review r1 M1, m1): a registered member beyond the per-file limit is not read
// and leaves the replay recorded (exit 3), not a failed check; a failure found before it
// still fails the replay.
func TestReplayUnreadMemberOverLimit(t *testing.T) {
	f := newFxNative()
	f.extra["responses/00001-c2.json"] = bytes.Repeat([]byte("r"), 100*1024)
	root, reg := f.write(t, nil)
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 64 * 1024; return l }
	defer func() { testHookReplayLimits = nil }()
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	if err != nil || r.Assessment != AssessUnresolved || r.EvidenceMode != ModeRecordedNotRecomputed || !slices.Contains(codes(r.Findings), "RAW_OVER_LIMIT") {
		t.Fatalf("over-limit response: %v %s %s %v", err, r.EvidenceMode, r.Assessment, r.Findings)
	}
	f.cases[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)["identities"] = []IdentityRef{{"producer", "tsgk-native-build/r1", fxExe}}
	root, reg = f.write(t, nil)
	if r, err = Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg}); err != nil || r.Assessment != AssessFail {
		t.Fatalf("a failure before the over-limit member is kept: %v %s %+v", err, r.Assessment, gateOf(r, "case-binding"))
	}
}

// S07-A14: the same reducer over the same bytes gives the same semantic result; host
// observations are not part of it.
func TestReplayDeterministic(t *testing.T) {
	root, reg := newFxNative().write(t, nil)
	a, _ := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	b, _ := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if !bytes.Equal(ja, jb) {
		t.Fatalf("replay is not deterministic:\n%s\n%s", ja, jb)
	}
}

func TestReplayTreeEnvelopeForms(t *testing.T) {
	for _, bad := range []string{"", "missing-full", "missing-summary", "summary-with-full", "record-with-full", "unknown", "schema", "status", "encoding", "summary-schema", "summary-input"} {
		t.Run(bad, func(t *testing.T) {
			f := newFxNative()
			tr := f.cases[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			switch bad {
			case "schema":
				tr["tree"].(map[string]any)["schema"] = "wrong"
			case "status":
				tr["tree"].(map[string]any)["status"] = StatusNotRun
			case "encoding":
				tr["tree"].(map[string]any)["input"].(map[string]any)["encoding"] = EncodingUTF16LE
			case "summary-schema":
				fxSummaryOut(tr)
				tr["summary"].(map[string]any)["schema"] = "wrong"
			case "summary-input":
				fxSummaryOut(tr)
				tr["summary"].(map[string]any)["input"].(map[string]any)["sha256"] = fxPolicy
			case "missing-full":
				delete(tr, "tree")
			case "missing-summary":
				tr["form"] = "summary"
				delete(tr, "tree")
			case "summary-with-full":
				tr["form"] = "summary"
				tr["summary"] = map[string]any{}
			case "record-with-full":
				tr["form"] = "record"
			case "unknown":
				tr["form"] = "unknown"
			}
			root, reg := f.write(t, nil)
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err != nil || (gateOf(r, "tree").Failed > 0) != (bad != "") {
				t.Fatalf("%s: %v %+v", bad, err, gateOf(r, "tree"))
			}
		})
	}
}
