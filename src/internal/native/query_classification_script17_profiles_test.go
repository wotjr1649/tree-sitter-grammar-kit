package native

func classificationEmptyProfileFor(name string) (classificationEmptyProfile, bool) {
	if profile, ok := classificationWeb16ProfileFor(name); ok {
		return profile, true
	}
	if profile, ok := classificationScript17ProfileFor(name); ok {
		return profile, true
	}
	return classificationDartPHP12ProfileFor(name)
}

func classificationEmptyProfileName(name string) bool {
	_, ok := classificationEmptyProfileFor(name)
	return ok
}

func classificationScript17ProfileName(name string) bool {
	_, ok := classificationScript17ProfileFor(name)
	return ok
}

func classificationScript17ProfileForRoute(route string) (classificationEmptyProfile, bool) {
	switch route {
	case "python":
		return classificationScript17ProfileFor("python8-empty")
	case "ruby":
		return classificationScript17ProfileFor("ruby4-empty")
	case "r":
		return classificationScript17ProfileFor("r3-empty")
	case "bash":
		return classificationScript17ProfileFor("bash2-empty")
	default:
		return classificationEmptyProfile{}, false
	}
}

func classificationScript17ProfileFor(name string) (classificationEmptyProfile, bool) {
	var profile classificationEmptyProfile
	switch name {
	case "python8-empty":
		profile = classificationEmptyProfile{
			name: "python8-empty", anchorFile: "python8-classification-anchors.json", anchorSHA: "a36dfdd9e2d2e517cf6bd6d41ece832042debb6d30d13eb1de57f81d3aff5b3b",
			route: "python", rootType: "module", symbol: "tree_sitter_python", caseID: "python8-empty-definitions", workload: "python8-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/python.json", queryBytes: 9291, querySHA: "76ed74d8a6176f11165842a097766e48bcecdae36f66d0ef60baea7dc4542ad5",
			anchorBytes: 4280, grammarCount: 5, symbolCount: 284, fieldCount: 32,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "065cf5de0604363066c0c39759638dcec9af935a0b1373696f691040fc9723a9", patterns: 4, captures: []string{"import", "class", "function", "type_alias"}},
				{id: "q.probe-b02", sha256: "cde154e002ca7e2f37c90a6e6bc801ddb89fa6a0e9864ddad4c36c180d4f43f0", patterns: 2, captures: []string{"function", "genexp", "key", "value"}},
				{id: "q.probe-v38", sha256: "5241a4423ad724acd2b33cdb765a642190e4e1ac8f095baec3034112b5cf13ad", patterns: 3, captures: []string{"slash", "name", "value", "debug"}},
				{id: "q.probe-v39", sha256: "31d97eeb189767f0ad59f0b0db5620eee6d9994b90521685488f006feec67989", patterns: 1, captures: []string{"expr"}},
				{id: "q.probe-v310", sha256: "30fd3966fac0a83b53e0cd6369d17ab7b7a9ca67bb587a28ff880146fe3c5664", patterns: 4, captures: []string{"subject", "guard", "class", "item"}},
				{id: "q.probe-v311", sha256: "3d277f382dd112715ae031529c21aee61258f81a426a532c0055d5d4c68e96c2", patterns: 3, captures: []string{"annotation", "return", "handler"}},
				{id: "q.probe-v313", sha256: "82ee085ab918978502e7bcf95185693374bfabed52f758823a1db9fe57cd3707", patterns: 3, captures: []string{"class", "params", "alias"}},
				{id: "q.probe-v314", sha256: "413712877f31e7200e9b2307cd3ff9cf78ce5d3e22cf50e99490ae3097f84d2c", patterns: 3, captures: []string{"expr", "conversion", "spec"}},
			},
		}
	case "ruby4-empty":
		profile = classificationEmptyProfile{
			name: "ruby4-empty", anchorFile: "ruby4-classification-anchors.json", anchorSHA: "abec8ccca94f26f4d258468eefb8eed6ecd2b04d67926e58a9b3288698facec9",
			route: "ruby", rootType: "program", symbol: "tree_sitter_ruby", caseID: "ruby4-empty-definitions", workload: "ruby4-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/ruby.json", queryBytes: 6291, querySHA: "265972373a9dedd0650675954b659f35348e929fc8d1791038c9e3ad37fbc8c4",
			anchorBytes: 2704, grammarCount: 5, symbolCount: 354, fieldCount: 32,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "a5cc4a7dc40c57581cdf56094b8b3673cdb3ea6ff393d0b6114c7b0671cf2587", patterns: 4, captures: []string{"module", "class", "method", "singleton_method"}},
				{id: "q.probe-b01", sha256: "dc33a0856cab4045ed1909eeac8894a50b243bea26dce84cd64244f1d3208028", patterns: 4, captures: []string{"interpolation", "regex", "heredoc_interpolation", "lambda_name", "lambda"}},
				{id: "q.probe-v27", sha256: "136c749e65d6b58678a32d1b959d8d75767663fab50eb9413bbcac7c0d55db4e", patterns: 4, captures: []string{"pattern", "guard", "rightward", "test"}},
				{id: "q.probe-v31", sha256: "b897352ef19ee12be7f3a7f2b975b51e38c95009978cd721508cbd188fcdf827", patterns: 5, captures: []string{"pair", "rest", "keyrest", "block", "receiver", "method"}},
			},
		}
	case "r3-empty":
		profile = classificationEmptyProfile{
			name: "r3-empty", anchorFile: "r3-classification-anchors.json", anchorSHA: "35a94bab40962b5d8467a8031728f1b9b20cb095fca289b96ebbbcc62671f6c8",
			route: "r", rootType: "program", symbol: "tree_sitter_r", caseID: "r3-empty-definitions", workload: "r3-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/r.json", queryBytes: 5850, querySHA: "5fae6fb13622e4eb18bde54a89436af437b1eb874f6be1f20040ad71eba42fb3",
			anchorBytes: 1995, grammarCount: 5, symbolCount: 144, fieldCount: 20,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "c792db5a2edb3d3a48cbeb61a6b2607bc95e65aef05ca6a65f14532c383ee6b3", patterns: 2, captures: []string{"function", "call"}},
				{id: "q.probe-v40", sha256: "9e5dce7edc8aa9e381da11611e1f263d994e92e89e6ac0a5ba06e402396ed663", patterns: 3, captures: []string{"raw_string", "lambda", "pipe_rhs"}},
				{id: "q.probe-v42", sha256: "3f65de0b8c997711434d558b4eb0a00c554e1f16612e0c3314ca283f94211e26", patterns: 2, captures: []string{"pipe_rhs", "arg_name", "placeholder"}},
			},
		}
	case "bash2-empty":
		profile = classificationEmptyProfile{
			name: "bash2-empty", anchorFile: "bash2-classification-anchors.json", anchorSHA: "90e8c57d2ed15d2659c2f4e2b38c6609df5ff167fe5e4ac32bf542abe90df032",
			route: "bash", rootType: "program", symbol: "tree_sitter_bash", caseID: "bash2-empty-definitions", workload: "bash2-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/bash.json", queryBytes: 3186, querySHA: "996b3b155f5fb9a534fe26c8b103fc09a9b1b5699e8c6bb382efd5356b9da359",
			anchorBytes: 1635, grammarCount: 5, symbolCount: 293, fieldCount: 19,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "c9f77947f4ea7b77754a9b447dc6e66af76b827c70bbf1237a0aef1a048bd355", patterns: 3, captures: []string{"function", "loop", "variable"}},
				{id: "q.probe-v4", sha256: "ad30db7ad9f830aab2136d3a579ab585d99f10126811f677b3de98b1dfc41578", patterns: 2, captures: []string{"assoc_name", "assoc_key", "fallthrough_item"}},
			},
		}
	default:
		return classificationEmptyProfile{}, false
	}
	profile.languageVersion, profile.runtimeLanguageVersion, profile.minCompatible = 15, 15, 13
	return profile, true
}
