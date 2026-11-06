package kit

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const classificationKotlin5AnchorsSHA = "ffd6d41c2a784900d328e0d6b34c3608ca57262a65f987972c40f889019ce591"

func classificationKotlin5DefinitionProfile() classificationCPP4Profile {
	return classificationCPP4Profile{
		Label: "Kotlin5", ScopeName: "Kotlin", Language: "Kotlin",
		AnchorFile: "kotlin5-classification-anchors.json", AnchorSHA: classificationKotlin5AnchorsSHA,
		Route: "kotlin", Case: "kotlin5-empty-definitions", Workload: "kotlin5-empty-definition-single-frame",
		QueryFile: "src/testdata/native/queries/kotlin.json", QuerySHA: "a32e41d67ec2c3f9b7eb50274a740408edf22d962927fa9735670619b13ced55",
		Set: "s06-kotlin", Symbol: "tree_sitter_kotlin", RootType: "source_file", QueryBytes: 7135, QueryCount: 5,
		Grammar: classificationKotlin5GrammarPins(),
		ABI:     eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 304, FieldCount: 9},
		Counts:  classificationGoDefinitionSlotCounts{Cases: 38, Edits: 32, CaseSteps: 70, TreeRoles: 102, PotentialQueryTreeRoles: 510},
		Queries: []classificationCPP4QuerySpec{
			{id: "q.structure", sha256: "3948d9b3b28919dc7d78644ed99c383b8a0d0287b499393fe87719c08026a2cc", patterns: 2, captures: []string{"class", "function"}},
			{id: "q.probe-b02", sha256: "22d934886244d3623cabdf879e9dd38c1be8e93496a25a60a4f639bc86ae3104", patterns: 2, captures: []string{"condition", "catch_var"}},
			{id: "q.probe-v14", sha256: "ea6b48fdac43474737477653de739b92c41d28282adf63e711c7763429f8af19", patterns: 4, captures: []string{"class", "object", "dnn", "range"}},
			{id: "q.probe-v22", sha256: "01e3fc670a0c7656a4e5c516e7cf150ce2a2018c2bc7529a9288c436a5d0c916", patterns: 3, captures: []string{"class", "object", "alias"}},
			{id: "q.probe-v24", sha256: "d4f39e572fa9004ae8b3e7f0eb62c68a511e143e013127ca0617076baa99036e", patterns: 3, captures: []string{"class", "function", "property"}},
		},
	}
}

