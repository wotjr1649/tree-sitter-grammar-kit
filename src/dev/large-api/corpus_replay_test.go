package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// This is a data-only replay fixture, not an observation of native execution.
func TestCorpusReplayRejectsBindingAndCSTTampering(t *testing.T) {
	for _, mutation := range []string{"none", "source", "page", "duplicate", "missing", "producer", "policy", "encoding", "coherent_UTF16LE", "coherent_UTF16BE", "coherent_unknown", "coherent_surrogateLE", "coherent_surrogateBE", "coherent_NULLE", "coherent_NULBE", "coherent_CP949", "cleanup", "memory", "scope", "navigation", "CST_flag", "CST_point", "CST_type", "CST_field", "short_row"} {
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
			binding := corpusBinding{ID: e.ID, Route: e.Route, Encoding: e.Encoding, Input: e.Input, Producer: producerID, Policy: corpusPolicySHA(corpusStorageBytes, 3600), Page: pinnedFile{Path: "page.bin", SHA: digest(page), Bytes: uint64(len(page))}, End: 2, Nodes: 2, Process: runner.Result{Status: runner.StatusCompleted, Reason: runner.ReasonExited, StdoutBytes: int64(len(page)), Cleanup: runner.Cleanup{Verified: true, Scope: "JOB_OBJECT"}, Memory: runner.MemoryObservation{Enforcement: runner.MemoryHard, Limit: 8589934592, PeakKnown: true}}}
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
		})
	}
}
