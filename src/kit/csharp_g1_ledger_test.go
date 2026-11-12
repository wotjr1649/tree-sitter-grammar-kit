package kit

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

type g1Assets struct {
	asset []byte
	src   CSharpG1Sources
	want  CSharpG1Ledger
	err   error
}

var g1Once sync.Once
var g1Loaded g1Assets

// g1Load reads the tracked asset and its pinned sources and derives the ledger once.
func g1Load(t *testing.T) g1Assets {
	t.Helper()
	g1Once.Do(func() {
		read := func(name string) []byte {
			data, err := os.ReadFile(name)
			if err != nil && g1Loaded.err == nil {
				g1Loaded.err = err
			}
			return data
		}
		a := &g1Loaded
		a.asset = read("../contracts/csharp-g1-ledger-r1.json")
		a.src = CSharpG1Sources{
			Inventory:    read("../contracts/qualification-c1.json"),
			Alternatives: read("../contracts/feature-alternatives.json"),
			FactMapping:  read("../contracts/fact-mapping.json"),
			Fixtures:     map[string][]byte{},
			Expected:     read("../contracts/csharp-g1-expected-b01b02-r1.json"),
		}
		for _, dir := range csharpG1Fixtures {
			a.src.Fixtures[dir] = read("../testdata/native/" + dir + "/csharp.json")
		}
		if a.err == nil {
			a.want, a.err = DeriveCSharpG1Ledger(a.src)
		}
	})
	if g1Loaded.err != nil {
		t.Fatal(g1Loaded.err)
	}
	return g1Loaded
}

