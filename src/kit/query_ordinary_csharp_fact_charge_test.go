package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/bits"
	"math/rand/v2"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// csharpFactGen builds the captures of a synthetic C# tree over a source it appends to or
// is given, with the node, match and point bookkeeping of the pack's dynamic SQL query.
type csharpFactGen struct {
	src   []byte
	utf16 bool
	caps  []Capture
	node  int64
	match uint32
	sites int
}

type csharpFactNode struct {
	id         int64
	typ        string
	start, end uint32
}

// text appends text to the source and returns its node.
func (g *csharpFactGen) text(typ, text string) csharpFactNode {
	start := len(g.src)
	for i := 0; i < len(text); i++ {
		g.src = append(g.src, text[i])
		if g.utf16 {
			g.src = append(g.src, 0)
		}
	}
	return g.span(typ, start, len(g.src))
}

func (g *csharpFactGen) span(typ string, start, end int) csharpFactNode {
	g.node++
	return csharpFactNode{g.node, typ, uint32(start), uint32(end)}
}

func (g *csharpFactGen) point(offset uint32) Point {
	before := g.src[:offset]
	row := bytes.Count(before, []byte("\n"))
	return Point{Row: uint32(row), Column: offset - uint32(bytes.LastIndexByte(before, '\n')+1)}
}

func (g *csharpFactGen) pair(parent string, p csharpFactNode, child string, c csharpFactNode) {
	g.match++
	for _, x := range []struct {
		name string
		n    csharpFactNode
	}{{parent, p}, {child, c}} {
		g.caps = append(g.caps, Capture{Match: g.match, Name: x.name, Node: x.n.id, Type: x.n.typ, Named: true, StartByte: x.n.start, EndByte: x.n.end, StartPoint: g.point(x.n.start), EndPoint: g.point(x.n.end)})
	}
}

// command adds `new <depth qualifiers>SqlCommand(arg)`.
func (g *csharpFactGen) command(depth int, arg string) {
	oc := g.text("object_creation_expression", "new")
	head := g.text("qualified_name", "Q")
	typ := head
	for i := 0; i < depth; i++ {
		next := g.text("qualified_name", "Q")
		g.pair("qn", typ, "qn.name", next)
		typ = next
	}
	name := g.text("identifier", "SqlCommand")
	if depth == 0 {
		head = name
	} else {
		g.pair("aq", typ, "aq.name", name)
	}
	g.pair("oc", oc, "oc.type", head)
	a := g.text("argument", "(")
	g.pair("oc.a", oc, "oc.arg", a)
	g.pair("arg", a, "arg.expr", g.text("identifier", arg))
	g.sites++
}

// assignment adds `x.CommandText = arg` as a member access or an initializer.
func (g *csharpFactGen) assignment(initializer bool, arg string) {
	as := g.text("assignment_expression", "=")
	left, right := g.text("identifier", "CommandText"), g.text("identifier", arg)
	if initializer {
		g.pair("ini.as", as, "ini.left", left)
		g.pair("ini.as", as, "ini.right", right)
	} else {
		g.pair("ma.as", as, "ma.name", left)
		g.pair("ma.as", as, "ma.right", right)
	}
	g.sites++
}

func csharpFactSites(sites int) *csharpFactGen {
	g := &csharpFactGen{}
	for i := 0; i < sites; i++ {
		if i%3 == 2 {
			g.assignment(i%2 == 0, fmt.Sprintf("q%d", i))
		} else {
			g.command(i%3, fmt.Sprintf("q%d", i))
		}
	}
	return g
}

var (
	csharpFixtureBinary = regexp.MustCompile(` \+ `)
	csharpFixtureIdent  = regexp.MustCompile(`^[A-Za-z_@\\][\w\\@]*$`)
	csharpFixtureAssign = regexp.MustCompile(`^(\s+)(\w+)\.(\S+) = (.+);$`)
)

