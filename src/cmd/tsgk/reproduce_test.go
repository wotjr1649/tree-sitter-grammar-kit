package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The CLI test binary doubles as a minimal JSON-mode generator for the reproduce command.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "generate" {
		var out string
		for i, a := range os.Args {
			if a == "--output" {
				out = os.Args[i+1]
			}
		}
		entry := os.Args[len(os.Args)-1]
		data, err := os.ReadFile(entry)
		if err != nil {
			os.Exit(2)
		}
		os.WriteFile(filepath.Join(out, "parser.c"), append([]byte("parser "), data...), 0o644)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// S04-A10: the CLI passes literal spaces, quotes and Unicode in authorized paths to the
// same reproduce core, keeps work/out/profile outside the root and maps exit codes.
func TestReproduceCLI(t *testing.T) {
	if runtime.GOOS == "linux" && os.Getenv("TSGK_CGROUP_PARENT") == "" {
		t.Skip("Linux reproduction requires the delegated cgroup backend (CI provides it)")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tool, _ := os.ReadFile(exe)
	base := t.TempDir()
	name := "src 'q' 한글"
	if runtime.GOOS != "windows" {
		name = `src 'q' "d" 한글`
	}
	root := filepath.Join(base, name)
	grammar := []byte(`{"name":"x"}`)
	writeTree(t, root, map[string]string{"src/grammar.json": string(grammar)})
	work := filepath.Join(base, "work dir ü")
	os.Mkdir(work, 0o755)
	parser := append([]byte("parser "), grammar...)
	profile := map[string]any{"schema": "tsgk-reproduce/r1", "id": "cli", "route": "fixture", "mode": "json",
		"generator": map[string]any{"name": "tree-sitter", "version": "test", "sha256": sum(tool), "bytes": len(tool)},
		"abi":       15, "optimize": true, "grammar": "src/grammar.json",
		"inputs":  []any{map[string]any{"path": "src/grammar.json", "role": "grammar_json", "sha256": sum(grammar), "bytes": len(grammar)}},
		"outputs": []any{map[string]any{"path": "parser.c", "reference": "PRESENT", "sha256": sum(parser), "bytes": len(parser)}},
		"limits": map[string]any{"wall_seconds": 60, "output_bytes": 1 << 20, "storage_bytes": 64 << 20, "memory_bytes": 1 << 30,
			"input_files": 4, "input_bytes": 1 << 20, "file_bytes": 1 << 20}}
	data, _ := json.Marshal(profile)
	profPath := filepath.Join(base, "profile ü.json")
	os.WriteFile(profPath, data, 0o644)
	args := func(out string, extra ...string) []string {
		a := []string{"reproduce", "--root", root, "--profile", profPath, "--out", out, "--work", work, "--tool", "tree-sitter=" + exe}
		if cg := os.Getenv("TSGK_CGROUP_PARENT"); cg != "" {
			a = append(a, "--cgroup-parent", cg)
		}
		return append(a, extra...)
	}
	ctx := context.Background()
	out := filepath.Join(base, "out 'x' ü")
	code, stdout, stderr := cli(t, ctx, args(out, "--allow", "EXEC_GENERATOR")...)
	if code != 0 || !strings.Contains(stdout, `"assessment":"PASS"`) || !strings.Contains(stdout, `"json_regeneration":"PASS"`) {
		t.Fatalf("reproduce: %d %s %s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "workspace-b", "out", "parser.c")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{args(filepath.Join(base, "o2")), 3, "CAPABILITY_NOT_GRANTED"},
		{args(out, "--allow", "EXEC_GENERATOR"), 2, "OUTPUT_EXISTS"},
		{args(filepath.Join(root, "o3"), "--allow", "EXEC_GENERATOR"), 2, "OUTPUT_INSIDE_INPUT"},
		{append(args(filepath.Join(base, "o4"), "--allow", "EXEC_GENERATOR"), "--work", root), 2, "WORK_INSIDE_INPUT"},
	} {
		code, _, stderr := cli(t, ctx, c.args...)
		if code != c.code || !strings.Contains(stderr, c.want) {
			t.Fatalf("%s: exit %d %s", c.want, code, stderr)
		}
	}
}
