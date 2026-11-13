package kit

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// G1 expected values of the lexical and type-name rows (#233, G1-3a): csharp-B01 (407 alternatives)
// and csharp-B02 (98).
//
// Each alternative gets either an expectation or a BLOCKED disposition with a reason code. An
// expectation is a set of witnesses: spans of the pinned Roslyn observation that exhibit the
// alternative, plus the tree-sitter node types the pinned grammar allows for them. Two bases exist:
//
//   - spec-derived: the alternative's lexeme or lexical form comes from ECMA-334 (csharp_g1_spec.go);
//     the observation only locates it. Every observed token is also checked against the same
//     derivation, and each disagreement is a disposition, never an adopted value.
//   - differential-oracle: the expectation is the structure Roslyn observed (node kinds, directive
//     trivia), translated by the kind map.
//
// The derivation reads the ledger, the Roslyn observation, the kind map and the case sources. It
// reads no native output (see TestCSharpG1ExpectedSeparation).

const (
	CSharpG1ExpectedSchema = "tsgk-csharp-g1-expected/r1"
	CSharpG1ExpectedPath   = "src/contracts/csharp-g1-expected-b01b02-r1.json"
	// shortcut: the asset is far below this today; raise only with a new revision.
	csharpG1ExpectedMax = 4 << 20

	G1BasisSpec = "spec-derived"
	G1BasisDiff = "differential-oracle"

	G1ExpExpected = "EXPECTED"
	G1ExpBlocked  = "BLOCKED"

	g1ExpWitnessLimit = 3
	g1ExpRowB01       = "csharp-B01"
	g1ExpRowB02       = "csharp-B02"
)

type G1ExpInputs struct {
	Ledger      CSharpG1Inputs        `json:"ledger_sources"`
	Observation queryregistrycore.Pin `json:"observation"`
	KindMap     queryregistrycore.Pin `json:"kind_map"`
}

