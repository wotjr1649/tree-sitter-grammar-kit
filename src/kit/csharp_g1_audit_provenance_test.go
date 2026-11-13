package kit

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// More of the mutants of the review that nothing killed, and the statement about basis_independent (#233, reviewG1c-1).

// A row that has no instance (stated consistently with the asset), a node that was taken away from a row that has one, and a
// rule table that has an entry no row has.
func TestCSharpG1Audit76RefusesRowsWithoutTheirParts(t *testing.T) {
	g1h76Run(t, map[string]g1h76Case{
		"a row without instances, stated consistently": {audit: func(x *CSharpG1Audit76) {
			r := g1a76Row(x, "csharp-B02.a094")
			r.Instances = nil
			r.Result, r.Relation, r.Initial = G1AuditMismatch, G1AuditRelNoInstance, G1AuditMismatch
			x.Counts = g1a76Counts(x.Rows)
		}, code: "G1_AUDIT76_ROW"},
		"a node taken away from a row that has one": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a382").Node = nil }, code: "G1_AUDIT76_GROUND"},
	})
	_, data, in := g1a76Tracked(t)
	g1a76Rules["csharp-zz.stale"] = &g1a76Rule{}
	defer delete(g1a76Rules, "csharp-zz.stale")
	if _, err := ParseCSharpG1Audit76(data, in); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_ROWS") {
		t.Errorf("a rule that no row has: %v", err)
	}
}

// The views refuse an r2 asset the audits could not read: not r2, a second witness, a correction record that is no JSON.
func TestCSharpG1AuditViewsRefuseWhatTheAuditsCannotRead(t *testing.T) {
	a := g1cLoad(t)
	if _, err := G1Audit76View(mustRead(t, "../contracts/csharp-g1-expected-b01b02-r1.json"), nil); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_EXPECTED") {
		t.Errorf("an r1 asset for the position audit: %v", err)
	}
	if _, err := G1TypeAudit76View(mustRead(t, "../contracts/csharp-g1-expected-b01b02-r1.json")); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_EXPECTED") {
		t.Errorf("an r1 asset for the type audit: %v", err)
	}
	mutated := func(f func(x *CSharpG1Expected)) []byte {
		x, err := DecodeCSharpG1Expected(a.r2Exp)
		if err != nil {
			t.Fatal(err)
		}
		x = g1cCloneExpected(x)
		x.Corrections = slices.Clone(x.Corrections)
		f(&x)
		data, err := MarshalCSharpG1Expected(x)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	two := mutated(func(x *CSharpG1Expected) {
		alt := g1a76Alt(x, "csharp-B02.a030")
		alt.Witnesses = append(slices.Clone(alt.Witnesses), alt.Witnesses[0])
	})
	if _, err := G1Audit76View(two, nil); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_WITNESS") {
		t.Errorf("two witnesses: %v", err)
	}
	for field, want := range map[string]string{G1FieldEvidence: "G1_AUDIT76_CORRECTION", G1FieldWitnessMatch: "G1_AUDIT76_CORRECTION"} {
		broken := mutated(func(x *CSharpG1Expected) {
			for i, c := range x.Corrections {
				if c.Field == field {
					x.Corrections[i].Old = "not json"
				}
			}
		})
		if _, err := G1Audit76View(broken, nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("a %s correction that is no JSON: %v", field, err)
		}
	}
	brokenTS := mutated(func(x *CSharpG1Expected) {
		for i, c := range x.Corrections {
			if c.Field == G1FieldTS {
				x.Corrections[i].Old = "not json"
			}
		}
	})
	if _, err := G1TypeAudit76View(brokenTS); err == nil || !strings.Contains(err.Error(), "G1_TYPEAUDIT76_CORRECTION") {
		t.Errorf("a ts correction that is no JSON: %v", err)
	}
}

