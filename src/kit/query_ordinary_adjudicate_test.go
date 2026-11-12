package kit

import (
	"bytes"
	"slices"
	"testing"

	jsonv2 "encoding/json/v2"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

func ordinaryAdjudicatorFixture(t *testing.T) (ordinaryTupleIdentity, nativewire.Tree, []byte, QueryWitnessMetadata, ordinaryExpectedTree, ordinaryExpectedCaptures) {
	t.Helper()
	source := []byte("ab")
	pin := OrdinaryPin{Path: "source-witness/" + digestHex(source) + ".utf8", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	id := ordinaryTupleIdentity{Case: "case1", Query: "q", Step: 0, TreeRole: "incremental", Source: pin}
	nodes := []TreeNode{
		{Parent: -1, Type: "root", Named: true, StartByte: 0, EndByte: 2, StartPoint: Point{}, EndPoint: Point{Column: 2}},
		{Parent: 0, Type: "identifier", Named: true, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}},
		{Parent: 0, Type: "identifier", Named: true, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
	}
	metadata := QueryWitnessMetadata{Patterns: 1, CaptureNames: []string{"x"}, Predicates: []QueryPredicate{}}
	rows := [][]int64{{0, 0, 0, 1, 0, 1, 0, 1, 0, 0, 0, 1}, {1, 0, 0, 2, 0, 1, 1, 2, 0, 1, 0, 2}}
	captures := []Capture{
		{Match: 0, Pattern: 0, Capture: 0, Name: "x", Node: 1, Type: "identifier", Named: true, StartByte: 0, EndByte: 1, StartPoint: Point{}, EndPoint: Point{Column: 1}},
		{Match: 1, Pattern: 0, Capture: 0, Name: "x", Node: 2, Type: "identifier", Named: true, StartByte: 1, EndByte: 2, StartPoint: Point{Column: 1}, EndPoint: Point{Column: 2}},
	}
	api := &nativewire.WireAPI{
		Revision: "tsgk-api/r1",
		Nodes: [][]int64{
			{-1, -1, -1, -1, -1, 1, 1, 2, 2, 3, 0, 0, 0},
			{0, -1, 2, -1, 2, -1, -1, 0, 0, 1, 1, 1, 1},
			{0, 1, -1, 1, -1, -1, -1, 0, 0, 1, 1, 2, 2},
		},
		ChildFields: [][]int64{}, FieldLookups: [][]int64{}, Points: [][]int64{}, Names: []string{},
	}
	actual := nativewire.Tree{Status: StatusCompleted, Form: "full", Nodes: nodes, API: api, Queries: []nativewire.WireQuery{{
		ID: "q", Status: StatusCompleted, Patterns: 1, CaptureNames: []string{"x"}, Predicates: []QueryPredicate{}, Matches: 2,
		Partial: false, Types: []string{"identifier"}, Captures: rows,
	}}}
	tree := ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: id.Case, Step: id.Step, TreeRole: id.TreeRole, Source: pin, Tree: slices.Clone(nodes)}
	ordered := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: id.Case, Query: id.Query, Step: id.Step, TreeRole: id.TreeRole, Source: pin, Matches: 2, Ordered: captures}
	return id, actual, source, metadata, tree, ordered
}

