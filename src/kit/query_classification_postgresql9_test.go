package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

const classificationPostgreSQL9AnchorsSHA = "9a6f0bd033a757e761d6321e58a9361397b97f3defeee93f844ef1b55ca2b9a6"

func classificationPostgreSQL9DefinitionProfile() classificationCPP4Profile {
	return classificationCPP4Profile{
		Label: "PostgreSQL9", ScopeName: "PostgreSQL", Language: "PostgreSQL",
		AnchorFile: "postgresql9-classification-anchors.json", AnchorSHA: classificationPostgreSQL9AnchorsSHA,
		Route: "postgresql-sql", Case: "postgresql9-empty-definitions", Workload: "postgresql9-empty-definition-single-frame",
		QueryFile: "src/testdata/native/queries/postgresql-sql.json", QuerySHA: "2ed90fffb67dda344fd85b6f01c1647d1dad4dce1d6befd2a05e5960676edebd",
		Set: "s06-postgresql-sql", Symbol: "tree_sitter_postgres", RootType: "source_file", QueryBytes: 8110, QueryCount: 9,
		Grammar: classificationPostgreSQL9GrammarPins(),
		ABI:     eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 1280, FieldCount: 0},
		Counts:  classificationGoDefinitionSlotCounts{Cases: 537, Edits: 64, CaseSteps: 601, TreeRoles: 665, PotentialQueryTreeRoles: 5985},
		Queries: []classificationCPP4QuerySpec{
			{id: "q.structure", sha256: "35e55e9d63eafae23dfd6fd23e9f52018574905884d99d78227fa7abad0a0a4e", patterns: 4, captures: []string{"table", "column", "cte", "function"}},
			{id: "q.probe-b01-strings", sha256: "9004103236ba4fdcb76b71442a6c2ba5a3b14ec031416f313633b974e92253fa", patterns: 4, captures: []string{"dollar", "escape", "bit", "hex"}},
			{id: "q.probe-b04-utility", sha256: "b4264eb4db18dd8c652f890dce53b1b07a82b628b80c583b0a73c8b4d7e080c7", patterns: 3, captures: []string{"listen_channel", "notify_channel", "prepared_name"}},
			{id: "q.probe-v10-partition", sha256: "01d1260c7c90dd7c0ee0286f4f6bbb850a84a903314ba94db7207dd10ca54da2", patterns: 2, captures: []string{"partition_spec", "partition_bound"}},
			{id: "q.probe-v13-merge", sha256: "b5e10f700059e1cfc7712a3dd2ca091ea3b76f152dc50d23894015946239c1ad", patterns: 1, captures: []string{"merge_when"}},
			{id: "q.probe-v16-json-table", sha256: "37e8d705152701242987e07600c6aff590be3f2b6ae9b75f907e179999175f8f", patterns: 1, captures: []string{"jt_column"}},
			{id: "q.probe-v18-returning", sha256: "0fbedd7084e35b58bd9f50c787e3015d3049c8b62dc145cdb548ef632199a6f3", patterns: 1, captures: []string{"returning_alias"}},
			{id: "q.probe-l01-oids", sha256: "b72a313c2f8e47b0fa1022d89a26983808000413e8634fb5bffc9143667d334d", patterns: 1, captures: []string{"opt_with"}},
			{id: "facts.declarations", sha256: "1f2284ae901d05903ec6fc8418420a9a459806e2e15ea9d1e45033aeeba528aa", patterns: 16, captures: []string{
				"c.0.0.0", "c.0.1.1", "c.0.1.0", "c.1.0.0", "c.1.1.1", "c.1.1.0", "c.2.0.0", "c.2.1.1", "c.2.1.0", "c.3.0.0", "c.3.1.1", "c.3.1.0",
				"c.4.0.0", "c.4.1.1", "c.4.1.0", "c.5.0.0", "c.5.1.1", "c.5.1.0", "c.6.0.0", "c.6.1.1", "c.6.1.0", "c.7.0.0", "c.7.1.1", "c.7.1.0"}},
		},
		FactQueries: []string{"facts.declarations"},
	}
}

