package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Sealed r1 digests: the post-native corrections never touch these files (commit 1e4738d).
const (
	g1cR1ExpectedSHA = "fe8cc88ef48c2596c391b07c88050539641e985e6cc28c22af98b4f0958fd869"
	g1cR1KindMapSHA  = "83322b72849ba6fd5101e206a7c08f1cb9e26f39aa3e8773d6ef4d50d47f0393"
	g1cR1ExecutedSHA = "f56eced3dd41a15977448dd19eb28047ba7b906331aef7f1f7137d6f2eca1b83"
)

type g1cAssets struct {
	r1Exp, r2Exp, r1Km, r2Km, obs []byte
	vocab                         []string
	expIn                         CSharpG1ExpectedR2Sources
	kmIn                          CSharpG1KindMapR2Sources
	err                           error
}

var g1cOnce sync.Once
var g1cLoaded g1cAssets

func g1cLoad(t *testing.T) *g1cAssets {
	t.Helper()
	g1cOnce.Do(func() {
		a := &g1cLoaded
		e := g1ExpLoad(t)
		read := func(name string) []byte {
			data, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return data
		}
		a.r1Exp, a.r1Km, a.obs = e.asset, e.kindmap, e.obs
		a.r2Exp = read("../contracts/csharp-g1-expected-b01b02-r2.json")
		a.r2Km = read("../contracts/csharp-g1-kind-map-r2.json")
		if a.err != nil {
			return
		}
		a.vocab, a.err = CSharpG1RoslynVocabulary(a.obs)
		a.expIn = CSharpG1ExpectedR2Sources{R1: e.in, Previous: a.r1Exp, KindMap: a.r2Km}
		a.kmIn = CSharpG1KindMapR2Sources{Previous: a.r1Km, Observed: a.vocab}
	})
	if g1cLoaded.err != nil {
		t.Fatal(g1cLoaded.err)
	}
	return &g1cLoaded
}

func TestCSharpG1CorrectR1StaysAsSealed(t *testing.T) {
	a := g1cLoad(t)
	for _, f := range []struct {
		name string
		data []byte
		n    int
		sha  string
	}{
		{"expected r1", a.r1Exp, 236615, g1cR1ExpectedSHA},
		{"kind map r1", a.r1Km, 55247, g1cR1KindMapSHA},
		{"executed r1", mustRead(t, "../contracts/csharp-g1-executed-b01b02-r1.json"), 211543, g1cR1ExecutedSHA},
	} {
		if len(f.data) != f.n || digestHex(f.data) != f.sha {
			t.Errorf("%s changed after the seal: %d/%s", f.name, len(f.data), digestHex(f.data))
		}
	}
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCSharpG1CorrectedAssetsVerify(t *testing.T) {
	a := g1cLoad(t)
	km, err := ParseCSharpG1KindMapR2(a.r2Km, a.kmIn)
	if err != nil {
		t.Fatal(err)
	}
	x, err := ParseCSharpG1ExpectedR2(a.r2Exp, a.expIn)
	if err != nil {
		t.Fatal(err)
	}
	if len(km.Corrections) != 5 || len(x.Corrections) != 18 {
		t.Fatalf("corrections: kind map %d, expected %d", len(km.Corrections), len(x.Corrections))
	}
	for _, c := range append(slices.Clone(km.Corrections), x.Corrections...) {
		if c.Reason == "" || len(c.Grounds) < 2 {
			t.Errorf("%s: correction lacks its record: %+v", c.Item, c)
		}
	}
	// the authored flags are pinned directly (nothing is inferred from an item name): every r2 correction is after_native,
	// the timing (the decision of 2026-11-13), and every ground is the specification or the grammar text, so
	// basis_independent holds for all of them
	type flag struct{ item, field string }
	wantFlags := func(got []G1Correction, want []flag) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%d corrections, want %d", len(got), len(want))
		}
		for i, c := range got {
			if c.Item != want[i].item || c.Field != want[i].field || !c.AfterNative || !c.BasisIndependent {
				t.Errorf("correction %d: %s %s after_native %v basis_independent %v", i, c.Item, c.Field, c.AfterNative, c.BasisIndependent)
			}
		}
	}
	wantFlags(x.Corrections, []flag{
		{"csharp-B01.a335", "ts"}, {"csharp-B01.a383", "witnesses[0].match"}, {"csharp-B01.a396", "witnesses[0].match"}, {"csharp-B01.a397", "witnesses[0].match"},
		{"csharp-B01.a383", "ts_basis"}, {"csharp-B01.a396", "ts_basis"}, {"csharp-B01.a397", "ts_basis"},
		// the audit of the differential-oracle alternatives (G1AUDIT76): the witness moves with its types, or only its match relaxes
		{"csharp-B02.a021", G1FieldEvidence}, {"csharp-B02.a062", G1FieldEvidence}, {"csharp-B02.a030", G1FieldEvidence}, {"csharp-B02.a072", G1FieldEvidence},
		{"csharp-B02.a084", G1FieldEvidence}, {"csharp-B02.a086", G1FieldEvidence}, {"csharp-B02.a087", G1FieldEvidence}, {"csharp-B02.a088", G1FieldEvidence},
		{"csharp-B02.a089", G1FieldEvidence}, {"csharp-B01.a381", "witnesses[0].match"}, {"csharp-B01.a395", "witnesses[0].match"},
	})
	wantFlags(km.Corrections, []flag{
		{"InterpolatedStringStartToken", "entry"}, {"InterpolatedVerbatimStringStartToken", "entry"},
		{"InterpolatedSingleLineRawStringStartToken", "entry"}, {"InterpolatedMultiLineRawStringStartToken", "entry"}, {"interpolation_start", "ts_only"},
	})
	// the derivation is what is tracked
	if d, err := DeriveCSharpG1ExpectedR2(a.expIn); err != nil {
		t.Fatal(err)
	} else if data, err := MarshalCSharpG1Expected(d); err != nil || !bytes.Equal(data, a.r2Exp) {
		t.Fatal("tracked r2 expected asset differs from the derivation")
	}
	if d, err := DeriveCSharpG1KindMapR2(a.kmIn); err != nil {
		t.Fatal(err)
	} else if data, err := MarshalCSharpG1KindMap(d); err != nil || !bytes.Equal(data, a.r2Km) {
		t.Fatal("tracked r2 kind map differs from the derivation")
	}
	// the counts of the expectation did not move: no alternative changed its status
	r1, _ := DecodeCSharpG1Expected(a.r1Exp)
	if x.Counts != r1.Counts {
		t.Fatalf("counts moved: %+v -> %+v", r1.Counts, x.Counts)
	}
}

