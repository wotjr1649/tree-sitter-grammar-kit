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

// qfx is a synthetic qualification: routes fxa (P, E and Q all covered; the row's one
// registered alternative is COMPLETE and listed by c1) and fxb (also requires N, which no
// case covers), three platforms and one historical detector role.
type qfx struct {
	inv  QualificationInventory
	root string
}

func qfxCases(route string) []QualCase {
	alpha, beta := "alpha", "beta"
	row := route + "-B01"
	return []QualCase{
		{ID: "c1", Role: "requirement", Input: NativeInput{SHA256: sum([]byte(alpha)), Bytes: 5}, Edits: []Edit{},
			Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{"word"}}}, QueryExpect: []QueryExpectation{}, Covers: map[string][]string{row: {"P"}},
			Alternatives: []string{row + ".a01"}},
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
		f.inv.Routes = append(f.inv.Routes, QualRoute{Route: r, Workload: qfxWorkload("s06-"+r, r, qfxCases(r)),
			Requirements: []QualRequirement{{Row: r + "-B01", Kinds: kinds, AlternativesStatus: AlternativesComplete, Alternatives: []string{r + "-B01.a01"}}}})
	}
	det := qfxCases("fxh")[:1]
	det[0].ID, det[0].Role, det[0].Covers, det[0].Alternatives = "d1", "detector", map[string][]string{}, nil
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

func qfxAPI() map[string]any {
	return map[string]any{"revision": "tsgk-api/r1", "consistent": true, "first_difference": nil, "position_navigation_divergences": 0, "first_divergence": nil}
}

