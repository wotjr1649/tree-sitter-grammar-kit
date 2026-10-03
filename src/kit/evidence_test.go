package kit

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func gateDoc(gate, status string, points ...map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"gate": gate, "status": status, "notes": []string{}, "points": points})
	return b
}

// rawFloat keeps a float literal exactly (json.Marshal of 1.0 would write the int 1).
type rawFloat float64

func (f rawFloat) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(f), 'g', 17, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return []byte(s), nil
}

func ulps(x float64, n int) float64 {
	for range n {
		x = math.Nextafter(x, math.Inf(1))
	}
	return x
}

// S07-A06: the historical BrightScript comparison is exact except the allowlisted
// log-derived exponents (<= 2 ULP, finite normal, same sign); signed zero and subnormals are
// bit-exact; a 2 ULP corridor may not straddle the original threshold; the threshold
// decision and the max winner must not change; ints and floats are different types.
func TestCompareGates(t *testing.T) {
	mem := func(e any) map[string]any {
		return map[string]any{"family": "f", "sizes": []int{1000, 4000}, "exponent": e, "pass": true}
	}
	cmp := func(a, b []byte) (GateComparison, string) {
		r, err := CompareGates(a, b)
		if err != nil {
			code, _, _ := strings.Cut(err.Error(), ": ")
			return r, code
		}
		return r, r.Status
	}
	base := 1.0594822625200103
	cases := []struct {
		name string
		a, b []byte
		want string
	}{
		{"bitwise", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), "BITWISE_EQUAL"},
		{"one ulp derived", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(ulps(base, 1)))), "REPLAY_EQUIVALENT_WITH_DECLARED_ROUNDING"},
		{"two ulp derived", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(ulps(base, 2)))), "REPLAY_EQUIVALENT_WITH_DECLARED_ROUNDING"},
		{"three ulp derived (a rounding mutant would pass)", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(ulps(base, 3)))), "ULP_LIMIT_EXCEEDED"},
		{"signed zero", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(0))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(math.Copysign(0, -1)))), "EXACT_FLOAT_DIFFERENCE"},
		{"subnormal one ulp", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(5e-324))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(1e-323))), "EXACT_FLOAT_DIFFERENCE"},
		{"corridor straddles threshold", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(1.5))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(1.5))), "NUMERIC_BOUNDARY_INDETERMINATE"},
		{"threshold decision", gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(1.4))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(1.6))), "THRESHOLD_DECISION_DIFFERENCE"},
		{"non-derived float exact", gateDoc("CANCEL", "PASS", map[string]any{"case": "c", "ms": rawFloat(base)}), gateDoc("CANCEL", "PASS", map[string]any{"case": "c", "ms": rawFloat(ulps(base, 1))}), "EXACT_FLOAT_DIFFERENCE"},
		{"int vs float", gateDoc("CANCEL", "PASS", map[string]any{"case": "c", "n": 1}), gateDoc("CANCEL", "PASS", map[string]any{"case": "c", "n": rawFloat(1)}), "TYPE_DIFFERENCE"},
		{"verdict", gateDoc("CANCEL", "PASS"), gateDoc("CANCEL", "FAIL"), "VALUE_DIFFERENCE"},
		{"point order", gateDoc("CANCEL", "PASS", map[string]any{"case": "a"}, map[string]any{"case": "b"}), gateDoc("CANCEL", "PASS", map[string]any{"case": "b"}, map[string]any{"case": "a"}), "VALUE_DIFFERENCE"},
		{"unknown gate", gateDoc("GATE-18", "PASS"), gateDoc("GATE-18", "PASS"), "UNKNOWN_GATE_ID"},
	}
	sweep := func(e, a, b float64) map[string]any {
		return map[string]any{"completed": true, "case": "s", "exponents": map[string]any{"400-20000": rawFloat(a), "4000-20000": rawFloat(b)}, "exponent": rawFloat(e)}
	}
	cases = append(cases,
		struct {
			name string
			a, b []byte
			want string
		}{"max winner changed", gateDoc("REGRESSION-SWEEP", "PASS", sweep(1.1, 1.1, 1.0)), gateDoc("REGRESSION-SWEEP", "PASS", sweep(1.1, 1.0, 1.1)), "AGGREGATE_SOURCE_CHANGED"},
		struct {
			name string
			a, b []byte
			want string
		}{"aggregate not the max", gateDoc("REGRESSION-SWEEP", "PASS", sweep(1.0, 1.1, 1.0)), gateDoc("REGRESSION-SWEEP", "PASS", sweep(1.0, 1.1, 1.0)), "AGGREGATE_VALUE_DIFFERENCE"})
	for _, c := range cases {
		if _, got := cmp(c.a, c.b); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	r, _ := cmp(gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(base))), gateDoc("B5-01-MEMORY", "PASS", mem(rawFloat(ulps(base, 2)))))
	if len(r.Differences) != 1 || r.Differences[0].ULPs != 2 || r.Policy != GateComparePolicy || r.Differences[0].Threshold != 1.5 {
		t.Fatalf("difference record %+v", r)
	}
}