type G1ExpWitness struct {
	Source string `json:"source"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Roslyn string `json:"roslyn,omitempty"`
	Match  string `json:"match"`
}

type G1ExpAlt struct {
	ID        string         `json:"id"`
	Row       string         `json:"row"`
	Status    string         `json:"status"`
	Basis     string         `json:"basis,omitempty"`
	Form      string         `json:"form,omitempty"`
	Spec      string         `json:"spec"`
	TS        []string       `json:"ts"`
	Witnesses []G1ExpWitness `json:"witnesses"`
	Block     string         `json:"block,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

// G1ExpMismatch is one class of disagreement between the spec derivation and the Roslyn observation.
type G1ExpMismatch struct {
	Code     string   `json:"code"`
	Roslyn   string   `json:"roslyn"`
	Spec     string   `json:"spec"`
	Count    int      `json:"count"`
	Examples []string `json:"examples"`
	// the disposition is authored in csharpG1Dispositions, keyed by Code
	Disposition string `json:"disposition"`
	Citation    string `json:"citation"`
	Decision    string `json:"decision"`
}

type G1ExpCounts struct {
	Alternatives   int `json:"alternatives"`
	Expected       int `json:"expected"`
	Blocked        int `json:"blocked"`
	SpecDerived    int `json:"spec_derived"`
	Differential   int `json:"differential_oracle"`
	Witnesses      int `json:"witnesses"`
	TokensChecked  int `json:"tokens_checked"`
	TokensMismatch int `json:"tokens_mismatch"`
	SourcesChecked int `json:"sources_checked"`
}

type G1ExpCross struct {
	Sources    int             `json:"sources"`
	Tokens     int             `json:"tokens"`
	Directives int             `json:"directives"`
	Mismatches []G1ExpMismatch `json:"mismatches"`
}

// G1ExpSymbols is a registered step the default mode leaves open: the ledger's syntax is ERROR while
// Roslyn accepts the source (the error is in a conditionally excluded section).
type G1ExpSymbols struct {
	Source      string   `json:"source"`
	Rows        []string `json:"rows"` // "<row>:<kind>" obligations of the two rows the step's case is linked to
	Disposition string   `json:"disposition"`
}

type CSharpG1Expected struct {
	Schema         string           `json:"schema"`
	Inputs         G1ExpInputs      `json:"inputs"`
	Basis          string           `json:"basis"`
	WitnessLimit   int              `json:"witness_limit"`
	Counts         G1ExpCounts      `json:"counts"`
	Alternatives   []G1ExpAlt       `json:"alternatives"`
	Cross          G1ExpCross       `json:"cross_check"`
	SymbolsContext []G1ExpSymbols   `json:"symbols_context"`
	Audit          G1ExpAudit       `json:"audit"`
	Metamorphic    G1ExpMetamorphic `json:"metamorphic"`
}

const csharpG1ExpectedBasis = "Expected values for the lexical row csharp-B01 and the type-name row csharp-B02, derived without any native output. An alternative is EXPECTED with witnesses (spans of the pinned Roslyn observation, accepted sources linked to the alternative by the ledger, at most one per case and three in all) and the tree-sitter node types the pinned grammar allows, or BLOCKED with a reason code. basis spec-derived: the lexeme or lexical form comes from the ECMA-334 lexical grammar as published in the C# standard draft v13 on learn.microsoft.com (clauses in the spec field) and Roslyn only locates it; basis differential-oracle: the structure is what the pinned Roslyn DLLs observed, translated by the kind map. match span: a node of one of the types spans exactly [start,end); starts-at: a node of one of the types starts at start. The cross_check section compares every observed token and directive with the spec derivation; its mismatches are dispositions, not adopted values. The sections audit and metamorphic record the specification sample audit and the metamorphic check. The single-oracle limit applies: a differential-oracle expectation is the behavior of one Roslyn build, not the specification."

func g1eFault(code string) error { return fail(KindInvalidInput, code, CSharpG1ExpectedPath, nil) }

// ---- index of the observation -------------------------------------------------------------

type g1Tok struct {
	kind       string
	start, end int
	text       string
}

type g1Node struct {
	kind       string
	start, end int
	parent     int
	direct     []int // indexes of the tokens whose innermost node is this one
}

type g1Src struct {
	id, caseID string
	step       int
	data       []byte
	accepted   bool
	toks       []g1Tok
	nodes      []g1Node
	dirs       []G1RoslynDirective
	disabled   []G1RoslynRange
}

type g1Ctx struct {
	obs      CSharpG1RoslynObserved
	km       CSharpG1KindMap
	kmIdx    map[string]G1KMKind
	order    []*g1Src
	byID     map[string]*g1Src
	byCase   map[string][]*g1Src
	kindByTx map[string]string // fixed lexeme -> Roslyn token kind (kind map)
	keywords map[string]bool   // reserved keyword lexemes (spec productions)
	puncts   map[string]bool   // operator and punctuator lexemes (spec productions)
}

func g1BuildCtx(obs CSharpG1RoslynObserved, km CSharpG1KindMap, sources []G1RoslynCaseSource) (*g1Ctx, error) {
	if len(obs.Sources) != len(sources) {
		return nil, g1eFault("G1_EXPECTED_SOURCE_SET")
	}
	c := &g1Ctx{obs: obs, km: km, kmIdx: km.Index(), byID: map[string]*g1Src{}, byCase: map[string][]*g1Src{}, kindByTx: map[string]string{}}
	for _, k := range km.Kinds {
		if k.Text != "" {
			if prev, dup := c.kindByTx[k.Text]; dup {
				return nil, fmt.Errorf("kind map: lexeme %q of %s and %s", k.Text, prev, k.Roslyn)
			}
			c.kindByTx[k.Text] = k.Roslyn
		}
	}
	for i, o := range obs.Sources {
		cs := sources[i]
		if o.ID != cs.ID {
			return nil, g1eFault("G1_EXPECTED_SOURCE_SET")
		}
		s := &g1Src{id: o.ID, caseID: cs.Case, step: cs.Step, data: cs.Data, accepted: o.Accepted != nil && *o.Accepted, dirs: o.Directives, disabled: o.Disabled}
		missing := map[int]bool{}
		for _, m := range o.Missing {
			missing[m] = true
		}
		for ti, t := range o.Tokens {
			if t[0] < 0 || t[0] >= len(obs.Kinds) || t[1] < 0 || t[2] < t[1] || t[2] > len(s.data) {
				return nil, g1eFault("G1_EXPECTED_OBSERVATION")
			}
			if missing[ti] {
				continue // a missing token has no source text and is not a lexeme
			}
			s.toks = append(s.toks, g1Tok{kind: obs.Kinds[t[0]], start: t[1], end: t[2], text: string(s.data[t[1]:t[2]])})
		}
		var stack []int
		for _, n := range o.Nodes {
			if n[0] < 0 || n[0] >= len(obs.Kinds) {
				return nil, g1eFault("G1_EXPECTED_OBSERVATION")
			}
			for len(stack) > 0 && !(s.nodes[stack[len(stack)-1]].start <= n[1] && n[2] <= s.nodes[stack[len(stack)-1]].end) {
				stack = stack[:len(stack)-1]
			}
			p := -1
			if len(stack) > 0 {
				p = stack[len(stack)-1]
			}
			s.nodes = append(s.nodes, g1Node{kind: obs.Kinds[n[0]], start: n[1], end: n[2], parent: p})
			stack = append(stack, len(s.nodes)-1)
		}
		for ti, t := range s.toks {
			if t.end == t.start {
				continue
			}
			for ni := len(s.nodes) - 1; ni >= 0; ni-- {
				if s.nodes[ni].start <= t.start && t.end <= s.nodes[ni].end {
					s.nodes[ni].direct = append(s.nodes[ni].direct, ti)
					break
				}
			}
		}
		c.order = append(c.order, s)
		c.byID[s.id] = s
		c.byCase[s.caseID] = append(c.byCase[s.caseID], s)
	}
	return c, nil
}

func (s *g1Src) text(start, end int) string { return string(s.data[start:end]) }

func (s *g1Src) nodeText(n g1Node) string { return s.text(n.start, n.end) }

// directLine is the text of a directive's line without its line terminator.
func (s *g1Src) directive(d G1RoslynDirective) (line string, end int) {
	end = d.LineEnd
	for end > d.LineStart && (s.data[end-1] == '\n' || s.data[end-1] == '\r') {
		end--
	}
	return s.text(d.LineStart, end), end
}

// ---- hits ---------------------------------------------------------------------------------

type g1Hit struct {
	start, end int
	roslyn     string
	match      string
}

type g1Finder func(c *g1Ctx, s *g1Src) []g1Hit

type g1Rule struct {
	basis   string
	form    string
	ts      []string // node types; for differential rules also derived from kinds
	kinds   []string // Roslyn kinds whose kind-map types are added to ts
	find    g1Finder
	inherit string // head whose alternatives' finders this alternative unions
	block   string
	reason  string
}

// ---- derivation ---------------------------------------------------------------------------

type G1ExpAltSrc struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Production string `json:"production"`
	Ref        string `json:"ref"`
}

