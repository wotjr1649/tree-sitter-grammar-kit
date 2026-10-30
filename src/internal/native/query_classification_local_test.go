package native

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func classificationNativeComplete(result CaseResult, capture *driverCapture) bool {
	return result.ExecutionStatus == kit.StatusCompleted && result.ResponseStatus == kit.StatusCompleted && result.Assessment == kit.AssessPass && result.Code == "" && len(result.Steps) == 1 && result.Process != nil && result.Process.Succeeded() && result.Process.Cleanup.ResidualAfterExit == 0 && capture != nil && capture.err == nil
}

func classificationBuildStopped(steps []BuildStep, driverAttempted bool, capture *driverCapture) bool {
	for _, step := range steps {
		if !step.Executed || !step.Result.Cleanup.Verified || step.Result.Cleanup.ResidualAfterExit != 0 {
			return false
		}
	}
	return !driverAttempted || capture != nil && capture.result != nil && capture.result.Cleanup.Verified && capture.result.Cleanup.ResidualAfterExit == 0
}

type classificationCaptureProfile struct {
	anchorFile, anchorSHA, route, symbol string
	queryCount, grammarCount             int
	paged                                bool
}

type classificationCJRQuerySpec struct {
	id, sha256 string
	patterns   uint32
	captures   []string
}

type classificationCJRGrammarFile struct {
	path, role, sha256 string
	bytes              uint64
}

type classificationCJRSpec struct {
	anchorFile, anchorSHA, route, symbol  string
	caseID, workload, rootType            string
	queryFile, querySHA                   string
	queryBytes, languageVersion           int
	runtimeLanguageVersion, minCompatible int
	symbolCount, fieldCount, grammarCount int
	querySpecs                            []classificationCJRQuerySpec
	grammarFiles                          []classificationCJRGrammarFile
}

