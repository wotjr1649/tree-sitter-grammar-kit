package kit

const classificationTSQL8AnchorsSHA = "a8b7da5feab22e08f6f08c980597eb3552d690b55dff108345035f6567bd74be"

func classificationTSQL8DefinitionProfile() classificationCPP4Profile {
	return classificationCPP4Profile{
		Label: "TSQL8", ScopeName: "TSQL", Language: "T-SQL",
		AnchorFile: "tsql8-classification-anchors.json", AnchorSHA: classificationTSQL8AnchorsSHA,
		Route: "tsql", Case: "tsql8-empty-definitions", Workload: "tsql8-empty-definition-single-frame",
		QueryFile: "src/testdata/native/queries/tsql.json", QuerySHA: "bce6ada698526ee5a162f4187b5fc0fb9e957ed279e6d0a954da9571a4eab71d",
		Set: "s06-tsql", Symbol: "tree_sitter_mssql", RootType: "program", QueryBytes: 11934, QueryCount: 8,
		Grammar: classificationTSQL8GrammarPins(),
		ABI:     eligibilityProducerABI{LanguageVersion: 15, RuntimeLanguageVersion: 15, RuntimeMinCompatible: 13, SymbolCount: 1386, FieldCount: 88},
		Counts:  classificationGoDefinitionSlotCounts{Cases: 1000, Edits: 991, CaseSteps: 1991, TreeRoles: 2982, PotentialQueryTreeRoles: 23856},
		Queries: []classificationCPP4QuerySpec{
			{id: "q.structure", sha256: "b251c313124e45c4db8fc470a5963f54d7b53df488a4ad9928203bd81d7e0a41", patterns: 4, captures: []string{"table", "column", "procedure", "exec"}},
			{id: "q.r6-structure", sha256: "3873c504301578837306f5fbe483433cfc58fb3ae30cebe913c12d029825ba63", patterns: 9, captures: []string{"exec", "column", "restore", "move_from", "move_to", "move_from_name", "assigned", "branch_order", "union_offset", "table"}},
			{id: "q.r6-begin", sha256: "5cca4ecb56cff2f575328d63276e1745a21021cb2944b276e845c8885e871674", patterns: 2, captures: []string{"block_cte", "nested_alias"}},
			{id: "q.probe-v16", sha256: "4159b9338522a4f7509f9d638f5817c6928f4677b2a31edbf4b38c4901f10aa8", patterns: 2, captures: []string{"period_column", "graph_match"}},
			{id: "q.probe-v22", sha256: "3fc130b6a79d424a3a07f06f7d1a7769b158c69c67099ae5be37cac60feba949", patterns: 1, captures: []string{"window_name"}},
			{id: "q.issue128-ownership", sha256: "c52218d06db46e4c9db7a8024626bce15236127ee15a43794b0d28281385f0ad", patterns: 3, captures: []string{"module.semi", "module.value", "batch.value"}},
			{id: "facts.declarations", sha256: "bfca18365c23af4a2cbcff969760772237fe443be4b6d8f4585f92f64489df23", patterns: 20, captures: []string{"c.0.0.0", "c.0.1.1", "c.0.1.0", "c.1.0.0", "c.1.1.1", "c.1.1.0", "c.2.0.0", "c.2.1.1", "c.2.1.0", "c.3.0.0", "c.3.1.1", "c.3.1.0", "c.4.0.0", "c.4.1.1", "c.4.1.0", "c.5.0.0", "c.5.1.1", "c.5.1.0", "c.6.0.0", "c.6.1.1", "c.6.1.0", "c.7.0.0", "c.7.1.1", "c.7.1.0", "c.8.0.0", "c.8.1.1", "c.8.1.0", "c.9.0.0", "c.9.1.1", "c.9.1.0"}},
			{id: "facts.dynamic_sql", sha256: "fa5fad90879b878dd42a2fae18dd43312a0c165e6ac8c9d0ffe96369138ae418", patterns: 14, captures: []string{"exec", "exec.child", "exec.parent", "proc", "proc.exec", "ref.name", "ref", "ref.schema", "ref.s", "arg.name", "arg", "arg.value", "arg.v", "bin.op", "bin", "field.name", "field", "semi", "err.first", "err", "errwith.first", "errwith", "batch.child", "batch.program"}},
		},
		FactQueries: []string{"facts.declarations", "facts.dynamic_sql"},
	}
}

func classificationTSQL8GrammarPins() []NativeInput {
	return []NativeInput{
		{Path: "src/parser.c", Role: "parser", Bytes: 68647192, SHA256: "542db94c263993ff68c0dddabcc12252dbb0bcf309e0d1acf2de8d4cd18abc24"},
		{Path: "src/scanner.c", Role: "scanner", Bytes: 14415, SHA256: "1efec197937cc59be09e480baae01008bbf8a2bec22ab95a7e731a670238bea4"},
		{Path: "src/tree_sitter/alloc.h", Role: "header", Bytes: 985, SHA256: "b29c1c9fb7cc82f58c84b376df1297d6e2737a1d655fd356db0859e3c29c2fea"},
		{Path: "src/tree_sitter/array.h", Role: "header", Bytes: 13453, SHA256: "20e6ba85abb9d5b7e2e6dedf675c9424a057a06feee03aa3e2dd48a1cb4bf8a2"},
		{Path: "src/tree_sitter/parser.h", Role: "header", Bytes: 7624, SHA256: "180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2"},
	}
}
