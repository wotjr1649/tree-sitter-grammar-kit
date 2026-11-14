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

// g1LoadOnce reads the tracked asset and its pinned sources and derives the ledger once (parallel tests share it).
var g1LoadOnce = sync.OnceValue(func() g1Assets {
	var loaded g1Assets
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil && loaded.err == nil {
			loaded.err = err
		}
		return data
	}
	a := &loaded
	a.asset = read("../contracts/csharp-g1-ledger-r1.json")
	a.src = CSharpG1Sources{
		Inventory:    read("../contracts/qualification-c1.json"),
		Alternatives: read("../contracts/feature-alternatives.json"),
		FactMapping:  read("../contracts/fact-mapping.json"),
		Fixtures:     map[string][]byte{},
		Expected:     read("../contracts/csharp-g1-expected-b01b02-r2.json"),
		Executed:     read("../contracts/csharp-g1-executed-b01b02-r2.json"),
	}
	for _, dir := range csharpG1Fixtures {
		a.src.Fixtures[dir] = read("../testdata/native/" + dir + "/csharp.json")
	}
	if a.err == nil {
		a.want, a.err = DeriveCSharpG1Ledger(a.src)
	}
	return loaded
})

func g1Load(t *testing.T) g1Assets {
	t.Helper()
	a := g1LoadOnce()
	if a.err != nil {
		t.Fatal(a.err)
	}
	return a
}

