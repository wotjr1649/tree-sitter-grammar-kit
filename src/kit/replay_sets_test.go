package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeSet writes files and returns the members registration with the given roles.
func writeSet(t *testing.T, root string, files map[string][]byte, role func(string) string) []any {
	t.Helper()
	var members []any
	for _, p := range sortedKeys(files) {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, files[p], 0o644); err != nil {
			t.Fatal(err)
		}
		members = append(members, map[string]any{"path": p, "role": role(p), "bytes": len(files[p]), "sha256": sum(files[p])})
	}
	return members
}

func registration(reducer, op, assessment string, ids map[string]string, records []string, members []any) map[string]any {
	return map[string]any{"schema": ReplaySchema, "id": "fx-" + reducer, "reducer": reducer, "operation": op,
		"subject":    map[string]any{"run": "200", "attempt": 1, "platform": "windows/amd64", "commit": "abc", "evidence_mode": "NEW_RUN", "execution_status": "COMPLETED", "assessment": assessment},
		"identities": ids, "records": records, "members": members}
}

func runReg(t *testing.T, root string, reg map[string]any) ReplayResult {
	t.Helper()
	data, _ := json.Marshal(reg)
	r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: data})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	return r
}

// fxOracleSet builds an S06 record set from the native fixture: an oracle profile, one
// record and one raw response per case and the manifest written last.
func fxOracleSet(t *testing.T, mutate func(records []map[string]any, files map[string][]byte)) (string, map[string]any) {
	f := newFxNative()
	prof := f.profile
	prof["schema"], prof["operation"] = OracleSchema, "native-query"
	prof["queries"], prof["fact_pack"], prof["api"] = []any{}, nil, true
	for _, c := range prof["cases"].([]any) {
		c.(map[string]any)["query_expect"], c.(map[string]any)["dynamic_sql_expect"] = []any{}, nil
	}
	pdata, _ := json.Marshal(prof)
	records := f.cases
	for _, c := range records {
		c["oracle_claims"] = map[string]string{"query_equality": "NOT_CLAIMED", "query_expectations": "NOT_CLAIMED", "api": "PASS", "fact_reproduction": "NOT_CLAIMED", "dynamic_sql": "NOT_CLAIMED"}
	}
	files := map[string][]byte{"workload.json": pdata}
	if mutate != nil {
		mutate(records, files)
	}
	var members []OracleMember
	for i, c := range records {
		body, _ := json.Marshal(c)
		name := fmt.Sprintf("%05d-%s.json", i, c["id"])
		rec := append(append([]byte(`{"schema":"tsgk-oracle-record/r1","case":"`+c["id"].(string)+`","workload":"fx",`), body[1:len(body)-1]...), []byte(`,"complete":true}`)...)
		raw := []byte(`{"response":` + fmt.Sprint(i) + `}`)
		in := c["input"].(map[string]any)
		for _, m := range []struct {
			role string
			b    []byte
		}{{"record", rec}, {"raw", raw}} {
			p := map[string]string{"record": "records/", "raw": "raw/"}[m.role] + name
			files[p] = m.b
			members = append(members, OracleMember{Path: p, Role: m.role, Case: c["id"].(string), Bytes: uint64(len(m.b)), SHA256: sum(m.b),
				InputBytes: uint64(in["bytes"].(int)), InputSHA256: in["sha256"].(string), ExecutionStatus: c["execution_status"].(string)})
		}
	}
	man := ordered("schema", OracleManifestSchema,
		"workload", map[string]string{"operation": "native-query", "output": "tree", "profile_id": "fx", "profile_sha256": sum(pdata), "route": "owned-plain"},
		"producer", map[string]string{"build_identity": fxProducer, "compiler_sha256": fxCompiler, "compiler_version": "cc 1", "executable_sha256": fxExe, "platform": "windows/amd64", "runtime_commit": fxRuntime},
		"policy", IdentityRef{"policy", "tsgk-native-policy/r1", fxPolicy}, "protocol", "tsgk-native/r2",
		"comparators", []string{"tsgk-tree-digest/r1", "tsgk-capture-compare/r1", "tsgk-api/r1", "tsgk-predicates/r1"},
		"queries", []IdentityRef{}, "fact_pack", nil, "execution_status", "COMPLETED", "assessment", "PASS", "records", 2, "cases", []string{"c1", "c2"}, "members", members, "complete", true)
	files["manifest.json"] = man
	root := t.TempDir()
	ms := writeSet(t, root, files, func(p string) string {
		switch {
		case p == "manifest.json":
			return "manifest"
		case p == "workload.json":
			return "workload-profile"
		case strings.HasPrefix(p, "records/"):
			return "record"
		}
		return "raw"
	})
	ids := map[string]string{"workload": sum(pdata), "operation": "native-query", "producer": fxProducer, "executable": fxExe, "compiler": fxCompiler, "runtime": fxRuntime,
		"platform": "windows/amd64", "policy": fxPolicy, "protocol": "tsgk-native/r2", "comparators": "tsgk-tree-digest/r1,tsgk-capture-compare/r1,tsgk-api/r1,tsgk-predicates/r1", "queries": "", "fact_pack": ""}
	return root, registration("oracle-set-r1", "evidence-replay", "PASS", ids, nil, ms)
}

