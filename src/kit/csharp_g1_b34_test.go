package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	jsonv2 "encoding/json/v2"
)

// The G1-3b expected values of csharp-B03 and csharp-B04, the two audits made before the seal and the kind map r3 (#233). The inputs are
// derived once per process (one ledger derivation and one observation index); the mutants compare altered assets with that derivation
// and never derive again.

type g1b34Assets struct {
	in       CSharpG1ExpectedSources
	want     CSharpG1B34
	texts    map[string][]byte
	expected []byte
	pos      []byte
	typ      []byte
	km3, km2 []byte
	err      error
}

var g1b34Once sync.Once
var g1b34Loaded g1b34Assets

func g1b34Load(t *testing.T) *g1b34Assets {
	t.Helper()
	g1b34Once.Do(func() {
		a := &g1b34Loaded
		read := func(name string) []byte {
			d, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return d
		}
		src := g1Load(t).src
		src.Expected, src.Executed = nil, nil
		a.km2, a.km3 = read("../contracts/csharp-g1-kind-map-r2.json"), read("../contracts/csharp-g1-kind-map-r3.json")
		a.in = CSharpG1ExpectedSources{Src: src, Observation: read("../contracts/csharp-g1-roslyn-observed-r3.json"), KindMap: a.km3}
		a.expected, a.pos, a.typ = read("../contracts/csharp-g1-expected-b03b04-r1.json"), read("../contracts/csharp-g1-audit-b03b04-r1.json"), read("../contracts/csharp-g1-typeaudit-b03b04-r1.json")
		if a.err == nil {
			a.want, a.texts, a.err = deriveCSharpG1B34(a.in, g1b34Picks, g1b34Overrides)
		}
	})
	if g1b34Loaded.err != nil {
		t.Fatal(g1b34Loaded.err)
	}
	return &g1b34Loaded
}

var g1b34UnfixedOnce sync.Once
var g1b34Unfixed CSharpG1B34
var g1b34UnfixedErr error

// g1b34Without is the derivation without the corrections of the audits (derived once).
func g1b34Without(t *testing.T) CSharpG1B34 {
	t.Helper()
	a := g1b34Load(t)
	g1b34UnfixedOnce.Do(func() { g1b34Unfixed, _, g1b34UnfixedErr = deriveCSharpG1B34(a.in, nil, nil) })
	if g1b34UnfixedErr != nil {
		t.Fatal(g1b34UnfixedErr)
	}
	return g1b34Unfixed
}

func g1b34Alt(x CSharpG1B34, id string) G1B34Alt {
	for _, a := range x.Alternatives {
		if a.ID == id {
			return a
		}
	}
	return G1B34Alt{}
}

func g1b34Clone(x CSharpG1B34) CSharpG1B34 {
	y := x
	y.Alternatives = slices.Clone(x.Alternatives)
	for i := range y.Alternatives {
		a := &y.Alternatives[i]
		a.Witnesses = slices.Clone(a.Witnesses)
		for j := range a.Witnesses {
			a.Witnesses[j].Span = slices.Clone(a.Witnesses[j].Span)
			a.Witnesses[j].Tokens = slices.Clone(a.Witnesses[j].Tokens)
		}
		a.TS, a.Anchors = slices.Clone(a.TS), slices.Clone(a.Anchors)
	}
	return y
}

