//go:build linux || darwin

package native

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBoundSourceRacedFIFO(t *testing.T) {
	if os.Getenv("TSGK_SOURCE_FIFO_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBoundSourceRacedFIFO$", "-test.timeout=5s")
		cmd.Env = append(os.Environ(), "TSGK_SOURCE_FIFO_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("FIFO control: %v timeout=%v %s", err, ctx.Err(), output)
		}
		return
	}
	base := t.TempDir()
	path := filepath.Join(base, "source")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	called := false
	testHookBeforeSourceLeaf = func(_ *os.Root, _ string) {
		called = true
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeSourceLeaf = nil }()
	if _, err := readBoundSource(root, "source", 8); !called || !errors.Is(err, errSourceLink) {
		t.Fatalf("FIFO was not refused: called=%v err=%v", called, err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("foreign FIFO changed: %v %v", info, err)
	}
}
