package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func fileEntry(t *testing.T, root, rel, role string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"path": rel, "role": role, "sha256": sum(data), "bytes": len(data)}
}

// S05: the CLI builds the driver for a profile, runs every case through the runner,
// publishes result.json and the raw responses without clobbering, and refuses before
// any effect without the capabilities.
func TestIncrementalCLI(t *testing.T) {
	for _, observe := range []bool{false, true} {
		t.Run(fmt.Sprint(observe), func(t *testing.T) { testIncrementalCLI(t, observe, "tree") })
	}
	t.Run("batch-observation", func(t *testing.T) { testIncrementalCLI(t, true, "batch") })
	t.Run("svc-observation", func(t *testing.T) { testIncrementalCLI(t, true, "svc") })
	t.Run("build-cache", func(t *testing.T) { testIncrementalCLI(t, false, "cache") })
	t.Run("source-witness", func(t *testing.T) { testIncrementalCLI(t, false, "source-witness") })
}

func testIncrementalCLI(t *testing.T, observe bool, mode string) {
	batch, svc := mode == "batch", mode == "svc"
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("native tools required")
		}
		t.Skip("native tools not configured")
	}
	// Runtime-only foundation has no registered grammars; route CI requires the closure.
	if svc && os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered C# prepared closure not configured")
	}
	base := t.TempDir()
	root := filepath.Join(base, "root with space")
	src := filepath.Join(repoRoot(t), "src", "testdata", "native", "plain", "src")
	for _, f := range []string{"parser.c", "tree_sitter/parser.h", "tree_sitter/alloc.h", "tree_sitter/array.h"} {
		data, _ := os.ReadFile(filepath.Join(src, filepath.FromSlash(f)))
		os.MkdirAll(filepath.Dir(filepath.Join(root, "src", filepath.FromSlash(f))), 0o755)
		os.WriteFile(filepath.Join(root, "src", filepath.FromSlash(f)), data, 0o644)
	}
	os.MkdirAll(filepath.Join(root, "cases"), 0o755)
	source := "a = f(1, [2, 3]);\n{ b = \"s\"; }\n"
	if svc {
		source = "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\nclass S { int X = 1; }"
	}
	os.WriteFile(filepath.Join(root, "cases", "one.txt"), []byte(source), 0o644)
	ccData, _ := os.ReadFile(cc)
	enc := base64.StdEncoding.EncodeToString
	edit := map[string]any{"start_byte": 4, "old_end_byte": 5, "new_end_byte": 5, "old": enc([]byte("f")), "new": enc([]byte("g"))}
	profile := map[string]any{
		"schema": "tsgk-incremental/r2", "id": "cli-plain", "route": "owned-plain", "operation": "native-parse-edit", "symbol": "tree_sitter_tsgk_plain",
		"encoding": "UTF-8", "output": "tree", "compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum(ccData), "bytes": len(ccData)},
		"grammar": []any{fileEntry(t, root, "src/parser.c", "parser"), fileEntry(t, root, "src/tree_sitter/alloc.h", "header"),
			fileEntry(t, root, "src/tree_sitter/array.h", "header"), fileEntry(t, root, "src/tree_sitter/parser.h", "header")},
		"declarations": map[string]any{"mapping": "owned-r1", "items": []any{map[string]any{"fact": "type_declaration", "node": "assignment", "name": "field:name"}}},
		"cases": []any{map[string]any{"id": "one", "input": fileEntry(t, root, "cases/one.txt", "case"), "edits": []any{edit}, "points": []any{},
			"expect": []any{map[string]any{"step": 1, "syntax": "NO_ERROR", "contains": []any{"call"}, "declarations": "PASS"}}}},
	}
	if batch {
		profile["operation"], profile["output"] = "private-corpus-local", "record"
		c := profile["cases"].([]any)[0].(map[string]any)
		c["edits"], c["expect"] = []any{}, []any{}
	}
	if svc {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), "src", "contracts", "native-routes.json"))
		if err != nil {
			t.Fatal(err)
		}
		var registry struct {
			Routes []struct {
				Route, Symbol string
				Files         []kit.NativeInput
			}
		}
		if err := json.Unmarshal(data, &registry); err != nil {
			t.Fatal(err)
		}
		var grammar []any
		for _, route := range registry.Routes {
			if route.Route != "csharp" {
				continue
			}
			profile["symbol"] = route.Symbol
			for _, file := range route.Files {
				data, err := os.ReadFile(filepath.Join(os.Getenv("TSGK_BENCH_PREPARED"), "routes", "csharp", filepath.FromSlash(file.Path)))
				if err != nil || sum(data) != file.SHA256 || uint64(len(data)) != file.Bytes {
					t.Fatalf("registered C# source mismatch: %s %v", file.Path, err)
				}
				path := filepath.Join(root, filepath.FromSlash(file.Path))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
				grammar = append(grammar, fileEntry(t, root, file.Path, file.Role))
			}
		}
		if len(grammar) == 0 {
			t.Fatal("registered C# closure missing")
		}
		profile["grammar"] = grammar
		profile["declarations"] = nil
		profile["format"] = kit.SvcFormat
		c := profile["cases"].([]any)[0].(map[string]any)
		c["svc_source"] = enc([]byte(source))
		end := strings.Index(source, "%>")
		restore := strings.Index(source, `Service="S" `)
		c["edits"] = []any{
			map[string]any{"start_byte": end, "old_end_byte": end + 2, "new_end_byte": end, "old": enc([]byte("%>")), "new": ""},
			map[string]any{"start_byte": restore, "old_end_byte": restore + len(`Service="S" `), "new_end_byte": restore + len(`Service="S" %>`), "old": enc([]byte(`Service="S" `)), "new": enc([]byte(`Service="S" %>`))},
		}
		c["expect"] = []any{map[string]any{"step": 0, "syntax": "NO_ERROR", "contains": []any{}, "declarations": ""}, map[string]any{"step": 1, "syntax": "ERROR", "contains": []any{}, "declarations": ""}, map[string]any{"step": 2, "syntax": "NO_ERROR", "contains": []any{}, "declarations": ""}}
	}
	if mode == "source-witness" {
		profile["schema"], profile["operation"] = kit.OracleSchema, "native-query"
		profile["queries"], profile["fact_pack"], profile["api"] = []any{}, nil, false
		c := profile["cases"].([]any)[0].(map[string]any)
		c["query_expect"], c["dynamic_sql_expect"] = []any{}, nil
	}
	data, _ := json.Marshal(profile)
	pf := filepath.Join(base, "profile.json")
	os.WriteFile(pf, data, 0o644)
	work := filepath.Join(base, "work")
	os.MkdirAll(work, 0o755)
	out := filepath.Join(base, "out")
	args := []string{"incremental", "--root", root, "--profile", pf, "--runtime", rt, "--tool", "cc=" + cc, "--work", work, "--out", out}
	if mode == "source-witness" {
		args = append([]string{"oracle", "record", "--source-witness"}, args[1:]...)
	}
	if mode == "cache" {
		cache := filepath.Join(base, "run-cache")
		if err := os.Mkdir(cache, 0o700); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--build-cache", cache)
	}
	if observe {
		args = append(args, "--reuse-observation")
	}
	ctx := context.Background()
	if code, _, stderr := cli(t, ctx, args...); code != 3 || !strings.Contains(stderr, "CAPABILITY_NOT_GRANTED") {
		t.Fatalf("without --allow: %d %s", code, stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("a refused run must not create --out")
	}
	args = append(args, "--allow", "BUILD_NATIVE", "--allow", "EXEC_NATIVE")
	if cg := os.Getenv("TSGK_CGROUP_PARENT"); cg != "" {
		args = append(args, "--cgroup-parent", cg)
	}
	code, stdout, stderr := cli(t, ctx, args...)
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stderr, stdout)
	}
	if mode == "source-witness" {
		var result native.OracleResult
		if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.Set == nil || !result.Set.Valid {
			t.Fatalf("source witness CLI: %v %s", err, stdout)
		}
		got, err := os.ReadFile(filepath.Join(out, "source", "00000-one.bin"))
		if err != nil || string(got) != source {
			t.Fatalf("CLI did not retain original source: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
		var m kit.OracleManifest
		if err != nil || json.Unmarshal(data, &m) != nil || m.Schema != kit.OracleSourceManifestSchema {
			t.Fatalf("CLI witness manifest: %v", err)
		}
		return
	}
	var res struct {
		ExecutionStatus string `json:"execution_status"`
		Assessment      string `json:"assessment"`
		BuildRemoved    string `json:"build_removed"`
		Cases           []struct {
			Claims map[string]string `json:"claims"`
		} `json:"cases"`
	}
	published, err := os.ReadFile(filepath.Join(out, "result.json"))
	wantRoute := "PASS"
	if batch || svc {
		wantRoute = "NOT_CLAIMED"
	}
	if err != nil || json.Unmarshal(published, &res) != nil || res.Assessment != "PASS" || res.BuildRemoved != "REMOVED" || res.Cases[0].Claims["incremental_route"] != wantRoute {
		t.Fatalf("published result %v %s", err, published[:min(len(published), 400)])
	}
	response := "00000-one.json"
	if svc {
		response = "00000-one-segment-00000.json"
	}
	if _, err := os.Stat(filepath.Join(out, "responses", response)); err != nil {
		t.Fatal("raw response not preserved")
	}
	if observe {
		testCLIReuseReplay(t, out, data, published, []byte(source))
	}
	if mode == "cache" {
		var cold, warm native.Result
		if err := json.Unmarshal(published, &cold); err != nil || cold.Build == nil || cold.Build.Reuse == nil || cold.Build.Reuse.Hit || len(cold.Build.Steps) == 0 {
			t.Fatalf("cold build evidence: %v", err)
		}
		warmArgs := append([]string(nil), args...)
		for i := range warmArgs {
			if warmArgs[i] == "--out" {
				warmArgs[i+1] = filepath.Join(base, "warm-output")
			}
		}
		code, text, stderr := cli(t, ctx, warmArgs...)
		if code != 0 || json.Unmarshal([]byte(text), &warm) != nil || warm.Build == nil || warm.Build.Reuse == nil || !warm.Build.Reuse.Hit || len(warm.Build.Steps) != 0 ||
			warm.Build.Identity != cold.Build.Identity || warm.Build.ExecutableSHA256 != cold.Build.ExecutableSHA256 || warm.EvidenceMode != kit.ModeNewRun || warm.BuildRemoved != "REMOVED" ||
			len(warm.Cases) != 1 || warm.Cases[0].Process == nil || warm.Cases[0].Steps[0].Incremental.Digest != cold.Cases[0].Steps[0].Incremental.Digest {
			t.Fatalf("cross-process warm build: exit=%d %s", code, stderr)
		}
		// Execute another real cached run; a successful saved result cannot hide stdout failure.
		closedArgs := append([]string(nil), warmArgs...)
		closedOut := filepath.Join(base, "closed-stdout-output")
		for i := range closedArgs {
			if closedArgs[i] == "--out" {
				closedArgs[i+1] = closedOut
			}
		}
		closed, err := os.CreateTemp(t.TempDir(), "closed-stdout")
		if err != nil {
			t.Fatal(err)
		}
		if err := closed.Close(); err != nil {
			t.Fatal(err)
		}
		var closedStderr strings.Builder
		if code := run(ctx, closedArgs, closed, &closedStderr); code != exitIO || !strings.Contains(closedStderr.String(), "OUTPUT_WRITE_FAILED") {
			t.Fatalf("successful native run hid stdout failure: %d %s", code, closedStderr.String())
		}
		savedBytes, err := os.ReadFile(filepath.Join(closedOut, "result.json"))
		var saved native.Result
		if err != nil || json.Unmarshal(savedBytes, &saved) != nil || saved.ExecutionStatus != kit.StatusCompleted || saved.Assessment != kit.AssessPass ||
			saved.Build == nil || saved.Build.Reuse == nil || !saved.Build.Reuse.Hit || len(saved.Cases) != 1 || saved.Cases[0].Process == nil {
			t.Fatalf("stdout failure did not follow successful native execution: %v", err)
		}
		// Cache is another execution destination: it cannot be an input directory.
		for i := range warmArgs {
			if warmArgs[i] == "--build-cache" {
				warmArgs[i+1] = root
			}
		}
		if code, _, stderr := cli(t, ctx, warmArgs...); code != 2 || !strings.Contains(stderr, "CACHE_INSIDE_INPUT") {
			t.Fatalf("cache inside input: %d %s", code, stderr)
		}
	}
	if entries, _ := os.ReadDir(work); len(entries) != 0 {
		t.Fatalf("build directory left behind: %v", entries)
	}
	if code, _, stderr := cli(t, ctx, args...); code != 2 || !strings.Contains(stderr, "OUTPUT_EXISTS") {
		t.Fatalf("existing --out: %d %s", code, stderr)
	}
	// A changed case input is refused per case before its process starts.
	os.WriteFile(filepath.Join(root, "cases", "one.txt"), []byte(source+" "), 0o644)
	for i := range args {
		if args[i] == "--out" {
			args[i+1] = filepath.Join(base, "out2")
		}
	}
	code, stdout, _ = cli(t, ctx, args...)
	if code == 0 || !strings.Contains(stdout, `"code":"SOURCE_MISMATCH"`) {
		t.Fatalf("changed input: %d %s", code, fmt.Sprint(len(stdout)))
	}
}

func testCLIReuseReplay(t *testing.T, out string, profile, published, source []byte) {
	t.Helper()
	prof, err := kit.ParseIncrementalProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		ResultSchema           string            `json:"result_schema"`
		ObservationRunID       string            `json:"observation_run_id"`
		ReuseObservationPolicy string            `json:"reuse_observation_policy"`
		Platform               string            `json:"platform"`
		Identities             []kit.IdentityRef `json:"identities"`
		Build                  struct {
			Identity         string `json:"identity"`
			ExecutableSHA256 string `json:"executable_sha256"`
			RuntimeCommit    string `json:"runtime_commit"`
			Compiler         struct {
				SHA256 string `json:"sha256"`
			} `json:"compiler"`
		} `json:"build"`
		Cases []struct {
			ReuseObservationClaim string `json:"reuse_observation_claim"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(published, &top); err != nil {
		t.Fatal(err)
	}
	if top.ResultSchema != "tsgk-incremental-result/r3" || top.ObservationRunID == "" || top.ReuseObservationPolicy != "tsgk-reuse-log/r1" || top.Cases[0].ReuseObservationClaim != "PASS" {
		t.Fatalf("observation stdout contract: %+v", top)
	}
	if err := os.WriteFile(filepath.Join(out, "workload.json"), profile, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(out, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "sources", "00000-one.bin"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	var members []any
	items := []struct{ path, role string }{{"workload.json", "workload-profile"}, {"result.json", "result"}, {"sources/00000-one.bin", "case-source"}}
	responses, err := os.ReadDir(filepath.Join(out, "responses"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range responses {
		items = append(items, struct{ path, role string }{"responses/" + entry.Name(), "response"})
	}
	for _, item := range items {
		entry := fileEntry(t, out, item.path, item.role)
		members = append(members, entry)
	}
	policy := ""
	for _, id := range top.Identities {
		if id.Role == "policy" {
			policy = id.SHA256
		}
	}
	reg, err := json.Marshal(map[string]any{"schema": kit.ReplaySchema, "id": "cli-reuse-replay", "reducer": "native-result-r4", "operation": "evidence-replay",
		"subject":    map[string]any{"run": "cli-test", "attempt": 1, "platform": top.Platform, "commit": "fixture", "evidence_mode": "NEW_RUN", "execution_status": "COMPLETED", "assessment": "PASS"},
		"identities": map[string]string{"workload": sum(profile), "policy": policy, "operation": prof.Operation, "platform": top.Platform, "producer": top.Build.Identity, "executable": top.Build.ExecutableSHA256, "compiler": top.Build.Compiler.SHA256, "runtime": top.Build.RuntimeCommit}, "records": nil, "members": members})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := kit.Replay(ctx, kit.ReplayRequest{Root: out, Profile: reg})
	wantAssessment := "UNRESOLVED"
	if prof.Operation == "private-corpus-local" || prof.Format == kit.SvcFormat {
		wantAssessment = "PASS" // these workloads have no declaration expectation
	}
	if err != nil || !r.EvidenceValid || r.Assessment != wantAssessment {
		t.Fatalf("actual CLI offline replay: %v assessment=%s valid=%v", err, r.Assessment, r.EvidenceValid)
	}
	observed := false
	points := false
	for _, g := range r.Gates {
		if g.Failed != 0 {
			t.Fatalf("actual CLI replay gate failed: %+v", g)
		}
		if g.ID == "reuse-observation" {
			observed = g.Assessment == "PASS" && g.NotRecomputed == 0
		}
		if g.ID == "edit-points" {
			points = g.Assessment == "PASS" && g.NotRecomputed == 0
		}
		if prof.Operation != "private-corpus-local" && prof.Format != kit.SvcFormat && g.ID == "expectations" && (g.Assessment != "UNRESOLVED" || g.NotRecomputed != 1) {
			t.Fatal("r1 full-tree declaration limitation was promoted")
		}
	}
	if !observed || prof.Operation != "private-corpus-local" && !points {
		t.Fatal("actual CLI observation was not fully replayed")
	}
	if prof.Format == kit.SvcFormat {
		first, last := "responses/00000-one-segment-00000.json", "responses/00000-one-segment-00002.json"
		a, err := os.ReadFile(filepath.Join(out, first))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(out, last))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, first), b, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, last), a, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, item := range members {
			m := item.(map[string]any)
			if m["path"] == first || m["path"] == last {
				updated := fileEntry(t, out, m["path"].(string), "response")
				m["sha256"], m["bytes"] = updated["sha256"], updated["bytes"]
			}
		}
		var registration map[string]any
		if err := json.Unmarshal(reg, &registration); err != nil {
			t.Fatal(err)
		}
		registration["members"] = members
		reg, err = json.Marshal(registration)
		if err != nil {
			t.Fatal(err)
		}
		changed, err := kit.Replay(ctx, kit.ReplayRequest{Root: out, Profile: reg})
		if err != nil || changed.EvidenceValid {
			t.Fatalf("SVC segment transplant not rejected semantically: %v valid=%v", err, changed.EvidenceValid)
		}
		failed := false
		for _, g := range changed.Gates {
			failed = failed || g.ID == "reuse-observation" && g.Failed > 0
		}
		if !failed {
			t.Fatal("SVC transplant did not reach observation binding gate")
		}
		t.Log("SVC native r3 -> record -> replay PASS; same-input segment transplant detected by observation binding")
	}
}

// #217: oracle record takes the observation flag (protocol r4); alone it is still a usage error.
func TestOracleReuseObservationFlag(t *testing.T) {
	code, _, stderr := cli(t, context.Background(), "oracle", "record", "--reuse-observation")
	if code != 2 || strings.Contains(stderr, "flag provided but not defined") || !strings.Contains(stderr, "[--reuse-observation]") {
		t.Fatalf("oracle flag: %d %s", code, stderr)
	}
}

func TestSourceWitnessFlagScope(t *testing.T) {
	code, _, stderr := cli(t, context.Background(), "incremental", "--source-witness")
	if code != 2 || !strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("incremental accepted source witness: %d %s", code, stderr)
	}
	code, _, stderr = cli(t, context.Background(), "oracle", "record", "--source-witness")
	if code != 2 || strings.Contains(stderr, "flag provided but not defined") || !strings.Contains(stderr, "USAGE") {
		t.Fatalf("oracle source witness flag: %d %s", code, stderr)
	}
}
