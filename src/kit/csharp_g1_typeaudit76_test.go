package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// g1t76Input is the input of the type audit made from an r2 asset (the bytes may be a mutant of the tracked one).
func g1t76Input(t *testing.T, expected []byte) G1TypeAudit76Input {
	t.Helper()
	in, err := G1TypeAudit76View(expected)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func g1t76Tracked(t *testing.T) (CSharpG1TypeAudit76, []byte, G1TypeAudit76Input) {
	t.Helper()
	data := mustRead(t, "../contracts/csharp-g1-typeaudit76-r1.json")
	in := g1t76Input(t, g1cLoad(t).r2Exp)
	x, err := ParseCSharpG1TypeAudit76(data, in)
	if err != nil {
		t.Fatal(err)
	}
	return x, data, in
}

func g1t76Row(x *CSharpG1TypeAudit76, id string) *G1TypeAudit76Row {
	for i := range x.Rows {
		if x.Rows[i].ID == id {
			return &x.Rows[i]
		}
	}
	return nil
}

// g1t76Mutant parses the type audit after mutate changed the decoded asset (re-marshalled canonically) or the r2 asset.
func g1t76Mutant(t *testing.T, mutateAudit func(x *CSharpG1TypeAudit76), mutateExpected func(x *CSharpG1Expected)) error {
	t.Helper()
	x, _, in := g1t76Tracked(t)
	if mutateAudit != nil {
		x.Rows = slices.Clone(x.Rows)
		for i := range x.Rows {
			x.Rows[i].Types = slices.Clone(x.Rows[i].Types)
			x.Rows[i].Grounds = slices.Clone(x.Rows[i].Grounds)
		}
		mutateAudit(&x)
	}
	if mutateExpected != nil {
		r2, err := DecodeCSharpG1Expected(g1cLoad(t).r2Exp)
		if err != nil {
			t.Fatal(err)
		}
		r2 = g1cCloneExpected(r2)
		mutateExpected(&r2)
		mutated, err := MarshalCSharpG1Expected(r2)
		if err != nil {
			t.Fatal(err)
		}
		in = g1t76Input(t, mutated)
	}
	data, err := MarshalCSharpG1TypeAudit76(x)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCSharpG1TypeAudit76(data, in)
	return err
}

// The tracked type audit parses, its rows are exactly the 76 differential-oracle alternatives of r2, the comparison it states is
// the fresh one, and the classes are what the grammar text allows: the three alternatives that the text leaves open keep their
// candidate set, and the external scanner rows are marked as type only.
func TestCSharpG1TypeAudit76IsConsistentWithR2(t *testing.T) {
	x, _, in := g1t76Tracked(t)
	alts := in.Alts
	if len(alts) != 76 || len(x.Rows) != 76 {
		t.Fatalf("alternatives %d, rows %d", len(alts), len(x.Rows))
	}
	want := G1TypeAudit76Counts{Rows: 76, Match: 76, Mismatch: 0, Node: 55, NoOwnNode: 5, Alias: 6, External: 7, NotDerivable: 3, Corrected: 2}
	if x.Counts != want {
		t.Fatalf("counts %+v, want %+v", x.Counts, want)
	}
	ids := func(pick func(r G1TypeAudit76Row) bool) []string {
		var out []string
		for _, r := range x.Rows {
			if pick(r) {
				out = append(out, strings.TrimPrefix(r.ID, "csharp-"))
			}
		}
		return out
	}
	// the type audit found two rows whose ts the grammar text contradicted; r2 corrected them
	if got := ids(func(r G1TypeAudit76Row) bool { return r.Initial == G1AuditMismatch }); !slices.Equal(got, []string{"B02.a088", "B02.a089"}) {
		t.Errorf("corrected rows %v", got)
	}
	// the grammar text does not settle the parameter ref: modifier or the first token of ref_type; the candidate set is kept
	for _, r := range x.Rows {
		if r.Class != G1TypeNotDerivable {
			continue
		}
		if !slices.Equal(r.Types, []string{`"ref"`, "modifier"}) || r.Relation != G1TypeRelCandidates {
			t.Errorf("%s: %+v", r.ID, r)
		}
	}
	if got := ids(func(r G1TypeAudit76Row) bool { return r.Class == G1TypeNotDerivable }); !slices.Equal(got, []string{"B02.a084", "B02.a086", "B02.a087"}) {
		t.Errorf("not derivable rows %v", got)
	}
	// an external scanner token has its type in the grammar text and no extent there
	if got := ids(func(r G1TypeAudit76Row) bool { return r.Class == G1TypeExternal }); !slices.Equal(got, []string{"B01.a383", "B01.a384", "B01.a386", "B01.a396", "B01.a397", "B01.a398", "B01.a400"}) {
		t.Errorf("external rows %v", got)
	}
	for _, r := range x.Rows {
		if (r.Extent == G1TypeExtentElsewhere) != (r.Class == G1TypeExternal) || r.Result != G1AuditMatch || len(r.Grounds) == 0 {
			t.Errorf("row %s: %+v", r.ID, r)
		}
		for _, g := range r.Grounds {
			if g.Doc != G1DocGrammar || g.DocSHA256 != g1gGrammarSHA {
				t.Errorf("row %s: a ground that is not the pinned grammar.js: %+v", r.ID, g)
			}
		}
	}
}

func TestCSharpG1TypeAudit76Rejects(t *testing.T) {
	if err := g1t76Mutant(t, nil, nil); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		audit    func(x *CSharpG1TypeAudit76)
		expected func(x *CSharpG1Expected)
		code     string
	}{
		// a wrong type: the row derives another node than the r2 asset holds
		"a wrong type": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Types = []string{"array_type"} }, code: "G1_TYPEAUDIT76_COMPARISON"},
		"a wrong type in r2 (a node for a token)": {expected: func(x *CSharpG1Expected) {
			g1a76Alt(x, "csharp-B02.a016").TS = []string{"predefined_type", "identifier"}
		}, code: "G1_TYPEAUDIT76_COMPARISON"},
		"a wrong type in r2 (replaced)": {expected: func(x *CSharpG1Expected) { g1a76Alt(x, "csharp-B02.a016").TS = []string{"identifier"} }, code: "G1_TYPEAUDIT76_COMPARISON"},
		// the decision: the candidate set of a row the grammar text leaves open is kept, neither narrowed nor widened
		"a narrowed candidate set": {expected: func(x *CSharpG1Expected) { g1a76Alt(x, "csharp-B02.a084").TS = []string{"modifier"} }, code: "G1_TYPEAUDIT76_COMPARISON"},
		"a widened candidate set": {expected: func(x *CSharpG1Expected) {
			g1a76Alt(x, "csharp-B02.a087").TS = []string{`"ref"`, "modifier", "parameter"}
		}, code: "G1_TYPEAUDIT76_COMPARISON"},
		// the two corrected rows: the anonymous token comes back
		"the anonymous out back in r2": {expected: func(x *CSharpG1Expected) { g1a76Alt(x, "csharp-B02.a088").TS = []string{`"out"`, "modifier"} }, code: "G1_TYPEAUDIT76_COMPARISON"},
		// a row that states the wrong result, relation or initial
		"a row whose result disagrees": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Result = G1AuditMismatch }, code: "G1_TYPEAUDIT76_COMPARISON"},
		"a row whose relation changed": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Relation = G1TypeRelCoincident }, code: "G1_TYPEAUDIT76_COMPARISON"},
		"a row whose initial changed":  {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a088").Initial = G1AuditMatch }, code: "G1_TYPEAUDIT76_COMPARISON"},
		// removing the record of the ts correction: the ts before it can no longer be read
		"a removed correction record": {expected: func(x *CSharpG1Expected) {
			x.Corrections = slices.DeleteFunc(slices.Clone(x.Corrections), func(c G1Correction) bool {
				return c.Item == "csharp-B02.a089" && c.Field == G1FieldTS
			})
		}, code: "G1_TYPEAUDIT76_COMPARISON"},
		// rows
		"a missing row":      {audit: func(x *CSharpG1TypeAudit76) { x.Rows = x.Rows[1:] }, code: "G1_TYPEAUDIT76_ROWS"},
		"a missing last row": {audit: func(x *CSharpG1TypeAudit76) { x.Rows = x.Rows[:len(x.Rows)-1] }, code: "G1_TYPEAUDIT76_ROWS"},
		"a swapped row":      {audit: func(x *CSharpG1TypeAudit76) { x.Rows[0], x.Rows[1] = x.Rows[1], x.Rows[0] }, code: "G1_TYPEAUDIT76_ROWS"},
		"an unknown class":   {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Class = "GUESS" }, code: "G1_TYPEAUDIT76_ROW"},
		"two derived types": {audit: func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a008").Types = []string{"array_type", "pointer_type"}
		}, code: "G1_TYPEAUDIT76_ROW"},
		"an unsorted candidate":  {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a084").Types = []string{"modifier", `"ref"`} }, code: "G1_TYPEAUDIT76_ROW"},
		"a type that is no name": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Types = []string{"Pointer Type"} }, code: "G1_TYPEAUDIT76_ROW"},
		"a row without a ground": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Grounds = nil }, code: "G1_TYPEAUDIT76_ROW"},
		"a specification ground": {audit: func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a008").Grounds[0].Doc = G1DocSpec
		}, code: "G1_TYPEAUDIT76_ROW"},
		"a ground of another document": {audit: func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a008").Grounds[0].DocSHA256 = strings.Repeat("0", 64)
		}, code: "G1_TYPEAUDIT76_ROW"},
		"a row without a reason": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Reason = "" }, code: "G1_TYPEAUDIT76_ROW"},
		// an external scanner row audits the type only, and only such a row says so
		"an external row without the extent mark": {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B01.a384").Extent = "" }, code: "G1_TYPEAUDIT76_ROW"},
		"an extent mark on a node row":            {audit: func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Extent = G1TypeExtentElsewhere }, code: "G1_TYPEAUDIT76_ROW"},
		"counts that moved":                       {audit: func(x *CSharpG1TypeAudit76) { x.Counts.External-- }, code: "G1_TYPEAUDIT76_COUNTS"},
		"a changed basis":                         {audit: func(x *CSharpG1TypeAudit76) { x.Basis += " " }, code: "G1_TYPEAUDIT76_SCHEMA"},
	} {
		err := g1t76Mutant(t, c.audit, c.expected)
		if err == nil || !strings.Contains(err.Error(), c.code) {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
	// a row that reads another source of types: the decoder knows no such member
	_, data, in := g1t76Tracked(t)
	for name, member := range map[string]string{"kind map types": `"kind_map_ts":["pointer_type"],`, "a ts member": `"ts":["pointer_type"],`} {
		spliced := []byte(strings.Replace(string(data), `{"id":"csharp-B02.a008",`, `{"id":"csharp-B02.a008",`+member, 1))
		if _, err := ParseCSharpG1TypeAudit76(spliced, in); err == nil || !strings.Contains(err.Error(), "G1_TYPEAUDIT76_JSON") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The type audit must not read the kind map: the types of the 76 alternatives were translated from it, and an audit that reads
// the map would only repeat the translation. That is proved structurally (csharp_g1_audit_independence_test.go: the input type
// has no member for it, the call closure reaches no file that holds it). This is the secondary check, of the file itself: no
// identifier that names the kind map and no string that is one of its members.
func g1t76NamesKindMap(src string) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "typeaudit76.go", src, 0)
	if err != nil {
		return false, err
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			low := strings.ToLower(id.Name)
			found = found || strings.Contains(low, "kindmap") || strings.HasPrefix(low, "g1km")
		}
		if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {
			if v, err := strconv.Unquote(l.Value); err == nil && (v == "kinds" || v == "kind_map" || v == "roslyn") {
				found = true
			}
		}
		return !found
	})
	return found, nil
}