func g1Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestCSharpG1LedgerSealedAndBound(t *testing.T) {
	a := g1Load(t)
	if len(a.asset) != 711266 || digestHex(a.asset) != "42863f7c74ab81b09183c42759b4c929c2dd5604473b0d8f17dbb7bbad7a27d1" {
		t.Fatalf("sealed ledger changed: %d/%s", len(a.asset), digestHex(a.asset))
	}
	x, err := ParseCSharpG1Ledger(a.asset, a.src)
	if err != nil {
		t.Fatal(err)
	}
	c := x.Counts
	if c.Rows != 15 || c.Obligations != 65 || c.Alternatives != 1531 || c.Production != 1425 || c.Fact != 106 || c.Cases != 389 || c.AlternativeLink != 2937 || c.NoCase != 0 {
		t.Fatalf("counts %+v", c)
	}
	if c.LiteralMatch+c.LiteralAbsent+c.Unverified != c.Alternatives {
		t.Fatalf("presence does not partition the alternatives: %+v", c)
	}
	perRow := map[string]int{"csharp-B01": 407, "csharp-B02": 98, "csharp-B03": 257, "csharp-B04": 131, "csharp-B05": 356, "csharp-V73": 10, "csharp-V08": 57, "csharp-V09": 47, "csharp-V10": 27, "csharp-V11": 59, "csharp-V12": 18, "csharp-V13": 11, "csharp-V14a": 19, "csharp-V14b": 25, "csharp-V14c": 9}
	got := map[string]int{}
	for _, r := range x.Rows {
		got[r.Row] = r.Alternatives
		for _, o := range r.Obligations {
			// no row has every alternative sealed (blocked alternatives stay NO_EXPECTED), so no obligation is SEALED
			if o.Status != G1NoExpected || len(o.Links) == 0 {
				t.Errorf("%s %s: status %q links %d", r.Row, o.Kind, o.Status, len(o.Links))
			}
		}
	}
	if len(got) != len(perRow) {
		t.Fatalf("rows %v", got)
	}
	for row, n := range perRow {
		if got[row] != n {
			t.Errorf("%s: %d alternatives, want %d", row, got[row], n)
		}
	}
	// the alternatives are exactly the inventory's rows' lists, in order, each with a case and no expected value
	inv, err := ParseQualificationInventory(a.src.Inventory)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, r := range inv.Routes {
		if r.Route == "csharp" {
			for _, q := range r.Requirements {
				want = append(want, q.Alternatives...)
			}
		}
	}
	// SEALED is exactly the set the expected asset holds EXPECTED (G1-3a); the asset is verified in its own test
	var expDoc struct {
		Alternatives []struct {
			ID, Status string
		}
	}
	if err := json.Unmarshal(a.src.Expected, &expDoc); err != nil {
		t.Fatal(err)
	}
	sealed := map[string]bool{}
	for _, e := range expDoc.Alternatives {
		if e.Status == G1ExpExpected {
			sealed[e.ID] = true
		}
	}
	if len(sealed) != 426 {
		t.Fatalf("sealed alternatives %d", len(sealed))
	}
	var ids []string
	cases := map[string]bool{}
	for _, c := range x.Cases {
		cases[c.ID] = true
	}
	for _, alt := range x.Alternatives {
		ids = append(ids, alt.ID)
		wantStatus := G1NoExpected
		if sealed[alt.ID] {
			wantStatus = G1Sealed
		}
		if alt.Status != wantStatus || len(alt.Links) == 0 {
			t.Errorf("%s: status %q want %q links %d", alt.ID, alt.Status, wantStatus, len(alt.Links))
		}
		for _, l := range alt.Links {
			if !cases[l.Case] || len(l.Steps) == 0 {
				t.Errorf("%s: link %+v", alt.ID, l)
			}
			for _, m := range l.Matched {
				if !slices.Contains(l.Steps, m) {
					t.Errorf("%s: matched step %d is not a candidate step", alt.ID, m)
				}
			}
		}
	}
	if !slices.Equal(ids, want) {
		t.Fatal("ledger alternatives differ from the inventory's rows")
	}
	// the 20 declaration nodes and every out-of-mapping construct are in the declared scope
	if len(x.Declarations) != 20 || len(x.OutsideMapping) != len(csharpG1Constructs) {
		t.Fatalf("declarations %d outside %d", len(x.Declarations), len(x.OutsideMapping))
	}
	for _, o := range x.OutsideMapping {
		if o.Scope != "IN_DECLARED_SCOPE" || len(o.Alternatives) == 0 || o.Status != G1NoExpected {
			t.Errorf("%s: %s with %d alternatives", o.Construct, o.Scope, len(o.Alternatives))
		}
	}
	if len(x.ExtraRoles) != 3 {
		t.Fatalf("extra roles %+v", x.ExtraRoles)
	}
}