type g1ExpAltFile struct {
	Routes map[string]map[string]struct {
		Alternatives []G1ExpAltSrc `json:"alternatives"`
	} `json:"routes"`
}

// DeriveCSharpG1Expected derives the expected asset from the pinned inputs. The first derivation,
// before the metamorphic Roslyn run, passes no sealed results.
// g1ExpContext derives the ledger (no status set) and builds the observation index.
func g1ExpContext(in CSharpG1ExpectedSources) (G1ExpInputs, *g1Ctx, CSharpG1Ledger, error) {
	var inputs G1ExpInputs
	// the expectation is derived from the ledger before any status is sealed: no expected asset here
	src := in.Src
	src.Expected, src.Executed = nil, nil
	ledger, _, err := deriveCSharpG1(src)
	if err != nil {
		return inputs, nil, ledger, err
	}
	sources, err := CSharpG1RoslynSources(src)
	if err != nil {
		return inputs, nil, ledger, err
	}
	var obs CSharpG1RoslynObserved
	if err := jsonv2.Unmarshal(in.Observation, &obs, jsonv2.RejectUnknownMembers(true)); err != nil {
		return inputs, nil, ledger, fail(KindInvalidInput, "G1_EXPECTED_OBSERVATION", CSharpG1RoslynPath, err)
	}
	km, err := ParseCSharpG1KindMap(in.KindMap, obs.Kinds)
	if err != nil {
		return inputs, nil, ledger, err
	}
	c, err := g1BuildCtx(obs, km, sources)
	if err != nil {
		return inputs, nil, ledger, err
	}
	inputs = G1ExpInputs{
		Ledger:      ledger.Inputs,
		Observation: queryregistrycore.FilePin(CSharpG1RoslynPath, in.Observation),
		KindMap:     queryregistrycore.FilePin(CSharpG1KindMapPath, in.KindMap),
	}
	return inputs, c, ledger, nil
}