// expr adds the expression at src[start:end] with its binary operator, when it has one.
func (g *csharpFactGen) expr(start int, text string) csharpFactNode {
	end := start + len(text)
	if at := csharpFixtureBinary.FindStringIndex(text); at != nil {
		bin := g.span("binary_expression", start, end)
		g.pair("bin", bin, "bin.op", g.span("+", start+at[0]+1, start+at[0]+2))
		return bin
	}
	typ := "member_access_expression"
	switch {
	case strings.HasPrefix(text, `"""`):
		typ = "raw_string_literal"
	case strings.HasPrefix(text, `@"`):
		typ = "verbatim_string_literal"
	case strings.HasPrefix(text, `$"`):
		typ = "interpolated_string_expression"
	case strings.HasPrefix(text, `"`):
		typ = "string_literal"
	case csharpFixtureIdent.MatchString(text):
		typ = "identifier"
	case strings.Contains(text, "("):
		typ = "invocation_expression"
	}
	return g.span(typ, start, end)
}

// csharpFixtureGen builds the captures the pack's C# query yields on the tracked fixture
// src/testdata/native/dynamic-sql/csharp-command.cs, statement by statement.
func csharpFixtureGen(t *testing.T) *csharpFactGen {
	t.Helper()
	src, err := os.ReadFile("../testdata/native/dynamic-sql/csharp-command.cs")
	if err != nil {
		t.Fatal(err)
	}
	g := &csharpFactGen{src: src}
	offset := 0
	for _, line := range strings.SplitAfter(string(src), "\n") {
		trimmed := strings.TrimRight(line, "\n")
		base := offset
		offset += len(line)
		if m := csharpFixtureAssign.FindStringSubmatchIndex(trimmed); m != nil && !strings.Contains(trimmed, "new ") {
			as := g.span("assignment_expression", base+m[2*2], base+m[2*4+1])
			right := g.expr(base+m[2*4], trimmed[m[2*4]:m[2*4+1]])
			g.pair("ma.as", as, "ma.name", g.span("identifier", base+m[2*3], base+m[2*3+1]))
			g.pair("ma.as", as, "ma.right", right)
			continue
		}
		at := strings.Index(trimmed, "new ")
		if at < 0 || strings.HasPrefix(strings.TrimSpace(trimmed), "//") {
			continue
		}
		typeEnd := at + 4 + strings.IndexAny(trimmed[at+4:], "( ")
		oc := g.span("object_creation_expression", base+at, base+len(trimmed)-1)
		typ := g.span("identifier", base+at+4, base+typeEnd)
		if strings.Contains(trimmed[at+4:typeEnd], ".") {
			typ.typ = "qualified_name"
			dot := strings.LastIndex(trimmed[:typeEnd], ".")
			g.pair("qn", typ, "qn.name", g.span("identifier", base+dot+1, base+typeEnd))
		}
		g.pair("oc", oc, "oc.type", typ)
		rest := trimmed[typeEnd:]
		if strings.HasPrefix(rest, "(") && rest != "();" {
			args := rest[1 : len(rest)-2]
			if cut := strings.LastIndex(args, ", "); cut >= 0 && csharpFixtureIdent.MatchString(args[cut+2:]) {
				args = args[:cut]
			}
			arg := g.span("argument", base+typeEnd+1, base+typeEnd+1+len(args))
			g.pair("oc.a", oc, "oc.arg", arg)
			g.pair("arg", arg, "arg.expr", g.expr(base+typeEnd+1, args))
		}
		if body, ok := strings.CutPrefix(rest, " { "); ok {
			from := base + typeEnd + 3
			for _, member := range strings.Split(strings.TrimSuffix(body, " };"), ", ") {
				name, value, _ := strings.Cut(member, " = ")
				as := g.span("assignment_expression", from, from+len(member))
				g.pair("ini.as", as, "ini.left", g.span("identifier", from, from+len(name)))
				g.pair("ini.as", as, "ini.right", g.expr(from+len(name)+3, value))
				from += len(member) + 2
			}
		}
	}
	return g
}

