package kit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func kindOf(t *testing.T, err error, kind, code string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e == nil || e.Kind != kind || e.Code != code {
		t.Fatalf("want %s/%s, got %v", kind, code, err)
	}
}

var vectorFiles = map[string]string{
	"grammar.js":    "module.exports = 1;\n",
	"queries/a.scm": "; \xed\x95\x9c\xea\xb8\x80\n(x) @y\n",
	"src/scanner.c": "\xef\xbb\xbfint x;\r\n",
}

var vectorSelection = []FileSelection{{"src/scanner.c", "scanner"}, {"grammar.js", "grammar"}, {"queries/a.scm", "query"}}

// independentSet rebuilds the r2 preimage from the written contract, not from setSHA256.
func independentSet(records [][]string) string {
	var b strings.Builder
	b.WriteString("tsgk-files/r2\nportable-default\ndetect-r1\n")
	slices.SortFunc(records, func(a, c []string) int { return strings.Compare(a[0], c[0]) })
	for _, r := range records {
		b.WriteString(strings.Join(r, "\x00") + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func rec(path, role, data, source string) []string {
	sum := sha256.Sum256([]byte(data))
	return []string{path, role, "100644", "POLICY_DEFAULT", fmt.Sprint(len(data)), hex.EncodeToString(sum[:]), "PASS", "UTF-8", source, ""}
}

func identity(t *testing.T, root string, sel Selection) (IdentityResult, error) {
	t.Helper()
	return Identity(testCtx(t), IdentityRequest{Root: root, Selection: sel, Limits: DefaultLimits()})
}

// S01-A04/A05/A06: externally computed vector, location independence, and mutations.
func TestIdentityVector(t *testing.T) {
	const external = "b0f3c85737726a84d636015bd9f628b77d0dd99a796a05a1e8ed1602c958fff9" // Python hashlib, outside the product
	base := [][]string{rec("grammar.js", "grammar", vectorFiles["grammar.js"], "VALIDATION"), rec("queries/a.scm", "query", vectorFiles["queries/a.scm"], "VALIDATION"), rec("src/scanner.c", "scanner", vectorFiles["src/scanner.c"], "BOM")}
	if got := independentSet(base); got != external {
		t.Fatalf("independent oracle disagrees with external vector: %s", got)
	}
	one, two := t.TempDir(), filepath.Join(t.TempDir(), "elsewhere", "deeper")
	writeTree(t, one, vectorFiles)
	writeTree(t, two, vectorFiles)
	a, err := identity(t, one, Selection{Grammar: ".", Files: vectorSelection})
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity(t, two, Selection{Grammar: ".", Files: vectorSelection})
	if err != nil {
		t.Fatal(err)
	}
	if a.SetSHA256 != external || b.SetSHA256 != external {
		t.Fatalf("set identity %s / %s, want %s", a.SetSHA256, b.SetSHA256, external)
	}
	for _, f := range a.Manifest.Files { // A06: raw size/hash of BOM, CRLF and non-ASCII bytes
		sum := sha256.Sum256([]byte(vectorFiles[f.Path]))
		if f.Size != uint64(len(vectorFiles[f.Path])) || f.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("raw identity changed for %s", f.Path)
		}
	}
	if a.ExecutionStatus != StatusCompleted || a.EvidenceMode != ModeNewRun || a.Assessment != AssessNotAssessed {
		t.Fatalf("E0 axes %s/%s/%s", a.ExecutionStatus, a.EvidenceMode, a.Assessment)
	}
	mutate := func(name string, files map[string]string, sel []FileSelection, want [][]string) {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, files)
			got, err := identity(t, root, Selection{Grammar: ".", Files: sel})
			if err != nil {
				t.Fatal(err)
			}
			if got.SetSHA256 == external {
				t.Fatal("mutation did not change identity")
			}
			if exp := independentSet(want); got.SetSHA256 != exp {
				t.Fatalf("identity %s, independent oracle %s", got.SetSHA256, exp)
			}
		})
	}
	byteChanged := map[string]string{"grammar.js": "module.exports = 2;\n", "queries/a.scm": vectorFiles["queries/a.scm"], "src/scanner.c": vectorFiles["src/scanner.c"]}
	mutate("one-byte", byteChanged, vectorSelection, [][]string{rec("grammar.js", "grammar", "module.exports = 2;\n", "VALIDATION"), base[1], base[2]})
	lf := map[string]string{"grammar.js": vectorFiles["grammar.js"], "queries/a.scm": vectorFiles["queries/a.scm"], "src/scanner.c": "\xef\xbb\xbfint x;\n"}
	mutate("crlf-not-normalized", lf, vectorSelection, [][]string{base[0], base[1], rec("src/scanner.c", "scanner", "\xef\xbb\xbfint x;\n", "BOM")})
	moved := map[string]string{"grammar.js": vectorFiles["grammar.js"], "queries/b.scm": vectorFiles["queries/a.scm"], "src/scanner.c": vectorFiles["src/scanner.c"]}
	mutate("same-bytes-other-path", moved, []FileSelection{{"src/scanner.c", "scanner"}, {"grammar.js", "grammar"}, {"queries/b.scm", "query"}}, [][]string{base[0], rec("queries/b.scm", "query", vectorFiles["queries/a.scm"], "VALIDATION"), base[2]})
	mutate("role", vectorFiles, []FileSelection{{"src/scanner.c", "generated"}, {"grammar.js", "grammar"}, {"queries/a.scm", "query"}}, [][]string{base[0], base[1], rec("src/scanner.c", "generated", vectorFiles["src/scanner.c"], "BOM")})
	mutate("membership-removed", vectorFiles, vectorSelection[:2], [][]string{base[0], base[2]})
	extra := map[string]string{"x.txt": "x"}
	for k, v := range vectorFiles {
		extra[k] = v
	}
	mutate("membership-added", extra, append(slices.Clone(vectorSelection), FileSelection{"x.txt", "metadata"}), [][]string{base[0], base[1], base[2], rec("x.txt", "metadata", "x", "VALIDATION")})
	t.Run("mode-field-participates", func(t *testing.T) {
		m := Manifest{Schema: ManifestSchema, Algorithm: "sha256", ModePolicy: ModePolicy, EncodingPolicy: "detect-r1", Files: []FileIdentity{{Path: "a", Role: "grammar", Mode: "100644", ModeProvenance: "POLICY_DEFAULT", Size: 1, SHA256: "00"}}}
		before := setSHA256(m)
		m.Files[0].Mode = "100755"
		if setSHA256(m) == before {
			t.Fatal("mode not bound")
		}
	})
	t.Run("filesystem-mode-is-policy-default", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no executable bit to change")
		}
		root := t.TempDir()
		writeTree(t, root, vectorFiles)
		if err := os.Chmod(filepath.Join(root, "grammar.js"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := identity(t, root, Selection{Grammar: ".", Files: vectorSelection})
		if err != nil || got.SetSHA256 != external {
			t.Fatalf("portable-default mode policy must not observe filesystem mode: %v %s", err, got.SetSHA256)
		}
	})
}

