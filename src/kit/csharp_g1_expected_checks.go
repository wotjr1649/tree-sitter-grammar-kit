package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Cross-checks, the specification sample audit and the metamorphic check of the G1-3a expectation.

// ---- cross check: spec derivation against every observed token and directive ---------------

var g1DirectiveKinds = map[string][]string{
	"define": {"DefineDirectiveTrivia"}, "undef": {"UndefDirectiveTrivia"}, "if": {"IfDirectiveTrivia"},
	"elif": {"ElifDirectiveTrivia"}, "else": {"ElseDirectiveTrivia"}, "endif": {"EndIfDirectiveTrivia"},
	"error": {"ErrorDirectiveTrivia"}, "warning": {"WarningDirectiveTrivia"}, "region": {"RegionDirectiveTrivia"},
	"endregion": {"EndRegionDirectiveTrivia"}, "line": {"LineDirectiveTrivia", "LineSpanDirectiveTrivia"},
	"nullable": {"NullableDirectiveTrivia"}, "pragma": {"PragmaWarningDirectiveTrivia", "PragmaChecksumDirectiveTrivia"},
}

type g1DispositionKey struct{ Code, Roslyn, Spec string }

type g1Disposition struct {
	Disposition string // SPEC_AMBIGUOUS | ROSLYN_DEVIATION | GRAMMAR_DEFECT_CANDIDATE | BEYOND_SPEC_EDITION
	Citation    string
	Decision    string
}

func g1CrossCheck(c *g1Ctx) (G1ExpCross, error) {
	var out G1ExpCross
	acc := map[g1DispositionKey]*G1ExpMismatch{}
	note := func(code, roslyn, spec string, s *g1Src, at int) {
		k := g1DispositionKey{code, roslyn, spec}
		m := acc[k]
		if m == nil {
			m = &G1ExpMismatch{Code: code, Roslyn: roslyn, Spec: spec, Examples: []string{}}
			acc[k] = m
		}
		m.Count++
		if len(m.Examples) < 3 {
			m.Examples = append(m.Examples, fmt.Sprintf("%s@%d", s.id, at))
		}
	}
	numeric := map[string]bool{"NumericLiteralToken": true}
	str := map[string]bool{"StringLiteralToken": true, "Utf8StringLiteralToken": true}
	for _, s := range c.order {
		if !s.accepted {
			continue
		}
		out.Sources++
		for _, t := range s.toks {
			if t.end == t.start {
				continue
			}
			out.Tokens++
			km := c.kmIdx[t.kind]
			forms := g1sFormsOf(t.text)
			isForm := func(prefix string) bool {
				for _, f := range forms {
					if strings.HasPrefix(f, prefix) {
						return true
					}
				}
				return false
			}
			switch {
			case c.keywords[t.text]:
				if km.Text != t.text {
					note("KEYWORD_KIND", t.kind, "reserved keyword (6.4.4)", s, t.start)
				}
			case c.puncts[t.text]:
				if km.Text != t.text {
					note("PUNCTUATOR_KIND", t.kind, "operator or punctuator (6.4.6)", s, t.start)
				}
			case numeric[t.kind]:
				if !isForm("int.") && !isForm("real.") {
					note("NUMERIC_FORM", t.kind, "no integer or real literal form (6.4.5.3, 6.4.5.4)", s, t.start)
				}
			case t.kind == "CharacterLiteralToken":
				if !isForm("char") {
					note("CHARACTER_FORM", t.kind, "no character literal form (6.4.5.5)", s, t.start)
				}
			case str[t.kind]:
				if !isForm("string.") {
					note("STRING_FORM", t.kind, "no string literal form (6.4.5.6)", s, t.start)
				}
			case t.kind == "IdentifierToken":
				if !isForm("ident.") {
					note("IDENTIFIER_FORM", t.kind, "no identifier form (6.4.3)", s, t.start)
				}
			case strings.Contains(t.kind, "RawString"):
				note("TOKEN_OF_OTHER_ROW", t.kind, "raw string literal (6.4.5.6 Raw_String_Literal): an alternative of row csharp-V11, not of csharp-B01", s, t.start)
			case strings.HasPrefix(t.kind, "Interpolated"):
				// the parts of an interpolated string are checked through their node, not as lexemes
			default:
				switch {
				case isForm("int.") || isForm("real."):
					note("NUMERIC_KIND", t.kind, "integer or real literal form", s, t.start)
				case isForm("char"):
					note("CHARACTER_KIND", t.kind, "character literal form", s, t.start)
				case isForm("string."):
					note("STRING_KIND", t.kind, "string literal form", s, t.start)
				case isForm("ident.") && km.Text != t.text && t.kind != "UnderscoreToken":
					note("IDENTIFIER_KIND", t.kind, "identifier form (6.4.3)", s, t.start)
				}
			}
		}
		for _, d := range s.dirs {
			out.Directives++
			line, _ := s.directive(d)
			k := g1sDirectiveKind(line)
			switch {
			case k == "":
				note("DIRECTIVE_BEYOND_SPEC", d.Kind, "not a directive of 6.5", s, d.Start)
			case !slices.Contains(g1DirectiveKinds[k], d.Kind):
				note("DIRECTIVE_KIND", d.Kind, "#"+k+" (6.5)", s, d.Start)
			}
		}
	}
	keys := make([]g1DispositionKey, 0, len(acc))
	for k := range acc {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b g1DispositionKey) int {
		return strings.Compare(a.Code+"/"+a.Roslyn+"/"+a.Spec, b.Code+"/"+b.Roslyn+"/"+b.Spec)
	})
	out.Mismatches = []G1ExpMismatch{}
	for _, k := range keys {
		m := *acc[k]
		d, ok := csharpG1Dispositions[k.Code+"/"+k.Roslyn]
		if !ok {
			d = g1Disposition{Disposition: "UNDISPOSED"}
		}
		m.Disposition, m.Citation, m.Decision = d.Disposition, d.Citation, d.Decision
		out.Mismatches = append(out.Mismatches, m)
	}
	return out, nil
}

