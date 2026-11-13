package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

type g1xAssets struct {
	asset []byte
	in    CSharpG1ExecutedSources
	x     CSharpG1Executed
	ctx   g1xContext
	err   error
}

var g1xOnce sync.Once
var g1xLoaded g1xAssets

// g1xLoad reads the tracked executed asset with the sealed assets it is bound to.
func g1xLoad(t *testing.T) *g1xAssets {
	t.Helper()
	g1xOnce.Do(func() {
		a := &g1xLoaded
		l := g1Load(t)
		read := func(name string) []byte {
			data, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return data
		}
		a.asset = read("../contracts/csharp-g1-executed-b01b02-r1.json")
		a.in = CSharpG1ExecutedSources{Src: l.src, Expected: read("../contracts/csharp-g1-expected-b01b02-r1.json"),
			KindMap: read("../contracts/csharp-g1-kind-map-r1.json"), Observation: read("../contracts/csharp-g1-roslyn-observed-r2.json")}
		if a.err != nil {
			return
		}
		if a.x, a.err = DecodeCSharpG1Executed(a.asset); a.err != nil {
			return
		}
		a.ctx, a.err = g1xContextOf(a.in)
	})
	if g1xLoaded.err != nil {
		t.Fatal(g1xLoaded.err)
	}
	return &g1xLoaded
}

func TestCSharpG1ExecutedSealedAndBound(t *testing.T) {
	a := g1xLoad(t)
	if len(a.asset) != 211543 || digestHex(a.asset) != "f56eced3dd41a15977448dd19eb28047ba7b906331aef7f1f7137d6f2eca1b83" {
		t.Fatalf("executed asset changed: %d/%s", len(a.asset), digestHex(a.asset))
	}
	x, err := ParseCSharpG1Executed(a.asset, a.in)
	if err != nil {
		t.Fatal(err)
	}
	want := G1ExecCounts{Alternatives: 426, Executed: 426, Match: 422, Mismatch: 4, NotApplicable: 0, Witnesses: 626, WitnessMatch: 622, WitnessMismatch: 4, WitnessNotApplicable: 0,
		MapInside: 555, MapOutside: 3, MapFragment: 64, Sources: 72, SourcesCompleted: 72, NativeErrorAccepted: 0}
	if x.Counts != want {
		t.Fatalf("counts %+v, want %+v", x.Counts, want)
	}
	// the mismatches are exactly the four the native step reported, each with its authored cause
	causes := map[string]string{}
	for _, d := range x.Dispositions {
		causes[d.Alternative] = d.Cause
	}
	wantCauses := map[string]string{"csharp-B01.a335": G1CauseExpected, "csharp-B01.a383": G1CauseKindMap, "csharp-B01.a396": G1CauseKindMap, "csharp-B01.a397": G1CauseKindMap}
	if len(causes) != len(wantCauses) {
		t.Fatalf("dispositions %v", causes)
	}
	for id, c := range wantCauses {
		if causes[id] != c {
			t.Errorf("%s: cause %q, want %q", id, causes[id], c)
		}
	}
	// the alternatives and witnesses are the sealed ones, member by member
	exp, err := DecodeCSharpG1Expected(a.in.Expected)
	if err != nil {
		t.Fatal(err)
	}
	var sealed []G1ExpAlt
	for _, alt := range exp.Alternatives {
		if alt.Status == G1ExpExpected {
			sealed = append(sealed, alt)
		}
	}
	if len(sealed) != len(x.Alternatives) {
		t.Fatalf("%d sealed alternatives, %d executed", len(sealed), len(x.Alternatives))
	}
	for i, alt := range x.Alternatives {
		if alt.ID != sealed[i].ID || len(alt.Witnesses) != len(sealed[i].Witnesses) {
			t.Fatalf("alternative %d: %s", i, alt.ID)
		}
		for j, w := range alt.Witnesses {
			s := sealed[i].Witnesses[j]
			if w.Source != s.Source || w.Start != s.Start || w.End != s.End || w.Match != s.Match || w.Roslyn != s.Roslyn {
				t.Fatalf("%s witness %d differs from the sealed one", alt.ID, j)
			}
		}
	}
	// a witness that is a whole Roslyn token or node matches a type the kind map lists for it, except
	// the three the kind map itself got wrong
	outside := 0
	for _, alt := range x.Alternatives {
		for _, w := range alt.Witnesses {
			if w.Map == "OUTSIDE_MAP" {
				outside++
				if alt.Result != G1ResMismatch {
					t.Errorf("%s: a matching witness outside the kind map", alt.ID)
				}
			}
		}
	}
	if outside != 3 {
		t.Errorf("%d witnesses outside the kind map", outside)
	}
	// the native build identity: the registered grammar closure of the route, run under the declared limits
	if x.Native.Route != "csharp" || x.Native.Operation != "native-parse-edit" || len(x.Native.Grammar) != 8 || x.Native.Grammar[0].Path != "src/parser.c" || x.Native.Platform == "" || x.Native.DriverIdentity == "" {
		t.Errorf("native provenance %+v", x.Native)
	}
}

// The raw native observation is a local artifact: when it is configured, the asset must be exactly the
// comparison re-derived from it.
func TestCSharpG1ExecutedMatchesRawNative(t *testing.T) {
	path := os.Getenv("TSGK_G1_NATIVE_RAW")
	if path == "" {
		t.Skip("TSGK_G1_NATIVE_RAW (the raw observation of csharp-g1-native) is not configured")
	}
	a := g1xLoad(t)
	in := a.in
	var err error
	if in.Raw, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(a.asset, in); err != nil {
		t.Fatal(err)
	}
}

