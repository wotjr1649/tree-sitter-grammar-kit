package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const qfxCandidate = "0123456789abcdef0123456789abcdef01234567"

var qfxPlatforms = []QualPlatform{{"windows-amd64", "windows", "amd64"}, {"linux-amd64", "linux", "amd64"}, {"darwin-arm64", "darwin", "arm64"}}

const qfxQuery = "(word) @w\n"

// qfx is a synthetic qualification: routes fxa (P, E and Q all covered) and fxb (also
// requires N, which no case covers), three platforms and one historical detector role.
type qfx struct {
	inv  QualificationInventory
	root string
}

func qfxCases(route string) []QualCase {
	alpha, beta := "alpha", "beta"
	row := route + "-B01"
	return []QualCase{
		{ID: "c1", Role: "requirement", Input: NativeInput{SHA256: sum([]byte(alpha)), Bytes: 5}, Edits: []Edit{},
			Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"word"}}}, QueryExpect: []QueryExpectation{}, Covers: map[string][]string{row: {"P"}}},
		{ID: "c2", Role: "requirement", Input: NativeInput{SHA256: sum([]byte(beta)), Bytes: 4}, Edits: []Edit{{StartByte: 3, OldEndByte: 4, NewEndByte: 4, Old: []byte("a"), New: []byte("z")}},
			Expect: []StepExpectation{}, QueryExpect: []QueryExpectation{}, Covers: map[string][]string{row: {"E"}}},
		{ID: "q1", Role: "requirement", Input: NativeInput{SHA256: sum([]byte(alpha)), Bytes: 5}, Edits: []Edit{}, Expect: []StepExpectation{},
			QueryExpect: []QueryExpectation{{Query: "q.x", Step: 0, Status: "COMPLETED", Captures: &[]CaptureExpectation{{Name: "w", Type: "word", Text: "alpha"}}}},
			Covers:      map[string][]string{row: {"Q"}}, Source: &alpha},
	}
}

func qfxWorkload(set, route string, cases []QualCase) QualWorkload {
	return QualWorkload{Set: set, Profile: set, Route: route, Operation: "native-query", Output: "tree", Symbol: "tree_sitter_fx",
		Grammar: []NativeInput{{Path: "src/parser.c", Role: "parser", SHA256: fxCompiler, Bytes: 1}}, Queries: []QualQuery{{ID: "q.x", SHA256: sum([]byte(qfxQuery))}}, API: true, Cases: cases}
}

func newQfx(t *testing.T) *qfx {
	f := &qfx{root: t.TempDir()}
	f.inv = QualificationInventory{Schema: QualificationInventorySchema, ID: "fx-qualification", Campaign: "FX", Cells: 6, CoverageRule: CoverageRule,
		Kinds: slices.Clone(caseKinds), Platforms: qfxPlatforms}
	for _, r := range []string{"fxa", "fxb"} {
		kinds := []string{"P", "E", "Q"}
		if r == "fxb" {
			kinds = []string{"P", "N", "E", "Q"}
		}
		f.inv.Routes = append(f.inv.Routes, QualRoute{Route: r, Workload: qfxWorkload("s06-"+r, r, qfxCases(r)), Requirements: []QualRequirement{{Row: r + "-B01", Kinds: kinds}}})
	}
	det := qfxCases("fxh")[:1]
	det[0].ID, det[0].Role, det[0].Covers = "d1", "detector", map[string][]string{}
	f.inv.ExtraRoles = []QualRole{
		{ID: "fx-history", Role: "historical", Status: RoleExecuted, Reason: "registered historical defect detector", Platforms: []string{"windows-amd64"},
			NotApplicable: map[string]string{"linux-amd64": "windows only", "darwin-arm64": "windows only"}, Workloads: []QualWorkload{qfxWorkload("s06-fxh", "fxh", det)}},
		{ID: "fx-maintained", Role: "maintained", Status: RoleNotRun, Reason: "not prepared", Platforms: []string{}, NotApplicable: map[string]string{}},
	}
	return f
}

