package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestCorpusSourceEncodingAndSvcReplayEligibility(t *testing.T) {
	t.Chdir("../../..")
	valid := `<%@ ServiceHost Language="C#" Service="C" %>`
	type svcTest struct {
		name, encoding, text string
		bytes                []byte
		pass                 bool
	}
	tests := []svcTest{
		{name: "UTF8", encoding: kit.EncodingUTF8, text: valid, pass: true},
		{name: "CP949", encoding: kit.EncodingCP949, text: valid, pass: true},
		{name: "UTF16LE", encoding: kit.EncodingUTF16LE, text: valid, pass: true},
		{name: "UTF16BE", encoding: kit.EncodingUTF16BE, text: valid, pass: true},
		{name: "inline", encoding: kit.EncodingUTF8, text: valid + "\nclass C {}"},
		{name: "inline-no-language", encoding: kit.EncodingUTF8, text: `<%@ ServiceHost Service="C" %>` + "\nclass C {}"},
		{name: "absent", encoding: kit.EncodingUTF8, text: "class C {}"},
		{name: "malformed", encoding: kit.EncodingUTF8, text: `<%@ ServiceHost Language="C#"`},
		{name: "diagnostic", encoding: kit.EncodingUTF8, text: `<%@ ServiceHost Language=C# %>`},
		{name: "odd-LE", encoding: kit.EncodingUTF16LE, bytes: []byte{'a'}},
		{name: "odd-BE", encoding: kit.EncodingUTF16BE, bytes: []byte{'a'}},
		{name: "surrogate-LE", encoding: kit.EncodingUTF16LE, bytes: []byte{0, 0xd8}},
		{name: "surrogate-BE", encoding: kit.EncodingUTF16BE, bytes: []byte{0xd8, 0}},
		{name: "NUL", encoding: kit.EncodingUTF16LE, bytes: []byte{0, 0}},
		{name: "bad-CP949", encoding: kit.EncodingCP949, bytes: []byte{0x81, 0x20}},
		{name: "unknown", encoding: "unknown", text: valid},
	}
	for _, enc := range []string{kit.EncodingUTF8, kit.EncodingUTF16LE, kit.EncodingUTF16BE, kit.EncodingCP949} {
		if enc != kit.EncodingCP949 {
			tests = append(tests, svcTest{name: enc + "-Unicode-space", encoding: enc, text: "\u3000" + valid + "\u00a0", pass: true})
		}
		for _, pattern := range []struct {
			name, text string
			pass       bool
		}{
			{"assembly-after", valid + "\n<%@ Assembly Name=\"A\" %>", true},
			{"assembly-before", "<%@ Assembly Name=\"A\" %>\n" + valid, true},
			{"duplicate", valid + "\n<%@ ServiceHost Service=\"D\" %>", false},
			{"unknown-additional", valid + "\n<%@ Unknown Name=\"A\" %>", false},
			{"diagnostic-additional", valid + "\n<%@ Assembly %>", false},
			{"missing-close-additional", valid + "\n<%@ Assembly Name=\"A\"", false},
			{"prefix-code", "class C {}\n" + valid, false},
			{"gap-code", valid + "\nclass C {}\n<%@ Assembly Name=\"A\" %>", false},
		} {
			tests = append(tests, svcTest{name: enc + "-" + pattern.name, encoding: enc, text: pattern.text, pass: pattern.pass})
		}
	}
	for _, enc := range []string{kit.EncodingUTF8, kit.EncodingUTF16LE, kit.EncodingUTF16BE} {
		tests = append(tests, svcTest{name: enc + "-BOM", encoding: enc, text: valid, pass: true})
	}
	tests = append(tests, svcTest{name: "CP949-Unicode-space", encoding: kit.EncodingCP949, bytes: append(append([]byte{0xa1, 0xa1}, []byte(valid)...), 0xa1, 0xa1), pass: true})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			sourceDir, dataDir := filepath.Join(dir, "source"), filepath.Join(dir, "data")
			for _, p := range []string{sourceDir, dataDir} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			b := test.bytes
			if b == nil {
				if test.encoding == kit.EncodingUTF16LE || test.encoding == kit.EncodingUTF16BE {
					for _, u := range utf16.Encode([]rune(test.text)) {
						if test.encoding == kit.EncodingUTF16LE {
							b = binary.LittleEndian.AppendUint16(b, u)
						} else {
							b = binary.BigEndian.AppendUint16(b, u)
						}
					}
				} else {
					b = []byte(test.text)
				}
			}
			if strings.HasSuffix(test.name, "-BOM") {
				bom := map[string][]byte{kit.EncodingUTF8: {0xef, 0xbb, 0xbf}, kit.EncodingUTF16LE: {0xff, 0xfe}, kit.EncodingUTF16BE: {0xfe, 0xff}}[test.encoding]
				b = append(bom, b...)
			}
			marshal := func(v any) []byte {
				t.Helper()
				x, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return x
			}
			write := func(p string, b []byte) {
				t.Helper()
				if err := os.WriteFile(p, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(sourceDir, "input"), b)
			e := corpusExpected{ID: "svc", Route: "svc", Encoding: test.encoding, Input: kit.NativeInput{Path: "input", Role: "case", SHA256: digest(b), Bytes: uint64(len(b))}}
			plan := corpusPlan{Schema: "tsgk-corpus-cst-api-plan/r1", Candidate: strings.Repeat("a", 40), Expected: []corpusExpected{e}, Profiles: []pinnedFile{corpusTestProfile(t, dir, "profile.json", b, nil, e)}}
			planBytes := marshal(plan)
			write(filepath.Join(dir, "plan.json"), planBytes)
			policy := corpusPolicySHA(corpusStorageBytes, 3600)
			// Coherent pins and the real observation deliberately do not establish eligibility.
			page := marshal(map[string]any{"case": e, "policy": policy, "api": "NOT_CLAIMED", "observation": kit.ObserveServiceHost(e.Encoding, b)})
			write(filepath.Join(dataDir, "page.json"), page)
			producer := corpusTestProducer(t)
			binding := corpusBinding{ID: e.ID, Route: e.Route, Encoding: e.Encoding, Input: e.Input, Producer: digest(marshal(producer)), Policy: policy, Page: pinnedFile{Path: "page.json", SHA: digest(page), Bytes: uint64(len(page))}}
			bindings := append(marshal(binding), '\n')
			write(filepath.Join(dataDir, "bindings.jsonl"), bindings)
			result := corpusResult{Schema: "tsgk-corpus-cst-api-result/r1", PlanSHA: digest(planBytes), Candidate: plan.Candidate, Policy: policy, HostSHA: strings.Repeat("b", 64), StorageLimit: corpusStorageBytes, WallLimit: 3600, Expected: 1, Completed: 1, SvcNotClaimed: 1, Bytes: uint64(len(page) + len(bindings)), Producers: map[string]corpusProducer{"csharp": producer}, BindingsSHA: digest(bindings), Complete: true}
			resultBytes := marshal(result)
			write(filepath.Join(dataDir, "result.json"), resultBytes)
			registration := corpusRegistration{Schema: "tsgk-corpus-cst-api-registration/r1", PlanSHA: result.PlanSHA, HostSHA: result.HostSHA, Result: pinnedFile{Path: "result.json", SHA: digest(resultBytes), Bytes: uint64(len(resultBytes))}, Bindings: pinnedFile{Path: "bindings.jsonl", SHA: digest(bindings), Bytes: uint64(len(bindings))}}
			write(filepath.Join(dir, "registration.json"), marshal(registration))
			err := replayCorpus(dataDir, sourceDir, filepath.Join(dir, "plan.json"), filepath.Join(dir, "registration.json"), filepath.Join(dir, "report.json"))
			if (err == nil) != test.pass {
				t.Fatalf("pass=%v err=%v", test.pass, err)
			}
		})
	}
}

func TestCorpusRefusesWritesInOriginalRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{root, filepath.Join(root, "nested")} {
		if name != root {
			if err := os.Mkdir(name, 0700); err != nil {
				t.Fatal(err)
			}
		}
		inside := filepath.Join(name, "new-output")
		outside := filepath.Join(dir, "new-output")
		for _, paths := range [][2]string{{inside, outside}, {outside, inside}} {
			if err := executeCorpus("unused", "unused", paths[0], paths[1], root, "unused"); err == nil || err.Error() != "CORPUS_WRITE_INSIDE_SOURCE_ROOT" {
				t.Fatalf("execution boundary: %v", err)
			}
		}
		if err := replayCorpus("unused", root, "unused", "unused", inside); err == nil || err.Error() != "CORPUS_WRITE_INSIDE_SOURCE_ROOT" {
			t.Fatalf("replay boundary: %v", err)
		}
		if _, err := os.Stat(inside); !os.IsNotExist(err) {
			t.Fatal("rejected output was created")
		}
	}
	if !outsideCorpusRoot(filepath.Join(dir, "outside"), root) {
		t.Fatal("external task output refused")
	}
}

func TestCorpusOnlyAcceptsR2AndRegisteredFormats(t *testing.T) {
	p := kit.OracleProfile{Native: kit.IncrementalProfile{Schema: kit.OracleSchema, Operation: "native-query-large", Output: kit.OutputAuto}, API: true}
	for _, format := range []string{"", kit.SvcFormat} {
		p.Native.Format = format
		if !corpusProfileAllowed(p) {
			t.Fatal("r2 profile refused")
		}
		p.Native.Schema = "tsgk-oracle/r1"
		if corpusProfileAllowed(p) {
			t.Fatal("r1 profile accepted")
		}
		p.Native.Schema = kit.OracleSchema
	}
	p.Native.Format = kit.SvcLegacyFormat
	if corpusProfileAllowed(p) {
		t.Fatal("legacy format accepted")
	}
	// Both schemas remain valid product input; only the corpus mode narrows acceptance.
	for _, schema := range []string{kit.OracleSchema, "tsgk-oracle/r1"} {
		x := map[string]any{"schema": schema, "id": "public", "route": "csharp", "operation": "native-query-large", "symbol": "tree_sitter_c_sharp", "encoding": kit.EncodingUTF8, "output": kit.OutputAuto,
			"compiler": kit.ToolIdentity{Name: "cc", Version: "host", SHA256: strings.Repeat("a", 64), Bytes: 1}, "grammar": []kit.NativeInput{{Path: "src/parser.c", Role: "parser", SHA256: strings.Repeat("a", 64), Bytes: 1}}, "declarations": nil,
			"queries": []any{}, "fact_pack": nil, "api": true, "cases": []map[string]any{{"id": "public", "input": kit.NativeInput{Path: "input", Role: "case", SHA256: strings.Repeat("a", 64), Bytes: 1}, "edits": []any{}, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil}}}
		b, err := json.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := kit.ParseOracleProfile(b)
		if err != nil || corpusProfileAllowed(parsed) != (schema == kit.OracleSchema) {
			t.Fatalf("schema=%s err=%v", schema, err)
		}
	}
}