func classificationCJRSpecFor(name string) (classificationCJRSpec, bool) {
	switch name {
	case "c4-empty":
		return classificationCJRSpec{
			anchorFile: "c4-classification-anchors.json", anchorSHA: "c4a8b34393c268a0ca479a9f82e99617d0946e795fa7e35bdf24ad2437e04f2d",
			route: "c", symbol: "tree_sitter_c", caseID: "c4-empty-definitions", workload: "c4-empty-definition-single-frame", rootType: "translation_unit",
			queryFile: "c.json", queryBytes: 6204, querySHA: "7089c8e7b84a224076b431782070fc675735420bc7971689d5530b08197533fd",
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 403, fieldCount: 40, grammarCount: 4,
			querySpecs: []classificationCJRQuerySpec{
				{id: "q.structure", sha256: "e9eba020e1b34816c5f12c21545fbea0b895e03996974593562cb23e1ff911a7", patterns: 3, captures: []string{"type", "function", "call"}},
				{id: "q.probe-c-V11", sha256: "d3930fba7799c448bbe5fdf9a75fb102d7b5ffd896228600d140dfea3202a0e3", patterns: 3, captures: []string{"align", "alignof", "assoc"}},
				{id: "q.probe-c-V23", sha256: "0d001ee87956853e839787e05f3da182b14d8a06777426fb984d7dfea2c0b757", patterns: 4, captures: []string{"attr", "true", "false", "null"}},
				{id: "q.probe-c-L01", sha256: "6e33506bbd6c2914ca580874e90c3ec8fe65a70f26e2b9a1beb8b66f58656f1f", patterns: 2, captures: []string{"param", "decl"}},
			},
			grammarFiles: []classificationCJRGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 5611460, sha256: "e18ba3335e75a2d029ced1b302b8ecb50bd2539c7b0c4394f9abbfdb1719cd5b"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	case "java5-empty":
		return classificationCJRSpec{
			anchorFile: "java5-classification-anchors.json", anchorSHA: "41b3011c129007a7ab582e666e3f857fca12cfd756271e5c02358fbff30e451b",
			route: "java", symbol: "tree_sitter_java", caseID: "java5-empty-definitions", workload: "java5-empty-definition-single-frame", rootType: "program",
			queryFile: "java.json", queryBytes: 5565, querySHA: "ca65cb697b4fc3d89de01c2874e3d01360d48f446d6e4e8d6ac0582643998c42",
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 323, fieldCount: 40, grammarCount: 4,
			querySpecs: []classificationCJRQuerySpec{
				{id: "q.structure", sha256: "6b31aaa0626f4d2ef8e0e542cfcd3e3102581f267e6ac3482c5265616b478d01", patterns: 4, captures: []string{"class", "interface", "record", "method"}},
				{id: "q.probe-v9", sha256: "956f820c26bf247d5877cc8450aa7c639496ee332b34217c85edc318c1916bf6", patterns: 2, captures: []string{"module", "requires"}},
				{id: "q.probe-v21", sha256: "e163eac14537175162a1865aeb315fd470d2ac2e215de2d491abfc908ee16763", patterns: 2, captures: []string{"record", "guard"}},
				{id: "q.probe-v25-compact", sha256: "bd65786741e63b7da62527a2241064c04583045523824f0d4adb963c4d533dff", patterns: 1, captures: []string{"method"}},
				{id: "q.probe-v25-ctor", sha256: "30c5ee77cc3d50b374cdaf7ad8faaa13f0154fa8dc169d1f7deec57a67250280", patterns: 1, captures: []string{"invocation"}},
			},
			grammarFiles: []classificationCJRGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 2600326, sha256: "7c427c9538452461d5d658a93eff64b5e375049b244614a12563ef6adac8e8e6"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	case "rust4-empty":
		return classificationCJRSpec{
			anchorFile: "rust4-classification-anchors.json", anchorSHA: "46cbfe87a0caf2d8ebc43aaca341cfde77e6eda5ae78b0f4909064c1e2cd8569",
			route: "rust", symbol: "tree_sitter_rust", caseID: "rust4-empty-definitions", workload: "rust4-empty-definition-single-frame", rootType: "source_file",
			queryFile: "rust.json", queryBytes: 5349, querySHA: "43fb6848561cb11d855c194d617b3deb976c73f715fbf180009e70366da31ab5",
			languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 360, fieldCount: 31, grammarCount: 5,
			querySpecs: []classificationCJRQuerySpec{
				{id: "q.structure", sha256: "e3bddbcb1621ba8a3c546d30532ff349d0073b2a2728972d291f1a3b5d9c56a2", patterns: 4, captures: []string{"struct", "enum", "trait", "function"}},
				{id: "q.probe-rust-B02", sha256: "138adb98d09e76d140e5895c158c948c5eacfe53577f8cd297bb190d8ff8d2b9", patterns: 3, captures: []string{"meta", "param", "macro"}},
				{id: "q.probe-rust-V18", sha256: "39db79528b589ba56f1e88a5797302065732f5e87d5b5d72c095ba780e6407bb", patterns: 4, captures: []string{"constparam", "async", "const", "else"}},
				{id: "q.probe-rust-V24", sha256: "5297a5811821401bc11f7881d439c135076abc6760147b56223bc496701d16f2", patterns: 4, captures: []string{"captured", "frag", "letpat"}},
			},
			grammarFiles: []classificationCJRGrammarFile{
				{path: "src/parser.c", role: "parser", bytes: 7448615, sha256: "be00454d4938a698e56f0c00a42b7e40ed761a2fe3d5c01533380b1e5b21ea5b"},
				{path: "src/scanner.c", role: "scanner", bytes: 12532, sha256: "caecbcba33fa87f7a75d56a0c47aa2938744bae5cd358db7911ec0d97b141cae"},
				{path: "src/tree_sitter/alloc.h", role: "header", bytes: 985, sha256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
				{path: "src/tree_sitter/array.h", role: "header", bytes: 13453, sha256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
				{path: "src/tree_sitter/parser.h", role: "header", bytes: 7624, sha256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
			},
		}, true
	default:
		return classificationCJRSpec{}, false
	}
}

func classificationCJRProfile(name string) bool {
	_, ok := classificationCJRSpecFor(name)
	return ok
}

type classificationLocalAnchors struct {
	Case       string                `json:"case,omitempty"`
	API        bool                  `json:"api,omitempty"`
	Operation  string                `json:"operation,omitempty"`
	DriverMode string                `json:"driver_mode,omitempty"`
	Workload   string                `json:"workload,omitempty"`
	Expect     []kit.StepExpectation `json:"expect,omitempty"`
	Schema     string                `json:"schema"`
	Route      string                `json:"route"`
	Source     struct {
		UTF8   string `json:"utf8"`
		Bytes  uint64 `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"source"`
	Edits   []kit.Edit     `json:"edits"`
	Nodes   []kit.TreeNode `json:"nodes_each_version"`
	Queries []struct {
		ID       string                   `json:"id"`
		Source   string                   `json:"source"`
		SHA256   string                   `json:"sha256"`
		Metadata kit.QueryWitnessMetadata `json:"metadata"`
	} `json:"queries"`
	Required []kit.QueryWitnessTuple `json:"required"`
	ABI      struct {
		LanguageVersion        uint32 `json:"language_version"`
		RuntimeLanguageVersion uint32 `json:"runtime_language_version"`
		RuntimeMinCompatible   uint32 `json:"runtime_min_compatible"`
		SymbolCount            uint32 `json:"symbol_count"`
		FieldCount             uint32 `json:"field_count"`
	} `json:"expected_producer_abi"`
}

func classificationCaptureProfileFor(name string) (classificationCaptureProfile, error) {
	switch name {
	case "":
		return classificationCaptureProfile{"go3-classification-anchors.json", "096785af5cbd09ece1f649ab7ea41648f11b45c751386636238537787bec0f19", "go", "tree_sitter_go", 3, 5, false}, nil
	case "positive-batch":
		return classificationCaptureProfile{"go-positive-batch-anchors.json", "921d28f6e805bf443260961e7394cfb4bcd0526066425b60057b37d514169288", "go", "tree_sitter_go", 3, 5, false}, nil
	case "csharp15-empty":
		return classificationCaptureProfile{"csharp15-classification-anchors.json", "06933d6516106308ec9fc20762e8eebd9718b26b2453ceea2cec090f382c938b", "csharp", "tree_sitter_c_sharp", 15, 5, true}, nil
	default:
		if spec, ok := classificationCJRSpecFor(name); ok {
			return classificationCaptureProfile{spec.anchorFile, spec.anchorSHA, spec.route, spec.symbol, len(spec.querySpecs), spec.grammarCount, false}, nil
		}
		return classificationCaptureProfile{}, fmt.Errorf("unknown closed classification profile %q", name)
	}
}

func classificationCJRAnchorsValid(profileName string, a classificationLocalAnchors) error {
	spec, ok := classificationCJRSpecFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed C/Java/Rust classification profile %q", profileName)
	}
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != spec.route || a.Case != spec.caseID || a.API || a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != spec.workload ||
		a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != sha(nil) || a.Edits == nil || len(a.Edits) != 0 || len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" || len(a.Queries) != len(spec.querySpecs) || len(a.Required) != len(spec.querySpecs) {
		return fmt.Errorf("%s empty-source definition profile changed", profileName)
	}
	wantRoot := []kit.TreeNode{{Parent: -1, Type: spec.rootType, Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) || a.ABI.LanguageVersion != uint32(spec.languageVersion) || a.ABI.RuntimeLanguageVersion != uint32(spec.runtimeLanguageVersion) || a.ABI.RuntimeMinCompatible != uint32(spec.minCompatible) || a.ABI.SymbolCount != uint32(spec.symbolCount) || a.ABI.FieldCount != uint32(spec.fieldCount) {
		return fmt.Errorf("%s ABI or independent empty-source CST changed", profileName)
	}
	seen := make(map[string]bool, len(a.Queries))
	for i, q := range a.Queries {
		want := spec.querySpecs[i]
		if q.ID != want.id || seen[q.ID] || q.SHA256 != want.sha256 || q.SHA256 != sha([]byte(q.Source)) || q.Metadata.Patterns != want.patterns || q.Metadata.CaptureNames == nil || !reflect.DeepEqual(q.Metadata.CaptureNames, want.captures) || q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			return fmt.Errorf("%s query source or complete predicate-free metadata changed at %d", profileName, i)
		}
		seen[q.ID] = true
		tuple := a.Required[i]
		if tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s denominator or empty capture anchors changed at %d", profileName, i)
		}
	}
	return nil
}

func classificationCJRQueryRegistryValid(profileName string, a classificationLocalAnchors) error {
	spec, ok := classificationCJRSpecFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed C/Java/Rust classification profile %q", profileName)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "queries", spec.queryFile))
	if err != nil {
		return fmt.Errorf("read pinned %s query registry: %w", profileName, err)
	}
	return classificationCJRQueryRegistryBytesValid(profileName, a, data)
}

func classificationCJRQueryRegistryBytesValid(profileName string, a classificationLocalAnchors, data []byte) error {
	spec, ok := classificationCJRSpecFor(profileName)
	if !ok {
		return fmt.Errorf("unknown closed C/Java/Rust classification profile %q", profileName)
	}
	if len(data) != spec.queryBytes || sha(data) != spec.querySHA {
		return fmt.Errorf("%s query registry file pin changed", profileName)
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
		return fmt.Errorf("decode %s query registry: %w", profileName, err)
	}
	if registry.Schema != "tsgk-native-query-cases/r1" || registry.Route != spec.route || len(registry.Queries) != len(spec.querySpecs) || len(a.Queries) != len(spec.querySpecs) {
		return fmt.Errorf("%s query registry profile changed", profileName)
	}
	for i, query := range registry.Queries {
		if query.ID != spec.querySpecs[i].id || query.ID != a.Queries[i].ID || query.Source != a.Queries[i].Source {
			return fmt.Errorf("%s query registry source/order differs at %d", profileName, i)
		}
	}
	return nil
}

func classificationCJRGrammarValid(profileName string, grammar []kit.NativeInput) bool {
	spec, ok := classificationCJRSpecFor(profileName)
	if !ok || len(grammar) != len(spec.grammarFiles) || len(grammar) != spec.grammarCount {
		return false
	}
	for i, file := range grammar {
		want := spec.grammarFiles[i]
		if file.Path != want.path || file.Role != want.role || file.Bytes != want.bytes || file.SHA256 != want.sha256 {
			return false
		}
		if file.Role == "parser" && file.Bytes > 16<<20 {
			return false
		}
	}
	return true
}

func classificationCSharpAnchorsValid(a classificationLocalAnchors) error {
	if a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != "csharp" || a.Case != "csharp15-empty-definitions" || a.API || a.Operation != "native-query" || a.DriverMode != "single" || a.Workload != "csharp15-empty-definition-single-frame" ||
		a.Source.UTF8 != "" || a.Source.Bytes != 0 || a.Source.SHA256 != sha(nil) || a.Edits == nil || len(a.Edits) != 0 || len(a.Expect) != 1 || a.Expect[0].Step != 0 || a.Expect[0].Syntax != "NO_ERROR" || len(a.Queries) != 15 || len(a.Required) != 15 {
		return fmt.Errorf("C# 15 empty-source definition profile changed")
	}
	wantRoot := []kit.TreeNode{{Parent: -1, Type: "compilation_unit", Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) || a.ABI.LanguageVersion != 15 || a.ABI.RuntimeLanguageVersion != 15 || a.ABI.RuntimeMinCompatible != 13 || a.ABI.SymbolCount != 654 || a.ABI.FieldCount != 26 {
		return fmt.Errorf("C# 15 ABI or independent empty-source CST changed")
	}
	seen := make(map[string]bool, len(a.Queries))
	for i, q := range a.Queries {
		if q.ID == "" || seen[q.ID] || q.SHA256 != sha([]byte(q.Source)) || q.Metadata.Patterns == 0 || len(q.Metadata.CaptureNames) == 0 || q.Metadata.CaptureNames == nil || q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			return fmt.Errorf("C# definition source or complete predicate-free metadata changed at %d", i)
		}
		seen[q.ID] = true
		tuple := a.Required[i]
		if tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("C# definition denominator or empty capture anchors changed at %d", i)
		}
	}
	return nil
}

func classificationCSharpEmptyStderr(result *runner.Result) {
	if result != nil && result.StderrBytes == 0 && result.Stderr == nil {
		result.Stderr = []byte{}
	}
}

func TestQueryClassificationCSharpEmptyStderr(t *testing.T) {
	result := &runner.Result{}
	classificationCSharpEmptyStderr(result)
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: result}
	files, err := capture.files("capture/csharp-empty")
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
		t.Fatal("zero-byte stderr was omitted from the serialized capture")
	}
	classificationCSharpEmptyStderr(nil)
	missing := &runner.Result{StderrBytes: 1}
	classificationCSharpEmptyStderr(missing)
	if missing.Stderr != nil {
		t.Fatal("a missing nonempty stream was replaced with an empty member")
	}
}

