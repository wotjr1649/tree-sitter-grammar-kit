package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
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
// correction is marked post_native: it was made after the native result was seen, so it counts apart
// from the independent expectations (see the specification, "Corrections after the seal").
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
	G1FieldTSOnly       = "ts_only"

	// where a ground is written
	G1DocGrammar = "grammar.js"
	G1DocSpec    = "spec"
)

// G1Ground is one place in the pinned grammar text or the specification that supports a correction.
// Quote is a verbatim text of that place (whitespace runs count as one space).
type G1Ground struct {
	Doc   string `json:"doc"`
	At    string `json:"at"`
	Quote string `json:"quote"`
}

// G1Correction replaces Old by New in the field Field of the item Item. Old and New are JSON texts:
// the old value r1 holds and the value r2 holds.
type G1Correction struct {
	Item       string     `json:"item"`
	Field      string     `json:"field"`
	Old        string     `json:"old"`
	New        string     `json:"new"`
	Reason     string     `json:"reason"`
	Grounds    []G1Ground `json:"ground"`
	PostNative bool       `json:"post_native"`
}

func g1cFault(code string) error { return fail(KindInvalidInput, code, CSharpG1ExpectedPathR2, nil) }

const (
	g1gSpecPre = "csharpstandard@79ad4e90ef637621b9c25c706428a30278711deb (alpha-v13) "
	g1gLex     = g1gSpecPre + "standard/lexical-structure.md "
	g1gExpr    = g1gSpecPre + "standard/expressions.md "
)

// g1ExpectedCorrections are the four corrections of the expected asset, authored from the grammar and
// specification text only.
var g1ExpectedCorrections = []G1Correction{
	{
		Item: "csharp-B01.a335", Field: G1FieldTS, Old: `["identifier"]`, New: `["preproc_arg"]`, PostNative: true,
		Reason: "The specification defines the symbol of #define and #undef as PP_Conditional_Symbol, a lexeme, and names no tree-sitter type. The pinned grammar text builds preproc_define and preproc_undef from the directive and one preproc_arg token, which runs to the end of the line, and has no identifier rule in them (identifier appears only in the #if and #elif condition). The witness is the symbol DEBUG of '#define DEBUG' followed directly by the line end, so the token has the witness's span.",
		Grounds: []G1Ground{
			{G1DocGrammar, "preproc_define", "preproc_define: $ => seq( preprocessor('define'), $.preproc_arg, /\\n/, ),"},
			{G1DocGrammar, "preproc_undef", "preproc_undef: $ => seq( preprocessor('undef'), $.preproc_arg, /\\n/, ),"},
			{G1DocGrammar, "preproc_arg", "preproc_arg: _ => token(prec(-1, /\\S([^/\\n]|\\/[^*]|\\\\\\r?\\n)*/)),"},
			{G1DocSpec, g1gLex + "6.5.4 PP_Declaration", "'define' PP_Whitespace PP_Conditional_Symbol"},
			{G1DocSpec, g1gLex + "6.5.2 PP_Conditional_Symbol", "fragment PP_Conditional_Symbol // Must not be equal to tokens TRUE or FALSE. See note below. : Basic_Identifier ;"},
		},
	},
	{
		Item: "csharp-B01.a383", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, PostNative: true,
		Reason: "The specification makes Interpolated_Regular_String_Start one lexeme of two characters ($\"). The pinned grammar text has no node of that extent: interpolated_string_expression begins with two nodes, an interpolation_start (an alias of the external token interpolation_regular_start) and then a quote node. The grammar text does not give the extent of the external token, so only the start is derivable: an interpolation_start node starts where the lexeme starts.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_regular_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;"},
		},
	},
	{
		Item: "csharp-B01.a396", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, PostNative: true,
		Reason: "The specification makes Interpolated_Verbatim_String_Start one lexeme ($@\" or @$\"). The pinned grammar text begins the verbatim form with an interpolation_start (an alias of the external token interpolation_verbatim_start) and then a quote node, so no node has the lexeme's extent, and only the start is derivable: an interpolation_start node starts where the lexeme starts.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;"},
		},
	},
	{
		Item: "csharp-B01.a397", Field: G1FieldWitnessMatch, Old: `"span"`, New: `"starts-at"`, PostNative: true,
		Reason: "Same lexeme class and grammar rule as csharp-B01.a396: the verbatim start @$\" is one lexeme in the specification and begins with an interpolation_start (alias of interpolation_verbatim_start) followed by a quote node in the pinned grammar text, so only the start is derivable.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;"},
		},
	},
}