// g1cDiff lists the JSON paths where two documents differ, found by comparing the decoded values
// rather than by the verifier's own engine. A list of another length is one path (the list).
func g1cDiff(path string, a, b any, out *[]string) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, path)
			return
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		for k := range keys {
			x, inA := av[k]
			y, inB := bv[k]
			if inA != inB {
				*out = append(*out, path+"/"+k)
				continue
			}
			g1cDiff(path+"/"+k, x, y, out)
		}
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			*out = append(*out, path)
			return
		}
		for i := range av {
			g1cDiff(fmt.Sprintf("%s/%d", path, i), av[i], bv[i], out)
		}
	default:
		if !g1eSame(a, b) {
			*out = append(*out, path)
		}
	}
}

func g1cDiffFiles(t *testing.T, a, b []byte) []string {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	var out []string
	g1cDiff("", x, y, &out)
	slices.Sort(out)
	return out
}

// The r2 files differ from the r1 files at exactly the listed corrections and the pins, and nowhere else.
func TestCSharpG1CorrectedAssetsDifferOnlyByTheCorrections(t *testing.T) {
	a := g1cLoad(t)
	x, _ := DecodeCSharpG1Expected(a.r2Exp)
	idx := func(id string) int {
		for i, alt := range x.Alternatives {
			if alt.ID == id {
				return i
			}
		}
		t.Fatalf("no alternative %s", id)
		return -1
	}
	want := []string{"/schema", "/corrections", "/inputs/previous", "/inputs/kind_map/path", "/inputs/kind_map/bytes", "/inputs/kind_map/sha256",
		fmt.Sprintf("/alternatives/%d/ts/0", idx("csharp-B01.a335"))}
	for _, id := range []string{"csharp-B01.a383", "csharp-B01.a396", "csharp-B01.a397", "csharp-B01.a381", "csharp-B01.a395"} {
		want = append(want, fmt.Sprintf("/alternatives/%d/witnesses/0/match", idx(id)))
	}
	// an evidence correction changes the members of its record that differ between the old and the new value, nowhere else
	for _, c := range x.Corrections {
		if c.Field != G1FieldEvidence {
			continue
		}
		var o, n g1cEvidence
		if err := json.Unmarshal([]byte(c.Old), &o); err != nil || json.Unmarshal([]byte(c.New), &n) != nil {
			t.Fatal(c.Item, err)
		}
		at := fmt.Sprintf("/alternatives/%d", idx(c.Item))
		if len(o.TS) != len(n.TS) {
			want = append(want, at+"/ts")
		}
		for i := range o.TS {
			if len(o.TS) == len(n.TS) && o.TS[i] != n.TS[i] {
				want = append(want, fmt.Sprintf("%s/ts/%d", at, i))
			}
		}
		for name, differs := range map[string]bool{"start": o.Witness.Start != n.Witness.Start, "end": o.Witness.End != n.Witness.End, "roslyn": o.Witness.Roslyn != n.Witness.Roslyn, "match": o.Witness.Match != n.Witness.Match} {
			if differs {
				want = append(want, at+"/witnesses/0/"+name)
			}
		}
	}
	slices.Sort(want)
	if got := g1cDiffFiles(t, a.r1Exp, a.r2Exp); !slices.Equal(got, want) {
		t.Fatalf("expected r2 differs from r1 at %v, want %v", got, want)
	}
	// the kind map: two entries, the counts they move, the schema, the pin and the record
	km, _ := ParseCSharpG1KindMap(a.r2Km, a.vocab)
	kidx := func(name string) int {
		return slices.IndexFunc(km.Kinds, func(k G1KMKind) bool { return k.Roslyn == name })
	}
	var wantKm []string
	for _, name := range []string{"InterpolatedStringStartToken", "InterpolatedVerbatimStringStartToken", "InterpolatedSingleLineRawStringStartToken", "InterpolatedMultiLineRawStringStartToken"} {
		wantKm = append(wantKm, fmt.Sprintf("/kinds/%d/map", kidx(name)), fmt.Sprintf("/kinds/%d/ts", kidx(name)), fmt.Sprintf("/kinds/%d/reason", kidx(name)))
	}
	wantKm = append(wantKm, "/schema", "/previous", "/corrections", "/counts/one_to_one", "/counts/none", "/counts/ts_referenced", "/counts/ts_named_only", "/counts/ts_shared_by_many_kinds", "/ts_only")
	slices.Sort(wantKm)
	if got := g1cDiffFiles(t, a.r1Km, a.r2Km); !slices.Equal(got, wantKm) {
		t.Fatalf("kind map r2 differs from r1 at %v, want %v", got, wantKm)
	}
}

// ---- counterexamples ----------------------------------------------------------------------------

func g1cExpectedMutant(t *testing.T, a *g1cAssets, mutate func(x *CSharpG1Expected)) error {
	t.Helper()
	x, err := DecodeCSharpG1Expected(a.r2Exp)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&x)
	data, err := MarshalCSharpG1Expected(x) // canonical bytes: a rejection comes from the check, not the layout
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCSharpG1ExpectedR2(data, a.expIn)
	return err
}

func g1cAltOf(x *CSharpG1Expected, id string) *G1ExpAlt {
	for i := range x.Alternatives {
		if x.Alternatives[i].ID == id {
			return &x.Alternatives[i]
		}
	}
	return nil
}

// extraCorrection is a well-formed correction nobody authored.
func g1cExtra(item string) G1Correction {
	c := g1ExpectedCorrections[1]
	c.Item = item
	return c
}