func TestCSharpG1B34TrackedAssetIsTheDerivation(t *testing.T) {
	a := g1b34Load(t)
	data, err := MarshalCSharpG1B34(a.want)
	if err != nil || !bytes.Equal(data, a.expected) {
		t.Fatalf("the tracked %s is not the derivation (%v)", CSharpG1B34Path, err)
	}
	x, err := DecodeCSharpG1B34(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCSharpG1B34(a.expected, x, a.want); err != nil {
		t.Fatal(err)
	}
	if x.Inputs.KindMap.Path != CSharpG1KindMapPathR3 || x.Inputs.Observation.Path != CSharpG1RoslynPathR3 {
		t.Errorf("the pins name %s and %s", x.Inputs.KindMap.Path, x.Inputs.Observation.Path)
	}
}

// The numbers of the sealed asset. They replace the dry run of the classification (A 198, B 100, C 90): the audits moved a028 and a209 from B to
// blocked, so B is 98 and C 92.
func TestCSharpG1B34Counts(t *testing.T) {
	a := g1b34Load(t)
	c := a.want.Counts
	if c != (G1B34Counts{Alternatives: 388, A: 198, B: 98, C: 92, Expected: 296, Blocked: 92, Strong: 180, Weak: 116, Witnesses: 296}) {
		t.Errorf("counts moved: %+v", c)
	}
	rows := map[string]int{}
	blocked := map[string]int{}
	weakB, strongB := 0, 0
	for _, alt := range a.want.Alternatives {
		rows[alt.Row+" "+alt.Group+" "+alt.Status]++
		if alt.Status == G1ExpBlocked {
			blocked[alt.Block]++
			if alt.Group != G1ClassBlocked || len(alt.Witnesses) != 0 || alt.Basis != "" || alt.Reason == "" || !slices.Contains(G1ClassBlockCodes, alt.Block) {
				t.Errorf("%s: a blocked alternative is malformed: %+v", alt.ID, alt)
			}
			continue
		}
		if alt.Group == G1ClassStructure {
			if alt.Discriminating {
				strongB++
			} else {
				weakB++
			}
		}
	}
	wantRows := map[string]int{"csharp-B03 A EXPECTED": 137, "csharp-B03 B EXPECTED": 59, "csharp-B03 C BLOCKED": 61, "csharp-B04 A EXPECTED": 61, "csharp-B04 B EXPECTED": 39, "csharp-B04 C BLOCKED": 31}
	if !g1eSame(rows, wantRows) {
		t.Errorf("rows %v, want %v", rows, wantRows)
	}
	if !g1eSame(blocked, map[string]int{"ABSTRACT_CATEGORY": 36, "NO_OWN_NODE_FOR_CONSTRUCT": 31, "POSITION_DEPENDENT": 14, "SEMANTIC_BINDING": 9, "NO_WITNESS_IN_LINKED_SOURCES": 2}) {
		t.Errorf("blocked %v", blocked)
	}
	if strongB != 72 || weakB != 26 {
		t.Errorf("B: %d discriminating, %d weak; want 72 and 26", strongB, weakB)
	}
}

// Every expectation is anchored in a source text: an A witness has one span per anchor that holds the anchor's text, in order; a B witness is a
// node span of the Roslyn kind that the alternative's rule names. No A witness is the span of a bare punctuation lexeme found by accident
// without the anchor text, and no alternative has more witnesses than the limit.
func TestCSharpG1B34WitnessesLieInTheirSources(t *testing.T) {
	a := g1b34Load(t)
	for _, alt := range a.want.Alternatives {
		if alt.Status != G1ExpExpected {
			continue
		}
		if len(alt.Witnesses) != 1 || len(alt.Witnesses) > g1ExpWitnessLimit {
			t.Errorf("%s: %d witnesses", alt.ID, len(alt.Witnesses))
			continue
		}
		w, text := alt.Witnesses[0], a.texts[alt.Witnesses[0].Source]
		if text == nil || alt.Spec == "" || alt.Form == "" {
			t.Errorf("%s: source %q, spec %q, form %q", alt.ID, w.Source, alt.Spec, alt.Form)
			continue
		}
		switch alt.Group {
		case G1ClassLiteral:
			if alt.Basis != G1BasisSpec || w.Match != g1b34MatchTokens || len(w.Tokens) != len(alt.Anchors) || len(alt.Anchors) == 0 || len(alt.TS) != 0 {
				t.Errorf("%s: malformed A expectation %+v", alt.ID, alt)
				continue
			}
			for i, tk := range w.Tokens {
				if tk[0] < 0 || tk[0] >= tk[1] || tk[1] > len(text) || string(text[tk[0]:tk[1]]) != alt.Anchors[i].Text || len(alt.Anchors[i].TS) == 0 || i > 0 && tk[0] < w.Tokens[i-1][1] {
					t.Errorf("%s: anchor %d %+v does not hold %q", alt.ID, i, tk, alt.Anchors[i].Text)
				}
			}
		case G1ClassStructure:
			if alt.Basis != G1BasisDiff || w.Match != g1b34MatchSpan || len(w.Span) != 2 || w.Span[0] < 0 || w.Span[0] >= w.Span[1] || w.Span[1] > len(text) || !slices.Contains(alt.Kinds, w.Roslyn) || len(alt.TS) == 0 || len(alt.Anchors) != 0 {
				t.Errorf("%s: malformed B expectation %+v", alt.ID, alt)
			}
		default:
			t.Errorf("%s: group %s is expected", alt.ID, alt.Group)
		}
	}
}

// The audits' corrections change the derivation: without them the audited alternatives are B alternatives with the witness the audit refused or
// with no witness of the specification's extent, so a test that drops a correction is killed here.
func TestCSharpG1B34CorrectionsOfTheAuditsMatter(t *testing.T) {
	a := g1b34Load(t)
	unfixed := g1b34Without(t)
	for id := range g1b34Picks {
		if g1eSame(g1b34Alt(a.want, id).Witnesses, g1b34Alt(unfixed, id).Witnesses) {
			t.Errorf("%s: the pick changes nothing", id)
		}
	}
	for id := range g1b34Overrides {
		if g1b34Alt(unfixed, id).Status != G1ExpExpected || g1b34Alt(a.want, id).Status != G1ExpBlocked || g1b34Alt(a.want, id).Block != G1ClassBlockNoNode {
			t.Errorf("%s: the override does not turn an expectation into a blocked alternative", id)
		}
	}
	// the audit rows are the picks and overrides: each finding of the position audit names one of them, and nothing else is corrected
	var pos []string
	for _, f := range g1b34Findings("position") {
		pos = append(pos, f.ID)
	}
	var fixed []string
	for id := range g1b34Picks {
		fixed = append(fixed, id)
	}
	for id := range g1b34Overrides {
		fixed = append(fixed, id)
	}
	slices.Sort(pos)
	slices.Sort(fixed)
	if !slices.Equal(pos, fixed) {
		t.Errorf("the position findings %v are not the corrected alternatives %v", pos, fixed)
	}
}

func TestCSharpG1B34RejectsAlteredAssets(t *testing.T) {
	a := g1b34Load(t)
	x0, err := DecodeCSharpG1B34(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	type mutant struct {
		name string
		edit func(*CSharpG1B34)
		code string
	}
	first := func(x *CSharpG1B34, group string) *G1B34Alt {
		for i := range x.Alternatives {
			if x.Alternatives[i].Group == group && x.Alternatives[i].Status == G1ExpExpected {
				return &x.Alternatives[i]
			}
		}
		return nil
	}
	ms := []mutant{
		{"a type of a B alternative", func(x *CSharpG1B34) { first(x, "B").TS = []string{"identifier"} }, "G1_B34_ALTERNATIVES"},
		{"the span of a B witness", func(x *CSharpG1B34) { first(x, "B").Witnesses[0].Span[1]++ }, "G1_B34_ALTERNATIVES"},
		{"the type of an anchor", func(x *CSharpG1B34) { first(x, "A").Anchors[0].TS = []string{`"x"`} }, "G1_B34_ALTERNATIVES"},
		{"the token of an A witness", func(x *CSharpG1B34) { first(x, "A").Witnesses[0].Tokens[0][1]++ }, "G1_B34_ALTERNATIVES"},
		{"the weak flag", func(x *CSharpG1B34) { first(x, "B").Discriminating = !first(x, "B").Discriminating }, "G1_B34_ALTERNATIVES"},
		{"a status", func(x *CSharpG1B34) { first(x, "A").Status = G1ExpBlocked }, "G1_B34_ALTERNATIVES"},
		{"a blocked alternative made an expectation", func(x *CSharpG1B34) {
			for i := range x.Alternatives {
				if x.Alternatives[i].Status == G1ExpBlocked {
					x.Alternatives[i].Status, x.Alternatives[i].Block = G1ExpExpected, ""
					return
				}
			}
		}, "G1_B34_ALTERNATIVES"},
		{"an alternative left out", func(x *CSharpG1B34) { x.Alternatives = x.Alternatives[1:] }, "G1_B34_ALTERNATIVES"},
		{"two alternatives swapped", func(x *CSharpG1B34) { x.Alternatives[0], x.Alternatives[1] = x.Alternatives[1], x.Alternatives[0] }, "G1_B34_ALTERNATIVES"},
		{"the reason of a block", func(x *CSharpG1B34) {
			for i := range x.Alternatives {
				if x.Alternatives[i].Status == G1ExpBlocked {
					x.Alternatives[i].Block = G1ClassBlockSemantic + "X"
					return
				}
			}
		}, "G1_B34_ALTERNATIVES"},
		{"the counts", func(x *CSharpG1B34) { x.Counts.Weak++ }, "G1_B34_COUNTS"},
		{"the pin of the kind map", func(x *CSharpG1B34) { x.Inputs.KindMap.SHA256 = strings.Repeat("0", 64) }, "G1_B34_PIN"},
		{"the pin of the observation", func(x *CSharpG1B34) { x.Inputs.Observation.Bytes++ }, "G1_B34_PIN"},
		{"the basis", func(x *CSharpG1B34) { x.Basis += " " }, "G1_B34_COUNTS"},
	}
	for _, m := range ms {
		x := g1b34Clone(x0)
		m.edit(&x)
		if err := CheckCSharpG1B34(a.expected, x, a.want); g1Code(err) != m.code {
			t.Errorf("%s: %v, want %s", m.name, err, m.code)
		}
	}
	// the bytes: a canonical asset is one line per alternative and nothing else
	for name, data := range map[string][]byte{"trailing space": append(slices.Clone(a.expected), ' '), "a space inside": bytes.Replace(a.expected, []byte(`"schema": "`), []byte(`"schema":  "`), 1)} {
		if err := CheckCSharpG1B34(data, x0, a.want); g1Code(err) != "G1_B34_BYTES" {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the decoder
	for name, c := range map[string]struct {
		data []byte
		code string
	}{
		"unknown member": {bytes.Replace(a.expected, []byte(`"witness_limit"`), []byte(`"x": 1, "witness_limit"`), 1), "G1_B34_JSON"},
		"schema":         {bytes.Replace(a.expected, []byte(CSharpG1B34Schema), []byte("tsgk-csharp-g1-expected/r1"), 1), "G1_B34_SCHEMA"},
		"too large":      {make([]byte, csharpG1B34Max+1), "G1_B34_SIZE"},
		"not JSON":       {[]byte("{"), "G1_B34_JSON"},
	} {
		if _, err := DecodeCSharpG1B34(c.data); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
}

// The tightest run: the narrowest ordered run of the anchors, the first of equals, and none when the anchors do not occur in order.
func TestCSharpG1B34TightestRun(t *testing.T) {
	tok := func(texts ...string) []g1Tok {
		var out []g1Tok
		for i, s := range texts {
			out = append(out, g1Tok{start: i * 2, end: i*2 + 1, text: s})
		}
		return out
	}
	for _, c := range []struct {
		toks    []g1Tok
		anchors []string
		want    []int
	}{
		{tok("a", "(", "x", "a", "(", ")"), []string{"a", "("}, []int{0, 1}},
		{tok("a", "x", "x", "a", "("), []string{"a", "("}, []int{3, 4}},
		{tok("(", "a"), []string{"a", "("}, nil},
		{tok("a", "b", "c"), []string{"a", "b", "c"}, []int{0, 1, 2}},
		{tok("a"), []string{"a"}, []int{0}},
	} {
		if got := g1b34Tightest(c.toks, c.anchors); !slices.Equal(got, c.want) {
			t.Errorf("%v in %v: %v, want %v", c.anchors, c.toks, got, c.want)
		}
	}
}

// ---- the kind map r3 -------------------------------------------------------------------------------------------

func TestCSharpG1KindMapR3IsDerivedFromR2(t *testing.T) {
	a := g1b34Load(t)
	x, err := ParseCSharpG1KindMapR3(a.km3, a.km2)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Contextual) != 1 || x.Previous.Path != CSharpG1KindMapPathR2 || len(x.Kinds) != 461 {
		t.Errorf("r3: %d contextual entries, previous %s, %d kinds", len(x.Contextual), x.Previous.Path, len(x.Kinds))
	}
	// the kinds are r2's, entry for entry
	r2, err := ParseCSharpG1KindMap(a.km2, nil)
	if err != nil || !g1eSame(r2.Kinds, x.Kinds) || !g1eSame(r2.TSOnly, x.TSOnly) || !g1eSame(r2.Vocabulary, x.Vocabulary) {
		t.Errorf("r3 changes a kind of r2 (%v)", err)
	}
	for _, c := range []struct {
		kind, parent string
		want         []string
	}{
		{"SimpleAssignmentExpression", "WithInitializerExpression", []string{"with_initializer"}},
		{"SimpleAssignmentExpression", "ExpressionStatement", []string{"assignment_expression"}},
		{"SimpleAssignmentExpression", "", []string{"assignment_expression"}},
		{"CheckedExpression", "ExpressionStatement", []string{"checked_expression"}},
		{"NoSuchKind", "", nil},
	} {
		if got := x.TypesOf(c.kind, c.parent); !slices.Equal(got, c.want) {
			t.Errorf("TypesOf(%s, %s) = %v, want %v", c.kind, c.parent, got, c.want)
		}
	}
	// the one 1:n node kind the B alternatives use is split, and no other 1:n kind is used by a B alternative
	a1n := map[string]bool{}
	for _, k := range x.Kinds {
		if k.Class == "node" && k.Map == G1MapOneToN {
			a1n[k.Roslyn] = true
		}
	}
	for _, alt := range a.want.Alternatives {
		for _, k := range alt.Kinds {
			if a1n[k] && !slices.ContainsFunc(x.Contextual, func(c G1KMContextual) bool { return c.Roslyn == k }) {
				t.Errorf("%s: the 1:n kind %s has no contextual entry", alt.ID, k)
			}
		}
	}
	if g1b34Alt(a.want, "csharp-B03.a226").TS[0] != "assignment_expression" || len(g1b34Alt(a.want, "csharp-B03.a226").TS) != 1 {
		t.Errorf("a226 types: %v", g1b34Alt(a.want, "csharp-B03.a226").TS)
	}
}

func TestCSharpG1KindMapR3RejectsAlteredMaps(t *testing.T) {
	a := g1b34Load(t)
	x, err := ParseCSharpG1KindMapR3(a.km3, a.km2)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() CSharpG1KindMapR3 {
		y := x
		y.Contextual = slices.Clone(x.Contextual)
		y.Kinds = slices.Clone(x.Kinds)
		return y
	}
	write := func(y CSharpG1KindMapR3) []byte {
		d, err := MarshalCSharpG1KindMapR3(y)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	// the checks of the entries (a content that no derivation can give is rejected by its own code before its bytes are compared)
	checks := []struct {
		name string
		edit func(*CSharpG1KindMapR3)
		code string
	}{
		{"an entry that does not partition the types", func(y *CSharpG1KindMapR3) { y.Contextual[0].Otherwise = []string{"assignment_expression", "x"} }, "G1_KINDMAP3_PARTITION"},
		{"an entry of a kind that is not 1:n", func(y *CSharpG1KindMapR3) { y.Contextual[0].Roslyn = "CheckedExpression" }, "G1_KINDMAP3_ENTRY"},
		{"a parent that is not a kind", func(y *CSharpG1KindMapR3) { y.Contextual[0].Parent = []string{"NoSuchKind"} }, "G1_KINDMAP3_PARENT"},
		{"a parent token kind", func(y *CSharpG1KindMapR3) { y.Contextual[0].Parent = []string{"SemicolonToken"} }, "G1_KINDMAP3_PARENT"},
		{"an entry without a ground", func(y *CSharpG1KindMapR3) { y.Contextual[0].Grounds = nil }, "G1_KINDMAP3_ENTRY"},
		{"a ground of the specification", func(y *CSharpG1KindMapR3) { y.Contextual[0].Grounds[0].Doc = G1DocSpec }, "G1_KINDMAP3_GROUND"},
		{"a ground quote that is too short", func(y *CSharpG1KindMapR3) { y.Contextual[0].Grounds[0].Quote = "with_initializer" }, "G1_KINDMAP3_GROUND"},
	}
	// what differs from the derivation of r2: the pin and the content
	cases := []struct {
		name string
		edit func(*CSharpG1KindMapR3)
		code string
	}{
		{"the pin of r2", func(y *CSharpG1KindMapR3) { y.Previous.SHA256 = strings.Repeat("0", 64) }, "G1_KINDMAP3_PREVIOUS"},
		{"an entry of another parent", func(y *CSharpG1KindMapR3) { y.Contextual[0].Parent = []string{"Block"} }, "G1_KINDMAP3_BYTES"},
		{"a changed kind", func(y *CSharpG1KindMapR3) { y.Kinds[3].Map = G1MapOneToN }, "G1_KINDMAP3_BYTES"},
		{"another reason", func(y *CSharpG1KindMapR3) { y.Contextual[0].Reason += "." }, "G1_KINDMAP3_BYTES"},
	}
	fresh := func() CSharpG1KindMapR3 {
		y := clone()
		y.Contextual = []G1KMContextual{x.Contextual[0]}
		y.Contextual[0].Parent = slices.Clone(x.Contextual[0].Parent)
		y.Contextual[0].Grounds = slices.Clone(x.Contextual[0].Grounds)
		return y
	}
	for _, c := range checks {
		y := fresh()
		c.edit(&y)
		if err := g1km3Check(y); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	for _, c := range cases {
		y := fresh()
		c.edit(&y)
		if _, err := ParseCSharpG1KindMapR3(write(y), a.km2); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := ParseCSharpG1KindMapR3(bytes.Replace(a.km3, []byte(CSharpG1KindMapSchemaR3), []byte(CSharpG1KindMapSchemaR2), 1), a.km2); g1Code(err) != "G1_KINDMAP3_SCHEMA" {
		t.Errorf("schema: %v", err)
	}
	if _, err := ParseCSharpG1KindMapR3(a.km3, a.km3); err == nil {
		t.Errorf("r3 over r3 is accepted")
	}
}

// ---- the audits ------------------------------------------------------------------------------------------------

func TestCSharpG1AuditB34Verifies(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34AuditView(a.expected, a.texts)
	if err != nil {
		t.Fatal(err)
	}
	x, err := ParseCSharpG1AuditB34(a.pos, view)
	if err != nil {
		t.Fatal(err)
	}
	want := G1B34AuditCounts{Rows: 98, Match: 98, Mismatch: 0, Node: 1, Anchors: 198, AnchorMatch: 198, AnchorMismatch: 0, Findings: 8}
	if x.Counts != want {
		t.Errorf("counts %+v, want %+v", x.Counts, want)
	}
	if len(g1b34PosRuleTexts) != 98 {
		t.Errorf("%d rules for 98 rows", len(g1b34PosRuleTexts))
	}
	// the rows are the B alternatives of the expected asset and the anchor rows its A alternatives
	for _, alt := range a.want.Alternatives {
		if alt.Status != G1ExpExpected {
			continue
		}
		inRows := slices.ContainsFunc(x.Rows, func(r G1B34PosRow) bool { return r.ID == alt.ID })
		inAnchors := slices.ContainsFunc(x.Anchors, func(r G1B34AnchorRow) bool { return r.ID == alt.ID })
		if inRows != (alt.Group == G1ClassStructure) || inAnchors != (alt.Group == G1ClassLiteral) {
			t.Errorf("%s: row %t anchors %t for group %s", alt.ID, inRows, inAnchors, alt.Group)
		}
	}
}

// What the position audit compares is the witness: a witness moved by one byte, or an anchor token moved by one byte, is a MISMATCH of the
// derivation (not a failure of the verifier: the verifier then rejects the asset because its stored result is another).
func TestCSharpG1AuditB34ComparesTheWitnesses(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34AuditView(a.expected, a.texts)
	if err != nil {
		t.Fatal(err)
	}
	x, err := DecodeCSharpG1AuditB34(a.pos)
	if err != nil {
		t.Fatal(err)
	}
	shifted := func(group string) G1B34AuditInput {
		v := view
		v.Alts = slices.Clone(view.Alts)
		for i := range v.Alts {
			if v.Alts[i].Group != group {
				continue
			}
			if group == G1ClassStructure {
				v.Alts[i].Witness.End++
			} else {
				v.Alts[i].Tokens = slices.Clone(v.Alts[i].Tokens)
				v.Alts[i].Tokens[0][1]++
			}
			return v
		}
		return v
	}
	for _, group := range []string{G1ClassStructure, G1ClassLiteral} {
		v := shifted(group)
		got, err := DeriveCSharpG1AuditB34(x.Rows, v)
		if err != nil {
			t.Fatal(err)
		}
		if got.Counts.Mismatch+got.Counts.AnchorMismatch != 1 {
			t.Errorf("a moved %s witness: %+v", group, got.Counts)
		}
		if _, err := ParseCSharpG1AuditB34(a.pos, v); err == nil || !strings.HasPrefix(g1Code(err), "G1_AUDITB34_") {
			t.Errorf("the verifier accepts the audit of a moved %s witness: %v", group, err)
		}
	}
	// a witness of another source, and a missing alternative
	v := view
	v.Alts = slices.Clone(view.Alts)
	for i := range v.Alts {
		if v.Alts[i].Group == G1ClassStructure {
			v.Alts[i].Witness.Source = "alt-csharp-B03-01#0"
			break
		}
	}
	if _, err := ParseCSharpG1AuditB34(a.pos, v); g1Code(err) != "G1_AUDITB34_ROWS" {
		t.Errorf("another source: %v", err)
	}
	v.Alts = view.Alts[1:]
	if _, err := ParseCSharpG1AuditB34(a.pos, v); g1Code(err) != "G1_AUDITB34_ROWS" {
		t.Errorf("a missing alternative: %v", err)
	}
}

func TestCSharpG1AuditB34RejectsAlteredAudits(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34AuditView(a.expected, a.texts)
	if err != nil {
		t.Fatal(err)
	}
	x0, err := DecodeCSharpG1AuditB34(a.pos)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() CSharpG1AuditB34 {
		x := x0
		x.Rows = slices.Clone(x0.Rows)
		x.Anchors = slices.Clone(x0.Anchors)
		x.Findings = slices.Clone(x0.Findings)
		return x
	}
	cases := []struct {
		name string
		edit func(*CSharpG1AuditB34)
		code string
	}{
		{"an instance moved", func(x *CSharpG1AuditB34) {
			x.Rows[0].Instances = [][2]int{{x.Rows[0].Instances[0][0] + 1, x.Rows[0].Instances[0][1] + 1}}
		}, "G1_AUDITB34_"},
		{"an instance of the wrong text", func(x *CSharpG1AuditB34) { x.Rows[1].Instances = [][2]int{{0, 3}} }, "G1_AUDITB34_RULE"},
		{"no instance", func(x *CSharpG1AuditB34) { x.Rows[0].Instances = nil }, "G1_AUDITB34_ROW"},
		{"a result", func(x *CSharpG1AuditB34) { x.Rows[0].Result = G1AuditMismatch }, "G1_AUDITB34_COMPARISON"},
		{"a relation", func(x *CSharpG1AuditB34) { x.Rows[0].Relation = G1AuditRelNodeSpan }, "G1_AUDITB34_COMPARISON"},
		{"a row left out", func(x *CSharpG1AuditB34) { x.Rows = x.Rows[1:] }, "G1_AUDITB34_ROWS"},
		{"a row of another alternative", func(x *CSharpG1AuditB34) { x.Rows[0].ID = x.Rows[1].ID }, "G1_AUDITB34_ROWS"},
		{"a ground quote edited", func(x *CSharpG1AuditB34) {
			g := slices.Clone(x.Rows[0].Grounds)
			g[0].Quote += " x"
			x.Rows[0].Grounds = g
		}, "G1_AUDITB34_GROUND"},
		{"a ground of another row", func(x *CSharpG1AuditB34) { x.Rows[0].Grounds = x.Rows[2].Grounds }, "G1_AUDITB34_GROUND"},
		{"a ground without the specification", func(x *CSharpG1AuditB34) {
			g := slices.Clone(x.Rows[0].Grounds)
			g[0].Doc = G1DocGrammar
			x.Rows[0].Grounds = g
		}, "G1_AUDITB34_ROW"},
		{"an anchor row result", func(x *CSharpG1AuditB34) { x.Anchors[0].Result = G1AuditMismatch }, "G1_AUDITB34_ANCHORS"},
		{"an anchor span", func(x *CSharpG1AuditB34) {
			x.Anchors[0].Tokens = slices.Clone(x.Anchors[0].Tokens)
			x.Anchors[0].Tokens[0][1]++
		}, "G1_AUDITB34_ANCHORS"},
		{"a finding removed", func(x *CSharpG1AuditB34) { x.Findings = x.Findings[1:] }, "G1_AUDITB34_FINDINGS"},
		{"the counts", func(x *CSharpG1AuditB34) { x.Counts.Match-- }, "G1_AUDITB34_COUNTS"},
	}
	for _, c := range cases {
		x := clone()
		c.edit(&x)
		data, err := MarshalCSharpG1AuditB34(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1AuditB34(data, view); err == nil || !strings.HasPrefix(g1Code(err), c.code) {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	for name, c := range map[string]struct {
		data []byte
		code string
	}{
		"unknown member": {bytes.Replace(a.pos, []byte(`"counts"`), []byte(`"x": 1, "counts"`), 1), "G1_AUDITB34_JSON"},
		"schema":         {bytes.Replace(a.pos, []byte(CSharpG1AuditB34Schema), []byte("tsgk-csharp-g1-audit76/r1"), 1), "G1_AUDITB34_SCHEMA"},
		"bytes":          {append(slices.Clone(a.pos), ' '), "G1_AUDITB34_BYTES"},
	} {
		if _, err := ParseCSharpG1AuditB34(c.data, view); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
}

// The lexer of the anchor check reads comments, strings, characters, numbers, identifiers and punctuators of the C# lexical grammar; it
// knows nothing of the Roslyn observation.
func TestCSharpG1AuditB34Lexemes(t *testing.T) {
	src := []byte("a>>=b; // x(y)\n/* ( */ var s = \"(\" + @\"\"\"(\" + 'c' + '\\'' + $\"{x}\"; List<List<int>> l = 1.5f >> 2; \n#if A\n x?.y ?? z;\n#endif\n")
	got := g1b34Lexemes(src)
	at := func(text string, n int) [2]int { // the n-th occurrence of text outside the quoted places
		i := -1
		for k := 0; k <= n; k++ {
			i = bytes.Index(src[i+1:], []byte(text)) + i + 1
		}
		return [2]int{i, i + len(text)}
	}
	for _, c := range []struct {
		span [2]int
		text string
	}{
		{at("a", 0), "a"}, {at(">>=", 0), ">>="}, {at(";", 0), ";"}, {at("var", 0), "var"}, {at("=", 2), "="}, {at("+", 0), "+"},
		{at("1.5f", 0), "1.5f"}, {at(">>", 1), ">>"}, {at("?", 0), "?"}, {at(".", 1), "."}, {at("??", 0), "??"},
	} {
		if got[c.span] != c.text {
			t.Errorf("%v: %q, want %q", c.span, got[c.span], c.text)
		}
	}
	for _, off := range []int{bytes.Index(src, []byte("x(y)")), bytes.Index(src, []byte("( */")), bytes.Index(src, []byte("\"(\"")) + 1, bytes.Index(src, []byte("#if"))} {
		for span := range got {
			if span[0] <= off && off < span[1] {
				t.Errorf("a lexeme %v %q covers the comment, string or directive text at %d", span, got[span], off)
			}
		}
	}
	// the closing of a generic argument list: > and > as single characters inside >>
	i := bytes.Index(src, []byte(">>")) // the first >> is the one of a>>=b: not a generic; the second is List<List<int>>
	j := bytes.Index(src[i+3:], []byte(">>")) + i + 3
	if got[[2]int{j, j + 1}] != ">" || got[[2]int{j + 1, j + 2}] != ">" || got[[2]int{j, j + 2}] != ">>" {
		t.Errorf(">> of a generic list: %q %q %q", got[[2]int{j, j + 1}], got[[2]int{j + 1, j + 2}], got[[2]int{j, j + 2}])
	}
	if g1b34AnchorResult([]string{"a", ";"}, [][2]int{at("a", 0), at(";", 0)}, got) != G1AuditMatch ||
		g1b34AnchorResult([]string{";", "a"}, [][2]int{at(";", 0), at("a", 0)}, got) != G1AuditMismatch ||
		g1b34AnchorResult([]string{"("}, [][2]int{{bytes.Index(src, []byte("( */")), bytes.Index(src, []byte("( */")) + 1}}, got) != G1AuditMismatch ||
		g1b34AnchorResult(nil, nil, got) != G1AuditMismatch {
		t.Errorf("the anchor check")
	}
}

func TestCSharpG1TypeAuditB34Verifies(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34TypeView(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	x, err := ParseCSharpG1TypeAuditB34(a.typ, view)
	if err != nil {
		t.Fatal(err)
	}
	if x.Counts != (G1B34TypeCounts{Rows: 98, Match: 98, Anchors: 119, AnchorMatch: 119, Findings: 3}) {
		t.Errorf("counts %+v", x.Counts)
	}
	classes := map[string]int{}
	for _, r := range x.Rows {
		classes[r.Class]++
	}
	for _, r := range x.Anchors {
		classes["anchor "+r.Class]++
	}
	if !g1eSame(classes, map[string]int{G1TypeNode: 93, G1TypeNoOwnNode: 4, G1TypeAlias: 1, "anchor NODE": 92, "anchor NOT_DERIVABLE": 27}) {
		t.Errorf("classes %v", classes)
	}
	// the type audit sees the types of the expected asset and nothing else: its view has no span
	if len(view.Alts) != 98 || len(view.Anchors) != 119 {
		t.Errorf("view: %d alternatives, %d anchors", len(view.Alts), len(view.Anchors))
	}
}

func TestCSharpG1TypeAuditB34ComparesTheTypes(t *testing.T) {
	a := g1b34Load(t)
	view, err := G1B34TypeView(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	x, err := DecodeCSharpG1TypeAuditB34(a.typ)
	if err != nil {
		t.Fatal(err)
	}
	// the expected types of one alternative widened, narrowed, replaced: each is a MISMATCH with its own relation
	for name, ts := range map[string][]string{"superset": append(slices.Clone(view.Alts[0].TS), "identifier"), "other": {"identifier"}, "none": nil} {
		v := view
		v.Alts = slices.Clone(view.Alts)
		v.Alts[0].TS = ts
		got, err := DeriveCSharpG1TypeAuditB34(x.Rows, x.Anchors, v)
		if err != nil {
			t.Fatal(err)
		}
		if got.Counts.Mismatch != 1 {
			t.Errorf("%s: %+v", name, got.Counts)
		}
		if _, err := ParseCSharpG1TypeAuditB34(a.typ, v); err == nil {
			t.Errorf("%s: the verifier accepts a type audit of other types", name)
		}
	}
	// the r2 entry of SimpleAssignmentExpression, as the audit found it: the grammar derives assignment_expression
	if res, rel := g1b34TypeCompare(G1TypeNode, []string{"assignment_expression"}, nil, []string{"assignment_expression", "with_initializer"}); res != G1AuditMismatch || rel != G1TypeRelSuperset {
		t.Errorf("a226 before r3: %s %s", res, rel)
	}
	v := view
	v.Anchors = slices.Clone(view.Anchors)
	v.Anchors[0].TS = []string{"identifier"}
	if got, err := DeriveCSharpG1TypeAuditB34(x.Rows, x.Anchors, v); err != nil || got.Counts.AnchorMismatch != 1 {
		t.Errorf("an anchor of other types: %+v %v", got.Counts, err)
	}
	if _, err := ParseCSharpG1TypeAuditB34(a.typ, G1B34TypeInput{Alts: view.Alts[1:], Anchors: view.Anchors}); g1Code(err) != "G1_TYPEAUDITB34_ROWS" {
		t.Errorf("a missing alternative: %v", err)
	}
}

func TestCSharpG1TypeAuditB34RejectsAlteredAudits(t *testing.T) {
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
		x.Rows, x.Anchors, x.Findings = slices.Clone(x0.Rows), slices.Clone(x0.Anchors), slices.Clone(x0.Findings)
		return x
	}
	cases := []struct {
		name string
		edit func(*CSharpG1TypeAuditB34)
		code string
	}{
		{"types of a row", func(x *CSharpG1TypeAuditB34) { x.Rows[0].Types = []string{"identifier"} }, "G1_TYPEAUDITB34_COMPARISON"},
		{"unsorted types", func(x *CSharpG1TypeAuditB34) { x.Rows[0].Types = []string{"b", "a"} }, "G1_TYPEAUDITB34_ROW"},
		{"a class", func(x *CSharpG1TypeAuditB34) { x.Rows[0].Class = G1TypeNotDerivable }, "G1_TYPEAUDITB34_ROW"},
		{"a result", func(x *CSharpG1TypeAuditB34) { x.Rows[0].Result = G1AuditMismatch }, "G1_TYPEAUDITB34_COMPARISON"},
		{"a ground edited", func(x *CSharpG1TypeAuditB34) {
			g := slices.Clone(x.Rows[0].Grounds)
			g[0].Quote += " x"
			x.Rows[0].Grounds = g
		}, "G1_TYPEAUDITB34_GROUND"},
		{"a ground of the specification", func(x *CSharpG1TypeAuditB34) {
			g := slices.Clone(x.Rows[0].Grounds)
			g[0].Doc = G1DocSpec
			x.Rows[0].Grounds = g
		}, "G1_TYPEAUDITB34_ROW"},
		{"a row left out", func(x *CSharpG1TypeAuditB34) { x.Rows = x.Rows[1:] }, "G1_TYPEAUDITB34_ROWS"},
		{"an anchor of other types", func(x *CSharpG1TypeAuditB34) { x.Anchors[0].Types = []string{`"zzz"`} }, "G1_TYPEAUDITB34_COMPARISON"},
		{"a finding changed", func(x *CSharpG1TypeAuditB34) { x.Findings[0].Fix = "none" }, "G1_TYPEAUDITB34_FINDINGS"},
		{"the counts", func(x *CSharpG1TypeAuditB34) { x.Counts.Match-- }, "G1_TYPEAUDITB34_COUNTS"},
	}
	for _, c := range cases {
		x := clone()
		c.edit(&x)
		data, err := MarshalCSharpG1TypeAuditB34(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1TypeAuditB34(data, view); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := ParseCSharpG1TypeAuditB34(append(slices.Clone(a.typ), ' '), view); g1Code(err) != "G1_TYPEAUDITB34_BYTES" {
		t.Errorf("bytes: %v", err)
	}
}

// The findings are authored once and held by both assets: each finding names an alternative of the expected asset, and the fix it states is
// the correction the derivation carries.
func TestCSharpG1B34FindingsNameCorrectedAlternatives(t *testing.T) {
	a := g1b34Load(t)
	if len(g1b34Findings("position")) != 8 || len(g1b34Findings("type")) != 3 || len(g1b34AuditFindings) != 11 {
		t.Errorf("findings: %d position, %d type", len(g1b34Findings("position")), len(g1b34Findings("type")))
	}
	for _, f := range g1b34AuditFindings {
		if g1b34Alt(a.want, f.ID).ID == "" || f.What == "" || f.Fix == "" || f.Audit != "position" && f.Audit != "type" {
			t.Errorf("finding %+v", f)
		}
	}
}

// ---- seal ------------------------------------------------------------------------------------------------------

// The assets of this stage are sealed: their SHA-256 is pinned here, before any native result is read. A change is a new revision, not an edit.
func TestCSharpG1B34AssetsAreSealed(t *testing.T) {
	sealed := map[string]string{
		CSharpG1B34Path:          "f37563567c1298900a41751bec3eff299e8c81bff25adc72352624c59d631bcc",
		CSharpG1KindMapPathR3:    "ecba9e296c22baacf5eb5feabe6c5f25c802681700b5247c83e8cd1c3e87aadf",
		CSharpG1AuditB34Path:     "d18fafc9b60de0bf7d48c05dc4087e6f807d2aafe6d47faebb79021d836008d0",
		CSharpG1TypeAuditB34Path: "d3a7b23b9f168547ee6722bdf98ec51f33784cf5c4c1e8169313f91eb92e5ceb",
	}
	for p, want := range sealed {
		data, err := os.ReadFile("../../" + p)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s is %s, sealed %s", p, got, want)
		}
	}
}

// With the pinned documents (a checkout of grammar.js and of the specification files, as in the audit of the 76 alternatives) every ground of
// the new assets is a quote of a document at its place.
func TestCSharpG1B34GroundsAgainstTheDocuments(t *testing.T) {
	gp, sp := os.Getenv("TSGK_G1_GRAMMAR_JS"), os.Getenv("TSGK_G1_SPEC_DIR")
	if gp == "" || sp == "" {
		t.Skip("TSGK_G1_GRAMMAR_JS and TSGK_G1_SPEC_DIR are not set: the grounds are checked by their pins only")
	}
	g, err := os.ReadFile(gp)
	if err != nil {
		t.Fatal(err)
	}
	spec := map[string]string{}
	for _, name := range G1SpecFiles {
		d, err := os.ReadFile(sp + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		spec[name] = string(d)
	}
	a := g1b34Load(t)
	pos, err := DecodeCSharpG1AuditB34(a.pos)
	if err != nil {
		t.Fatal(err)
	}
	typ, err := DecodeCSharpG1TypeAuditB34(a.typ)
	if err != nil {
		t.Fatal(err)
	}
	var km3 CSharpG1KindMapR3
	if err := jsonv2.Unmarshal(a.km3, &km3); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"position audit": CheckG1AuditB34Grounds(pos.Rows, string(g), spec),
		"type audit":     CheckG1TypeAuditB34Grounds(typ, string(g)),
		"kind map r3":    CheckG1KindMapR3Grounds(km3, string(g)),
	} {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
