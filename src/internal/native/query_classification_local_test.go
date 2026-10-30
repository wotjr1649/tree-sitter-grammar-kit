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
		return classificationCaptureProfile{}, fmt.Errorf("unknown closed classification profile %q", name)
	}
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
	if os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE") == "csharp15-empty" {
		t.Skip("C# 15 uses the dedicated local capture entry")
	}
	classificationNativeLocalCapture(t)
}

func TestQueryClassificationCSharpLocalCapture(t *testing.T) {
	if os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE") != "csharp15-empty" {
		t.Skip("explicit C# 15 local capture profile is required")
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
	if profileName == "csharp15-empty" {
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