// The grammar directory is postgres/, so the logical closure paths carry that prefix.
func classificationPostgreSQL9GrammarPins() []NativeInput {
	return []NativeInput{
		{Path: "postgres/src/parser.c", Role: "parser", Bytes: 104285731, SHA256: "0db8885c7e1f6c57d3c9dabf066f1397b4d3ec41e934079d02aa74998fc2882e"},
		{Path: "postgres/src/scanner.c", Role: "scanner", Bytes: 4855, SHA256: "ccbf351fc4e04def876cd2ae07ff081c860f4c2f53265723855d1c36729849a7"},
		{Path: "postgres/src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "postgres/src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "postgres/src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
}

func classificationPostgreSQL9Envelope(t *testing.T, queryFile, factPack []byte) []byte {
	t.Helper()
	data, err := jsonv2.Marshal(classificationCSharpDefinitionQueryInputs{QueryFile: queryFile, FactQueryPack: factPack})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func classificationPostgreSQL9RowsTestFixture(t *testing.T) classificationCPP4RowsFixture {
	t.Helper()
	p := classificationPostgreSQL9DefinitionProfile()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "postgresql9-empty")
	anchors, _ := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", AnchorsSHA256: anchors.sha256, NativeContext: semanticPagedContext,
		SourceRevision: strings.Repeat("a", 40), RunID: "postgresql9-rows-test", RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64), Grammar: classificationPostgreSQL9GrammarPins()}
	preparedRoot := filepath.Join(t.TempDir(), "prepared")
	t.Setenv("TSGK_BENCH_PREPARED", preparedRoot)
	planRoot := filepath.Join(t.TempDir(), "postgresql9-plan")
	grammarRoot, output := filepath.Join(preparedRoot, "routes", "postgresql-sql"), filepath.Join(planRoot, "library", "postgresql-sql.dll")
	args, err := classificationGoCompilerArgs("postgresql-sql", grammarRoot, output, plan.Grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan.LibraryRequest = eligibilityProcessRequest{Path: "C:/toolchain/cc.exe", Args: args, Dir: planRoot, Env: []string{}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := classificationPostgreSQL9Envelope(t, actualJSONRead(t, "../testdata/native/queries/postgresql-sql.json"), actualJSONRead(t, "../contracts/fact-query-pack.json"))
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

func TestQueryClassificationPostgreSQL9DefinitionBindings(t *testing.T) {
	f := classificationPostgreSQL9RowsTestFixture(t)
	p := classificationPostgreSQL9DefinitionProfile()
	c := f.inputs.coordinates
	wantPins := []classificationGoDefinitionQuerySourcePin{
		{Path: "src/testdata/native/queries/postgresql-sql.json", Bytes: 8110, SHA256: p.QuerySHA},
		{Path: "src/contracts/fact-query-pack.json", Bytes: 9702, SHA256: classificationCSharpFactQueryPackSHA},
	}
	if c.Cases != 537 || c.Edits != 64 || c.CaseSteps != 601 || c.TreeRoles != 665 || c.Definitions != 9 || c.TreeRolesPerDefinition != 665 || c.PotentialQueryTreeRoles != 5985 ||
		c.QuerySourceFileSHA256 != "" || !reflect.DeepEqual(c.QuerySourceFiles, wantPins) {
		t.Fatalf("PostgreSQL9 ordinary denominator or source pins changed: %#v", c)
	}
	rows, err := classificationPagedDefinitionRows(p, f.anchors, f.inputs, f.witness, f.native, f.references)
	if err != nil || len(rows) != 9 {
		t.Fatalf("PostgreSQL9 rows: %d %v", len(rows), err)
	}
	for i, row := range rows {
		wantFile := p.QueryFile
		if i == 8 {
			wantFile = "src/contracts/fact-query-pack.json"
		}
		if row.Query != f.anchors.Queries[i].ID || row.SourceFile != wantFile || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" ||
			row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 665 {
			t.Fatalf("PostgreSQL9 row %d differs: %#v", i, row)
		}
	}
	for _, mutate := range []struct {
		name string
		edit func(*classificationCPP4RowsFixture)
	}{
		{"source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/tsql.json"
		}},
		{"fact-source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[8].SourceFile = p.QueryFile
		}},
		{"ordinary-as-fact-source", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/contracts/fact-query-pack.json"
		}},
		{"witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[0].Metadata.Predicates = nil }},
		{"native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[0].Predicates = nil }},
		{"reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[0].Query.Predicates = nil }},
		{"fact-witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[8].Metadata.Predicates = nil }},
		{"fact-native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[8].Predicates = nil }},
		{"fact-reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[8].Query.Predicates = nil }},
		{"witness-missing-query", func(changed *classificationCPP4RowsFixture) { changed.witness = changed.witness[:8] }},
		{"native-missing-query", func(changed *classificationCPP4RowsFixture) { changed.native = changed.native[:8] }},
		{"reference-missing-query", func(changed *classificationCPP4RowsFixture) { changed.references = changed.references[:8] }},
		{"slot-group-missing-query", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries = changed.inputs.slotGroup.Queries[:8]
		}},
		{"raw-source-missing-fact", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.queryFile.Queries = changed.inputs.queryFile.Queries[:8]
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := classificationCPP4CloneRowsFixture(f)
			mutate.edit(&changed)
			if _, err := classificationPagedDefinitionRows(p, changed.anchors, changed.inputs, changed.witness, changed.native, changed.references); err == nil {
				t.Fatal("PostgreSQL9 definition row mutation accepted")
			}
		})
	}
}

func TestQueryClassificationPostgreSQL9ProfileAndPagedContext(t *testing.T) {
	f := classificationPostgreSQL9RowsTestFixture(t)
	a, plan := f.anchors, f.plan
	p := classificationPostgreSQL9DefinitionProfile()
	if err := classificationPagedAnchorsValid(p, a); err != nil || a.nativeContext() != semanticPagedContext || a.nativeSymbol() != p.Symbol || a.libraryName() != "postgresql-sql.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatalf("PostgreSQL9 closed route binding differs: %v", err)
	}
	if !classificationPlanContextMatches(a, plan) {
		t.Fatal("closed PostgreSQL9 paged context rejected")
	}
	// The parser is the largest paged parser: six full 16 MiB parts and a short tail, still within the build-file ceiling.
	const page = 16 << 20
	if parser := p.Grammar[0].Bytes; parser <= page || parser > 100<<20 || (parser+page-1)/page != 7 || parser-6*page != 3622435 {
		t.Fatal("PostgreSQL9 parser is not the expected seven-part paged parser")
	}
	// The four paged profiles never accept each other's anchors, plans, or grammar closures.
	others := map[string]classificationCPP4Profile{"cpp4": classificationCPP4DefinitionProfile(), "kotlin5": classificationKotlin5DefinitionProfile(), "swift8": classificationSwift8DefinitionProfile()}
	for name, other := range others {
		otherPlan := plan
		otherPlan.AnchorsSHA256 = other.AnchorSHA
		swapped := a
		swapped.sha256 = other.AnchorSHA
		if classificationPagedAnchorsValid(other, a) == nil || classificationPagedGrammarValid(p, other.Grammar) || classificationPagedGrammarValid(other, plan.Grammar) ||
			classificationPlanContextMatches(swapped, otherPlan) || classificationPagedDefinitionPlanValid(other, a, plan) == nil {
			t.Fatalf("PostgreSQL9 and the %s paged closure are interchangeable", name)
		}
	}
	unpaged := plan
	unpaged.NativeContext = "full-input-utf8/r2"
	if classificationPlanContextMatches(a, unpaged) || classificationPagedDefinitionPlanValid(p, a, unpaged) == nil {
		t.Fatal("PostgreSQL9 accepted the unpaged context")
	}
	for _, mutate := range []func(*classificationPlan){
		func(changed *classificationPlan) { changed.Grammar[0].Bytes-- },
		func(changed *classificationPlan) { changed.Grammar[0].Path = "src/parser.c" },
		func(changed *classificationPlan) { changed.Grammar[1].Path = "postgres/src/scanner.cc" },
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
			t.Fatal("PostgreSQL9 plan mutation accepted")
		}
	}
	uniqueReferenceIDs := map[string]string{}
	for i, q := range a.Queries {
		uniqueReferenceIDs[q.ID] = "current-ref-" + string(rune('0'+i))
	}
	if ids, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err != nil || len(ids) != 9 {
		t.Fatalf("PostgreSQL9 distinct references rejected: %v", err)
	}
	uniqueReferenceIDs[a.Queries[8].ID] = uniqueReferenceIDs[a.Queries[0].ID]
	if _, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err == nil {
		t.Fatal("PostgreSQL9 reused reference invocation accepted")
	}
	// The raw query file alone, another route's file, or an envelope with a missing, altered, or reordered fact pack must fail.
	queryFile := actualJSONRead(t, "../testdata/native/queries/postgresql-sql.json")
	factPack := actualJSONRead(t, "../contracts/fact-query-pack.json")
	for name, bad := range map[string][]byte{
		"raw-query-file-only":    queryFile,
		"other-route-raw-file":   actualJSONRead(t, "../testdata/native/queries/kotlin.json"),
		"fact-pack-missing":      classificationPostgreSQL9Envelope(t, queryFile, nil),
		"query-file-missing":     classificationPostgreSQL9Envelope(t, nil, factPack),
		"fact-pack-extra-byte":   classificationPostgreSQL9Envelope(t, queryFile, append(slices.Clone(factPack), ' ')),
		"query-file-extra-byte":  classificationPostgreSQL9Envelope(t, append(slices.Clone(queryFile), ' '), factPack),
		"swapped-envelope-files": classificationPostgreSQL9Envelope(t, factPack, queryFile),
	} {
		if _, err := classificationPagedDefinitionLoadInputs(p, a, plan, f.inventoryData, f.qualificationData, bad); err == nil {
			t.Fatalf("PostgreSQL9 query source mutation %s accepted", name)
		}
	}
	// The fact pack is pinned by hash, so route-level checks are exercised directly on altered packs of the same shape.
	facts, err := classificationPagedFactQueries(p, factPack)
	if err != nil || len(facts) != 1 || facts[0].ID != "facts.declarations" || facts[0].Source != a.Queries[8].Source {
		t.Fatalf("PostgreSQL9 fact route rejected or differs: %v", err)
	}
	replaceNth := func(data []byte, old string, n int, replacement string) []byte {
		index := -1
		for range n {
			next := bytes.Index(data[index+1:], []byte(old))
			if next < 0 {
				t.Fatalf("fact pack lacks occurrence %d of %q", n, old)
			}
			index += 1 + next
		}
		return append(append(slices.Clone(data[:index]), replacement...), data[index+len(old):]...)
	}
	for name, bad := range map[string][]byte{
		"route-missing": replaceNth(factPack, `"route": "postgresql-sql"`, 1, `"route": "postgresql-sqx"`),
		"route-duplicated": replaceNth(replaceNth(replaceNth(factPack, `"route": "xml"`, 1, `"route": "postgresql-sql"`),
			`"id": "facts.xml_structure"`, 1, `"id": "facts.declarations"`), `"facts": "xml_structure"`, 1, `"facts": "declarations"`),
		"fact-id-changed":   replaceNth(factPack, `"id": "facts.declarations"`, 3, `"id": "facts.declaration"`),
		"facts-key-changed": replaceNth(factPack, `"facts": "declarations"`, 3, `"facts": "dynamic_sql"`),
		"revision-changed":  replaceNth(factPack, `fact-queries-r1`, 1, `fact-queries-r2`),
	} {
		if _, err := classificationPagedFactQueries(p, bad); err == nil {
			t.Fatalf("PostgreSQL9 fact pack mutation %s accepted", name)
		}
	}
	// A profile without fact queries never accepts the PostgreSQL envelope (the subtest restores this test's environment).
	t.Run("swift8-rejects-envelope", func(t *testing.T) {
		swift := classificationSwift8RowsTestFixture(t)
		if _, err := classificationPagedDefinitionLoadInputs(classificationSwift8DefinitionProfile(), swift.anchors, swift.plan, swift.inventoryData, swift.qualificationData, f.queryData); err == nil {
			t.Fatal("Swift8 accepted the PostgreSQL9 query source envelope")
		}
	})
	grammarRoot, output := filepath.Join("prepared", "routes", "postgresql-sql"), filepath.Join("library", "postgresql-sql.dll")
	args, err := classificationGoCompilerArgs("postgresql-sql", grammarRoot, output, classificationPostgreSQL9GrammarPins())
	if err != nil || !slices.Contains(args, filepath.Join(grammarRoot, "postgres", "src", "scanner.c")) || args[6] != filepath.Join(grammarRoot, "postgres", "src", "scanner.c") ||
		args[len(args)-2] != "-o" || args[len(args)-1] != output {
		t.Fatalf("PostgreSQL9 scanner or compiler output closure differs: %v %v", args, err)
	}
	if _, err := classificationGoCompilerArgs("postgresql-sql", grammarRoot, output, classificationSwift8GrammarPins()); err == nil {
		t.Fatal("PostgreSQL9 compiler argv accepted an unprefixed grammar closure")
	}
	for name, mutate := range map[string]func(*eligibilityProcessRequest){
		"compiler-path": func(request *eligibilityProcessRequest) { request.Path = "cc.exe" },
		"output":        func(request *eligibilityProcessRequest) { request.Args[len(request.Args)-1] += ".other" },
		"scanner-argv": func(request *eligibilityProcessRequest) {
			request.Args[6] = filepath.Join(grammarRoot, "postgres", "src", "other.c")
		},
		"unprefixed-include": func(request *eligibilityProcessRequest) { request.Args[4] = filepath.Join(grammarRoot, "src") },
		"wall":               func(request *eligibilityProcessRequest) { request.Wall++ },
		"memory":             func(request *eligibilityProcessRequest) { request.Memory.Hard = false },
		"stdout":             func(request *eligibilityProcessRequest) { request.StdoutBytes-- },
		"stderr":             func(request *eligibilityProcessRequest) { request.StderrBytes-- },
	} {
		badRequest := plan
		badRequest.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
		badRequest.LibraryRequest.Env = slices.Clone(plan.LibraryRequest.Env)
		mutate(&badRequest.LibraryRequest)
		if classificationPagedDefinitionPlanValid(p, a, badRequest) == nil {
			t.Fatalf("PostgreSQL9 noncanonical compiler request %s accepted", name)
		}
	}
	badConsumerPlan := plan
	badConsumerPlan.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
	badConsumerPlan.LibraryRequest.Args[len(badConsumerPlan.LibraryRequest.Args)-1] += ".preapproved-alias"
	if _, err := classificationPagedDefinitionLoadInputs(p, a, badConsumerPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("PostgreSQL9 consumer accepted a noncanonical pre-fixed compiler output plan")
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
	badFactSource := a
	badFactSource.Queries = slices.Clone(a.Queries)
	badFactSource.Queries[8].Source += " "
	badPatterns := a
	badPatterns.Queries = slices.Clone(a.Queries)
	badPatterns.Queries[2].Metadata.Patterns++
	badCaptures := a
	badCaptures.Queries = slices.Clone(a.Queries)
	badCaptures.Queries[2].Metadata.CaptureNames = []string{"listen_channel", "notify_channel"}
	badCount := a
	badCount.Queries = slices.Clone(a.Queries[:8])
	badRequired := a
	badRequired.Required = slices.Clone(a.Required)
	badRequired.Required[0].Structural = nil
	for name, changed := range map[string]classificationAnchors{"abi-symbols": badABI, "abi-fields": badFields, "root": badRoot, "query-order": badOrder,
		"query-source": badSource, "fact-query-source": badFactSource, "patterns": badPatterns, "captures": badCaptures, "query-count": badCount, "required-null": badRequired} {
		if classificationPagedAnchorsValid(p, changed) == nil {
			t.Fatalf("changed PostgreSQL9 %s anchor accepted", name)
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
				t.Fatal("changed PostgreSQL9 source inventory input accepted")
			}
		})
	}
	if _, err := classificationPagedDefinitionProjection(p, a, plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("ungrounded PostgreSQL9 projection accepted: %v", err)
	}
}

func TestQueryClassificationPostgreSQL9ClosedNames(t *testing.T) {
	if got, ok := classificationPagedProfileForName("postgresql9-empty"); !ok || got.Route != "postgresql-sql" || !reflect.DeepEqual(got, classificationPostgreSQL9DefinitionProfile()) {
		t.Fatal("postgresql9-empty is not the closed PostgreSQL9 paged profile")
	}
	if got, ok := classificationPagedProfileForRoute("postgresql-sql"); !ok || !reflect.DeepEqual(got, classificationPostgreSQL9DefinitionProfile()) {
		t.Fatal("postgresql-sql is not the closed PostgreSQL9 paged route")
	}
	if got, ok := classificationPagedProfileForName("cpp4-empty"); !ok || !reflect.DeepEqual(got, classificationCPP4DefinitionProfile()) || got.FactQueries != nil {
		t.Fatal("cpp4-empty paged profile changed")
	}
	if got, ok := classificationPagedProfileForName("kotlin5-empty"); !ok || !reflect.DeepEqual(got, classificationKotlin5DefinitionProfile()) || got.FactQueries != nil {
		t.Fatal("kotlin5-empty paged profile changed")
	}
	if got, ok := classificationPagedProfileForName("swift8-empty"); !ok || !reflect.DeepEqual(got, classificationSwift8DefinitionProfile()) || got.FactQueries != nil {
		t.Fatal("swift8-empty paged profile changed")
	}
	if !classificationGoDefinitionProfileAllowed("postgresql9-empty") || !classificationGoLocalSkipsProfile("postgresql9-empty") {
		t.Fatal("postgresql9-empty is not admitted by the closed definition and local-skip entries")
	}
	for _, alias := range []string{"", "postgresql-sql", "postgresql9", "postgresql9-empty ", "POSTGRESQL9-empty", "postgres9-empty", "tsql8-empty", "swift8"} {
		if _, ok := classificationPagedProfileForName(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if classificationGoDefinitionProfileAllowed(alias) {
			t.Fatalf("near-alias definition profile %q accepted", alias)
		}
	}
	for _, route := range []string{"", "PostgreSQL-sql", "postgresql-sql ", "postgres", "tsql"} {
		if classificationPagedRoute(route) {
			t.Fatalf("route %q entered the closed paged set", route)
		}
	}
	if !classificationPagedRoute("postgresql-sql") {
		t.Fatal("postgresql-sql is not in the closed paged set")
	}
	if _, err := classificationCJRProfileForName("postgresql9-empty"); err == nil {
		t.Fatal("PostgreSQL9 was admitted by the CJR entry")
	}
	if _, ok := classificationCJRProfileForRoute("postgresql-sql"); ok {
		t.Fatal("PostgreSQL route entered the CJR map")
	}
	if _, ok := classificationEmptyProfileForRoute("postgresql-sql"); ok || classificationSmall17ProfileName("postgresql9-empty") || classificationWeb16ProfileName("postgresql9-empty") {
		t.Fatal("PostgreSQL9 crossed into an unpaged empty-profile family")
	}
}

func TestQueryClassificationPostgreSQL9StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "postgresql-sql"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("PostgreSQL9 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("PostgreSQL9 explicit empty stderr rejected: %v", err)
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 1}, read); err == nil {
		t.Fatal("PostgreSQL9 stderr byte count mismatch accepted")
	}
}
