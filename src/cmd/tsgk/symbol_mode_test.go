package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// #218 (218-B): the C# symbol mode end to end on the registered grammar. The CLI refuses the
// mode without the logger observation, runs it with the parse of the selected section only
// (an edit that turns the branch is recomputed), stops a zero-active case before any driver
// runs, and native-result-r5 replays the result from the original source witnesses; a
// changed witness, observation, policy, stopped outcome or reducer is rejected.
func TestIncrementalCLISymbolMode(t *testing.T) {
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("native tools required")
		}
		t.Skip("native tools not configured")
	}
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered C# prepared closure not configured")
	}
	base := t.TempDir()
	root := filepath.Join(base, "root")
	symbol, grammar := csharpClosure(t, root)
	one, zero := "#if A\nclass X {}\n#else\nclass Y {}\n#endif\nclass Z {}\n", "#if B\nclass Q {}\n#endif\n"
	if err := os.MkdirAll(filepath.Join(root, "cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{"one": one, "zero": zero} {
		if err := os.WriteFile(filepath.Join(root, "cases", name+".cs"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ccData, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.StdEncoding.EncodeToString
	step := func(k int) map[string]any {
		return map[string]any{"step": k, "syntax": "NO_ERROR", "contains": []any{}, "declarations": ""}
	}
	profile := map[string]any{
		"schema": kit.IncrementalPreprocessSchema, "id": "cli-symbols", "route": "csharp", "operation": "native-parse-edit", "symbol": symbol,
		"encoding": "UTF-8", "output": "tree", "compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum(ccData), "bytes": len(ccData)},
		"grammar": grammar, "declarations": nil, "preprocess": map[string]any{"mode": kit.PreprocessMode, "symbols": []any{"A"}},
		"cases": []any{
			map[string]any{"id": "one", "input": fileEntry(t, root, "cases/one.cs", "case"), "points": []any{}, "expect": []any{step(0), step(1)},
				"edits": []any{map[string]any{"start_byte": 4, "old_end_byte": 5, "new_end_byte": 5, "old": enc([]byte("A")), "new": enc([]byte("B"))}}},
			map[string]any{"id": "zero", "input": fileEntry(t, root, "cases/zero.cs", "case"), "points": []any{}, "expect": []any{}, "edits": []any{}},
		},
	}
	pdata, _ := json.Marshal(profile)
	pf := filepath.Join(base, "profile.json")
	if err := os.WriteFile(pf, pdata, 0o644); err != nil {
		t.Fatal(err)
	}
	work, out := filepath.Join(base, "work"), filepath.Join(base, "out")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"incremental", "--root", root, "--profile", pf, "--runtime", rt, "--tool", "cc=" + cc, "--work", work, "--out", out, "--allow", "BUILD_NATIVE", "--allow", "EXEC_NATIVE"}
	if cg := os.Getenv("TSGK_CGROUP_PARENT"); cg != "" {
		args = append(args, "--cgroup-parent", cg)
	}
	ctx := context.Background()
	if code, _, stderr := cli(t, ctx, args...); code == 0 || !strings.Contains(stderr, "PREPROCESS_OBSERVATION_REQUIRED") {
		t.Fatalf("without --reuse-observation: %d %s", code, stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("a refused run must not create --out")
	}
	cli(t, ctx, append(args, "--reuse-observation")...) // the zero-active case blocks the run; the result is read below
	published, err := os.ReadFile(filepath.Join(out, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		ResultSchema    string            `json:"result_schema"`
		ExecutionStatus string            `json:"execution_status"`
		Assessment      string            `json:"assessment"`
		Platform        string            `json:"platform"`
		Identities      []kit.IdentityRef `json:"identities"`
		Build           struct {
			Identity         string `json:"identity"`
			ExecutableSHA256 string `json:"executable_sha256"`
			RuntimeCommit    string `json:"runtime_commit"`
			Compiler         struct {
				SHA256 string `json:"sha256"`
			} `json:"compiler"`
		} `json:"build"`
		Cases []struct {
			ExecutionStatus string               `json:"execution_status"`
			Code            string               `json:"code"`
			Preprocess      []kit.PreprocessStep `json:"preprocess"`
			Steps           []struct {
				Incremental struct {
					Tree struct {
						Nodes []kit.TreeNode `json:"nodes"`
					} `json:"tree"`
				} `json:"incremental"`
			} `json:"steps"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(published, &res); err != nil || res.ResultSchema != "tsgk-incremental-result/r4" || len(res.Cases) != 2 {
		t.Fatalf("result: %v %s", err, published[:min(len(published), 300)])
	}
	c0, c1 := res.Cases[0], res.Cases[1]
	if c0.ExecutionStatus != kit.StatusCompleted || len(c0.Steps) != 2 || len(c0.Preprocess) != 2 || c1.ExecutionStatus != kit.StatusNotRun || c1.Code != "PREPROCESS_ZERO_ACTIVE" || len(c1.Steps) != 0 {
		t.Fatalf("cases: %+v %+v", c0.ExecutionStatus, c1)
	}
	inside := func(nodes []kit.TreeNode, src, word string) bool {
		at := strings.Index(src, word)
		for _, n := range nodes {
			if n.Parent >= 0 && int(n.StartByte) >= at && int(n.EndByte) <= at+len(word) && n.EndByte > n.StartByte {
				return true
			}
		}
		return false
	}
	after := strings.Replace(one, "#if A", "#if B", 1)
	if !inside(c0.Steps[0].Incremental.Tree.Nodes, one, "class X") || inside(c0.Steps[0].Incremental.Tree.Nodes, one, "class Y") ||
		!inside(c0.Steps[1].Incremental.Tree.Nodes, after, "class Y") || inside(c0.Steps[1].Incremental.Tree.Nodes, after, "class X") {
		t.Fatal("the parsed section does not follow the symbols and the edit")
	}
	// offline replay with the original source witnesses
	if err := os.WriteFile(filepath.Join(out, "workload.json"), pdata, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(out, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	witness := func(name, src string) {
		if err := os.WriteFile(filepath.Join(out, "sources", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	witness("00000-one.bin", one)
	witness("00001-zero.bin", zero)
	policy := ""
	for _, id := range res.Identities {
		if id.Role == "policy" {
			policy = id.SHA256
		}
	}
	replay := func(reducer string, skip string) (kit.ReplayResult, error) {
		t.Helper()
		items := []struct{ path, role string }{{"workload.json", "workload-profile"}, {"result.json", "result"}, {"sources/00000-one.bin", "case-source"}, {"sources/00001-zero.bin", "case-source"}}
		entries, err := os.ReadDir(filepath.Join(out, "responses"))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			items = append(items, struct{ path, role string }{"responses/" + entry.Name(), "response"})
		}
		var members []any
		for _, item := range items {
			if item.path != skip {
				members = append(members, fileEntry(t, out, item.path, item.role))
			}
		}
		reg, _ := json.Marshal(map[string]any{"schema": kit.ReplaySchema, "id": "cli-symbol-replay", "reducer": reducer, "operation": "evidence-replay",
			"subject":    map[string]any{"run": "cli-test", "attempt": 1, "platform": res.Platform, "commit": "fixture", "evidence_mode": "NEW_RUN", "execution_status": res.ExecutionStatus, "assessment": res.Assessment},
			"identities": map[string]string{"workload": sum(pdata), "policy": policy, "operation": "native-parse-edit", "platform": res.Platform, "producer": res.Build.Identity, "executable": res.Build.ExecutableSHA256, "compiler": res.Build.Compiler.SHA256, "runtime": res.Build.RuntimeCommit},
			"records":    nil, "members": members})
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		return kit.Replay(rctx, kit.ReplayRequest{Root: out, Profile: reg})
	}
	r, err := replay("native-result-r5", "")
	if err != nil || !r.EvidenceValid {
		t.Fatalf("replay: %v valid=%v %+v findings=%+v %v", err, r.EvidenceValid, r.Gates, r.Findings, r.Explanation)
	}
	pre := false
	for _, g := range r.Gates {
		if g.Failed != 0 {
			t.Fatalf("replay gate failed: %+v", g)
		}
		pre = pre || g.ID == "preprocess" && g.Recomputed > 0
	}
	if !pre {
		t.Fatal("the preprocess gate did not recompute")
	}
	rejected := func(name, code string, r kit.ReplayResult, err error) {
		t.Helper()
		var ke *kit.Error
		if errors.As(err, &ke) && ke.Code == code {
			return
		}
		for _, g := range r.Gates {
			if g.Code == code {
				return
			}
		}
		t.Errorf("%s: not rejected with %s: %v %+v", name, code, err, r.Gates)
	}
	r, err = replay("native-result-r4", "")
	rejected("r4 reducer", "PREPROCESS_REDUCER_MISMATCH", r, err)
	r, err = replay("native-result-r5", "sources/00000-one.bin")
	rejected("missing witness", "PREPROCESS_SOURCE_MISSING", r, err)
	witness("00000-one.bin", strings.Replace(one, "X", "W", 1))
	r, err = replay("native-result-r5", "")
	rejected("changed witness", "PREPROCESS_SOURCE_MISMATCH", r, err)
	witness("00000-one.bin", one)
	tamper := func(name, code string, change func(doc map[string]any)) {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(published, &doc); err != nil {
			t.Fatal(err)
		}
		change(doc)
		data, _ := json.Marshal(doc)
		if err := os.WriteFile(filepath.Join(out, "result.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(filepath.Join(out, "result.json"), published, 0o644)
		r, err := replay("native-result-r5", "")
		rejected(name, code, r, err)
	}
	cases := func(doc map[string]any) []any { return doc["cases"].([]any) }
	tamper("observation", "PREPROCESS_MISMATCH", func(doc map[string]any) {
		s := cases(doc)[0].(map[string]any)["preprocess"].([]any)[0].(map[string]any)
		s["directives"].([]any)[0].(map[string]any)["taken"] = false
	})
	tamper("stopped outcome", "PREPROCESS_OUTCOME_MISMATCH", func(doc map[string]any) {
		cases(doc)[1].(map[string]any)["assessment"] = kit.AssessNotAssessed
	})
	tamper("missing observation", "PREPROCESS_MISSING", func(doc map[string]any) {
		delete(cases(doc)[1].(map[string]any), "preprocess")
	})
	tamper("policy", "PREPROCESS_POLICY_MISMATCH", func(doc map[string]any) {
		op := kit.NativeOperations()["native-parse-edit"]
		r1 := kit.NativePolicy(op, "tree", nil)
		for _, id := range doc["identities"].([]any) {
			if m := id.(map[string]any); m["role"] == "policy" {
				m["schema"], m["sha256"] = r1.Schema, r1.SHA256
			}
		}
	})
}