// A matched alternative's evidence names a source offset: its literals sit there in the registered
// case source (checked against the fixture, not through the derivation's own matcher).
func TestCSharpG1LedgerEvidenceOffsets(t *testing.T) {
	a := g1Load(t)
	var f struct {
		Cases []struct {
			ID     string `json:"id"`
			Source string `json:"source_utf8"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(a.src.Fixtures["routes"], &f); err != nil {
		t.Fatal(err)
	}
	src := map[string]string{}
	for _, c := range f.Cases {
		src[c.ID] = c.Source
	}
	checked := 0
	for _, alt := range a.want.Alternatives {
		e := alt.Presence.Evidence
		if (alt.Presence.Status == G1PresenceMatch) != (e != nil) {
			t.Fatalf("%s: status %s with evidence %v", alt.ID, alt.Presence.Status, e)
		}
		s, ok := src[alt.Presence.Evidence.caseOrEmpty()]
		if e == nil || e.Step != 0 || !ok || len(e.Offsets) != len(alt.Presence.Literals) {
			continue
		}
		for i, off := range e.Offsets {
			lit := alt.Presence.Literals[i]
			if int(off)+len(lit) > len(s) || s[off:int(off)+len(lit)] != lit {
				t.Errorf("%s: literal %q is not at byte %d", alt.ID, lit, off)
			}
		}
		checked++
	}
	if checked < 300 {
		t.Fatalf("only %d evidence entries checked", checked)
	}
}

func (e *G1Evidence) caseOrEmpty() string {
	if e == nil {
		return ""
	}
	return e.Case
}

func TestG1RequiredLiterals(t *testing.T) {
	for _, tc := range []struct {
		rhs    string
		lits   []string
		reason string
	}{
		{`'default'`, []string{"default"}, ""},
		{`'[' expression ']'`, []string{"[", "]"}, ""},
		{`'using' 'unsafe'? identifier '=' type ';'`, []string{"using", "=", ";"}, ""},
		{`'a' ('b' | 'c')? 'd'`, []string{"a", "d"}, ""},
		{`'\'' Character '\''`, []string{"'", "'"}, ""},
		{`'\u000D\u000A'`, []string{"\r\n"}, ""},
		{`'x'*  'y'`, []string{"y"}, ""},
		{`expression`, nil, "NO_REQUIRED_LITERAL"},
		{`'a' | 'b'`, nil, "TOP_LEVEL_ALTERNATION"},
		{`'a'..'z'`, nil, "CHARACTER_CLASS_OR_RANGE"},
		{`~['"']`, nil, "CHARACTER_CLASS_OR_RANGE"},
		{`'open`, nil, "UNTERMINATED_LITERAL"},
	} {
		lits, reason := g1RequiredLiterals(tc.rhs)
		if !slices.Equal(lits, tc.lits) || reason != tc.reason {
			t.Errorf("%s: %q %q, want %q %q", tc.rhs, lits, reason, tc.lits, tc.reason)
		}
	}
	head, rhs, ok := g1SplitProduction("fragment Integer_Type_Suffix: 'L'  [alt 3/12]")
	if !ok || head != "Integer_Type_Suffix" || rhs != "'L'" {
		t.Errorf("split %q %q %v", head, rhs, ok)
	}
}

func TestG1Subsequence(t *testing.T) {
	text := []byte("int x = default(T); classy class C { }")
	for _, tc := range []struct {
		lits    []string
		bounded bool
		want    []uint32
	}{
		{[]string{"default"}, true, []uint32{8}},
		{[]string{"class", "{"}, true, []uint32{27, 35}}, // "classy" is not the word class
		{[]string{"class"}, false, []uint32{20}},
		{[]string{"{", "class"}, true, nil}, // order matters
		{[]string{"missing"}, true, nil},
		{[]string{"x", "=", ";"}, true, []uint32{4, 6, 18}},
	} {
		if got := g1Subsequence(text, tc.lits, tc.bounded); !slices.Equal(got, tc.want) {
			t.Errorf("%q bounded=%v: %v, want %v", tc.lits, tc.bounded, got, tc.want)
		}
	}
}

// Every well-formed but wrong ledger is rejected with the code of what is wrong.
func TestCSharpG1LedgerForgeries(t *testing.T) {
	a := g1Load(t)
	clone := func() CSharpG1Ledger {
		var c CSharpG1Ledger
		data, _ := json.Marshal(a.want)
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	type forge struct {
		name string
		code string
		do   func(*CSharpG1Ledger)
	}
	last := func(l *CSharpG1Ledger) *G1Alternative { return &l.Alternatives[len(l.Alternatives)-1] }
	for _, f := range []forge{
		{"missing alternative", "G1_LEDGER_ALTERNATIVE_MISSING", func(l *CSharpG1Ledger) { l.Alternatives = l.Alternatives[1:] }},
		{"last alternative dropped", "G1_LEDGER_ALTERNATIVE_MISSING", func(l *CSharpG1Ledger) { l.Alternatives = l.Alternatives[:len(l.Alternatives)-1] }},
		{"duplicate alternative", "G1_LEDGER_ALTERNATIVE_DUPLICATE", func(l *CSharpG1Ledger) { l.Alternatives[5] = l.Alternatives[4] }},
		{"extra alternative", "G1_LEDGER_ALTERNATIVE_EXTRA", func(l *CSharpG1Ledger) {
			l.Alternatives = append(l.Alternatives, G1Alternative{ID: "csharp-B01.a999", Row: "csharp-B01", Kind: "production", Status: G1NoExpected})
		}},
		{"reordered alternatives", "G1_LEDGER_ALTERNATIVE_ORDER", func(l *CSharpG1Ledger) { l.Alternatives[0], l.Alternatives[1] = l.Alternatives[1], l.Alternatives[0] }},
		{"alternative link dropped", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { last(l).Links = []G1Link{} }},
		{"alternative step added", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) {
			l.Alternatives[2].Links[0].Steps = append(l.Alternatives[2].Links[0].Steps, 99)
		}},
		{"presence promoted", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { last(l).Presence.Status = G1PresenceMatch }},
		{"literal changed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[0].Presence.Literals = []string{"nothing"} }},
		{"alternative row changed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[0].Row = "csharp-B02" }},
		{"alternative sealed without an expectation", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[0].Status = G1Sealed }},
		{"sealed alternative unsealed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[1].Status = G1NoExpected }},
		{"alternative executed", "G1_LEDGER_STATUS", func(l *CSharpG1Ledger) { l.Alternatives[1].Status = G1Executed }},
		{"obligation executed", "G1_LEDGER_STATUS", func(l *CSharpG1Ledger) { l.Rows[0].Obligations[0].Status = G1Executed }},
		{"declaration sealed", "G1_LEDGER_DECLARATIONS", func(l *CSharpG1Ledger) { l.Declarations[0].Status = G1Sealed }},
		{"role status", "G1_LEDGER_STATUS", func(l *CSharpG1Ledger) { l.ExtraRoles[0].Status = "PASS" }},
		{"row dropped", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows = l.Rows[1:] }},
		{"kind dropped", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows[0].Obligations = l.Rows[0].Obligations[1:] }},
		{"row denominator shrunk", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows[0].Alternatives-- }},
		{"obligation link dropped", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows[1].Obligations[0].Links = l.Rows[1].Obligations[0].Links[1:] }},
		{"case step types", "G1_LEDGER_CASES", func(l *CSharpG1Ledger) { l.Cases[0].Steps[0].Types = append(l.Cases[0].Steps[0].Types, "x") }},
		{"case dropped", "G1_LEDGER_CASES", func(l *CSharpG1Ledger) { l.Cases = l.Cases[1:] }},
		{"declaration dropped", "G1_LEDGER_DECLARATIONS", func(l *CSharpG1Ledger) { l.Declarations = l.Declarations[1:] }},
		{"construct out of scope", "G1_LEDGER_OUTSIDE_MAPPING", func(l *CSharpG1Ledger) { l.OutsideMapping[0].Scope = "NOT_IN_DECLARED_SCOPE" }},
		{"construct dropped", "G1_LEDGER_OUTSIDE_MAPPING", func(l *CSharpG1Ledger) { l.OutsideMapping = l.OutsideMapping[1:] }},
		{"role dropped", "G1_LEDGER_ROLES", func(l *CSharpG1Ledger) { l.ExtraRoles = l.ExtraRoles[1:] }},
		{"counts", "G1_LEDGER_COUNTS", func(l *CSharpG1Ledger) { l.Counts.Alternatives-- }},
		{"pin sha", "G1_LEDGER_PIN", func(l *CSharpG1Ledger) { l.Inputs.Alternatives.SHA256 = strings.Repeat("0", 64) }},
		{"pin bytes", "G1_LEDGER_PIN", func(l *CSharpG1Ledger) { l.Inputs.Inventory.Bytes++ }},
		{"pin path", "G1_LEDGER_PIN", func(l *CSharpG1Ledger) { l.Inputs.FactMapping.Path = "x" }},
		{"fixture pin dropped", "G1_LEDGER_PIN", func(l *CSharpG1Ledger) { l.Inputs.Fixtures = l.Inputs.Fixtures[1:] }},
		{"schema", "G1_LEDGER_SCHEMA", func(l *CSharpG1Ledger) { l.Schema = "x" }},
	} {
		l := clone()
		f.do(&l)
		data, err := MarshalCSharpG1Ledger(l)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := checkCSharpG1Ledger(data, a.want); g1Code(err) != f.code {
			t.Errorf("%s: %v, want %s", f.name, err, f.code)
		}
	}
	// text-level forgeries of the tracked bytes
	text := string(a.asset)
	for name, forged := range map[string]struct{ text, code string }{
		"unknown member": {strings.Replace(text, `"basis":`, `"extra": 1,`+"\n"+` "basis":`, 1), "G1_LEDGER_JSON"},
		"not json":       {text[:len(text)/2], "G1_LEDGER_JSON"},
		"whitespace":     {strings.Replace(text, "\n \"rows\"", "\n\n \"rows\"", 1), "G1_LEDGER_BYTES"},
		"basis edited":   {strings.Replace(text, "no expected value is derived", "an expected value is derived", 1), "G1_LEDGER_BYTES"},
	} {
		if forged.text == text {
			t.Fatalf("%s: forgery did not change the asset", name)
		}
		if _, err := checkCSharpG1Ledger([]byte(forged.text), a.want); g1Code(err) != forged.code {
			t.Errorf("%s: %v, want %s", name, err, forged.code)
		}
	}
	if _, err := checkCSharpG1Ledger(append(a.asset, make([]byte, csharpG1MaxBytes)...), a.want); g1Code(err) != "G1_LEDGER_SIZE" {
		t.Errorf("oversize: %v", err)
	}
}