// ---- synthetic native observations ----------------------------------------------------------------

// g1xSynthetic builds a raw native observation in which every sealed witness has a node of the first
// sealed type of its alternative, then lets mutate change a source. It tests the comparison logic only:
// it is made from the expected asset and is never evidence of native behaviour.
func g1xSynthetic(t *testing.T, a *g1xAssets, mutate func(s *G1NativeSource)) []byte {
	t.Helper()
	byID := map[string]*G1NativeSource{}
	for _, id := range a.ctx.ids {
		src := a.ctx.sources[id]
		byID[id] = &G1NativeSource{ID: id, SHA256: src.SHA256, Bytes: len(src.Data), Status: G1NativeCompleted, Types: []string{"compilation_unit"},
			Nodes: [][5]int{{0, G1NodeNamed, 0, len(src.Data), -1}}}
	}
	for _, alt := range a.ctx.exp.Alternatives {
		for _, w := range alt.Witnesses {
			if alt.Status != G1ExpExpected {
				continue
			}
			s := byID[w.Source]
			ty := alt.TS[0]
			i := slices.Index(s.Types, ty)
			if i < 0 {
				i = len(s.Types)
				s.Types = append(s.Types, ty)
			}
			s.Nodes = append(s.Nodes, [5]int{i, 0, w.Start, w.End, 0})
		}
	}
	raw := CSharpG1NativeRaw{Schema: CSharpG1NativeRawSchema, Provenance: g1xProvenance()}
	for _, id := range a.ctx.ids {
		s := byID[id]
		if mutate != nil {
			mutate(s)
		}
		if s.Status == G1NativeCompleted {
			s.TreeDigest = G1NativeNodesDigest(*s)
		}
		raw.Sources = append(raw.Sources, *s)
	}
	data, err := MarshalCSharpG1NativeRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func g1xProvenance() G1NativeProvenance {
	return G1NativeProvenance{Route: "csharp", Symbol: "tree_sitter_c_sharp", Platform: "test/test", Grammar: []G1NativeFile{{Path: "src/parser.c", SHA256: strings.Repeat("a", 64), Bytes: 1}},
		DriverIdentity: strings.Repeat("b", 64), Compiler: "cc", CompilerSHA256: strings.Repeat("c", 64), CompilerVersion: "test", RuntimeCommit: "test",
		Operation: "native-parse-edit", NativeMemory: 1 << 30, NativeWallMS: 30000, BuildMemory: 4 << 30, BuildWallMS: 120000}
}

func TestCSharpG1ExecutedSyntheticComparison(t *testing.T) {
	a := g1xLoad(t)
	in := a.in
	in.Raw = g1xSynthetic(t, a, nil)
	x, err := DeriveCSharpG1Executed(in)
	if err != nil {
		t.Fatal(err)
	}
	// every witness has a node of a sealed type with its span: all MATCH, nothing to dispose of
	if x.Counts.Match != 426 || x.Counts.Mismatch != 0 || x.Counts.WitnessMatch != 626 || len(x.Dispositions) != 0 {
		t.Fatalf("all-match counts %+v", x.Counts)
	}
	data, err := MarshalCSharpG1Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(data, in); err != nil {
		t.Fatalf("all-match asset: %v", err)
	}
	// the asset is the comparison re-derived from the raw observation, and not from another one
	other := in
	other.Raw = g1xSynthetic(t, a, func(s *G1NativeSource) { s.Types = append(s.Types, "unused") })
	if _, err := ParseCSharpG1Executed(data, other); g1Code(err) != "G1_EXECUTED_PIN" && g1Code(err) != "G1_EXECUTED_RAW" {
		t.Errorf("another raw observation: %v", err)
	}
	// a changed type makes the witnesses of that source MISMATCH, and an undisposed mismatch is refused
	var victim string
	changed := in
	changed.Raw = g1xSynthetic(t, a, func(s *G1NativeSource) {
		if victim == "" && len(s.Nodes) > 1 {
			victim = s.ID
		}
		if s.ID == victim {
			for i := 1; i < len(s.Nodes); i++ {
				s.Types[s.Nodes[i][0]] = "nothing_like_it"
			}
		}
	})
	y, err := DeriveCSharpG1Executed(changed)
	if err != nil {
		t.Fatal(err)
	}
	if y.Counts.Mismatch == 0 || y.Counts.WitnessMismatch == 0 || len(y.Dispositions) != y.Counts.Mismatch {
		t.Fatalf("mismatch counts %+v", y.Counts)
	}
	for _, d := range y.Dispositions {
		if d.Cause != "UNDISPOSED" {
			t.Errorf("%s: disposition %q for a synthetic mismatch", d.Alternative, d.Cause)
		}
	}
	bad, err := MarshalCSharpG1Executed(y)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(bad, changed); g1Code(err) != "G1_EXECUTED_UNDISPOSED" {
		t.Errorf("undisposed mismatch: %v", err)
	}
	// the expectation is not an output of the comparison: it is the same bytes before and after
	if !bytes.Equal(a.in.Expected, in.Expected) {
		t.Fatal("expected bytes changed")
	}
	if _, err := ParseCSharpG1Expected(a.in.Expected, CSharpG1ExpectedSources{Src: g1Load(t).src, Observation: a.in.Observation, KindMap: a.in.KindMap, Sealed: g1ExpLoad(t).x.Metamorphic.Results()}); err != nil {
		// the ledger carries statuses now; the expected derivation ignores them (src.Expected and Executed are cleared)
		t.Fatalf("expected asset after the comparison: %v", err)
	}
}

// A native parse that did not complete cannot be compared: its witnesses are NOT_APPLICABLE and an
// alternative with no completed witness is not EXECUTED.
func TestCSharpG1ExecutedNotApplicable(t *testing.T) {
	a := g1xLoad(t)
	in := a.in
	in.Raw = g1xSynthetic(t, a, func(s *G1NativeSource) {
		s.Status, s.Code, s.Nodes, s.Types, s.TreeDigest, s.HasError = "RESOURCE_LIMIT", "PARSE_TIME_LIMIT", [][5]int{}, []string{}, "", false
	})
	x, err := DeriveCSharpG1Executed(in)
	if err != nil {
		t.Fatal(err)
	}
	c := x.Counts
	if c.NotApplicable != 426 || c.Executed != 0 || c.WitnessNotApplicable != 626 || c.Match != 0 || c.Mismatch != 0 || c.SourcesCompleted != 0 {
		t.Fatalf("counts %+v", c)
	}
	data, err := MarshalCSharpG1Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(data, in); err != nil {
		t.Fatal(err)
	}
	if len(x.ExecutedIDs()) != 0 {
		t.Fatal("alternatives without a comparison are EXECUTED")
	}
	// a witness that was not compared carries no evidence and no map label
	for name, forge := range map[string]func(w *G1ExecWitness){
		"found":  func(w *G1ExecWitness) { w.Found = []G1ExecFound{{Type: "x", Start: w.Start, End: w.End}} },
		"near":   func(w *G1ExecWitness) { w.Near = []G1ExecFound{{Type: "x", Start: w.Start, End: w.End}} },
		"map":    func(w *G1ExecWitness) { w.Map = "IN_MAP" },
		"result": func(w *G1ExecWitness) { w.Result = G1ResMatch },
	} {
		var y CSharpG1Executed
		if err := json.Unmarshal(data, &y); err != nil {
			t.Fatal(err)
		}
		forge(&y.Alternatives[0].Witnesses[0])
		bad, err := MarshalCSharpG1Executed(y)
		if err != nil {
			t.Fatal(err)
		}
		offline := in
		offline.Raw = nil
		if _, err := ParseCSharpG1Executed(bad, offline); g1Code(err) != "G1_EXECUTED_RESULT" {
			t.Errorf("forged %s on a witness that was not compared: %v", name, err)
		}
	}
	// a completed witness on an otherwise unparsed source set: one source fails, the rest compare
	one := in
	failed := ""
	one.Raw = g1xSynthetic(t, a, func(s *G1NativeSource) {
		if failed == "" {
			failed = s.ID
		}
		if s.ID == failed {
			s.Status, s.Code, s.Nodes, s.Types, s.TreeDigest = "RESOURCE_LIMIT", "PARSE_TIME_LIMIT", [][5]int{}, []string{}, ""
		}
	})
	y, err := DeriveCSharpG1Executed(one)
	if err != nil {
		t.Fatal(err)
	}
	if y.Counts.WitnessNotApplicable == 0 || y.Counts.WitnessMatch == 0 || y.Counts.Mismatch != 0 || y.Counts.SourcesCompleted != 71 {
		t.Fatalf("one failed source: %+v", y.Counts)
	}
	for _, alt := range y.Alternatives {
		if alt.Result == G1ResNotApplicable && alt.Executed {
			t.Errorf("%s: NOT_APPLICABLE but EXECUTED", alt.ID)
		}
	}
}

// The rules of the comparison on small trees: the sealed relation (span or starts-at), the type set,
// and the aggregation over witnesses.
func TestCSharpG1ExecutedJudge(t *testing.T) {
	tree := g1xTree{src: G1NativeSource{Status: G1NativeCompleted, Types: []string{"a", "b", `"kw"`}, Nodes: [][5]int{{0, 1, 0, 20, -1}, {1, 1, 5, 9, 0}, {2, 0, 5, 7, 1}, {0, 1, 10, 12, 0}}}}
	c := g1xContext{kinds: map[string][]string{"KwKeyword": {`"kw"`}}, spans: map[string]bool{g1xSpanKey("s", "KwKeyword", 5, 7): true}}
	cases := []struct {
		name  string
		w     G1ExpWitness
		ts    []string
		want  string
		found int
	}{
		{"span with a sealed type", G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span"}, []string{"b"}, G1ResMatch, 1},
		{"span with a type that is not sealed", G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span"}, []string{"a"}, G1ResMismatch, 1},
		{"span longer than the node", G1ExpWitness{Source: "s", Start: 5, End: 8, Match: "span"}, []string{"b"}, G1ResMismatch, 0},
		{"starts-at ignores the end", G1ExpWitness{Source: "s", Start: 5, End: 8, Match: "starts-at"}, []string{"b"}, G1ResMatch, 2},
		{"nothing at the start", G1ExpWitness{Source: "s", Start: 6, End: 9, Match: "starts-at"}, []string{"a", "b"}, G1ResMismatch, 0},
		{"anonymous type is quoted", G1ExpWitness{Source: "s", Start: 5, End: 7, Match: "span", Roslyn: "KwKeyword"}, []string{`"kw"`}, G1ResMatch, 1},
		{"unquoted anonymous type is another type", G1ExpWitness{Source: "s", Start: 5, End: 7, Match: "span"}, []string{"kw"}, G1ResMismatch, 1},
	}
	for _, tc := range cases {
		got := g1xCompare(tc.w, tc.ts, c, tree)
		if got.Result != tc.want || len(got.Found) != tc.found {
			t.Errorf("%s: %s with %d found, want %s %d", tc.name, got.Result, len(got.Found), tc.want, tc.found)
		}
		if got.Result == G1ResMatch && !slices.Contains(tc.ts, got.Found[0].Type) {
			t.Errorf("%s: the matching node is not first", tc.name)
		}
	}
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 7, Match: "span", Roslyn: "KwKeyword"}, []string{`"kw"`}, c, tree); got.Map != "IN_MAP" {
		t.Errorf("map %q", got.Map)
	}
	// a whole Roslyn element whose native node is of a type the kind map does not list, or lists nothing for
	c.spans[g1xSpanKey("s", "KwKeyword", 5, 9)] = true
	c.spans[g1xSpanKey("s", "NoMapKind", 5, 9)] = true
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span", Roslyn: "KwKeyword"}, []string{"b"}, c, tree); got.Result != G1ResMatch || got.Map != "OUTSIDE_MAP" {
		t.Errorf("a native type outside the kind map: %s %q", got.Result, got.Map)
	}
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span", Roslyn: "NoMapKind"}, []string{"b"}, c, tree); got.Map != "NO_MAP" {
		t.Errorf("a kind the map has no type for: %q", got.Map)
	}
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span", Roslyn: "KwKeyword"}, []string{"b"}, c, tree); got.Map != "OUTSIDE_MAP" {
		t.Errorf("map %q", got.Map)
	}
	delete(c.spans, g1xSpanKey("s", "KwKeyword", 5, 9))
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span", Roslyn: "KwKeyword"}, []string{"b"}, c, tree); got.Map != "FRAGMENT" {
		t.Errorf("a witness that is not a whole Roslyn element is labelled %q", got.Map)
	}
	// a mismatch shows the nodes around the span
	got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 7, Match: "span"}, []string{"b"}, c, g1xTree{src: G1NativeSource{Status: G1NativeCompleted, Types: []string{"a", "b"},
		Nodes: [][5]int{{0, 1, 0, 20, -1}, {1, 1, 5, 6, 0}, {1, 1, 6, 7, 0}}}})
	if got.Result != G1ResMismatch || len(got.Near) != 2 {
		t.Errorf("near nodes of a split token: %+v", got)
	}
	if got := g1xCompare(G1ExpWitness{Source: "s", Start: 5, End: 9, Match: "span"}, []string{"b"}, c, g1xTree{src: G1NativeSource{Status: "FAILED", Code: "X"}}); got.Result != G1ResNotApplicable {
		t.Errorf("an unparsed source: %s", got.Result)
	}
	// aggregation: any MISMATCH wins, then MATCH, and only a compared alternative is executed
	for _, tc := range []struct {
		results  []string
		want     string
		executed bool
	}{
		{[]string{G1ResMatch, G1ResMatch}, G1ResMatch, true},
		{[]string{G1ResMatch, G1ResMismatch}, G1ResMismatch, true},
		{[]string{G1ResNotApplicable, G1ResMatch}, G1ResMatch, true},
		{[]string{G1ResNotApplicable, G1ResMismatch}, G1ResMismatch, true},
		{[]string{G1ResNotApplicable}, G1ResNotApplicable, false},
		{nil, G1ResNotApplicable, false},
	} {
		var ws []G1ExecWitness
		for _, r := range tc.results {
			ws = append(ws, G1ExecWitness{Result: r})
		}
		if res, ex := g1xAltResult(ws); res != tc.want || ex != tc.executed {
			t.Errorf("%v: %s %v, want %s %v", tc.results, res, ex, tc.want, tc.executed)
		}
	}
}

