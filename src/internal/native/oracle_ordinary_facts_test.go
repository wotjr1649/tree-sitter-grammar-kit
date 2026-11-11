package native

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestOrdinaryStepFactsUsesEditedVersionAndIndependentFreshCaptures(t *testing.T) {
	packBytes, err := os.ReadFile("../../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := os.ReadFile("../../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := kit.ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	route := slices.IndexFunc(pack.Routes, func(r kit.FactPackRoute) bool { return r.Route == "tsql" })
	if route < 0 {
		t.Fatal("tsql fact route missing")
	}
	queries := []kit.OracleQuery{{ID: "q.general", Source: "(source_file) @source"}}
	for _, q := range pack.Routes[route].Queries {
		queries = append(queries, kit.OracleQuery{ID: q.ID, Source: q.Source})
	}
	initial := []byte("EXEC('a')")
	edits := []kit.Edit{
		{StartByte: 5, OldEndByte: 8, NewEndByte: 7, Old: []byte("'a'"), New: []byte("@x")},
		{StartByte: 8, OldEndByte: 8, NewEndByte: 9, Old: []byte{}, New: []byte(";")},
	}
	versions, _, err := kit.ApplyEdits(kit.EncodingUTF8, initial, edits, 1024)
	if err != nil {
		t.Fatal(err)
	}
	profile := kit.OracleProfile{
		FactPack:    &kit.FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: "tsql"},
		Queries:     queries,
		OracleCases: []kit.OracleCase{{IncrementalCase: kit.IncrementalCase{ID: "edit-two", Encoding: kit.EncodingUTF8, Edits: edits}}},
	}
	mappingPin, factPins, suffixSHA, err := validateOrdinaryFactPins(profile, &pack, mapping)
	if err != nil || mappingPin.SHA256 != sha(mapping) || len(factPins) != 2 || suffixSHA == "" {
		t.Fatalf("ordinary fact preflight: %+v %+v %s %v", mappingPin, factPins, suffixSHA, err)
	}
	queryRows := func(dynamic []kit.Capture) []QueryOut {
		rows := []QueryOut{{ID: "q.general", SHA256: sha([]byte(queries[0].Source)), Status: kit.StatusCompleted, Evaluation: EvalStructural, Captures: []kit.Capture{}}}
		for _, q := range pack.Routes[route].Queries {
			captures := []kit.Capture{}
			if q.Facts == kit.FactDynamicSQL {
				captures = dynamic
			}
			rows = append(rows, QueryOut{ID: q.ID, SHA256: sha([]byte(q.Source)), Status: kit.StatusCompleted, Evaluation: EvalStructural, Captures: captures})
		}
		return rows
	}
	literal := ordinaryExecCapture("literal", 5, 8)
	variable := ordinaryExecCapture("variable", 5, 7)
	steps := []StepResult{
		{Step: 0, SourceBytes: uint64(len(versions[0])), SourceSHA256: sha(versions[0]), Incremental: &TreeOut{Status: kit.StatusCompleted, Queries: queryRows(literal), Summary: ordinaryEmptyDeclarationSummary()}},
		{Step: 1, SourceBytes: uint64(len(versions[1])), SourceSHA256: sha(versions[1]), Incremental: &TreeOut{Status: kit.StatusCompleted, Queries: queryRows(variable), Summary: ordinaryEmptyDeclarationSummary()},
			Fresh: &TreeOut{Status: kit.StatusCompleted, Queries: queryRows([]kit.Capture{}), Summary: ordinaryEmptyDeclarationSummary()}},
		{Step: 2, SourceBytes: uint64(len(versions[2])), SourceSHA256: sha(versions[2]), Incremental: &TreeOut{Status: kit.StatusCompleted, Queries: queryRows(variable), Summary: ordinaryEmptyDeclarationSummary()},
			Fresh: &TreeOut{Status: kit.StatusCompleted, Queries: queryRows([]kit.Capture{}), Summary: ordinaryEmptyDeclarationSummary()}},
	}
	cr := &CaseResult{ID: "edit-two", ExecutionStatus: kit.StatusCompleted, Oracle: &OracleClaims{}, Steps: steps}
	x := Context{Op: kit.NativeOperation{InputBytes: 1024}}
	got, err := ordinaryStepFacts(cr, profile, &pack, x, initial, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != ordinaryStepFactsSchema || got.Mapping.SHA256 != sha(mapping) || got.FactSuffixSHA256 == "" {
		t.Fatalf("ordinary fact pins missing: %+v", got)
	}
	if !slices.Equal([]string{got.FactQueries[0].ID, got.FactQueries[1].ID}, []string{"facts.declarations", "facts.dynamic_sql"}) {
		t.Fatalf("fact suffix order: %+v", got.FactQueries)
	}
	stepZero := ordinaryFactsAt(t, got, 0, "incremental")
	if stepZero.Queries[1].DynamicSQL == nil || len(stepZero.Queries[1].DynamicSQL.Items) != 1 || stepZero.Queries[1].DynamicSQL.Items[0].ArgumentKind != "literal_char" {
		t.Fatalf("step 0 facts: %+v", stepZero.Queries[1])
	}
	for step := 1; step <= 2; step++ {
		incremental := ordinaryFactsAt(t, got, step, "incremental")
		fresh := ordinaryFactsAt(t, got, step, "fresh")
		if incremental.SourceSHA256 != sha(versions[step]) || incremental.SourceBytes != uint64(len(versions[step])) ||
			incremental.Queries[1].DynamicSQL == nil || len(incremental.Queries[1].DynamicSQL.Items) != 1 ||
			incremental.Queries[1].DynamicSQL.Items[0].ArgumentKind != "variable" || incremental.Queries[1].DynamicSQL.Items[0].Variable == nil || *incremental.Queries[1].DynamicSQL.Items[0].Variable != "@x" {
			t.Fatalf("step %d incremental facts were not derived from its source version: %+v", step, incremental)
		}
		if fresh.Queries[1].DynamicSQL == nil || len(fresh.Queries[1].DynamicSQL.Items) != 0 || fresh.Queries[1].Status != kit.StatusCompleted {
			t.Fatalf("step %d fresh facts copied incremental captures: %+v", step, fresh.Queries[1])
		}
	}
	failedBase := *cr
	failedBase.Steps = slices.Clone(cr.Steps)
	failedTree := *cr.Steps[1].Incremental
	failedSummary := *failedTree.Summary
	failedSummary.Declarations.Items = []DeclItem{{Fact: "type_declaration", NodeType: "class_declaration", Status: "PASS"}}
	failedTree.Summary = &failedSummary
	failedBase.Steps[1].Incremental = &failedTree
	failedFacts, err := ordinaryStepFacts(&failedBase, profile, &pack, x, initial, mapping)
	if err != nil {
		t.Fatal(err)
	}
	failedRole := ordinaryFactsAt(t, failedFacts, 1, "incremental")
	if failedRole.Status != kit.StatusFailed || failedRole.Queries[0].Code != "FACT_REPRODUCTION_MISMATCH" || failedRole.Queries[1].Status != kit.StatusCompleted || len(failedRole.Queries[1].DynamicSQL.Items) != 1 {
		t.Fatalf("declaration mismatch hid the later dynamic SQL derivation: %+v", failedRole)
	}
	initialMutant, err := kit.DeriveDynamicSQLChecked("tsql", kit.EncodingUTF8, variable, initial)
	if err != nil || len(initialMutant.Items) != 1 || initialMutant.Items[0].ArgumentKind != "bare_word" {
		t.Fatalf("initial-source mutant did not produce a distinguishing result: %+v, %v", initialMutant, err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var stored OrdinaryStepFacts
	if err := json.Unmarshal(encoded, &stored); err != nil || stored.FactSuffixSHA256 != got.FactSuffixSHA256 || !slices.Equal(stored.FactQueries, got.FactQueries) {
		t.Fatalf("stored ordered fact pins changed: %v", err)
	}
}

func TestOrdinaryStepFactsRejectsChangedPinsAndFactSuffix(t *testing.T) {
	packBytes, err := os.ReadFile("../../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := os.ReadFile("../../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := kit.ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	route := slices.IndexFunc(pack.Routes, func(r kit.FactPackRoute) bool { return r.Route == "tsql" })
	queries := []kit.OracleQuery{{ID: "q.general", Source: "(source_file) @source"}}
	for _, q := range pack.Routes[route].Queries {
		queries = append(queries, kit.OracleQuery{ID: q.ID, Source: q.Source})
	}
	profile := kit.OracleProfile{FactPack: &kit.FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: "tsql"}, Queries: queries,
		OracleCases: []kit.OracleCase{{IncrementalCase: kit.IncrementalCase{ID: "one", Encoding: kit.EncodingUTF8}}}}
	check := func(p kit.OracleProfile, factPack *kit.FactPack, mapBytes []byte) bool {
		_, _, _, err := validateOrdinaryFactPins(p, factPack, mapBytes)
		return err != nil
	}
	if check(profile, &pack, mapping) {
		t.Fatal("valid fact pins rejected")
	}
	mutated := profile
	mutated.Queries = slices.Clone(profile.Queries)
	mutated.Queries[len(mutated.Queries)-2], mutated.Queries[len(mutated.Queries)-1] = mutated.Queries[len(mutated.Queries)-1], mutated.Queries[len(mutated.Queries)-2]
	if !check(mutated, &pack, mapping) {
		t.Fatal("reordered fact suffix accepted")
	}
	mutated = profile
	mutated.Queries = slices.Clone(profile.Queries)
	mutated.Queries[len(mutated.Queries)-1].Source += " "
	if !check(mutated, &pack, mapping) {
		t.Fatal("changed fact query source accepted")
	}
	mutated = profile
	mutated.FactPack = &kit.FactPackRef{Revision: pack.Revision, SHA256: "different", Route: "tsql"}
	if !check(mutated, &pack, mapping) {
		t.Fatal("changed fact pack hash accepted")
	}
	badMapping := []byte(`{"schema":"tsgk-fact-mapping/r1","revision":"different"}`)
	if !check(profile, &pack, badMapping) {
		t.Fatal("changed fact mapping revision accepted")
	}
}

func ordinaryFactsAt(t *testing.T, facts OrdinaryStepFacts, step int, role string) OrdinaryTreeRoleFacts {
	t.Helper()
	for _, got := range facts.Steps {
		if got.Step == step && got.TreeRole == role {
			return got
		}
	}
	t.Fatalf("missing step %d %s facts", step, role)
	return OrdinaryTreeRoleFacts{}
}

func ordinaryEmptyDeclarationSummary() *SummaryEnvelope {
	return &SummaryEnvelope{Declarations: SummaryDecls{Items: []DeclItem{}}}
}

func ordinaryExecCapture(argument string, start, end uint32) []kit.Capture {
	argType := "literal"
	if argument == "variable" {
		argType = "identifier"
	}
	return []kit.Capture{
		{Match: 0, Name: "exec", Node: 1, Type: "EXEC", Named: true, StartByte: 0, EndByte: 8},
		{Match: 1, Name: "exec.parent", Node: 1, Type: "exec", Named: true, StartByte: 0, EndByte: 8},
		{Match: 1, Name: "exec.child", Node: 2, Type: "(", StartByte: 4, EndByte: 5},
		{Match: 1, Name: "exec.child", Node: 3, Type: argType, Named: true, StartByte: start, EndByte: end},
	}
}
