package kit

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func classificationTSQL8Envelope(t *testing.T, queryFile, factPack []byte) []byte {
	t.Helper()
	data, err := jsonv2.Marshal(classificationCSharpDefinitionQueryInputs{QueryFile: queryFile, FactQueryPack: factPack})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func classificationTSQL8RowsTestFixture(t *testing.T) classificationCPP4RowsFixture {
	t.Helper()
	p := classificationTSQL8DefinitionProfile()
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", "tsql8-empty")
	anchors, _ := classificationGoAnchors(t)
	plan := classificationPlan{Schema: "tsgk-query-classification-plan/r1", AnchorsSHA256: anchors.sha256, NativeContext: semanticPagedBinariesContext,
		SourceRevision: strings.Repeat("a", 40), RunID: "tsql8-rows-test", RuntimeManifestSHA256: strings.Repeat("b", 64), CompilerSHA256: strings.Repeat("c", 64), Grammar: classificationTSQL8GrammarPins()}
	preparedRoot := filepath.Join(t.TempDir(), "prepared")
	t.Setenv("TSGK_BENCH_PREPARED", preparedRoot)
	planRoot := filepath.Join(t.TempDir(), "tsql8-plan")
	grammarRoot, output := filepath.Join(preparedRoot, "routes", "tsql"), filepath.Join(planRoot, "library", "tsql.dll")
	args, err := classificationGoCompilerArgs("tsql", grammarRoot, output, plan.Grammar)
	if err != nil {
		t.Fatal(err)
	}
	plan.LibraryRequest = eligibilityProcessRequest{Path: "C:/toolchain/cc.exe", Args: args, Dir: planRoot, Env: []string{}, StdoutBytes: 16 << 20, StderrBytes: 4 << 20, Wall: 120e9, Grace: 2e9}
	plan.LibraryRequest.Memory.Bytes, plan.LibraryRequest.Memory.Hard = 4<<30, true
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryData := classificationTSQL8Envelope(t, actualJSONRead(t, "../testdata/native/queries/tsql.json"), actualJSONRead(t, "../contracts/fact-query-pack.json"))
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

func TestQueryClassificationTSQL8DefinitionBindings(t *testing.T) {
	f := classificationTSQL8RowsTestFixture(t)
	p := classificationTSQL8DefinitionProfile()
	c := f.inputs.coordinates
	wantPins := []classificationGoDefinitionQuerySourcePin{
		{Path: "src/testdata/native/queries/tsql.json", Bytes: 11934, SHA256: p.QuerySHA},
		{Path: "src/contracts/fact-query-pack.json", Bytes: 9702, SHA256: classificationCSharpFactQueryPackSHA},
	}
	if c.Cases != 1000 || c.Edits != 991 || c.CaseSteps != 1991 || c.TreeRoles != 2982 || c.Definitions != 8 || c.TreeRolesPerDefinition != 2982 || c.PotentialQueryTreeRoles != 23856 ||
		c.QuerySourceFileSHA256 != "" || !reflect.DeepEqual(c.QuerySourceFiles, wantPins) {
		t.Fatalf("TSQL8 ordinary denominator or source pins changed: %#v", c)
	}
	rows, err := classificationPagedDefinitionRows(p, f.anchors, f.inputs, f.witness, f.native, f.references)
	if err != nil || len(rows) != 8 {
		t.Fatalf("TSQL8 rows: %d %v", len(rows), err)
	}
	for i, row := range rows {
		wantFile := p.QueryFile
		if i >= 6 {
			wantFile = "src/contracts/fact-query-pack.json"
		}
		if row.Query != f.anchors.Queries[i].ID || row.SourceFile != wantFile || row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" ||
			row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != 2982 {
			t.Fatalf("TSQL8 row %d differs: %#v", i, row)
		}
	}
	for _, mutate := range []struct {
		name string
		edit func(*classificationCPP4RowsFixture)
	}{
		{"source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/postgresql-sql.json"
		}},
		{"fact-source-alias", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[7].SourceFile = p.QueryFile
		}},
		{"fact-order", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.queryFile.Queries[6], changed.inputs.queryFile.Queries[7] = changed.inputs.queryFile.Queries[7], changed.inputs.queryFile.Queries[6]
		}},
		{"declarations-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[6].Metadata.Predicates = nil }},
		{"ordinary-as-fact-source", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries[0].SourceFile = "src/contracts/fact-query-pack.json"
		}},
		{"witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[0].Metadata.Predicates = nil }},
		{"native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[0].Predicates = nil }},
		{"reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[0].Query.Predicates = nil }},
		{"fact-witness-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.witness[7].Metadata.Predicates = nil }},
		{"fact-native-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.native[7].Predicates = nil }},
		{"fact-reference-predicate-null", func(changed *classificationCPP4RowsFixture) { changed.references[7].Query.Predicates = nil }},
		{"witness-missing-query", func(changed *classificationCPP4RowsFixture) { changed.witness = changed.witness[:7] }},
		{"native-missing-query", func(changed *classificationCPP4RowsFixture) { changed.native = changed.native[:7] }},
		{"reference-missing-query", func(changed *classificationCPP4RowsFixture) { changed.references = changed.references[:7] }},
		{"slot-group-missing-query", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.slotGroup.Queries = changed.inputs.slotGroup.Queries[:7]
		}},
		{"raw-source-missing-fact", func(changed *classificationCPP4RowsFixture) {
			changed.inputs.queryFile.Queries = changed.inputs.queryFile.Queries[:7]
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := classificationCPP4CloneRowsFixture(f)
			mutate.edit(&changed)
			if _, err := classificationPagedDefinitionRows(p, changed.anchors, changed.inputs, changed.witness, changed.native, changed.references); err == nil {
				t.Fatal("TSQL8 definition row mutation accepted")
			}
		})
	}
}

func TestQueryClassificationTSQL8ProfileAndPagedContext(t *testing.T) {
	f := classificationTSQL8RowsTestFixture(t)
	a, plan := f.anchors, f.plan
	p := classificationTSQL8DefinitionProfile()
	if err := classificationPagedAnchorsValid(p, a); err != nil || a.nativeContext() != semanticPagedBinariesContext || a.nativeSymbol() != p.Symbol || a.libraryName() != "tsql.dll" || classificationNativePrefix(a) != "oracle/" {
		t.Fatalf("TSQL8 closed route binding differs: %v", err)
	}
	if !classificationPlanContextMatches(a, plan) {
		t.Fatal("closed TSQL8 paged context rejected")
	}
	// Four full 16 MiB parts and a short tail remain within the build-file ceiling.
	const page = 16 << 20
	if parser := p.Grammar[0].Bytes; parser <= page || parser > 100<<20 || (parser+page-1)/page != 5 || parser-4*page != 1538328 {
		t.Fatal("TSQL8 parser is not the expected five-part paged parser")
	}
	// The four paged profiles never accept each other's anchors, plans, or grammar closures.
	others := map[string]classificationCPP4Profile{"cpp4": classificationCPP4DefinitionProfile(), "kotlin5": classificationKotlin5DefinitionProfile(), "swift8": classificationSwift8DefinitionProfile(), "postgresql9": classificationPostgreSQL9DefinitionProfile()}
	for name, other := range others {
		otherPlan := plan
		otherPlan.AnchorsSHA256 = other.AnchorSHA
		swapped := a
		swapped.sha256 = other.AnchorSHA
		if classificationPagedAnchorsValid(other, a) == nil || classificationPagedGrammarValid(p, other.Grammar) || classificationPagedGrammarValid(other, plan.Grammar) ||
			classificationPlanContextMatches(swapped, otherPlan) || classificationPagedDefinitionPlanValid(other, a, plan) == nil {
			t.Fatalf("TSQL8 and the %s paged closure are interchangeable", name)
		}
	}
	unpaged := plan
	unpaged.NativeContext = "full-input-utf8/r2"
	if classificationPlanContextMatches(a, unpaged) || classificationPagedDefinitionPlanValid(p, a, unpaged) == nil {
		t.Fatal("TSQL8 accepted the unpaged context")
	}
	unpaged.NativeContext = semanticPagedContext
	if classificationPlanContextMatches(a, unpaged) || classificationPagedDefinitionPlanValid(p, a, unpaged) == nil {
		t.Fatal("TSQL8 accepted the parser-only context")
	}
	for _, mutate := range []func(*classificationPlan){
		func(changed *classificationPlan) { changed.Grammar[0].Bytes-- },
		func(changed *classificationPlan) { changed.Grammar[0].Path = "other/src/parser.c" },
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
			t.Fatal("TSQL8 plan mutation accepted")
		}
	}
	uniqueReferenceIDs := map[string]string{}
	for i, q := range a.Queries {
		uniqueReferenceIDs[q.ID] = "current-ref-" + string(rune('0'+i))
	}
	if ids, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err != nil || len(ids) != 8 {
		t.Fatalf("TSQL8 distinct references rejected: %v", err)
	}
	uniqueReferenceIDs[a.Queries[7].ID] = uniqueReferenceIDs[a.Queries[0].ID]
	if _, err := classificationPagedOrderedReferenceIDs(p, a, uniqueReferenceIDs); err == nil {
		t.Fatal("TSQL8 reused reference invocation accepted")
	}
	// The raw query file alone, another route's file, or an envelope with a missing, altered, or reordered fact pack must fail.
	queryFile := actualJSONRead(t, "../testdata/native/queries/tsql.json")
	factPack := actualJSONRead(t, "../contracts/fact-query-pack.json")
	for name, bad := range map[string][]byte{
		"raw-query-file-only":    queryFile,
		"other-route-raw-file":   actualJSONRead(t, "../testdata/native/queries/kotlin.json"),
		"fact-pack-missing":      classificationTSQL8Envelope(t, queryFile, nil),
		"query-file-missing":     classificationTSQL8Envelope(t, nil, factPack),
		"fact-pack-extra-byte":   classificationTSQL8Envelope(t, queryFile, append(slices.Clone(factPack), ' ')),
		"query-file-extra-byte":  classificationTSQL8Envelope(t, append(slices.Clone(queryFile), ' '), factPack),
		"swapped-envelope-files": classificationTSQL8Envelope(t, factPack, queryFile),
	} {
		if _, err := classificationPagedDefinitionLoadInputs(p, a, plan, f.inventoryData, f.qualificationData, bad); err == nil {
			t.Fatalf("TSQL8 query source mutation %s accepted", name)
		}
	}
	// The fact pack is pinned by hash, so route-level checks are exercised directly on altered packs of the same shape.
	facts, err := classificationPagedFactQueries(p, factPack)
	if err != nil || len(facts) != 2 || facts[0].ID != "facts.declarations" || facts[0].Source != a.Queries[6].Source || facts[1].ID != "facts.dynamic_sql" || facts[1].Source != a.Queries[7].Source {
		t.Fatalf("TSQL8 fact route rejected or differs: %v", err)
	}
	for _, name := range []string{"order", "missing", "duplicate-route"} {
		t.Run("fact-"+name, func(t *testing.T) {
			var pack classificationCSharpDefinitionFactQueryPack
			if err := jsonv2.Unmarshal(factPack, &pack); err != nil {
				t.Fatal(err)
			}
			for i := range pack.Routes {
				if pack.Routes[i].Route != "tsql" {
					continue
				}
				switch name {
				case "order":
					pack.Routes[i].Queries[0], pack.Routes[i].Queries[1] = pack.Routes[i].Queries[1], pack.Routes[i].Queries[0]
				case "missing":
					pack.Routes[i].Queries = pack.Routes[i].Queries[:1]
				case "duplicate-route":
					pack.Routes[(i+1)%len(pack.Routes)] = pack.Routes[i]
				}
				break
			}
			if _, err := classificationPagedFactQueries(p, eligibilityJSON(t, pack)); err == nil {
				t.Fatal("changed two-fact pack accepted")
			}
		})
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
		"route-missing": replaceNth(factPack, `"route": "tsql"`, 1, `"route": "tsqlx"`),
		"route-duplicated": replaceNth(replaceNth(replaceNth(factPack, `"route": "xml"`, 1, `"route": "tsql"`),
			`"id": "facts.xml_structure"`, 1, `"id": "facts.declarations"`), `"facts": "xml_structure"`, 1, `"facts": "declarations"`),
		"fact-id-changed":   replaceNth(factPack, `"id": "facts.declarations"`, 2, `"id": "facts.declaration"`),
		"facts-key-changed": replaceNth(factPack, `"facts": "declarations"`, 2, `"facts": "dynamic_sql"`),
		"revision-changed":  replaceNth(factPack, `fact-queries-r1`, 1, `fact-queries-r2`),
	} {
		if _, err := classificationPagedFactQueries(p, bad); err == nil {
			t.Fatalf("TSQL8 fact pack mutation %s accepted", name)
		}
	}
	// A profile without fact queries never accepts the TSQL envelope.
	t.Run("swift8-rejects-envelope", func(t *testing.T) {
		swift := classificationSwift8RowsTestFixture(t)
		if _, err := classificationPagedDefinitionLoadInputs(classificationSwift8DefinitionProfile(), swift.anchors, swift.plan, swift.inventoryData, swift.qualificationData, f.queryData); err == nil {
			t.Fatal("Swift8 accepted the TSQL8 query source envelope")
		}
	})
	grammarRoot, output := filepath.Join("prepared", "routes", "tsql"), filepath.Join("library", "tsql.dll")
	args, err := classificationGoCompilerArgs("tsql", grammarRoot, output, classificationTSQL8GrammarPins())
	if err != nil || !slices.Contains(args, filepath.Join(grammarRoot, "src", "scanner.c")) || args[6] != filepath.Join(grammarRoot, "src", "scanner.c") ||
		args[len(args)-2] != "-o" || args[len(args)-1] != output {
		t.Fatalf("TSQL8 scanner or compiler output closure differs: %v %v", args, err)
	}
	if _, err := classificationGoCompilerArgs("tsql", grammarRoot, output, classificationPostgreSQL9GrammarPins()); err == nil {
		t.Fatal("TSQL8 compiler argv accepted an unprefixed grammar closure")
	}
	for name, mutate := range map[string]func(*eligibilityProcessRequest){
		"compiler-path": func(request *eligibilityProcessRequest) { request.Path = "cc.exe" },
		"output":        func(request *eligibilityProcessRequest) { request.Args[len(request.Args)-1] += ".other" },
		"scanner-argv": func(request *eligibilityProcessRequest) {
			request.Args[6] = filepath.Join(grammarRoot, "src", "other.c")
		},
		"unprefixed-include": func(request *eligibilityProcessRequest) { request.Args[4] = filepath.Join(grammarRoot, "other", "src") },
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
			t.Fatalf("TSQL8 noncanonical compiler request %s accepted", name)
		}
	}
	badConsumerPlan := plan
	badConsumerPlan.LibraryRequest.Args = slices.Clone(plan.LibraryRequest.Args)
	badConsumerPlan.LibraryRequest.Args[len(badConsumerPlan.LibraryRequest.Args)-1] += ".preapproved-alias"
	if _, err := classificationPagedDefinitionLoadInputs(p, a, badConsumerPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
		t.Fatal("TSQL8 consumer accepted a noncanonical pre-fixed compiler output plan")
	}
	badABI := a
	badABI.ABI.SymbolCount--
	badFields := a
	badFields.ABI.FieldCount++
	badRoot := a
	badRoot.Nodes = slices.Clone(a.Nodes)
	badRoot.Nodes[0].Type = "source_file"
	badOrder := a
	badOrder.Queries = slices.Clone(a.Queries)
	badOrder.Queries[0], badOrder.Queries[1] = badOrder.Queries[1], badOrder.Queries[0]
	badSource := a
	badSource.Queries = slices.Clone(a.Queries)
	badSource.Queries[2].Source += " "
	badFactSource := a
	badFactSource.Queries = slices.Clone(a.Queries)
	badFactSource.Queries[7].Source += " "
	badPatterns := a
	badPatterns.Queries = slices.Clone(a.Queries)
	badPatterns.Queries[2].Metadata.Patterns++
	badCaptures := a
	badCaptures.Queries = slices.Clone(a.Queries)
	badCaptures.Queries[2].Metadata.CaptureNames = []string{"listen_channel", "notify_channel"}
	badCount := a
	badCount.Queries = slices.Clone(a.Queries[:7])
	badRequired := a
	badRequired.Required = slices.Clone(a.Required)
	badRequired.Required[0].Structural = nil
	for name, changed := range map[string]classificationAnchors{"abi-symbols": badABI, "abi-fields": badFields, "root": badRoot, "query-order": badOrder,
		"query-source": badSource, "fact-query-source": badFactSource, "patterns": badPatterns, "captures": badCaptures, "query-count": badCount, "required-null": badRequired} {
		if classificationPagedAnchorsValid(p, changed) == nil {
			t.Fatalf("changed TSQL8 %s anchor accepted", name)
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
				t.Fatal("changed TSQL8 source inventory input accepted")
			}
		})
	}
	if _, err := classificationPagedDefinitionProjection(p, a, plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
		t.Fatalf("ungrounded TSQL8 projection accepted: %v", err)
	}
}