// S07-A04/A06: through replay, the gate comparison is recomputed but the archived gate
// recomputation from raw is not ported: the result is UNRESOLVED, never PASS.
func TestReplayGateCompare(t *testing.T) {
	root := t.TempDir()
	a := gateDoc("CANCEL", "PASS", map[string]any{"case": "c", "n": 1})
	files := map[string][]byte{"recorded/CANCEL.json": a, "recomputed/CANCEL.json": a}
	ms := writeSet(t, root, files, func(p string) string {
		if strings.HasPrefix(p, "recorded/") {
			return "recorded-gate"
		}
		return "recomputed-gate"
	})
	r := runReg(t, root, registration("bs-gate-compare-r1", "evidence-replay", "PASS", map[string]string{}, []string{"CANCEL"}, ms))
	if !r.EvidenceValid || r.Assessment != AssessUnresolved || gateOf(r, "gate-compare").EvidenceMode != ModeReplayedRaw || gateOf(r, "raw-recompute").EvidenceMode != ModeRecordedNotRecomputed {
		t.Fatalf("gate compare replay: %s %v %+v", r.Assessment, r.Findings, r.Gates)
	}
	os.WriteFile(filepath.Join(root, "recomputed", "CANCEL.json"), gateDoc("CANCEL", "FAIL", map[string]any{"case": "c", "n": 1}), 0o644)
	ms = writeSet(t, root, map[string][]byte{"recorded/CANCEL.json": a, "recomputed/CANCEL.json": gateDoc("CANCEL", "FAIL", map[string]any{"case": "c", "n": 1})}, func(p string) string {
		if strings.HasPrefix(p, "recorded/") {
			return "recorded-gate"
		}
		return "recomputed-gate"
	})
	r = runReg(t, root, registration("bs-gate-compare-r1", "evidence-replay", "PASS", map[string]string{}, []string{"CANCEL"}, ms))
	if r.EvidenceValid || gateOf(r, "gate-compare").Code != "VALUE_DIFFERENCE" {
		t.Fatalf("changed verdict: %+v", gateOf(r, "gate-compare"))
	}
}

// fxGraph is an evidence graph: an old FAIL run, a fixed new PASS run that supersedes it, a
// replay of the old run's raw and an authorized carry of the new run.
type fxGraph struct {
	nodes  []map[string]any
	policy map[string]any
	files  map[string][]byte
}

func ident(policy, run string) map[string]string {
	return map[string]string{"source": fxExe, "tool": fxCompiler, "input": fxProducer, "query": "", "policy": policy, "comparator": "tsgk-tree-digest/r1",
		"protocol": "tsgk-native/r1", "workload": "w", "platform": "windows/amd64", "run": run, "attempt": "1"}
}