func TestCSharpG1CorrectedExpectedRejects(t *testing.T) {
	a := g1cLoad(t)
	// an alternative that no correction touches and whose first witness is a span
	var spare string
	r1, _ := DecodeCSharpG1Expected(a.r1Exp)
	for _, alt := range r1.Alternatives {
		if alt.Status == G1ExpExpected && len(alt.Witnesses) > 0 && alt.Witnesses[0].Match == "span" && !strings.HasPrefix(alt.ID, "csharp-B01.a3") {
			spare = alt.ID
			break
		}
	}
	if spare == "" {
		t.Fatal("no spare alternative")
	}
	blocked := ""
	for _, alt := range r1.Alternatives {
		if alt.Status == G1ExpBlocked {
			blocked = alt.ID
			break
		}
	}
	if blocked == "" {
		t.Fatal("no blocked alternative")
	}
	cases := []struct {
		name, code string
		mutate     func(x *CSharpG1Expected)
	}{
		{"a difference no correction lists (type)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { g1cAltOf(x, spare).TS = []string{"identifier", "x_extra"} }},
		{"a difference no correction lists (match)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { g1cAltOf(x, spare).Witnesses[0].Match = "starts-at" }},
		{"a difference no correction lists (witness span)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { g1cAltOf(x, spare).Witnesses[0].End++ }},
		{"a difference no correction lists (status)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { g1cAltOf(x, spare).Status = G1ExpBlocked }},
		{"a difference no correction lists (counts)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Counts.Witnesses++ }},
		{"a difference no correction lists (cross check)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Cross.Sources++ }},
		{"a difference no correction lists (symbols context)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.SymbolsContext = x.SymbolsContext[1:] }},
		{"a difference no correction lists (metamorphic)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Metamorphic.Seed += "x" }},
		{"a difference no correction lists (witness limit)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.WitnessLimit++ }},
		{"a difference no correction lists (audit)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Audit.RatePercent++ }},
		{"a difference no correction lists (basis)", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Basis += " " }},
		{"a correction removed, its change kept", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { x.Corrections = x.Corrections[1:] }},
		{"a change reverted, its correction kept", "G1_CORRECTION_UNLISTED", func(x *CSharpG1Expected) { g1cAltOf(x, "csharp-B01.a335").TS = []string{"identifier"} }},
		{"a correction without grounds", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds = nil }},
		{"a correction with a grammar ground only", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) {
			x.Corrections[1].Grounds = x.Corrections[1].Grounds[:1]
		}},
		{"a correction with a spec ground only", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) {
			x.Corrections[1].Grounds = x.Corrections[1].Grounds[1:]
		}},
		{"a ground without a quote", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Quote = "  " }},
		{"a ground without a place", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].At = "" }},
		{"a ground in an unknown document", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Doc = "native" }},
		{"a correction not marked after_native", "G1_CORRECTION_FORM", func(x *CSharpG1Expected) { x.Corrections[0].AfterNative = false }},
		{"a basis_independent flag that is not the authored one", "G1_CORRECTION_UNAUTHORED", func(x *CSharpG1Expected) { x.Corrections[0].BasisIndependent = false }},
		// the grounds are checkable offline (F5)
		{"a quote below the minimum length", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Quote = "preproc_define: $ => seq(" }},
		{"a ground of another grammar.js", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].DocSHA256 = strings.Repeat("0", 64) }},
		{"a spec ground of the other spec file", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[3].DocSHA256 = g1gExprSHA }},
		{"a heading of another section", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[3].Heading = "### 6.5.5 Line directives" }},
		{"a grammar heading of another rule", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Heading = "preproc_undef:" }},
		{"a gap beyond the bound", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[3].Gap = 201 }},
		{"a negative gap", "G1_CORRECTION_GROUND", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Gap = -1 }},
		{"a ts_basis record on a spec-derived alternative", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[4].Item = "csharp-B01.a335" }},
		{"a correction without a reason", "G1_CORRECTION_FORM", func(x *CSharpG1Expected) { x.Corrections[0].Reason = "" }},
		{"a correction that changes nothing", "G1_CORRECTION_FORM", func(x *CSharpG1Expected) { x.Corrections[0].New = x.Corrections[0].Old }},
		{"a correction of an unknown field", "G1_CORRECTION_FORM", func(x *CSharpG1Expected) { x.Corrections[0].Field = "status" }},
		{"the same field corrected twice", "G1_CORRECTION_FORM", func(x *CSharpG1Expected) { x.Corrections = append(x.Corrections, x.Corrections[0]) }},
		{"a correction of an unknown alternative", "G1_CORRECTION_ITEM", func(x *CSharpG1Expected) { x.Corrections[0].Item = "csharp-B01.a99999" }},
		{"a correction of a blocked alternative", "G1_CORRECTION_ITEM", func(x *CSharpG1Expected) { x.Corrections[0].Item = blocked }},
		{"a correction whose old value is not r1's", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[0].Old = `["name"]` }},
		{"a correction whose new value is no type list", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[0].New = `[]` }},
		{"a match correction whose old value is not r1's", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[1].Old = `"x"` }},
		{"a correction whose new type list is not sorted", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[0].New = `["preproc_arg","identifier"]` }},
		{"a correction whose new type is empty", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[0].New = `[""]` }},
		{"a correction whose new match is unknown", "G1_CORRECTION_OLD", func(x *CSharpG1Expected) { x.Corrections[1].New = `"anywhere"` }},
		{"a well-formed correction nobody authored", "G1_CORRECTION_UNAUTHORED", func(x *CSharpG1Expected) {
			c := g1cExtra(spare)
			x.Corrections = append(x.Corrections, c)
			g1cAltOf(x, spare).Witnesses[0].Match = "starts-at"
		}},
		{"an authored correction with its reason changed", "G1_CORRECTION_UNAUTHORED", func(x *CSharpG1Expected) { x.Corrections[0].Reason += " (changed)" }},
		{"an authored correction with a ground changed", "G1_CORRECTION_UNAUTHORED", func(x *CSharpG1Expected) { x.Corrections[0].Grounds[0].Quote += " x" }},
		{"the pin of r1 changed", "G1_CORRECTION_PIN", func(x *CSharpG1Expected) { x.Inputs.Previous.SHA256 = strings.Repeat("0", 64) }},
		{"the pin of the kind map changed", "G1_CORRECTION_PIN", func(x *CSharpG1Expected) { x.Inputs.KindMap.SHA256 = strings.Repeat("0", 64) }},
		{"the pin of the kind map set back to r1", "G1_CORRECTION_PIN", func(x *CSharpG1Expected) {
			x.Inputs.KindMap.Path = CSharpG1KindMapPath
		}},
		{"the schema of r1 with corrections", "G1_EXPECTED_SCHEMA", func(x *CSharpG1Expected) { x.Schema = CSharpG1ExpectedSchema }},
		{"the schema of r2 without corrections", "G1_EXPECTED_SCHEMA", func(x *CSharpG1Expected) { x.Corrections = nil }},
	}
	for _, c := range cases {
		err := g1cExpectedMutant(t, a, c.mutate)
		if got := g1Code(err); got != c.code {
			t.Errorf("%s: code %q (%v), want %s", c.name, got, err, c.code)
		}
	}
	// the layout is part of the asset
	var tampered bytes.Buffer
	tampered.Write(bytes.Replace(a.r2Exp, []byte(`"witness_limit": 3`), []byte(`"witness_limit":  3`), 1))
	if _, err := ParseCSharpG1ExpectedR2(tampered.Bytes(), a.expIn); g1Code(err) != "G1_CORRECTION_BYTES" {
		t.Errorf("non-canonical layout: %v", err)
	}
	// the r2 verifier refuses an r1 file and the r1 verifier an r2 file
	if _, err := ParseCSharpG1ExpectedR2(a.r1Exp, a.expIn); g1Code(err) != "G1_EXPECTED_SCHEMA" {
		t.Errorf("r1 asset given to the r2 verifier: %v", err)
	}
	if _, err := ParseCSharpG1Expected(a.r2Exp, a.expIn.R1); g1Code(err) != "G1_EXPECTED_SCHEMA" {
		t.Errorf("r2 asset given to the r1 verifier: %v", err)
	}
}

