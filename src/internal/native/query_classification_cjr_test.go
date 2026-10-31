package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"reflect"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationCJRStaticProfiles(t *testing.T) {
	profiles := []string{"c4-empty", "java5-empty", "rust4-empty"}
	for _, name := range profiles {
		t.Run(name, func(t *testing.T) {
			spec, ok := classificationCJRSpecFor(name)
			if !ok {
				t.Fatal("closed C/Java/Rust profile missing")
			}
			profile, err := classificationCaptureProfileFor(name)
			if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA || profile.route != spec.route || profile.symbol != spec.symbol || profile.queryCount != len(spec.querySpecs) || profile.grammarCount != len(spec.grammarFiles) || profile.paged {
				t.Fatalf("closed unpaged profile changed: %#v, %v", profile, err)
			}
			data, err := os.ReadFile("../../testdata/query-semantics/" + spec.anchorFile)
			if err != nil || sha(data) != spec.anchorSHA {
				t.Fatal("pinned empty-definition anchors changed", err)
			}
			var anchors classificationLocalAnchors
			if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			if err := classificationCJRAnchorsValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			if err := classificationCJRQueryRegistryValid(name, anchors); err != nil {
				t.Fatal(err)
			}
			var registry struct {
				Routes []struct {
					Route  string            `json:"route"`
					Symbol string            `json:"symbol"`
					Files  []kit.NativeInput `json:"files"`
				} `json:"routes"`
			}
			registryData, err := os.ReadFile("../../contracts/native-routes.json")
			if err != nil || json.Unmarshal(registryData, &registry) != nil {
				t.Fatal("native grammar registry", err)
			}
			matches := 0
			for _, route := range registry.Routes {
				if route.Route == spec.route {
					matches++
					if route.Symbol != spec.symbol || !classificationCJRGrammarValid(name, route.Files) {
						t.Fatal("pinned route symbol or closed unpaged grammar closure changed")
					}
				}
			}
			if matches != 1 {
				t.Fatal("closed route missing or duplicated", spec.route)
			}
		})
	}
	for _, name := range []string{"c4-empty-paged", "java-empty", "rust4-empty-extra"} {
		if profile, err := classificationCaptureProfileFor(name); err == nil {
			t.Fatalf("unknown C/Java/Rust profile was accepted: %q, %#v", name, profile)
		}
	}
}

func TestQueryClassificationCJRAnchorGuards(t *testing.T) {
	for _, name := range []string{"c4-empty", "java5-empty", "rust4-empty"} {
		t.Run(name, func(t *testing.T) {
			spec, _ := classificationCJRSpecFor(name)
			data, err := os.ReadFile("../../testdata/query-semantics/" + spec.anchorFile)
			if err != nil {
				t.Fatal(err)
			}
			clone := func() classificationLocalAnchors {
				t.Helper()
				var anchors classificationLocalAnchors
				if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
					t.Fatal(err)
				}
				return anchors
			}
			for _, test := range []struct {
				name   string
				change func(*classificationLocalAnchors)
			}{
				{"route", func(a *classificationLocalAnchors) { a.Route = "other" }},
				{"case", func(a *classificationLocalAnchors) { a.Case = "other" }},
				{"workload", func(a *classificationLocalAnchors) { a.Workload = "other" }},
				{"api", func(a *classificationLocalAnchors) { a.API = true }},
				{"operation", func(a *classificationLocalAnchors) { a.Operation = "native-query-batch" }},
				{"driver-mode", func(a *classificationLocalAnchors) { a.DriverMode = "batch" }},
				{"source", func(a *classificationLocalAnchors) { a.Source.UTF8 = "x"; a.Source.Bytes = 1 }},
				{"source-sha", func(a *classificationLocalAnchors) {
					a.Source.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
				}},
				{"root", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "wrong_root" }},
				{"abi", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
				{"expect", func(a *classificationLocalAnchors) { a.Expect[0].Syntax = "ERROR" }},
				{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
				{"query-source", func(a *classificationLocalAnchors) { a.Queries[0].Source += "\n" }},
				{"query-id", func(a *classificationLocalAnchors) { a.Queries[0].ID = "other" }},
				{"pattern-count", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Patterns++ }},
				{"capture-name", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames[0] = "other" }},
				{"capture-names-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil }},
				{"capture-names-empty", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = []string{} }},
				{"predicate-list-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
				{"predicate-added", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}} }},
				{"required-count", func(a *classificationLocalAnchors) { a.Required = a.Required[:len(a.Required)-1] }},
				{"required-role", func(a *classificationLocalAnchors) { a.Required[0].TreeRole = "fresh" }},
				{"structural-null", func(a *classificationLocalAnchors) { a.Required[0].Structural = nil }},
				{"expected-null", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
			} {
				t.Run(test.name, func(t *testing.T) {
					anchors := clone()
					test.change(&anchors)
					if err := classificationCJRAnchorsValid(name, anchors); err == nil {
						t.Fatal("mutated source, CST, ABI, query metadata, or tuple accepted")
					}
				})
			}
			registryData, err := os.ReadFile("../../testdata/native/queries/" + spec.queryFile)
			if err != nil {
				t.Fatal(err)
			}
			if err := classificationCJRQueryRegistryBytesValid(name, clone(), registryData); err != nil {
				t.Fatal("positive source registry rejected", err)
			}
			if err := classificationCJRQueryRegistryBytesValid(name, clone(), append(append([]byte(nil), registryData...), ' ')); err == nil {
				t.Fatal("query registry bytes outside the exact pin accepted")
			}
			if len(clone().Queries) == 0 || !reflect.DeepEqual(clone().Required[0].Expected, []kit.Capture{}) {
				t.Fatal("positive empty tuple fixture changed")
			}
		})
	}
}