// S07-A01/A04/A13: an S06 record set replays through the shared record checks and the S05
// gates; the API claim lives in the raw response only, so it stays recorded and the replay
// is UNRESOLVED, never PASS.
func TestReplayOracleSet(t *testing.T) {
	root, reg := fxOracleSet(t, nil)
	r := runReg(t, root, reg)
	if !r.EvidenceValid || r.Assessment != AssessUnresolved || r.EvidenceMode != ModeReplayedRaw {
		t.Fatalf("oracle set: valid %v %s %s %v %+v", r.EvidenceValid, r.EvidenceMode, r.Assessment, r.Findings, r.Gates)
	}
	if g := gateOf(r, "api"); g.EvidenceMode != ModeRecordedNotRecomputed || g.NotRecomputed != 2 {
		t.Fatalf("api gate %+v", g)
	}
	if g := gateOf(r, "tree"); g.EvidenceMode != ModeReplayedRaw || g.Failed != 0 {
		t.Fatalf("tree gate %+v", g)
	}
	// a changed protocol or comparator list is a stale set
	reg["identities"].(map[string]string)["comparators"] = "tsgk-tree-digest/r1"
	r = runReg(t, root, reg)
	if r.EvidenceValid || !slices.Contains(codes(r.Findings), "IDENTITY_MISMATCH") {
		t.Fatalf("stale comparator: %v", r.Findings)
	}
	// a record whose case claims PASS while its API claim is FAIL
	root, reg = fxOracleSet(t, func(records []map[string]any, files map[string][]byte) {
		records[0]["oracle_claims"].(map[string]string)["api"] = "FAIL"
	})
	r = runReg(t, root, reg)
	if r.EvidenceValid || gateOf(r, "verdict").Code != "VERDICT_MISMATCH" {
		t.Fatalf("PASS label over FAIL claim: %+v", gateOf(r, "verdict"))
	}
	// an extra file next to the set is not part of it
	root, reg = fxOracleSet(t, nil)
	os.WriteFile(filepath.Join(root, "records", "00002-c3.json"), []byte("{}"), 0o644)
	if r = runReg(t, root, reg); !slices.Contains(codes(r.Findings), "MEMBER_UNLISTED") {
		t.Fatalf("unlisted record: %v", r.Findings)
	}
}

// S07-A02 (review r1 M2): a registered member the reducer never consumes, or one with a
// role the reducer does not know, is rejected; a member registered as retained is not.
func TestReplayUnconsumedMembers(t *testing.T) {
	root, reg := fxOracleSet(t, nil)
	stale := []byte(`{"stale":true}`)
	os.WriteFile(filepath.Join(root, "records", "00009-old.json"), stale, 0o644)
	reg["members"] = append(reg["members"].([]any), map[string]any{"path": "records/00009-old.json", "role": "record", "bytes": len(stale), "sha256": sum(stale)})
	if r := runReg(t, root, reg); r.EvidenceValid || !slices.Contains(codes(r.Findings), "MEMBER_UNUSED") {
		t.Fatalf("unconsumed record member: %v", r.Findings)
	}
	ms := reg["members"].([]any)
	ms[len(ms)-1].(map[string]any)["role"] = "notes"
	if r := runReg(t, root, reg); r.EvidenceValid || !slices.Contains(codes(r.Findings), "MEMBER_ROLE_UNKNOWN") {
		t.Fatalf("unknown role: %v", r.Findings)
	}
	ms[len(ms)-1].(map[string]any)["role"] = "retained"
	if r := runReg(t, root, reg); !r.EvidenceValid || r.Retained != 1 {
		t.Fatalf("retained member: %v %d", r.Findings, r.Retained)
	}
}

