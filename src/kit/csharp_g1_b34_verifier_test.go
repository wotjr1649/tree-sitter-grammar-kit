package kit

import (
	"slices"
	"testing"
)

// Checks of the audit verifiers and the weak rule that a mutation run over the PR2 commit showed no test holds (review of PR2, finding 4): the
// order and duplicates of the instances, the range of an instance, the grounds of a node row, the order of the anchor rows, the order and
// duplicates of the types, the reason of a row, the verbatim string of the lexer, the order of the contextual entries of the kind map r3 and
// the rule "five or more types" of the weak match. Each case is an edit that only that check can reject.

func TestCSharpG1AuditB34InstancesAreOrderedAndInTheText(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34AuditView(a.expected, a.texts)
	if err != nil {
		t.Fatal(err)
	}
	x0, err := DecodeCSharpG1AuditB34(a.pos)
	if err != nil {
		t.Fatal(err)
	}
	row := slices.IndexFunc(x0.Rows, func(r G1B34PosRow) bool { return len(r.Instances) >= 2 })
	if row < 0 {
		t.Fatal("no row has two instances")
	}
	last := len(x0.Rows[row].Instances) - 1
	textLen := len(a.texts[x0.Rows[row].Source])
	for name, edit := range map[string]func([][2]int) [][2]int{
		"two instances swapped":      func(in [][2]int) [][2]int { in[0], in[1] = in[1], in[0]; return in },
		"an instance twice":          func(in [][2]int) [][2]int { in[1] = in[0]; return in },
		"an instance past the text":  func(in [][2]int) [][2]int { in[last] = [2]int{in[last][0], textLen + 1}; return in },
		"an instance after the text": func(in [][2]int) [][2]int { in[last] = [2]int{textLen, textLen + 4}; return in },
		"an empty instance":          func(in [][2]int) [][2]int { in[last] = [2]int{in[last][0], in[last][0]}; return in },
	} {
		x := x0
		x.Rows = slices.Clone(x0.Rows)
		x.Rows[row].Instances = edit(slices.Clone(x0.Rows[row].Instances))
		data, err := MarshalCSharpG1AuditB34(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1AuditB34(data, view); g1Code(err) != "G1_AUDITB34_INSTANCE" {
			t.Errorf("%s: %v, want G1_AUDITB34_INSTANCE", name, err)
		}
	}
}

// The row that names the terminals the grammar adds (node) has its own grounds, pinned and of the grammar text.
func TestCSharpG1AuditB34NodeRowsAreGroundedInTheGrammar(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34AuditView(a.expected, a.texts)
	if err != nil {
		t.Fatal(err)
	}
	x0, err := DecodeCSharpG1AuditB34(a.pos)
	if err != nil {
		t.Fatal(err)
	}
	row := slices.IndexFunc(x0.Rows, func(r G1B34PosRow) bool { return r.Node != nil })
	if row < 0 {
		t.Fatal("no row has a node")
	}
	for name, c := range map[string]struct {
		edit func(*G1Audit76Node)
		code string
	}{
		"a node ground quote edited":         {func(n *G1Audit76Node) { g := slices.Clone(n.Grounds); g[0].Quote += " x"; n.Grounds = g }, "G1_AUDITB34_GROUND"},
		"a node ground of the specification": {func(n *G1Audit76Node) { g := slices.Clone(n.Grounds); g[0].Doc = G1DocSpec; n.Grounds = g }, "G1_AUDITB34_ROW"},
		"a node ground removed":              {func(n *G1Audit76Node) { n.Grounds = nil }, "G1_AUDITB34_ROW"},
	} {
		x := x0
		x.Rows = slices.Clone(x0.Rows)
		n := *x0.Rows[row].Node
		c.edit(&n)
		x.Rows[row].Node = &n
		data, err := MarshalCSharpG1AuditB34(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1AuditB34(data, view); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
	// the node dropped: the pin of the node remains and no row can have lost it
	x := x0
	x.Rows = slices.Clone(x0.Rows)
	x.Rows[row].Node = nil
	data, err := MarshalCSharpG1AuditB34(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1AuditB34(data, view); g1Code(err) != "G1_AUDITB34_GROUND" {
		t.Errorf("a node dropped: %v", err)
	}
	// and a node on a row that has none
	other := slices.IndexFunc(x0.Rows, func(r G1B34PosRow) bool { return r.Node == nil })
	y := x0
	y.Rows = slices.Clone(x0.Rows)
	n := *x0.Rows[row].Node
	y.Rows[other].Node = &n
	data, err = MarshalCSharpG1AuditB34(y)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1AuditB34(data, view); g1Code(err) != "G1_AUDITB34_GROUND" {
		t.Errorf("a node on another row: %v", err)
	}
}

func TestCSharpG1TypeAuditB34RowsAreOrderedAndTheirTypesWellFormed(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34TypeView(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	x0, err := DecodeCSharpG1TypeAuditB34(a.typ)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() CSharpG1TypeAuditB34 {
		x := x0
		x.Rows, x.Anchors = slices.Clone(x0.Rows), slices.Clone(x0.Anchors)
		return x
	}
	nd := slices.IndexFunc(x0.Anchors, func(r G1B34AnchorTypeRow) bool { return r.Class == G1TypeNotDerivable && len(r.Types) == 2 })
	if nd < 0 {
		t.Fatal("no candidate set of two types")
	}
	types := x0.Anchors[nd].Types
	cases := []struct {
		name string
		edit func(*CSharpG1TypeAuditB34)
	}{
		{"two anchor rows swapped", func(x *CSharpG1TypeAuditB34) { x.Anchors[0], x.Anchors[1] = x.Anchors[1], x.Anchors[0] }},
		{"two rows swapped", func(x *CSharpG1TypeAuditB34) { x.Rows[0], x.Rows[1] = x.Rows[1], x.Rows[0] }},
		{"the types of a candidate set out of order", func(x *CSharpG1TypeAuditB34) { x.Anchors[nd].Types = []string{types[1], types[0]} }},
		{"a type of a candidate set twice", func(x *CSharpG1TypeAuditB34) { x.Anchors[nd].Types = []string{types[0], types[0], types[1]} }},
		{"the reason of a row gone", func(x *CSharpG1TypeAuditB34) { x.Rows[0].Reason = "" }},
		{"the reason of an anchor row gone", func(x *CSharpG1TypeAuditB34) { x.Anchors[0].Reason = "" }},
	}
	want := []string{"G1_TYPEAUDITB34_ROWS", "G1_TYPEAUDITB34_ROWS", "G1_TYPEAUDITB34_ROW", "G1_TYPEAUDITB34_ROW", "G1_TYPEAUDITB34_ROW", "G1_TYPEAUDITB34_ROW"}
	for i, c := range cases {
		x := clone()
		c.edit(&x)
		data, err := MarshalCSharpG1TypeAuditB34(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1TypeAuditB34(data, view); g1Code(err) != want[i] {
			t.Errorf("%s: %v, want %s", c.name, err, want[i])
		}
	}
}

// The lexer reads a verbatim string to its closing quote: a doubled quote inside is part of the string and a backslash is a character of it (in a
// regular string it escapes the quote), so the text after `@"a""\"` is code. A lexer that ends the string at the first quote of the pair reads
// the rest as a regular string whose escaped quote swallows the `;` and the `y`.
func TestCSharpG1AuditB34LexerReadsVerbatimStrings(t *testing.T) {
	src := []byte("x = @\"a\"\"\\\" ; y\n")
	got := g1b34Lexemes(src)
	semi := [2]int{bytesIndex(src, ";"), bytesIndex(src, ";") + 1}
	yy := [2]int{bytesIndex(src, "y"), bytesIndex(src, "y") + 1}
	if got[semi] != ";" || got[yy] != "y" {
		t.Errorf("the lexemes after the verbatim string: %q %q", got[semi], got[yy])
	}
	for span, text := range got {
		if span[0] > bytesIndex(src, "@") && span[1] <= bytesIndex(src, ";") {
			t.Errorf("a lexeme %q %v inside the verbatim string", text, span)
		}
	}
}

func bytesIndex(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}

// A contextual entry of the kind map r3 is in order of its Roslyn kind and without duplicate. The map has one entry today, so the check is
// shown on entries made for other 1:n kinds.
func TestCSharpG1KindMapR3EntriesAreOrdered(t *testing.T) {
	a := g1b34Load(t)
	x, err := ParseCSharpG1KindMapR3(a.km3, a.km2)
	if err != nil {
		t.Fatal(err)
	}
	first := x.Contextual[0]
	var other *G1KMKind
	for i, k := range x.Kinds {
		if k.Class == "node" && k.Map == G1MapOneToN && len(k.TS) >= 2 && k.Roslyn < first.Roslyn {
			other = &x.Kinds[i]
			break
		}
	}
	if other == nil {
		t.Skip("no 1:n node kind sorts before the first entry")
	}
	second := first
	second.Roslyn, second.TS, second.Otherwise = other.Roslyn, []string{other.TS[0]}, slices.Clone(other.TS[1:])
	y := x
	y.Contextual = []G1KMContextual{second, first}
	if err := g1km3Check(y); err != nil {
		t.Fatalf("entries in order are rejected: %v", err)
	}
	y.Contextual = []G1KMContextual{first, second}
	if err := g1km3Check(y); g1Code(err) != "G1_KINDMAP3_ENTRY" {
		t.Errorf("entries out of order: %v", err)
	}
	y.Contextual = []G1KMContextual{first, first}
	if err := g1km3Check(y); g1Code(err) != "G1_KINDMAP3_ENTRY" {
		t.Errorf("an entry twice: %v", err)
	}
}

// A weak match is an anchor set or type set that holds identifier or five or more types (R-5, D-1): four types are not weak, five are.
func TestCSharpG1B34WeakRule(t *testing.T) {
	for _, c := range []struct {
		ts   []string
		weak bool
	}{
		{[]string{"a"}, false},
		{[]string{"a", "b", "c", "d"}, false},
		{[]string{"a", "b", "c", "d", "e"}, true},
		{[]string{"a", "b", "c", "d", "e", "f"}, true},
		{[]string{"identifier"}, true},
		{[]string{"a", "identifier"}, true},
		{nil, false},
	} {
		if g1b34Weak(c.ts) != c.weak {
			t.Errorf("%v: weak %t, want %t", c.ts, g1b34Weak(c.ts), c.weak)
		}
	}
}

// D-1 and optional literals (review of PR2, finding 6): an anchor is a literal the production needs in every use, outside parentheses and not
// followed by ? or *. A literal that an instance may lack (`'scoped'?`, `(',' x)*`, `('=' x)?`) is no anchor, so a production with only such
// literals has none and is group B, an alternative of a node. The four alternatives of the review are B by this one rule.
func TestCSharpG1B34OptionalLiteralsAreNotAnchors(t *testing.T) {
	for _, c := range []struct {
		rhs  string
		lits []string
		why  string
	}{
		{"'scoped'? local_variable_type identifier", nil, "NO_REQUIRED_LITERAL"},
		{"argument (',' argument)*", nil, "NO_REQUIRED_LITERAL"},
		{"identifier ('=' local_variable_initializer)?", nil, "NO_REQUIRED_LITERAL"},
		{"'scoped'? 'var' implicitly_typed_local_variable_declarator", []string{"var"}, ""},
		{"'const' type constant_declarators", []string{"const"}, ""},
		{"'a' x | 'b' y", nil, "TOP_LEVEL_ALTERNATION"},
	} {
		if lits, why := g1RequiredLiterals(c.rhs); !slices.Equal(lits, c.lits) || why != c.why {
			t.Errorf("%s: %v %q, want %v %q", c.rhs, lits, why, c.lits, c.why)
		}
	}
	a := g1b34Load(t)
	led := g1Load(t).want
	for _, id := range []string{"csharp-B03.a013", "csharp-B03.a182", "csharp-B04.a031", "csharp-B04.a033"} {
		alt := g1b34Alt(a.want, id)
		var literals []string
		for _, l := range led.Alternatives {
			if l.ID == id {
				literals = l.Presence.Literals
			}
		}
		if alt.Group != G1ClassStructure || alt.Basis != G1BasisDiff || len(alt.Anchors) != 0 || len(literals) != 0 {
			t.Errorf("%s: group %s, basis %s, %d anchors, required literals %v", id, alt.Group, alt.Basis, len(alt.Anchors), literals)
		}
	}
}
