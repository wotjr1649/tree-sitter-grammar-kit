package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// corpusTestProfile writes beside the plan a corpus-mode profile of the registered route that
// covers exactly the given cases, as executeCorpus requires, and returns its pin. The caller
// runs from the repository root, where the route registry is read.
func corpusTestProfile(t *testing.T, dir, name string, source []byte, edit func(map[string]any), cases ...corpusExpected) pinnedFile {
	t.Helper()
	data, err := os.ReadFile("src/contracts/native-routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Routes []struct {
			Route  string            `json:"route"`
			Symbol string            `json:"symbol"`
			Files  []kit.NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err = json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(registry.Routes, func(r struct {
		Route  string            `json:"route"`
		Symbol string            `json:"symbol"`
		Files  []kit.NativeInput `json:"files"`
	}) bool {
		return r.Route == "csharp"
	})
	r := registry.Routes[i]
	slices.SortFunc(r.Files, func(a, b kit.NativeInput) int { return strings.Compare(a.Path, b.Path) })
	var list []map[string]any
	for _, c := range cases {
		list = append(list, map[string]any{"id": c.ID, "input": c.Input, "edits": []any{}, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil})
	}
	doc := map[string]any{"schema": kit.OracleSchema, "id": "corpus", "route": r.Route, "operation": "native-query-large", "symbol": r.Symbol, "encoding": cases[0].Encoding, "output": kit.OutputAuto,
		"compiler": kit.ToolIdentity{Name: "cc", Version: "host", SHA256: strings.Repeat("a", 64), Bytes: 1}, "grammar": r.Files, "declarations": nil,
		"queries": []any{}, "fact_pack": nil, "api": true, "cases": list}
	if cases[0].Route == "svc" {
		doc["format"] = kit.SvcFormat
		for _, c := range list {
			c["svc_source"] = base64.StdEncoding.EncodeToString(source)
		}
	}
	if edit != nil {
		edit(doc)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
		t.Fatal(err)
	}
	return pinnedFile{Path: name, SHA: digest(b), Bytes: uint64(len(b))}
}

// This is a data-only replay fixture, not an observation of native execution.
func TestCorpusReplayRejectsBindingAndCSTTampering(t *testing.T) {
	t.Chdir("../../..")
	for _, mutation := range []string{"none", "profile_missing", "profile_case", "profile_route", "profile_bytes", "source", "page", "duplicate", "missing", "producer", "policy", "encoding", "coherent_UTF16LE", "coherent_UTF16BE", "coherent_unknown", "coherent_surrogateLE", "coherent_surrogateBE", "coherent_NULLE", "coherent_NULBE", "coherent_CP949", "cleanup", "memory", "scope", "method", "detail", "backend", "escaped", "navigation", "CST_flag", "CST_point", "CST_type", "CST_field", "short_row"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			dataDir := filepath.Join(dir, "data")
			sourceDir := filepath.Join(dir, "source")
			for _, p := range []string{dataDir, sourceDir} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			write := func(name string, b []byte) {
				t.Helper()
				if err := os.WriteFile(name, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			marshal := func(x any) []byte {
				t.Helper()
				b, err := json.Marshal(x)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			source := []byte("a")
			switch mutation {
			case "coherent_surrogateLE":
				source = []byte{0, 0xd8}
			case "coherent_surrogateBE":
				source = []byte{0xd8, 0}
			case "coherent_NULLE", "coherent_NULBE":
				source = []byte{0, 0}
			case "coherent_CP949":
				source = []byte{0x81, 0x20}
			}
			write(filepath.Join(sourceDir, "input"), source)
			p := publicPage()
			tree := newCorpusTree()
			if err := tree.consume(p, 1); err != nil {
				t.Fatal(err)
			}
			errFlag := false
			e := corpusExpected{ID: "control", Route: "csharp", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "input", Role: "case", SHA256: digest([]byte("a")), Bytes: 1}, Nodes: 2, Digest: kit.TreeDigest([]kit.TreeNode{{Parent: -1, Type: "root", Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}, {Parent: 0, Type: "leaf", Field: &p.CST.Fields[0], Named: true, EndByte: 1, EndPoint: kit.Point{Column: 1}}}), HasError: &errFlag}
			if strings.HasPrefix(mutation, "coherent_") {
				e.Encoding = map[string]string{"coherent_UTF16LE": kit.EncodingUTF16LE, "coherent_UTF16BE": kit.EncodingUTF16BE, "coherent_unknown": "unknown", "coherent_surrogateLE": kit.EncodingUTF16LE, "coherent_surrogateBE": kit.EncodingUTF16BE, "coherent_NULLE": kit.EncodingUTF16LE, "coherent_NULBE": kit.EncodingUTF16BE, "coherent_CP949": kit.EncodingCP949}[mutation]
				p.Encoding = e.Encoding
				e.Input.SHA256, e.Input.Bytes = digest(source), uint64(len(source))
				p.SourceSHA, p.SourceBytes = e.Input.SHA256, e.Input.Bytes
			}
			plan := corpusPlan{Schema: "tsgk-corpus-cst-api-plan/r1", Candidate: strings.Repeat("a", 40), Expected: []corpusExpected{e}}
			// the profile is pinned with the plan; a later change to its cases, route or bytes must not replay
			pe := e
			switch mutation {
			case "profile_case":
				pe.ID = "other"
			case "profile_route":
				pe.Route = "svc"
			}
			if mutation != "profile_missing" {
				plan.Profiles = []pinnedFile{corpusTestProfile(t, dir, "profile.json", source, nil, pe)}
			}
			if mutation == "profile_bytes" {
				write(filepath.Join(dir, "profile.json"), []byte("{}"))
			}
			planBytes := marshal(plan)
			planPath := filepath.Join(dir, "plan.json")
			write(planPath, planBytes)
			producer := corpusProducer{Route: "csharp"}
			producerID := digest(marshal(producer))
			if mutation == "navigation" {
				p.Divergences = 1
				p.FirstDivergence = []int64{1, 2, 1, -1}
			}
			if mutation == "CST_flag" {
				p.CST.Nodes[1][3] |= 2
			}
			if mutation == "CST_point" {
				p.CST.Nodes[1][7] = 1
			}
			if mutation == "CST_type" {
				p.CST.Types[1] = "other"
			}
			if mutation == "CST_field" {
				p.CST.Fields[0] = "other"
			}
			if mutation == "short_row" {
				p.CST.Nodes[1] = p.CST.Nodes[1][:9]
			}
			page := native.Frame(marshal(p))
			write(filepath.Join(dataDir, "page.bin"), page)
			binding := corpusBinding{ID: e.ID, Route: e.Route, Encoding: e.Encoding, Input: e.Input, Producer: producerID, Policy: corpusPolicySHA(corpusStorageBytes, 3600), Page: pinnedFile{Path: "page.bin", SHA: digest(page), Bytes: uint64(len(page))}, End: 2, Nodes: 2, Process: runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, StdoutBytes: int64(len(page)), Cleanup: runner.Cleanup{Method: "JOB_OBJECT", Verified: true, Scope: "JOB_OBJECT"}, Capabilities: runner.Capabilities{OS: "windows/amd64", Backend: "windows-job-object", TreeCleanup: "JOB_OBJECT", EscapedDescendants: runner.Contained, Memory: runner.MemoryHard}, Memory: runner.MemoryObservation{Enforcement: runner.MemoryHard, Limit: 8589934592, PeakKnown: true}}}
			switch mutation {
			case "producer":
				binding.Producer = strings.Repeat("b", 64)
			case "policy":
				binding.Policy = strings.Repeat("b", 64)
			case "encoding":
				binding.Encoding = "CP949"
			case "cleanup":
				binding.Process.Cleanup.Verified = false
			case "memory":
				binding.Process.Memory.Limit = 1
			case "scope":
				binding.Process.Cleanup.Scope = "PROCESS_GROUP"
			case "method":
				binding.Process.Cleanup.Method = "PROCESS_GROUP"
			case "detail":
				binding.Process.Cleanup.Detail = "tree query failed"
			case "backend":
				binding.Process.Capabilities.Backend = "linux-cgroup-v2"
			case "escaped":
				binding.Process.Capabilities.EscapedDescendants = "NOT_CONTAINED"
			}
			bindings := append(marshal(binding), '\n')
			if mutation == "duplicate" {
				bindings = append(bindings, bindings...)
			}
			if mutation == "missing" {
				bindings = nil
			}
			write(filepath.Join(dataDir, "bindings.jsonl"), bindings)
			result := corpusResult{Schema: "tsgk-corpus-cst-api-result/r1", PlanSHA: digest(planBytes), Candidate: plan.Candidate, Policy: corpusPolicySHA(corpusStorageBytes, 3600), HostSHA: strings.Repeat("a", 64), StorageLimit: corpusStorageBytes, WallLimit: 3600, Expected: 1, Completed: 1, API: 1, Nodes: 2, Pages: 1, Bytes: uint64(len(bindings) + len(page)), Checks: 37, Producers: map[string]corpusProducer{"csharp": producer}, BindingsSHA: digest(bindings), Complete: true}
			resultBytes := marshal(result)
			write(filepath.Join(dataDir, "result.json"), resultBytes)
			registration := corpusRegistration{Schema: "tsgk-corpus-cst-api-registration/r1", PlanSHA: result.PlanSHA, HostSHA: result.HostSHA, Result: pinnedFile{Path: "result.json", SHA: digest(resultBytes), Bytes: uint64(len(resultBytes))}, Bindings: pinnedFile{Path: "bindings.jsonl", SHA: digest(bindings), Bytes: uint64(len(bindings))}}
			registrationPath := filepath.Join(dir, "registration.json")
			write(registrationPath, marshal(registration))
			if mutation == "source" {
				write(filepath.Join(sourceDir, "input"), []byte("b"))
			}
			if mutation == "page" {
				write(filepath.Join(dataDir, "page.bin"), append(page, 0))
			}
			err := replayCorpus(dataDir, sourceDir, planPath, registrationPath, filepath.Join(dir, "report.json"))
			if (err == nil) != (mutation == "none") {
				t.Fatalf("mutation=%s err=%v", mutation, err)
			}
			if want, ok := map[string]string{"profile_missing": "CORPUS_REPLAY_PLAN_INVALID", "profile_case": "CORPUS_PROFILE_SET_MISMATCH", "profile_route": "CORPUS_PROFILE_SET_MISMATCH", "profile_bytes": "PINNED_INPUT_SIZE_MISMATCH"}[mutation]; ok && err.Error() != want {
				t.Fatalf("mutation=%s err=%v, want %s", mutation, err, want)
			}
		})
	}
}

