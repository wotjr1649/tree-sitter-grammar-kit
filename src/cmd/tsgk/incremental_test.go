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
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("native tools required")
		}
		t.Skip("native tools not configured")
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
	os.WriteFile(filepath.Join(root, "cases", "one.txt"), []byte(source), 0o644)
	ccData, _ := os.ReadFile(cc)
	enc := base64.StdEncoding.EncodeToString
	edit := map[string]any{"start_byte": 4, "old_end_byte": 5, "new_end_byte": 5, "old": enc([]byte("f")), "new": enc([]byte("g"))}
	profile := map[string]any{
		"schema": "tsgk-incremental/r1", "id": "cli-plain", "route": "owned-plain", "operation": "native-parse-edit", "symbol": "tree_sitter_tsgk_plain",
		"encoding": "UTF-8", "output": "tree", "compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum(ccData), "bytes": len(ccData)},
		"grammar": []any{fileEntry(t, root, "src/parser.c", "parser"), fileEntry(t, root, "src/tree_sitter/alloc.h", "header"),
			fileEntry(t, root, "src/tree_sitter/array.h", "header"), fileEntry(t, root, "src/tree_sitter/parser.h", "header")},
		"declarations": map[string]any{"mapping": "owned-r1", "items": []any{map[string]any{"fact": "type_declaration", "node": "assignment", "name": "field:name"}}},
		"cases": []any{map[string]any{"id": "one", "input": fileEntry(t, root, "cases/one.txt", "case"), "edits": []any{edit}, "points": []any{},
			"expect": []any{map[string]any{"step": 1, "syntax": "NO_ERROR", "contains": []any{"call"}, "declarations": "PASS"}}}},
	}
	data, _ := json.Marshal(profile)
	pf := filepath.Join(base, "profile.json")
	os.WriteFile(pf, data, 0o644)
	work := filepath.Join(base, "work")
	os.MkdirAll(work, 0o755)
	out := filepath.Join(base, "out")
	args := []string{"incremental", "--root", root, "--profile", pf, "--runtime", rt, "--tool", "cc=" + cc, "--work", work, "--out", out}
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
	var res struct {
		ExecutionStatus string `json:"execution_status"`
		Assessment      string `json:"assessment"`
		BuildRemoved    string `json:"build_removed"`
		Cases           []struct {
			Claims map[string]string `json:"claims"`
		} `json:"cases"`
	}
	published, err := os.ReadFile(filepath.Join(out, "result.json"))
	if err != nil || json.Unmarshal(published, &res) != nil || res.Assessment != "PASS" || res.BuildRemoved != "REMOVED" || res.Cases[0].Claims["incremental_route"] != "PASS" {
		t.Fatalf("published result %v %s", err, published[:min(len(published), 400)])
	}
	if _, err := os.Stat(filepath.Join(out, "responses", "00000-one.json")); err != nil {
		t.Fatal("raw response not preserved")
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