func qfxStep(i int, s string, fresh, query bool) map[string]any {
	inc := fxTreeOut(s, false)
	inc["api"] = qfxAPI()
	if query {
		n := uint32(len(s))
		inc["queries"] = []any{map[string]any{"id": "q.x", "status": "COMPLETED", "code": "", "evaluation": "NOT_EVALUATED",
			"captures": []Capture{{Name: "w", Node: 1, Type: "word", Named: true, EndByte: n, EndPoint: Point{Column: n}}}}}
	}
	m := map[string]any{"step": i, "source_bytes": len(s), "source_sha256": sum([]byte(s)), "edit": nil, "route": nil, "comparison": nil, "incremental": inc, "fresh": nil}
	if fresh {
		freshTree := fxTreeOut(s, false)
		freshTree["api"] = qfxAPI()
		m["fresh"] = freshTree
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
		if w.Format != "" {
			prof["format"] = w.Format
		}
		if m.profile != nil {
			m.profile(w.Set, prof)
		}
		pdata, _ := json.Marshal(prof)
		write("profiles/"+w.Profile+".json", pdata)
		var recs []map[string]any
		for _, c := range w.Cases {
			src := map[string]string{"c1": "alpha", "c2": "beta", "q1": "alpha", "d1": "alpha", "s1": "alpha"}[c.ID]
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

// S08-A07: an SVC composite names its platform's native build as producer, as every tree
// does. With a different build on each host only that differs, so the comparison passes
// and the build stays a host observation; a composite producer other than the run's build
// is a mixed result on that host; a real composite difference still fails the comparison.
func TestQualifyCompositeHostIdentity(t *testing.T) {
	other := strings.Repeat("9", 64)
	host := func(build string, edit func(c map[string]any)) qfxMut {
		return qfxMut{record: func(s string, recs []map[string]any) {
			if s != "s06-fxa" {
				return
			}
			for _, rc := range recs {
				for _, st := range rc["steps"].([]any) {
					for _, k := range []string{"incremental", "fresh"} {
						if tr, ok := st.(map[string]any)[k].(map[string]any); ok {
							tr["tree"].(map[string]any)["identities"].([]IdentityRef)[0].SHA256 = build
						}
					}
				}
			}
			ids := func() []IdentityRef {
				return []IdentityRef{{"producer", "tsgk-native-build/r1", build}, {"source", "tsgk-source-bytes/r1", sum([]byte("alpha"))}, {"policy", "tsgk-native-policy/r1", fxPolicy}}
			}
			c := map[string]any{"schema": "tsgk-svc-composite/r1", "language": map[string]any{"status": "CSHARP", "value": nil}, "identities": ids(),
				"directive": &SvcDirective{Close: &Span{}, Diagnostics: []string{}}, "coverage": SvcCoverage{Inline: "OBSERVED"},
				"inline": map[string]any{"tree": map[string]any{"identities": ids()}}}
			if edit != nil {
				edit(c)
			}
			for _, rc := range recs {
				for _, st := range rc["steps"].([]any) {
					st.(map[string]any)["composite"] = c
				}
			}
		}, manifest: func(s string, m map[string]any) {
			if s == "s06-fxa" {
				m["producer"].(map[string]string)["build_identity"] = build
			}
		}}
	}
	fixture := func() *qfx {
		f := newQfx(t)
		f.inv.Routes[0].Workload.Format = SvcLegacyFormat
		f.inv.Routes[0].Workload.Symbol = "tree_sitter_c_sharp"
		return f
	}
	f := fixture()
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": host(fxProducer, nil), "linux-amd64": host(other, nil), "darwin-arm64": host(fxProducer, nil)}))
	for _, pl := range qfxPlatforms {
		if c := cellOf(r, "fxa", pl.ID); c.Comparison != AssessPass || c.Mechanism != AssessPass {
			t.Fatalf("build-only composite difference on %s: %s %s %v", pl.ID, c.Comparison, c.Mechanism, r.Comparisons[0].Differences)
		}
	}
	if l := cellOf(r, "fxa", "linux-amd64"); l.Set.Host["build_identity"] != other {
		t.Fatalf("host build identity not retained: %v", l.Set.Host)
	}

	f = fixture()
	mixed := host(other, func(c map[string]any) { c["identities"].([]IdentityRef)[0].SHA256 = fxProducer })
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": host(fxProducer, nil), "linux-amd64": mixed, "darwin-arm64": host(fxProducer, nil)}))
	if l := cellOf(r, "fxa", "linux-amd64"); l.Mechanism == AssessPass || !slices.ContainsFunc(l.Set.Gates, func(g ReplayGate) bool { return g.ID == "case-binding" && g.Code == "MIXED_IDENTITY" }) {
		t.Fatalf("composite from another build accepted: %s %+v", l.Mechanism, l.Set.Gates)
	}

	f = fixture()
	inline := host(other, func(c map[string]any) {
		c["inline"].(map[string]any)["tree"].(map[string]any)["identities"].([]IdentityRef)[0].SHA256 = fxProducer
	})
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": host(fxProducer, nil), "linux-amd64": inline, "darwin-arm64": host(fxProducer, nil)}))
	if l := cellOf(r, "fxa", "linux-amd64"); l.Mechanism == AssessPass || !slices.ContainsFunc(l.Set.Gates, func(g ReplayGate) bool { return g.ID == "case-binding" && g.Code == "MIXED_IDENTITY" }) {
		t.Fatalf("composite inline tree from another build accepted: %s %+v", l.Mechanism, l.Set.Gates)
	}

	// only the producer is left out: a different composite source is still a difference
	f = fixture()
	src := host(fxProducer, func(c map[string]any) { c["identities"].([]IdentityRef)[1].SHA256 = sum([]byte("other")) })
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": host(fxProducer, nil), "linux-amd64": host(other, nil), "darwin-arm64": src}))
	if c := cellOf(r, "fxa", "darwin-arm64"); c.Comparison != AssessFail || len(r.Comparisons[0].Differences) == 0 {
		t.Fatalf("composite source difference not reported: %s %v", c.Comparison, r.Comparisons[0].Differences)
	}

	f = fixture()
	vb := host(fxProducer, func(c map[string]any) { c["language"] = "VB" })
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": host(fxProducer, nil), "linux-amd64": host(other, nil), "darwin-arm64": vb}))
	if c := cellOf(r, "fxa", "darwin-arm64"); c.Comparison != AssessFail || len(r.Comparisons[0].Differences) == 0 {
		t.Fatalf("composite difference not reported: %s %v", c.Comparison, r.Comparisons[0].Differences)
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
		if err == nil || res.ExecutionStatus != StatusResourceLimit || res.Assessment == AssessPass || len(res.PlatformClaims) != 0 {
			t.Fatalf("%s: %v %s %s %v", name, err, res.ExecutionStatus, res.Assessment, res.PlatformClaims)
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

func roleRow(r QualificationResult, id, plat string) QualRoleRow {
	for _, x := range r.ExtraRoles {
		if x.ID == id && x.Platform == plat {
			return x
		}
	}
	return QualRoleRow{}
}

// svcRole adds a windows-only owned role whose SVC-SERVICEHOST-r1 workload has one support
// case s1 without expectations or covers and returns the role's workload.
func (f *qfx) svcRole() *QualWorkload {
	w := qfxWorkload("s06-fxs", "fxs", []QualCase{{ID: "s1", Role: "support", Input: NativeInput{SHA256: sum([]byte("alpha")), Bytes: 5}, Edits: []Edit{},
		Expect: []StepExpectation{}, QueryExpect: []QueryExpectation{}, Covers: map[string][]string{}}})
	w.Symbol, w.Format = "tree_sitter_c_sharp", SvcLegacyFormat
	f.inv.ExtraRoles = append(f.inv.ExtraRoles, QualRole{ID: "fx-svc", Role: "owned", Status: RoleExecuted, Reason: "owned .svc fixture", Platforms: []string{"windows-amd64"},
		NotApplicable: map[string]string{"linux-amd64": "windows only", "darwin-arm64": "windows only"}, Workloads: []QualWorkload{w}})
	return &f.inv.ExtraRoles[len(f.inv.ExtraRoles)-1].Workloads[0]
}

// qfxSvcObserved turns the s06-fxs record into an SVC observation-only one: a directive
// without %> (inline code unresolved) and no tree, so the S05 verdict is BLOCKED with
// SVC_INLINE_UNRESOLVED.
func qfxSvcObserved(set string, recs []map[string]any) {
	if set != "s06-fxs" {
		return
	}
	st := recs[0]["steps"].([]any)[0].(map[string]any)
	st["incremental"], st["fresh"] = nil, nil
	st["composite"] = map[string]any{"schema": "tsgk-svc-composite/r1", "identities": []IdentityRef{},
		"directive": SvcDirective{Attributes: []SvcAttribute{}, Diagnostics: []string{"TERMINATOR_MISSING"}},
		"coverage":  SvcCoverage{Directive: "OBSERVED", CodeBehind: "ABSENT", Inline: "UNRESOLVED"}}
	recs[0]["assessment"], recs[0]["code"] = AssessBlocked, "SVC_INLINE_UNRESOLVED"
	recs[0]["oracle_claims"].(map[string]string)["api"] = "NOT_CLAIMED"
}

// #76 platform claims: a platform is SUPPORTED only with global evidence integrity, its
// own cells' mechanism and requirement PASS and its executed extra-role rows PASS. Another
// platform's requirement and the cross-platform comparison stay in the global claim.
func TestQualifyPlatformClaims(t *testing.T) {
	covered := func() *qfx {
		f := newQfx(t)
		f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
		return f
	}
	claims := func(r QualificationResult) string {
		return r.PlatformClaims["windows-amd64"] + " " + r.PlatformClaims["linux-amd64"] + " " + r.PlatformClaims["darwin-arm64"]
	}
	f := covered()
	r := f.run(t, f.all(t, nil))
	if r.SupportClaim != "SUPPORTED" || claims(r) != "SUPPORTED SUPPORTED SUPPORTED" || len(r.PlatformClaims) != 3 {
		t.Fatalf("all pass: %s %v", r.SupportClaim, r.PlatformClaims)
	}
	if !slices.ContainsFunc(r.Explanation, func(s string) bool { return strings.Contains(s, "windows-amd64 SUPPORTED") }) {
		t.Fatalf("explanation without platform claims: %v", r.Explanation)
	}
	// the same all-pass evidence ending at the output limit after aggregation claims nothing,
	// in the fields and in the explanation
	f = covered()
	hosts := f.all(t, nil)
	var hs []QualifyHost
	for _, p := range qfxPlatforms {
		hs = append(hs, QualifyHost{Platform: p.ID, Root: hosts[p.ID]})
	}
	testHookQualifyLimits = func(l ReplayLimits) ReplayLimits { l.OutputBytes = 100; return l }
	r, err := Qualify(ctxT(t), QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate, Hosts: hs})
	testHookQualifyLimits = nil
	if err == nil || r.SupportClaim != "BLOCKED" || len(r.PlatformClaims) != 0 ||
		slices.ContainsFunc(r.Explanation, func(s string) bool { return strings.Contains(s, "SUPPORTED") }) {
		t.Fatalf("output limit after aggregation: %v %s %v %v", err, r.SupportClaim, r.PlatformClaims, r.Explanation)
	}

	// another platform's requirement fails: that platform and the global claim are BLOCKED
	f = covered()
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
	r = f.run(t, f.all(t, map[string]qfxMut{"linux-amd64": {record: gap}}))
	w, l := cellOf(r, "fxa", "windows-amd64"), cellOf(r, "fxa", "linux-amd64")
	if l.Requirement != AssessFail || w.Mechanism != AssessPass || w.Requirement != AssessPass || r.SupportClaim != "BLOCKED" || r.Assessment != AssessFail ||
		claims(r) != "SUPPORTED BLOCKED SUPPORTED" {
		t.Fatalf("linux requirement: %s %s/%s %s %v", l.Requirement, w.Mechanism, w.Requirement, r.SupportClaim, r.PlatformClaims)
	}

	// only the comparison fails: the platform claims stand, the global claim is BLOCKED
	f = covered()
	api := func(s string, recs []map[string]any) {
		if s == "s06-fxa" {
			tr := recs[2]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
			tr["queries"].([]any)[0].(map[string]any)["captures"].([]Capture)[0].Pattern = 1
		}
	}
	r = f.run(t, f.all(t, map[string]qfxMut{"darwin-arm64": {record: api}}))
	w = cellOf(r, "fxa", "windows-amd64")
	if w.Comparison != AssessFail || w.Mechanism != AssessPass || w.Requirement != AssessPass || r.SupportClaim != "BLOCKED" || r.MechanismGate != AssessFail ||
		r.PlatformClaims["windows-amd64"] != "SUPPORTED" {
		t.Fatalf("comparison only: %s %s/%s %s %s %v", w.Comparison, w.Mechanism, w.Requirement, r.SupportClaim, r.MechanismGate, r.PlatformClaims)
	}

	// a finding (evidence outside the inventory on one host) blocks every platform
	f = covered()
	hosts = f.all(t, nil)
	os.MkdirAll(filepath.Join(hosts["darwin-arm64"], "records", "s06-extra"), 0o755)
	os.WriteFile(filepath.Join(hosts["darwin-arm64"], "records", "s06-extra", "manifest.json"), []byte("{}"), 0o644)
	r = f.run(t, hosts)
	if len(r.Findings) == 0 || cellOf(r, "fxa", "windows-amd64").Status != CellPass || claims(r) != "BLOCKED BLOCKED BLOCKED" {
		t.Fatalf("finding: %v %s %v", codes(r.Findings), cellOf(r, "fxa", "windows-amd64").Status, r.PlatformClaims)
	}
	// a missing set on one host fails completeness: every platform is BLOCKED
	f = covered()
	r = f.run(t, f.all(t, map[string]qfxMut{"linux-amd64": {skip: func(s string) bool { return s == "s06-fxb" }}}))
	if r.Completeness != AssessFail || claims(r) != "BLOCKED BLOCKED BLOCKED" {
		t.Fatalf("completeness: %s %v", r.Completeness, r.PlatformClaims)
	}

	// an executed windows extra-role row is INCOMPLETE: only windows is BLOCKED
	f = covered()
	f.svcRole()
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: qfxSvcObserved}}))
	row := roleRow(r, "fx-svc", "windows-amd64")
	if row.Status != CellIncomplete || row.Mechanism != AssessPass || row.Checks != claimBlocked || cellOf(r, "fxa", "windows-amd64").Status != CellPass ||
		claims(r) != "BLOCKED SUPPORTED SUPPORTED" || r.SupportClaim != "BLOCKED" {
		t.Fatalf("windows role row: %s %s %s %v %v", row.Status, row.Mechanism, row.Checks, r.PlatformClaims, codes(r.Findings))
	}
}

// #76 a registered SVC observation-only verdict: the check passes exactly when the
// recorded assessment and code are the registered ones; any difference fails the row.
func TestQualifySvcExpectedAssessment(t *testing.T) {
	run := func(assess, code string) QualRoleRow {
		f := newQfx(t)
		w := f.svcRole()
		w.Cases[0].ExpectAssessment, w.Cases[0].ExpectCode = assess, code
		r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: qfxSvcObserved}}))
		return roleRow(r, "fx-svc", "windows-amd64")
	}
	if x := run(AssessBlocked, "SVC_INLINE_UNRESOLVED"); x.Status != CellPass || x.Checks != claimPass || x.Mechanism != AssessPass {
		t.Fatalf("expected verdict: %s %s %s", x.Status, x.Checks, x.Mechanism)
	}
	for _, w := range [][2]string{{AssessBlocked, "SVC_INLINE_UNSUPPORTED"}, {AssessPass, ""}} {
		if x := run(w[0], w[1]); x.Status != CellFail || x.Checks != claimFail || x.Mechanism != AssessPass {
			t.Fatalf("other verdict %v: %s %s %s", w, x.Status, x.Checks, x.Mechanism)
		}
	}
	// a registered verdict on a tree record (not observation-only, so its code is not
	// recomputed by the verdict gate) never confirms the registration
	f := newQfx(t)
	w := f.svcRole()
	w.Cases[0].ExpectAssessment, w.Cases[0].ExpectCode = AssessBlocked, "SVC_INLINE_UNRESOLVED"
	claimed := func(s string, recs []map[string]any) {
		if s == "s06-fxs" {
			recs[0]["assessment"], recs[0]["code"] = AssessBlocked, "SVC_INLINE_UNRESOLVED"
		}
	}
	x := roleRow(f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: claimed}})), "fx-svc", "windows-amd64")
	if x.Checks != claimFail || x.Status == CellPass {
		t.Fatalf("tree record with a registered verdict: %s %s %s", x.Status, x.Checks, x.Mechanism)
	}
}

