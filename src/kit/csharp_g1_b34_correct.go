package kit

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Post-seal corrections of the sealed G1-3b expectations of csharp-B03 and csharp-B04 (#233, r2).
//
// The r1 asset (csharp-g1-expected-b03b04-r1.json) was sealed before any native output existed. The native comparison then showed five
// values that contradict the pinned grammar text: the anchor `var` of four alternatives (csharp-B03.a184, a236, csharp-B04.a028, a029) and
// the witness of csharp-B03.a239. The sealed r1 file stays byte for byte as sealed. The r2 file is the r1 file plus an explicit list of
// corrections, made the way G1-3a made its r2 (csharp_g1_correct.go): each correction has the value it replaces, the value it sets and its
// grounds in the pinned specification and grammar text, and is marked after_native (it was made after the native result was seen) and
// basis_independent (its grounds are the specification and the grammar text only). Corrections count apart from the independent
// expectations (post_seal in the asset).
//
//   - The four anchors get the type the pinned grammar text gives the lexeme `var` in that context: implicit_type (`implicit_type: 'var'`)
//     and its anonymous token "var". The kind map's type for the Roslyn IdentifierToken observed there (identifier, implicit_parameter) is
//     right for an identifier and was the wrong source for a type position. The discriminating flag of a corrected alternative is not
//     changed: a stronger claim made after the native result would count the mismatch as a win.
//   - csharp-B03.a239 becomes BLOCKED with the registered reason code POSITION_DEPENDENT: the node of the construct depends on the slot
//     it occupies (Roslyn ParenthesizedVariableDesignation, grammar tuple_pattern or parenthesized_variable_designation), and the
//     alternative has one witness only.
//
// No function of this file reads a native record: the corrections are authored literals and the code applies them to the verified r1
// derivation (the closure of the separation test proves the call path; the time of authoring is the after_native flag).

const (
	CSharpG1B34SchemaR2 = "tsgk-csharp-g1-expected-b03b04/r2"
	CSharpG1B34PathR2   = "src/contracts/csharp-g1-expected-b03b04-r2.json"

	// g1b34FieldAlternative replaces the whole alternative (old and new are its JSON): the alternative becomes BLOCKED.
	g1b34FieldAlternative = "alternative"
)

// g1b34FieldAnchorTS is the field "anchors[N].ts": the types of the Nth anchor of an A alternative.
var g1b34FieldAnchorTS = regexp.MustCompile(`^anchors\[(0|[1-9][0-9]*)\]\.ts$`)

// G1B34PostSeal counts the post-seal corrections apart from the expectations: every correction is after_native, and the alternatives
// they touch are counted by what the correction does.
type G1B34PostSeal struct {
	Corrections      int `json:"corrections"`
	AfterNative      int `json:"after_native"`
	BasisIndependent int `json:"basis_independent"`
	Alternatives     int `json:"alternatives"`
	AnchorTypes      int `json:"anchor_types"`
	Blocked          int `json:"blocked"`
}

const csharpG1B34BasisR2 = csharpG1B34Basis + " Revision r2 is the sealed r1 expected asset with the corrections listed in `corrections`, made after the native comparison and each with its grounds in the specification and grammar text (after_native true, basis_independent true); r1 is unchanged. The counts of `post_seal` are the post-seal corrections, apart from the expectations made before the seal."

func g1b34cFault(code string) error { return fail(KindInvalidInput, code, CSharpG1B34PathR2, nil) }

const g1b34cVarReason = "The sealed types of the anchor `var` are the kind map's types for the Roslyn IdentifierToken observed there (identifier, implicit_parameter), a context-free list that is right for an identifier. In this alternative `var` is the implicit type of a declaration: the pinned grammar text defines implicit_type: 'var' and lists implicit_type in type, so the types the grammar gives the lexeme are the named node implicit_type and its anonymous token \"var\". The type audit r2 derives the same set from the grammar text alone."

// g1B34Corrections are the corrections of the expected asset, authored from the grammar and specification text only.
var g1B34Corrections = []G1Correction{
	g1b34AnchorVar("csharp-B03.a184", g1b34cVarReason+" Here `var` is the type of a declaration expression (local_variable_type of 12.20).",
		G1Ground{G1DocGrammar, "declaration_expression", "declaration_expression: $ => prec.dynamic(1, seq( field('type', $.type),", g1gGrammarSHA, "declaration_expression:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.20 local_variable_type", ": type | 'var' ; ``` ‘scoped’ shall only be permitted with *local_variable_type* if *local_variable_type* is ‘var’ or a ref struct type, and *identifer* is not a discard.", g1gExprSHA, "## 12.20 Declaration expressions", 10}),
	g1b34AnchorVar("csharp-B03.a236", g1b34cVarReason+" Here `var` is the type of an abridged deconstructor `var (a, b)`, which the specification defines as the shorthand of declaration expressions with the implicit type (12.24.3.2); the grammar text gives it as the type of a variable_declaration whose declarator is a tuple_pattern.",
		G1Ground{G1DocGrammar, "variable_declaration", "variable_declaration: $ => seq( field('type', $.type), commaSep1($.variable_declarator),", g1gGrammarSHA, "variable_declaration:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.24.3.2 abridged_deconstructor", ": 'var' abridged_deconstructor_elements ;", g1gExprSHA, "#### 12.24.3.2 Abridged deconstructors", 6}),
	g1b34AnchorVar("csharp-B04.a028", g1b34cVarReason+" Here `var` is the type of an implicitly typed local variable declaration (13.6.2.2), the type of a variable_declaration.",
		G1Ground{G1DocGrammar, "variable_declaration", "variable_declaration: $ => seq( field('type', $.type), commaSep1($.variable_declarator),", g1gGrammarSHA, "variable_declaration:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/statements.md 13.6.2.2 implicitly_typed_local_variable_declaration", "'scoped'? 'var' implicitly_typed_local_variable_declarator", g1gStatementsSHA, "#### 13.6.2.2 Implicitly typed local variable declarations", 4}),
	g1b34AnchorVar("csharp-B04.a029", g1b34cVarReason+" Here `var` is the type of an implicitly typed ref local variable declaration (13.6.2.2, second alternative), the type of a variable_declaration whose type is a ref_type over implicit_type.",
		G1Ground{G1DocGrammar, "ref_type", "ref_type: $ => seq( 'ref', optional('readonly'), field('type', $.type), ),", g1gGrammarSHA, "ref_type:", 0},
		G1Ground{G1DocSpec, g1gSpecPre + "standard/statements.md 13.6.2.2 implicitly_typed_local_variable_declaration", "'scoped'? 'var' implicitly_typed_local_variable_declarator", g1gStatementsSHA, "#### 13.6.2.2 Implicitly typed local variable declarations", 4}),
	{
		Item: "csharp-B03.a239", Field: g1b34FieldAlternative, AfterNative: true, BasisIndependent: true,
		Old:    `{"id":"csharp-B03.a239","row":"csharp-B03","group":"B","status":"EXPECTED","basis":"differential-oracle","discriminating":true,"form":"node ParenthesizedVariableDesignation under ParenthesizedVariableDesignation","spec":"C# standard v13 §12.2.4.3.2 (expressions#122432-abridged-deconstructors)","kinds":["ParenthesizedVariableDesignation"],"parent":["ParenthesizedVariableDesignation"],"ts":["parenthesized_variable_designation"],"witnesses":[{"source":"alt-csharp-B03-09#0","match":"span","span":[287,293],"roslyn":"ParenthesizedVariableDesignation"}]}`,
		New:    `{"id":"csharp-B03.a239","row":"csharp-B03","group":"C","status":"BLOCKED","discriminating":false,"spec":"C# standard v13 §12.2.4.3.2 (expressions#122432-abridged-deconstructors)","witnesses":[],"block":"POSITION_DEPENDENT","reason":"Roslyn's ParenthesizedVariableDesignation and the grammar's node are not one to one in this context: the nested designation of an implicitly typed deconstruction is a tuple_pattern of a variable_declarator in the pinned grammar text, while parenthesized_variable_designation exists only under _variable_designation (patterns). The node is decided by the slot it occupies and the alternative has one witness only, so it has no expectation."}`,
		Reason: "The sealed expectation (a node parenthesized_variable_designation with the span of the nested designation) holds for the pattern contexts of the alternative, but its only witness is the nested designation of `var (a, (b, c)) = ...`. There the pinned grammar text has variable_declarator choose between a name and a tuple_pattern, and tuple_pattern nests tuple_pattern, so the grammar's node is a tuple_pattern and parenthesized_variable_designation does not occur. Roslyn's kind and the grammar's node are not one to one in this context, and a second witness in a pattern context is a new expectation, not a correction. The alternative is BLOCKED with the registered code POSITION_DEPENDENT (the node is decided by the slot), the code the classification gives the other alternatives whose node depends on the slot (tuple_element).",
		Grounds: []G1Ground{
			{G1DocGrammar, "variable_declarator", "variable_declarator: $ => seq( choice(field('name', $.identifier), $.tuple_pattern),", g1gGrammarSHA, "variable_declarator:", 0},
			{G1DocGrammar, "tuple_pattern", "tuple_pattern: $ => seq( '(', commaSep1(choice( field('name', $.identifier),", g1gGrammarSHA, "tuple_pattern:", 0},
			{G1DocGrammar, "_variable_designation", "_variable_designation: $ => prec(1, choice( $.discard,", g1gGrammarSHA, "_variable_designation:", 0},
			{G1DocSpec, g1gSpecPre + "standard/expressions.md 12.24.3.2 abridged_deconstructor_elements", ": '(' abridged_deconstructor_element (',' abridged_deconstructor_element)+ ')'", g1gExprSHA, "#### 12.24.3.2 Abridged deconstructors", 10},
		},
	},
}

// g1b34AnchorVar is the correction of the types of the first anchor `var` of an alternative: the types the grammar text gives it as an implicit type.
func g1b34AnchorVar(item, reason string, extra ...G1Ground) G1Correction {
	grounds := append([]G1Ground{
		{G1DocGrammar, "implicit_type", "implicit_type: _ => prec.dynamic(1, 'var'), array_type: $ => seq(", g1gGrammarSHA, "implicit_type:", 0},
		{G1DocGrammar, "type", "type: $ => choice( $.implicit_type, $.array_type, $._name,", g1gGrammarSHA, "type:", 0},
	}, extra...)
	return G1Correction{
		Item: item, Field: "anchors[0].ts", Old: `["identifier","implicit_parameter"]`, New: `["\"var\"","implicit_type"]`,
		Reason: reason, Grounds: grounds, AfterNative: true, BasisIndependent: true,
	}
}

func g1b34cField(f string) bool {
	return f == g1b34FieldAlternative || g1b34FieldAnchorTS.MatchString(f)
}

func g1b34cClone(x CSharpG1B34) CSharpG1B34 {
	alts := make([]G1B34Alt, len(x.Alternatives))
	for i, a := range x.Alternatives {
		a.Anchors = slices.Clone(a.Anchors)
		a.TS = slices.Clone(a.TS)
		a.Witnesses = slices.Clone(a.Witnesses)
		alts[i] = a
	}
	x.Alternatives = alts
	return x
}

// g1b34cApply returns x with the corrections applied. Each must name an EXPECTED alternative and a field whose current value is the
// correction's Old; the New value must be well formed.
func g1b34cApply(x CSharpG1B34, cs []G1Correction) (CSharpG1B34, error) {
	out := g1b34cClone(x)
	for _, c := range cs {
		i := slices.IndexFunc(out.Alternatives, func(a G1B34Alt) bool { return a.ID == c.Item })
		if i < 0 || out.Alternatives[i].Status != G1ExpExpected {
			return out, g1b34cFault("G1_CORRECTION_ITEM")
		}
		alt := &out.Alternatives[i]
		if c.Field == g1b34FieldAlternative {
			var n G1B34Alt
			if g1cJSON(*alt) != c.Old || json.Unmarshal([]byte(c.New), &n) != nil || g1cJSON(n) != c.New {
				return out, g1b34cFault("G1_CORRECTION_OLD")
			}
			if n.ID != alt.ID || n.Row != alt.Row || n.Spec != alt.Spec || n.Group != G1ClassBlocked || n.Status != G1ExpBlocked || !slices.Contains(G1ClassBlockCodes, n.Block) || n.Reason == "" ||
				n.Basis != "" || n.Discriminating || n.Form != "" || len(n.Anchors)+len(n.Kinds)+len(n.Parent)+len(n.TS)+len(n.Witnesses) != 0 || n.Witnesses == nil {
				return out, g1b34cFault("G1_CORRECTION_OLD")
			}
			*alt = n
			continue
		}
		m := g1b34FieldAnchorTS.FindStringSubmatch(c.Field)
		if m == nil {
			return out, g1b34cFault("G1_CORRECTION_FORM")
		}
		k, _ := strconv.Atoi(m[1])
		var ts []string
		if alt.Group != G1ClassLiteral || k >= len(alt.Anchors) || g1cJSON(alt.Anchors[k].TS) != c.Old || json.Unmarshal([]byte(c.New), &ts) != nil ||
			len(ts) == 0 || !slices.IsSorted(ts) || len(slices.Compact(slices.Clone(ts))) != len(ts) || slices.Contains(ts, "") {
			return out, g1b34cFault("G1_CORRECTION_OLD")
		}
		alt.Anchors[k].TS = ts
	}
	out.Counts = g1b34Counts(out.Alternatives)
	return out, nil
}

func g1b34cPostSeal(cs []G1Correction) *G1B34PostSeal {
	p := &G1B34PostSeal{Corrections: len(cs)}
	items := map[string]bool{}
	for _, c := range cs {
		items[c.Item] = true
		if c.AfterNative {
			p.AfterNative++
		}
		if c.BasisIndependent {
			p.BasisIndependent++
		}
		if c.Field == g1b34FieldAlternative {
			p.Blocked++
		} else {
			p.AnchorTypes++
		}
	}
	p.Alternatives = len(items)
	return p
}

// CSharpG1B34R2Sources are the inputs of the r2 asset: the inputs of the r1 derivation and the sealed r1 asset.
type CSharpG1B34R2Sources struct {
	R1       CSharpG1ExpectedSources
	Previous []byte
}

func g1b34cBuild(r1 CSharpG1B34, previous []byte, cs []G1Correction) (CSharpG1B34, error) {
	x, err := g1b34cApply(r1, cs)
	if err != nil {
		return x, err
	}
	x.Schema, x.Basis = CSharpG1B34SchemaR2, csharpG1B34BasisR2
	x.Inputs.Previous = queryregistrycore.FilePin(CSharpG1B34Path, previous)
	x.PostSeal = g1b34cPostSeal(cs)
	x.Corrections = slices.Clone(cs)
	return x, nil
}

// DeriveCSharpG1B34R2 derives the r2 asset: the verified r1 asset with the authored corrections.
func DeriveCSharpG1B34R2(in CSharpG1B34R2Sources) (CSharpG1B34, error) {
	r1, err := ParseCSharpG1B34(in.Previous, in.R1)
	if err != nil {
		return r1, err
	}
	return g1b34cBuild(r1, in.Previous, g1B34Corrections)
}

// ParseCSharpG1B34R2 decodes the r2 asset and verifies it: the r1 asset it pins verifies as the sealed asset, every correction is well
// formed, has grounds and replaces the value r1 holds, the asset is the r1 asset with exactly its corrections applied and the corrections
// are the authored ones.
func ParseCSharpG1B34R2(data []byte, in CSharpG1B34R2Sources) (CSharpG1B34, error) {
	x, err := DecodeCSharpG1B34(data)
	if err != nil {
		return x, err
	}
	if x.Schema != CSharpG1B34SchemaR2 {
		return x, g1b34Fault("G1_B34_SCHEMA")
	}
	r1, err := ParseCSharpG1B34(in.Previous, in.R1)
	if err != nil {
		return x, err
	}
	if err := g1CheckCorrections(x.Corrections, g1b34cField); err != nil {
		return x, err
	}
	want, err := g1b34cBuild(r1, in.Previous, x.Corrections)
	if err != nil {
		return x, err
	}
	if !g1b34Same(x.Inputs, want.Inputs) {
		return x, g1b34cFault("G1_CORRECTION_PIN")
	}
	if !g1b34Same(x.Alternatives, want.Alternatives) || x.Counts != want.Counts || !g1b34Same(x.PostSeal, want.PostSeal) || x.WitnessLimit != want.WitnessLimit || x.Basis != want.Basis {
		return x, g1b34cFault("G1_CORRECTION_UNLISTED")
	}
	if !g1b34Same(x.Corrections, g1B34Corrections) {
		return x, g1b34cFault("G1_CORRECTION_UNAUTHORED")
	}
	canonical, err := MarshalCSharpG1B34(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1b34cFault("G1_CORRECTION_BYTES")
	}
	return x, nil
}

// G1B34Corrections returns a copy of the authored corrections, for the tool that checks their grounds against the pinned documents.
func G1B34Corrections() []G1Correction { return slices.Clone(g1B34Corrections) }
