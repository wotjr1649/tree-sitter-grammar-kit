package kit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

func semanticRegistrationFixture(t *testing.T) (semanticRegistration, OracleProfile) {
	t.Helper()
	var anchors struct {
		Queries  []QueryWitnessQuery `json:"queries"`
		Required []QueryWitnessTuple `json:"required"`
	}
	data, err := os.ReadFile(filepath.Join("..", "testdata", "query-semantics", "small-json-anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &anchors); err != nil {
		t.Fatal(err)
	}
	c := IncrementalCase{ID: "c1", Input: NativeInput{Path: "c1.json", Role: "case", Bytes: 11, SHA256: digestHex([]byte(`{"a":[1,1]}`))}, Encoding: EncodingUTF8,
		Edits: []Edit{{StartByte: 8, OldEndByte: 9, NewEndByte: 9, Old: []byte("1"), New: []byte("2")}}, Points: []NativePoint{},
		Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Contains: []string{}}, {Step: 1, Syntax: "NO_ERROR", Contains: []string{}}}}
	profile := OracleProfile{Native: IncrementalProfile{Schema: OracleSemanticsSchema, ID: "semantic-json", Route: "json", Operation: "native-query", Symbol: "tree_sitter_json",
		Encoding: EncodingUTF8, Output: OutputTree, Compiler: ToolIdentity{Name: "cc", Version: "test", SHA256: strings.Repeat("a", 64), Bytes: 1},
		Grammar: []NativeInput{{Path: "src/parser.c", Role: "parser", Bytes: 23126, SHA256: "86971e7cb4e6dae9c5395bedf51abf525b2b662c6bcb632d33b72e4519aaab83"}}, Cases: []IncrementalCase{c}},
		Queries: []OracleQuery{}, OracleCases: []OracleCase{{IncrementalCase: c, QueryExpect: []QueryExpectation{}}}}
	for _, query := range anchors.Queries {
		profile.Queries = append(profile.Queries, OracleQuery{ID: query.ID, Source: query.Source})
	}
	op := NativeOperations()["native-query"]
	reg := semanticRegistration{QueryWitnessRegistration: QueryWitnessRegistration{Schema: querySemanticsRegistrationSchema, Policy: querypolicy.Revision,
		Case: c.ID, Route: "json", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8, Input: c.Input, Edits: c.Edits, Queries: anchors.Queries, Required: anchors.Required},
		Context: "full-input-utf8/r2", Profile: semanticMeaning(profile), NativePolicy: NativePolicy(op, OutputTree, nil), Limits: semanticLimitsFor(op), Classes: make([]string, 24),
		PlannedMembers: []semanticPlannedMember{{Owner: "D", Path: "capture/00000-c1.json", Role: "capture"}, {Owner: "O", Path: "records/00000-c1.json", Role: "record"}}}
	for i := range reg.Classes {
		reg.Classes[i] = "semantic-required"
	}
	return reg, profile
}

func semanticEncodeRegistration(t *testing.T, reg semanticRegistration, profile OracleProfile) ([]byte, OracleProfile) {
	t.Helper()
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	profile.QuerySemanticsRegistrationSHA256 = digestHex(data)
	return data, profile
}

func TestSemanticRegistrationAndProfile(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	data, profile := semanticEncodeRegistration(t, reg, profile)
	actual, e := semanticTestBudget(t).semanticsRegistration(data, profile)
	if e != nil || len(actual.Required) != 24 || !reflect.DeepEqual(reg, actual) {
		t.Fatalf("preregistration %v", e)
	}
	meaning, err := json.Marshal(reg.Profile)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(meaning, &fields); err != nil {
		t.Fatal(err)
	}
	fields["schema"] = json.RawMessage(`"tsgk-oracle/r3"`)
	fields["query_semantics_registration_sha256"], _ = json.Marshal(profile.QuerySemanticsRegistrationSHA256)
	profileData, _ := json.Marshal(fields)
	parsed, e := semanticTestBudget(t).oracleProfile(profileData)
	if e != nil || !reflect.DeepEqual(semanticMeaning(parsed), canonicalSemanticMeaning(reg.Profile)) {
		t.Fatalf("profile3 meaning %v", e)
	}
	legacy, err := ParseOracleProfile(profileData)
	if err != nil || !reflect.DeepEqual(parsed, legacy) {
		t.Fatalf("shared AST changed public parse: %v", err)
	}
	if _, e := semanticTestBudget(t).semanticsRegistration(data, parsed); e != nil {
		t.Fatalf("actual parsed empty arrays rejected: %v", e)
	}
	for _, revision := range []string{OracleSchema, oracleSchemaR1} {
		fields["schema"], _ = json.Marshal(revision)
		for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`""`), json.RawMessage(`"` + strings.Repeat("a", 64) + `"`)} {
			fields["query_semantics_registration_sha256"] = value
			mutant, _ := json.Marshal(fields)
			if _, err := ParseOracleProfile(mutant); err == nil {
				t.Fatal("historical revision accepted semantic member")
			}
		}
	}
	fields["schema"] = json.RawMessage(`"tsgk-oracle/r3"`)
	delete(fields, "query_semantics_registration_sha256")
	missing, _ := json.Marshal(fields)
	if _, err := ParseOracleProfile(missing); err == nil {
		t.Fatal("profile3 accepted missing registration digest")
	}
}