func classificationKotlin5GrammarPins() []NativeInput {
	return []NativeInput{
		{Path: "src/parser.c", Role: "parser", Bytes: 33870017, SHA256: "c13a776638ead65def356f8feeda4f612121cc96485fb4c78eed80757d906097"},
		{Path: "src/scanner.c", Role: "scanner", Bytes: 25985, SHA256: "8b51cea2bd9560201c1d681f56a329f3e637495eac2aaafba3622600c8abdf83"},
		{Path: "src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
}

// Closed paged-parser profiles: one grammar parser above 16 MiB, separate from the CJR and empty-profile maps.
func classificationPagedProfileForRoute(route string) (classificationCPP4Profile, bool) {
	switch route {
	case "cpp":
		return classificationCPP4DefinitionProfile(), true
	case "kotlin":
		return classificationKotlin5DefinitionProfile(), true
	case "swift":
		return classificationSwift8DefinitionProfile(), true
	default:
		return classificationCPP4Profile{}, false
	}
}

func classificationPagedProfileForName(name string) (classificationCPP4Profile, bool) {
	switch name {
	case "cpp4-empty":
		return classificationCPP4DefinitionProfile(), true
	case "kotlin5-empty":
		return classificationKotlin5DefinitionProfile(), true
	case "swift8-empty":
		return classificationSwift8DefinitionProfile(), true
	default:
		return classificationCPP4Profile{}, false
	}
}

func classificationPagedRoute(route string) bool {
	_, ok := classificationPagedProfileForRoute(route)
	return ok
}

func classificationKotlin5RowsTestFixture(t *testing.T) classificationCPP4RowsFixture {
	t.Helper()
	p := classificationKotlin5DefinitionProfile()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "kotlin5-empty")
	anchors, _ := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", AnchorsSHA256: anchors.sha256, NativeContext: semanticPagedContext,
		SourceRevision: strings.Repeat("a", 40), RunID: "kotlin5-rows-test", RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64), Grammar: classificationKotlin5GrammarPins()}
	preparedRoot := filepath.Join(t.TempDir(), "prepared")
	t.Setenv("TSGK_BENCH_PREPARED", preparedRoot)
	planRoot := filepath.Join(t.TempDir(), "kotlin5-plan")
	grammarRoot, output := filepath.Join(preparedRoot, "routes", "kotlin"), filepath.Join(planRoot, "library", "kotlin.dll")
	args, err := classificationGoCompilerArgs("kotlin", grammarRoot, output, plan.Grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan.LibraryRequest = eligibilityProcessRequest{Path: "C:/toolchain/cc.exe", Args: args, Dir: planRoot, Env: []string{}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := actualJSONRead(t, "../testdata/native/queries/kotlin.json")
	inputs, err := classificationPagedDefinitionLoadInputs(p, anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	f := classificationCPP4RowsFixture{anchors: anchors, plan: plan, inputs: inputs, inventoryData: inventoryData, qualificationData: qualificationData, queryData: queryData,
		witness: make([]QueryWitnessQuery, len(anchors.Queries)), native: make([]querywire.Query, len(anchors.Queries)), references: make([]*instrumentedReference, len(anchors.Queries))}
	for i, q := range anchors.Queries {
		metadata := classificationGoDefinitionCloneMetadata(q.Metadata)
		f.witness[i] = QueryWitnessQuery{ID: q.ID, Source: q.Source, Metadata: &metadata}
		f.native[i] = querywire.Query{ID: q.ID, Status: StatusCompleted, Patterns: q.Metadata.Patterns, CaptureNames: slices.Clone(q.Metadata.CaptureNames), Predicates: slices.Clone(q.Metadata.Predicates), Captures: [][]int64{}}
		reference := &instrumentedReference{Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = q.ID, q.SHA256
		reference.Query.Patterns, reference.Query.CaptureNames, reference.Query.Predicates = q.Metadata.Patterns, slices.Clone(q.Metadata.CaptureNames), slices.Clone(q.Metadata.Predicates)
		f.references[i] = reference
	}
	return f
}

func TestQueryClassificationKotlin5DefinitionBindings(t *testing.T) {
	f := classificationKotlin5RowsTestFixture(t)
	p := classificationKotlin5DefinitionProfile()
	if f.inputs.coordinates.Cases != 38 || f.inputs.coordinates.Edits != 32 || f.inputs.coordinates.CaseSteps != 70 || f.inputs.coordinates.TreeRoles != 102 ||
		f.inputs.coordinates.Definitions != 5 || f.inputs.coordinates.TreeRolesPerDefinition != 102 || f.inputs.coordinates.PotentialQueryTreeRoles != 510 ||
		f.inputs.coordinates.QuerySourceFileSHA256 != p.QuerySHA {
		t.Fatalf("Kotlin5 ordinary denominator changed: %#v", f.inputs.coordinates)
	}
	rows, err := classificationPagedDefinitionRows(p, f.anchors, f.inputs, f.witness, f.native, f.references)
	if err != nil || len(rows) != 5 {
		t.Fatalf("Kotlin5 rows: %d %v", len(rows), err)
	}
	for i, row := range rows {
		if row.Query != f.anchors.Queries[i].ID || row.SourceFile != p.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" ||
			row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 102 {
			t.Fatalf("Kotlin5 row %d differs: %#v", i, row)
		}
	}
	for _, mutate := range []struct {
		name string
		edit func(*classificationCPP4RowsFixture)
	}{
		{"source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/cpp.json"
		}},
		{"witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[0].Metadata.Predicates = nil }},
		{"native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[0].Predicates = nil }},
		{"reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[0].Query.Predicates = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := classificationCPP4CloneRowsFixture(f)
			mutate.edit(&changed)
			if _, err := classificationPagedDefinitionRows(p, changed.anchors, changed.inputs, changed.witness, changed.native, changed.references); err == nil {
				t.Fatal("Kotlin5 definition row mutation accepted")
			}
		})
	}
}

func TestQueryClassificationKotlin5ProfileAndPagedContext(t *testing.T) {
	f := classificationKotlin5RowsTestFixture(t)
	a, plan := f.anchors, f.plan
	p := classificationKotlin5DefinitionProfile()
	if err := classificationPagedAnchorsValid(p, a); err != nil || a.nativeContext() != semanticPagedContext || a.nativeSymbol() != p.Symbol || a.libraryName() != "kotlin.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatalf("Kotlin5 closed route binding differs: %v", err)
	}
	if !classificationPlanContextMatches(a, plan) {
		t.Fatal("closed Kotlin5 paged context rejected")
	}
	// The two paged profiles never accept each other's anchors, plans, or grammar closures.
	if classificationCPP4AnchorsValid(a) == nil || classificationPagedGrammarValid(p, classificationCPP4GrammarPins()) || classificationCPP4GrammarValid(plan.Grammar) {
		t.Fatal("Kotlin5 and CPP4 paged closures are interchangeable")
	}
	cppPlan := plan
	cppPlan.AnchorsSHA256 = classificationCPP4AnchorsSHA
	swapped := a
	swapped.sha256 = classificationCPP4AnchorsSHA
	if classificationPlanContextMatches(swapped, cppPlan) || classificationPagedDefinitionPlanValid(classificationCPP4DefinitionProfile(), a, plan) == nil {
		t.Fatal("Kotlin5 route accepted the CPP4 anchor or plan")
	}
	unpaged := plan
	unpaged.NativeContext = "full-input-utf8/r2"
	if classificationPlanContextMatches(a, unpaged) || classificationPagedDefinitionPlanValid(p, a, unpaged) == nil {
		t.Fatal("Kotlin5 accepted the unpaged context")
	}
	for _, mutate := range []func(*classificationPlan){
		func(changed *classificationPlan) { changed.Grammar[0].Bytes-- },
		func(changed *classificationPlan) { changed.Grammar[1].Path = "src/scanner.cc" },
		func(changed *classificationPlan) { changed.Grammar = changed.Grammar[:4] },
		func(changed *classificationPlan) {
			changed.LibraryRequest.Args = slices.Clone(changed.LibraryRequest.Args[:len(changed.LibraryRequest.Args)-1])
		},
		func(changed *classificationPlan) { changed.LibraryRequest.Memory.Bytes++ },
	} {
		changed := plan
		changed.Grammar = slices.Clone(plan.Grammar)
		changed.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
		mutate(&changed)
		if classificationPagedDefinitionPlanValid(p, a, changed) == nil {
			t.Fatal("Kotlin5 plan mutation accepted")
		}
	}
	uniqueReferenceIDs := map[string]string{}
	for i, q := range a.Queries {
		uniqueReferenceIDs[q.ID] = "current-ref-" + string(rune('0'+i))
	}
	if ids, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err != nil || len(ids) != 5 {
		t.Fatalf("Kotlin5 distinct references rejected: %v", err)
	}
	uniqueReferenceIDs[a.Queries[1].ID] = uniqueReferenceIDs[a.Queries[0].ID]
	if _, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err == nil {
		t.Fatal("Kotlin5 reused reference invocation accepted")
	}
	for _, bad := range []func(*classificationCPP4RowsFixture) []byte{
		func(changed *classificationCPP4RowsFixture) []byte {
			return append(slices.Clone(changed.queryData), ' ')
		},
		func(changed *classificationCPP4RowsFixture) []byte {
			return actualJSONRead(t, "../testdata/native/queries/cpp.json")
		},
	} {
		if _, err := classificationPagedDefinitionLoadInputs(p, a, plan, f.inventoryData, f.qualificationData, bad(&f)); err == nil {
			t.Fatal("Kotlin5 query source mutation accepted")
		}
	}
	grammarRoot, output := filepath.Join("prepared", "routes", "kotlin"), filepath.Join("library", "kotlin.dll")
	args, err := classificationGoCompilerArgs("kotlin", grammarRoot, output, classificationKotlin5GrammarPins())
	if err != nil || !slices.Contains(args, filepath.Join(grammarRoot, "src", "scanner.c")) || args[len(args)-2] != "-o" || args[len(args)-1] != output {
		t.Fatalf("Kotlin5 scanner or compiler output closure differs: %v %v", args, err)
	}
	for name, mutate := range map[string]func(*eligibilityProcessRequest){
		"compiler-path": func(request *eligibilityProcessRequest) { request.Path = "cc.exe" },
		"output":        func(request *eligibilityProcessRequest) { request.Args[len(request.Args)-1] += ".other" },
		"scanner-argv": func(request *eligibilityProcessRequest) {
			request.Args[6] = filepath.Join(grammarRoot, "src", "other.c")
		},
		"wall":   func(request *eligibilityProcessRequest) { request.Wall++ },
		"memory": func(request *eligibilityProcessRequest) { request.Memory.Hard = false },
		"stdout": func(request *eligibilityProcessRequest) { request.StdoutBytes-- },
		"stderr": func(request *eligibilityProcessRequest) { request.StderrBytes-- },
	} {
		badRequest := plan
		badRequest.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
		badRequest.LibraryRequest.Env = slices.Clone(plan.LibraryRequest.Env)
		mutate(&badRequest.LibraryRequest)
		if classificationPagedDefinitionPlanValid(p, a, badRequest) == nil {
			t.Fatalf("Kotlin5 noncanonical compiler request %s accepted", name)
		}
	}
	badConsumerPlan := plan
	badConsumerPlan.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
	badConsumerPlan.LibraryRequest.Args[len(badConsumerPlan.LibraryRequest.Args)-1] += ".preapproved-alias"
	if _, err := classificationPagedDefinitionLoadInputs(p, a, badConsumerPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("Kotlin5 consumer accepted a noncanonical pre-fixed compiler output plan")
	}
	badABI := a
	badABI.ABI.FieldCount++
	badRoot := a
	badRoot.Nodes = slices.Clone(a.Nodes)
	badRoot.Nodes[0].Type = "program"
	badOrder := a
	badOrder.Queries = slices.Clone(a.Queries)
	badOrder.Queries[0], badOrder.Queries[1] = badOrder.Queries[1], badOrder.Queries[0]
	badRequired := a
	badRequired.Required = slices.Clone(a.Required)
	badRequired.Required[0].Structural = nil
	for name, changed := range map[string]classificationAnchors{"abi": badABI, "root": badRoot, "query-order": badOrder, "required-null": badRequired} {
		if classificationPagedAnchorsValid(p, changed) == nil {
			t.Fatalf("changed Kotlin5 %s anchor accepted", name)
		}
	}
	for _, test := range []struct {
		name                            string
		query, inventory, qualification []byte
	}{
		{"inventory-bytes", f.queryData, append(slices.Clone(f.inventoryData), ' '), f.qualificationData},
		{"qualification-bytes", f.queryData, f.inventoryData, append(slices.Clone(f.qualificationData), ' ')},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := classificationPagedDefinitionLoadInputs(p, a, plan, test.inventory, test.qualification, test.query); err == nil {
				t.Fatal("changed Kotlin5 source inventory input accepted")
			}
		})
	}
	if _, err := classificationPagedDefinitionProjection(p, a, plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("ungrounded Kotlin5 projection accepted: %v", err)
	}
}