func TestQueryClassificationCSharpProfile(t *testing.T) {
	profile, err := classificationCaptureProfileFor("csharp15-empty")
	if err != nil || profile.route != "csharp" || profile.symbol != "tree_sitter_c_sharp" || profile.anchorFile != "csharp15-classification-anchors.json" || profile.anchorSHA != "06933d6516106308ec9fc20762e8eebd9718b26b2453ceea2cec090f382c938b" || profile.queryCount != 15 || profile.grammarCount != 5 || !profile.paged {
		t.Fatalf("C# 15 capture profile differs: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch"} {
		other, err := classificationCaptureProfileFor(name)
		if err != nil || other.route != "go" || other.paged {
			t.Fatalf("Go profile unexpectedly selects parser paging: %q, %#v, %v", name, other, err)
		}
	}
	if _, err := classificationCaptureProfileFor("unknown"); err == nil {
		t.Fatal("unknown profile accepted")
	}
	data, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
	if err != nil || sha(data) != profile.anchorSHA {
		t.Fatal("pinned C# 15 anchors changed", err)
	}
	var anchors classificationLocalAnchors
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if err := classificationCSharpAnchorsValid(anchors); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*classificationLocalAnchors)
	}{
		{"nonempty-source", func(a *classificationLocalAnchors) { a.Source.UTF8 = "x"; a.Source.Bytes = 1 }},
		{"abi", func(a *classificationLocalAnchors) { a.ABI.FieldCount++ }},
		{"denominator", func(a *classificationLocalAnchors) { a.Required = a.Required[:14] }},
		{"metadata-captures-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.CaptureNames = nil }},
		{"metadata-null", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"metadata-predicate", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = []kit.QueryPredicate{{}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var changed classificationLocalAnchors
			if err := jsonv2.Unmarshal(data, &changed, jsonv2.RejectUnknownMembers(true)); err != nil {
				t.Fatal(err)
			}
			test.change(&changed)
			if err := classificationCSharpAnchorsValid(changed); err == nil {
				t.Fatal("changed C# source, ABI, denominator, or metadata accepted")
			}
		})
	}
}

