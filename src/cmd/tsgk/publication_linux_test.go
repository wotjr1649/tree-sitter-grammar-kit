package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishLinuxHasNoTemporaryLeaf(t *testing.T) {
	parent := t.TempDir()
	foreign := filepath.Join(parent, ".tsgk-out-foreign.tmp")
	testHookBeforeLink = func() {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Fatalf("temporary source gained a pathname: %v %v", entries, err)
		}
		if err := os.WriteFile(foreign, []byte("unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeLink = nil }()
	final := filepath.Join(parent, "결과-é.json")
	code, err := publish(final, t.TempDir(), []byte("owned result"))
	if code != "" || err != nil {
		t.Fatalf("publication: %s %v", code, err)
	}
	for path, want := range map[string]string{final: "owned result", foreign: "unrelated"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("publication changed %s: %q %v", filepath.Base(path), data, err)
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 2 {
		t.Fatalf("publication left entries: %v %v", entries, err)
	}
}

func TestPublishLinuxWriteFailureClosesUnnamedFile(t *testing.T) {
	parent := t.TempDir()
	testHookBeforeOutputWrite = func(file *os.File) {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeOutputWrite = nil }()
	code, err := publish(filepath.Join(parent, "result.json"), t.TempDir(), []byte("result"))
	if code != "OUTPUT_CLEANUP_FAILED" || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write/close failure lost: %s %v", code, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("write failure published or left a temporary entry: %v %v", entries, err)
	}
}

func TestPublishLinuxRejectsNonBasename(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"", ".", "..", "../outside", "dir/file", "/absolute"} {
		if code, err := publishOutput(root, name, []byte("invalid")); code != "OUTPUT_INVALID" || !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid final %q: %s %v", name, code, err)
		}
	}
	entries, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer entries.Close()
	names, err := entries.Readdirnames(-1)
	if err != nil || len(names) != 0 {
		t.Fatalf("invalid target changed directory: %v %v", names, err)
	}
}
