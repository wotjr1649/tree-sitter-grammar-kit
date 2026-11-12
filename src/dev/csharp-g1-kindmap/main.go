// Command csharp-g1-kindmap writes src/contracts/csharp-g1-kind-map-r1.json (#233 G1-3a): the
// Roslyn SyntaxKind -> tree-sitter-c-sharp node type map shared by the G1 expectation stages.
//
// The tree-sitter side is authored below from the pinned grammar text (grammar.js and
// src/node-types.json of the pinned tree-sitter-c-sharp commit, passed with -grammar), never from
// native output. The Roslyn side is the kind vocabulary of the pinned Roslyn observation. The command
// reads no native output and runs no Roslyn or dotnet.
//
//	go run ./src/dev/csharp-g1-kindmap -root . -grammar <tree-sitter-c-sharp checkout> [-write]
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	pinnedRepo   = "tree-sitter/tree-sitter-c-sharp"
	pinnedCommit = "9150f7d56bb47f1a809fa23623f1ba1413e93fa9"
	patchOrigin  = "src/dev/c2-patches/csharp.json"
)

func q(s string) string { return kit.G1KindMapAnon(s) }

// punctuators: Roslyn token kind -> its fixed lexeme (ECMA-334 6.4.6 operators and punctuators).
var punctuators = map[string]string{
	"OpenBraceToken": "{", "CloseBraceToken": "}", "OpenBracketToken": "[", "CloseBracketToken": "]",
	"OpenParenToken": "(", "CloseParenToken": ")", "DotToken": ".", "CommaToken": ",", "ColonToken": ":",
	"SemicolonToken": ";", "PlusToken": "+", "MinusToken": "-", "AsteriskToken": "*", "SlashToken": "/",
	"PercentToken": "%", "AmpersandToken": "&", "BarToken": "|", "CaretToken": "^", "ExclamationToken": "!",
	"TildeToken": "~", "EqualsToken": "=", "LessThanToken": "<", "GreaterThanToken": ">", "QuestionToken": "?",
	"QuestionQuestionToken": "??", "ColonColonToken": "::", "PlusPlusToken": "++", "MinusMinusToken": "--",
	"AmpersandAmpersandToken": "&&", "BarBarToken": "||", "MinusGreaterThanToken": "->", "EqualsEqualsToken": "==",
	"ExclamationEqualsToken": "!=", "LessThanEqualsToken": "<=", "GreaterThanEqualsToken": ">=",
	"PlusEqualsToken": "+=", "MinusEqualsToken": "-=", "AsteriskEqualsToken": "*=", "SlashEqualsToken": "/=",
	"PercentEqualsToken": "%=", "AmpersandEqualsToken": "&=", "BarEqualsToken": "|=", "CaretEqualsToken": "^=",
	"LessThanLessThanToken": "<<", "LessThanLessThanEqualsToken": "<<=", "EqualsGreaterThanToken": "=>",
	"QuestionQuestionEqualsToken": "??=", "DotDotToken": "..", "GreaterThanGreaterThanToken": ">>",
	"GreaterThanGreaterThanEqualsToken": ">>=", "GreaterThanGreaterThanGreaterThanToken": ">>>",
	"GreaterThanGreaterThanGreaterThanEqualsToken": ">>>=",
}

// keyword lexemes that are not the lower-cased kind name.
var keywordText = map[string]string{
	"ForEachKeyword": "foreach", "OrderByKeyword": "orderby", "ReadOnlyKeyword": "readonly", "SByteKeyword": "sbyte",
	"UIntKeyword": "uint", "ULongKeyword": "ulong", "UShortKeyword": "ushort", "SizeOfKeyword": "sizeof",
	"TypeOfKeyword": "typeof", "StackAllocKeyword": "stackalloc", "TypeVarKeyword": "typevar",
}