// A tampered r1 asset is refused by the r2 verifier, whatever the r2 asset says.
func TestCSharpG1CorrectedExpectedRejectsATamperedR1(t *testing.T) {
	a := g1cLoad(t)
	r1, err := DecodeCSharpG1Expected(a.r1Exp)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(x *CSharpG1Expected)
	}{
		{"a witness moved", func(x *CSharpG1Expected) { g1cAltOf(x, "csharp-B01.a335").Witnesses[0].End++ }},
		{"a sealed type changed", func(x *CSharpG1Expected) { g1cAltOf(x, "csharp-B01.a335").TS = []string{"preproc_arg"} }},
		{"the value a correction replaces already changed", func(x *CSharpG1Expected) { g1cAltOf(x, "csharp-B01.a383").Witnesses[0].Match = "starts-at" }},
	} {
		y := r1
		y.Alternatives = slices.Clone(r1.Alternatives)
		for i := range y.Alternatives {
			y.Alternatives[i].Witnesses = slices.Clone(y.Alternatives[i].Witnesses)
			y.Alternatives[i].TS = slices.Clone(y.Alternatives[i].TS)
		}
		tc.mutate(&y)
		data, err := MarshalCSharpG1Expected(y)
		if err != nil {
			t.Fatal(err)
		}
		in := a.expIn
		in.Previous = data
		_, err = ParseCSharpG1ExpectedR2(a.r2Exp, in)
		if err == nil || !strings.HasPrefix(g1Code(err), "G1_EXPECTED_") {
			t.Errorf("%s: tampered r1 accepted or wrong code: %v", tc.name, err)
		}
	}
	// one flipped byte of the r1 file
	bad := bytes.Replace(a.r1Exp, []byte(`"match":"span"`), []byte(`"match":"spam"`), 1)
	in := a.expIn
	in.Previous = bad
	if _, err := ParseCSharpG1ExpectedR2(a.r2Exp, in); err == nil {
		t.Error("a flipped byte of r1 was accepted")
	}
}

