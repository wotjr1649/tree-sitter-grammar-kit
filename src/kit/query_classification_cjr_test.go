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

// These pins define closed preregistration expectations; only the later live
// guarded native/reference capture can supply observed compiled metadata.
type classificationCJRDefinitionProfile struct {
	Name, AnchorFile, AnchorSHA string
	Route, Case, Workload       string
	QueryFile, QuerySHA         string
	Set, Symbol, RootType       string
	Language                    string
	QueryBytes, QueryCount      int
	GrammarCount                int
	ABI                         eligibilityProducerABI
	Counts                      classificationGoDefinitionSlotCounts
}

func classificationCJRProfileForName(name string) (classificationCJRDefinitionProfile, error) {
	switch name {
	case "c4-empty":
		profile, ok := classificationCJRProfileForRoute("c")
		if !ok {
			return classificationCJRDefinitionProfile{}, fmt.Errorf("C profile binding is missing")
		}
		return profile, nil
	case "java5-empty":
		profile, ok := classificationCJRProfileForRoute("java")
		if !ok {
			return classificationCJRDefinitionProfile{}, fmt.Errorf("Java profile binding is missing")
		}
		return profile, nil
	case "rust4-empty":
		profile, ok := classificationCJRProfileForRoute("rust")
		if !ok {
			return classificationCJRDefinitionProfile{}, fmt.Errorf("Rust profile binding is missing")
		}
		return profile, nil
	default:
		return classificationCJRDefinitionProfile{}, fmt.Errorf("unknown closed C/Java/Rust profile %q", name)
	}
}

func classificationCJRProfileForRoute(route string) (classificationCJRDefinitionProfile, bool) {
	abi15 := func(symbols, fields uint32) eligibilityProducerABI {
		return eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: symbols, FieldCount: fields}
	}
	switch route {
	case "c":
		return classificationCJRDefinitionProfile{
			Name: "c4-empty", AnchorFile: "c4-classification-anchors.json", AnchorSHA: "c4a8b34393c268a0ca479a9f82e99617d0946e795fa7e35bdf24ad2437e04f2d",
			Route: "c", Case: "c4-empty-definitions", Workload: "c4-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/c.json", QuerySHA: "7089c8e7b84a224076b431782070fc675735420bc7971689d5530b08197533fd",
			Set: "s06-c", Symbol: "tree_sitter_c", RootType: "translation_unit", Language: "C",
			QueryBytes: 6204, QueryCount: 4, GrammarCount: 4, ABI: abi15(403, 40),
			Counts: classificationGoDefinitionSlotCounts{Cases: 58, Edits: 18, CaseSteps: 76, TreeRoles: 94, PotentialQueryTreeRoles: 376},
		}, true
	case "java":
		return classificationCJRDefinitionProfile{
			Name: "java5-empty", AnchorFile: "java5-classification-anchors.json", AnchorSHA: "41b3011c129007a7ab582e666e3f857fca12cfd756271e5c02358fbff30e451b",
			Route: "java", Case: "java5-empty-definitions", Workload: "java5-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/java.json", QuerySHA: "ca65cb697b4fc3d89de01c2874e3d01360d48f446d6e4e8d6ac0582643998c42",
			Set: "s06-java", Symbol: "tree_sitter_java", RootType: "program", Language: "Java",
			QueryBytes: 5565, QueryCount: 5, GrammarCount: 4, ABI: abi15(323, 40),
			Counts: classificationGoDefinitionSlotCounts{Cases: 40, Edits: 32, CaseSteps: 72, TreeRoles: 104, PotentialQueryTreeRoles: 520},
		}, true
	case "rust":
		return classificationCJRDefinitionProfile{
			Name: "rust4-empty", AnchorFile: "rust4-classification-anchors.json", AnchorSHA: "46cbfe87a0caf2d8ebc43aaca341cfde77e6eda5ae78b0f4909064c1e2cd8569",
			Route: "rust", Case: "rust4-empty-definitions", Workload: "rust4-empty-definition-single-frame",
			QueryFile: "src/testdata/native/queries/rust.json", QuerySHA: "43fb6848561cb11d855c194d617b3deb976c73f715fbf180009e70366da31ab5",
			Set: "s06-rust", Symbol: "tree_sitter_rust", RootType: "source_file", Language: "Rust",
			QueryBytes: 5349, QueryCount: 4, GrammarCount: 5, ABI: abi15(360, 31),
			Counts: classificationGoDefinitionSlotCounts{Cases: 33, Edits: 14, CaseSteps: 47, TreeRoles: 61, PotentialQueryTreeRoles: 244},
		}, true
	default:
		return classificationCJRDefinitionProfile{}, false
	}
}

func classificationCJRAnchorsValid(profile classificationCJRDefinitionProfile, a classificationAnchors) error {
	if a.sha256 != profile.AnchorSHA || a.Schema != "tsgk-query-classification-anchors/r1" || a.Route != profile.Route || a.Case != profile.Case || a.Workload != profile.Workload ||
		a.Operation != "native-query" || a.DriverMode != "single" || a.API || a.Source.Bytes != 0 || a.Source.UTF8 != "" || a.Source.SHA256 != digestHex(nil) ||
		a.Edits == nil || len(a.Edits) != 0 || len(a.Queries) != profile.QueryCount || len(a.Required) != profile.QueryCount || len(a.Nodes) != 1 ||
		!reflect.DeepEqual(a.Expect, []StepExpectation{{Step: 0, Syntax: "NO_ERROR"}}) || a.ABI != profile.ABI {
		return fmt.Errorf("%s empty single-frame anchor changed", profile.Name)
	}
	wantRoot := []TreeNode{{Parent: -1, Type: profile.RootType, Named: true}}
	if !reflect.DeepEqual(a.Nodes, wantRoot) {
		return fmt.Errorf("%s independent empty-source root changed", profile.Name)
	}
	seen := make(map[string]bool, profile.QueryCount)
	for i, query := range a.Queries {
		if query.ID == "" || seen[query.ID] || query.SHA256 != digestHex([]byte(query.Source)) || !classificationGoDefinitionMetadataValid(query.Metadata) {
			return fmt.Errorf("%s definition anchor %d has invalid identity or metadata", profile.Name, i)
		}
		seen[query.ID] = true
		tuple := a.Required[i]
		if tuple.Query != query.ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
			return fmt.Errorf("%s required empty-capture tuple %d changed", profile.Name, i)
		}
	}
	return nil
}