func TestQueryClassificationKotlin5ClosedNames(t *testing.T) {
	if got, ok := classificationPagedProfileForName("kotlin5-empty"); !ok || got.Route != "kotlin" || !reflect.DeepEqual(got, classificationKotlin5DefinitionProfile()) {
		t.Fatal("kotlin5-empty is not the closed Kotlin5 paged profile")
	}
	if got, ok := classificationPagedProfileForName("cpp4-empty"); !ok || !reflect.DeepEqual(got, classificationCPP4DefinitionProfile()) {
		t.Fatal("cpp4-empty paged profile changed")
	}
	if !classificationGoDefinitionProfileAllowed("kotlin5-empty") || !classificationGoLocalSkipsProfile("kotlin5-empty") {
		t.Fatal("kotlin5-empty is not admitted by the closed definition and local-skip entries")
	}
	for _, alias := range []string{"", "kotlin", "kotlin5", "kotlin5-empty ", "KOTLIN5-empty", "kotlin5-empty-alias", "kotlin8-empty", "swift8"} {
		if _, ok := classificationPagedProfileForName(alias); ok || classificationGoDefinitionProfileAllowed(alias) {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
	}
	for _, route := range []string{"", "Kotlin", "kotlin ", "Swift", "swift ", "java", "csharp"} {
		if classificationPagedRoute(route) {
			t.Fatalf("route %q entered the closed paged set", route)
		}
	}
	if _, err := classificationCJRProfileForName("kotlin5-empty"); err == nil {
		t.Fatal("Kotlin5 was admitted by the CJR entry")
	}
	if _, ok := classificationCJRProfileForRoute("kotlin"); ok {
		t.Fatal("Kotlin route entered the CJR map")
	}
	if _, ok := classificationEmptyProfileForRoute("kotlin"); ok || classificationSmall17ProfileName("kotlin5-empty") || classificationWeb16ProfileName("kotlin5-empty") {
		t.Fatal("Kotlin5 crossed into an unpaged empty-profile family")
	}
}

func TestQueryClassificationKotlin5StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "kotlin"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("Kotlin5 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("Kotlin5 explicit empty stderr rejected: %v", err)
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 1}, read); err == nil {
		t.Fatal("Kotlin5 stderr byte count mismatch accepted")
	}
}
