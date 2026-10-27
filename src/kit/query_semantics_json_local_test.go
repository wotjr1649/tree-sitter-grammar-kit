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

// This explicit local harness preserves real captures; it never supplies its own approval.
func TestQuerySemanticsJSONLocal(t *testing.T) {
	stage := os.Getenv("TSGK_QUERY_JSON_LOCAL_STAGE")
	if stage == "" {
		t.Skip("explicit local JSON reference capture is required")
	}
	root := os.Getenv("TSGK_QUERY_JSON_LOCAL_ROOT")
	if !filepath.IsAbs(root) {
		t.Fatal("local capture root must be absolute")
	}
	switch stage {
	case "plan":
		actualJSONPlan(t, root)
	default:
		t.Fatalf("unknown local stage %q", stage)
	}
}

func actualJSONRead(t *testing.T, path string) []byte {
	t.Helper()
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxDocumentBytes {
		t.Fatalf("bounded regular input %s: %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || int64(len(data)) != st.Size() {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func actualJSONWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write %s: %v %v", path, writeErr, closeErr)
	}
}

func actualJSONPlan(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("plan root must be a new empty caller-owned directory", err)
	}
	var anchors struct {
		Sources []struct {
			UTF8   string `json:"utf8"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"sources"`
		Queries  []QueryWitnessQuery `json:"queries"`
		Required []QueryWitnessTuple `json:"required"`
	}
	anchorData := actualJSONRead(t, "../testdata/query-semantics/small-json-anchors.json")
	if err := jsonv2.Unmarshal(anchorData, &anchors); err != nil {
		t.Fatal(err)
	}
	if len(anchors.Sources) != 2 || len(anchors.Queries) != 8 || len(anchors.Required) != 24 {
		t.Fatal("independent anchor denominator changed")
	}
	for _, source := range anchors.Sources {
		if uint64(len(source.UTF8)) != source.Bytes || digestHex([]byte(source.UTF8)) != source.SHA256 {
			t.Fatal("source anchor identity")
		}
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
		if route.Route == "json" {
			grammar = route.Files
		}
	}
	if len(grammar) != 4 || !slices.IsSortedFunc(grammar, func(a, b NativeInput) int { return bytes.Compare([]byte(a.Path), []byte(b.Path)) }) {
		t.Fatal("JSON grammar closure/order")
	}
	fresh := os.Getenv("TSGK_QUERY_JSON_FRESH_REFERENCE")
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
	input := NativeInput{Path: "cases/c1.json", Role: "case", Bytes: anchors.Sources[0].Bytes, SHA256: anchors.Sources[0].SHA256}
	edits := []Edit{{StartByte: 8, OldEndByte: 9, NewEndByte: 9, Old: []byte("1"), New: []byte("2")}}
	changed, _, err := ApplyEdits(EncodingUTF8, []byte(anchors.Sources[0].UTF8), edits, NativeOperations()["native-query"].InputBytes)
	if err != nil || len(changed) != 2 || !bytes.Equal(changed[1], []byte(anchors.Sources[1].UTF8)) {
		t.Fatal("independent edit")
	}
	queries := make([]OracleQuery, len(anchors.Queries))
	for i, q := range anchors.Queries {
		queries[i] = OracleQuery{ID: q.ID, Source: q.Source}
	}
	meaning := canonicalSemanticMeaning(semanticOracleMeaning{ID: "json24-reference", Route: "json", Operation: "native-query", Symbol: "tree_sitter_json", Encoding: EncodingUTF8, Output: OutputTree,
		Compiler: ToolIdentity{Name: "cc", Version: "host", SHA256: digestHex(ccData), Bytes: uint64(len(ccData))}, Grammar: grammar, Queries: queries, API: false,
		Cases: []semanticOracleCase{{IncrementalCase: IncrementalCase{ID: "c1", Input: input, Encoding: EncodingUTF8, Edits: edits, Points: []NativePoint{}, Expect: []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 1, Syntax: "NO_ERROR"}}}}}})
	reg := semanticRegistration{QueryWitnessRegistration: QueryWitnessRegistration{Schema: querySemanticsRegistrationSchema, Policy: querypolicy.Revision, Case: "c1", Route: "json", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8, Input: input, Edits: edits, Queries: anchors.Queries, Required: anchors.Required},
		Context: "full-input-utf8/r2", Profile: meaning, NativePolicy: NativePolicy(NativeOperations()["native-query"], OutputTree, nil), Limits: semanticLimitsFor(NativeOperations()["native-query"]), Classes: make([]string, 24)}
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
		add("D", fmt.Sprintf("capture/build-input-%05d.bin", i), role)
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
	if len(reg.PlannedMembers) != 102 {
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
	actualJSONWrite(t, filepath.Join(root, "input", "cases", "c1.json"), []byte(anchors.Sources[0].UTF8))
	for i, s := range anchors.Sources {
		actualJSONWrite(t, filepath.Join(root, "input", fmt.Sprintf("source-%d.bin", i)), []byte(s.UTF8))
	}
	for _, q := range anchors.Queries {
		actualJSONWrite(t, filepath.Join(root, "input", "queries", q.ID+".scm"), []byte(q.Source))
	}
	actualJSONWrite(t, filepath.Join(root, "plan", "anchor.json"), anchorData)
	t.Log("preregistered102members/8queries/24tuples; no producer or caller approval executed")
}