func classificationGoCompilerArgs(route, grammarRoot, output string, grammar []NativeInput) ([]string, error) {
	withScanner := false
	switch route {
	case "go", "csharp", "rust":
		withScanner = true
	case "c", "java":
	default:
		return nil, fmt.Errorf("unsupported closed classification compiler route %q", route)
	}
	want := []NativeInput{{Path: "src/parser.c", Role: "parser"}}
	if withScanner {
		want = append(want, NativeInput{Path: "src/scanner.c", Role: "scanner"})
	}
	want = append(want,
		NativeInput{Path: "src/tree_sitter/alloc.h", Role: "header"},
		NativeInput{Path: "src/tree_sitter/array.h", Role: "header"},
		NativeInput{Path: "src/tree_sitter/parser.h", Role: "header"},
	)
	if len(grammar) != len(want) {
		return nil, fmt.Errorf("%s exact parser/scanner/header closure changed", route)
	}
	for i, pin := range grammar {
		if pin.Path != want[i].Path || pin.Role != want[i].Role || pin.Bytes == 0 || !hex64.MatchString(pin.SHA256) {
			return nil, fmt.Errorf("%s exact parser/scanner/header closure changed", route)
		}
	}
	args := []string{"-shared", "-O0", "-std=gnu11", "-I", filepath.Join(grammarRoot, "src"), filepath.Join(grammarRoot, "src", "parser.c")}
	if withScanner {
		args = append(args, filepath.Join(grammarRoot, "src", "scanner.c"))
	}
	return append(args, "-o", output), nil
}