// #76 inventory guard: the registered verdict belongs only to an SVC-format case that
// covers no row and needs no tree, as a PASS without code or a BLOCKED with one.
func TestQualificationInventorySvcExpectation(t *testing.T) {
	// the fxa route workload in the SVC format; c2 has no step or query expectation
	base := func() *qfx {
		f := newQfx(t)
		f.inv.Routes[0].Workload.Symbol, f.inv.Routes[0].Workload.Format = "tree_sitter_c_sharp", SvcLegacyFormat
		c := &f.inv.Routes[0].Workload.Cases[1]
		c.Covers, c.ExpectAssessment, c.ExpectCode = map[string][]string{}, AssessBlocked, "SVC_INLINE_UNRESOLVED"
		return f
	}
	if _, err := ParseQualificationInventory(base().invBytes(t)); err != nil {
		t.Fatalf("registered SVC verdict rejected: %v", err)
	}
	for name, mut := range map[string]func(inv *QualificationInventory){
		"covering": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[1].Covers = map[string][]string{"fxa-B01": {"E"}}
		},
		"non-svc": func(inv *QualificationInventory) { inv.Routes[0].Workload.Format = "" },
		"step-expect": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[1].Expect = []StepExpectation{{Step: 1, Syntax: "ERROR", Contains: []string{}}}
		},
		"code-only":    func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[1].ExpectAssessment = "" },
		"blocked-bare": func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[1].ExpectCode = "" },
		"pass-code":    func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[1].ExpectAssessment = AssessPass },
		"fail":         func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[1].ExpectAssessment = AssessFail },
		"over-limit":   func(inv *QualificationInventory) { inv.Routes[0].Workload.Cases[1].ExpectStatus = StatusResourceLimit },
		"other-route": func(inv *QualificationInventory) {
			c := &inv.Routes[1].Workload.Cases[1] // fxb stays in the default format
			c.Covers, c.ExpectAssessment, c.ExpectCode = map[string][]string{}, AssessBlocked, "SVC_INLINE_UNRESOLVED"
		},
	} {
		f := base()
		mut(&f.inv)
		if _, err := ParseQualificationInventory(f.invBytes(t)); err == nil || !strings.Contains(err.Error(), "CASE_INVALID") {
			t.Fatalf("%s: %v", name, err)
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
		"cover-w": func(inv *QualificationInventory) { // a case without a registered sample never covers W
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

func obligation(c QualCell, row, kind string) QualObligation {
	for _, o := range c.Obligations {
		if o.Row == row && o.Kind == kind {
			return o
		}
	}
	return QualObligation{}
}

// expectCode parses a mutated fixture inventory and requires the error code.
func expectCode(t *testing.T, name, code string, mut func(inv *QualificationInventory)) {
	t.Helper()
	f := newQfx(t)
	mut(&f.inv)
	if _, err := ParseQualificationInventory(f.invBytes(t)); err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("%s: want %s, got %v", name, code, err)
	}
}

// #76 coverage rule r2, production alternatives: a row's P obligation is covered only when
// its alternatives are COMPLETE and each is listed by a case covering the row with P; the
// result is then the worst covering case. A PENDING row or an unlisted alternative leaves
// P NOT_COVERED, and the obligation counts its alternatives.
func TestQualifyAlternatives(t *testing.T) {
	covered := func() *qfx {
		f := newQfx(t)
		f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
		return f
	}
	f := covered()
	r := f.run(t, f.all(t, nil))
	ob := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "P")
	if r.SupportClaim != "SUPPORTED" || ob.Result != claimPass || ob.Alternatives == nil || *ob.Alternatives != 1 || *ob.AlternativesUncovered != 0 {
		t.Fatalf("complete and covered: %s %+v", r.SupportClaim, ob)
	}
	if e := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "E"); e.Alternatives != nil || e.AlternativesUncovered != nil {
		t.Fatalf("alternative counts on a non-P obligation: %+v", e)
	}
	if b, _ := json.Marshal(obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "Q")); strings.Contains(string(b), "alternatives") {
		t.Fatalf("non-P obligation encodes alternative counts: %s", b)
	}
	if b, _ := json.Marshal(ob); !strings.Contains(string(b), `"alternatives":1,"alternatives_uncovered":0`) {
		t.Fatalf("P obligation encoding: %s", b)
	}

	// a second registered alternative no case lists
	f = covered()
	f.inv.Routes[0].Requirements[0].Alternatives = append(f.inv.Routes[0].Requirements[0].Alternatives, "fxa-B01.a02")
	r = f.run(t, f.all(t, nil))
	c := cellOf(r, "fxa", "windows-amd64")
	ob = obligation(c, "fxa-B01", "P")
	if ob.Result != ObligationNotCovered || *ob.Alternatives != 2 || *ob.AlternativesUncovered != 1 || c.Status != CellIncomplete || r.SupportClaim != "BLOCKED" {
		t.Fatalf("one alternative uncovered: %+v %s %s", ob, c.Status, r.SupportClaim)
	}

	// PENDING: the alternatives are not all registered yet, whatever the cases list
	f = covered()
	f.inv.Routes[0].Requirements[0].AlternativesStatus = AlternativesPending
	r = f.run(t, f.all(t, nil))
	c = cellOf(r, "fxa", "windows-amd64")
	ob = obligation(c, "fxa-B01", "P")
	if ob.Result != ObligationNotCovered || *ob.AlternativesUncovered != 0 || c.Requirement != CellIncomplete || r.Totals.NotCoveredByKind["P"] != 3 {
		t.Fatalf("pending row: %+v %s %v", ob, c.Requirement, r.Totals.NotCoveredByKind)
	}
	// PENDING without any alternative: the same
	f.inv.Routes[0].Requirements[0].Alternatives = nil
	f.inv.Routes[0].Workload.Cases[0].Alternatives = nil
	f.root = t.TempDir()
	r = f.run(t, f.all(t, nil))
	if ob = obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "P"); ob.Result != ObligationNotCovered || *ob.Alternatives != 0 {
		t.Fatalf("pending empty row: %+v", ob)
	}

	// the case listing the alternative fails: the row fails
	f = covered()
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
	r = f.run(t, f.all(t, muts))
	c = cellOf(r, "fxa", "windows-amd64")
	if ob = obligation(c, "fxa-B01", "P"); ob.Result != claimFail || c.Status != CellFail || c.Mechanism != AssessPass || *ob.AlternativesUncovered != 0 {
		t.Fatalf("failing alternative case: %+v %s %s", ob, c.Status, c.Mechanism)
	}

	// a PENDING row with a failing P case: the obligation stays NOT_COVERED and the
	// failure reaches the requirement axis through the registered checks
	f = covered()
	f.inv.Routes[0].Requirements[0].AlternativesStatus = AlternativesPending
	r = f.run(t, f.all(t, muts))
	c = cellOf(r, "fxa", "windows-amd64")
	if ob = obligation(c, "fxa-B01", "P"); ob.Result != ObligationNotCovered || c.Counts.Fail != 0 || c.Checks != claimFail ||
		!slices.Contains(c.CheckFailures, "c1=FAIL") || c.Requirement != AssessFail || c.Status != CellFail {
		t.Fatalf("pending row with a failing P case: %+v %+v %s %v %s %s", ob, c.Counts, c.Checks, c.CheckFailures, c.Requirement, c.Status)
	}
}

