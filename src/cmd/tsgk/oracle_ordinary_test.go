package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func ordinaryTestFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, size), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func ordinaryTestRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root, dir
}

func ordinaryCode(err error) string {
	var ne *native.Error
	if errors.As(err, &ne) {
		return ne.Kind + "/" + ne.Code
	}
	return "<" + errString(err) + ">"
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

// countingOpener is the independent measure of what the loader really opened and read: every open
// adds the size of the file that is read to the end.
type countingOpener struct {
	open  ordinaryOpener
	opens uint64
	bytes uint64
}

func (c *countingOpener) fn() ordinaryOpener {
	return func() (*os.File, os.FileInfo, error) {
		f, info, err := c.open()
		if err == nil {
			c.opens++
			c.bytes += uint64(info.Size())
		}
		return f, info, err
	}
}

func TestOrdinaryLedgerLimitsAcceptTheLimitAndRejectOneMore(t *testing.T) {
	ctx := context.Background()
	root, dir := ordinaryTestRoot(t)
	ordinaryTestFile(t, dir, "f", 100)
	read := func(l *ordinaryLedger) error {
		_, _, err := l.readFile(ctx, "f", ordinaryRootOpener(root, dir, "f"))
		return err
	}
	// One 100-byte file costs one entry, work 64+1, logical 3*101, read 100.
	base := kit.OrdinaryPlanLimits()
	for _, tc := range []struct {
		name   string
		mutate func(*kit.OrdinaryLimits, int64)
	}{
		{"entries", func(l *kit.OrdinaryLimits, d int64) { l.Entries = uint64(1 + d) }},
		{"work", func(l *kit.OrdinaryLimits, d int64) { l.Work = uint64(64 + 1 + d) }},
		{"logical", func(l *kit.OrdinaryLimits, d int64) { l.Logical = uint64(3*101 + d) }},
		{"read", func(l *kit.OrdinaryLimits, d int64) { l.ReadBytes = uint64(100 + d) }},
		{"file", func(l *kit.OrdinaryLimits, d int64) { l.FileBytes = uint64(100 + d) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := base
			tc.mutate(&limits, 0)
			l := &ordinaryLedger{limits: limits, start: ordinaryClock()}
			if err := read(l); err != nil {
				t.Fatalf("limit exactly reached must be accepted: %v", err)
			}
			limits = base
			tc.mutate(&limits, -1)
			l = &ordinaryLedger{limits: limits, start: ordinaryClock()}
			err := read(l)
			want := "RESOURCE_LIMIT/ORDINARY_RESOURCE_LIMIT"
			if tc.name == "file" {
				want = "RESOURCE_LIMIT/ORDINARY_FILE_LIMIT"
			}
			if got := ordinaryCode(err); got != want {
				t.Fatalf("limit-1 must be rejected with %s, got %s", want, got)
			}
		})
	}
	t.Run("entries-accumulate", func(t *testing.T) {
		l := &ordinaryLedger{limits: base, start: ordinaryClock()}
		l.limits.Entries = 2
		for i := 0; i < 2; i++ {
			if err := read(l); err != nil {
				t.Fatalf("entry %d: %v", i, err)
			}
		}
		if got := ordinaryCode(read(l)); got != "RESOURCE_LIMIT/ORDINARY_RESOURCE_LIMIT" {
			t.Fatalf("third entry must be rejected, got %s", got)
		}
	})
}

func TestOrdinaryLedgerFileLimitIsSixteenMiB(t *testing.T) {
	ctx := context.Background()
	root, dir := ordinaryTestRoot(t)
	limit := int(kit.OrdinaryPlanLimits().FileBytes)
	ordinaryTestFile(t, dir, "exact", limit)
	ordinaryTestFile(t, dir, "over", limit+1)
	if _, _, err := newOrdinaryLedger().readFile(ctx, "exact", ordinaryRootOpener(root, dir, "exact")); err != nil {
		t.Fatalf("16 MiB file must be accepted: %v", err)
	}
	l := newOrdinaryLedger()
	if _, _, err := l.readFile(ctx, "over", ordinaryRootOpener(root, dir, "over")); ordinaryCode(err) != "RESOURCE_LIMIT/ORDINARY_FILE_LIMIT" {
		t.Fatalf("16 MiB + 1 file must be rejected, got %v", err)
	}
	if l.read != 0 {
		t.Fatalf("a rejected oversize file must not be charged as read: %d", l.read)
	}
}

func TestOrdinaryLedgerWallBoundary(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1000, 0)
	old := ordinaryClock
	ordinaryClock = func() time.Time { return now }
	t.Cleanup(func() { ordinaryClock = old })
	l := newOrdinaryLedger()
	now = now.Add(120 * time.Second)
	if err := l.charge(ctx, 1, 0, 0, 0); err != nil {
		t.Fatalf("exactly 120 s must be accepted: %v", err)
	}
	now = now.Add(time.Nanosecond)
	if got := ordinaryCode(l.charge(ctx, 1, 0, 0, 0)); got != "RESOURCE_LIMIT/ORDINARY_WALL_LIMIT" {
		t.Fatalf("120 s + 1 ns must be rejected, got %s", got)
	}
}