// ---- symbols context ----------------------------------------------------------------------

// g1SymbolsContext lists the registered steps whose ledger syntax is ERROR while Roslyn accepts the
// source in the default mode (no preprocessor symbols): the error sits in a conditionally excluded
// section, so the default observation cannot judge it. Only steps of cases the ledger links to an
// obligation of the two rows are listed, with those obligations.
func g1SymbolsContext(c *g1Ctx, ledger CSharpG1Ledger, alts []G1ExpAlt) []G1ExpSymbols {
	linked := map[string][]string{} // case -> "<row>:<kind>" of the two rows
	for _, r := range ledger.Rows {
		if r.Row != g1ExpRowB01 && r.Row != g1ExpRowB02 {
			continue
		}
		for _, o := range r.Obligations {
			for _, l := range o.Links {
				linked[l.Case] = append(linked[l.Case], r.Row+":"+o.Kind)
			}
		}
	}
	out := []G1ExpSymbols{}
	for _, cs := range ledger.Cases {
		for _, st := range cs.Steps {
			if st.Syntax != "ERROR" {
				continue
			}
			s := c.byID[fmt.Sprintf("%s#%d", cs.ID, st.Step)]
			if s == nil || !s.accepted || linked[cs.ID] == nil {
				continue
			}
			out = append(out, G1ExpSymbols{Source: s.id, Rows: linked[cs.ID], Disposition: "DEFAULT_MODE_ONLY_SYMBOLS_CONTEXT_UNJUDGED"})
		}
	}
	return out
}

// ---- specification sample audit (D2) -------------------------------------------------------

const (
	g1AuditSeed    = "csharp-g1-3a-audit/r1"
	g1AuditPercent = 5
)

type G1ExpAuditHand struct {
	Status string   `json:"status"`
	Block  string   `json:"block,omitempty"`
	TS     []string `json:"ts"`
	Text   string   `json:"text,omitempty"` // regexp every witness text must match, written from the specification
}

type G1ExpAuditSample struct {
	ID      string         `json:"id"`
	Row     string         `json:"row"`
	Clause  string         `json:"clause"`
	Hand    G1ExpAuditHand `json:"hand"`
	Outcome string         `json:"outcome"` // MATCH, COVERAGE_GAP (no linked source exhibits it) or MISMATCH
	Why     string         `json:"why,omitempty"`
}

type G1ExpAuditEstimate struct {
	Samples     int    `json:"samples"`
	Mismatches  int    `json:"mismatches"`
	Upper95     string `json:"upper_95_percent"`
	Description string `json:"description"`
}

type G1ExpAudit struct {
	Seed        string             `json:"seed"`
	RatePercent int                `json:"rate_percent"`
	Estimate    G1ExpAuditEstimate `json:"estimate"`
	Samples     []G1ExpAuditSample `json:"samples"`
}

