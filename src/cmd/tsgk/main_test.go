package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var fixture = map[string]string{
	"tree-sitter.json":               `{"grammars":[{"name":"alpha","path":"alpha"}]}`,
	"common/define.js":               "module.exports = (d) => d;\n",
	"alpha/grammar.js":               "module.exports = require('../common/define')({});\n",
	"alpha/src/grammar.json":         "{}",
	"alpha/src/scanner.c":            "#include \"tree_sitter/parser.h\"\r\n",
	"alpha/src/tree_sitter/parser.h": "#pragma once\n",
	"alpha/queries/highlights.scm":   "(x) @y\n",
	"alpha/test/corpus/basic.txt":    "==\nx\n",
	"corpus-like/App/Form1.cs":       "class F { void A() { InitializeComponent(); } }\n",
	"corpus-like/App/App.csproj":     `<Project><ItemGroup><Compile Include="Form1.cs" /></ItemGroup></Project>`,
	"corpus-like/Keys/key.snk":       "never read",
	"corpus-like/bin/Debug/skip.cs":  "excluded",
	"corpus-like/Data/q.sql":         "select 1;\n",
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func cli(t *testing.T, ctx context.Context, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(ctx, args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestExitCodes(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, fixture)
	ctx := context.Background()
	for _, tc := range []struct {
		args   []string
		code   int
		stderr string
	}{
		{nil, 2, "USAGE"},
		{[]string{"verify", "--root", root}, 2, "UNSUPPORTED_COMMAND"},
		{[]string{"schema", "check"}, 2, "UNSUPPORTED_COMMAND"},
		{[]string{"bogus"}, 2, "UNKNOWN_COMMAND"},
		{[]string{"inspect", "--root", root, "--profile", "p.json"}, 2, "PROFILE_UNSUPPORTED"},
		{[]string{"identity", "--root", root, "--grammar", "../x"}, 2, "GRAMMAR_INVALID"},
		{[]string{"identity", "--root", root, "--grammar", "."}, 2, "NO_GRAMMAR_SELECTED"},
		{[]string{"identity", "--root", root, "--encoding-profile", "latin1"}, 2, "USAGE"},
		{[]string{"inspect", "--root", root, "extra"}, 2, "USAGE"},
		{[]string{"inspect", "--root", filepath.Join(root, "missing")}, 2, "ROOT_NOT_FOUND"},
	} {
		code, _, stderr := cli(t, ctx, tc.args...)
		if code != tc.code || !strings.Contains(stderr, tc.stderr) {
			t.Errorf("%v: exit %d stderr %q", tc.args, code, stderr)
		}
	}
	code, stdout, _ := cli(t, ctx, "identity", "--root", root, "--grammar", "alpha", "--declare", "alpha/grammar.js=utf-8")
	var rep map[string]any
	if code != 0 || json.Unmarshal([]byte(stdout), &rep) != nil || rep["schema"] != "tsgk-report/r1" || rep["set_sha256"] == "" {
		t.Fatalf("identity: %d %s", code, stdout)
	}
	code, stdout, _ = cli(t, ctx, "identity", "--root", root, "--grammar", "../x")
	if code != 2 || json.Unmarshal([]byte(stdout), &rep) != nil || rep["execution_status"] != "NOT_RUN" {
		t.Fatalf("failure report must be complete JSON on stdout: %s", stdout)
	}
	code, stdout, _ = cli(t, ctx, "corpus", "--root", filepath.Join(root, "corpus-like"))
	if code != 0 || !strings.Contains(stdout, `"PRESENCE_ONLY"`) || strings.Contains(stdout, "skip.cs") {
		t.Fatalf("corpus: %d %s", code, stdout)
	}
	big := t.TempDir()
	writeTree(t, big, map[string]string{"grammar.js": "module.exports = {};\n"})
	f, err := os.Create(filepath.Join(big, "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(16777216 + 1)
	f.Close()
	if code, _, stderr := cli(t, ctx, "identity", "--root", big, "--file", "grammar.json=grammar"); code != 3 || !strings.Contains(stderr, "FILE_BYTES_LIMIT") {
		t.Fatalf("resource limit exit: %d %s", code, stderr)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if code, _, stderr := cli(t, cancelled, "inspect", "--root", root, "--grammar", "alpha"); code != 130 || !strings.Contains(stderr, "CANCELLED") {
		t.Fatalf("cancel exit: %d %s", code, stderr)
	}
}

// S01-A10: publication is no-clobber, outside the input, and never follows an existing link.
func TestOutPublication(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, fixture)
	ctx := context.Background()
	outDir := t.TempDir()
	out := filepath.Join(outDir, "inspect.json")
	code, stdout, _ := cli(t, ctx, "inspect", "--root", root, "--grammar", "alpha", "--out", out)
	_, direct, _ := cli(t, ctx, "inspect", "--root", root, "--grammar", "alpha")
	got, err := os.ReadFile(out)
	if code != 0 || stdout != "" || err != nil || string(got) != direct {
		t.Fatalf("published result differs: %d %v", code, err)
	}
	if code, _, stderr := cli(t, ctx, "inspect", "--root", root, "--grammar", "alpha", "--out", out); code != 4 || !strings.Contains(stderr, "OUTPUT_EXISTS") {
		t.Fatalf("existing output: %d %s", code, stderr)
	}
	if again, _ := os.ReadFile(out); !bytes.Equal(again, got) {
		t.Fatal("existing output modified")
	}
	inside := filepath.Join(root, "alpha", "result.json")
	if code, _, stderr := cli(t, ctx, "inspect", "--root", root, "--grammar", "alpha", "--out", inside); code != 4 || !strings.Contains(stderr, "OUTPUT_INSIDE_INPUT") {
		t.Fatalf("inside input: %d %s", code, stderr)
	}
	if _, err := os.Lstat(inside); !os.IsNotExist(err) {
		t.Fatal("output created inside input")
	}
	failed := filepath.Join(outDir, "failed.json")
	if code, _, _ := cli(t, ctx, "identity", "--root", root, "--grammar", "../x", "--out", failed); code != 2 {
		t.Fatal("failed run must not succeed")
	}
	if _, err := os.Lstat(failed); !os.IsNotExist(err) {
		t.Fatal("failure report published as a result")
	}
	t.Run("link", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target.json")
		link := filepath.Join(outDir, "link.json")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if code, _, stderr := cli(t, ctx, "inspect", "--root", root, "--grammar", "alpha", "--out", link); code != 4 || !strings.Contains(stderr, "OUTPUT_EXISTS") {
			t.Fatalf("link output: %d %s", code, stderr)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatal("link target written")
		}
	})
	t.Run("collision", func(t *testing.T) {
		dest := filepath.Join(outDir, "race.json")
		testHookBeforeLink = func() { os.WriteFile(dest, []byte("winner"), 0o644) }
		defer func() { testHookBeforeLink = nil }()
		code, err := publish(dest, root, []byte("loser"))
		if code != "OUTPUT_EXISTS" || err == nil {
			t.Fatalf("collision: %s %v", code, err)
		}
		if data, _ := os.ReadFile(dest); string(data) != "winner" {
			t.Fatal("concurrently created output clobbered")
		}
		entries, _ := os.ReadDir(outDir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".tsgk-out-") {
				t.Fatalf("temporary file left: %s", e.Name())
			}
		}
	})
}

func goEnv(extra ...string) []string {
	env := []string{}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "GOWORK", "GOTOOLCHAIN", "CGO_ENABLED", "GOPROXY", "GOFLAGS":
			continue
		}
		env = append(env, kv)
	}
	return append(env, append([]string{"GOWORK=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOPROXY=off"}, extra...)...)
}

