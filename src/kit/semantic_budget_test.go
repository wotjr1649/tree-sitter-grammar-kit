package kit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func semanticBudgetRun(t *testing.T) *run {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	r, e := startRun(ctx, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	return r
}

func semanticBudgetRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRunReadCeilingAndActualCount(t *testing.T) {
	r := &run{totalRead: 4, readCeiling: 9}
	if remaining, e := r.remainingRead(20, "doc"); e != nil || remaining != 5 {
		t.Fatalf("remainingRead = %d, %v; want 5, nil", remaining, e)
	}
	if e := r.chargeRead(5, 20, "doc"); e != nil || r.totalRead != 9 {
		t.Fatalf("exact boundary charge = %d, %v; want 9, nil", r.totalRead, e)
	}
	if remaining, e := r.remainingRead(20, "doc"); e != nil || remaining != 0 {
		t.Fatalf("zero remaining = %d, %v; want 0, nil", remaining, e)
	}
	if e := r.chargeRead(1, 20, "doc"); e == nil || e.Code != "TOTAL_BYTES_LIMIT" || r.totalRead != 10 {
		t.Fatalf("over-ceiling charge = %d, %v; want 10 and TOTAL_BYTES_LIMIT", r.totalRead, e)
	}
	r.totalRead = ^uint64(0) - 1
	if e := r.chargeRead(2, ^uint64(0), "doc"); e == nil || e.Code != "TOTAL_BYTES_LIMIT" || r.totalRead != ^uint64(0) {
		t.Fatalf("overflow charge = %d, %v; want saturated count and TOTAL_BYTES_LIMIT", r.totalRead, e)
	}
}

func TestRunCheckInheritsParentWall(t *testing.T) {
	parent, child := semanticBudgetRun(t), semanticBudgetRun(t)
	child.parent = parent
	parent.cancel()
	if e := child.check(); e == nil || e.Kind != KindResourceLimit || e.Code != "WALL_LIMIT" {
		t.Fatalf("child ignored parent wall: %v", e)
	}
}

func TestGuardReadFileSemanticCeilingBoundaryGrowthAndActualCount(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exact"), []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	r.readCeiling = 3
	guard := newGuard(root)
	before, err := root.Lstat("exact")
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	testHookBeforeRead = func(string) { reads++ }
	t.Cleanup(func() { testHookBeforeRead = nil })
	stats, e := guard.readFile(r, "exact", before, 10, 10, 10)
	if e != nil || stats.size != 3 || string(stats.content) != "abc" || r.totalRead != 3 || reads != 1 {
		t.Fatalf("exact-boundary read: stats=%+v bytes=%d reads=%d err=%v", stats, r.totalRead, reads, e)
	}
	if err := os.WriteFile(filepath.Join(dir, "too-large"), []byte("ab"), 0600); err != nil {
		t.Fatal(err)
	}
	tooLarge, err := root.Lstat("too-large")
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	testHookOpen = func(string) { opened = true }
	t.Cleanup(func() { testHookOpen = nil })
	r.totalRead, r.readCeiling = 0, 1
	if _, e := guard.readFile(r, "too-large", tooLarge, 10, 10, 0); e == nil || e.Code != "TOTAL_BYTES_LIMIT" || opened || r.totalRead != 0 {
		t.Fatalf("known length over ceiling: bytes=%d opened=%v err=%v", r.totalRead, opened, e)
	}

	if err := os.WriteFile(filepath.Join(dir, "growth"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	growthInfo, err := root.Lstat("growth")
	if err != nil {
		t.Fatal(err)
	}
	r.totalRead, r.readCeiling = 0, 10
	mutated := false
	testHookBeforeRead = func(name string) {
		if name == "growth" && !mutated {
			mutated = true
			f, openErr := os.OpenFile(filepath.Join(dir, "growth"), os.O_APPEND|os.O_WRONLY, 0)
			if openErr == nil {
				_, openErr = f.Write([]byte("b"))
				_ = f.Close()
			}
			if openErr != nil {
				t.Errorf("append during read: %v", openErr)
			}
		}
	}
	_, e = guard.readFile(r, "growth", growthInfo, 10, 10, 0)
	if e == nil || e.Code != "SOURCE_CHANGED" || r.totalRead != 1 {
		t.Fatalf("growth read: bytes=%d err=%v; want one confirmed byte and SOURCE_CHANGED", r.totalRead, e)
	}

	if err := os.WriteFile(filepath.Join(dir, "partial"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	partialInfo, err := root.Lstat("partial")
	if err != nil {
		t.Fatal(err)
	}
	r.totalRead, r.readCeiling = 0, 0
	mutated = false
	testHookBeforeRead = func(name string) {
		if name == "partial" && !mutated {
			mutated = true
			f, openErr := os.OpenFile(filepath.Join(dir, "partial"), os.O_APPEND|os.O_WRONLY, 0)
			if openErr == nil {
				_, openErr = f.Write([]byte("b"))
				_ = f.Close()
			}
			if openErr != nil {
				t.Errorf("append before hardMax read: %v", openErr)
			}
		}
	}
	_, e = guard.readFile(r, "partial", partialInfo, 1, 10, 0)
	if e == nil || e.Code != "FILE_BYTES_LIMIT" || r.totalRead != 2 {
		t.Fatalf("partial over-hardMax read: bytes=%d err=%v; want 2 and FILE_BYTES_LIMIT", r.totalRead, e)
	}
}

func TestSemanticEntryBudgetNamespacesAndHostLimit(t *testing.T) {
	firstDir, secondDir := t.TempDir(), t.TempDir()
	for _, dir := range []string{firstDir, secondDir} {
		if err := os.WriteFile(filepath.Join(dir, "entry"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first, second := semanticBudgetRoot(t, firstDir), semanticBudgetRoot(t, secondDir)
	ledger := newSemanticEntryBudget(1)
	hostFirst, hostSecond := uint64(0), uint64(0)
	if e := ledger.begin(first, &hostFirst, 10); e != nil {
		t.Fatal(e)
	}
	info, err := first.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if added, e := ledger.record(first, "entry", info); e != nil || !added || ledger.used != 1 || hostFirst != 1 {
		t.Fatalf("first namespace entry: added=%v used=%d host=%d err=%v", added, ledger.used, hostFirst, e)
	}
	if e := ledger.begin(second, &hostSecond, 10); e != nil {
		t.Fatal(e)
	}
	info, err = second.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, e := ledger.record(second, "entry", info); e == nil || e.Code != "SEMANTIC_ENTRY_LIMIT" || ledger.used != 1 || hostSecond != 0 {
		t.Fatalf("cumulative second namespace: used=%d host=%d err=%v", ledger.used, hostSecond, e)
	}

	alias, err := os.OpenRoot(firstDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = alias.Close() })
	if e := ledger.begin(alias, &hostSecond, 10); e == nil || e.Code != "SEMANTIC_ROOT_OVERLAP" {
		t.Fatalf("reused root identity accepted: %v", e)
	}

	hostLimited := newSemanticEntryBudget(10)
	hostCount := uint64(1)
	if e := hostLimited.begin(second, &hostCount, 1); e != nil {
		t.Fatal(e)
	}
	info, err = second.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, e := hostLimited.record(second, "entry", info); e == nil || e.Code != "FILE_COUNT_LIMIT" || hostLimited.used != 0 || hostCount != 1 {
		t.Fatalf("host cap: used=%d host=%d err=%v", hostLimited.used, hostCount, e)
	}
}

func TestSemanticEntryBudgetNestedDuplicateAndIdentityChange(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "evidence"), 0700); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(dir, "evidence", "entry")
	if err := os.WriteFile(entryPath, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	outer := semanticBudgetRoot(t, dir)
	child, err := outer.OpenRoot("evidence")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Close() })
	ledger := newSemanticEntryBudget(10)
	hostCount := uint64(0)
	if e := ledger.begin(outer, &hostCount, 10); e != nil {
		t.Fatal(e)
	}
	outerInfo, err := outer.Lstat("evidence/entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, e := ledger.record(outer, "evidence/entry", outerInfo); e != nil {
		t.Fatal(e)
	}
	if e := ledger.bindChild(outer, "evidence", child); e != nil {
		t.Fatal(e)
	}
	childInfo, err := child.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if added, e := ledger.record(child, "entry", childInfo); e != nil || added || ledger.used != 1 || hostCount != 1 {
		t.Fatalf("nested duplicate: added=%v used=%d host=%d err=%v", added, ledger.used, hostCount, e)
	}

	replacement := filepath.Join(dir, "evidence", "replacement")
	if err := os.WriteFile(replacement, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, entryPath); err != nil {
		t.Fatal(err)
	}
	changedInfo, err := child.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, e := ledger.record(child, "entry", changedInfo); e == nil || e.Code != "SOURCE_CHANGED" || ledger.used != 1 || hostCount != 1 {
		t.Fatalf("changed nested identity: used=%d host=%d err=%v", ledger.used, hostCount, e)
	}
}

func TestReadDirWithoutSemanticLedgerKeepsLegacyCount(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	var count uint64
	_, e := newGuard(root).readDir(r, ".", &count, 1)
	if e == nil || e.Code != "FILE_COUNT_LIMIT" || count != 2 || r.semanticEntries != nil {
		t.Fatalf("legacy readDir: count=%d ledger=%v err=%v", count, r.semanticEntries != nil, e)
	}
}

func TestReadDirSemanticLedgerDeduplicatesPhysicalEntries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	r.eligibility = &eligibilityBudget{r: r}
	ledger := newSemanticEntryBudget(10)
	hostCount := uint64(0)
	if e := ledger.begin(root, &hostCount, 10); e != nil {
		t.Fatal(e)
	}
	r.semanticEntries = ledger
	var localCount uint64
	for i := 0; i < 2; i++ {
		if _, e := newGuard(root).readDir(r, ".", &localCount, 10); e != nil {
			t.Fatal(e)
		}
	}
	if localCount != 4 || ledger.used != 2 || hostCount != 2 {
		t.Fatalf("local=%d global=%d host=%d; want 4, 2, 2", localCount, ledger.used, hostCount)
	}
	if r.eligibility.work == 0 || r.eligibility.logical == 0 {
		t.Fatalf("semantic readDir was not charged: work=%d logical=%d", r.eligibility.work, r.eligibility.logical)
	}
}

func TestReadDirSemanticBudgetPrechargesBatchAndEntryWork(t *testing.T) {
	for _, tc := range []struct {
		name        string
		workLeft    uint64
		wantLogical uint64
	}{
		{name: "batch", workLeft: 1},
		{name: "entry", workLeft: 2, wantLogical: 128 * 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "entry"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			root := semanticBudgetRoot(t, dir)
			r := semanticBudgetRun(t)
			r.eligibility = &eligibilityBudget{r: r, work: eligibilityWorkLimit - tc.workLeft}
			ledger := newSemanticEntryBudget(10)
			hostCount := uint64(0)
			if e := ledger.begin(root, &hostCount, 10); e != nil {
				t.Fatal(e)
			}
			r.semanticEntries = ledger
			var localCount uint64
			_, e := newGuard(root).readDir(r, ".", &localCount, 10)
			if e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" || localCount != 0 || ledger.used != 0 || hostCount != 0 || r.eligibility.logical != tc.wantLogical {
				t.Fatalf("precharge boundary: local=%d used=%d host=%d work=%d logical=%d err=%v", localCount, ledger.used, hostCount, r.eligibility.work, r.eligibility.logical, e)
			}
		})
	}
}

func TestReadDirLedgerRejectsReplacementAtWalkIdentityRecheck(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry")
	if err := os.WriteFile(entryPath, []byte("same"), 0600); err != nil {
		t.Fatal(err)
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	r.eligibility = &eligibilityBudget{r: r}
	ledger := newSemanticEntryBudget(10)
	hostCount := uint64(0)
	if e := ledger.begin(root, &hostCount, 10); e != nil {
		t.Fatal(e)
	}
	r.semanticEntries = ledger
	var localCount uint64
	if _, e := newGuard(root).readDir(r, ".", &localCount, 10); e != nil {
		t.Fatal(e)
	}
	if localCount != 1 || ledger.used != 1 || hostCount != 1 {
		t.Fatalf("initial walk: local=%d used=%d host=%d", localCount, ledger.used, hostCount)
	}
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("same"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, entryPath); err != nil {
		t.Fatal(err)
	}
	actualWalkInfo, err := root.Lstat("entry")
	if err != nil {
		t.Fatal(err)
	}
	if added, e := ledger.record(root, "entry", actualWalkInfo); e == nil || e.Code != "SOURCE_CHANGED" || added || ledger.used != 1 || hostCount != 1 {
		t.Fatalf("replacement at actual walk identity check: added=%v used=%d host=%d err=%v", added, ledger.used, hostCount, e)
	}
}

func TestReadDirSemanticSortWorkIsReserved(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"common-prefix-second", "common-prefix-first"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root := semanticBudgetRoot(t, dir)
	r := semanticBudgetRun(t)
	r.eligibility = &eligibilityBudget{r: r, work: eligibilityWorkLimit - 259}
	r.semanticEntries = newSemanticEntryBudget(10)
	var host, local uint64
	if e := r.semanticEntries.begin(root, &host, 10); e != nil {
		t.Fatal(e)
	}
	if _, e := newGuard(root).readDir(r, ".", &local, 10); e == nil || e.Code != "ELIGIBILITY_WORK_LIMIT" || e.Path != "semantic-directory-sort" || local != 2 || host != 2 {
		t.Fatalf("directory sorting escaped the shared work budget: local=%d host=%d err=%v", local, host, e)
	}
}