// g1KindMapCorrections are the kind map entries of the start tokens of interpolated strings, and the one
// consequence for the list of unreferenced types. The map relates a Roslyn kind to the node types of
// the same source extent; for a start token of an interpolated string the pinned grammar text has no
// node of that extent. The first two entries are behind the three mismatching alternatives (found
// after the native result: post_native). The two raw entries have the same structure in the grammar
// text and no native mismatch showed it (post_native false): they are derived from the specification
// and the grammar text alone, on the Root decision to correct the same structural error everywhere.
var g1KindMapCorrections = []G1Correction{
	{
		Item: "InterpolatedStringStartToken", Field: G1FieldEntry, PostNative: true,
		Old:    `{"roslyn":"InterpolatedStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "The map relates a Roslyn kind to the node types of the same extent. The token $\" is one lexeme in the specification, but the pinned grammar text builds the start of an interpolated regular string from two nodes (interpolation_start, then a quote node), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_regular_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;"},
		},
	},
	{
		Item: "InterpolatedVerbatimStringStartToken", Field: G1FieldEntry, PostNative: true,
		Old:    `{"roslyn":"InterpolatedVerbatimStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedVerbatimStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "The tokens $@\" and @$\" are one lexeme in the specification, but the pinned grammar text builds the start of an interpolated verbatim string from two nodes (interpolation_start, then a quote node), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_verbatim_start, $.interpolation_start), alias($.interpolation_start_quote, '\"'),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Verbatim_String_Start", "Interpolated_Verbatim_String_Start : '$@\"' | '@$\"' ;"},
		},
	},
	{
		Item: "InterpolatedSingleLineRawStringStartToken", Field: G1FieldEntry, PostNative: false,
		Old:    `{"roslyn":"InterpolatedSingleLineRawStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedSingleLineRawStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "Same structural error as the regular and verbatim starts, derived from the specification and the grammar text alone (no native mismatch showed it): Interpolated_Raw_String_Start is one lexeme (the dollar signs and the raw string delimiter), but the pinned grammar text begins the raw form with an interpolation_start (an alias of the external token interpolation_raw_start) and then a quote node (interpolation_quote), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_raw_start, $.interpolation_start), alias($.interpolation_start_quote, $.interpolation_quote),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Raw_String_Start", "Interpolated_Raw_String_Start : Interpolated_Raw_String_Prefix Raw_String_Literal_Delimiter ;"},
		},
	},
	{
		Item: "InterpolatedMultiLineRawStringStartToken", Field: G1FieldEntry, PostNative: false,
		Old:    `{"roslyn":"InterpolatedMultiLineRawStringStartToken","class":"token","map":"1:1","ts":["interpolation_start"]}`,
		New:    `{"roslyn":"InterpolatedMultiLineRawStringStartToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`,
		Reason: "Same as InterpolatedSingleLineRawStringStartToken: the multi-line raw form uses the same Interpolated_Raw_String_Start lexeme and the same grammar rule (one interpolated_string_expression choice for both), so no node type has the extent of the token.",
		Grounds: []G1Ground{
			{G1DocGrammar, "interpolated_string_expression", "seq( alias($.interpolation_raw_start, $.interpolation_start), alias($.interpolation_start_quote, $.interpolation_quote),"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Raw_String_Start", "Interpolated_Raw_String_Start : Interpolated_Raw_String_Prefix Raw_String_Literal_Delimiter ;"},
		},
	},
	{
		Item: "interpolation_start", Field: G1FieldTSOnly, PostNative: true,
		Old:    `null`,
		New:    `{"type":"interpolation_start","reason":"EXTERNAL_SCANNER_NODE"}`,
		Reason: "A consequence of the four entries above: interpolation_start was the type of the four start kinds and no kind refers to it now, so it joins the named types without a Roslyn kind. The pinned grammar text declares the three tokens it aliases (interpolation_regular_start, interpolation_verbatim_start, interpolation_raw_start) as external scanner tokens, the reason r1 gives to interpolation_brace and the other aliases of external tokens.",
		Grounds: []G1Ground{
			{G1DocGrammar, "externals", "$.interpolation_regular_start, $.interpolation_verbatim_start, $.interpolation_raw_start,"},
			{G1DocSpec, g1gExpr + "12.8.3 Interpolated_Regular_String_Start", "Interpolated_Regular_String_Start : '$\"' ;"},
		},
	},
}

// g1CheckCorrections checks the form of a list of corrections: at least one, each with the allowed
// field, a different old and new value, a reason, a ground in the grammar text (the tree-sitter side
// of every correction comes from there) and a ground in the specification; no (item, field) twice. The
// post_native flag says whether the native result prompted the correction; its value is authored with
// the correction and checked by the comparison with the authored list.
func g1CheckCorrections(cs []G1Correction, field func(string) bool) error {
	if len(cs) == 0 {
		return g1cFault("G1_CORRECTION_FORM")
	}
	seen := map[string]bool{}
	for _, c := range cs {
		key := c.Item + "|" + c.Field
		if c.Item == "" || !field(c.Field) || seen[key] || c.Old == "" || c.New == "" || c.Old == c.New || c.Reason == "" {
			return g1cFault("G1_CORRECTION_FORM")
		}
		seen[key] = true
		grammar, spec := false, false
		for _, g := range c.Grounds {
			if g.At == "" || strings.TrimSpace(g.Quote) == "" || g.Doc != G1DocGrammar && g.Doc != G1DocSpec {
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

// G1CorrectionNorm collapses whitespace runs, the form a ground's quote is compared in.
func G1CorrectionNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// CheckG1Grounds checks that every ground of the corrections quotes its document: the grammar text for
// a grammar ground and the specification text of the named file (lexical-structure.md or
// expressions.md) for a spec ground. It reads the documents the caller supplies and nothing else.
func CheckG1Grounds(cs []G1Correction, grammar string, spec map[string]string) error {
	for _, c := range cs {
		for _, g := range c.Grounds {
			doc := grammar
			if g.Doc == G1DocSpec {
				doc = ""
				for name, text := range spec {
					if strings.Contains(g.At, "standard/"+name+" ") {
						doc = text
					}
				}
			}
			if !strings.Contains(G1CorrectionNorm(doc), G1CorrectionNorm(g.Quote)) {
				return fmt.Errorf("correction %s: the %s ground %q is not in the document", c.Item, g.Doc, g.At)
			}
		}
	}
	return nil
}

func g1cJSON(v any) string {
	d, _ := json.Marshal(v)
	return string(d)
}

// ---- expected asset -----------------------------------------------------------------------------

func g1cExpectedField(f string) bool { return f == G1FieldTS || f == G1FieldWitnessMatch }

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