func TestSemanticRegistrationCounterexamples(t *testing.T) {
	for _, mutation := range []string{"profile-id", "query-order", "edited-source", "limit", "missing-tuple", "duplicate-tuple", "class", "anchor-null", "metadata", "plan-duplicate", "plan-case-alias", "plan-owner", "plan-empty", "digest"} {
		t.Run(mutation, func(t *testing.T) {
			reg, profile := semanticRegistrationFixture(t)
			switch mutation {
			case "profile-id":
				reg.Profile.ID += "-changed"
			case "query-order":
				reg.Queries[0], reg.Queries[1] = reg.Queries[1], reg.Queries[0]
			case "edited-source":
				reg.Edits = append([]Edit{}, reg.Edits...)
				reg.Edits[0].New = []byte("3")
			case "limit":
				reg.Limits.Captures++
			case "missing-tuple":
				reg.Required, reg.Classes = reg.Required[:23], reg.Classes[:23]
			case "duplicate-tuple":
				reg.Required[1] = reg.Required[0]
			case "class":
				reg.Classes[0] = "structural-only"
			case "anchor-null":
				reg.Required[0].Expected = nil
			case "metadata":
				reg.Queries[0].Metadata = nil
			case "plan-duplicate":
				reg.PlannedMembers = append(reg.PlannedMembers, reg.PlannedMembers[0])
			case "plan-case-alias":
				reg.PlannedMembers = append(reg.PlannedMembers, semanticPlannedMember{Owner: "D", Path: "Capture/00000-c1.json", Role: "capture"})
			case "plan-owner":
				reg.PlannedMembers[0].Owner = "reference"
			case "plan-empty":
				reg.PlannedMembers = []semanticPlannedMember{}
			}
			data, profile := semanticEncodeRegistration(t, reg, profile)
			if mutation == "digest" {
				profile.QuerySemanticsRegistrationSHA256 = strings.Repeat("b", 64)
			}
			if _, e := semanticTestBudget(t).semanticsRegistration(data, profile); e == nil {
				t.Fatal("preregistration counterexample accepted")
			}
		})
	}
}

func TestSemanticMeaningCanonicalArrays(t *testing.T) {
	var captures []CaptureExpectation
	original := semanticOracleMeaning{Cases: []semanticOracleCase{{IncrementalCase: IncrementalCase{Edits: []Edit{{New: []byte("a")}}, Expect: []StepExpectation{{Anchors: []ExpectAnchor{}}}},
		QueryExpect: []QueryExpectation{{Captures: &captures}, {Captures: nil}}}}}
	actual := canonicalSemanticMeaning(original)
	if actual.Cases[0].Edits[0].Old == nil || actual.Cases[0].Points == nil || actual.Cases[0].Expect[0].Contains == nil || actual.Cases[0].Expect[0].Anchors != nil ||
		actual.Cases[0].QueryExpect[0].Captures == nil || *actual.Cases[0].QueryExpect[0].Captures == nil || actual.Cases[0].QueryExpect[1].Captures != nil {
		t.Fatal("logical arrays or missing capture assertion changed")
	}
	if original.Cases[0].Edits[0].Old != nil || original.Cases[0].Expect[0].Contains != nil || original.Cases[0].Expect[0].Anchors == nil || *original.Cases[0].QueryExpect[0].Captures != nil {
		t.Fatal("profile canonicalization mutated caller-owned values")
	}
}

