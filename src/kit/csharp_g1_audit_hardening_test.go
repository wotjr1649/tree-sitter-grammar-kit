package kit

import (
	"slices"
	"strings"
	"testing"
)

// Tests added after the review of the audits (#233, reviewG1c-1): the grounds are pinned without the documents, the instances
// are checked against the source text, and the mutants the review found alive are killed.

// g1h76Case is a mutation of the position audit and the code its parse must give.
type g1h76Case struct {
	audit    func(x *CSharpG1Audit76)
	expected func(x *CSharpG1Expected)
	code     string
}

func g1h76Run(t *testing.T, cases map[string]g1h76Case) {
	t.Helper()
	for name, c := range cases {
		err := g1a76Mutant(t, c.audit, c.expected)
		if err == nil || !strings.Contains(err.Error(), c.code) {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
}

// Finding 2: a ground that was invented, copied from another row, swapped or edited fails without the documents.
func TestCSharpG1Audit76GroundsArePinnedWithoutTheDocuments(t *testing.T) {
	typ, _, _ := g1t76Tracked(t)
	g1h76Run(t, map[string]g1h76Case{
		// the probe of the review: a quote that is long enough and has nothing to do with the production
		"a fabricated quote": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B02.a094").Grounds[0].Quote = "this quote is entirely invented and it is irrelevant to the production"
		}, code: "G1_AUDIT76_GROUND"},
		"a short quote": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Grounds[0].Quote = "too short" }, code: "G1_AUDIT76_ROW"},
		// the other probe: the ground of another row, which is a real quote
		"a ground copied from another row": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B02.a094").Grounds = slices.Clone(g1a76Row(x, "csharp-B02.a021").Grounds)
		}, code: "G1_AUDIT76_GROUND"},
		"grounds swapped between two rows": {audit: func(x *CSharpG1Audit76) {
			a, b := g1a76Row(x, "csharp-B02.a094"), g1a76Row(x, "csharp-B02.a021")
			a.Grounds, b.Grounds = b.Grounds, a.Grounds
		}, code: "G1_AUDIT76_GROUND"},
		"an edited quote (a word changed)": {audit: func(x *CSharpG1Audit76) {
			g := &g1a76Row(x, "csharp-B01.a378").Grounds[0]
			g.Quote = strings.Replace(g.Quote, "interpolated_regular_string_expression", "interpolated_regular_string_expressions", 1)
		}, code: "G1_AUDIT76_GROUND"},
		"an edited heading": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a378").Grounds[0].Heading += "x" }, code: "G1_AUDIT76_GROUND"},
		"an edited gap":     {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a378").Grounds[0].Gap++ }, code: "G1_AUDIT76_GROUND"},
		"an edited place":   {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B01.a378").Grounds[0].At += "x" }, code: "G1_AUDIT76_GROUND"},
		"a ground added to a row": {audit: func(x *CSharpG1Audit76) {
			r := g1a76Row(x, "csharp-B01.a378")
			r.Grounds = append(slices.Clone(r.Grounds), r.Grounds[0])
		}, code: "G1_AUDIT76_GROUND"},
		"an edited node ground": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B01.a382").Node.Grounds[0].Quote += " and more text of no document"
		}, code: "G1_AUDIT76_GROUND"},
		// a grammar ground in a row that wants the specification: a valid ground of the pinned grammar.js of the type audit
		"a grammar ground in a specification row": {audit: func(x *CSharpG1Audit76) { g1a76Row(x, "csharp-B02.a094").Grounds = slices.Clone(typ.Rows[0].Grounds) }, code: "G1_AUDIT76_ROW"},
	})
	// the same for the type audit
	for name, c := range map[string]struct {
		audit func(x *CSharpG1TypeAudit76)
		code  string
	}{
		"a fabricated quote": {func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a008").Grounds[0].Quote = "this quote is entirely invented and it is irrelevant to the production"
		}, "G1_TYPEAUDIT76_GROUND"},
		"a ground copied from another row": {func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a008").Grounds = slices.Clone(g1t76Row(x, "csharp-B02.a016").Grounds)
		}, "G1_TYPEAUDIT76_GROUND"},
		"an edited gap": {func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a008").Grounds[0].Gap++ }, "G1_TYPEAUDIT76_GROUND"},
	} {
		if err := g1t76Mutant(t, c.audit, nil); err == nil || !strings.Contains(err.Error(), c.code) {
			t.Errorf("type audit, %s: %v, want %s", name, err, c.code)
		}
	}
	// the tables hold exactly the tracked rows (a stale entry or a missing one is a change of an audit without its pin)
	pos, _, _ := g1a76Tracked(t)
	if got := G1Audit76GroundPins(pos.Rows); len(got) != len(g1a76GroundPins) || !mapsEqual(got, g1a76GroundPins) {
		t.Error("the position pin table is not the table of the tracked audit")
	}
	if got := G1TypeAudit76GroundPins(typ.Rows); !mapsEqual(got, g1t76GroundPins) {
		t.Error("the type pin table is not the table of the tracked audit")
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Finding 3: an instance is checked against the source text by the rule of its row.
func TestCSharpG1Audit76InstancesAreCheckedAgainstTheText(t *testing.T) {
	x, _, in := g1a76Tracked(t)
	// the rule table holds exactly the rows, and every listed instance of the tracked audit satisfies its rule
	if len(g1a76Rules) != len(x.Rows) {
		t.Fatalf("%d rules for %d rows", len(g1a76Rules), len(x.Rows))
	}
	for _, r := range x.Rows {
		if g1a76Rules[r.ID] == nil {
			t.Errorf("no rule for %s", r.ID)
		}
		for _, i := range r.Instances {
			if !g1a76Rules[r.ID].accepts(string(in.Texts[r.Source][i[0]:i[1]])) {
				t.Errorf("%s: the instance %v does not satisfy its rule", r.ID, i)
			}
		}
	}
	// the probe of the review: the witness of a030 (the token ?) moves to the text "n" and the instance list names that place; the
	// audit is derived afresh (so its result says MATCH and the comparison fields agree) and the rule still refuses it
	r2, err := DecodeCSharpG1Expected(g1cLoad(t).r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	r2 = g1cCloneExpected(r2)
	a := g1a76Alt(&r2, "csharp-B02.a030")
	a.Witnesses[0].Start, a.Witnesses[0].End = 3, 4
	mutated, err := MarshalCSharpG1Expected(r2)
	if err != nil {
		t.Fatal(err)
	}
	moved := g1a76Sources(t, mutated)
	if text := moved.Texts[a.Witnesses[0].Source]; string(text[3:4]) == "?" {
		t.Fatalf("the moved place holds %q", text[3:4])
	}
	rows := slices.Clone(x.Rows)
	for i := range rows {
		if rows[i].ID == "csharp-B02.a030" {
			rows[i].Instances = [][2]int{{3, 4}}
		}
	}
	d, err := DeriveCSharpG1Audit76(rows, moved)
	if err != nil {
		t.Fatal(err)
	}
	if got := g1a76Row(&d, "csharp-B02.a030"); got.Result != G1AuditMatch {
		t.Fatalf("the probe does not reach MATCH: %+v", got)
	}
	data, err := MarshalCSharpG1Audit76(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Audit76(data, moved); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_RULE") {
		t.Errorf("a moved instance with its witness: %v", err)
	}
	// instances in a wrong order, twice, off the text, or of another source
	g1h76Run(t, map[string]g1h76Case{
		"an instance list that is not sorted": {audit: func(x *CSharpG1Audit76) {
			r := g1a76Row(x, "csharp-B01.a378")
			r.Instances = [][2]int{r.Instances[1], r.Instances[0], r.Instances[2]}
		}, code: "G1_AUDIT76_INSTANCE"},
		"an instance twice": {audit: func(x *CSharpG1Audit76) {
			r := g1a76Row(x, "csharp-B01.a378")
			r.Instances = [][2]int{r.Instances[0], r.Instances[0], r.Instances[2]}
		}, code: "G1_AUDIT76_INSTANCE"},
		"an empty instance": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B02.a094").Instances[0][1] = g1a76Row(x, "csharp-B02.a094").Instances[0][0]
		}, code: "G1_AUDIT76_INSTANCE"},
		"an instance that is another token": {audit: func(x *CSharpG1Audit76) {
			// the text of the first instance of a047 (int) moves one character on: "nt " is no keyword int
			r := g1a76Row(x, "csharp-B02.a047")
			r.Instances[0][0]++
			r.Instances[0][1]++
		}, code: "G1_AUDIT76_RULE"},
		// a row of another source: the witness the row is compared with is that of its own alternative
		"a row of another source": {audit: func(x *CSharpG1Audit76) {
			g1a76Row(x, "csharp-B02.a094").Source = g1a76Row(x, "csharp-B01.a378").Source
		}, code: "G1_AUDIT76_WITNESS"},
	})
	// the rules refuse the neighbours of the form (a text that is near the form and is not an instance of it)
	for _, c := range []struct {
		id   string
		ok   []string
		deny []string
	}{
		{"csharp-B02.a030", []string{"?"}, []string{"n", "??", "", " ?"}},
		{"csharp-B02.a047", []string{"int"}, []string{"uint", "ints", " int", "in", "Int"}},
		{"csharp-B02.a043", []string{"sbyte"}, []string{"byte", "usbyte"}},
		{"csharp-B02.a044", []string{"byte"}, []string{"sbyte", "bytes"}},
		{"csharp-B02.a008", []string{"int*", "int**", "void*", "Ptr*"}, []string{"int", "*", "int*[", "1*"}},
		{"csharp-B02.a090", []string{"int*", "void**", "Ptr*"}, []string{"void*", "int"}},
		{"csharp-B02.a092", []string{"int**", "Ptr*"}, []string{"void*", "void**"}},
		{"csharp-B02.a091", []string{"void*"}, []string{"void**", "void", "int*"}},
		{"csharp-B02.a093", []string{"void**", "void***"}, []string{"void*", "int**"}},
		{"csharp-B02.a094", []string{"void*"}, []string{"void", "void**", "*"}},
		{"csharp-B02.a064", []string{"in v"}, []string{"ref v", "in", "in "}},
		{"csharp-B02.a065", []string{"ref v"}, []string{"in v", "ref"}},
		{"csharp-B02.a086", []string{"ref", "out", "in"}, []string{"refs", "readonly"}},
		{"csharp-B01.a383", []string{`$"`}, []string{`$@"`, `@$"`, `"`, `$`}},
		{"csharp-B01.a396", []string{`$@"`}, []string{`@$"`, `$"`}},
		{"csharp-B01.a397", []string{`@$"`}, []string{`$@"`, `$"`}},
		{"csharp-B01.a386", []string{`"`}, []string{`""`, ``, `'`}},
		{"csharp-B01.a381", []string{"x", "y,5", "x,-5:N0"}, []string{"{x}", "", "{"}},
		{"csharp-B01.a382", []string{"5", "-5", "3"}, []string{"x", "5,", "--5"}},
		{"csharp-B01.a384", []string{"a", "d{{e}}f", `\t\x41\u0042{{}}`}, []string{`a"b`, "a{b", `\q`, ""}},
		{"csharp-B01.a398", []string{`C:\`, `""q""{{x}}`, `\n`}, []string{`"q"`, "a{b", ""}},
		{"csharp-B01.a385", []string{":F2"}, []string{"F2", ":", `:"`}},
		{"csharp-B01.a378", []string{`$"a{x}b"`, `$""`}, []string{`$@"a"`, `"a"`, `$"a`}},
		{"csharp-B02.a021", []string{"int*"}, []string{"int", "int[]"}},
		{"csharp-B02.a028", []string{"[]", "[,,]"}, []string{"[", "[x]", ""}},
		{"csharp-B02.a083", []string{"new()"}, []string{"new", "new(x)"}},
	} {
		rule := g1a76Rules[c.id]
		for _, s := range c.ok {
			if !rule.accepts(s) {
				t.Errorf("%s: the rule refuses %q", c.id, s)
			}
		}
		for _, s := range c.deny {
			if rule.accepts(s) {
				t.Errorf("%s: the rule accepts %q", c.id, s)
			}
		}
	}
	// a row with no rule accepts nothing
	var none *g1a76Rule
	if none.accepts("x") {
		t.Error("a nil rule accepts")
	}
}

// Finding 4: mutants of the position audit that no test killed.
func TestCSharpG1Audit76KillsTheMutantsOfTheReview(t *testing.T) {
	cases := map[string]g1h76Case{}
	// A03: the five starts-at witnesses leave the start of their instance (the witness moves by one character)
	for _, id := range []string{"csharp-B01.a381", "csharp-B01.a383", "csharp-B01.a395", "csharp-B01.a396", "csharp-B01.a397"} {
		cases["a starts-at witness off the start: "+id] = g1h76Case{expected: func(x *CSharpG1Expected) {
			w := &g1a76Alt(x, id).Witnesses[0]
			if w.Match != "starts-at" {
				t.Fatalf("%s is not a starts-at witness", id)
			}
			w.Start++
			w.End++
		}, code: "G1_AUDIT76_COMPARISON"}
	}
	// a span witness that is a starts-at one (the row states MATCH for the weaker claim)
	cases["a starts-at witness that claims a span of the right start"] = g1h76Case{expected: func(x *CSharpG1Expected) {
		g1a76Alt(x, "csharp-B01.a383").Witnesses[0].Match = "span"
	}, code: "G1_AUDIT76_COMPARISON"}
	// A17, A23: the basis and the schema of the asset
	cases["a changed basis"] = g1h76Case{audit: func(x *CSharpG1Audit76) { x.Basis += " " }, code: "G1_AUDIT76_SCHEMA"}
	cases["a changed schema"] = g1h76Case{audit: func(x *CSharpG1Audit76) { x.Schema = "tsgk-csharp-g1-audit76/r2" }, code: "G1_AUDIT76_SCHEMA"}
	g1h76Run(t, cases)
	// A18, T14: the bytes are the canonical bytes (spaces, a final newline, another order of members)
	_, data, in := g1a76Tracked(t)
	for name, mutated := range map[string][]byte{
		"a trailing blank line":  append(slices.Clone(data), '\n'),
		"a space after a colon":  []byte(strings.Replace(string(data), `"schema": "tsgk`, `"schema":  "tsgk`, 1)),
		"no final newline":       data[:len(data)-1],
		"a row indented further": []byte(strings.Replace(string(data), "\n  {\"id\":\"csharp-B01.a379\"", "\n   {\"id\":\"csharp-B01.a379\"", 1)),
		"members reordered":      []byte(strings.Replace(string(data), `"id":"csharp-B01.a378","source":"alt-csharp-B01-09#0"`, `"source":"alt-csharp-B01-09#0","id":"csharp-B01.a378"`, 1)),
	} {
		if _, err := ParseCSharpG1Audit76(mutated, in); err == nil || !strings.Contains(err.Error(), "G1_AUDIT76_BYTES") {
			t.Errorf("position audit, %s: %v", name, err)
		}
	}
	_, tdata, tin := g1t76Tracked(t)
	for name, mutated := range map[string][]byte{
		"a trailing blank line": append(slices.Clone(tdata), '\n'),
		"a space after a colon": []byte(strings.Replace(string(tdata), `"schema": "tsgk`, `"schema":  "tsgk`, 1)),
		"no final newline":      tdata[:len(tdata)-1],
	} {
		if _, err := ParseCSharpG1TypeAudit76(mutated, tin); err == nil || !strings.Contains(err.Error(), "G1_TYPEAUDIT76_BYTES") {
			t.Errorf("type audit, %s: %v", name, err)
		}
	}
}

// Finding 4, type audit: T11, T02, T06.
func TestCSharpG1TypeAudit76KillsTheMutantsOfTheReview(t *testing.T) {
	x, _, _ := g1t76Tracked(t)
	// T11: a row that lists coincident nodes still needs the node types it derived: a ts with the coincident ones only is no match
	n := 0
	for _, r := range x.Rows {
		if len(r.Coincident) == 0 {
			continue
		}
		n++
		id, coincident := r.ID, slices.Clone(r.Coincident)
		err := g1t76Mutant(t, nil, func(e *CSharpG1Expected) { g1a76Alt(e, id).TS = coincident })
		if err == nil || !strings.Contains(err.Error(), "G1_TYPEAUDIT76_COMPARISON") {
			t.Errorf("%s: a ts of the coincident nodes only: %v", id, err)
		}
		// and with the derived types and the coincident ones it is a match (the relation the row states)
		if err := g1t76Mutant(t, nil, func(e *CSharpG1Expected) {
			g1a76Alt(e, id).TS = slices.Concat(slices.Clone(r.Types), coincident)
		}); err != nil && !strings.Contains(err.Error(), "G1_TYPEAUDIT76_COMPARISON") {
			t.Errorf("%s: %v", id, err)
		}
	}
	if n == 0 {
		t.Fatal("no row with coincident nodes")
	}
	// T02: a row the grammar text leaves open keeps at least two candidates and no coincident node
	// T06: a coincident node is a node type that is not a derived one
	for name, c := range map[string]func(x *CSharpG1TypeAudit76){
		"a not derivable row with one candidate": func(x *CSharpG1TypeAudit76) { g1t76Row(x, "csharp-B02.a084").Types = []string{`"ref"`} },
		"a not derivable row with a coincident node": func(x *CSharpG1TypeAudit76) {
			g1t76Row(x, "csharp-B02.a084").Coincident = []string{"parameter"}
		},
		"a coincident node that is the derived type": func(x *CSharpG1TypeAudit76) {
			r := g1t76Row(x, "csharp-B02.a008")
			r.Coincident = []string{r.Types[0]}
		},
	} {
		if err := g1t76Mutant(t, c, nil); err == nil || !strings.Contains(err.Error(), "G1_TYPEAUDIT76_ROW") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Finding 5 and the report: the stratum of the differential-oracle alternatives counts the corrections the native comparison
// found (csharp-B01.a383, a396, a397) with the ones the audit found, and states the bound both ways; every row of the report
// is pinned, so a change of how the totals are built shows.
func TestCSharpG1AuditStrataCountTheNativeFoundCorrectionsToo(t *testing.T) {
	pos, _, _ := g1a76Tracked(t)
	typ, _, _ := g1t76Tracked(t)
	r2, err := DecodeCSharpG1Expected(g1cLoad(t).r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	want := []G1AuditStratum{
		{Stratum: "spec-derived", Samples: 16, Compared: true, Upper95: "17.1", LaterWrong: 1, Upper95Later: "26.4"},
		{Stratum: "differential-oracle", Samples: 76, Compared: true, Upper95: "3.9", Upper95Later: "3.9",
			Found: 14, FoundAudit: 11, FoundNative: 3, Upper95Found: "27.3", Upper95FoundAudit: "22.8",
			TypeSamples: 76, TypeFound: 2, TypeNotDerivable: 3, TypeUpper95: "3.9", TypeUpper95Found: "8.1"},
		{Stratum: "blocked", Samples: 5, Compared: true, Upper95: "45.1", Upper95Later: "45.1"},
		{Stratum: "coverage-gap", Samples: 1},
		{Stratum: "expected values compared (spec-derived and differential-oracle)", Samples: 92, Compared: true, Upper95: "3.2", LaterWrong: 1, Upper95Later: "5.1",
			Found: 14, FoundAudit: 11, FoundNative: 3, Upper95Found: "24.0", Upper95FoundAudit: "20.3"},
		{Stratum: "all samples", Samples: 98, Compared: true, Upper95: "3.0", LaterWrong: 1, Upper95Later: "4.7",
			Found: 14, FoundAudit: 11, FoundNative: 3, Upper95Found: "22.6", Upper95FoundAudit: "19.1"},
	}
	got := G1AuditStrataWithTypes(r2, pos.Rows, typ.Rows)
	if !slices.Equal(got, want) {
		for i := range got {
			if i < len(want) && got[i] != want[i] {
				t.Errorf("row %d:\n got %+v\nwant %+v", i, got[i], want[i])
			}
		}
		t.Fatalf("%d rows, want %d", len(got), len(want))
	}
	// the three are the alternatives with a witness correction after the native result that the audit derived as matching at first
	native := map[string]bool{}
	audit := map[string]bool{}
	for _, c := range r2.Corrections {
		if c.AfterNative && c.Field == G1FieldWitnessMatch {
			native[c.Item] = true
		}
	}
	for _, r := range pos.Rows {
		if r.Initial == G1AuditMismatch {
			audit[r.ID] = true
		}
	}
	var ids []string
	for id := range native {
		if !audit[id] {
			ids = append(ids, strings.TrimPrefix(id, "csharp-"))
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"B01.a383", "B01.a396", "B01.a397"}) {
		t.Errorf("native-found corrections %v", ids)
	}
	// without the witness correction of one of them the stratum counts one fewer
	r2m := g1cCloneExpected(r2)
	r2m.Corrections = slices.DeleteFunc(slices.Clone(r2m.Corrections), func(c G1Correction) bool {
		return c.Item == "csharp-B01.a396" && c.Field == G1FieldWitnessMatch
	})
	for _, g := range G1AuditStrata(r2m, pos.Rows) {
		if g.Stratum == G1StratumDifferential && (g.Found != 13 || g.FoundNative != 2 || g.FoundAudit != 11) {
			t.Errorf("without one native correction: %+v", g)
		}
	}
}
