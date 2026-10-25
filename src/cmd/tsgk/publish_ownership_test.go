package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// The barrier replaces the checked pathname before publication and cleanup.
func TestPublishKeepsParentIdentity(t *testing.T) {
	base, input := t.TempDir(), t.TempDir()
	parent, moved := filepath.Join(base, "output"), filepath.Join(base, "moved")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	var tempName string
	blocked := false
	testHookBeforeLink = func() {
		entries, err := os.ReadDir(parent)
		if runtime.GOOS == "linux" {
			if err != nil || len(entries) != 0 {
				t.Fatalf("unnamed publication fixture: %v %v", entries, err)
			}
			tempName = ".tsgk-out-foreign.tmp"
		} else {
			if err != nil || len(entries) != 1 || entries[0].IsDir() {
				t.Fatalf("temporary publication fixture: %v %v", entries, err)
			}
			tempName = entries[0].Name()
		}
		err = os.Rename(parent, moved)
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(32)) {
			blocked = true // the retained Windows handle prevents directory replacement
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(parent, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, tempName), []byte("unrelated replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeLink = nil }()
	code, err := publish(filepath.Join(parent, "result.json"), input, []byte("owned result"))
	if err != nil || code != "" {
		t.Fatalf("publication: %s %v", code, err)
	}
	owned := moved
	if blocked {
		owned = parent
	} else {
		if data, err := os.ReadFile(filepath.Join(parent, tempName)); err != nil || string(data) != "unrelated replacement" {
			t.Fatalf("replacement entry changed by publication/cleanup: %q %v", data, err)
		}
		if _, err := os.Lstat(filepath.Join(parent, "result.json")); !os.IsNotExist(err) {
			t.Fatal("published in replacement directory")
		}
	}
	if data, err := os.ReadFile(filepath.Join(owned, "result.json")); err != nil || string(data) != "owned result" {
		t.Fatalf("held directory result: %q %v", data, err)
	}
	entries, err := os.ReadDir(owned)
	if err != nil || len(entries) != 1 || entries[0].Name() != "result.json" {
		t.Fatalf("owned temporary file was not cleaned: %v %v", entries, err)
	}
}

func TestPublishChecksEveryRoot(t *testing.T) {
	roots := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	for i, root := range roots {
		code, err := publish(filepath.Join(root, "result.json"), roots[0], []byte("result"), roots[1:]...)
		if code != "OUTPUT_INSIDE_INPUT" || err == nil {
			t.Fatalf("input %d not checked: %s %v", i, code, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("refusal left entries: %v %v", entries, err)
		}
	}
	outside := t.TempDir()
	code, err := publish(filepath.Join(outside, "result.json"), roots[0], []byte("result"), roots[1:]...)
	if code != "" || err != nil {
		t.Fatalf("outside publication: %s %v", code, err)
	}
}

func TestPublishReportsFinalizationFailureAfterPublication(t *testing.T) {
	parent := t.TempDir()
	testHookAfterLink = func(file *os.File) {
		if file != nil {
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			return
		}
		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".tsgk-out-") {
				continue
			}
			name := filepath.Join(parent, entry.Name())
			if err := os.Rename(name, filepath.Join(parent, "retained-temp")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(name, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(name, "unrelated"), []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
		t.Fatal("temporary publication entry missing")
	}
	defer func() { testHookAfterLink = nil }()
	code, err := publish(filepath.Join(parent, "result.json"), t.TempDir(), []byte("complete"))
	want := "OUTPUT_CLEANUP_FAILED"
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		want = "OUTPUT_CLOSE_FAILED"
	}
	if code != want || err == nil || !strings.Contains(err.Error(), "output was published") {
		t.Fatalf("published cleanup failure hidden: %s %v", code, err)
	}
	if data, err := os.ReadFile(filepath.Join(parent, "result.json")); err != nil || string(data) != "complete" {
		t.Fatalf("published result changed after cleanup failure: %q %v", data, err)
	}
}

func TestQualifyRechecksEveryHostBeforePublish(t *testing.T) {
	for _, index := range []int{1, 2} {
		t.Run(string(rune('1'+index)), func(t *testing.T) {
			base, outside := t.TempDir(), t.TempDir()
			hosts := []string{filepath.Join(base, "first"), filepath.Join(base, "second"), filepath.Join(base, "third")}
			args := []string{"qualify", "--inventory", filepath.Join(repoRoot(t), "src", "contracts", "qualification-c1.json"),
				"--candidate", strings.Repeat("a", 40), "--out", filepath.Join(outside, "result.json")}
			for i, platform := range []string{"windows-amd64", "linux-amd64", "darwin-arm64"} {
				if err := os.Mkdir(hosts[i], 0o755); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--host", platform+"="+hosts[i])
			}
			// Change only a host pathname after preflight and analysis have completed.
			testHookBeforePublish = func() {
				if err := os.Remove(hosts[index]); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, hosts[index]); err != nil {
					t.Fatal("host replacement fixture unavailable:", err)
				}
			}
			defer func() { testHookBeforePublish = nil }()
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), args, &stdout, &stderr)
			if code != exitIO || !strings.Contains(stderr.String(), "OUTPUT_INSIDE_INPUT") {
				t.Fatalf("host %d final check: %d %s", index, code, stderr.String())
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("refused publication left entries: %v %v", entries, err)
			}
		})
	}
}

func TestDirectoryReplacementDuringScan(t *testing.T) {
	for _, kind := range []string{"directory", "contained-alias", "outside-alias"} {
		t.Run(kind, func(t *testing.T) {
			input, outside := t.TempDir(), t.TempDir()
			child := filepath.Join(input, "child")
			if err := os.Mkdir(child, 0o755); err != nil {
				t.Fatal(err)
			}
			called := false
			testHookBeforeChildOpen = func(_ *os.Root, name string) {
				if called || name != "child" {
					return
				}
				called = true
				if err := os.Rename(child, filepath.Join(input, "old")); err != nil {
					t.Fatal(err)
				}
				var err error
				switch kind {
				case "directory":
					err = os.Mkdir(child, 0o755)
				case "contained-alias":
					err = os.Symlink("old", child)
				case "outside-alias":
					err = os.Symlink(outside, child)
				}
				if err != nil {
					t.Fatal("replacement fixture unavailable:", err)
				}
			}
			defer func() { testHookBeforeChildOpen = nil }()
			found, err := inside(outside, input)
			if !called || found || err == nil {
				t.Fatalf("changed scan accepted: called=%v found=%v err=%v", called, found, err)
			}
		})
	}
}