func g1cKindMapMutant(t *testing.T, a *g1cAssets, mutate func(m *CSharpG1KindMap)) error {
	t.Helper()
	m, err := ParseCSharpG1KindMap(a.r2Km, a.vocab)
	if err != nil {
		t.Fatal(err)
	}
	m.Kinds = slices.Clone(m.Kinds)
	m.Corrections = slices.Clone(m.Corrections)
	for i := range m.Corrections {
		m.Corrections[i].Grounds = slices.Clone(m.Corrections[i].Grounds)
	}
	mutate(&m)
	m.Counts = DeriveCSharpG1KindMapCounts(m)
	data, err := MarshalCSharpG1KindMap(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseCSharpG1KindMapR2(data, a.kmIn)
	return err
}

func TestCSharpG1CorrectedKindMapRejects(t *testing.T) {
	a := g1cLoad(t)
	kind := func(m *CSharpG1KindMap, name string) *G1KMKind {
		for i := range m.Kinds {
			if m.Kinds[i].Roslyn == name {
				return &m.Kinds[i]
			}
		}
		t.Fatalf("no kind %s", name)
		return nil
	}
	cases := []struct {
		name, code string
		mutate     func(m *CSharpG1KindMap)
	}{
		{"an entry no correction lists", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { kind(m, "InterpolatedStringStartToken").Text = "$" }},
		{"an entry no correction lists (a lexeme)", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { kind(m, "IntKeyword").Text = "integer" }},
		{"an entry no correction lists (the end token)", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) {
			k := kind(m, "InterpolatedStringEndToken")
			k.Map, k.TS, k.Reason = G1MapNone, []string{}, "NO_SEPARATE_NODE"
		}},
		{"a difference in the spec list", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { m.Spec = append(slices.Clone(m.Spec), "extra") }},
		{"a difference in the vocabulary", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) {
			// rename the last anonymous type no kind refers to: the counts, the kinds and the lists stay as they are
			used := map[string]bool{}
			for _, k := range m.Kinds {
				for _, e := range k.TS {
					used[e] = true
				}
			}
			m.Vocabulary.Anonymous = slices.Clone(m.Vocabulary.Anonymous)
			last := len(m.Vocabulary.Anonymous) - 1
			for used[G1KindMapAnon(m.Vocabulary.Anonymous[last])] {
				last--
			}
			m.Vocabulary.Anonymous[last] += "~"
			if last+1 < len(m.Vocabulary.Anonymous) && m.Vocabulary.Anonymous[last] >= m.Vocabulary.Anonymous[last+1] {
				t.Fatal("the renamed type is no longer in order")
			}
		}},
		{"a difference in the unreferenced types", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) {
			m.TSOnly = slices.Clone(m.TSOnly)
			m.TSOnly[0].Reason = map[bool]string{true: "SUPERTYPE", false: "TS_WRAPPER"}[m.TSOnly[0].Reason == "TS_WRAPPER"]
		}},
		{"a difference in the basis", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { m.Basis += " " }},
		{"a difference in the pinned grammar", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { m.Grammar.Commit = strings.Repeat("a", 40) }},
		{"a correction removed, its change kept", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { m.Corrections = m.Corrections[1:] }},
		{"a change reverted, its correction kept", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) {
			k := kind(m, "InterpolatedStringStartToken")
			k.Map, k.TS, k.Reason = G1MapOneToOne, []string{"interpolation_start"}, ""
			// the type is referenced again, so it leaves the list of unreferenced types (a valid map)
			m.TSOnly = slices.DeleteFunc(slices.Clone(m.TSOnly), func(o G1KMTSOnly) bool { return o.Type == "interpolation_start" })
		}},
		{"a correction without grounds", "G1_CORRECTION_GROUND", func(m *CSharpG1KindMap) { m.Corrections[0].Grounds = nil }},
		{"a correction with a grammar ground only", "G1_CORRECTION_GROUND", func(m *CSharpG1KindMap) { m.Corrections[0].Grounds = m.Corrections[0].Grounds[:1] }},
		{"a correction not marked after_native", "G1_CORRECTION_FORM", func(m *CSharpG1KindMap) { m.Corrections[0].AfterNative = false }},
		{"a raw start correction not marked after_native", "G1_CORRECTION_FORM", func(m *CSharpG1KindMap) { m.Corrections[2].AfterNative = false }},
		{"a basis_independent flag that is not the authored one", "G1_CORRECTION_UNAUTHORED", func(m *CSharpG1KindMap) { m.Corrections[2].BasisIndependent = false }},
		{"the unreferenced type correction removed, its change kept", "G1_CORRECTION_UNLISTED", func(m *CSharpG1KindMap) { m.Corrections = m.Corrections[:4] }},
		{"an unreferenced type no grammar has", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[4].Item = "no_such_type"
			m.Corrections[4].New = `{"type":"no_such_type","reason":"EXTERNAL_SCANNER_NODE"}`
		}},
		{"an unreferenced type whose new entry names another type", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[4].New = `{"type":"interpolation_brace","reason":"EXTERNAL_SCANNER_NODE"}`
		}},
		{"an unreferenced type with an unknown reason", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[4].New = `{"type":"interpolation_start","reason":"NOPE"}`
		}},
		{"an unreferenced type already listed by r1", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[4].Item = "interpolation_brace"
			m.Corrections[4].New = `{"type":"interpolation_brace","reason":"EXTERNAL_SCANNER_NODE"}`
		}},
		{"an unreferenced type correction with an old value", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) { m.Corrections[4].Old = `"x"` }},
		{"a correction of an unknown field", "G1_CORRECTION_FORM", func(m *CSharpG1KindMap) { m.Corrections[0].Field = "status" }},
		{"a correction of an unknown kind", "G1_CORRECTION_ITEM", func(m *CSharpG1KindMap) { m.Corrections[0].Item = "NoSuchToken" }},
		{"a correction whose old entry is not r1's", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[0].Old = strings.Replace(m.Corrections[0].Old, "interpolation_start", "string_content", 1)
		}},
		{"a correction that renames the kind", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			// the last entry correction: a later one cannot notice the renamed kind
			m.Corrections[3].New = strings.Replace(m.Corrections[3].New, `"roslyn":"InterpolatedMultiLineRawStringStartToken"`, `"roslyn":"InterpolatedRawStringEndToken"`, 1)
		}},
		{"a correction that changes the kind's class", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[0].New = strings.Replace(m.Corrections[0].New, `"class":"token"`, `"class":"node"`, 1)
		}},
		{"a correction that changes the kind's lexeme", "G1_CORRECTION_OLD", func(m *CSharpG1KindMap) {
			m.Corrections[0].New = strings.Replace(m.Corrections[0].New, `"ts":[]`, `"ts":[],"text":"$"`, 1)
		}},
		{"a well-formed correction nobody authored", "G1_CORRECTION_UNAUTHORED", func(m *CSharpG1KindMap) {
			c := m.Corrections[0]
			c.Item = "InterpolatedStringEndToken"
			c.Old = `{"roslyn":"InterpolatedStringEndToken","class":"token","map":"1:1","ts":["\"\"\""]}`
			c.New = `{"roslyn":"InterpolatedStringEndToken","class":"token","map":"none","ts":[],"reason":"NO_SEPARATE_NODE"}`
			m.Corrections = append(m.Corrections, c)
			k := kind(m, c.Item)
			k.Map, k.TS, k.Reason = G1MapNone, []string{}, "NO_SEPARATE_NODE"
		}},
		{"an authored correction with its reason changed", "G1_CORRECTION_UNAUTHORED", func(m *CSharpG1KindMap) { m.Corrections[0].Reason += " (changed)" }},
		{"the pin of r1 changed", "G1_CORRECTION_PIN", func(m *CSharpG1KindMap) { m.Previous.SHA256 = strings.Repeat("0", 64) }},
		{"the schema of r1 with corrections", "G1_KINDMAP_SCHEMA", func(m *CSharpG1KindMap) { m.Schema = CSharpG1KindMapSchema }},
	}
	for _, c := range cases {
		err := g1cKindMapMutant(t, a, c.mutate)
		if got := g1Code(err); got != c.code {
			t.Errorf("%s: code %q (%v), want %s", c.name, got, err, c.code)
		}
	}
	if _, err := ParseCSharpG1KindMapR2(a.r1Km, a.kmIn); g1Code(err) != "G1_KINDMAP_SCHEMA" {
		t.Errorf("r1 map given to the r2 verifier: %v", err)
	}
	// a tampered r1 map
	in := a.kmIn
	in.Previous = bytes.Replace(a.r1Km, []byte(`"map":"1:1","ts":["interpolation_start"]`), []byte(`"map":"1:1","ts":["interpolation_quote"]`), 1)
	if _, err := ParseCSharpG1KindMapR2(a.r2Km, in); err == nil {
		t.Error("a tampered r1 kind map was accepted")
	}
}

