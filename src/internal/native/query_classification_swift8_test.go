package native

import (
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationSwift8StaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("swift8-empty")
	if !ok || spec.label != "Swift8" || spec.route != "swift" || spec.rootType != "source_file" || len(spec.querySpecs) != 8 || spec.grammarCount != 5 {
		t.Fatal("closed Swift8 paged profile missing or changed")
	}
	profile, err := classificationCaptureProfileFor("swift8-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA || profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != 8 || profile.grammarCount != 5 || !profile.paged {
		t.Fatalf("Swift8 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty", "cpp4-empty", "kotlin5-empty", "dart7-empty", "php5-empty", "javascript4-empty"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty" || name == "cpp4-empty" || name == "kotlin5-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("Swift8 paging changed another profile: %q, %#v, %v", name, other, err)
		}
	}
	for _, alias := range []string{"swift", "swift8", "swift8-empty ", "SWIFT8-empty", "swift5-empty", "kotlin8-empty"} {
		if _, ok := classificationCPPProfileSpecFor(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if _, err := classificationCaptureProfileFor(alias); err == nil {
			t.Fatalf("near-alias capture profile %q accepted", alias)
		}
	}
	if _, ok := classificationCJRSpecFor("swift8-empty"); ok || classificationCJRProfile("swift8-empty") || classificationEmptyProfileName("swift8-empty") {
		t.Fatal("Swift8 entered an unpaged closed family")
	}
	if _, ok := classificationSmall17ProfileFor("swift8-empty"); ok {
		t.Fatal("Swift8 entered the Small17 family")
	}
	if !classificationGoCaptureEntrySkipsProfile("swift8-empty") {
		t.Fatal("generic Go capture entry would take the Swift8 profile")
	}
	data, anchors := classificationPagedReadAnchors(t, "swift8-empty")
	if sha(data) != spec.anchorSHA {
		t.Fatal("Swift8 anchor pin differs")
	}
	if err := classificationPagedAnchorsValid("swift8-empty", anchors); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryValid("swift8-empty", anchors); err != nil {
		t.Fatal(err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/swift.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryBytesValid("swift8-empty", anchors, append(slices.Clone(queryData), ' ')); err == nil {
		t.Fatal("Swift8 query registry byte mutation accepted")
	}
	// The three paged profiles never accept each other's anchors.
	if classificationCPPAnchorsValid(anchors) == nil || classificationPagedAnchorsValid("kotlin5-empty", anchors) == nil || classificationPagedQueryRegistryValid("kotlin5-empty", anchors) == nil {
		t.Fatal("another paged validator accepted the Swift8 anchors")
	}
	_, cppAnchors := classificationCPPReadAnchors(t)
	_, kotlinAnchors := classificationPagedReadAnchors(t, "kotlin5-empty")
	for name, other := range map[string]classificationLocalAnchors{"CPP4": cppAnchors, "Kotlin5": kotlinAnchors} {
		if classificationPagedAnchorsValid("swift8-empty", other) == nil || classificationPagedQueryRegistryValid("swift8-empty", other) == nil {
			t.Fatalf("Swift8 validator accepted the %s anchors", name)
		}
	}
	if classificationCJRAnchorsValid("swift8-empty", anchors) == nil {
		t.Fatal("CJR validator accepted the Swift8 profile")
	}
}

func TestQueryClassificationSwift8AnchorGuards(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*classificationLocalAnchors)
	}{
		{"route", func(a *classificationLocalAnchors) { a.Route = "kotlin" }},
		{"case", func(a *classificationLocalAnchors) { a.Case = "kotlin5-empty-definitions" }},
		{"api", func(a *classificationLocalAnchors) { a.API = true }},
		{"source", func(a *classificationLocalAnchors) { a.Source.UTF8, a.Source.Bytes = "\n", 1 }},
		{"root-type", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "translation_unit" }},
		{"root-named", func(a *classificationLocalAnchors) { a.Nodes[0].Named = false }},
		{"abi-symbols", func(a *classificationLocalAnchors) { a.ABI.SymbolCount = 595 }},
		{"abi-fields", func(a *classificationLocalAnchors) { a.ABI.FieldCount = 47 }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) { a.Queries[2].Source += " " }},
		{"patterns", func(a *classificationLocalAnchors) { a.Queries[2].Metadata.Patterns = 4 }},
		{"captures", func(a *classificationLocalAnchors) {
			a.Queries[2].Metadata.CaptureNames = []string{"opaque", "raw"}
		}},
		{"predicates-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"query-count", func(a *classificationLocalAnchors) { a.Queries = a.Queries[:7] }},
		{"required", func(a *classificationLocalAnchors) { a.Required = a.Required[:7] }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, anchors := classificationPagedReadAnchors(t, "swift8-empty")
			mutate.edit(&anchors)
			if err := classificationPagedAnchorsValid("swift8-empty", anchors); err == nil {
				t.Fatal("Swift8 anchor mutation accepted")
			}
		})
	}
}

func TestQueryClassificationSwift8GrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("swift8-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationPagedGrammarValid("swift8-empty", files) {
		t.Fatal("Swift8 grammar closure differs from the frozen native-routes row")
	}
	if files[0].Bytes <= 16<<20 || files[0].Bytes > 100<<20 || (files[0].Bytes+(16<<20)-1)/(16<<20) != 2 {
		t.Fatal("Swift8 parser is not the expected two-part paged parser")
	}
	if classificationCPPGrammarValid(files) || classificationCJRGrammarValid("swift8-empty", files) || classificationPagedGrammarValid("kotlin5-empty", files) {
		t.Fatal("another closed grammar validator accepted the Swift8 closure")
	}
	for _, mutate := range []func([]kit.NativeInput) []kit.NativeInput{
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Bytes--; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].SHA256 = f[2].SHA256; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[2].Path = "src/alloc.h"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].Role = "parser"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { return f[:4] },
		func(f []kit.NativeInput) []kit.NativeInput { f[0], f[1] = f[1], f[0]; return f },
	} {
		if classificationPagedGrammarValid("swift8-empty", mutate(slices.Clone(files))) {
			t.Fatal("Swift8 grammar mutation accepted")
		}
	}
}

func TestQueryClassificationSwift8EmptyStderrCaptureMember(t *testing.T) {
	processResult, ownerResult := &runner.Result{}, &runner.Result{}
	result := &CaseResult{Process: processResult}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
	classificationNormalizeCaptureStderr("swift8-empty", result, capture)
	for _, process := range []*runner.Result{processResult, ownerResult} {
		if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
			t.Fatal("Swift8 zero-byte stderr was not made explicit in both process records")
		}
	}
	files, err := capture.files("capture/swift8-empty-definitions")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, file := range files {
		if file.pin.Role == "stderr" {
			rows++
			if file.pin.Bytes != 0 || file.pin.SHA256 != sha(nil) || file.data == nil || len(file.data) != 0 {
				t.Fatal("Swift8 empty stderr member differs from the process")
			}
		}
	}
	if rows != 1 {
		t.Fatal("Swift8 empty stderr member was not serialized exactly once")
	}
	stderr := []byte{0x41}
	processResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	ownerResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	classificationNormalizeCaptureStderr("swift8-empty", &CaseResult{Process: processResult}, &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult})
	if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) {
		t.Fatal("Swift8 nonempty stderr was rewritten")
	}
}