func classificationCJRDefinitionLoadInputs(a classificationAnchors, plan classificationPlan, inventoryData, qualificationData, queryData []byte) (classificationGoDefinitionInputs, error) {
	var out classificationGoDefinitionInputs
	profile, ok := classificationCJRProfileForRoute(a.Route)
	if !ok {
		return out, fmt.Errorf("unsupported closed C/Java/Rust definition route %q", a.Route)
	}
	if err := classificationCJRAnchorsValid(profile, a); err != nil {
		return out, err
	}
	if plan.Schema != "tsgk-query-classification-plan/r1" || plan.AnchorsSHA256 != profile.AnchorSHA || plan.APIAnchorsSHA256 != "" || plan.NativeContext != "full-input-utf8/r2" ||
		!eligibilityRevision(plan.SourceRevision) || !validID(plan.RunID) || !hex64.MatchString(plan.RuntimeManifestSHA256) || !hex64.MatchString(plan.CompilerSHA256) || len(plan.Grammar) != profile.GrammarCount {
		return out, fmt.Errorf("%s preregistration differs from its empty single-frame profile", profile.Name)
	}
	parserCount := 0
	for _, grammar := range plan.Grammar {
		if grammar.Role == "parser" {
			parserCount++
			if grammar.Bytes == 0 || grammar.Bytes > 16<<20 {
				return out, fmt.Errorf("%s parser exceeds the plain-input context bound", profile.Name)
			}
		}
	}
	if parserCount != 1 {
		return out, fmt.Errorf("%s parser grammar input is missing or duplicated", profile.Name)
	}
	if digestHex(inventoryData) != classificationGoDefinitionInventorySHA {
		return out, fmt.Errorf("query slot inventory pin differs")
	}
	if digestHex(qualificationData) != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("qualification inventory pin differs")
	}
	if len(queryData) != profile.QueryBytes || digestHex(queryData) != profile.QuerySHA {
		return out, fmt.Errorf("%s query source file pin differs", profile.Name)
	}
	var slots classificationGoDefinitionSlotInventory
	if err := jsonv2.Unmarshal(inventoryData, &slots, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("query slot inventory shape: %w", err)
	}
	if slots.Schema != "tsgk-query-slot-inventory/r1" || slots.ExpansionRule != "case-query-edit-tree-role/r1" || !slices.Equal(slots.Coordinate, []string{"qualification_inventory.sha256", "set", "case", "query", "step", "tree_role"}) || len(slots.Ordinary) != 26 || len(slots.SemanticRequired) == 0 || !bytes.Equal(bytes.TrimSpace(slots.SemanticRequired), []byte("null")) {
		return out, fmt.Errorf("query slot inventory r1 contract differs")
	}
	if slots.Qualification.Path != "src/contracts/qualification-c1.json" || slots.Qualification.Bytes != uint64(len(qualificationData)) || slots.Qualification.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("slot qualification pin differs")
	}
	if err := jsonv2.Unmarshal(queryData, &out.queryFile, jsonv2.RejectUnknownMembers(true)); err != nil {
		return out, fmt.Errorf("%s query file shape: %w", profile.Name, err)
	}
	if out.queryFile.Schema != "tsgk-native-query-cases/r1" || out.queryFile.Route != profile.Route || len(out.queryFile.Queries) != profile.QueryCount {
		return out, fmt.Errorf("%s query file identity differs", profile.Name)
	}
	inv, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		return out, fmt.Errorf("qualification inventory: %w", err)
	}
	if inv.SHA256 != classificationGoDefinitionQualificationSHA {
		return out, fmt.Errorf("parsed qualification inventory pin differs")
	}
	var route *QualRoute
	routeCount := 0
	for i := range inv.Routes {
		if inv.Routes[i].Route == profile.Route {
			routeCount++
			route = &inv.Routes[i]
		}
	}
	if routeCount != 1 || route == nil {
		return out, fmt.Errorf("%s qualification route is missing or duplicated", profile.Name)
	}
	w := route.Workload
	if w.Set != profile.Set || w.Profile != profile.Set || w.Operation != "native-query-batch" || w.Output != "tree" || w.Symbol != profile.Symbol || len(w.Grammar) != profile.GrammarCount || len(w.Queries) != profile.QueryCount {
		return out, fmt.Errorf("%s qualification workload is missing, duplicated, or changed", profile.Name)
	}
	if !reflect.DeepEqual(w.Grammar, plan.Grammar) {
		return out, fmt.Errorf("%s qualification and preregistration grammar pins differ", profile.Name)
	}
	for _, extra := range inv.ExtraRoles {
		for _, workload := range extra.Workloads {
			if workload.Route == profile.Route && workload.Set == profile.Set {
				return out, fmt.Errorf("%s workload is duplicated in an extra role", profile.Name)
			}
		}
	}
	groupMatches := 0
	for _, group := range slots.Ordinary {
		if group.Set == profile.Set && (group.Route != profile.Route || group.Profile != profile.Set) {
			return out, fmt.Errorf("%s slot group appears under another route or profile", profile.Name)
		}
		if group.Route == profile.Route && group.Set == profile.Set {
			groupMatches++
			out.slotGroup = group
		}
	}
	if groupMatches != 1 || out.slotGroup.Profile != profile.Set || out.slotGroup.Operation != "native-query-batch" || len(out.slotGroup.Queries) != profile.QueryCount || out.slotGroup.Counts != profile.Counts {
		return out, fmt.Errorf("%s ordinary query group or fixed counts differ", profile.Name)
	}
	sourcePinMatches := 0
	for _, pin := range slots.QuerySources {
		if pin.Path == profile.QueryFile {
			sourcePinMatches++
			if pin.Bytes != uint64(profile.QueryBytes) || pin.SHA256 != profile.QuerySHA {
				return out, fmt.Errorf("%s query source inventory pin differs", profile.Name)
			}
		}
	}
	if sourcePinMatches != 1 {
		return out, fmt.Errorf("%s query source inventory pin is missing or duplicated", profile.Name)
	}
	queryIDs := make([]string, profile.QueryCount)
	for i, anchor := range a.Queries {
		query := out.queryFile.Queries[i]
		registered := w.Queries[i]
		slot := out.slotGroup.Queries[i]
		sha := digestHex([]byte(anchor.Source))
		if !classificationGoDefinitionMetadataValid(anchor.Metadata) || anchor.SHA256 != sha || query.ID != anchor.ID || query.Source != anchor.Source || digestHex([]byte(query.Source)) != sha ||
			registered.ID != anchor.ID || registered.SHA256 != sha || slot.ID != anchor.ID || slot.SHA256 != sha || slot.SourceFile != profile.QueryFile || slot.Class != "STRUCTURAL_CANDIDATE" || slot.CompiledMetadata != "UNVERIFIED" {
			return out, fmt.Errorf("%s query %s is not linked in the exact registered order", profile.Name, anchor.ID)
		}
		queryIDs[i] = anchor.ID
	}
	coordinates, err := classificationGoDefinitionCoordinateProjection(inv.SHA256, profile.Set, w, queryIDs)
	if err != nil {
		return out, err
	}
	coordinates.QuerySlotInventorySHA256 = classificationGoDefinitionInventorySHA
	coordinates.QuerySourceFileSHA256 = profile.QuerySHA
	if coordinates.Cases != profile.Counts.Cases || coordinates.Edits != profile.Counts.Edits || coordinates.CaseSteps != profile.Counts.CaseSteps || coordinates.TreeRoles != profile.Counts.TreeRoles ||
		coordinates.Definitions != uint64(profile.QueryCount) || coordinates.PotentialQueryTreeRoles != profile.Counts.PotentialQueryTreeRoles {
		return out, fmt.Errorf("%s query coordinate denominator differs", profile.Name)
	}
	out.qualification, out.workload, out.coordinates = inv, w, coordinates
	return out, nil
}

func classificationCJRDefinitionRows(a classificationAnchors, inputs classificationGoDefinitionInputs, witnessQueries []QueryWitnessQuery, nativeQueries []querywire.Query, references []*instrumentedReference) ([]classificationGoDefinition, error) {
	profile, ok := classificationCJRProfileForRoute(a.Route)
	if !ok {
		return nil, fmt.Errorf("unsupported closed C/Java/Rust definition route %q", a.Route)
	}
	if err := classificationCJRAnchorsValid(profile, a); err != nil {
		return nil, err
	}
	sourceFiles := make([]string, profile.QueryCount)
	for i := range sourceFiles {
		sourceFiles[i] = profile.QueryFile
	}
	return classificationDefinitionRows(a, inputs, witnessQueries, nativeQueries, references, profile.Language, sourceFiles)
}

type classificationCJRDefinitionRowsFixture struct {
	profile           classificationCJRDefinitionProfile
	anchors           classificationAnchors
	plan              classificationPlan
	inputs            classificationGoDefinitionInputs
	queryData         []byte
	inventoryData     []byte
	qualificationData []byte
	witness           []QueryWitnessQuery
	native            []querywire.Query
	references        []*instrumentedReference
}