func semanticProfileBytes(t *testing.T, profile OracleProfile) []byte {
	t.Helper()
	data, err := json.Marshal(semanticMeaning(profile))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	fields["schema"], _ = json.Marshal(profile.Native.Schema)
	fields["query_semantics_registration_sha256"], _ = json.Marshal(profile.QuerySemanticsRegistrationSHA256)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSemanticRegistrationNoEditsAndCaptureAssertion(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	profile.OracleCases[0].Edits = nil
	profile.OracleCases[0].Expect = profile.OracleCases[0].Expect[:1]
	profile.Native.Cases = []IncrementalCase{profile.OracleCases[0].IncrementalCase}
	reg.Edits = []Edit{}
	reg.Required = reg.Required[:0:0]
	// Every query retains its initial required tuple when there are no edits.
	fixture, _ := semanticRegistrationFixture(t)
	for _, tuple := range fixture.Required {
		if tuple.Step == 0 {
			reg.Required = append(reg.Required, tuple)
		}
	}
	reg.Classes = reg.Classes[:len(reg.Required)]
	empty := []CaptureExpectation{}
	profile.OracleCases[0].QueryExpect = []QueryExpectation{{Query: profile.Queries[0].ID, Step: 0, Status: StatusCompleted, Captures: &empty}}
	reg.Profile = semanticMeaning(profile)
	data, profile := semanticEncodeRegistration(t, reg, profile)
	parsed, e := semanticTestBudget(t).oracleProfile(semanticProfileBytes(t, profile))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := semanticTestBudget(t).semanticsRegistration(data, parsed); e != nil {
		t.Fatalf("no-edit parsed profile: %v", e)
	}
	parsed.OracleCases[0].QueryExpect[0].Captures = nil
	if _, e := semanticTestBudget(t).semanticsRegistration(data, parsed); e == nil || e.Code != "SEMANTICS_PROFILE_MEANING" {
		t.Fatalf("missing and empty capture assertions merged: %v", e)
	}
}

func TestSemanticProfilePostASTBudgetAndCancellation(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	_, profile = semanticEncodeRegistration(t, reg, profile)
	data := semanticProfileBytes(t, profile)
	values := &valueBudget{max: 2000000}
	v, e := decodeBounded("oracle", data, MaxDocumentBytes, values)
	if e != nil {
		t.Fatal(e)
	}
	b := semanticTestBudget(t)
	b.work = eligibilityWorkLimit - 5*values.n - 1
	if _, e := b.oracleProfile(data); e == nil || e.Kind != KindResourceLimit || e.Path != "oracle-typed" {
		t.Fatalf("post-AST reservation missing: %v", e)
	}
	b = semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit - uint64(len(data)) + 1
	if _, e := b.oracleProfile(data); e == nil || e.Kind != KindResourceLimit || b.work != 0 {
		t.Fatalf("AST allocated before reservation: %v", e)
	}
	stopped := fail(KindCancelled, "CANCELLED", "profile-walk", context.Canceled)
	checks := 0
	_, e = parseOracleValue(data, v, func() *Error {
		checks++
		if checks == 40 {
			return stopped
		}
		return nil
	})
	if e != stopped || checks != 40 {
		t.Fatalf("typed walk cancellation lost: %v checks=%d", e, checks)
	}
}

func TestSemanticRegistrationPostDecodeBudgetAndCancellation(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	data, profile := semanticEncodeRegistration(t, reg, profile)
	probe := semanticTestBudget(t)
	var decoded semanticRegistration
	if e := probe.decode("semantics-registration", data, &decoded, true); e != nil {
		t.Fatal(e)
	}
	b := semanticTestBudget(t)
	b.work = eligibilityWorkLimit - probe.work
	if _, e := b.semanticsRegistration(data, profile); e == nil || e.Kind != KindResourceLimit || e.Path != "semantics-metadata" {
		t.Fatalf("post-decode metadata reservation missing: %v", e)
	}
	stopped := fail(KindCancelled, "CANCELLED", "metadata-walk", context.Canceled)
	checks := 0
	_, e := validateQueryWitnessRegistrationChecked(reg.QueryWitnessRegistration, querySemanticsRegistrationSchema, func() *Error {
		checks++
		if checks == 5 {
			return stopped
		}
		return nil
	})
	if e != stopped || checks != 5 {
		t.Fatalf("metadata cancellation lost: %v checks=%d", e, checks)
	}
}

func TestSemanticGrammarRepeatedObjectCancellation(t *testing.T) {
	data := []byte(`[{"path":"src/parser.c","role":"parser","sha256":"` + strings.Repeat("a", 64) + `","bytes":1}]`)
	v, e := decodeStrict("grammar", data, MaxDocumentBytes)
	if e != nil {
		t.Fatal(e)
	}
	stopped := fail(KindCancelled, "CANCELLED", "grammar-second-object", context.Canceled)
	checks := 0
	_, e = parseGrammarInputs(typed{doc: "grammar", check: func() *Error {
		checks++
		if checks == 7 {
			return stopped
		}
		return nil
	}}, v)
	if e != stopped || checks != 7 {
		t.Fatalf("repeated object error lost: %v checks=%d", e, checks)
	}
}

func TestSemanticSparseOracleProfile(t *testing.T) {
	reg, profile := semanticRegistrationFixture(t)
	_, profile = semanticEncodeRegistration(t, reg, profile)
	data := semanticProfileBytes(t, profile)
	data = append(data, []byte(strings.Repeat(" ", (5<<20)-len(data)))...)
	if _, e := semanticTestBudget(t).oracleProfile(data); e != nil {
		t.Fatalf("5MiB sparse profile rejected: %v", e)
	}
}