// g1ExpRuleSrcs lists the alternatives of the two rows in ledger order.
func g1ExpRuleSrcs(in CSharpG1ExpectedSources) ([]g1RuleSrc, error) {
	var alts g1ExpAltFile
	if err := json.Unmarshal(in.Src.Alternatives, &alts); err != nil {
		return nil, g1eFault("G1_EXPECTED_ALTERNATIVES")
	}
	var out []g1RuleSrc
	for _, row := range []string{g1ExpRowB01, g1ExpRowB02} {
		for _, a := range alts.Routes[g1Route][row].Alternatives {
			out = append(out, g1RuleSrc{ID: a.ID, Row: row, Production: a.Production, Kind: a.Kind, Ref: a.Ref})
		}
	}
	return out, nil
}

// DeriveCSharpG1Expected derives the expected asset from the pinned inputs. The first derivation,
// before the metamorphic Roslyn run, passes no sealed results.
func DeriveCSharpG1Expected(in CSharpG1ExpectedSources) (CSharpG1Expected, error) {
	var out CSharpG1Expected
	inputs, c, ledger, err := g1ExpContext(in)
	if err != nil {
		return out, err
	}
	recs, err := g1ExpRuleSrcs(in)
	if err != nil {
		return out, err
	}
	out.Schema, out.Basis, out.WitnessLimit, out.Inputs = CSharpG1ExpectedSchema, csharpG1ExpectedBasis, g1ExpWitnessLimit, inputs
	// links of the ledger
	links := map[string][]G1Link{}
	for _, a := range ledger.Alternatives {
		links[a.ID] = a.Links
	}
	rules, err := g1BuildRules(c, recs)
	if err != nil {
		return out, err
	}
	for i, a := range recs {
		rule := rules[i]
		alt := G1ExpAlt{ID: a.ID, Row: a.Row, Spec: g1SpecLabel(a.Ref), TS: []string{}, Witnesses: []G1ExpWitness{}}
		if rule.block != "" {
			alt.Status, alt.Block, alt.Reason = G1ExpBlocked, rule.block, rule.reason
			out.Alternatives = append(out.Alternatives, alt)
			continue
		}
		ts := slices.Clone(rule.ts)
		for _, k := range rule.kinds {
			ts = append(ts, c.kmIdx[k].TS...)
		}
		slices.Sort(ts)
		ts = slices.Compact(ts)
		if len(ts) == 0 {
			alt.Status, alt.Block, alt.Reason = G1ExpBlocked, "NOT_IN_PINNED_GRAMMAR", "the pinned grammar has no node type for the construct"
			out.Alternatives = append(out.Alternatives, alt)
			continue
		}
		// witnesses: accepted sources of linked cases, first hit of a case, at most the limit
		seen := map[string]bool{}
		for _, l := range links[a.ID] {
			if len(alt.Witnesses) >= g1ExpWitnessLimit || seen[l.Case] {
				continue
			}
			for _, s := range c.byCase[l.Case] {
				if !s.accepted {
					continue
				}
				if hits := rule.find(c, s); len(hits) > 0 {
					h := hits[0]
					alt.Witnesses = append(alt.Witnesses, G1ExpWitness{Source: s.id, Start: h.start, End: h.end, Roslyn: h.roslyn, Match: h.match})
					seen[l.Case] = true
					break
				}
			}
		}
		if len(alt.Witnesses) == 0 {
			alt.Status, alt.Block, alt.Reason = G1ExpBlocked, "NO_WITNESS_IN_LINKED_SOURCES", "no accepted source of a linked case exhibits the alternative"
			out.Alternatives = append(out.Alternatives, alt)
			continue
		}
		alt.Status, alt.Basis, alt.Form, alt.TS = G1ExpExpected, rule.basis, rule.form, ts
		out.Alternatives = append(out.Alternatives, alt)
	}
	cross, err := g1CrossCheck(c)
	if err != nil {
		return out, err
	}
	out.Cross = cross
	out.SymbolsContext = g1SymbolsContext(c, ledger, out.Alternatives)
	prod := map[string]string{}
	for _, a := range recs {
		prod[a.ID] = a.Production
	}
	out.Audit, err = g1Audit(c, out.Alternatives, prod)
	if err != nil {
		return out, err
	}
	out.Metamorphic, err = g1Metamorphic(c, ledger, in.Sealed)
	if err != nil {
		return out, err
	}
	for _, a := range out.Alternatives {
		out.Counts.Alternatives++
		if a.Status == G1ExpExpected {
			out.Counts.Expected++
			out.Counts.Witnesses += len(a.Witnesses)
			if a.Basis == G1BasisSpec {
				out.Counts.SpecDerived++
			} else {
				out.Counts.Differential++
			}
		} else {
			out.Counts.Blocked++
		}
	}
	out.Counts.TokensChecked, out.Counts.SourcesChecked = out.Cross.Tokens, out.Cross.Sources
	for _, m := range out.Cross.Mismatches {
		out.Counts.TokensMismatch += m.Count
	}
	return out, nil
}

