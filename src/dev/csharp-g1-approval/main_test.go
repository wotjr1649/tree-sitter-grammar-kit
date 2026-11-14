package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const firstObject = "9a5dbd010ce2e4e0ba9e977f88c9aaf253c082170b03c67802f96d6c655eff94"

var pendingAssets = []string{"csharp-g1-expected-b03b04-r1.json", "csharp-g1-kind-map-r3.json", "csharp-g1-audit-b03b04-r1.json", "csharp-g1-typeaudit-b03b04-r1.json", "csharp-g1-executed-b03b04-r1.json", "csharp-g1-ledger-r2.json"}

// repoCopy is a root with the tracked G1 assets and the first approval object: the second bundle has no object in it.
func repoCopy(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	contracts := filepath.Join(root, "src", "contracts")
	objects := filepath.Join(contracts, "evidence-pins", "objects")
	if err := os.MkdirAll(objects, 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("../../contracts/csharp-g1-*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no G1 assets: %v", err)
	}
	for _, f := range append(files, filepath.Join("../../contracts/evidence-pins/objects", firstObject)) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(contracts, filepath.Base(f))
		if filepath.Base(f) == firstObject {
			dst = filepath.Join(objects, firstObject)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The first bundle approved and the second one not: -verify says so in the pending line and in its own exit code, which is neither a pass (0)
// nor a missing first object (3). With the second object written, the line is gone and the exit code is 0 (review of PR2, finding 2).
func TestVerifyPendingAssetsHaveTheirOwnExitCode(t *testing.T) {
	root := repoCopy(t)
	var out, errOut bytes.Buffer
	if code := run(root, false, true, false, &out, &errOut); code != exitPendingAssets || exitPendingAssets == exitPending || exitPendingAssets == exitMismatch || exitPendingAssets == 0 {
		t.Fatalf("exit %d (want %d): %q %q", code, exitPendingAssets, out.String(), errOut.String())
	}
	s := out.String()
	if !strings.Contains(s, "APPROVED by object "+firstObject) || !strings.Contains(s, "PENDING_APPROVAL") || !strings.Contains(s, "pending, not approved:") {
		t.Fatalf("stdout %q", s)
	}
	for _, a := range pendingAssets {
		if !strings.Contains(s[strings.Index(s, "pending, not approved:"):], a) {
			t.Errorf("%s is not listed as pending: %q", a, s)
		}
	}
	// the second tuple, confirmed and written: the assets leave the pending list
	out.Reset()
	errOut.Reset()
	if code := run(root, true, false, true, &out, &errOut); code != 0 {
		t.Fatalf("print -b34: exit %d: %s", code, errOut.String())
	}
	sum := sha256.Sum256(out.Bytes())
	name := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(root, "src", "contracts", "evidence-pins", "objects", name), out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := run(root, false, true, false, &out, &errOut); code != 0 {
		t.Fatalf("both bundles approved: exit %d: %q %q", code, out.String(), errOut.String())
	}
	s = out.String()
	if !strings.Contains(s, "APPROVED by object "+firstObject) || !strings.Contains(s, "APPROVED by object "+name+" (scope csharp-g1-b34-bundle, base "+firstObject+")") || strings.Contains(s, "pending, not approved") || strings.Contains(s, "PENDING_APPROVAL") {
		t.Fatalf("stdout %q", s)
	}
	// a tampered asset of the second bundle is a mismatch, not a pending asset
	if err := os.WriteFile(filepath.Join(root, "src", "contracts", pendingAssets[4]), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run(root, false, true, false, &out, &errOut); code != exitMismatch {
		t.Fatalf("a tampered approved asset: exit %d: %q %q", code, out.String(), errOut.String())
	}
}

// Without an approval object -verify reports PENDING_APPROVAL with its own exit code: it is neither a pass nor a mismatch.
func TestVerifyReportsPendingNotPass(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "contracts", "evidence-pins", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := run(root, false, true, false, &out, &errOut)
	if code != exitPending || !strings.Contains(out.String(), "PENDING_APPROVAL") || strings.Contains(out.String(), "APPROVED by") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	if code := run(root, true, true, false, &out, &errOut); code != exitUsage {
		t.Fatalf("both flags: exit %d", code)
	}
}

// -print over the real repository writes the tuple bytes alone on stdout, so a redirect gives the object bytes.
func TestPrintWritesOnlyTheTuple(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run("../../..", true, false, false, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !bytes.HasPrefix(out.Bytes(), []byte(`{"schema":"tsgk-csharp-g1-approval/r1"`)) || bytes.Count(out.Bytes(), []byte("\n")) != 1 || !strings.Contains(errOut.String(), "sha256 ") {
		t.Fatalf("stdout %q, stderr %q", out.String(), errOut.String())
	}
}

// Over the real repository -verify reports the first bundle APPROVED and the second one as PENDING_APPROVAL with its assets listed as
// pending, not approved, or APPROVED with none pending once its object exists. The second tuple is printed with the first object as its base.
func TestVerifyReportsTheSecondBundle(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run("../../..", false, true, false, &out, &errOut)
	s := out.String()
	if !strings.Contains(s, "APPROVED by object "+firstObject) {
		t.Fatalf("the first bundle is not approved by the first object: %q", s)
	}
	// the pending line and the exit code go together: exit 0 only when nothing is pending
	if (code == exitPendingAssets) != strings.Contains(s, "pending, not approved") || code != 0 && code != exitPendingAssets {
		t.Fatalf("exit %d: %q %q", code, s, errOut.String())
	}
	if strings.Contains(s, "PENDING_APPROVAL") {
		for _, a := range pendingAssets {
			if !strings.Contains(s[strings.Index(s, "pending, not approved:"):], a) {
				t.Errorf("%s is not reported as pending: %q", a, s)
			}
		}
	} else if strings.Contains(s, "pending, not approved") {
		t.Errorf("an approved second bundle still lists pending assets: %q", s)
	}
	out.Reset()
	errOut.Reset()
	if code := run("../../..", true, false, true, &out, &errOut); code != 0 {
		t.Fatalf("print -b34: exit %d: %s", code, errOut.String())
	}
	if !bytes.HasPrefix(out.Bytes(), []byte(`{"schema":"tsgk-csharp-g1-approval/r1","scope":"csharp-g1-b34-bundle"`)) || !strings.Contains(out.String(), `"base":"9a5dbd010ce2e4e0ba9e977f88c9aaf253c082170b03c67802f96d6c655eff94"`) || bytes.Count(out.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("tuple %q", out.String())
	}
	if code := run("../../..", false, true, true, &out, &errOut); code != exitUsage {
		t.Fatalf("-b34 with -verify: exit %d", code)
	}
}
