package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// These expectations were fixed and independently checked before the capture implementation.
func TestDriverCaptureJSONSemantics24(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("new semantics validation is excluded on macOS")
	}
	rt, cc := nativeTools(t)
	prepared := os.Getenv("TSGK_BENCH_PREPARED")
	if prepared == "" {
		t.Skip("TSGK_BENCH_PREPARED is required for the pinned JSON grammar")
	}
	var expected struct {
		Sources []struct {
			UTF8   string `json:"utf8"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"sources"`
		Nodes   []kit.TreeNode `json:"nodes_each_version"`
		Queries []struct {
			ID       string                   `json:"id"`
			Source   string                   `json:"source"`
			SHA256   string                   `json:"sha256"`
			Metadata kit.QueryWitnessMetadata `json:"metadata"`
		} `json:"queries"`
		Required      []kit.QueryWitnessTuple `json:"required"`
		RequiredCount int                     `json:"required_count"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "query-semantics", "small-json-anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if expected.RequiredCount != 24 || len(expected.Required) != 24 || len(expected.Sources) != 2 || len(expected.Queries) != 8 {
		t.Fatal("independent denominator changed")
	}
	var registry struct {
		Routes []struct {
			Route     string                     `json:"route"`
			Generated map[string]kit.NativeInput `json:"prepare_generated"`
		} `json:"routes"`
	}
	data, err = os.ReadFile(filepath.Join("..", "..", "contracts", "reproduction-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	grammarRoot := filepath.Join(prepared, "routes", "json")
	var grammar []kit.NativeInput
	for _, route := range registry.Routes {
		if route.Route != "json" {
			continue
		}
		for _, name := range []string{"parser.c", "tree_sitter/parser.h", "tree_sitter/alloc.h", "tree_sitter/array.h"} {
			pin, ok := route.Generated[name]
			if !ok {
				t.Fatalf("registered grammar input absent: %s", name)
			}
			pin.Path, pin.Role = "src/"+name, "header"
			if name == "parser.c" {
				pin.Role = "parser"
			}
			grammar = append(grammar, pin)
		}
	}
	if len(grammar) != 4 {
		t.Fatal("JSON grammar registration missing")
	}
	sum, size := digestOf(t, cc)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	request := testBuildRequest(t.TempDir(), rt, grammarRoot, grammar, cc, kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: size})
	request.Symbol = "tree_sitter_json"
	b, err := NewBuild(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Remove(); err != nil {
			t.Error(err)
		}
	})
	x := oracleContext(false)
	x.Route, x.Protocol, x.ObservationRunID = "json", ProtocolR4, "semantic-capture-24"
	for _, query := range expected.Queries {
		if sha([]byte(query.Source)) != query.SHA256 {
			t.Fatal("registered query bytes changed")
		}
		x.Queries = append(x.Queries, QuerySource{ID: query.ID, Source: []byte(query.Source)})
	}
	source := []byte(expected.Sources[0].UTF8)
	c := kit.IncrementalCase{ID: "c1", Encoding: kit.EncodingUTF8, Input: kit.NativeInput{Path: "c1.json", Role: "case", Bytes: uint64(len(source)), SHA256: sha(source)},
		Edits:  []kit.Edit{{StartByte: 8, OldEndByte: 9, NewEndByte: 9, Old: []byte("1"), New: []byte("2")}},
		Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 1, Syntax: "NO_ERROR"}}}
	capture := &driverCapture{}
	result := b.runCase(ctx, x, c, source, capture)
	if result.ExecutionStatus != kit.StatusCompleted || len(result.Steps) != 2 || result.Process == nil || !result.Process.Succeeded() {
		t.Fatalf("native case %s/%s/%s %+v", result.ExecutionStatus, result.Assessment, result.Code, result.Process)
	}
	verified := 0
	counts := [3]int{}
	for _, tuple := range expected.Required {
		step := result.Steps[tuple.Step]
		tree := step.Incremental
		index := 0
		if tuple.Step == 1 {
			index = 1
		}
		if tuple.TreeRole == "fresh" {
			tree, index = step.Fresh, 2
		}
		if tree == nil || tree.Tree == nil || tree.Form != "full" || tree.Status != kit.StatusCompleted || kit.CompareTrees(tree.Tree.Nodes, expected.Nodes) != nil {
			t.Fatalf("independent CST %s/%d/%s", tuple.Query, tuple.Step, tuple.TreeRole)
		}
		identity := expected.Sources[tuple.Step]
		if step.SourceBytes != identity.Bytes || step.SourceSHA256 != identity.SHA256 {
			t.Fatal("version source identity changed")
		}
		found := false
		for i, query := range tree.Queries {
			if query.ID != tuple.Query {
				continue
			}
			metadata := expected.Queries[i].Metadata
			if query.Status != kit.StatusCompleted || query.Code != "" || query.Evaluation != EvalEvaluated || query.Partial || query.Captures == nil ||
				query.Patterns != metadata.Patterns || !reflect.DeepEqual(query.CaptureNames, metadata.CaptureNames) || !reflect.DeepEqual(query.Predicates, metadata.Predicates) ||
				kit.CompareCaptures(query.structural, tuple.Structural) != nil || kit.CompareCaptures(query.Captures, tuple.Expected) != nil {
				t.Fatalf("independent query expectation %s/%d/%s: %+v", tuple.Query, tuple.Step, tuple.TreeRole, query)
			}
			verified++
			counts[index] += len(query.Captures)
			found = true
		}
		if !found {
			t.Fatalf("required tuple omitted: %+v", tuple)
		}
	}
	if verified != 24 || counts != [3]int{8, 7, 7} {
		t.Fatalf("required24 verified%d kept%v", verified, counts)
	}
	stored, wire := captureFiles(t, capture)
	payload, err := SplitFrame(stored[wire.Stdout.Path])
	if err != nil || string(payload) != string(result.Raw) || wire.Spec.Stdin == nil || wire.Error != nil {
		t.Fatalf("actual framing/capture mismatch %v", err)
	}
	t.Logf("independent required=24 evaluated=%d kept=%v; native base=%s code=%s; external eligibility is not approved by this test", verified, counts, result.Assessment, result.Code)
}
