package native

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOutputDirectoryWindowsNamesAndAliases(t *testing.T) {
	parent, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	for _, name := range []string{"", ".", "..", "../outside", `a\b`, "a/b", "a:b", "CON", "COM1", "nul.txt", "trailing.", "trailing "} {
		if d, err := createOutputDirectory(parent, name); d != nil || err == nil {
			if d != nil {
				d.Close()
			}
			t.Fatalf("invalid output name accepted: %q %v %v", name, d, err)
		}
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(parent.Name(), "alias")); err != nil {
		t.Fatal("alias collision fixture unavailable:", err)
	}
	if d, err := createOutputDirectory(parent, "alias"); d != nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("alias collision: %v %v", d, err)
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatalf("alias target changed: %v %v", entries, err)
	}
}

func TestOutputDirectoryWindowsDotPrefix(t *testing.T) {
	d, err := newOutputDirectory(filepath.Join(t.TempDir(), ".output"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := createExclusive(d.Root, "member.json", []byte("owned")); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := newOutputDirectory(d.Name(), nil); duplicate != nil || !errors.Is(err, os.ErrExist) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatalf("dot-prefixed output collision: %v %v", duplicate, err)
	}
}
