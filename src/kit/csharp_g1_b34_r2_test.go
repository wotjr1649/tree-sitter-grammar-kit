package kit

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The post-seal corrections of the G1-3b expectations (expected r2), the type audit r2 and the native comparison of the corrected asset
// (executed r2) (#233). The r1 assets are frozen: these tests check r2 against them and never change them.

type g1b34r2Assets struct {
	src      CSharpG1B34R2Sources
	x        CSharpG1B34
	data     []byte
	typ2     []byte
	typ1     []byte
	pairs    []G1B34AnchorPair
	pairsR1  []G1B34AnchorPair
	err      error
	executed []byte
}

var g1b34r2Once sync.Once
var g1b34r2Loaded g1b34r2Assets

func g1b34r2Load(t *testing.T) *g1b34r2Assets {
	t.Helper()
	a := g1b34Load(t)
	g1b34r2Once.Do(func() {
		r := &g1b34r2Loaded
		read := func(name string) []byte {
			d, err := os.ReadFile(name)
			if err != nil && r.err == nil {
				r.err = err
			}
			return d
		}
		r.data = read("../contracts/csharp-g1-expected-b03b04-r2.json")
		r.typ2, r.typ1 = read("../contracts/csharp-g1-typeaudit-b03b04-r2.json"), a.typ
		r.executed = read("../contracts/csharp-g1-executed-b03b04-r2.json")
		r.src = CSharpG1B34R2Sources{R1: a.in, Previous: a.expected}
		if r.err != nil {
			return
		}
		if r.x, r.err = ParseCSharpG1B34R2(r.data, r.src); r.err != nil {
			return
		}
		if r.pairs, r.err = G1B34ContextView(r.data); r.err != nil {
			return
		}
		r.pairsR1, r.err = G1B34ContextView(a.expected)
	})
	if g1b34r2Loaded.err != nil {
		t.Fatal(g1b34r2Loaded.err)
	}
	return &g1b34r2Loaded
}