// This fixture synthesizes matching metadata only for the row validator. The
// projection test below still requires the live guarded eligibility session.
func classificationCJRDefinitionFixture(t *testing.T, profileName string) classificationCJRDefinitionRowsFixture {
	t.Helper()
	profile, err := classificationCJRProfileForName(profileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSGK_QUERY_CLASSIFICATION_PROFILE", profileName)
	anchors, _ := classificationGoAnchors(t)
	inventoryData := actualJSONRead(t, "../contracts/query-slot-inventory-r1.json")
	qualificationData := actualJSONRead(t, "../contracts/qualification-c1.json")
	queryPath := filepath.Join("..", filepath.FromSlash(strings.TrimPrefix(profile.QueryFile, "src/")))
	queryData := actualJSONRead(t, queryPath)
	qualification, err := ParseQualificationInventory(qualificationData)
	if err != nil {
		t.Fatal(err)
	}
	var route *QualRoute
	for i := range qualification.Routes {
		if qualification.Routes[i].Route == profile.Route {
			route = &qualification.Routes[i]
		}
	}
	if route == nil {
		t.Fatal("C/Java/Rust qualification route missing", profile.Route)
	}
	plan := classificationPlan{
		Schema: "tsgk-query-classification-plan/r1", SourceRevision: strings.Repeat("a", 40), RunID: "cjr-definition-test-run",
		AnchorsSHA256: anchors.sha256, RuntimeManifestSHA256: strings.Repeat("b", 64), NativeContext: anchors.nativeContext(),
		DonorRegistrationSHA256: classificationDonorRegistrationSHA, Grammar: slices.Clone(route.Workload.Grammar), CompilerSHA256: strings.Repeat("c", 64),
	}
	inputs, err := classificationCJRDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		t.Fatal(err)
	}
	witnessRegistration := classificationGoRegistration(anchors)
	witness := slices.Clone(witnessRegistration.Queries)
	native := make([]querywire.Query, profile.QueryCount)
	references := make([]*instrumentedReference, profile.QueryCount)
	for i, query := range anchors.Queries {
		metadata := classificationGoDefinitionCloneMetadata(query.Metadata)
		native[i] = querywire.Query{ID: query.ID, Status: StatusCompleted, Patterns: metadata.Patterns, CaptureNames: metadata.CaptureNames, Predicates: metadata.Predicates}
		reference := &instrumentedReference{Schema: instrumentedReferenceSchema, Status: StatusCompleted}
		reference.Query.ID, reference.Query.SHA256 = query.ID, query.SHA256
		reference.Query.Patterns = metadata.Patterns
		reference.Query.CaptureNames = slices.Clone(metadata.CaptureNames)
		reference.Query.Predicates = slices.Clone(metadata.Predicates)
		references[i] = reference
	}
	return classificationCJRDefinitionRowsFixture{
		profile: profile, anchors: anchors, plan: plan, inputs: inputs, queryData: queryData,
		inventoryData: inventoryData, qualificationData: qualificationData, witness: witness, native: native, references: references,
	}
}

func TestQueryClassificationCJRProfilesAndCompilerClosure(t *testing.T) {
	for _, name := range []string{"c4-empty", "java5-empty", "rust4-empty"} {
		t.Run(name, func(t *testing.T) {
			fixture := classificationCJRDefinitionFixture(t, name)
			p, a := fixture.profile, fixture.anchors
			if err := classificationCJRAnchorsValid(p, a); err != nil || a.Route != p.Route || a.nativeContext() != "full-input-utf8/r2" || a.nativeSymbol() != p.Symbol || a.libraryName() != p.Route+".dll" || classificationNativePrefix(a) != "current/native/" {
				t.Fatalf("closed route selection differs: profile=%#v anchors=%#v err=%v", p, a, err)
			}
			if !classificationPlanContextMatches(a, fixture.plan) || classificationPlanContextMatches(a, classificationPlan{}) {
				t.Fatal("full-input route context was not independently selected")
			}
			registration := classificationGoRegistration(a)
			if _, err := validateQueryWitnessRegistration(registration, queryEligibilityWitnessSchema); err != nil {
				t.Fatal("C/Java/Rust registration rejected", err)
			}
			if registration.Route != p.Route || registration.Input.Bytes != 0 || registration.Input.SHA256 != digestHex(nil) || len(registration.Queries) != p.QueryCount || len(registration.Required) != p.QueryCount {
				t.Fatal("closed empty native query registration differs")
			}
			for i, tuple := range registration.Required {
				if tuple.Query != a.Queries[i].ID || tuple.Step != 0 || tuple.TreeRole != "incremental" || tuple.Structural == nil || len(tuple.Structural) != 0 || tuple.Expected == nil || len(tuple.Expected) != 0 {
					t.Fatalf("empty capture denominator changed at %d", i)
				}
			}
			var registry struct {
				Routes []struct {
					Route string        `json:"route"`
					Files []NativeInput `json:"files"`
				} `json:"routes"`
			}
			actualJSONDecode(t, "../contracts/native-routes.json", &registry)
			var grammar []NativeInput
			matches := 0
			for _, row := range registry.Routes {
				if row.Route == p.Route {
					matches++
					grammar = row.Files
				}
			}
			if matches != 1 || len(grammar) != p.GrammarCount {
				t.Fatal("native route grammar count missing or duplicated", matches, len(grammar))
			}
			root := filepath.Join(t.TempDir(), "prepared", "routes", p.Route)
			output := filepath.Join(t.TempDir(), "case", "library", p.Route+".dll")
			args, err := classificationGoCompilerArgs(p.Route, root, output, grammar)
			if err != nil || !filepath.IsAbs(root) || !filepath.IsAbs(output) || args[len(args)-2] != "-o" || args[len(args)-1] != output {
				t.Fatalf("route compiler/library binding differs: %v %v", args, err)
			}
			hasScanner := slices.Contains(args, filepath.Join(root, "src", "scanner.c"))
			if hasScanner != (p.Route == "rust") {
				t.Fatal("route scanner selection changed", p.Route, args)
			}
			badGrammar := slices.Clone(grammar)
			badGrammar = badGrammar[:len(badGrammar)-1]
			if _, err := classificationGoCompilerArgs(p.Route, root, output, badGrammar); err == nil {
				t.Fatal("incomplete compiler grammar closure accepted")
			}
		})
	}
	if _, ok := classificationCJRProfileForRoute("cpp"); ok {
		t.Fatal("C++ static candidate entered the closed C/Java/Rust profile set")
	}
	if _, err := classificationCJRProfileForName("unknown"); err == nil {
		t.Fatal("unknown C/Java/Rust profile accepted")
	}
	unknown := classificationAnchors{Route: "unknown"}
	if classificationPlanContextMatches(unknown, classificationPlan{}) || unknown.nativeContext() != "" || unknown.nativeSymbol() != "" || classificationNativePrefix(unknown) != "" {
		t.Fatal("unknown route selected a classification context, symbol, or capture namespace")
	}
}

