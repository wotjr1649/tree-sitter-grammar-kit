package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/drivers"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

// This manual preregistration runs before any C# query capture and supplies no approval.
func TestQuerySemanticsCSharpLocal(t *testing.T) {
	stage := os.Getenv("TSGK_QUERY_CSHARP_LOCAL_STAGE")
	if stage == "" {
		t.Skip("explicit local C# reference capture is required")
	}
	root := os.Getenv("TSGK_QUERY_CSHARP_LOCAL_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("local capture root must be absolute")
	}
	switch stage {
	case "plan":
		actualCSharpPlan(t, root)
	case "assemble":
		actualJSONAssemble(t, root, os.Getenv("TSGK_QUERY_CSHARP_FRESH_REFERENCE"), os.Getenv("TSGK_QUERY_CSHARP_CAPTURE"))
	case "verify":
		actualJSONVerify(t, os.Getenv("TSGK_QUERY_CSHARP_ASSEMBLY"), os.Getenv("TSGK_QUERY_CSHARP_APPROVAL"))
	default:
		t.Fatalf("unknown local stage %q", stage)
	}
}

func actualCSharpPlan(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("plan root must be a new empty caller-owned directory", err)
	}
	var draft struct {
		SourcePins map[string]struct {
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"source_pins"`
		Metadata     QueryWitnessMetadata   `json:"query_metadata"`
		Required     []QueryWitnessTuple    `json:"required"`
		ABI          eligibilityProducerABI `json:"expected_producer_abi"`
		ChildAnchors jsontext.Value         `json:"independent_cst_anchors"`
	}
	draftPath := os.Getenv("TSGK_QUERY_CSHARP_DRAFT")
	if !filepath.IsAbs(draftPath) {
		t.Fatal("draft must be absolute")
	}
	anchorData := actualJSONRead(t, draftPath)
	if digestHex(anchorData) != "555079bb3f892d7008a40d726ad4da9550814c9f2d9ef7f476e09b2cf6be5138" {
		t.Fatal("reviewed static draft changed")
	}
	if err := jsonv2.Unmarshal(anchorData, &draft); err != nil {
		t.Fatal(err)
	}
	sources := []string{"class C {}", "class D {}"}
	query := `((compilation_unit) @root (#eq? @root "class C {}"))`
	for name, value := range map[string]string{"source-0.bin": sources[0], "source-1.bin": sources[1], "query.scm": query} {
		pin := draft.SourcePins[name]
		if uint64(len(value)) != pin.Bytes || digestHex([]byte(value)) != pin.SHA256 {
			t.Fatal("static source/query identity", name)
		}
	}
	queriesWitness := []QueryWitnessQuery{{ID: "root-eq", Source: query, Metadata: &draft.Metadata}}
	if len(draft.Required) != 3 || draft.ABI != (eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 654, FieldCount: 26}) {
		t.Fatal("static denominator/ABI")
	}
	var registry struct {
		Routes []struct {
			Route string        `json:"route"`
			Files []NativeInput `json:"files"`
		} `json:"routes"`
	}
	if err := jsonv2.Unmarshal(actualJSONRead(t, "../contracts/native-routes.json"), &registry); err != nil {
		t.Fatal(err)
	}
	var grammar []NativeInput
	for _, route := range registry.Routes {
		if route.Route == "csharp" {
			grammar = route.Files
		}
	}
	if len(grammar) != 5 || !slices.IsSortedFunc(grammar, func(a, b NativeInput) int { return bytes.Compare([]byte(a.Path), []byte(b.Path)) }) {
		t.Fatal("C# grammar closure/order")
	}
	fresh := os.Getenv("TSGK_QUERY_CSHARP_FRESH_REFERENCE")
	if !filepath.IsAbs(fresh) {
		t.Fatal("fresh reference root must be absolute")
	}
	var identity struct {
		Tools     map[string]string `json:"tools"`
		ToolPaths map[string]string `json:"tool_paths"`
	}
	if err := jsonv2.Unmarshal(actualJSONRead(t, filepath.Join(fresh, "build", "identity.json")), &identity); err != nil {
		t.Fatal(err)
	}
	cc := identity.ToolPaths["cc"]
	ccData := actualJSONRead(t, cc)
	if digestHex(ccData) != identity.Tools["cc"] {
		t.Fatal("compiler changed after reference capture")
	}
	input := NativeInput{Path: "cases/c1.cs", Role: "case", Bytes: draft.SourcePins["source-0.bin"].Bytes, SHA256: draft.SourcePins["source-0.bin"].SHA256}
	edits := []Edit{{StartByte: 6, OldEndByte: 7, NewEndByte: 7, Old: []byte("C"), New: []byte("D")}}
	changed, _, err := ApplyEdits(EncodingUTF8, []byte(sources[0]), edits, NativeOperations()["native-query"].InputBytes)
	if err != nil || len(changed) != 2 || !bytes.Equal(changed[1], []byte(sources[1])) {
		t.Fatal("independent edit")
	}
	queries := make([]OracleQuery, len(queriesWitness))
	for i, q := range queriesWitness {
		queries[i] = OracleQuery{ID: q.ID, Source: q.Source}
	}
	meaning := canonicalSemanticMeaning(semanticOracleMeaning{ID: "csharp3-reference", Route: "csharp", Operation: "native-query", Symbol: "tree_sitter_c_sharp", Encoding: EncodingUTF8, Output: OutputTree,
		Compiler: ToolIdentity{Name: "cc", Version: "host", SHA256: digestHex(ccData), Bytes: uint64(len(ccData))}, Grammar: grammar, Queries: queries, API: false,
		Cases: []semanticOracleCase{{IncrementalCase: IncrementalCase{ID: "c1", Input: input, Encoding: EncodingUTF8, Edits: edits, Points: []NativePoint{}, Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR", Anchors: []ExpectAnchor{{Type: "class_declaration", StartByte: 0, EndByte: 10}, {Type: "identifier", StartByte: 6, EndByte: 7}}}, {Step: 1, Syntax: "NO_ERROR", Anchors: []ExpectAnchor{{Type: "class_declaration", StartByte: 0, EndByte: 10}, {Type: "identifier", StartByte: 6, EndByte: 7}}}}}}}})
	reg := semanticRegistration{QueryWitnessRegistration: QueryWitnessRegistration{Schema: querySemanticsRegistrationSchema, Policy: querypolicy.Revision, Case: "c1", Route: "csharp", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8, Input: input, Edits: edits, Queries: queriesWitness, Required: draft.Required},
		Context: semanticPagedContext, Profile: meaning, NativePolicy: NativePolicy(NativeOperations()["native-query"], OutputTree, nil), Limits: semanticLimitsFor(NativeOperations()["native-query"]), Classes: make([]string, 3)}
	for i := range reg.Classes {
		reg.Classes[i] = "semantic-required"
	}
	add := func(owner, path, role string) {
		reg.PlannedMembers = append(reg.PlannedMembers, semanticPlannedMember{Owner: owner, Path: path, Role: role})
	}
	add("O", "manifest.json", "manifest")
	add("D", "capture/driver.bin", "driver")
	runtimeData, err := drivers.NativeC.ReadFile("native-c/runtime-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var runtime struct {
		Files []jsontext.Value `json:"files"`
	}
	if err := jsonv2.Unmarshal(runtimeData, &runtime); err != nil || len(runtime.Files) != 83 {
		t.Fatal("runtime closure", err)
	}
	for i := 0; i < len(runtime.Files)+len(grammar)+3; i++ {
		role := "build-input"
		if i >= len(runtime.Files) && i < len(runtime.Files)+len(grammar) {
			role = "grammar-" + grammar[i-len(runtime.Files)].Role
		}
		if role == "grammar-parser" {
			pin := grammar[i-len(runtime.Files)]
			if pin.Bytes != 51472926 || pin.Bytes <= 16777216 {
				t.Fatal("parser paging prerequisite")
			}
			add("D", fmt.Sprintf("capture/build-input-%05d.pages.json", i), "grammar-paged-descriptor")
			for part, offset := 0, uint64(0); offset < pin.Bytes; part, offset = part+1, offset+16777216 {
				add("D", fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", i, part), "grammar-paged-part")
			}
		} else {
			add("D", fmt.Sprintf("capture/build-input-%05d.bin", i), role)
		}
	}
	add("D", "capture/build.json", "build")
	add("D", "capture/build-manifest.txt", "build-manifest")
	add("O", "projection/00000-c1.json", "projection")
	for _, x := range []struct{ Suffix, Role string }{{"-stdin.bin", "stdin-frame"}, {"-process.json", "process"}, {"-stdout.bin", "stdout-frame"}, {".json", "capture"}} {
		add("D", "capture/00000-c1"+x.Suffix, x.Role)
	}
	add("O", "source/00000-c1.bin", "source")
	add("D", "raw/00000-c1.json", "raw")
	add("O", "records/00000-c1.json", "record")
	if len(reg.PlannedMembers) != 107 {
		t.Fatal("member denominator")
	}
	regData := eligibilityJSON(t, reg)
	var profile map[string]jsontext.Value
	if err := jsonv2.Unmarshal(eligibilityJSON(t, meaning), &profile); err != nil {
		t.Fatal(err)
	}
	profile["schema"] = jsontext.Value(`"tsgk-oracle/r3"`)
	profile["query_semantics_registration_sha256"] = jsontext.Value(fmt.Sprintf("%q", digestHex(regData)))
	profileData := eligibilityJSON(t, profile)
	parsed, parseErr := ParseOracleProfile(profileData)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if _, err := semanticTestBudget(t).semanticsRegistration(regData, parsed); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"plan", "input", "input/cases", "input/queries", "native-work", "evidence"} {
		if err := os.Mkdir(filepath.Join(root, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	actualJSONWrite(t, filepath.Join(root, "plan", "registration.json"), regData)
	actualJSONWrite(t, filepath.Join(root, "plan", "profile.json"), profileData)
	actualJSONWrite(t, filepath.Join(root, "input", "cases", "c1.cs"), []byte(sources[0]))
	for i, s := range sources {
		actualJSONWrite(t, filepath.Join(root, "input", fmt.Sprintf("source-%d.bin", i)), []byte(s))
	}
	for _, q := range queriesWitness {
		actualJSONWrite(t, filepath.Join(root, "input", "queries", q.ID+".scm"), []byte(q.Source))
	}
	actualJSONWrite(t, filepath.Join(root, "plan", "anchor.json"), anchorData)
	actualJSONWrite(t, filepath.Join(root, "plan", "expected-abi.json"), eligibilityJSON(t, draft.ABI))
	actualJSONWrite(t, filepath.Join(root, "plan", "child-anchors.json"), draft.ChildAnchors)
	t.Log("preregistered107members/1query/3tuples; no producer or caller approval executed")
}
