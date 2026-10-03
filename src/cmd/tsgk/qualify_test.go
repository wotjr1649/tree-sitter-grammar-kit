package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// S08-A10/A11: `tsgk qualify` runs the same core as kit.Qualify (same bytes) with no tool on
// PATH, reads the tracked inventory's 78 cells, fails completeness without evidence, keeps
// trust documents and results outside the host directories and never replaces --out.
func TestQualifyCLI(t *testing.T) {
	t.Setenv("PATH", "")
	inv := filepath.Join(repoRoot(t), "src", "contracts", "qualification-c1.json")
	host := t.TempDir()
	os.WriteFile(filepath.Join(host, "summary.json"), []byte("{}"), 0o644)
	cand := strings.Repeat("a", 40)
	args := []string{"qualify", "--inventory", inv, "--candidate", cand, "--host", "linux-amd64=" + host}
	var out, errb bytes.Buffer
	if code := run(context.Background(), args, &out, &errb); code != exitFail {
		t.Fatalf("no evidence exit %d: %s", code, errb.String())
	}
	data, _ := os.ReadFile(inv)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	api, err := kit.Qualify(ctx, kit.QualifyRequest{Inventory: data, Candidate: cand, Hosts: []kit.QualifyHost{{Platform: "linux-amd64", Root: host}}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(api)
	if strings.TrimSuffix(out.String(), "\n") != string(want) {
		t.Fatalf("CLI and API differ")
	}
	if len(api.Cells) != 78 || api.Completeness != kit.AssessFail || api.SupportClaim != "BLOCKED" || api.Totals.Status[kit.CellMissing] != 78 {
		t.Fatalf("cells %d %s %s %v", len(api.Cells), api.Completeness, api.SupportClaim, api.Totals.Status)
	}
	inside := filepath.Join(host, "inv.json")
	os.WriteFile(inside, data, 0o644)
	errb.Reset()
	if code := run(context.Background(), []string{"qualify", "--inventory", inside, "--candidate", cand, "--host", "linux-amd64=" + host}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "INVENTORY_INSIDE_INPUT") {
		t.Fatalf("inventory inside host: %d %s", code, errb.String())
	}
	os.Remove(inside)
	errb.Reset()
	if code := run(context.Background(), append(args, "--out", filepath.Join(host, "r.json")), &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "OUTPUT_INSIDE_INPUT") {
		t.Fatalf("out inside host: %d %s", code, errb.String())
	}
	exists := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(exists, []byte("keep"), 0o644)
	errb.Reset()
	if code := run(context.Background(), append(args, "--out", exists), &out, &errb); code != exitIO || !strings.Contains(errb.String(), "OUTPUT_EXISTS") {
		t.Fatalf("existing out: %d %s", code, errb.String())
	}
	if b, _ := os.ReadFile(exists); string(b) != "keep" {
		t.Fatal("existing --out was replaced")
	}
	errb.Reset()
	if code := run(context.Background(), []string{"qualify", "--inventory", inv, "--candidate", "HEAD", "--host", "linux-amd64=" + host}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "CANDIDATE_INVALID") {
		t.Fatalf("candidate: %d %s", code, errb.String())
	}
}