func (f *qfx) invBytes(t *testing.T) []byte {
	b, err := json.Marshal(f.inv)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// qfxMut changes one host's evidence before it is written: the records and manifest of a
// set, the run identity, or nothing.
type qfxMut struct {
	record   func(set string, recs []map[string]any)
	manifest func(set string, man map[string]any)
	profile  func(set string, prof map[string]any)
	identity func(id map[string]any)
	skip     func(set string) bool
}

func qfxRecord(id, src string, edits bool, query bool) map[string]any {
	steps := []any{qfxStep(0, src, false, query)}
	claims := map[string]string{"incremental_equality": "NOT_CLAIMED", "incremental_route": "NOT_CLAIMED", "expectations": "PASS"}
	expectations := []any{map[string]any{"step": 0, "syntax": "NO_ERROR", "contains": []string{"word"}, "declarations": "", "result": "PASS"}}
	if id != "c1" && id != "d1" {
		claims["expectations"], expectations = "NOT_CLAIMED", []any{}
	}
	if edits {
		steps = append(steps, qfxStep(1, "betz", true, false))
		claims["incremental_equality"], claims["incremental_route"] = "PASS", "PASS"
	}
	qe, qeq := "NOT_CLAIMED", "NOT_CLAIMED"
	if query {
		qe = "PASS"
	}
	if edits {
		qeq = "PASS"
	}
	return map[string]any{"id": id, "input": map[string]any{"path": "cases/" + id + ".txt", "role": "case", "sha256": sum([]byte(src)), "bytes": len(src)}, "encoding": "UTF-8",
		"execution_status": "COMPLETED", "assessment": "PASS", "code": "", "claims": claims, "response_status": "COMPLETED", "response_code": "",
		"producer": map[string]any{"query": "tsgk-query/r1"}, "process": map[string]any{"wall_ms": 10, "memory": map[string]any{"peak_bytes": 1000}},
		"steps": steps, "expectations": expectations,
		"oracle_claims": map[string]string{"query_equality": qeq, "query_expectations": qe, "api": "PASS", "fact_reproduction": "NOT_CLAIMED", "dynamic_sql": "NOT_CLAIMED"}}
}

func qfxStep(i int, s string, fresh, query bool) map[string]any {
	inc := fxTreeOut(s, false)
	inc["api"] = map[string]any{"revision": "tsgk-api/r1", "consistent": true, "first_difference": nil}
	if query {
		n := uint32(len(s))
		inc["queries"] = []any{map[string]any{"id": "q.x", "status": "COMPLETED", "code": "", "evaluation": "NOT_EVALUATED",
			"captures": []Capture{{Name: "w", Node: 1, Type: "word", Named: true, EndByte: n, EndPoint: Point{Column: n}}}}}
	}
	m := map[string]any{"step": i, "source_bytes": len(s), "source_sha256": sum([]byte(s)), "edit": nil, "route": nil, "comparison": nil, "incremental": inc, "fresh": nil}
	if fresh {
		m["fresh"] = fxTreeOut(s, false)
		m["route"] = map[string]any{"edit_has_changes": true, "edited_root_end_byte": len(s), "reused_nodes": 1, "fresh_reused_nodes": 0, "proven": true}
		m["comparison"] = map[string]any{"equal": true, "first_difference": nil}
		m["query_comparison"] = map[string]any{"equal": true}
	}
	return m
}

// host writes one platform's evidence directory for every workload of the inventory that
// names the platform and returns it.
func (f *qfx) host(t *testing.T, p QualPlatform, m qfxMut) string {
	t.Helper()
	dir := filepath.Join(f.root, p.ID)
	write := func(rel string, b []byte) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := map[string]any{"schema": RunIdentitySchema, "repository": "o/r", "workflow": "Foundation", "run_id": "500", "run_attempt": "1", "event": "pull_request",
		"sha": qfxCandidate, "head_sha": "89abcdef0123456789abcdef0123456789abcdef", "checkout": qfxCandidate, "job": "native routes", "runner_os": "X", "runner_arch": "X64",
		"image": "img/1", "goos": p.GOOS, "goarch": p.GOARCH, "go_version": "go1.27.1", "evidence_mode": "NEW_RUN"}
	if m.identity != nil {
		m.identity(id)
	}
	b, _ := json.Marshal(id)
	write("run-identity.json", b)
	write("summary.json", []byte(`{"retained":true}`))
	ws := []QualWorkload{}
	for _, r := range f.inv.Routes {
		ws = append(ws, r.Workload)
	}
	for _, x := range f.inv.ExtraRoles {
		if slices.Contains(x.Platforms, p.ID) {
			ws = append(ws, x.Workloads...)
		}
	}
	for _, w := range ws {
		if m.skip != nil && m.skip(w.Set) {
			continue
		}
		var pcases []any
		for _, c := range w.Cases {
			edits := []any{}
			for _, e := range c.Edits {
				edits = append(edits, e)
			}
			expect := []any{}
			for _, e := range c.Expect {
				expect = append(expect, e)
			}
			qe := []any{}
			for _, q := range c.QueryExpect {
				qe = append(qe, q)
			}
			pcases = append(pcases, map[string]any{"id": c.ID, "input": map[string]any{"path": "cases/" + c.ID + ".txt", "role": "case", "sha256": c.Input.SHA256, "bytes": c.Input.Bytes},
				"edits": edits, "points": []any{}, "expect": expect, "query_expect": qe, "dynamic_sql_expect": nil})
		}
		prof := map[string]any{"schema": OracleSchema, "id": w.Profile, "route": w.Route, "operation": w.Operation, "symbol": w.Symbol, "encoding": "UTF-8", "output": w.Output,
			"compiler": map[string]any{"name": "cc", "version": "host", "sha256": fxCompiler, "bytes": 1}, "grammar": w.Grammar, "declarations": nil, "cases": pcases,
			"queries": []any{map[string]any{"id": "q.x", "source": qfxQuery}}, "fact_pack": nil, "api": w.API}
		if m.profile != nil {
			m.profile(w.Set, prof)
		}
		pdata, _ := json.Marshal(prof)
		write("profiles/"+w.Profile+".json", pdata)
		var recs []map[string]any
		for _, c := range w.Cases {
			src := map[string]string{"c1": "alpha", "c2": "beta", "q1": "alpha", "d1": "alpha"}[c.ID]
			recs = append(recs, qfxRecord(c.ID, src, len(c.Edits) > 0, len(c.QueryExpect) > 0))
		}
		if m.record != nil {
			m.record(w.Set, recs)
		}
		var members []OracleMember
		var ids []string
		for i, c := range recs {
			body, _ := json.Marshal(c)
			name := fmt.Sprintf("%05d-%s.json", i, c["id"])
			rec := append(append([]byte(`{"schema":"tsgk-oracle-record/r1","case":"`+c["id"].(string)+`","workload":"`+w.Profile+`",`), body[1:len(body)-1]...), []byte(`,"complete":true}`)...)
			raw := []byte(`{"response":` + fmt.Sprint(i) + `}`)
			in := c["input"].(map[string]any)
			for _, mm := range []struct {
				role string
				b    []byte
			}{{"record", rec}, {"raw", raw}} {
				pth := map[string]string{"record": "records/", "raw": "raw/"}[mm.role] + name
				write("records/"+w.Set+"/"+pth, mm.b)
				members = append(members, OracleMember{Path: pth, Role: mm.role, Case: c["id"].(string), Bytes: uint64(len(mm.b)), SHA256: sum(mm.b),
					InputBytes: uint64(in["bytes"].(int)), InputSHA256: in["sha256"].(string), ExecutionStatus: c["execution_status"].(string)})
			}
			ids = append(ids, c["id"].(string))
		}
		man := map[string]any{"schema": OracleManifestSchema,
			"workload": map[string]string{"operation": w.Operation, "output": w.Output, "profile_id": w.Profile, "profile_sha256": sum(pdata), "route": w.Route},
			"producer": map[string]string{"build_identity": fxProducer, "compiler_sha256": fxCompiler, "compiler_version": "cc 1", "executable_sha256": fxExe, "platform": p.pair(), "runtime_commit": fxRuntime},
			"policy":   IdentityRef{"policy", "tsgk-native-policy/r1", fxPolicy}, "protocol": "tsgk-native/r2",
			"comparators": []string{"tsgk-tree-digest/r1", "tsgk-capture-compare/r1", "tsgk-api/r1", "tsgk-predicates/r1"},
			"queries":     []IdentityRef{{"query:q.x", "tsgk-query-source/r1", sum([]byte(qfxQuery))}}, "fact_pack": nil, "execution_status": "COMPLETED", "assessment": "PASS",
			"records": len(recs), "cases": ids, "members": members}
		if m.manifest != nil {
			m.manifest(w.Set, man)
		}
		keys := []string{"schema", "workload", "producer", "policy", "protocol", "comparators", "queries", "fact_pack", "execution_status", "assessment", "records", "cases", "members"}
		var kv []any
		for _, k := range keys {
			kv = append(kv, k, man[k])
		}
		kv = append(kv, "complete", true)
		write("records/"+w.Set+"/manifest.json", ordered(kv...))
	}
	return dir
}

func (f *qfx) run(t *testing.T, hosts map[string]string) QualificationResult {
	t.Helper()
	var hs []QualifyHost
	for _, p := range qfxPlatforms {
		if d, ok := hosts[p.ID]; ok {
			hs = append(hs, QualifyHost{Platform: p.ID, Root: d})
		}
	}
	res, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs})
	if err != nil {
		t.Fatalf("qualify: %v", err)
	}
	return res
}

