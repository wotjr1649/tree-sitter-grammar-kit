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

const classificationSwift8AnchorsSHA = "d5a5ef3d5189a2e7e0e6c2dbb18431ab8a66144e4824392374facf8b480e43b6"

func classificationSwift8DefinitionProfile() classificationCPP4Profile {
	return classificationCPP4Profile{
		Label: "Swift8", ScopeName: "Swift", Language: "Swift",
		AnchorFile: "swift8-classification-anchors.json", AnchorSHA: classificationSwift8AnchorsSHA,
		Route: "swift", Case: "swift8-empty-definitions", Workload: "swift8-empty-definition-single-frame",
		QueryFile: "src/testdata/native/queries/swift.json", QuerySHA: "29ae544ed6d729af5abf6d72653ab2df3829eec0cf8dfcfaf1fe8ddfb342c12c",
		Set: "s06-swift", Symbol: "tree_sitter_swift", RootType: "source_file", QueryBytes: 9390, QueryCount: 8,
		Grammar: classificationSwift8GrammarPins(),
		ABI:     eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 596, FieldCount: 48},
		Counts:  classificationGoDefinitionSlotCounts{Cases: 76, Edits: 62, CaseSteps: 138, TreeRoles: 200, PotentialQueryTreeRoles: 1600},
		Queries: []classificationCPP4QuerySpec{
			{id: "q.structure", sha256: "6d88d7735d57a8811212c5b2a1530bb38edba699c2410fe88e776df4c5bb1459", patterns: 4, captures: []string{"type", "protocol", "requirement", "function"}},
			{id: "q.probe-b02", sha256: "633bb081a17c16f86332a6342d67f80fa14a1d6800ca6e5f1e118ff094dc490b", patterns: 2, captures: []string{"capture", "keypath"}},
			{id: "q.probe-v51", sha256: "593535bc4f0c3366d2cbfca37ae26365609b21e0f8e0fa51a595e6258d03728e", patterns: 3, captures: []string{"opaque", "raw", "trailing_label"}},
			{id: "q.probe-v55", sha256: "be22a5c1e67ccf88e9c21ff5ec8a9d3e87fede0c98d7e2107ff56724f95618db", patterns: 2, captures: []string{"actor", "existential"}},
			{id: "q.probe-v59", sha256: "3fd8897fa2284fb0e46e430ed8eb25e4db523756ec5dfa49ae53d43a6a0c2088", patterns: 3, captures: []string{"macro", "thrown", "expansion"}},
			{id: "q.probe-v61", sha256: "1053aec87985bcc9739a03cc87e6d5f2d54e9a77432d345d0b2ea401a6ef2018", patterns: 3, captures: []string{"extension", "tparam", "capture"}},
			{id: "q.probe-v62", sha256: "ff3f2f1a325e9f09f80b6ef9a6184e764ddbb132f2eff2708f7ad58eaa7d5a37", patterns: 2, captures: []string{"attr", "unsafe"}},
			{id: "q.probe-v64", sha256: "aa4d6b32c8c84ce015800e49dc64f07132b5fe695ce8096c537112c6eff44745", patterns: 2, captures: []string{"optional", "attr"}},
		},
	}
}

