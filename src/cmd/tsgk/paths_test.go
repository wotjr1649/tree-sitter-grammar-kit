package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDirectoryMembership(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dir, root string
		want      bool
	}{{root, root, true}, {child, root, true}, {outside, root, false}, {outside, "", false}} {
		got, err := inside(tc.dir, tc.root)
		if err != nil || got != tc.want {
			t.Fatalf("membership: got %v %v, want %v", got, err, tc.want)
		}
	}
	// Exercise descendant identity lookup directly with real handle-derived FileInfo;
	// a resolved symlink/junction alone would only exercise the old ancestor check.
	f, err := os.Open(child)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	check := func(parents []os.FileInfo, depth, entries int, deadline time.Time) (bool, error) {
		f, err := os.OpenRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		return containsDirectory(f, parents, depth, &entries, deadline)
	}
	if found, err := check([]os.FileInfo{info}, 0, 1, time.Now().Add(pathWall)); !found || err != nil {
		t.Fatalf("descendant identity missed: %v %v", found, err)
	}
	for _, tc := range []struct {
		depth, entries int
		deadline       time.Time
	}{{pathDepth + 1, 1, time.Now().Add(pathWall)}, {0, pathEntries, time.Now().Add(pathWall)}, {0, 1, time.Now().Add(-time.Second)}} {
		if found, err := check(nil, tc.depth, tc.entries, tc.deadline); found || err == nil {
			t.Fatalf("incomplete traversal accepted: %v %v", found, err)
		}
	}
	t.Run("closed-identity", func(t *testing.T) {
		f, err := os.OpenRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		n := 1
		if found, err := containsDirectory(f, nil, 0, &n, time.Now().Add(pathWall)); found || err == nil {
			t.Fatal("unavailable identity accepted")
		}
	})
	t.Run("internal-alias", func(t *testing.T) {
		aliasRoot := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(aliasRoot, "alias")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if found, err := inside(outside, aliasRoot); found || err == nil {
			t.Fatal("internal alias treated as complete traversal")
		}
	})
	if _, err := inside(outside, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing root accepted as outside")
	}
	if _, err := inside(filepath.Join(outside, "missing"), root); err == nil {
		t.Fatal("missing destination parent accepted as outside")
	}
	if code, err := publish(filepath.Join(outside, "new.json"), filepath.Join(root, "missing"), []byte("data")); code != "OUTPUT_PATH_UNVERIFIED" || err == nil {
		t.Fatalf("unknown publication: %s %v", code, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("unknown publication left files: %v %v", entries, err)
	}
}

func TestUnknownInputPathCLI(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "missing")
	doc := filepath.Join(base, "profile.json")
	if err := os.WriteFile(doc, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(base, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"verify", "--root", root, "--expected", doc},
		{"replay", "--input", root, "--profile", doc},
		{"evidence", "verify", "--input", root, "--profile", doc},
		{"qualify", "--inventory", doc, "--candidate", strings.Repeat("a", 40), "--host", "windows/amd64=" + root},
		{"reproduce", "--root", root, "--profile", doc, "--work", work, "--out", filepath.Join(base, "result")},
		{"incremental", "--root", root, "--profile", doc, "--runtime", base, "--tool", "cc=" + doc, "--work", work, "--out", filepath.Join(base, "result")},
		{"oracle", "record", "--root", root, "--profile", doc, "--runtime", base, "--tool", "cc=" + doc, "--work", work, "--out", filepath.Join(base, "result")},
	} {
		code, out, stderr := cli(t, context.Background(), args...)
		if code != exitUsage || out != "" || !strings.Contains(stderr, "INPUT_PATH_UNVERIFIED") {
			t.Fatalf("%s accepted unknown membership: %d %s %s", args[0], code, out, stderr)
		}
	}
	if _, err := os.Lstat(filepath.Join(base, "result")); !os.IsNotExist(err) {
		t.Fatal("unknown path created a result")
	}
}

func TestUnknownExecutionParentCLI(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "input")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"reproduce", "incremental", "oracle"} {
		args := []string{kind}
		if kind == "oracle" {
			args = append(args, "record")
		}
		args = append(args, "--root", root, "--profile", filepath.Join(base, "profile.json"), "--work", filepath.Join(base, "missing"), "--out", filepath.Join(base, "result"))
		if kind != "reproduce" {
			args = append(args, "--runtime", base, "--tool", "cc="+root)
		}
		code, stdout, stderr := cli(t, context.Background(), args...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "INPUT_PATH_UNVERIFIED") {
			t.Fatalf("%s unknown parent classified as inside: %d %s %s", kind, code, stdout, stderr)
		}
	}
}