func newFxGraph() *fxGraph {
	node := func(id, kind, status, mode, assess string, ids map[string]string, refs ...map[string]string) map[string]any {
		return map[string]any{"id": id, "kind": kind, "execution_status": status, "evidence_mode": mode, "assessment": assess, "recorded_assessment": "",
			"identities": ids, "files": []any{}, "refs": refs}
	}
	g := &fxGraph{files: map[string][]byte{"raw/old.json": []byte(`{"old":true}`)}}
	old := node("old", "run", "COMPLETED", "NEW_RUN", "FAIL", ident("p1", "10"))
	old["files"] = []any{map[string]any{"path": "raw/old.json", "role": "raw", "bytes": 12, "sha256": sum(g.files["raw/old.json"])}}
	g.nodes = []map[string]any{
		old,
		node("new", "run", "COMPLETED", "NEW_RUN", "PASS", ident("p2", "11"), map[string]string{"relation": "supersedes", "node": "old"}),
		node("old-replay", "replay", "COMPLETED", "REPLAYED_RAW", "FAIL", ident("p1", "12"), map[string]string{"relation": "subject", "node": "old"}),
		node("new-carry", "carry", "NOT_RUN", "CARRIED_FORWARD", "PASS", ident("p2", "13"), map[string]string{"relation": "carries", "node": "new"}),
		node("hist", "record", "COMPLETED", "RECORDED_NOT_RECOMPUTED", "UNRESOLVED", ident("p0", "9")),
	}
	g.nodes[4]["recorded_assessment"] = "PASS"
	g.policy = map[string]any{"schema": EvidencePolicySchema, "id": "fx-policy", "evidence_sha256": "",
		"required":      []any{map[string]any{"node": "new", "kind": "run", "identities": map[string]string{"policy": "p2", "run": "11", "attempt": "1"}}},
		"carry_forward": []any{map[string]any{"node": "new-carry", "origin": "new", "relation": CarryUnchangedDependency, "authorized_by": "S07 fixture"}},
		"eligibility":   nil}
	return g
}

func (g *fxGraph) run(t *testing.T, edit func(root string, policy map[string]any)) EvidenceResult {
	t.Helper()
	root := t.TempDir()
	doc, _ := json.Marshal(map[string]any{"schema": EvidenceSchema, "nodes": g.nodes})
	files := map[string][]byte{"evidence.json": doc}
	for k, v := range g.files {
		files[k] = v
	}
	writeSet(t, root, files, func(string) string { return "" })
	g.policy["evidence_sha256"] = sum(doc)
	if edit != nil {
		edit(root, g.policy)
	}
	pol, _ := json.Marshal(g.policy)
	r, err := VerifyEvidence(ctxT(t), EvidenceRequest{Root: root, Policy: pol})
	if err != nil {
		t.Fatalf("verify evidence: %v", err)
	}
	return r
}

func evidenceFails(t *testing.T, g *fxGraph, edit func(string, map[string]any), want string) {
	t.Helper()
	r := g.run(t, edit)
	if r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), want) {
		t.Fatalf("%s: %s %v", want, r.Assessment, r.Findings)
	}
}