func TestQueryClassificationCJRGrammarGuards(t *testing.T) {
	for _, name := range []string{"c4-empty", "java5-empty", "rust4-empty"} {
		t.Run(name, func(t *testing.T) {
			spec, _ := classificationCJRSpecFor(name)
			var registry struct {
				Routes []struct {
					Route string            `json:"route"`
					Files []kit.NativeInput `json:"files"`
				} `json:"routes"`
			}
			data, err := os.ReadFile("../../contracts/native-routes.json")
			if err != nil || json.Unmarshal(data, &registry) != nil {
				t.Fatal("native grammar registry", err)
			}
			var grammar []kit.NativeInput
			for _, route := range registry.Routes {
				if route.Route == spec.route {
					grammar = route.Files
				}
			}
			if !classificationCJRGrammarValid(name, grammar) {
				t.Fatal("actual route grammar closure rejected")
			}
			if classificationCJRGrammarValid(name, grammar[:len(grammar)-1]) {
				t.Fatal("incomplete grammar closure accepted")
			}
			changed := append([]kit.NativeInput(nil), grammar...)
			changed[len(changed)-1].Role = "scanner"
			if classificationCJRGrammarValid(name, changed) {
				t.Fatal("changed or added scanner/header role accepted")
			}
			changed = append([]kit.NativeInput(nil), grammar...)
			changed[0].SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
			if classificationCJRGrammarValid(name, changed) {
				t.Fatal("changed parser source pin accepted")
			}
			changed = append([]kit.NativeInput(nil), grammar...)
			changed[0].Bytes++
			if classificationCJRGrammarValid(name, changed) {
				t.Fatal("changed parser source byte count accepted")
			}
		})
	}
}

func TestQueryClassificationCJREmptyStderr(t *testing.T) {
	for _, name := range []string{"c4-empty", "java5-empty", "rust4-empty"} {
		t.Run(name, func(t *testing.T) {
			spec, _ := classificationCJRSpecFor(name)
			result := &runner.Result{}
			classificationCSharpEmptyStderr(result)
			capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: result}
			files, err := capture.files("capture/" + spec.caseID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range files {
				if file.pin.Role == "stderr" {
					found = file.pin.Bytes == 0 && file.pin.SHA256 == sha(nil) && file.data != nil && len(file.data) == 0
				}
			}
			if !found {
				t.Fatal("zero-byte stderr was omitted from the serialized C/Java/Rust capture")
			}
			missing := &runner.Result{StderrBytes: 1}
			classificationCSharpEmptyStderr(missing)
			if missing.Stderr != nil {
				t.Fatal("a missing nonempty stream was replaced with an empty member")
			}
		})
	}
}
