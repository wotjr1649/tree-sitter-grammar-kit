package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Owned padding preserves the JSON grammar; this control does not approve a reference.
func TestOracleSemanticsPagedProducerControl(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("new semantic producer validation excluded on macOS")
	}
	prepared := os.Getenv("TSGK_BENCH_PREPARED")
	if prepared == "" {
		t.Skip("pinned JSON grammar required")
	}
	rt, cc := nativeTools(t)
	symbol, registered := registeredGrammar(t, "json")
	ccHash, ccSize := digestOf(t, cc)
	var anchors struct{ Queries []kit.OracleQuery }
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "query-semantics", "small-json-anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &anchors); err != nil {
		t.Fatal(err)
	}
	runtimeData, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var pinnedRuntime struct{ Files []json.RawMessage }
	if err := json.Unmarshal(runtimeData, &pinnedRuntime); err != nil || len(pinnedRuntime.Files) != 83 {
		t.Fatal("runtime closure", err)
	}
	for _, mode := range []string{"normal", "missing-manifest", "unexpected-stderr"} {
		t.Run(mode, func(t *testing.T) {
			grammarRoot := t.TempDir()
			grammar := append([]kit.NativeInput(nil), registered...)
			parserIndex := -1
			for i := range grammar {
				pin := &grammar[i]
				data, err := os.ReadFile(filepath.Join(prepared, "routes", "json", filepath.FromSlash(pin.Path)))
				if err != nil || uint64(len(data)) != pin.Bytes || sha(data) != pin.SHA256 {
					t.Fatal("pinned grammar changed", err)
				}
				if pin.Role == "parser" {
					parserIndex = len(pinnedRuntime.Files) + i
					data = append(data, bytes.Repeat([]byte(" "), kit.MaxDocumentBytes+1-len(data))...)
					if mode == "unexpected-stderr" {
						data = append(data, []byte("\n#include <stdio.h>\n__attribute__((constructor)) static void paging_control(void) { fputs(\"paging stderr control\\n\", stderr); }\n")...)
					}
					pin.Bytes, pin.SHA256 = uint64(len(data)), sha(data)
				}
				path := filepath.Join(grammarRoot, filepath.FromSlash(pin.Path))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if parserIndex < 0 {
				t.Fatal("missing parser")
			}
			plan := &semanticCapturePlan{Context: semanticPagedContext}
			add := func(owner, path, role string) {
				plan.Members = append(plan.Members, semanticCaptureMember{Owner: owner, Path: path, Role: role})
			}
			if mode != "missing-manifest" {
				add("O", "manifest.json", "manifest")
			}
			add("D", "capture/driver.bin", "driver")
			for i := 0; i < len(pinnedRuntime.Files)+len(grammar)+3; i++ {
				role := "build-input"
				if i >= len(pinnedRuntime.Files) && i < len(pinnedRuntime.Files)+len(grammar) {
					role = "grammar-" + grammar[i-len(pinnedRuntime.Files)].Role
				}
				if i == parserIndex {
					add("D", fmt.Sprintf("capture/build-input-%05d.pages.json", i), "grammar-paged-descriptor")
					for part := 0; part < 2; part++ {
						add("D", fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", i, part), "grammar-paged-part")
					}
				} else {
					add("D", fmt.Sprintf("capture/build-input-%05d.bin", i), role)
				}
			}
			add("D", "capture/build.json", "build")
			add("D", "capture/build-manifest.txt", "build-manifest")
			add("O", "projection/00000-c1.json", "projection")
			for _, x := range []struct{ suffix, role string }{{"-stdin.bin", "stdin-frame"}, {"-process.json", "process"}, {"-stdout.bin", "stdout-frame"}, {".json", "capture"}} {
				add("D", "capture/00000-c1"+x.suffix, x.role)
			}
			add("O", "source/00000-c1.bin", "source")
			add("D", "raw/00000-c1.json", "raw")
			add("O", "records/00000-c1.json", "record")
			registration, err := json.Marshal(map[string]any{"schema": "tsgk-query-semantics-registration/r1", "context": semanticPagedContext, "planned_members": plan.Members, "producer_control": "not_external_approval"})
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			source := []byte("{\"a\":[1,1]}")
			if err := os.WriteFile(filepath.Join(root, "c1.json"), source, 0600); err != nil {
				t.Fatal(err)
			}
			input := kit.NativeInput{Path: "c1.json", Role: "case", Bytes: uint64(len(source)), SHA256: sha(source)}
			c := map[string]any{"id": "c1", "input": input, "encoding": kit.EncodingUTF8, "edits": []kit.Edit{{StartByte: 8, OldEndByte: 9, NewEndByte: 9, Old: []byte("1"), New: []byte("2")}}, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil}
			profile, err := json.Marshal(map[string]any{"schema": kit.OracleSemanticsSchema, "id": "paged-owned-control", "route": "json", "operation": "native-query", "symbol": symbol, "encoding": kit.EncodingUTF8, "output": kit.OutputTree, "compiler": kit.ToolIdentity{Name: "cc", Version: "control", SHA256: ccHash, Bytes: ccSize}, "grammar": grammar, "declarations": nil, "queries": anchors.Queries, "fact_pack": nil, "api": false, "cases": []any{c}, "query_semantics_registration_sha256": sha(registration)})
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "set")
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			req := OracleRequest{Root: root, GrammarRoot: grammarRoot, Profile: profile, Runtime: rt, Compiler: cc, Work: t.TempDir(), Out: out, Allow: []string{AllowBuild, AllowExec}, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT"), QuerySemanticsRegistration: registration}
			result, err := Oracle(ctx, req)
			if mode == "missing-manifest" {
				if err == nil || result.Build != nil {
					t.Fatalf("missing manifest reached compiler: %+v %v", result, err)
				}
				if _, e := os.Stat(out); !os.IsNotExist(e) {
					t.Fatal("preflight refusal wrote output", e)
				}
				return
			}
			if err != nil || result.BuildRemoved != "REMOVED" {
				t.Fatalf("capture/cleanup: %+v %v", result, err)
			}
			if mode == "unexpected-stderr" {
				stderr, e := os.ReadFile(filepath.Join(out, "capture", "00000-c1-stderr.bin"))
				expected := "paging stderr control\n"
				// The constructor runs before the driver sets its stream modes.
				if runtime.GOOS == "windows" {
					expected = "paging stderr control\r\n"
				}
				if e != nil || string(stderr) != expected {
					t.Fatalf("stderr not preserved %q %v", stderr, e)
				}
				if result.ExecutionStatus != kit.StatusFailed || result.Set != nil {
					t.Fatalf("unexpected stderr completed set %+v", result)
				}
				if _, e := os.Stat(filepath.Join(out, "manifest.json")); !os.IsNotExist(e) {
					t.Fatal("unexpected stderr emitted complete marker", e)
				}
				return
			}
			if result.ExecutionStatus != kit.StatusCompleted || result.Assessment != kit.AssessBlocked || result.Set == nil || !result.Set.Valid {
				t.Fatalf("paged producer not pending valid: %+v", result)
			}
			data, e := os.ReadFile(filepath.Join(out, "manifest.json"))
			if e != nil {
				t.Fatal(e)
			}
			var m kit.OracleManifest
			if e := json.Unmarshal(data, &m); e != nil || m.Schema != kit.OraclePagedSemanticsManifestSchema {
				t.Fatal("wrong manifest", e)
			}
			descriptors, parts := 0, 0
			for _, member := range m.Members {
				if member.Bytes > kit.MaxDocumentBytes {
					t.Fatal("oversized physical member")
				}
				if member.Role == "grammar-paged-descriptor" {
					descriptors++
				}
				if member.Role == "grammar-paged-part" {
					parts++
				}
			}
			if descriptors != 1 || parts != 2 {
				t.Fatalf("paged physical closure %d/%d", descriptors, parts)
			}
		})
	}
}
