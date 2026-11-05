package kit

import "fmt"

func classificationDartPHP12ProfileName(name string) bool {
	_, err := classificationDartPHP12ProfileForName(name)
	return err == nil
}

func classificationDartPHP12ProfileForName(name string) (classificationEmptyDefinitionProfile, error) {
	route := map[string]string{"dart7-empty": "dart", "php5-empty": "php"}[name]
	profile, ok := classificationDartPHP12ProfileForRoute(route)
	if !ok || profile.Name != name {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("unknown closed DartPHP12 profile %q", name)
	}
	return profile, nil
}

func classificationDartPHP12ProfileForRoute(route string) (classificationEmptyDefinitionProfile, bool) {
	profile := classificationEmptyDefinitionProfile{Route: route, Set: "s06-" + route, Symbol: "tree_sitter_" + route, RootType: "program", QueryFile: "src/testdata/native/queries/" + route + ".json", ABI: eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13}}
	switch route {
	case "dart":
		profile.Name, profile.Language = "dart7-empty", "Dart"
		profile.AnchorFile, profile.AnchorSHA = "dart7-classification-anchors.json", "7dc0fe53780198aae6984b6ecec896029bb52ed8ed1b2179a26f45b5a4948f6e"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 10437, SHA256: "c853a16a12fe1d51eab89779146541dc0442c8b80f98fe15bd17051d1b343755"}
		profile.QueryIDs = []string{"q.structure", "q.probe-dart-B02", "q.probe-dart-V213", "q.probe-dart-V30", "q.probe-dart-V36", "q.probe-dart-V310", "q.probe-dart-L01"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 20, 20, 19
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 533, 22
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 53, Edits: 24, CaseSteps: 77, TreeRoles: 101, PotentialQueryTreeRoles: 707}
		profile.GrammarShape = []classificationEmptyGrammarFile{{Path: "src/parser.c", Role: "parser"}, {Path: "src/scanner.c", Role: "scanner"}, {Path: "src/tree_sitter/alloc.h", Role: "header"}, {Path: "src/tree_sitter/array.h", Role: "header"}, {Path: "src/tree_sitter/parser.h", Role: "header"}}
		profile.CompilerIncludeDirs = []string{"src"}
		profile.CompilerSourceFiles = []string{"src/parser.c", "src/scanner.c"}
	case "php":
		profile.Name, profile.Language = "php5-empty", "PHP"
		profile.AnchorFile, profile.AnchorSHA = "php5-classification-anchors.json", "55f21a99a80dc6f5f895caff66733a1ff67cb9f9a1d144aa3c61718941a27225"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 7603, SHA256: "4d2e3d204fa36c01eea491a21ec06eb39f7dba2bc30c0038983f1299f8e75a4a"}
		profile.QueryIDs = []string{"q.structure", "q.probe-v56", "q.probe-v72", "q.probe-v82", "q.probe-v84"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 22, 23, 22
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 437, 31
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 48, Edits: 46, CaseSteps: 94, TreeRoles: 140, PotentialQueryTreeRoles: 700}
		profile.GrammarShape = []classificationEmptyGrammarFile{{Path: "common/scanner.h", Role: "header"}, {Path: "php/src/parser.c", Role: "parser"}, {Path: "php/src/scanner.c", Role: "scanner"}, {Path: "php/src/tree_sitter/alloc.h", Role: "header"}, {Path: "php/src/tree_sitter/array.h", Role: "header"}, {Path: "php/src/tree_sitter/parser.h", Role: "header"}}
		profile.CompilerIncludeDirs = []string{"php/src", "common"}
		profile.CompilerSourceFiles = []string{"php/src/parser.c", "php/src/scanner.c"}
	default:
		return classificationEmptyDefinitionProfile{}, false
	}
	profile.Case = profile.Name + "-definitions"
	profile.Workload = profile.Name + "-definition-single-frame"
	profile.QueryCount = len(profile.QueryIDs)
	return profile, true
}