func (f *qfx) all(t *testing.T, muts map[string]qfxMut) map[string]string {
	out := map[string]string{}
	for _, p := range qfxPlatforms {
		out[p.ID] = f.host(t, p, muts[p.ID])
	}
	return out
}

func cellOf(r QualificationResult, route, plat string) QualCell {
	for _, c := range r.Cells {
		if c.Route == route && c.Platform == plat {
			return c
		}
	}
	return QualCell{}
}

func setCodes(c QualCell) []string { return codes(c.Set.Findings) }

// S08-A01/A02/A03: the exact route × platform cells from the inventory; a fully covered
// route passes on every host, a route with an uncovered required kind is INCOMPLETE and
// never PASS, and the support claim stays BLOCKED until every cell passes.
func TestQualifyBaseline(t *testing.T) {
	f := newQfx(t)
	r := f.run(t, f.all(t, nil))
	if len(r.Cells) != 6 || r.Completeness != AssessPass || r.Assessment != AssessBlocked || r.SupportClaim != "BLOCKED" || r.MechanismGate != AssessPass {
		t.Fatalf("baseline: %d cells %s %s %s %v", len(r.Cells), r.Completeness, r.Assessment, r.SupportClaim, r.Findings)
	}
	for _, p := range qfxPlatforms {
		a, b := cellOf(r, "fxa", p.ID), cellOf(r, "fxb", p.ID)
		if a.Status != CellPass || a.Mechanism != AssessPass || a.Requirement != AssessPass || a.Comparison != AssessPass || a.Counts.Pass != 3 {
			t.Fatalf("fxa %s: %+v %v", p.ID, a.Counts, setCodes(a))
		}
		if b.Status != CellIncomplete || b.Requirement != CellIncomplete || b.Counts.NotCovered != 1 {
			t.Fatalf("fxb %s: %s %+v", p.ID, b.Status, b.Counts)
		}
		if a.Set.Host["compiler_sha256"] != fxCompiler || a.Set.Host["process_wall_ms_max"] != "10" {
			t.Fatalf("host observations not retained: %v", a.Set.Host)
		}
	}
	if r.Run == nil || r.Run.RunID != "500" || len(r.Hosts) != 3 {
		t.Fatalf("run cohort %+v %v", r.Run, r.Hosts)
	}
	// extra roles stay separate rows: executed on windows, not applicable elsewhere, not run
	rows := map[string]string{}
	for _, x := range r.ExtraRoles {
		rows[x.ID+"@"+x.Platform] = x.Status
	}
	if rows["fx-history@windows-amd64"] != CellPass || rows["fx-history@linux-amd64"] != RoleNotApplicable || rows["fx-maintained@darwin-arm64"] != RoleNotRun {
		t.Fatalf("extra roles %v", rows)
	}
	// every required kind covered: all cells pass and only then the support claim
	f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
	f.root = t.TempDir()
	r = f.run(t, f.all(t, nil))
	if r.Assessment != AssessPass || r.SupportClaim != "SUPPORTED" || r.Totals.Status[CellPass] != 6 {
		t.Fatalf("all covered: %s %s %v", r.Assessment, r.SupportClaim, r.Totals.Status)
	}
	// every cell passes, but the executed role row has no set on its platform
	f.root = t.TempDir()
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {skip: func(s string) bool { return s == "s06-fxh" }}}))
	if r.SupportClaim == "SUPPORTED" || r.Assessment == AssessPass || r.Completeness != AssessFail || r.MechanismGate != AssessFail {
		t.Fatalf("missing role row: %s %s %s %s", r.SupportClaim, r.Assessment, r.Completeness, r.MechanismGate)
	}
	for _, x := range r.ExtraRoles {
		if x.ID == "fx-history" && x.Platform == "windows-amd64" && (x.Status != CellMissing || x.Checks != "") {
			t.Fatalf("missing row reports checks: %s %q", x.Status, x.Checks)
		}
	}
}

