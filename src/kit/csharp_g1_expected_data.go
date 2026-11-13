package kit

// Authored data of the G1-3a expectation: the dispositions of the spec/Roslyn disagreements and the
// hand derivations of the specification sample audit.

// csharpG1Dispositions are keyed by "<code>/<Roslyn kind>"; every disagreement the cross-check finds
// must have one (the asset must not carry an UNDISPOSED entry).
var csharpG1Dispositions = map[string]g1Disposition{
	"TOKEN_OF_OTHER_ROW/SingleLineRawStringLiteralToken":           rawRowDisposition,
	"TOKEN_OF_OTHER_ROW/MultiLineRawStringLiteralToken":            rawRowDisposition,
	"TOKEN_OF_OTHER_ROW/InterpolatedSingleLineRawStringStartToken": rawRowDisposition,
	"TOKEN_OF_OTHER_ROW/InterpolatedMultiLineRawStringStartToken":  rawRowDisposition,
	"TOKEN_OF_OTHER_ROW/InterpolatedRawStringEndToken":             rawRowDisposition,
}

// Raw string literals are specified (6.4.5.6, Raw_String_Literal) but the inventory lists their
// productions under csharp-V11, so the lexical row's string alternatives (regular and verbatim only)
// do not describe them. Roslyn is not deviating from the specification.
var rawRowDisposition = g1Disposition{
	Disposition: "OTHER_ROW",
	Citation:    "C# standard v13 6.4.5.6: String_Literal includes Raw_String_Literal (Single_Line_Raw_String_Literal, Multi_Line_Raw_String_Literal)",
	Decision:    "not a deviation; the expectation belongs to the csharp-V11 alternatives (G1-3d)",
}

func hand(clause, status, block string, ts []string, text string) g1AuditEntry {
	return g1AuditEntry{Clause: clause, G1ExpAuditHand: G1ExpAuditHand{Status: status, Block: block, TS: ts, Text: text}}
}

const (
	hX = G1ExpExpected
	hB = G1ExpBlocked
)

func q1(s string) string { return G1KindMapAnon(s) }

// csharpG1AuditHand are the hand derivations from the specification, by alternative ID. They were
// written from the C# standard draft v13 (lexical-structure, types, expressions, unsafe-code pages)
// and the pinned grammar.js before the expectation was read; the sample IDs come from the fixed seed.
var csharpG1AuditHand = map[string]g1AuditEntry{
	"csharp-B01.a002": hand("6.4.5.7 null literal: the keyword null", hX, "", []string{"null_literal"}, `^null$`),
	"csharp-B01.a021": hand("6.3.2 line terminators: U+000A is a New_Line_Character; whitespace-like, no token", hB, g1BlockNoNode, nil, ""),
	"csharp-B01.a031": hand("6.3.4 white space: U+000B is Whitespace, no token", hB, g1BlockNoNode, nil, ""),
	"csharp-B01.a041": hand("6.4.2 Unicode character escape sequences: a backslash, U and eight hex digits, processed in identifiers, character and regular string literals", hX, "", []string{"identifier", "string_literal"}, `\\U[0-9A-Fa-f]{8}`),
	"csharp-B01.a092": hand("6.4.4 keywords: explicit", hX, "", []string{q1("explicit")}, `^explicit$`),
	"csharp-B01.a093": hand("6.4.4 keywords: extern (also a modifier in grammar.js)", hX, "", []string{q1("extern"), "modifier"}, `^extern$`),
	"csharp-B01.a110": hand("6.4.4 keywords: namespace", hX, "", []string{q1("namespace")}, `^namespace$`),
	"csharp-B01.a112": hand("6.4.4 keywords: null (null literal)", hX, "", []string{"null_literal"}, `^null$`),
	"csharp-B01.a169": hand("6.4.4 contextual keywords: let is an identifier with special meaning in query expressions", hX, "", []string{"identifier", q1("let")}, `^let$`),
	"csharp-B01.a209": hand("6.4.5.3 integer literals: the suffix u (lower case, alone)", hX, "", []string{"integer_literal"}, `^(?:[0-9][0-9_]*|0[xX][0-9A-Fa-f_]+|0[bB][01_]+)u$`),
	"csharp-B01.a217": hand("6.4.5.3 integer literals: the suffix Lu", hX, "", []string{"integer_literal"}, `^(?:[0-9][0-9_]*|0[xX][0-9A-Fa-f_]+|0[bB][01_]+)Lu$`),
	"csharp-B01.a254": hand("6.4.5.5 character literals: the simple escape sequence backslash f (form feed)", hX, "", []string{"escape_sequence"}, `^\\f$`),
	"csharp-B01.a293": hand("6.4.6 operators and punctuators: ~", hX, "", []string{q1("~")}, `^~$`),
	"csharp-B01.a294": hand("6.4.6 operators and punctuators: =", hX, "", []string{q1("=")}, `^=$`),
	"csharp-B01.a335": hand("6.5.4 definition directives: the conditional symbol of #define or #undef is an identifier", hX, "", []string{"identifier"}, `^[A-Za-z_][A-Za-z0-9_]*$`),
	"csharp-B01.a350": hand("6.5.5 conditional compilation directives: #else", hX, "", []string{"preproc_else"}, `^#[ \t]*else\b`),
	"csharp-B01.a353": hand("6.5.5 conditional compilation directives: #elif", hX, "", []string{"preproc_elif"}, `^#[ \t]*elif\b`),
	"csharp-B01.a356": hand("6.5.6 diagnostic directives: #error", hX, "", []string{"preproc_error"}, `^#[ \t]*error\b`),
	"csharp-B01.a359": hand("6.5.7 region directives: #region", hX, "", []string{"preproc_region"}, `^#[ \t]*region\b`),
	"csharp-B01.a379": hand("12.8.3 interpolated string expressions: the verbatim form starts with dollar-at-quote or at-dollar-quote", hX, "", []string{"interpolated_string_expression"}, `^(?:\$@|@\$)"`),
	"csharp-B01.a391": hand("12.8.3 / 6.4.5.6: two open braces inside interpolated text; grammar.js does not say how tree-sitter's external scanner represents it", hB, g1BlockExternal, nil, ""),
	"csharp-B02.a002": hand("7.8.1 namespace and type names: type_name is a namespace_or_type_name, told apart by its position", hB, g1BlockPosition, nil, ""),
	"csharp-B02.a015": hand("8.2.1 reference types: a class_type name is told from other type names only by binding", hB, g1BlockSemantic, nil, ""),
	"csharp-B02.a028": hand("17.2 array types: a rank_specifier is [ followed by zero or more commas and ]", hX, "", []string{"array_rank_specifier"}, `^\[\s*(?:,\s*)*\]$`),
	"csharp-B02.a045": hand("8.3.6 integral types: short", hX, "", []string{"predefined_type"}, `^short$`),
	"csharp-B02.a092": hand("24.3 pointer types: a data pointer type is a value type followed by one or more *", hX, "", []string{"pointer_type"}, `(?s)^.+\*$`),
}