// review r2 N2/N5: a mixed tree identity found before an over-limit record still fails the
// replay; an over-limit retained member degrades the replay but does not hide an
// unconsumed registered member.
func TestReplayOverLimitKeepsChecks(t *testing.T) {
	root, reg := fxOracleSet(t, func(records []map[string]any, files map[string][]byte) {
		records[0]["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)["identities"] = []IdentityRef{{"producer", "tsgk-native-build/r1", fxExe}}
		records[1]["pad"] = strings.Repeat("p", 40*1024)
	})
	testHookReplayLimits = func(l ReplayLimits) ReplayLimits { l.FileBytes = 20 * 1024; return l }
	defer func() { testHookReplayLimits = nil }()
	if r := runReg(t, root, reg); r.Assessment != AssessFail || gateOf(r, "case-binding").Code != "MIXED_IDENTITY" {
		t.Fatalf("mixed identity before an over-limit record: %s %+v", r.Assessment, gateOf(r, "case-binding"))
	}
	root, reg = fxOracleSet(t, nil)
	big, stale := bytes.Repeat([]byte("b"), 40*1024), []byte(`{"stale":true}`)
	os.WriteFile(filepath.Join(root, "tool.bin"), big, 0o644)
	os.WriteFile(filepath.Join(root, "records", "00009-old.json"), stale, 0o644)
	reg["members"] = append(reg["members"].([]any), map[string]any{"path": "tool.bin", "role": "retained", "bytes": len(big), "sha256": sum(big)},
		map[string]any{"path": "records/00009-old.json", "role": "record", "bytes": len(stale), "sha256": sum(stale)})
	if r := runReg(t, root, reg); r.Assessment != AssessFail || !slices.Contains(codes(r.Findings), "MEMBER_UNUSED") {
		t.Fatalf("unconsumed member next to an over-limit retained member: %s %v", r.Assessment, r.Findings)
	}
}

// fxPrivate builds a tiny NET461-style corpus run: inventory (one routed tsql file, one
// routed file blocked by encoding, one presence-only and one unrouted file), the tsql
// route profile and result, the per-file projection and the summary.
func fxPrivate(t *testing.T, mutate func(files map[string][]byte, sum map[string]any, proj []map[string]any)) (string, map[string]any) {
	src := "select 1"
	inv := map[string]any{"schema": ReportSchema, "command": "corpus", "summary": map[string]any{"encoding_codes": map[string]int{"AMBIGUOUS_ENCODING": 1}},
		"records": []any{
			map[string]any{"path": "a/x.sql", "route": "tsql", "state": "COMPLETED", "size": len(src), "sha256": sum([]byte(src)), "encoding": map[string]any{"assessment": "PASS", "encoding": "UTF-8", "code": ""}},
			map[string]any{"path": "a/y.sql", "route": "tsql", "state": "COMPLETED", "size": 3, "sha256": fxExe, "encoding": map[string]any{"assessment": "BLOCKED", "encoding": "", "code": "AMBIGUOUS_ENCODING"}},
			map[string]any{"path": "a/k.pfx", "route": "", "state": "PRESENCE_ONLY", "size": 9},
			map[string]any{"path": "a/n.txt", "route": "", "state": "COMPLETED", "size": 1, "sha256": fxExe, "encoding": map[string]any{"assessment": "PASS", "encoding": "UTF-8", "code": ""}},
		}}
	f := newFxNative()
	input := map[string]any{"path": "a/x.sql", "role": "case", "sha256": sum([]byte(src)), "bytes": len(src)}
	f.profile["id"], f.profile["route"], f.profile["operation"], f.profile["output"] = "s05-corpus-tsql", "tsql", "private-corpus-local", "record"
	f.profile["cases"] = []any{map[string]any{"id": "f000001", "encoding": "UTF-8", "input": input, "edits": []any{}, "points": []any{}, "expect": []any{}}}
	recTree := map[string]any{"status": "COMPLETED", "code": "", "form": "record", "parse_ms": 1, "descendant_count": 4, "max_depth": 2, "has_error": true, "digest": fxExe, "summary": nil}
	f.cases = []map[string]any{{"id": "f000001", "input": input, "encoding": "UTF-8", "execution_status": "COMPLETED", "assessment": "PASS", "code": "",
		"claims": map[string]string{"incremental_equality": "NOT_CLAIMED", "incremental_route": "NOT_CLAIMED", "expectations": "NOT_CLAIMED"},
		"steps":  []any{map[string]any{"step": 0, "source_bytes": len(src), "source_sha256": sum([]byte(src)), "edit": nil, "route": nil, "comparison": nil, "incremental": recTree, "fresh": nil}}, "expectations": []any{}}}
	f.top["summary"] = map[string]any{"cases": 1, "execution_statuses": map[string]int{"COMPLETED": 1}, "assessments": map[string]int{"PASS": 1}, "codes": map[string]int{}, "has_error": 1}
	nroot, _ := f.write(t, nil)
	pdata, _ := os.ReadFile(filepath.Join(nroot, "workload.json"))
	rdata, _ := os.ReadFile(filepath.Join(nroot, "result.json"))
	tr, form, d, n := true, "record", fxExe, uint64(4)
	proj := []map[string]any{
		{"id": "f000001", "path": "a/x.sql", "route": "tsql", "size": len(src), "execution_status": "COMPLETED", "assessment": "PASS", "code": "", "has_error": tr, "descendant_count": n, "digest": d, "form": form},
		{"id": "f000002", "path": "a/y.sql", "route": "tsql", "size": 3, "execution_status": "NOT_RUN", "assessment": "BLOCKED", "code": "AMBIGUOUS_ENCODING", "has_error": nil, "descendant_count": nil, "digest": nil, "form": nil},
	}
	sm := map[string]any{"schema": "tsgk-s05-private-corpus-run/r1", "corpus": "NET461-PHASE2-LOCAL-r1", "operation": "private-corpus-local",
		"candidate_commit": strings.Repeat("a", 40), "clean_tree": true, "dirty_entries": 0, "host": map[string]any{"os": "Microsoft Windows NT 10.0", "arch": "X64"},
		"tools":           map[string]any{"compiler_sha256": fxCompiler, "runtime_commit": fxRuntime},
		"inventory":       map[string]any{"records": 4, "routed_non_presence": 2, "presence_only": 1, "unrouted": 2, "encoding_codes": map[string]int{"AMBIGUOUS_ENCODING": 1}},
		"by_route_status": map[string]int{"tsql:COMPLETED:PASS": 1, "tsql:NOT_RUN:BLOCKED": 1}, "has_error": map[string]int{"tsql:True": 1, "null": 1},
		"codes": map[string]int{"null": 1, "tsql:AMBIGUOUS_ENCODING": 1}, "forms": map[string]int{"tsql:record": 1, "null": 1}, "not_run": 0,
		"runs": []any{map[string]any{"group": "tsql", "execution_status": "COMPLETED", "assessment": "PASS", "cases": 1}}}
	files := map[string][]byte{"profile-tsql.json": pdata, "result-tsql/result.json": rdata}
	if mutate != nil {
		mutate(files, sm, proj)
	}
	invb, _ := json.Marshal(inv)
	smb, _ := json.Marshal(sm)
	var lines []byte
	for _, p := range proj {
		b, _ := json.Marshal(p)
		lines = append(append(lines, b...), '\n')
	}
	files["inventory.json"], files["summary.json"], files["records.jsonl"] = invb, smb, lines
	root := t.TempDir()
	ms := writeSet(t, root, files, func(p string) string {
		return map[string]string{"inventory.json": "inventory", "summary.json": "summary", "records.jsonl": "projection", "profile-tsql.json": "workload-profile:tsql", "result-tsql/result.json": "result:tsql"}[p]
	})
	ids := map[string]string{"commit": strings.Repeat("a", 40), "clean_tree": "true", "host": "Microsoft Windows NT 10.0/X64", "compiler": fxCompiler, "runtime": fxRuntime,
		"tsql:workload": sum(pdata), "tsql:policy": fxPolicy, "tsql:operation": "native-parse-edit", "tsql:platform": "windows/amd64", "tsql:producer": fxProducer,
		"tsql:executable": fxExe, "tsql:compiler": fxCompiler, "tsql:runtime": fxRuntime}
	return root, registration("private-corpus-r1", "private-corpus-replay", "PASS", ids, nil, ms)
}

// S07-A15: the private corpus workload binds every routed inventory record to exactly one
// route case, recomputes the projection and the counts, checks the local-run identity and
// reports ordinals, never paths.
func TestReplayPrivateCorpus(t *testing.T) {
	root, reg := fxPrivate(t, nil)
	r := runReg(t, root, reg)
	if !r.EvidenceValid || r.Assessment != AssessPass {
		t.Fatalf("private corpus: %s %v %+v", r.Assessment, r.Findings, r.Gates)
	}
	if g := gateOf(r, "tree"); g.EvidenceMode != ModeRecordedNotRecomputed {
		t.Fatalf("record-form digests are recorded: %+v", g)
	}
	out, _ := json.Marshal(r)
	if strings.Contains(string(out), "a/x.sql") || strings.Contains(string(out), "f000001") {
		t.Fatalf("private names leaked into the result: %s", out)
	}
	for name, m := range map[string]func(map[string][]byte, map[string]any, []map[string]any){
		"LOCAL_RUN_DIRTY": func(_ map[string][]byte, s map[string]any, _ []map[string]any) {
			s["clean_tree"], s["dirty_entries"] = false, 2
		},
		"SUMMARY_MISMATCH": func(_ map[string][]byte, s map[string]any, _ []map[string]any) {
			s["has_error"] = map[string]int{"tsql:False": 1, "null": 1}
		},
		"PROJECTION_MISMATCH":    func(_ map[string][]byte, _ map[string]any, p []map[string]any) { p[0]["has_error"] = false },
		"CONSUMPTION_INCOMPLETE": func(_ map[string][]byte, _ map[string]any, p []map[string]any) { p[1]["id"] = "f000009" },
	} {
		root, reg := fxPrivate(t, m)
		r := runReg(t, root, reg)
		found := slices.Contains(codes(r.Findings), name)
		for _, g := range r.Gates {
			found = found || g.Code == name
		}
		if r.EvidenceValid || !found {
			t.Fatalf("%s: %v %+v", name, r.Findings, r.Gates)
		}
	}
	// a route profile that registers a file the inventory does not route
	root, reg = fxPrivate(t, nil)
	reg["operation"] = "evidence-replay"
	data, _ := json.Marshal(reg)
	if _, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: data}); err == nil {
		t.Fatal("private corpus replay accepted the public evidence-replay operation")
	}
}