// grammar.js wrappers around a keyword: predefined_type is token(choice('bool', ...)), modifier and
// the aliased modifier of parameters are named nodes spanning the keyword, the *_literal rules wrap
// their keyword, identifier wraps the _reserved_identifier words.
var (
	predefined = strings.Fields("bool byte char decimal double float int long object sbyte short string uint ulong ushort void")
	modifiers  = strings.Fields("abstract async const extern file fixed internal new override partial private protected public readonly required sealed static unsafe virtual volatile ref out in this scoped")
	reservedID = strings.Fields("alias ascending by descending equals field file from global group into join let method notnull on orderby param property scoped select type typevar unmanaged var when where yield")
)

// tokens that are not a fixed lexeme: literal, identifier and text tokens.
var otherTokens = map[string][]string{
	"IdentifierToken":                           {"identifier", "implicit_parameter"},
	"UnderscoreToken":                           {"discard", "identifier"},
	"NumericLiteralToken":                       {"integer_literal", "real_literal"},
	"CharacterLiteralToken":                     {"character_literal"},
	"StringLiteralToken":                        {"string_literal", "verbatim_string_literal"},
	"Utf8StringLiteralToken":                    {"string_literal", "verbatim_string_literal"},
	"SingleLineRawStringLiteralToken":           {"raw_string_literal"},
	"MultiLineRawStringLiteralToken":            {"raw_string_literal"},
	"InterpolatedStringStartToken":              {"interpolation_start"},
	"InterpolatedVerbatimStringStartToken":      {"interpolation_start"},
	"InterpolatedSingleLineRawStringStartToken": {"interpolation_start"},
	"InterpolatedMultiLineRawStringStartToken":  {"interpolation_start"},
	"InterpolatedStringEndToken":                {q(`"`)},
	"InterpolatedRawStringEndToken":             {"interpolation_quote"},
	"InterpolatedStringTextToken":               {"string_content"},
}

var tokenNone = map[string]string{
	"EndOfFileToken":                  "ROSLYN_ONLY",
	"OmittedArraySizeExpressionToken": "ROSLYN_ONLY",
	"OmittedTypeArgumentToken":        "ROSLYN_ONLY",
}

