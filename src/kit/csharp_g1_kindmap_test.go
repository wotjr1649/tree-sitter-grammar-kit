package kit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCSharpG1KindMap(t *testing.T) {
	a := g1ExpLoad(t)
	m, err := ParseCSharpG1KindMap(a.kindmap, g1ObservedKinds(t, a))
	if err != nil {
		t.Fatal(err)
	}
	c := m.Counts
	if c.RoslynKinds != 461 || c.Tokens != 193 || c.Nodes != 268 || c.OneToOne != 365 || c.OneToN != 68 || c.None != 28 || c.OneToOne+c.OneToN+c.None != c.RoslynKinds {
		t.Fatalf("counts %+v", c)
	}
	// the tree-sitter vocabulary is the pinned grammar's, so the types the registered cases already
	// expect are in it (named types only: expectations never name anonymous tokens)
	l := g1Load(t)
	types := map[string]bool{}
	for _, cs := range l.want.Cases {
		for _, s := range cs.Steps {
			for _, ty := range s.Types {
				types[ty] = true
			}
		}
	}
	if len(types) != 203 {
		t.Fatalf("expected type vocabulary %d", len(types))
	}
	for ty := range types {
		if !G1KindMapHasType(m.Vocabulary, ty, true) {
			t.Errorf("registered expectation type %s is not in the pinned grammar vocabulary", ty)
		}
	}
	// every grammar addition is one of the four patch node types the cases use
	if len(m.Grammar.Additions) != 4 {
		t.Errorf("additions %v", m.Grammar.Additions)
	}
	// fixed-lexeme kinds agree with the observation: a token of such a kind has exactly that text
	_, ctx, _, err := g1ExpContext(a.in)
	if err != nil {
		t.Fatal(err)
	}
	idx := m.Index()
	for _, s := range ctx.order {
		for _, tk := range s.toks {
			// a raw interpolated string with $$ opens an interpolation with a repeated brace ("{{" is one OpenBraceToken)
			if k := idx[tk.kind]; k.Text != "" && tk.end > tk.start && tk.text != k.Text && tk.text != strings.Repeat(k.Text, len(tk.text)/len(k.Text)) {
				t.Fatalf("%s: token %s has text %q, the map says %q", s.id, tk.kind, tk.text, k.Text)
			}
		}
	}
	// 1:1 and 1:n and none are consistent with the entries; every none has a reason
	for _, k := range m.Kinds {
		if (len(k.TS) == 0) != (k.Map == G1MapNone) || (k.Map == G1MapNone) != (k.Reason != "") {
			t.Errorf("%s: %+v", k.Roslyn, k)
		}
	}
}

func g1ObservedKinds(t *testing.T, a *g1ExpAssets) []string {
	t.Helper()
	var obs struct {
		Kinds []string `json:"kinds"`
	}
	if err := json.Unmarshal(a.obs, &obs); err != nil {
		t.Fatal(err)
	}
	return obs.Kinds
}

