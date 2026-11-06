package kit

import "fmt"

func classificationPowerShell3ProfileName(name string) bool {
	_, err := classificationPowerShell3ProfileForName(name)
	return err == nil
}

func classificationPowerShell3ProfileForName(name string) (classificationEmptyDefinitionProfile, error) {
	profile, ok := classificationPowerShell3ProfileForRoute("powershell")
	if !ok || profile.Name != name {
		return classificationEmptyDefinitionProfile{}, fmt.Errorf("unknown closed PowerShell3 profile %q", name)
	}
	return profile, nil
}

// classificationPowerShell3ExpectedNodes is the pre-observation static CST of the one-byte source ";".
// An observed CST that differs requires a new preregistered profile, never an edit of these pins.
func classificationPowerShell3ExpectedNodes() []TreeNode {
	node := func(parent int64, typ string, named bool) TreeNode {
		return TreeNode{Parent: parent, Type: typ, Named: named, EndByte: 1, EndPoint: Point{Column: 1}}
	}
	return []TreeNode{node(-1, "program", true), node(0, "statement_list", true), node(1, "empty_statement", true), node(2, ";", false)}
}

func classificationPowerShell3ProfileForRoute(route string) (classificationEmptyDefinitionProfile, bool) {
	if route != "powershell" {
		return classificationEmptyDefinitionProfile{}, false
	}
	profile := classificationEmptyDefinitionProfile{
		Name: "powershell3-tiny", Route: route, Set: "s06-powershell", Symbol: "tree_sitter_powershell", RootType: "program", Language: "PowerShell",
		QueryFile:  "src/testdata/native/queries/powershell.json",
		AnchorFile: "powershell3-classification-anchors.json", AnchorSHA: "624ba74b07070fd18ceab2c8583ecff84e3433e2b32f7d7de37bf4387c45513e",
		QueryFilePin:       classificationGoDefinitionQuerySourcePin{Path: "src/testdata/native/queries/powershell.json", Bytes: 3451, SHA256: "d53b1588703d33fb033cbbc87d5f8e0508e05dc1038ecd942b6665ebc53eb79a"},
		QueryIDs:           []string{"q.structure", "q.probe-v7-chain", "q.probe-v7-blocks"},
		Patterns:           6,
		CaptureNameEntries: 6,
		UniqueCaptureNames: 6,
		ABI:                eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 445, FieldCount: 16},
		Counts:             classificationGoDefinitionSlotCounts{Cases: 39, Edits: 24, CaseSteps: 63, TreeRoles: 87, PotentialQueryTreeRoles: 261},
		GrammarShape: []classificationEmptyGrammarFile{
			{Path: "src/parser.c", Role: "parser"}, {Path: "src/scanner.c", Role: "scanner"},
			{Path: "src/tree_sitter/alloc.h", Role: "header"}, {Path: "src/tree_sitter/array.h", Role: "header"}, {Path: "src/tree_sitter/parser.h", Role: "header"},
		},
		CompilerIncludeDirs: []string{"src"},
		CompilerSourceFiles: []string{"src/parser.c", "src/scanner.c"},
		SourceUTF8:          ";",
		Nodes:               classificationPowerShell3ExpectedNodes(),
	}
	profile.Case = "powershell3-tiny-definitions"
	profile.Workload = "powershell3-tiny-definition-single-frame"
	profile.QueryCount = len(profile.QueryIDs)
	return profile, true
}