// csharpFactSteps runs the adapter and counts the steps it really takes.
func csharpFactSteps(t *testing.T, caps []Capture, src []byte, enc string) (facts DynamicSQLFacts, pairSteps, hops, decoded uint64) {
	t.Helper()
	prev := factStepHook
	factStepHook = func(kind string, n uint64) {
		switch kind {
		case "pair-steps":
			pairSteps += n
		case "chain-hops":
			hops += n
		case "decoded-bytes":
			decoded += n
		}
	}
	defer func() { factStepHook = prev }()
	facts, err := DeriveDynamicSQLChecked("csharp", enc, caps, src)
	if err != nil {
		t.Fatal(err)
	}
	return facts, pairSteps, hops, decoded
}

// csharpFactCharges returns the three C# terms (sorted passes, chain, decode) of the reservation.
func csharpFactCharges(t *testing.T, caps []Capture, src []byte) (terms [3]uint64, logical uint64, err *Error) {
	t.Helper()
	b := semanticTestBudget(t)
	var got []uint64
	prev := ordinaryChargeHook
	ordinaryChargeHook = func(_ *eligibilityBudget, work, _ uint64, at string, _, _ uint64) {
		if at == "ordinary-base-fact-derive" {
			got = append(got, work)
		}
	}
	defer func() { ordinaryChargeHook = prev }()
	err = b.ordinaryBaseFactReservation(FactQuery{Facts: FactDynamicSQL}, FactPackRoute{Route: "csharp"}, ordinaryFactBudgetQuery(caps), src)
	if err == nil {
		copy(terms[:], got[1:]) // got[0] is the shared capture conversion
	}
	return terms, b.logical, err
}

// Every step the adapter takes costs 64 work in the matching term: the unit of the shared
// capture conversion charge. The sorted passes cover the scans, comparisons and outputs of
// the eleven pair calls and the site sort; the chain term the lastIdent hops; the decode
// term the bytes handed to srcText.span.
func assertCSharpFactChargeCovers(t *testing.T, label string, caps []Capture, src []byte, enc string) DynamicSQLFacts {
	t.Helper()
	facts, pairSteps, hops, decoded := csharpFactSteps(t, caps, src, enc)
	terms, _, err := csharpFactCharges(t, caps, src)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	t.Logf("%s: captures=%d steps pair=%d hops=%d decoded=%d, charged terms=%v", label, len(caps), pairSteps, hops, decoded, terms)
	if 64*pairSteps > terms[0] || 64*hops > terms[1] || 64*decoded > terms[2] {
		t.Fatalf("%s: steps pair=%d hops=%d decoded=%d (x64) exceed charged terms %v", label, pairSteps, hops, decoded, terms)
	}
	return facts
}

func TestOrdinaryBaseCSharpFactChargeCoversTrackedFixture(t *testing.T) {
	g := csharpFixtureGen(t)
	facts := assertCSharpFactChargeCovers(t, "csharp-command.cs", g.caps, g.src, EncodingUTF8)
	var doc struct {
		Files []struct {
			Path  string           `json:"path"`
			Facts []DynamicSQLFact `json:"facts"`
		} `json:"files"`
	}
	data, err := os.ReadFile("../testdata/native/dynamic-sql/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, f := range doc.Files {
		if f.Path == "csharp-command.cs" {
			if len(f.Facts) == 0 || !reflect.DeepEqual(f.Facts, facts.Items) {
				t.Fatalf("derived facts differ from the tracked expected facts:\nwant %+v\n got %+v", f.Facts, facts.Items)
			}
			return
		}
	}
	t.Fatal("expected.json has no csharp-command.cs")
}

// The reservation used to be 64·n²·(n+source+text+1) work: cubic in the captures.
func TestOrdinaryBaseCSharpFactChargeGrowsNearLinearly(t *testing.T) {
	var prevWork, first, last uint64
	for sites := 8; sites <= 128; sites *= 2 {
		g := csharpFactSites(sites)
		terms, logical, err := csharpFactCharges(t, g.caps, g.src)
		if err != nil {
			t.Fatalf("%d sites (%d captures): %v", sites, len(g.caps), err)
		}
		work := terms[0] + terms[1] + terms[2]
		if prevWork != 0 && work > prevWork*5/2 {
			t.Fatalf("doubling to %d sites grew work %d -> %d (more than n log n)", sites, prevWork, work)
		}
		if logical > 1024*16*(uint64(len(g.caps))+1)+(1<<20) {
			t.Fatalf("%d sites: logical %d is not linear in %d captures", sites, logical, len(g.caps))
		}
		if first == 0 {
			first = work
		}
		prevWork, last = work, work
	}
	if last > first*26 { // 16x the captures: n log n is about 19x, a cubic charge 4096x
		t.Fatalf("16x the sites grew work %d -> %d", first, last)
	}
	// 128 sites is about 770 captures: within the 32M pass limit, where the old charge needed 1e11.
	if last > eligibilityWorkLimit/2 {
		t.Fatalf("work %d is not small for 128 sites", last)
	}
}