// The grounds are checked against the pinned documents when a checkout is supplied (the grammar text
// and the specification are not tracked). A quote that is not in the document is refused.
func TestCSharpG1CorrectionGroundsAreQuotes(t *testing.T) {
	grammar := "x\npreproc_define: $ => seq(\n  preprocessor('define'),\n  $.preproc_arg,\n  /\\n/,\n),\n y"
	lex := "intro\n### 6.5.4 Definition directives\n\n'define' PP_Whitespace  PP_Conditional_Symbol\n"
	spec := map[string]string{"lexical-structure.md": lex}
	c := []G1Correction{{Item: "i", Grounds: []G1Ground{
		{G1DocGrammar, "preproc_define", "preproc_define: $ => seq( preprocessor('define'), $.preproc_arg, /\\n/, ),", digestHex([]byte(grammar)), "preproc_define:", 0},
		{G1DocSpec, g1gSpecPre + "standard/lexical-structure.md 6.5.4", "'define' PP_Whitespace PP_Conditional_Symbol", digestHex([]byte(lex)), "### 6.5.4 Definition directives", 2}}}}
	if err := CheckG1Grounds(c, grammar, spec); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(c []G1Correction){
		"a quote that is not in the document": func(c []G1Correction) {
			c[0].Grounds[0].Quote = "preproc_define: $ => seq( preprocessor('define'), $.identifier,"
		},
		"another quote":                         func(c []G1Correction) { c[0].Grounds[1].Quote = "'undef' PP_Whitespace" },
		"a spec ground of another file":         func(c []G1Correction) { c[0].Grounds[1].At = g1gSpecPre + "standard/expressions.md 12.8.3" },
		"a document that is not the pinned one": func(c []G1Correction) { c[0].Grounds[1].DocSHA256 = strings.Repeat("0", 64) },
		"a gap that is too short":               func(c []G1Correction) { c[0].Grounds[1].Gap = 1 },
		"a gap that is too long":                func(c []G1Correction) { c[0].Grounds[1].Gap = 3 },
		"a heading that is not there":           func(c []G1Correction) { c[0].Grounds[1].Heading = "### 6.5.5 Line directives" },
		"a grammar heading that is not there":   func(c []G1Correction) { c[0].Grounds[0].Heading = "preproc_undef:" },
	} {
		cc := slices.Clone(c)
		cc[0].Grounds = slices.Clone(c[0].Grounds)
		mut(cc)
		if CheckG1Grounds(cc, grammar, spec) == nil {
			t.Errorf("%s: a ground that quotes nothing was accepted", name)
		}
	}
	// when the pinned documents are at hand, every authored ground quotes them
	g, s := os.Getenv("TSGK_G1_GRAMMAR_JS"), os.Getenv("TSGK_G1_SPEC_DIR")
	if g == "" || s == "" {
		t.Log("TSGK_G1_GRAMMAR_JS and TSGK_G1_SPEC_DIR not set: authored grounds not checked against the documents (go run ./src/dev/csharp-g1-correct -grammar … -spec … does)")
		return
	}
	gt, st := mustRead(t, g), map[string]string{}
	for _, name := range []string{"lexical-structure.md", "expressions.md"} {
		st[name] = string(mustRead(t, s+"/"+name))
	}
	for _, cs := range [][]G1Correction{g1ExpectedCorrections, g1KindMapCorrections} {
		if err := CheckG1Grounds(cs, string(gt), st); err != nil {
			t.Error(err)
		}
	}
}

// ---- the r2 executed asset ------------------------------------------------------------------------

func g1cExecIn(t *testing.T, a *g1cAssets) CSharpG1ExecutedSources {
	t.Helper()
	return CSharpG1ExecutedSources{Src: g1Load(t).src, Expected: a.r2Exp, KindMap: a.r2Km, Observation: a.obs}
}

// The native result after the corrections: the 411 alternatives that matched when sealed and were not corrected still match,
// and the fifteen corrected ones (four after the native comparison, eleven after the audit of the differential-oracle
// alternatives) are counted apart.
func TestCSharpG1ExecutedR2SplitsTheStatistics(t *testing.T) {
	a := g1cLoad(t)
	data := mustRead(t, "../contracts/csharp-g1-executed-b01b02-r2.json")
	in := g1cExecIn(t, a)
	x, err := ParseCSharpG1Executed(data, in)
	if err != nil {
		t.Fatal(err)
	}
	r1 := g1xLoad(t).x
	if x.Schema != CSharpG1ExecutedSchemaR2 || r1.Counts.Match != 422 || r1.Counts.Mismatch != 4 {
		t.Fatalf("schema %s, r1 match %d mismatch %d", x.Schema, r1.Counts.Match, r1.Counts.Mismatch)
	}
	if x.Counts.SealedMatch != r1.Counts.Match-11 || x.Counts.Corrected != 15 || x.Counts.CorrectedMatch != 15 || x.Counts.Match != 426 || x.Counts.Mismatch != 0 || len(x.Dispositions) != 0 {
		t.Fatalf("r2 counts %+v", x.Counts)
	}
	// the alternatives the native comparison contradicted are the four of the first corrections, and nothing else matches
	// newly; the eleven corrections of the audit matched in r1 too (the native result could not tell them wrong) and still match
	corrected := map[string]bool{}
	for _, c := range g1ExpectedCorrections[:7] {
		corrected[c.Item] = true
	}
	r1res := map[string]string{}
	for _, alt := range r1.Alternatives {
		r1res[alt.ID] = alt.Result
	}
	for _, alt := range x.Alternatives {
		if corrected[alt.ID] != (r1res[alt.ID] == G1ResMismatch) {
			t.Errorf("%s: r1 %s, r2 %s, corrected %v", alt.ID, r1res[alt.ID], alt.Result, corrected[alt.ID])
		}
	}
	// the same native observation: the raw pin is the one of r1
	if x.Inputs.Raw != r1.Inputs.Raw || x.Native.DriverIdentity != r1.Native.DriverIdentity {
		t.Error("the r2 comparison did not use the native observation of r1")
	}
	// the revisions do not mix
	for name, bad := range map[string]func(in *CSharpG1ExecutedSources){
		"r1 expected asset": func(in *CSharpG1ExecutedSources) { in.Expected = a.r1Exp },
		"r1 kind map":       func(in *CSharpG1ExecutedSources) { in.KindMap = a.r1Km },
	} {
		in := g1cExecIn(t, a)
		bad(&in)
		if _, err := ParseCSharpG1Executed(data, in); err == nil {
			t.Errorf("%s: accepted by the r2 executed asset", name)
		}
	}
	r1data := mustRead(t, "../contracts/csharp-g1-executed-b01b02-r1.json")
	if _, err := ParseCSharpG1Executed(r1data, in); g1Code(err) != "G1_EXECUTED_SCHEMA" {
		t.Errorf("r1 executed asset against r2 inputs: %v", err)
	}
	for _, c := range []struct {
		name, code string
		mutate     func(x *CSharpG1Executed)
	}{
		{"corrected count forged", "G1_EXECUTED_COUNTS", func(x *CSharpG1Executed) { x.Counts.CorrectedMatch-- }},
		{"sealed count forged", "G1_EXECUTED_COUNTS", func(x *CSharpG1Executed) { x.Counts.SealedMatch++ }},
		{"schema set back to r1", "G1_EXECUTED_SCHEMA", func(x *CSharpG1Executed) { x.Schema = CSharpG1ExecutedSchema }},
		{"a corrected witness turned into a mismatch without a disposition", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) {
			for i := range x.Alternatives {
				if x.Alternatives[i].ID == "csharp-B01.a335" {
					w := &x.Alternatives[i].Witnesses[0]
					w.Found, w.Result = []G1ExecFound{{Type: "identifier", Start: w.Start, End: w.End}}, G1ResMismatch
					x.Alternatives[i].Result = G1ResMismatch
				}
			}
			x.Counts = g1xCounts(*x, map[string]bool{"csharp-B01.a335": true, "csharp-B01.a383": true, "csharp-B01.a396": true, "csharp-B01.a397": true})
		}},
	} {
		y, err := DecodeCSharpG1Executed(data)
		if err != nil {
			t.Fatal(err)
		}
		c.mutate(&y)
		d, err := MarshalCSharpG1Executed(y)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1Executed(d, in); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
}

