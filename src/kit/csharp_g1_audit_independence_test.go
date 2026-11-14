package kit

import (
	"go/ast"
	"go/parser"
	"go/types"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The independence of the two audits of the differential-oracle alternatives (#233, R-4): the position audit does not read the
// expected types, the type audit does not read the kind map. The proof is structural, in three parts; a syntactic check of
// the audit file is only a fourth, secondary one (TestCSharpG1Audit76NeverNamesTheExpectedTypes and its twin).
//
//  1. What an audit is given is a narrow value: the position audit the witnesses and the source text, the type audit the types.
//     TestCSharpG1AuditInputsAreNarrow pins every member of those types, so the data of the other audit has no member to
//     travel in, and no member is a byte string of the r2 asset to decode (a JSON map access needs bytes).
//  2. A function of an audit cannot name what the input does not carry: a read of the expected asset in the audit file does not
//     type-check (the case S2 below).
//  3. The call closure of an audit reaches no declaration of a G1 file that is not its own, its grounds or its rules: the
//     expected asset, the correction records, the kind map and the other audit are in other files (the cases S below).
//
// The files each audit may reach are listed here; a new file is a reviewed change of this list.
var g1SepAudits = []struct {
	name    string
	roots   []string
	allowed []string
}{
	{"position audit",
		[]string{"DeriveCSharpG1Audit76", "ParseCSharpG1Audit76", "DecodeCSharpG1Audit76", "MarshalCSharpG1Audit76", "CheckG1Audit76Grounds"},
		[]string{"csharp_g1_audit76.go", "csharp_g1_audit76_rules.go", "csharp_g1_ground.go", "csharp_g1_ground_pins.go"}},
	{"type audit",
		[]string{"DeriveCSharpG1TypeAudit76", "ParseCSharpG1TypeAudit76", "DecodeCSharpG1TypeAudit76", "MarshalCSharpG1TypeAudit76", "CheckG1TypeAudit76Grounds"},
		[]string{"csharp_g1_typeaudit76.go", "csharp_g1_audit76.go", "csharp_g1_ground.go", "csharp_g1_ground_pins.go"}},
}

// g1SepAuditOutside lists the declarations of G1 files outside allowed that the closure of the roots reaches.
func g1SepAuditOutside(files map[string]string, roots, allowed []string) ([]string, error) {
	g, err := g1SepParse(files)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range roots {
		if len(g.nodes[r]) == 0 {
			out = append(out, "root "+r+" does not exist")
		}
	}
	for key := range g1SepReach(g, roots) {
		for _, n := range g.nodes[key] {
			if strings.HasPrefix(n.file, "csharp_g1_") && !slices.Contains(allowed, n.file) {
				out = append(out, key+" in "+n.file)
			}
		}
	}
	sort.Strings(out)
	return slices.Compact(out), nil
}

func TestCSharpG1AuditsReachOnlyTheirOwnInputs(t *testing.T) {
	t.Parallel()
	files := g1SepReadPackage(t)
	for _, a := range g1SepAudits {
		got, err := g1SepAuditOutside(files, a.roots, a.allowed)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("%s reaches %v", a.name, got)
		}
	}
}

// g1SepTypeErrors are the type errors of the package files (the standard library and the pinned module package resolve, other
// packages are empty, so the base has errors of its own; a mutant is compared with it by message).
func g1SepTypeErrors(files map[string]string) ([]string, error) {
	fset, imp := g1SepShared()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var parsed []*ast.File
	for _, n := range names {
		f, err := parser.ParseFile(fset, n, files[n], 0)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, f)
	}
	var msgs []string
	conf := types.Config{Importer: imp, Error: func(err error) {
		if te, ok := err.(types.Error); ok {
			msgs = append(msgs, te.Msg)
		}
	}}
	_, _ = conf.Check("kit", fset, parsed, nil)
	return msgs, nil
}

// g1SepWith is the package files with each edit (one place, replaced) and each append applied.
func g1SepWith(t *testing.T, files map[string]string, edits map[string][2]string, appends map[string]string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	for f, e := range edits {
		if strings.Count(out[f], e[0]) != 1 {
			t.Fatalf("%s: %q is not a single place", f, e[0])
		}
		out[f] = strings.Replace(out[f], e[0], e[1], 1)
	}
	for f, a := range appends {
		out[f] += "\n" + a + "\n"
	}
	return out
}