// ---- tampering ------------------------------------------------------------------------------------

func TestCSharpG1ExecutedRejectsTampering(t *testing.T) {
	a := g1xLoad(t)
	clone := func() CSharpG1Executed {
		var x CSharpG1Executed
		if err := json.Unmarshal(a.asset, &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	altOf := func(x *CSharpG1Executed, id string) *G1ExecAlt {
		for i := range x.Alternatives {
			if x.Alternatives[i].ID == id {
				return &x.Alternatives[i]
			}
		}
		panic(id)
	}
	for _, f := range []struct {
		name string
		code string
		do   func(x *CSharpG1Executed)
	}{
		{"mismatch reported as a match", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { altOf(x, "csharp-B01.a335").Result = G1ResMatch }},
		{"mismatching witness reported as a match", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { altOf(x, "csharp-B01.a335").Witnesses[0].Result = G1ResMatch }},
		{"match reported as a mismatch", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { x.Alternatives[0].Result = G1ResMismatch }},
		{"match reported as not applicable", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { x.Alternatives[0].Result = G1ResNotApplicable }},
		{"executed flag cleared", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { x.Alternatives[0].Executed = false }},
		{"witness result cleared", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Result = "" }},
		{"evidence type forged without the result", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) {
			w := &altOf(x, "csharp-B01.a335").Witnesses[0]
			w.Found = []G1ExecFound{{Type: "identifier", Start: w.Start, End: w.End}}
		}},
		{"evidence start moved", "G1_EXECUTED_EVIDENCE", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Found[0].Start++ }},
		{"evidence end moved for a span witness", "G1_EXECUTED_EVIDENCE", func(x *CSharpG1Executed) {
			for i := range x.Alternatives {
				if w := &x.Alternatives[i].Witnesses[0]; w.Match == "span" {
					w.Found[0].End++
					return
				}
			}
		}},
		{"evidence list too long", "G1_EXECUTED_EVIDENCE", func(x *CSharpG1Executed) {
			w := &x.Alternatives[0].Witnesses[0]
			for len(w.Found) <= g1ExecFoundLimit {
				w.Found = append(w.Found, w.Found[0])
			}
		}},
		{"near nodes on a match", "G1_EXECUTED_EVIDENCE", func(x *CSharpG1Executed) {
			w := &x.Alternatives[0].Witnesses[0]
			w.Near = []G1ExecFound{{Type: "x", Start: w.Start, End: w.End}}
		}},
		{"map label changed", "G1_EXECUTED_RESULT", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Map = "OUTSIDE_MAP" }},
		{"witness start changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Start++ }},
		{"witness end changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].End++ }},
		{"witness relation changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Match = "starts-at" }},
		{"witness source changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses[0].Source = x.Sources[1].ID }},
		{"witness dropped", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Witnesses = x.Alternatives[0].Witnesses[1:] }},
		{"sealed types widened to fit the native result", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) {
			x.Alternatives[0].TS = append(x.Alternatives[0].TS, "preproc_arg")
		}},
		{"sealed types of the mismatch widened", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) {
			alt := altOf(x, "csharp-B01.a335")
			alt.TS = append(alt.TS, "preproc_arg")
		}},
		{"basis changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Basis = "differential-oracle" }},
		{"row changed", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0].Row = "csharp-B02" }},
		{"alternative dropped", "G1_EXECUTED_ALTERNATIVES", func(x *CSharpG1Executed) { x.Alternatives = x.Alternatives[1:] }},
		{"alternative added", "G1_EXECUTED_ALTERNATIVES", func(x *CSharpG1Executed) { x.Alternatives = append(x.Alternatives, x.Alternatives[0]) }},
		{"alternatives swapped", "G1_EXECUTED_EXPECTATION", func(x *CSharpG1Executed) { x.Alternatives[0], x.Alternatives[1] = x.Alternatives[1], x.Alternatives[0] }},
		{"expected pin", "G1_EXECUTED_PIN", func(x *CSharpG1Executed) { x.Inputs.Expected.SHA256 = strings.Repeat("0", 64) }},
		{"expected pin bytes", "G1_EXECUTED_PIN", func(x *CSharpG1Executed) { x.Inputs.Expected.Bytes++ }},
		{"kind map pin", "G1_EXECUTED_PIN", func(x *CSharpG1Executed) { x.Inputs.KindMap.SHA256 = strings.Repeat("0", 64) }},
		{"observation pin", "G1_EXECUTED_PIN", func(x *CSharpG1Executed) { x.Inputs.Observation.SHA256 = strings.Repeat("0", 64) }},
		{"source sha", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0].SHA256 = strings.Repeat("0", 64) }},
		{"source bytes", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0].Bytes++ }},
		{"source dropped", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources = x.Sources[1:] }},
		{"last source dropped", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources = x.Sources[:len(x.Sources)-1] }},
		{"source swapped", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0], x.Sources[1] = x.Sources[1], x.Sources[0] }},
		{"Roslyn acceptance flipped", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0].RoslynAccepted = !x.Sources[0].RoslynAccepted }},
		{"completed source without a tree", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0].Nodes = 0 }},
		{"failed source with a tree digest", "G1_EXECUTED_SOURCES", func(x *CSharpG1Executed) { x.Sources[0].Status, x.Sources[0].Code = "FAILED", "X" }},
		{"counts", "G1_EXECUTED_COUNTS", func(x *CSharpG1Executed) { x.Counts.Match++ }},
		{"disposition dropped", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) { x.Dispositions = x.Dispositions[1:] }},
		{"disposition added", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) {
			x.Dispositions = append(x.Dispositions, G1ExecDisposition{Alternative: x.Alternatives[0].ID, Cause: G1CauseDesign, Reason: "x"})
		}},
		{"disposition cause changed", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) { x.Dispositions[0].Cause = G1CauseDesign }},
		{"disposition cause unknown", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) { x.Dispositions[0].Cause = "UNDISPOSED" }},
		{"disposition reason emptied", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) { x.Dispositions[0].Reason = "" }},
		{"disposition for another alternative", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) { x.Dispositions[0].Alternative = x.Alternatives[0].ID }},
		{"native memory limit", "G1_EXECUTED_NATIVE", func(x *CSharpG1Executed) { x.Native.NativeMemory = 4 << 30 }},
		{"native wall limit", "G1_EXECUTED_NATIVE", func(x *CSharpG1Executed) { x.Native.NativeWallMS = 60000 }},
		{"build limits", "G1_EXECUTED_NATIVE", func(x *CSharpG1Executed) { x.Native.BuildMemory = 8 << 30 }},
		{"grammar closure dropped", "G1_EXECUTED_NATIVE", func(x *CSharpG1Executed) { x.Native.Grammar = nil }},
		{"raw pin dropped", "G1_EXECUTED_NATIVE", func(x *CSharpG1Executed) { x.Inputs.Raw.SHA256 = "" }},
		{"schema", "G1_EXECUTED_SCHEMA", func(x *CSharpG1Executed) { x.Schema = "x" }},
	} {
		x := clone()
		f.do(&x)
		data, err := MarshalCSharpG1Executed(x)
		if err != nil {
			t.Fatal(err)
		}
		// the context of the sealed inputs is built once; the table changes the asset only
		var got CSharpG1Executed
		if got, err = DecodeCSharpG1Executed(data); err == nil {
			err = g1xCheck(data, got, a.ctx, a.in)
		}
		if g1Code(err) != f.code {
			t.Errorf("%s: %v, want %s", f.name, err, f.code)
		}
	}
	text := string(a.asset)
	for name, forged := range map[string]struct{ text, code string }{
		"unknown member": {strings.Replace(text, `"basis":`, `"extra": 1,`+"\n"+` "basis":`, 1), "G1_EXECUTED_JSON"},
		"not json":       {text[:len(text)/2], "G1_EXECUTED_JSON"},
		"whitespace":     {strings.Replace(text, "\n \"sources\"", "\n\n \"sources\"", 1), "G1_EXECUTED_BYTES"},
		"basis edited":   {strings.Replace(text, "are not changed by this asset", "are changed by this asset", 1), "G1_EXECUTED_BYTES"},
	} {
		if forged.text == text {
			t.Fatalf("%s: forgery did not change the asset", name)
		}
		if _, err := ParseCSharpG1Executed([]byte(forged.text), a.in); g1Code(err) != forged.code {
			t.Errorf("%s: %v, want %s", name, err, forged.code)
		}
	}
	if _, err := ParseCSharpG1Executed(append(slices.Clone(a.asset), make([]byte, csharpG1ExecutedMax)...), a.in); g1Code(err) != "G1_EXECUTED_SIZE" {
		t.Errorf("oversize: %v", err)
	}
	// the sealed inputs are the ones the asset is bound to: a changed expected asset, kind map or observation is another pin
	for name, edit := range map[string]func(in *CSharpG1ExecutedSources){
		"expected":    func(in *CSharpG1ExecutedSources) { in.Expected = append(slices.Clone(in.Expected), '\n') },
		"kind map":    func(in *CSharpG1ExecutedSources) { in.KindMap = append(slices.Clone(in.KindMap), '\n') },
		"observation": func(in *CSharpG1ExecutedSources) { in.Observation = append(slices.Clone(in.Observation), '\n') },
	} {
		in := a.in
		edit(&in)
		if _, err := ParseCSharpG1Executed(a.asset, in); err == nil {
			t.Errorf("%s changed under the executed asset: accepted", name)
		}
	}
}