// Review r1 M1: a registered expectation that covers no row kind (here the NO_ERROR step
// after an edit) still decides the requirement axis; a failing detector fails its role
// row without touching the kit gate.
func TestQualifyRegisteredChecks(t *testing.T) {
	f := newQfx(t)
	f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
	for _, r := range f.inv.Routes {
		r.Workload.Cases[1].Expect = []StepExpectation{{Step: 1, Syntax: "NO_ERROR", Contains: []string{}}}
	}
	bad := func(set string, recs []map[string]any) {
		switch set {
		case "s06-fxa":
			nodes := fxTree("betz", true)
			st := recs[1]["steps"].([]any)[1].(map[string]any)
			for _, k := range []string{"incremental", "fresh"} {
				tr := st[k].(map[string]any)
				tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["has_error"] = nodes, TreeDigest(nodes), true
			}
			recs[1]["expectations"] = []any{map[string]any{"step": 1, "syntax": "NO_ERROR", "contains": []string{}, "declarations": "", "result": "FAIL"}}
			recs[1]["claims"].(map[string]string)["expectations"], recs[1]["assessment"] = "FAIL", "FAIL"
		case "s06-fxb":
			recs[1]["expectations"] = []any{map[string]any{"step": 1, "syntax": "NO_ERROR", "contains": []string{}, "declarations": "", "result": "PASS"}}
			recs[1]["claims"].(map[string]string)["expectations"] = "PASS"
		case "s06-fxh":
			nodes := fxTree("alpha", true)
			tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["has_error"] = nodes, TreeDigest(nodes), true
			recs[0]["expectations"].([]any)[0].(map[string]any)["result"] = "FAIL"
			recs[0]["claims"].(map[string]string)["expectations"], recs[0]["assessment"] = "FAIL", "FAIL"
		}
	}
	muts := map[string]qfxMut{}
	for _, p := range qfxPlatforms {
		muts[p.ID] = qfxMut{record: bad}
	}
	r := f.run(t, f.all(t, muts))
	a, b := cellOf(r, "fxa", "windows-amd64"), cellOf(r, "fxb", "windows-amd64")
	if a.Counts.Fail != 0 || a.Checks != claimFail || a.Requirement != AssessFail || a.Status != CellFail || !slices.Contains(a.CheckFailures, "c2=FAIL") {
		t.Fatalf("uncovered expectation: %+v %s %s %v", a.Counts, a.Checks, a.Requirement, a.CheckFailures)
	}
	if b.Status != CellPass || b.Checks != claimPass {
		t.Fatalf("passing route: %s %s %v", b.Status, b.Checks, b.CheckFailures)
	}
	for _, x := range r.ExtraRoles {
		if x.ID == "fx-history" && x.Platform == "windows-amd64" && (x.Status != CellFail || x.Mechanism != AssessPass || x.Checks != claimFail) {
			t.Fatalf("failing detector row: %s %s %s", x.Status, x.Mechanism, x.Checks)
		}
	}
	if r.MechanismGate != AssessPass || r.Assessment != AssessFail || r.SupportClaim != "BLOCKED" {
		t.Fatalf("gate %s assessment %s support %s", r.MechanismGate, r.Assessment, r.SupportClaim)
	}
}

