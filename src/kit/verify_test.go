package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const goodProfile = `{"schema":"tsgk-profile/r1","id":"p1","files":[{"path":"grammar.js","role":"grammar","required":true}]}`

// S02-A03/A04: one strict decoder rejects every form before use, with the JSON pointer.
func TestStrictDocuments(t *testing.T) {
	nest := func(n int) string { return strings.Repeat(`{"a":`, n) + "1" + strings.Repeat("}", n) }
	for _, tc := range []struct {
		name, doc, kind, code, path string
	}{
		{"duplicate", `{"schema":"tsgk-profile/r1","id":"a","id":"b"}`, KindInvalidInput, "JSON_DUPLICATE_KEY", "profile#/id"},
		{"escaped-duplicate", `{"schema":"tsgk-profile/r1","id":"a","\u0069d":"b"}`, KindInvalidInput, "JSON_DUPLICATE_KEY", "profile#/id"},
		{"case-key", `{"schema":"tsgk-profile/r1","ID":"a"}`, KindInvalidInput, "JSON_UNKNOWN_FIELD", "profile#/ID"},
		{"unknown-key", `{"schema":"tsgk-profile/r1","id":"a","hooks":[]}`, KindInvalidInput, "JSON_UNKNOWN_FIELD", "profile#/hooks"},
		{"trailing-object", goodProfile + `{}`, KindInvalidInput, "JSON_TRAILING_VALUE", "profile"},
		{"trailing-garbage", goodProfile + ` x`, KindInvalidInput, "JSON_SYNTAX", "profile#"},
		{"missing", `{"schema":"tsgk-profile/r1"}`, KindInvalidInput, "JSON_MISSING_FIELD", "profile#/id"},
		{"null", `{"schema":"tsgk-profile/r1","id":null}`, KindInvalidInput, "JSON_NULL", "profile#/id"},
		{"null-bool", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"g.js","role":"grammar","required":null}]}`, KindInvalidInput, "JSON_NULL", "profile#/files/0/required"},
		{"string-bool", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"g.js","role":"grammar","required":"true"}]}`, KindInvalidInput, "JSON_TYPE", "profile#/files/0/required"},
		{"number-bool", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"g.js","role":"grammar","required":1}]}`, KindInvalidInput, "JSON_TYPE", "profile#/files/0/required"},
		{"string-int", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":"10"}}`, KindInvalidInput, "JSON_TYPE", "profile#/limits/files"},
		{"bool-int", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":true}}`, KindInvalidInput, "JSON_TYPE", "profile#/limits/files"},
		{"fraction", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":10.0}}`, KindInvalidInput, "JSON_NUMBER_NOT_INTEGER", "profile#/limits/files"},
		{"exponent", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":1e3}}`, KindInvalidInput, "JSON_NUMBER_NOT_INTEGER", "profile#/limits/files"},
		{"negative", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":-1}}`, KindInvalidInput, "JSON_NUMBER_NOT_INTEGER", "profile#/limits/files"},
		{"negative-zero", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":-0}}`, KindInvalidInput, "JSON_NUMBER_NOT_INTEGER", "profile#/limits/files"},
		{"leading-zero", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":010}}`, KindInvalidInput, "JSON_SYNTAX", "profile#/limits"},
		{"max-plus-one", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":9007199254740992}}`, KindInvalidInput, "JSON_INTEGER_RANGE", "profile#/limits/files"},
		{"huge", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":123456789012345678901234567890}}`, KindInvalidInput, "JSON_INTEGER_RANGE", "profile#/limits/files"},
		{"zero-limit", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":0}}`, KindInvalidInput, "PROFILE_LIMIT_INVALID", "profile#/limits/files"},
		{"first-error-in-document-order", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":0,"depth":"x","total_bytes":1.5}}`, KindInvalidInput, "PROFILE_LIMIT_INVALID", "profile#/limits/files"},
		{"nan", `{"schema":"tsgk-profile/r1","id":"a","limits":{"files":NaN}}`, KindInvalidInput, "JSON_SYNTAX", "profile#/limits/files"},
		{"invalid-utf8", "{\"schema\":\"tsgk-profile/r1\",\"id\":\"\xff\"}", KindInvalidInput, "JSON_INVALID_UTF8", "profile"},
		{"lone-surrogate", `{"schema":"tsgk-profile/r1","id":"\ud800"}`, KindInvalidInput, "JSON_SYNTAX", "profile#/id"},
		{"bom", "\xef\xbb\xbf" + goodProfile, KindInvalidInput, "JSON_SYNTAX", "profile#"},
		{"truncated", `{"schema":"tsgk-profile/r1"`, KindInvalidInput, "JSON_TRUNCATED", "profile#"},
		{"empty", ``, KindInvalidInput, "JSON_TRUNCATED", "profile#"},
		{"depth-at-limit", nest(jsonMaxDepth), KindInvalidInput, "JSON_UNKNOWN_FIELD", "profile#/a"},
		{"depth-over", nest(jsonMaxDepth + 1), KindResourceLimit, "JSON_DEPTH_LIMIT", "profile#" + strings.Repeat("/a", jsonMaxDepth)},
		{"wrong-schema", `{"schema":"tsgk-profile/r0","id":"a"}`, KindInvalidInput, "SCHEMA_UNSUPPORTED", "profile#/schema"},
		{"id", `{"schema":"tsgk-profile/r1","id":"a b"}`, KindInvalidInput, "PROFILE_ID_INVALID", "profile#/id"},
		{"empty-files", `{"schema":"tsgk-profile/r1","id":"a","files":[]}`, KindInvalidInput, "EMPTY_SELECTION", "profile#/files"},
		{"role", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"g.js","role":"hook","required":true}]}`, KindInvalidInput, "ROLE_INVALID", "profile#/files/0/role"},
		{"path", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"../g.js","role":"grammar","required":true}]}`, KindInvalidInput, "SELECTION_INVALID", "profile#/files/0/path"},
		{"dup-path", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"g.js","role":"grammar","required":true},{"path":"g.js","role":"grammar","required":true}]}`, KindInvalidInput, "SELECTION_DUPLICATE", "profile#/files/1/path"},
		{"unsorted", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"h.js","role":"grammar","required":true},{"path":"g.js","role":"grammar","required":true}]}`, KindInvalidInput, "SELECTION_UNSORTED", "profile#/files/1/path"},
		{"case-path", `{"schema":"tsgk-profile/r1","id":"a","files":[{"path":"G.js","role":"grammar","required":true},{"path":"g.js","role":"grammar","required":true}]}`, KindInvalidInput, "SELECTION_CASE_COLLISION", "profile#/files/1/path"},
		{"unknown-limit", `{"schema":"tsgk-profile/r1","id":"a","limits":{"json_depth":4}}`, KindInvalidInput, "JSON_UNKNOWN_FIELD", "profile#/limits/json_depth"},
		{"enc-unknown", `{"schema":"tsgk-profile/r1","id":"a","encoding":{"files":[{"path":"g.js","encoding":"latin1"}]}}`, KindInvalidInput, "ENCODING_DECLARATION_INVALID", "profile#/encoding/files/0"},
		{"enc-dup", `{"schema":"tsgk-profile/r1","id":"a","encoding":{"files":[{"path":"g.js","encoding":"utf-8"},{"path":"g.js","encoding":"cp949"}]}}`, KindInvalidInput, "ENCODING_DECLARATION_DUPLICATE", "profile#/encoding/files/1"},
		{"enc-field", `{"schema":"tsgk-profile/r1","id":"a","encoding":{"files":[{"path":"g.js","encoding":"utf-8","bom":true}]}}`, KindInvalidInput, "JSON_UNKNOWN_FIELD", "profile#/encoding/files/0/bom"},
		{"enc-profile", `{"schema":"tsgk-profile/r1","id":"a","encoding":{"profile":"euc-kr"}}`, KindInvalidInput, "ENCODING_PROFILE_INVALID", "profile#/encoding/profile"},
		{"enc-empty", `{"schema":"tsgk-profile/r1","id":"a","encoding":{"files":[]}}`, KindInvalidInput, "ENCODING_DECLARATION_INVALID", "profile#/encoding/files"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := parseProfile([]byte(tc.doc))
			kindOf(t, e, tc.kind, tc.code)
			if e.Path != tc.path {
				t.Fatalf("path %q, want %q", e.Path, tc.path)
			}
		})
	}
	for _, name := range []string{"CON .txt", "a/conout$", "COM\u00b9.txt", "lpt\u00b3"} {
		if portable(name, false) {
			t.Fatalf("reserved device name accepted: %q", name)
		}
	}
	if _, e := parseProfile(make([]byte, MaxDocumentBytes+1)); e == nil || e.Code != "DOCUMENT_BYTES_LIMIT" {
		t.Fatalf("oversized document: %v", e)
	}
	p, e := parseProfile([]byte(`{"schema":"tsgk-profile/r1","id":"a","limits":{"files":9007199254740991}}`))
	if e != nil || p.limits["files"] != 9007199254740991 {
		t.Fatalf("2^53-1 must round-trip exactly: %v %v", e, p)
	}
}

// Tracked examples: valid ones decode, each invalid one fails with the code in its name.
func TestContractExamples(t *testing.T) {
	dir := filepath.Join("..", "contracts", "examples")
	for _, name := range []string{"profile-r1.json", "expected-r1.json", "reproduce-r1.json", "incremental-r2.json", "replay-r1.json", "evidence-policy-r1.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(name, "profile") {
			_, e := parseProfile(data)
			if e != nil {
				t.Fatalf("%s: %v", name, e)
			}
		} else if strings.HasPrefix(name, "reproduce") {
			if _, e := parseReproduce(data); e != nil {
				t.Fatalf("%s: %v", name, e)
			}
		} else if strings.HasPrefix(name, "incremental") {
			if _, e := parseIncremental(data); e != nil {
				t.Fatalf("%s: %v", name, e)
			}
		} else if strings.HasPrefix(name, "replay") {
			if _, e := parseReplay(data); e != nil {
				t.Fatalf("%s: %v", name, e)
			}
		} else if strings.HasPrefix(name, "evidence") {
			if _, e := parseEvidencePolicy(data); e != nil {
				t.Fatalf("%s: %v", name, e)
			}
		} else if _, e := parseExpected(data); e != nil {
			t.Fatalf("%s: %v", name, e)
		}
	}
	invalid, err := os.ReadDir(filepath.Join(dir, "invalid"))
	if err != nil || len(invalid) == 0 {
		t.Fatalf("invalid examples: %v", err)
	}
	for _, ent := range invalid {
		doc, code, _ := strings.Cut(strings.TrimSuffix(ent.Name(), ".json"), "-")
		data, err := os.ReadFile(filepath.Join(dir, "invalid", ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var e *Error
		switch doc {
		case "profile":
			_, e = parseProfile(data)
		case "expected":
			_, e = parseExpected(data)
		case "reproduce":
			_, e = parseReproduce(data)
		case "incremental":
			_, e = parseIncremental(data)
		case "replay":
			_, e = parseReplay(data)
		case "evidence":
			_, e = parseEvidencePolicy(data)
		default:
			t.Fatalf("unknown example %s", ent.Name())
		}
		if e == nil || e.Code != code {
			t.Errorf("%s: got %v", ent.Name(), e)
		}
	}
}

var verifyTree = map[string]string{
	"tree-sitter.json":       `{"grammars":[{"name":"x","path":"."}]}`,
	"grammar.js":             "module.exports = grammar({name: 'x'});\n",
	"src/parser.c":           "int p;\n",
	"src/scanner.c":          "int s;\n",
	"queries/highlights.scm": "(x) @y\n",
}

func trustedExpected(t *testing.T, root string, profile []byte) []byte {
	t.Helper()
	res, err := Identity(testCtx(t), IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits(), Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	return expectedFor(t, res.Manifest.Files)
}

func verifyDir(t *testing.T, root string, exp, profile []byte) (VerifyResult, error) {
	t.Helper()
	return Verify(testCtx(t), VerifyRequest{Root: root, Selection: Selection{Grammar: "."}, Expected: exp, Profile: profile, Limits: DefaultLimits()})
}

func codesOf(res VerifyResult) string {
	var out []string
	for _, d := range res.Differences {
		out = append(out, d.Code+":"+d.Path)
	}
	return strings.Join(out, " ")
}

// S02-A01/A02: an independently prepared expected set matches; each single-member change
// yields its specific difference and FAIL, never a generic success.
func TestVerifyDirectory(t *testing.T) {
	ref := t.TempDir()
	writeTree(t, ref, verifyTree)
	exp := trustedExpected(t, ref, nil)
	fresh := func() string {
		root := t.TempDir()
		writeTree(t, root, verifyTree)
		return root
	}
	res, err := verifyDir(t, fresh(), exp, nil)
	if err != nil || res.Assessment != AssessPass || res.ExecutionStatus != StatusCompleted || len(res.Differences) != 0 || res.Scope != ScopeKnownPaths {
		t.Fatalf("exact set: %v %+v", err, res)
	}
	var x map[string]any
	json.Unmarshal(exp, &x)
	if res.ActualSetSHA256 != x["set_sha256"] || res.Expected.SetSHA256 != x["set_sha256"] || res.Identities[0].Role != "expected-set" || res.Identities[1].SHA256 != res.ActualSetSHA256 {
		t.Fatalf("selected-set identity: %+v", res.Identities)
	}
	for _, tc := range []struct {
		name   string
		change func(root string)
		want   string
	}{
		{"remove", func(r string) { os.Remove(filepath.Join(r, "src/scanner.c")) }, "MISSING_REQUIRED:src/scanner.c"},
		{"add", func(r string) { writeTree(t, r, map[string]string{"queries/tags.scm": "(t) @t\n"}) }, "UNEXPECTED_FILE:queries/tags.scm"},
		{"rename", func(r string) { os.Rename(filepath.Join(r, "src/scanner.c"), filepath.Join(r, "src/scanner2.c")) }, "MISSING_REQUIRED:src/scanner.c UNEXPECTED_FILE:src/scanner2.c"},
		{"alter", func(r string) { writeTree(t, r, map[string]string{"src/scanner.c": "int S;\n"}) }, "CONTENT_CHANGED:src/scanner.c"},
		{"grow", func(r string) { writeTree(t, r, map[string]string{"src/scanner.c": "int s; \n"}) }, "SIZE_CHANGED:src/scanner.c CONTENT_CHANGED:src/scanner.c"},
		{"encoding", func(r string) { writeTree(t, r, map[string]string{"src/scanner.c": "int \xffs\n"}) }, "CONTENT_CHANGED:src/scanner.c ENCODING_CHANGED:src/scanner.c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fresh()
			tc.change(root)
			res, err := verifyDir(t, root, exp, nil)
			if err != nil || res.Assessment != AssessFail || codesOf(res) != tc.want {
				t.Fatalf("%v %s: %s", err, res.Assessment, codesOf(res))
			}
		})
	}
	// A role difference: the same bytes listed under another role by the expected record.
	var doc struct {
		Manifest Manifest `json:"manifest"`
	}
	json.Unmarshal(exp, &doc)
	files := append([]FileIdentity(nil), doc.Manifest.Files...)
	for i := range files {
		if files[i].Path == "src/parser.c" {
			files[i].Role = "scanner"
		}
	}
	res, err = verifyDir(t, fresh(), expectedFor(t, files), nil)
	if err != nil || codesOf(res) != "ROLE_CHANGED:src/parser.c" {
		t.Fatalf("role: %v %s", err, codesOf(res))
	}
	// S02-A07: a link inside the directory subject is rejected, never followed or compared.
	linked := fresh()
	if err := os.Symlink(filepath.Join(ref, "src", "scanner.c"), filepath.Join(linked, "src", "link.c")); err != nil {
		t.Logf("symlink unavailable: %v", err)
	} else {
		res, err := verifyDir(t, linked, exp, nil)
		kindOf(t, err, KindInvalidInput, "LINK_OR_SPECIAL_REJECTED")
		if len(res.Actual.Files) != 0 || res.Assessment == AssessPass {
			t.Fatalf("partial result after link: %+v", res)
		}
	}
	// Encoding policy is a declared parameter, not inferred from the expected record.
	_, err = Verify(testCtx(t), VerifyRequest{Root: fresh(), Selection: Selection{Grammar: "."}, Expected: exp, Limits: DefaultLimits(), Encoding: EncodingPolicy{Profile: "cp949"}})
	kindOf(t, err, KindInvalidInput, "ENCODING_POLICY_MISMATCH")
}

// S02-A02: requiredness comes from the predeclared profile, in a listed scope.
func TestVerifyOptional(t *testing.T) {
	prof := []byte(`{"schema":"tsgk-profile/r1","id":"listed","files":[{"path":"grammar.js","role":"grammar","required":true},{"path":"queries/highlights.scm","role":"query","required":false},{"path":"src/scanner.c","role":"scanner","required":true}]}`)
	ref := t.TempDir()
	writeTree(t, ref, verifyTree)
	exp := trustedExpected(t, ref, prof)
	for _, tc := range []struct {
		remove, want, assessment string
	}{
		{"", "", AssessPass},
		{"queries/highlights.scm", "MISSING_OPTIONAL:queries/highlights.scm", AssessPass},
		{"src/scanner.c", "MISSING_REQUIRED:src/scanner.c", AssessFail},
	} {
		root := t.TempDir()
		writeTree(t, root, verifyTree)
		if tc.remove != "" {
			os.Remove(filepath.Join(root, tc.remove))
		}
		res, err := verifyDir(t, root, exp, prof)
		if err != nil || res.Assessment != tc.assessment || codesOf(res) != tc.want || res.Scope != ScopeListed {
			t.Fatalf("%s: %v %s %s", tc.remove, err, res.Assessment, codesOf(res))
		}
	}
	// A required listed file absent from both the subject and the expected record still fails.
	ref2 := t.TempDir()
	writeTree(t, ref2, map[string]string{"grammar.js": "x\n"})
	small := expectedFor(t, []FileIdentity{rec2("grammar.js", "grammar", "x\n")})
	res, err := verifyDir(t, ref2, small, prof)
	if err != nil || res.Assessment != AssessFail || codesOf(res) != "MISSING_REQUIRED:src/scanner.c" || !hasCode(res.Findings, "OPTIONAL_FILE_ABSENT") {
		t.Fatalf("listed required: %v %s", err, codesOf(res))
	}
}

// Expected self-consistency and name policy are checked before any comparison.
func TestExpectedDocument(t *testing.T) {
	good := []FileIdentity{rec2("a.js", "grammar", "a\n"), rec2("b.js", "grammar", "b\n")}
	base := func() map[string]any {
		var m map[string]any
		json.Unmarshal(expectedFor(t, good), &m)
		return m
	}
	files := func(m map[string]any) []any { return m["manifest"].(map[string]any)["files"].([]any) }
	for _, tc := range []struct {
		name   string
		mutate func(m map[string]any)
		kind   string
		code   string
	}{
		{"count", func(m map[string]any) { m["file_count"] = 3 }, KindInvalidInput, "EXPECTED_COUNT_MISMATCH"},
		{"set", func(m map[string]any) { m["set_sha256"] = strings.Repeat("0", 64) }, KindInvalidInput, "EXPECTED_SET_MISMATCH"},
		{"record-hash", func(m map[string]any) { files(m)[0].(map[string]any)["sha256"] = strings.Repeat("1", 64) }, KindInvalidInput, "EXPECTED_SET_MISMATCH"},
		{"upper-hex", func(m map[string]any) { files(m)[0].(map[string]any)["sha256"] = strings.Repeat("A", 64) }, KindInvalidInput, "SHA256_INVALID"},
		{"unsorted", func(m map[string]any) { f := files(m); f[0], f[1] = f[1], f[0] }, KindInvalidInput, "EXPECTED_PATH_UNSORTED"},
		{"duplicate", func(m map[string]any) { f := files(m); f[1] = f[0] }, KindInvalidInput, "EXPECTED_PATH_DUPLICATE"},
		{"traversal", func(m map[string]any) { files(m)[0].(map[string]any)["path"] = "../a.js" }, KindInvalidInput, "EXPECTED_PATH_TRAVERSAL"},
		{"not-ascii", func(m map[string]any) { files(m)[0].(map[string]any)["path"] = "á.js" }, KindUnsupported, "EXPECTED_PATH_NOT_ASCII"},
		{"mode", func(m map[string]any) { files(m)[0].(map[string]any)["mode"] = "100755" }, KindInvalidInput, "MODE_INVALID"},
		{"encoding-empty", func(m map[string]any) { files(m)[0].(map[string]any)["encoding"].(map[string]any)["source"] = "" }, KindInvalidInput, "ENCODING_OUTCOME_INVALID"},
		{"encoding-null", func(m map[string]any) { files(m)[0].(map[string]any)["encoding"].(map[string]any)["code"] = nil }, KindInvalidInput, "JSON_NULL"},
		{"size-float", func(m map[string]any) { files(m)[0].(map[string]any)["size"] = 2.5 }, KindInvalidInput, "JSON_NUMBER_NOT_INTEGER"},
		{"empty", func(m map[string]any) { m["manifest"].(map[string]any)["files"] = []any{}; m["file_count"] = 0 }, KindInvalidInput, "EMPTY_SELECTION"},
		{"provenance", func(m map[string]any) { m["provenance"] = "" }, KindInvalidInput, "PROVENANCE_INVALID"},
		{"schema", func(m map[string]any) { m["schema"] = "tsgk-manifest/r2" }, KindInvalidInput, "SCHEMA_UNSUPPORTED"},
		{"manifest-schema", func(m map[string]any) { m["manifest"].(map[string]any)["schema"] = "tsgk-manifest/r1" }, KindInvalidInput, "MANIFEST_VALUE_UNSUPPORTED"},
		{"extra-field", func(m map[string]any) { m["trusted"] = true }, KindInvalidInput, "JSON_UNKNOWN_FIELD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mutate(m)
			_, e := parseExpected(mustJSON(t, m))
			kindOf(t, e, tc.kind, tc.code)
		})
	}
	if _, e := parseExpected(mustJSON(t, base())); e != nil {
		t.Fatal(e)
	}
	_, err := Verify(testCtx(t), VerifyRequest{Root: t.TempDir(), Selection: Selection{Grammar: "."}, Limits: DefaultLimits()})
	kindOf(t, err, KindInvalidInput, "EXPECTED_REQUIRED")
	_, err = Verify(testCtx(t), VerifyRequest{Root: t.TempDir(), Archive: "x.zip", Expected: expectedFor(t, good), Limits: DefaultLimits(), ArchiveLimits: DefaultArchiveLimits()})
	kindOf(t, err, KindInvalidInput, "SUBJECT_INVALID")
}

// S02-A16/A15: profile limits only narrow; encoding declarations are strict; profile
// inputs are data with explicit bounds and no execution or relaxation fields.
func TestProfileLimits(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, verifyTree)
	run := func(prof string, mutate func(*IdentityRequest)) (IdentityResult, error) {
		req := IdentityRequest{Root: root, Selection: Selection{Grammar: "."}, Limits: DefaultLimits(), Profile: []byte(prof)}
		if mutate != nil {
			mutate(&req)
		}
		return Identity(testCtx(t), req)
	}
	lim := func(body string) string { return `{"schema":"tsgk-profile/r1","id":"l","limits":{` + body + `}}` }
	base, err := run(lim(""), nil)
	if err != nil {
		t.Fatal(err)
	}
	at, err := run(lim(`"file_bytes":16777216,"files":10000,"total_bytes":268435456,"depth":64,"output_bytes":16777216`), nil)
	if err != nil || at.SetSHA256 != base.SetSHA256 || at.Policy.ref() != base.Policy.ref() {
		t.Fatalf("limits equal to the operation: %v", err)
	}
	for _, key := range []string{"files", "file_bytes", "total_bytes", "depth", "output_bytes"} {
		_, err := run(lim(`"`+key+`":`+strconv.FormatUint(map[string]uint64{"files": 10001, "file_bytes": 16777217, "total_bytes": 268435457, "depth": 65, "output_bytes": 16777217}[key], 10)), nil)
		kindOf(t, err, KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION")
	}
	// The caller's own lower limit is the bound, not the product default.
	_, err = run(lim(`"files":6`), func(r *IdentityRequest) { r.Limits.Files = 5 })
	kindOf(t, err, KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION")
	narrowed, err := run(lim(`"file_bytes":7`), nil)
	kindOf(t, err, KindResourceLimit, "FILE_BYTES_LIMIT")
	if narrowed.Policy.FileBytes != 7 || narrowed.Policy.ref() == base.Policy.ref() {
		t.Fatalf("narrowed policy not visible: %+v", narrowed.Policy)
	}
	for _, key := range []string{"records", "archive_entries", "archive_bytes", "archive_depth"} {
		_, err := run(lim(`"`+key+`":1`), nil)
		kindOf(t, err, KindInvalidInput, "PROFILE_LIMIT_NOT_APPLICABLE")
	}
	// The registered large-file exception is a request field; a profile cannot name it.
	_, err = run(`{"schema":"tsgk-profile/r1","id":"l","large_file_profile":"pg-large-source-r1"}`, nil)
	kindOf(t, err, KindInvalidInput, "JSON_UNKNOWN_FIELD")
	// Encoding declarations bind into identity exactly like request declarations.
	enc, err := run(`{"schema":"tsgk-profile/r1","id":"e","encoding":{"files":[{"path":"src/scanner.c","encoding":"utf-8"}]}}`, nil)
	if err != nil || enc.Manifest.Files[3].Path != "src/scanner.c" || enc.Manifest.Files[3].Encoding.Source != SourceDeclaration || enc.SetSHA256 == base.SetSHA256 {
		t.Fatalf("profile declaration: %v %+v", err, enc.Manifest.Files)
	}
	_, err = run(`{"schema":"tsgk-profile/r1","id":"e","encoding":{"files":[{"path":"src/missing.c","encoding":"utf-8"}]}}`, nil)
	kindOf(t, err, KindInvalidInput, "DECLARATION_UNMATCHED")
	_, err = run(`{"schema":"tsgk-profile/r1","id":"e","encoding":{"profile":"cp949"}}`, func(r *IdentityRequest) { r.Encoding.Profile = "cp949" })
	kindOf(t, err, KindInvalidInput, "ENCODING_SOURCE_CONFLICT")
	_, err = run(goodProfile, func(r *IdentityRequest) { r.Selection.Files = []FileSelection{{"grammar.js", "grammar"}} })
	kindOf(t, err, KindInvalidInput, "SELECTION_SOURCE_CONFLICT")
	if res, err := run(goodProfile, nil); err != nil || res.Identities[len(res.Identities)-1].Role != "profile" || len(res.Manifest.Files) != 1 {
		t.Fatalf("profile selection: %v %+v", err, res.Identities)
	}
}

// S02-A16: a corpus profile is bounded by the caller's limits and by the tracked
// private-corpus-local values, and cannot select files.
func TestCorpusProfile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.cs": "class A {}\n"})
	run := func(prof string, l CorpusLimits) error {
		_, err := Corpus(testCtx(t), CorpusRequest{Root: root, Limits: l, Profile: []byte(prof)})
		return err
	}
	tracked := DefaultCorpusLimits()
	raised := tracked
	raised.Files, raised.FileBytes = tracked.Files*2, tracked.FileBytes*2
	lim := func(body string) string { return `{"schema":"tsgk-profile/r1","id":"c","limits":{` + body + `}}` }
	if err := run(lim(`"files":26000,"records":26000,"file_bytes":33554432,"total_bytes":3489660928,"depth":32,"output_bytes":67108864`), tracked); err != nil {
		t.Fatalf("tracked values: %v", err)
	}
	kindOf(t, run(lim(`"files":26001`), raised), KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION")
	kindOf(t, run(lim(`"file_bytes":33554433`), raised), KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION")
	low := tracked
	low.Records = 10
	kindOf(t, run(lim(`"records":11`), low), KindInvalidInput, "PROFILE_LIMIT_ABOVE_OPERATION")
	kindOf(t, run(lim(`"archive_entries":1`), tracked), KindInvalidInput, "PROFILE_LIMIT_NOT_APPLICABLE")
	kindOf(t, run(goodProfile, tracked), KindInvalidInput, "PROFILE_FILES_NOT_APPLICABLE")
	kindOf(t, run(lim(`"records":0`), tracked), KindInvalidInput, "PROFILE_LIMIT_INVALID")
	res, err := Corpus(testCtx(t), CorpusRequest{Root: root, Limits: tracked, Profile: []byte(`{"schema":"tsgk-profile/r1","id":"c","encoding":{"profile":"cp949"},"limits":{"files":1}}`)})
	if err != nil || res.Policy.EncodingPolicy != "detect-r1;profile=cp949" || res.Policy.Files != 1 || res.Identities[len(res.Identities)-1].Role != "profile" {
		t.Fatalf("corpus profile: %v %+v", err, res.Policy)
	}
}
