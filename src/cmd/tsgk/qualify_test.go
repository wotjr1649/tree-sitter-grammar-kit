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

// treeHashes maps every file below root to its bytes, so a workflow can be shown to leave
// its inputs untouched.
func treeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

// S08-A10: the tool-free grammar-update workflow through the CLI. A baseline snapshot's
// identity becomes the expected document; the candidate snapshot is verified against it
// (FAIL with the changed files reported) and its node schema diffed against the baseline's
// (review risks). Nothing is adopted: both snapshots are unchanged and nothing is written
// except an explicit --out.
func TestGrammarUpdateWorkflow(t *testing.T) {
	ctx := context.Background()
	bin := buildCLI(t)
	offline := []string{"PATH=", "SystemRoot=" + os.Getenv("SystemRoot")}
	base, cand := t.TempDir(), t.TempDir()
	writeTree(t, base, fixture)
	writeTree(t, cand, fixture)
	writeTree(t, base, map[string]string{"alpha/src/node-types.json": cliBase})
	writeTree(t, cand, map[string]string{"alpha/src/node-types.json": cliCandidate, "alpha/src/grammar.json": `{"name":"alpha"}`})
	before := map[string]map[string]string{base: treeHashes(t, base), cand: treeHashes(t, cand)}
	exp := expectedFromIdentity(t, ctx, base)
	if code, stdout, stderr := runBin(t, offline, bin, "verify", "--root", base, "--grammar", "alpha", "--expected", exp); code != exitOK || !strings.Contains(stdout, `"assessment":"PASS"`) {
		t.Fatalf("baseline against its own identity: %d %s", code, stderr)
	}
	code, stdout, stderr := runBin(t, offline, bin, "verify", "--root", cand, "--grammar", "alpha", "--expected", exp)
	if code != exitFail || !strings.Contains(stdout, `"assessment":"FAIL"`) || !strings.Contains(stdout, "node-types.json") || !strings.Contains(stdout, "grammar.json") {
		t.Fatalf("candidate against the baseline identity: %d %s %s", code, stdout, stderr)
	}
	report := filepath.Join(t.TempDir(), "diff.json")
	if code, _, stderr := runBin(t, offline, bin, "schema", "diff", "--before", filepath.Join(base, "alpha", "src", "node-types.json"),
		"--after", filepath.Join(cand, "alpha", "src", "node-types.json"), "--out", report); code != exitOK && code != exitFail {
		t.Fatalf("schema diff: %d %s", code, stderr)
	}
	var diff struct {
		Differences []struct {
			Risk string `json:"risk"`
		} `json:"differences"`
	}
	data, _ := os.ReadFile(report)
	if json.Unmarshal(data, &diff) != nil || len(diff.Differences) == 0 {
		t.Fatalf("schema diff reported no review item: %s", data)
	}
	for root, files := range before {
		after := treeHashes(t, root)
		if len(after) != len(files) {
			t.Fatalf("files added or removed under %s", root)
		}
		for p, b := range files {
			if after[p] != b {
				t.Fatalf("workflow changed %s", p)
			}
		}
	}
}