func ordinaryAdjudicateTest(t *testing.T, id ordinaryTupleIdentity, actual nativewire.Tree, source []byte, metadata QueryWitnessMetadata, tree ordinaryExpectedTree, captures ordinaryExpectedCaptures) ordinaryTupleJudgment {
	t.Helper()
	metadataData, err := jsonv2.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	treeData := ordinaryJSON(t, tree)
	captureData := ordinaryJSON(t, captures)
	judgment, e := adjudicateOrdinaryTuple(semanticTestBudget(t), id, actual, source, metadataData, treeData, captureData, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	return judgment
}

func TestOrdinaryAdjudicateStructuralTuple(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
	if got.State != ordinaryTupleVerified || got.Code != "" || len(got.Findings) != 0 {
		t.Fatalf("valid structural tuple: %+v", got)
	}
}

func TestOrdinaryAdjudicateMetadataFailureSurvivesFieldOnlyTreeBlock(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	field := "named"
	tree.Tree[1].Field = &field
	actual.Queries[0].Predicates = []QueryPredicate{{Pattern: 0, Steps: []QueryPredicateStep{}}}
	got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
	if got.State != ordinaryTupleFailed || got.Code != "ORDINARY_STRUCTURAL_PREDICATES" {
		t.Fatalf("metadata failure lost: %+v", got)
	}
	var metadataFail, treeBlocked bool
	for _, finding := range got.Findings {
		metadataFail = metadataFail || finding.Severity == "error" && finding.Code == "ORDINARY_STRUCTURAL_PREDICATES"
		treeBlocked = treeBlocked || finding.Severity == "warning" && finding.Code == "ORDINARY_EXPECTED_TREE_FIELD_DIFFERENCE"
	}
	if !metadataFail || !treeBlocked {
		t.Fatalf("expected independent metadata FAIL and CST field BLOCKED, got %+v", got.Findings)
	}
}

func TestOrdinaryAdjudicateNonFieldTreeMismatchFails(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	tree.Tree[1].Type = "field_identifier"
	for i := range captures.Ordered {
		captures.Ordered[i].Type = "field_identifier"
	}
	got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
	if got.State != ordinaryTupleFailed || !slices.ContainsFunc(got.Findings, func(f ordinaryTupleFinding) bool { return f.Code == "ORDINARY_EXPECTED_TREE_MISMATCH" }) {
		t.Fatalf("non-field CST mismatch was not a failure: %+v", got)
	}
}

func TestOrdinaryAdjudicateAPIContentMismatchFailsAfterShape(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	actual.API.Nodes[0][9]++
	got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
	if got.State != ordinaryTupleFailed || got.Code != "ORDINARY_API_MISMATCH" {
		t.Fatalf("shape-valid API mutation was accepted: %+v", got)
	}
}

func TestOrdinaryAdjudicateCaptureOrderAndDuplicatesAreExact(t *testing.T) {
	for _, mutate := range []string{"reverse", "duplicate"} {
		t.Run(mutate, func(t *testing.T) {
			id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
			switch mutate {
			case "reverse":
				captures.Ordered[0], captures.Ordered[1] = captures.Ordered[1], captures.Ordered[0]
			case "duplicate":
				actual.Queries[0].Captures = append(actual.Queries[0].Captures, actual.Queries[0].Captures[1])
			}
			got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
			if got.State != ordinaryTupleFailed || got.Code != "ORDINARY_CAPTURE_MISMATCH" && got.Code != "ORDINARY_CAPTURE_MATCHES_MISMATCH" && got.Code != "ORDINARY_ACTUAL_CAPTURE_INVALID" {
				t.Fatalf("ordered capture discrepancy was accepted: %+v", got)
			}
		})
	}
}

func TestOrdinaryExpectedCaptureNullIsNotEmpty(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	metadataData, _ := jsonv2.Marshal(metadata)
	treeData := ordinaryJSON(t, tree)
	captures.Ordered = nil
	captureData, err := jsonv2.Marshal(captures, jsonv2.FormatNilSliceAsNull(true))
	if err != nil || !bytes.Contains(captureData, []byte(`"ordered_captures":null`)) {
		t.Fatalf("could not form explicit null capture control: %v %s", err, captureData)
	}
	got, e := adjudicateOrdinaryTuple(semanticTestBudget(t), id, actual, source, metadataData, treeData, captureData, nil, nil)
	if e != nil || got.State != ordinaryTupleFailed || !slices.ContainsFunc(got.Findings, func(f ordinaryTupleFinding) bool {
		return f.Code == "JSON_NULL" || f.Code == "JSON_FIELD_SHAPE" || f.Code == "JSON_SHAPE"
	}) {
		t.Fatalf("null captures read as a complete empty stream: err=%v judgment=%+v", e, got)
	}
}

func ordinaryXMLFactInputs(t *testing.T, id ordinaryTupleIdentity) (ordinaryFactInputs, []byte) {
	t.Helper()
	pack := FactPack{Schema: FactPackSchema, Revision: "facts-r1", Mapping: "declaration-facts-r1", DynamicSQL: "dynamic-sql-r1", XML: "xml-structure-r1",
		Routes: []FactPackRoute{{Route: "go", Declarations: []NativeDeclaration{}, Queries: []FactQuery{{ID: "q", Facts: FactXMLStructure, Source: "(identifier) @x"}}}}}
	packData := ordinaryJSON(t, pack)
	parsed, err := ParseFactPack(packData)
	if err != nil {
		t.Fatal(err)
	}
	packPin := OrdinaryPin{Path: "facts/pack.json", Bytes: uint64(len(packData)), SHA256: digestHex(packData)}
	ref := FactPackRef{Revision: parsed.Revision, SHA256: packPin.SHA256, Route: "go"}
	mapping := []byte(`{"schema":"tsgk-fact-mapping/r1","revision":"declaration-facts-r1"}`)
	mappingPin := ordinaryFactMappingPin{Schema: "tsgk-fact-mapping/r1", Revision: "declaration-facts-r1", SHA256: digestHex(mapping)}
	queryPin := ordinaryFactQueryPin{ID: "q", SHA256: digestHex([]byte(pack.Routes[0].Queries[0].Source)), Kind: FactXMLStructure}
	wantXML := map[string]int{"x": 2}
	expected := ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: id.Case, Query: id.Query, Step: id.Step, TreeRole: id.TreeRole, Source: id.Source,
		Pack: ref, Mapping: mappingPin, FactQuery: queryPin, Status: StatusCompleted, Code: "", XML: &wantXML}
	producer := ordinaryProducerFactRecord{Schema: "tsgk-ordinary-step-facts/r1", Case: id.Case, Pack: ref, Mapping: mappingPin,
		DynamicSQLRevision: pack.DynamicSQL, XMLRevision: pack.XML, FactQueries: []ordinaryFactQueryPin{queryPin}, FactSuffixSHA256: ordinaryFactSuffixDigest([]ordinaryFactQueryPin{queryPin}),
		Steps: []ordinaryProducerFactStep{{Step: int(id.Step), TreeRole: id.TreeRole, SourceBytes: id.Source.Bytes, SourceSHA256: id.Source.SHA256, Status: StatusCompleted,
			Queries: []ordinaryProducerFactQuery{{ID: "q", SHA256: queryPin.SHA256, Kind: FactXMLStructure, Status: StatusCompleted, XML: map[string]int{"x": 2}}}}}}
	producerData := ordinaryJSON(t, producer)
	inputs := ordinaryFactInputs{Pack: parsed, PackData: packData, PackPin: packPin, PackRef: ref, Mapping: mapping, MappingPin: mappingPin, ProducerByCase: map[string][]byte{id.Case: producerData}, Ready: true}
	return inputs, ordinaryJSON(t, expected)
}