// S08-A04: a missing host or set, or a duplicate row, fails completeness.
func TestQualifyCompleteness(t *testing.T) {
	f := newQfx(t)
	hosts := f.all(t, nil)
	one := map[string]string{"windows-amd64": hosts["windows-amd64"], "linux-amd64": hosts["linux-amd64"]}
	r := f.run(t, one)
	if r.Completeness != AssessFail || r.Assessment != AssessFail || cellOf(r, "fxa", "darwin-arm64").Status != CellMissing || len(r.Cells) != 6 || r.MechanismGate != AssessFail {
		t.Fatalf("missing host: %s %s %s", r.Completeness, r.Assessment, cellOf(r, "fxa", "darwin-arm64").Status)
	}
	if c := cellOf(r, "fxa", "windows-amd64"); c.Comparison != AssessPass {
		t.Fatalf("two hosts still compare: %s", c.Comparison)
	}
	// one route's set deleted on one host
	f2 := newQfx(t)
	h := f2.all(t, map[string]qfxMut{"linux-amd64": {skip: func(s string) bool { return s == "s06-fxb" }}})
	r = f2.run(t, h)
	if r.Completeness != AssessFail || cellOf(r, "fxb", "linux-amd64").Status != CellMissing || !slices.Contains(codes(r.Findings), "COMPLETENESS_FAILED") {
		t.Fatalf("missing set: %s %s %v", r.Completeness, cellOf(r, "fxb", "linux-amd64").Status, codes(r.Findings))
	}
	// a duplicate row: the same platform twice
	res, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: []QualifyHost{
		{"windows-amd64", hosts["windows-amd64"]}, {"linux-amd64", hosts["linux-amd64"]}, {"darwin-arm64", hosts["darwin-arm64"]}, {"linux-amd64", hosts["linux-amd64"]}}})
	if err != nil || res.Completeness != AssessFail || !slices.Contains(codes(res.Findings), "CELL_DUPLICATE") {
		t.Fatalf("duplicate: %v %s %v", err, res.Completeness, codes(res.Findings))
	}
	// an unregistered set directory is evidence outside the inventory
	os.MkdirAll(filepath.Join(hosts["darwin-arm64"], "records", "s06-extra"), 0o755)
	os.WriteFile(filepath.Join(hosts["darwin-arm64"], "records", "s06-extra", "manifest.json"), []byte("{}"), 0o644)
	if r = f.run(t, hosts); r.Completeness != AssessFail || !slices.Contains(codes(r.Findings), "HOST_FILE_UNREGISTERED") {
		t.Fatalf("unregistered set: %v", codes(r.Findings))
	}
}

// S08-A05: mixed run cohorts, wrong architecture and a workload other than the
// registration are rejected; the other hosts' cells are not repaired from them.
func TestQualifyCohort(t *testing.T) {
	cases := []struct {
		name string
		mut  qfxMut
		code string
	}{
		{"other-run", qfxMut{identity: func(id map[string]any) { id["run_id"] = "501" }}, "COHORT_MISMATCH"},
		{"other-attempt", qfxMut{identity: func(id map[string]any) { id["run_attempt"] = "2" }}, "COHORT_MISMATCH"},
		{"wrong-arch-identity", qfxMut{identity: func(id map[string]any) { id["goarch"] = "arm64" }}, "PLATFORM_MISMATCH"},
		{"wrong-arch-set", qfxMut{manifest: func(s string, m map[string]any) { m["producer"].(map[string]string)["platform"] = "linux/arm64" }}, "PLATFORM_MISMATCH"},
		{"other-grammar", qfxMut{profile: func(s string, p map[string]any) {
			p["grammar"] = []any{map[string]any{"path": "src/parser.c", "role": "parser", "sha256": fxExe, "bytes": 1}}
		}}, "REGISTRATION_MISMATCH"},
		{"other-query", qfxMut{profile: func(s string, p map[string]any) {
			p["queries"] = []any{map[string]any{"id": "q.x", "source": "(doc) @w\n"}}
		}}, "REGISTRATION_MISMATCH"},
		{"unregistered-input", qfxMut{profile: func(s string, p map[string]any) {
			p["cases"].([]any)[0].(map[string]any)["input"].(map[string]any)["sha256"] = sum([]byte("other"))
		}}, "REGISTRATION_MISMATCH"},
		{"other-workload-profile", qfxMut{manifest: func(s string, m map[string]any) { m["workload"].(map[string]string)["profile_sha256"] = fxExe }}, "WORKLOAD_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			r := f.run(t, f.all(t, map[string]qfxMut{"linux-amd64": tc.mut}))
			c := cellOf(r, "fxa", "linux-amd64")
			if r.Assessment != AssessFail || c.Status != CellFail || !slices.Contains(setCodes(c), tc.code) || r.MechanismGate != AssessFail {
				t.Fatalf("%s: %s %s %v", tc.name, r.Assessment, c.Status, setCodes(c))
			}
			if w := cellOf(r, "fxa", "windows-amd64"); w.Status != CellPass && w.Status != CellIncomplete {
				t.Fatalf("windows cell changed: %s", w.Status)
			}
		})
	}
	// identities other hosts disagree on: the comparison fails rather than picking one
	f := newQfx(t)
	r := f.run(t, f.all(t, map[string]qfxMut{"darwin-arm64": {manifest: func(s string, m map[string]any) {
		m["producer"].(map[string]string)["runtime_commit"] = strings.Repeat("a", 40)
	}}}))
	if c := cellOf(r, "fxa", "windows-amd64"); c.Comparison != AssessFail || c.Status != CellFail {
		t.Fatalf("runtime mix: %s %s", c.Comparison, c.Status)
	}
}

