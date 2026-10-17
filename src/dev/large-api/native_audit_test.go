package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/largeaudit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The Windows audit and corpus wrapper include the driver, so compile and execute both callers.
func TestNativeAuditSharedParser(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("audit process backend requires windows/amd64")
	}
	rt, cc := os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if rt == "" || cc == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("required native runtime/compiler absent")
		}
		t.Skip("native runtime/compiler not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	grammar, err := filepath.Abs("../../testdata/native/plain")
	if err != nil {
		t.Fatal(err)
	}
	var files []kit.NativeInput
	for _, name := range []string{"src/parser.c", "src/tree_sitter/parser.h", "src/tree_sitter/alloc.h", "src/tree_sitter/array.h"} {
		data, err := os.ReadFile(filepath.Join(grammar, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		role := "header"
		if name == "src/parser.c" {
			role = "parser"
		}
		files = append(files, kit.NativeInput{Path: name, Role: role, SHA256: digest(data), Bytes: uint64(len(data))})
	}
	compiler, err := os.ReadFile(cc)
	if err != nil {
		t.Fatal(err)
	}
	b, err := native.NewBuild(ctx, native.BuildRequest{Work: t.TempDir(), Runtime: rt, GrammarRoot: grammar, Grammar: files, Symbol: "tree_sitter_tsgk_plain", Compiler: cc, CompilerID: kit.ToolIdentity{Name: "cc", Version: "host", SHA256: digest(compiler), Bytes: uint64(len(compiler))}})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := os.ReadFile("audit.c")
	if err != nil {
		t.Fatal(err)
	}
	if err = writeExclusive(filepath.Join(b.Dir, "audit.c"), audit); err != nil {
		t.Fatal(err)
	}
	op := kit.NativeOperations()["native-query-large"]
	req := native.Request{ID: "shared-audit", Encoding: kit.EncodingUTF8, Output: kit.OutputAuto, Limits: native.LimitsFor(op), Source: []byte("a=f(1,[2,3]);")}
	baseline := b.RunCase(ctx, native.Context{Op: kit.NativeOperations()["native-parse-edit"], Output: kit.OutputTree}, kit.IncrementalCase{ID: req.ID, Encoding: kit.EncodingUTF8}, req.Source)
	if baseline.ExecutionStatus != kit.StatusCompleted || len(baseline.Steps) != 1 || baseline.Steps[0].Incremental == nil {
		t.Fatalf("baseline: %+v", baseline)
	}
	nodes := baseline.Steps[0].Incremental.DescendantCount
	for _, corpus := range []bool{false, true} {
		name := "audit.c"
		if corpus {
			name = "corpus-audit.c"
			if err = writeExclusive(filepath.Join(b.Dir, name), []byte("#define TSGK_CORPUS_CST\n#include \"audit.c\"\n")); err != nil {
				t.Fatal(err)
			}
		}
		exe, _, err := compile(ctx, b, cc, filepath.Join(b.Dir, name), "")
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if corpus {
			result, err := runner.Run(ctx, runner.Spec{Path: exe, Args: []string{"0", "100"}, Dir: b.Dir, Stdin: native.Frame(req.Encode()), StdoutBytes: int64(op.OutputBytes) + 4, StderrBytes: 65536, Wall: op.ParseWall, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
			if err != nil || !result.Succeeded() || !result.Cleanup.Verified {
				t.Fatalf("corpus process: %+v %v", result, err)
			}
			if _, err = decodeCorpusPage(result.Stdout, req, 0, 100, nodes); err != nil {
				t.Fatal(err)
			}
		} else {
			resource := largeaudit.ResourceRequest()
			limited := run(ctx, exe, digest(data), b.Dir, resource, 0, op.Nodes, nodes)
			if limited.Error != "RESOURCE_LIMIT:ALLOCATION_LIMIT" || limited.Resource == nil {
				t.Fatalf("actual resource capture: error=%s", limited.Error)
			}
			encoded, err := json.Marshal(limited)
			if err != nil {
				t.Fatal(err)
			}
			var replay observation
			if err = json.Unmarshal(encoded, &replay); err != nil {
				t.Fatal(err)
			}
			if _, err = largeaudit.CheckResource(replay.Resource, replay.Process); err != nil {
				t.Fatal(err)
			}
			replay.Resource.Stdout[4] ^= 1
			if _, err = largeaudit.CheckResource(replay.Resource, replay.Process); err == nil {
				t.Fatal("actual resource frame tampering accepted")
			}
			for _, protocol := range []string{native.Protocol, native.ProtocolR2} {
				req.Protocol = protocol
				result := run(ctx, exe, digest(data), b.Dir, req, 0, 100, nodes)
				if result.Error != "" || result.Receipt.Differences != 0 || !result.Receipt.Complete {
					t.Fatalf("audit %s: %+v", protocol, result)
				}
			}
			trace := b.RunCase(ctx, native.Context{Op: kit.NativeOperations()["native-parse-edit"], Output: kit.OutputTree, Protocol: native.ProtocolR3, ObservationRunID: "audit-r3-unsupported"}, kit.IncrementalCase{ID: req.ID, Encoding: kit.EncodingUTF8}, req.Source)
			if trace.ExecutionStatus != kit.StatusCompleted || trace.ReuseObservationClaim != native.ClaimPass {
				t.Fatalf("valid native r3 control: %+v", trace)
			}
			unsupported := req
			unsupported.Protocol, unsupported.ObservationRunID, unsupported.ObservationProducerSHA256 = native.ProtocolR3, "audit-r3-unsupported", b.Identity
			denied, err := runner.Run(ctx, runner.Spec{Path: exe, Args: []string{"0", "100"}, Dir: b.Dir, Stdin: native.Frame(unsupported.Encode()), StdoutBytes: 65536, StderrBytes: 65536, Wall: op.ParseWall, Grace: 5 * time.Second, Memory: runner.Memory{Bytes: op.MemoryBytes, Hard: true}})
			if err != nil || denied.ExitCode != 2 || !denied.Cleanup.Verified || len(denied.Stdout) != 0 {
				t.Fatalf("audit consumed unsupported r3 or failed unsafely: %+v %v", denied, err)
			}
		}
	}
	testNativeSession(t, ctx, b, cc, req, nodes)

}
