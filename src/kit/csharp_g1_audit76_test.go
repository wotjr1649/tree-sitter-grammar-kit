package kit

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func g1a76Sources(t *testing.T, expected []byte) G1Audit76Sources {
	t.Helper()
	a := g1cLoad(t)
	src := a.expIn.R1.Src
	src.Expected, src.Executed = nil, nil
	all, err := CSharpG1RoslynSources(src)
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string][]byte{}
	for _, s := range all {
		texts[s.ID] = s.Data
	}
	return G1Audit76Sources{Expected: expected, Texts: texts}
}

func g1a76Tracked(t *testing.T) (CSharpG1Audit76, []byte, G1Audit76Sources) {
	t.Helper()
	data := mustRead(t, "../contracts/csharp-g1-audit76-r1.json")
	in := g1a76Sources(t, g1cLoad(t).r2Exp)
	x, err := ParseCSharpG1Audit76(data, in)
	if err != nil {
		t.Fatal(err)
	}
	return x, data, in
}

// The tracked audit parses, its rows are exactly the 76 differential-oracle alternatives of r2, and the comparison it
// states is the fresh one: after the corrections of r2 every row is consistent.
func TestCSharpG1Audit76IsConsistentWithR2(t *testing.T) {
	x, _, in := g1a76Tracked(t)
	r2, alts, err := g1a76Differential(in.Expected)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Counts.Differential != 76 || len(alts) != 76 || len(x.Rows) != 76 {
		t.Fatalf("differential %d, alternatives %d, rows %d", r2.Counts.Differential, len(alts), len(x.Rows))
	}
	if want := (G1Audit76Counts{Rows: 76, Match: 76, Mismatch: 0, Corrected: 11, Node: 3}); x.Counts != want {
		t.Fatalf("counts %+v, want %+v", x.Counts, want)
	}
	for i, r := range x.Rows {
		if r.ID != alts[i].ID || r.Result != G1AuditMatch || len(r.Grounds) == 0 {
			t.Errorf("row %s: %+v", r.ID, r)
		}
	}
	// the eleven rows the audit found are exactly the eleven evidence and relaxed-match corrections of r2
	var found, recorded []string
	for _, r := range x.Rows {
		if r.Initial == G1AuditMismatch {
			found = append(found, r.ID)
		}
	}
	for _, c := range r2.Corrections {
		if c.Field == G1FieldEvidence || c.Item == "csharp-B01.a381" || c.Item == "csharp-B01.a395" {
			recorded = append(recorded, c.Item)
		}
	}
	slices.Sort(found)
	slices.Sort(recorded)
	if !slices.Equal(found, recorded) || len(found) != 11 {
		t.Errorf("found %v, recorded corrections %v", found, recorded)
	}
}

// g1a76Mutant parses the audit after mutate changed the decoded asset (re-marshalled canonically) or the r2 asset.
func g1a76Mutant(t *testing.T, mutateAudit func(x *CSharpG1Audit76), mutateExpected func(x *CSharpG1Expected)) error {
	t.Helper()
	x, _, in := g1a76Tracked(t)
	if mutateAudit != nil {
		x.Rows = slices.Clone(x.Rows)
		mutateAudit(&x)
	}
	if mutateExpected != nil {
		r2, err := DecodeCSharpG1Expected(in.Expected)
		if err != nil {
			t.Fatal(err)
		}
		r2 = g1cCloneExpected(r2)
		mutateExpected(&r2)
		if in.Expected, err = MarshalCSharpG1Expected(r2); err != nil {
			t.Fatal(err)
		}
	}
	data, err := MarshalCSharpG1Audit76(x)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCSharpG1Audit76(data, in)
	return err
}

func g1a76Alt(x *CSharpG1Expected, id string) *G1ExpAlt {
	for i := range x.Alternatives {
		if x.Alternatives[i].ID == id {
			return &x.Alternatives[i]
		}
	}
	return nil
}

func g1a76Row(x *CSharpG1Audit76, id string) *G1Audit76Row {
	for i := range x.Rows {
		if x.Rows[i].ID == id {
			return &x.Rows[i]
		}
	}
	return nil
}