// qfxErrorNode adds a named node of type typ under the word node of a recorded step's
// incremental tree (has_error stays false: tree-sitter does not flag named error nodes).
func qfxErrorNode(set, typ string, step int) func(string, []map[string]any) {
	return func(s string, recs []map[string]any) {
		if s != set {
			return
		}
		tr := recs[0]["steps"].([]any)[step].(map[string]any)["incremental"].(map[string]any)
		nodes := tr["tree"].(map[string]any)["nodes"].([]TreeNode)
		nodes = append(slices.Clone(nodes), TreeNode{Parent: 1, Type: typ, Named: true, EndByte: nodes[1].EndByte, EndPoint: nodes[1].EndPoint})
		tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["descendant_count"] = nodes, TreeDigest(nodes), len(nodes)
	}
}

func qfxAll(m qfxMut) map[string]qfxMut {
	out := map[string]qfxMut{}
	for _, p := range qfxPlatforms {
		out[p.ID] = m
	}
	return out
}

// Review #81: a P step on a route with registered error node types passes only when its
// recorded tree holds none of them (has_error does not see them). A registered error node
// in the tree fails P; a route without the registration keeps P.
func TestQualifyErrorNodesInTree(t *testing.T) {
	covered := func(errNodes []string) *qfx {
		f := newQfx(t)
		f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
		f.inv.Routes[0].ErrorNodes = errNodes
		return f
	}
	f := covered([]string{"bad"})
	r := f.run(t, f.all(t, qfxAll(qfxMut{record: qfxErrorNode("s06-fxa", "bad", 0)})))
	c := cellOf(r, "fxa", "windows-amd64")
	if ob := obligation(c, "fxa-B01", "P"); ob.Result != claimFail || c.Mechanism != AssessPass || c.Checks != claimPass || c.Status != CellFail {
		t.Fatalf("P with a registered error node in the tree: %+v %s %s %s %v", ob, c.Mechanism, c.Checks, c.Status, setCodes(c))
	}
	// the same tree on a route that registers another type, or none: P passes
	for _, nodes := range [][]string{{"other"}, nil} {
		f = covered(nodes)
		r = f.run(t, f.all(t, qfxAll(qfxMut{record: qfxErrorNode("s06-fxa", "bad", 0)})))
		if ob := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "P"); ob.Result != claimPass {
			t.Fatalf("error nodes %v: %+v", nodes, ob)
		}
	}
	// a P step without a full tree on an error-node route is BLOCKED, never PASS
	f = covered([]string{"bad"})
	record := func(s string, recs []map[string]any) {
		if s == "s06-fxa" {
			recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["form"] = "record"
		}
	}
	r = f.run(t, f.all(t, qfxAll(qfxMut{record: record})))
	if ob := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "P"); ob.Result != claimBlocked {
		t.Fatalf("P without a full tree: %+v", ob)
	}
}