// nodes: "Roslyn kind: ts types" or "Roslyn kind: - REASON". Names are grammar.js rule names.
const nodeTable = `
AccessorList: accessor_list
AddAccessorDeclaration: accessor_declaration
AddAssignmentExpression: assignment_expression
AddExpression: binary_expression
AddressOfExpression: prefix_unary_expression
AliasQualifiedName: alias_qualified_name
AllowsConstraintClause: - NOT_IN_PINNED_GRAMMAR
AndAssignmentExpression: assignment_expression
AndPattern: and_pattern
AnonymousMethodExpression: anonymous_method_expression
AnonymousObjectCreationExpression: anonymous_object_creation_expression
AnonymousObjectMemberDeclarator: - HIDDEN_NODE
Argument: argument
ArgumentList: argument_list
ArrayCreationExpression: array_creation_expression
ArrayInitializerExpression: initializer_expression
ArrayRankSpecifier: array_rank_specifier
ArrayType: array_type
ArrowExpressionClause: arrow_expression_clause
AsExpression: as_expression
AscendingOrdering: - HIDDEN_NODE
Attribute: attribute
AttributeArgument: attribute_argument
AttributeArgumentList: attribute_argument_list
AttributeList: attribute_list global_attribute
AttributeTargetSpecifier: attribute_target_specifier
AwaitExpression: await_expression
BaseConstructorInitializer: constructor_initializer
BaseExpression: "base"
BaseList: base_list
BitwiseAndExpression: binary_expression
BitwiseNotExpression: prefix_unary_expression
BitwiseOrExpression: binary_expression
Block: block
BracketedArgumentList: bracketed_argument_list element_binding_expression
BracketedParameterList: bracketed_parameter_list
BreakStatement: break_statement
CasePatternSwitchLabel: - NO_SEPARATE_NODE
CaseSwitchLabel: - NO_SEPARATE_NODE
CastExpression: cast_expression
CatchClause: catch_clause
CatchDeclaration: catch_declaration
CatchFilterClause: catch_filter_clause
CharacterLiteralExpression: character_literal
CheckedExpression: checked_expression
CheckedStatement: checked_statement
ClassConstraint: type_parameter_constraint
ClassDeclaration: class_declaration
CoalesceAssignmentExpression: assignment_expression
CoalesceExpression: binary_expression
CollectionExpression: collection_expression
CollectionInitializerExpression: initializer_expression
CompilationUnit: compilation_unit
ComplexElementInitializerExpression: initializer_expression
ConditionalAccessExpression: conditional_access_expression
ConditionalExpression: conditional_expression
ConstantPattern: constant_pattern
ConstructorConstraint: constructor_constraint type_parameter_constraint
ConstructorDeclaration: constructor_declaration
ContinueStatement: continue_statement
ConversionOperatorDeclaration: conversion_operator_declaration
DeclarationExpression: declaration_expression
DeclarationPattern: declaration_pattern
DefaultConstraint: - NOT_IN_PINNED_GRAMMAR
DefaultExpression: default_expression
DefaultLiteralExpression: default_expression
DefaultSwitchLabel: - NO_SEPARATE_NODE
DelegateDeclaration: delegate_declaration
DescendingOrdering: - HIDDEN_NODE
DestructorDeclaration: destructor_declaration
DiscardDesignation: discard
DiscardPattern: discard
DivideAssignmentExpression: assignment_expression
DivideExpression: binary_expression
DoStatement: do_statement
ElementAccessExpression: element_access_expression
ElementBindingExpression: element_binding_expression
ElseClause: - NO_SEPARATE_NODE
EmptyStatement: empty_statement
EnumDeclaration: enum_declaration
EnumMemberDeclaration: enum_member_declaration
EqualsExpression: binary_expression
EqualsValueClause: - NO_SEPARATE_NODE
EventDeclaration: event_declaration
EventFieldDeclaration: event_field_declaration
ExclusiveOrAssignmentExpression: assignment_expression
ExclusiveOrExpression: binary_expression
ExplicitInterfaceSpecifier: explicit_interface_specifier
ExpressionColon: - NO_SEPARATE_NODE
ExpressionElement: collection_element expression_element
ExpressionStatement: expression_statement
ExtensionBlockDeclaration: extension_declaration
ExternAliasDirective: extern_alias_directive
FieldDeclaration: field_declaration
FieldExpression: identifier
FileScopedNamespaceDeclaration: file_scoped_namespace_declaration
FinallyClause: finally_clause
FixedStatement: fixed_statement
FalseLiteralExpression: boolean_literal
ForEachStatement: foreach_statement
ForEachVariableStatement: foreach_statement
ForStatement: for_statement
FromClause: from_clause
FunctionPointerCallingConvention: calling_convention
FunctionPointerParameter: function_pointer_parameter
FunctionPointerParameterList: - NO_SEPARATE_NODE
FunctionPointerType: function_pointer_type
FunctionPointerUnmanagedCallingConvention: - NO_SEPARATE_NODE
FunctionPointerUnmanagedCallingConventionList: - NO_SEPARATE_NODE
GenericName: generic_name
GetAccessorDeclaration: accessor_declaration
GlobalStatement: global_statement
GotoCaseStatement: goto_statement
GotoDefaultStatement: goto_statement
GotoStatement: goto_statement
GreaterThanExpression: binary_expression
GreaterThanOrEqualExpression: binary_expression
GroupClause: group_clause
IdentifierName: identifier
IfStatement: if_statement
ImplicitArrayCreationExpression: implicit_array_creation_expression
ImplicitElementAccess: element_binding_expression
ImplicitObjectCreationExpression: implicit_object_creation_expression
ImplicitStackAllocArrayCreationExpression: implicit_stackalloc_expression
IncompleteMember: - ROSLYN_ONLY
IndexExpression: prefix_unary_expression
IndexerDeclaration: indexer_declaration
InitAccessorDeclaration: accessor_declaration
InterfaceDeclaration: interface_declaration
InterpolatedStringExpression: interpolated_string_expression
InterpolatedStringText: string_content
Interpolation: interpolation
InterpolationAlignmentClause: interpolation_alignment_clause
InterpolationFormatClause: interpolation_format_clause
InvocationExpression: invocation_expression
IsExpression: is_expression is_pattern_expression
IsPatternExpression: is_pattern_expression
JoinClause: join_clause
JoinIntoClause: join_into_clause
LabeledStatement: labeled_statement
LeftShiftAssignmentExpression: assignment_expression
LeftShiftExpression: binary_expression
LessThanExpression: binary_expression
LessThanOrEqualExpression: binary_expression
LetClause: let_clause
ListPattern: list_pattern
LocalDeclarationStatement: local_declaration_statement
LocalFunctionStatement: local_function_statement
LockStatement: lock_statement
LogicalAndExpression: binary_expression
LogicalNotExpression: prefix_unary_expression
LogicalOrExpression: binary_expression
MemberBindingExpression: member_binding_expression
MethodDeclaration: method_declaration
ModuloAssignmentExpression: assignment_expression
ModuloExpression: binary_expression
MultiplyAssignmentExpression: assignment_expression
MultiplyExpression: binary_expression
NameColon: - NO_SEPARATE_NODE
NameEquals: - NO_SEPARATE_NODE
NamespaceDeclaration: namespace_declaration
NotEqualsExpression: binary_expression
NotPattern: negated_pattern
NullLiteralExpression: null_literal
NullableType: nullable_type
NumericLiteralExpression: integer_literal real_literal
ObjectCreationExpression: object_creation_expression
ObjectInitializerExpression: initializer_expression
OmittedArraySizeExpression: - ROSLYN_ONLY
OmittedTypeArgument: - ROSLYN_ONLY
OperatorDeclaration: operator_declaration
OrAssignmentExpression: assignment_expression
OrPattern: or_pattern
OrderByClause: order_by_clause
Parameter: implicit_parameter parameter receiver_parameter
ParameterList: parameter_list
ParenthesizedExpression: parenthesized_expression
ParenthesizedLambdaExpression: lambda_expression
ParenthesizedPattern: parenthesized_pattern
ParenthesizedVariableDesignation: parenthesized_variable_designation
PointerIndirectionExpression: prefix_unary_expression
PointerMemberAccessExpression: member_access_expression
PointerType: pointer_type
PositionalPatternClause: positional_pattern_clause
PostDecrementExpression: postfix_unary_expression
PostIncrementExpression: postfix_unary_expression
PreDecrementExpression: prefix_unary_expression
PreIncrementExpression: prefix_unary_expression
PredefinedType: predefined_type
PrimaryConstructorBaseType: primary_constructor_base_type
PropertyDeclaration: property_declaration
PropertyPatternClause: property_pattern_clause
QualifiedName: qualified_name
QueryBody: - HIDDEN_NODE
QueryContinuation: - NO_SEPARATE_NODE
QueryExpression: query_expression
RangeExpression: range_expression
RecordDeclaration: record_declaration
RecordStructDeclaration: record_declaration
RecursivePattern: recursive_pattern
RefExpression: ref_expression
RefStructConstraint: - NOT_IN_PINNED_GRAMMAR
RefType: ref_type
RelationalPattern: relational_pattern
RemoveAccessorDeclaration: accessor_declaration
ReturnStatement: return_statement
RightShiftAssignmentExpression: assignment_expression
RightShiftExpression: binary_expression
ScopedType: scoped_type
SelectClause: select_clause
SetAccessorDeclaration: accessor_declaration
SimpleAssignmentExpression: assignment_expression with_initializer
SimpleBaseType: - NO_SEPARATE_NODE
SimpleLambdaExpression: lambda_expression
SimpleMemberAccessExpression: member_access_expression
SingleVariableDesignation: identifier
SizeOfExpression: sizeof_expression
SlicePattern: slice_pattern
SpreadElement: collection_element spread_element
StackAllocArrayCreationExpression: stackalloc_expression
StringLiteralExpression: raw_string_literal string_literal verbatim_string_literal
StructConstraint: type_parameter_constraint
StructDeclaration: struct_declaration
Subpattern: subpattern
SubtractAssignmentExpression: assignment_expression
SubtractExpression: binary_expression
SuppressNullableWarningExpression: postfix_unary_expression
SwitchExpression: switch_expression
SwitchExpressionArm: switch_expression_arm
SwitchSection: switch_section
SwitchStatement: switch_statement
ThisConstructorInitializer: constructor_initializer
ThisExpression: "this"
ThrowExpression: throw_expression
ThrowStatement: throw_statement
TrueLiteralExpression: boolean_literal
TryStatement: try_statement
TupleElement: tuple_element
TupleExpression: tuple_expression
TupleType: tuple_type
TypeArgumentList: type_argument_list
TypeConstraint: type_parameter_constraint
TypeOfExpression: typeof_expression
TypeParameter: type_parameter
TypeParameterConstraintClause: type_parameter_constraints_clause
TypeParameterList: type_parameter_list
TypePattern: type_pattern
UnaryMinusExpression: prefix_unary_expression
UnaryPlusExpression: prefix_unary_expression
UncheckedExpression: checked_expression
UncheckedStatement: checked_statement
UnknownAccessorDeclaration: accessor_declaration
UnsafeStatement: unsafe_statement
UnsignedRightShiftAssignmentExpression: assignment_expression
UnsignedRightShiftExpression: binary_expression
UsingDirective: using_directive
UsingStatement: using_statement
Utf8StringLiteralExpression: raw_string_literal string_literal verbatim_string_literal
VarPattern: var_pattern
VariableDeclaration: variable_declaration
VariableDeclarator: variable_declarator
WhenClause: when_clause
WhereClause: where_clause
WhileStatement: while_statement
WithExpression: with_expression
WithInitializerExpression: - NO_SEPARATE_NODE
YieldBreakStatement: yield_statement
YieldReturnStatement: yield_statement
`