func TestCSharpG1TypeAudit76NeverReadsTheKindMap(t *testing.T) {
	src := string(mustRead(t, "csharp_g1_typeaudit76.go"))
	if named, err := g1t76NamesKindMap(src); err != nil || named {
		t.Fatalf("the type audit file names the kind map (%v, %v)", named, err)
	}
	// the inputs of the type audit are the rows and the r2 asset
	if strings.Contains(src, "Observation") || strings.Contains(src, "Executed") {
		t.Error("the type audit file names an observation or an executed asset")
	}
	// mutants: a type audit that reads the kind map, by call, by type and by the pinned path
	for name, snippet := range map[string]string{
		"call": "func f(b []byte) error { _, err := ParseCSharpG1KindMap(b, nil); return err }",
		"type": "func f(m CSharpG1KindMap) int { return len(m.Kinds) }",
		"kind": "func f(k G1KMKind) []string { return k.TS }",
		"path": "var p = CSharpG1KindMapPathR2",
		// S2 of the review: a decode of the map into a generic value and a read of its key
		"map": "func f(m map[string]any) any { return m[\"kinds\"] }",
	} {
		if named, err := g1t76NamesKindMap("package kit\n" + snippet); err != nil || !named {
			t.Errorf("%s: a read of the kind map was not found (%v, %v)", name, named, err)
		}
	}
	// the checker passes the identifiers of the file that merely belong to other words
	if named, err := g1t76NamesKindMap("package kit\nfunc f(kind int) int { return kind }"); err != nil || named {
		t.Errorf("a plain identifier was taken for the kind map (%v, %v)", named, err)
	}
}