func TestCSharpG1Audit76Rejects(t *testing.T) {
	if err := g1a76Mutant(t, nil, nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		audit    func(x *CSharpG1Audit76)
		expected func(x *CSharpG1Expected)
		code     string
	}{
		// a wrong token witness: the r2 witness of a030 moves off the instance (the audit states MATCH)
		"a wrong token witness": {expected: func(x *CSharpG1Expected) { g1a76Alt(x, "csharp-B02.a030").Witnesses[0].Start = 88 }, code: "G1_AUDIT76_COMPARISON"},
		"a witness that is a containing node": {expected: func(x *CSharpG1Expected) {
			w := &g1a76Alt(x, "csharp-B02.a088").Witnesses[0]
			w.Start, w.End = 42, 51
		}, code: "G1_AUDIT76_COMPARISON"},
		"a span claim on an external extent": {expected: func(x *CSharpG1Expected) { g1a76Alt(x, "csharp-B01.a381").Witnesses[0].Match = "span" }, code: "G1_AUDIT76_COMPARISON"},
		// a row whose result disagrees with r2
		"a row whose result disagrees": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a021").Result = G1AuditMismatch }, code: "G1_AUDIT76_COMPARISON"},
		"a row whose relation changed": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a021").Relation = G1AuditRelStartsAt }, code: "G1_AUDIT76_COMPARISON"},
		"a row whose initial changed":  {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a021").Initial = G1AuditMatch }, code: "G1_AUDIT76_COMPARISON"},
		"an instance that moved":       {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Instances[0][0]++ }, code: "G1_AUDIT76_COMPARISON"},
		"an instance outside the text": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Instances[0] = [2]int{0, 1 << 30} }, code: "G1_AUDIT76_INSTANCE"},
		"a row without a spec ground":  {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Grounds = nil }, code: "G1_AUDIT76_ROW"},
		"a spec ground of another file": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B02.a094").Grounds[0].DocSHA256 = strings.Repeat("0", 64)
		}, code: "G1_AUDIT76_ROW"},
		"a row that is not accepted":    {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Accepted = false }, code: "G1_AUDIT76_ROW"},
		"a missing row":                 {audit: func(x *CSharpG1Audit76) { x.Rows = x.Rows[1:] }, code: "G1_AUDIT76_ROWS"},
		"a node without grammar ground": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a382").Node.Grounds = nil }, code: "G1_AUDIT76_ROW"},
		"a terminal that is not there":  {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a382").Node.Leading = ";" }, code: "G1_AUDIT76_COMPARISON"},
		"counts that moved":             {audit: func(x *CSharpG1Audit76) { x.Counts.Corrected-- }, code: "G1_AUDIT76_COUNTS"},
		// removing the record of a correction: the witness before it can no longer be read
		"a removed correction record": {expected: func(x *CSharpG1Expected) {
			x.Corrections = slices.DeleteFunc(slices.Clone(x.Corrections), func(c G1Correction) bool { return c.Item == "csharp-B02.a030" })
		}, code: "G1_AUDIT76_COMPARISON"},
	} {
		err := g1a76Mutant(t, c.audit, c.expected)
		if err == nil || !strings.Contains(err.Error(), c.code) {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
	// a row that reads the expected types: the decoder knows no such member
	_, data, in := g1a76Tracked(t)
	spliced := []byte(strings.Replace(string(data), `{"id":"csharp-B02.a021",`, `{"id":"csharp-B02.a021","ts":["pointer_type"],`, 1))
	if _, err := ParseCSharpG1Audit76(spliced, in); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_JSON") {
		t.Errorf("a row with a ts member: %v", err)
	}
}

// The audit derivation must not read the expected types. The closure test proves that the derivation cannot reach native
// input; it cannot express which fields of an alternative a function reads. So this checks the audit file syntactically: no
// selector of the field TS. The comparison takes the witness and the source text only; the old record of a correction is
// read for its witness (the TS member of its record is decoded and never named).
func g1a76NamesTS(src string) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "audit76.go", src, 0)
	if err != nil {
		return false, err
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "TS" {
			found = true
		}
		if k, ok := n.(*ast.KeyValueExpr); ok {
			if id, ok := k.Key.(*ast.Ident); ok && id.Name == "TS" {
				found = true
			}
		}
		return !found
	})
	return found, nil
}

func TestCSharpG1Audit76NeverNamesTheExpectedTypes(t *testing.T) {
	src := string(mustRead(t, "csharp_g1_audit76.go"))
	if named, err := g1a76NamesTS(src); err != nil || named {
		t.Fatalf("the audit file names the field TS (%v, %v)", named, err)
	}
	// mutants: a row that reads the types, by selector and by composite literal
	for name, snippet := range map[string]string{
		"selector": "func f(a G1ExpAlt) int { return len(a.TS) }",
		"literal":  "func f() any { return G1ExpAlt{TS: nil} }",
	} {
		if named, err := g1a76NamesTS("package kit\n" + snippet); err != nil || !named {
			t.Errorf("%s: a read of TS was not found (%v, %v)", name, named, err)
		}
	}
}

