package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCorpusOriginalRootJunctionAlias(t *testing.T) {
	dir := t.TempDir()
	root, alias := filepath.Join(dir, "source"), filepath.Join(dir, "alias")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "junction.ps1")
	if err := os.WriteFile(script, []byte("param([string]$Link,[string]$Target)\n$ErrorActionPreference='Stop'\nNew-Item -ItemType Junction -Path $Link -Target $Target | Out-Null\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if b, err := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-File", script, "-Link", alias, "-Target", root).CombinedOutput(); err != nil {
		t.Fatalf("task-owned junction: %v %s", err, b)
	}
	inside, outside := filepath.Join(alias, "new-output"), filepath.Join(dir, "new-output")
	for _, paths := range [][2]string{{inside, outside}, {outside, inside}} {
		if err := executeCorpus("unused", "unused", paths[0], paths[1], root, "unused"); err == nil || err.Error() != "CORPUS_WRITE_INSIDE_SOURCE_ROOT" {
			t.Fatalf("execution alias boundary: %v", err)
		}
	}
	if err := replayCorpus("unused", root, "unused", "unused", inside); err == nil || err.Error() != "CORPUS_WRITE_INSIDE_SOURCE_ROOT" {
		t.Fatalf("replay alias boundary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new-output")); !os.IsNotExist(err) {
		t.Fatal("rejected alias output was created")
	}
}
