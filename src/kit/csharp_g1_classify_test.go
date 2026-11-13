package kit

import (
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The G1-3b classification of csharp-B03 and csharp-B04 (#233): the counts of the real inputs, the determinism, the reason
// codes, the narrow input, the independence from the expected types and the mutants of the input and of the rule table.

type g1ClsAssets struct {
	in    G1ClassifyInput
	rep   G1ClassifyReport
	kinds []string // the Roslyn vocabulary
	err   error
}

var g1ClsOnce sync.Once
var g1ClsLoaded g1ClsAssets

func g1ClsLoad(t *testing.T) *g1ClsAssets {
	t.Helper()
	g1ClsOnce.Do(func() {
		a := &g1ClsLoaded
		l := g1Load(t)
		src := l.src
		src.Expected, src.Executed = nil, nil
		obs, err := os.ReadFile("../contracts/csharp-g1-roslyn-observed-r3.json")
		if err != nil {
			a.err = err
			return
		}
		km, err := os.ReadFile("../contracts/csharp-g1-kind-map-r2.json")
		if err != nil {
			a.err = err
			return
		}
		if a.kinds, a.err = CSharpG1RoslynVocabulary(obs); a.err != nil {
			return
		}
		if a.in, a.err = G1ClassifyView(CSharpG1ExpectedSources{Src: src, Observation: obs, KindMap: km}); a.err != nil {
			return
		}
		a.rep, a.err = ClassifyCSharpG1(a.in)
	})
	if g1ClsLoaded.err != nil {
		t.Fatal(g1ClsLoaded.err)
	}
	return &g1ClsLoaded
}

// cloneInput is a deep enough copy for a mutant: the slices a mutant changes are copied.
func g1ClsClone(in G1ClassifyInput) G1ClassifyInput {
	out := G1ClassifyInput{Alts: slices.Clone(in.Alts), Kinds: slices.Clone(in.Kinds)}
	for _, s := range in.Sources {
		s.Tokens, s.Nodes = slices.Clone(s.Tokens), slices.Clone(s.Nodes)
		out.Sources = append(out.Sources, s)
	}
	return out
}

func g1ClsResult(rep G1ClassifyReport, id string) G1ClassifyResult {
	for _, r := range rep.Results {
		if r.ID == id {
			return r
		}
	}
	return G1ClassifyResult{}
}

// The counts of the dry run on the pinned inputs. They replace the estimates of the design (A 175-190, B 95-125, C 80-110).
func TestCSharpG1ClassifyCountsOfThePinnedInputs(t *testing.T) {
	a := g1ClsLoad(t)
	c := a.rep.Counts
	if c.Alternatives != 388 || c.A+c.B+c.C != 388 || len(a.rep.Results) != 388 {
		t.Fatalf("the groups do not sum to the 388 alternatives of csharp-B03 and csharp-B04: %+v", c)
	}
	if c.A != 198 || c.ADiscr != 131 || c.AWeak != 67 || c.B != 100 || c.BDiscr != 74 || c.BWeak != 26 || c.C != 90 {
		t.Errorf("counts moved: %+v", c)
	}
	wantRows := []G1ClassifyCount{{"csharp-B03 A", 137}, {"csharp-B03 B", 61}, {"csharp-B03 C", 59}, {"csharp-B04 A", 61}, {"csharp-B04 B", 39}, {"csharp-B04 C", 31}}
	if !slices.Equal(c.Rows, wantRows) {
		t.Errorf("rows %v, want %v", c.Rows, wantRows)
	}
	wantBlocked := []G1ClassifyCount{{"ABSTRACT_CATEGORY", 36}, {"NO_OWN_NODE_FOR_CONSTRUCT", 29}, {"NO_WITNESS_IN_LINKED_SOURCES", 2}, {"POSITION_DEPENDENT", 14}, {"SEMANTIC_BINDING", 9}}
	if !slices.Equal(c.Blocked, wantBlocked) {
		t.Errorf("blocked %v, want %v", c.Blocked, wantBlocked)
	}
	// the alternatives are those of the two rows, once each, in the order of the declarations
	var rows [2]int
	seen := map[string]bool{}
	for _, r := range a.rep.Results {
		if seen[r.ID] {
			t.Errorf("%s classified twice", r.ID)
		}
		seen[r.ID] = true
		switch r.Row {
		case "csharp-B03":
			rows[0]++
		case "csharp-B04":
			rows[1]++
		default:
			t.Errorf("%s: row %s", r.ID, r.Row)
		}
	}
	if rows != [2]int{257, 131} {
		t.Errorf("alternatives per row %v, want [257 131]", rows)
	}
}

func TestCSharpG1ClassifyIsDeterministic(t *testing.T) {
	a := g1ClsLoad(t)
	again, err := ClassifyCSharpG1(g1ClsClone(a.in))
	if err != nil || !g1eSame(again, a.rep) {
		t.Fatalf("a second classification differs: %v", err)
	}
	// the order of the kind table and of the alternatives changes nothing but the order of the results
	shuffled := g1ClsClone(a.in)
	slices.Reverse(shuffled.Kinds)
	slices.Reverse(shuffled.Alts)
	got, err := ClassifyCSharpG1(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.A != a.rep.Counts.A || got.Counts.B != a.rep.Counts.B || got.Counts.C != a.rep.Counts.C {
		t.Errorf("a reordered input changed the counts: %+v", got.Counts)
	}
	for _, r := range a.rep.Results {
		if !g1eSame(g1ClsResult(got, r.ID), r) {
			t.Errorf("%s: a reordered input changed the result", r.ID)
		}
	}
}

// Every blocked row has a registered reason and a text, every A has anchors, a witness and the spec basis, every B has kinds, a
// witness and the differential basis, and the codes and bases are the ones of the expectation.
func TestCSharpG1ClassifyRowsAreWellFormed(t *testing.T) {
	a := g1ClsLoad(t)
	for _, r := range a.rep.Results {
		switch r.Group {
		case G1ClassBlocked:
			if !slices.Contains(G1ClassBlockCodes, r.Block) || r.Reason == "" || r.Basis != "" || r.Anchors != nil || r.Kinds != nil || r.Witness != "" || r.Discriminating {
				t.Errorf("%s: malformed blocked row %+v", r.ID, r)
			}
		case G1ClassLiteral:
			if len(r.Anchors) == 0 || r.Witness == "" || r.Basis != G1BasisSpec || r.Block != "" || r.Kinds != nil || r.Rule != "anchors" {
				t.Errorf("%s: malformed literal row %+v", r.ID, r)
			}
		case G1ClassStructure:
			if len(r.Kinds) == 0 || r.Witness == "" || r.Basis != G1BasisDiff || r.Block != "" || r.Anchors != nil || r.Rule == "" {
				t.Errorf("%s: malformed structure row %+v", r.ID, r)
			}
		default:
			t.Errorf("%s: group %q", r.ID, r.Group)
		}
	}
	// the codes of the classification are the ones the expectation of G1-3a already uses, plus the two this one adds
	for got, want := range map[string]string{G1ClassBlockCategory: g1BlockUnion, G1ClassBlockPosition: g1BlockPosition, G1ClassBlockSemantic: g1BlockSemantic, G1ClassBasisSpec: G1BasisSpec, G1ClassBasisDiff: G1BasisDiff} {
		if got != want {
			t.Errorf("code %q differs from the expectation's %q", got, want)
		}
	}
	if G1ClassBlockNoNode != "NO_OWN_NODE_FOR_CONSTRUCT" || G1ClassBlockNoWitness != "NO_WITNESS_IN_LINKED_SOURCES" || len(G1ClassBlockCodes) != 5 {
		t.Errorf("the registered codes are %v", G1ClassBlockCodes)
	}
}

// A rule that no alternative uses, or one that names a kind the observation does not have, is dead weight that a change of the
// alternatives would leave behind unnoticed.
func TestCSharpG1ClassifyRulesAreAllUsedAndKnown(t *testing.T) {
	a := g1ClsLoad(t)
	used := map[string]bool{}
	for _, r := range a.rep.Results {
		used[r.Rule] = true
	}
	for key, rule := range g1clsRules {
		if !used[key] {
			t.Errorf("rule %q is used by no alternative", key)
		}
		if (rule.block != "") == (len(rule.kinds) > 0) || rule.block != "" && (rule.why == "" || !slices.Contains(G1ClassBlockCodes, rule.block)) {
			t.Errorf("rule %q is neither a block with a text nor a list of kinds: %+v", key, rule)
		}
		for _, k := range append(slices.Clone(rule.kinds), rule.parent...) {
			if !slices.Contains(a.kinds, k) {
				t.Errorf("rule %q names %q, which the observation does not have", key, k)
			}
		}
	}
}

// What the classification is given is a narrow value: no member for an expected value, a node type or the bytes of an asset.
func TestCSharpG1ClassifyInputIsNarrow(t *testing.T) {
	want := []string{
		"G1ClassifyAlt.Cases []string", "G1ClassifyAlt.ID string", "G1ClassifyAlt.Literals []string", "G1ClassifyAlt.Presence string",
		"G1ClassifyAlt.Production string", "G1ClassifyAlt.Row string",
		"G1ClassifyInput.Alts []kit.G1ClassifyAlt", "G1ClassifyInput.Kinds []kit.G1ClassifyKind", "G1ClassifyInput.Sources []kit.G1ClassifySource",
		"G1ClassifyKind.Class string", "G1ClassifyKind.Map string", "G1ClassifyKind.Name string", "G1ClassifyKind.Reason string", "G1ClassifyKind.Shares []string",
		"G1ClassifyNode.Kind string", "G1ClassifyNode.Parent string",
		"G1ClassifySource.Accepted bool", "G1ClassifySource.Case string", "G1ClassifySource.ID string", "G1ClassifySource.Nodes []kit.G1ClassifyNode", "G1ClassifySource.Tokens []string",
	}
	slices.Sort(want)
	if got := g1TypeMembers(G1ClassifyInput{}); !slices.Equal(got, want) {
		t.Errorf("the members of the classification's input are %q, want %q", got, want)
	}
}

// The secondary, syntactic check of the classification files: no selector or key of the field TS, no string "ts", "kind" or "kinds",
// and no name of the expected asset, its alternatives or the correction records. The structural proof is the closure of the audits'
// test file (the entry "classification") and TestCSharpG1ClassifyInputIsNarrow.
func TestCSharpG1ClassifyNeverNamesTheExpectedTypes(t *testing.T) {
	for _, f := range []string{"csharp_g1_classify.go", "csharp_g1_classify_rules.go"} {
		if named, err := g1a76NamesTS(string(mustRead(t, f))); err != nil || named {
			t.Errorf("%s names the expected types (%v, %v)", f, named, err)
		}
	}
	for name, snippet := range map[string]string{
		"selector": "func f(a G1ExpAlt) int { return len(a.TS) }",
		"literal":  "func f() any { return G1ClassifyKind{TS: nil} }",
		"type":     "func f(a CSharpG1Expected) int { return len(a.Alternatives) }",
		"decoder":  "func f(b []byte) error { _, err := DecodeCSharpG1Expected(b); return err }",
	} {
		if named, err := g1a76NamesTS("package kit\n" + snippet); err != nil || !named {
			t.Errorf("%s: a read of the expected types was not found (%v, %v)", name, named, err)
		}
	}
}

// ---- mutants of the input ---------------------------------------------------------------------

func g1ClsRun(t *testing.T, in G1ClassifyInput) G1ClassifyReport {
	t.Helper()
	rep, err := ClassifyCSharpG1(in)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestCSharpG1ClassifyKillsTheMutantsOfTheInput(t *testing.T) {
	a := g1ClsLoad(t)
	altCases := func(in G1ClassifyInput, id string) []string {
		for _, x := range in.Alts {
			if x.ID == id {
				return x.Cases
			}
		}
		return nil
	}
	// a source that Roslyn rejects is no witness: the alternative whose only witnesses are in it falls to NO_WITNESS_IN_LINKED_SOURCES
	t.Run("rejected source", func(t *testing.T) {
		r := g1ClsResult(a.rep, "csharp-B03.a024")
		if r.Group != G1ClassStructure {
			t.Fatalf("%+v", r)
		}
		in := g1ClsClone(a.in)
		for i := range in.Sources {
			if slices.Contains(altCases(in, r.ID), in.Sources[i].Case) {
				in.Sources[i].Accepted = false
			}
		}
		if got := g1ClsResult(g1ClsRun(t, in), r.ID); got.Group != G1ClassBlocked || got.Block != G1ClassBlockNoWitness {
			t.Errorf("%s with no accepted source: %+v", r.ID, got)
		}
	})
	// the anchors are looked up as tokens, in order: without the token, or in the wrong order, the anchor alternative is blocked
	t.Run("anchor token removed", func(t *testing.T) {
		r := g1ClsResult(a.rep, "csharp-B03.a121") // 'unchecked' '(' ')'
		if r.Group != G1ClassLiteral {
			t.Fatalf("%+v", r)
		}
		in := g1ClsClone(a.in)
		for i := range in.Sources {
			in.Sources[i].Tokens = slices.DeleteFunc(in.Sources[i].Tokens, func(s string) bool { return s == "unchecked" })
		}
		if got := g1ClsResult(g1ClsRun(t, in), r.ID); got.Group != G1ClassBlocked || got.Block != G1ClassBlockNoWitness {
			t.Errorf("%s without its keyword token: %+v", r.ID, got)
		}
	})
	t.Run("anchor order reversed", func(t *testing.T) {
		// the sources of the linked case hold exactly the three tokens: in order they are the anchors, reversed they are not
		for tokens, want := range map[string]string{"unchecked ( )": G1ClassLiteral, ") ( unchecked": G1ClassBlocked} {
			in := g1ClsClone(a.in)
			for i := range in.Sources {
				in.Sources[i].Tokens = strings.Fields(tokens)
			}
			if got := g1ClsResult(g1ClsRun(t, in), "csharp-B03.a121"); got.Group != want {
				t.Errorf("tokens %q: %+v, want group %s", tokens, got, want)
			}
		}
	})
	// the kind map says a kind has no node: the structure alternative is blocked with the node-less code, not turned into a literal
	t.Run("kind map without a node", func(t *testing.T) {
		in := g1ClsClone(a.in)
		for i := range in.Kinds {
			if in.Kinds[i].Name == "TypeOfExpression" {
				in.Kinds[i].Map, in.Kinds[i].Reason = "none", "HIDDEN_NODE"
			}
		}
		if got := g1ClsResult(g1ClsRun(t, in), "csharp-B03.a035"); got.Group != G1ClassBlocked || got.Block != G1ClassBlockNoNode || !strings.Contains(got.Reason, "HIDDEN_NODE") {
			t.Errorf("a kind without a node: %+v", got)
		}
	})
	// a node type shared with a kind outside the construct makes the match weak; a kind map entry that is a candidate set does too
	t.Run("shared node type and candidate set", func(t *testing.T) {
		in := g1ClsClone(a.in)
		for i := range in.Kinds {
			switch in.Kinds[i].Name {
			case "TypeOfExpression":
				in.Kinds[i].Shares = []string{"SizeOfExpression"}
			case "CastExpression":
				in.Kinds[i].Map = "1:n"
			}
		}
		rep := g1ClsRun(t, in)
		for _, id := range []string{"csharp-B03.a035", "csharp-B03.a141"} {
			if got := g1ClsResult(rep, id); got.Group != G1ClassStructure || got.Discriminating {
				t.Errorf("%s: %+v, want a weak structure", id, got)
			}
		}
		if !g1ClsResult(a.rep, "csharp-B03.a035").Discriminating || !g1ClsResult(a.rep, "csharp-B03.a141").Discriminating {
			t.Error("the unmutated results are not discriminating: the mutant proves nothing")
		}
	})
	// the parent kind of a rule places the construct in its slot: with another parent the witness is gone
	t.Run("parent kind", func(t *testing.T) {
		r := g1ClsResult(a.rep, "csharp-B04.a059")
		if !slices.Equal(r.Parent, []string{"ExpressionStatement"}) {
			t.Fatalf("%+v", r)
		}
		in := g1ClsClone(a.in)
		for i := range in.Sources {
			for j := range in.Sources[i].Nodes {
				if in.Sources[i].Nodes[j].Parent == "ExpressionStatement" {
					in.Sources[i].Nodes[j].Parent = "Block"
				}
			}
		}
		if got := g1ClsResult(g1ClsRun(t, in), r.ID); got.Group != G1ClassBlocked || got.Block != G1ClassBlockNoWitness {
			t.Errorf("%s with another parent: %+v", r.ID, got)
		}
	})
	// an alternative of another case is no witness: the links decide
	t.Run("links", func(t *testing.T) {
		in := g1ClsClone(a.in)
		for i := range in.Alts {
			if in.Alts[i].ID == "csharp-B03.a121" {
				in.Alts[i].Cases = nil
			}
		}
		if got := g1ClsResult(g1ClsRun(t, in), "csharp-B03.a121"); got.Group != G1ClassBlocked || got.Block != G1ClassBlockNoWitness {
			t.Errorf("an alternative without a linked case: %+v", got)
		}
	})
	// the ledger's literals are the anchors: without them the alternative leaves group A (it has a nonterminal rule or none)
	t.Run("literals removed", func(t *testing.T) {
		in := g1ClsClone(a.in)
		for i := range in.Alts {
			if in.Alts[i].ID == "csharp-B03.a121" {
				in.Alts[i].Literals = nil
			}
		}
		if _, err := ClassifyCSharpG1(in); err == nil || !strings.Contains(err.Error(), "G1_CLASSIFY_RULE:csharp-B03.a121") {
			t.Errorf("a literal alternative without literals needs a rule: %v", err)
		}
	})
	// ill-formed inputs fail, they are not classified by default
	t.Run("ill-formed", func(t *testing.T) {
		dupKind := g1ClsClone(a.in)
		dupKind.Kinds = append(dupKind.Kinds, dupKind.Kinds[0])
		dupAlt := g1ClsClone(a.in)
		dupAlt.Alts = append(dupAlt.Alts, dupAlt.Alts[0])
		badProd := g1ClsClone(a.in)
		badProd.Alts[0].Production = "no colon here"
		for name, in := range map[string]G1ClassifyInput{"duplicate kind": dupKind, "duplicate alternative": dupAlt, "production without a colon": badProd} {
			if _, err := ClassifyCSharpG1(in); err == nil || !strings.Contains(err.Error(), "G1_CLASSIFY_") {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

// ---- mutants of the rule table ----------------------------------------------------------------

func g1ClsWithRule(t *testing.T, key string, rule *g1clsRule, in G1ClassifyInput) (G1ClassifyReport, error) {
	t.Helper()
	old, had := g1clsRules[key]
	t.Cleanup(func() {
		if had {
			g1clsRules[key] = old
		} else {
			delete(g1clsRules, key)
		}
	})
	if rule == nil {
		delete(g1clsRules, key)
	} else {
		g1clsRules[key] = *rule
	}
	return ClassifyCSharpG1(in)
}

func TestCSharpG1ClassifyKillsTheMutantsOfTheRules(t *testing.T) {
	a := g1ClsLoad(t)
	// an alternative without a rule is an error naming it, never a default group
	t.Run("rule removed", func(t *testing.T) {
		if _, err := g1ClsWithRule(t, ">invocation_expression", nil, a.in); err == nil || !strings.Contains(err.Error(), "G1_CLASSIFY_RULE:") || !strings.Contains(err.Error(), "csharp-B03.a024") {
			t.Errorf("a removed rule: %v", err)
		}
	})
	// a rule that names a kind the kind map lacks, or a token kind, fails
	t.Run("unknown kind", func(t *testing.T) {
		if _, err := g1ClsWithRule(t, ">invocation_expression", &g1clsRule{kinds: []string{"NoSuchKind"}}, a.in); err == nil || !strings.Contains(err.Error(), "G1_CLASSIFY_KIND:") {
			t.Errorf("an unknown kind: %v", err)
		}
	})
	t.Run("token kind", func(t *testing.T) {
		if _, err := g1ClsWithRule(t, ">invocation_expression", &g1clsRule{kinds: []string{"OpenParenToken"}}, a.in); err == nil || !strings.Contains(err.Error(), "G1_CLASSIFY_KIND:") {
			t.Errorf("a token kind in a structure rule: %v", err)
		}
	})
	// the lookup order: a pair rule beats the head rule, which beats the target rule
	t.Run("lookup order", func(t *testing.T) {
		rep, err := g1ClsWithRule(t, "statement_expression>invocation_expression", &g1clsRule{block: G1ClassBlockSemantic, why: "mutant"}, a.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := g1ClsResult(rep, "csharp-B04.a059"); got.Group != G1ClassBlocked || got.Block != G1ClassBlockSemantic || got.Rule != "statement_expression>invocation_expression" {
			t.Errorf("the pair rule did not decide: %+v", got)
		}
		// the same alternative under the target rule alone: a structure without the parent kind
		rep, err = g1ClsWithRule(t, "statement_expression>invocation_expression", nil, a.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := g1ClsResult(rep, "csharp-B04.a059"); got.Rule != ">invocation_expression" || len(got.Parent) != 0 {
			t.Errorf("the target rule did not decide: %+v", got)
		}
	})
}

// The production text is parsed the way the rules are keyed.
func TestCSharpG1ClassifySplitsProductions(t *testing.T) {
	for _, c := range []struct {
		prod, head, rhs string
		idx             int
	}{
		{"primary_expression: tuple_literal  [alt 5/30]", "primary_expression", "tuple_literal", 5},
		{"fragment x: 'a' b", "x", "'a' b", 1},
		{"pattern: logical_pattern", "pattern", "logical_pattern", 1},
	} {
		head, rhs, idx, ok := g1clsSplit(c.prod)
		if !ok || head != c.head || rhs != c.rhs || idx != c.idx {
			t.Errorf("%q: %q %q %d %v", c.prod, head, rhs, idx, ok)
		}
	}
	if got := g1clsNonterminals("'new' type '(' argument_list? ')' object_or_collection_initializer?"); !slices.Equal(got, []string{"type", "argument_list", "object_or_collection_initializer"}) {
		t.Errorf("nonterminals %v", got)
	}
	if g1clsAnchorDiscriminates([]string{"("}) || !g1clsAnchorDiscriminates([]string{"=>"}) || !g1clsAnchorDiscriminates([]string{"new", "("}) {
		t.Error("an anchor of one punctuation character is weak, a word or two characters is not")
	}
}