func TestCSharpG1KindMapRejectsTampering(t *testing.T) {
	a := g1ExpLoad(t)
	observed := g1ObservedKinds(t, a)
	base, err := ParseCSharpG1KindMap(a.kindmap, observed)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() CSharpG1KindMap {
		var m CSharpG1KindMap
		b, _ := json.Marshal(base)
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	find := func(m *CSharpG1KindMap, kind string) *G1KMKind {
		for i := range m.Kinds {
			if m.Kinds[i].Roslyn == kind {
				return &m.Kinds[i]
			}
		}
		t.Fatal("no kind " + kind)
		return nil
	}
	for _, f := range []struct {
		name, code string
		do         func(m *CSharpG1KindMap)
	}{
		{"schema", "G1_KINDMAP_SCHEMA", func(m *CSharpG1KindMap) { m.Schema = "x" }},
		{"grammar pin", "G1_KINDMAP_GRAMMAR", func(m *CSharpG1KindMap) { m.Grammar.NodeTypes.SHA256 = "x" }},
		{"limit statement dropped", "G1_KINDMAP_GRAMMAR", func(m *CSharpG1KindMap) { m.Grammar.Limit = "" }},
		{"vocabulary unsorted", "G1_KINDMAP_VOCABULARY", func(m *CSharpG1KindMap) {
			m.Vocabulary.Named[0], m.Vocabulary.Named[1] = m.Vocabulary.Named[1], m.Vocabulary.Named[0]
		}},
		{"addition outside the vocabulary", "G1_KINDMAP_ADDITION", func(m *CSharpG1KindMap) { m.Grammar.Additions[0].Type = "nonexistent_node" }},
		{"kind dropped", "G1_KINDMAP_COVERAGE", func(m *CSharpG1KindMap) { m.Kinds = m.Kinds[1:] }},
		{"kind unsorted", "G1_KINDMAP_KINDS", func(m *CSharpG1KindMap) { m.Kinds[0], m.Kinds[1] = m.Kinds[1], m.Kinds[0] }},
		{"class flipped", "G1_KINDMAP_CLASS", func(m *CSharpG1KindMap) { find(m, "AbstractKeyword").Class = "node" }},
		{"class flipped without a lexeme", "G1_KINDMAP_CLASS", func(m *CSharpG1KindMap) { find(m, "IdentifierToken").Class = "node" }},
		{"text on a node", "G1_KINDMAP_CLASS", func(m *CSharpG1KindMap) { find(m, "Block").Text = "{" }},
		{"unknown type", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { find(m, "Block").TS = []string{"nonexistent_node"} }},
		{"anonymous written bare", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { find(m, "AbstractKeyword").TS = []string{"abstract"} }},
		{"label 1:1 for two types", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { find(m, "NumericLiteralToken").Map = G1MapOneToOne }},
		{"none without reason", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { find(m, "EndOfFileToken").Reason = "" }},
		{"none with a type", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { find(m, "EndOfFileToken").TS = []string{"block"} }},
		{"type unsorted", "G1_KINDMAP_ENTRY", func(m *CSharpG1KindMap) { k := find(m, "NumericLiteralToken"); k.TS[0], k.TS[1] = k.TS[1], k.TS[0] }},
		{"ts-only dropped", "G1_KINDMAP_TS_ONLY", func(m *CSharpG1KindMap) { m.TSOnly = m.TSOnly[1:] }},
		{"last ts-only dropped", "G1_KINDMAP_TS_ONLY", func(m *CSharpG1KindMap) { m.TSOnly = m.TSOnly[:len(m.TSOnly)-1] }},
		{"ts-only reason", "G1_KINDMAP_TS_ONLY", func(m *CSharpG1KindMap) { m.TSOnly[0].Reason = "x" }},
		{"counts", "G1_KINDMAP_COUNTS", func(m *CSharpG1KindMap) { m.Counts.OneToOne++ }},
	} {
		m := clone()
		f.do(&m)
		data, err := MarshalCSharpG1KindMap(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1KindMap(data, observed); g1Code(err) != f.code {
			t.Errorf("%s: %v, want %s", f.name, err, f.code)
		}
	}
	text := string(a.kindmap)
	for name, f := range map[string]struct {
		data []byte
		code string
	}{
		"truncated":      {a.kindmap[:len(a.kindmap)/2], "G1_KINDMAP_JSON"},
		"unknown member": {[]byte(strings.Replace(text, `"basis":`, `"extra": 1,`+"\n"+` "basis":`, 1)), "G1_KINDMAP_JSON"},
		"oversize":       {append(bytes.Clone(a.kindmap), bytes.Repeat([]byte(" "), csharpG1KindMapMax)...), "G1_KINDMAP_SIZE"},
		"non-canonical":  {bytes.Replace(a.kindmap, []byte("\n \"basis\""), []byte("\n  \"basis\""), 1), "G1_KINDMAP_BYTES"},
	} {
		if _, err := ParseCSharpG1KindMap(f.data, observed); g1Code(err) != f.code {
			t.Errorf("%s: %v, want %s", name, err, f.code)
		}
	}
	// a kind the observation lacks is not accepted either
	if _, err := ParseCSharpG1KindMap(a.kindmap, append(append([]string{}, observed...), "ZzzKind")); g1Code(err) != "G1_KINDMAP_COVERAGE" {
		t.Errorf("coverage: %v", err)
	}
}
