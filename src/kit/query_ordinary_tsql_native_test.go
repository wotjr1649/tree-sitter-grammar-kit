package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// This file compares the independent TSQL expectations with the actual owned-native r8 one
// tuple at a time through the product's tuple adjudicator. It exists because the public pass
// of the selected TSQL shard stops at its reference-row stage on the unchanged logical limit
// (see docs): the adjudicator itself is the same function the public pass calls at stage
// independent-tuples, but it runs here with its own fresh budget.

type ordinaryTSQLTuple struct {
	Query string
	Step  uint32
	Role  string
}

func (k ordinaryTSQLTuple) String() string { return fmt.Sprintf("%s/s%d/%s", k.Query, k.Step, k.Role) }

// ordinaryTSQLJudgeR8 adjudicates every (query, step, role) tuple of the selected case against
// the native raw response of an owned producer run, with expectation bodies from assets.
func ordinaryTSQLJudgeR8(t *testing.T, produced ordinarySQLProduced, bodies map[string][]byte) (map[ordinaryTSQLTuple]ordinaryTupleJudgment, []ordinaryTSQLTuple) {
	t.Helper()
	profile, err := ParseOracleProfile(produced.Req.Profile)
	if err != nil || len(profile.Native.Cases) != 1 || profile.FactPack == nil {
		t.Fatalf("executable profile: err=%v", err)
	}
	caseProfile := profile.Native.Cases[0]
	r8 := filepath.Join(produced.ProducerDir, "r8")
	read := func(rel string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(r8, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	raw := read("raw/00000-" + caseProfile.ID + ".json")
	response, err := nativewire.DecodeResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	versions, points, err := ApplyEdits(EncodingUTF8, produced.Source, caseProfile.Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	var observations []struct {
		RunID          string `json:"run_id"`
		ProducerSHA256 string `json:"producer_sha256"`
		HistoryStart   uint64 `json:"history_start"`
	}
	if err := json.Unmarshal(response.Steps[0].Observations, &observations); err != nil || len(observations) == 0 {
		t.Fatalf("native observations: %v", err)
	}
	expect := nativewire.Expect{Revision: nativewire.ProtocolR4, ID: caseProfile.ID, Nodes: 100000, API: profile.API, SourceBytes: uint64(len(produced.Source)), EditPoints: points,
		Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: observations[0].RunID, ProducerSHA256: observations[0].ProducerSHA256, HistoryStart: observations[0].HistoryStart}}
	for _, query := range profile.Queries {
		expect.Queries = append(expect.Queries, querywire.Source{ID: query.ID, Source: []byte(query.Source)})
	}
	for _, point := range caseProfile.Points {
		expect.Points = append(expect.Points, point.Byte)
	}
	for _, edit := range caseProfile.Edits {
		expect.Edits = append(expect.Edits, nativewire.EditSpan{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte})
	}
	for _, version := range versions {
		expect.Sources = append(expect.Sources, nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)})
	}
	exit := 0
	expect.Exit = &exit
	checked, err := nativewire.Check(response, expect)
	if err != nil || len(checked.Steps) != len(versions) {
		t.Fatalf("native response does not check: %v (steps %d)", err, len(checked.Steps))
	}
	var record struct {
		StepFacts json.RawMessage `json:"step_facts"`
	}
	if err := json.Unmarshal(read("records/00000-"+caseProfile.ID+".json"), &record); err != nil || len(record.StepFacts) == 0 {
		t.Fatalf("producer record: %v", err)
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	mappingBytes, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var mapping struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(mappingBytes, &mapping); err != nil {
		t.Fatal(err)
	}
	facts := ordinaryFactInputs{Pack: pack, PackData: packBytes, PackRef: *profile.FactPack, PackPin: OrdinaryPin{Path: "facts/pack.json", Bytes: uint64(len(packBytes)), SHA256: digestHex(packBytes)},
		Mapping: mappingBytes, MappingPin: ordinaryFactMappingPin{Schema: mapping.Schema, Revision: mapping.Revision, SHA256: digestHex(mappingBytes)},
		ProducerByCase: map[string][]byte{caseProfile.ID: record.StepFacts}, Ready: true}

	b := ordinaryTestBudget(t)
	// The 72 tuples read 9 trees: share their decoded assets as the coordinator does.
	b.decoded = newOrdinaryDecodedAssets()
	judgments := map[ordinaryTSQLTuple]ordinaryTupleJudgment{}
	var order []ordinaryTSQLTuple
	for step, version := range versions {
		pin := OrdinaryPin{Path: "source-witness/" + digestHex(version) + ".utf8", Bytes: uint64(len(version)), SHA256: digestHex(version)}
		roles := []string{"incremental"}
		if step > 0 {
			roles = append(roles, "fresh")
		}
		for _, role := range roles {
			actual := checked.Steps[step].Incremental
			if role == "fresh" {
				actual = *checked.Steps[step].Fresh
			}
			for _, query := range profile.Queries {
				key := ordinaryTSQLTuple{query.ID, uint32(step), role}
				suffix := fmt.Sprintf("s%d-%s", step, role)
				body := func(id string) []byte {
					data, ok := bodies["expected/tsql/"+id+".json"]
					if !ok {
						t.Fatalf("expected asset %s is missing", id)
					}
					return data
				}
				var factData []byte
				if strings.HasPrefix(query.ID, "facts.") {
					factData = body("tsql-facts-" + query.ID + "-" + suffix)
				}
				id := ordinaryTupleIdentity{Case: caseProfile.ID, Query: query.ID, Step: uint32(step), TreeRole: role, Source: pin}
				judgment, e := adjudicateOrdinaryTuple(b, id, actual, version, body("meta-"+query.ID), body("tsql-tree-"+suffix), body("tsql-captures-"+query.ID+"-"+suffix), factData, &facts)
				if e != nil {
					t.Fatalf("%s: adjudicator stopped: %v", key, e)
				}
				judgments[key] = judgment
				order = append(order, key)
			}
		}
	}
	return judgments, order
}

func ordinaryTSQLCounts(judgments map[ordinaryTSQLTuple]ordinaryTupleJudgment) map[string]int {
	counts := map[string]int{}
	for _, j := range judgments {
		counts[j.State+":"+j.Code]++
	}
	return counts
}

// TestOrdinaryTSQLExpectedAgainstNative runs one owned native producer for the selected TSQL
// shard, requires all 72 independently authored tuples to be VERIFIED by the product tuple
// adjudicator, and then shows that one tampered node, capture or fact fails exactly the tuples
// that consume it.
func TestOrdinaryTSQLExpectedAgainstNative(t *testing.T) {
	artifactRoot := ordinarySQLEnv(t)
	produced := ordinarySQLProduce(t, filepath.Join(artifactRoot, "sql-tsql-direct"), "tsql", nil)
	judgments, order := ordinaryTSQLJudgeR8(t, produced, produced.AssetBodies)
	if len(order) != 72 {
		t.Fatalf("expected 72 tuples, adjudicated %d", len(order))
	}
	for _, key := range order {
		if j := judgments[key]; j.State != ordinaryTupleVerified || j.Code != "" {
			t.Errorf("%s: %s %s %+v", key, j.State, j.Code, j.Findings)
		}
	}
	t.Logf("unmutated expectations: %v", ordinaryTSQLCounts(judgments))
	if t.Failed() {
		return
	}

	// Reference rows are synthetic; their decode cost is measured with the product accounting to show the capacity need of the public pass.
	var refLogical, refWork uint64
	{
		profile, err := ParseOracleProfile(produced.Req.Profile)
		if err != nil {
			t.Fatal(err)
		}
		versions, _, err := ApplyEdits(EncodingUTF8, produced.Source, profile.Native.Cases[0].Edits, 65536)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range ordinarySQLSyntheticReferences(t, produced.Profile, produced.Source, produced.Registration, produced.Expected, produced.Metadata) {
			b := ordinaryTestBudget(t)
			var head struct {
				Schema string `json:"schema"`
			}
			var decoded instrumentedReference
			if e := b.decodeWitnessTyped("reference-response", ref.Response, &head, false); e != nil {
				t.Fatal(e)
			}
			if e := b.decodeWitnessTyped("reference-response", ref.Response, &decoded, true); e != nil {
				t.Fatal(e)
			}
			if e := b.reserveReference(&decoded, versions); e != nil {
				t.Fatal(e)
			}
			refLogical += b.logical
			refWork += b.work
		}
		t.Logf("40 synthetic reference rows: decode+reserve logical=%d work=%d (selected-shard rows only; excludes graph, receipts and later stages)", refLogical, refWork)
	}

	mutations := []struct {
		name     string
		mutation ordinaryTSQLMutation
		want     map[ordinaryTSQLTuple]string
	}{
		{"tree node", ordinaryTSQLMutation{Tree: func(step uint32, role string, nodes []TreeNode) []TreeNode {
			if step == 1 && role == "incremental" {
				nodes[5].Type = "tampered"
			}
			return nodes
		}}, map[ordinaryTSQLTuple]string{}},
		{"capture", ordinaryTSQLMutation{Captures: func(query string, step uint32, role string, c *ordinaryExpectedCaptures) {
			if query == "q.r6-structure" && step == 0 && role == "incremental" {
				c.Ordered[0].Match++
			}
		}}, map[ordinaryTSQLTuple]string{{"q.r6-structure", 0, "incremental"}: "ORDINARY_CAPTURE_MISMATCH"}},
		{"fact", ordinaryTSQLMutation{Fact: func(query string, step uint32, role string, f *ordinaryExpectedFact) {
			if query == "facts.declarations" && step == 0 && role == "incremental" {
				f.Declarations.Items = append(f.Declarations.Items, DeclarationItem{Fact: "create_object", NodeType: "create_table", Name: &ByteSpan{StartByte: 0, EndByte: 1}, Status: AssessPass})
				f.Declarations.Assessment = AssessPass
			}
		}}, map[ordinaryTSQLTuple]string{{"facts.declarations", 0, "incremental"}: "ORDINARY_FACT_MISMATCH"}},
	}
	for _, c := range mutations[:1] {
		for _, query := range []string{"q.structure", "q.r6-structure", "q.r6-begin", "q.probe-v16", "q.probe-v22", "q.issue128-ownership", "facts.declarations", "facts.dynamic_sql"} {
			c.want[ordinaryTSQLTuple{query, 1, "incremental"}] = "ORDINARY_EXPECTED_TREE_MISMATCH"
		}
	}
	for _, c := range mutations {
		t.Run(c.name, func(t *testing.T) {
			_, bodies, _, _ := ordinaryTSQLAssetsFromProduced(t, produced, &c.mutation)
			got, order := ordinaryTSQLJudgeR8(t, produced, bodies)
			failedKeys := []string{}
			for _, key := range order {
				j := got[key]
				switch want, tampered := c.want[key]; {
				case tampered && (j.State != ordinaryTupleFailed || j.Code != want):
					t.Errorf("%s: tampered expectation gave %s %s, want FAILED %s", key, j.State, j.Code, want)
				case !tampered && (j.State != ordinaryTupleVerified || j.Code != ""):
					t.Errorf("%s: untouched tuple gave %s %s %+v", key, j.State, j.Code, j.Findings)
				}
				if j.State == ordinaryTupleFailed {
					failedKeys = append(failedKeys, key.String()+"="+j.Code)
				}
			}
			slices.Sort(failedKeys)
			t.Logf("%s tampered: %d tuples FAILED: %v", c.name, len(failedKeys), failedKeys)
		})
	}
}

// ordinaryTSQLAssetsFromProduced rebuilds the expectation bodies of an existing run, optionally mutated.
func ordinaryTSQLAssetsFromProduced(t *testing.T, produced ordinarySQLProduced, mutation *ordinaryTSQLMutation) (OrdinaryAssetRegistration, map[string][]byte, ordinaryTSQLExpected, map[string]QueryWitnessMetadata) {
	t.Helper()
	return ordinaryTSQLAssets(t, "alt-tsql-B01-r001", produced.Source, produced.Req.Profile, mutation)
}
