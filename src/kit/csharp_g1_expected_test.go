package kit

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

type g1ExpAssets struct {
	asset   []byte
	kindmap []byte
	obs     []byte
	in      CSharpG1ExpectedSources
	x       CSharpG1Expected
	want    CSharpG1Expected
	err     error
}

var g1ExpOnce sync.Once
var g1ExpLoaded g1ExpAssets

func g1ExpLoad(t *testing.T) *g1ExpAssets {
	t.Helper()
	g1ExpOnce.Do(func() {
		a := &g1ExpLoaded
		l := g1Load(t)
		read := func(name string) []byte {
			data, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return data
		}
		a.asset = read("../contracts/csharp-g1-expected-b01b02-r1.json")
		a.kindmap = read("../contracts/csharp-g1-kind-map-r1.json")
		a.obs = read("../contracts/csharp-g1-roslyn-observed-r2.json")
		src := l.src
		src.Expected = nil // the expectation is derived before the ledger is sealed
		a.in = CSharpG1ExpectedSources{Src: src, Observation: a.obs, KindMap: a.kindmap}
		if a.err != nil {
			return
		}
		if a.x, a.err = DecodeCSharpG1Expected(a.asset); a.err != nil {
			return
		}
		in := a.in
		in.Sealed = a.x.Metamorphic.Results()
		a.want, a.err = DeriveCSharpG1Expected(in)
	})
	if g1ExpLoaded.err != nil {
		t.Fatal(g1ExpLoaded.err)
	}
	return &g1ExpLoaded
}

// The native comparison step re-pinned the observation (its ledger pin follows the ledger's new states)
// and so changed one member of this asset. Nothing else changed: with the observation pin blanked the
// content digest is the one of the seal commit (76317fb), so no expectation moved.
func TestCSharpG1ExpectedContentIsTheSealedOne(t *testing.T) {
	a := g1ExpLoad(t)
	x, err := DecodeCSharpG1Expected(a.asset)
	if err != nil {
		t.Fatal(err)
	}
	x.Inputs.Observation.SHA256 = ""
	content, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	if got := digestHex(content); got != "3a7c45a292bc05741156ca42e92ffbd0175acd441da80a43e4ad455aa8433b2e" {
		t.Fatalf("an expectation moved after the seal: content digest %s", got)
	}
}