// The stratum of the differential-oracle alternatives counts the full audit, all 76, and states the exact bound.
func TestCSharpG1AuditStrataCountTheFullAudit(t *testing.T) {
	x, _, _ := g1a76Tracked(t)
	r2, err := DecodeCSharpG1Expected(mustRead(t, "../contracts/csharp-g1-expected-b01b02-r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var diff *G1AuditStratum
	rows := G1AuditStrata(r2, x.Rows)
	for i := range rows {
		if rows[i].Stratum == G1StratumDifferential {
			diff = &rows[i]
		}
	}
	if diff == nil || diff.Samples != 76 || diff.Mismatches != 0 || diff.Found != 11 || diff.Upper95 != "3.9" || diff.Upper95Found != strconv.FormatFloat(G1AuditUpper95(11, 76), 'f', 1, 64) {
		t.Fatalf("differential stratum: %+v", diff)
	}
	if got := rows[len(rows)-1]; got.Stratum != "all samples" || got.Samples != 16+76+5+1 || got.Found != 11 {
		t.Errorf("all samples: %+v", got)
	}
	// a mismatching row moves the bound of its stratum
	bad := slices.Clone(x.Rows)
	bad[0].Result = G1AuditMismatch
	for _, g := range G1AuditStrata(r2, bad) {
		if g.Stratum == G1StratumDifferential && (g.Mismatches != 1 || g.Upper95 != strconv.FormatFloat(G1AuditUpper95(1, 76), 'f', 1, 64)) {
			t.Errorf("one mismatch in 76: %+v", g)
		}
	}
	// without the full audit the samples stay
	for _, g := range G1AuditStrata(r2, nil) {
		if g.Stratum == G1StratumDifferential && (g.Samples != 4 || g.Found != 0) {
			t.Errorf("sample audit: %+v", g)
		}
	}
}

// When the pinned documents are at hand, every ground of the audit quotes them.
func TestCSharpG1Audit76GroundsQuoteTheDocuments(t *testing.T) {
	x, _, _ := g1a76Tracked(t)
	g, s := os.Getenv("TSGK_G1_GRAMMAR_JS"), os.Getenv("TSGK_G1_SPEC_DIR")
	if g == "" || s == "" {
		t.Log("TSGK_G1_GRAMMAR_JS and TSGK_G1_SPEC_DIR not set: audit grounds not checked against the documents (go run ./src/dev/csharp-g1-audit76 -grammar … -spec … does)")
		return
	}
	spec := map[string]string{}
	for _, name := range G1SpecFiles {
		spec[name] = string(mustRead(t, s+"/"+name))
	}
	if err := CheckG1Audit76Grounds(x.Rows, string(mustRead(t, g)), spec); err != nil {
		t.Error(err)
	}
}

// A witness correction must point at a place Roslyn observed, with types the kind map gives that kind.
func TestCSharpG1CorrectedWitnessesAreObserved(t *testing.T) {
	a := g1cLoad(t)
	for name, mut := range map[string]func(w *G1ExpWitness, ts *[]string){
		"a span nobody observed": func(w *G1ExpWitness, ts *[]string) { w.End++ },
		"a kind that is not the span's": func(w *G1ExpWitness, ts *[]string) {
			w.Roslyn = "OutKeyword"
		},
		"a type the map does not give": func(w *G1ExpWitness, ts *[]string) { *ts = []string{"pointer_type"} },
	} {
		x, err := DecodeCSharpG1Expected(a.r2Exp)
		if err != nil {
			t.Fatal(err)
		}
		x = g1cCloneExpected(x)
		x.Corrections = slices.Clone(x.Corrections)
		for i, c := range x.Corrections {
			if c.Item != "csharp-B02.a030" || c.Field != G1FieldEvidence {
				continue
			}
			var n g1cEvidence
			if err := json.Unmarshal([]byte(c.New), &n); err != nil {
				t.Fatal(err)
			}
			mut(&n.Witness, &n.TS)
			x.Corrections[i].New = g1cJSON(n)
			g1a76Alt(&x, c.Item).TS, g1a76Alt(&x, c.Item).Witnesses[0] = n.TS, n.Witness
		}
		data, err := MarshalCSharpG1Expected(x)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1ExpectedR2(data, a.expIn); err == nil || !strings.Contains(err.Error(), "G1_CORRECTION_WITNESS") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