// The shared profile check holds one producer per route and the pinned source of an SVC case,
// which execution needs and replay must not skip.
func TestCorpusProfilesSharedChecks(t *testing.T) {
	t.Chdir("../../..")
	src := []byte("<%@ ServiceHost Service=\"A\" %>")
	in := kit.NativeInput{Path: "input", Role: "case", SHA256: digest(src), Bytes: uint64(len(src))}
	a := corpusExpected{ID: "a", Route: "csharp", Encoding: kit.EncodingUTF8, Input: in}
	b := corpusExpected{ID: "b", Route: "csharp", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "other", Role: "case", SHA256: digest(src), Bytes: uint64(len(src))}}
	svc := corpusExpected{ID: "s", Route: "svc", Encoding: kit.EncodingUTF8, Input: in}
	otherCompiler := func(doc map[string]any) {
		doc["compiler"] = kit.ToolIdentity{Name: "cc", Version: "other", SHA256: strings.Repeat("b", 64), Bytes: 1}
	}
	noSource := func(doc map[string]any) {
		for _, c := range doc["cases"].([]map[string]any) {
			delete(c, "svc_source")
		}
	}
	for _, tc := range []struct {
		name  string
		build func(dir string) []pinnedFile
		cases []corpusExpected
		want  string
	}{
		{"same-producer", func(dir string) []pinnedFile {
			return []pinnedFile{corpusTestProfile(t, dir, "a.json", src, nil, a), corpusTestProfile(t, dir, "b.json", src, nil, b)}
		}, []corpusExpected{a, b}, ""},
		{"compiler-differs", func(dir string) []pinnedFile {
			return []pinnedFile{corpusTestProfile(t, dir, "a.json", src, nil, a), corpusTestProfile(t, dir, "b.json", src, otherCompiler, b)}
		}, []corpusExpected{a, b}, "CORPUS_PRODUCER_MISMATCH"},
		{"svc-with-source", func(dir string) []pinnedFile { return []pinnedFile{corpusTestProfile(t, dir, "s.json", src, nil, svc)} }, []corpusExpected{svc}, ""},
		{"svc-without-source", func(dir string) []pinnedFile {
			return []pinnedFile{corpusTestProfile(t, dir, "s.json", src, noSource, svc)}
		}, []corpusExpected{svc}, "CORPUS_PROFILE_SET_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plan := corpusPlan{Profiles: tc.build(dir)}
			expected := map[string]corpusExpected{}
			for _, e := range tc.cases {
				expected[e.ID] = e
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if _, err = corpusProfiles(root, plan, expected); (err == nil) != (tc.want == "") || err != nil && err.Error() != tc.want {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}