func TestCSharpG1ExpectedSealedAndBound(t *testing.T) {
	t.Parallel()
	a := g1ExpLoad(t)
	if len(a.asset) != 236615 || digestHex(a.asset) != "fe8cc88ef48c2596c391b07c88050539641e985e6cc28c22af98b4f0958fd869" {
		t.Fatalf("sealed expected asset changed: %d/%s", len(a.asset), digestHex(a.asset))
	}
	if len(a.kindmap) != 55247 || digestHex(a.kindmap) != g1KindMapSHA {
		t.Fatalf("sealed kind map changed: %d/%s", len(a.kindmap), digestHex(a.kindmap))
	}
	if err := CheckCSharpG1Expected(a.asset, a.x, a.want); err != nil {
		t.Fatal(err)
	}
	c := a.x.Counts
	if c.Alternatives != 505 || c.Expected != 426 || c.Blocked != 79 || c.SpecDerived != 350 || c.Differential != 76 || c.Witnesses != 626 || c.TokensMismatch != 22 {
		t.Fatalf("counts %+v", c)
	}
	// every alternative of the two rows, in the ledger's order, once
	l := g1Load(t)
	var ids []string
	for _, alt := range l.want.Alternatives {
		if alt.Row == g1ExpRowB01 || alt.Row == g1ExpRowB02 {
			ids = append(ids, alt.ID)
		}
	}
	var got []string
	for _, alt := range a.x.Alternatives {
		got = append(got, alt.ID)
		if alt.Status == G1ExpExpected {
			if alt.Basis != G1BasisSpec && alt.Basis != G1BasisDiff || len(alt.Witnesses) == 0 || len(alt.Witnesses) > g1ExpWitnessLimit || len(alt.TS) == 0 || alt.Spec == "" || alt.Form == "" || alt.Block != "" {
				t.Errorf("%s: malformed expectation %+v", alt.ID, alt)
			}
		} else if alt.Status != G1ExpBlocked || alt.Block == "" || alt.Reason == "" || len(alt.Witnesses) != 0 || alt.Basis != "" {
			t.Errorf("%s: malformed disposition %+v", alt.ID, alt)
		}
	}
	if !slices.Equal(ids, got) {
		t.Fatal("expected asset alternatives differ from the ledger's B01 and B02 alternatives")
	}
	// the ledger carries exactly the EXPECTED alternatives past NO_EXPECTED (SEALED, or EXECUTED once
	// the executed asset ran them) and the BLOCKED ones as BLOCKED with the asset's reason code
	sealed := map[string]bool{}
	for _, id := range a.x.SealedIDs() {
		sealed[id] = true
	}
	blocked := map[string]string{}
	for _, alt := range a.x.Alternatives {
		if alt.Status == G1ExpBlocked {
			blocked[alt.ID] = alt.Block
		}
	}
	n, nb := 0, 0
	for _, alt := range l.want.Alternatives {
		switch {
		case alt.Status == G1Sealed || alt.Status == G1Executed:
			n++
			if !sealed[alt.ID] {
				t.Errorf("%s is %s in the ledger without an expectation", alt.ID, alt.Status)
			}
		case alt.Status == G1Blocked:
			nb++
			if blocked[alt.ID] == "" || alt.Block != blocked[alt.ID] {
				t.Errorf("%s is BLOCKED in the ledger with reason %q, the expected asset says %q", alt.ID, alt.Block, blocked[alt.ID])
			}
		case sealed[alt.ID] || blocked[alt.ID] != "":
			t.Errorf("%s has a disposition in the expected asset but is %s in the ledger", alt.ID, alt.Status)
		}
	}
	if n != 426 || nb != 79 {
		t.Fatalf("ledger carries %d expectations and %d blocked alternatives", n, nb)
	}
	// witnesses sit inside observed accepted sources, with types of the kind-map vocabulary
	in := a.in
	in.Sealed = a.x.Metamorphic.Results()
	_, ctx, _, err := g1ExpContext(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, alt := range a.x.Alternatives {
		for _, ty := range alt.TS {
			if !G1KindMapHasEntry(ctx.km.Vocabulary, ty) {
				t.Errorf("%s: type %s is not in the vocabulary", alt.ID, ty)
			}
		}
		for _, w := range alt.Witnesses {
			s := ctx.byID[w.Source]
			if s == nil || !s.accepted || w.Start < 0 || w.Start > w.End || w.End > len(s.data) || w.Match != "span" && w.Match != "starts-at" {
				t.Errorf("%s: witness %+v", alt.ID, w)
			}
		}
	}
	// no disagreement is left undisposed; the audit and the metamorphic check have their results
	for _, m := range a.x.Cross.Mismatches {
		if m.Disposition == "" || m.Disposition == "UNDISPOSED" || m.Citation == "" || m.Decision == "" {
			t.Errorf("undisposed mismatch %+v", m)
		}
	}
	au := a.x.Audit
	rows := map[string]int{}
	for _, s := range au.Samples {
		rows[s.Row]++
		if s.Outcome != "MATCH" && s.Outcome != "COVERAGE_GAP" || s.Clause == "" {
			t.Errorf("audit sample %+v", s)
		}
	}
	if len(au.Samples) != 26 || rows[g1ExpRowB01] != 21 || rows[g1ExpRowB02] != 5 || au.Estimate.Mismatches != 0 {
		t.Errorf("audit %d samples %v estimate %+v", len(au.Samples), rows, au.Estimate)
	}
	mm := a.x.Metamorphic
	if len(mm.Variants) != g1MetaCount*len(g1MetaTransforms) {
		t.Fatalf("variants %d", len(mm.Variants))
	}
	for _, v := range mm.Variants {
		if !v.Same || v.Structure == "" || v.Structure != v.BaseStructure || v.SHA256 == "" {
			t.Errorf("variant %+v", v)
		}
	}
	if len(a.x.SymbolsContext) != 13 {
		t.Errorf("symbols context %d", len(a.x.SymbolsContext))
	}
	for _, s := range a.x.SymbolsContext {
		if s.Disposition != "DEFAULT_MODE_ONLY_SYMBOLS_CONTEXT_UNJUDGED" || len(s.Rows) == 0 {
			t.Errorf("symbols context %+v", s)
		}
	}
	// the public entry point accepts the tracked bytes
	if _, err := ParseCSharpG1Expected(a.asset, a.in); err != nil {
		t.Fatal(err)
	}
}

// g1KindMapSHA seals the kind map (see TestCSharpG1KindMap).
const g1KindMapSHA = "83322b72849ba6fd5101e206a7c08f1cb9e26f39aa3e8773d6ef4d50d47f0393"

func TestCSharpG1ExpectedRejectsTampering(t *testing.T) {
	t.Parallel()
	a := g1ExpLoad(t)
	clone := func() CSharpG1Expected {
		var x CSharpG1Expected
		b, _ := json.Marshal(a.x)
		if err := json.Unmarshal(b, &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	first := func(x *CSharpG1Expected, status string) *G1ExpAlt {
		for i := range x.Alternatives {
			if x.Alternatives[i].Status == status {
				return &x.Alternatives[i]
			}
		}
		t.Fatal("no alternative with status " + status)
		return nil
	}
	cases := []struct {
		name, code string
		do         func(x *CSharpG1Expected)
	}{
		{"observation pin", "G1_EXPECTED_PIN", func(x *CSharpG1Expected) { x.Inputs.Observation.SHA256 = strings.Repeat("0", 64) }},
		{"kind map pin", "G1_EXPECTED_PIN", func(x *CSharpG1Expected) { x.Inputs.KindMap.Bytes++ }},
		{"source pin", "G1_EXPECTED_PIN", func(x *CSharpG1Expected) { x.Inputs.Ledger.Alternatives.SHA256 = strings.Repeat("1", 64) }},
		{"expectation promoted", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpBlocked).Status = G1ExpExpected }},
		{"expectation dropped", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { x.Alternatives = x.Alternatives[1:] }},
		{"alternative repeated", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { x.Alternatives = append(x.Alternatives, x.Alternatives[0]) }},
		{"alternatives swapped", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { x.Alternatives[0], x.Alternatives[1] = x.Alternatives[1], x.Alternatives[0] }},
		{"witness moved", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpExpected).Witnesses[0].Start++ }},
		{"witness added", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) {
			e := first(x, G1ExpExpected)
			e.Witnesses = append(e.Witnesses, e.Witnesses[0])
		}},
		{"witness dropped", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpExpected).Witnesses = []G1ExpWitness{} }},
		{"type added", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { e := first(x, G1ExpExpected); e.TS = append(e.TS, "block") }},
		{"basis relabelled", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpExpected).Basis = G1BasisDiff }},
		{"block reason changed", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpBlocked).Block = "FACT_DESCRIPTION" }},
		{"spec clause changed", "G1_EXPECTED_ALTERNATIVES", func(x *CSharpG1Expected) { first(x, G1ExpExpected).Spec = "x" }},
		{"mismatch dropped", "G1_EXPECTED_CROSS", func(x *CSharpG1Expected) { x.Cross.Mismatches = x.Cross.Mismatches[1:] }},
		{"mismatch redisposed", "G1_EXPECTED_CROSS", func(x *CSharpG1Expected) { x.Cross.Mismatches[0].Disposition = "ROSLYN_DEVIATION" }},
		{"token count", "G1_EXPECTED_CROSS", func(x *CSharpG1Expected) { x.Cross.Tokens-- }},
		{"symbols step dropped", "G1_EXPECTED_SYMBOLS", func(x *CSharpG1Expected) { x.SymbolsContext = x.SymbolsContext[1:] }},
		{"audit sample dropped", "G1_EXPECTED_AUDIT", func(x *CSharpG1Expected) { x.Audit.Samples = x.Audit.Samples[1:] }},
		{"audit outcome edited", "G1_EXPECTED_AUDIT", func(x *CSharpG1Expected) { x.Audit.Samples[0].Outcome = "MISMATCH" }},
		{"audit estimate edited", "G1_EXPECTED_AUDIT", func(x *CSharpG1Expected) { x.Audit.Estimate.Mismatches = 1 }},
		{"audit seed edited", "G1_EXPECTED_AUDIT", func(x *CSharpG1Expected) { x.Audit.Seed = "x" }},
		{"variant dropped", "G1_EXPECTED_METAMORPHIC", func(x *CSharpG1Expected) { x.Metamorphic.Variants = x.Metamorphic.Variants[1:] }},
		{"variant source edited", "G1_EXPECTED_METAMORPHIC", func(x *CSharpG1Expected) { x.Metamorphic.Variants[0].SHA256 = strings.Repeat("2", 64) }},
		{"variant said same", "G1_EXPECTED_METAMORPHIC", func(x *CSharpG1Expected) { x.Metamorphic.Variants[0].Same = !x.Metamorphic.Variants[0].Same }},
		{"counts", "G1_EXPECTED_COUNTS", func(x *CSharpG1Expected) { x.Counts.Expected++ }},
	}
	for _, f := range cases {
		x := clone()
		f.do(&x)
		data, err := MarshalCSharpG1Expected(x)
		if err != nil {
			t.Fatal(err)
		}
		// the sealed digests come from the asset itself, so a changed digest is judged by a fresh derivation
		want := a.want
		if strings.HasPrefix(f.name, "variant") {
			in := a.in
			in.Sealed = x.Metamorphic.Results()
			if want, err = DeriveCSharpG1Expected(in); err != nil {
				t.Fatal(err)
			}
		}
		if err := CheckCSharpG1Expected(data, x, want); g1Code(err) != f.code {
			t.Errorf("%s: %v, want %s", f.name, err, f.code)
		}
	}
	// a changed Roslyn digest: the asset says the variant is the same while the digest differs
	x := clone()
	x.Metamorphic.Variants[0].Structure = strings.Repeat("3", 64)
	in := a.in
	in.Sealed = x.Metamorphic.Results()
	want, err := DeriveCSharpG1Expected(in)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := MarshalCSharpG1Expected(x)
	if err := CheckCSharpG1Expected(data, x, want); g1Code(err) != "G1_EXPECTED_METAMORPHIC" {
		// the asset is self-consistent with a different digest and Same=false: it must at least be unequal to the tracked bytes
		if bytes.Equal(data, a.asset) {
			t.Errorf("digest tamper left the bytes unchanged: %v", err)
		}
	}
	// an unsealed variant (no Roslyn digest) and an undisposed mismatch make an asset unsealed
	x = clone()
	x.Metamorphic.Variants[0].Structure, x.Metamorphic.Variants[0].Same = "", false
	in.Sealed = x.Metamorphic.Results()
	if want, err = DeriveCSharpG1Expected(in); err != nil {
		t.Fatal(err)
	}
	data, _ = MarshalCSharpG1Expected(x)
	if err := CheckCSharpG1Expected(data, x, want); g1Code(err) != "G1_EXPECTED_METAMORPHIC_UNSEALED" {
		t.Errorf("unsealed variant: %v", err)
	}
	// byte-level forgeries
	text := string(a.asset)
	for name, f := range map[string]struct{ data []byte }{
		"truncated":      {a.asset[:len(a.asset)/2]},
		"empty":          {nil},
		"oversize":       {append(bytes.Clone(a.asset), bytes.Repeat([]byte(" "), csharpG1ExpectedMax)...)},
		"unknown member": {[]byte(strings.Replace(text, `"basis":`, `"extra": 1,`+"\n"+` "basis":`, 1))},
		"schema":         {[]byte(strings.Replace(text, CSharpG1ExpectedSchema, "x", 1))},
	} {
		x, err := DecodeCSharpG1Expected(f.data)
		if err == nil {
			err = CheckCSharpG1Expected(f.data, x, a.want)
		}
		want := map[string]string{"truncated": "G1_EXPECTED_JSON", "empty": "G1_EXPECTED_JSON", "oversize": "G1_EXPECTED_SIZE", "unknown member": "G1_EXPECTED_JSON", "schema": "G1_EXPECTED_SCHEMA"}[name]
		if g1Code(err) != want {
			t.Errorf("%s: %v, want %s", name, err, want)
		}
	}
	// whitespace the canonical writer would not write
	spaced := bytes.Replace(a.asset, []byte("\n \"basis\""), []byte("\n  \"basis\""), 1)
	if x, err := DecodeCSharpG1Expected(spaced); err != nil || g1Code(CheckCSharpG1Expected(spaced, x, a.want)) != "G1_EXPECTED_BYTES" {
		t.Errorf("non-canonical bytes accepted: %v", err)
	}
}