// named node types that no Roslyn kind maps to, with the reason.
var tsOnlyReasons = map[string]string{}

func init() {
	set := func(reason string, names string) {
		for _, n := range strings.Fields(names) {
			tsOnlyReasons[n] = reason
		}
	}
	set("SUPERTYPE", "declaration expression literal lvalue_expression non_lvalue_expression pattern statement type type_declaration")
	set("TS_WRAPPER", "character_literal_content declaration_list enum_member_declaration_list extension_body modifier string_literal_content switch_body")
	set("TRIVIA_OR_DIRECTIVE", "comment unary_expression preproc_arg preproc_define preproc_elif preproc_else preproc_endregion preproc_error preproc_if preproc_if_in_attribute_list preproc_line preproc_nullable preproc_pragma preproc_region preproc_undef preproc_warning shebang_directive")
	set("EXTERNAL_SCANNER_NODE", "escape_sequence interpolation_brace interpolation_quote raw_string_content raw_string_end raw_string_start string_literal_encoding")
	set("ROSLYN_KIND_NOT_OBSERVED", "makeref_expression reftype_expression refvalue_expression tuple_pattern")
	set("PATCH_ONLY_CONSTRUCT", "file_based_app_directive if_header preproc_skipped_section preproc_try_statement")
}

type nodeType struct {
	Type  string `json:"type"`
	Named bool   `json:"named"`
	Sub   []any  `json:"subtypes"`
}

