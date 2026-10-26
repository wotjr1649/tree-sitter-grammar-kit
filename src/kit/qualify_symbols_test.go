package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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
	w.Cases[0].PreprocessCSTExpect = []PreprocessCSTStep{{Step: 0, SourceSHA256: w.Cases[0].Input.SHA256, SourceBytes: 5, Result: "MATCH", HostHashes: []uint32{}, CSTHashes: []uint32{}, HostOnly: []uint32{}, CSTOnly: []uint32{}}}
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

func TestQualificationHistoricalCSTMember(t *testing.T) {
	for _, schema := range []string{qualificationInventoryR5, qualificationInventoryR6} {
		f := newQfx(t)
		role := f.qfxSymbolRole()
		role.Workloads[0].Cases[0].PreprocessCSTExpect = nil
		f.inv.Schema = schema
		base := f.invBytes(t)
		if _, err := ParseQualificationInventory(base); err != nil {
			t.Fatal("historical baseline", err)
		}
		for _, value := range []string{"null", "[]"} {
			changed := bytes.Replace(base, []byte(`"id":"ok"`), []byte(`"preprocess_cst_expect":`+value+`,"id":"ok"`), 1)
			if bytes.Equal(base, changed) {
				t.Fatal("case anchor missing")
			}
			if _, err := ParseQualificationInventory(changed); err == nil {
				t.Fatalf("%s accepted new member %s", schema, value)
			}
		}
	}
}

func TestQualificationCSTRequiredMembers(t *testing.T) {
	f := newQfx(t)
	f.qfxSymbolRole()
	base := f.invBytes(t)
	for _, replacement := range []string{`"result":"MATCH"`, `"result":"MATCH","code":null`} {
		changed := bytes.Replace(base, []byte(`"result":"MATCH","code":""`), []byte(replacement), 1)
		if bytes.Equal(base, changed) {
			t.Fatal("CST step anchor missing")
		}
		if _, err := ParseQualificationInventory(changed); err == nil {
			t.Fatal("missing/null registered code accepted")
		}
	}
}

