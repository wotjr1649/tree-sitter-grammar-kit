package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Roslyn SyntaxKind -> tree-sitter-c-sharp node type map (#233, G1-3a).
//
// The map is shared by every G1 expectation stage. Its tree-sitter side comes from the pinned
// grammar text only (grammar.js and src/node-types.json of the pinned tree-sitter-c-sharp commit,
// plus the node names the repository's C2 patch adds); its Roslyn side is the kind vocabulary of the
// pinned Roslyn observation. It reads no native output. The asset is authored data: this verifier
// checks its structure and its consistency with the pinned vocabularies, not its judgments.

const (
	CSharpG1KindMapSchema = "tsgk-csharp-g1-kind-map/r1"
	CSharpG1KindMapPath   = "src/contracts/csharp-g1-kind-map-r1.json"
	csharpG1KindMapMax    = 1 << 20

	G1MapOneToOne = "1:1"
	G1MapOneToN   = "1:n"
	G1MapNone     = "none"
)

// Reasons a Roslyn kind has no tree-sitter node type.
var g1KindNoneReasons = []string{"HIDDEN_NODE", "NOT_IN_PINNED_GRAMMAR", "NO_SEPARATE_NODE", "ROSLYN_ONLY"}

// Reasons a tree-sitter named node type has no Roslyn kind.
var g1KindTSOnlyReasons = []string{"EXTERNAL_SCANNER_NODE", "PATCH_ONLY_CONSTRUCT", "ROSLYN_KIND_NOT_OBSERVED", "SUPERTYPE", "TRIVIA_OR_DIRECTIVE", "TS_WRAPPER"}

var (
	g1kmSHA  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	g1kmName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
)

type G1KMFile struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type G1KMAddition struct {
	Type   string `json:"type"`
	Named  bool   `json:"named"`
	Origin string `json:"origin"`
}

type G1KMGrammar struct {
	Repository string         `json:"repository"`
	Commit     string         `json:"commit"`
	GrammarJS  G1KMFile       `json:"grammar_js"`
	NodeTypes  G1KMFile       `json:"node_types"`
	Additions  []G1KMAddition `json:"additions"`
	Limit      string         `json:"limit"`
}

type G1KMVocabulary struct {
	Named     []string `json:"named"`
	Anonymous []string `json:"anonymous"`
}

// G1KMKind maps one Roslyn kind. Text is the fixed lexeme of a keyword or punctuator token.
type G1KMKind struct {
	Roslyn string   `json:"roslyn"`
	Class  string   `json:"class"`
	Map    string   `json:"map"`
	TS     []string `json:"ts"`
	Text   string   `json:"text,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

type G1KMTSOnly struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type G1KMCounts struct {
	RoslynKinds   int `json:"roslyn_kinds"`
	Tokens        int `json:"tokens"`
	Nodes         int `json:"nodes"`
	OneToOne      int `json:"one_to_one"`
	OneToN        int `json:"one_to_n"`
	None          int `json:"none"`
	TSNamed       int `json:"ts_named"`
	TSAnonymous   int `json:"ts_anonymous"`
	TSReferenced  int `json:"ts_referenced"`
	TSOnly        int `json:"ts_named_only"`
	TSManyToOne   int `json:"ts_shared_by_many_kinds"`
	TSUnreachable int `json:"ts_anonymous_unreferenced"`
}

type CSharpG1KindMap struct {
	Schema     string         `json:"schema"`
	Basis      string         `json:"basis"`
	Grammar    G1KMGrammar    `json:"grammar"`
	Spec       []string       `json:"spec"`
	Vocabulary G1KMVocabulary `json:"vocabulary"`
	Counts     G1KMCounts     `json:"counts"`
	Kinds      []G1KMKind     `json:"kinds"`
	TSOnly     []G1KMTSOnly   `json:"ts_only"`
	// Previous pins the sealed r1 map a r2 map corrects; Corrections lists the post-native changes. Both are empty in r1.
	Previous    queryregistrycore.Pin `json:"previous,omitzero"`
	Corrections []G1Correction        `json:"corrections,omitempty"`
}

func g1kmFault(code string) error { return fail(KindInvalidInput, code, CSharpG1KindMapPath, nil) }

// G1KindMapIsToken reports whether a Roslyn kind name denotes a token (Roslyn's own naming).
func G1KindMapIsToken(roslyn string) bool {
	return strings.HasSuffix(roslyn, "Keyword") || strings.HasSuffix(roslyn, "Token")
}

// DeriveCSharpG1KindMapCounts computes the statistics block of a map from its entries.
func DeriveCSharpG1KindMapCounts(m CSharpG1KindMap) G1KMCounts {
	c := G1KMCounts{RoslynKinds: len(m.Kinds), TSNamed: len(m.Vocabulary.Named), TSAnonymous: len(m.Vocabulary.Anonymous), TSOnly: len(m.TSOnly)}
	use := map[string]int{}
	for _, k := range m.Kinds {
		if k.Class == "token" {
			c.Tokens++
		} else {
			c.Nodes++
		}
		switch k.Map {
		case G1MapOneToOne:
			c.OneToOne++
		case G1MapOneToN:
			c.OneToN++
		default:
			c.None++
		}
		for _, t := range k.TS {
			use[t]++
		}
	}
	c.TSReferenced = len(use)
	for _, n := range use {
		if n > 1 {
			c.TSManyToOne++
		}
	}
	for _, a := range m.Vocabulary.Anonymous {
		if use[G1KindMapAnon(a)] == 0 {
			c.TSUnreachable++
		}
	}
	return c
}

// MarshalCSharpG1KindMap writes the canonical asset: one line per kind and per ts-only entry.
func MarshalCSharpG1KindMap(m CSharpG1KindMap) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	scalar := func(name string, v any) {
		fmt.Fprintf(&b, " %q: ", name)
		put(v)
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
	b.WriteString("{\n")
	scalar("schema", m.Schema)
	scalar("basis", m.Basis)
	scalar("grammar", m.Grammar)
	scalar("spec", m.Spec)
	scalar("vocabulary", m.Vocabulary)
	scalar("counts", m.Counts)
	if m.Schema == CSharpG1KindMapSchemaR2 {
		scalar("previous", m.Previous)
		list("corrections", len(m.Corrections), func(i int) any { return m.Corrections[i] }, false)
	}
	list("kinds", len(m.Kinds), func(i int) any { return m.Kinds[i] }, false)
	list("ts_only", len(m.TSOnly), func(i int) any { return m.TSOnly[i] }, true)
	b.WriteString("}\n")
	return b.Bytes(), werr
}

// ParseCSharpG1KindMap decodes the asset and checks it. observed lists the Roslyn kind names the
// map must cover exactly (the pinned observation's vocabulary); nil skips only that comparison.
func ParseCSharpG1KindMap(data []byte, observed []string) (CSharpG1KindMap, error) {
	var m CSharpG1KindMap
	if len(data) > csharpG1KindMapMax {
		return m, fail(KindResourceLimit, "G1_KINDMAP_SIZE", CSharpG1KindMapPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &m, jsonv2.RejectUnknownMembers(true)); err != nil {
		return m, fail(KindInvalidInput, "G1_KINDMAP_JSON", CSharpG1KindMapPath, err)
	}
	r2 := m.Schema == CSharpG1KindMapSchemaR2
	if m.Schema != CSharpG1KindMapSchema && !r2 || m.Basis == "" || len(m.Spec) == 0 || r2 != (len(m.Corrections) > 0) || !r2 && m.Previous != (queryregistrycore.Pin{}) {
		return m, g1kmFault("G1_KINDMAP_SCHEMA")
	}
	g := m.Grammar
	for _, f := range []G1KMFile{g.GrammarJS, g.NodeTypes} {
		if f.Path == "" || f.Bytes == 0 || !g1kmSHA.MatchString(f.SHA256) {
			return m, g1kmFault("G1_KINDMAP_GRAMMAR")
		}
	}
	if g.Repository == "" || len(g.Commit) != 40 || g.Limit == "" {
		return m, g1kmFault("G1_KINDMAP_GRAMMAR")
	}
	for _, list := range [][]string{m.Vocabulary.Named, m.Vocabulary.Anonymous} {
		if !slices.IsSorted(list) {
			return m, g1kmFault("G1_KINDMAP_VOCABULARY")
		}
		for j, v := range list {
			if v == "" || j > 0 && list[j-1] == v {
				return m, g1kmFault("G1_KINDMAP_VOCABULARY")
			}
		}
	}
	for _, a := range g.Additions {
		if a.Origin == "" || !G1KindMapHasType(m.Vocabulary, a.Type, a.Named) {
			return m, g1kmFault("G1_KINDMAP_ADDITION")
		}
	}
	use := map[string]bool{}
	for i, k := range m.Kinds {
		if !g1kmName.MatchString(k.Roslyn) || i > 0 && m.Kinds[i-1].Roslyn >= k.Roslyn {
			return m, g1kmFault("G1_KINDMAP_KINDS")
		}
		if want := map[bool]string{true: "token", false: "node"}[G1KindMapIsToken(k.Roslyn)]; k.Class != want {
			return m, g1kmFault("G1_KINDMAP_CLASS")
		}
		if k.Text != "" && k.Class != "token" {
			return m, g1kmFault("G1_KINDMAP_CLASS")
		}
		if !slices.IsSorted(k.TS) {
			return m, g1kmFault("G1_KINDMAP_ENTRY")
		}
		for j, t := range k.TS {
			if !G1KindMapHasEntry(m.Vocabulary, t) || j > 0 && k.TS[j-1] == t {
				return m, g1kmFault("G1_KINDMAP_ENTRY")
			}
			use[t] = true
		}
		switch {
		case len(k.TS) == 0:
			if k.Map != G1MapNone || !slices.Contains(g1KindNoneReasons, k.Reason) {
				return m, g1kmFault("G1_KINDMAP_ENTRY")
			}
		case len(k.TS) == 1:
			if k.Map != G1MapOneToOne || k.Reason != "" {
				return m, g1kmFault("G1_KINDMAP_ENTRY")
			}
		default:
			if k.Map != G1MapOneToN || k.Reason != "" {
				return m, g1kmFault("G1_KINDMAP_ENTRY")
			}
		}
	}
	if observed != nil {
		names := make([]string, len(m.Kinds))
		for i, k := range m.Kinds {
			names[i] = k.Roslyn
		}
		if !slices.Equal(names, observed) {
			return m, g1kmFault("G1_KINDMAP_COVERAGE")
		}
	}
	// the named types no kind refers to are listed with a reason, exactly
	var only []string
	for _, n := range m.Vocabulary.Named {
		if !use[n] {
			only = append(only, n)
		}
	}
	if len(only) != len(m.TSOnly) {
		return m, g1kmFault("G1_KINDMAP_TS_ONLY")
	}
	for i, o := range m.TSOnly {
		if o.Type != only[i] || !slices.Contains(g1KindTSOnlyReasons, o.Reason) {
			return m, g1kmFault("G1_KINDMAP_TS_ONLY")
		}
	}
	if m.Counts != DeriveCSharpG1KindMapCounts(m) {
		return m, g1kmFault("G1_KINDMAP_COUNTS")
	}
	canonical, err := MarshalCSharpG1KindMap(m)
	if err != nil || !bytes.Equal(canonical, data) {
		return m, g1kmFault("G1_KINDMAP_BYTES")
	}
	return m, nil
}

// G1KindMapIndex indexes a verified map by Roslyn kind.
func (m CSharpG1KindMap) Index() map[string]G1KMKind {
	out := make(map[string]G1KMKind, len(m.Kinds))
	for _, k := range m.Kinds {
		out[k.Roslyn] = k
	}
	return out
}

// G1KindMapAnon spells an anonymous node type as a map entry: the quoted lexeme, as tree-sitter
// queries write it. A bare word is a named node type, so `type` (a named supertype) and "type" (the
// attribute-target keyword) stay apart.
func G1KindMapAnon(lexeme string) string { return `"` + lexeme + `"` }

// G1KindMapHasType reports whether the vocabulary holds the type as a named or anonymous node.
func G1KindMapHasType(v G1KMVocabulary, typ string, named bool) bool {
	list := v.Anonymous
	if named {
		list = v.Named
	}
	_, ok := slices.BinarySearch(list, typ)
	return ok
}

// G1KindMapHasEntry reports whether a map entry (bare named type or quoted anonymous type) is in the vocabulary.
func G1KindMapHasEntry(v G1KMVocabulary, entry string) bool {
	if len(entry) >= 3 && entry[0] == '"' && entry[len(entry)-1] == '"' {
		return G1KindMapHasType(v, entry[1:len(entry)-1], false)
	}
	return G1KindMapHasType(v, entry, true)
}