func TestEncodingBoundIntoIdentity(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.cs": "\xc7\xd1", "b.cs": "\xff\xfea\x00\x3d\xd8", "c.cs": "ok"})
	sel := Selection{Grammar: ".", Files: []FileSelection{{"a.cs", "corpus"}, {"b.cs", "corpus"}, {"c.cs", "corpus"}}}
	plain, err := identity(t, root, sel)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := Identity(testCtx(t), IdentityRequest{Root: root, Selection: sel, Limits: DefaultLimits(), Encoding: EncodingPolicy{Profile: "cp949", Files: []FileEncoding{{"c.cs", "utf-8"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if plain.SetSHA256 == cp.SetSHA256 || plain.Manifest.Files[0].SHA256 != cp.Manifest.Files[0].SHA256 {
		t.Fatal("encoding policy must change set identity but not raw hashes")
	}
	got := map[string]EncodingOutcome{}
	for _, f := range cp.Manifest.Files {
		got[f.Path] = f.Encoding
	}
	// S05 completed the table-dependent outcomes: a.cs is a mapped index-euc-kr pair.
	if got["a.cs"] != (EncodingOutcome{Assessment: "PASS", Encoding: EncodingCP949, Source: SourceValidation}) || !hasFinding(cp.Findings, "UTF16_UNPAIRED_SURROGATE", "b.cs") {
		t.Fatalf("cp949 outcome %+v", got["a.cs"])
	}
	if got["b.cs"].Assessment != "BLOCKED" || got["b.cs"].Code != "UTF16_UNPAIRED_SURROGATE" || cp.Manifest.Files[1].Size != 6 {
		t.Fatalf("UTF-16 outcome %+v", got["b.cs"])
	}
	if got["c.cs"].Source != SourceDeclaration {
		t.Fatalf("declaration not bound: %+v", got["c.cs"])
	}
	_, err = Identity(testCtx(t), IdentityRequest{Root: root, Selection: sel, Limits: DefaultLimits(), Encoding: EncodingPolicy{Files: []FileEncoding{{"missing.cs", "utf-8"}}}})
	kindOf(t, err, KindInvalidInput, "DECLARATION_UNMATCHED")
	_, err = Identity(testCtx(t), IdentityRequest{Root: root, Selection: sel, Limits: DefaultLimits(), Encoding: EncodingPolicy{Profile: "latin1"}})
	kindOf(t, err, KindInvalidInput, "ENCODING_PROFILE_INVALID")
}

var multiGrammar = map[string]string{
	"tree-sitter.json":               `{"grammars":[{"name":"alpha","path":"alpha","highlights":"queries/highlights.scm"},{"name":"beta","path":"beta"}]}`,
	"common/define.js":               "module.exports = function (d) { return d; };\n",
	"common/shared.h":                "#pragma once\n",
	"queries/highlights.scm":         "(x) @y\n",
	"alpha/grammar.js":               "const define = require('../common/define');\nmodule.exports = define({});\n",
	"alpha/src/grammar.json":         "{}",
	"alpha/src/scanner.c":            "#include \"../../common/shared.h\"\n#include \"tree_sitter/parser.h\"\n",
	"alpha/src/tree_sitter/parser.h": "#pragma once\n",
	"beta/grammar.js":                "module.exports = {};\n",
	"beta/src/grammar.json":          "{}",
}

func paths(entries []InventoryEntry, state string) []string {
	var out []string
	for _, e := range entries {
		if e.State == state {
			out = append(out, e.Role+":"+e.Path)
		}
	}
	return out
}

// S01-A02/A03: explicit selection inside a multi-grammar root with shared files outside src.
func TestInspectLayouts(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, multiGrammar)
	ctx := testCtx(t)
	top, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	if top.ClosureState != ClosureNotApplicable || len(top.Grammars) != 2 || !hasFinding(top.Findings, "UNKNOWN_LAYOUT", ".") || !hasFinding(top.Findings, "GRAMMAR_CANDIDATE", "alpha") {
		t.Fatalf("multi-grammar root observation: %+v %+v", top.Grammars, top.Findings)
	}
	alpha, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "alpha"}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"grammar:alpha/grammar.js", "grammar:alpha/src/grammar.json", "scanner:alpha/src/scanner.c", "generated:alpha/src/tree_sitter/parser.h", "grammar:common/define.js", "scanner:common/shared.h", "query:queries/highlights.scm", "metadata:tree-sitter.json"}
	slices.Sort(want)
	got := paths(alpha.Entries, StateFound)
	slices.Sort(got)
	if !slices.Equal(got, want) || alpha.ClosureState != ClosureObserved {
		t.Fatalf("alpha selection %v closure %s findings %+v", got, alpha.ClosureState, alpha.Findings)
	}
	for _, e := range alpha.Entries {
		if strings.HasPrefix(e.Path, "beta/") {
			t.Fatalf("sibling grammar input selected: %s", e.Path)
		}
	}
	// No language-name switch: the same layout under another directory name behaves the same.
	renamed := map[string]string{}
	for k, v := range multiGrammar {
		renamed[strings.Replace(strings.Replace(k, "alpha/", "zz-other/", 1), "alpha", "zz-other", 1)] = strings.ReplaceAll(v, `"alpha"`, `"zz-other"`)
	}
	root2 := t.TempDir()
	writeTree(t, root2, renamed)
	other, err := Inspect(ctx, InspectRequest{Root: root2, Selection: Selection{Grammar: "zz-other"}, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	got2 := paths(other.Entries, StateFound)
	slices.Sort(got2)
	var want2 []string
	for _, w := range want {
		want2 = append(want2, strings.Replace(w, "alpha/", "zz-other/", 1))
	}
	slices.Sort(want2)
	if !slices.Equal(got2, want2) {
		t.Fatalf("renamed grammar differs: %v", got2)
	}
	t.Run("legacy-without-tree-sitter-json", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"package.json": `{"tree-sitter":[{"scope":"source.x"}]}`, "grammar.js": "module.exports = {};\n", "corpus/a.txt": "x"})
		res, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
		if err != nil || res.ClosureState != ClosureObserved || !hasFinding(res.Findings, "METADATA_ABSENT", "tree-sitter.json") || len(res.Grammars) != 1 {
			t.Fatalf("legacy layout: %v %+v", err, res)
		}
		if !slices.Contains(paths(res.Entries, StateNotFound), "generated:src/parser.c") || !slices.Contains(paths(res.Entries, StateFound), "corpus:corpus/a.txt") {
			t.Fatalf("legacy entries: %+v", res.Entries)
		}
	})
	t.Run("unknown-layout", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"README.md": "x", "Makefile": "all:\n"})
		res, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
		if err != nil || res.ClosureState != ClosureNotApplicable || res.Assessment != AssessNotAssessed || !hasFinding(res.Findings, "UNKNOWN_LAYOUT", ".") {
			t.Fatalf("unknown layout: %v %+v", err, res)
		}
		_, err = Identity(ctx, IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
		kindOf(t, err, KindInvalidInput, "NO_GRAMMAR_SELECTED")
	})
	t.Run("unresolved-references", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"grammar.js": "const c = require('tree-sitter-c/grammar');\nconst n = require(name);\nconst m = require('./missing');\n", "src/scanner.c": "#include \"nope.h\"\n"})
		res, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
		if err != nil || res.ClosureState != ClosureUnresolved {
			t.Fatalf("unresolved closure: %v %s", err, res.ClosureState)
		}
		for _, code := range []string{"EXTERNAL_MODULE_REFERENCE", "UNPARSED_REFERENCE_TOKEN", "REFERENCE_NOT_FOUND", "INCLUDE_NOT_FOUND"} {
			if !hasCode(res.Findings, code) {
				t.Fatalf("missing finding %s: %+v", code, res.Findings)
			}
		}
		if !slices.Contains(res.Coverage.Unsupported, "js-closure-proof") {
			t.Fatal("JS closure proof must be reported unsupported")
		}
	})
}

