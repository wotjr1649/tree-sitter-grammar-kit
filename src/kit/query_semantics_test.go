package kit

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSemanticAPIFixedDenominatorBeforeApproval(t *testing.T) {
	reg, parsed := semanticRegistrationFixture(t)
	registration, parsed := semanticEncodeRegistration(t, reg, parsed)
	profile := semanticProfileBytes(t, parsed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := VerifyQuerySemantics(ctx, QuerySemanticsRequest{Evidence: EvidenceRequest{Root: t.TempDir()}, Profile: profile, Registration: registration})
	if err == nil || result.Tuple.Required != 24 || result.Tuple.Unprocessed != 24 || result.Semantic != AssessBlocked || result.Complete {
		t.Fatalf("denominator vanished before missing root/approval: %v %+v", err, result)
	}
	legacy, err := VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{})
	if err != nil || legacy.Tuple.Required != 0 || legacy.Evidence != nil || legacy.Assessment != AssessBlocked {
		t.Fatalf("legacy ABSENT behavior changed: %v %+v", err, legacy)
	}
}

func TestSemanticAPIBorrowedRunPreservesBudgetAndRoot(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	registration, profile := semanticEncodeRegistration(t, reg, profile)
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	b := semanticTestBudget(t)
	if e := b.charge(7, 128, "outer"); e != nil {
		t.Fatal(e)
	}
	res, err := verifyQuerySemanticsInRun(b.r.caller, QuerySemanticsRequest{Profile: semanticProfileBytes(t, profile), Registration: registration}, b.r, &semanticReplayScope{root: root})
	if err == nil || res.Tuple.Required != 24 || res.Work != b.work || b.work <= 7 || b.logical <= 128 || res.Complete {
		t.Fatalf("borrowed semantic run reset: %v %+v", err, res)
	}
	if e := b.r.check(); e != nil {
		t.Fatalf("borrowed call cancelled outer run: %v", e)
	}
	if _, err := root.Stat("."); err != nil {
		t.Fatalf("borrowed call closed outer root: %v", err)
	}
}

func TestSemanticSnapshotPhysicalIdentity(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "oracle"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rootPath, "oracle", "member.bin")
	if err := os.WriteFile(path, []byte("same bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	child, err := root.OpenRoot("oracle")
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	dir, err := root.Lstat("oracle")
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.Lstat("oracle/member.bin")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &evidenceSnapshot{directories: map[string]fs.FileInfo{"oracle": dir}, observed: map[string]fs.FileInfo{"oracle/member.bin": file}}
	b := semanticTestBudget(t)
	if e := b.semanticSnapshotIdentity(root, child, snapshot); e != nil {
		t.Fatal(e)
	}
	if err := os.Rename(path, filepath.Join(rootPath, "old-member.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("same bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, file.ModTime(), file.ModTime()); err != nil {
		t.Fatal(err)
	}
	if e := b.semanticSnapshotIdentity(root, child, snapshot); e == nil || e.Code != "SOURCE_CHANGED" {
		t.Fatalf("equal hash/size/mtime replaced physical identity: %v", e)
	}
}