// The adapter itself is near-linear: its counted steps grow like n log n.
func TestCSharpDeriveStepsGrowNearLinearly(t *testing.T) {
	steps := func(sites int) uint64 {
		g := csharpFactSites(sites)
		_, p, h, d := csharpFactSteps(t, g.caps, g.src, EncodingUTF8)
		return p + h + d
	}
	small, large := steps(16), steps(256)
	if large > small*16*2 {
		t.Fatalf("16x the sites: steps %d -> %d", small, large)
	}
}

// The charge must cover the steps the adapter really takes, on random shapes and encodings.
func TestOrdinaryBaseCSharpFactChargeCoversRandomShapes(t *testing.T) {
	rng := rand.New(rand.NewPCG(438, 1))
	for round := 0; round < 60; round++ {
		g := &csharpFactGen{utf16: round%3 == 1}
		for i, sites := 0, 1+rng.IntN(40); i < sites; i++ {
			arg := fmt.Sprintf("q%d", rng.IntN(1000))
			if rng.IntN(3) == 0 {
				g.assignment(rng.IntN(2) == 0, arg)
			} else {
				g.command(rng.IntN(5), arg)
			}
		}
		rng.Shuffle(len(g.caps), func(i, j int) { g.caps[i], g.caps[j] = g.caps[j], g.caps[i] })
		enc := EncodingUTF8
		if g.utf16 {
			enc = EncodingUTF16LE
		}
		facts := assertCSharpFactChargeCovers(t, fmt.Sprintf("round %d", round), g.caps, g.src, enc)
		if len(facts.Items) != g.sites {
			t.Fatalf("round %d: %d facts for %d sites", round, len(facts.Items), g.sites)
		}
		n := uint64(len(g.caps)) + 1
		if terms, _, _ := csharpFactCharges(t, g.caps, g.src); terms[0] < 64*12*n*uint64(bits.Len64(n)) {
			t.Fatalf("round %d: sorted-pass term %d is below 12 passes over %d captures", round, terms[0], n)
		}
	}
}

// Crafted qualified-name captures can loop (a node that is its own last identifier, or two
// that name each other). The chain walk must end, leave the type unresolved and keep the
// other sites; its steps stay inside the chain term.
func TestCSharpDeriveSurvivesQualifiedNameCycles(t *testing.T) {
	for _, loop := range []string{"self", "pair"} {
		g := &csharpFactGen{}
		g.command(0, "before")
		oc := g.text("object_creation_expression", "new")
		a, b := g.text("qualified_name", "A"), g.text("qualified_name", "B")
		g.pair("qn", a, "qn.name", map[string]csharpFactNode{"self": a, "pair": b}[loop])
		if loop == "pair" {
			g.pair("aq", b, "aq.name", a)
		}
		g.pair("oc", oc, "oc.type", a)
		arg := g.text("argument", "(")
		g.pair("oc.a", oc, "oc.arg", arg)
		g.pair("arg", arg, "arg.expr", g.text("identifier", "cycle"))
		g.command(0, "after")
		facts := assertCSharpFactChargeCovers(t, loop, g.caps, g.src, EncodingUTF8)
		if len(facts.Items) != 2 || facts.Items[0].Variable == nil || *facts.Items[0].Variable != "before" || *facts.Items[1].Variable != "after" {
			t.Fatalf("%s: %+v", loop, facts.Items)
		}
	}
}