func TestQueryClassificationCJRInventoryAggregate(t *testing.T) {
	var aggregate classificationGoDefinitionSlotCounts
	definitions := 0
	for _, route := range []string{"c", "java", "rust"} {
		profile, ok := classificationCJRProfileForRoute(route)
		if !ok {
			t.Fatal("closed C/Java/Rust profile missing", route)
		}
		definitions += profile.QueryCount
		aggregate.Cases += profile.Counts.Cases
		aggregate.Edits += profile.Counts.Edits
		aggregate.CaseSteps += profile.Counts.CaseSteps
		aggregate.TreeRoles += profile.Counts.TreeRoles
		aggregate.PotentialQueryTreeRoles += profile.Counts.PotentialQueryTreeRoles
	}
	if definitions != 13 || aggregate != (classificationGoDefinitionSlotCounts{Cases: 131, Edits: 64, CaseSteps: 195, TreeRoles: 259, PotentialQueryTreeRoles: 1140}) {
		t.Fatalf("C/Java/Rust ordinary denominator changed: definitions=%d counts=%#v", definitions, aggregate)
	}
}

func TestQueryClassificationCJRDefinitionBindings(t *testing.T) {
	for _, name := range []string{"c4-empty", "java5-empty", "rust4-empty"} {
		t.Run(name, func(t *testing.T) {
			f := classificationCJRDefinitionFixture(t, name)
			rows, err := classificationCJRDefinitionRows(f.anchors, f.inputs, f.witness, f.native, f.references)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != f.profile.QueryCount || f.inputs.coordinates.Cases != f.profile.Counts.Cases || f.inputs.coordinates.Edits != f.profile.Counts.Edits ||
				f.inputs.coordinates.CaseSteps != f.profile.Counts.CaseSteps || f.inputs.coordinates.TreeRoles != f.profile.Counts.TreeRoles || f.inputs.coordinates.Definitions != uint64(f.profile.QueryCount) ||
				f.inputs.coordinates.PotentialQueryTreeRoles != f.profile.Counts.PotentialQueryTreeRoles || f.inputs.coordinates.QuerySourceFileSHA256 != f.profile.QuerySHA {
				t.Fatalf("closed inventory denominator differs: %#v", f.inputs.coordinates)
			}
			for i, row := range rows {
				want := f.anchors.Queries[i]
				if row.Query != want.ID || row.Source != want.Source || row.SourceSHA256 != want.SHA256 || row.SourceFile != f.profile.QueryFile || !reflect.DeepEqual(row.Metadata, want.Metadata) ||
					row.Class != "STRUCTURAL_ONLY" || row.InventoryClass != "STRUCTURAL_CANDIDATE" || row.CompiledMetadata != "UNVERIFIED" || row.Assessment != "NOT_ASSESSED" || row.SemanticRequired != nil || row.SemanticEvaluated || row.PotentialTreeRoles != f.profile.Counts.TreeRoles {
					t.Fatalf("C/Java/Rust row %d changed its static state or source binding: %#v", i, row)
				}
			}
			if _, err := classificationCJRDefinitionProjection(f.anchors, f.plan, nil, nil, QueryReferenceEligibilityResult{}, f.inventoryData, f.qualificationData, f.queryData, strings.Repeat("d", 40)); err == nil || !strings.Contains(err.Error(), "live guarded eligibility session") {
				t.Fatalf("synthetic metadata projection passed without the live guard: %v", err)
			}

			badInputs := f.inputs
			badInputs.slotGroup.Queries = slices.Clone(f.inputs.slotGroup.Queries)
			badInputs.slotGroup.Queries[0].SourceFile = "src/testdata/native/queries/go.json"
			if _, err := classificationCJRDefinitionRows(f.anchors, badInputs, f.witness, f.native, f.references); err == nil {
				t.Fatal("aliased query source path accepted")
			}
			badInputs = f.inputs
			badInputs.slotGroup.Queries = slices.Clone(f.inputs.slotGroup.Queries)
			badInputs.slotGroup.Queries[0].ID, badInputs.slotGroup.Queries[1].ID = badInputs.slotGroup.Queries[1].ID, badInputs.slotGroup.Queries[0].ID
			if _, err := classificationCJRDefinitionRows(f.anchors, badInputs, f.witness, f.native, f.references); err == nil {
				t.Fatal("reordered query inventory accepted")
			}
			badInputs = f.inputs
			badInputs.queryFile.Queries = slices.Clone(f.inputs.queryFile.Queries)
			badInputs.queryFile.Queries[0].ID, badInputs.queryFile.Queries[1].ID = badInputs.queryFile.Queries[1].ID, badInputs.queryFile.Queries[0].ID
			if _, err := classificationCJRDefinitionRows(f.anchors, badInputs, f.witness, f.native, f.references); err == nil {
				t.Fatal("reordered source-file query definitions accepted")
			}
			if _, err := classificationCJRDefinitionRows(f.anchors, f.inputs, f.witness[:len(f.witness)-1], f.native, f.references); err == nil {
				t.Fatal("missing witness definition accepted")
			}
			badReferences := slices.Clone(f.references)
			badReferences[0] = nil
			if _, err := classificationCJRDefinitionRows(f.anchors, f.inputs, f.witness, f.native, badReferences); err == nil {
				t.Fatal("missing reference metadata accepted")
			}
			badWitness := slices.Clone(f.witness)
			badMetadata := *badWitness[0].Metadata
			badMetadata.Predicates = nil
			badWitness[0].Metadata = &badMetadata
			if _, err := classificationCJRDefinitionRows(f.anchors, f.inputs, badWitness, f.native, f.references); err == nil {
				t.Fatal("null witness predicate list accepted")
			}
			badNative := slices.Clone(f.native)
			badNative[0].Predicates = []QueryPredicate{{}}
			if _, err := classificationCJRDefinitionRows(f.anchors, f.inputs, f.witness, badNative, f.references); err == nil {
				t.Fatal("nonempty native predicate list accepted")
			}

			badAnchors := f.anchors
			badAnchors.ABI.SymbolCount++
			if _, err := classificationCJRDefinitionLoadInputs(badAnchors, f.plan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
				t.Fatal("changed producer ABI accepted")
			}
			badPlan := f.plan
			badPlan.Grammar = slices.Clone(f.plan.Grammar)
			badPlan.Grammar[0].Bytes++
			if _, err := classificationCJRDefinitionLoadInputs(f.anchors, badPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
				t.Fatal("changed grammar pin accepted")
			}
			badPlan = f.plan
			badPlan.NativeContext = semanticPagedContext
			if _, err := classificationCJRDefinitionLoadInputs(f.anchors, badPlan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
				t.Fatal("paged parser context accepted for a plain-input route")
			}
			badAnchors = f.anchors
			badAnchors.API = true
			if _, err := classificationCJRDefinitionLoadInputs(badAnchors, f.plan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
				t.Fatal("API-enabled CJR anchor accepted")
			}
			badAnchors = f.anchors
			badAnchors.Queries = slices.Clone(f.anchors.Queries)
			badAnchors.Queries[0].Metadata.Predicates = nil
			if _, err := classificationCJRDefinitionLoadInputs(badAnchors, f.plan, f.inventoryData, f.qualificationData, f.queryData); err == nil {
				t.Fatal("null anchor predicate list accepted")
			}
			badQuery := slices.Clone(f.queryData)
			badQuery[len(badQuery)-1] ^= 1
			if _, err := classificationCJRDefinitionLoadInputs(f.anchors, f.plan, f.inventoryData, f.qualificationData, badQuery); err == nil {
				t.Fatal("changed route query source bytes accepted")
			}
			badInventory := slices.Clone(f.inventoryData)
			badInventory[len(badInventory)-1] ^= 1
			if _, err := classificationCJRDefinitionLoadInputs(f.anchors, f.plan, badInventory, f.qualificationData, f.queryData); err == nil {
				t.Fatal("inventory r1 drift accepted")
			}
		})
	}
}