// The ledger takes only identifiers and states from the assets: the r2 pair gives the tracked ledger.
func TestCSharpG1LedgerFromTheR2Assets(t *testing.T) {
	a := g1cLoad(t)
	l := g1Load(t)
	src := l.src
	src.Expected, src.Executed = a.r2Exp, mustRead(t, "../contracts/csharp-g1-executed-b01b02-r2.json")
	got, err := DeriveCSharpG1Ledger(src)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalCSharpG1Ledger(got)
	if err != nil || !bytes.Equal(data, l.asset) {
		t.Fatal("the ledger derived from the r2 assets differs from the tracked ledger")
	}
}

// The comparison logic of r2 on a synthetic native observation (every witness has a node of a sealed
// type): the derivation follows the r2 revision and the verifier accepts what it derives.
func TestCSharpG1ExecutedR2SyntheticDerivation(t *testing.T) {
	a := g1cLoad(t)
	in := g1cExecIn(t, a)
	ctx, err := g1xContextOf(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Raw = g1xSynthetic(t, &g1xAssets{in: in, ctx: ctx}, nil)
	x, err := DeriveCSharpG1Executed(in)
	if err != nil {
		t.Fatal(err)
	}
	if x.Schema != CSharpG1ExecutedSchemaR2 || x.Basis != csharpG1ExecutedBasisR2 || x.Counts.Corrected != 15 || x.Counts.CorrectedMatch != 15 || x.Counts.SealedMatch != 411 {
		t.Fatalf("schema %s counts %+v", x.Schema, x.Counts)
	}
	data, err := MarshalCSharpG1Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(data, in); err != nil {
		t.Fatal(err)
	}
	// a corrected witness whose native tree has no node of the corrected type is a mismatch, and needs a disposition
	for _, mismatch := range []string{"csharp-B01.a335", "csharp-B01.a383"} {
		var src string
		for _, alt := range ctx.exp.Alternatives {
			if alt.ID == mismatch {
				src = alt.Witnesses[0].Source
			}
		}
		raw := g1xSynthetic(t, &g1xAssets{in: in, ctx: ctx}, func(s *G1NativeSource) {
			if s.ID == src {
				s.Types = []string{"compilation_unit"}
				s.Nodes = s.Nodes[:1]
			}
		})
		in2 := in
		in2.Raw = raw
		y, err := DeriveCSharpG1Executed(in2)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range y.Dispositions {
			if d.Alternative == mismatch {
				found = true
				if d.Cause != "UNDISPOSED" || d.Reason != "" {
					t.Errorf("%s: r2 has a disposition for a mismatch it must not dispose: %+v", mismatch, d)
				}
			}
		}
		if !found || y.Counts.Mismatch == 0 || len(y.Dispositions) != y.Counts.Mismatch {
			t.Errorf("%s: counts %+v dispositions %+v", mismatch, y.Counts, y.Dispositions)
		}
	}
}

func TestCSharpG1ExecutedR2MatchesRawNative(t *testing.T) {
	path := os.Getenv("TSGK_G1_NATIVE_RAW")
	if path == "" {
		t.Skip("TSGK_G1_NATIVE_RAW (the raw observation of csharp-g1-native) is not configured")
	}
	in := g1cExecIn(t, g1cLoad(t))
	var err error
	if in.Raw, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1Executed(mustRead(t, "../contracts/csharp-g1-executed-b01b02-r2.json"), in); err != nil {
		t.Fatal(err)
	}
}

func TestCSharpG1ExecutedR2ContextAndRevisionCases(t *testing.T) {
	a := g1cLoad(t)
	data := mustRead(t, "../contracts/csharp-g1-executed-b01b02-r2.json")
	// the context refuses inputs the expected asset does not pin
	in := g1cExecIn(t, a)
	in.Observation = append(slices.Clone(a.obs), ' ')
	if _, err := g1xContextOf(in); g1Code(err) != "G1_EXECUTED_PIN" {
		t.Errorf("a different observation: %v", err)
	}
	in = g1cExecIn(t, a)
	in.KindMap = a.r1Km
	if _, err := g1xContextOf(in); g1Code(err) != "G1_EXECUTED_PIN" {
		t.Errorf("the r1 kind map with the r2 expected asset: %v", err)
	}
	in = g1cExecIn(t, a)
	for _, c := range []struct {
		name, code string
		mutate     func(x *CSharpG1Executed)
	}{
		{"a disposition of r1 in the r2 asset", "G1_EXECUTED_UNDISPOSED", func(x *CSharpG1Executed) {
			d := g1ExecDispositions["csharp-B01.a335"]
			d.Alternative = "csharp-B01.a335"
			x.Dispositions = []G1ExecDisposition{d}
			for i := range x.Alternatives {
				if x.Alternatives[i].ID == "csharp-B01.a335" {
					w := &x.Alternatives[i].Witnesses[0]
					w.Found, w.Result = []G1ExecFound{{Type: "identifier", Start: w.Start, End: w.End}}, G1ResMismatch
					x.Alternatives[i].Result = G1ResMismatch
				}
			}
			x.Counts = g1xCounts(*x, map[string]bool{"csharp-B01.a335": true, "csharp-B01.a383": true, "csharp-B01.a396": true, "csharp-B01.a397": true})
		}},
		{"the basis of r1 in the r2 asset", "G1_EXECUTED_BYTES", func(x *CSharpG1Executed) { x.Basis = csharpG1ExecutedBasis }},
	} {
		y, err := DecodeCSharpG1Executed(data)
		if err != nil {
			t.Fatal(err)
		}
		c.mutate(&y)
		d, err := MarshalCSharpG1Executed(y)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCSharpG1Executed(d, in); g1Code(err) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
}

// An empty list of corrections is no record: the decoders already tie the r2 schema to a non-empty
// list, and the form check refuses it on its own as well.
func TestCSharpG1CorrectionFormRefusesAnEmptyList(t *testing.T) {
	if err := g1CheckCorrections(nil, g1cExpectedField); g1Code(err) != "G1_CORRECTION_FORM" {
		t.Fatalf("empty list: %v", err)
	}
	if err := g1CheckCorrections(g1ExpectedCorrections, g1cExpectedField); err != nil {
		t.Fatal(err)
	}
}

// The decoders tie the schema to the record: r1 carries neither a pin of a previous revision nor
// corrections, r2 carries corrections.
func TestCSharpG1DecodersTieTheSchemaToTheRecord(t *testing.T) {
	a := g1cLoad(t)
	x, err := DecodeCSharpG1Expected(a.r1Exp)
	if err != nil {
		t.Fatal(err)
	}
	x.Inputs.Previous = x.Inputs.KindMap
	if data, err := MarshalCSharpG1Expected(x); err != nil {
		t.Fatal(err)
	} else if _, err := DecodeCSharpG1Expected(data); g1Code(err) != "G1_EXPECTED_SCHEMA" {
		t.Errorf("r1 expected asset with a previous pin: %v", err)
	}
	x, _ = DecodeCSharpG1Expected(a.r1Exp)
	x.Corrections = g1ExpectedCorrections
	if data, err := MarshalCSharpG1Expected(x); err != nil {
		t.Fatal(err)
	} else if _, err := DecodeCSharpG1Expected(data); g1Code(err) != "G1_EXPECTED_SCHEMA" {
		t.Errorf("r1 expected asset with corrections: %v", err)
	}
	m, err := ParseCSharpG1KindMap(a.r1Km, a.vocab)
	if err != nil {
		t.Fatal(err)
	}
	m.Previous = x.Inputs.KindMap
	if data, err := json.Marshal(m); err != nil {
		t.Fatal(err)
	} else if _, err := ParseCSharpG1KindMap(data, a.vocab); g1Code(err) != "G1_KINDMAP_SCHEMA" {
		t.Errorf("r1 kind map with a previous pin: %v", err)
	}
	m, _ = ParseCSharpG1KindMap(a.r1Km, a.vocab)
	m.Corrections = g1KindMapCorrections
	if data, err := json.Marshal(m); err != nil {
		t.Fatal(err)
	} else if _, err := ParseCSharpG1KindMap(data, a.vocab); g1Code(err) != "G1_KINDMAP_SCHEMA" {
		t.Errorf("r1 kind map with corrections: %v", err)
	}
}

// F4: the r2 expected asset pins the r2 kind map, but the ts of three alternatives is not what the r2 kind map gives
// (the map relates the start tokens of interpolated strings to no node type). Every EXPECTED ts of r2 equals the
// derivation from the r2 kind map, or its alternative is listed in the change record (a ts or ts_basis correction).
func g1cTSOffMap(x, derived CSharpG1Expected) (unlisted, vacuous []string) {
	by := map[string]G1ExpAlt{}
	for _, a := range derived.Alternatives {
		by[a.ID] = a
	}
	listed, basis := map[string]bool{}, map[string]bool{}
	for _, c := range x.Corrections {
		listed[c.Item] = listed[c.Item] || c.Field == G1FieldTS || c.Field == G1FieldTSBasis || c.Field == G1FieldEvidence
		basis[c.Item] = basis[c.Item] || c.Field == G1FieldTSBasis
	}
	off := map[string]bool{}
	for _, a := range x.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		if d := by[a.ID]; d.Status != G1ExpExpected || !slices.Equal(d.TS, a.TS) {
			off[a.ID] = true
			if !listed[a.ID] {
				unlisted = append(unlisted, a.ID)
			}
		}
	}
	for item, recorded := range basis {
		if recorded && !off[item] {
			vacuous = append(vacuous, item)
		}
	}
	slices.Sort(vacuous)
	return unlisted, vacuous
}

func TestCSharpG1CorrectedExpectedTSFollowsTheR2KindMap(t *testing.T) {
	a := g1cLoad(t)
	x, err := ParseCSharpG1ExpectedR2(a.r2Exp, a.expIn)
	if err != nil {
		t.Fatal(err)
	}
	in := a.expIn.R1
	in.KindMap = a.r2Km
	derived, err := DeriveCSharpG1Expected(in)
	if err != nil {
		t.Fatal(err)
	}
	unlisted, vacuous := g1cTSOffMap(x, derived)
	if len(unlisted) != 0 || len(vacuous) != 0 {
		t.Fatalf("r2 EXPECTED ts off the r2 kind map and not in the change record: %v; ts_basis records of a ts the map gives: %v", unlisted, vacuous)
	}
	// the three that the map does not give are exactly the ts_basis records
	var basis []string
	for _, c := range x.Corrections {
		if c.Field == G1FieldTSBasis {
			basis = append(basis, c.Item)
		}
	}
	if !slices.Equal(basis, []string{"csharp-B01.a383", "csharp-B01.a396", "csharp-B01.a397"}) {
		t.Errorf("ts_basis records: %v", basis)
	}
	// mutants: a record dropped, a ts changed without a record, a record of a ts that the map gives
	dropped := x
	dropped.Corrections = slices.DeleteFunc(slices.Clone(x.Corrections), func(c G1Correction) bool { return c.Item == "csharp-B01.a383" && c.Field == G1FieldTSBasis })
	if u, _ := g1cTSOffMap(dropped, derived); !slices.Equal(u, []string{"csharp-B01.a383"}) {
		t.Errorf("a dropped ts_basis record was not reported: %v", u)
	}
	moved := g1cCloneExpected(x)
	for i := range moved.Alternatives {
		if moved.Alternatives[i].Status == G1ExpExpected && moved.Alternatives[i].Basis == G1BasisSpec && !slices.Contains(basis, moved.Alternatives[i].ID) && moved.Alternatives[i].ID != "csharp-B01.a335" {
			moved.Alternatives[i].TS = append(moved.Alternatives[i].TS, "zz_type")
			slices.Sort(moved.Alternatives[i].TS)
			break
		}
	}
	if u, _ := g1cTSOffMap(moved, derived); len(u) != 1 {
		t.Errorf("a ts changed without a record was not reported: %v", u)
	}
	for _, alt := range x.Alternatives {
		if alt.Status == G1ExpExpected && alt.Basis == G1BasisDiff && !slices.Contains(basis, alt.ID) {
			extra := x
			extra.Corrections = append(slices.Clone(x.Corrections), g1TSBasis(alt.ID, "x", 1, "q", "a", "q", 1))
			if _, v := g1cTSOffMap(extra, derived); !slices.Equal(v, []string{alt.ID}) {
				t.Errorf("a ts_basis record of a ts the map gives was not reported: %v", v)
			}
			break
		}
	}
}
