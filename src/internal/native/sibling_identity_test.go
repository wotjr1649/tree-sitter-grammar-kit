package native

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
)

func TestRuntimeSiblingIdentityZeroChain(t *testing.T) {
	b := fixtureBuild(t, "plain")
	_, cc := nativeTools(t)
	source, err := filepath.Abs(filepath.Join("..", "..", "testdata", "native", "sibling_identity.c"))
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "sibling-identity")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	args := []string{"-O2", "-std=c11", "-I", "runtime/lib/include", "-I", "runtime/lib/src", source, "lib.o", "parser.o", "-o", executable}
	if b.Sanitize {
		args = append([]string{"-fsanitize=address,undefined", "-fno-omit-frame-pointer", "-fno-sanitize-recover=undefined"}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	spec := runner.Spec{Path: cc, Args: args, Dir: b.Dir, Env: b.env, StdoutBytes: buildOutput, StderrBytes: buildOutput, Wall: buildWall, Grace: buildGrace, Memory: runner.Memory{Bytes: buildMemory, Hard: runtime.GOOS != "darwin"}, CgroupParent: b.cgroup}
	result, err := runner.Run(ctx, spec)
	if err != nil || !result.Succeeded() {
		t.Fatalf("identity fixture build: %v %s %s", err, result.Status, result.Stderr)
	}
	spec.Path, spec.Args, spec.Wall = executable, nil, 10*time.Second
	spec.StdoutBytes, spec.StderrBytes = 4096, 4096
	result, err = runner.Run(ctx, spec)
	if err != nil || !result.Succeeded() {
		t.Fatalf("identity fixture run: %v %s %s", err, result.Status, result.Stderr)
	}
	t.Log(string(result.Stdout))
}