func collectVocabulary(raw []byte) (named, anon []string, err error) {
	var top []any
	if err = json.Unmarshal(raw, &top); err != nil {
		return
	}
	n, a := map[string]bool{}, map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if t, ok := x["type"].(string); ok {
				if nm, ok := x["named"].(bool); ok {
					if nm {
						n[t] = true
					} else {
						a[t] = true
					}
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(top)
	for k := range n {
		named = append(named, k)
	}
	for k := range a {
		anon = append(anon, k)
	}
	slices.Sort(named)
	slices.Sort(anon)
	return
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func build(root, grammar string) (kit.CSharpG1KindMap, error) {
	var m kit.CSharpG1KindMap
	obsData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1RoslynPath)))
	if err != nil {
		return m, err
	}
	var obs struct {
		Kinds []string `json:"kinds"`
	}
	if err := json.Unmarshal(obsData, &obs); err != nil {
		return m, err
	}
	gjs, err := os.ReadFile(filepath.Join(grammar, "grammar.js"))
	if err != nil {
		return m, err
	}
	nt, err := os.ReadFile(filepath.Join(grammar, "src", "node-types.json"))
	if err != nil {
		return m, err
	}
	named, anon, err := collectVocabulary(nt)
	if err != nil {
		return m, err
	}
	adds := []kit.G1KMAddition{}
	for _, t := range []string{"file_based_app_directive", "if_header", "preproc_skipped_section", "preproc_try_statement"} {
		if slices.Contains(named, t) {
			return m, fmt.Errorf("%s is already upstream", t)
		}
		named = append(named, t)
		adds = append(adds, kit.G1KMAddition{Type: t, Named: true, Origin: patchOrigin})
	}
	slices.Sort(named)
	m = kit.CSharpG1KindMap{
		Schema: kit.CSharpG1KindMapSchema,
		Basis:  "Maps every Roslyn SyntaxKind of the pinned observation (node and token kinds) to the tree-sitter-c-sharp node types the pinned grammar text allows for the same source extent. 1:1 has one type, 1:n several (the grammar allows more than one node type for the construct: a supertype member, an alias, or a wrapper that spans the same text), none has a reason. Anonymous node types are written quoted. The tree-sitter side is read from grammar.js and node-types.json only; no native output and no Roslyn output other than the kind names was read.",
		Spec: []string{
			"C# standard draft v13 as published at learn.microsoft.com/dotnet/csharp/language-reference/language-specification (dotnet/csharpstandard alpha-v13, commit 79ad4e90ef63; the ECMA-334 successor text): 6.4.4 keywords, 6.4.5 literals, 6.4.6 operators and punctuators, 12 expressions, 13 statements, 14-19 declarations",
			"tree-sitter-c-sharp grammar.js and src/node-types.json at the pinned commit",
		},
		Grammar: kit.G1KMGrammar{
			Repository: pinnedRepo, Commit: pinnedCommit,
			GrammarJS: kit.G1KMFile{Path: "grammar.js", Bytes: uint64(len(gjs)), SHA256: sha(gjs)},
			NodeTypes: kit.G1KMFile{Path: "src/node-types.json", Bytes: uint64(len(nt)), SHA256: sha(nt)},
			Additions: adds,
			Limit:     "The vocabulary is the pinned upstream node-types.json plus the four named node types the repository's C2 patch (src/dev/c2-patches/csharp.json) adds and the registered case expectations already use. Other node types the patch chain may add are not enumerated; the generated parser is not read.",
		},
		Vocabulary: kit.G1KMVocabulary{Named: named, Anonymous: anon},
	}
	vocab := m.Vocabulary
	has := func(e string) bool { return kit.G1KindMapHasEntry(vocab, e) }

	nodes := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(nodeTable), "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if !ok || nodes[k] != "" {
			return m, fmt.Errorf("bad node line %q", line)
		}
		nodes[k] = v
	}
	var missing []string
	for _, k := range obs.Kinds {
		e := kit.G1KMKind{Roslyn: k, Class: "node", TS: []string{}}
		switch {
		case kit.G1KindMapIsToken(k):
			e.Class = "token"
			var ts []string
			switch {
			case tokenNone[k] != "":
				e.Reason = tokenNone[k]
			case otherTokens[k] != nil:
				for _, t := range otherTokens[k] {
					ts = append(ts, t)
				}
			case punctuators[k] != "":
				e.Text = punctuators[k]
				ts = []string{q(e.Text)}
			case strings.HasSuffix(k, "Keyword"):
				w := keywordText[k]
				if w == "" {
					w = strings.ToLower(strings.TrimSuffix(k, "Keyword"))
				}
				e.Text = w
				if has(q(w)) {
					ts = append(ts, q(w))
				}
				if slices.Contains(predefined, w) {
					ts = append(ts, "predefined_type")
				}
				if slices.Contains(modifiers, w) {
					ts = append(ts, "modifier")
				}
				if slices.Contains(reservedID, w) {
					ts = append(ts, "identifier")
				}
				switch w {
				case "true", "false":
					ts = append(ts, "boolean_literal")
				case "null":
					ts = append(ts, "null_literal")
				case "var":
					ts = append(ts, "implicit_type")
				}
				if len(ts) == 0 {
					e.Reason = "NOT_IN_PINNED_GRAMMAR"
				}
			default:
				return m, fmt.Errorf("token kind %s has no rule", k)
			}
			e.TS = dedupe(ts)
		default:
			spec, ok := nodes[k]
			if !ok {
				missing = append(missing, k)
				continue
			}
			delete(nodes, k)
			if strings.HasPrefix(spec, "- ") {
				e.Reason = strings.TrimPrefix(spec, "- ")
			} else {
				e.TS = dedupe(strings.Fields(spec))
			}
		}
		for _, t := range e.TS {
			if !has(t) {
				return m, fmt.Errorf("%s: %s is not in the vocabulary", k, t)
			}
		}
		switch len(e.TS) {
		case 0:
			e.Map = kit.G1MapNone
		case 1:
			e.Map = kit.G1MapOneToOne
		default:
			e.Map = kit.G1MapOneToN
		}
		m.Kinds = append(m.Kinds, e)
	}
	if len(missing) != 0 {
		return m, fmt.Errorf("node kinds without an entry: %v", missing)
	}
	if len(nodes) != 0 {
		return m, fmt.Errorf("node entries for kinds that are not observed: %v", nodes)
	}
	use := map[string]bool{}
	for _, k := range m.Kinds {
		for _, t := range k.TS {
			use[t] = true
		}
	}
	var unreasoned []string
	for _, n := range vocab.Named {
		if use[n] {
			continue
		}
		r, ok := tsOnlyReasons[n]
		if !ok {
			unreasoned = append(unreasoned, n)
			continue
		}
		m.TSOnly = append(m.TSOnly, kit.G1KMTSOnly{Type: n, Reason: r})
	}
	if len(unreasoned) != 0 {
		return m, fmt.Errorf("unmapped named types without a reason: %v", unreasoned)
	}
	m.Counts = kit.DeriveCSharpG1KindMapCounts(m)
	return m, nil
}

