package native

import (
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationTSQL8StaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("tsql8-empty")
	if !ok || spec.label != "TSQL8" || spec.route != "tsql" || spec.symbol != "tree_sitter_mssql" || spec.rootType != "program" ||
		len(spec.querySpecs) != 8 || spec.factQueries != 2 || spec.grammarCount != 5 || spec.symbolCount != 1386 || spec.fieldCount != 88 {
		t.Fatal("closed TSQL8 paged profile missing or changed")
	}
	profile, err := classificationCaptureProfileFor("tsql8-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA || profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != 8 || profile.grammarCount != 5 || !profile.paged {
		t.Fatalf("TSQL8 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty", "cpp4-empty", "kotlin5-empty", "swift8-empty", "dart7-empty", "php5-empty", "javascript4-empty", "powershell3-tiny"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty" || name == "cpp4-empty" || name == "kotlin5-empty" || name == "swift8-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("TSQL8 paging changed another profile: %q, %#v, %v", name, other, err)
		}
		if other, ok := classificationCPPProfileSpecFor(name); ok && other.factQueries != 0 {
			t.Fatalf("fact queries leaked into the %q paged profile", name)
		}
	}
	for _, alias := range []string{"tsql", "tsql8", "tsql8-empty ", "TSQL8-empty", "tsql9-empty"} {
		if _, ok := classificationCPPProfileSpecFor(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if _, err := classificationCaptureProfileFor(alias); err == nil {
			t.Fatalf("near-alias capture profile %q accepted", alias)
		}
	}
	if _, ok := classificationCJRSpecFor("tsql8-empty"); ok || classificationCJRProfile("tsql8-empty") || classificationEmptyProfileName("tsql8-empty") {
		t.Fatal("TSQL8 entered an unpaged closed family")
	}
	if _, ok := classificationSmall17ProfileFor("tsql8-empty"); ok {
		t.Fatal("TSQL8 entered the Small17 family")
	}
	if !classificationGoCaptureEntrySkipsProfile("tsql8-empty") {
		t.Fatal("generic Go capture entry would take the TSQL8 profile")
	}
	data, anchors := classificationPagedReadAnchors(t, "tsql8-empty")
	if sha(data) != spec.anchorSHA {
		t.Fatal("TSQL8 anchor pin differs")
	}
	if err := classificationPagedAnchorsValid("tsql8-empty", anchors); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryValid("tsql8-empty", anchors); err != nil {
		t.Fatal(err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/tsql.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryBytesValid("tsql8-empty", anchors, append(slices.Clone(queryData), ' ')); err == nil {
		t.Fatal("TSQL8 query registry byte mutation accepted")
	}
	// Six ordinary queries precede the two pinned fact queries.
	if err := classificationPagedQueryRegistryBytesValid("tsql8-empty", anchors, queryData); err != nil {
		t.Fatal(err)
	}
	factData, err := os.ReadFile(classificationSmall17FactPackPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedFactPackValid("tsql8-empty", anchors, factData); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedFactPackValid("tsql8-empty", anchors, append(slices.Clone(factData), ' ')); err == nil {
		t.Fatal("TSQL8 fact query pack byte mutation accepted")
	}
	if err := classificationPagedFactPackValid("tsql8-empty", anchors, queryData); err == nil {
		t.Fatal("TSQL8 accepted the query file as the fact query pack")
	}
	if err := classificationPagedFactPackValid("swift8-empty", anchors, factData); err == nil {
		t.Fatal("a profile without fact queries accepted a fact query pack")
	}
	for name, change := range map[string]func(*classificationLocalAnchors){
		"fact-source": func(a *classificationLocalAnchors) { a.Queries[7].Source += " " },
		"fact-id":     func(a *classificationLocalAnchors) { a.Queries[7].ID = "facts.declarations" },
		"fact-count":  func(a *classificationLocalAnchors) { a.Queries = a.Queries[:7] },
	} {
		changed := anchors
		changed.Queries = slices.Clone(anchors.Queries)
		change(&changed)
		if classificationPagedFactPackValid("tsql8-empty", changed, factData) == nil {
			t.Fatalf("detached TSQL8 fact anchor %s accepted", name)
		}
	}
	// The ordinary registry does not cover either trailing fact query.
	detached := anchors
	detached.Queries = slices.Clone(anchors.Queries)
	detached.Queries[0].Source += "\n"
	if classificationPagedQueryRegistryBytesValid("tsql8-empty", detached, queryData) == nil {
		t.Fatal("TSQL8 anchor query source detached from the pinned registry accepted")
	}
	// The four paged profiles never accept each other's anchors.
	for _, other := range []string{"cpp4-empty", "kotlin5-empty", "swift8-empty", "postgresql9-empty"} {
		if classificationPagedAnchorsValid(other, anchors) == nil || classificationPagedQueryRegistryValid(other, anchors) == nil {
			t.Fatalf("the %s validator accepted the TSQL8 anchors", other)
		}
		_, otherAnchors := classificationPagedReadAnchors(t, other)
		if classificationPagedAnchorsValid("tsql8-empty", otherAnchors) == nil || classificationPagedQueryRegistryValid("tsql8-empty", otherAnchors) == nil {
			t.Fatalf("the TSQL8 validator accepted the %s anchors", other)
		}
	}
	if classificationCJRAnchorsValid("tsql8-empty", anchors) == nil {
		t.Fatal("CJR validator accepted the TSQL8 profile")
	}
}

func TestQueryClassificationTSQL8AnchorGuards(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*classificationLocalAnchors)
	}{
		{"route", func(a *classificationLocalAnchors) { a.Route = "postgresql-sql" }},
		{"case", func(a *classificationLocalAnchors) { a.Case = "swift8-empty-definitions" }},
		{"api", func(a *classificationLocalAnchors) { a.API = true }},
		{"source", func(a *classificationLocalAnchors) { a.Source.UTF8, a.Source.Bytes = "\n", 1 }},
		{"root-type", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "source_file" }},
		{"root-named", func(a *classificationLocalAnchors) { a.Nodes[0].Named = false }},
		{"abi-symbols", func(a *classificationLocalAnchors) { a.ABI.SymbolCount = 1385 }},
		{"abi-fields", func(a *classificationLocalAnchors) { a.ABI.FieldCount = 1 }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) { a.Queries[2].Source += " " }},
		{"fact-query-source", func(a *classificationLocalAnchors) { a.Queries[7].Source += " " }},
		{"patterns", func(a *classificationLocalAnchors) { a.Queries[2].Metadata.Patterns = 4 }},
		{"fact-patterns", func(a *classificationLocalAnchors) { a.Queries[7].Metadata.Patterns = 13 }},
		{"captures", func(a *classificationLocalAnchors) {
			a.Queries[2].Metadata.CaptureNames = []string{"listen_channel", "notify_channel"}
		}},
		{"predicates-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"query-count", func(a *classificationLocalAnchors) { a.Queries = a.Queries[:7] }},
		{"required", func(a *classificationLocalAnchors) { a.Required = a.Required[:7] }},
		{"required-null", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, anchors := classificationPagedReadAnchors(t, "tsql8-empty")
			mutate.edit(&anchors)
			if err := classificationPagedAnchorsValid("tsql8-empty", anchors); err == nil {
				t.Fatal("TSQL8 anchor mutation accepted")
			}
		})
	}
}