// probe output lines in the PREPARE probe's serialization
func probeTree(kind string, end int, err bool) string {
	return fmt.Sprintf(`{"kind":%q,"field":"","start":0,"end":%d,"start_point":[0,0],"end_point":[0,%d],"named":true,"missing":false,"error":%v,"extra":false,"children":[]}`, kind, end, end, err)
}

func fxPrepare(t *testing.T, mutate func(rows []map[string]any, raw map[string]string)) (string, map[string]any) {
	line := func(stage string, hasErr bool, tree string) string {
		return fmt.Sprintf(`{"stage":%q,"has_error":%v,"tree":%s}`, stage, hasErr, tree)
	}
	ok, bad := probeTree("program", 5, false), probeTree("ERROR", 4, true)
	raw := map[string]string{
		"raw/case-p-a.stdout": line("original", false, ok) + "\n",
		"raw/case-p-b.stdout": strings.Join([]string{line("original", false, ok), line("damaged-incremental", true, bad), line("damaged-fresh", true, bad),
			`{"comparison":"damaged","equal":true}`, line("restored-incremental", false, ok), line("restored-fresh", false, ok), `{"comparison":"restored","equal":true}`}, "\n") + "\n",
		"raw/case-p-n.stdout": line("original", true, bad) + "\n",
		"raw/case-q-a.stdout": line("original", false, ok) + "\n",
	}
	row := func(id, producer, rawPath, syntax string, exit int, edit bool) map[string]any {
		c := map[string]any{"expected": map[string]any{"syntax": syntax}, "edit": nil}
		if edit {
			c["edit"] = map[string]any{"start_byte": 1}
		}
		return map[string]any{"id": id, "producer": producer, "state": "EXITED", "exit_code": exit, "raw_stdout": rawPath, "case": c}
	}
	rows := []map[string]any{
		row("P-A", "p", "raw/case-p-a.stdout", "NO_ERROR_OR_MISSING", 0, false),
		row("P-B", "p", "raw/case-p-b.stdout", "NO_ERROR_OR_MISSING", 0, true),
		row("P-N", "p", "raw/case-p-n.stdout", "ERROR_OR_MISSING_REQUIRED", 2, false),
		row("P-A", "q", "raw/case-q-a.stdout", "NO_ERROR_OR_MISSING", 0, false),
	}
	if mutate != nil {
		mutate(rows, raw)
	}
	led, _ := json.Marshal(map[string]any{"rows": rows})
	files := map[string][]byte{"records/case-ledger.json": led}
	for k, v := range raw {
		files[k] = []byte(v)
	}
	var inv []map[string]any
	for _, p := range sortedKeys(files) {
		inv = append(inv, map[string]any{"path": p, "bytes": len(files[p]), "sha256": sum(files[p])})
	}
	invb, _ := json.Marshal(inv)
	root := t.TempDir()
	all := map[string][]byte{"evidence-manifest.json": invb, "records/case-ledger.json": led}
	ms := writeSet(t, root, all, func(p string) string {
		return map[string]string{"evidence-manifest.json": "inventory", "records/case-ledger.json": "ledger"}[p]
	})
	writeSet(t, root, files, func(string) string { return "" })
	return root, registration("prepare-native-r1", "evidence-replay", "PASS", map[string]string{"producer": "p", "inventory": sum(invb)}, []string{"P-A", "P-B", "P-N"}, ms)
}