func TestQueryClassificationCJRStderrBinding(t *testing.T) {
	for _, route := range []string{"c", "java", "rust"} {
		for _, mismatch := range []string{"", "nonzero-present", "missing-zero", "missing-nonzero", "owner", "count", "role"} {
			t.Run(route+"/"+mismatch, func(t *testing.T) {
				a := classificationAnchors{Route: route}
				b := semanticTestBudget(t)
				path, id := classificationNativePrefix(a)+"capture/stderr.bin", "native-fixture"
				data := []byte{}
				if mismatch == "nonzero-present" || mismatch == "missing-nonzero" || mismatch == "owner" || mismatch == "count" {
					data = []byte("x")
				}
				pin := ReplayMember{Path: "capture/stderr.bin", Role: "stderr", Bytes: uint64(len(data)), SHA256: digestHex(data)}
				mapped := pin
				mapped.Path = path
				member := eligibilityMember{NodeID: id, Member: mapped}
				s := &eligibilitySession{r: b.r, b: b, res: &QueryReferenceEligibilityResult{}, owned: map[string]eligibilityMember{path: member}, owners: map[string]string{path: id}, snapshot: &evidenceSnapshot{files: map[string][]byte{path: data}, verifiedMembers: map[string]ReplayMember{path: mapped}}}
				capture := semanticProcessCapture{Stderr: &pin}
				process := eligibilityExecution{StderrBytes: uint64(len(data))}
				switch mismatch {
				case "missing-zero", "missing-nonzero":
					capture.Stderr = nil
				case "owner":
					member.NodeID = "another-native"
					s.owned[path], s.owners[path] = member, member.NodeID
				case "count":
					process.StderrBytes++
				case "role":
					capture.Stderr.Role = "stdout-frame"
				}
				read := func(pin *ReplayMember) ([]byte, error) {
					mapped := *pin
					mapped.Path = classificationNativePrefix(a) + mapped.Path
					data, ok, err := s.member(&eligibilityMember{NodeID: id, Member: mapped})
					if err != nil || !ok {
						return nil, fmt.Errorf("guarded stderr read: %v", err)
					}
					return data, nil
				}
				err := classificationCaptureStderr(a, capture, process, read)
				wantPass := mismatch == "" || mismatch == "nonzero-present"
				if (err == nil) != wantPass {
					t.Fatalf("C/Java/Rust stderr binding %s/%s: %v", route, mismatch, err)
				}
			})
		}
	}
	if err := classificationCaptureStderr(classificationAnchors{Route: "cpp"}, semanticProcessCapture{}, eligibilityExecution{}, nil); err == nil {
		t.Fatal("C++ candidate or unknown route selected the classification stderr policy")
	}
}