func classificationSwift8GrammarPins() []NativeInput {
	return []NativeInput{
		{Path: "src/parser.c", Role: "parser", Bytes: 30636167, SHA256: "512ac3affb50c25a5f7b3106c4c1577aaa28e03e68f268fe4f037759af9e0ae4"},
		{Path: "src/scanner.c", Role: "scanner", Bytes: 39994, SHA256: "aee1af6476556021d0ff0cbad064818e21fde836eea3a8b919f31a2a326cdd9d"},
		{Path: "src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
}

func classificationSwift8RowsTestFixture(t *testing.T) classificationCPP4RowsFixture {
	t.Helper()
	p := classificationSwift8DefinitionProfile()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "swift8-empty")
	anchors, _ := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", AnchorsSHA256: anchors.sha256, NativeContext: semanticPagedContext,
		SourceRevision: strings.Repeat("a", 40), RunID: "swift8-rows-test", RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64), Grammar: classificationSwift8GrammarPins()}
	preparedRoot := filepath.Join(t.TempDir(), "prepared")
	t.Setenv("TSGK_BENCH_PREPARED", preparedRoot)
	planRoot := filepath.Join(t.TempDir(), "swift8-plan")
	grammarRoot, output := filepath.Join(preparedRoot, "routes", "swift"), filepath.Join(planRoot, "library", "swift.dll")
	args, err := classificationGoCompilerArgs("swift", grammarRoot, output, plan.Grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan.LibraryRequest = eligibilityProcessRequest{Path: "C:/toolchain/cc.exe", Args: args, Dir: planRoot, Env: []string{}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := actualJSONRead(t, "../testdata/native/queries/swift.json")
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

func TestQueryClassificationSwift8DefinitionBindings(t *testing.T) {
	f := classificationSwift8RowsTestFixture(t)
	p := classificationSwift8DefinitionProfile()
	if f.inputs.coordinates.Cases != 76 || f.inputs.coordinates.Edits != 62 || f.inputs.coordinates.CaseSteps != 138 || f.inputs.coordinates.TreeRoles != 200 ||
		f.inputs.coordinates.Definitions != 8 || f.inputs.coordinates.TreeRolesPerDefinition != 200 || f.inputs.coordinates.PotentialQueryTreeRoles != 1600 ||
		f.inputs.coordinates.QuerySourceFileSHA256 != p.QuerySHA {
		t.Fatalf("Swift8 ordinary denominator changed: %#v", f.inputs.coordinates)
	}
	rows, err := classificationPagedDefinitionRows(p, f.anchors, f.inputs, f.witness, f.native, f.references)
	if err != nil || len(rows) != 8 {
		t.Fatalf("Swift8 rows: %d %v", len(rows), err)
	}
	for i, row := range rows {
		if row.Query != f.anchors.Queries[i].ID || row.SourceFile != p.QueryFile || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" ||
			row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 200 {
			t.Fatalf("Swift8 row %d differs: %#v", i, row)
		}
	}
	for _, mutate := range []struct {
		name string
		edit func(*classificationCPP4RowsFixture)
	}{
		{"source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/kotlin.json"
		}},
		{"witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[0].Metadata.Predicates = nil }},
		{"native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[0].Predicates = nil }},
		{"reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[0].Query.Predicates = nil }},
		{"witness-missing-query", func(changed *classificationCPP4RowsFixture) { changed.witness = changed.witness[:7] }},
		{"native-missing-query", func(changed *classificationCPP4RowsFixture) { changed.native = changed.native[:7] }},
		{"reference-missing-query", func(changed *classificationCPP4RowsFixture) { changed.references = changed.references[:7] }},
		{"slot-group-missing-query", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries = changed.inputs.slotGroup.Queries[:7]
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := classificationCPP4CloneRowsFixture(f)
			mutate.edit(&changed)
			if _, err := classificationPagedDefinitionRows(p, changed.anchors, changed.inputs, changed.witness, changed.native, changed.references); err == nil {
				t.Fatal("Swift8 definition row mutation accepted")
			}
		})
	}
}

func TestQueryClassificationSwift8ProfileAndPagedContext(t *testing.T) {
	f := classificationSwift8RowsTestFixture(t)
	a, plan := f.anchors, f.plan
	p := classificationSwift8DefinitionProfile()
	if err := classificationPagedAnchorsValid(p, a); err != nil || a.nativeContext() != semanticPagedContext || a.nativeSymbol() != p.Symbol || a.libraryName() != "swift.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatalf("Swift8 closed route binding differs: %v", err)
	}
	if !classificationPlanContextMatches(a, plan) {
		t.Fatal("closed Swift8 paged context rejected")
	}
	// The parser is above the 16 MiB small reader and below the build-file ceiling: exactly two pages.
	if parser := p.Grammar[0].Bytes; parser <= 16<<20 || parser > 100<<20 || (parser+(16<<20)-1)/(16<<20) != 2 {
		t.Fatal("Swift8 parser is not the expected two-part paged parser")
	}
	// The three paged profiles never accept each other's anchors, plans, or grammar closures.
	kotlin, cpp := classificationKotlin5DefinitionProfile(), classificationCPP4DefinitionProfile()
	if classificationCPP4AnchorsValid(a) == nil || classificationPagedAnchorsValid(kotlin, a) == nil ||
		classificationPagedGrammarValid(p, classificationCPP4GrammarPins()) || classificationPagedGrammarValid(p, classificationKotlin5GrammarPins()) ||
		classificationPagedGrammarValid(kotlin, plan.Grammar) || classificationCPP4GrammarValid(plan.Grammar) {
		t.Fatal("Swift8 and another paged closure are interchangeable")
	}
	for name, other := range map[string]classificationCPP4Profile{"cpp4": cpp, "kotlin5": kotlin} {
		otherPlan := plan
		otherPlan.AnchorsSHA256 = other.AnchorSHA
		swapped := a
		swapped.sha256 = other.AnchorSHA
		if classificationPlanContextMatches(swapped, otherPlan) || classificationPagedDefinitionPlanValid(other, a, plan) == nil {
			t.Fatalf("Swift8 route accepted the %s anchor or plan", name)
		}
	}
	unpaged := plan
	unpaged.NativeContext = "full-input-utf8/r2"
	if classificationPlanContextMatches(a, unpaged) || classificationPagedDefinitionPlanValid(p, a, unpaged) == nil {
		t.Fatal("Swift8 accepted the unpaged context")
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
			t.Fatal("Swift8 plan mutation accepted")
		}
	}
	uniqueReferenceIDs := map[string]string{}
	for i, q := range a.Queries {
		uniqueReferenceIDs[q.ID] = "current-ref-" + string(rune('0'+i))
	}
	if ids, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err != nil || len(ids) != 8 {
		t.Fatalf("Swift8 distinct references rejected: %v", err)
	}
	uniqueReferenceIDs[a.Queries[1].ID] = uniqueReferenceIDs[a.Queries[0].ID]
	if _, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err == nil {
		t.Fatal("Swift8 reused reference invocation accepted")
	}
	for _, bad := range []func(*classificationCPP4RowsFixture) []byte{
		func(changed *classificationCPP4RowsFixture) []byte {
			return append(slices.Clone(changed.queryData), ' ')
		},
		func(changed *classificationCPP4RowsFixture) []byte {
			return actualJSONRead(t, "../testdata/native/queries/kotlin.json")
		},
	} {
		if _, err := classificationPagedDefinitionLoadInputs(p, a, plan, f.inventoryData, f.qualificationData, bad(&f)); err == nil {
			t.Fatal("Swift8 query source mutation accepted")
		}
	}
	grammarRoot, output := filepath.Join("prepared", "routes", "swift"), filepath.Join("library", "swift.dll")
	args, err := classificationGoCompilerArgs("swift", grammarRoot, output, classificationSwift8GrammarPins())
	if err != nil || !slices.Contains(args, filepath.Join(grammarRoot, "src", "scanner.c")) || args[len(args)-2] != "-o" || args[len(args)-1] != output {
		t.Fatalf("Swift8 scanner or compiler output closure differs: %v %v", args, err)
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
			t.Fatalf("Swift8 noncanonical compiler request %s accepted", name)
		}
	}
	badConsumerPlan := plan
	badConsumerPlan.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
	badConsumerPlan.LibraryRequest.Args[len(badConsumerPlan.LibraryRequest.Args)-1] += ".preapproved-alias"
	if _, err := classificationPagedDefinitionLoadInputs(p, a, badConsumerPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("Swift8 consumer accepted a noncanonical pre-fixed compiler output plan")
	}
	badABI := a
	badABI.ABI.SymbolCount--
	badFields := a
	badFields.ABI.FieldCount++
	badRoot := a
	badRoot.Nodes = slices.Clone(a.Nodes)
	badRoot.Nodes[0].Type = "program"
	badOrder := a
	badOrder.Queries = slices.Clone(a.Queries)
	badOrder.Queries[0], badOrder.Queries[1] = badOrder.Queries[1], badOrder.Queries[0]
	badSource := a
	badSource.Queries = slices.Clone(a.Queries)
	badSource.Queries[2].Source += " "
	badPatterns := a
	badPatterns.Queries = slices.Clone(a.Queries)
	badPatterns.Queries[2].Metadata.Patterns++
	badCaptures := a
	badCaptures.Queries = slices.Clone(a.Queries)
	badCaptures.Queries[2].Metadata.CaptureNames = []string{"opaque", "raw"}
	badCount := a
	badCount.Queries = slices.Clone(a.Queries[:7])
	badRequired := a
	badRequired.Required = slices.Clone(a.Required)
	badRequired.Required[0].Structural = nil
	for name, changed := range map[string]classificationAnchors{"abi-symbols": badABI, "abi-fields": badFields, "root": badRoot, "query-order": badOrder,
		"query-source": badSource, "patterns": badPatterns, "captures": badCaptures, "query-count": badCount, "required-null": badRequired} {
		if classificationPagedAnchorsValid(p, changed) == nil {
			t.Fatalf("changed Swift8 %s anchor accepted", name)
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
				t.Fatal("changed Swift8 source inventory input accepted")
			}
		})
	}
	if _, err := classificationPagedDefinitionProjection(p, a, plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("ungrounded Swift8 projection accepted: %v", err)
	}
}