// g1AuditPick selects ceil(5%) of each row's alternatives, at least one, by the smallest
// SHA-256(seed|id): deterministic and independent of the expectation.
func g1AuditPick(ids map[string][]string) []string {
	var out []string
	for _, row := range []string{g1ExpRowB01, g1ExpRowB02} {
		n := len(ids[row])
		k := (n*g1AuditPercent + 99) / 100
		if k < 1 {
			k = 1
		}
		type kv struct{ id, h string }
		var all []kv
		for _, id := range ids[row] {
			h := sha256.Sum256([]byte(g1AuditSeed + "|" + id))
			all = append(all, kv{id, hex.EncodeToString(h[:])})
		}
		slices.SortFunc(all, func(a, b kv) int { return strings.Compare(a.h, b.h) })
		for _, e := range all[:k] {
			out = append(out, e.id)
		}
	}
	slices.Sort(out)
	return out
}

func g1Audit(c *g1Ctx, alts []G1ExpAlt, prod map[string]string) (G1ExpAudit, error) {
	a := G1ExpAudit{Seed: g1AuditSeed, RatePercent: g1AuditPercent, Samples: []G1ExpAuditSample{}}
	ids := map[string][]string{}
	byID := map[string]G1ExpAlt{}
	for _, alt := range alts {
		ids[alt.Row] = append(ids[alt.Row], alt.ID)
		byID[alt.ID] = alt
	}
	for _, id := range g1AuditPick(ids) {
		alt := byID[id]
		hand, ok := csharpG1AuditHand[id]
		if !ok {
			return a, fmt.Errorf("audit sample %s has no hand derivation (%s)", id, prod[id])
		}
		s := G1ExpAuditSample{ID: id, Row: alt.Row, Clause: hand.Clause, Hand: hand.G1ExpAuditHand, Outcome: "MATCH"}
		fail := func(why string) {
			s.Outcome = "MISMATCH"
			if s.Why == "" {
				s.Why = why
			}
		}
		switch {
		case hand.Status == G1ExpExpected && alt.Block == "NO_WITNESS_IN_LINKED_SOURCES":
			s.Outcome, s.Why = "COVERAGE_GAP", "the specification derives an expectation but no accepted linked source exhibits it"
		case hand.Status != alt.Status:
			fail("hand derivation status " + hand.Status + " differs from the expectation status " + alt.Status)
		case hand.Status == G1ExpBlocked && hand.Block != alt.Block:
			fail("hand derivation block " + hand.Block + " differs from " + alt.Block)
		case hand.Status == G1ExpExpected:
			for _, t := range hand.TS {
				if !slices.Contains(alt.TS, t) {
					fail("type " + t + " derived by hand is not allowed by the expectation")
				}
			}
			re, err := regexp.Compile(hand.Text)
			if err != nil {
				return a, err
			}
			for _, w := range alt.Witnesses {
				if src := c.byID[w.Source]; src == nil || !re.MatchString(src.text(w.Start, w.End)) {
					fail(fmt.Sprintf("witness %s@%d does not read as %s", w.Source, w.Start, hand.Text))
				}
			}
		}
		a.Samples = append(a.Samples, s)
	}
	n := len(a.Samples)
	a.Estimate = G1ExpAuditEstimate{Samples: n, Description: "specification sample audit: a hand derivation from the specification, written before the expectation was read, compared with the expectation. A mismatch counts as a Roslyn-or-derivation error of the sample; with no mismatch the upper bound is the rule of three (3/n at 95%)"}
	for _, s := range a.Samples {
		if s.Outcome == "MISMATCH" {
			a.Estimate.Mismatches++
		}
	}
	// shortcut: with no mismatch the sealed r1 bytes hold the rule of three (300/n, above the exact bound), so it
	// stays; with mismatches the bound is the exact binomial one (a point estimate is not an upper bound).
	if a.Estimate.Mismatches == 0 && n > 0 {
		a.Estimate.Upper95 = fmt.Sprintf("%.1f", 300/float64(n))
	} else if n > 0 {
		a.Estimate.Upper95 = fmt.Sprintf("%.1f", G1AuditUpper95(a.Estimate.Mismatches, n))
	}
	return a, nil
}

type g1AuditEntry struct {
	Clause string
	G1ExpAuditHand
}

// ---- metamorphic check (D2) ---------------------------------------------------------------

const (
	g1MetaSeed  = "csharp-g1-3a-metamorphic/r1"
	g1MetaCount = 20
	g1MetaLimit = 8000 // bytes: representative sources stay small
)

var g1MetaTransforms = []G1ExpMetaTransform{
	{ID: "whitespace", Description: "double the first byte of every whitespace-only gap between two tokens"},
	{ID: "comment", Description: "insert /*c*/ at the end of every whitespace-only gap between two tokens"},
	{ID: "rename", Description: "rename each distinct plain identifier that is not a keyword or contextual keyword to Zq<n>, consistently"},
	{ID: "combined", Description: "apply whitespace, comment and rename together"},
}

