package kit

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Ledger r2 (#233 G1-3b): the r2 pair of csharp-B01 and csharp-B02 and the pair of csharp-B03 and csharp-B04. Ledger r1 is frozen.

type g1l2Assets struct {
	asset []byte
	src   CSharpG1Sources // the sources of ledger r2
	want  CSharpG1Ledger
	plain CSharpG1Ledger // the declarations alone: every state NO_EXPECTED
	err   error
}

var g1l2Once sync.Once
var g1l2Loaded g1l2Assets

func g1l2Load(t *testing.T) *g1l2Assets {
	t.Helper()
	g1l2Once.Do(func() {
		a := &g1l2Loaded
		read := func(name string) []byte {
			d, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return d
		}
		a.asset = read("../contracts/csharp-g1-ledger-r2.json")
		a.src = g1Load(t).src
		a.src.ExpectedB34, a.src.ExecutedB34 = read("../contracts/csharp-g1-expected-b03b04-r1.json"), read("../contracts/csharp-g1-executed-b03b04-r1.json")
		if a.err != nil {
			return
		}
		if a.want, a.err = DeriveCSharpG1LedgerR2(a.src); a.err != nil {
			return
		}
		bare := a.src
		bare.Expected, bare.Executed, bare.ExpectedB34, bare.ExecutedB34 = nil, nil, nil, nil
		a.plain, a.err = DeriveCSharpG1Ledger(bare)
	})
	if g1l2Loaded.err != nil {
		t.Fatal(g1l2Loaded.err)
	}
	return &g1l2Loaded
}

func g1l2Clone(t *testing.T, l CSharpG1Ledger) CSharpG1Ledger {
	t.Helper()
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var y CSharpG1Ledger
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return y
}

func g1l2Statuses(l CSharpG1Ledger) map[string]int {
	n := map[string]int{}
	for _, a := range l.Alternatives {
		n[a.Row+" "+a.Status]++
	}
	return n
}

func TestCSharpG1LedgerR2IsTheDerivation(t *testing.T) {
	a := g1l2Load(t)
	x, err := ParseCSharpG1LedgerR2(a.asset, a.src)
	if err != nil {
		t.Fatal(err)
	}
	if x.Schema != CSharpG1LedgerSchemaR2 || CSharpG1LedgerPathR2 != "src/contracts/csharp-g1-ledger-r2.json" {
		t.Errorf("schema %s", x.Schema)
	}
	n := g1l2Statuses(x)
	for k, want := range map[string]int{
		"csharp-B01 EXECUTED": 370, "csharp-B01 BLOCKED": 37, "csharp-B02 EXECUTED": 56, "csharp-B02 BLOCKED": 42,
		"csharp-B03 EXECUTED": 193, "csharp-B03 SEALED": 3, "csharp-B03 BLOCKED": 61,
		"csharp-B04 EXECUTED": 98, "csharp-B04 SEALED": 2, "csharp-B04 BLOCKED": 31,
	} {
		if n[k] != want {
			t.Errorf("%s: %d, want %d", k, n[k], want)
		}
	}
	for k, c := range n {
		row, status, _ := strings.Cut(k, " ")
		if !slices.Contains([]string{"csharp-B01", "csharp-B02", "csharp-B03", "csharp-B04"}, row) && status != G1NoExpected {
			t.Errorf("%s: %d alternatives outside the four rows left NO_EXPECTED", k, c)
		}
	}
	// the P obligations of B03 and B04 stay NO_EXPECTED: a row of alternatives with BLOCKED ones is neither SEALED nor EXECUTED
	for _, r := range x.Rows {
		for _, o := range r.Obligations {
			if o.Status != G1NoExpected {
				t.Errorf("%s %s is %s", r.Row, o.Kind, o.Status)
			}
		}
	}
	// each state follows the two assets: EXPECTED and MATCH is EXECUTED, EXPECTED and MISMATCH is SEALED, BLOCKED keeps its reason code
	exp, err := DecodeCSharpG1B34(a.src.ExpectedB34)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := DecodeCSharpG1B34Executed(a.src.ExecutedB34)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, alt := range exe.Alternatives {
		result[alt.ID] = alt.Result
	}
	byID := map[string]G1Alternative{}
	for _, alt := range x.Alternatives {
		byID[alt.ID] = alt
	}
	for _, e := range exp.Alternatives {
		got := byID[e.ID]
		switch {
		case e.Status == G1ExpBlocked:
			if got.Status != G1Blocked || got.Block != e.Block {
				t.Errorf("%s: blocked %q became %s %q", e.ID, e.Block, got.Status, got.Block)
			}
		case result[e.ID] == G1ResMatch:
			if got.Status != G1Executed {
				t.Errorf("%s: MATCH is %s", e.ID, got.Status)
			}
		default:
			if got.Status != G1Sealed {
				t.Errorf("%s: %s is %s", e.ID, result[e.ID], got.Status)
			}
		}
	}
	// r2 differs from r1 in the schema, the basis and the alternatives of csharp-B03 and csharp-B04, and in nothing else
	r1 := g1l2Clone(t, g1Load(t).want)
	y := g1l2Clone(t, x)
	y.Schema, y.Basis = r1.Schema, r1.Basis
	for i := range y.Alternatives {
		if y.Alternatives[i].Row == "csharp-B03" || y.Alternatives[i].Row == "csharp-B04" {
			y.Alternatives[i].Status, y.Alternatives[i].Block = G1NoExpected, ""
		}
	}
	if !g1Same(y, r1) {
		t.Error("ledger r2 differs from r1 beyond the schema, the basis and the states of csharp-B03 and csharp-B04")
	}
}

// Ledger r1 is frozen: its code path takes no B03/B04 asset, its tracked bytes still are its derivation, and each revision rejects the other's
// asset.
func TestCSharpG1LedgerR1StaysFrozen(t *testing.T) {
	a := g1l2Load(t)
	r1 := g1Load(t)
	if _, err := ParseCSharpG1Ledger(r1.asset, r1.src); err != nil {
		t.Fatalf("ledger r1: %v", err)
	}
	if g1Same(r1.want.Schema, a.want.Schema) || r1.want.Schema != CSharpG1LedgerSchema {
		t.Errorf("schemas %s %s", r1.want.Schema, a.want.Schema)
	}
	if _, err := ParseCSharpG1Ledger(a.asset, a.src); g1Code(err) != "G1_LEDGER_SEALED" {
		t.Errorf("r1 derivation given the B03/B04 assets: %v", err)
	}
	if _, err := checkCSharpG1Ledger(a.asset, r1.want); g1Code(err) != "G1_LEDGER_SCHEMA" {
		t.Errorf("r2 asset against r1: %v", err)
	}
	if _, err := checkCSharpG1Ledger(r1.asset, a.want); g1Code(err) != "G1_LEDGER_SCHEMA" {
		t.Errorf("r1 asset against r2: %v", err)
	}
	for name, src := range map[string]func(s *CSharpG1Sources){
		"r1 with the expected asset of B03 and B04":  func(s *CSharpG1Sources) { s.Executed, s.ExecutedB34 = nil, nil },
		"r1 with the executed asset of B03 and B04":  func(s *CSharpG1Sources) { s.ExpectedB34 = nil },
		"r1 with only the executed asset of B03/B04": func(s *CSharpG1Sources) { s.ExpectedB34 = nil; s.Executed = nil },
	} {
		s := a.src
		src(&s)
		if _, err := DeriveCSharpG1Ledger(s); g1Code(err) != "G1_LEDGER_SEALED" {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, edit := range map[string]func(s *CSharpG1Sources){
		"r2 without the expected asset of B03 and B04": func(s *CSharpG1Sources) { s.ExpectedB34, s.ExecutedB34 = nil, nil },
		"r2 without the r2 pair of B01 and B02":        func(s *CSharpG1Sources) { s.Expected, s.Executed = nil, nil },
	} {
		s := a.src
		edit(&s)
		if _, err := DeriveCSharpG1LedgerR2(s); g1Code(err) != "G1_LEDGER_SEALED" {
			t.Errorf("%s: %v", name, err)
		}
	}
	s := a.src
	s.ExpectedB34 = nil
	if _, err := DeriveCSharpG1LedgerR2(s); g1Code(err) != "G1_LEDGER_SEALED" {
		t.Errorf("executed asset of B03/B04 without its expected asset: %v", err)
	}
}

// The state rules of the pair of B03 and B04 (R-2, R-8), applied to a copy of the declarations-only ledger with hand-made assets: each rule
// has a counterexample, and each counterexample is judged by the same function the derivation uses.
func TestCSharpG1LedgerR2StateRules(t *testing.T) {
	a := g1l2Load(t)
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
	var ids []string
	rowOf := map[string]string{}
	for _, alt := range a.plain.Alternatives {
		if alt.Row == "csharp-B03" || alt.Row == "csharp-B04" {
			ids = append(ids, alt.ID)
			rowOf[alt.ID] = alt.Row
		}
	}
	if len(ids) != 388 {
		t.Fatalf("%d alternatives in B03 and B04", len(ids))
	}
	allExp := func() []expAlt {
		var out []expAlt
		for _, id := range ids {
			out = append(out, expAlt{id, rowOf[id], G1ExpExpected, ""})
		}
		return out
	}
	allExe := func(result string) []exeAlt {
		var out []exeAlt
		for _, id := range ids {
			out = append(out, exeAlt{id, rowOf[id], result == G1ResMatch || result == G1ResMismatch, result})
		}
		return out
	}
	apply := func(exp []expAlt, exe []exeAlt) (CSharpG1Ledger, error) {
		l := g1l2Clone(t, a.plain)
		e, _ := json.Marshal(map[string]any{"alternatives": exp})
		var x []byte
		if exe != nil {
			x, _ = json.Marshal(map[string]any{"alternatives": exe})
		}
		return l, g1ApplyStates(&l, e, x, g1LedgerRowsB34, true)
	}
	state := func(l CSharpG1Ledger, id string) (string, string) {
		for _, alt := range l.Alternatives {
			if alt.ID == id {
				return alt.Status, alt.Block
			}
		}
		return "", ""
	}
	pStatus := func(l CSharpG1Ledger, row string) string {
		for _, r := range l.Rows {
			if r.Row == row {
				for _, o := range r.Obligations {
					if o.Kind == "P" {
						return o.Status
					}
				}
			}
		}
		return ""
	}
	// all EXPECTED, all executed with a MATCH: both rows' P obligations are EXECUTED, the other kinds stay
	l, err := apply(allExp(), allExe(G1ResMatch))
	if err != nil || pStatus(l, "csharp-B03") != G1Executed || pStatus(l, "csharp-B04") != G1Executed {
		t.Fatalf("all executed: %v %s %s", err, pStatus(l, "csharp-B03"), pStatus(l, "csharp-B04"))
	}
	for _, r := range l.Rows {
		for _, o := range r.Obligations {
			if (r.Row == "csharp-B03" || r.Row == "csharp-B04") && o.Kind != "P" && o.Status != G1NoExpected {
				t.Errorf("%s %s became %s", r.Row, o.Kind, o.Status)
			}
		}
	}
	// the B01 and B02 alternatives are not touched by this pair
	for _, alt := range l.Alternatives {
		if alt.Row != "csharp-B03" && alt.Row != "csharp-B04" && alt.Status != G1NoExpected {
			t.Fatalf("%s of %s became %s", alt.ID, alt.Row, alt.Status)
		}
	}
	// EXECUTED means MATCH: a MISMATCH or NOT_APPLICABLE alternative stays SEALED, and so does the P obligation of its row
	for _, bad := range []string{G1ResMismatch, G1ResNotApplicable} {
		exe := allExe(G1ResMatch)
		exe[5] = exeAlt{ids[5], rowOf[ids[5]], bad == G1ResMismatch, bad}
		l, err := apply(allExp(), exe)
		if st, _ := state(l, ids[5]); err != nil || st != G1Sealed || pStatus(l, rowOf[ids[5]]) != G1Sealed {
			t.Fatalf("%s: %v %q P %q", bad, err, st, pStatus(l, rowOf[ids[5]]))
		}
		if st, _ := state(l, ids[6]); st != G1Executed {
			t.Errorf("%s moved a neighbour to %q", bad, st)
		}
	}
	// SEALED without a run stays SEALED
	l, err = apply(allExp(), nil)
	if st, _ := state(l, ids[0]); err != nil || st != G1Sealed || pStatus(l, "csharp-B03") != G1Sealed {
		t.Fatalf("no executed asset: %v %q", err, st)
	}
	// a BLOCKED alternative keeps its reason and keeps its row at NO_EXPECTED
	exp := allExp()
	exp[2] = expAlt{ids[2], rowOf[ids[2]], G1ExpBlocked, "SEMANTIC_BINDING"}
	exe := slices.Delete(allExe(G1ResMatch), 2, 3)
	l, err = apply(exp, exe)
	if st, why := state(l, ids[2]); err != nil || st != G1Blocked || why != "SEMANTIC_BINDING" || pStatus(l, rowOf[ids[2]]) != G1NoExpected {
		t.Fatalf("one blocked: %v %q %q P %q", err, st, why, pStatus(l, rowOf[ids[2]]))
	}
	// the counterexamples of R-8 and of the transitions
	dup := append(allExp(), allExp()[0])
	wrongRow := allExp()
	wrongRow[1].Row = "csharp-B04"
	if rowOf[ids[1]] == "csharp-B04" {
		wrongRow[1].Row = "csharp-B03"
	}
	otherRow := append(allExp()[1:], expAlt{"csharp-B01.a001", "csharp-B01", G1ExpExpected, ""})
	unknown := append(allExp()[1:], expAlt{"csharp-B03.a9999", "csharp-B03", G1ExpExpected, ""})
	blockedExe := slices.Clone(exp)
	for _, tc := range []struct {
		name     string
		exp      []expAlt
		exe      []exeAlt
		wantCode string
	}{
		{"an alternative named twice", dup, nil, "G1_LEDGER_SEALED"},
		{"an alternative missing", allExp()[1:], nil, "G1_LEDGER_SEALED"},
		{"an alternative in the wrong row", wrongRow, nil, "G1_LEDGER_SEALED"},
		{"an alternative of another pair", otherRow, nil, "G1_LEDGER_SEALED"},
		{"an alternative the ledger does not have", unknown, nil, "G1_LEDGER_SEALED"},
		{"a blocked alternative without a reason", append(allExp()[1:], expAlt{ids[0], rowOf[ids[0]], G1ExpBlocked, ""}), nil, "G1_LEDGER_SEALED"},
		{"an unknown expected status", append(allExp()[1:], expAlt{ids[0], rowOf[ids[0]], "MAYBE", ""}), nil, "G1_LEDGER_SEALED"},
		{"executing a blocked alternative", blockedExe, allExe(G1ResMatch), "G1_LEDGER_EXECUTED"},
		{"executing an alternative twice", allExp(), append(allExe(G1ResMatch), allExe(G1ResMatch)[0]), "G1_LEDGER_EXECUTED"},
		{"executing an alternative of the wrong row", allExp(), []exeAlt{{ids[0], "csharp-B01", true, G1ResMatch}}, "G1_LEDGER_EXECUTED"},
		{"executing an unknown alternative", allExp(), []exeAlt{{"csharp-B03.a9999", "csharp-B03", true, G1ResMatch}}, "G1_LEDGER_EXECUTED"},
		{"a MATCH that did not execute", allExp(), []exeAlt{{ids[0], rowOf[ids[0]], false, G1ResMatch}}, "G1_LEDGER_EXECUTED"},
		{"a NOT_APPLICABLE that executed", allExp(), []exeAlt{{ids[0], rowOf[ids[0]], true, G1ResNotApplicable}}, "G1_LEDGER_EXECUTED"},
		{"an unknown result", allExp(), []exeAlt{{ids[0], rowOf[ids[0]], true, "MAYBE"}}, "G1_LEDGER_EXECUTED"},
	} {
		if _, err := apply(tc.exp, tc.exe); g1Code(err) != tc.wantCode {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.wantCode)
		}
	}
}

// A hand-edited ledger r2 is rejected by the derivation check.
func TestCSharpG1LedgerR2Forgeries(t *testing.T) {
	a := g1l2Load(t)
	find := func(l *CSharpG1Ledger, row, status string) *G1Alternative {
		for i := range l.Alternatives {
			if l.Alternatives[i].Row == row && l.Alternatives[i].Status == status {
				return &l.Alternatives[i]
			}
		}
		t.Fatalf("no %s %s", row, status)
		return nil
	}
	for _, tc := range []struct {
		name string
		edit func(l *CSharpG1Ledger)
		code string
	}{
		{"BLOCKED executed", func(l *CSharpG1Ledger) { find(l, "csharp-B03", G1Blocked).Status = G1Executed }, "G1_LEDGER_ALTERNATIVE"},
		{"a MISMATCH executed", func(l *CSharpG1Ledger) { find(l, "csharp-B04", G1Sealed).Status = G1Executed }, "G1_LEDGER_ALTERNATIVE"},
		{"EXECUTED sealed again", func(l *CSharpG1Ledger) { find(l, "csharp-B04", G1Executed).Status = G1Sealed }, "G1_LEDGER_ALTERNATIVE"},
		{"a reason dropped", func(l *CSharpG1Ledger) { find(l, "csharp-B03", G1Blocked).Block = "" }, "G1_LEDGER_ALTERNATIVE"},
		{"a reason changed", func(l *CSharpG1Ledger) { find(l, "csharp-B03", G1Blocked).Block = "SEMANTIC_BINDING_X" }, "G1_LEDGER_ALTERNATIVE"},
		{"the P obligation executed", func(l *CSharpG1Ledger) {
			l.Rows[slices.IndexFunc(l.Rows, func(r G1Row) bool { return r.Row == "csharp-B03" })].Obligations[0].Status = G1Executed
		}, "G1_LEDGER_ROWS"},
		{"a status outside the vocabulary", func(l *CSharpG1Ledger) { find(l, "csharp-B03", G1Blocked).Status = "MAYBE" }, "G1_LEDGER_STATUS"},
		{"an alternative repeated in place of another", func(l *CSharpG1Ledger) {
			i := slices.IndexFunc(l.Alternatives, func(x G1Alternative) bool { return x.Row == "csharp-B04" })
			l.Alternatives[i+1] = l.Alternatives[i]
		}, "G1_LEDGER_ALTERNATIVE_DUPLICATE"},
		{"an alternative missing", func(l *CSharpG1Ledger) { l.Alternatives = l.Alternatives[1:] }, "G1_LEDGER_ALTERNATIVE_MISSING"},
		{"the schema of r1", func(l *CSharpG1Ledger) { l.Schema = CSharpG1LedgerSchema }, "G1_LEDGER_SCHEMA"},
		{"the basis of r1", func(l *CSharpG1Ledger) { l.Basis = csharpG1Basis }, "G1_LEDGER_BYTES"},
	} {
		l := g1l2Clone(t, a.want)
		tc.edit(&l)
		data, err := MarshalCSharpG1Ledger(l)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := checkCSharpG1Ledger(data, a.want); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.code)
		}
	}
}
