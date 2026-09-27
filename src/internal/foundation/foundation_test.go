package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRepositoryFiles(t *testing.T) {
	if err := CheckFiles(os.DirFS(repository(t))); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryGit(t *testing.T) {
	root := repository(t)
	if _, err := os.Lstat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		t.Skip("source-only: Git index checks not applicable")
	} else if err != nil {
		t.Fatal(err)
	}
	if err := CheckGit(root); err != nil {
		t.Fatal(err)
	}
}

func TestNegativeControls(t *testing.T) {
	cases := []struct {
		name, diagnostic string
		mutate           func(fstest.MapFS)
	}{
		{"missing-doc", "required file docs/specs/scope.md", func(f fstest.MapFS) { delete(f, "docs/specs/scope.md") }},
		{"wrong-module", "module identity", func(f fstest.MapFS) { f["go.mod"].Data = []byte("module example.org/wrong\n") }},
		{"long-agents", "60 nonblank", func(f fstest.MapFS) {
			f["AGENTS.md"].Data = append(f["AGENTS.md"].Data, []byte(strings.Repeat("extra\n", 61))...)
		}},
		{"missing-ignore", "root ignore missing", func(f fstest.MapFS) { f[".gitignore"].Data = []byte("/bin/\n") }},
		{"broken-link", "broken link", func(f fstest.MapFS) { f["README.md"].Data = []byte("[missing](docs/missing.md)\n") }},
		{"outside-source", "outside src", func(f fstest.MapFS) { f["leak.go"] = &fstest.MapFile{Data: []byte("package leak\n")} }},
		{"nested-module", "boundary violation", func(f fstest.MapFS) { f["src/go.mod"] = &fstest.MapFile{Data: []byte("module wrong\n")} }},
		{"normalized-bytes", "byte preservation missing", func(f fstest.MapFS) { f[".gitattributes"].Data = []byte("* text=auto\n") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := make(fstest.MapFS)
			for _, name := range required {
				data, err := os.ReadFile(filepath.Join(repository(t), filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				files[name] = &fstest.MapFile{Data: data}
			}
			if err := CheckFiles(files); err != nil {
				t.Fatalf("control baseline: %v", err)
			}
			tc.mutate(files)
			if err := CheckFiles(files); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation was not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
}

func TestSourceOnlyDoesNotDiscoverParentGit(t *testing.T) {
	root := t.TempDir()
	// A source export nested in a checkout must remain a filesystem-only target.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("invalid parent marker\n"), 0600); err != nil {
		t.Fatal(err)
	}
	export := filepath.Join(root, "export")
	for _, name := range required {
		data, err := os.ReadFile(filepath.Join(repository(t), filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if name == "README.md" {
			data = append(data, []byte("\n`[inline example](missing.md)`\n\n```text\n[fenced example](missing.md)\n```\n")...)
		}
		target := filepath.Join(export, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", "")
	if err := CheckFiles(os.DirFS(export)); err != nil {
		t.Fatal(err)
	}
	if err := CheckGit(export); err == nil || !strings.Contains(err.Error(), "explicit checkout") {
		t.Fatalf("source-only checkout guard: %v", err)
	}
}

func TestCGOFreeDependencies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-test", "-json", "./src/...")
	cmd.Dir = repository(t)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0
	for {
		var pkg struct {
			ImportPath string
			CgoFiles   []string
		}
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if len(pkg.CgoFiles) != 0 || pkg.ImportPath == "runtime/cgo" {
			t.Fatalf("CGO dependency: %s", pkg.ImportPath)
		}
	}
	if count == 0 {
		t.Fatal("empty dependency audit")
	}
	t.Logf("audited %d dependency records; no CgoFiles/runtime/cgo", count)
}