// CSharpG1ExpectedSources are the pinned inputs of the derivation.
type CSharpG1ExpectedSources struct {
	Src         CSharpG1Sources
	Observation []byte
	KindMap     []byte
	Sealed      G1ExpMetaResults
}

// g1SpecLabel renders the specification reference of an alternative: the clause and the anchor.
func g1SpecLabel(ref string) string {
	const marker = "/language-specification/"
	i := strings.Index(ref, marker)
	if i < 0 {
		return "C# standard v13 (" + ref + ")"
	}
	doc, anchor, _ := strings.Cut(ref[i+len(marker):], "#")
	digits, _, _ := strings.Cut(anchor, "-")
	chapter := map[string]int{"lexical-structure": 1, "basic-concepts": 1, "types": 1, "variables": 1, "expressions": 2, "classes": 2, "unsafe-code": 2}[doc]
	if chapter == 0 || len(digits) <= chapter {
		return "C# standard v13 (" + doc + "#" + anchor + ")"
	}
	parts := []string{digits[:chapter]}
	rest := digits[chapter:]
	if doc == "lexical-structure" && rest == "510" { // 6.5.10 pragma directives
		parts = append(parts, "5", "10")
	} else {
		for _, r := range rest {
			parts = append(parts, string(r))
		}
	}
	return "C# standard v13 §" + strings.Join(parts, ".") + " (" + doc + "#" + anchor + ")"
}

// ---- canonical bytes and verification ------------------------------------------------------

