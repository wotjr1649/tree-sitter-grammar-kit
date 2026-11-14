package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// The native comparison of the sealed B03 and B04 expectations (#233 G1-3b PR3). The context (one ledger derivation) is built once per
// process; the mutants edit a copy of the decoded asset and are checked against that context, they never derive again.

type g1bxAssets struct {
	asset []byte
	in    CSharpG1B34ExecutedSources
	x     CSharpG1B34Executed
	ctx   g1bxContext
	err   error
}

var g1bxOnce sync.Once
var g1bxLoaded g1bxAssets

func g1bxLoad(t *testing.T) *g1bxAssets {
	t.Helper()
	g1bxOnce.Do(func() {
		a := &g1bxLoaded
		read := func(name string) []byte {
			d, err := os.ReadFile(name)
			if err != nil && a.err == nil {
				a.err = err
			}
			return d
		}
		a.asset = read("../contracts/csharp-g1-executed-b03b04-r1.json")
		a.in = CSharpG1B34ExecutedSources{Src: g1Load(t).src, Expected: read("../contracts/csharp-g1-expected-b03b04-r1.json"), Observation: read("../contracts/csharp-g1-roslyn-observed-r3.json")}
		if a.err != nil {
			return
		}
		if a.x, a.err = DecodeCSharpG1B34Executed(a.asset); a.err != nil {
			return
		}
		a.ctx, a.err = g1bxContextOf(a.in)
	})
	if g1bxLoaded.err != nil {
		t.Fatal(g1bxLoaded.err)
	}
	return &g1bxLoaded
}

func g1bxClone(t *testing.T, x CSharpG1B34Executed) CSharpG1B34Executed {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var y CSharpG1B34Executed
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	return y
}

// g1bxCheckEdited re-marshals an edited asset and checks it offline against the shared context.
func g1bxCheckEdited(t *testing.T, a *g1bxAssets, y CSharpG1B34Executed) error {
	t.Helper()
	data, err := MarshalCSharpG1B34Executed(y)
	if err != nil {
		t.Fatal(err)
	}
	x, err := DecodeCSharpG1B34Executed(data)
	if err != nil {
		return err
	}
	return g1bxCheck(data, x, a.ctx, a.in)
}

func g1bxAlt(x *CSharpG1B34Executed, id string) *G1B34ExecAlt {
	for i := range x.Alternatives {
		if x.Alternatives[i].ID == id {
			return &x.Alternatives[i]
		}
	}
	return nil
}

func TestCSharpG1B34ExecutedSealedAndBound(t *testing.T) {
	a := g1bxLoad(t)
	if err := g1bxCheck(a.asset, a.x, a.ctx, a.in); err != nil {
		t.Fatalf("the tracked asset does not verify offline: %v", err)
	}
	if a.x.Schema != CSharpG1B34ExecutedSchema || a.x.Inputs.Expected != queryregistrycore.FilePin(CSharpG1B34Path, a.in.Expected) || a.x.Inputs.Observation.Path != CSharpG1RoslynPathR3 {
		t.Errorf("schema or pins: %s %+v", a.x.Schema, a.x.Inputs)
	}
	// the numbers of the comparison: 296 EXPECTED alternatives, 291 MATCH (group A in the tiers word, candidate and operator and weak, group B
	// discriminating and weak apart), 5 MISMATCH
	want := G1B34ExecCounts{Alternatives: 296, Executed: 296, Match: 291, Mismatch: 5, MatchStrongA: 62, MatchCandidateA: 19, MatchOperatorA: 27, MatchWeakA: 86, MatchStrongB: 71, MatchWeakB: 26,
		Witnesses: 296, Parts: 425, PartMatch: 420, PartMismatch: 5, Sources: 23, SourcesCompleted: 23}
	if a.x.Counts != want {
		t.Errorf("counts %+v, want %+v", a.x.Counts, want)
	}
	if got := a.x.Counts.MatchStrongA + a.x.Counts.MatchCandidateA + a.x.Counts.MatchOperatorA + a.x.Counts.MatchWeakA + a.x.Counts.MatchStrongB + a.x.Counts.MatchWeakB; got != a.x.Counts.Match {
		t.Errorf("the six match counts add to %d, not %d", got, a.x.Counts.Match)
	}
	// every alternative is the sealed one: same ID, group, discriminating flag and witness source
	sealed := a.ctx.exp.ExpectedIDs()
	if len(a.x.Alternatives) != len(sealed) {
		t.Fatalf("%d alternatives, sealed %d", len(a.x.Alternatives), len(sealed))
	}
	for i, alt := range a.x.Alternatives {
		if alt.ID != sealed[i] || !alt.Executed {
			t.Errorf("%s: position %d, executed %v", alt.ID, i, alt.Executed)
		}
		for _, w := range alt.Witnesses {
			if w.Match != g1b34MatchTokens && w.Match != g1b34MatchSpan {
				t.Errorf("%s: match %q is not a documented value", alt.ID, w.Match)
			}
			if (alt.Group == G1ClassLiteral) != (w.Match == g1b34MatchTokens) {
				t.Errorf("%s: group %s with match %s", alt.ID, alt.Group, w.Match)
			}
		}
	}
	// the executed asset does not own a copy of the sealed values: they are the sealed asset's, byte for byte, by the check above
	if !bytes.Equal(a.in.Expected, mustRead(t, "../contracts/csharp-g1-expected-b03b04-r1.json")) {
		t.Fatal("the expected asset changed")
	}
}

