package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// qfxSymbolRole adds an owned symbol-mode role (#218) executed on windows and deferred
// elsewhere: one all-OK case and one zero-active case.
func (f *qfx) qfxSymbolRole() *QualRole {
	w := QualWorkload{Set: "s05-fx-symbols", Profile: "s05-fx-symbols", Route: "csharp", Operation: "native-parse-edit", Output: "tree", Symbol: "tree_sitter_c_sharp",
		Grammar: []NativeInput{{Path: "src/parser.c", Role: "parser", SHA256: fxCompiler, Bytes: 1}}, Queries: []QualQuery{},
		Preprocess: &Preprocess{Mode: PreprocessMode, Symbols: []string{"A"}}, RuntimeCommit: strings.Repeat("a", 40)}
	sel := func(status string, active ...ByteRange) []QualPreprocessStep {
		return []QualPreprocessStep{{Step: 0, Status: status, Directives: []PreprocessDirective{}, Active: append([]ByteRange{}, active...), Inactive: []ByteRange{}}}
	}
	w.Cases = []QualCase{
		{ID: "ok", Role: "support", Input: NativeInput{SHA256: sum([]byte("alpha")), Bytes: 5}, Edits: []Edit{}, Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{}}},
			QueryExpect: []QueryExpectation{}, Covers: map[string][]string{}, PreprocessExpect: sel(PreprocessOK, ByteRange{0, 5})},
		{ID: "zero", Role: "support", Input: NativeInput{SHA256: sum([]byte("beta")), Bytes: 4}, Edits: []Edit{}, Expect: []StepExpectation{}, QueryExpect: []QueryExpectation{},
			Covers: map[string][]string{}, ExpectStatus: StatusNotRun, ExpectAssessment: AssessBlocked, ExpectCode: "PREPROCESS_ZERO_ACTIVE", PreprocessExpect: sel(PreprocessZeroActive)},
	}
	f.inv.ExtraRoles = append(f.inv.ExtraRoles, QualRole{ID: "fx-symbols", Role: "owned", Status: RoleExecuted, Reason: "owned symbol-mode fixture", Platforms: []string{"windows-amd64"},
		NotApplicable: map[string]string{}, Deferred: map[string]string{"linux-amd64": "deferred", "darwin-arm64": "deferred"}, Workloads: []QualWorkload{w}})
	return &f.inv.ExtraRoles[len(f.inv.ExtraRoles)-1]
}

