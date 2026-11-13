package kit

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Post-seal corrections of the G1 expected asset and kind map (#233, G1-3a, r2).
//
// The r1 assets were sealed before any native output existed. The native comparison then showed four
// values that contradict the pinned grammar text: one expected value (csharp-B01.a335) and one kind map
// entry shared by three alternatives (csharp-B01.a383, a396, a397). The sealed r1 files stay byte for
// byte as sealed. A r2 file is the r1 file plus an explicit list of corrections, each with the value
// it replaces, the value it sets, and its grounds in the pinned specification and grammar text. Every
// correction is marked after_native: it was made after the native result was seen, so it counts apart
// from the independent expectations (see the specification, "Corrections after the seal"). The second mark,
// basis_independent, says whether the grounds are the specification and grammar text only.
//
// A r2 verifier accepts a file only when (1) the r1 file it pins verifies as the sealed asset, (2) the
// file equals the r1 file with exactly its own corrections applied (no other difference), (3) every
// correction is well formed, has grounds, and replaces the value r1 holds, and (4) the corrections are
// the authored ones below. The corrections are authored from the grammar and specification text; the
// native tree is not an input of any function in this file.

const (
	CSharpG1ExpectedSchemaR2 = "tsgk-csharp-g1-expected/r2"
	CSharpG1ExpectedPathR2   = "src/contracts/csharp-g1-expected-b01b02-r2.json"
	CSharpG1KindMapSchemaR2  = "tsgk-csharp-g1-kind-map/r2"
	CSharpG1KindMapPathR2    = "src/contracts/csharp-g1-kind-map-r2.json"

	G1FieldTS           = "ts"
	G1FieldWitnessMatch = "witnesses[0].match"
	G1FieldEntry        = "entry"
	// G1FieldTSBasis records that the ts of a differential-oracle alternative no longer comes from the kind map
	G1FieldTSBasis = "ts_basis"
	G1FieldTSOnly  = "ts_only"
	// G1FieldEvidence replaces the ts and the first witness of an alternative together (the old and the new value are one
	// JSON record of both): a witness that moves to another node has the types of that node
	G1FieldEvidence = "ts+witnesses[0]"
)

// G1Correction replaces Old by New in the field Field of the item Item. Old and New are JSON texts:
// the old value r1 holds and the value r2 holds.
type G1Correction struct {
	Item    string     `json:"item"`
	Field   string     `json:"field"`
	Old     string     `json:"old"`
	New     string     `json:"new"`
	Reason  string     `json:"reason"`
	Grounds []G1Ground `json:"ground"`
	// AfterNative: the correction was made after the native result was seen (the timing). It is true for every r2
	// correction; the statistics of post-seal changes count by it.
	AfterNative bool `json:"after_native"`
	// BasisIndependent: the grounds are the specification and grammar text only (the basis), whatever prompted the
	// correction.
	BasisIndependent bool `json:"basis_independent"`
}

func g1cFault(code string) error { return fail(KindInvalidInput, code, CSharpG1ExpectedPathR2, nil) }

const (
	g1gSpecPre = "csharpstandard@79ad4e90ef637621b9c25c706428a30278711deb (alpha-v13) "
	g1gLex     = g1gSpecPre + "standard/lexical-structure.md "
	g1gExpr    = g1gSpecPre + "standard/expressions.md "
)

// g1ExpectedCorrections are the corrections of the expected asset (the first four values come from the native comparison, then the eleven of the audit of the differential-oracle alternatives), authored from the grammar and
// specification text only.
var g1ExpectedCorrections = []G1Correction{
	{
		Item: "csharp-B01.a335", Field: G1FieldTS, Old: `["identifier"]`, New: `["preproc_arg"]`, AfterNative: true, BasisIndependent: true,
		Reason: "The specification defines the symbol of #define and #undef as PP_Conditional_Symbol, a lexeme, and names no tree-sitter type. The pinned grammar text builds preproc_define and preproc_undef from the directive and one preproc_arg token, which runs to the end of the line, and has no identifier rule in them (identifier appears only in the #if and #elif condition). The witness is the symbol DEBUG of '#define DEBUG' followed directly by the line end, so the token has the witness's span.",
		Grounds: []G1Ground{
			{G1DocGrammar, "preproc_define", "preproc_define: $ => seq( preprocessor('define'), $.preproc_arg, /\\n/, ),", g1gGrammarSHA, "preproc_define:", 0},
			{G1DocGrammar, "preproc_undef", "preproc_undef: $ => seq( preprocessor('undef'), $.preproc_arg, /\\n/, ),", g1gGrammarSHA, "preproc_undef:", 0},
			{G1DocGrammar, "preproc_arg", "preproc_arg: _ => token(prec(-1, /\\S([^/\\n]|\\/[^*]|\\\\\\r?\\n)*/)),", g1gGrammarSHA, "preproc_arg:", 0},
			{G1DocSpec, g1gLex + "6.5.4 PP_Declaration", "'define' PP_Whitespace PP_Conditional_Symbol", g1gLexSHA, "### 6.5.4 Definition directives", 6},
			{G1DocSpec, g1gLex + "6.5.2 PP_Conditional_Symbol", "fragment PP_Conditional_Symbol // Must not be equal to tokens TRUE or FALSE. See note below. : Basic_Identifier ;", g1gLexSHA, "### 6.5.2 Conditional compilation symbols", 5},
		},
	},
	{
		Item: "csharp-B01.a383", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, AfterNative: true, BasisIndependent: true,
		Reason: "The specification makes Interpolated_Regular_String_Start one lexeme of two characters ($\"). The pinned grammar text has no node of that extent: interpolated_string_expression begins with two nodes, an interpolation_start (an alias of the external token interpolation_regular_start) and then a quote node. The grammar text does not give the extent of the external token, so only the start is derivable: an interpolation_start node starts where the lexeme starts.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_regular_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),", g1gGrammarSHA, "interpolated_string_expression:", 1},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 37},
		},
	},
	{
		Item: "csharp-B01.a396", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, AfterNative: true, BasisIndependent: true,
		Reason: "The specification makes Interpolated_Verbatim_String_Start one lexeme ($@\" or @$\"). The pinned grammar text begins the verbatim form with an interpolation_start (an alias of the external token interpolation_verbatim_start) and then a quote node, so no node has the lexeme's extent, and only the start is derivable: an interpolation_start node starts where the lexeme starts.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),", g1gGrammarSHA, "interpolated_string_expression:", 7},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 83},
		},
	},
	{
		Item: "csharp-B01.a397", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, AfterNative: true, BasisIndependent: true,
		Reason: "Same lexeme class and grammar rule as csharp-B01.a396: the verbatim start @$\" is one lexeme in the specification and begins with an interpolation_start (alias of interpolation_verbatim_start) followed by a quote node in the pinned grammar text, so only the start is derivable.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),", g1gGrammarSHA, "interpolated_string_expression:", 7},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 83},
		},
	},
	// The kind map r2 gives the start tokens of interpolated strings no node type (see g1KindMapCorrections), so the ts of
	// these three differential-oracle alternatives no longer comes from the map. It is the type the pinned grammar text
	// names for the start of the interpolated string; the basis of the value is the grammar text, not the kind map. The
	// alternatives keep their differential-oracle label for the witness (the lexeme and its position are what Roslyn
	// observed).
	g1TSBasis("csharp-B01.a383", "interpolation_regular_start", 1,
		"seq( alias($.interpolation_regular_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),",
		g1gExpr+"12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;", 37),
	g1TSBasis("csharp-B01.a396", "interpolation_verbatim_start", 7,
		"seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),",
		g1gExpr+"12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;", 83),
	g1TSBasis("csharp-B01.a397", "interpolation_verbatim_start", 7,
		"seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),",
		g1gExpr+"12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;", 83),

	// The audit of the 76 differential-oracle alternatives (G1AUDIT76, after the native result) found eleven witnesses that the
	// specification and the pinned grammar text do not support. Each is corrected from those texts alone; the native result
	// shows only that the corrected value is not contradicted (the executed asset). Nine move the witness to the token or node
	// that is the instance (with the types of that node), two keep the node and claim only its start.
	g1Evidence("csharp-B02.a021",
		[]string{"pointer_type"}, G1ExpWitness{"alt-csharp-B02-02#0", 24, 28, "PointerType", "span"},
		[]string{"pointer_type"}, G1ExpWitness{"alt-csharp-B02-02#0", 62, 66, "PointerType", "span"},
		"non_array_type: pointer_type is the pointer type as the element type of an array type (8.2.1; 24.3.2 lists int*[] as an array of pointers to int). The old witness is the type of the field 'int* p' (24-28), which is the instance of type: pointer_type (a008) and not of this alternative. The instance here is the element type 'int*' of 'int*[] arr' (62-66): the pinned grammar text lets array_type take a pointer_type as its base, and a pointer_type covers 'int*' exactly. The Roslyn observation holds a PointerType at 62-66.",
		G1Ground{G1DocGrammar, "_array_base_type", "_array_base_type: $ => choice( $.array_type, $._name, $.nullable_type, $.pointer_type,", g1gGrammarSHA, "_array_base_type:", 0},
		G1Ground{G1DocGrammar, "pointer_type", "pointer_type: $ => seq(field('type', $._pointer_base_type), '*'),", g1gGrammarSHA, "pointer_type:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/types.md 8.2.1 non_array_type", "non_array_type : non_array_non_nullable_type nullable_type_annotation? | pointer_type", g1gTypesSHA, "### 8.2.1 General", 33},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/unsafe-code.md 24.3.2 int*[]", "Single-dimensional array of pointers to `int`", g1gUnsafeSHA, "### 24.3.2 Data pointers", 19},
	),
	g1Evidence("csharp-B02.a062",
		[]string{"pointer_type"}, G1ExpWitness{"alt-csharp-B02-02#0", 24, 28, "PointerType", "span"},
		[]string{"pointer_type"}, G1ExpWitness{"alt-csharp-B02-02#0", 211, 215, "PointerType", "span"},
		"unmanaged_type: pointer_type is the pointer type as the operand of sizeof (8.8; 12.8.19 sizeof_expression takes an unmanaged_type). The old witness is the type of the field 'int* p' (24-28), the instance of type: pointer_type (a008). The instance here is the operand 'int*' of 'sizeof(int*)' (211-215): the pinned grammar text gives sizeof_expression a type field and a pointer_type covers 'int*' exactly. The Roslyn observation holds a PointerType at 211-215.",
		G1Ground{G1DocGrammar, "sizeof_expression", "sizeof_expression: $ => seq( 'sizeof', '(', field('type', $.type), ')', ),", g1gGrammarSHA, "sizeof_expression:", 0},
		G1Ground{G1DocGrammar, "pointer_type", "pointer_type: $ => seq(field('type', $._pointer_base_type), '*'),", g1gGrammarSHA, "pointer_type:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/types.md 8.8 unmanaged_type", "unmanaged_type : value_type | pointer_type", g1gTypesSHA, "## 8.8 Unmanaged types", 3},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.8.19 sizeof_expression", "sizeof_expression : 'sizeof' '(' unmanaged_type ')'", g1gExprSHA, "### 12.8.19 The sizeof operator", 5},
	),
	g1Evidence("csharp-B02.a030",
		[]string{"nullable_type"}, G1ExpWitness{"alt-csharp-B02-01#0", 86, 90, "NullableType", "span"},
		[]string{"\"?\""}, G1ExpWitness{"alt-csharp-B02-01#0", 89, 90, "QuestionToken", "span"},
		"nullable_type_annotation is the token '?' (8.2.1, 6.4.6). The old witness is the nullable type 'int?' (86-90), a node that contains the instance. In the pinned grammar text the '?' is an anonymous token of nullable_type, so the token '?' (89-90, Roslyn QuestionToken) is the exact witness and its type is the anonymous token '\"?\"', the type the kind map r2 gives QuestionToken.",
		G1Ground{G1DocGrammar, "nullable_type", "nullable_type: $ => seq(field('type', $._nullable_base_type), '?'),", g1gGrammarSHA, "nullable_type:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/types.md 8.2.1 nullable_type_annotation", "nullable_type_annotation : '?' ; ``` *pointer_type*", g1gTypesSHA, "### 8.2.1 General", 59},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/lexical-structure.md 6.4.6 operator_or_punctuator", "| '=' | '<' | '>' | '?' | '??' | '::' | '++'", g1gLexSHA, "### 6.4.6 Operators and punctuators", 12},
	),
	g1Evidence("csharp-B02.a072",
		[]string{"type_parameter_constraints_clause"}, G1ExpWitness{"alt-csharp-B02-04#0", 66, 81, "TypeParameterConstraintClause", "span"},
		[]string{"type_parameter_constraint"}, G1ExpWitness{"alt-csharp-B02-04#0", 76, 81, "ClassConstraint", "span"},
		"type_parameter_constraints is the constraint list of a where clause (15.2.5). The old witness is the whole clause 'where T : class' (66-81), a node that contains the instance. The pinned grammar text has no node for the list: the clause is 'where', an identifier, ':' and a comma separated list of type_parameter_constraint nodes. In this source the list holds one constraint, so the instance is that constraint, 'class' (76-81, Roslyn ClassConstraint), which has the extent of the list. The witness therefore proves a single constraint list only; a list of several constraints is not shown.",
		G1Ground{G1DocGrammar, "type_parameter_constraints_clause", "type_parameter_constraints_clause: $ => seq( 'where', $.identifier, ':', commaSep1($.type_parameter_constraint),", g1gGrammarSHA, "type_parameter_constraints_clause:", 0},
		G1Ground{G1DocGrammar, "type_parameter_constraint", "type_parameter_constraint: $ => choice( seq('class', optional('?')), 'struct',", g1gGrammarSHA, "type_parameter_constraint:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.2.5 type_parameter_constraints", "type_parameter_constraints : restrictive_type_parameter_constraints (',' anti_constraints_clause)? | anti_constraints_clause ;", g1gClassesSHA, "### 15.2.5 Type parameter constraints", 9},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.2.5 primary_constraint", "primary_constraint : class_type nullable_type_annotation? | 'class' nullable_type_annotation?", g1gClassesSHA, "### 15.2.5 Type parameter constraints", 20},
	),
	g1Evidence("csharp-B02.a084",
		[]string{"implicit_parameter", "parameter", "receiver_parameter"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 40, "Parameter", "span"},
		[]string{"\"ref\"", "modifier"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 34, "RefKeyword", "span"},
		"ref_kind: 'ref' is the keyword ref of a parameter (15.6.1). The old witness is the whole parameter 'ref int a' (31-40), a node that contains the instance. The pinned grammar text makes every parameter modifier keyword its own modifier node, so the exact witness is the keyword 'ref' (31-34, Roslyn RefKeyword), typed as the anonymous token and the modifier node the kind map r2 gives RefKeyword.",
		G1Ground{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped',", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.1 ref_kind", "ref_kind : 'ref' | 'ref' 'readonly' ; ref_method_modifiers", g1gClassesSHA, "### 15.6.1 General", 17},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
	),
	g1Evidence("csharp-B02.a086",
		[]string{"implicit_parameter", "parameter", "receiver_parameter"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 40, "Parameter", "span"},
		[]string{"\"ref\"", "modifier"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 34, "RefKeyword", "span"},
		"parameter_modifier: parameter_mode_modifier is a ref, out or in keyword on a parameter (15.6.2.1). The old witness is the whole parameter 'ref int a' (31-40), a node that contains the instance. The pinned grammar text makes each such keyword its own modifier node, so the exact witness is the keyword 'ref' (31-34, Roslyn RefKeyword), typed as the kind map r2 types RefKeyword.",
		G1Ground{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped',", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_modifier", "parameter_modifier : parameter_mode_modifier | 'this' 'scoped'? parameter_mode_modifier?", g1gClassesSHA, "#### 15.6.2.1 General", 27},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
	),
	g1Evidence("csharp-B02.a087",
		[]string{"implicit_parameter", "parameter", "receiver_parameter"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 40, "Parameter", "span"},
		[]string{"\"ref\"", "modifier"}, G1ExpWitness{"alt-csharp-B02-03#0", 31, 34, "RefKeyword", "span"},
		"parameter_mode_modifier: ref_kind is the ref keyword form of the modifier (15.6.2.1, 15.6.1). The old witness is the whole parameter 'ref int a' (31-40), a node that contains the instance. The pinned grammar text makes the keyword its own modifier node, so the exact witness is 'ref' (31-34, Roslyn RefKeyword), typed as the kind map r2 types RefKeyword.",
		G1Ground{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped',", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.1 ref_kind", "ref_kind : 'ref' | 'ref' 'readonly' ; ref_method_modifiers", g1gClassesSHA, "### 15.6.1 General", 17},
	),
	g1Evidence("csharp-B02.a088",
		[]string{"implicit_parameter", "parameter", "receiver_parameter"}, G1ExpWitness{"alt-csharp-B02-03#0", 42, 51, "Parameter", "span"},
		[]string{"\"out\"", "modifier"}, G1ExpWitness{"alt-csharp-B02-03#0", 42, 45, "OutKeyword", "span"},
		"parameter_mode_modifier: 'out' is the keyword out of a parameter (15.6.2.1). The old witness is the whole parameter 'out int b' (42-51), a node that contains the instance. The pinned grammar text makes the keyword its own modifier node, so the exact witness is 'out' (42-45, Roslyn OutKeyword), typed as the kind map r2 types OutKeyword.",
		G1Ground{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped',", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
	),
	g1Evidence("csharp-B02.a089",
		[]string{"implicit_parameter", "parameter", "receiver_parameter"}, G1ExpWitness{"alt-csharp-B02-03#0", 53, 61, "Parameter", "span"},
		[]string{"\"in\"", "modifier"}, G1ExpWitness{"alt-csharp-B02-03#0", 53, 55, "InKeyword", "span"},
		"parameter_mode_modifier: 'in' is the keyword in of a parameter (15.6.2.1). The old witness is the whole parameter 'in int c' (53-61), a node that contains the instance. The pinned grammar text makes the keyword its own modifier node, so the exact witness is 'in' (53-55, Roslyn InKeyword), typed as the kind map r2 types InKeyword.",
		G1Ground{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped',", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
	),
	{
		Item: "csharp-B01.a381", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, AfterNative: true, BasisIndependent: true,
		Reason: "The specification's regular_interpolation is the expression, alignment and format between the braces; the braces are terminals of the enclosing rule. The pinned grammar text builds the interpolation node with an interpolation_brace first child (an alias of the external token interpolation_open_brace) and one last (an alias of interpolation_close_brace). The node therefore starts at the opening brace and ends with the closing brace; the grammar text does not give the extent of an external token, so only the start is derivable (the principle of csharp-B01.a383): an interpolation node starts where its opening brace starts.",
		Grounds: []G1Ground{
			G1Ground{G1DocGrammar, "interpolation", "interpolation: $ => seq( alias($.interpolation_open_brace, $.interpolation_brace), $.expression,", g1gGrammarSHA, "interpolation:", 0},
			G1Ground{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.8.3 regular_interpolation", "regular_interpolation : expression (',' interpolation_minimum_width)? Regular_Interpolation_Format?", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 28},
		},
	},
	{
		Item: "csharp-B01.a395", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, AfterNative: true, BasisIndependent: true,
		Reason: "Same node and grammar rule as csharp-B01.a381 for a verbatim interpolated string: the verbatim string content uses the same interpolation rule, whose first and last children are aliases of the external tokens interpolation_open_brace and interpolation_close_brace, so only the start of the node is derivable.",
		Grounds: []G1Ground{
			G1Ground{G1DocGrammar, "interpolation", "interpolation: $ => seq( alias($.interpolation_open_brace, $.interpolation_brace), $.expression,", g1gGrammarSHA, "interpolation:", 0},
			G1Ground{G1DocGrammar, "_interpolated_verbatim_string_content", "_interpolated_verbatim_string_content: $ => choice( alias($.interpolation_string_content,", g1gGrammarSHA, "_interpolated_verbatim_string_content:", 0},
			G1Ground{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.8.3 verbatim_interpolation", "verbatim_interpolation : expression (',' interpolation_minimum_width)? Verbatim_Interpolation_Format?", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 78},
		},
	},
	// The type audit (G1TYPEAUDIT76, grammar text only, after the native result) found that the ts of two parameter modifier
	// keywords names an anonymous token that the grammar text cannot produce in that place. The ts the evidence corrections
	// above took over from the kind map r2 is narrowed to the one type the grammar text gives; the kind map is not changed.
	g1NarrowModifierTS("csharp-B02.a088", "out", `["\"out\"","modifier"]`, "OutKeyword"),
	g1NarrowModifierTS("csharp-B02.a089", "in", `["\"in\"","modifier"]`, "InKeyword"),
}

// g1cEvidence is the value of a G1FieldEvidence correction: the ts and the first witness of an alternative.
type g1cEvidence struct {
	TS      []string     `json:"ts"`
	Witness G1ExpWitness `json:"witness"`
}

// g1Evidence is the correction that moves the witness of an alternative to another token or node, with the ts of that node.
func g1Evidence(item string, oldTS []string, oldW G1ExpWitness, newTS []string, newW G1ExpWitness, reason string, grounds ...G1Ground) G1Correction {
	return G1Correction{
		Item: item, Field: G1FieldEvidence, Old: g1cJSON(g1cEvidence{oldTS, oldW}), New: g1cJSON(g1cEvidence{newTS, newW}),
		Reason: reason, Grounds: grounds, AfterNative: true, BasisIndependent: true,
	}
}

// g1NarrowModifierTS narrows the ts of a parameter modifier keyword (out, in) from the anonymous token and modifier to modifier.
func g1NarrowModifierTS(item, keyword, old, roslyn string) G1Correction {
	return G1Correction{
		Item: item, Field: G1FieldTS, Old: old, New: `["modifier"]`, AfterNative: true, BasisIndependent: true,
		Reason: "The ts of this alternative came from the kind map r2, which relates " + roslyn + " to both the anonymous token and modifier because the keyword is a plain token in other places. In a parameter the pinned grammar text builds the keyword only through alias(choice('this', 'scoped', 'ref', 'out', 'in', 'readonly'), modifier), so the node of the witness is a modifier leaf named by the alias; no anonymous '" + keyword + "' token is produced there, and the ref_type rule that competes with the modifier for 'ref' has no '" + keyword + "'. The grounds are the grammar text and the specification only, and the stored native observation is compared again after the change (the executed asset).",
		Grounds: []G1Ground{
			{G1DocGrammar, "_parameter_type_with_modifiers", "_parameter_type_with_modifiers: $ => seq( repeat(prec.left(alias( choice('this', 'scoped', 'ref', 'out', 'in', 'readonly'), $.modifier, ))),", g1gGrammarSHA, "_parameter_type_with_modifiers:", 0},
			{G1DocGrammar, "ref_type", "ref_type: $ => seq( 'ref', optional('readonly'), field('type', $.type), ),", g1gGrammarSHA, "ref_type:", 0},
			{G1DocSpec, g1gSpecPre + "standard/classes.md 15.6.2.1 parameter_mode_modifier", "parameter_mode_modifier : ref_kind | 'out' | 'in' ;", g1gClassesSHA, "#### 15.6.2.1 General", 34},
		},
	}
}

// g1TSBasis is the correction that records the new basis of the ts of an interpolated string start alternative.
func g1TSBasis(item, external string, gap int, gramQuote, specAt, specQuote string, specGap int) G1Correction {
	return G1Correction{
		Item: item, Field: G1FieldTSBasis, Old: `"kind-map"`, New: `"grammar-text"`, AfterNative: true, BasisIndependent: true,
		Reason: "The ts of this differential-oracle alternative, interpolation_start, was translated from the kind map r1 entry of its Roslyn kind. The kind map r2 relates that kind to no node type (the lexeme has no node of its extent), so the value is no longer a translation. It stays interpolation_start because the pinned grammar text begins the interpolated string with an interpolation_start node (an alias of the external token " + external + "): the basis of the ts is that grammar text.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", gramQuote, g1gGrammarSHA, "interpolated_string_expression:", gap},
			{G1DocSpec, specAt, specQuote, g1gExprSHA, "### 12.8.3 Interpolated string expressions", specGap},
		},
	}
}

// g1KindMapCorrections are the kind map entries of the start tokens of interpolated strings, and the one
// consequence for the list of unreferenced types. The map relates a Roslyn kind to the node types of
// the same source extent; for a start token of an interpolated string the pinned grammar text has no
// node of that extent. The first two entries are behind the three mismatching alternatives (found
// after the native result). The two raw entries have the same structure in the grammar text and no native
// mismatch showed it; they were corrected on the Root decision to correct the same structural error everywhere,
// so after the native result as well, and they too are derived from the specification and the grammar text alone.
var g1KindMapCorrections = []G1Correction{
	{
		Item: "InterpolatedStringStartToken", Field: G1FieldEntry, AfterNative: true, BasisIndependent: true,
		Old:    `{"roslyn":"InterpolatedStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "The map relates a Roslyn kind to the node types of the same extent. The token $\" is one lexeme in the specification, but the pinned grammar text builds the start of an interpolated regular string from two nodes (interpolation_start, then a quote node), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_regular_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),", g1gGrammarSHA, "interpolated_string_expression:", 1},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 37},
		},
	},
	{
		Item: "InterpolatedVerbatimStringStartToken", Field: G1FieldEntry, AfterNative: true, BasisIndependent: true,
		Old:    `{"roslyn":"InterpolatedVerbatimStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedVerbatimStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "The tokens $@\" and @$\" are one lexeme in the specification, but the pinned grammar text builds the start of an interpolated verbatim string from two nodes (interpolation_start, then a quote node), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),", g1gGrammarSHA, "interpolated_string_expression:", 7},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 83},
		},
	},
	{
		Item: "InterpolatedSingleLineRawStringStartToken", Field: G1FieldEntry, AfterNative: true, BasisIndependent: true,
		Old:    `{"roslyn":"InterpolatedSingleLineRawStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedSingleLineRawStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "Same structural error as the regular and verbatim starts, derived from the specification and the grammar text alone (no native mismatch showed it): Interpolated_Raw_String_Start is one lexeme (the dollar signs and the raw string delimiter), but the pinned grammar text begins the raw form with an interpolation_start (an alias of the external token interpolation_raw_start) and then a quote node (interpolation_quote), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_raw_start, $.interpolation_start), alias($.interpolation_start_quote, $.interpolation_quote),", g1gGrammarSHA, "interpolated_string_expression:", 13},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Raw_String_Start", "Interpolated_Raw_String_Start : Interpolated_Raw_String_Prefix Raw_String_Literal_Delimiter ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 140},
		},
	},
	{
		Item: "InterpolatedMultiLineRawStringStartToken", Field: G1FieldEntry, AfterNative: true, BasisIndependent: true,
		Old:    `{"roslyn":"InterpolatedMultiLineRawStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedMultiLineRawStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "Same as InterpolatedSingleLineRawStringStartToken: the multi-line raw form uses the same Interpolated_Raw_String_Start lexeme and the same grammar rule (one interpolated_string_expression choice for both), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_raw_start, $.interpolation_start), alias($.interpolation_start_quote, $.interpolation_quote),", g1gGrammarSHA, "interpolated_string_expression:", 13},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Raw_String_Start", "Interpolated_Raw_String_Start : Interpolated_Raw_String_Prefix Raw_String_Literal_Delimiter ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 140},
		},
	},
	{
		Item: "interpolation_start", Field: G1FieldTSOnly, AfterNative: true, BasisIndependent: true,
		Old:    `null`,
		New:    `{"type":"interpolation_start","reason":"EXTERNAL_SCANNER_NODE"}`,
		Reason: "A consequence of the four entries above: interpolation_start was the type of the four start kinds and no kind refers to it now, so it joins the named types without a Roslyn kind. The pinned grammar text declares the three tokens it aliases (interpolation_regular_start, interpolation_verbatim_start, interpolation_raw_start) as external scanner tokens, the reason r1 gives to interpolation_brace and the other aliases of external tokens.",
		Grounds: []G1Ground{
			{G1DocGrammar, "externals", "$.interpolation_regular_start, $.interpolation_verbatim_start, $.interpolation_raw_start,", g1gGrammarSHA, "externals:", 2},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;", g1gExprSHA, "### 12.8.3 Interpolated string expressions", 37},
		},
	},
}

// g1CheckCorrections checks the form of a list of corrections: at least one, each with the allowed
// field, a different old and new value, a reason, a ground in the grammar text (the tree-sitter side
// of every correction comes from there) and a ground in the specification; no (item, field) twice. Every
// ground is checkable offline (g1CheckGround). after_native is true for every r2 correction (the decision
// of 2026-11-13: a r2 correction is one made after the native result); basis_independent is authored with the
// correction and checked by the comparison with the authored list.
func g1CheckCorrections(cs []G1Correction, field func(string) bool) error {
	if len(cs) == 0 {
		return g1cFault("G1_CORRECTION_FORM")
	}
	seen := map[string]bool{}
	for _, c := range cs {
		key := c.Item + "|" + c.Field
		if c.Item == "" || !field(c.Field) || seen[key] || c.Old == "" || c.New == "" || c.Old == c.New || c.Reason == "" || !c.AfterNative {
			return g1cFault("G1_CORRECTION_FORM")
		}
		seen[key] = true
		grammar, spec := false, false
		for _, g := range c.Grounds {
			if !g1CheckGround(g) {
				return g1cFault("G1_CORRECTION_GROUND")
			}
			grammar = grammar || g.Doc == G1DocGrammar
			spec = spec || g.Doc == G1DocSpec
		}
		if !grammar || !spec {
			return g1cFault("G1_CORRECTION_GROUND")
		}
	}
	return nil
}

// CheckG1Grounds checks every ground of the corrections against the documents themselves: the grammar text for
// a grammar ground and the specification text of the named file (lexical-structure.md or expressions.md) for a spec
// ground. The document has the SHA-256 of the ground, the quote is in it, and the heading line of the ground is
// exactly Gap lines before the line where that quote starts. It reads the documents the caller supplies and nothing
// else; it is the live counterpart of the offline check (g1CheckGround) and runs where the documents are present.
func CheckG1Grounds(cs []G1Correction, grammar string, spec map[string]string) error {
	for _, c := range cs {
		if err := CheckG1GroundList(c.Item, c.Grounds, grammar, spec); err != nil {
			return err
		}
	}
	return nil
}

func g1cJSON(v any) string {
	d, _ := json.Marshal(v)
	return string(d)
}

// ---- expected asset -----------------------------------------------------------------------------

func g1cExpectedField(f string) bool {
	return f == G1FieldTS || f == G1FieldWitnessMatch || f == G1FieldTSBasis || f == G1FieldEvidence
}

// g1cCloneExpected copies the alternatives, the only part a correction changes.
func g1cCloneExpected(x CSharpG1Expected) CSharpG1Expected {
	alts := make([]G1ExpAlt, len(x.Alternatives))
	for i, a := range x.Alternatives {
		a.TS = slices.Clone(a.TS)
		a.Witnesses = slices.Clone(a.Witnesses)
		alts[i] = a
	}
	x.Alternatives = alts
	return x
}

// g1cApplyExpected returns x with the corrections applied. Each must name an EXPECTED alternative and a
// field whose current value is the correction's Old; the New value must be well formed.
func g1cApplyExpected(x CSharpG1Expected, cs []G1Correction) (CSharpG1Expected, error) {
	out := g1cCloneExpected(x)
	for _, c := range cs {
		var alt *G1ExpAlt
		for i := range out.Alternatives {
			if out.Alternatives[i].ID == c.Item {
				alt = &out.Alternatives[i]
			}
		}
		if alt == nil || alt.Status != G1ExpExpected {
			return out, g1cFault("G1_CORRECTION_ITEM")
		}
		switch c.Field {
		case G1FieldTS:
			var ts []string
			if g1cJSON(alt.TS) != c.Old || json.Unmarshal([]byte(c.New), &ts) != nil || len(ts) == 0 || !slices.IsSorted(ts) || slices.Contains(ts, "") {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
			alt.TS = ts
		case G1FieldTSBasis:
			// a record, not a change of value: the ts of a differential-oracle alternative that the kind map no longer gives
			if c.Old != `"kind-map"` || c.New != `"grammar-text"` || alt.Basis != G1BasisDiff {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
		case G1FieldEvidence:
			var n g1cEvidence
			if len(alt.Witnesses) != 1 || g1cJSON(g1cEvidence{alt.TS, alt.Witnesses[0]}) != c.Old || json.Unmarshal([]byte(c.New), &n) != nil || g1cJSON(n) != c.New {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
			w := n.Witness
			if len(n.TS) == 0 || !slices.IsSorted(n.TS) || slices.Contains(n.TS, "") || w.Source != alt.Witnesses[0].Source || w.Start < 0 || w.Start >= w.End ||
				w.Roslyn == "" || w.Match != "span" && w.Match != "starts-at" {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
			alt.TS, alt.Witnesses[0] = n.TS, w
		case G1FieldWitnessMatch:
			var m string
			if len(alt.Witnesses) == 0 || g1cJSON(alt.Witnesses[0].Match) != c.Old || json.Unmarshal([]byte(c.New), &m) != nil || m != "span" && m != "starts-at" {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
			alt.Witnesses[0].Match = m
		default:
			return out, g1cFault("G1_CORRECTION_FORM")
		}
	}
	return out, nil
}

// g1cCheckEvidence checks the new value of every evidence correction against the observation and the r2 kind map: the
// witness is a token or node of its source that Roslyn observed, and every type it carries is a type the kind map gives
// the Roslyn kind of that witness. So a witness cannot move to a place nobody observed or take a type the map does not
// relate to it.
func g1cCheckEvidence(cs []G1Correction, observation, kindMap []byte) error {
	var obs CSharpG1RoslynObserved
	var km CSharpG1KindMap
	loaded := false
	for _, c := range cs {
		if c.Field != G1FieldEvidence {
			continue
		}
		if !loaded {
			if err := jsonv2.Unmarshal(observation, &obs, jsonv2.RejectUnknownMembers(true)); err != nil {
				return fail(KindInvalidInput, "G1_EXPECTED_OBSERVATION", CSharpG1RoslynPath, err)
			}
			var err error
			if km, err = ParseCSharpG1KindMap(kindMap, obs.Kinds); err != nil {
				return err
			}
			loaded = true
		}
		var n g1cEvidence
		if json.Unmarshal([]byte(c.New), &n) != nil {
			return g1cFault("G1_CORRECTION_OLD")
		}
		kind := slices.Index(obs.Kinds, n.Witness.Roslyn)
		seen := false
		for _, s := range obs.Sources {
			if s.ID != n.Witness.Source {
				continue
			}
			for _, list := range [][][3]int{s.Tokens, s.Nodes} {
				seen = seen || slices.Contains(list, [3]int{kind, n.Witness.Start, n.Witness.End})
			}
		}
		i := slices.IndexFunc(km.Kinds, func(k G1KMKind) bool { return k.Roslyn == n.Witness.Roslyn })
		if kind < 0 || !seen || i < 0 {
			return g1cFault("G1_CORRECTION_WITNESS")
		}
		for _, t := range n.TS {
			if !slices.Contains(km.Kinds[i].TS, t) {
				return g1cFault("G1_CORRECTION_WITNESS")
			}
		}
	}
	return nil
}

// CSharpG1ExpectedR2Sources are the inputs of the r2 expected asset: the inputs of the r1 derivation
// (its KindMap is the r1 map), the sealed r1 asset, and the r2 kind map the r2 asset pins.
type CSharpG1ExpectedR2Sources struct {
	R1       CSharpG1ExpectedSources
	Previous []byte
	KindMap  []byte
}

// DeriveCSharpG1ExpectedR2 derives the r2 asset: the verified r1 asset with the authored corrections.
func DeriveCSharpG1ExpectedR2(in CSharpG1ExpectedR2Sources) (CSharpG1Expected, error) {
	r1, err := ParseCSharpG1Expected(in.Previous, in.R1)
	if err != nil {
		return r1, err
	}
	return g1cBuildExpected(r1, in, g1ExpectedCorrections)
}

func g1cBuildExpected(r1 CSharpG1Expected, in CSharpG1ExpectedR2Sources, cs []G1Correction) (CSharpG1Expected, error) {
	x, err := g1cApplyExpected(r1, cs)
	if err != nil {
		return x, err
	}
	x.Schema = CSharpG1ExpectedSchemaR2
	x.Inputs.KindMap = queryregistrycore.FilePin(CSharpG1KindMapPathR2, in.KindMap)
	x.Inputs.Previous = queryregistrycore.FilePin(CSharpG1ExpectedPath, in.Previous)
	x.Corrections = slices.Clone(cs)
	return x, nil
}

// ParseCSharpG1ExpectedR2 decodes the r2 asset and verifies it as described at the top of this file.
func ParseCSharpG1ExpectedR2(data []byte, in CSharpG1ExpectedR2Sources) (CSharpG1Expected, error) {
	x, err := DecodeCSharpG1Expected(data)
	if err != nil {
		return x, err
	}
	if x.Schema != CSharpG1ExpectedSchemaR2 {
		return x, g1eFault("G1_EXPECTED_SCHEMA")
	}
	// (1) the sealed r1 asset is intact
	r1, err := ParseCSharpG1Expected(in.Previous, in.R1)
	if err != nil {
		return x, err
	}
	// (3) the corrections are well formed and replace what r1 holds
	if err := g1CheckCorrections(x.Corrections, g1cExpectedField); err != nil {
		return x, err
	}
	want, err := g1cBuildExpected(r1, in, x.Corrections)
	if err != nil {
		return x, err
	}
	if err := g1cCheckEvidence(x.Corrections, in.R1.Observation, in.KindMap); err != nil {
		return x, err
	}
	// (2) no difference beyond the corrections
	if !g1eSame(x.Inputs, want.Inputs) {
		return x, g1cFault("G1_CORRECTION_PIN")
	}
	if !g1eSame(x.Alternatives, want.Alternatives) {
		return x, g1cFault("G1_CORRECTION_UNLISTED")
	}
	if x.Basis != want.Basis || x.WitnessLimit != want.WitnessLimit || !g1eSame(x.Counts, want.Counts) || !g1eSame(x.Cross, want.Cross) ||
		!g1eSame(x.SymbolsContext, want.SymbolsContext) || !g1eSame(x.Audit, want.Audit) || !g1eSame(x.Metamorphic, want.Metamorphic) {
		return x, g1cFault("G1_CORRECTION_UNLISTED")
	}
	// (4) the authored corrections
	if !g1eSame(x.Corrections, g1ExpectedCorrections) {
		return x, g1cFault("G1_CORRECTION_UNAUTHORED")
	}
	canonical, err := MarshalCSharpG1Expected(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1cFault("G1_CORRECTION_BYTES")
	}
	return x, nil
}

// ---- kind map -----------------------------------------------------------------------------------

func g1cKindMapField(f string) bool { return f == G1FieldEntry || f == G1FieldTSOnly }

// g1cApplyKindMap returns m with the corrections applied: the entry of the Roslyn kind must be the
// correction's Old, and the New entry must keep the kind, the class and the lexeme.
func g1cApplyKindMap(m CSharpG1KindMap, cs []G1Correction) (CSharpG1KindMap, error) {
	out := m
	out.Kinds = make([]G1KMKind, len(m.Kinds))
	for i, k := range m.Kinds {
		k.TS = slices.Clone(k.TS)
		out.Kinds[i] = k
	}
	out.TSOnly = slices.Clone(m.TSOnly)
	if out.TSOnly == nil {
		out.TSOnly = []G1KMTSOnly{}
	}
	for _, c := range cs {
		if c.Field == G1FieldTSOnly {
			// a named type that no kind refers to any more: absent from r1's list, a known reason, kept in type order
			var n G1KMTSOnly
			if c.Old != "null" || json.Unmarshal([]byte(c.New), &n) != nil || n.Type != c.Item || !slices.Contains(g1KindTSOnlyReasons, n.Reason) ||
				slices.ContainsFunc(out.TSOnly, func(o G1KMTSOnly) bool { return o.Type == c.Item }) || !G1KindMapHasType(m.Vocabulary, c.Item, true) {
				return out, g1cFault("G1_CORRECTION_OLD")
			}
			out.TSOnly = append(out.TSOnly, n)
			slices.SortFunc(out.TSOnly, func(a, b G1KMTSOnly) int { return strings.Compare(a.Type, b.Type) })
			continue
		}
		i := slices.IndexFunc(out.Kinds, func(k G1KMKind) bool { return k.Roslyn == c.Item })
		if i < 0 {
			return out, g1cFault("G1_CORRECTION_ITEM")
		}
		var n G1KMKind
		if g1cJSON(out.Kinds[i]) != c.Old || json.Unmarshal([]byte(c.New), &n) != nil || n.Roslyn != c.Item || n.Class != out.Kinds[i].Class || n.Text != out.Kinds[i].Text {
			return out, g1cFault("G1_CORRECTION_OLD")
		}
		if n.TS == nil {
			n.TS = []string{}
		}
		out.Kinds[i] = n
	}
	out.Counts = DeriveCSharpG1KindMapCounts(out)
	return out, nil
}

// CSharpG1KindMapR2Sources are the sealed r1 map and the Roslyn kind names the map must cover.
type CSharpG1KindMapR2Sources struct {
	Previous []byte
	Observed []string
}

func g1cBuildKindMap(r1 CSharpG1KindMap, previous []byte, cs []G1Correction) (CSharpG1KindMap, error) {
	m, err := g1cApplyKindMap(r1, cs)
	if err != nil {
		return m, err
	}
	m.Schema = CSharpG1KindMapSchemaR2
	if m.Grammar.GrammarJS.SHA256 != g1gGrammarSHA {
		return m, g1cFault("G1_CORRECTION_GROUND")
	}
	m.Previous = queryregistrycore.FilePin(CSharpG1KindMapPath, previous)
	m.Corrections = slices.Clone(cs)
	// the list of unreferenced types changes only by the ts_only corrections; that it is still exactly the
	// types no kind refers to is the structural check of the r2 map (G1_KINDMAP_TS_ONLY)
	return m, nil
}

// DeriveCSharpG1KindMapR2 derives the r2 map: the verified r1 map with the authored corrections.
func DeriveCSharpG1KindMapR2(in CSharpG1KindMapR2Sources) (CSharpG1KindMap, error) {
	r1, err := ParseCSharpG1KindMap(in.Previous, in.Observed)
	if err != nil {
		return r1, err
	}
	if r1.Schema != CSharpG1KindMapSchema {
		return r1, g1kmFault("G1_KINDMAP_SCHEMA")
	}
	return g1cBuildKindMap(r1, in.Previous, g1KindMapCorrections)
}

// ParseCSharpG1KindMapR2 decodes the r2 map and verifies it like the r2 expected asset.
func ParseCSharpG1KindMapR2(data []byte, in CSharpG1KindMapR2Sources) (CSharpG1KindMap, error) {
	m, err := ParseCSharpG1KindMap(data, in.Observed) // structure, consistency and canonical bytes of the r2 map itself
	if err != nil {
		return m, err
	}
	if m.Schema != CSharpG1KindMapSchemaR2 {
		return m, g1kmFault("G1_KINDMAP_SCHEMA")
	}
	r1, err := ParseCSharpG1KindMap(in.Previous, in.Observed)
	if err != nil {
		return m, err
	}
	if r1.Schema != CSharpG1KindMapSchema {
		return m, g1kmFault("G1_KINDMAP_SCHEMA")
	}
	if err := g1CheckCorrections(m.Corrections, g1cKindMapField); err != nil {
		return m, err
	}
	want, err := g1cBuildKindMap(r1, in.Previous, m.Corrections)
	if err != nil {
		return m, err
	}
	if m.Previous != want.Previous {
		return m, g1cFault("G1_CORRECTION_PIN")
	}
	if m.Basis != want.Basis || !g1eSame(m.Grammar, want.Grammar) || !g1eSame(m.Spec, want.Spec) || !g1eSame(m.Vocabulary, want.Vocabulary) ||
		!g1eSame(m.Kinds, want.Kinds) || !g1eSame(m.TSOnly, want.TSOnly) || m.Counts != want.Counts {
		return m, g1cFault("G1_CORRECTION_UNLISTED")
	}
	if !g1eSame(m.Corrections, g1KindMapCorrections) {
		return m, g1cFault("G1_CORRECTION_UNAUTHORED")
	}
	return m, nil
}

// G1ExpectedCorrections and G1KindMapCorrections return copies of the authored corrections, for the
// tool that checks their grounds against the pinned documents.
func G1ExpectedCorrections() []G1Correction { return slices.Clone(g1ExpectedCorrections) }
func G1KindMapCorrections() []G1Correction  { return slices.Clone(g1KindMapCorrections) }

// CSharpG1RoslynVocabulary returns the Roslyn kind names of the pinned observation, the list a kind map
// must cover exactly.
func CSharpG1RoslynVocabulary(observation []byte) ([]string, error) {
	var obs CSharpG1RoslynObserved
	if err := jsonv2.Unmarshal(observation, &obs, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, fail(KindInvalidInput, "G1_EXPECTED_OBSERVATION", CSharpG1RoslynPath, err)
	}
	return obs.Kinds, nil
}
