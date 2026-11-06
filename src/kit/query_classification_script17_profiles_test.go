package kit

import "fmt"

func classificationScript17ProfileName(name string) bool {
	_, err := classificationScript17ProfileForName(name)
	return err == nil
}

func classificationScript17ProfileForName(name string) (classificationEmptyDefinitionProfile, error) {
	routes := map[string]string{
		"python8-empty": "python",
		"ruby4-empty":   "ruby",
		"r3-empty":      "r",
		"bash2-empty":   "bash",
	}
	route := routes[name]
	if route == "" {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("unknown closed Script17 profile %q", name)
	}
	profile, ok := classificationScript17ProfileForRoute(route)
	if !ok || profile.Name != name {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("closed Script17 profile binding is missing: %q", name)
	}
	return profile, nil
}

func classificationEmptyProfileForName(name string) (classificationEmptyDefinitionProfile, error) {
	if profile, err := classificationWeb16ProfileForName(name); err == nil {
		return profile, nil
	}
	if profile, err := classificationScript17ProfileForName(name); err == nil {
		return profile, nil
	}
	if profile, err := classificationDartPHP12ProfileForName(name); err == nil {
		return profile, nil
	}
	return classificationPowerShell3ProfileForName(name)
}

func classificationEmptyProfileForRoute(route string) (classificationEmptyDefinitionProfile, bool) {
	if profile, ok := classificationWeb16ProfileForRoute(route); ok {
		return profile, true
	}
	if profile, ok := classificationScript17ProfileForRoute(route); ok {
		return profile, true
	}
	if profile, ok := classificationDartPHP12ProfileForRoute(route); ok {
		return profile, true
	}
	return classificationPowerShell3ProfileForRoute(route)
}

func classificationScript17ProfileForRoute(route string) (classificationEmptyDefinitionProfile, bool) {
	profile := classificationEmptyDefinitionProfile{
		Route: route, Set: "s06-" + route, Symbol: "tree_sitter_" + route,
		RootType: "program", QueryFile: "src/testdata/native/queries/" + route + ".json",
		ABI: eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13},
		GrammarShape: []classificationEmptyGrammarFile{
			{Path: "src/parser.c", Role: "parser"},
			{Path: "src/scanner.c", Role: "scanner"},
			{Path: "src/tree_sitter/alloc.h", Role: "header"},
			{Path: "src/tree_sitter/array.h", Role: "header"},
			{Path: "src/tree_sitter/parser.h", Role: "header"},
		},
		CompilerIncludeDirs: []string{"src"},
		CompilerSourceFiles: []string{"src/parser.c", "src/scanner.c"},
	}
	switch route {
	case "python":
		profile.Name, profile.Language = "python8-empty", "Python"
		profile.AnchorFile, profile.AnchorSHA = "python8-classification-anchors.json", "a36dfdd9e2d2e517cf6bd6d41ece832042debb6d30d13eb1de57f81d3aff5b3b"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 9291, SHA256: "76ed74d8a6176f11165842a097766e48bcecdae36f66d0ef60baea7dc4542ad5"}
		profile.QueryIDs = []string{"q.structure", "q.probe-b02", "q.probe-v38", "q.probe-v39", "q.probe-v310", "q.probe-v311", "q.probe-v313", "q.probe-v314"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 23, 26, 21
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 284, 32
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 49, Edits: 60, CaseSteps: 109, TreeRoles: 169, PotentialQueryTreeRoles: 1352}
		profile.RootType = "module"
	case "ruby":
		profile.Name, profile.Language = "ruby4-empty", "Ruby"
		profile.AnchorFile, profile.AnchorSHA = "ruby4-classification-anchors.json", "abec8ccca94f26f4d258468eefb8eed6ecd2b04d67926e58a9b3288698facec9"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 6291, SHA256: "265972373a9dedd0650675954b659f35348e929fc8d1791038c9e3ad37fbc8c4"}
		profile.QueryIDs = []string{"q.structure", "q.probe-b01", "q.probe-v27", "q.probe-v31"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 17, 19, 18
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 354, 32
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 30, Edits: 34, CaseSteps: 64, TreeRoles: 98, PotentialQueryTreeRoles: 392}
	case "r":
		profile.Name, profile.Language = "r3-empty", "R"
		profile.AnchorFile, profile.AnchorSHA = "r3-classification-anchors.json", "35a94bab40962b5d8467a8031728f1b9b20cb095fca289b96ebbbcc62671f6c8"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 5850, SHA256: "5fae6fb13622e4eb18bde54a89436af437b1eb874f6be1f20040ad71eba42fb3"}
		profile.QueryIDs = []string{"q.structure", "q.probe-v40", "q.probe-v42"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 7, 8, 7
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 144, 20
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 17, Edits: 28, CaseSteps: 45, TreeRoles: 73, PotentialQueryTreeRoles: 219}
	case "bash":
		profile.Name, profile.Language = "bash2-empty", "Bash"
		profile.AnchorFile, profile.AnchorSHA = "bash2-classification-anchors.json", "90e8c57d2ed15d2659c2f4e2b38c6609df5ff167fe5e4ac32bf542abe90df032"
		profile.QueryFilePin = classificationGoDefinitionQuerySourcePin{Path: profile.QueryFile, Bytes: 3186, SHA256: "996b3b155f5fb9a534fe26c8b103fc09a9b1b5699e8c6bb382efd5356b9da359"}
		profile.QueryIDs = []string{"q.structure", "q.probe-v4"}
		profile.Patterns, profile.CaptureNameEntries, profile.UniqueCaptureNames = 5, 6, 6
		profile.ABI.SymbolCount, profile.ABI.FieldCount = 293, 19
		profile.Counts = classificationGoDefinitionSlotCounts{Cases: 31, Edits: 18, CaseSteps: 49, TreeRoles: 67, PotentialQueryTreeRoles: 134}
	default:
		return classificationEmptyDefinitionProfile{}, false
	}
	profile.Case = profile.Name + "-definitions"
	profile.Workload = profile.Name + "-definition-single-frame"
	profile.QueryCount = len(profile.QueryIDs)
	return profile, true
}
