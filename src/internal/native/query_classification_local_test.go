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
	name, expectedSHA := "go3-classification-anchors.json", "096785af5cbd09ece1f649ab7ea41648f11b45c751386636238537787bec0f19"
	profile := os.Getenv("TSGK_QUERY_CLASSIFICATION_PROFILE")
	if profile == "positive-batch" {
		name, expectedSHA = "go-positive-batch-anchors.json", "921d28f6e805bf443260961e7394cfb4bcd0526066425b60057b37d514169288"
	} else if profile != "" {
		t.Fatal("unknown closed classification profile", profile)
	}
	data, err := os.ReadFile("../../testdata/query-semantics/" + name)
	if err != nil || sha(data) != expectedSHA {
		t.Fatal("reviewed Go anchors changed", err)
	}
	var anchors struct {
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
	if err := jsonv2.Unmarshal(data, &anchors, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if anchors.Schema != "tsgk-query-classification-anchors/r1" || anchors.Route != "go" || anchors.Source.Bytes != uint64(len(anchors.Source.UTF8)) || anchors.Source.SHA256 != sha([]byte(anchors.Source.UTF8)) || len(anchors.Edits) != 0 || len(anchors.Queries) != 3 || len(anchors.Required) != 3 {
		t.Fatal("independent definition denominator or source changed")
	}
	if profile == "" {
		if anchors.Source.Bytes != 0 || len(anchors.Nodes) != 1 || anchors.Case != "" || anchors.API || anchors.Operation != "" || anchors.DriverMode != "" || anchors.Workload != "" || len(anchors.Expect) != 0 {
			t.Fatal("legacy Go3 profile changed")
		}
		anchors.Case, anchors.Operation, anchors.DriverMode, anchors.Workload = "go3", "native-query", "single", "go3-definition-classification"
		anchors.Expect = []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}
	} else if anchors.Case != "probe-go-B04-P-6" || !anchors.API || anchors.Operation != "native-query-batch" || anchors.DriverMode != "batch" || anchors.Workload != "go-positive-single-frame-batch" || anchors.Source.Bytes != 50 || len(anchors.Nodes) != 25 || len(anchors.Expect) != 1 {
		t.Fatal("positive batch profile changed")
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
	for _, row := range registry.Routes {
		if row.Route == "go" && row.Symbol == "tree_sitter_go" {
			grammar = row.Files
		}
	}
	if len(grammar) != 5 {
		t.Fatal("registered Go grammar closure missing")
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
	request := testBuildRequest(work, rt, filepath.Join(prepared, "routes", "go"), grammar, cc, kit.ToolIdentity{Name: "cc", Version: "host", SHA256: sum, Bytes: size})
	request.Symbol = "tree_sitter_go"
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
	files, err := semanticBuildFiles(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	pins := make([]kit.ReplayMember, 0, len(files)+6)
	for _, file := range files {
		write(file.pin.Path, file.data)
		pins = append(pins, file.pin)
	}
	op := kit.NativeOperations()[anchors.Operation]
	x := Context{Route: "go", Op: op, Output: kit.OutputTree, PolicyRef: PolicyRef(op, kit.OutputTree), Protocol: ProtocolR4, API: anchors.API, ObservationRunID: runID, CgroupParent: os.Getenv("TSGK_CGROUP_PARENT")}
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
	if tree == nil || tree.Tree == nil || tree.Form != "full" || tree.Status != kit.StatusCompleted || kit.CompareTrees(tree.Tree.Nodes, anchors.Nodes) != nil || len(tree.Queries) != 3 {
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
	report, err := jsonv2.Marshal(map[string]any{"schema": "tsgk-query-classification-native-capture/r1", "route": "go", "anchors_sha256": sha(data), "grammar": grammar, "compiler": b.Compiler, "build_identity": b.Identity, "executable_path": b.Executable, "files": pins, "raw_sha256": sha(result.Raw), "native_independent_cst": "PASS", "structural_definitions": 3, "semantic_pass": false})
	if err != nil {
		t.Fatal(err)
	}
	write("native-complete.json", report)
	t.Log("independent Go CST/ABI/metadata3 checked; no external eligibility or semantic approval")
}