// #218 inventory r5: deferred platforms partition an executed role with its executed and not
// applicable ones, and a symbol-mode workload is a C# parse-edit run of an extra role whose
// cases register the selection of every step up to the first stopped one and either the
// syntax of every step or the fixed stopped outcome. Without host evidence the executed row
// is MISSING and the deferred rows stay DEFERRED with no axis.
func TestQualificationInventorySymbolRole(t *testing.T) {
	f := newQfx(t)
	f.qfxSymbolRole()
	if _, err := ParseQualificationInventory(f.invBytes(t)); err != nil {
		t.Fatal(err)
	}
	ok := func(inv *QualificationInventory) *QualCase { return &inv.ExtraRoles[2].Workloads[0].Cases[0] }
	zero := func(inv *QualificationInventory) *QualCase { return &inv.ExtraRoles[2].Workloads[0].Cases[1] }
	w := func(inv *QualificationInventory) *QualWorkload { return &inv.ExtraRoles[2].Workloads[0] }
	for name, tc := range map[string]struct {
		code string
		mut  func(inv *QualificationInventory)
	}{
		"deferred not executed": {"ROLE_INVALID", func(inv *QualificationInventory) { inv.ExtraRoles[1].Deferred = map[string]string{"linux-amd64": "x"} }},
		"deferred and executed": {"ROLE_INVALID", func(inv *QualificationInventory) { inv.ExtraRoles[2].Deferred["windows-amd64"] = "x" }},
		"deferred no reason":    {"ROLE_INVALID", func(inv *QualificationInventory) { inv.ExtraRoles[2].Deferred["linux-amd64"] = "" }},
		"platform unnamed":      {"ROLE_INVALID", func(inv *QualificationInventory) { delete(inv.ExtraRoles[2].Deferred, "darwin-arm64") }},
		"platform twice": {"ROLE_INVALID", func(inv *QualificationInventory) {
			inv.ExtraRoles[2].Platforms = []string{"windows-amd64", "windows-amd64"}
		}},
		"deferred unknown": {"ROLE_INVALID", func(inv *QualificationInventory) { inv.ExtraRoles[2].Deferred["plan9-amd64"] = "x" }},
		// a route workload that is otherwise a valid symbol workload of a csharp route
		"preprocess in route": {"WORKLOAD_INVALID", func(inv *QualificationInventory) {
			r := &inv.Routes[0]
			r.Route, r.Workload = "csharp", *w(inv)
			r.Workload.Set, r.Workload.Profile = "s05-fx-route", "s05-fx-route"
			r.Requirements = []QualRequirement{{Row: "csharp-B01", Kinds: []string{"P"}, AlternativesStatus: AlternativesPending, Alternatives: []string{}}}
		}},
		"preprocess queries": {"WORKLOAD_INVALID", func(inv *QualificationInventory) {
			w(inv).Queries = []QualQuery{{ID: "q.x", SHA256: sum([]byte(qfxQuery))}}
		}},
		"preprocess route":     {"WORKLOAD_INVALID", func(inv *QualificationInventory) { w(inv).Route = "fxa" }},
		"preprocess operation": {"WORKLOAD_INVALID", func(inv *QualificationInventory) { w(inv).Operation = "native-query" }},
		"preprocess symbol": {"WORKLOAD_INVALID", func(inv *QualificationInventory) {
			w(inv).Preprocess = &Preprocess{Mode: PreprocessMode, Symbols: []string{"true"}}
		}},
		"preprocess mode": {"WORKLOAD_INVALID", func(inv *QualificationInventory) {
			w(inv).Preprocess = &Preprocess{Mode: "csharp-symbols/r2", Symbols: []string{}}
		}},
		"runtime missing":      {"WORKLOAD_INVALID", func(inv *QualificationInventory) { w(inv).RuntimeCommit = "" }},
		"runtime without mode": {"WORKLOAD_INVALID", func(inv *QualificationInventory) { inv.Routes[0].Workload.RuntimeCommit = strings.Repeat("a", 40) }},
		"selection elsewhere": {"CASE_INVALID", func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[0].PreprocessExpect = ok(inv).PreprocessExpect
		}},
		"selection missing": {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).PreprocessExpect = nil }},
		"selection step":    {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).PreprocessExpect[0].Step = 1 }},
		"selection status":  {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).PreprocessExpect[0].Status = "SKIPPED" }},
		"step after stop": {"CASE_INVALID", func(inv *QualificationInventory) {
			c := zero(inv)
			c.Edits = []Edit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("b"), New: []byte("c")}}
			c.PreprocessExpect = append(c.PreprocessExpect, QualPreprocessStep{Step: 1, Status: PreprocessZeroActive, Directives: []PreprocessDirective{}, Active: []ByteRange{}, Inactive: []ByteRange{}})
		}},
		"edit step unselected": {"CASE_INVALID", func(inv *QualificationInventory) {
			ok(inv).Edits = []Edit{{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("a"), New: []byte("b")}}
		}},
		"syntax missing": {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).Expect = []StepExpectation{} }},
		"ok with status": {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).ExpectStatus = StatusNotRun }},
		"stopped code":   {"CASE_INVALID", func(inv *QualificationInventory) { zero(inv).ExpectCode = "PREPROCESS_MALFORMED" }},
		"stopped syntax": {"CASE_INVALID", func(inv *QualificationInventory) { zero(inv).Expect = ok(inv).Expect }},
		"stopped complete": {"CASE_INVALID", func(inv *QualificationInventory) {
			zero(inv).ExpectStatus, zero(inv).ExpectAssessment, zero(inv).ExpectCode = "", "", ""
		}},
		"symbol covers":      {"SUPPORT_COVERS", func(inv *QualificationInventory) { ok(inv).Covers = map[string][]string{"fxa-B01": {"P"}} }},
		"symbol requirement": {"CASE_INVALID", func(inv *QualificationInventory) { ok(inv).Role = "requirement" }},
	} {
		expectCode(t, name, tc.code, func(inv *QualificationInventory) {
			f2 := newQfx(t)
			f2.qfxSymbolRole()
			*inv = f2.inv
			tc.mut(inv)
		})
	}
	res, _ := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: strings.Repeat("c", 40)})
	if r := roleRow(res, "fx-symbols", "windows-amd64"); r.Status != CellMissing {
		t.Fatalf("executed row without evidence: %+v", r)
	}
	for _, p := range []string{"linux-amd64", "darwin-arm64"} {
		if r := roleRow(res, "fx-symbols", p); r.Status != RoleDeferred || r.Mechanism != "" || r.Checks != "" || len(r.Sets) != 0 || r.Reason != "deferred" {
			t.Fatalf("deferred row %s: %+v", p, r)
		}
	}
	if !slices.ContainsFunc(res.Explanation, func(s string) bool { return strings.Contains(s, "fx-symbols의 linux-amd64 행은 DEFERRED") }) ||
		!slices.ContainsFunc(res.Explanation, func(s string) bool { return strings.Contains(s, "symbol 모드 행 fx-symbols") }) {
		t.Fatalf("explanation: %v", res.Explanation)
	}
}