// #76 inventory r2 guards for alternatives: a row's status is COMPLETE (with at least one
// alternative) or PENDING, its ids are "<row>.aNN" and unique; a case lists only known
// alternatives of rows it covers with P.
func TestQualificationInventoryAlternatives(t *testing.T) {
	for name, mut := range map[string]func(inv *QualificationInventory){
		"unknown-id": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[0].Alternatives = []string{"fxa-B01.a09"}
		},
		"other-route-id": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[0].Alternatives = []string{"fxb-B01.a01"}
		},
		"row-not-p": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[1].Alternatives = []string{"fxa-B01.a01"}
		},
		"duplicate": func(inv *QualificationInventory) {
			inv.Routes[0].Workload.Cases[0].Alternatives = []string{"fxa-B01.a01", "fxa-B01.a01"}
		},
		"support-case": func(inv *QualificationInventory) {
			c := &inv.Routes[0].Workload.Cases[1]
			c.Role, c.Covers, c.Alternatives = "support", map[string][]string{}, []string{"fxa-B01.a01"}
		},
	} {
		expectCode(t, name, "CASE_INVALID", mut)
	}
	for name, mut := range map[string]func(inv *QualificationInventory){
		"status-empty":   func(inv *QualificationInventory) { inv.Routes[0].Requirements[0].AlternativesStatus = "" },
		"status-other":   func(inv *QualificationInventory) { inv.Routes[0].Requirements[0].AlternativesStatus = "DONE" },
		"complete-empty": func(inv *QualificationInventory) { inv.Routes[0].Requirements[0].Alternatives = nil },
		"foreign-prefix": func(inv *QualificationInventory) {
			inv.Routes[0].Requirements[0].Alternatives = []string{"fxb-B01.a01"}
		},
		"bad-suffix": func(inv *QualificationInventory) {
			inv.Routes[0].Requirements[0].Alternatives = []string{"fxa-B01.alt1"}
		},
		"short-suffix": func(inv *QualificationInventory) { inv.Routes[0].Requirements[0].Alternatives = []string{"fxa-B01.a1"} },
		"dup-id": func(inv *QualificationInventory) {
			inv.Routes[0].Requirements[0].Alternatives = []string{"fxa-B01.a01", "fxa-B01.a01"}
		},
	} {
		expectCode(t, name, "REQUIREMENT_INVALID", mut)
	}
	// controls: a PENDING row may list some alternatives, and a case may list none
	f := newQfx(t)
	f.inv.Routes[0].Requirements[0].AlternativesStatus = AlternativesPending
	f.inv.Routes[1].Workload.Cases[0].Alternatives = nil
	if _, err := ParseQualificationInventory(f.invBytes(t)); err != nil {
		t.Fatal(err)
	}
}

