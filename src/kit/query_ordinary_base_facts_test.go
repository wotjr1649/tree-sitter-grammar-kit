package kit

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func ordinaryBaseFactFixture(t *testing.T) (OracleProfile, *ordinaryCaseCache, ordinaryBaseProjection, *ordinaryFactInputs) {
	t.Helper()
	pack := FactPack{Schema: FactPackSchema, Revision: "facts-r1", Mapping: "mapping-r1", DynamicSQL: "dynamic-sql-r1", XML: "xml-structure-r1",
		Routes: []FactPackRoute{{Route: "tsql", Declarations: []NativeDeclaration{}, Queries: []FactQuery{
			{ID: "decl", Facts: FactDeclarations, Source: "(root) @c.0.0.0"}, {ID: "dynamic", Facts: FactDynamicSQL, Source: "(root) @exec.arg"},
		}}}}
	packData := ordinaryJSON(t, pack)
	pack.SHA256 = digestHex(packData)
	ref := FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: "tsql"}
	mapping := []byte(`{"schema":"tsgk-fact-mapping/r1","revision":"mapping-r1"}`)
	inputs := &ordinaryFactInputs{Pack: pack, PackRef: ref, PackData: packData, PackPin: OrdinaryPin{Path: "pack.json", Bytes: uint64(len(packData)), SHA256: pack.SHA256},
		Mapping: mapping, MappingPin: ordinaryFactMappingPin{Schema: "tsgk-fact-mapping/r1", Revision: "mapping-r1", SHA256: digestHex(mapping)}, Ready: true}
	source := []byte("EXEC('a')")
	versions, _, err := ApplyEdits(EncodingUTF8, source, []Edit{{StartByte: 5, OldEndByte: 8, NewEndByte: 7, Old: []byte("'a'"), New: []byte("@x")}}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	profile := OracleProfile{FactPack: &ref, Native: IncrementalProfile{Encoding: EncodingUTF8}, OracleCases: []OracleCase{{}}}
	for _, q := range pack.Routes[0].Queries {
		profile.Queries = append(profile.Queries, OracleQuery{ID: q.ID, Source: q.Source})
	}
	pins := ordinaryPackFactQueryPins(pack, ref.Route)
	record := ordinaryProducerFactRecord{Schema: "tsgk-ordinary-step-facts/r1", Case: "c1", Pack: ref, Mapping: inputs.MappingPin,
		DynamicSQLRevision: pack.DynamicSQL, XMLRevision: pack.XML, FactQueries: pins, FactSuffixSHA256: ordinaryFactSuffixDigest(pins)}
	empty := []nativewire.DeclItem{}
	cache := &ordinaryCaseCache{Case: "c1", raw: nativewire.Response{Status: StatusCompleted}, versions: versions, checked: make([]nativewire.CheckedStep, len(versions)), ready: true}
	var first ordinaryBaseFacts
	for step, version := range versions {
		for _, role := range []string{"incremental", "fresh"} {
			if step == 0 && role == "fresh" {
				continue
			}
			end := int64(8)
			if step > 0 {
				end = 7
			}
			argType := "literal"
			if step > 0 {
				argType = "identifier"
			}
			rows := [][]int64{
				{0, 0, 0, 1, 0, 1, 0, 8, 0, 0, 0, 8},
				{1, 0, 1, 1, 1, 1, 0, 8, 0, 0, 0, 8},
				{1, 0, 2, 2, 2, 0, 4, 5, 0, 4, 0, 5},
				{1, 0, 2, 3, 3, 1, 5, end, 0, 5, 0, end},
			}
			if role == "fresh" {
				rows = [][]int64{}
			}
			queries := []querywire.Query{
				{ID: "decl", Status: StatusCompleted, Patterns: 1, CaptureNames: []string{}, Predicates: []querypolicy.Predicate{}, Types: []string{}, Captures: [][]int64{}},
				{ID: "dynamic", Status: StatusCompleted, Patterns: 1, CaptureNames: []string{"exec", "exec.parent", "exec.child"}, Predicates: []querypolicy.Predicate{}, Types: []string{"EXEC", "exec", "(", argType}, Captures: rows},
			}
			tree := nativewire.Tree{Status: StatusCompleted, Form: "full", Queries: queries, Wire: nativewire.WireTree{Declarations: &empty}}
			if role == "incremental" {
				cache.checked[step].Incremental = tree
			} else {
				cache.checked[step].Fresh = &tree
			}
			captures := []Capture{}
			for _, row := range rows {
				captures = append(captures, witnessCapture(queries[1], row))
			}
			facts, err := DeriveDynamicSQLChecked("tsql", EncodingUTF8, captures, version)
			if err != nil {
				t.Fatal(err)
			}
			decl := &ordinaryProducerDeclFacts{Items: []DeclarationItem{}, Assessment: "NOT_APPLICABLE", Reproduces: true}
			record.Steps = append(record.Steps, ordinaryProducerFactStep{Step: step, TreeRole: role, SourceBytes: uint64(len(version)), SourceSHA256: digestHex(version), Status: StatusCompleted,
				Queries: []ordinaryProducerFactQuery{{ID: "decl", SHA256: pins[0].SHA256, Kind: FactDeclarations, Status: StatusCompleted, Declarations: decl},
					{ID: "dynamic", SHA256: pins[1].SHA256, Kind: FactDynamicSQL, Status: StatusCompleted, DynamicSQL: &facts}}})
			if role == "incremental" && step > 0 && (len(facts.Items) != 1 || facts.Items[0].Variable == nil || *facts.Items[0].Variable != "@x") {
				t.Fatalf("edited fact missing: %+v", facts)
			}
			if step == 0 {
				first = ordinaryBaseFacts{Pack: ref, Declarations: decl, DynamicSQL: &facts}
				profile.OracleCases[0].DynamicSQL = &DynamicSQLExpectation{Facts: slices.Clone(facts.Items), KnownMisses: []ByteSpan{}}
			}
		}
	}
	cache.stepFacts = ordinaryJSON(t, record)
	return profile, cache, ordinaryBaseProjection{Facts: ordinaryJSON(t, first)}, inputs
}