// ---- separation of the deriver, the observation and native input ----------------------------

func g1ParseFile(t *testing.T, name string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// g1Mentions returns the identifiers and string literals of a file's code (comments excluded).
func g1Mentions(f *ast.File) (idents, strs []string) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			idents = append(idents, x.Name)
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				strs = append(strs, x.Value)
			}
		}
		return true
	})
	return
}

func g1Imports(f *ast.File) []string {
	var out []string
	for _, im := range f.Imports {
		out = append(out, strings.Trim(im.Path.Value, `"`))
	}
	return out
}

func TestCSharpG1ExpectedSeparation(t *testing.T) {
	t.Parallel()
	deriver := []string{"csharp_g1_spec.go"}
	derivation := []string{"csharp_g1_spec.go", "csharp_g1_expected.go", "csharp_g1_expected_rules.go", "csharp_g1_expected_checks.go", "csharp_g1_expected_data.go", "csharp_g1_kindmap.go"}
	// the derivation reads bytes it is handed: no file, process, network or native package
	forbiddenImport := regexp.MustCompile(`^(os|os/exec|io/ioutil|io/fs|net|net/http|syscall|unsafe|plugin|` +
		`github\.com/wotjr1649/tree-sitter-grammar-kit/src/(internal/(native|runner|reproduce)|kit/(nativewire|querywire|reuseobs)))`)
	nativeWord := regexp.MustCompile(`(?i)native|reproduce|runner|\bresponses?\b|primary_cst`)
	noAlt := regexp.MustCompile(`(?i)alternativ`) // "alternative" holds the letters of "native"
	for _, name := range derivation {
		f := g1ParseFile(t, name)
		for _, im := range g1Imports(f) {
			if forbiddenImport.MatchString(im) {
				t.Errorf("%s imports %s", name, im)
			}
		}
		idents, strs := g1Mentions(f)
		for _, id := range idents {
			if nativeWord.MatchString(noAlt.ReplaceAllString(id, "")) {
				t.Errorf("%s: identifier %s names native input", name, id)
			}
		}
		for _, s := range strs {
			if nativeWord.MatchString(noAlt.ReplaceAllString(s, "")) && !strings.Contains(s, "native output") {
				t.Errorf("%s: string %s names native input", name, s)
			}
		}
	}
	// the spec deriver reads neither the observation nor the kind map nor the expectation
	for _, name := range deriver {
		f := g1ParseFile(t, name)
		for _, im := range g1Imports(f) {
			if !slices.Contains([]string{"regexp", "strings", "unicode/utf8"}, im) {
				t.Errorf("%s imports %s", name, im)
			}
		}
		idents, strs := g1Mentions(f)
		bad := regexp.MustCompile(`(?i)roslyn|kindmap|observ|expected|ledger|alternative|g1km`)
		for _, id := range idents {
			if bad.MatchString(id) {
				t.Errorf("%s: identifier %s reads another source", name, id)
			}
		}
		for _, s := range strs {
			if bad.MatchString(s) {
				t.Errorf("%s: string %s reads another source", name, s)
			}
		}
	}
	// the observation and the kind map do not read the expectation or the deriver
	for _, name := range []string{"csharp_g1_roslyn.go", "csharp_g1_kindmap.go"} {
		idents, _ := g1Mentions(g1ParseFile(t, name))
		for _, id := range idents {
			if strings.HasPrefix(id, "g1s") || strings.Contains(id, "Expected") && name != "csharp_g1_roslyn.go" || id == "CSharpG1Expected" || id == "g1Rule" {
				t.Errorf("%s: identifier %s reads the expectation side", name, id)
			}
		}
	}
	// the pinned inputs of the asset are declarations and the two observation-side assets, nothing else
	a := g1ExpLoad(t)
	var pins []string
	l := a.x.Inputs.Ledger
	for _, p := range []string{l.Inventory.Path, l.Alternatives.Path, l.FactMapping.Path} {
		pins = append(pins, p)
	}
	for _, f := range l.Fixtures {
		pins = append(pins, f.Path)
	}
	pins = append(pins, a.x.Inputs.Observation.Path, a.x.Inputs.KindMap.Path)
	allowed := []string{
		"src/contracts/qualification-c1.json", "src/contracts/feature-alternatives.json", "src/contracts/fact-mapping.json",
		"src/testdata/native/routes/csharp.json", "src/testdata/native/gaps/csharp.json", "src/testdata/native/n461/csharp.json", "src/testdata/native/queries/csharp.json",
		CSharpG1RoslynPath, CSharpG1KindMapPath,
	}
	if !slices.Equal(pins, allowed) {
		t.Errorf("pinned inputs %v", pins)
	}
	// the native fixtures are case sources (source text and edits); their declared native outputs do not exist there
	for _, f := range l.Fixtures {
		data, err := os.ReadFile("../../" + f.Path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Schema string `json:"schema"`
			Cases  []map[string]json.RawMessage
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		for _, c := range doc.Cases {
			for k := range c {
				if regexp.MustCompile(`(?i)response|result|raw_output|actual|observed`).MatchString(k) {
					t.Errorf("%s: case member %s looks like native output", f.Path, k)
				}
			}
		}
	}
	// the derivation is repeatable: the same inputs give the same asset. That alone does not show that it
	// reads only its inputs; TestCSharpG1SeparationClosure (the call closure) and
	// TestCSharpG1ExpectedFollowsItsInputsOnly (another observation gives another asset) do.
	again, err := DeriveCSharpG1Expected(CSharpG1ExpectedSources{Src: a.in.Src, Observation: a.obs, KindMap: a.kindmap, Sealed: a.x.Metamorphic.Results()})
	if err != nil || !g1eSame(again, a.want) {
		t.Errorf("derivation is not a function of its inputs: %v", err)
	}
}

// ---- the derivation's own units --------------------------------------------------------------

func TestCSharpG1SpecForms(t *testing.T) {
	yes := map[string][]string{
		"int.dec": {"0", "1_000", "12u", "7UL"}, "int.hex": {"0x1F", "0XdEaD_beef", "0x1_Fu"}, "int.bin": {"0b101", "0B1_0L"},
		"real.1": {"1.5", "1_0.2_5e+3f", "0.0M"}, "real.2": {".5", ".5e-2d"}, "real.3": {"1e10", "2E+3F"}, "real.4": {"1f", "3D", "4m"},
		"char": {`'a'`, `'\n'`, `'\x41'`, `'\u0041'`, `'\U00000041'`, `'\''`}, "string.regular": {`""`, `"a\tb"`, `"x"u8`},
		"string.verbatim": {`@""`, `@"a""b"`, `@"c:\x"`}, "ident.basic": {"a", "_x", "é1", `\u0041b`}, "ident.escaped": {"@class", "@_"},
	}
	for form, texts := range yes {
		for _, s := range texts {
			if !g1sForms[form].MatchString(s) {
				t.Errorf("%s should match %s", form, s)
			}
		}
	}
	no := map[string][]string{
		"int.dec": {"", "_1", "1_", "1x", "0x"}, "int.hex": {"0x", "0x_", "0xg"}, "int.bin": {"0b2", "0b"},
		"real.1": {"1.", "1.e3", ".5."}, "char": {`''`, `'ab'`, `'\q'`, "'\n'"}, "string.regular": {`"a`, "\"a\nb\"", `"\q"`}, "string.verbatim": {`@"a"b"`},
		"ident.basic": {"1a", "", "a-b"}, "ident.escaped": {"class", "@"},
	}
	for form, texts := range no {
		for _, s := range texts {
			if g1sForms[form].MatchString(s) {
				t.Errorf("%s should not match %q", form, s)
			}
		}
	}
	// maximal-munch escapes
	got := g1sEscapes(`"\x41B\n\u0041\U00000041\q"`)
	var forms []string
	for _, e := range got {
		forms = append(forms, e.Form)
	}
	if !slices.Equal(forms, []string{"hex", "simple:n", "unicode4", "unicode8"}) || got[0].End-got[0].Start != 5 {
		t.Errorf("escapes %v", got)
	}
	// comments between tokens; directive lines and disabled text are opaque
	src := []byte("a // c1\n/* c2 */ b #if X // not a comment here\n/* hidden */\n#endif")
	cs := g1sComments(src, 1, len(src), [][2]int{{19, len(src)}})
	if len(cs) != 2 || cs[0].Kind != "line-comment" || cs[1].Kind != "delimited-comment" || string(src[cs[1].Start:cs[1].End]) != "/* c2 */" {
		t.Errorf("comments %+v", cs)
	}
	if g1sDirectiveKind("  # region x") != "region" || g1sDirectiveKind("#iffy") != "" || g1sDirectiveKind("#if") != "if" || g1sDirectiveKind("#!/bin/sh") != "" {
		t.Error("directive kinds")
	}
	if f := g1sCondForms("#if A && !(B || true) // c"); !slices.Equal(f, []string{"or", "and", "not", "paren", "true", "symbol"}) && !slices.Equal(f, []string{"and", "or", "not", "paren", "true", "symbol"}) {
		// order is the fixed order of g1sCondForms
		t.Errorf("condition forms %v", f)
	}
	if k, a, b := g1sLineForm(`#line 5 "f.cs"`); k != "number-file" || `#line 5 "f.cs"`[a:b] != `"f.cs"` {
		t.Errorf("line form %s", k)
	}
	if k, _, _ := g1sLineForm("#line hidden"); k != "hidden" {
		t.Errorf("line form %s", k)
	}
	if act, _, tgt, _ := g1sNullableParts("#nullable enable warnings"); act != "enable" || tgt != "warnings" {
		t.Error("nullable parts")
	}
}

func TestCSharpG1ExpectedMetamorphicVariants(t *testing.T) {
	t.Parallel()
	a := g1ExpLoad(t)
	vs, err := G1MetaVariantSources(a.in)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != len(a.x.Metamorphic.Variants) {
		t.Fatalf("%d variants, asset %d", len(vs), len(a.x.Metamorphic.Variants))
	}
	_, c, _, err := g1ExpContext(a.in)
	if err != nil {
		t.Fatal(err)
	}
	perTransform := map[string]int{}
	for i, v := range vs {
		w := a.x.Metamorphic.Variants[i]
		id, tr, _ := strings.Cut(v.ID, "|")
		if v.ID != w.Source+"|"+w.Transform || v.SHA256 != w.SHA256 || len(v.Data) != w.Bytes {
			t.Errorf("variant %d: %s differs from the asset", i, v.ID)
		}
		// a transform that changes nothing proves nothing
		if bytes.Equal(v.Data, c.byID[id].data) {
			t.Errorf("%s: the transform left the source unchanged", v.ID)
		}
		if !g1sValidUTF8(v.Data) {
			t.Errorf("%s: not UTF-8", v.ID)
		}
		perTransform[tr]++
	}
	for _, tr := range g1MetaTransforms {
		if perTransform[tr.ID] != g1MetaCount {
			t.Errorf("transform %s: %d variants", tr.ID, perTransform[tr.ID])
		}
	}
}

func mustRuleSrcs(t *testing.T, in CSharpG1ExpectedSources) []g1RuleSrc {
	t.Helper()
	r, err := g1ExpRuleSrcs(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestCSharpG1AuditDetectsWrongHand: the audit must report a hand derivation that disagrees with the
// expectation, in each of its four ways, and must not report one that agrees.
func TestCSharpG1AuditDetectsWrongHand(t *testing.T) {
	a := g1ExpLoad(t)
	_, ctx, _, err := g1ExpContext(a.in)
	if err != nil {
		t.Fatal(err)
	}
	recs := mustRuleSrcs(t, a.in)
	prod := map[string]string{}
	for _, r := range recs {
		prod[r.ID] = r.Production
	}
	run := func() G1ExpAudit {
		au, err := g1Audit(ctx, a.want.Alternatives, prod)
		if err != nil {
			t.Fatal(err)
		}
		return au
	}
	if run().Estimate.Mismatches != 0 {
		t.Fatal("the tracked hand derivations disagree")
	}
	expected, blocked := "csharp-B01.a002", "csharp-B01.a021" // an expectation and a disposition of the sample
	for _, c := range []struct {
		name, id string
		edit     func(e *g1AuditEntry)
	}{
		{"status", expected, func(e *g1AuditEntry) { e.Status, e.Block = G1ExpBlocked, g1BlockNoNode }},
		{"status with the same empty reason", expected, func(e *g1AuditEntry) { e.Status, e.Block, e.TS, e.Text = G1ExpBlocked, "", nil, "" }},
		{"disposition code", blocked, func(e *g1AuditEntry) { e.Block = g1BlockMeta }},
		{"type", expected, func(e *g1AuditEntry) { e.TS = []string{"block"} }},
		{"text", expected, func(e *g1AuditEntry) { e.Text = `^nul$` }},
		{"expectation where blocked", blocked, func(e *g1AuditEntry) { e.Status, e.Block, e.TS, e.Text = G1ExpExpected, "", []string{"block"}, `x` }},
	} {
		old := csharpG1AuditHand[c.id]
		e := old
		c.edit(&e)
		csharpG1AuditHand[c.id] = e
		got := run()
		csharpG1AuditHand[c.id] = old
		if got.Estimate.Mismatches != 1 {
			t.Errorf("%s: %d mismatches, want 1", c.name, got.Estimate.Mismatches)
		}
	}
	// a sample without a hand derivation is an error, not a silent pass
	old := csharpG1AuditHand[expected]
	delete(csharpG1AuditHand, expected)
	_, err = g1Audit(ctx, a.want.Alternatives, prod)
	csharpG1AuditHand[expected] = old
	if err == nil {
		t.Error("a sample without a hand derivation was accepted")
	}
	// the sample is the smallest hashes of each row, at least one per row, about 5 percent in all
	ids := map[string][]string{}
	for _, r := range recs {
		ids[r.Row] = append(ids[r.Row], r.ID)
	}
	if p := g1AuditPick(ids); len(p) != 26 {
		t.Errorf("sample %d", len(p))
	}
	if p := g1AuditPick(map[string][]string{g1ExpRowB01: {"x"}, g1ExpRowB02: {"y"}}); len(p) != 2 {
		t.Errorf("minimum of one per row: %v", p)
	}
}
