package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGuardReadDetectorAndStreamSink(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 2*readChunk+37)
	for i := range data {
		data[i] = byte(i*31 + 7)
	}
	if err := os.WriteFile(filepath.Join(dir, "input"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	r.eligibility = &eligibilityBudget{r: r}
	r.readCeiling = uint64(4 * len(data))
	guard := newGuard(root)
	before, err := root.Lstat("input")
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)

	withDetector := sha256.New()
	totalLimit := uint64(4 * len(data))
	stats, e := guard.readFileContent(r, "input", before, uint64(len(data)), totalLimit, 0, true, false, withDetector)
	if e != nil || stats.encoding == nil || stats.content != nil || stats.size != uint64(len(data)) || stats.sha256 != hex.EncodeToString(want[:]) || hex.EncodeToString(withDetector.Sum(nil)) != stats.sha256 {
		t.Fatalf("detector read: stats=%+v sink=%x err=%v", stats, withDetector.Sum(nil), e)
	}
	charged := r.eligibility.logical

	withoutDetector := sha256.New()
	stats, e = guard.readFileContent(r, "input", before, uint64(len(data)), totalLimit, 0, false, false, withoutDetector)
	if e != nil || stats.encoding != nil || stats.content != nil || stats.size != uint64(len(data)) || stats.sha256 != hex.EncodeToString(want[:]) || hex.EncodeToString(withoutDetector.Sum(nil)) != stats.sha256 {
		t.Fatalf("stream read: stats=%+v sink=%x err=%v", stats, withoutDetector.Sum(nil), e)
	}
	if delta := charged - (r.eligibility.logical - charged); delta != uint64(len(data))+1 {
		t.Fatalf("disabling detector saved %d logical bytes, want %d", delta, len(data)+1)
	}
}

func TestGuardStreamRejectsSourceChangeDuringRead(t *testing.T) {
	for _, mutation := range []string{"samefile", "size", "mtime"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input")
			if err := os.WriteFile(path, []byte("original bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			root := semanticBudgetRoot(t, dir)
			r := semanticBudgetRun(t)
			guard := newGuard(root)
			before, err := root.Lstat("input")
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "samefile" {
				oldHook := testHookOpen
				testHookOpen = func(name string) {
					if name != "input" {
						return
					}
					testHookOpen = oldHook
					moved := filepath.Join(dir, "original")
					if err := os.Rename(path, moved); err != nil {
						t.Errorf("move original: %v", err)
						return
					}
					if err := os.WriteFile(path, []byte("replacement!!!"), 0600); err != nil {
						t.Errorf("write replacement: %v", err)
						return
					}
					stamp := before.ModTime()
					if err := os.Chtimes(path, stamp, stamp); err != nil {
						t.Errorf("match replacement mtime: %v", err)
					}
				}
				t.Cleanup(func() { testHookOpen = oldHook })
				_, e := guard.readFileContent(r, "input", before, 100, 100, 0, false, false, sha256.New())
				if e == nil || e.Code != "SOURCE_CHANGED" {
					t.Fatalf("read accepted a replacement with matching size and mtime: %v", e)
				}
				return
			}
			oldHook := testHookBeforeRead
			testHookBeforeRead = func(name string) {
				if name != "input" {
					return
				}
				testHookBeforeRead = oldHook
				if mutation == "size" {
					f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
					if e != nil {
						t.Errorf("open for append: %v", e)
						return
					}
					_, e = f.Write([]byte("!"))
					if closeErr := f.Close(); e == nil {
						e = closeErr
					}
					if e != nil {
						t.Errorf("append during read: %v", e)
					}
					return
				}
				if err := os.WriteFile(path, []byte("changed bytes!"), 0600); err != nil {
					t.Errorf("replace bytes in place: %v", err)
					return
				}
				stamp := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Errorf("change mtime: %v", err)
				}
			}
			t.Cleanup(func() { testHookBeforeRead = oldHook })
			_, e := guard.readFileContent(r, "input", before, 100, 100, 0, false, false, sha256.New())
			if e == nil || e.Code != "SOURCE_CHANGED" {
				t.Fatalf("read accepted changed source: %v", e)
			}
		})
	}
}