// S07-A07/A08/A09/A03: the historical FAIL and the new PASS stay two attributable runs, a
// replay keeps its subject's run, only the adopted unchanged-dependency carry succeeds,
// NEW_RUN eligibility rejects replayed and carried rows even when integrity passes, and a
// changed run or attempt is a stale result.
func TestVerifyEvidence(t *testing.T) {
	if r := newFxGraph().run(t, nil); r.Assessment != AssessPass || r.Nodes != 5 || r.Files != 1 {
		t.Fatalf("valid graph: %s %v", r.Assessment, r.Findings)
	}
	g := newFxGraph()
	g.nodes[3]["identities"].(map[string]string)["comparator"] = "tsgk-tree-digest/r2"
	evidenceFails(t, g, nil, "CARRY_DEPENDENCY_CHANGED")
	g = newFxGraph()
	g.nodes[3]["identities"].(map[string]string)["source"] = fxPolicy
	evidenceFails(t, g, nil, "CARRY_DEPENDENCY_CHANGED")
	evidenceFails(t, newFxGraph(), func(_ string, p map[string]any) { p["carry_forward"] = []any{} }, "CARRY_FORWARD_UNAUTHORIZED")
	evidenceFails(t, newFxGraph(), func(_ string, p map[string]any) {
		p["carry_forward"].([]any)[0].(map[string]any)["relation"] = "compatible-comparator-r1"
	}, "CARRY_RELATION_UNSUPPORTED")
	g = newFxGraph()
	g.nodes[3]["refs"] = []map[string]string{{"relation": "carries", "node": "old"}}
	g.nodes[3]["identities"] = ident("p1", "13")
	g.nodes[3]["assessment"] = "FAIL"
	evidenceFails(t, g, nil, "CARRY_RELATION_UNSUPPORTED")
	g = newFxGraph()
	g.nodes[3]["assessment"] = "FAIL"
	evidenceFails(t, g, nil, "CARRY_ASSESSMENT_CHANGED")
	// A07: a fix relabelled onto the old run identity
	g = newFxGraph()
	g.nodes[1]["identities"] = ident("p2", "10")
	evidenceFails(t, g, nil, "RUN_RELABELED")
	g = newFxGraph()
	g.nodes[4]["identities"] = ident("p0", "11") // the historical record relabelled as the new run
	evidenceFails(t, g, nil, "RUN_IDENTITY_DUPLICATE")
	g = newFxGraph()
	g.nodes[2]["identities"] = ident("p1", "10")
	evidenceFails(t, g, nil, "REPLAY_RELABELS_SUBJECT")
	g = newFxGraph()
	g.nodes[2]["identities"].(map[string]string)["input"] = fxPolicy
	evidenceFails(t, g, nil, "REPLAY_SUBJECT_MISMATCH")
	// A09: final qualification requires NEW_RUN
	evidenceFails(t, newFxGraph(), func(_ string, p map[string]any) {
		p["eligibility"] = map[string]any{"nodes": []string{"new", "old-replay", "new-carry"}, "modes": []string{"NEW_RUN"}}
	}, "ELIGIBILITY_REJECTED")
	if r := newFxGraph().run(t, func(_ string, p map[string]any) {
		p["eligibility"] = map[string]any{"nodes": []string{"new"}, "modes": []string{"NEW_RUN"}}
	}); r.Assessment != AssessPass {
		t.Fatalf("NEW_RUN row eligible: %v", r.Findings)
	}
	// A03: run, attempt and policy changes are stale
	for role, val := range map[string]string{"run": "99", "attempt": "2", "policy": "p3"} {
		evidenceFails(t, newFxGraph(), func(_ string, p map[string]any) {
			p["required"].([]any)[0].(map[string]any)["identities"].(map[string]string)[role] = val
		}, "IDENTITY_MISMATCH")
	}
	evidenceFails(t, newFxGraph(), func(_ string, p map[string]any) { p["evidence_sha256"] = fxExe }, "EVIDENCE_ANCHOR_MISMATCH")
	// A05 and integrity
	g = newFxGraph()
	g.nodes[0]["refs"] = []map[string]string{{"relation": "supersedes", "node": "new"}}
	evidenceFails(t, g, nil, "GRAPH_CYCLE")
	evidenceFails(t, newFxGraph(), func(root string, _ map[string]any) {
		os.WriteFile(filepath.Join(root, "raw", "old.json"), []byte(`{"old":false}`), 0o644)
	}, "FILE_MEMBER_MISMATCH")
	evidenceFails(t, newFxGraph(), func(root string, _ map[string]any) {
		os.WriteFile(filepath.Join(root, "raw", "extra.json"), []byte(`{}`), 0o644)
	}, "FILE_UNLISTED")
	g = newFxGraph()
	g.nodes[4]["assessment"] = "PASS"
	evidenceFails(t, g, nil, "AXIS_INVALID")
	g = newFxGraph()
	g.nodes = append(g.nodes, g.nodes[0])
	evidenceFails(t, g, nil, "NODE_DUPLICATE")
	g = newFxGraph()
	g.nodes[1]["refs"] = []map[string]string{{"relation": "subject", "node": "old"}}
	evidenceFails(t, g, nil, "REF_RELATION_INVALID")
	g = newFxGraph()
	g.nodes[0]["files"] = []any{map[string]any{"path": "../raw/old.json", "role": "raw", "bytes": 12, "sha256": fxExe}}
	evidenceFails(t, g, nil, "FILE_INVALID")
	// a file beyond the operation limits ends RESOURCE_LIMIT, not a failed check
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 4; return l }
	defer func() { testHookReplayLimits = nil }()
	g = newFxGraph()
	root := t.TempDir()
	doc, _ := json.Marshal(map[string]any{"schema": EvidenceSchema, "nodes": g.nodes})
	writeSet(t, root, map[string][]byte{"evidence.json": doc, "raw/old.json": g.files["raw/old.json"]}, func(string) string { return "" })
	g.policy["evidence_sha256"] = sum(doc)
	pol, _ := json.Marshal(g.policy)
	r, err := VerifyEvidence(ctxT(t), EvidenceRequest{Root: root, Policy: pol})
	if err == nil || r.ExecutionStatus != StatusResourceLimit || r.Assessment == AssessFail {
		t.Fatalf("over-limit file: %v %s %s", err, r.ExecutionStatus, r.Assessment)
	}
}