func TestOrdinaryAdjudicateRecomputesIndependentFactsAndProducerPayload(t *testing.T) {
	id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
	facts, factData := ordinaryXMLFactInputs(t, id)
	judgment, err := adjudicateOrdinaryTuple(semanticTestBudget(t), id, actual, source, jsonv2MustMarshal(t, metadata), ordinaryJSON(t, tree), ordinaryJSON(t, captures), factData, &facts)
	if err != nil || judgment.State != ordinaryTupleVerified {
		t.Fatalf("matching independent facts: err=%v judgment=%+v", err, judgment)
	}
	originalProducer := append([]byte(nil), facts.ProducerByCase[id.Case]...)
	var producer ordinaryProducerFactRecord
	if err := jsonv2.Unmarshal(facts.ProducerByCase[id.Case], &producer); err != nil {
		t.Fatal(err)
	}
	producer.Steps[0].Queries[0].XML = map[string]int{"x": 1}
	facts.ProducerByCase[id.Case] = ordinaryJSON(t, producer)
	judgment, err = adjudicateOrdinaryTuple(semanticTestBudget(t), id, actual, source, jsonv2MustMarshal(t, metadata), ordinaryJSON(t, tree), ordinaryJSON(t, captures), factData, &facts)
	if err != nil || judgment.State != ordinaryTupleFailed || judgment.Code != "ORDINARY_PRODUCER_FACT_MISMATCH" {
		t.Fatalf("rehashed producer fact payload was accepted: err=%v judgment=%+v", err, judgment)
	}

	facts.ProducerByCase[id.Case] = originalProducer
	var expected ordinaryExpectedFact
	if err := jsonv2.Unmarshal(factData, &expected); err != nil {
		t.Fatal(err)
	}
	expected.Status, expected.Code = StatusFailed, "EXPECTED_FACT_FAILURE"
	var producerRecord ordinaryProducerFactRecord
	if err := jsonv2.Unmarshal(originalProducer, &producerRecord); err != nil {
		t.Fatal(err)
	}
	producerRecord.Steps[0].Queries[0].Status, producerRecord.Steps[0].Queries[0].Code = StatusFailed, expected.Code
	facts.ProducerByCase[id.Case] = ordinaryJSON(t, producerRecord)
	judgment, err = adjudicateOrdinaryTuple(semanticTestBudget(t), id, actual, source, jsonv2MustMarshal(t, metadata), ordinaryJSON(t, tree), ordinaryJSON(t, captures), ordinaryJSON(t, expected), &facts)
	if err != nil || judgment.State != ordinaryTupleFailed || judgment.Code != expected.Code {
		t.Fatalf("independent expected fact failure was lost: err=%v judgment=%+v", err, judgment)
	}
}

func jsonv2MustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := jsonv2.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
