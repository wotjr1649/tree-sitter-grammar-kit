package native

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type classificationCPPQuerySpec struct {
	id, sha256 string
	patterns   uint32
	captures   []string
}

type classificationCPPGrammarFile struct {
	path, role, sha256 string
	bytes              uint64
}

type classificationCPPProfileSpec struct {
	anchorFile, anchorSHA, route, symbol string
	caseID, workload, rootType           string
	queryFile, querySHA                  string
	queryBytes, grammarCount             int
	languageVersion, runtimeVersion      int
	minCompatible, symbolCount           int
	fieldCount                           int
	querySpecs                           []classificationCPPQuerySpec
	grammarFiles                         []classificationCPPGrammarFile
}

func classificationCPPProfileSpecFor(name string) (classificationCPPProfileSpec, bool) {
	if name != "cpp4-empty" {
		return classificationCPPProfileSpec{}, false
	}
	return classificationCPPProfileSpec{
		anchorFile:      "cpp4-classification-anchors.json",
		anchorSHA:       "e807345cb9a528facbaddaae3b914027acc4c99c8148f65f365e83f4b58ee5b4",
		route:           "cpp",
		symbol:          "tree_sitter_cpp",
		caseID:          "cpp4-empty-definitions",
		workload:        "cpp4-empty-definition-single-frame",
		rootType:        "translation_unit",
		queryFile:       "cpp.json",
		queryBytes:      4734,
		querySHA:        "fc90268c86c9133e2801b2fb7a20bda479137aacc0445911fa5046f11fd65766",
		languageVersion: 15, runtimeVersion: 15, minCompatible: 13,
		symbolCount: 621, fieldCount: 53, grammarCount: 5,
		querySpecs: []classificationCPPQuerySpec{
			{id: "q.structure", sha256: "2a57e6e5cb3c6136878e5d27ced7c7a090d491ec2e9e5e3d51f6305c6227945e", patterns: 4, captures: []string{"namespace", "struct", "concept", "class"}},
			{id: "q.probe-cpp-V14", sha256: "1077f4c6e42da4eb404eb4a819b5338f3c2aeeabbfeb1605678e3a4dd3d28f31", patterns: 3, captures: []string{"init", "vartemplate", "num"}},
			{id: "q.probe-cpp-V17", sha256: "29e660f47b5bb900c4321ea914fe6ea74f94d4662912773075e3aac7bf744c49", patterns: 2, captures: []string{"ns", "binding"}},
			{id: "q.probe-cpp-V23", sha256: "fde3be27ac5b9a89fa6e366ed03a24f035e1f21d0ea4434a80e1d666aef3a467", patterns: 2, captures: []string{"self", "index"}},
		},
		grammarFiles: []classificationCPPGrammarFile{
			{path: "src/parser.c", role: "parser", bytes: 49598509, sha256: "3856d51ead4b6ecf5d4e1ec3fd39c52c9ba0cb535e69ea1785822280c45b75fc"},
			{path: "src/scanner.c", role: "scanner", bytes: 6772, sha256: "55b66d60ce5551e354112bc0c050237e786402764d59893d0296ac5f88176546"},
			{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
			{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
			{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
		},
	}, true
}

func classificationCPPAnchorsValid(a classificationLocalAnchors) error {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != spec.route || a.Case != spec.caseID || a.API ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != spec.workload ||
		a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != sha(nil) ||
		a.Edits == nil || len(a.Edits) != 0 || len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" ||
		len(a.Queries) != len(spec.querySpecs) || len(a.Required) != len(spec.querySpecs) {
		return fmt.Errorf("CPP4 empty-source profile changed")
	}
	wantRoot := []kit.TreeNode{{Parent: -1, Type: spec.rootType, Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) ||
		a.ABI.LanguageVersion != uint32(spec.languageVersion) ||
		a.ABI.RuntimeLanguageVersion != uint32(spec.runtimeVersion) ||
		a.ABI.RuntimeMinCompatible != uint32(spec.minCompatible) ||
		a.ABI.SymbolCount != uint32(spec.symbolCount) ||
		a.ABI.FieldCount != uint32(spec.fieldCount) {
		return fmt.Errorf("CPP4 ABI or independent empty-source CST changed")
	}
	seen := make(map[string]bool, len(a.Queries))
	for i, q := range a.Queries {
		want := spec.querySpecs[i]
		if q.ID != want.id || seen[q.ID] || q.SHA256 != want.sha256 || q.SHA256 != sha([]byte(q.Source)) ||
			q.Metadata.Patterns != want.patterns || q.Metadata.CaptureNames == nil ||
			!reflect.DeepEqual(q.Metadata.CaptureNames, want.captures) ||
			q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			return fmt.Errorf("CPP4 query identity or predicate-free metadata changed at %d", i)
		}
		seen[q.ID] = true
		tuple := a.Required[i]
		if tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" ||
			tuple.Structural == nil || len(tuple.Structural) != 0 ||
			tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("CPP4 empty capture tuple changed at %d", i)
		}
	}
	return nil
}

func classificationCPPQueryRegistryValid(a classificationLocalAnchors) error {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	data, err := os.ReadFile("../../testdata/native/queries/" + spec.queryFile)
	if err != nil {
		return err
	}
	return classificationCPPQueryRegistryBytesValid(a, data)
}

func classificationCPPQueryRegistryBytesValid(a classificationLocalAnchors, data []byte) error {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	if len(data) != spec.queryBytes || sha(data) != spec.querySHA {
		return fmt.Errorf("CPP query registry file pin changed")
	}
	var registry struct {
		Schema  string `json:"schema"`
		Route   string `json:"route"`
		Queries []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return fmt.Errorf("decode CPP query registry: %w", err)
	}
	if registry.Schema != "tsgk-native-query-cases/r1" || registry.Route != spec.route ||
		len(registry.Queries) != len(spec.querySpecs) || len(a.Queries) != len(spec.querySpecs) {
		return fmt.Errorf("CPP query registry profile changed")
	}
	for i, query := range registry.Queries {
		if query.ID != spec.querySpecs[i].id || query.ID != a.Queries[i].ID ||
			query.Source != a.Queries[i].Source || sha([]byte(query.Source)) != spec.querySpecs[i].sha256 {
			return fmt.Errorf("CPP query registry source/order differs at %d", i)
		}
	}
	return nil
}

func classificationCPPGrammarValid(files []kit.NativeInput) bool {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	if len(files) != len(spec.grammarFiles) || len(files) != spec.grammarCount {
		return false
	}
	for i, file := range files {
		want := spec.grammarFiles[i]
		if file.Path != want.path || file.Role != want.role || file.Bytes != want.bytes || file.SHA256 != want.sha256 {
			return false
		}
	}
	return true
}

func classificationCPPRouteFiles(t *testing.T, route string) (string, []kit.NativeInput) {
	t.Helper()
	data, err := os.ReadFile("../../contracts/native-routes.json")
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
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	var symbol string
	var files []kit.NativeInput
	matches := 0
	for _, row := range registry.Routes {
		if row.Route == route {
			matches++
			symbol, files = row.Symbol, row.Files
		}
	}
	if matches != 1 {
		t.Fatalf("native route %q missing or duplicated: %d", route, matches)
	}
	return symbol, files
}

func classificationCPPReadAnchors(t *testing.T) ([]byte, classificationLocalAnchors) {
	t.Helper()
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	data, err := os.ReadFile("../../testdata/query-semantics/" + spec.anchorFile)
	if err != nil {
		t.Fatal(err)
	}
	if sha(data) != spec.anchorSHA {
		t.Fatal("pinned CPP4 anchors changed")
	}
	var anchors classificationLocalAnchors
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return data, anchors
}

func TestQueryClassificationCPPStaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("cpp4-empty")
	if !ok {
		t.Fatal("closed CPP4 profile missing")
	}
	profile, err := classificationCaptureProfileFor("cpp4-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA ||
		profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != len(spec.querySpecs) || profile.grammarCount != len(spec.grammarFiles) || !profile.paged {
		t.Fatalf("CPP4 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("CPP paging changed another profile: %q, %#v, %v", name, other, err)
		}
	}
	if _, err := classificationCaptureProfileFor("unknown-cpp"); err == nil {
		t.Fatal("unknown CPP profile accepted")
	}
	_, anchors := classificationCPPReadAnchors(t)
	if err := classificationCPPAnchorsValid(anchors); err != nil {
		t.Fatal("pinned CPP4 anchor rejected", err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/" + spec.queryFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationCPPQueryRegistryBytesValid(anchors, queryData); err != nil {
		t.Fatal("pinned CPP query registry rejected", err)
	}
	if err := classificationCPPQueryRegistryValid(anchors); err != nil {
		t.Fatal("on-disk CPP query registry rejected", err)
	}
	if _, ok := classificationCJRSpecFor("cpp4-empty"); ok || classificationCJRProfile("cpp4-empty") {
		t.Fatal("CPP profile entered the closed C/Java/Rust set")
	}
}

func TestQueryClassificationCPPAnchorGuards(t *testing.T) {
	data, _ := classificationCPPReadAnchors(t)
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
		{"route-context", func(a *classificationLocalAnchors) { a.Route = "other" }},
		{"case-workload-mode", func(a *classificationLocalAnchors) { a.Case = "other"; a.Workload = "other"; a.DriverMode = "batch" }},
		{"api-operation", func(a *classificationLocalAnchors) { a.API = true; a.Operation = "native-query-batch" }},
		{"source", func(a *classificationLocalAnchors) {
			a.Source.UTF8 = "x"
			a.Source.Bytes = 1
			a.Source.SHA256 = sha([]byte("x"))
		}},
		{"root", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "other_root" }},
		{"abi", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) {
			a.Queries[0].Source += "\n"
			a.Queries[0].SHA256 = sha([]byte(a.Queries[0].Source))
		}},
		{"metadata-patterns", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Patterns++ }},
		{"metadata-captures-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil }},
		{"metadata-captures-empty", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = []string{} }},
		{"metadata-predicates-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"metadata-predicate", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}} }},
		{"required-count", func(a *classificationLocalAnchors) { a.Required = a.Required[:3] }},
		{"required-null", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
		{"required-role", func(a *classificationLocalAnchors) { a.Required[0].TreeRole = "fresh" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			anchors := clone()
			test.change(&anchors)
			if err := classificationCPPAnchorsValid(anchors); err == nil {
				t.Fatal("mutated CPP4 context, source, CST, ABI, query metadata, or tuple accepted")
			}
		})
	}
}

func TestQueryClassificationCPPQueryRegistryGuards(t *testing.T) {
	_, anchors := classificationCPPReadAnchors(t)
	data, err := os.ReadFile("../../testdata/native/queries/cpp.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationCPPQueryRegistryBytesValid(anchors, append(append([]byte(nil), data...), ' ')); err == nil {
		t.Fatal("query registry bytes outside the exact pin accepted")
	}
	changed := anchors
	changed.Queries[0].Source += "\n"
	if err := classificationCPPQueryRegistryBytesValid(changed, data); err == nil {
		t.Fatal("anchor query source detached from the pinned registry accepted")
	}
}

func TestQueryClassificationCPPGrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationCPPGrammarValid(files) {
		t.Fatal("pinned CPP route grammar closure rejected")
	}
	for _, changed := range [][]kit.NativeInput{
		files[:len(files)-1],
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[1].Role = "header"
			return copy
		}(),
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[0].SHA256 = strings.Repeat("0", 64)
			return copy
		}(),
		func() []kit.NativeInput {
			copy := append([]kit.NativeInput(nil), files...)
			copy[0].Bytes--
			return copy
		}(),
	} {
		if classificationCPPGrammarValid(changed) {
			t.Fatal("incomplete or modified CPP grammar closure accepted")
		}
	}
	if classificationCJRGrammarValid("cpp4-empty", files) {
		t.Fatal("CPP parser escaped the CJR closed-profile rejection")
	}
	c4Symbol, c4Files := classificationCPPRouteFiles(t, "c")
	if c4Symbol != "tree_sitter_c" || !classificationCJRGrammarValid("c4-empty", c4Files) {
		t.Fatal("C route symbol changed")
	}
	c4WithLargeParser := append([]kit.NativeInput(nil), c4Files...)
	c4WithLargeParser[0] = files[0]
	if classificationCJRGrammarValid("c4-empty", c4WithLargeParser) {
		t.Fatal("C4 profile accepted a CPP parser substitution")
	}
	_, anchors := classificationCPPReadAnchors(t)
	if err := classificationCJRAnchorsValid("cpp4-empty", anchors); err == nil {
		t.Fatal("CPP anchors entered the CJR validator")
	}
	if err := classificationCJRQueryRegistryBytesValid("cpp4-empty", anchors, nil); err == nil {
		t.Fatal("CPP query registry entered the CJR validator")
	}
}

func TestQueryClassificationCPPEmptyStderr(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("cpp4-empty")
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
		t.Fatal("zero-byte CPP stderr was omitted from the serialized capture")
	}
	missing := &runner.Result{StderrBytes: 1}
	classificationCSharpEmptyStderr(missing)
	if missing.Stderr != nil {
		t.Fatal("missing nonempty CPP stderr was replaced with an empty member")
	}
}
