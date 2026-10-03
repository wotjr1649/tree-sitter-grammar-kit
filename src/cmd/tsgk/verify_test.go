package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// expectedFromIdentity builds a trusted expected document from a `tsgk identity` report
// of a reference copy and writes it outside every subject.
func expectedFromIdentity(t *testing.T, ctx context.Context, ref string, extra ...string) string {
	t.Helper()
	code, stdout, stderr := cli(t, ctx, append([]string{"identity", "--root", ref, "--grammar", "alpha"}, extra...)...)
	if code != 0 {
		t.Fatalf("identity: %d %s", code, stderr)
	}
	var id struct {
		Manifest  json.RawMessage `json:"manifest"`
		SetSHA256 string          `json:"set_sha256"`
	}
	if err := json.Unmarshal([]byte(stdout), &id); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Files []json.RawMessage `json:"files"`
	}
	json.Unmarshal(id.Manifest, &m)
	doc, _ := json.Marshal(map[string]any{"schema": "tsgk-expected/r1", "provenance": "reference copy", "file_count": len(m.Files), "set_sha256": id.SetSHA256, "manifest": id.Manifest})
	p := filepath.Join(t.TempDir(), "expected.json")
	if err := os.WriteFile(p, doc, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// zipOf archives root-relative files under prefix with Go's writer.
func zipOf(t *testing.T, root, prefix string, names ...string) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			t.Fatal(err)
		}
		f, _ := w.Create(prefix + n)
		f.Write(data)
	}
	w.Close()
	p := filepath.Join(t.TempDir(), "subject.zip")
	os.WriteFile(p, b.Bytes(), 0o644)
	return p
}

var alphaFiles = []string{"tree-sitter.json", "common/define.js", "alpha/grammar.js", "alpha/queries/highlights.scm", "alpha/src/grammar.json",
	"alpha/src/scanner.c", "alpha/src/tree_sitter/parser.h", "alpha/test/corpus/basic.txt"}