func TestQueryClassificationTSQL8ClosedNames(t *testing.T) {
	if got, ok := classificationPagedProfileForName("tsql8-empty"); !ok || got.Route != "tsql" || !reflect.DeepEqual(got, classificationTSQL8DefinitionProfile()) {
		t.Fatal("tsql8-empty is not the closed TSQL8 paged profile")
	}
	if got, ok := classificationPagedProfileForRoute("tsql"); !ok || !reflect.DeepEqual(got, classificationTSQL8DefinitionProfile()) {
		t.Fatal("tsql is not the closed TSQL8 paged route")
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
	if !classificationGoDefinitionProfileAllowed("tsql8-empty") || !classificationGoLocalSkipsProfile("tsql8-empty") {
		t.Fatal("tsql8-empty is not admitted by the closed definition and local-skip entries")
	}
	for _, alias := range []string{"", "tsql", "tsql8", "tsql8-empty ", "TSQL8-empty", "tsql9-empty", "swift8"} {
		if _, ok := classificationPagedProfileForName(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if classificationGoDefinitionProfileAllowed(alias) {
			t.Fatalf("near-alias definition profile %q accepted", alias)
		}
	}
	for _, route := range []string{"", "TSQL", "tsql ", "mssql", "tsql8"} {
		if classificationPagedRoute(route) {
			t.Fatalf("route %q entered the closed paged set", route)
		}
	}
	if !classificationPagedRoute("tsql") {
		t.Fatal("tsql is not in the closed paged set")
	}
	if _, err := classificationCJRProfileForName("tsql8-empty"); err == nil {
		t.Fatal("TSQL8 was admitted by the CJR entry")
	}
	if _, ok := classificationCJRProfileForRoute("tsql"); ok {
		t.Fatal("PostgreSQL route entered the CJR map")
	}
	if _, ok := classificationEmptyProfileForRoute("tsql"); ok || classificationSmall17ProfileName("tsql8-empty") || classificationWeb16ProfileName("tsql8-empty") {
		t.Fatal("TSQL8 crossed into an unpaged empty-profile family")
	}
}

func TestQueryClassificationTSQL8StderrBinding(t *testing.T) {
	a := classificationAnchors{Route: "tsql"}
	if err := classificationCaptureStderr(a, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("TSQL8 missing zero-byte stderr member accepted")
	}
	empty := ReplayMember{Role: "stderr", Bytes: 0, SHA256: digestHex(nil)}
	read := func(pin *ReplayMember) ([]byte, error) {
		if pin.Role != empty.Role || pin.Bytes != empty.Bytes || pin.SHA256 != empty.SHA256 {
			return nil, fmt.Errorf("stderr pin changed")
		}
		return []byte{}, nil
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 0}, read); err != nil {
		t.Fatalf("TSQL8 explicit empty stderr rejected: %v", err)
	}
	if err := classificationCaptureStderr(a, semanticProcessCapture{Stderr: &empty}, eligibilityExecution{StderrBytes: 1}, read); err == nil {
		t.Fatal("TSQL8 stderr byte count mismatch accepted")
	}
}
