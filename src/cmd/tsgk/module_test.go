package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	modulePath    = "github.com/wotjr1649/tree-sitter-grammar-kit"
	moduleVersion = "v0.0.0-20261004000000-000000000000"
)

// moduleZip builds the module zip a consumer would download for the checked-out revision:
// every tracked file under the module path and version prefix, as a module proxy serves it.
// It rejects files a default Go product must not carry: executables, native libraries and
// objects.
func moduleZip(t *testing.T) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = repoRoot(t)
	list, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := 0
	for _, name := range strings.Split(strings.TrimSuffix(string(list), "\x00"), "\x00") {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".lib", ".pdb", ".wasm", ".node":
			t.Fatalf("binary in the module source: %s", name)
		}
		data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(modulePath + "@" + moduleVersion + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
		files++
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if files == 0 || buf.Len() > 500<<20 {
		t.Fatalf("module zip: %d files, %d bytes", files, buf.Len())
	}
	return buf.Bytes()
}

func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// S08-A10: a separate Go module requires the kit by module path and pseudo-version
// through a file module proxy (the module source representation a consumer downloads,
// with the pinned golang.org/x/sys taken from the module cache) — no replace, no
// checkout path. Built with no network and run with no PATH, its kit.Qualify result is
// byte-identical to `tsgk qualify` on the same inputs.
func TestModuleProxyConsumer(t *testing.T) {
	root := repoRoot(t)
	proxy, cache, module := t.TempDir(), t.TempDir(), t.TempDir()
	modDir := filepath.Join(proxy, filepath.FromSlash(modulePath), "@v")
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, modDir, map[string]string{
		"list": moduleVersion + "\n", moduleVersion + ".info": `{"Version":"` + moduleVersion + `","Time":"2026-10-04T00:00:00Z"}`,
		moduleVersion + ".mod": string(gomod), moduleVersion + ".zip": string(moduleZip(t))})
	// the pinned dependency from the local module cache (fetched by the CI download step)
	gomodcache := strings.TrimSpace(string(goRun(t, root, "env", "GOMODCACHE")))
	src := filepath.Join(gomodcache, "cache", "download", "golang.org", "x", "sys", "@v")
	dst := filepath.Join(proxy, "golang.org", "x", "sys", "@v")
	os.MkdirAll(dst, 0o755)
	for _, f := range []string{"v0.48.0.info", "v0.48.0.mod", "v0.48.0.zip"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatalf("pinned golang.org/x/sys not in the module cache (run the pinned module download first): %v", err)
		}
		os.WriteFile(filepath.Join(dst, f), data, 0o644)
	}
	os.WriteFile(filepath.Join(dst, "list"), []byte("v0.48.0\n"), 0o644)
	main, err := os.ReadFile(filepath.Join(root, "src", "testdata", "consumer", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, module, map[string]string{"main.go": string(main),
		"go.mod": "module example.invalid/tsgkconsumer\n\ngo 1.27.1\n\nrequire " + modulePath + " " + moduleVersion + "\n"})
	consumer := filepath.Join(module, "consumer"+exeSuffix())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", consumer, ".")
	build.Dir = module
	// -modcacherw: the module cache lives in a t.TempDir, which Unix cleanup could not
	// remove if Go left it read-only.
	build.Env = goEnv("GOPROXY="+fileURL(proxy), "GOFLAGS=-mod=mod -modcacherw", "GOSUMDB=off", "GONOSUMDB=", "GOPRIVATE=", "GOMODCACHE="+cache)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("consumer build through the module proxy: %v\n%s", err, out)
	}
	sum, _ := os.ReadFile(filepath.Join(module, "go.sum"))
	if !strings.Contains(string(sum), modulePath+" "+moduleVersion+" h1:") {
		t.Fatalf("the consumer did not resolve the kit module by version:\n%s", sum)
	}
	inv := filepath.Join(root, "src", "contracts", "qualification-c1.json")
	host := t.TempDir()
	cand := strings.Repeat("c", 40)
	offline := []string{"PATH=", "SystemRoot=" + os.Getenv("SystemRoot")}
	_, line, _ := runBin(t, offline, consumer, "qualify", inv, cand, "linux-amd64", host)
	parts := strings.SplitN(strings.TrimSpace(line), "\t", 3)
	if len(parts) != 3 || parts[0] != "qualify" || parts[1] != "-" {
		t.Fatalf("consumer qualify: %q", line)
	}
	code, stdout, stderr := runBin(t, offline, buildCLI(t), "qualify", "--inventory", inv, "--candidate", cand, "--host", "linux-amd64="+host)
	if code != exitFail || strings.TrimSuffix(stdout, "\n") != parts[2] {
		t.Fatalf("module consumer and CLI differ (exit %d %s)", code, stderr)
	}
}