func TestQueryClassificationTSQL8GrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("tsql8-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationPagedGrammarValid("tsql8-empty", files) {
		t.Fatal("TSQL8 grammar closure differs from the frozen native-routes row")
	}
	// Four full 16 MiB parts plus a short tail, within the 100 MiB build-file ceiling.
	if files[0].Bytes <= 16<<20 || files[0].Bytes > 100<<20 || (files[0].Bytes+(16<<20)-1)/(16<<20) != 5 || files[0].Bytes-4*(16<<20) != 1538328 {
		t.Fatal("TSQL8 parser is not the expected five-part paged parser")
	}
	if classificationCPPGrammarValid(files) || classificationCJRGrammarValid("tsql8-empty", files) || classificationPagedGrammarValid("kotlin5-empty", files) || classificationPagedGrammarValid("swift8-empty", files) {
		t.Fatal("another closed grammar validator accepted the TSQL8 closure")
	}
	for _, mutate := range []func([]kit.NativeInput) []kit.NativeInput{
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Bytes--; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Path = "other/src/parser.c"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].SHA256 = f[2].SHA256; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[2].Path = "postgres/src/alloc.h"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].Role = "parser"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { return f[:4] },
		func(f []kit.NativeInput) []kit.NativeInput { f[0], f[1] = f[1], f[0]; return f },
	} {
		if classificationPagedGrammarValid("tsql8-empty", mutate(slices.Clone(files))) {
			t.Fatal("TSQL8 grammar mutation accepted")
		}
	}
}

func TestQueryClassificationTSQL8EmptyStderrCaptureMember(t *testing.T) {
	processResult, ownerResult := &runner.Result{}, &runner.Result{}
	result := &CaseResult{Process: processResult}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
	classificationNormalizeCaptureStderr("tsql8-empty", result, capture)
	for _, process := range []*runner.Result{processResult, ownerResult} {
		if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
			t.Fatal("TSQL8 zero-byte stderr was not made explicit in both process records")
		}
	}
	files, err := capture.files("capture/tsql8-empty-definitions")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, file := range files {
		if file.pin.Role == "stderr" {
			rows++
			if file.pin.Bytes != 0 || file.pin.SHA256 != sha(nil) || file.data == nil || len(file.data) != 0 {
				t.Fatal("TSQL8 empty stderr member differs from the process")
			}
		}
	}
	if rows != 1 {
		t.Fatal("TSQL8 empty stderr member was not serialized exactly once")
	}
	stderr := []byte{0x41}
	processResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	ownerResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	classificationNormalizeCaptureStderr("tsql8-empty", &CaseResult{Process: processResult}, &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult})
	if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) {
		t.Fatal("TSQL8 nonempty stderr was rewritten")
	}
}