func TestOrdinaryLedgerKeepsTheFirstFailure(t *testing.T) {
	root, dir := ordinaryTestRoot(t)
	ordinaryTestFile(t, dir, "f", 10)
	l := newOrdinaryLedger()
	l.limits.Logical = 5
	_, _, first := l.readFile(context.Background(), "f", ordinaryRootOpener(root, dir, "f"))
	if ordinaryCode(first) != "RESOURCE_LIMIT/ORDINARY_RESOURCE_LIMIT" {
		t.Fatalf("setup failure: %v", first)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.charge(canceled, 1, 0, 0, 0); err != first {
		t.Fatalf("a later cancellation replaced the first failure: %v", err)
	}
	totals := [4]uint64{l.work, l.logical, l.read, l.entries}
	if err := l.charge(context.Background(), 1, 0, 0, 0); err != first || totals != [4]uint64{l.work, l.logical, l.read, l.entries} {
		t.Fatalf("a failed ledger must stay failed and unchanged: %v %v", err, totals)
	}
	fresh := newOrdinaryLedger()
	if err := fresh.charge(canceled, 1, 0, 0, 0); ordinaryCode(err) != "CANCELLED/CANCELLED" {
		t.Fatalf("a cancelled context must report cancellation first: %v", err)
	}
}

func TestOrdinaryVerifyBindsThePathToTheFileThatWasRead(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, dir string)
		reject bool
	}{
		{"unchanged", func(t *testing.T, dir string) {}, false},
		{"same-size-content-changed", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "f"), bytes.Repeat([]byte{'y'}, 100), 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"grown", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "f"), bytes.Repeat([]byte{'x'}, 101), 0o600); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"replaced-by-identical-bytes", func(t *testing.T, dir string) {
			ordinaryTestFile(t, dir, "g", 100)
			if err := os.Remove(filepath.Join(dir, "f")); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(dir, "g"), filepath.Join(dir, "f")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"deleted", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "f")); err != nil {
				t.Fatal(err)
			}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, dir := ordinaryTestRoot(t)
			ordinaryTestFile(t, dir, "f", 100)
			l := newOrdinaryLedger()
			_, bound, err := l.readFile(ctx, "f", ordinaryRootOpener(root, dir, "f"))
			if err != nil {
				t.Fatal(err)
			}
			tc.change(t, dir)
			err = l.verify(ctx, bound)
			if tc.reject && ordinaryCode(err) != "INVALID_INPUT/ORDINARY_INPUT_CHANGED" {
				t.Fatalf("a file changed between the check and the use must be rejected, got %v", err)
			}
			if !tc.reject && err != nil {
				t.Fatalf("an unchanged file must verify: %v", err)
			}
		})
	}
}