// Group A is split in tiers (review of PR2, user decision on #233): the sealed asset and its `discriminating` flag are unchanged, and only the
// word tier counts as discriminating in the executed asset. The tier follows from the sealed anchors alone.
func TestCSharpG1B34ExecutedTiers(t *testing.T) {
	a := g1bxLoad(t)
	if g1bxWord.String() != g1clsWord.String() {
		t.Fatalf("the comparison's word rule %s is not the classifier's %s", g1bxWord, g1clsWord)
	}
	sealed := map[string]G1B34Alt{}
	for _, s := range a.ctx.exp.Alternatives {
		sealed[s.ID] = s
	}
	tiers := map[string][]string{}
	for _, alt := range a.x.Alternatives {
		s := sealed[alt.ID]
		isA := alt.Group == G1ClassLiteral
		switch {
		case isA && alt.Discriminating:
			if alt.Tier != G1B34TierWord && alt.Tier != G1B34TierCandidate && alt.Tier != G1B34TierOperator {
				t.Errorf("%s: a discriminating group A alternative with tier %q", alt.ID, alt.Tier)
			}
		case alt.Tier != "":
			t.Errorf("%s: tier %q on a weak or group B alternative", alt.ID, alt.Tier)
		}
		tiers[alt.Tier] = append(tiers[alt.Tier], alt.ID)
		// the three tiers by their definition, read from the sealed anchors
		if alt.Tier == G1B34TierWord && !slices.ContainsFunc(s.Anchors, func(x G1B34Anchor) bool { return g1clsWord.MatchString(x.Text) && len(x.TS) == 1 }) ||
			alt.Tier == G1B34TierCandidate && (!slices.ContainsFunc(s.Anchors, func(x G1B34Anchor) bool { return g1clsWord.MatchString(x.Text) && len(x.TS) > 1 }) ||
				slices.ContainsFunc(s.Anchors, func(x G1B34Anchor) bool { return g1clsWord.MatchString(x.Text) && len(x.TS) == 1 })) ||
			alt.Tier == G1B34TierOperator && slices.ContainsFunc(s.Anchors, func(x G1B34Anchor) bool { return g1clsWord.MatchString(x.Text) }) {
			t.Errorf("%s: tier %s does not follow from the anchors %v", alt.ID, alt.Tier, s.Anchors)
		}
	}
	// the numbers: 108 discriminating group A alternatives of the sealed asset are 62 word, 19 candidate and 27 operator (all MATCH)
	if len(tiers[G1B34TierWord]) != 62 || len(tiers[G1B34TierCandidate]) != 19 || len(tiers[G1B34TierOperator]) != 27 {
		t.Errorf("tiers: word %d, candidate %d, operator %d", len(tiers[G1B34TierWord]), len(tiers[G1B34TierCandidate]), len(tiers[G1B34TierOperator]))
	}
	// the review's examples: a keyword that may also be a modifier is a candidate set, an operator alone is the operator tier, a keyword is word
	for id, want := range map[string]string{"csharp-B03.a075": G1B34TierCandidate, "csharp-B04.a055": G1B34TierCandidate, "csharp-B03.a166": G1B34TierOperator,
		"csharp-B03.a164": G1B34TierWord, "csharp-B04.a070": G1B34TierWord} {
		if got := g1bxAlt(&a.x, id).Tier; got != want {
			t.Errorf("%s: tier %q, want %q", id, got, want)
		}
	}
	// a tier that is claimed or hidden is caught: it is part of the check against the sealed asset, and the counts follow it
	for _, tc := range []struct {
		name string
		edit func(y *CSharpG1B34Executed)
		code string
	}{
		{"a candidate claimed as word", func(y *CSharpG1B34Executed) { g1bxAlt(y, tiers[G1B34TierCandidate][0]).Tier = G1B34TierWord }, "G1_B34X_EXPECTATION"},
		{"an operator claimed as word", func(y *CSharpG1B34Executed) { g1bxAlt(y, tiers[G1B34TierOperator][0]).Tier = G1B34TierWord }, "G1_B34X_EXPECTATION"},
		{"a tier hidden", func(y *CSharpG1B34Executed) { g1bxAlt(y, tiers[G1B34TierWord][0]).Tier = "" }, "G1_B34X_EXPECTATION"},
		{"a tier on a weak alternative", func(y *CSharpG1B34Executed) { g1bxAlt(y, tiers[""][0]).Tier = G1B34TierWord }, "G1_B34X_EXPECTATION"},
		{"counts of one tier moved to the word tier", func(y *CSharpG1B34Executed) { y.Counts.MatchStrongA++; y.Counts.MatchOperatorA-- }, "G1_B34X_COUNTS"},
	} {
		y := g1bxClone(t, a.x)
		tc.edit(&y)
		if err := g1bxCheckEdited(t, a, y); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.code)
		}
	}
	// the rule on its own: no anchor words or all words with sets
	for _, tc := range []struct {
		name string
		a    G1B34Alt
		want string
	}{
		{"weak", G1B34Alt{Group: G1ClassLiteral, Anchors: []G1B34Anchor{{Text: "x", TS: []string{"y"}}}}, ""},
		{"group B", G1B34Alt{Group: G1ClassStructure, Discriminating: true}, ""},
		{"operator", G1B34Alt{Group: G1ClassLiteral, Discriminating: true, Anchors: []G1B34Anchor{{Text: "==", TS: []string{`"=="`}}}}, G1B34TierOperator},
		{"set", G1B34Alt{Group: G1ClassLiteral, Discriminating: true, Anchors: []G1B34Anchor{{Text: "new", TS: []string{`"new"`, "modifier"}}}}, G1B34TierCandidate},
		{"a word with one type beside a set", G1B34Alt{Group: G1ClassLiteral, Discriminating: true, Anchors: []G1B34Anchor{{Text: "ref", TS: []string{`"ref"`, "modifier"}}, {Text: "while", TS: []string{`"while"`}}}}, G1B34TierWord},
		{"a set beside an operator", G1B34Alt{Group: G1ClassLiteral, Discriminating: true, Anchors: []G1B34Anchor{{Text: "=>", TS: []string{`"=>"`}}, {Text: "ref", TS: []string{`"ref"`, "modifier"}}}}, G1B34TierCandidate},
	} {
		if got := g1bxTier(tc.a); got != tc.want {
			t.Errorf("%s: tier %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The raw native observation is a local artifact: when it is configured, the asset must be exactly the comparison re-derived from it.
func TestCSharpG1B34ExecutedMatchesRawNative(t *testing.T) {
	path := os.Getenv("TSGK_G1_NATIVE_RAW_B34")
	if path == "" {
		t.Skip("TSGK_G1_NATIVE_RAW_B34 (the raw observation of csharp-g1-native -set b34) is not configured")
	}
	a := g1bxLoad(t)
	in := a.in
	var err error
	if in.Raw, err = os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSharpG1B34Executed(a.asset, in); err != nil {
		t.Fatal(err)
	}
	if a.x.Inputs.Raw != queryregistrycore.FilePin("csharp-g1-native-raw", in.Raw) {
		t.Errorf("the raw pin %+v is not the file's", a.x.Inputs.Raw)
	}
}

func TestCSharpG1B34NativeSourceIDs(t *testing.T) {
	a := g1bxLoad(t)
	ids := CSharpG1B34NativeSourceIDs(a.ctx.exp)
	if len(ids) != 23 || !slices.IsSorted(ids) || !slices.Equal(ids, a.ctx.ids) {
		t.Fatalf("%d ids %v", len(ids), ids)
	}
	witness := map[string]bool{}
	for _, alt := range a.ctx.exp.Alternatives {
		for _, w := range alt.Witnesses {
			witness[w.Source] = alt.Status == G1ExpExpected || witness[w.Source]
		}
	}
	for _, id := range ids {
		if !witness[id] {
			t.Errorf("%s is no witness source of an EXPECTED alternative", id)
		}
	}
	if len(CSharpG1B34NativeSourceIDs(CSharpG1B34{})) != 0 {
		t.Error("an empty asset names sources")
	}
}

// ---- synthetic native observations ----------------------------------------------------------------

// g1bxSynthetic builds a raw native observation in which every part of every witness has a node of its first sealed type with its span, then
// lets mutate change a source. It tests the comparison logic only: it is made from the expected asset and is never evidence of native
// behaviour.
func g1bxSynthetic(t *testing.T, a *g1bxAssets, mutate func(s *G1NativeSource)) []byte {
	t.Helper()
	byID := map[string]*G1NativeSource{}
	for _, id := range a.ctx.ids {
		src := a.ctx.sources[id]
		byID[id] = &G1NativeSource{ID: id, SHA256: src.SHA256, Bytes: len(src.Data), Status: G1NativeCompleted, Types: []string{"compilation_unit"},
			Nodes: [][5]int{{0, G1NodeNamed, 0, len(src.Data), -1}}}
	}
	for _, alt := range a.ctx.exp.Alternatives {
		if alt.Status != G1ExpExpected {
			continue
		}
		for _, w := range alt.Witnesses {
			parts, ok := g1bxParts(alt, w)
			if !ok {
				t.Fatalf("%s: no parts", alt.ID)
			}
			s := byID[w.Source]
			for _, p := range parts {
				i := slices.Index(s.Types, p.TS[0])
				if i < 0 {
					i = len(s.Types)
					s.Types = append(s.Types, p.TS[0])
				}
				s.Nodes = append(s.Nodes, [5]int{i, 0, p.Start, p.End, 0})
			}
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

func TestCSharpG1B34ExecutedSyntheticComparison(t *testing.T) {
	a := g1bxLoad(t)
	in := a.in
	in.Raw = g1bxSynthetic(t, a, nil)
	x, err := g1bxDerive(a.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	// every part has a node of a sealed type with its span: all MATCH, nothing to dispose of
	if x.Counts.Match != 296 || x.Counts.Mismatch != 0 || x.Counts.PartMatch != 425 || len(x.Dispositions) != 0 {
		t.Fatalf("all-match counts %+v", x.Counts)
	}
	data, err := MarshalCSharpG1B34Executed(x)
	if err != nil {
		t.Fatal(err)
	}
	y, err := DecodeCSharpG1B34Executed(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := g1bxCheck(data, y, a.ctx, in); err != nil {
		t.Fatalf("all-match asset: %v", err)
	}
	// the asset is the comparison re-derived from the raw observation, and not from another one
	other := in
	other.Raw = g1bxSynthetic(t, a, func(s *G1NativeSource) { s.Types = append(s.Types, "unused") })
	if err := g1bxCheck(data, y, a.ctx, other); g1Code(err) != "G1_B34X_RAW" {
		t.Errorf("another raw observation: %v", err)
	}

	// pick a group A alternative with at least two anchors and a group B alternative
	var two, spanAlt *G1B34Alt
	for i := range a.ctx.exp.Alternatives {
		alt := &a.ctx.exp.Alternatives[i]
		switch {
		case alt.Status != G1ExpExpected:
		case alt.Group == G1ClassLiteral && len(alt.Anchors) >= 2 && two == nil:
			two = alt
		case alt.Group == G1ClassStructure && spanAlt == nil:
			spanAlt = alt
		}
	}
	if two == nil || spanAlt == nil {
		t.Fatal("no alternative of group A with two anchors or of group B")
	}
	sourceOf := func(alt *G1B34Alt) string { return alt.Witnesses[0].Source }
	compare := func(label string, mutate func(s *G1NativeSource), id string) (G1B34ExecAlt, CSharpG1B34Executed) {
		c := in
		c.Raw = g1bxSynthetic(t, a, mutate)
		z, err := g1bxDerive(a.ctx, c)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		alt := g1bxAlt(&z, id)
		if alt == nil {
			t.Fatalf("%s: no %s", label, id)
		}
		return *alt, z
	}

	// group A needs every anchor: dropping the node of one anchor token (the others stay) is a MISMATCH of the alternative, and a
	// MISMATCH is executed (a comparison ran) but is not a match
	first := two.Witnesses[0].Tokens[1]
	alt, z := compare("one anchor", func(s *G1NativeSource) {
		if s.ID == sourceOf(two) {
			for i := range s.Nodes {
				if s.Nodes[i][2] == first[0] && s.Nodes[i][3] == first[1] {
					s.Nodes[i][3] = first[1] + 1 // not the token's span any more
				}
			}
		}
	}, two.ID)
	if alt.Result != G1ResMismatch || !alt.Executed || alt.Witnesses[0].Parts[0].Result != G1ResMatch || alt.Witnesses[0].Parts[1].Result != G1ResMismatch {
		t.Errorf("one anchor missing: %s executed %v parts %+v", alt.Result, alt.Executed, alt.Witnesses[0].Parts)
	}
	if len(alt.Witnesses[0].Parts[1].Near) == 0 && len(z.Dispositions) == 0 {
		t.Errorf("a mismatch with no near node and no disposition")
	}
	// group B is the span of the node: one position off, or the right span with a type that is not sealed
	sp := spanAlt.Witnesses[0].Span
	alt, _ = compare("off by one", func(s *G1NativeSource) {
		if s.ID == sourceOf(spanAlt) {
			for i := range s.Nodes {
				if s.Nodes[i][2] == sp[0] && s.Nodes[i][3] == sp[1] && s.Types[s.Nodes[i][0]] == spanAlt.TS[0] {
					s.Nodes[i][2]--
				}
			}
		}
	}, spanAlt.ID)
	if alt.Result != G1ResMismatch {
		t.Errorf("a span off by one: %s", alt.Result)
	}
	alt, _ = compare("other type", func(s *G1NativeSource) {
		if s.ID == sourceOf(spanAlt) {
			for i := range s.Nodes {
				if s.Nodes[i][2] == sp[0] && s.Nodes[i][3] == sp[1] && s.Types[s.Nodes[i][0]] == spanAlt.TS[0] {
					s.Types = append(s.Types, "nothing_like_it")
					s.Nodes[i][0] = len(s.Types) - 1
				}
			}
		}
	}, spanAlt.ID)
	if alt.Result != G1ResMismatch || len(alt.Witnesses[0].Parts[0].Found) == 0 {
		t.Errorf("a type that is not sealed: %s %+v", alt.Result, alt.Witnesses[0].Parts[0])
	}
	// a parse that did not complete cannot be compared: NOT_APPLICABLE, not executed
	alt, _ = compare("not completed", func(s *G1NativeSource) {
		if s.ID == sourceOf(spanAlt) {
			s.Status, s.Code, s.Nodes, s.Types, s.TreeDigest, s.HasError = "RESOURCE_LIMIT", "PARSE_TIME_LIMIT", [][5]int{}, []string{}, "", false
		}
	}, spanAlt.ID)
	if alt.Result != G1ResNotApplicable || alt.Executed {
		t.Errorf("an incomplete parse: %s executed %v", alt.Result, alt.Executed)
	}
	// an undisposed mismatch is refused by the verifier
	c := in
	c.Raw = g1bxSynthetic(t, a, func(s *G1NativeSource) {
		if s.ID == sourceOf(spanAlt) {
			for i := 1; i < len(s.Nodes); i++ {
				s.Types[s.Nodes[i][0]] = "nothing_like_it"
			}
		}
	})
	z, err = g1bxDerive(a.ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if z.Counts.Mismatch == 0 || len(z.Dispositions) != z.Counts.Mismatch {
		t.Fatalf("mismatch counts %+v", z.Counts)
	}
	for _, d := range z.Dispositions {
		if d.Cause != "UNDISPOSED" {
			t.Errorf("%s: disposition %q for a synthetic mismatch", d.Alternative, d.Cause)
		}
	}
	bad, err := MarshalCSharpG1B34Executed(z)
	if err != nil {
		t.Fatal(err)
	}
	zx, _ := DecodeCSharpG1B34Executed(bad)
	if err := g1bxCheck(bad, zx, a.ctx, c); g1Code(err) != "G1_B34X_UNDISPOSED" {
		t.Errorf("undisposed mismatch: %v", err)
	}
	// the sealed expectation is not an output of the comparison
	if !bytes.Equal(a.in.Expected, mustRead(t, "../contracts/csharp-g1-expected-b03b04-r1.json")) {
		t.Fatal("expected bytes changed")
	}
}

func TestCSharpG1B34ExecutedRejectsTampering(t *testing.T) {
	a := g1bxLoad(t)
	var mismatch, spanAlt, anchors, matchWith2 string
	for _, alt := range a.x.Alternatives {
		switch {
		case alt.Result == G1ResMismatch && mismatch == "":
			mismatch = alt.ID
		case alt.Result == G1ResMatch && alt.Group == G1ClassStructure && spanAlt == "":
			spanAlt = alt.ID
		case alt.Result == G1ResMatch && alt.Group == G1ClassLiteral && len(alt.Witnesses[0].Parts) >= 2 && anchors == "":
			anchors = alt.ID
		}
		if alt.Result == G1ResMatch && matchWith2 == "" {
			for _, p := range alt.Witnesses[0].Parts {
				if len(p.Found) >= 2 && slices.Contains(p.TS, p.Found[0].Type) {
					matchWith2 = alt.ID
				}
			}
		}
	}
	if mismatch == "" || spanAlt == "" || anchors == "" || matchWith2 == "" {
		t.Fatalf("fixtures: %q %q %q %q", mismatch, spanAlt, anchors, matchWith2)
	}
	for _, tc := range []struct {
		name string
		edit func(y *CSharpG1B34Executed)
		code string
	}{
		{"a mismatch turned into a match", func(y *CSharpG1B34Executed) {
			alt := g1bxAlt(y, mismatch)
			alt.Result = G1ResMatch
			alt.Witnesses[0].Result = G1ResMatch
		}, "G1_B34X_RESULT"},
		{"a mismatching part turned into a match", func(y *CSharpG1B34Executed) {
			for i := range g1bxAlt(y, mismatch).Witnesses[0].Parts {
				g1bxAlt(y, mismatch).Witnesses[0].Parts[i].Result = G1ResMatch
			}
		}, "G1_B34X_RESULT"},
		{"a match turned into a mismatch", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Result = G1ResMismatch }, "G1_B34X_RESULT"},
		{"a match that did not execute", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Executed = false }, "G1_B34X_RESULT"},
		{"a witness result changed", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Witnesses[0].Result = G1ResNotApplicable }, "G1_B34X_RESULT"},
		{"a sealed-type node moved behind another", func(y *CSharpG1B34Executed) {
			p := &g1bxAlt(y, matchWith2).Witnesses[0].Parts[0]
			for i := range g1bxAlt(y, matchWith2).Witnesses[0].Parts {
				if q := &g1bxAlt(y, matchWith2).Witnesses[0].Parts[i]; len(q.Found) >= 2 && slices.Contains(q.TS, q.Found[0].Type) {
					p = q
				}
			}
			p.Found[0], p.Found[1] = p.Found[1], p.Found[0]
		}, "G1_B34X_EVIDENCE"},
		{"evidence of another span", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Witnesses[0].Parts[0].Found[0].End++ }, "G1_B34X_EVIDENCE"},
		{"near of a match", func(y *CSharpG1B34Executed) {
			p := &g1bxAlt(y, spanAlt).Witnesses[0].Parts[0]
			p.Near = []G1ExecFound{{Type: "x", Start: p.Start, End: p.End}}
		}, "G1_B34X_EVIDENCE"},
		{"a part of another span", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Witnesses[0].Parts[0].Start++ }, "G1_B34X_EXPECTATION"},
		{"sealed types widened", func(y *CSharpG1B34Executed) {
			p := &g1bxAlt(y, spanAlt).Witnesses[0].Parts[0]
			p.TS = append(slices.Clone(p.TS), "identifier")
		}, "G1_B34X_EXPECTATION"},
		{"anchor text changed", func(y *CSharpG1B34Executed) { g1bxAlt(y, anchors).Witnesses[0].Parts[0].Text += "x" }, "G1_B34X_EXPECTATION"},
		{"an anchor part dropped", func(y *CSharpG1B34Executed) {
			w := &g1bxAlt(y, anchors).Witnesses[0]
			w.Parts = w.Parts[:len(w.Parts)-1]
		}, "G1_B34X_EXPECTATION"},
		{"a witness match changed", func(y *CSharpG1B34Executed) { g1bxAlt(y, anchors).Witnesses[0].Match = g1b34MatchSpan }, "G1_B34X_EXPECTATION"},
		{"a witness source changed", func(y *CSharpG1B34Executed) { g1bxAlt(y, spanAlt).Witnesses[0].Source += "x" }, "G1_B34X_EXPECTATION"},
		{"a weak match claimed discriminating", func(y *CSharpG1B34Executed) {
			alt := g1bxAlt(y, spanAlt)
			alt.Discriminating = !alt.Discriminating
		}, "G1_B34X_EXPECTATION"},
		{"an alternative dropped", func(y *CSharpG1B34Executed) { y.Alternatives = y.Alternatives[1:] }, "G1_B34X_ALTERNATIVES"},
		{"a duplicate alternative row", func(y *CSharpG1B34Executed) { y.Alternatives = append(y.Alternatives, y.Alternatives[0]) }, "G1_B34X_ALTERNATIVES"},
		{"an alternative replaced by its neighbour", func(y *CSharpG1B34Executed) { y.Alternatives[1] = y.Alternatives[0] }, "G1_B34X_EXPECTATION"},
		{"the disposition dropped", func(y *CSharpG1B34Executed) { y.Dispositions = y.Dispositions[1:] }, "G1_B34X_UNDISPOSED"},
		{"the disposition cause changed", func(y *CSharpG1B34Executed) { y.Dispositions[0].Cause = G1CauseGrammar }, "G1_B34X_UNDISPOSED"},
		{"the disposition reason changed", func(y *CSharpG1B34Executed) { y.Dispositions[0].Reason += " " }, "G1_B34X_UNDISPOSED"},
		{"the disposition reason emptied", func(y *CSharpG1B34Executed) { y.Dispositions[0].Reason = "" }, "G1_B34X_UNDISPOSED"},
		{"counts changed", func(y *CSharpG1B34Executed) { y.Counts.Match++ }, "G1_B34X_COUNTS"},
		{"a source digest changed", func(y *CSharpG1B34Executed) { y.Sources[0].SHA256 = strings.Repeat("0", 64) }, "G1_B34X_SOURCES"},
		{"a source dropped", func(y *CSharpG1B34Executed) { y.Sources = y.Sources[1:] }, "G1_B34X_SOURCES"},
		{"a source not completed with a tree", func(y *CSharpG1B34Executed) { y.Sources[0].Status = "RESOURCE_LIMIT" }, "G1_B34X_SOURCES"},
		{"the expected pin changed", func(y *CSharpG1B34Executed) { y.Inputs.Expected.SHA256 = strings.Repeat("0", 64) }, "G1_B34X_PIN"},
		{"the observation pin changed", func(y *CSharpG1B34Executed) { y.Inputs.Observation.SHA256 = strings.Repeat("0", 64) }, "G1_B34X_PIN"},
		{"the basis changed", func(y *CSharpG1B34Executed) { y.Basis += " " }, "G1_B34X_BYTES"},
		{"the native limits changed", func(y *CSharpG1B34Executed) { y.Native.NativeMemory = 1 << 31 }, "G1_B34X_NATIVE"},
		{"no raw pin", func(y *CSharpG1B34Executed) { y.Inputs.Raw.SHA256 = "" }, "G1_B34X_NATIVE"},
	} {
		y := g1bxClone(t, a.x)
		tc.edit(&y)
		if err := g1bxCheckEdited(t, a, y); g1Code(err) != tc.code {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.code)
		}
	}
	// the bytes: whitespace, a wrong schema, an unknown member and a size beyond the bound
	if _, err := ParseCSharpG1B34Executed(append([]byte(" "), a.asset...), a.in); g1Code(err) == "" {
		t.Error("non-canonical bytes accepted")
	}
	if err := g1bxCheck(append([]byte(" "), a.asset...), a.x, a.ctx, a.in); g1Code(err) != "G1_B34X_BYTES" {
		t.Errorf("non-canonical bytes: %v", err)
	}
	if _, err := DecodeCSharpG1B34Executed([]byte(strings.Replace(string(a.asset), CSharpG1B34ExecutedSchema, "tsgk-csharp-g1-executed/r1", 1))); g1Code(err) != "G1_B34X_SCHEMA" {
		t.Errorf("another schema: %v", err)
	}
	if _, err := DecodeCSharpG1B34Executed([]byte(strings.Replace(string(a.asset), `"schema"`, `"note":1,"schema"`, 1))); g1Code(err) != "G1_B34X_JSON" {
		t.Errorf("unknown member: %v", err)
	}
	if _, err := DecodeCSharpG1B34Executed(append(slices.Clone(a.asset), make([]byte, csharpG1B34ExecutedMax)...)); g1Code(err) != "G1_B34X_SIZE" {
		t.Errorf("size bound: %v", err)
	}
	// a changed sealed asset or observation breaks the pins: the executed asset is bound to the exact sealed bytes
	for name, edit := range map[string]func(in *CSharpG1B34ExecutedSources){
		"expected":    func(in *CSharpG1B34ExecutedSources) { in.Expected = append(slices.Clone(in.Expected), '\n') },
		"observation": func(in *CSharpG1B34ExecutedSources) { in.Observation = append(slices.Clone(in.Observation), '\n') },
	} {
		in := a.in
		edit(&in)
		if _, err := ParseCSharpG1B34Executed(a.asset, in); g1Code(err) == "" {
			t.Errorf("a changed %s was accepted", name)
		}
	}
	in := a.in
	in.Expected = bytes.Replace(in.Expected, []byte(`"EXPECTED"`), []byte(`"EXPECTED "`), 1)
	if _, err := ParseCSharpG1B34Executed(a.asset, in); err == nil {
		t.Error("an edited sealed expected asset was accepted")
	}
}

// Evidence that is consistent with itself passes the offline check, which cannot run native; the raw observation catches it.
func TestCSharpG1B34ExecutedForgedEvidenceNeedsTheRaw(t *testing.T) {
	a := g1bxLoad(t)
	y := g1bxClone(t, a.x)
	var forged string
	for i := range y.Alternatives {
		alt := &y.Alternatives[i]
		if alt.Result != G1ResMismatch {
			continue
		}
		forged = alt.ID
		for j := range alt.Witnesses[0].Parts {
			p := &alt.Witnesses[0].Parts[j]
			if p.Result == G1ResMismatch {
				p.Found = append([]G1ExecFound{{Type: p.TS[0], Start: p.Start, End: p.End}}, p.Found...)
				if len(p.Found) > g1ExecFoundLimit {
					p.Found = p.Found[:g1ExecFoundLimit]
				}
				p.Result, p.Near = G1ResMatch, nil
			}
		}
		alt.Witnesses[0].Result, alt.Result = G1ResMatch, G1ResMatch
		break
	}
	y.Dispositions = slices.DeleteFunc(y.Dispositions, func(d G1ExecDisposition) bool { return d.Alternative == forged })
	y.Counts = g1bxCounts(y)
	data, err := MarshalCSharpG1B34Executed(y)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := DecodeCSharpG1B34Executed(data)
	if err := g1bxCheck(data, x, a.ctx, a.in); err != nil {
		t.Fatalf("self-consistent forged evidence: %v (the offline check cannot see it)", err)
	}
	in := a.in
	in.Raw = g1bxSynthetic(t, a, nil)
	if err := g1bxCheck(data, x, a.ctx, in); g1Code(err) != "G1_B34X_RAW" {
		t.Errorf("forged evidence against a raw observation: %v", err)
	}
}

// The authored dispositions are the mismatches of the tracked asset and nothing else; each names a cause of the documented set and says what
// the sealed value was and what native gave.
func TestCSharpG1B34ExecutedDispositionTable(t *testing.T) {
	a := g1bxLoad(t)
	var mismatches []string
	for _, alt := range a.x.Alternatives {
		if alt.Result == G1ResMismatch {
			mismatches = append(mismatches, alt.ID)
		}
	}
	keys := make([]string, 0, len(g1B34ExecDispositions))
	for k := range g1B34ExecDispositions {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	sorted := slices.Clone(mismatches)
	slices.Sort(sorted)
	if !slices.Equal(keys, sorted) || len(a.x.Dispositions) != len(mismatches) {
		t.Fatalf("dispositions %v, mismatches %v", keys, sorted)
	}
	for i, d := range a.x.Dispositions {
		if d.Alternative != mismatches[i] || len(d.Reason) < 200 || !slices.Contains([]string{G1CauseGrammar, G1CauseKindMap, G1CauseExpected, G1CauseDesign}, d.Cause) {
			t.Errorf("%s: cause %s, %d characters", d.Alternative, d.Cause, len(d.Reason))
		}
		if !strings.Contains(d.Reason, "sealed") {
			t.Errorf("%s: the reason does not say what was sealed", d.Alternative)
		}
	}
	// the disposition does not repair: the sealed asset still has the value native contradicts
	for _, id := range mismatches {
		alt := g1b34Alt(a.ctx.exp, id)
		if alt.Status != G1ExpExpected || len(alt.Witnesses) != 1 {
			t.Errorf("%s: sealed status %s with %d witnesses", id, alt.Status, len(alt.Witnesses))
		}
	}
}

// The two values of `match` the comparison uses are documented where the comparison is, with their meaning.
func TestCSharpG1B34ExecutedMatchValuesAreDocumented(t *testing.T) {
	doc := string(mustRead(t, "../../docs/specs/csharp-g1-expected.md"))
	_, section, ok := strings.Cut(doc, "## G1-3b native 비교 (B03·B04, PR3)")
	if !ok {
		t.Fatal("the documentation has no section for the B03/B04 native comparison")
	}
	section, _, _ = strings.Cut(section, "## G1 번들 승인 tuple")
	for _, v := range []string{g1b34MatchTokens, g1b34MatchSpan} {
		if !strings.Contains(section, "`"+v+"`") {
			t.Errorf("match value %q is not documented", v)
		}
	}
	a := g1bxLoad(t)
	seen := map[string]bool{}
	for _, alt := range a.x.Alternatives {
		for _, w := range alt.Witnesses {
			seen[w.Match] = true
		}
	}
	if len(seen) != 2 || !seen[g1b34MatchTokens] || !seen[g1b34MatchSpan] {
		t.Errorf("the asset uses the match values %v", seen)
	}
}