// S02-A01/A02/A11 through the CLI: exit 0 PASS, exit 1 FAIL with a complete report,
// exit 2 for invalid trust documents, 3 for unsupported archive features.
func TestVerifyCLI(t *testing.T) {
	ctx := context.Background()
	ref, root := t.TempDir(), t.TempDir()
	writeTree(t, ref, fixture)
	writeTree(t, root, fixture)
	exp := expectedFromIdentity(t, ctx, ref)
	code, stdout, stderr := cli(t, ctx, "verify", "--root", root, "--grammar", "alpha", "--expected", exp)
	if code != 0 || !strings.Contains(stdout, `"assessment":"PASS"`) {
		t.Fatalf("PASS: %d %s %s", code, stdout, stderr)
	}
	writeTree(t, root, map[string]string{"alpha/src/scanner.c": "changed\n"})
	out := filepath.Join(t.TempDir(), "report.json")
	code, _, stderr = cli(t, ctx, "verify", "--root", root, "--grammar", "alpha", "--expected", exp, "--out", out)
	data, _ := os.ReadFile(out)
	if code != 1 || !strings.Contains(string(data), `"CONTENT_CHANGED"`) || !strings.Contains(string(data), `"assessment":"FAIL"`) {
		t.Fatalf("FAIL: %d %s %s", code, data, stderr)
	}
	// Trust documents inside the subject root would certify the subject themselves.
	inside := filepath.Join(root, "expected.json")
	raw, _ := os.ReadFile(exp)
	os.WriteFile(inside, raw, 0o644)
	code, _, stderr = cli(t, ctx, "verify", "--root", root, "--grammar", "alpha", "--expected", inside)
	if code != 2 || !strings.Contains(stderr, "EXPECTED_INSIDE_INPUT") {
		t.Fatalf("expected inside root: %d %s", code, stderr)
	}
	code, _, stderr = cli(t, ctx, "verify", "--root", root, "--grammar", "alpha", "--expected", exp, "--profile", inside)
	if code != 2 || !strings.Contains(stderr, "PROFILE_INSIDE_INPUT") {
		t.Fatalf("profile inside root: %d %s", code, stderr)
	}
	dup := filepath.Join(t.TempDir(), "dup.json")
	os.WriteFile(dup, []byte(`{"schema":"tsgk-expected/r1","schema":"x"}`), 0o644)
	code, stdout, stderr = cli(t, ctx, "verify", "--root", root, "--grammar", "alpha", "--expected", dup)
	if code != 2 || !strings.Contains(stderr, "JSON_DUPLICATE_KEY expected#/schema") || !strings.Contains(stdout, `"NOT_RUN"`) {
		t.Fatalf("duplicate key: %d %s", code, stderr)
	}
	// Archive subject: same expected record, members mapped below --archive-root.
	z := zipOf(t, ref, "repo-1/", alphaFiles...)
	code, stdout, stderr = cli(t, ctx, "verify", "--archive", z, "--archive-root", "repo-1", "--expected", exp)
	if code != 0 || !strings.Contains(stdout, `"subject":"ARCHIVE"`) {
		t.Fatalf("archive PASS: %d %s %s", code, stdout, stderr)
	}
	if code, _, stderr = cli(t, ctx, "verify", "--archive", z, "--root", root, "--expected", exp); code != 2 || !strings.Contains(stderr, "USAGE") {
		t.Fatalf("--archive with --root: %d %s", code, stderr)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	w.Create("alpha/../../escape")
	w.Close()
	os.WriteFile(bad, b.Bytes(), 0o644)
	if code, _, stderr = cli(t, ctx, "verify", "--archive", bad, "--expected", exp); code != 2 || !strings.Contains(stderr, "ARCHIVE_PATH_TRAVERSAL") {
		t.Fatalf("traversal: %d %s", code, stderr)
	}
	b.Reset()
	w = zip.NewWriter(&b)
	w.Create("café.txt")
	w.Close()
	os.WriteFile(bad, b.Bytes(), 0o644)
	if code, _, stderr = cli(t, ctx, "verify", "--archive", bad, "--expected", exp); code != 3 || !strings.Contains(stderr, "ARCHIVE_PATH_NOT_ASCII") {
		t.Fatalf("unicode: %d %s", code, stderr)
	}
	// Profiles: identity selection, corpus bounds and the encoding source rule.
	prof := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(prof, []byte(`{"schema":"tsgk-profile/r1","id":"p","files":[{"path":"alpha/grammar.js","role":"grammar","required":true}],"limits":{"files":1}}`), 0o644)
	if code, stdout, stderr = cli(t, ctx, "identity", "--root", root, "--profile", prof); code != 0 || strings.Count(stdout, `"sha256":`) != 4 {
		t.Fatalf("identity --profile: %d %s %s", code, stdout, stderr)
	}
	os.WriteFile(prof, []byte(`{"schema":"tsgk-profile/r1","id":"p","limits":{"files":26001}}`), 0o644)
	if code, _, stderr = cli(t, ctx, "corpus", "--root", filepath.Join(root, "corpus-like"), "--profile", prof); code != 2 || !strings.Contains(stderr, "PROFILE_LIMIT_ABOVE_OPERATION") {
		t.Fatalf("corpus profile above tracked: %d %s", code, stderr)
	}
	os.WriteFile(prof, []byte(`{"schema":"tsgk-profile/r1","id":"p","encoding":{"profile":"cp949"}}`), 0o644)
	if code, _, stderr = cli(t, ctx, "corpus", "--root", filepath.Join(root, "corpus-like"), "--profile", prof, "--encoding-profile", "cp949"); code != 2 || !strings.Contains(stderr, "ENCODING_SOURCE_CONFLICT") {
		t.Fatalf("encoding conflict: %d %s", code, stderr)
	}
	if code, stdout, _ = cli(t, ctx, "corpus", "--root", filepath.Join(root, "corpus-like"), "--profile", prof); code != 0 || !strings.Contains(stdout, `"encoding_policy":"detect-r1;profile=cp949"`) {
		t.Fatalf("corpus profile encoding: %d", code)
	}
}