// MarshalCSharpG1Expected writes the canonical asset: one line per alternative and per list entry.
func MarshalCSharpG1Expected(x CSharpG1Expected) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	scalar := func(name string, v any, last bool) {
		fmt.Fprintf(&b, " %q: ", name)
		put(v)
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	list := func(name string, n int, at func(int) any, last bool) {
		fmt.Fprintf(&b, " %q: [\n", name)
		for i := 0; i < n; i++ {
			b.WriteString("  ")
			put(at(i))
			if i < n-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" ]")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("{\n")
	scalar("schema", x.Schema, false)
	scalar("inputs", x.Inputs, false)
	scalar("basis", x.Basis, false)
	scalar("witness_limit", x.WitnessLimit, false)
	scalar("counts", x.Counts, false)
	list("alternatives", len(x.Alternatives), func(i int) any { return x.Alternatives[i] }, false)
	b.WriteString(` "cross_check": {` + "\n")
	fmt.Fprintf(&b, "  \"sources\": %d, \"tokens\": %d, \"directives\": %d,\n  \"mismatches\": [\n", x.Cross.Sources, x.Cross.Tokens, x.Cross.Directives)
	for i, m := range x.Cross.Mismatches {
		b.WriteString("   ")
		put(m)
		if i < len(x.Cross.Mismatches)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n },\n")
	list("symbols_context", len(x.SymbolsContext), func(i int) any { return x.SymbolsContext[i] }, false)
	b.WriteString(` "audit": {` + "\n")
	fmt.Fprintf(&b, "  \"seed\": %q, \"rate_percent\": %d, \"estimate\": ", x.Audit.Seed, x.Audit.RatePercent)
	put(x.Audit.Estimate)
	b.WriteString(",\n  \"samples\": [\n")
	for i, s := range x.Audit.Samples {
		b.WriteString("   ")
		put(s)
		if i < len(x.Audit.Samples)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n },\n")
	b.WriteString(` "metamorphic": {` + "\n")
	fmt.Fprintf(&b, "  \"seed\": %q, \"transforms\": ", x.Metamorphic.Seed)
	put(x.Metamorphic.Transforms)
	b.WriteString(",\n  \"variants\": [\n")
	for i, v := range x.Metamorphic.Variants {
		b.WriteString("   ")
		put(v)
		if i < len(x.Metamorphic.Variants)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n }\n")
	b.WriteString("}\n")
	return b.Bytes(), werr
}

func g1eSame(a, b any) bool {
	x, e1 := json.Marshal(a)
	y, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && bytes.Equal(x, y)
}

// DecodeCSharpG1Expected decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1Expected(data []byte) (CSharpG1Expected, error) {
	var x CSharpG1Expected
	if len(data) > csharpG1ExpectedMax {
		return x, fail(KindResourceLimit, "G1_EXPECTED_SIZE", CSharpG1ExpectedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_EXPECTED_JSON", CSharpG1ExpectedPath, err)
	}
	if x.Schema != CSharpG1ExpectedSchema || x.Basis == "" {
		return x, g1eFault("G1_EXPECTED_SCHEMA")
	}
	return x, nil
}

// ParseCSharpG1Expected decodes the asset and requires it to equal the derivation from the pinned
// inputs, member by member, with the codes below.
func ParseCSharpG1Expected(data []byte, in CSharpG1ExpectedSources) (CSharpG1Expected, error) {
	x, err := DecodeCSharpG1Expected(data)
	if err != nil {
		return x, err
	}
	// the sealed Roslyn digests are the one input the asset supplies itself
	in.Sealed = x.Metamorphic.Results()
	want, err := DeriveCSharpG1Expected(in)
	if err != nil {
		return x, err
	}
	return x, CheckCSharpG1Expected(data, x, want)
}

// CheckCSharpG1Expected compares a decoded asset with the derivation, member by member.
func CheckCSharpG1Expected(data []byte, x, want CSharpG1Expected) error {
	if !g1eSame(x.Inputs, want.Inputs) {
		return g1eFault("G1_EXPECTED_PIN")
	}
	for _, s := range []struct {
		code string
		a, b any
	}{
		{"G1_EXPECTED_ALTERNATIVES", x.Alternatives, want.Alternatives},
		{"G1_EXPECTED_CROSS", x.Cross, want.Cross},
		{"G1_EXPECTED_SYMBOLS", x.SymbolsContext, want.SymbolsContext},
		{"G1_EXPECTED_AUDIT", x.Audit, want.Audit},
		{"G1_EXPECTED_METAMORPHIC", x.Metamorphic, want.Metamorphic},
		{"G1_EXPECTED_COUNTS", x.Counts, want.Counts},
	} {
		if !g1eSame(s.a, s.b) {
			return g1eFault(s.code)
		}
	}
	// a sealed asset has every disposition authored and every variant observed
	for _, m := range want.Cross.Mismatches {
		if m.Disposition == "" || m.Disposition == "UNDISPOSED" {
			return g1eFault("G1_EXPECTED_UNDISPOSED")
		}
	}
	for _, v := range want.Metamorphic.Variants {
		if v.Structure == "" {
			return g1eFault("G1_EXPECTED_METAMORPHIC_UNSEALED")
		}
	}
	canonical, err := MarshalCSharpG1Expected(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return g1eFault("G1_EXPECTED_BYTES")
	}
	return nil
}

// SealedIDs lists the alternatives of the asset whose status is EXPECTED, in asset order.
func (x CSharpG1Expected) SealedIDs() []string {
	var out []string
	for _, a := range x.Alternatives {
		if a.Status == G1ExpExpected {
			out = append(out, a.ID)
		}
	}
	return out
}

// g1SortedKeys returns the keys of m in order.
func g1SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.SortFunc(out, cmp.Compare[string])
	return out
}