type G1ExpMetaTransform struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// G1ExpMetaVariant is one transformed source. Structure is the digest of Roslyn's observation of the
// variant (sealed data); BaseStructure is the digest of the original observation. Same is
// Structure == BaseStructure.
type G1ExpMetaVariant struct {
	Source        string `json:"source"`
	Transform     string `json:"transform"`
	Bytes         int    `json:"bytes"`
	SHA256        string `json:"sha256"`
	BaseStructure string `json:"base_structure"`
	Structure     string `json:"structure"`
	Same          bool   `json:"same"`
}

type G1ExpMetamorphic struct {
	Seed       string               `json:"seed"`
	Transforms []G1ExpMetaTransform `json:"transforms"`
	Variants   []G1ExpMetaVariant   `json:"variants"`
}

// G1ExpMetaResults are the sealed structure digests of the Roslyn run, by "<source>|<transform>".
type G1ExpMetaResults map[string]string

// Results returns the sealed digests the asset carries.
func (m G1ExpMetamorphic) Results() G1ExpMetaResults {
	out := G1ExpMetaResults{}
	for _, v := range m.Variants {
		if v.Structure != "" {
			out[v.Source+"|"+v.Transform] = v.Structure
		}
	}
	return out
}

// CSharpG1StructureDigest is the digest the metamorphic check compares: the acceptance, the diagnostic
// IDs, the node kinds with their depth and the token kinds with their missing flag. Spans are left out:
// a transform moves them.
func CSharpG1StructureDigest(kinds []string, o G1RoslynSource) string {
	var b strings.Builder
	acc := o.Accepted != nil && *o.Accepted
	fmt.Fprintf(&b, "accepted %v\n", acc)
	for _, d := range o.Diagnostics {
		fmt.Fprintf(&b, "diag %s\n", d.ID)
	}
	var ends []int
	for _, n := range o.Nodes {
		for len(ends) > 0 && !(n[2] <= ends[len(ends)-1]) {
			ends = ends[:len(ends)-1]
		}
		fmt.Fprintf(&b, "node %d %s\n", len(ends), kinds[n[0]])
		ends = append(ends, n[2])
	}
	miss := map[int]bool{}
	for _, m := range o.Missing {
		miss[m] = true
	}
	for i, t := range o.Tokens {
		fmt.Fprintf(&b, "token %s %v\n", kinds[t[0]], miss[i])
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// g1MetaGaps returns the whitespace-only gaps between tokens of a source: [start,end) byte ranges.
func g1MetaGaps(s *g1Src) [][2]int {
	var out [][2]int
	prev := 0
	gap := func(to int) {
		if to > prev {
			ok := true
			for _, b := range s.data[prev:to] {
				if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, [2]int{prev, to})
			}
		}
	}
	for _, t := range s.toks {
		gap(t.start)
		prev = t.end
	}
	gap(len(s.data))
	return out
}

func g1MetaKeepName(c *g1Ctx) func(string) bool {
	keep := map[string]bool{}
	for k := range c.keywords {
		keep[k] = true
	}
	for _, k := range c.km.Kinds {
		if k.Text != "" {
			keep[k.Text] = true // contextual keyword kinds carry their lexeme too
		}
	}
	for _, w := range strings.Fields("dynamic nameof field extension async await var value record managed unmanaged notnull and or not with when where yield get set init add remove partial global alias scoped required file from select let join on equals into orderby ascending descending group by allows") {
		keep[w] = true
	}
	return func(n string) bool { return keep[n] }
}

var g1MetaPlainIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// g1MetaApply returns the source after the named transform, from the observed tokens.
func g1MetaApply(c *g1Ctx, s *g1Src, transform string) []byte {
	doWS := transform == "whitespace" || transform == "combined"
	doCmt := transform == "comment" || transform == "combined"
	doRen := transform == "rename" || transform == "combined"
	keep := g1MetaKeepName(c)
	names := map[string]string{}
	type edit struct {
		at, del int
		ins     string
	}
	var edits []edit
	if doWS || doCmt {
		for _, g := range g1MetaGaps(s) {
			if doWS {
				edits = append(edits, edit{g[0], 0, string(s.data[g[0]])})
			}
			if doCmt {
				edits = append(edits, edit{g[1], 0, "/*c*/"})
			}
		}
	}
	if doRen {
		for _, t := range s.toks {
			if t.kind != "IdentifierToken" || !g1MetaPlainIdent.MatchString(t.text) || keep(t.text) || t.text == "_" {
				continue
			}
			n, ok := names[t.text]
			if !ok {
				n = fmt.Sprintf("Zq%d", len(names)+1)
				names[t.text] = n
			}
			edits = append(edits, edit{t.start, t.end - t.start, n})
		}
	}
	slices.SortStableFunc(edits, func(a, b edit) int { return a.at - b.at })
	var out []byte
	pos := 0
	for _, e := range edits {
		out = append(out, s.data[pos:e.at]...)
		out = append(out, e.ins...)
		pos = e.at + e.del
	}
	return append(out, s.data[pos:]...)
}

// G1MetaVariantSources returns the transformed sources of the metamorphic check in asset order, for
// the Roslyn run: id "<source>|<transform>".
func G1MetaVariantSources(in CSharpG1ExpectedSources) ([]G1RoslynCaseSource, error) {
	_, c, ledger, err := g1ExpContext(in)
	if err != nil {
		return nil, err
	}
	recs, err := g1ExpRuleSrcs(in)
	if err != nil {
		return nil, err
	}
	if _, err := g1BuildRules(c, recs); err != nil { // fills the keyword set the rename transform keeps
		return nil, err
	}
	var out []G1RoslynCaseSource
	for _, id := range g1MetaPick(c, ledger) {
		s := c.byID[id]
		for _, tr := range g1MetaTransforms {
			b := g1MetaApply(c, s, tr.ID)
			out = append(out, G1RoslynCaseSource{ID: id + "|" + tr.ID, Case: s.caseID, Step: s.step, SHA256: digestHex(b), Data: b})
		}
	}
	return out, nil
}

// g1MetaPick chooses the representative sources: accepted sources of cases linked to the two rows,
// small, by the smallest SHA-256(seed|id).
func g1MetaPick(c *g1Ctx, ledger CSharpG1Ledger) []string {
	linked := map[string]bool{}
	for _, a := range ledger.Alternatives {
		if a.Row == g1ExpRowB01 || a.Row == g1ExpRowB02 {
			for _, l := range a.Links {
				linked[l.Case] = true
			}
		}
	}
	type kv struct{ id, h string }
	var all []kv
	for _, s := range c.order {
		if s.accepted && linked[s.caseID] && len(s.data) <= g1MetaLimit && len(s.toks) > 8 {
			h := sha256.Sum256([]byte(g1MetaSeed + "|" + s.id))
			all = append(all, kv{s.id, hex.EncodeToString(h[:])})
		}
	}
	slices.SortFunc(all, func(a, b kv) int { return strings.Compare(a.h, b.h) })
	var out []string
	for i := 0; i < len(all) && i < g1MetaCount; i++ {
		out = append(out, all[i].id)
	}
	return out
}

func g1Metamorphic(c *g1Ctx, ledger CSharpG1Ledger, sealed G1ExpMetaResults) (G1ExpMetamorphic, error) {
	m := G1ExpMetamorphic{Seed: g1MetaSeed, Transforms: g1MetaTransforms, Variants: []G1ExpMetaVariant{}}
	for _, id := range g1MetaPick(c, ledger) {
		s := c.byID[id]
		var o G1RoslynSource
		for _, x := range c.obs.Sources {
			if x.ID == id {
				o = x
			}
		}
		base := CSharpG1StructureDigest(c.obs.Kinds, o)
		for _, tr := range g1MetaTransforms {
			b := g1MetaApply(c, s, tr.ID)
			v := G1ExpMetaVariant{Source: id, Transform: tr.ID, Bytes: len(b), SHA256: digestHex(b), BaseStructure: base}
			v.Structure = sealed[id+"|"+tr.ID]
			v.Same = v.Structure != "" && v.Structure == base
			m.Variants = append(m.Variants, v)
		}
	}
	return m, nil
}

// CSharpG1AuditSamples lists the alternatives the audit samples (ID and production text), so the hand
// derivations can be written from the specification before any expectation is read.
func CSharpG1AuditSamples(in CSharpG1ExpectedSources) ([][2]string, error) {
	recs, err := g1ExpRuleSrcs(in)
	if err != nil {
		return nil, err
	}
	ids := map[string][]string{}
	prod := map[string]string{}
	for _, r := range recs {
		ids[r.Row] = append(ids[r.Row], r.ID)
		prod[r.ID] = r.Production
	}
	var out [][2]string
	for _, id := range g1AuditPick(ids) {
		out = append(out, [2]string{id, prod[id]})
	}
	return out, nil
}