func hasFinding(fs []Finding, code, path string) bool {
	for _, f := range fs {
		if f.Code == code && f.Path == path {
			return true
		}
	}
	return false
}

func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

// S01-A07: invalid selections and roots are rejected before any content is read.
func TestGuards(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, vectorFiles)
	ctx := testCtx(t)
	var opened []string
	testHookOpen = func(name string) { opened = append(opened, name) }
	t.Cleanup(func() { testHookOpen = nil })
	for _, g := range []string{"", "./x", "x/.", "x/../y", "../x", "/abs", "C:/x", `\\host\share`, `a\b`, "a//b", "CON", "x.", "a:b"} {
		_, err := identity(t, root, Selection{Grammar: g})
		kindOf(t, err, KindInvalidInput, "GRAMMAR_INVALID")
		_, err = identity(t, root, Selection{Grammar: ".", Files: []FileSelection{{g, "grammar"}}})
		if g != "" {
			kindOf(t, err, KindInvalidInput, "SELECTION_INVALID")
		}
	}
	_, err := identity(t, root, Selection{Grammar: ".", Files: []FileSelection{}})
	kindOf(t, err, KindInvalidInput, "EMPTY_SELECTION")
	_, err = identity(t, root, Selection{Grammar: ".", Files: []FileSelection{{"grammar.js", "unknown-role"}}})
	kindOf(t, err, KindInvalidInput, "SELECTION_INVALID")
	_, err = identity(t, root, Selection{Grammar: ".", Files: []FileSelection{{"grammar.js", "grammar"}, {"grammar.js", "query"}}})
	kindOf(t, err, KindInvalidInput, "SELECTION_DUPLICATE")
	_, err = identity(t, root, Selection{Grammar: ".", Files: []FileSelection{{"grammar.js", "grammar"}, {"absent.js", "grammar"}}})
	kindOf(t, err, KindInvalidInput, "SELECTED_FILE_NOT_FOUND")
	_, err = identity(t, root, Selection{Grammar: ".", Files: []FileSelection{{"src", "scanner"}}})
	kindOf(t, err, KindInvalidInput, "LINK_OR_SPECIAL_REJECTED")
	_, err = identity(t, filepath.Join(root, "missing"), Selection{Grammar: "."})
	kindOf(t, err, KindInvalidInput, "ROOT_NOT_FOUND")
	_, err = identity(t, filepath.Join(root, "grammar.js"), Selection{Grammar: "."})
	kindOf(t, err, KindInvalidInput, "ROOT_NOT_DIRECTORY")
	if runtime.GOOS == "windows" {
		_, err = identity(t, `\\localhost\c$`, Selection{Grammar: "."})
		kindOf(t, err, KindInvalidInput, "ROOT_NOT_LOCAL")
		_, err = identity(t, `\\?\`+root, Selection{Grammar: "."})
		kindOf(t, err, KindInvalidInput, "ROOT_NOT_LOCAL")
	}
	if runtime.GOOS == "windows" {
		unc := filepath.Join(t.TempDir(), "unc")
		if err := os.Symlink(`\\localhost\c$`, unc); err == nil {
			_, err = identity(t, unc, Selection{Grammar: "."})
			kindOf(t, err, KindInvalidInput, "ROOT_NOT_LOCAL")
		}
	}
	if len(opened) != 0 {
		t.Fatalf("content read before rejection: %v", opened)
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.c")
	writeTree(t, outside, map[string]string{"secret.c": "SECRET", "dir/x.scm": "SECRET"})
	links := t.TempDir()
	writeTree(t, links, map[string]string{"grammar.js": "module.exports = {};\n", "src/notes.txt": "x"})
	if err := os.Symlink(secret, filepath.Join(links, "src", "scanner.c")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(links, "queries")); err != nil {
		t.Fatal(err)
	}
	opened = nil
	res, err := Inspect(ctx, InspectRequest{Root: links, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	if err != nil || !slices.Contains(paths(res.Entries, StateUnsupported), "scanner:src/scanner.c") || !slices.Contains(paths(res.Entries, StateUnsupported), "query:queries") {
		t.Fatalf("links must be observed unsupported: %v %+v", err, res.Entries)
	}
	_, err = Identity(ctx, IdentityRequest{Root: links, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	kindOf(t, err, KindInvalidInput, "LINK_OR_SPECIAL_REJECTED")
	_, err = Identity(ctx, IdentityRequest{Root: links, Selection: Selection{Grammar: ".", Files: []FileSelection{{"queries/x.scm", "query"}}}, Limits: DefaultLimits()})
	kindOf(t, err, KindInvalidInput, "LINK_OR_SPECIAL_REJECTED")
	for _, name := range opened {
		if strings.Contains(name, "scanner.c") || strings.Contains(name, "queries") {
			t.Fatalf("link target opened: %s", name)
		}
	}
	t.Run("hardlink", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"grammar.js": "module.exports = {};\n"})
		if err := os.Link(filepath.Join(root, "grammar.js"), filepath.Join(t.TempDir(), "alias.js")); err != nil {
			t.Skipf("hard link unavailable: %v", err)
		}
		_, err := identity(t, root, Selection{Grammar: "."})
		kindOf(t, err, KindInvalidInput, "HARDLINK_REJECTED")
	})
	t.Run("junction", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions are Windows reparse points")
		}
		root := t.TempDir()
		writeTree(t, root, map[string]string{"grammar.js": "module.exports = {};\n"})
		cmd := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, "queries"), filepath.Join(outside, "dir"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("mklink /J: %v %s", err, out)
		}
		res, err := Inspect(ctx, InspectRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
		if err != nil || !slices.Contains(paths(res.Entries, StateUnsupported), "query:queries") {
			t.Fatalf("junction must not be followed: %v %+v", err, res.Entries)
		}
	})
	t.Run("special-file", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"grammar.js": "module.exports = {};\n"})
		if err := mkfifo(filepath.Join(root, "queries.scm")); err != nil {
			t.Skipf("special file unavailable: %v", err)
		}
		_, err := Identity(ctx, IdentityRequest{Root: root, Selection: Selection{Grammar: ".", Files: []FileSelection{{"queries.scm", "query"}}}, Limits: DefaultLimits()})
		kindOf(t, err, KindInvalidInput, "LINK_OR_SPECIAL_REJECTED")
	})
	t.Run("root-alias-is-resolved", func(t *testing.T) {
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(root, alias); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		a, err1 := identity(t, alias, Selection{Grammar: ".", Files: vectorSelection})
		b, err2 := identity(t, root, Selection{Grammar: ".", Files: vectorSelection})
		if err1 != nil || err2 != nil || a.SetSHA256 != b.SetSHA256 {
			t.Fatalf("explicit root alias: %v %v", err1, err2)
		}
	})
}

// S01-A08: limits at and one over, source changes and read failures; no partial PASS.
func TestLimits(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a/b/c.scm": "1234", "a/d.scm": "56"})
	sel := Selection{Grammar: ".", Files: []FileSelection{{"a/b/c.scm", "query"}, {"a/d.scm", "query"}}}
	run := func(l Limits) (IdentityResult, error) {
		return Identity(testCtx(t), IdentityRequest{Root: root, Selection: sel, Limits: l})
	}
	at := DefaultLimits()
	at.Files, at.FileBytes, at.TotalBytes, at.Depth = 2, 4, 6, 3
	ok, err := run(at)
	if err != nil {
		t.Fatalf("at limits: %v", err)
	}
	// The policy echoes output_bytes, so find the fixed point where the limit equals the
	// exact output size (JSON plus the CLI newline).
	for i := 0; i < 8; i++ {
		at.OutputBytes = uint64(len(mustJSON(t, ok))) + 1
		if ok, err = run(at); err != nil {
			t.Fatalf("output at limit: %v", err)
		}
	}
	if uint64(len(mustJSON(t, ok)))+1 != at.OutputBytes {
		t.Fatal("output size fixed point not reached")
	}
	for _, tc := range []struct {
		code   string
		mutate func(*Limits)
	}{
		{"FILE_COUNT_LIMIT", func(l *Limits) { l.Files = 1 }},
		{"FILE_BYTES_LIMIT", func(l *Limits) { l.FileBytes = 3 }},
		{"TOTAL_BYTES_LIMIT", func(l *Limits) { l.TotalBytes = 5 }},
		{"DEPTH_LIMIT", func(l *Limits) { l.Depth = 2 }},
		{"OUTPUT_LIMIT", func(l *Limits) { l.OutputBytes-- }},
		{"WALL_LIMIT", func(l *Limits) { l.Wall = time.Nanosecond }},
	} {
		t.Run(tc.code, func(t *testing.T) {
			l := at
			tc.mutate(&l)
			res, err := run(l)
			kindOf(t, err, KindResourceLimit, tc.code)
			if res.ExecutionStatus != StatusResourceLimit || res.Assessment != AssessBlocked || res.SetSHA256 != "" || len(res.Manifest.Files) != 0 {
				t.Fatalf("partial result after limit: %+v", res.Report)
			}
		})
	}
	for _, l := range []Limits{{}, {Files: 1, FileBytes: 1, TotalBytes: 1, Depth: 1, OutputBytes: 1}} {
		_, err := run(l)
		kindOf(t, err, KindInvalidInput, "LIMITS_INVALID")
	}
	t.Run("empty-directory-at-depth-limit", func(t *testing.T) {
		deep := t.TempDir()
		writeTree(t, deep, map[string]string{"grammar.js": "module.exports = {};\n"})
		if err := os.MkdirAll(filepath.Join(deep, "queries", "a", "b"), 0o755); err != nil {
			t.Fatal(err)
		}
		l := DefaultLimits()
		l.Depth = 3
		if _, err := Inspect(testCtx(t), InspectRequest{Root: deep, Selection: Selection{Grammar: "."}, Limits: l}); err != nil {
			t.Fatalf("directory at the depth limit: %v", err)
		}
		l.Depth = 2
		_, err := Inspect(testCtx(t), InspectRequest{Root: deep, Selection: Selection{Grammar: "."}, Limits: l})
		kindOf(t, err, KindResourceLimit, "DEPTH_LIMIT")
	})
	t.Run("source-changed", func(t *testing.T) {
		testHookAfterOpen = func(name string) {
			if name == "a/d.scm" {
				os.WriteFile(filepath.Join(root, "a", "d.scm"), []byte("5678"), 0o644)
			}
		}
		defer func() { testHookAfterOpen = nil }()
		res, err := run(DefaultLimits())
		kindOf(t, err, KindIO, "SOURCE_CHANGED")
		if res.ExecutionStatus != StatusFailed || res.SetSHA256 != "" {
			t.Fatalf("changed source must not complete: %+v", res.Report)
		}
	})
	t.Run("read-failure", func(t *testing.T) {
		writeTree(t, root, map[string]string{"a/gone.scm": "x"})
		testHookOpen = func(name string) {
			if name == "a/gone.scm" {
				os.Remove(filepath.Join(root, "a", "gone.scm"))
			}
		}
		defer func() { testHookOpen = nil }()
		_, err := Identity(testCtx(t), IdentityRequest{Root: root, Selection: Selection{Grammar: ".", Files: []FileSelection{{"a/gone.scm", "query"}}}, Limits: DefaultLimits()})
		kindOf(t, err, KindIO, "READ_FAILED")
	})
	t.Run("large-file-profile-is-identity-scoped", func(t *testing.T) {
		big := t.TempDir()
		writeTree(t, big, map[string]string{"src/parser.c": strings.Repeat("x", 17)})
		l := DefaultLimits()
		l.FileBytes = 16
		_, err := Identity(testCtx(t), IdentityRequest{Root: big, Selection: Selection{Grammar: ".", Files: []FileSelection{{"src/parser.c", "generated"}}}, Limits: l, LargeFileProfile: "pg-large-source-r1"})
		kindOf(t, err, KindResourceLimit, "FILE_BYTES_LIMIT")
		_, err = Identity(testCtx(t), IdentityRequest{Root: big, Selection: Selection{Grammar: "."}, Limits: l, LargeFileProfile: "unknown"})
		kindOf(t, err, KindInvalidInput, "LARGE_FILE_PROFILE_UNKNOWN")
	})
	t.Run("large-file-profile-admits-only-exact-identity", func(t *testing.T) {
		body, over := strings.Repeat("x", 17), strings.Repeat("x", 21)
		sum, overSum := sha256.Sum256([]byte(body)), sha256.Sum256([]byte(over))
		// over is registered too, so only the profile limit can reject it.
		largeFileProfiles["test-large-r1"] = largeFileProfile{limit: 20, ids: map[string]uint64{hex.EncodeToString(sum[:]): 17, hex.EncodeToString(overSum[:]): 21}}
		defer delete(largeFileProfiles, "test-large-r1")
		l := DefaultLimits()
		l.FileBytes = 16
		run := func(content string) (IdentityResult, error) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"src/parser.c": content})
			return Identity(testCtx(t), IdentityRequest{Root: root, Selection: Selection{Grammar: ".", Files: []FileSelection{{"src/parser.c", "generated"}}}, Limits: l, LargeFileProfile: "test-large-r1"})
		}
		res, err := run(body)
		if err != nil || len(res.Manifest.Files) != 1 || !slices.ContainsFunc(res.Findings, func(f Finding) bool { return f.Code == "LARGE_FILE_EXCEPTION" }) {
			t.Fatalf("exact identity: err=%v res=%+v", err, res)
		}
		_, err = run(strings.Repeat("y", 17)) // same size, other content
		kindOf(t, err, KindResourceLimit, "FILE_BYTES_LIMIT")
		_, err = run(over) // registered identity over the profile limit
		kindOf(t, err, KindResourceLimit, "FILE_BYTES_LIMIT")
	})
}

// countingCtx cancels deterministically after n checkpoint queries.
type countingCtx struct {
	context.Context
	left atomic.Int64
}

func (c *countingCtx) Err() error {
	if c.left.Add(-1) < 0 {
		return context.Canceled
	}
	return c.Context.Err()
}

// S01-A09: cancellation during a sizeable scan is bounded and leaves input unchanged.
func TestCancellation(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"grammar.js": "module.exports = {};\n"}
	for i := 0; i < 300; i++ {
		files[fmt.Sprintf("test/corpus/c%03d.txt", i)] = strings.Repeat("x", 4096)
	}
	writeTree(t, root, files)
	base, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx := &countingCtx{Context: base}
	ctx.left.Store(150)
	start := time.Now()
	res, err := Identity(ctx, IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	kindOf(t, err, KindCancelled, "CANCELLED")
	if !errors.Is(err, context.Canceled) || res.ExecutionStatus != StatusCancelled || res.SetSHA256 != "" || time.Since(start) > 10*time.Second {
		t.Fatalf("cancellation result %+v %v", res.Report, err)
	}
	after, err := identity(t, root, Selection{Grammar: "."})
	if err != nil || len(after.Manifest.Files) != 301 {
		t.Fatalf("input changed after cancellation: %v", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	_, err = Identity(expired, IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller deadline must be CANCELLED/DeadlineExceeded: %v", err)
	}
	kindOf(t, err, KindCancelled, "CANCELLED")
	_, err = Identity(context.Background(), IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	kindOf(t, err, KindInvalidInput, "DEADLINE_REQUIRED")
	//lint:ignore SA1012 nil context is the tested invalid input
	_, err = Identity(nil, IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()}) //nolint:staticcheck
	kindOf(t, err, KindInvalidInput, "NIL_CONTEXT")
}

// S01-A12: concurrent independent roots, no process-global mutation.
func TestConcurrentRoots(t *testing.T) {
	wd, _ := os.Getwd()
	env := os.Environ()
	var roots []string
	for i := 0; i < 8; i++ {
		root := t.TempDir()
		writeTree(t, root, map[string]string{"grammar.js": fmt.Sprintf("module.exports = %d;\n", i), "src/parser.c": strings.Repeat("y", 1000*i)})
		roots = append(roots, root)
	}
	want := make([]string, len(roots))
	for i, r := range roots {
		res, err := identity(t, r, Selection{Grammar: "."})
		if err != nil {
			t.Fatal(err)
		}
		want[i] = res.SetSHA256
	}
	var wg sync.WaitGroup
	got := make([]string, len(roots))
	errs := make([]error, len(roots))
	for i, r := range roots {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := Identity(testCtx(t), IdentityRequest{Root: r, Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
			got[i], errs[i] = res.SetSHA256, err
		}()
	}
	wg.Wait()
	for i := range roots {
		if errs[i] != nil || got[i] != want[i] {
			t.Fatalf("concurrent root %d: %v", i, errs[i])
		}
	}
	if wd2, _ := os.Getwd(); wd2 != wd || !slices.Equal(env, os.Environ()) {
		t.Fatal("process state mutated")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
