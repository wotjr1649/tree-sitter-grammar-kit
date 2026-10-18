package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPublishRetainsTemporaryLeafIdentity(t *testing.T) {
	parent, input := t.TempDir(), t.TempDir()
	blocked := false
	testHookBeforeLink = func() {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 1 || entries[0].IsDir() {
			t.Fatalf("temporary leaf fixture: %v %v", entries, err)
		}
		name := filepath.Join(parent, entries[0].Name())
		err = os.Rename(name, filepath.Join(parent, "moved-temp"))
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			blocked = true
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("unrelated replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeLink = nil }()
	final := filepath.Join(parent, "결과-é.json")
	code, err := publish(final, input, []byte("owned result"))
	if err != nil || code != "" {
		t.Fatalf("publication: %s %v", code, err)
	}
	data, err := os.ReadFile(final)
	if err != nil || string(data) != "owned result" || !blocked {
		t.Fatalf("temporary leaf lost ownership: %q blocked=%v err=%v", data, blocked, err)
	}
	if err := os.Rename(final, filepath.Join(parent, "released.json")); err != nil {
		t.Fatal("rename after guard release:", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "released.json" {
		t.Fatalf("publication left temporary entries: %v %v", entries, err)
	}
}

func TestPublishWindowsWriteFailureRetainsUnverifiedTemp(t *testing.T) {
	parent := t.TempDir()
	testHookBeforeOutputWrite = func(file *os.File) {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeOutputWrite = nil }()
	code, err := publish(filepath.Join(parent, "result.json"), t.TempDir(), []byte("result"))
	if code != "OUTPUT_CLEANUP_FAILED" || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write/cleanup error lost: %s %v", code, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].IsDir() || entries[0].Name() == "result.json" {
		t.Fatalf("unverified temp was removed by pathname or output was published: %v %v", entries, err)
	}
}

func TestPublishWindowsTempCollisionPreservesExisting(t *testing.T) {
	parent := t.TempDir()
	var collision string
	testHookBeforeOutputCreate = func(name string) {
		collision = filepath.Join(parent, name)
		if err := os.WriteFile(collision, []byte("unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { testHookBeforeOutputCreate = nil }()
	code, err := publish(filepath.Join(parent, "result.json"), t.TempDir(), []byte("replacement"))
	if code != "OUTPUT_PUBLISH_FAILED" || !errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
		t.Fatalf("temp collision not refused: %s %v", code, err)
	}
	data, err := os.ReadFile(collision)
	if err != nil || string(data) != "unrelated" {
		t.Fatalf("existing temp was adopted or cleaned up: %q %v", data, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temp collision left output: %v %v", entries, err)
	}
}

func TestPublishWindowsOutputNames(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"", ".", "..", "../escape", `a\b`, "a/b", "a:b", "CON", "COM1", "nul.txt", "trailing.", "trailing "} {
		code, err := publishOutput(root, name, []byte("invalid"))
		if code != "OUTPUT_INVALID" || !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("invalid basename: %q %s %v", name, code, err)
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid basename left entries: %v %v", entries, err)
	}
	testHookBeforeLink = func() {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 1 || strings.HasPrefix(entries[0].Name(), ".") {
			t.Fatalf("dot-prefixed target shares the temporary namespace: %v %v", entries, err)
		}
		if _, err := root.Lstat(".output"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("final name visible before publication: %v", err)
		}
	}
	defer func() { testHookBeforeLink = nil }()
	code, err := publishOutput(root, ".output", []byte("valid"))
	if err != nil || code != "" {
		t.Fatalf("dot-prefixed basename: %s %v", code, err)
	}
	if data, err := root.ReadFile(".output"); err != nil || string(data) != "valid" {
		t.Fatalf("dot-prefixed bytes: %q %v", data, err)
	}
}

func TestPublishWindowsShortAliasFinal(t *testing.T) {
	fixture := t.TempDir()
	long := filepath.Join(fixture, ".tsgk-out-short-alias-fixture.tmp")
	if err := os.WriteFile(long, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	find := func(t *testing.T, path string) windows.Win32finddata {
		t.Helper()
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			t.Fatal(err)
		}
		var data windows.Win32finddata
		h, err := windows.FindFirstFile(name, &data)
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.FindClose(h); err != nil {
			t.Fatal(err)
		}
		return data
	}
	data := find(t, long)
	alias := windows.UTF16ToString(data.AlternateFileName[:])
	if alias == "" || strings.EqualFold(alias, filepath.Base(long)) {
		t.Fatal("8.3 alias fixture unavailable; volume settings were not changed")
	}
	if err := os.Remove(long); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual long-name alias: %s", alias)
	for _, name := range []string{alias, "ASTATUS.TMP", "bstatus.tmp", ".output"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			final := filepath.Join(parent, name)
			testHookBeforeLink = func() {
				entries, err := os.ReadDir(parent)
				if err != nil || len(entries) != 1 {
					t.Fatalf("short-name publication fixture: %v %v", entries, err)
				}
				if _, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("8.3 final name visible before publication: %v", err)
				}
				source := entries[0].Name()
				parts := strings.Split(source, ".")
				if len(parts) != 2 || len(parts[0]) != 8 || parts[1] != "TMP" || strings.ContainsAny(source, " ~") {
					t.Fatalf("temporary name permits a separate short alias: %q", source)
				}
				info := find(t, filepath.Join(parent, source))
				if short := windows.UTF16ToString(info.AlternateFileName[:]); short != "" && !strings.EqualFold(short, source) {
					t.Fatalf("temporary has a second alias: %q %q", source, short)
				}
			}
			defer func() { testHookBeforeLink = nil }()
			code, err := publish(final, t.TempDir(), []byte("complete"))
			if code != "" || err != nil {
				t.Fatalf("short-name publication: %s %v", code, err)
			}
			if data, err := os.ReadFile(final); err != nil || string(data) != "complete" {
				t.Fatalf("short-name final bytes: %q %v", data, err)
			}
		})
	}
}
