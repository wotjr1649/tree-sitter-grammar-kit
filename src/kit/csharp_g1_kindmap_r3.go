package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The kind map r3 (#233, G1-3b): the r2 map plus the contextual entries the class-B alternatives of csharp-B03 and csharp-B04 need.
//
// A Roslyn kind that the map sends to several tree-sitter types (1:n) leaves a B expectation wider than the construct: it would accept
// either type at a place where the grammar text allows one. The grammar-only type audit (csharp_g1_b34_audit.go) found one such kind in
// the B alternatives, SimpleAssignmentExpression: `with_initializer` exists only inside the `with` expression body, every other
// assignment is `assignment_expression`. A contextual entry splits the 1:n types by the Roslyn kind of the direct parent. The kinds and
// the rest of the map are r2's, byte for byte in content; r2 is pinned and stays as it is. r3 is derived from r2 and the entries below
// and holds nothing else, and nothing in it comes from a native result: the grounds are quotes of the pinned grammar.js.

const (
	CSharpG1KindMapSchemaR3 = "tsgk-csharp-g1-kind-map/r3"
	CSharpG1KindMapPathR3   = "src/contracts/csharp-g1-kind-map-r3.json"
)

// G1KMContextual types a node by its parent: TS when the Roslyn kind of the direct parent is one of Parent, Otherwise for every
// other parent. TS and Otherwise partition the types of the kind's r2 entry.
type G1KMContextual struct {
	Roslyn    string     `json:"roslyn"`
	Parent    []string   `json:"parent"`
	TS        []string   `json:"ts"`
	Otherwise []string   `json:"otherwise"`
	Reason    string     `json:"reason"`
	Grounds   []G1Ground `json:"ground"`
}

type CSharpG1KindMapR3 struct {
	Schema     string                `json:"schema"`
	Basis      string                `json:"basis"`
	Grammar    G1KMGrammar           `json:"grammar"`
	Spec       []string              `json:"spec"`
	Vocabulary G1KMVocabulary        `json:"vocabulary"`
	Counts     G1KMCounts            `json:"counts"`
	Previous   queryregistrycore.Pin `json:"previous"`
	Contextual []G1KMContextual      `json:"contextual"`
	Kinds      []G1KMKind            `json:"kinds"`
	TSOnly     []G1KMTSOnly          `json:"ts_only"`
}

// g1KindMapContextual is the authored list of the contextual entries.
var g1KindMapContextual = []G1KMContextual{{
	Roslyn:    "SimpleAssignmentExpression",
	Parent:    []string{"WithInitializerExpression"},
	TS:        []string{"with_initializer"},
	Otherwise: []string{"assignment_expression"},
	Reason:    "grammar.js builds with_initializer only in the body of a with expression; an assignment anywhere else is assignment_expression, and the object initializer holds assignment_expression nodes",
	Grounds: []G1Ground{
		{Doc: G1DocGrammar, At: "_with_body", Quote: "_with_body: $ => seq( '{', commaSep($.with_initializer), optional(','), '}', ),", DocSHA256: g1gGrammarSHA, Heading: "_with_body:", Gap: 0},
		{Doc: G1DocGrammar, At: "assignment_expression", Quote: "assignment_expression: $ => seq( field('left', $.lvalue_expression), field('operator', choice( '=',", DocSHA256: g1gGrammarSHA, Heading: "assignment_expression:", Gap: 0},
	},
}}

const csharpG1KindMapR3Basis = "The kind map r2 (pinned in previous) with contextual entries for the 1:n node kinds that the class-B alternatives of csharp-B03 and csharp-B04 use. An entry gives the types of a kind by the Roslyn kind of its direct parent. Authored from the pinned grammar.js alone (grounds); no native output."

func g1km3Fault(code string) error { return fail(KindInvalidInput, code, CSharpG1KindMapPathR3, nil) }

// DeriveCSharpG1KindMapR3 builds r3 from the bytes of r2.
func DeriveCSharpG1KindMapR3(r2 []byte) (CSharpG1KindMapR3, error) {
	var out CSharpG1KindMapR3
	m, err := ParseCSharpG1KindMap(r2, nil)
	if err != nil {
		return out, err
	}
	if m.Schema != CSharpG1KindMapSchemaR2 {
		return out, g1km3Fault("G1_KINDMAP3_PREVIOUS")
	}
	out = CSharpG1KindMapR3{Schema: CSharpG1KindMapSchemaR3, Basis: csharpG1KindMapR3Basis, Grammar: m.Grammar, Spec: m.Spec, Vocabulary: m.Vocabulary, Counts: m.Counts,
		Previous: queryregistrycore.FilePin(CSharpG1KindMapPathR2, r2), Contextual: slices.Clone(g1KindMapContextual), Kinds: m.Kinds, TSOnly: m.TSOnly}
	return out, g1km3Check(out)
}

// g1km3Check checks the contextual entries against the kinds of the map: each names a node kind with a 1:n entry, its two type
// lists partition that entry, its parent kinds are kinds of the map, and its grounds are well formed.
func g1km3Check(x CSharpG1KindMapR3) error {
	idx := map[string]G1KMKind{}
	for _, k := range x.Kinds {
		idx[k.Roslyn] = k
	}
	for i, c := range x.Contextual {
		k, ok := idx[c.Roslyn]
		if !ok || k.Class != "node" || k.Map != G1MapOneToN || len(c.Parent) == 0 || len(c.TS) == 0 || len(c.Otherwise) == 0 || len(c.Grounds) == 0 || c.Reason == "" {
			return g1km3Fault("G1_KINDMAP3_ENTRY")
		}
		if i > 0 && x.Contextual[i-1].Roslyn >= c.Roslyn {
			return g1km3Fault("G1_KINDMAP3_ENTRY")
		}
		for _, p := range c.Parent {
			if pk, ok := idx[p]; !ok || pk.Class != "node" {
				return g1km3Fault("G1_KINDMAP3_PARENT")
			}
		}
		all := slices.Concat(c.TS, c.Otherwise)
		slices.Sort(all)
		if !slices.Equal(all, k.TS) || !slices.IsSorted(c.TS) || !slices.IsSorted(c.Otherwise) {
			return g1km3Fault("G1_KINDMAP3_PARTITION")
		}
		for _, g := range c.Grounds {
			if g.Doc != G1DocGrammar || !g1CheckGround(g) {
				return g1km3Fault("G1_KINDMAP3_GROUND")
			}
		}
	}
	return nil
}

// MarshalCSharpG1KindMapR3 writes the canonical asset: one line per entry and per kind.
func MarshalCSharpG1KindMapR3(x CSharpG1KindMapR3) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	b.WriteString("{\n")
	for _, f := range []struct {
		name string
		v    any
	}{{"schema", x.Schema}, {"basis", x.Basis}, {"grammar", x.Grammar}, {"spec", x.Spec}, {"vocabulary", x.Vocabulary}, {"counts", x.Counts}, {"previous", x.Previous}} {
		fmt.Fprintf(&b, " %q: ", f.name)
		put(f.v)
		b.WriteString(",\n")
	}
	list := func(name string, n int, at func(int) any, last bool) {
		fmt.Fprintf(&b, " %q: [\n", name)
		for i := 0; i < n; i++ {
			b.WriteString("  ")
			put(at(i))
			if i < n-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" ]")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	list("contextual", len(x.Contextual), func(i int) any { return x.Contextual[i] }, false)
	list("kinds", len(x.Kinds), func(i int) any { return x.Kinds[i] }, false)
	list("ts_only", len(x.TSOnly), func(i int) any { return x.TSOnly[i] }, true)
	b.WriteString("}\n")
	return b.Bytes(), werr
}

// ParseCSharpG1KindMapR3 decodes r3 and requires it to be the derivation from r2 (the bytes the previous pin names).
func ParseCSharpG1KindMapR3(data, r2 []byte) (CSharpG1KindMapR3, error) {
	var x CSharpG1KindMapR3
	if len(data) > csharpG1KindMapMax {
		return x, fail(KindResourceLimit, "G1_KINDMAP3_SIZE", CSharpG1KindMapPathR3, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_KINDMAP3_JSON", CSharpG1KindMapPathR3, err)
	}
	if x.Schema != CSharpG1KindMapSchemaR3 {
		return x, g1km3Fault("G1_KINDMAP3_SCHEMA")
	}
	want, err := DeriveCSharpG1KindMapR3(r2)
	if err != nil {
		return x, err
	}
	if x.Previous != want.Previous {
		return x, g1km3Fault("G1_KINDMAP3_PREVIOUS")
	}
	canonical, err := MarshalCSharpG1KindMapR3(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return x, g1km3Fault("G1_KINDMAP3_BYTES")
	}
	return want, nil
}

// Base is r3 as the plain map (the r2 kinds); the contextual entries are read through TypesOf.
func (x CSharpG1KindMapR3) Base() CSharpG1KindMap {
	return CSharpG1KindMap{Schema: CSharpG1KindMapSchema, Basis: x.Basis, Grammar: x.Grammar, Spec: x.Spec, Vocabulary: x.Vocabulary, Counts: x.Counts, Kinds: x.Kinds, TSOnly: x.TSOnly}
}

// TypesOf returns the tree-sitter types of a node of the Roslyn kind whose direct parent has the Roslyn kind parent ("" at the root).
func (x CSharpG1KindMapR3) TypesOf(kind, parent string) []string {
	for _, c := range x.Contextual {
		if c.Roslyn == kind {
			if slices.Contains(c.Parent, parent) {
				return slices.Clone(c.TS)
			}
			return slices.Clone(c.Otherwise)
		}
	}
	for _, k := range x.Kinds {
		if k.Roslyn == kind {
			return slices.Clone(k.TS)
		}
	}
	return nil
}

// g1ParseKindMapAny reads a kind map of any revision as the plain map; an r3 map is also checked for its own consistency and for covering the
// observed kinds (the pin to r2 is checked by ParseCSharpG1KindMapR3). The second result is the r3 map, or nil.
func g1ParseKindMapAny(data []byte, observed []string) (CSharpG1KindMap, *CSharpG1KindMapR3, error) {
	var head struct {
		Schema string `json:"schema"`
	}
	if jsonv2.Unmarshal(data, &head) == nil && head.Schema == CSharpG1KindMapSchemaR3 {
		var x CSharpG1KindMapR3
		if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
			return CSharpG1KindMap{}, nil, fail(KindInvalidInput, "G1_KINDMAP3_JSON", CSharpG1KindMapPathR3, err)
		}
		if err := g1km3Check(x); err != nil {
			return CSharpG1KindMap{}, nil, err
		}
		names := make([]string, len(x.Kinds))
		for i, k := range x.Kinds {
			names[i] = k.Roslyn
		}
		if observed != nil && !slices.Equal(names, observed) {
			return CSharpG1KindMap{}, nil, g1kmFault("G1_KINDMAP_COVERAGE")
		}
		return x.Base(), &x, nil
	}
	m, err := ParseCSharpG1KindMap(data, observed)
	return m, nil, err
}

// CheckG1KindMapR3Grounds checks the grounds of the contextual entries against the pinned grammar.js.
func CheckG1KindMapR3Grounds(x CSharpG1KindMapR3, grammar string) error {
	for _, c := range x.Contextual {
		if err := CheckG1GroundList(c.Roslyn, c.Grounds, grammar, nil); err != nil {
			return fmt.Errorf("kind map r3 entry %w", err)
		}
	}
	return nil
}
