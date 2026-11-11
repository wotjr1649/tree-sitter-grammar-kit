package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedRootInput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readRoot(root, "input.json", 2)
	if err != nil || string(got) != "{}" {
		t.Fatalf("valid bounded input: got=%q err=%v", got, err)
	}
	if _, err := readRoot(root, "../input.json", 2); err == nil {
		t.Fatal("parent traversal accepted")
	}
	if _, err := readRoot(root, "input.json", 1); err == nil {
		t.Fatal("oversized input accepted")
	}
}