// The report counts an evidence correction after the native result as a later wrong value of a sample too (not only a ts
// correction), and a correction made before it as none; the type bound counts a mismatch and the corrected rows together.
func TestCSharpG1AuditStrataCountLaterWrongAndTypeMismatches(t *testing.T) {
	r2, err := DecodeCSharpG1Expected(g1cLoad(t).r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	all := func(x CSharpG1Expected) G1AuditStratum {
		rows := G1AuditStrata(x, nil)
		return rows[len(rows)-1]
	}
	base := all(r2)
	have := map[string]bool{}
	for _, c := range r2.Corrections {
		have[c.Item] = true
	}
	id := ""
	for _, s := range r2.Audit.Samples {
		if s.Outcome != "MISMATCH" && s.Outcome != "COVERAGE_GAP" && !have[s.ID] {
			id = s.ID
			break
		}
	}
	if id == "" {
		t.Fatal("no sample without a correction")
	}
	with := func(after bool) CSharpG1Expected {
		m := g1cCloneExpected(r2)
		m.Corrections = append(slices.Clone(m.Corrections), G1Correction{Item: id, Field: G1FieldEvidence, AfterNative: after})
		return m
	}
	if got := all(with(true)); got.LaterWrong != base.LaterWrong+1 {
		t.Errorf("an evidence correction after the native result: later wrong %d, want %d", got.LaterWrong, base.LaterWrong+1)
	}
	if got := all(with(false)); got.LaterWrong != base.LaterWrong {
		t.Errorf("a correction before the native result: later wrong %d, want %d", got.LaterWrong, base.LaterWrong)
	}
	// one mismatching type row and the two corrected ones: the type bound with them counts three
	pos, _, _ := g1a76Tracked(t)
	typ, _, _ := g1t76Tracked(t)
	bad := slices.Clone(typ.Rows)
	bad[0].Result = G1AuditMismatch
	for _, g := range G1AuditStrataWithTypes(r2, pos.Rows, bad) {
		if g.Stratum != G1StratumDifferential {
			continue
		}
		if g.TypeMismatches != 1 || g.TypeFound != 2 || g.TypeUpper95Found != strconv.FormatFloat(G1AuditUpper95(3, 76), 'f', 1, 64) {
			t.Errorf("one type mismatch and two corrections: %+v, want the bound %s", g, strconv.FormatFloat(G1AuditUpper95(3, 76), 'f', 1, 64))
		}
	}
}

// basis_independent says where the grounds come from, not where the new value comes from. Six evidence corrections take their new
// ts from the kind map r2 (their reason says so); their grounds are the grammar and the specification only; and the type audit,
// which derives the type from grammar.js alone, tells for each whether the ts is a grammar derivation (a030: equal), was narrowed
// to one (a088, a089) or is a candidate set the grammar text cannot settle (a084, a086, a087).
func TestCSharpG1EvidenceTSProvenanceIsStated(t *testing.T) {
	var fromMap []string
	for _, c := range G1ExpectedCorrections() {
		if c.Field != G1FieldEvidence {
			continue
		}
		docs := map[string]bool{}
		for _, g := range c.Grounds {
			docs[g.Doc] = true
		}
		if !c.AfterNative || !c.BasisIndependent || len(docs) != 2 || !docs[G1DocGrammar] || !docs[G1DocSpec] {
			t.Errorf("%s: after_native %v, basis_independent %v, grounds from %v", c.Item, c.AfterNative, c.BasisIndependent, docs)
		}
		if strings.Contains(c.Reason, "kind map r2") {
			fromMap = append(fromMap, c.Item)
		}
	}
	slices.Sort(fromMap)
	want := []string{"csharp-B02.a030", "csharp-B02.a084", "csharp-B02.a086", "csharp-B02.a087", "csharp-B02.a088", "csharp-B02.a089"}
	if !slices.Equal(fromMap, want) {
		t.Fatalf("evidence corrections whose ts is the kind map's: %v, want %v", fromMap, want)
	}
	typ, _, _ := g1t76Tracked(t)
	for _, r := range typ.Rows {
		switch r.ID {
		case "csharp-B02.a030":
			if r.Class == G1TypeNotDerivable || r.Relation != G1TypeRelEqual || r.Initial != G1AuditMatch {
				t.Errorf("%s: %+v", r.ID, r)
			}
		case "csharp-B02.a088", "csharp-B02.a089":
			// the ts of the kind map did not agree with the grammar text and was narrowed to the type the grammar text gives
			if r.Class != G1TypeAlias || r.Initial != G1AuditMismatch || r.Result != G1AuditMatch {
				t.Errorf("%s: %+v", r.ID, r)
			}
		case "csharp-B02.a084", "csharp-B02.a086", "csharp-B02.a087":
			// the grammar text leaves the candidate set open: the ts is the kind map's, not a grammar derivation
			if r.Class != G1TypeNotDerivable || r.Relation != G1TypeRelCandidates {
				t.Errorf("%s: %+v", r.ID, r)
			}
		}
	}
}
