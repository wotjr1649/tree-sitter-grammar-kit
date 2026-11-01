package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationScript17StaticProfilesAndPins(t *testing.T) {
	registryData, err := os.ReadFile("../../contracts/native-routes.json")
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
	if err := json.Unmarshal(registryData, &registry); err != nil {
		t.Fatal(err)
	}
	var definitions, patterns, captureEntries uint32
	allCaptureNames := make(map[string]bool)
	for _, name := range []string{"python8-empty", "ruby4-empty", "r3-empty", "bash2-empty"} {
		t.Run(name, func(t *testing.T) {
			profile, ok := classificationScript17ProfileFor(name)
			routeProfile, routeOK := classificationScript17ProfileForRoute(profile.route)
			if !ok || !routeOK || routeProfile.name != name || !classificationGoCaptureEntrySkipsProfile(name) {
				t.Fatal("closed Script17 route or capture entry differs")
			}
			if _, ok := classificationWeb16ProfileFor(name); ok {
				t.Fatal("Script17 profile entered the closed Web16 set")
			}
			if _, ok := classificationSmall17ProfileFor(name); ok {
				t.Fatal("Script17 profile entered the closed Small17 set")
			}
			anchorData, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
			if err != nil || len(anchorData) != profile.anchorBytes || sha(anchorData) != profile.anchorSHA {
				t.Fatal("Script17 anchor pin differs", err)
			}
			var anchors classificationLocalAnchors
			if err := jsonv2.Unmarshal(anchorData, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			if err := classificationEmptyAnchorsValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			if err := classificationEmptyQueryRegistryValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			var grammar []kit.NativeInput
			matches := 0
			for _, row := range registry.Routes {
				if row.Route == profile.route {
					matches++
					if row.Symbol != profile.symbol {
						t.Fatal("Script17 grammar symbol differs")
					}
					grammar = row.Files
				}
			}
			if matches != 1 {
				t.Fatal("Script17 grammar route missing or duplicated")
			}
			if err := classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, grammar); err != nil {
				t.Fatal(err)
			}
			capture, err := classificationCaptureProfileFor(name)
			if err != nil || capture.anchorFile != profile.anchorFile || capture.anchorSHA != profile.anchorSHA || capture.route != profile.route ||
				capture.symbol != profile.symbol || capture.queryCount != len(profile.queries) || capture.grammarCount != 5 || capture.paged {
				t.Fatal("Script17 capture dispatch differs", err)
			}
			for _, mutate := range []func(*classificationLocalAnchors){
				func(a *classificationLocalAnchors) { a.API = true },
				func(a *classificationLocalAnchors) { a.Route = "javascript" },
				func(a *classificationLocalAnchors) { a.ABI.SymbolCount++ },
				func(a *classificationLocalAnchors) { a.Nodes[0].Type = "wrong_root" },
				func(a *classificationLocalAnchors) { a.Nodes[0].Named = false },
				func(a *classificationLocalAnchors) { a.Nodes[0].EndByte = 1 },
				func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] },
				func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil },
				func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}} },
				func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil },
				func(a *classificationLocalAnchors) { a.Required[0].Expected = nil },
			} {
				bad := anchors
				bad.Nodes = slices.Clone(anchors.Nodes)
				bad.Queries = slices.Clone(anchors.Queries)
				bad.Required = slices.Clone(anchors.Required)
				mutate(&bad)
				if classificationEmptyAnchorsValid(name, bad) == nil {
					t.Fatal("mutated Script17 source/API/CST/ABI/order/metadata accepted")
				}
			}
			badGrammar := slices.Clone(grammar)
			badGrammar[0].Bytes = 16<<20 + 1
			if classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, badGrammar) == nil {
				t.Fatal("parser beyond the plain-input cap accepted")
			}
			badGrammar = slices.Clone(grammar)
			badGrammar[1].Path += ".alias"
			if classificationEmptyGrammarValid(name, registryData, profile.route, profile.symbol, badGrammar) == nil {
				t.Fatal("scanner alias accepted")
			}
			queryPath := "../../" + strings.TrimPrefix(profile.queryFile, "src/")
			queryData, err := os.ReadFile(queryPath)
			if err != nil {
				t.Fatal(err)
			}
			if classificationEmptyQueryRegistryBytesValid(name, anchors, append(slices.Clone(queryData), ' ')) == nil ||
				classificationEmptyGrammarValid(name, append(slices.Clone(registryData), ' '), profile.route, profile.symbol, grammar) == nil {
				t.Fatal("changed query or native registry accepted")
			}
			definitions += uint32(len(profile.queries))
			for _, query := range profile.queries {
				patterns += query.patterns
				captureEntries += uint32(len(query.captures))
				for _, captureName := range query.captures {
					allCaptureNames[captureName] = true
				}
			}
		})
	}
	if definitions != 17 || patterns != 52 || captureEntries != 59 || len(allCaptureNames) != 47 {
		t.Fatal("closed Script17 metadata denominator differs", definitions, patterns, captureEntries, len(allCaptureNames))
	}
	for _, name := range []string{"", "positive-batch", "javascript4-empty", "python", "python8-empty-alias", "ruby4-empty ", "r3", "bash2-empty2"} {
		if classificationScript17ProfileName(name) {
			t.Fatal("non-Script17 name entered its closed entry", name)
		}
	}
}

func TestQueryClassificationScript17LocalCapture(t *testing.T) {
	if !classificationScript17ProfileName(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")) {
		t.Skip("explicit closed Script17 classification profile required")
	}
	classificationNativeLocalCapture(t)
}

func TestQueryClassificationScript17EmptyStderrCaptureMembers(t *testing.T) {
	for _, name := range []string{"python8-empty", "ruby4-empty", "r3-empty", "bash2-empty"} {
		for _, stderr := range [][]byte{{}, {0x41, 0x42}} {
			process := &runner.Result{StderrBytes: int64(len(stderr))}
			owner := &runner.Result{StderrBytes: int64(len(stderr))}
			if len(stderr) != 0 {
				process.Stderr, owner.Stderr = slices.Clone(stderr), slices.Clone(stderr)
			}
			result := &CaseResult{Process: process}
			capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: owner}
			classificationNormalizeCaptureStderr(name, result, capture)
			if process.Stderr == nil || owner.Stderr == nil || !slices.Equal(process.Stderr, stderr) || !slices.Equal(owner.Stderr, stderr) {
				t.Fatal("Script17 stderr bytes were changed or not made explicit")
			}
			files, err := capture.files("capture/" + name)
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, file := range files {
				if file.pin.Role == "stderr" {
					found++
					if file.pin.Bytes != uint64(len(stderr)) || file.pin.SHA256 != sha(stderr) || file.data == nil || !slices.Equal(file.data, stderr) {
						t.Fatal("Script17 stderr member differs")
					}
				}
			}
			if found != 1 {
				t.Fatal("Script17 stderr member missing or duplicated")
			}
		}
	}
}
