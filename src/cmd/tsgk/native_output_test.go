package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The real build rejects the missing runtime closure after output acquisition.
// No compiler or driver process is launched by these namespace controls.
func nativeOutputArgs(t *testing.T, kind string) ([]string, string, string) {
	t.Helper()
	base := t.TempDir()
	for _, name := range []string{"input", "grammar", "work", "destination"} {
		if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root, grammar := filepath.Join(base, "input"), filepath.Join(base, "grammar")
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte("a = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(grammar, "parser.c"), []byte("not a parser"), 0o600); err != nil {
		t.Fatal(err)
	}
	cc := filepath.Join(base, "cc-not-executable")
	if err := os.WriteFile(cc, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{
		"schema": "tsgk-incremental/r2", "id": "output-boundary", "route": "owned", "operation": "native-parse-edit",
		"symbol": "tree_sitter_tsgk_plain", "encoding": "UTF-8", "output": "tree",
		"compiler": map[string]any{"name": "cc", "version": "test", "sha256": sum([]byte("x")), "bytes": 1},
		"grammar":  []any{fileEntry(t, grammar, "parser.c", "parser")}, "declarations": nil,
		"cases": []any{map[string]any{"id": "one", "input": fileEntry(t, root, "one.txt", "case"), "edits": []any{}, "points": []any{}, "expect": []any{}}},
	}
	if kind == "oracle" {
		profile["schema"], profile["operation"] = kit.OracleSchema, "native-query"
		profile["queries"], profile["fact_pack"], profile["api"] = []any{}, nil, false
		c := profile["cases"].([]any)[0].(map[string]any)
		c["query_expect"], c["dynamic_sql_expect"] = []any{}, nil
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if kind == "oracle" {
		if _, err := kit.ParseOracleProfile(data); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := kit.ParseIncrementalProfile(data); err != nil {
			t.Fatal(err)
		}
	}
	doc := filepath.Join(base, "profile.json")
	if err := os.WriteFile(doc, data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{kind}
	if kind == "oracle" {
		args = append(args, "record")
	}
	args = append(args, "--root", root, "--grammar-root", grammar, "--profile", doc, "--runtime", base, "--tool", "cc="+cc,
		"--work", filepath.Join(base, "work"), "--out", filepath.Join(base, "destination", "out"), "--allow", native.AllowBuild, "--allow", native.AllowExec)
	if cg := os.Getenv("TSGK_CGROUP_PARENT"); cg != "" {
		args = append(args, "--cgroup-parent", cg)
	}
	return args, base, root
}

func TestNativeRetainsCheckedOutputParent(t *testing.T) {
	if os.Getenv("TSGK_NATIVE_RUNTIME") == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("native validation context required")
		}
		t.Skip("native validation context, including host memory control, not configured")
	}
	for _, kind := range []string{"incremental", "oracle"} {
		for _, mode := range []string{"replacement", "closed", "collision"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				args, base, root := nativeOutputArgs(t, kind)
				parent, moved := filepath.Join(base, "destination"), filepath.Join(base, "moved")
				called, blocked := false, false
				testHookBeforeNativeRun = func(target *native.OutputTarget) {
					called = true
					switch mode {
					case "replacement":
						if err := os.Rename(parent, moved); err != nil {
							if runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(5))) {
								blocked = true
								return
							}
							t.Fatal(err)
						}
						if err := os.Mkdir(parent, 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(parent, "unrelated"), []byte("keep"), 0o600); err != nil {
							t.Fatal(err)
						}
					case "closed":
						if target == nil {
							t.Fatal("no checked output target")
						}
						if err := target.Parent.Close(); err != nil {
							t.Fatal(err)
						}
					case "collision":
						if err := os.Mkdir(filepath.Join(parent, "out"), 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(parent, "out", "unrelated"), []byte("keep"), 0o600); err != nil {
							t.Fatal(err)
						}
					}
				}
				defer func() { testHookBeforeNativeRun = nil }()
				code, text, stderr := cli(t, context.Background(), args...)
				if !called {
					t.Fatal("preflight barrier not reached")
				}
				switch mode {
				case "replacement":
					var result struct {
						Build *native.Build `json:"build"`
					}
					if code != exitUsage || !strings.Contains(text, "RUNTIME_MISMATCH") || json.Unmarshal([]byte(text), &result) != nil || result.Build == nil || len(result.Build.Steps) != 0 {
						t.Fatalf("expected real build refusal after acquisition: %d %s %s", code, stderr, text)
					}
					if blocked {
						t.Log("parent rename was blocked while the checked handle was held")
						if err := os.Rename(parent, moved); err != nil {
							t.Fatalf("parent guard did not release: %v", err)
						}
					} else {
						entries, err := os.ReadDir(parent)
						data, readErr := os.ReadFile(filepath.Join(parent, "unrelated"))
						if err != nil || readErr != nil || len(entries) != 1 || string(data) != "keep" {
							t.Fatalf("replacement changed: %v %q %v %v", entries, data, err, readErr)
						}
					}
					if info, err := os.Stat(filepath.Join(moved, "out")); err != nil || !info.IsDir() {
						t.Fatalf("checked parent not used: %v", err)
					}

				case "closed":
					want := exitIO
					if kind == "oracle" {
						want = exitUsage
					}
					if code != want || !strings.Contains(stderr, "OUTPUT_FAILED") {
						t.Fatalf("closed parent fallback: %d %s", code, stderr)
					}
					if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
						t.Fatalf("closed parent produced output: %v %v", entries, err)
					}
				case "collision":
					if code != exitUsage || !strings.Contains(stderr, "OUTPUT_EXISTS") {
						t.Fatalf("collision: %d %s", code, stderr)
					}
					entries, err := os.ReadDir(filepath.Join(parent, "out"))
					data, readErr := os.ReadFile(filepath.Join(parent, "out", "unrelated"))
					if err != nil || readErr != nil || len(entries) != 1 || string(data) != "keep" {
						t.Fatalf("existing output changed: %v %q %v %v", entries, data, err, readErr)
					}
				}
				if data, err := os.ReadFile(filepath.Join(root, "one.txt")); err != nil || string(data) != "a = 1;\n" {
					t.Fatalf("input changed: %q %v", data, err)
				}
			})
		}
	}
}

