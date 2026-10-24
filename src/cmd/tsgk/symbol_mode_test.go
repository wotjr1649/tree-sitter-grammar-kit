package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
// runs, and native-result-r6 replays the result from the original source witnesses; a
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
	one, zero, bad := "#if A\nclass X {}\n#else\nclass Y {}\n#endif\nclass Z {}\n", "#if B\nclass Q {}\n#endif\n", "class B {}\n"
	large, gone := "class L {"+strings.Repeat(" int a;", 3000)+" }\n", "class G {}\n"
	switchOnly := "#if true\nclass X {}\n#else\nclass Y {}\n#endif\n"
	if err := os.MkdirAll(filepath.Join(root, "cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{"one": one, "zero": zero, "bad": bad, "large": large, "gone": gone, "switch": switchOnly} {
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
			// an edit refused before any evaluation (its old bytes are not the source's)
			map[string]any{"id": "bad", "input": fileEntry(t, root, "cases/bad.cs", "case"), "points": []any{}, "expect": []any{},
				"edits": []any{map[string]any{"start_byte": 0, "old_end_byte": 1, "new_end_byte": 0, "old": enc([]byte("x")), "new": ""}}},
			// honest ends the replay must accept: the node limit, and an input gone before the
			// run (its refused edit is never evaluated)
			map[string]any{"id": "large", "input": fileEntry(t, root, "cases/large.cs", "case"), "points": []any{}, "expect": []any{}, "edits": []any{}},
			map[string]any{"id": "gone", "input": fileEntry(t, root, "cases/gone.cs", "case"), "points": []any{}, "expect": []any{},
				"edits": []any{map[string]any{"start_byte": 0, "old_end_byte": 1, "new_end_byte": 0, "old": enc([]byte("x")), "new": ""}}},
			map[string]any{"id": "switch", "input": fileEntry(t, root, "cases/switch.cs", "case"), "points": []any{}, "expect": []any{step(0), step(1)},
				"edits": []any{map[string]any{"start_byte": 4, "old_end_byte": 8, "new_end_byte": 11, "old": enc([]byte("true")), "new": enc([]byte("(false)"))}}},
		},
	}
	pdata, _ := json.Marshal(profile)
	if err := os.Remove(filepath.Join(root, "cases", "gone.cs")); err != nil {
		t.Fatal(err)
	}
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
			ExecutionStatus string `json:"execution_status"`
			Assessment      string `json:"assessment"`
			Claims          struct {
				Equality string `json:"incremental_equality"`
				Route    string `json:"incremental_route"`
			} `json:"claims"`
			Code       string               `json:"code"`
			Preprocess []kit.PreprocessStep `json:"preprocess"`
			Steps      []struct {
				Incremental struct {
					Tree struct {
						Nodes []kit.TreeNode `json:"nodes"`
					} `json:"tree"`
				} `json:"incremental"`
			} `json:"steps"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(published, &res); err != nil || res.ResultSchema != "tsgk-incremental-result/r5" || len(res.Cases) != 6 {
		t.Fatalf("result: %v %s", err, published[:min(len(published), 300)])
	}
	c0, c1, c2 := res.Cases[0], res.Cases[1], res.Cases[2]
	if c0.Assessment != kit.AssessPass || c0.Claims.Route != "PASS" || res.Cases[5].Assessment != kit.AssessBlocked || res.Cases[5].Claims.Equality != "PASS" || res.Cases[5].Claims.Route != "BLOCKED" || res.Cases[5].Code != kit.SelectionRoutePrefix+"1" {
		t.Fatalf("shared trailer=%s/%s; selection switch=%+v", c0.Assessment, c0.Claims.Route, res.Cases[5])
	}
	if c0.ExecutionStatus != kit.StatusCompleted || len(c0.Steps) != 2 || len(c0.Preprocess) != 2 || c1.ExecutionStatus != kit.StatusNotRun || c1.Code != "PREPROCESS_ZERO_ACTIVE" || len(c1.Steps) != 0 ||
		c2.ExecutionStatus != kit.StatusNotRun || c2.Code != "EDIT_OLD_MISMATCH" || c2.Preprocess != nil ||
		res.Cases[3].ExecutionStatus != kit.StatusResourceLimit || res.Cases[3].Code != "NODE_LIMIT" || res.Cases[4].Code != "SOURCE_MISSING" {
		t.Fatalf("cases: %+v %+v %+v %+v %+v", c0.ExecutionStatus, c1, c2, res.Cases[3].Code, res.Cases[4].Code)
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
	witness("00002-bad.bin", bad)
	witness("00003-large.bin", large)
	witness("00004-gone.bin", gone)
	witness("00005-switch.bin", switchOnly)
	policy := ""
	for _, id := range res.Identities {
		if id.Role == "policy" {
			policy = id.SHA256
		}
	}
	replay := func(reducer string, skip string) (kit.ReplayResult, error) {
		t.Helper()
		items := []struct{ path, role string }{{"workload.json", "workload-profile"}, {"result.json", "result"}, {"sources/00000-one.bin", "case-source"}, {"sources/00001-zero.bin", "case-source"}, {"sources/00002-bad.bin", "case-source"},
			{"sources/00003-large.bin", "case-source"}, {"sources/00004-gone.bin", "case-source"}, {"sources/00005-switch.bin", "case-source"}}
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
	r, err := replay("native-result-r6", "")
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
	r, err = replay("native-result-r5", "")
	rejected("historical r5 reducer", "OBSERVATION_SCHEMA_UNSUPPORTED", r, err)
	r, err = replay("native-result-r6", "sources/00000-one.bin")
	rejected("missing witness", "PREPROCESS_SOURCE_MISSING", r, err)
	witness("00000-one.bin", strings.Replace(one, "X", "W", 1))
	r, err = replay("native-result-r6", "")
	rejected("changed witness", "PREPROCESS_SOURCE_MISMATCH", r, err)
	witness("00000-one.bin", one)
	tamper := func(name, code string, change func(doc map[string]any)) kit.ReplayResult {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(published, &doc); err != nil {
			t.Fatal(err)
		}
		change(doc)
		data, _ := json.Marshal(doc)
		// keep the recorded operation bytes unless the change is to them: a re-marshalled map
		// reorders its members, which the canonical operation check alone would reject
		var orig, top map[string]json.RawMessage
		if err := json.Unmarshal(published, &orig); err != nil || json.Unmarshal(data, &top) != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(name, "operation") {
			top["operation"] = orig["operation"]
			data, _ = json.Marshal(top)
		}
		if err := os.WriteFile(filepath.Join(out, "result.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(filepath.Join(out, "result.json"), published, 0o644)
		r, err := replay("native-result-r6", "")
		rejected(name, code, r, err)
		return r
	}
	cases := func(doc map[string]any) []any { return doc["cases"].([]any) }
	for _, role := range []int{0, 1} {
		truncate := func(doc map[string]any) {
			o := doc["steps"].([]any)[1].(map[string]any)["observations"].([]any)[role].(map[string]any)
			o["status"], o["reason"] = "TRUNCATED", "MESSAGE_LIMIT"
			o["counts"].(map[string]any)["done"] = float64(0)
		}
		rawPath := filepath.Join(out, "responses", "00005-switch.json")
		raw, err := os.ReadFile(rawPath)
		if err != nil {
			t.Fatal(err)
		}
		var rawDoc map[string]any
		if err := json.Unmarshal(raw, &rawDoc); err != nil {
			t.Fatal(err)
		}
		truncate(rawDoc)
		changed, _ := json.Marshal(rawDoc)
		if err := os.WriteFile(rawPath, changed, 0o644); err != nil {
			t.Fatal(err)
		}
		r := tamper(fmt.Sprintf("selection capped start role %d", role), "CLAIM_MISMATCH", func(doc map[string]any) {
			truncate(cases(doc)[5].(map[string]any))
		})
		if err := os.WriteFile(rawPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, g := range r.Gates {
			if g.ID == "reuse-observation" && g.Failed != 0 {
				t.Fatalf("matched raw/projection prefix must pass observation binding: %+v", g)
			}
		}
	}
	tamper("selection route promoted", "CLAIM_MISMATCH", func(doc map[string]any) {
		cases(doc)[5].(map[string]any)["claims"].(map[string]any)["incremental_route"] = "PASS"
	})
	tamper("selection edit changed", "OBSERVATION_EDIT_POINT_MISMATCH", func(doc map[string]any) {
		cases(doc)[5].(map[string]any)["steps"].([]any)[1].(map[string]any)["edit"].(map[string]any)["new_end_byte"] = 12
	})
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
	tree := func(doc map[string]any) map[string]any {
		return cases(doc)[0].(map[string]any)["steps"].([]any)[0].(map[string]any)["incremental"].(map[string]any)["tree"].(map[string]any)
	}
	tamper("tree policy removed", "PREPROCESS_POLICY_MISMATCH", func(doc map[string]any) {
		tr := tree(doc)
		ids := []any{}
		for _, id := range tr["identities"].([]any) {
			if id.(map[string]any)["role"] != "policy" {
				ids = append(ids, id)
			}
		}
		tr["identities"] = ids
	})
	tamper("tree policy schema", "PREPROCESS_POLICY_MISMATCH", func(doc map[string]any) {
		for _, id := range tree(doc)["identities"].([]any) {
			if m := id.(map[string]any); m["role"] == "policy" {
				m["schema"] = "tsgk-native-policy/r1"
			}
		}
	})
	tamper("edit refusal relabelled", "PREPROCESS_OUTCOME_MISMATCH", func(doc map[string]any) {
		cases(doc)[2].(map[string]any)["code"] = "EDIT_RANGE_INVALID"
	})
	tamper("process on a stopped case", "PREPROCESS_OUTCOME_MISMATCH", func(doc map[string]any) {
		cases(doc)[1].(map[string]any)["process"] = map[string]any{"status": "COMPLETED", "exit_code": 0}
	})
	tamper("operation member case-folded", "PREPROCESS_POLICY_MISMATCH", func(doc map[string]any) {
		doc["operation"].(map[string]any)["RUN_WALL_NS"] = 1
	})
	tamper("leaf in a skipped section", "PREPROCESS_TREE_OUTSIDE_RANGES", func(doc map[string]any) {
		at := strings.Index(one, "class Y")
		tr := tree(doc)
		tr["nodes"] = append(tr["nodes"].([]any), map[string]any{"parent": 0, "type": "identifier", "field": nil, "named": true, "extra": false, "is_error": false, "has_error": false,
			"is_missing": false, "start_byte": at + 6, "end_byte": at + 7, "start_point": map[string]any{"row": 3, "column": 6}, "end_point": map[string]any{"row": 3, "column": 7}})
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