// S07-A16: PREPARE probe evidence replays the probe's own decision from its printed trees
// for one producer's registered rows; the review helper's fact checks stay recorded.
func TestReplayPrepareNative(t *testing.T) {
	root, reg := fxPrepare(t, nil)
	r := runReg(t, root, reg)
	if !r.EvidenceValid || r.Assessment != AssessUnresolved || r.Consumption.Consumed != 3 || r.Consumption.Excluded != 1 {
		t.Fatalf("prepare: %s %v %+v %+v", r.Assessment, r.Findings, r.Consumption, r.Gates)
	}
	if g := gateOf(r, "inventory"); g.EvidenceMode != ModeRecordedNotRecomputed || g.Failed != 0 {
		t.Fatalf("inventory entries are checked by size only: %+v", g)
	}
	for _, id := range []string{"exit", "syntax", "raw-binding", "ledger-binding"} {
		if g := gateOf(r, id); g.EvidenceMode != ModeReplayedRaw || g.Failed != 0 {
			t.Fatalf("gate %s %+v", id, g)
		}
	}
	if g := gateOf(r, "fact-checks"); g.EvidenceMode != ModeRecordedNotRecomputed {
		t.Fatalf("fact checks %+v", g)
	}
	// a registered row whose raw is absent stays recorded, never a pass or a failure
	root, reg = fxPrepare(t, nil)
	os.Remove(filepath.Join(root, "raw", "case-p-n.stdout"))
	r = runReg(t, root, reg)
	if !r.EvidenceValid || r.Assessment != AssessUnresolved || gateOf(r, "raw-binding").NotRecomputed != 1 || gateOf(r, "exit").EvidenceMode != ModeRecordedNotRecomputed {
		t.Fatalf("absent raw: %s %v %+v", r.Assessment, r.Findings, r.Gates)
	}
	// review r2 N4: with no registered raw present nothing is recomputed: recorded
	root, reg = fxPrepare(t, nil)
	for _, n := range []string{"case-p-a.stdout", "case-p-b.stdout", "case-p-n.stdout"} {
		os.Remove(filepath.Join(root, "raw", n))
	}
	if r = runReg(t, root, reg); r.EvidenceMode != ModeRecordedNotRecomputed || r.Assessment != AssessUnresolved {
		t.Fatalf("all raw absent: %s %s %v", r.EvidenceMode, r.Assessment, r.Findings)
	}
	for want, m := range map[string]func([]map[string]any, map[string]string){
		"EXIT_MISMATCH": func(rows []map[string]any, _ map[string]string) { rows[2]["exit_code"] = 0 },
		"SYNTAX_MISMATCH": func(rows []map[string]any, _ map[string]string) {
			rows[0]["case"].(map[string]any)["expected"].(map[string]any)["syntax"] = "ERROR_OR_MISSING_REQUIRED"
		},
		"RAW_MALFORMED": func(_ []map[string]any, raw map[string]string) {
			raw["raw/case-p-b.stdout"] = strings.Replace(raw["raw/case-p-b.stdout"], `"equal":true`, `"equal":false`, 1)
		},
		"CONSUMPTION_INCOMPLETE": func(rows []map[string]any, _ map[string]string) { rows[1]["id"] = "P-X" },
	} {
		root, reg := fxPrepare(t, m)
		r := runReg(t, root, reg)
		found := slices.Contains(codes(r.Findings), want)
		for _, g := range r.Gates {
			found = found || g.Code == want
		}
		if r.EvidenceValid || !found {
			t.Fatalf("%s: %v %+v", want, r.Findings, r.Gates)
		}
	}
}