func TestQueryClassificationSwift8ClosedNames(t *testing.T) {
	if got, ok := classificationPagedProfileForName("swift8-empty"); !ok || got.Route != "swift" || !reflect.DeepEqual(got, classificationSwift8DefinitionProfile()) {
		t.Fatal("swift8-empty is not the closed Swift8 paged profile")
	}
	if got, ok := classificationPagedProfileForRoute("swift"); !ok || !reflect.DeepEqual(got, classificationSwift8DefinitionProfile()) {
		t.Fatal("swift is not the closed Swift8 paged route")
	}
	if got, ok := classificationPagedProfileForName("cpp4-empty"); !ok || !reflect.DeepEqual(got, classificationCPP4DefinitionProfile()) {
		t.Fatal("cpp4-empty paged profile changed")
	}
	if got, ok := classificationPagedProfileForName("kotlin5-empty"); !ok || !reflect.DeepEqual(got, classificationKotlin5DefinitionProfile()) {
		t.Fatal("kotlin5-empty paged profile changed")
	}
	if !classificationGoDefinitionProfileAllowed("swift8-empty") || !classificationGoLocalSkipsProfile("swift8-empty") {
		t.Fatal("swift8-empty is not admitted by the closed definition and local-skip entries")
	}
	for _, alias := range []string{"", "swift", "swift8", "swift8-empty ", "SWIFT8-empty", "swift8-empty-alias", "swift5-empty", "kotlin8-empty", "kotlin5"} {
		if _, ok := classificationPagedProfileForName(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if classificationGoDefinitionProfileAllowed(alias) {
			t.Fatalf("near-alias definition profile %q accepted", alias)
		}
	}
	for _, route := range []string{"", "Swift", "swift ", "swift8", "java", "csharp", "dart"} {
		if classificationPagedRoute(route) {
			t.Fatalf("route %q entered the closed paged set", route)
		}
	}
	if _, err := classificationCJRProfileForName("swift8-empty"); err == nil {
		t.Fatal("Swift8 was admitted by the CJR entry")
	}
	if _, ok := classificationCJRProfileForRoute("swift"); ok {
		t.Fatal("Swift route entered the CJR map")
	}
	if _, ok := classificationEmptyProfileForRoute("swift"); ok || classificationSmall17ProfileName("swift8-empty") || classificationWeb16ProfileName("swift8-empty") {
		t.Fatal("Swift8 crossed into an unpaged empty-profile family")
	}
}

func TestQueryClassificationSwift8StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "swift"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("Swift8 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("Swift8 explicit empty stderr rejected: %v", err)
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 1}, read); err == nil {
		t.Fatal("Swift8 stderr byte count mismatch accepted")
	}
}
