package native

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func codeOf(err error) string {
	var ne *Error
	if errors.As(err, &ne) {
		return ne.Code
	}
	return ""
}

// S05-A11: a changed parser, define set, runtime, compiler identity or source gives a new
// build identity or a refusal, and a modified executable is never run.
func TestBuildIdentity(t *testing.T) {
	rt, cc := nativeTools(t)
	base := fixtureBuild(t, "plain")
	if fault := fixtureBuild(t, "plain", "TSGK_FAULT_OMIT_EDIT"); fault.Identity == base.Identity {
		t.Fatal("a define must change the build identity")
	}
	sum, n := digestOf(t, cc)
	compiler := kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: n}
	work := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	request := func(root string, g []kit.NativeInput) BuildRequest {
		return testBuildRequest(work, rt, root, g, cc, compiler)
	}

	changed := t.TempDir()
	copyTree(t, fixtureRoot(t, "plain"), changed)
	parser := filepath.Join(changed, "src", "parser.c")
	data, _ := os.ReadFile(parser)
	os.WriteFile(parser, append(data, "\n/* changed */\n"...), 0o644)
	g := fixtureGrammar(t, "plain")
	if _, err := NewBuild(ctx, request(changed, g)); codeOf(err) != "SOURCE_MISMATCH" {
		t.Fatalf("changed parser bytes under the old identity: %v", err)
	}
	g[0].SHA256, g[0].Bytes = digestOf(t, parser)
	nb, err := NewBuild(ctx, request(changed, g))
	if err != nil || nb.Identity == base.Identity {
		t.Fatalf("changed parser must build under a new identity: %v", err)
	}

	bad := compiler
	bad.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	r := request(fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"))
	r.CompilerID = bad
	if _, err := NewBuild(ctx, r); codeOf(err) != "TOOL_IDENTITY_MISMATCH" {
		t.Fatalf("compiler identity: %v", err)
	}

	runtimeCopy := t.TempDir()
	copyTree(t, rt, runtimeCopy)
	lib := filepath.Join(runtimeCopy, "lib", "src", "lib.c")
	data, _ = os.ReadFile(lib)
	os.WriteFile(lib, append(data, ' '), 0o644)
	r = request(fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"))
	r.Runtime = runtimeCopy
	if _, err := NewBuild(ctx, r); codeOf(err) != "RUNTIME_MISMATCH" {
		t.Fatalf("runtime: %v", err)
	}
	r = request(fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"))
	r.Symbol = "tree_sitter_x); system(\"x"
	if _, err := NewBuild(ctx, r); codeOf(err) != "SYMBOL_INVALID" {
		t.Fatalf("symbol: %v", err)
	}

	// A stale or replaced executable is rejected before launch.
	tampered := *nb
	data, _ = os.ReadFile(nb.Executable)
	tampered.Executable = filepath.Join(t.TempDir(), "driver-copy")
	os.WriteFile(tampered.Executable, append(data, 0), 0o755)
	res := tampered.RunCase(ctx, testContext("native-parse-edit"), kit.IncrementalCase{ID: "x", Encoding: kit.EncodingUTF8}, []byte("a = 1;"))
	if res.ExecutionStatus != kit.StatusNotRun || res.Code != "EXECUTABLE_MISMATCH" || res.Process != nil {
		t.Fatalf("tampered executable: %s %s", res.ExecutionStatus, res.Code)
	}
	if err := nb.Remove(); err != nil {
		t.Fatal(err)
	}
}

// A host compiler reached through a link (Ubuntu /usr/bin/gcc -> gcc-13) is identified
// and run as the file it resolves to; a dangling link is TOOL_MISSING with its cause.
func TestBuildCompilerLink(t *testing.T) {
	rt, cc := nativeTools(t)
	dir := t.TempDir()
	link := filepath.Join(dir, "cc-link"+filepath.Ext(cc))
	if err := os.Symlink(cc, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sum, n := digestOf(t, cc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // stop at the first compiler step: only the refusal before it matters here
	req := testBuildRequest(dir, rt, fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"), link, kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: n})
	b, err := NewBuild(ctx, req)
	var ne *Error
	if b == nil || !errors.As(err, &ne) || ne.Code != "CANCELLED" {
		t.Fatalf("linked compiler refused before its first step: %v", err)
	}
	req.Compiler = filepath.Join(dir, "dangling")
	if err := os.Symlink(filepath.Join(dir, "absent"), req.Compiler); err != nil {
		t.Fatal(err)
	}
	b, err = NewBuild(context.Background(), req)
	if b != nil || !errors.As(err, &ne) || ne.Code != "TOOL_MISSING" || ne.Cause == nil {
		t.Fatalf("dangling compiler link: %v", err)
	}
	msg := buildFailure(err, b)
	if !strings.Contains(msg, "TOOL_MISSING") || !strings.Contains(msg, "absent") {
		t.Fatalf("build failure hides its cause: %q", msg)
	}
	failed := &Build{Steps: []BuildStep{{Name: "parser", Stderr: "parser.c:1: error: boom"}}}
	if msg := buildFailure(refuse(kit.KindIO, "BUILD_FAILED", errors.New("parser exit 1")), failed); !strings.Contains(msg, "parser stderr: parser.c:1: error: boom") {
		t.Fatalf("step stderr missing: %q", msg)
	}
}
