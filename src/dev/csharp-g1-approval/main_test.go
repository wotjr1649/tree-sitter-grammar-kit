package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without an approval object -verify reports PENDING_APPROVAL with its own exit code: it is neither a pass nor a mismatch.
func TestVerifyReportsPendingNotPass(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "contracts", "evidence-pins", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := run(root, false, true, &out, &errOut)
	if code != exitPending || !strings.Contains(out.String(), "PENDING_APPROVAL") || strings.Contains(out.String(), "APPROVED by") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	if code := run(root, true, true, &out, &errOut); code != exitUsage {
		t.Fatalf("both flags: exit %d", code)
	}
}

// -print over the real repository writes the tuple bytes alone on stdout, so a redirect gives the object bytes.
func TestPrintWritesOnlyTheTuple(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run("../../..", true, false, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !bytes.HasPrefix(out.Bytes(), []byte(`{"schema":"tsgk-csharp-g1-approval/r1"`)) || bytes.Count(out.Bytes(), []byte("\n")) != 1 || !strings.Contains(errOut.String(), "sha256 ") {
		t.Fatalf("stdout %q, stderr %q", out.String(), errOut.String())
	}
}