func g1b34r2Edit(t *testing.T, x CSharpG1B34, edit func(*CSharpG1B34)) []byte {
	t.Helper()
	y := g1b34Clone(x)
	y.Corrections = slices.Clone(x.Corrections)
	for i := range y.Corrections {
		y.Corrections[i].Grounds = slices.Clone(y.Corrections[i].Grounds)
	}
	ps := *x.PostSeal
	y.PostSeal = &ps
	edit(&y)
	d, err := MarshalCSharpG1B34(y)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// r2 is the sealed r1 asset with exactly the five post-seal corrections: the four anchors `var` get the types the grammar text gives an implicit
// type, and csharp-B03.a239 is BLOCKED with a registered code. Nothing else differs, and the counts keep the corrections apart.
func TestCSharpG1B34ExpectedR2IsTheCorrectedR1(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	x := r.x
	if x.Schema != CSharpG1B34SchemaR2 || x.Inputs.Previous.Path != CSharpG1B34Path || x.Inputs.Previous.SHA256 != "f37563567c1298900a41751bec3eff299e8c81bff25adc72352624c59d631bcc" {
		t.Errorf("schema or pin: %s %+v", x.Schema, x.Inputs.Previous)
	}
	if x.Counts != (G1B34Counts{Alternatives: 388, A: 198, B: 97, C: 93, Expected: 295, Blocked: 93, Strong: 180 - 1, Weak: 116, Witnesses: 295}) {
		t.Errorf("counts %+v", x.Counts)
	}
	if *x.PostSeal != (G1B34PostSeal{Corrections: 5, AfterNative: 5, BasisIndependent: 5, Alternatives: 5, AnchorTypes: 4, Blocked: 1}) {
		t.Errorf("post_seal %+v", *x.PostSeal)
	}
	var changed []string
	for i, alt := range x.Alternatives {
		if !g1b34Same(alt, a.want.Alternatives[i]) {
			changed = append(changed, alt.ID)
		}
	}
	if want := []string{"csharp-B03.a184", "csharp-B03.a236", "csharp-B03.a239", "csharp-B04.a028", "csharp-B04.a029"}; !slices.Equal(changed, want) {
		t.Fatalf("r2 differs from r1 in %v, want %v", changed, want)
	}
	for _, id := range []string{"csharp-B03.a184", "csharp-B03.a236", "csharp-B04.a028", "csharp-B04.a029"} {
		alt, old := g1b34Alt(x, id), g1b34Alt(a.want, id)
		// the types are the ones the grammar gives an implicit type; the discriminating flag and the witness are the sealed ones
		if !slices.Equal(alt.Anchors[0].TS, []string{`"var"`, "implicit_type"}) || alt.Discriminating != old.Discriminating || alt.Discriminating || !g1b34Same(alt.Witnesses, old.Witnesses) {
			t.Errorf("%s: %+v", id, alt)
		}
	}
	b := g1b34Alt(x, "csharp-B03.a239")
	if b.Status != G1ExpBlocked || b.Block != G1ClassBlockPosition || b.Group != G1ClassBlocked || len(b.Witnesses) != 0 || !slices.Contains(G1ClassBlockCodes, b.Block) {
		t.Errorf("a239: %+v", b)
	}
	// every correction is after_native and basis_independent and has its two kinds of ground
	for _, c := range x.Corrections {
		if !c.AfterNative || !c.BasisIndependent || c.Reason == "" {
			t.Errorf("%s: %+v", c.Item, c)
		}
	}
	if !g1b34Same(x.Corrections, g1B34Corrections) {
		t.Errorf("the corrections are not the authored ones")
	}
}

// r1 is sealed: it cannot be read as r2 or the other way round, and the r2 verifier checks the r1 asset it pins.
func TestCSharpG1B34ExpectedR2IsBoundToR1(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	if _, err := ParseCSharpG1B34(r.data, a.in); g1Code(err) != "G1_B34_SCHEMA" {
		t.Errorf("r2 as r1: %v", err)
	}
	if _, err := ParseCSharpG1B34R2(a.expected, r.src); g1Code(err) != "G1_B34_SCHEMA" {
		t.Errorf("r1 as r2: %v", err)
	}
	bad := r.src
	bad.Previous = append(slices.Clone(a.expected), ' ')
	if _, err := ParseCSharpG1B34R2(r.data, bad); err == nil {
		t.Errorf("r2 over a changed r1")
	}
	derived, err := DeriveCSharpG1B34R2(r.src)
	if err != nil {
		t.Fatal(err)
	}
	if d, err := MarshalCSharpG1B34(derived); err != nil || !bytes.Equal(d, r.data) {
		t.Errorf("the tracked r2 is not the derivation: %v", err)
	}
}

func TestCSharpG1B34ExpectedR2RejectsAlteredAssets(t *testing.T) {
	r := g1b34r2Load(t)
	cases := []struct {
		name string
		edit func(*CSharpG1B34)
		code string
	}{
		{"a correction removed", func(y *CSharpG1B34) { y.Corrections = y.Corrections[1:] }, "G1_CORRECTION_UNLISTED"},
		{"no correction", func(y *CSharpG1B34) { y.Corrections = nil }, "G1_CORRECTION_FORM"},
		{"after_native false", func(y *CSharpG1B34) { y.Corrections[0].AfterNative = false }, "G1_CORRECTION_FORM"},
		{"a new value edited", func(y *CSharpG1B34) { y.Corrections[0].New = `["\"var\""]` }, "G1_CORRECTION_UNLISTED"},
		{"an old value that r1 does not hold", func(y *CSharpG1B34) { y.Corrections[0].Old = `["identifier"]` }, "G1_CORRECTION_OLD"},
		{"a correction of an alternative that is not expected", func(y *CSharpG1B34) { y.Corrections[4].Item = "csharp-B03.a028" }, "G1_CORRECTION_ITEM"},
		{"a field that is no field", func(y *CSharpG1B34) { y.Corrections[0].Field = "anchors[0].text" }, "G1_CORRECTION_FORM"},
		{"an anchor that does not exist", func(y *CSharpG1B34) { y.Corrections[0].Field = "anchors[7].ts" }, "G1_CORRECTION_OLD"},
		{"a reason that is gone", func(y *CSharpG1B34) { y.Corrections[0].Reason = "" }, "G1_CORRECTION_FORM"},
		{"a ground without the grammar", func(y *CSharpG1B34) {
			y.Corrections[0].Grounds = slices.DeleteFunc(y.Corrections[0].Grounds, func(g G1Ground) bool { return g.Doc == G1DocGrammar })
		}, "G1_CORRECTION_GROUND"},
		{"a ground edited", func(y *CSharpG1B34) { y.Corrections[0].Grounds[0].Quote += " x" }, "G1_CORRECTION_UNAUTHORED"},
		{"a value changed without a correction", func(y *CSharpG1B34) { y.Alternatives[0].Spec += "." }, "G1_CORRECTION_UNLISTED"},
		{"the new alternative of a239 is not blocked", func(y *CSharpG1B34) {
			y.Corrections[4].New = strings.Replace(y.Corrections[4].New, `"block":"POSITION_DEPENDENT"`, `"block":"MADE_UP"`, 1)
		}, "G1_CORRECTION_OLD"},
		{"post_seal", func(y *CSharpG1B34) { y.PostSeal.Blocked = 0 }, "G1_CORRECTION_UNLISTED"},
		{"the pin of r1", func(y *CSharpG1B34) { y.Inputs.Previous.SHA256 = strings.Repeat("0", 64) }, "G1_CORRECTION_PIN"},
	}
	for _, c := range cases {
		data := g1b34r2Edit(t, r.x, c.edit)
		if _, err := ParseCSharpG1B34R2(data, r.src); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := ParseCSharpG1B34R2(append(slices.Clone(r.data), ' '), r.src); g1Code(err) != "G1_CORRECTION_BYTES" {
		t.Errorf("bytes")
	}
}

// ---- the type audit r2 ------------------------------------------------------------------------------------------

func TestCSharpG1TypeAuditB34R2Verifies(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	x, err := ParseCSharpG1TypeAuditB34R2(r.typ2, a.typ, r.pairs)
	if err != nil {
		t.Fatal(err)
	}
	// 55 anchors of the 27 lexemes whose type depends on the place, 31 places; none disjoint: 7 equal, 48 with types the grammar does not give there
	if x.Counts != (G1B34ContextCounts{Roles: 31, Contexts: 55, Match: 55, Equal: 7, Wider: 48, Findings: 4}) {
		t.Errorf("counts %+v", x.Counts)
	}
	if x.Previous.Path != CSharpG1TypeAuditB34Path || x.Previous.SHA256 != "d3a7b23b9f168547ee6722bdf98ec51f33784cf5c4c1e8169313f91eb92e5ceb" {
		t.Errorf("previous %+v", x.Previous)
	}
}

// The audit r1 compares an anchor lexeme with the union of the types over the alternatives, so it does not see the four `var` anchors of
// the sealed asset (it verifies the sealed asset); the audit r2 compares each anchor with the types of its place and finds exactly them.
// After the correction the audit r2 has no disagreement.
func TestCSharpG1TypeAuditB34R2FindsWhatR1Missed(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	r1view, err := G1B34TypeView(a.expected)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1TypeAuditB34(a.typ, r1view); err != nil {
		t.Fatalf("the audit r1 does not accept the sealed asset: %v", err)
	}
	// the type audit r1 row of var is the candidate set the union of all alternatives fills, and the four anchors are in it
	r1, err := DecodeCSharpG1TypeAuditB34(a.typ)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(r1.Anchors, func(x G1B34AnchorTypeRow) bool { return x.Text == "var" })
	if i < 0 || r1.Anchors[i].Class != G1TypeNotDerivable || r1.Anchors[i].Result != G1AuditMatch || r1.Anchors[i].Relation != G1TypeRelCandidates {
		t.Fatalf("the r1 row of var: %+v", r1.Anchors[i])
	}
	x2, err := DecodeCSharpG1TypeAuditB34R2(r.typ2)
	if err != nil {
		t.Fatal(err)
	}
	onR1, err := DeriveCSharpG1TypeAuditB34R2(x2.Roles, x2.Contexts, a.typ, r.pairsR1)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, c := range onR1.Contexts {
		if c.Result == G1AuditMismatch {
			found = append(found, c.Alt)
			if c.Relation != g1b34RelDisjoint || c.Text != "var" {
				t.Errorf("%s: %+v", c.Alt, c)
			}
		}
	}
	if want := []string{"csharp-B03.a184", "csharp-B03.a236", "csharp-B04.a028", "csharp-B04.a029"}; !slices.Equal(found, want) {
		t.Errorf("the audit r2 on the sealed asset finds %v, want %v", found, want)
	}
	onR2, err := DeriveCSharpG1TypeAuditB34R2(x2.Roles, x2.Contexts, a.typ, r.pairs)
	if err != nil || onR2.Counts.Mismatch != 0 {
		t.Errorf("the audit r2 on the corrected asset: %+v %v", onR2.Counts, err)
	}
	// the verifier does not accept the asset r2 against the sealed r1 anchors (its stored results are those of the corrected asset)
	if _, err := ParseCSharpG1TypeAuditB34R2(r.typ2, a.typ, r.pairsR1); g1Code(err) != "G1_TYPEAUDITB34R2_COMPARISON" {
		t.Errorf("the verifier accepts r2 over the sealed anchors: %v", err)
	}
	// the corrected types of the four anchors are the types of the place they are in
	for _, c := range x2.Contexts {
		if c.Text == "var" && c.Role == "implicit type" && !slices.Equal(c.Types, []string{`"var"`, "implicit_type"}) {
			t.Errorf("%+v", c)
		}
	}
}

// Re-deriving the comparison cannot authorize a different grammar type from the same candidate set.
func TestCSharpG1TypeAuditB34R2RejectsRederivedRoleTypes(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	for _, tc := range []struct {
		name, lexeme, role string
		types              []string
	}{
		{"async alias replaced by keyword", "async", "modifier of an anonymous function", []string{`"async"`}},
		{"var widened to hide r1 mismatches", "var", "implicit type", []string{`"var"`, "identifier", "implicit_type"}},
	} {
		for _, input := range []struct {
			name  string
			pairs []G1B34AnchorPair
		}{{"r1", r.pairsR1}, {"r2", r.pairs}} {
			t.Run(tc.name+"/"+input.name, func(t *testing.T) {
				x, err := DecodeCSharpG1TypeAuditB34R2(r.typ2)
				if err != nil {
					t.Fatal(err)
				}
				i := slices.IndexFunc(x.Roles, func(role G1B34Role) bool { return role.Lexeme == tc.lexeme && role.Role == tc.role })
				if i < 0 {
					t.Fatal("the authored role is missing")
				}
				x.Roles[i].Types = slices.Clone(tc.types)
				forged, err := DeriveCSharpG1TypeAuditB34R2(x.Roles, x.Contexts, a.typ, input.pairs)
				if err != nil {
					t.Fatal(err)
				}
				if tc.lexeme == "async" && input.name == "r2" && forged.Counts != x.Counts {
					t.Fatal("the async mutation must preserve every headline count")
				}
				if tc.lexeme == "var" && forged.Counts.Mismatch != 0 {
					t.Fatal("the widened var role must hide the four r1 mismatches")
				}
				data, err := MarshalCSharpG1TypeAuditB34R2(forged)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ParseCSharpG1TypeAuditB34R2(data, a.typ, input.pairs); g1Code(err) != "G1_TYPEAUDITB34R2_GROUND" {
					t.Fatalf("re-derived role types: %v, want G1_TYPEAUDITB34R2_GROUND", err)
				}
			})
		}
	}
}

func TestCSharpG1TypeAuditB34R2Compare(t *testing.T) {
	for _, c := range []struct {
		role, ts      []string
		result, relat string
	}{
		{[]string{"a"}, []string{"a"}, G1AuditMatch, g1b34RelEqual},
		{[]string{"a"}, []string{"a", "b"}, G1AuditMatch, g1b34RelWider},
		{[]string{"a", "b"}, []string{"a"}, G1AuditMatch, g1b34RelNarrower},
		{[]string{"a", "b"}, []string{"b", "c"}, G1AuditMatch, g1b34RelOverlap},
		{[]string{"a", "b"}, []string{"c"}, G1AuditMismatch, g1b34RelDisjoint},
		{[]string{"a"}, nil, G1AuditMismatch, g1b34RelDisjoint},
	} {
		if res, rel := g1b34ContextCompare(c.role, c.ts); res != c.result || rel != c.relat {
			t.Errorf("%v vs %v: %s %s", c.role, c.ts, res, rel)
		}
	}
}

func TestCSharpG1TypeAuditB34R2RejectsAlteredAudits(t *testing.T) {
	a, r := g1b34Load(t), g1b34r2Load(t)
	x0, err := DecodeCSharpG1TypeAuditB34R2(r.typ2)
	if err != nil {
		t.Fatal(err)
	}
	clone := func() CSharpG1TypeAuditB34R2 {
		x := x0
		x.Roles, x.Contexts, x.Findings = slices.Clone(x0.Roles), slices.Clone(x0.Contexts), slices.Clone(x0.Findings)
		return x
	}
	roleOf := func(x *CSharpG1TypeAuditB34R2, lexeme, role string) *G1B34Role {
		i := slices.IndexFunc(x.Roles, func(r G1B34Role) bool { return r.Lexeme == lexeme && r.Role == role })
		return &x.Roles[i]
	}
	cases := []struct {
		name string
		edit func(*CSharpG1TypeAuditB34R2)
		code string
	}{
		{"a context left out", func(x *CSharpG1TypeAuditB34R2) {
			i := slices.IndexFunc(x.Contexts, func(c G1B34Context) bool { return c.Text == "new" })
			x.Contexts = slices.Delete(x.Contexts, i, i+1)
		}, "G1_TYPEAUDITB34R2_ROWS"},
		{"a context of another anchor", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Index = 5 }, "G1_TYPEAUDITB34R2_GROUND"},
		{"a role that the lexeme does not have", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Role = "fixed statement" }, "G1_TYPEAUDITB34R2_ROW"},
		{"a role assignment changed", func(x *CSharpG1TypeAuditB34R2) {
			for i := range x.Contexts {
				if x.Contexts[i].Alt == "csharp-B04.a028" {
					x.Contexts[i].Role = "var pattern"
				}
			}
		}, "G1_TYPEAUDITB34R2_GROUND"},
		{"a place edited", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Place += "." }, "G1_TYPEAUDITB34R2_GROUND"},
		{"a result", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Result = G1AuditMismatch }, "G1_TYPEAUDITB34R2_COMPARISON"},
		{"a relation", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Relation = g1b34RelDisjoint }, "G1_TYPEAUDITB34R2_COMPARISON"},
		{"types of a context", func(x *CSharpG1TypeAuditB34R2) { x.Contexts[0].Types = []string{"identifier"} }, "G1_TYPEAUDITB34R2_COMPARISON"},
		{"types of a role outside the candidate set", func(x *CSharpG1TypeAuditB34R2) { roleOf(x, "var", "implicit type").Types = []string{`"var"`, "zzz"} }, "G1_TYPEAUDITB34R2_ROLE"},
		{"unsorted types of a role", func(x *CSharpG1TypeAuditB34R2) {
			roleOf(x, "var", "implicit type").Types = []string{"implicit_type", `"var"`}
		}, "G1_TYPEAUDITB34R2_ROLE"},
		{"duplicate types of a role", func(x *CSharpG1TypeAuditB34R2) {
			roleOf(x, "var", "implicit type").Types = []string{`"var"`, `"var"`, "implicit_type"}
		}, "G1_TYPEAUDITB34R2_ROLE"},
		{"a reason that is gone", func(x *CSharpG1TypeAuditB34R2) { roleOf(x, "var", "implicit type").Reason = "" }, "G1_TYPEAUDITB34R2_ROLE"},
		{"a ground edited", func(x *CSharpG1TypeAuditB34R2) {
			g := slices.Clone(roleOf(x, "var", "implicit type").Grounds)
			g[0].Quote += " x"
			roleOf(x, "var", "implicit type").Grounds = g
		}, "G1_TYPEAUDITB34R2_GROUND"},
		{"a ground of the specification", func(x *CSharpG1TypeAuditB34R2) {
			g := slices.Clone(roleOf(x, "var", "implicit type").Grounds)
			g[0].Doc = G1DocSpec
			roleOf(x, "var", "implicit type").Grounds = g
		}, "G1_TYPEAUDITB34R2_ROLE"},
		{"a role of a lexeme that is derivable", func(x *CSharpG1TypeAuditB34R2) { roleOf(x, "var", "var pattern").Lexeme = "while" }, "G1_TYPEAUDITB34R2_ROLE"},
		{"every context of a role left out: the role is unused", func(x *CSharpG1TypeAuditB34R2) {
			x.Contexts = slices.DeleteFunc(x.Contexts, func(c G1B34Context) bool { return c.Text == "fixed" })
		}, "G1_TYPEAUDITB34R2_ROLE"},
		{"a role that no context uses", func(x *CSharpG1TypeAuditB34R2) {
			x.Roles = append(x.Roles, G1B34Role{Lexeme: "yield", Role: "zzz", Types: []string{`"yield"`}, Reason: "x", Grounds: roleOf(x, "yield", "yield_statement").Grounds})
		}, "G1_TYPEAUDITB34R2_GROUND"},
		{"roles out of order", func(x *CSharpG1TypeAuditB34R2) { x.Roles[0], x.Roles[1] = x.Roles[1], x.Roles[0] }, "G1_TYPEAUDITB34R2_ROLE"},
		{"a finding changed", func(x *CSharpG1TypeAuditB34R2) { x.Findings[0].Fix = "none" }, "G1_TYPEAUDITB34R2_FINDINGS"},
		{"the counts", func(x *CSharpG1TypeAuditB34R2) { x.Counts.Match-- }, "G1_TYPEAUDITB34R2_COUNTS"},
		{"the pin of r1", func(x *CSharpG1TypeAuditB34R2) { x.Previous.SHA256 = strings.Repeat("0", 64) }, "G1_TYPEAUDITB34R2_PIN"},
	}
	for _, c := range cases {
		x := clone()
		c.edit(&x)
		data, err := MarshalCSharpG1TypeAuditB34R2(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1TypeAuditB34R2(data, a.typ, r.pairs); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := ParseCSharpG1TypeAuditB34R2(append(slices.Clone(r.typ2), ' '), a.typ, r.pairs); g1Code(err) != "G1_TYPEAUDITB34R2_BYTES" {
		t.Errorf("bytes")
	}
	if _, err := ParseCSharpG1TypeAuditB34R2(r.typ2, append(slices.Clone(a.typ), ' '), r.pairs); g1Code(err) != "G1_TYPEAUDITB34R2_PIN" {
		t.Errorf("a changed r1 audit file")
	}
	if _, err := ParseCSharpG1TypeAuditB34R2(bytes.Replace(r.typ2, []byte(CSharpG1TypeAuditB34SchemaR2), []byte(CSharpG1TypeAuditB34Schema), 1), a.typ, r.pairs); g1Code(err) != "G1_TYPEAUDITB34R2_SCHEMA" {
		t.Errorf("schema")
	}
}

// The view of the audit r2 holds anchors only (TestCSharpG1AuditInputsAreNarrow pins its members): the pairs are the anchors of the EXPECTED A alternatives of r2.
func TestCSharpG1TypeAuditB34R2InputIsNarrow(t *testing.T) {
	r := g1b34r2Load(t)
	// 198 A alternatives, 295 expected: the pairs are the anchors of the EXPECTED A alternatives of r2
	n := 0
	for _, a := range r.x.Alternatives {
		if a.Status == G1ExpExpected && a.Group == G1ClassLiteral {
			n += len(a.Anchors)
		}
	}
	if n != len(r.pairs) {
		t.Errorf("%d anchors, %d pairs", n, len(r.pairs))
	}
}

// ---- the executed asset r2 --------------------------------------------------------------------------------------

type g1bxR2Assets struct {
	asset []byte
	in    CSharpG1B34ExecutedSources
	x     CSharpG1B34Executed
	ctx   g1bxContext
	err   error
}

var g1bxR2Once sync.Once
var g1bxR2Loaded g1bxR2Assets

func g1bxR2Load(t *testing.T) *g1bxR2Assets {
	t.Helper()
	r := g1b34r2Load(t)
	g1bxR2Once.Do(func() {
		a := &g1bxR2Loaded
		a.asset = r.executed
		a.in = CSharpG1B34ExecutedSources{Src: g1Load(t).src, Expected: r.data, Observation: mustRead(t, "../contracts/csharp-g1-roslyn-observed-r3.json"), R2: true}
		if a.x, a.err = DecodeCSharpG1B34Executed(a.asset); a.err != nil {
			return
		}
		a.ctx, a.err = g1bxContextOf(a.in)
	})
	if g1bxR2Loaded.err != nil {
		t.Fatal(g1bxR2Loaded.err)
	}
	return &g1bxR2Loaded
}

// The comparison of the corrected asset with the same raw native observation: 295 expected alternatives, all MATCH, the four corrected ones
// counted apart, and no mismatch is left to dispose of.
func TestCSharpG1B34ExecutedR2(t *testing.T) {
	a := g1bxR2Load(t)
	if err := g1bxCheck(a.asset, a.x, a.ctx, a.in); err != nil {
		t.Fatalf("the tracked asset does not verify offline: %v", err)
	}
	if a.x.Schema != CSharpG1B34ExecutedSchemaR2 || a.x.Inputs.Expected.Path != CSharpG1B34PathR2 {
		t.Errorf("schema or pin: %s %+v", a.x.Schema, a.x.Inputs.Expected)
	}
	old := g1bxLoad(t)
	if a.x.Inputs.Raw != old.x.Inputs.Raw || !g1b34Same(a.x.Native, old.x.Native) || !g1b34Same(a.x.Sources, old.x.Sources) {
		t.Errorf("r2 is not a comparison with the raw observation of r1")
	}
	want := G1B34ExecCounts{Alternatives: 295, Executed: 295, Match: 295, MatchStrongA: 62, MatchCandidateA: 19, MatchOperatorA: 27, MatchWeakA: 90, MatchStrongB: 71, MatchWeakB: 26,
		Witnesses: 295, Parts: 424, PartMatch: 424, Sources: 23, SourcesCompleted: 23, Corrected: 4, CorrectedMatch: 4, SealedMatch: 291}
	if a.x.Counts != want {
		t.Errorf("counts %+v, want %+v", a.x.Counts, want)
	}
	if len(a.x.Dispositions) != 0 {
		t.Errorf("dispositions %v", a.x.Dispositions)
	}
	// the sealed alternatives a correction did not touch are compared with their sealed values and have the result r1 has
	for _, alt := range a.x.Alternatives {
		o := g1bxAlt(&old.x, alt.ID)
		if alt.Corrected {
			if o == nil || o.Result != G1ResMismatch {
				t.Errorf("%s: corrected, r1 result %v", alt.ID, o)
			}
			continue
		}
		if o == nil || o.Result != G1ResMatch || !g1b34Same(alt.Witnesses, o.Witnesses) {
			t.Errorf("%s: r1 %v", alt.ID, o)
		}
	}
	if g1bxAlt(&a.x, "csharp-B03.a239") != nil {
		t.Errorf("a239 is BLOCKED in r2: it has no comparison")
	}
}

// r2 is bound to the expected asset r2: it does not verify against the sealed asset, and the executed asset r1 does not verify as r2.
func TestCSharpG1B34ExecutedR2IsBound(t *testing.T) {
	a, r := g1bxR2Load(t), g1b34r2Load(t)
	old := g1bxLoad(t)
	if _, err := g1bxContextOf(CSharpG1B34ExecutedSources{Src: a.in.Src, Expected: old.in.Expected, Observation: a.in.Observation, R2: true}); g1Code(err) != "G1_B34X_PIN" {
		t.Errorf("r2 over the sealed asset: %v", err)
	}
	if _, err := g1bxContextOf(CSharpG1B34ExecutedSources{Src: a.in.Src, Expected: r.data, Observation: a.in.Observation}); g1Code(err) != "G1_B34X_PIN" {
		t.Errorf("r1 over the corrected asset: %v", err)
	}
	if err := g1bxCheck(old.asset, old.x, a.ctx, a.in); g1Code(err) != "G1_B34X_SCHEMA" {
		t.Errorf("the executed asset r1 verifies as r2: %v", err)
	}
	// the authored dispositions are those of the sealed comparison: r2 has no mismatch and so no disposition to take
	if n, m := len(a.ctx.dispositions()), len(old.ctx.dispositions()); n != 0 || m != 5 {
		t.Errorf("%d dispositions for r2, %d for r1", n, m)
	}
	// an alternative marked corrected that no correction names, one that a correction names but is not marked, a mismatch with no disposition
	plain := ""
	for _, alt := range a.x.Alternatives {
		if !alt.Corrected {
			plain = alt.ID
			break
		}
	}
	for name, edit := range map[string]func(*CSharpG1B34Executed){
		"a corrected flag removed": func(y *CSharpG1B34Executed) { g1bxAlt(y, "csharp-B04.a028").Corrected = false },
		"a corrected flag added":   func(y *CSharpG1B34Executed) { g1bxAlt(y, plain).Corrected = true },
		"a mismatch":               func(y *CSharpG1B34Executed) { g1bxAlt(y, plain).Result = G1ResMismatch },
		"counts":                   func(y *CSharpG1B34Executed) { y.Counts.CorrectedMatch-- },
	} {
		y := g1bxClone(t, a.x)
		edit(&y)
		data, err := MarshalCSharpG1B34Executed(y)
		if err != nil {
			t.Fatal(err)
		}
		x, err := DecodeCSharpG1B34Executed(data)
		if err == nil {
			err = g1bxCheck(data, x, a.ctx, a.in)
		}
		if err == nil {
			t.Errorf("%s is accepted", name)
		}
	}
}

// With the stored raw observation (env TSGK_G1_NATIVE_RAW_B34) the tracked executed asset r2 is exactly what the comparison of the corrected
// expected asset with that observation derives, and the observation is the one r1 compared (no native run was repeated).
func TestCSharpG1B34ExecutedR2MatchesRawNative(t *testing.T) {
	path := os.Getenv("TSGK_G1_NATIVE_RAW_B34")
	if path == "" {
		t.Skip("TSGK_G1_NATIVE_RAW_B34 (the raw observation of csharp-g1-native -set b34) is not configured")
	}
	a := g1bxR2Load(t)
	in := a.in
	var err error
	if in.Raw, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1B34Executed(a.asset, in); err != nil {
		t.Fatal(err)
	}
	if a.x.Inputs.Raw != g1bxLoad(t).x.Inputs.Raw {
		t.Errorf("the raw pin %+v is not the one of r1", a.x.Inputs.Raw)
	}
}

// With the pinned documents every ground of the post-seal corrections and of the roles of the type audit r2 is a quote of a document at its place.
func TestCSharpG1B34R2GroundsAgainstTheDocuments(t *testing.T) {
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
	if err := CheckG1Grounds(G1B34Corrections(), string(g), spec); err != nil {
		t.Errorf("corrections: %v", err)
	}
	x, err := DecodeCSharpG1TypeAuditB34R2(g1b34r2Load(t).typ2)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckG1TypeAuditB34R2Grounds(x, string(g)); err != nil {
		t.Errorf("type audit r2: %v", err)
	}
}