func TestOrdinaryRootOpenerStaysInsideTheRoot(t *testing.T) {
	root, dir := ordinaryTestRoot(t)
	ordinaryTestFile(t, dir, "f", 4)
	for _, rel := range []string{"../f", "/f", "a/../f", "a\\f", "C:f", ""} {
		if ordinaryCLIPath(rel) {
			t.Fatalf("path %q must be refused", rel)
		}
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ordinaryTestFile(t, outside, "secret", 4)
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, _, err := newOrdinaryLedger().readFile(context.Background(), "link", ordinaryRootOpener(root, dir, "link")); err == nil {
		t.Fatal("a link out of the root was read")
	}
}

func TestOrdinaryExternalOpenerRefusesInputRootsAndLinks(t *testing.T) {
	ctx := context.Background()
	inputRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inside := ordinaryTestFile(t, inputRoot, "doc", 4)
	ok := ordinaryTestFile(t, outside, "doc", 4)
	if _, _, err := newOrdinaryLedger().readFile(ctx, "inside", ordinaryExternalOpener(inside, []string{inputRoot})); ordinaryCode(err) != "INVALID_INPUT/ORDINARY_INPUT_INSIDE_CASE_ROOT" {
		t.Fatalf("a trusted document inside an input root must be refused: %v", err)
	}
	if _, _, err := newOrdinaryLedger().readFile(ctx, "outside", ordinaryExternalOpener(ok, []string{inputRoot})); err != nil {
		t.Fatalf("a document outside every input root must be read: %v", err)
	}
	if err := os.Symlink(ok, filepath.Join(outside, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	if _, _, err := newOrdinaryLedger().readFile(ctx, "link", ordinaryExternalOpener(filepath.Join(outside, "link"), []string{inputRoot})); ordinaryCode(err) != "INVALID_INPUT/ORDINARY_PATH_ALIAS" {
		t.Fatalf("a link leaf must be refused as an alias: %v", err)
	}
}

// The ledger is cumulative: a read and its verification are both charged, and the totals are at least
// what was independently counted. Dropping a charged term makes this test fail.
func TestOrdinaryLedgerChargesAtLeastWhatWasActuallyRead(t *testing.T) {
	ctx := context.Background()
	root, dir := ordinaryTestRoot(t)
	l := newOrdinaryLedger()
	var counters []*countingOpener
	var bounds []*ordinaryBound
	for i, size := range []int{1, 4096, 70000, 0} {
		name := string(rune('a' + i))
		ordinaryTestFile(t, dir, name, size)
		counter := &countingOpener{open: ordinaryRootOpener(root, dir, name)}
		counters = append(counters, counter)
		_, bound, err := l.readFile(ctx, name, counter.fn())
		if err != nil {
			t.Fatal(err)
		}
		bounds = append(bounds, bound)
	}
	for _, bound := range bounds {
		if err := l.verify(ctx, bound); err != nil {
			t.Fatal(err)
		}
	}
	var opens, read uint64
	for _, counter := range counters {
		opens += counter.opens
		read += counter.bytes
	}
	if opens != 8 || read != 2*(1+4096+70000) {
		t.Fatalf("independent count changed: opens=%d read=%d", opens, read)
	}
	if l.entries < opens || l.read < read || l.work < 65*opens || l.logical < read {
		t.Fatalf("ledger is below what was actually read: ledger entries=%d read=%d work=%d logical=%d, actual opens=%d bytes=%d", l.entries, l.read, l.work, l.logical, opens, read)
	}
}

func TestOrdinaryRecordFlagsRequireEveryInput(t *testing.T) {
	for name, args := range map[string][]string{
		"only-root":      {"--ordinary-root", "x"},
		"no-shard":       {"--ordinary-root", "x", "--ordinary-witness", "w", "--ordinary-plan", "p", "--ordinary-profile", "p", "--ordinary-assets", "a", "--ordinary-registry-sha256", "s", "--ordinary-registration", "r"},
		"empty-shard":    {"--ordinary-root", "x", "--ordinary-witness", "w", "--ordinary-plan", "p", "--ordinary-profile", "p", "--ordinary-assets", "a", "--ordinary-registry-sha256", "s", "--ordinary-registration", "r", "--ordinary-shard", ""},
		"with-semantics": {"--query-semantics", "q", "--ordinary-root", "x", "--ordinary-witness", "w", "--ordinary-plan", "p", "--ordinary-profile", "p", "--ordinary-assets", "a", "--ordinary-registry-sha256", "s", "--ordinary-registration", "r", "--ordinary-shard", "id"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			full := append([]string{"--root", "r", "--profile", "p", "--runtime", "rt", "--out", "o", "--work", "w", "--tool", "cc=c"}, args...)
			if code := runNative(context.Background(), "oracle", full, &stdout, &stderr); code != exitUsage || !strings.Contains(stderr.String(), "USAGE") {
				t.Fatalf("code=%d stderr=%s", code, stderr.String())
			}
		})
	}
	var stderr bytes.Buffer
	if code := runNative(context.Background(), "incremental", []string{"--ordinary-root", "x"}, &bytes.Buffer{}, &stderr); code != exitUsage {
		t.Fatalf("incremental must not take ordinary flags: %d", code)
	}
}

func TestOrdinaryRecordKeepsAnEstablishedFailAheadOfLaterErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ordinary   bool
		assessment string
		written    bool
		result     int
		want       int
	}{
		{"fail-written", true, kit.AssessFail, true, exitFail, exitFail},
		{"fail-then-output-failure", true, kit.AssessFail, false, exitFail, exitFail},
		{"fail-then-cancellation", true, kit.AssessFail, true, exitCanceled, exitFail},
		{"fail-then-resource-stop", true, kit.AssessFail, true, exitBlocked, exitFail},
		{"fail-then-cancellation-and-output-failure", true, kit.AssessFail, false, exitCanceled, exitFail},
		{"blocked-written", true, kit.AssessBlocked, true, exitBlocked, exitBlocked},
		{"blocked-output-failure", true, kit.AssessBlocked, false, exitBlocked, exitIO},
		{"non-ordinary-fail-output-failure", false, kit.AssessFail, false, exitFail, exitIO},
		{"non-ordinary-fail-cancellation", false, kit.AssessFail, true, exitCanceled, exitCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := recordExit(tc.ordinary, tc.assessment, tc.written, func() int { return tc.result }); got != tc.want {
				t.Fatalf("exit=%d want %d", got, tc.want)
			}
		})
	}
}