func g1Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestCSharpG1LedgerSealedAndBound(t *testing.T) {
	t.Parallel()
	a := g1Load(t)
	if len(a.asset) != 714461 || digestHex(a.asset) != "5b42627b3bb47cbb656f37541292b3a1d313f808401e85b3e438bf9c39acee77" {
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
	// the states follow the two assets (each verified in its own test): the expected asset's EXPECTED
	// alternatives are SEALED and its BLOCKED ones BLOCKED with the same reason code, and the alternatives
	// the executed asset ran are EXECUTED
	var expDoc struct {
		Alternatives []struct {
			ID, Status, Block string
		}
	}
	if err := json.Unmarshal(a.src.Expected, &expDoc); err != nil {
		t.Fatal(err)
	}
	var exeDoc struct {
		Alternatives []struct {
			ID       string
			Executed bool
			Result   string
		}
	}
	if err := json.Unmarshal(a.src.Executed, &exeDoc); err != nil {
		t.Fatal(err)
	}
	sealed, blocked, executed := map[string]bool{}, map[string]string{}, map[string]bool{}
	for _, e := range expDoc.Alternatives {
		switch e.Status {
		case G1ExpExpected:
			sealed[e.ID] = true
		case G1ExpBlocked:
			blocked[e.ID] = e.Block
		}
	}
	for _, e := range exeDoc.Alternatives {
		// EXECUTED means compared with native and matched
		if e.Executed && e.Result == G1ResMatch {
			executed[e.ID] = true
		}
	}
	if len(sealed) != 426 || len(blocked) != 79 || len(executed) != 426 {
		t.Fatalf("sealed %d blocked %d executed %d", len(sealed), len(blocked), len(executed))
	}
	byState := map[string]int{}
	var ids []string
	cases := map[string]bool{}
	for _, c := range x.Cases {
		cases[c.ID] = true
	}
	for _, alt := range x.Alternatives {
		ids = append(ids, alt.ID)
		wantStatus, wantBlock := G1NoExpected, ""
		switch {
		case executed[alt.ID]:
			wantStatus = G1Executed
		case sealed[alt.ID]:
			wantStatus = G1Sealed
		case blocked[alt.ID] != "":
			wantStatus, wantBlock = G1Blocked, blocked[alt.ID]
		}
		byState[alt.Status]++
		if alt.Status != wantStatus || alt.Block != wantBlock || len(alt.Links) == 0 {
			t.Errorf("%s: status %q block %q want %q %q links %d", alt.ID, alt.Status, alt.Block, wantStatus, wantBlock, len(alt.Links))
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
	if byState[G1NoExpected] != 1026 || byState[G1Sealed] != 0 || byState[G1Blocked] != 79 || byState[G1Executed] != 426 {
		t.Fatalf("states %v", byState)
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
// g1First is the index of the first alternative in the given state.
func g1First(l *CSharpG1Ledger, status string) int {
	for i, a := range l.Alternatives {
		if a.Status == status {
			return i
		}
	}
	panic("no alternative in state " + status)
}

func TestCSharpG1LedgerForgeries(t *testing.T) {
	t.Parallel()
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
		{"unknown alternative state", "G1_LEDGER_STATUS", func(l *CSharpG1Ledger) { l.Alternatives[1].Status = "PASS" }},
		{"alternative without a comparison executed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { last(l).Status = G1Executed }},
		{"executed alternative back to sealed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[1].Status = G1Sealed }},
		{"blocked alternative executed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[g1First(l, G1Blocked)].Status = G1Executed }},
		{"blocked alternative sealed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[g1First(l, G1Blocked)].Status = G1Sealed }},
		{"blocked alternative back to no expectation", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) {
			i := g1First(l, G1Blocked)
			l.Alternatives[i].Status, l.Alternatives[i].Block = G1NoExpected, ""
		}},
		{"blocked reason dropped", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[g1First(l, G1Blocked)].Block = "" }},
		{"blocked reason changed", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[g1First(l, G1Blocked)].Block = "NO_TREE_COUNTERPART_X" }},
		{"reason on an alternative that is not blocked", "G1_LEDGER_ALTERNATIVE", func(l *CSharpG1Ledger) { l.Alternatives[1].Block = "NO_TREE_COUNTERPART" }},
		{"obligation executed", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows[0].Obligations[0].Status = G1Executed }},
		{"obligation blocked", "G1_LEDGER_ROWS", func(l *CSharpG1Ledger) { l.Rows[0].Obligations[0].Status = G1Blocked }},
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
	t.Parallel()
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

// The state rules of the two lexical rows: NO_EXPECTED becomes SEALED or BLOCKED by the expected asset,
// only a SEALED alternative becomes EXECUTED by the executed asset, and a row's P obligation follows its
// alternatives. The ledger derivation is given hand-made assets (it reads only IDs, rows, statuses and
// reason codes), so each rule has a counterexample.
func TestCSharpG1LedgerStateRules(t *testing.T) {
	t.Parallel()
	a := g1Load(t)
	base := a.src
	base.Expected, base.Executed = nil, nil
	plain, err := DeriveCSharpG1Ledger(base)
	if err != nil {
		t.Fatal(err)
	}
	type expAlt struct {
		ID     string `json:"id"`
		Row    string `json:"row"`
		Status string `json:"status"`
		Block  string `json:"block,omitempty"`
	}
	type exeAlt struct {
		ID       string `json:"id"`
		Row      string `json:"row"`
		Executed bool   `json:"executed"`
		Result   string `json:"result"`
	}
	var b02 []string
	for _, alt := range plain.Alternatives {
		if alt.Row == g1ExpRowB02 {
			b02 = append(b02, alt.ID)
		}
	}
	// mkRes is mk with the result of each executed alternative chosen by the caller ("" = not listed)
	mkRes := func(status func(i int) (string, string), result func(i int) string) CSharpG1Sources {
		var exp []expAlt
		var exe []exeAlt
		for i, id := range b02 {
			s, block := status(i)
			exp = append(exp, expAlt{id, g1ExpRowB02, s, block})
			if r := result(i); r != "" {
				exe = append(exe, exeAlt{id, g1ExpRowB02, r == G1ResMatch || r == G1ResMismatch, r})
			}
		}
		e, _ := json.Marshal(map[string]any{"alternatives": exp})
		s := base
		s.Expected = e
		if len(exe) > 0 {
			x, _ := json.Marshal(map[string]any{"alternatives": exe})
			s.Executed = x
		}
		return s
	}
	mk := func(status func(i int) (string, string), executed func(i int) (bool, bool)) CSharpG1Sources {
		return mkRes(status, func(i int) string {
			switch ok, listed := executed(i); {
			case !listed:
				return ""
			case ok:
				return G1ResMatch
			}
			return G1ResNotApplicable
		})
	}
	allExpected := func(int) (string, string) { return G1ExpExpected, "" }
	pStatus := func(l CSharpG1Ledger) string {
		for _, r := range l.Rows {
			if r.Row == g1ExpRowB02 {
				for _, o := range r.Obligations {
					if o.Kind == "P" {
						return o.Status
					}
				}
			}
		}
		return ""
	}
	altStatus := func(l CSharpG1Ledger, id string) (string, string) {
		for _, alt := range l.Alternatives {
			if alt.ID == id {
				return alt.Status, alt.Block
			}
		}
		return "", ""
	}
	// every alternative EXPECTED and executed: the row's P obligation is EXECUTED, the others stay
	l, err := DeriveCSharpG1Ledger(mk(allExpected, func(int) (bool, bool) { return true, true }))
	if err != nil || pStatus(l) != G1Executed {
		t.Fatalf("all executed: %v, P %q", err, pStatus(l))
	}
	for _, r := range l.Rows {
		for _, o := range r.Obligations {
			if r.Row == g1ExpRowB02 && o.Kind != "P" && o.Status != G1NoExpected {
				t.Errorf("%s %s became %s", r.Row, o.Kind, o.Status)
			}
		}
	}
	// SEALED without a run stays SEALED, and the row follows its weakest alternative
	l, err = DeriveCSharpG1Ledger(mk(allExpected, func(i int) (bool, bool) { return i != 0, true }))
	if st, _ := altStatus(l, b02[0]); err != nil || st != G1Sealed || pStatus(l) != G1Sealed {
		t.Fatalf("one sealed only: %v %q P %q", err, st, pStatus(l))
	}
	// a BLOCKED alternative keeps its reason and keeps the row at NO_EXPECTED
	blockOne := func(i int) (string, string) {
		if i == 1 {
			return G1ExpBlocked, "SEMANTIC_BINDING"
		}
		return G1ExpExpected, ""
	}
	l, err = DeriveCSharpG1Ledger(mk(blockOne, func(i int) (bool, bool) { return i != 1, i != 1 }))
	if st, why := altStatus(l, b02[1]); err != nil || st != G1Blocked || why != "SEMANTIC_BINDING" || pStatus(l) != G1NoExpected {
		t.Fatalf("one blocked: %v %q %q P %q", err, st, why, pStatus(l))
	}
	// EXECUTED means compared with native and matched: a MISMATCH (or NOT_APPLICABLE) alternative stays
	// SEALED, and a row whose alternative is not EXECUTED stays SEALED too
	for _, bad := range []string{G1ResMismatch, G1ResNotApplicable} {
		l, err = DeriveCSharpG1Ledger(mkRes(allExpected, func(i int) string {
			if i == 2 {
				return bad
			}
			return G1ResMatch
		}))
		if st, _ := altStatus(l, b02[2]); err != nil || st != G1Sealed || pStatus(l) != G1Sealed {
			t.Fatalf("%s alternative: %v %q P %q, want SEALED", bad, err, st, pStatus(l))
		}
		if st, _ := altStatus(l, b02[3]); st != G1Executed {
			t.Errorf("%s of one alternative moved its neighbour to %q", bad, st)
		}
	}
	// the tracked data: with the r1 executed asset the four mismatches of the sealed values are not EXECUTED
	r1 := a.src
	r1.Expected, r1.Executed = mustRead(t, "../contracts/csharp-g1-expected-b01b02-r1.json"), mustRead(t, "../contracts/csharp-g1-executed-b01b02-r1.json")
	l, err = DeriveCSharpG1Ledger(r1)
	if err != nil {
		t.Fatal(err)
	}
	n := map[string]int{}
	for _, alt := range l.Alternatives {
		n[alt.Status]++
		if alt.ID == "csharp-B01.a335" && alt.Status != G1Sealed {
			t.Errorf("a335 is a MISMATCH in r1 but %s", alt.Status)
		}
	}
	if n[G1Executed] != 422 || n[G1Sealed] != 4 {
		t.Errorf("r1 executed asset: states %v, want 422 EXECUTED and 4 SEALED", n)
	}
	// counterexamples of the transitions
	for _, tc := range []struct {
		name string
		src  CSharpG1Sources
		code string
	}{
		{"executing a blocked alternative", mk(blockOne, func(int) (bool, bool) { return true, true }), "G1_LEDGER_EXECUTED"},
		{"executing an alternative that is not in the expected asset", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return true, true })
			s.Executed = []byte(`{"alternatives":[{"id":"csharp-B02.a9999","row":"csharp-B02","executed":true,"result":"MATCH"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"executing an alternative twice", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B02","executed":true,"result":"MATCH"},{"id":"` + b02[0] + `","row":"csharp-B02","executed":true,"result":"MATCH"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"executing without a sealed expected asset", func() CSharpG1Sources {
			s := base
			s.Executed = a.src.Executed
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"executed row mismatch", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B01","executed":true,"result":"MATCH"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"executed without a result", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B02","executed":true}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"unknown result", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B02","executed":true,"result":"MAYBE"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"MATCH that did not execute", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B02","executed":false,"result":"MATCH"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"NOT_APPLICABLE that executed", func() CSharpG1Sources {
			s := mk(allExpected, func(int) (bool, bool) { return false, false })
			s.Executed = []byte(`{"alternatives":[{"id":"` + b02[0] + `","row":"csharp-B02","executed":true,"result":"NOT_APPLICABLE"}]}`)
			return s
		}(), "G1_LEDGER_EXECUTED"},
		{"blocked without a reason", mk(func(i int) (string, string) { return G1ExpBlocked, "" }, func(int) (bool, bool) { return false, false }), "G1_LEDGER_SEALED"},
		{"unknown expected status", mk(func(i int) (string, string) { return "MAYBE", "" }, func(int) (bool, bool) { return false, false }), "G1_LEDGER_SEALED"},
	} {
		if _, err := DeriveCSharpG1Ledger(tc.src); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.code)
		}
	}
}

// The ledger reads the result words under its own names (it must not reach the comparison file); they are
// the executed asset's words.
func TestCSharpG1LedgerResultWordsAreTheExecutedAssets(t *testing.T) {
	if g1LedgerResMatch != G1ResMatch || g1LedgerResMismatch != G1ResMismatch || g1LedgerResNone != G1ResNotApplicable {
		t.Fatal("the ledger's result words differ from the executed asset's")
	}
}