func TestOrdinaryBaseFactsRecomputeEveryStepAndRole(t *testing.T) {
	profile, cache, projection, inputs := ordinaryBaseFactFixture(t)
	claims := map[string]string{}
	e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, inputs, claims, func(code, at, detail string) { t.Errorf("%s %s %s", code, at, detail) })
	if e != nil || claims["fact-reproduction"] != claimPass || claims["dynamic-sql"] != claimPass {
		t.Fatalf("recomputed claims=%v err=%v", claims, e)
	}
}

func TestOrdinaryBaseFactsKeepStepZeroClaimScope(t *testing.T) {
	profile, cache, projection, inputs := ordinaryBaseFactFixture(t)
	cache.checked[1].Fresh = nil
	var record ordinaryProducerFactRecord
	if err := json.Unmarshal(cache.stepFacts, &record); err != nil {
		t.Fatal(err)
	}
	step := &record.Steps[2]
	step.Status, step.Code = StatusNotRun, "FACT_TREE_UNAVAILABLE"
	for i := range step.Queries {
		step.Queries[i].Status, step.Queries[i].Code = StatusNotRun, step.Code
		step.Queries[i].Declarations, step.Queries[i].DynamicSQL = nil, nil
	}
	cache.stepFacts = ordinaryJSON(t, record)
	claims := map[string]string{}
	e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, inputs, claims, func(code, at, detail string) { t.Errorf("%s %s %s", code, at, detail) })
	if e != nil || claims["fact-reproduction"] != claimPass || claims["dynamic-sql"] != claimPass {
		t.Fatalf("later unavailable role changed legacy step0 claims: %v %v", e, claims)
	}
}

func TestOrdinaryBaseFactsPartialRawKeepsUnclaimedFacts(t *testing.T) {
	profile, cache, projection, inputs := ordinaryBaseFactFixture(t)
	cache.raw.Status = StatusResourceLimit
	cache.ready = false
	cache.checked[1].Fresh = nil
	projection.Facts = nil
	claims := map[string]string{}
	e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, inputs, claims, func(code, at, detail string) { t.Errorf("%s %s %s", code, at, detail) })
	if e != nil || len(claims) != 0 {
		t.Fatalf("partial producer without a base facts projection gained case claims: %v %v", e, claims)
	}
}

func TestOrdinaryBaseFactsIncompleteQueryKeepsDynamicUnclaimed(t *testing.T) {
	profile, cache, projection, inputs := ordinaryBaseFactFixture(t)
	cache.checked[0].Incremental.Queries[1].Status = "INVALID_QUERY"
	cache.checked[0].Incremental.Queries[1].Captures = nil
	var record ordinaryProducerFactRecord
	var first ordinaryBaseFacts
	if err := json.Unmarshal(cache.stepFacts, &record); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(projection.Facts, &first); err != nil {
		t.Fatal(err)
	}
	record.Steps[0].Status, record.Steps[0].Code = StatusNotRun, "FACT_QUERY_NOT_COMPLETE_STRUCTURAL"
	query := &record.Steps[0].Queries[1]
	query.Status, query.Code, query.DynamicSQL = StatusNotRun, record.Steps[0].Code, nil
	cache.stepFacts = ordinaryJSON(t, record)
	first.DynamicSQL = nil
	projection.Facts = ordinaryJSON(t, first)
	claims := map[string]string{}
	e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, inputs, claims, func(code, at, detail string) { t.Errorf("%s %s %s", code, at, detail) })
	if e != nil || claims["fact-reproduction"] != claimBlocked || claims["dynamic-sql"] != claimNotClaimed {
		t.Fatalf("incomplete query changed legacy claim scope: %v %v", e, claims)
	}
}