// #76 per-route error node types: a NO_ERROR step naming a registered error node type
// derives N, and R when it also names another node type; it never derives P. The step
// still passes its own NO_ERROR + contains expectation. Without the registration the
// same contains derives nothing new.
func TestQualifyErrorNodes(t *testing.T) {
	// fxb: c1 expects NO_ERROR with doc and word, and covers N and R instead of P
	errCase := func(errNodes []string, contains []string, kinds ...string) *qfx {
		f := newQfx(t)
		r := &f.inv.Routes[1]
		r.ErrorNodes = errNodes
		r.Requirements[0] = QualRequirement{Row: "fxb-B01", Kinds: []string{"N", "R", "E", "Q"}, AlternativesStatus: AlternativesPending}
		c := &r.Workload.Cases[0]
		c.Expect[0].Contains, c.Covers, c.Alternatives = contains, map[string][]string{"fxb-B01": kinds}, nil
		return f
	}
	contains := func(list []string) func(string, []map[string]any) {
		return func(s string, recs []map[string]any) {
			if s == "s06-fxb" {
				recs[0]["expectations"].([]any)[0].(map[string]any)["contains"] = list
			}
		}
	}
	f := errCase([]string{"word"}, []string{"doc", "word"}, "N", "R")
	mut := qfxMut{record: contains([]string{"doc", "word"})}
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut, "linux-amd64": mut, "darwin-arm64": mut}))
	c := cellOf(r, "fxb", "windows-amd64")
	if c.Status != CellPass || c.Checks != claimPass || obligation(c, "fxb-B01", "N").Result != claimPass || obligation(c, "fxb-B01", "R").Result != claimPass {
		t.Fatalf("registered error node: %s %s %+v %v", c.Status, c.Checks, c.Counts, setCodes(c))
	}
	// the registered error node alone: N, not R
	if _, err := ParseQualificationInventory(errCase([]string{"word"}, []string{"word"}, "N").invBytes(t)); err != nil {
		t.Fatalf("error node alone for N: %v", err)
	}
	for name, f := range map[string]*qfx{
		"error-node-alone-r": errCase([]string{"word"}, []string{"word"}, "N", "R"),
		"error-step-p":       errCase([]string{"word"}, []string{"doc", "word"}, "P"),
		"unregistered-n":     errCase(nil, []string{"doc", "word"}, "N"),
		"unregistered-r":     errCase(nil, []string{"doc", "word"}, "R"),
		"other-route-n":      errCase([]string{"other"}, []string{"doc", "word"}, "N"),
	} {
		if _, err := ParseQualificationInventory(f.invBytes(t)); err == nil || !strings.Contains(err.Error(), "COVERAGE_RULE_VIOLATION") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, nodes := range map[string][]string{"error": {"ERROR"}, "dup": {"word", "word"}, "name": {"bad node"}, "empty": {""}} {
		expectCode(t, name, "ROUTE_INVALID", func(inv *QualificationInventory) { inv.Routes[0].ErrorNodes = nodes })
	}
}