func TestQueryClassificationBuildCleanup(t *testing.T) {
	known := BuildStep{Executed: true, Result: runner.Result{Cleanup: runner.Cleanup{Verified: true}}}
	if !classificationBuildStopped([]BuildStep{known}, false, nil) {
		t.Fatal("verified stopped build rejected")
	}
	for _, step := range []BuildStep{{Executed: false}, {Executed: true}, {Executed: true, Result: runner.Result{Cleanup: runner.Cleanup{Verified: true, ResidualAfterExit: 1}}}} {
		if classificationBuildStopped([]BuildStep{known, step}, false, nil) {
			t.Fatal("unknown start/attach or cleanup permitted build removal")
		}
	}
	for _, capture := range []*driverCapture{nil, {}, {stage: "VERIFY_REFUSED"}, {result: &runner.Result{}}, {result: &runner.Result{Cleanup: runner.Cleanup{Verified: true, ResidualAfterExit: 1}}}} {
		if classificationBuildStopped([]BuildStep{known}, true, capture) {
			t.Fatal("attempted driver with unknown start/cleanup permitted build removal")
		}
	}
	if !classificationBuildStopped([]BuildStep{known}, true, &driverCapture{result: &runner.Result{Cleanup: runner.Cleanup{Verified: true}}}) {
		t.Fatal("verified stopped driver rejected")
	}
}

