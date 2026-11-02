package native

func classificationDartPHP12ProfileName(name string) bool {
	_, ok := classificationDartPHP12ProfileFor(name)
	return ok
}

func classificationDartPHP12ProfileFor(name string) (classificationEmptyProfile, bool) {
	var profile classificationEmptyProfile
	switch name {
	case "dart7-empty":
		profile = classificationEmptyProfile{
			name: "dart7-empty", anchorFile: "dart7-classification-anchors.json", anchorSHA: "7dc0fe53780198aae6984b6ecec896029bb52ed8ed1b2179a26f45b5a4948f6e",
			route: "dart", rootType: "program", symbol: "tree_sitter_dart", caseID: "dart7-empty-definitions", workload: "dart7-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/dart.json", queryBytes: 10437, querySHA: "c853a16a12fe1d51eab89779146541dc0442c8b80f98fe15bd17051d1b343755",
			anchorBytes: 3760, grammarCount: 5, symbolCount: 533, fieldCount: 22,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "9be21f392aaa4a7b47ca93c8d4150af880520dbe1c884e8e881015d6b258a52d", patterns: 4, captures: []string{"class", "enum", "function", "getter"}},
				{id: "q.probe-dart-B02", sha256: "5f8d4ba8ceaabd923cca46c0d557b415733227eb99b40d549ff7f3a9e19ed598", patterns: 3, captures: []string{"then", "else", "body", "spread"}},
				{id: "q.probe-dart-V213", sha256: "f9d341d65ec3667741775ca19458a44ca67dbad00ca1ecce16054129130adf13", patterns: 3, captures: []string{"alias", "superparam", "tearoff"}},
				{id: "q.probe-dart-V30", sha256: "4dd168f6989acfea5b2059d37709b3239d510fd5a20e48a1ab5bda25dc2f5846", patterns: 3, captures: []string{"class", "exttype", "rep"}},
				{id: "q.probe-dart-V36", sha256: "085280dddc558a58a2650baabdaaa0f7512b5dfd60a2b5dc4e82089cd9c03c8b", patterns: 3, captures: []string{"int", "hex", "float"}},
				{id: "q.probe-dart-V310", sha256: "6cbc215635bcb2888fada030d7ac8945eb51eaf5905124dd422858574cacbc55", patterns: 2, captures: []string{"ctorparam", "shorthand"}},
				{id: "q.probe-dart-L01", sha256: "9d3e9c99f7545318e2f1d93104413b8fd3e221812d099ea1df8980aab0de3a3a", patterns: 2, captures: []string{"default"}},
			},
		}
	case "php5-empty":
		profile = classificationEmptyProfile{
			name: "php5-empty", anchorFile: "php5-classification-anchors.json", anchorSHA: "55f21a99a80dc6f5f895caff66733a1ff67cb9f9a1d144aa3c61718941a27225",
			route: "php", rootType: "program", symbol: "tree_sitter_php", caseID: "php5-empty-definitions", workload: "php5-empty-definition-single-frame",
			queryFile: "src/testdata/native/queries/php.json", queryBytes: 7603, querySHA: "4d2e3d204fa36c01eea491a21ec06eb39f7dba2bc30c0038983f1299f8e75a4a",
			anchorBytes: 3373, grammarCount: 6, symbolCount: 437, fieldCount: 31,
			queries: []classificationEmptyQuerySpec{
				{id: "q.structure", sha256: "8a679972baa8493da206d5cedb2c0bedf68e82af503185a41281ab0b64de0d51", patterns: 4, captures: []string{"enum", "class", "method", "function"}},
				{id: "q.probe-v56", sha256: "95f1911dfbfa7e2bbe61b967b8be979d118f1201ac2c135f624c7a8b6049745d", patterns: 6, captures: []string{"group_item", "variadic", "nullable", "spaceship", "unpack", "catch_types"}},
				{id: "q.probe-v72", sha256: "260d4f5dcc6720772b4a2ca705c77b79f8c5703fffcc902ea0a9b7312165c9ab", patterns: 4, captures: []string{"property_type", "heredoc", "coalesce_assign", "arrow_body"}},
				{id: "q.probe-v82", sha256: "4b97a5952f1301c1df3f47bdf7dfaa20cc12360f00e66e4c2a7d045920ccb4c3", patterns: 4, captures: []string{"readonly", "class", "const_type", "dnf", "dynamic_fetch"}},
				{id: "q.probe-v84", sha256: "80ee339d6c2f7a0aacab439200e8f48161aa193805ffbefd68d2f1df362d8f8d", patterns: 4, captures: []string{"visibility", "hook", "new_object", "pipe_target"}},
			},
		}
	default:
		return classificationEmptyProfile{}, false
	}
	profile.languageVersion, profile.runtimeLanguageVersion, profile.minCompatible = 15, 15, 13
	return profile, true
}