// S08-A06/A07: a planted tree, capture, range or ERROR difference on one host is detected
// and fails the route on every platform; host timing, memory and build identities differ
// freely and are retained.
func TestQualifyComparison(t *testing.T) {
	plant := map[string]func(recs []map[string]any){
		"tree-type": func(recs []map[string]any) {
			nodes := fxTree("alpha", false)
			nodes[1].Type = "name"
			tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["tree"].(map[string]any)["nodes"], tr["digest"] = nodes, TreeDigest(nodes)
		},
		"range": func(recs []map[string]any) {
			nodes := fxTree("alpha", false)
			nodes[1].EndByte, nodes[1].EndPoint = 4, Point{Column: 4}
			tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["tree"].(map[string]any)["nodes"], tr["digest"] = nodes, TreeDigest(nodes)
		},
		"capture": func(recs []map[string]any) {
			tr := recs[2]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			caps := tr["queries"].([]any)[0].(map[string]any)["captures"].([]Capture)
			caps[0].Pattern = 1
		},
		"api": func(recs []map[string]any) {
			tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["api"] = map[string]any{"revision": "tsgk-api/r1", "consistent": true, "first_difference": nil, "position_navigation_divergences": 1}
		},
		"error": func(recs []map[string]any) {
			nodes := fxTree("alpha", true)
			tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["has_error"] = nodes, TreeDigest(nodes), true
			recs[0]["expectations"].([]any)[0].(map[string]any)["result"] = "FAIL"
			recs[0]["claims"].(map[string]string)["expectations"], recs[0]["assessment"] = "FAIL", "FAIL"
		},
	}
	for name, p := range plant {
		t.Run(name, func(t *testing.T) {
			f := newQfx(t)
			r := f.run(t, f.all(t, map[string]qfxMut{"darwin-arm64": {record: func(s string, recs []map[string]any) {
				if s == "s06-fxa" {
					p(recs)
				}
			}}}))
			for _, pl := range qfxPlatforms {
				if c := cellOf(r, "fxa", pl.ID); c.Comparison != AssessFail || c.Status != CellFail {
					t.Fatalf("%s %s: %s %s", name, pl.ID, c.Comparison, c.Status)
				}
			}
			if c := cellOf(r, "fxb", "darwin-arm64"); c.Comparison != AssessPass {
				t.Fatalf("other route affected: %s", c.Comparison)
			}
			cmp := r.Comparisons[0]
			if cmp.Result != AssessFail || len(cmp.Differences) == 0 || r.MechanismGate != AssessFail {
				t.Fatalf("difference not reported: %+v", cmp)
			}
		})
	}
	// only host observations differ: timing, memory, parse time, compiler and executable
	f := newQfx(t)
	host := qfxMut{record: func(s string, recs []map[string]any) {
		for _, rc := range recs {
			rc["process"] = map[string]any{"wall_ms": 999, "memory": map[string]any{"peak_bytes": 77}}
			rc["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["parse_ms"] = 42
		}
	}, manifest: func(s string, m map[string]any) {
		m["producer"].(map[string]string)["compiler_sha256"] = fxPolicy
		m["producer"].(map[string]string)["executable_sha256"] = fxPolicy
		m["producer"].(map[string]string)["compiler_version"] = "clang 17"
	}}
	r := f.run(t, f.all(t, map[string]qfxMut{"linux-amd64": host}))
	a, l := cellOf(r, "fxa", "windows-amd64"), cellOf(r, "fxa", "linux-amd64")
	if a.Comparison != AssessPass || l.Status != CellPass {
		t.Fatalf("host-only differences changed the semantic result: %s %s %v", a.Comparison, l.Status, r.Comparisons[0].Differences)
	}
	if l.Set.Host["process_wall_ms_max"] != "999" || l.Set.Host["compiler_version"] != "clang 17" || a.Set.Host["compiler_version"] != "cc 1" {
		t.Fatalf("original host facts not retained: %v / %v", l.Set.Host, a.Set.Host)
	}
}

// S08-A08: a registered historical detector passes its row while a required mainstream
// failure still fails its cell and keeps the support claim blocked.
func TestQualifyDetectorAndGap(t *testing.T) {
	f := newQfx(t)
	f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
	gap := func(s string, recs []map[string]any) {
		if s != "s06-fxa" {
			return
		}
		nodes := fxTree("alpha", true)
		tr := recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
		tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["has_error"] = nodes, TreeDigest(nodes), true
		recs[0]["expectations"].([]any)[0].(map[string]any)["result"] = "FAIL"
		recs[0]["claims"].(map[string]string)["expectations"], recs[0]["assessment"] = "FAIL", "FAIL"
	}
	muts := map[string]qfxMut{}
	for _, p := range qfxPlatforms {
		muts[p.ID] = qfxMut{record: gap}
	}
	r := f.run(t, f.all(t, muts))
	c := cellOf(r, "fxa", "windows-amd64")
	if c.Mechanism != AssessPass || c.Requirement != AssessFail || c.Status != CellFail || r.SupportClaim != "BLOCKED" || r.MechanismGate != AssessPass {
		t.Fatalf("mainstream gap: %s %s %s %s", c.Mechanism, c.Requirement, c.Status, r.SupportClaim)
	}
	if ob := c.Obligations[slices.IndexFunc(c.Obligations, func(o QualObligation) bool { return o.Kind == "P" })]; ob.Result != claimFail || !slices.Contains(ob.Cases, "c1") {
		t.Fatalf("the failing case must stay on its obligation: %+v", ob)
	}
	for _, x := range r.ExtraRoles {
		if x.ID == "fx-history" && x.Platform == "windows-amd64" && (x.Status != CellPass || x.Sets[0].Detectors["d1"] != "PASS") {
			t.Fatalf("detector row: %s %v", x.Status, x.Sets[0].Detectors)
		}
	}
	// a detector never satisfies a requirement row
	f.inv.ExtraRoles[0].Workloads[0].Cases[0].Covers = map[string][]string{"fxa-B01": {"P"}}
	if _, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate}); err == nil || !strings.Contains(err.Error(), "DETECTOR_COVERS") {
		t.Fatalf("detector covering a requirement: %v", err)
	}
}

