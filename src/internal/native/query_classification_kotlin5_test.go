package native

import (
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationKotlin5StaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("kotlin5-empty")
	if !ok || spec.label != "Kotlin5" || spec.route != "kotlin" || spec.rootType != "source_file" || len(spec.querySpecs) != 5 || spec.grammarCount != 5 {
		t.Fatal("closed Kotlin5 paged profile missing or changed")
	}
	profile, err := classificationCaptureProfileFor("kotlin5-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA || profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != 5 || profile.grammarCount != 5 || !profile.paged {
		t.Fatalf("Kotlin5 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty", "cpp4-empty", "swift8-empty", "dart7-empty", "php5-empty", "javascript4-empty"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty" || name == "cpp4-empty" || name == "swift8-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("Kotlin5 paging changed another profile: %q, %#v, %v", name, other, err)
		}
	}
	for _, alias := range []string{"kotlin", "kotlin5", "kotlin5-empty ", "KOTLIN5-empty", "swift8"} {
		if _, ok := classificationCPPProfileSpecFor(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if _, err := classificationCaptureProfileFor(alias); err == nil {
			t.Fatalf("near-alias capture profile %q accepted", alias)
		}
	}
	if _, ok := classificationCJRSpecFor("kotlin5-empty"); ok || classificationCJRProfile("kotlin5-empty") || classificationEmptyProfileName("kotlin5-empty") {
		t.Fatal("Kotlin5 entered an unpaged closed family")
	}
	if _, ok := classificationSmall17ProfileFor("kotlin5-empty"); ok {
		t.Fatal("Kotlin5 entered the Small17 family")
	}
	if !classificationGoCaptureEntrySkipsProfile("kotlin5-empty") {
		t.Fatal("generic Go capture entry would take the Kotlin5 profile")
	}
	data, anchors := classificationPagedReadAnchors(t, "kotlin5-empty")
	if sha(data) != spec.anchorSHA {
		t.Fatal("Kotlin5 anchor pin differs")
	}
	if err := classificationPagedAnchorsValid("kotlin5-empty", anchors); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryValid("kotlin5-empty", anchors); err != nil {
		t.Fatal(err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/kotlin.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryBytesValid("kotlin5-empty", anchors, append(slices.Clone(queryData), ' ')); err == nil {
		t.Fatal("Kotlin5 query registry byte mutation accepted")
	}
	// The two paged profiles never accept each other's anchors.
	if classificationCPPAnchorsValid(anchors) == nil {
		t.Fatal("CPP4 validator accepted the Kotlin5 anchors")
	}
	_, cppAnchors := classificationCPPReadAnchors(t)
	if classificationPagedAnchorsValid("kotlin5-empty", cppAnchors) == nil || classificationPagedQueryRegistryValid("kotlin5-empty", cppAnchors) == nil {
		t.Fatal("Kotlin5 validator accepted the CPP4 anchors")
	}
	if classificationCJRAnchorsValid("kotlin5-empty", anchors) == nil {
		t.Fatal("CJR validator accepted the Kotlin5 profile")
	}
}

func TestQueryClassificationKotlin5AnchorGuards(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*classificationLocalAnchors)
	}{
		{"route", func(a *classificationLocalAnchors) { a.Route = "cpp" }},
		{"case", func(a *classificationLocalAnchors) { a.Case = "cpp4-empty-definitions" }},
		{"api", func(a *classificationLocalAnchors) { a.API = true }},
		{"source", func(a *classificationLocalAnchors) { a.Source.UTF8, a.Source.Bytes = "\n", 1 }},
		{"root-type", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "translation_unit" }},
		{"root-named", func(a *classificationLocalAnchors) { a.Nodes[0].Named = false }},
		{"abi-symbols", func(a *classificationLocalAnchors) { a.ABI.SymbolCount = 303 }},
		{"abi-fields", func(a *classificationLocalAnchors) { a.ABI.FieldCount = 8 }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) { a.Queries[2].Source += " " }},
		{"patterns", func(a *classificationLocalAnchors) { a.Queries[2].Metadata.Patterns = 3 }},
		{"captures", func(a *classificationLocalAnchors) {
			a.Queries[4].Metadata.CaptureNames = []string{"class", "function"}
		}},
		{"predicates-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"required", func(a *classificationLocalAnchors) { a.Required = a.Required[:4] }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, anchors := classificationPagedReadAnchors(t, "kotlin5-empty")
			mutate.edit(&anchors)
			if err := classificationPagedAnchorsValid("kotlin5-empty", anchors); err == nil {
				t.Fatal("Kotlin5 anchor mutation accepted")
			}
		})
	}
}

func TestQueryClassificationKotlin5GrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("kotlin5-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationPagedGrammarValid("kotlin5-empty", files) {
		t.Fatal("Kotlin5 grammar closure differs from the frozen native-routes row")
	}
	if files[0].Bytes <= 16<<20 || files[0].Bytes > 100<<20 || (files[0].Bytes+(16<<20)-1)/(16<<20) != 3 {
		t.Fatal("Kotlin5 parser is not the expected three-part paged parser")
	}
	if classificationCPPGrammarValid(files) || classificationCJRGrammarValid("kotlin5-empty", files) {
		t.Fatal("another closed grammar validator accepted the Kotlin5 closure")
	}
	for _, mutate := range []func([]kit.NativeInput) []kit.NativeInput{
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Bytes--; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].SHA256 = f[2].SHA256; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[2].Path = "src/alloc.h"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].Role = "parser"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { return f[:4] },
		func(f []kit.NativeInput) []kit.NativeInput { f[0], f[1] = f[1], f[0]; return f },
	} {
		if classificationPagedGrammarValid("kotlin5-empty", mutate(slices.Clone(files))) {
			t.Fatal("Kotlin5 grammar mutation accepted")
		}
	}
}

func TestQueryClassificationKotlin5EmptyStderrCaptureMember(t *testing.T) {
	processResult, ownerResult := &runner.Result{}, &runner.Result{}
	result := &CaseResult{Process: processResult}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
	classificationNormalizeCaptureStderr("kotlin5-empty", result, capture)
	for _, process := range []*runner.Result{processResult, ownerResult} {
		if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
			t.Fatal("Kotlin5 zero-byte stderr was not made explicit in both process records")
		}
	}
	files, err := capture.files("capture/kotlin5-empty-definitions")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, file := range files {
		if file.pin.Role == "stderr" {
			rows++
			if file.pin.Bytes != 0 || file.pin.SHA256 != sha(nil) || file.data == nil || len(file.data) != 0 {
				t.Fatal("Kotlin5 empty stderr member differs from the process")
			}
		}
	}
	if rows != 1 {
		t.Fatal("Kotlin5 empty stderr member was not serialized exactly once")
	}
	stderr := []byte{0x41}
	processResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	ownerResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	classificationNormalizeCaptureStderr("kotlin5-empty", &CaseResult{Process: processResult}, &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult})
	if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) {
		t.Fatal("Kotlin5 nonempty stderr was rewritten")
	}
}