func TestSelectionOutcomeRegistration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		mut  func(*QualificationInventory, *QualCase)
		ok   bool
	}{
		{"current", func(*QualificationInventory, *QualCase) {}, true},
		{"historical", func(i *QualificationInventory, _ *QualCase) { i.Schema = qualificationInventoryR5 }, false},
		{"promoted-pass", func(_ *QualificationInventory, c *QualCase) { c.ExpectAssessment = AssessPass }, false},
		{"step-zero", func(_ *QualificationInventory, c *QualCase) { c.ExpectCode = SelectionRoutePrefix + "0" }, false},
		{"noncanonical-step", func(_ *QualificationInventory, c *QualCase) { c.ExpectCode = SelectionRoutePrefix + "01" }, false},
		{"missing-syntax", func(_ *QualificationInventory, c *QualCase) { c.Expect = nil }, false},
		{"same-selection", func(_ *QualificationInventory, c *QualCase) {
			c.PreprocessExpect[1].Active = []ByteRange{{12, 23}}
			c.PreprocessExpect[1].Inactive = []ByteRange{{29, 40}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := ParseQualificationInventory(data)
			if err != nil {
				t.Fatal(err)
			}
			var c *QualCase
			for i := range inv.ExtraRoles {
				for j := range inv.ExtraRoles[i].Workloads {
					w := &inv.ExtraRoles[i].Workloads[j]
					if w.Set == "s05-csharp-symbols-switch" {
						c = &w.Cases[0]
					}
				}
			}
			if c == nil {
				t.Fatal("switch registration missing")
			}
			tc.mut(&inv, c)
			encoded, err := json.Marshal(inv)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ParseQualificationInventory(encoded)
			if (err == nil) != tc.ok {
				t.Fatalf("accepted=%t, want %t: %v", err == nil, tc.ok, err)
			}
		})
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
// windows/amd64 for s05-csharp-symbols-a-c) under the qualification limits. The record binds
// the current registration, runtime commit and native policy. Historical fixtures keep their
// pinned original registration and are rejected under the current grammar. When the C# grammar, the
// pinned runtime, the operation limits or the policy text change, the baseline fails and
// the set is recorded again with run-routes.ps1 -Routes none -Symbols (copy
// incremental/s05-csharp-symbols-a-c). It passes as
// registered; a replay member beyond the replay file limit leaves the row unverified
// (INCOMPLETE, mechanism BLOCKED), never a pass; the replay counts against the host byte
// budget and the set's cases against the host record limit.
func TestQualifySymbolSetLimits(t *testing.T) {
	for _, schema := range []string{QualificationInventorySchema, qualificationInventoryR6, qualificationInventoryR5} {
		t.Run(schema, func(t *testing.T) { qualifySymbolSetLimits(t, schema) })
	}
}

func qualifySymbolSetLimits(t *testing.T, schema string) {
	data, err := os.ReadFile(filepath.Join("..", "contracts", "qualification-c1.json"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseQualificationInventory(data)
	if err != nil {
		t.Fatal(err)
	}
	fixture := "qualify-symbols"
	inv.Schema = schema
	if schema == qualificationInventoryR5 {
		fixture = "qualify-symbols-r4"
	} else if schema == qualificationInventoryR6 {
		fixture = "qualify-symbols-r5"
	}
	var w QualWorkload
	for _, x := range inv.ExtraRoles {
		for _, y := range x.Workloads {
			if x.ID == "csharp-symbols" && y.Set == "s05-csharp-symbols-a-c" {
				w = y
			}
		}
	}
	currentGrammar := w.Grammar
	if schema != QualificationInventorySchema {
		// Only the immutable repository fixture defines the historical registration.
		profile, err := os.ReadFile(filepath.Join("..", "testdata", fixture, w.Set, "profile.json"))
		if err != nil {
			t.Fatal(err)
		}
		if sum(profile) != "5e4c24787031a29ce4ecdc218f9e16ad4f31a9a0ceea2631f9b2e649a7622b4e" {
			t.Fatal("historical symbol profile changed")
		}
		old, err := ParseIncrementalProfile(profile)
		if err != nil {
			t.Fatal(err)
		}
		w.Grammar = old.Grammar
		for i := range w.Cases {
			w.Cases[i].PreprocessCSTExpect = nil
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
	if err := os.CopyFS(set, os.DirFS(filepath.Join("..", "testdata", fixture, w.Set))); err != nil {
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
		wantSchema := QualificationResultSchema
		if schema == qualificationInventoryR5 {
			wantSchema = qualificationResultR6
		} else if schema == qualificationInventoryR6 {
			wantSchema = qualificationResultR7
		}
		if err == nil && r.ResultSchema != wantSchema {
			t.Fatalf("result revision %s, want %s", r.ResultSchema, wantSchema)
		}
		return roleRow(r, "csharp-symbols", win.ID), err
	}
	registered := invData
	wrongGrammar := append([]NativeInput{}, currentGrammar...)
	if schema == QualificationInventorySchema {
		wrongGrammar[0].SHA256 = strings.Repeat("0", 64)
	}
	inv.ExtraRoles[0].Workloads[0].Grammar = wrongGrammar
	invData, err = json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := qualify(); err != nil || r.Status != CellFail || r.Mechanism != AssessFail || len(r.Sets) != 1 || len(r.Sets[0].Findings) != 1 || r.Sets[0].Findings[0].Code != "REGISTRATION_MISMATCH" || !strings.HasSuffix(r.Sets[0].Findings[0].Message, ": grammar") {
		t.Fatalf("fixture under wrong grammar: %v %+v", err, r)
	}
	inv.ExtraRoles[0].Workloads[0].Grammar = w.Grammar
	invData = registered
	if r, err := qualify(); err != nil || r.Status != CellPass || r.Mechanism != AssessPass {
		t.Fatalf("recorded set: %v %+v", err, r)
	}
	// a recorded code on a case whose steps are all OK is not fixed by its selection: with the
	// summary kept consistent the replay accepts it, and the registered outcome refuses it
	resPath := filepath.Join(set, "result.json")
	recorded, err := os.ReadFile(resPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(recorded, []byte(`"assessment":"PASS","code":"",`), []byte(`"assessment":"PASS","code":"CONTROL",`), 1)
	changed = bytes.Replace(changed, []byte(`"codes":{}`), []byte(`"codes":{"CONTROL":1}`), 1)
	if bytes.Equal(changed, recorded) || os.WriteFile(resPath, changed, 0o644) != nil {
		t.Fatal("recorded code not changed")
	}
	if r, err := qualify(); err != nil || r.Status != CellFail || r.Mechanism != AssessPass || r.Checks != claimFail {
		t.Fatalf("recorded code: %v %+v", err, r)
	}
	if err := os.WriteFile(resPath, recorded, 0o644); err != nil {
		t.Fatal(err)
	}
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 1024; return l }
	r, err := qualify()
	testHookReplayLimits = nil
	if err != nil || r.Status != CellIncomplete || r.Mechanism != AssessBlocked {
		t.Fatalf("replay member over its limit: %v %+v", err, r)
	}
	// a set file beyond the host file limit is not read either: unverified, not failed
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 4096; return l }
	r, err = qualify()
	testHookQualifyLimits = nil
	if err != nil || r.Status != CellIncomplete || r.Mechanism != AssessBlocked {
		t.Fatalf("host file over its limit: %v %+v", err, r)
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
	if schema == QualificationInventorySchema {
		pre += size("primary/responses/00000-issue198-nested-directives.json")
	}
	replayStarted := false
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { replayStarted = true; return l }
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.TotalBytes = pre + 64; return l }
	_, err = qualify()
	testHookQualifyLimits = nil
	testHookReplayLimits = nil
	if asError(err).Code != "TOTAL_BYTES_LIMIT" || !replayStarted {
		t.Fatalf("replay beyond the host budget: %v", err)
	}
	// the qualification wall's deadline passing while the set replays is the qualification's
	// WALL_LIMIT (RESOURCE_LIMIT), not a caller cancellation: the replay starts after it
	var deadline time.Time
	testHookWall = func() (context.Context, context.CancelFunc) {
		if deadline.IsZero() {
			deadline = time.Now().Add(2 * time.Second)
			return context.WithDeadline(context.Background(), deadline)
		}
		time.Sleep(time.Until(deadline) + 50*time.Millisecond)
		return context.WithCancel(context.Background())
	}
	_, err = qualify()
	testHookWall = nil
	if e := asError(err); e.Kind != KindResourceLimit || e.Code != "WALL_LIMIT" {
		t.Fatalf("wall during the replay: %v", err)
	}
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.Records = 0; return l }
	_, err = qualify()
	testHookQualifyLimits = nil
	if asError(err).Code != "RECORDS_LIMIT" {
		t.Fatalf("cases beyond the host record limit: %v", err)
	}
}