// The bypasses of the review (S, S2 and their kin) written into the real files: each must be found by the proof, and the
// package without them is clean.
func TestCSharpG1AuditsRejectTheBypassesOfTheReview(t *testing.T) {
	t.Parallel()
	files := g1SepReadPackage(t)
	posFill, typFill := "r.Result, r.Relation = g1a76Compare(*r, alt.Witness, text)", "r.Result, r.Relation = g1t76Compare(*r, alt.TS)"
	call := func(f, old, stmt string) map[string][2]string {
		return map[string][2]string{f: {old, stmt + "\n\t" + old}}
	}
	cases := []struct {
		name    string
		audit   int
		mutated map[string]string
		want    string
	}{
		// S: a helper of the expected file reads the field the audit may not know
		{"S, a helper in another file (position audit)", 0, g1SepWith(t, files, call("csharp_g1_audit76.go", posFill, "_ = g1Peek(G1ExpAlt{})"),
			map[string]string{"csharp_g1_expected.go": "func g1Peek(a G1ExpAlt) int { return len(a.TS) }"}), "g1Peek in csharp_g1_expected.go"},
		// S: a helper of the kind map file decodes the map
		{"S, a helper in another file (type audit)", 1, g1SepWith(t, files, call("csharp_g1_typeaudit76.go", typFill, "_ = g1Peek2()"),
			map[string]string{"csharp_g1_kindmap.go": "func g1Peek2() int { var m CSharpG1KindMap; return len(m.Kinds) }"}), "g1Peek2 in csharp_g1_kindmap.go"},
		{"a call of the decoder of the expected asset", 0, g1SepWith(t, files, call("csharp_g1_audit76.go", posFill, "_, _ = DecodeCSharpG1Expected(nil)"), nil), "DecodeCSharpG1Expected in csharp_g1_expected.go"},
		{"a read of the correction table", 0, g1SepWith(t, files, call("csharp_g1_audit76.go", posFill, "_ = len(g1ExpectedCorrections)"), nil), "g1ExpectedCorrections in csharp_g1_correct.go"},
		{"a call of the parser of the kind map", 1, g1SepWith(t, files, call("csharp_g1_typeaudit76.go", typFill, "_, _ = ParseCSharpG1KindMapR2(nil, CSharpG1KindMapSources{})"), nil), "ParseCSharpG1KindMapR2 in csharp_g1_correct.go"},
		{"the position audit calls the type audit", 0, g1SepWith(t, files, call("csharp_g1_audit76.go", posFill, "_ = g1t76Compare"), nil), "g1t76Compare in csharp_g1_typeaudit76.go"},
	}
	for _, a := range g1SepAudits {
		if got, err := g1SepAuditOutside(files, a.roots, a.allowed); err != nil || len(got) != 0 {
			t.Fatalf("%s: the clean package fails: %v %v", a.name, got, err)
		}
	}
	for _, c := range cases {
		got, err := g1SepAuditOutside(c.mutated, g1SepAudits[c.audit].roots, g1SepAudits[c.audit].allowed)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(got, c.want) {
			t.Errorf("%s: %q not in %v", c.name, c.want, got)
		}
	}
	// S2: a decode of the expected asset into a map in the audit file itself (the review read am["ts"]). The input has no bytes of
	// the asset: the statement does not type-check, so it cannot be written without a member that the pinned types lack.
	base, err := g1SepTypeErrors(files)
	if err != nil {
		t.Fatal(err)
	}
	newErrors := func(mutated map[string]string) []string {
		got, err := g1SepTypeErrors(mutated)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, m := range base {
			seen[m] = true
		}
		var out []string
		for _, m := range got {
			if !seen[m] {
				out = append(out, m)
			}
		}
		return out
	}
	anchor := "if len(x.Rows) != len(in.Alts) || len(g1a76Rules) != len(in.Alts) {"
	s2 := newErrors(g1SepWith(t, files, call("csharp_g1_audit76.go", anchor, "var am map[string]any\n\t_ = json.Unmarshal(in.Expected, &am)\n\t_ = am[\"ts\"]"), nil))
	if !slices.ContainsFunc(s2, func(m string) bool { return strings.Contains(m, "in.Expected undefined") }) {
		t.Errorf("S2: the read of the expected bytes in the position audit type-checks (new errors %v)", s2)
	}
	s2t := newErrors(g1SepWith(t, files, call("csharp_g1_typeaudit76.go", typFill, "_ = alt.KindMap"), nil))
	if !slices.ContainsFunc(s2t, func(m string) bool { return strings.Contains(m, "alt.KindMap undefined") }) {
		t.Errorf("S2 for the type audit: a read of a kind map member of the input type-checks (new errors %v)", s2t)
	}
}

// Every member of what an audit is given is pinned: a member for the other audit's data (or for the bytes of the r2 asset) is a
// reviewed change of this list.
func TestCSharpG1AuditInputsAreNarrow(t *testing.T) {
	var walk func(rt reflect.Type, seen map[reflect.Type]bool, out *[]string)
	walk = func(rt reflect.Type, seen map[reflect.Type]bool, out *[]string) {
		switch rt.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Pointer:
			if rt.Kind() == reflect.Map {
				walk(rt.Key(), seen, out)
			}
			walk(rt.Elem(), seen, out)
		case reflect.Struct:
			if seen[rt] {
				return
			}
			seen[rt] = true
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				*out = append(*out, rt.Name()+"."+f.Name+" "+f.Type.String())
				walk(f.Type, seen, out)
			}
		case reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			*out = append(*out, rt.String()+" (a member that can carry anything)")
		}
	}
	for _, c := range []struct {
		name string
		v    any
		want []string
	}{
		{"position audit", G1Audit76Input{}, []string{
			"G1Audit76Alt.ID string", "G1Audit76Alt.Initial kit.G1Audit76Witness", "G1Audit76Alt.Witness kit.G1Audit76Witness",
			"G1Audit76Input.Alts []kit.G1Audit76Alt", "G1Audit76Input.Texts map[string][]uint8",
			"G1Audit76Witness.End int", "G1Audit76Witness.Match string", "G1Audit76Witness.Source string", "G1Audit76Witness.Start int"}},
		{"type audit", G1TypeAudit76Input{}, []string{
			"G1TypeAudit76Alt.ID string", "G1TypeAudit76Alt.InitialTS []string", "G1TypeAudit76Alt.TS []string", "G1TypeAudit76Input.Alts []kit.G1TypeAudit76Alt"}},
	} {
		var got []string
		walk(reflect.TypeOf(c.v), map[reflect.Type]bool{}, &got)
		sort.Strings(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: the members of its input are %q, want %q", c.name, got, c.want)
		}
	}
}