// A forged piece of evidence that is consistent with its result passes the offline check (the trees are
// not in the asset), so the raw observation is what catches it.
func TestCSharpG1ExecutedForgedEvidenceNeedsTheRaw(t *testing.T) {
	a := g1xLoad(t)
	in := a.in
	in.Raw = g1xSynthetic(t, a, nil)
	x, err := DeriveCSharpG1Executed(in)
	if err != nil {
		t.Fatal(err)
	}
	w := &x.Alternatives[0].Witnesses[0]
	w.Found = append([]G1ExecFound{{Type: w.Found[0].Type, Start: w.Start, End: w.End}}, G1ExecFound{Type: "extra_node", Start: w.Start, End: w.End})
	data, err := MarshalCSharpG1Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	offline := in
	offline.Raw = nil
	if _, err := ParseCSharpG1Executed(data, offline); err != nil {
		t.Fatalf("offline: %v", err)
	}
	if _, err := ParseCSharpG1Executed(data, in); g1Code(err) != "G1_EXECUTED_RAW" {
		t.Errorf("with the raw observation: %v", err)
	}
}

// The raw observation must cover exactly the sources the comparison needs, with the ledger's identities.
func TestCSharpG1ExecutedRawCoverage(t *testing.T) {
	a := g1xLoad(t)
	for name, tc := range map[string]struct {
		mutate func(raw *CSharpG1NativeRaw)
		code   string
	}{
		"source dropped": {func(r *CSharpG1NativeRaw) { r.Sources = r.Sources[1:] }, "G1_EXECUTED_SOURCES"},
		"extra source": {func(r *CSharpG1NativeRaw) {
			extra := r.Sources[0]
			extra.ID = "extra#0"
			r.Sources = append(r.Sources, extra)
		}, "G1_EXECUTED_SOURCES"},
		"source added":       {func(r *CSharpG1NativeRaw) { r.Sources = append(r.Sources, r.Sources[0]) }, "G1_NATIVE_SOURCE"},
		"source sha":         {func(r *CSharpG1NativeRaw) { r.Sources[0].SHA256 = strings.Repeat("0", 64) }, "G1_EXECUTED_SOURCES"},
		"source bytes":       {func(r *CSharpG1NativeRaw) { r.Sources[0].Bytes++ }, "G1_EXECUTED_SOURCES"},
		"source reorder":     {func(r *CSharpG1NativeRaw) { r.Sources[0], r.Sources[1] = r.Sources[1], r.Sources[0] }, "G1_EXECUTED_SOURCES"},
		"tree digest":        {func(r *CSharpG1NativeRaw) { r.Sources[0].TreeDigest = strings.Repeat("0", 64) }, "G1_NATIVE_TREE"},
		"node out of range":  {func(r *CSharpG1NativeRaw) { r.Sources[0].Nodes[1][3] = r.Sources[0].Bytes + 1 }, "G1_NATIVE_TREE"},
		"parent after child": {func(r *CSharpG1NativeRaw) { r.Sources[0].Nodes[1][4] = 5 }, "G1_NATIVE_TREE"},
		"root with a parent": {func(r *CSharpG1NativeRaw) { r.Sources[0].Nodes[0][4] = 0 }, "G1_NATIVE_TREE"},
		"type index":         {func(r *CSharpG1NativeRaw) { r.Sources[0].Nodes[1][0] = 99 }, "G1_NATIVE_TREE"},
		"schema":             {func(r *CSharpG1NativeRaw) { r.Schema = "x" }, "G1_NATIVE_SCHEMA"},
		"failed with nodes":  {func(r *CSharpG1NativeRaw) { r.Sources[0].Status, r.Sources[0].Code = "FAILED", "X" }, "G1_NATIVE_SOURCE"},
	} {
		in := a.in
		in.Raw = g1xSynthetic(t, a, nil)
		var raw CSharpG1NativeRaw
		if err := json.Unmarshal(in.Raw, &raw); err != nil {
			t.Fatal(err)
		}
		tc.mutate(&raw)
		// re-seal the digests the mutation is not about, so the test hits one rule at a time
		if name != "tree digest" && name != "failed with nodes" {
			for i := range raw.Sources {
				if raw.Sources[i].Status == G1NativeCompleted {
					raw.Sources[i].TreeDigest = G1NativeNodesDigest(raw.Sources[i])
				}
			}
		}
		data, err := MarshalCSharpG1NativeRaw(raw)
		if err != nil {
			t.Fatal(err)
		}
		in.Raw = data
		if _, err := DeriveCSharpG1Executed(in); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", name, err, tc.code)
		}
	}
	if _, err := DecodeCSharpG1NativeRaw([]byte(`{"schema":"tsgk-csharp-g1-native-raw/r1","extra":1}`)); g1Code(err) != "G1_NATIVE_JSON" {
		t.Errorf("unknown member: %v", err)
	}
	if _, err := DecodeCSharpG1NativeRaw(make([]byte, csharpG1NativeRawMax+1)); g1Code(err) != "G1_NATIVE_SIZE" {
		t.Errorf("oversize: %v", err)
	}
}