func classificationCJRDefinitionProjection(anchors classificationAnchors, plan classificationPlan, s *eligibilitySession, cache *semanticNativeCache, core QueryReferenceEligibilityResult, inventoryData, qualificationData, queryData []byte, readerRevision string) (classificationGoDefinitionProjectionResult, error) {
	var result classificationGoDefinitionProjectionResult
	profile, ok := classificationCJRProfileForRoute(anchors.Route)
	if !ok {
		return result, fmt.Errorf("unsupported closed C/Java/Rust definition route %q", anchors.Route)
	}
	inputs, err := classificationCJRDefinitionLoadInputs(anchors, plan, inventoryData, qualificationData, queryData)
	if err != nil {
		return result, err
	}
	if !hex40.MatchString(readerRevision) {
		return result, fmt.Errorf("separately pinned reader revision is invalid")
	}
	if s == nil || cache == nil || s.res == nil || s.r == nil || s.semantic != cache || !s.bound || !cache.ready || !cache.record.haveRaw {
		return result, fmt.Errorf("C/Java/Rust definition projection requires the live guarded eligibility session")
	}
	if err := s.r.check(); err != nil {
		return result, err
	}
	wantTuple := QueryEligibilityCounts{Required: uint64(profile.QueryCount), Verified: uint64(profile.QueryCount)}
	if core.ResultSchema != QueryReferenceEligibilityResultSchema || core.ExecutionStatus != StatusCompleted || core.Assessment != AssessPass || !core.Complete || core.Tuple != wantTuple ||
		s.res.ResultSchema != core.ResultSchema || s.res.ExecutionStatus != core.ExecutionStatus || s.res.Assessment != core.Assessment || s.res.Complete != core.Complete || s.res.Tuple != wantTuple ||
		s.res.ApprovalSHA256 != core.ApprovalSHA256 || s.res.RegistrationSHA256 != core.RegistrationSHA256 || s.res.WitnessSHA256 != core.WitnessSHA256 || s.res.CatalogueSHA256 != core.CatalogueSHA256 || s.res.GraphSHA256 != core.GraphSHA256 {
		return result, fmt.Errorf("C/Java/Rust definition projection requires the same completed guarded tuple")
	}
	if !classificationLibraryMatches(s) {
		return result, fmt.Errorf("C/Java/Rust definition reference library differs from guarded compiler output")
	}
	if plan.NativeContext != anchors.nativeContext() || plan.NativeContext != "full-input-utf8/r2" || s.reg.Context != plan.NativeContext || s.reg.Native == nil || s.reg.Native.Symbol != profile.Symbol || s.reg.Native.InvocationID != "native-"+anchors.Case ||
		s.witness.Schema != queryEligibilityWitnessSchema || s.witness.Policy != "tsgk-predicates/r1" || s.witness.Case != anchors.Case || s.witness.Route != profile.Route || s.witness.Role != "owned" || s.witness.Encoding != EncodingUTF8 || s.witness.Platform != s.approval.Candidate.Platform ||
		s.witness.Input.Path != "source.bin" || s.witness.Input.Bytes != 0 || s.witness.Input.SHA256 != anchors.Source.SHA256 || s.witness.Input.Role != "case" ||
		s.witness.Native.RequestID != anchors.Case || s.witness.Native.API || s.witness.Native.ProducerABI != anchors.ABI || s.witness.Native.RuntimeManifestSHA256 != plan.RuntimeManifestSHA256 || s.witness.Native.ObservationRunID != plan.RunID || len(s.witness.Native.Points) != 0 ||
		plan.SourceRevision != s.approval.Candidate.SourceRevision || plan.RunID != s.approval.Candidate.Run || s.approval.Candidate.Platform != "windows/amd64" || s.approval.Candidate.Attempt != 1 ||
		!reflect.DeepEqual(s.witness.Required, anchors.Required) || !reflect.DeepEqual(s.witness.Edits, []Edit{}) || len(s.witness.Queries) != profile.QueryCount || len(s.reg.Native.Grammar) != len(plan.Grammar) {
		return result, fmt.Errorf("%s guarded candidate, full-input context, or witness differs from the fixed anchor", profile.Name)
	}
	if err := classificationGrammarContext(s, anchors, plan); err != nil {
		return result, fmt.Errorf("%s guarded grammar pin differs: %w", profile.Name, err)
	}
	if len(cache.checked) != 1 || cache.checked[0].Step.Step != 0 || cache.checked[0].Step.SourceBytes != 0 || cache.checked[0].Step.SourceSHA256 != anchors.Source.SHA256 ||
		cache.checked[0].Step.Edit != nil || cache.checked[0].Fresh != nil || CompareTrees(cache.checked[0].Incremental.Nodes, anchors.Nodes) != nil || len(cache.checked[0].Incremental.Queries) != profile.QueryCount {
		return result, fmt.Errorf("%s guarded empty single-frame query result differs from the fixed definition scope", profile.Name)
	}
	references := make(map[string]*instrumentedReference, len(anchors.Required))
	referenceIDs := make(map[string]string, len(anchors.Required))
	var referenceExecutableSHA string
	for _, tuple := range anchors.Required {
		observation, found := s.observation(tuple)
		if !found || observation.Query != tuple.Query || observation.QueryID != tuple.Query || observation.Step != 0 || observation.TreeRole != "incremental" || observation.SourceBytes != 0 || observation.SourceSHA256 != anchors.Source.SHA256 {
			return result, fmt.Errorf("%s guarded reference observation differs: %s", profile.Name, tuple.Query)
		}
		out := s.outputs[observation.InvocationID]
		if out == nil || observation.ResponseOrdinal >= uint64(len(out.responses)) || observation.ResponseOrdinal >= uint64(len(out.rows)) || out.responses[observation.ResponseOrdinal] == nil {
			return result, fmt.Errorf("%s guarded reference response is missing: %s", profile.Name, tuple.Query)
		}
		response := out.responses[observation.ResponseOrdinal]
		row := out.rows[observation.ResponseOrdinal]
		if response.Schema != instrumentedReferenceSchema || response.Status != StatusCompleted || response.Code != "" || response.Input.Encoding != EncodingUTF8 || response.Input.Bytes != 0 || response.Input.SHA256 != anchors.Source.SHA256 ||
			response.Grammar.Symbol != profile.Symbol || CompareTrees(response.Tree.Nodes, anchors.Nodes) != nil || uint64(len(row)) != observation.ByteLength || digestHex(row) != observation.SHA256 {
			return result, fmt.Errorf("%s guarded reference source frame or complete CST differs: %s", profile.Name, tuple.Query)
		}
		invocation := s.invocations[observation.InvocationID]
		if invocation == nil || invocation.Executable == nil {
			return result, fmt.Errorf("%s guarded reference executable is missing: %s", profile.Name, tuple.Query)
		}
		if referenceExecutableSHA == "" {
			referenceExecutableSHA = invocation.Executable.Member.SHA256
		} else if referenceExecutableSHA != invocation.Executable.Member.SHA256 {
			return result, fmt.Errorf("%s reference executable bindings differ", profile.Name)
		}
		if !hex64.MatchString(referenceExecutableSHA) {
			return result, fmt.Errorf("%s reference executable pin is invalid", profile.Name)
		}
		references[tuple.Query] = response
		referenceIDs[tuple.Query] = observation.InvocationID
	}
	orderedReferences := make([]*instrumentedReference, profile.QueryCount)
	orderedReferenceIDs := make([]string, profile.QueryCount)
	for i, query := range anchors.Queries {
		orderedReferences[i], orderedReferenceIDs[i] = references[query.ID], referenceIDs[query.ID]
		if orderedReferences[i] == nil || orderedReferenceIDs[i] == "" {
			return result, fmt.Errorf("%s guarded reference query is missing: %s", profile.Name, query.ID)
		}
	}
	definitions, err := classificationCJRDefinitionRows(anchors, inputs, s.witness.Queries, cache.checked[0].Incremental.Queries, orderedReferences)
	if err != nil {
		return result, err
	}
	nativeInvocation, err := classificationNativeInvocation(s, anchors)
	if err != nil {
		return result, err
	}
	libraryPin, libraryOK := s.owned["current/go.dll"]
	if nativeInvocation == nil || nativeInvocation.Executable == nil || nativeInvocation.Executable.Member.SHA256 != s.witness.Native.ExecutableSHA256 || !hex64.MatchString(nativeInvocation.Executable.Member.SHA256) {
		return result, fmt.Errorf("%s guarded native driver executable is missing or mismatched", profile.Name)
	}
	if !libraryOK || libraryPin.NodeID != "go-library-build" || libraryPin.Member.Role != "runtime-library" || !hex64.MatchString(libraryPin.Member.SHA256) || s.reg.Build == nil || s.reg.Native.ReferenceLibrary == nil || !hex64.MatchString(s.reg.Native.ReferenceLibrary.Member.SHA256) {
		return result, fmt.Errorf("%s guarded build or route library binding is missing", profile.Name)
	}
	libraryNode, found := s.nodes["go-library-build"]
	if !found || !classificationGoLibraryNodeMatches(libraryNode, plan, anchors.Workload) || libraryNode.Identities["tool"] != plan.CompilerSHA256 {
		return result, fmt.Errorf("%s guarded compiler node differs from preregistration", profile.Name)
	}
	for _, pin := range []string{s.reg.Build.Identity.Member.SHA256, s.reg.Build.SourceSnapshot.Member.SHA256, s.reg.Build.VendorSnapshot.Member.SHA256} {
		if !hex64.MatchString(pin) {
			return result, fmt.Errorf("%s guarded build identity pin is invalid", profile.Name)
		}
	}
	result = classificationGoDefinitionProjectionResult{
		Scope: fmt.Sprintf("conditional historical classification of %d predicate-free %s definitions and %d potential %s links; one empty-source full-input native-query frame is the guard, not per-case or semantic completion", profile.QueryCount, profile.Language, inputs.coordinates.PotentialQueryTreeRoles, profile.Set),
		Guard: classificationGoDefinitionGuardBinding{
			ReaderRevision: readerRevision, NativeContext: plan.NativeContext, ObservationSourceRevision: plan.SourceRevision, ObservationRunID: plan.RunID,
			ObservationAttempt: s.approval.Candidate.Attempt, ObservationPlatform: s.approval.Candidate.Platform,
			Case: anchors.Case, Route: anchors.Route, Workload: anchors.Workload, Operation: anchors.Operation,
			NativeInvocationID: nativeInvocation.ID, ReferenceInvocationIDs: orderedReferenceIDs,
			CaseSourceSHA256: anchors.Source.SHA256, AnchorsSHA256: anchors.sha256, RuntimeManifestSHA256: plan.RuntimeManifestSHA256,
			ProducerABI: s.witness.Native.ProducerABI, DriverSHA256: nativeInvocation.Executable.Member.SHA256, ReferenceExecutableSHA256: referenceExecutableSHA,
			ReferenceLibrarySHA256: s.reg.Native.ReferenceLibrary.Member.SHA256, CurrentLibrarySHA256: libraryPin.Member.SHA256, CompilerSHA256: libraryNode.Identities["tool"],
			BuildIdentitySHA256: s.reg.Build.Identity.Member.SHA256, SourceSnapshotSHA256: s.reg.Build.SourceSnapshot.Member.SHA256, VendorSnapshotSHA256: s.reg.Build.VendorSnapshot.Member.SHA256,
			Grammar: slices.Clone(plan.Grammar), ApprovalSHA256: core.ApprovalSHA256, RegistrationSHA256: core.RegistrationSHA256, WitnessSHA256: core.WitnessSHA256,
			CatalogueSHA256: core.CatalogueSHA256, GraphSHA256: core.GraphSHA256, Provenance: slices.Clone(s.approval.Provenance), NativeObservationCreated: false,
			EligibilityResultSchema: core.ResultSchema, EligibilityExecutionStatus: core.ExecutionStatus, EligibilityAssessment: core.Assessment, EligibilityComplete: core.Complete, EligibilityTuple: core.Tuple,
		},
		Inventory: inputs.coordinates, Definitions: definitions,
	}
	return result, nil
}
