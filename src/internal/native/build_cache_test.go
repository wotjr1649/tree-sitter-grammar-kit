package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestBuildCache(t *testing.T) {
	t.Parallel()
	rt, cc := nativeTools(t)
	sum, size := digestOf(t, cc)
	req := testBuildRequest(t.TempDir(), rt, fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"), cc, kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: size})
	req.CacheDir = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	start := time.Now()
	first, err := NewBuild(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	cold := time.Since(start)
	if first.Reuse == nil || first.Reuse.Hit || len(first.Steps) == 0 {
		t.Fatalf("first request did not compile: %+v", first.Reuse)
	}
	identity, executable := first.Identity, first.ExecutableSHA256
	baseline := runCase(t, first, "cache", kit.EncodingUTF8, plainSource, nil)
	if baseline.ExecutionStatus != kit.StatusCompleted || len(baseline.Steps) != 1 {
		t.Fatalf("cold execution: %s %s", baseline.ExecutionStatus, baseline.Code)
	}
	if err := first.Remove(); err != nil {
		t.Fatal(err)
	}
	// A new CLI process has no live Build pointer; only the trusted run cache survives.
	req.Work = t.TempDir()
	start = time.Now()
	second, err := NewBuild(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	warm := time.Since(start)
	if !second.Reuse.Hit || len(second.Steps) != 0 || len(second.Reuse.SourceSteps) != len(first.Steps) || second.Identity != identity || second.ExecutableSHA256 != executable || second.Dir == first.Dir {
		t.Fatal("cache did not preserve identity with zero new compilation steps")
	}
	replayed := runCase(t, second, "cache", kit.EncodingUTF8, plainSource, nil)
	if replayed.ExecutionStatus != kit.StatusCompleted || len(replayed.Steps) != 1 || replayed.Steps[0].Incremental.Digest != baseline.Steps[0].Incremental.Digest {
		t.Fatalf("warm execution differs: %s %s", replayed.ExecutionStatus, replayed.Code)
	}
	// The private launch copy remains pinned even if the shared cache changes later.
	launchBytes, err := os.ReadFile(second.Executable)
	if err != nil {
		t.Fatal(err)
	}
	sharedExe := filepath.Join(req.CacheDir, first.Reuse.Key, exeName())
	changedShared := append([]byte(nil), launchBytes...)
	changedShared[len(changedShared)-1] ^= 1
	if err := os.WriteFile(sharedExe, changedShared, 0o600); err != nil {
		t.Fatal(err)
	}
	stillPrivate := runCase(t, second, "private-copy", kit.EncodingUTF8, plainSource, nil)
	if stillPrivate.ExecutionStatus != kit.StatusCompleted || len(stillPrivate.Steps) != 1 || stillPrivate.Steps[0].Incremental.Digest != baseline.Steps[0].Incremental.Digest {
		t.Fatal("shared file modification changed the already verified private copy")
	}
	corrupt, err := NewBuild(ctx, req)
	if codeOf(err) != "BUILD_CACHE_INVALID" || corrupt == nil || len(corrupt.Steps) != 0 {
		t.Fatalf("next reader accepted modified shared executable: %v", err)
	}
	if err := corrupt.Remove(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedExe, launchBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second.Executable, append(launchBytes, 0), 0o700); err != nil {
		t.Fatal(err)
	}
	refused := runCase(t, second, "tampered", kit.EncodingUTF8, plainSource, nil)
	if refused.Process != nil || refused.Code != "EXECUTABLE_MISMATCH" {
		t.Fatal("tampered private executable was launched")
	}
	if err := second.Remove(); err != nil {
		t.Fatal(err)
	}
	t.Logf("same-run owned grammar: cold=%s warm=%s compiler processes=%d -> 0 (parallel functional phase, includes contention; n=1, not a performance guarantee)", cold, warm, len(first.Steps))

	entry := filepath.Join(req.CacheDir, first.Reuse.Key)
	recordPath, exePath := filepath.Join(entry, "build.json"), filepath.Join(entry, exeName())
	original, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var record cachedBuild
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*cachedBuild)
	}{
		{"identity", func(r *cachedBuild) { r.Identity = "wrong" }},
		{"key", func(r *cachedBuild) { r.Key = "wrong" }},
		{"schema", func(r *cachedBuild) { r.Schema = "wrong" }},
		{"hash", func(r *cachedBuild) { r.ExecutableSHA = string(make([]byte, 64)) }},
		{"size", func(r *cachedBuild) { r.ExecutableBytes++ }},
		{"oversized-executable", func(r *cachedBuild) { r.ExecutableBytes = buildCacheExecutableLimit + 1 }},
		{"steps-missing", func(r *cachedBuild) { r.Steps = nil }},
		{"argv", func(r *cachedBuild) { r.Steps[0].Argv = []string{"--unexpected"} }},
		{"step-failed", func(r *cachedBuild) { r.Steps[0].Executed = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var changed cachedBuild
			if err := json.Unmarshal(original, &changed); err != nil {
				t.Fatal(err)
			}
			tc.edit(&changed)
			data, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(recordPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			b, err := NewBuild(ctx, req)
			if codeOf(err) != "BUILD_CACHE_INVALID" || b == nil || len(b.Steps) != 0 {
				t.Fatalf("corruption silently rebuilt or reused: %v", err)
			}
			if err := b.Remove(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := os.WriteFile(recordPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`,"identity":"` + record.Identity + `"}`, `,"unexpected":true}`} {
		if err := os.WriteFile(recordPath, append(append([]byte(nil), original[:len(original)-1]...), extra...), 0o600); err != nil {
			t.Fatal(err)
		}
		b, err := NewBuild(ctx, req)
		if codeOf(err) != "BUILD_CACHE_INVALID" || b == nil || len(b.Steps) != 0 {
			t.Fatalf("duplicate or unknown cache record accepted: %v", err)
		}
		if err := b.Remove(); err != nil {
			t.Fatal(err)
		}
	}
	oversizedJSON := append(append([]byte(nil), original...), bytes.Repeat([]byte(" "), buildCacheRecordLimit-len(original)+1)...)
	if !json.Valid(oversizedJSON) {
		t.Fatal("oversized control must remain valid JSON")
	}
	if err := os.WriteFile(recordPath, oversizedJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	oversized, err := NewBuild(ctx, req)
	if codeOf(err) != "BUILD_CACHE_INVALID" || oversized == nil || len(oversized.Steps) != 0 {
		t.Fatalf("valid oversized JSON silently rebuilt or reused: %v", err)
	}
	if err := oversized.Remove(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []bool{false, true} {
		if missing {
			if err := os.Remove(exePath); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(exePath, exe[:len(exe)-1], 0o600); err != nil {
			t.Fatal(err)
		}
		b, err := NewBuild(ctx, req)
		if codeOf(err) != "BUILD_CACHE_INVALID" || b == nil || len(b.Steps) != 0 {
			t.Fatalf("missing/truncated cache silently rebuilt: %v", err)
		}
		if err := b.Remove(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exePath, exe, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	publisher := *first
	publisher.Dir = t.TempDir()
	publisher.Executable = filepath.Join(publisher.Dir, exeName())
	if err := os.WriteFile(publisher.Executable, exe, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := publisher.saveBuild(req.CacheDir); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate publisher did not refuse entry collision: %v", err)
	}
	recordAfter, recordErr := os.ReadFile(recordPath)
	exeAfter, exeErr := os.ReadFile(exePath)
	if recordErr != nil || exeErr != nil || !bytes.Equal(recordAfter, original) || !bytes.Equal(exeAfter, exe) {
		t.Fatal("duplicate publisher changed the original entry")
	}
	exe[len(exe)-1] ^= 1 // Same-size tampering must fail by hash, not just length.
	if err := os.WriteFile(exePath, exe, 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := NewBuild(ctx, req)
	if codeOf(err) != "BUILD_CACHE_INVALID" || b == nil || len(b.Steps) != 0 {
		t.Fatalf("tampered cache executable: %v", err)
	}
	if err := b.Remove(); err != nil {
		t.Fatal(err)
	}

	req.Defines = []string{"TSGK_FAULT_OMIT_EDIT"}
	fault, err := NewBuild(ctx, req)
	if err != nil || fault.Reuse.Hit || fault.Identity == identity || fault.Reuse.Key == first.Reuse.Key || len(fault.Steps) == 0 {
		t.Fatalf("fault reused a normal build: %v", err)
	}
	if err := fault.Remove(); err != nil {
		t.Fatal(err)
	}
	// A cache hit must not bypass fresh compiler/source validation.
	req.Defines, req.CompilerID.SHA256 = nil, "wrong"
	if _, err := NewBuild(ctx, req); codeOf(err) != "TOOL_IDENTITY_MISMATCH" {
		t.Fatalf("cached compiler bypass: %v", err)
	}
	req.CompilerID.SHA256 = sum
	runtimeCopy := t.TempDir()
	copyTree(t, rt, runtimeCopy)
	runtimeLib := filepath.Join(runtimeCopy, "lib", "src", "lib.c")
	if err := os.WriteFile(runtimeLib, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	req.Runtime = runtimeCopy
	if b, err := NewBuild(ctx, req); codeOf(err) != "RUNTIME_MISMATCH" {
		t.Fatalf("cached runtime bypass: %v", err)
	} else if b != nil {
		if err := b.Remove(); err != nil {
			t.Fatal(err)
		}
	}
	req.Runtime = rt
	req.Grammar = append([]kit.NativeInput(nil), req.Grammar...)
	req.Grammar[0].SHA256 = "wrong"
	if b, err := NewBuild(ctx, req); codeOf(err) != "SOURCE_MISMATCH" {
		t.Fatalf("cached source bypass: %v", err)
	} else if b != nil {
		if err := b.Remove(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildCacheRecordBounds(t *testing.T) {
	req := testBuildRequest(t.TempDir(), "", "", nil, "cc", kit.ToolIdentity{})
	req.CacheDir = t.TempDir()
	b := &Build{Dir: t.TempDir(), Identity: "expected", env: []string{"PATH=compiler", "TMP=one"}}
	if hit, err := b.reuseBuild(req, nil); err != nil || hit {
		t.Fatalf("empty cache: %v", err)
	}
	key := b.Reuse.Key
	b.env[1] = "TMP=two"
	if _, err := b.reuseBuild(req, nil); err != nil || b.Reuse.Key != key {
		t.Fatal("temporary path changed the cache key")
	}
	b.env = append(b.env, "SDKROOT=other")
	if _, err := b.reuseBuild(req, nil); err != nil || b.Reuse.Key == key {
		t.Fatal("effective environment did not change the cache key")
	}
	entry := filepath.Join(req.CacheDir, b.Reuse.Key)
	if err := os.Mkdir(entry, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte(`{"schema":null}`), []byte(`{"schema":"x","schema":"x"}`), []byte(`{"unknown":true}`), make([]byte, buildCacheRecordLimit+1)} {
		if data != nil {
			if err := os.WriteFile(filepath.Join(entry, "build.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if hit, err := b.reuseBuild(req, nil); err == nil || hit {
			t.Fatal("partial, malformed or oversized cache became a hit or miss")
		}
	}
	full := t.TempDir()
	for i := range buildCacheEntries {
		if err := os.Mkdir(filepath.Join(full, fmt.Sprint(i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.saveBuild(full); err == nil || err.Error() != "cache entry limit reached" {
		t.Fatalf("full cache did not refuse at the entry bound: %v", err)
	}
}