func TestOrdinaryBaseFactsRecomputeXML(t *testing.T) {
	id, tree, source, _, _, _ := ordinaryAdjudicatorFixture(t)
	inputs, _ := ordinaryXMLFactInputs(t, id)
	profile := OracleProfile{FactPack: &inputs.PackRef, Native: IncrementalProfile{Encoding: EncodingUTF8}, OracleCases: []OracleCase{{}},
		Queries: []OracleQuery{{ID: "q", Source: inputs.Pack.Routes[0].Queries[0].Source}}}
	cache := &ordinaryCaseCache{Case: id.Case, raw: nativewire.Response{Status: StatusCompleted}, versions: [][]byte{source}, checked: []nativewire.CheckedStep{{Incremental: tree}}, stepFacts: inputs.ProducerByCase[id.Case]}
	projection := ordinaryBaseProjection{Facts: ordinaryJSON(t, ordinaryBaseFacts{Pack: inputs.PackRef, XML: map[string]int{"x": 2}})}
	claims := map[string]string{}
	e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, &inputs, claims, func(code, at, detail string) { t.Errorf("%s %s %s", code, at, detail) })
	if e != nil || claims["fact-reproduction"] != claimNotClaimed || claims["dynamic-sql"] != claimNotClaimed {
		t.Fatalf("XML facts changed non-declaration case claims: %v %v", e, claims)
	}
}

func TestOrdinaryBaseFactsRejectTampering(t *testing.T) {
	for _, mutation := range []string{"initial-source", "fresh-copy", "pack", "mapping", "suffix-order", "suffix-hash", "query-hash", "stored-fact", "projection-fact", "missing-role", "duplicate-role", "role-status", "profile-suffix"} {
		t.Run(mutation, func(t *testing.T) {
			profile, cache, projection, inputs := ordinaryBaseFactFixture(t)
			var record ordinaryProducerFactRecord
			if err := json.Unmarshal(cache.stepFacts, &record); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "initial-source":
				q := cache.checked[1].Incremental.Queries[1]
				facts, err := DeriveDynamicSQLChecked("tsql", EncodingUTF8, func() []Capture {
					out := []Capture{}
					for _, row := range q.Captures {
						out = append(out, witnessCapture(q, row))
					}
					return out
				}(), cache.versions[0])
				if err != nil || len(facts.Items) != 1 || facts.Items[0].ArgumentKind != "bare_word" {
					t.Fatalf("initial source must distinguish the mutation: %v %+v", err, facts)
				}
				record.Steps[1].Queries[1].DynamicSQL = &facts
			case "fresh-copy":
				record.Steps[2].Queries = record.Steps[1].Queries
			case "pack":
				record.Pack.SHA256 = digestHex([]byte("other pack"))
			case "mapping":
				record.Mapping.SHA256 = digestHex([]byte("other mapping"))
			case "suffix-order":
				slices.Reverse(record.FactQueries)
				record.FactSuffixSHA256 = ordinaryFactSuffixDigest(record.FactQueries)
			case "suffix-hash":
				record.FactSuffixSHA256 = digestHex([]byte("other suffix"))
			case "query-hash":
				record.Steps[1].Queries[1].SHA256 = digestHex([]byte("other query"))
			case "stored-fact":
				record.Steps[1].Queries[1].DynamicSQL.Items[0].ArgumentKind = "literal_char"
			case "projection-fact":
				var first ordinaryBaseFacts
				if err := json.Unmarshal(projection.Facts, &first); err != nil {
					t.Fatal(err)
				}
				first.Declarations.Reproduces = false
				projection.Facts = ordinaryJSON(t, first)
			case "missing-role":
				record.Steps = record.Steps[:2]
			case "duplicate-role":
				record.Steps[2] = record.Steps[1]
			case "role-status":
				record.Steps[2].Status = StatusFailed
			case "profile-suffix":
				slices.Reverse(profile.Queries)
			}
			cache.stepFacts = ordinaryJSON(t, record)
			findings := []string{}
			e := semanticTestBudget(t).ordinaryCompareBaseFacts(profile, cache, projection, inputs, map[string]string{}, func(code, _, _ string) { findings = append(findings, code) })
			if e == nil && len(findings) == 0 {
				t.Fatal("tampered fact accepted without a failure")
			}
		})
	}
}