func dedupe(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func run(root, grammar string, write bool) error {
	m, err := build(root, grammar)
	if err != nil {
		return err
	}
	data, err := kit.MarshalCSharpG1KindMap(m)
	if err != nil {
		return err
	}
	obsData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(kit.CSharpG1RoslynPath)))
	if err != nil {
		return err
	}
	var obs struct {
		Kinds []string `json:"kinds"`
	}
	if err := json.Unmarshal(obsData, &obs); err != nil {
		return err
	}
	if _, err := kit.ParseCSharpG1KindMap(data, obs.Kinds); err != nil {
		return fmt.Errorf("generated map does not parse: %w", err)
	}
	path := filepath.Join(root, filepath.FromSlash(kit.CSharpG1KindMapPath))
	if !write {
		old, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(old, data) {
			return errors.New("tracked kind map differs from the generated one")
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d bytes %+v\n", kit.CSharpG1KindMapPath, len(data), m.Counts)
	return nil
}

func main() {
	root := flag.String("root", ".", "repository root")
	grammar := flag.String("grammar", "", "pinned tree-sitter-c-sharp checkout (grammar.js, src/node-types.json)")
	write := flag.Bool("write", false, "write the asset instead of comparing")
	flag.Parse()
	if *grammar == "" {
		fmt.Fprintln(os.Stderr, "-grammar is required")
		os.Exit(2)
	}
	if err := run(*root, *grammar, *write); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