func TestQueryClassificationNativeCompletion(t *testing.T) {
	result := CaseResult{ExecutionStatus: kit.StatusCompleted, ResponseStatus: kit.StatusCompleted, Assessment: kit.AssessPass, Steps: []StepResult{{}}, Process: &runner.Result{Status: runner.StatusCompleted, ExitCode: 0, Cleanup: runner.Cleanup{Verified: true}}}
	if !classificationNativeComplete(result, &driverCapture{}) {
		t.Fatal("normal completion rejected")
	}
	for _, change := range []func(*CaseResult){
		func(r *CaseResult) { r.Assessment = kit.AssessFail; r.Code = "REUSE_OBSERVATION_FAILED" },
		func(r *CaseResult) { r.Assessment = kit.AssessFail },
		func(r *CaseResult) { r.Code = "unexplained-code" },
		func(r *CaseResult) {
			r.Process = &runner.Result{Status: runner.StatusCompleted, ExitCode: 0, Cleanup: runner.Cleanup{Verified: true, ResidualAfterExit: 1}}
		},
	} {
		other := result
		change(&other)
		if classificationNativeComplete(other, &driverCapture{}) {
			t.Fatal("failed or unclean observation became a completed classification capture")
		}
	}
}

// This explicit local capture records structural metadata, never a semantic PASS.
func TestQueryClassificationGoLocalCapture(t *testing.T) {
	profileName := os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")
	if profileName == "csharp15-empty" {
		t.Skip("C# 15 uses the dedicated local capture entry")
	}
	if _, ok := classificationCJRSpecFor(profileName); ok {
		t.Skip("C/Java/Rust use the dedicated local capture entry")
	}
	classificationNativeLocalCapture(t)
}