// The source list: the witness sources of every EXPECTED alternative and the P steps the ledger links
// to them, nothing of a BLOCKED alternative.
func TestCSharpG1NativeSourceIDs(t *testing.T) {
	a := g1xLoad(t)
	ids := CSharpG1NativeSourceIDs(a.ctx.ledger, a.ctx.exp)
	if len(ids) != 72 || !slices.IsSorted(ids) {
		t.Fatalf("%d source ids", len(ids))
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, alt := range a.ctx.exp.Alternatives {
		for _, w := range alt.Witnesses {
			if !set[w.Source] {
				t.Errorf("witness source %s is not listed", w.Source)
			}
		}
	}
	links := map[string][]G1Link{}
	for _, alt := range a.ctx.ledger.Alternatives {
		links[alt.ID] = alt.Links
	}
	for _, alt := range a.ctx.exp.Alternatives {
		for _, l := range links[alt.ID] {
			for _, s := range l.Steps {
				id := fmt.Sprintf("%s#%d", l.Case, s)
				if alt.Status == G1ExpExpected && !set[id] {
					t.Errorf("%s: linked step %s is not listed", alt.ID, id)
				}
			}
		}
	}
	// the ids are the ledger's source versions
	for _, id := range ids {
		if _, ok := a.ctx.sources[id]; !ok {
			t.Errorf("%s is not a ledger source", id)
		}
	}
}

// ---- the comparison never feeds an expectation ---------------------------------------------------

// The native comparison reads the sealed expected asset, the kind map and the observation as bytes and
// writes only its own asset. Nothing in the derivation side (the spec deriver, the expectation, its rules,
// checks and data, the kind map, the observation and the ledger derivation) names the native observation
// or the executed asset, and nothing in the comparison can produce an expectation or a kind map.
func TestCSharpG1NativeNeverFeedsExpectations(t *testing.T) {
	sealedSide := []string{"csharp_g1_spec.go", "csharp_g1_expected.go", "csharp_g1_expected_rules.go", "csharp_g1_expected_checks.go", "csharp_g1_expected_data.go", "csharp_g1_kindmap.go", "csharp_g1_roslyn.go"}
	comparisonSide := regexp.MustCompile(`^(G1Exec|g1x|G1Native|g1n[A-Z]|CSharpG1Executed|CSharpG1NativeRaw|DeriveCSharpG1Executed|ParseCSharpG1Executed|MarshalCSharpG1Executed|DecodeCSharpG1Executed|DecodeCSharpG1NativeRaw|MarshalCSharpG1NativeRaw|CSharpG1NativeSourceIDs)`)
	for _, name := range sealedSide {
		f := g1ParseFile(t, name)
		idents, strs := g1Mentions(f)
		for _, id := range idents {
			if comparisonSide.MatchString(id) {
				t.Errorf("%s: identifier %s belongs to the native comparison", name, id)
			}
			// the one reference: the expected derivation clears the ledger's executed input
			if id == "Executed" && name != "csharp_g1_expected.go" {
				t.Errorf("%s: identifier Executed", name)
			}
		}
		for _, s := range strs {
			if strings.Contains(s, "csharp-g1-executed") || strings.Contains(s, "csharp-g1-native") {
				t.Errorf("%s: string %q names the comparison assets", name, s)
			}
		}
	}
	// the comparison files cannot write, derive or check an expectation, the kind map or the observation
	forbidden := map[string]bool{"MarshalCSharpG1Expected": true, "DeriveCSharpG1Expected": true, "CheckCSharpG1Expected": true, "ParseCSharpG1Expected": true,
		"MarshalCSharpG1KindMap": true, "DeriveCSharpG1KindMap": true, "MarshalCSharpG1Roslyn": true, "g1ExpContext": true, "g1BuildRules": true, "g1BuildCtx": true}
	for _, name := range []string{"csharp_g1_executed.go", "csharp_g1_native.go"} {
		f := g1ParseFile(t, name)
		idents, _ := g1Mentions(f)
		for _, id := range idents {
			if forbidden[id] || strings.HasPrefix(id, "g1s") || id == "g1Rule" {
				t.Errorf("%s: identifier %s can change an expectation side", name, id)
			}
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Type.Results == nil {
				continue
			}
			for _, r := range fn.Type.Results.List {
				ast.Inspect(r.Type, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && (id.Name == "CSharpG1Expected" || id.Name == "CSharpG1KindMap" || id.Name == "G1ExpAlt") {
						t.Errorf("%s: %s returns %s", name, fn.Name.Name, id.Name)
					}
					return true
				})
			}
		}
	}
	// the executed asset names the sealed assets only by pin
	a := g1xLoad(t)
	if a.x.Inputs.Expected.Path != CSharpG1ExpectedPath || a.x.Inputs.Expected.SHA256 != digestHex(a.in.Expected) {
		t.Errorf("expected pin %+v", a.x.Inputs.Expected)
	}
	// behaviour: a comparison in which every native node disagrees leaves the expected derivation as it was
	in := a.in
	in.Raw = g1xSynthetic(t, a, func(s *G1NativeSource) {
		for i := range s.Types {
			s.Types[i] += "_disagreeing"
		}
	})
	x, err := DeriveCSharpG1Executed(in)
	if err != nil {
		t.Fatal(err)
	}
	if x.Counts.Match != 0 || x.Counts.Mismatch != 426 {
		t.Fatalf("a fully disagreeing native tree: %+v", x.Counts)
	}
	before, err := DeriveCSharpG1Expected(CSharpG1ExpectedSources{Src: g1Load(t).src, Observation: a.in.Observation, KindMap: a.in.KindMap, Sealed: g1ExpLoad(t).x.Metamorphic.Results()})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalCSharpG1Expected(before)
	if err != nil || !bytes.Equal(data, a.in.Expected) {
		t.Fatal("the expected derivation no longer reproduces the sealed asset")
	}
}