// S08-A09: a replay or an old commit's success cannot fill a current-candidate cell.
func TestQualifyEligibility(t *testing.T) {
	for name, mut := range map[string]func(id map[string]any){
		"old-sha":     func(id map[string]any) { id["sha"], id["checkout"] = strings.Repeat("b", 40), strings.Repeat("b", 40) },
		"replay-mode": func(id map[string]any) { id["evidence_mode"] = ModeReplayedRaw },
		"carried":     func(id map[string]any) { id["evidence_mode"] = ModeCarriedForward },
	} {
		t.Run(name, func(t *testing.T) {
			f := newQfx(t)
			muts := map[string]qfxMut{}
			for _, p := range qfxPlatforms {
				muts[p.ID] = qfxMut{identity: mut}
			}
			r := f.run(t, f.all(t, muts))
			c := cellOf(r, "fxa", "windows-amd64")
			if r.Assessment != AssessFail || c.Set.Evidence != "REJECTED" || !slices.Contains(setCodes(c), "ELIGIBILITY_REJECTED") || r.SupportClaim != "BLOCKED" {
				t.Fatalf("%s: %s %s %v", name, r.Assessment, c.Set.Evidence, setCodes(c))
			}
		})
	}
	// one host of an old commit: only its cells are rejected, the others keep their cohort
	f0 := newQfx(t)
	r0 := f0.run(t, f0.all(t, map[string]qfxMut{"windows-amd64": {identity: func(id map[string]any) { id["sha"], id["checkout"] = strings.Repeat("b", 40), strings.Repeat("b", 40) }}}))
	if c := cellOf(r0, "fxa", "linux-amd64"); c.Set.Evidence != ModeNewRun || slices.Contains(setCodes(c), "COHORT_MISMATCH") || r0.Run == nil || r0.Run.SHA != qfxCandidate {
		t.Fatalf("rejected host as cohort base: %s %v %+v", c.Set.Evidence, setCodes(c), r0.Run)
	}
	// a host without a run identity is not a run
	f := newQfx(t)
	hosts := f.all(t, nil)
	os.Remove(filepath.Join(hosts["linux-amd64"], "run-identity.json"))
	if c := cellOf(f.run(t, hosts), "fxa", "linux-amd64"); c.Status != CellFail || !slices.Contains(setCodes(c), "RUN_IDENTITY_MISSING") {
		t.Fatalf("no identity: %s %v", c.Status, setCodes(c))
	}
}

// S08-A12 controls inside the set: a dropped record, a record of another case, a kit
// claim failure, a claimed query result the registered captures do not give and a
// collapsed dialect (another route's set under this route) all fail the mechanism axis.
func TestQualifyMechanism(t *testing.T) {
	cases := []struct {
		name string
		mut  qfxMut
		code string
	}{
		{"dropped-record", qfxMut{manifest: func(s string, m map[string]any) {
			if s == "s06-fxa" {
				m["members"] = m["members"].([]OracleMember)[2:]
				m["records"] = 2
			}
		}}, "CASE_RECORD_MISMATCH"},
		{"kit-claim", qfxMut{record: func(s string, recs []map[string]any) {
			if s == "s06-fxa" {
				recs[1]["claims"].(map[string]string)["incremental_equality"] = "FAIL"
				recs[1]["assessment"] = "FAIL"
				recs[1]["steps"].([]any)[1].(map[string]any)["comparison"] = map[string]any{"equal": false, "first_difference": nil}
			}
		}}, "KIT_CLAIM_FAILED"},
		{"query-claim", qfxMut{record: func(s string, recs []map[string]any) {
			if s == "s06-fxa" {
				tr := recs[2]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
				tr["queries"].([]any)[0].(map[string]any)["captures"].([]Capture)[0].EndByte = 3
			}
		}}, ""},
		{"collapsed-dialect", qfxMut{manifest: func(s string, m map[string]any) {
			if s == "s06-fxa" {
				m["workload"].(map[string]string)["route"] = "fxb"
			}
		}}, "ROUTE_MISMATCH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newQfx(t)
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": tc.mut}))
			c := cellOf(r, "fxa", "windows-amd64")
			if c.Status != CellFail || r.Assessment != AssessFail || r.MechanismGate != AssessFail {
				t.Fatalf("%s: %s %s %v %+v", tc.name, c.Status, c.Mechanism, setCodes(c), c.Set.Gates)
			}
			if tc.code != "" && !slices.Contains(setCodes(c), tc.code) {
				t.Fatalf("%s: want %s in %v", tc.name, tc.code, setCodes(c))
			}
			if tc.name == "query-claim" {
				g := slices.IndexFunc(c.Set.Gates, func(g ReplayGate) bool { return g.ID == "query-expectations" })
				if g < 0 || c.Set.Gates[g].Failed == 0 {
					t.Fatalf("recorded query PASS over recomputed FAIL not caught: %+v", c.Set.Gates)
				}
			}
		})
	}
}