func TestNativeRejectsEveryInputRootOutput(t *testing.T) {
	for _, kind := range []string{"incremental", "oracle"} {
		for _, subject := range []string{"input", "grammar"} {
			t.Run(kind+"/"+subject, func(t *testing.T) {
				args, base, _ := nativeOutputArgs(t, kind)
				out := filepath.Join(base, subject, "out")
				for i := range args {
					if args[i] == "--out" {
						args[i+1] = out
					}
				}
				code, text, stderr := cli(t, context.Background(), args...)
				if code != exitUsage || text != "" || !strings.Contains(stderr, "OUTPUT_INSIDE_INPUT") {
					t.Fatalf("input output accepted: %d %s %s", code, stderr, text)
				}
				if _, err := os.Lstat(out); !os.IsNotExist(err) {
					t.Fatalf("input output created: %v", err)
				}
			})
		}
	}
}

func TestNativeStdoutWriteFailure(t *testing.T) {
	for _, kind := range []string{"incremental", "oracle"} {
		t.Run(kind, func(t *testing.T) {
			args, _, _ := nativeOutputArgs(t, kind)
			// Refuse capability before acquisition; serialization still uses the real stdout path.
			for i := range args {
				if args[i] == "--allow" {
					args[i+1] = "ungranted"
				}
			}
			file, err := os.CreateTemp(t.TempDir(), "closed-stdout")
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			if code := run(context.Background(), args, file, &stderr); code != exitIO || !strings.Contains(stderr.String(), "OUTPUT_WRITE_FAILED") {
				t.Fatalf("stdout error lost: %d %s", code, stderr.String())
			}
		})
	}
}