// The authored dispositions are a closed table: every entry has a cause of the vocabulary and a reason,
// none is stale, and a cause outside the vocabulary is refused even when the asset and the table agree.
func TestCSharpG1ExecutedDispositionTable(t *testing.T) {
	a := g1xLoad(t)
	causes := []string{G1CauseGrammar, G1CauseKindMap, G1CauseExpected, G1CauseDesign}
	for id, d := range g1ExecDispositions {
		if d.Reason == "" || !slices.Contains(causes, d.Cause) || d.Alternative != "" {
			t.Errorf("%s: malformed authored disposition %+v", id, d)
		}
	}
	if len(g1ExecDispositions) != len(a.x.Dispositions) {
		t.Errorf("%d authored dispositions, %d mismatches", len(g1ExecDispositions), len(a.x.Dispositions))
	}
	const id = "csharp-B01.a335"
	old := g1ExecDispositions[id]
	g1ExecDispositions[id] = G1ExecDisposition{Cause: "BOGUS", Reason: old.Reason}
	defer func() { g1ExecDispositions[id] = old }()
	x := a.x
	x.Dispositions = slices.Clone(a.x.Dispositions)
	for i := range x.Dispositions {
		if x.Dispositions[i].Alternative == id {
			x.Dispositions[i].Cause = "BOGUS"
		}
	}
	data, err := MarshalCSharpG1Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeCSharpG1Executed(data); err != nil {
		t.Fatal(err)
	} else if err := g1xCheck(data, got, a.ctx, a.in); g1Code(err) != "G1_EXECUTED_UNDISPOSED" {
		t.Errorf("a cause outside the vocabulary: %v", err)
	}
}

// The native source list on small structures: a BLOCKED alternative adds no source, a step linked to an
// EXPECTED alternative does, and every witness source is listed once.
func TestCSharpG1NativeSourceIDsSmall(t *testing.T) {
	ledger := CSharpG1Ledger{Alternatives: []G1Alternative{
		{ID: "x", Links: []G1Link{{Case: "blocked", Steps: []int{0, 1}}}},
		{ID: "y", Links: []G1Link{{Case: "linked", Steps: []int{0, 2}}}},
	}}
	exp := CSharpG1Expected{Alternatives: []G1ExpAlt{
		{ID: "x", Status: G1ExpBlocked},
		{ID: "y", Status: G1ExpExpected, Witnesses: []G1ExpWitness{{Source: "linked#0"}, {Source: "witness#1"}}},
	}}
	got := CSharpG1NativeSourceIDs(ledger, exp)
	if want := []string{"linked#0", "linked#2", "witness#1"}; !slices.Equal(got, want) {
		t.Errorf("source ids %v, want %v", got, want)
	}
}