// #76 W producer: a requirement case with a registered public sample (permissive SPDX
// license, 40-hex commit, relative path, the case input's sha256 and bytes, at most 65536
// bytes) covers W when its step 0 expects NO_ERROR; the recorded step 0 tree must stay
// within 10000 nodes.
func TestQualifySampleW(t *testing.T) {
	sample := func() *QualSample {
		return &QualSample{Repository: "owner/name", Commit: strings.Repeat("a", 40), Path: "src/alpha.txt", License: "MIT", SHA256: sum([]byte("alpha")), Bytes: 5}
	}
	withW := func() *qfx {
		f := newQfx(t)
		f.inv.Routes[1].Requirements[0].Kinds = []string{"P", "E", "Q"}
		f.inv.Routes[0].Requirements[0].Kinds = []string{"P", "E", "Q", "W"}
		c := &f.inv.Routes[0].Workload.Cases[0]
		c.Sample, c.Covers["fxa-B01"] = sample(), []string{"P", "W"}
		return f
	}
	f := withW()
	r := f.run(t, f.all(t, nil))
	c := cellOf(r, "fxa", "windows-amd64")
	if ob := obligation(c, "fxa-B01", "W"); ob.Result != claimPass || c.Status != CellPass || r.SupportClaim != "SUPPORTED" {
		t.Fatalf("valid sample: %+v %s %s", ob, c.Status, r.SupportClaim)
	}
	for _, lic := range []string{"Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "PostgreSQL"} {
		f := withW()
		f.inv.Routes[0].Workload.Cases[0].Sample.License = lic
		if _, err := ParseQualificationInventory(f.invBytes(t)); err != nil {
			t.Fatalf("license %s: %v", lic, err)
		}
	}
	// over the node bound the sample is not W evidence
	big := func(s string, recs []map[string]any) {
		if s == "s06-fxa" {
			recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["descendant_count"] = SampleMaxNodes + 1
		}
	}
	f = withW()
	r = f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": {record: big}}))
	if ob := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "W"); ob.Result != claimBlocked || cellOf(r, "fxa", "windows-amd64").Status == CellPass {
		t.Fatalf("over the node bound: %+v", ob)
	}
	// Review #81: on an error-node route the step 0 tree must hold no registered error
	// node (FAIL when it does) and must be a full tree to check (BLOCKED otherwise). The
	// sample step names no structure, so only this check can tell the cases apart.
	bare := func() *qfx {
		f := withW()
		f.inv.Routes[0].ErrorNodes = []string{"bad"}
		f.inv.Routes[0].Requirements[0] = QualRequirement{Row: "fxa-B01", Kinds: []string{"E", "Q", "W"}, AlternativesStatus: AlternativesPending}
		c := &f.inv.Routes[0].Workload.Cases[0]
		c.Expect[0].Contains, c.Covers["fxa-B01"], c.Alternatives = []string{}, []string{"W"}, nil
		return f
	}
	noContains := func(s string, recs []map[string]any) {
		if s == "s06-fxa" {
			recs[0]["expectations"].([]any)[0].(map[string]any)["contains"] = []string{}
		}
	}
	for name, tc := range map[string]struct {
		mut  func(string, []map[string]any)
		want string
	}{
		"clean":      {nil, claimPass},
		"error-node": {qfxErrorNode("s06-fxa", "bad", 0), claimFail},
		"no-full-tree": {func(s string, recs []map[string]any) {
			if s == "s06-fxa" {
				recs[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["form"] = "record"
			}
		}, claimBlocked},
	} {
		f := bare()
		mut := func(s string, recs []map[string]any) {
			noContains(s, recs)
			if tc.mut != nil {
				tc.mut(s, recs)
			}
		}
		r := f.run(t, f.all(t, qfxAll(qfxMut{record: mut})))
		if ob := obligation(cellOf(r, "fxa", "windows-amd64"), "fxa-B01", "W"); ob.Result != tc.want {
			t.Fatalf("%s: W %+v, want %s", name, ob, tc.want)
		}
	}
	// a sample case whose step 0 expects an error does not cover W
	f = withW()
	c0 := &f.inv.Routes[0].Workload.Cases[0]
	c0.Expect[0].Syntax = "ERROR"
	c0.Covers["fxa-B01"], c0.Alternatives = []string{"W"}, nil
	if _, err := ParseQualificationInventory(f.invBytes(t)); err == nil || !strings.Contains(err.Error(), "COVERAGE_RULE_VIOLATION") {
		t.Fatalf("error step 0 covering W: %v", err)
	}
	for name, mut := range map[string]func(s *QualSample, c *QualCase){
		"license-gpl":   func(s *QualSample, _ *QualCase) { s.License = "GPL-3.0-only" },
		"license-empty": func(s *QualSample, _ *QualCase) { s.License = "" },
		"sha-mismatch":  func(s *QualSample, _ *QualCase) { s.SHA256 = sum([]byte("other")) },
		"bytes-differ":  func(s *QualSample, _ *QualCase) { s.Bytes = 6 },
		"over-size": func(s *QualSample, c *QualCase) {
			s.Bytes, c.Input.Bytes = SampleMaxBytes+1, SampleMaxBytes+1
		},
		"commit":      func(s *QualSample, _ *QualCase) { s.Commit = "main" },
		"repository":  func(s *QualSample, _ *QualCase) { s.Repository = "https://github.com/owner/name" },
		"path-escape": func(s *QualSample, _ *QualCase) { s.Path = "../alpha.txt" },
		"path-abs":    func(s *QualSample, _ *QualCase) { s.Path = "/alpha.txt" },
		"support-case": func(_ *QualSample, c *QualCase) {
			c.Role, c.Covers, c.Alternatives = "support", map[string][]string{}, nil
		},
	} {
		expectCode(t, name, "CASE_INVALID", func(inv *QualificationInventory) {
			inv.Routes[0].Requirements[0].Kinds = []string{"P", "E", "Q", "W"}
			c := &inv.Routes[0].Workload.Cases[0]
			c.Sample, c.Covers["fxa-B01"] = sample(), []string{"P", "W"}
			mut(c.Sample, c)
		})
	}
}

// qfxRouteNoReuse makes fxa's E case (c2) reuse no node at its edit step, with an error in
// its step 0 tree when errTree, and records the route claim with its verdict and code.
func qfxRouteNoReuse(errTree bool, route, code string) func(string, []map[string]any) {
	return func(s string, recs []map[string]any) {
		if s != "s06-fxa" {
			return
		}
		c := recs[1]
		steps := c["steps"].([]any)
		if errTree {
			tr := steps[0].(map[string]any)["incremental"].(map[string]any)
			nodes := slices.Clone(tr["tree"].(map[string]any)["nodes"].([]TreeNode))
			nodes[0].HasError = true
			tr["tree"].(map[string]any)["nodes"], tr["digest"], tr["has_error"] = nodes, TreeDigest(nodes), true
		}
		r := steps[1].(map[string]any)["route"].(map[string]any)
		r["reused_nodes"], r["proven"] = 0, false
		c["claims"].(map[string]string)["incremental_route"] = route
		c["assessment"], c["code"] = route, code
	}
}

