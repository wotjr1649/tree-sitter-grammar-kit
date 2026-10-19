package native

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestBoundSourceConsumersUseHeldObject(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-bytes", true: "same-bytes"}[same], func(t *testing.T) {
			original, replacement := t.TempDir(), t.TempDir()
			want := []byte("original")
			other := []byte("replacement")
			if same {
				other = want
			}
			for path, data := range map[string][]byte{filepath.Join(original, "source"): want, filepath.Join(replacement, "source"): other} {
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			root, err := os.OpenRoot(original)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			originalFile, err := os.Open(filepath.Join(original, "source"))
			if err != nil {
				t.Fatal(err)
			}
			identity, statErr := originalFile.Stat()
			if err := errors.Join(statErr, originalFile.Close()); err != nil {
				t.Fatal(err)
			}
			display := replacement
			moved := filepath.Join(t.TempDir(), "moved")
			blocked := false
			if err := os.Rename(original, moved); err != nil {
				if runtime.GOOS != "windows" || (!errors.Is(err, syscall.Errno(5)) && !errors.Is(err, syscall.Errno(32))) {
					t.Fatal(err)
				}
				blocked = true
				t.Log("root rename blocked while handle held; distinct display root still tests no reopening")
			} else {
				display = original
				if err := os.Mkdir(original, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(original, "source"), other, 0o600); err != nil {
					t.Fatal(err)
				}
				t.Log("root pathname replaced; opened file must remain in moved original")
			}
			opened := 0
			testHookSourceOpened = func(f *os.File) {
				opened++
				got, err := f.Stat()
				if err != nil || !os.SameFile(identity, got) {
					t.Fatalf("opened replacement object: %v", err)
				}
			}
			defer func() { testHookSourceOpened = nil }()
			digest := sha256.Sum256(want)
			in := kit.NativeInput{Path: "source", SHA256: hex.EncodeToString(digest[:]), Bytes: uint64(len(want))}
			data, err := readCase(display, in, root)
			if err != nil || string(data) != string(want) {
				t.Fatalf("case binding: %q %v", data, err)
			}
			dst := filepath.Join(t.TempDir(), "copy")
			if err := copyVerified(display, in.Path, dst, in.SHA256, int64(in.Bytes), root); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(dst); err != nil || string(data) != string(want) || opened != 2 {
				t.Fatalf("grammar binding: %q %v opens=%d", data, err, opened)
			}
			bad := in
			bad.Bytes--
			if _, err := readCase(display, bad, root); err == nil {
				t.Fatal("case size mismatch accepted")
			}
			bad = in
			bad.SHA256 = strings.Repeat("0", 64)
			if _, err := readCase(display, bad, root); err == nil {
				t.Fatal("case hash mismatch accepted")
			}
			badDst := filepath.Join(t.TempDir(), "must-not-exist")
			if err := copyVerified(display, in.Path, badDst, bad.SHA256, int64(in.Bytes), root); err == nil {
				t.Fatal("unverified grammar copied")
			}
			if _, err := os.Stat(badDst); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("grammar mismatch created destination: %v", err)
			}
			if err := root.Close(); err != nil {
				t.Fatal(err)
			}
			if blocked {
				if err := os.Rename(original, moved); err != nil {
					t.Fatalf("borrowed root was not released: %v", err)
				}
			}
			if data, err := os.ReadFile(filepath.Join(moved, in.Path)); err != nil || string(data) != string(want) {
				t.Fatalf("fallback control must be valid: %q %v", data, err)
			}
			if _, err := readCase(moved, in, root); err == nil {
				t.Fatal("closed case binding fell back to path")
			}
			if err := copyVerified(moved, in.Path, badDst, in.SHA256, int64(in.Bytes), root); err == nil {
				t.Fatal("closed grammar binding fell back to path")
			}
			if _, err := os.Stat(badDst); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("closed grammar binding created destination: %v", err)
			}
		})
	}
}

func TestInputRootsRequireBothRoles(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, roots := range []*InputRoots{{}, {Cases: root}, {Grammar: root}} {
		if err := roots.validate(); err == nil {
			t.Fatal("partial binding accepted")
		}
	}
	if err := (&InputRoots{Cases: root, Grammar: root}).validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("."); err != nil {
		t.Fatalf("borrowed root closed: %v", err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := (&InputRoots{Cases: root, Grammar: root}).validate(); err == nil {
		t.Fatal("closed binding accepted")
	}
}

func TestBoundSourceIdentityAndLimits(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "child", "source"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := readBoundSource(root, "child/source", 8)
	if err != nil || string(data) != "original" {
		t.Fatalf("bound bytes: %q %v", data, err)
	}
	data, err = readBoundSource(root, "child/source", 2)
	if err != nil || string(data) != "ori" {
		t.Fatalf("declared size read limit: %q %v", data, err)
	}
	for _, name := range []string{"", ".", "..", "../outside", "a/../b", "/absolute", "a:b", `a\b`} {
		if _, err := readBoundSource(root, name, 8); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("unsafe name %q: %v", name, err)
		}
	}
	for _, size := range []int64{-1, 1<<63 - 1} {
		if _, err := readBoundSource(root, "child/source", size); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid size %d: %v", size, err)
		}
	}
	if _, err := readBoundSource(root, "child", 8); !errors.Is(err, errSourceLink) {
		t.Fatalf("directory leaf accepted: %v", err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundSource(root, "child/source", 8); err == nil {
		t.Fatal("closed root reopened by path")
	}
	if _, err := readBoundSource(nil, "child/source", 8); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nil root: %v", err)
	}
}

func TestBoundSourceLeafReplacement(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "source")
	if err := os.WriteFile(file, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	called := false
	testHookBeforeSourceLeaf = func(_ *os.Root, name string) {
		called = true
		if name != "source" {
			t.Fatal(name)
		}
		if err := os.Rename(file, filepath.Join(base, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeSourceLeaf = nil }()
	if data, err := readBoundSource(root, "source", 8); !called || err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("unobserved leaf replacement: %q %v called=%v", data, err, called)
	}
	for name, want := range map[string]string{"moved": "original", "source": "replacement"} {
		if data, err := os.ReadFile(filepath.Join(base, name)); err != nil || string(data) != want {
			t.Fatalf("read changed source %s: %q %v", name, data, err)
		}
	}
}

func TestBoundSourceRejectsHardLinks(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "source"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if data, err := readBoundSource(root, "source", 8); err != nil || string(data) != "original" {
		t.Fatalf("single-link source: %q %v", data, err)
	}
	if err := os.Link(filepath.Join(base, "source"), filepath.Join(base, "twin")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source", "twin"} {
		if _, err := readBoundSource(root, name, 8); !errors.Is(err, errSourceLink) {
			t.Fatalf("hard link %s accepted: %v", name, err)
		}
	}
}

func TestBoundSourceRejectsLinks(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "original"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "original", "source"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "original"), filepath.Join(base, "alias")); err != nil {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal(err)
		}
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := readBoundSource(root, "alias/source", 8); !errors.Is(err, errSourceLink) {
		t.Fatalf("directory link accepted: %v", err)
	}
	file := filepath.Join(base, "leaf")
	if err := os.WriteFile(file, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	testHookBeforeSourceLeaf = func(_ *os.Root, _ string) {
		called = true
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(base, "original", "source"), file); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeSourceLeaf = nil }()
	if data, err := readBoundSource(root, "leaf", 8); !called || err == nil {
		t.Fatalf("raced leaf link followed: %q %v", data, err)
	}
	if info, err := os.Lstat(file); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unowned link changed: %v %v", info, err)
	}
}