// The position audit stays what it was: sealed bytes, with no type in it. Its decoder rejects the members of the type audit.
func TestCSharpG1PositionAuditIsFrozenAndFreeOfTypes(t *testing.T) {
	data := mustRead(t, "../contracts/csharp-g1-audit76-r1.json")
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != "d2d0a400a28b513e150d600526b81b4188fb83045f52749509c48d5ba7cf53bc" {
		t.Errorf("the position audit r1 changed: %s", got)
	}
	_, _, in := g1a76Tracked(t)
	for _, member := range []string{`"types":["pointer_type"],`, `"class":"NODE",`} {
		spliced := []byte(strings.Replace(string(data), `{"id":"csharp-B02.a021",`, `{"id":"csharp-B02.a021",`+member, 1))
		if _, err := ParseCSharpG1Audit76(spliced, in); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_JSON") {
			t.Errorf("a position row with %s: %v", member, err)
		}
	}
	// and the tracked type audit does not parse as a position audit
	if _, err := ParseCSharpG1Audit76(mustRead(t, "../contracts/csharp-g1-typeaudit76-r1.json"), in); err == nil {
		t.Error("the type audit parsed as the position audit")
	}
}

// The stratum of the differential-oracle alternatives states the position bound and the type bound apart, each from its own audit.
func TestCSharpG1AuditStrataStateTypeBoundsApart(t *testing.T) {
	pos, _, _ := g1a76Tracked(t)
	typ, _, _ := g1t76Tracked(t)
	r2, err := DecodeCSharpG1Expected(g1cLoad(t).r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	find := func(rows []G1AuditStratum, name string) G1AuditStratum {
		for _, r := range rows {
			if r.Stratum == name {
				return r
			}
		}
		t.Fatalf("no stratum %s", name)
		return G1AuditStratum{}
	}
	rows := G1AuditStrataWithTypes(r2, pos.Rows, typ.Rows)
	d := find(rows, G1StratumDifferential)
	bound := func(k int) string { return strconv.FormatFloat(G1AuditUpper95(k, 76), 'f', 1, 64) }
	// position: 76 compared, none wrong, 14 found (11 by the audit, 3 by the native comparison); type: 76 compared, none wrong, 2
	// found, 3 left open by the grammar text
	if d.Samples != 76 || d.Mismatches != 0 || d.Found != 14 || d.Upper95 != bound(0) || d.Upper95Found != bound(14) || d.Upper95FoundAudit != bound(11) {
		t.Errorf("position bound: %+v", d)
	}
	if d.TypeSamples != 76 || d.TypeMismatches != 0 || d.TypeFound != 2 || d.TypeNotDerivable != 3 || d.TypeUpper95 != bound(0) || d.TypeUpper95Found != bound(2) {
		t.Errorf("type bound: %+v", d)
	}
	// no other stratum and no total carries a type bound
	for _, r := range rows {
		if r.Stratum != G1StratumDifferential && (r.TypeSamples != 0 || r.TypeUpper95 != "") {
			t.Errorf("%s carries a type bound: %+v", r.Stratum, r)
		}
	}
	// the old report is the new one without types
	if !slices.Equal(G1AuditStrata(r2, pos.Rows), G1AuditStrataWithTypes(r2, pos.Rows, nil)) {
		t.Error("G1AuditStrata differs from the report without type rows")
	}
	// a mismatching type row moves the type bound and not the position bound
	bad := slices.Clone(typ.Rows)
	bad[0].Result = G1AuditMismatch
	g := find(G1AuditStrataWithTypes(r2, pos.Rows, bad), G1StratumDifferential)
	if g.TypeMismatches != 1 || g.TypeUpper95 != bound(1) || g.Mismatches != 0 || g.Upper95 != bound(0) {
		t.Errorf("one type mismatch in 76: %+v", g)
	}
}

// When the pinned grammar.js is at hand, every ground of the type audit quotes it.
func TestCSharpG1TypeAudit76GroundsQuoteTheGrammar(t *testing.T) {
	x, _, _ := g1t76Tracked(t)
	g := os.Getenv("TSGK_G1_GRAMMAR_JS")
	if g == "" {
		t.Log("TSGK_G1_GRAMMAR_JS not set: type audit grounds not checked against grammar.js (go run ./src/dev/csharp-g1-typeaudit76 -grammar … does)")
		return
	}
	if err := CheckG1TypeAudit76Grounds(x.Rows, string(mustRead(t, g))); err != nil {
		t.Error(err)
	}
}

// The r2 narrowing of the two parameter modifier keywords: recorded as ts corrections after the native result with grammar and
// specification grounds, and the stored native observation still matches.
func TestCSharpG1ModifierNarrowingIsRecordedAndMatches(t *testing.T) {
	t.Parallel()
	a := g1cLoad(t)
	x, err := DecodeCSharpG1Expected(a.r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"csharp-B02.a088", "csharp-B02.a089"} {
		if got := g1a76Alt(&x, item).TS; !slices.Equal(got, []string{"modifier"}) {
			t.Errorf("%s: ts %v", item, got)
		}
		n := 0
		for _, c := range x.Corrections {
			if c.Item == item && c.Field == G1FieldTS {
				n++
				if !c.AfterNative || !c.BasisIndependent || c.New != `["modifier"]` || len(c.Grounds) < 2 {
					t.Errorf("%s: %+v", item, c)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s: %d ts corrections", item, n)
		}
	}
	// the tracked r2 asset is the derivation, and a stale old value is rejected
	if _, err := ParseCSharpG1ExpectedR2(a.r2Exp, a.expIn); err != nil {
		t.Fatal(err)
	}
	stale := g1cCloneExpected(x)
	stale.Corrections = slices.Clone(stale.Corrections)
	for i, c := range stale.Corrections {
		if c.Item == "csharp-B02.a088" && c.Field == G1FieldTS {
			stale.Corrections[i].Old = `["\"out\""]`
		}
	}
	data, err := MarshalCSharpG1Expected(stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1ExpectedR2(data, a.expIn); err == nil {
		t.Error("a stale old value was accepted")
	}
	// the stored native result: both alternatives match after the narrowing
	ex, err := ParseCSharpG1Executed(mustRead(t, "../contracts/csharp-g1-executed-b01b02-r2.json"), g1cExecIn(t, a))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, alt := range ex.Alternatives {
		if alt.ID == "csharp-B02.a088" || alt.ID == "csharp-B02.a089" {
			seen++
			if alt.Result != G1ResMatch {
				t.Errorf("%s: %s", alt.ID, alt.Result)
			}
		}
	}
	if seen != 2 || ex.Counts.Mismatch != 0 {
		t.Errorf("seen %d, counts %+v", seen, ex.Counts)
	}
}