func goRun(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir, cmd.Env = dir, goEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tsgk"+exeSuffix())
	goRun(t, repoRoot(t), "build", "-o", bin, "./src/cmd/tsgk")
	return bin
}

func exeSuffix() string {
	if filepath.Separator == '\\' {
		return ".exe"
	}
	return ""
}

func runBin(t *testing.T, env []string, bin string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), out.String(), errb.String()
	} else if err != nil {
		t.Fatal(err)
	}
	return 0, out.String(), errb.String()
}

// S01-A01/A11/A12: a module outside the checkout uses only the public API and gets the
// same semantic results and guards as the CLI; the CLI runs with no executable path.
func TestExternalConsumerAndCLI(t *testing.T) {
	bin := buildCLI(t)
	module := t.TempDir()
	tmpl, err := os.ReadFile(filepath.Join(repoRoot(t), "src", "testdata", "consumer", "go.mod.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "src", "testdata", "consumer", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(src, []byte("/src/internal")) {
		t.Fatal("consumer must not import internal packages")
	}
	gomod := strings.ReplaceAll(string(tmpl), "{{KIT}}", filepath.ToSlash(repoRoot(t)))
	writeTree(t, module, map[string]string{"go.mod": gomod, "main.go": string(src)})
	consumer := filepath.Join(module, "consumer"+exeSuffix())
	goRun(t, module, "build", "-o", consumer, ".")
	root := t.TempDir()
	writeTree(t, root, fixture)
	offline := []string{"PATH=", "SystemRoot=" + os.Getenv("SystemRoot")}
	for _, grammar := range []string{"alpha", ".", "../escape"} {
		_, lines, _ := runBin(t, offline, consumer, root, grammar)
		api := map[string][2]string{}
		for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
			parts := strings.SplitN(line, "\t", 3)
			api[parts[0]] = [2]string{parts[1], parts[2]}
		}
		for _, cmd := range []string{"inspect", "identity"} {
			code, stdout, stderr := runBin(t, offline, bin, cmd, "--root", root, "--grammar", grammar)
			if strings.TrimSuffix(stdout, "\n") != api[cmd][1] {
				t.Fatalf("%s %s: CLI and API results differ\nCLI %s\nAPI %s", cmd, grammar, stdout, api[cmd][1])
			}
			if api[cmd][0] == "-" {
				if code != 0 {
					t.Fatalf("%s %s exit %d: %s", cmd, grammar, code, stderr)
				}
			} else if code == 0 || !strings.Contains(stderr, strings.Split(api[cmd][0], "/")[1]) {
				t.Fatalf("%s %s guard differs: API %s CLI %d %s", cmd, grammar, api[cmd][0], code, stderr)
			}
		}
	}
}

// S01-A11: the product closure contains no process, network or plugin capability.
func TestOfflineClosure(t *testing.T) {
	out := goRun(t, repoRoot(t), "list", "-deps", "./src/kit", "./src/cmd/tsgk")
	for _, pkg := range strings.Fields(string(out)) {
		switch pkg {
		case "os/exec", "net", "net/http", "plugin", "crypto/tls", "net/url":
			t.Fatalf("forbidden capability in product closure: %s", pkg)
		}
	}
}