// #76 axis separation: a route that could not be observed because the grammar left an
// error in the tree is not a kit failure. The recomputed BLOCKED route keeps the kit axis
// PASS (no KIT_CLAIM_FAILED) and makes the E obligation BLOCKED, so the cell is INCOMPLETE
// and never PASS. A route that is not proven on clean trees still fails the kit axis, and
// an error-tree route recorded as FAIL is a claim mismatch.
func TestQualifyRouteOnErrorTree(t *testing.T) {
	f := newQfx(t)
	r := f.run(t, f.all(t, qfxAll(qfxMut{record: qfxRouteNoReuse(true, claimBlocked, "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1")})))
	for _, p := range qfxPlatforms {
		c := cellOf(r, "fxa", p.ID)
		ob := obligation(c, "fxa-B01", "E")
		if c.Mechanism != AssessPass || slices.Contains(setCodes(c), "KIT_CLAIM_FAILED") || ob.Result != claimBlocked ||
			c.Requirement != CellIncomplete || c.Status != CellIncomplete || c.Comparison != AssessPass {
			t.Fatalf("%s blocked route: mechanism %s requirement %s status %s E %+v codes %v gates %+v", p.ID, c.Mechanism, c.Requirement, c.Status, ob, setCodes(c), c.Set.Gates)
		}
	}
	if r.MechanismGate != AssessPass || r.Assessment == AssessPass || r.Assessment == AssessFail {
		t.Fatalf("blocked route: gate %s assessment %s %v", r.MechanismGate, r.Assessment, r.Findings)
	}
	for name, mut := range map[string]struct {
		rec  func(string, []map[string]any)
		code string
	}{
		"clean-tree-fail":          {qfxRouteNoReuse(false, claimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"), "KIT_CLAIM_FAILED"},
		"error-tree-recorded-fail": {qfxRouteNoReuse(true, claimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1"), ""},
		// the error tree is not a full tree: its has_error is not recomputed, so the
		// exception does not apply and a recorded BLOCKED is a mismatch
		"summary-error-tree-recorded-blocked": {func(s string, recs []map[string]any) {
			qfxRouteNoReuse(true, claimBlocked, "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1")(s, recs)
			if s == "s06-fxa" {
				tr := recs[1]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)
				tr["form"] = "summary"
				delete(tr, "tree")
			}
		}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newQfx(t)
			r := f.run(t, f.all(t, qfxAll(qfxMut{record: mut.rec})))
			c := cellOf(r, "fxa", "windows-amd64")
			if c.Mechanism != AssessFail || c.Status != CellFail || r.MechanismGate != AssessFail || (mut.code != "" && !slices.Contains(setCodes(c), mut.code)) {
				t.Fatalf("%s: mechanism %s status %s gate %s codes %v", name, c.Mechanism, c.Status, r.MechanismGate, setCodes(c))
			}
			if mut.code == "" {
				g := slices.IndexFunc(c.Set.Gates, func(g ReplayGate) bool { return g.ID == "incremental-route" })
				if g < 0 || c.Set.Gates[g].Code != "CLAIM_MISMATCH" {
					t.Fatalf("recorded route claim differing from the recomputed one not caught: %+v", c.Set.Gates)
				}
			} else if ob := obligation(c, "fxa-B01", "E"); ob.Result != claimFail {
				t.Fatalf("clean-tree route FAIL: E %+v", ob)
			}
		})
	}
}

func TestQualifyRejectsPositionalAPIPass(t *testing.T) {
	f := newQfx(t)
	mut := qfxMut{record: func(set string, records []map[string]any) {
		if set != "s06-fxa" {
			return
		}
		records[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["api"] = map[string]any{"revision": "tsgk-api/r1", "consistent": true, "position_navigation_divergences": 1}
	}}
	r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut, "linux-amd64": mut, "darwin-arm64": mut}))
	for _, platform := range qfxPlatforms {
		c := cellOf(r, "fxa", platform.ID)
		if c.Set.APIFails != 1 || c.Mechanism == AssessPass {
			t.Fatalf("positional API PASS: %+v", c)
		}
	}
}

func TestQualifyRequiresCompleteAPIProof(t *testing.T) {
	for _, bad := range []string{"", "claims", "claim", "api", "null", "empty", "revision", "consistent", "first_difference", "position_navigation_divergences", "first_divergence", "false", "difference", "divergence", "fresh"} {
		t.Run(bad, func(t *testing.T) {
			f := newQfx(t)
			mut := qfxMut{record: func(set string, records []map[string]any) {
				if set != "s06-fxa" {
					return
				}
				rec := records[1]
				step := rec["steps"].([]any)[1].(map[string]any)
				tree := step["incremental"].(map[string]any)
				api := tree["api"].(map[string]any)
				switch bad {
				case "claims":
					delete(rec, "oracle_claims")
				case "claim":
					delete(rec["oracle_claims"].(map[string]string), "api")
				case "api":
					delete(tree, "api")
				case "null":
					tree["api"] = nil
				case "empty":
					tree["api"] = map[string]any{}
				case "revision", "consistent", "first_difference", "position_navigation_divergences", "first_divergence":
					delete(api, bad)
				case "false":
					api["consistent"] = false
				case "difference":
					api["first_difference"] = map[string]any{"node": 1}
				case "divergence":
					api["first_divergence"] = map[string]any{"node": 1}
				case "fresh":
					delete(step["fresh"].(map[string]any), "api")
				}
			}}
			r := f.run(t, f.all(t, map[string]qfxMut{"windows-amd64": mut}))
			c := cellOf(r, "fxa", "windows-amd64")
			if (c.Mechanism == AssessPass) != (bad == "") || (c.Set.APIFails > 0) != (bad != "") {
				t.Fatalf("%s: mechanism %s API failures %d codes %v", bad, c.Mechanism, c.Set.APIFails, setCodes(c))
			}
		})
	}
}