func TestQueryClassificationCSharpLocalCapture(t *testing.T) {
	if os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE") != "csharp15-empty" {
		t.Skip("explicit C# 15 local capture profile is required")
	}
	classificationNativeLocalCapture(t)
}

func TestQueryClassificationCJRLocalCapture(t *testing.T) {
	if _, ok := classificationCJRSpecFor(os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")); !ok {
		t.Skip("explicit C/Java/Rust empty-definition profile is required")
	}
	classificationNativeLocalCapture(t)
}

func classificationNativeLocalCapture(t *testing.T) {
	root := os.Getenv("TSGK_QUERY_CLASSIFICATION_CAPTURE")
	if root == "" {
		t.Skip("explicit preregistered local capture is required")
	}
	if runtime.GOOS != "windows" || !filepath.IsAbs(root) {
		t.Fatal("this preregistration requires an absolute Windows capture root")
	}
	runID := os.Getenv("TSGK_QUERY_CLASSIFICATION_RUN")
	if runID == "" {
		t.Fatal("a separately preregistered observation run ID is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("capture root must be a new empty caller-owned directory", err)
	}
	profileName := os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")
	profile, err := classificationCaptureProfileFor(profileName)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../testdata/query-semantics/" + profile.anchorFile)
	if err != nil || sha(data) != profile.anchorSHA {
		t.Fatal("reviewed query-classification anchors changed", err)
	}
	var anchors classificationLocalAnchors
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if anchors.Schema != "tsgk-query-classification-anchors/r1" || anchors.Route != profile.route || anchors.Source.Bytes != uint64(len(anchors.Source.UTF8)) || anchors.Source.SHA256 != sha([]byte(anchors.Source.UTF8)) || len(anchors.Edits) != 0 || len(anchors.Queries) != profile.queryCount || len(anchors.Required) != profile.queryCount {
		t.Fatal("independent definition denominator or source changed")
	}
	switch profileName {
	case "":
		if anchors.Source.Bytes != 0 || len(anchors.Nodes) != 1 || anchors.Case != "" || anchors.API || anchors.Operation != "" || anchors.DriverMode != "" || anchors.Workload != "" || len(anchors.Expect) != 0 {
			t.Fatal("legacy Go3 profile changed")
		}
		anchors.Case, anchors.Operation, anchors.DriverMode, anchors.Workload = "go3", "native-query", "single", "go3-definition-classification"
		anchors.Expect = []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}
	case "positive-batch":
		if anchors.Case != "probe-go-B04-P-6" || !anchors.API || anchors.Operation != "native-query-batch" || anchors.DriverMode != "batch" || anchors.Workload != "go-positive-single-frame-batch" || anchors.Source.Bytes != 50 || len(anchors.Nodes) != 25 || len(anchors.Expect) != 1 {
			t.Fatal("positive batch profile changed")
		}
	case "csharp15-empty":
		if err := classificationCSharpAnchorsValid(anchors); err != nil {
			t.Fatal(err)
		}
	case "c4-empty", "java5-empty", "rust4-empty":
		if err := classificationCJRAnchorsValid(profileName, anchors); err != nil {
			t.Fatal(err)
		}
		if err := classificationCJRQueryRegistryValid(profileName, anchors); err != nil {
			t.Fatal(err)
		}
	}
	prepared := os.Getenv("TSGK_BENCH_PREPARED")
	if !filepath.IsAbs(prepared) {
		t.Fatal("an existing pinned prepared closure is required")
	}
	rt, cc := nativeTools(t)
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
	var grammar []kit.NativeInput
	routeMatches := 0
	for _, row := range registry.Routes {
		if row.Route == profile.route {
			routeMatches++
			if row.Symbol != profile.symbol {
				t.Fatal("registered grammar symbol differs from the selected route")
			}
			grammar = row.Files
		}
	}
	if routeMatches != 1 || len(grammar) != profile.grammarCount {
		t.Fatal("registered grammar closure missing or duplicated", profile.route, len(grammar))
	}
	if _, ok := classificationCJRSpecFor(profileName); ok && !classificationCJRGrammarValid(profileName, grammar) {
		t.Fatal("C/Java/Rust grammar roles or paths differ from the closed unpaged profile", profile.route)
	}
	write := func(path string, value []byte) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		n, writeErr := f.Write(value)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil || n != len(value) {
			t.Fatal("capture write", path, writeErr, closeErr)
		}
	}
	write("anchors.json", data)
	write("input/source.bin", []byte(anchors.Source.UTF8))
	for _, q := range anchors.Queries {
		if sha([]byte(q.Source)) != q.SHA256 || q.Metadata.Predicates == nil || len(q.Metadata.Predicates) != 0 {
			t.Fatal("independent query identity", q.ID)
		}
		write("input/"+q.ID+".scm", []byte(q.Source))
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	sum, size := digestOf(t, cc)
	request := testBuildRequest(work, rt, filepath.Join(prepared, "routes", profile.route), grammar, cc, kit.ToolIdentity{Name: "cc", Version: "host", SHA256: sum, Bytes: size})
	request.Symbol = profile.symbol
	b, err := NewBuild(ctx, request)
	removed := false
	driverAttempted := false
	var capture *driverCapture
	removeBuild := func() error {
		if b == nil || removed {
			return nil
		}
		rel, e := filepath.Rel(work, b.Dir)
		if e != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unexpected temporary build path: %s", b.Dir)
		}
		if !classificationBuildStopped(b.Steps, driverAttempted, capture) {
			return fmt.Errorf("process start/cleanup unverified; retained partial build: %s", b.Dir)
		}
		if err := b.Remove(); err != nil {
			return err
		}
		removed = true
		return nil
	}
	if b != nil {
		t.Cleanup(func() {
			if err := removeBuild(); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		if b != nil {
			data, marshalErr := jsonv2.Marshal(b)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			write("build-failure.json", data)
			for i, step := range b.Steps {
				write(fmt.Sprintf("build-failure-%d.stdout", i), step.Result.Stdout)
				write(fmt.Sprintf("build-failure-%d.stderr", i), step.Result.Stderr)
			}
		}
		t.Fatal(err)
	}
	var files []captureFile
	if profile.paged {
		files, err = semanticBuildFiles(ctx, b, true)
	} else {
		files, err = semanticBuildFiles(ctx, b)
	}
	if err != nil {
		t.Fatal(err)
	}
	pins := make([]kit.ReplayMember, 0, len(files)+6)
	for _, file := range files {
		write(file.pin.Path, file.data)
		pins = append(pins, file.pin)
	}
	op := kit.NativeOperations()[anchors.Operation]
	x := Context{Route: profile.route, Op: op, Output: kit.OutputTree, PolicyRef: PolicyRef(op, kit.OutputTree), Protocol: ProtocolR4, API: anchors.API, ObservationRunID: runID, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
	for _, q := range anchors.Queries {
		x.Queries = append(x.Queries, QuerySource{ID: q.ID, Source: []byte(q.Source)})
	}
	c := kit.IncrementalCase{ID: anchors.Case, Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "source.bin", Role: "case", Bytes: anchors.Source.Bytes, SHA256: anchors.Source.SHA256}, Edits: []kit.Edit{}, Points: []kit.NativePoint{}, Expect: anchors.Expect}
	capture = &driverCapture{}
	driverAttempted = true
	var result CaseResult
	if anchors.DriverMode == "single" {
		result = b.runCase(ctx, x, c, []byte(anchors.Source.UTF8), capture)
	} else {
		// This is one noninteractive frame plus EOF, not OracleRunBatch.
		x.ObservationProducerSHA256 = b.Identity
		result = newCaseResult(c)
		cr, done := b.prepareCase(ctx, x, c, []byte(anchors.Source.UTF8), &result)
		if done {
			t.Fatal("positive probe request refused", result.Code)
		}
		res, runErr := b.exec(ctx, "batch", Frame(cr.payload), cr.wall, x.Op, x.CgroupParent, capture)
		result.Process = &res
		frame, frameErr := SplitFrame(res.Stdout)
		result.Raw = frame
		if runErr == nil && frameErr == nil && res.Succeeded() && res.Cleanup.ResidualAfterExit == 0 {
			b.judgeFrame(x, c, cr, frame, nil, &result)
			x.observeCase(&result)
		} else {
			finishEarly(&result, kit.StatusFailed, kit.AssessNotAssessed, "BATCH_PROBE_INCOMPLETE")
		}
	}
	if profileName == "csharp15-empty" || classificationCJRProfile(profileName) {
		// The driver capture owns a clone, so preserve the explicit empty stream in both records.
		classificationCSharpEmptyStderr(result.Process)
		if capture != nil {
			classificationCSharpEmptyStderr(capture.result)
		}
	}
	captured, err := capture.files("capture/" + anchors.Case)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range captured {
		write(file.pin.Path, file.data)
		pins = append(pins, file.pin)
	}
	write("raw/"+anchors.Case+".json", result.Raw)
	if !classificationNativeComplete(result, capture) {
		t.Fatalf("native invocation incomplete %s/%s/%s", result.ExecutionStatus, result.Assessment, result.Code)
	}
	p := result.Producer
	if p == nil || p.SymbolCount == nil || p.FieldCount == nil || p.LanguageVersion != anchors.ABI.LanguageVersion || p.RuntimeLanguageVersion != anchors.ABI.RuntimeLanguageVersion || p.RuntimeMinCompatible != anchors.ABI.RuntimeMinCompatible || *p.SymbolCount != anchors.ABI.SymbolCount || *p.FieldCount != anchors.ABI.FieldCount {
		t.Fatal("independent producer ABI mismatch")
	}
	tree := result.Steps[0].Incremental
	if tree == nil || tree.Tree == nil || tree.Form != "full" || tree.Status != kit.StatusCompleted || kit.CompareTrees(tree.Tree.Nodes, anchors.Nodes) != nil || len(tree.Queries) != profile.queryCount {
		t.Fatal("independent full CST mismatch")
	}
	for i, q := range tree.Queries {
		expected, tuple := anchors.Queries[i], anchors.Required[i]
		if q.ID != expected.ID || tuple.Query != q.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || q.Status != kit.StatusCompleted || q.Evaluation != EvalStructural || q.Partial || q.Captures == nil || q.Code != "" || q.Patterns != expected.Metadata.Patterns || !reflect.DeepEqual(q.CaptureNames, expected.Metadata.CaptureNames) || !reflect.DeepEqual(q.Predicates, expected.Metadata.Predicates) || kit.CompareCaptures(q.structural, tuple.Structural) != nil || kit.CompareCaptures(q.Captures, tuple.Expected) != nil {
			t.Fatalf("independent structural metadata mismatch %s", expected.ID)
		}
	}
	stdout, err := SplitFrame(capture.result.Stdout)
	if err != nil || !bytes.Equal(stdout, result.Raw) {
		t.Fatal("native raw/frame identity", err)
	}
	if err := removeBuild(); err != nil {
		t.Fatal(err)
	}
	report, err := jsonv2.Marshal(map[string]any{"schema": "tsgk-query-classification-native-capture/r1", "route": profile.route, "anchors_sha256": sha(data), "grammar": grammar, "compiler": b.Compiler, "build_identity": b.Identity, "executable_path": b.Executable, "files": pins, "raw_sha256": sha(result.Raw), "native_independent_cst": "PASS", "structural_definitions": len(anchors.Queries), "semantic_pass": false})
	if err != nil {
		t.Fatal(err)
	}
	write("native-complete.json", report)
	t.Logf("independent %s CST/ABI/metadata%d checked; no external eligibility or semantic approval", profile.route, len(anchors.Queries))
}
