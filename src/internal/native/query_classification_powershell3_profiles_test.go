package native

import "github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"

func classificationPowerShell3ProfileName(name string) bool {
	_, ok := classificationPowerShell3ProfileFor(name)
	return ok
}

// classificationPowerShell3ExpectedNodes is the pre-observation static CST of the one-byte source ";".
// An observed CST that differs requires a new preregistered profile, never an edit of these pins.
func classificationPowerShell3ExpectedNodes() []kit.TreeNode {
	node := func(parent int64, typ string, named bool) kit.TreeNode {
		return kit.TreeNode{Parent: parent, Type: typ, Named: named, EndByte: 1, EndPoint: kit.Point{Column: 1}}
	}
	return []kit.TreeNode{node(-1, "program", true), node(0, "statement_list", true), node(1, "empty_statement", true), node(2, ";", false)}
}

func classificationPowerShell3ProfileFor(name string) (classificationEmptyProfile, bool) {
	if name != "powershell3-tiny" {
		return classificationEmptyProfile{}, false
	}
	return classificationEmptyProfile{
		name: "powershell3-tiny", anchorFile: "powershell3-classification-anchors.json", anchorSHA: "624ba74b07070fd18ceab2c8583ecff84e3433e2b32f7d7de37bf4387c45513e",
		route: "powershell", rootType: "program", symbol: "tree_sitter_powershell", caseID: "powershell3-tiny-definitions", workload: "powershell3-tiny-definition-single-frame",
		queryFile: "src/testdata/native/queries/powershell.json", queryBytes: 3451, querySHA: "d53b1588703d33fb033cbbc87d5f8e0508e05dc1038ecd942b6665ebc53eb79a",
		anchorBytes: 2571, grammarCount: 5, languageVersion: 15, runtimeLanguageVersion: 15, minCompatible: 13, symbolCount: 445, fieldCount: 16,
		source: ";", nodes: classificationPowerShell3ExpectedNodes(),
		queries: []classificationEmptyQuerySpec{
			{id: "q.structure", sha256: "36efa2ef8b8d235827d03a487b4f8296a3c3daf4e308a3a7a02b31b843ad13f4", patterns: 4, captures: []string{"enum", "class", "function", "command"}},
			{id: "q.probe-v7-chain", sha256: "82a52a565141daed32602f0f8861abcb2a0da3806f2875e726d32584f7a544b6", patterns: 1, captures: []string{"chain_op"}},
			{id: "q.probe-v7-blocks", sha256: "b842c7bff54bb402d9fa90c3c66a6ece314dca6cc0b9e62dc9aa40bb59017fa0", patterns: 1, captures: []string{"block_name"}},
		},
	}, true
}