// The ledger is derived from the pinned sources: a changed source is a different pin, and a source
// whose declarations disagree is refused before any ledger is judged.
func TestCSharpG1LedgerSourceTampering(t *testing.T) {
	a := g1Load(t)
	with := func(edit func(*CSharpG1Sources)) CSharpG1Sources {
		s := a.src
		s.Fixtures = map[string][]byte{}
		for k, v := range a.src.Fixtures {
			s.Fixtures[k] = v
		}
		edit(&s)
		return s
	}
	for name, tc := range map[string]struct {
		src  CSharpG1Sources
		code string
	}{
		"alternatives bytes": {with(func(s *CSharpG1Sources) { s.Alternatives = append(slices.Clone(s.Alternatives), ' ') }), "G1_LEDGER_PIN"},
		"mapping bytes":      {with(func(s *CSharpG1Sources) { s.FactMapping = append(slices.Clone(s.FactMapping), ' ') }), "G1_LEDGER_PIN"},
		"fixture bytes":      {with(func(s *CSharpG1Sources) { s.Fixtures["gaps"] = append(slices.Clone(s.Fixtures["gaps"]), ' ') }), "G1_LEDGER_PIN"},
		"missing fixture":    {with(func(s *CSharpG1Sources) { delete(s.Fixtures, "n461") }), "G1_LEDGER_FIXTURE"},
		"alternative id": {with(func(s *CSharpG1Sources) {
			s.Alternatives = []byte(strings.Replace(string(s.Alternatives), `"id": "csharp-B01.a002"`, `"id": "csharp-B01.a902"`, 1))
		}), "G1_LEDGER_ALTERNATIVES"},
		"case source": {with(func(s *CSharpG1Sources) {
			s.Fixtures["routes"] = []byte(strings.Replace(string(s.Fixtures["routes"]), "using System;", "using Systen;", 1))
		}), "G1_LEDGER_SOURCE"},
	} {
		if _, err := ParseCSharpG1Ledger(a.asset, tc.src); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", name, err, tc.code)
		}
	}
}