// #218: a symbol-mode set replays under its host's byte budget: the replay starts from the
// host's count, stops at the host's limit and leaves its own reads in the count.
func TestReplaySharedBudget(t *testing.T) {
	root, reg := newFxNative().write(t, nil)
	ctx := ctxT(t)
	host := &run{caller: ctx, wall: context.Background(), cancel: func() {}}
	alone, err := Replay(ctx, ReplayRequest{Root: root, Profile: reg})
	if err != nil || alone.BytesRead == 0 {
		t.Fatal(err)
	}
	host.totalRead = 100
	r, err := replayShared(ctx, ReplayRequest{Root: root, Profile: reg}, &replayBudget{r: host, total: 1 << 30})
	if err != nil || r.Assessment != AssessPass || host.totalRead != 100+alone.BytesRead {
		t.Fatalf("shared: %v %s host=%d alone=%d", err, r.Assessment, host.totalRead, alone.BytesRead)
	}
	host.totalRead = 100
	if _, err := replayShared(ctx, ReplayRequest{Root: root, Profile: reg}, &replayBudget{r: host, total: 100 + alone.BytesRead/2}); asError(err).Code != "TOTAL_BYTES_LIMIT" {
		t.Fatalf("over the host budget: %v", err)
	}
}

// #218: a recorded symbol-mode set (src/testdata/qualify-symbols: run-routes.ps1 -Symbols on
// windows/amd64 for s05-csharp-symbols-a-c) under the qualification limits. It passes as
// registered; a replay member beyond the replay file limit leaves the row unverified
// (INCOMPLETE, mechanism BLOCKED), never a pass; the replay counts against the host byte
// budget and the set's cases against the host record limit.
func TestQualifySymbolSetLimits(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseQualificationInventory(data)
	if err != nil {
		t.Fatal(err)
	}
	var w QualWorkload
	for _, x := range inv.ExtraRoles {
		for _, y := range x.Workloads {
			if x.ID == "csharp-symbols" && y.Set == "s05-csharp-symbols-a-c" {
				w = y
			}
		}
	}
	win, lin := inv.Platforms[0], inv.Platforms[1]
	if win.ID != "windows-amd64" || lin.ID != "linux-amd64" || w.Preprocess == nil {
		t.Fatalf("inventory platforms %v or workload missing", inv.Platforms)
	}
	inv.Platforms, inv.Routes, inv.Cells = []QualPlatform{win, lin}, inv.Routes[:1], 2
	inv.ExtraRoles = []QualRole{{ID: "csharp-symbols", Role: "owned", Status: RoleExecuted, Reason: "recorded", Platforms: []string{win.ID}, NotApplicable: map[string]string{},
		Deferred: map[string]string{lin.ID: "deferred"}, Workloads: []QualWorkload{w}}}
	invData, _ := json.Marshal(inv)
	host := t.TempDir()
	set := filepath.Join(host, "incremental", w.Set)
	if err := os.CopyFS(set, os.DirFS(filepath.Join("..", "testdata", "qualify-symbols", w.Set))); err != nil {
		t.Fatal(err)
	}
	cand := strings.Repeat("c", 40)
	id, _ := json.Marshal(RunIdentity{Schema: RunIdentitySchema, Repository: "o/r", Workflow: "foundation", RunID: "1", RunAttempt: "1", Event: "push", SHA: cand, HeadSHA: cand,
		Checkout: cand, Job: "native routes", RunnerOS: "Windows", RunnerArch: "X64", Image: "test", GOOS: "windows", GOARCH: "amd64", GoVersion: "go", EvidenceMode: ModeNewRun})
	if err := os.WriteFile(filepath.Join(host, "run-identity.json"), id, 0o644); err != nil {
		t.Fatal(err)
	}
	qualify := func() (QualRoleRow, error) {
		r, err := Qualify(ctxT(t), QualifyRequest{Inventory: invData, Candidate: cand, Hosts: []QualifyHost{{Platform: win.ID, Root: host}}})
		return roleRow(r, "csharp-symbols", win.ID), err
	}
	if r, err := qualify(); err != nil || r.Status != CellPass || r.Mechanism != AssessPass {
		t.Fatalf("recorded set: %v %+v", err, r)
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 1024; return l }
	r, err := qualify()
	testHookReplayLimits = nil
	if err != nil || r.Status != CellIncomplete || r.Mechanism != AssessBlocked {
		t.Fatalf("replay member over its limit: %v %+v", err, r)
	}
	// the host reads every set file before the replay: a budget just above that ends inside it
	size := func(p string) uint64 {
		st, err := os.Stat(filepath.Join(set, p))
		if err != nil {
			t.Fatal(err)
		}
		return uint64(st.Size())
	}
	var pre uint64 = uint64(len(id)) + size("profile.json") + size("result.json")
	for _, p := range []string{"profile.json", "result.json", "responses/00000-issue198-nested-directives.json", "sources/00000-issue198-nested-directives.bin"} {
		pre += size(p)
	}
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = pre + 64; return l }
	_, err = qualify()
	testHookQualifyLimits = nil
	if asError(err).Code != "TOTAL_BYTES_LIMIT" {
		t.Fatalf("replay beyond the host budget: %v", err)
	}
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.Records = 0; return l }
	_, err = qualify()
	testHookQualifyLimits = nil
	if asError(err).Code != "RECORDS_LIMIT" {
		t.Fatalf("cases beyond the host record limit: %v", err)
	}
}