// S08-A11: cancellation, limits and an oversized result end typed and never PASS.
func TestQualifyLimitsAndCancel(t *testing.T) {
	f := newQfx(t)
	hosts := f.all(t, nil)
	var hs []QualifyHost
	for _, p := range qfxPlatforms {
		hs = append(hs, QualifyHost{p.ID, hosts[p.ID]})
	}
	ctx, cancel := context.WithCancel(ctxT(t))
	cancel()
	if res, err := Qualify(ctx, QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs}); err == nil || res.ExecutionStatus != StatusCancelled {
		t.Fatalf("cancel: %v %s", err, res.ExecutionStatus)
	}
	for name, l := range map[string]func(ReplayLimits) ReplayLimits{
		"total":   func(l ReplayLimits) ReplayLimits { l.TotalBytes = 2000; return l },
		"files":   func(l ReplayLimits) ReplayLimits { l.Files = 3; return l },
		"output":  func(l ReplayLimits) ReplayLimits { l.OutputBytes = 100; return l },
		"records": func(l ReplayLimits) ReplayLimits { l.Records = 2; return l },
	} {
		testHookQualifyLimits = l
		res, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs})
		testHookQualifyLimits = nil
		if err == nil || res.ExecutionStatus != StatusResourceLimit || res.Assessment == AssessPass {
			t.Fatalf("%s: %v %s %s", name, err, res.ExecutionStatus, res.Assessment)
		}
	}
	// the wall expires during the aggregation
	testHookWall = func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errWall)
		return ctx, func() {}
	}
	res, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs})
	testHookWall = nil
	if err == nil || res.ExecutionStatus != StatusResourceLimit {
		t.Fatalf("wall: %v %s", err, res.ExecutionStatus)
	}
	// a link inside a host directory is rejected, never followed
	if err := os.Symlink(filepath.Join(hosts["linux-amd64"], "summary.json"), filepath.Join(hosts["windows-amd64"], "link.json")); err == nil {
		if _, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs}); err == nil || !strings.Contains(err.Error(), "LINK_OR_SPECIAL_REJECTED") {
			t.Fatalf("link: %v", err)
		}
	}
}

// Inventory guards: the declared cells are the exact product, coverage claims obey the
// rule, two routes cannot share a set (collapsed dialects) and sources match inputs.
func TestQualificationInventoryGuards(t *testing.T) {
	for name, mut := range map[string]func(inv *QualificationInventory){
		"cells":      func(inv *QualificationInventory) { inv.Cells = 5 },
		"dup-route":  func(inv *QualificationInventory) { inv.Routes[1] = inv.Routes[0] },
		"shared-set": func(inv *QualificationInventory) { inv.Routes[1].Workload.Set = inv.Routes[0].Workload.Set },
		"cover-rule": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[0].Covers["fxa-B01"] = []string{"P", "E"}
		},
		"cover-row": func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[0].Covers["fxa-B09"] = []string{"P"} },
		"cover-w": func(inv *QualificationInventory) {
			inv.Routes[0].Requirements[0].Kinds = append(inv.Routes[0].Requirements[0].Kinds, "W")
			inv.Routes[0].Workload.Cases[0].Covers["fxa-B01"] = []string{"P", "W"}
		},
		"source":          func(inv *QualificationInventory) { s := "alphz"; inv.Routes[0].Workload.Cases[2].Source = &s },
		"kinds":           func(inv *QualificationInventory) { inv.Kinds = []string{"P"} },
		"role-status":     func(inv *QualificationInventory) { inv.ExtraRoles[1].Status = "PASS" },
		"role-mainstream": func(inv *QualificationInventory) { inv.ExtraRoles[1].Role = "mainstream" },
		"no-rows":         func(inv *QualificationInventory) { inv.Routes[0].Requirements = nil },
		"step-range":      func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[0].Expect[0].Step = 1 },
		"step-negative":   func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[2].QueryExpect[0].Step = -1 },
		"support-covers":  func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[0].Role = "support" },
		"over-covers":     func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[0].ExpectStatus = StatusResourceLimit },
		"route-detector": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[1].Role, inv.Routes[0].Workload.Cases[1].Covers = "detector", map[string][]string{}
		},
	} {
		f := newQfx(t)
		mut(&f.inv)
		if _, err := ParseQualificationInventory(f.invBytes(t)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	f := newQfx(t)
	if _, err := ParseQualificationInventory(f.invBytes(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: "HEAD"}); err == nil {
		t.Fatal("candidate must be a commit id")
	}
}
