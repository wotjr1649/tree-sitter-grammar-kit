package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// G1 native comparison of the sealed B03 and B04 expectations (#233 G1-3b PR3, native step).
//
// The sealed asset csharp-g1-expected-b03b04-r1.json is a read-only input. For every EXPECTED alternative the comparison splits each
// witness into parts and looks, for every part, at the native tree-sitter-c-sharp tree of the witness's source:
//
//   - group A (witness match "tokens"): one part per anchor token, with the anchor's own sealed types and the token's span. The decision
//     D-1 holds: the node that holds the anchors is not compared.
//   - group B (witness match "span"): one part, the witness's span with the alternative's sealed types.
//
// A part is MATCH when a native node of one of its sealed types has exactly its span, else MISMATCH; the whole of a witness is MATCH when
// every part is, MISMATCH when one part is not, and NOT_APPLICABLE when the native parse of its source did not complete. The alternative
// has the result of its witness. Only a MATCH alternative is EXECUTED in the ledger (R-2); a mismatch is never repaired: the sealed
// assets stay as they are, the mismatch gets a disposition with a cause and is reported. Nothing in this file can write an expectation,
// the kind map or an observation.

const (
	CSharpG1B34ExecutedSchema = "tsgk-csharp-g1-executed-b03b04/r1"
	CSharpG1B34ExecutedPath   = "src/contracts/csharp-g1-executed-b03b04-r1.json"
	// r2 compares the expected asset r2 (the sealed r1 asset with the post-seal corrections) with the same raw native observation; r1 is
	// the comparison of the sealed asset and stays as it is.
	CSharpG1B34ExecutedSchemaR2 = "tsgk-csharp-g1-executed-b03b04/r2"
	CSharpG1B34ExecutedPathR2   = "src/contracts/csharp-g1-executed-b03b04-r2.json"
	// shortcut: the asset is far below this today; raise only with a new revision.
	csharpG1B34ExecutedMax = 4 << 20
)

// G1B34ExecPart is one native check of a witness: a node of one of TS with exactly [Start, End). Text is the anchor of a group A
// part. Found are the native nodes with exactly that span, the nodes of a sealed type first, at most g1ExecFoundLimit; Near (MISMATCH
// only) are the native nodes that overlap the span and are no longer than it.
type G1B34ExecPart struct {
	Start  int           `json:"start"`
	End    int           `json:"end"`
	Text   string        `json:"text,omitempty"`
	TS     []string      `json:"ts"`
	Result string        `json:"result"`
	Found  []G1ExecFound `json:"found"`
	Near   []G1ExecFound `json:"near,omitempty"`
}

// G1B34ExecWitness repeats the sealed witness (its source, match and Roslyn kind) and records the native comparison of its parts.
type G1B34ExecWitness struct {
	Source string          `json:"source"`
	Match  string          `json:"match"`
	Roslyn string          `json:"roslyn,omitempty"`
	Result string          `json:"result"`
	Parts  []G1B34ExecPart `json:"parts"`
}

type G1B34ExecAlt struct {
	ID             string             `json:"id"`
	Row            string             `json:"row"`
	Group          string             `json:"group"`
	Basis          string             `json:"basis"`
	Discriminating bool               `json:"discriminating"`
	Tier           string             `json:"tier,omitempty"`
	Corrected      bool               `json:"corrected,omitempty"` // r2: a post-seal correction touches the alternative
	Executed       bool               `json:"executed"`
	Result         string             `json:"result"`
	Witnesses      []G1B34ExecWitness `json:"witnesses"`
}

type G1B34ExecInputs struct {
	Expected    queryregistrycore.Pin `json:"expected"`
	Observation queryregistrycore.Pin `json:"observation"`
	Raw         queryregistrycore.Pin `json:"raw_native"`
}

// The tiers of a discriminating group A alternative (tier of the executed asset; the sealed `discriminating` flag is unchanged). Only the
// word tier counts as discriminating in the counts: the other two are discriminating by the sealed flag but prove less (review of PR2).
const (
	G1B34TierWord      = "word"      // a word anchor whose sealed type is one node type
	G1B34TierCandidate = "candidate" // the word anchors' sealed types are candidate sets (the type audit: NOT_DERIVABLE), e.g. ["x", modifier]
	G1B34TierOperator  = "operator"  // no word anchor: discriminating by an operator of two or more characters only
)

// g1bxWord is a word anchor: the comparison keeps its own copy of the classifier's rule (a comparison file does not use the derivation's
// variables; TestCSharpG1B34ExecutedTiers checks that the two are one expression).
var g1bxWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// g1bxTier is the tier of a sealed alternative: "" for group B and for a weak group A alternative.
func g1bxTier(a G1B34Alt) string {
	if a.Group != G1ClassLiteral || !a.Discriminating {
		return ""
	}
	tier := G1B34TierOperator
	for _, an := range a.Anchors {
		if !g1bxWord.MatchString(an.Text) {
			continue
		}
		if len(an.TS) == 1 {
			return G1B34TierWord
		}
		tier = G1B34TierCandidate
	}
	return tier
}

// G1B34ExecCounts keeps group A and group B, discriminating and weak matches apart: a weak match proves less (D-1, R-5). Group A is split
// in three: match_discriminating_a is the word tier only, match_candidate_a and match_operator_a are the other two discriminating tiers.
type G1B34ExecCounts struct {
	Alternatives        int `json:"alternatives"`
	Executed            int `json:"executed"`
	Match               int `json:"match"`
	Mismatch            int `json:"mismatch"`
	NotApplicable       int `json:"not_applicable"`
	MatchStrongA        int `json:"match_discriminating_a"`
	MatchCandidateA     int `json:"match_candidate_a"`
	MatchOperatorA      int `json:"match_operator_a"`
	MatchWeakA          int `json:"match_weak_a"`
	MatchStrongB        int `json:"match_discriminating_b"`
	MatchWeakB          int `json:"match_weak_b"`
	Witnesses           int `json:"witnesses"`
	Parts               int `json:"parts"`
	PartMatch           int `json:"part_match"`
	PartMismatch        int `json:"part_mismatch"`
	Sources             int `json:"sources"`
	SourcesCompleted    int `json:"sources_completed"`
	NativeErrorAccepted int `json:"native_error_roslyn_accepted"`
	// r2 only: the alternatives a post-seal correction touched, how many of them match, and how many sealed ones match.
	Corrected      int `json:"corrected,omitempty"`
	CorrectedMatch int `json:"corrected_match,omitempty"`
	SealedMatch    int `json:"sealed_match,omitempty"`
}

type CSharpG1B34Executed struct {
	Schema       string              `json:"schema"`
	Inputs       G1B34ExecInputs     `json:"inputs"`
	Basis        string              `json:"basis"`
	Native       G1NativeProvenance  `json:"native"`
	Counts       G1B34ExecCounts     `json:"counts"`
	Sources      []G1ExecSource      `json:"sources"`
	Alternatives []G1B34ExecAlt      `json:"alternatives"`
	Dispositions []G1ExecDisposition `json:"dispositions"`
}

const csharpG1B34ExecutedBasis = "Native comparison of the sealed expected asset for csharp-B03 and csharp-B04. For every EXPECTED alternative the witness (copied unchanged from the sealed asset) is split into parts: a group A witness (match tokens) has one part per anchor token, with the anchor's sealed types and the token's span; a group B witness (match span) has one part, the witness's span with the alternative's sealed types. The tree-sitter-c-sharp tree of the witness's source, parsed by the registered native build (provenance in native; raw_native pins the local raw observation), is searched for a node of one of the part's sealed types with exactly the part's span: MATCH, else MISMATCH. A witness is MATCH when every part is, MISMATCH when one is not, and NOT_APPLICABLE when the native parse did not complete; an alternative has the result of its only witness and is executed when a completed native witness was compared. Decision D-1: a group A match says the anchor tokens are the sealed node types at those spans, not that the node holding them has any shape, and a weak match (discriminating false) proves less than a discriminating one. A discriminating group A alternative has a tier: word (a word anchor with one sealed node type), candidate (its word anchors' sealed types are candidate sets such as a keyword that may also be a modifier) or operator (no word anchor, an operator of two or more characters only); only the word tier counts as discriminating, and the counts keep the tiers, the weak matches and group B apart. Each mismatch has a disposition with a cause: GRAMMAR_DEFECT, KIND_MAP_ERROR, EXPECTED_ERROR or DESIGN_DIFFERENCE. Expectations, the kind map and the observation are not changed by this asset; a sealed value that the native result contradicts stays as sealed and is reported. The comparison covers the witnesses only: it does not say the grammar parses every construct of an alternative, and a MATCH is a property of the pinned Roslyn observation's spans on these sources."

// csharpG1B34ExecutedBasisR2 is the basis of the r2 comparison: the r1 text and what r2 changes.
const csharpG1B34ExecutedBasisR2 = csharpG1B34ExecutedBasis + " Revision r2 compares the expected asset r2 (the sealed r1 asset with its post-seal corrections: four anchor types and one alternative moved to BLOCKED) with the same raw native observation as r1, so no native run was repeated: the sealed alternatives that no correction touched are compared with their sealed values, the corrected alternatives with the corrected ones (corrected, and counted apart in corrected_match and sealed_match). r1 is the comparison of the sealed asset, with the five mismatches and their dispositions, and stays as it is."

func g1bxFault(code string) error {
	return fail(KindInvalidInput, code, CSharpG1B34ExecutedPath, nil)
}

// g1B34ExecDispositions are the authored causes of the mismatches of this asset, keyed by alternative. A mismatch without an entry is
// rejected.
var g1B34ExecDispositions = map[string]G1ExecDisposition{
	"csharp-B03.a184": g1bxVarAnchor("alt-csharp-B03-09#0", "[238,241)", "a declaration_expression in a tuple expression"),
	"csharp-B03.a236": g1bxVarAnchor("alt-csharp-B03-09#0", "[238,241)", "a declaration_expression in a tuple expression"),
	"csharp-B04.a028": g1bxVarAnchor("alt-csharp-B04-01#0", "[82,85)", "the type of a local variable declaration"),
	"csharp-B04.a029": g1bxVarAnchor("alt-csharp-B04-01#0", "[82,85)", "the type of a local variable declaration"),
	"csharp-B03.a239": {Cause: G1CauseDesign, Reason: "The sealed witness is the nested designation [287,293) of a deconstructing declaration with an implicit type in alt-csharp-B03-09#0, which Roslyn reads as ParenthesizedVariableDesignation (the kind map and the type audit give parenthesized_variable_designation). The pinned grammar text has no such node there: variable_declarator is choice(name, tuple_pattern) and tuple_pattern nests tuple_pattern (grammar.js, variable_declarator and tuple_pattern), so native gives tuple_pattern for [287,293) inside tuple_pattern [286,297). parenthesized_variable_designation exists only under _variable_designation (patterns), a context the witness is not in. Both representations are valid; the sealed type is right for the pattern contexts of the alternative and was never compared there, because the alternative has one witness. The sealed assets stay as they are; a correction is a new witness in a pattern context or a context entry in a later kind map revision."},
}

// g1bxVarAnchor is the disposition of an anchor `var` whose sealed types are those of the Roslyn IdentifierToken the observation holds at that
// span (identifier, implicit_parameter): the grammar gives the contextual keyword the roles implicit_type, anonymous "var" (var_pattern) and
// identifier (_reserved_identifier), so the sealed set misses the role the witness is in.
func g1bxVarAnchor(source, span, role string) G1ExecDisposition {
	return G1ExecDisposition{Cause: G1CauseExpected, Reason: "The sealed anchor types of `var` are the kind map's types for the Roslyn IdentifierToken observed at " + span + " of " + source + " (identifier, implicit_parameter), a context-free list. The witness is `var` as " + role + ": the pinned grammar text defines implicit_type: 'var', so native gives the named node implicit_type and its anonymous child \"var\", both with exactly the token's span, and neither is in the sealed set. The type audit of the anchor vocabulary (typeaudit-b03b04-r1) already lists the candidate set \"var\", identifier, implicit_parameter, implicit_type for this lexeme from grammar.js alone (NOT_DERIVABLE: the type depends on the role of the lexeme); the sealed expectation took the narrower set of the kind map, which is the error. The kind map entry itself is right for identifiers. The sealed assets stay as they are."}
}

// CSharpG1B34NativeSourceIDs lists, in order, the source versions the comparison needs: the witness sources of the EXPECTED alternatives.
func CSharpG1B34NativeSourceIDs(x CSharpG1B34) []string {
	set := map[string]bool{}
	for _, a := range x.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		for _, w := range a.Witnesses {
			set[w.Source] = true
		}
	}
	return g1SortedKeys(set)
}

// g1bxParts splits a sealed witness into the parts that are compared.
func g1bxParts(a G1B34Alt, w G1B34Witness) ([]G1B34ExecPart, bool) {
	switch w.Match {
	case g1b34MatchTokens:
		if len(w.Tokens) == 0 || len(w.Tokens) != len(a.Anchors) {
			return nil, false
		}
		var out []G1B34ExecPart
		for i, t := range w.Tokens {
			out = append(out, G1B34ExecPart{Start: t[0], End: t[1], Text: a.Anchors[i].Text, TS: a.Anchors[i].TS})
		}
		return out, true
	case g1b34MatchSpan:
		if len(w.Span) != 2 {
			return nil, false
		}
		return []G1B34ExecPart{{Start: w.Span[0], End: w.Span[1], TS: a.TS}}, true
	}
	return nil, false
}

// g1bxComparePart compares one part with the native tree of its source.
func g1bxComparePart(p G1B34ExecPart, t g1xTree) G1B34ExecPart {
	p.Found = []G1ExecFound{}
	if !t.completed() {
		p.Result = G1ResNotApplicable
		return p
	}
	var hit, other []G1ExecFound
	for i := range t.src.Nodes {
		n := t.node(i)
		if n.Start != p.Start || n.End != p.End {
			continue
		}
		if slices.Contains(p.TS, n.Type) {
			hit = append(hit, n)
		} else {
			other = append(other, n)
		}
	}
	p.Found = slices.Concat(hit, other)
	if p.Found == nil {
		p.Found = []G1ExecFound{}
	}
	if len(p.Found) > g1ExecFoundLimit {
		p.Found = p.Found[:g1ExecFoundLimit]
	}
	if len(hit) > 0 {
		p.Result = G1ResMatch
		return p
	}
	p.Result = G1ResMismatch
	for i := range t.src.Nodes {
		n := t.node(i)
		if n.Start < p.End && n.End > p.Start && n.End-n.Start <= max(p.End-p.Start, 1) && len(p.Near) < g1ExecFoundLimit {
			p.Near = append(p.Near, n)
		}
	}
	return p
}

// g1bxWitnessResult is the result of a witness from the results of its parts.
func g1bxWitnessResult(parts []G1B34ExecPart) string {
	res := G1ResMatch
	for _, p := range parts {
		switch p.Result {
		case G1ResMismatch:
			return G1ResMismatch
		case G1ResNotApplicable:
			res = G1ResNotApplicable
		}
	}
	if len(parts) == 0 {
		return G1ResNotApplicable
	}
	return res
}

// g1bxAltResult aggregates witness results into the alternative's result, like the lexical rows do.
func g1bxAltResult(ws []G1B34ExecWitness) (result string, executed bool) {
	mismatch, match := false, false
	for _, w := range ws {
		switch w.Result {
		case G1ResMismatch:
			mismatch, executed = true, true
		case G1ResMatch:
			match, executed = true, true
		}
	}
	switch {
	case mismatch:
		return G1ResMismatch, executed
	case match:
		return G1ResMatch, executed
	}
	return G1ResNotApplicable, executed
}

// CSharpG1B34ExecutedSources are the inputs of the comparison. Raw is the local raw native observation; the offline verification runs
// without it.
type CSharpG1B34ExecutedSources struct {
	Src         CSharpG1Sources // the pinned declarations; Expected and Executed are ignored
	Expected    []byte          // csharp-g1-expected-b03b04-r1.json
	Observation []byte          // csharp-g1-roslyn-observed-r3.json
	Raw         []byte
	// R2 selects the revision: the expected asset is the r2 asset and the result is the r2 executed asset.
	R2 bool
}

type g1bxContext struct {
	r2       bool
	exp      CSharpG1B34
	sources  map[string]G1RoslynCaseSource
	accepted map[string]bool
	ids      []string
	inputs   G1B34ExecInputs
}

func g1bxContextOf(in CSharpG1B34ExecutedSources) (g1bxContext, error) {
	var c g1bxContext
	exp, err := DecodeCSharpG1B34(in.Expected)
	if err != nil {
		return c, err
	}
	var obs CSharpG1RoslynObserved
	if err := jsonv2.Unmarshal(in.Observation, &obs, jsonv2.RejectUnknownMembers(true)); err != nil {
		return c, fail(KindInvalidInput, "G1_B34X_OBSERVATION", CSharpG1RoslynPathR3, err)
	}
	src := in.Src
	src.Expected, src.Executed = nil, nil
	ledger, data, err := deriveCSharpG1(src)
	if err != nil {
		return c, err
	}
	all, err := g1rSourcesOf(ledger, data)
	if err != nil {
		return c, err
	}
	if (exp.Schema == CSharpG1B34SchemaR2) != in.R2 {
		return c, g1bxFault("G1_B34X_PIN")
	}
	c.r2 = in.R2
	c.exp, c.sources, c.accepted = exp, map[string]G1RoslynCaseSource{}, map[string]bool{}
	for _, s := range all {
		c.sources[s.ID] = s
	}
	for _, s := range obs.Sources {
		c.accepted[s.ID] = s.Accepted != nil && *s.Accepted
	}
	c.ids = CSharpG1B34NativeSourceIDs(exp)
	for _, id := range c.ids {
		if _, ok := c.sources[id]; !ok {
			return c, g1bxFault("G1_B34X_SOURCES")
		}
	}
	c.inputs = G1B34ExecInputs{
		Expected:    queryregistrycore.FilePin(map[bool]string{false: CSharpG1B34Path, true: CSharpG1B34PathR2}[in.R2], in.Expected),
		Observation: queryregistrycore.FilePin(CSharpG1RoslynPathR3, in.Observation),
	}
	// the expected asset pins the same observation and the same ledger sources
	if exp.Inputs.Observation != c.inputs.Observation || !g1Same(exp.Inputs.Ledger, ledger.Inputs) {
		return c, g1bxFault("G1_B34X_PIN")
	}
	return c, nil
}

// DeriveCSharpG1B34Executed compares the raw native observation with the sealed assets. A mismatch without an authored disposition is
// returned as is (the verifier rejects the asset).
func DeriveCSharpG1B34Executed(in CSharpG1B34ExecutedSources) (CSharpG1B34Executed, error) {
	c, err := g1bxContextOf(in)
	if err != nil {
		return CSharpG1B34Executed{}, err
	}
	return g1bxDerive(c, in)
}

func g1bxDerive(c g1bxContext, in CSharpG1B34ExecutedSources) (CSharpG1B34Executed, error) {
	var out CSharpG1B34Executed
	raw, err := DecodeCSharpG1NativeRaw(in.Raw)
	if err != nil {
		return out, err
	}
	trees := map[string]g1xTree{}
	for _, s := range raw.Sources {
		trees[s.ID] = g1xTree{src: s}
	}
	if len(raw.Sources) != len(c.ids) {
		return out, g1bxFault("G1_B34X_SOURCES")
	}
	for i, id := range c.ids {
		s, ok := trees[id]
		want := c.sources[id]
		if !ok || raw.Sources[i].ID != id || s.src.SHA256 != want.SHA256 || s.src.Bytes != len(want.Data) {
			return out, g1bxFault("G1_B34X_SOURCES")
		}
	}
	c.inputs.Raw = queryregistrycore.FilePin("csharp-g1-native-raw", in.Raw)
	out.Schema, out.Basis, out.Inputs, out.Native = CSharpG1B34ExecutedSchema, csharpG1B34ExecutedBasis, c.inputs, raw.Provenance
	if c.r2 {
		out.Schema, out.Basis = CSharpG1B34ExecutedSchemaR2, csharpG1B34ExecutedBasisR2
	}
	for _, id := range c.ids {
		s := trees[id].src
		out.Sources = append(out.Sources, G1ExecSource{ID: id, SHA256: s.SHA256, Bytes: s.Bytes, RoslynAccepted: c.accepted[id], Status: s.Status, Code: s.Code,
			NativeError: s.HasError, Nodes: len(s.Nodes), TreeDigest: s.TreeDigest})
	}
	for _, a := range c.exp.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		alt := G1B34ExecAlt{ID: a.ID, Row: a.Row, Group: a.Group, Basis: a.Basis, Discriminating: a.Discriminating, Tier: g1bxTier(a), Corrected: c.corrected(a.ID), Witnesses: []G1B34ExecWitness{}}
		for _, w := range a.Witnesses {
			parts, ok := g1bxParts(a, w)
			if !ok {
				return out, g1bxFault("G1_B34X_EXPECTATION")
			}
			ew := G1B34ExecWitness{Source: w.Source, Match: w.Match, Roslyn: w.Roslyn}
			for _, p := range parts {
				ew.Parts = append(ew.Parts, g1bxComparePart(p, trees[w.Source]))
			}
			ew.Result = g1bxWitnessResult(ew.Parts)
			alt.Witnesses = append(alt.Witnesses, ew)
		}
		alt.Result, alt.Executed = g1bxAltResult(alt.Witnesses)
		out.Alternatives = append(out.Alternatives, alt)
		if alt.Result == G1ResMismatch {
			d := c.dispositions()[a.ID]
			d.Alternative = a.ID
			if d.Cause == "" {
				d.Cause, d.Reason = "UNDISPOSED", ""
			}
			out.Dispositions = append(out.Dispositions, d)
		}
	}
	if out.Dispositions == nil {
		out.Dispositions = []G1ExecDisposition{}
	}
	out.Counts = g1bxCounts(out)
	return out, nil
}

// corrected reports whether a post-seal correction of the expected asset (r2 only) names the alternative.
func (c g1bxContext) corrected(id string) bool {
	return slices.ContainsFunc(c.exp.Corrections, func(x G1Correction) bool { return x.Item == id })
}

// dispositions are the authored causes of the mismatches of the revision: r2 has none (a mismatch of r2 is rejected as undisposed).
func (c g1bxContext) dispositions() map[string]G1ExecDisposition {
	if c.r2 {
		return nil
	}
	return g1B34ExecDispositions
}

func g1bxCounts(x CSharpG1B34Executed) G1B34ExecCounts {
	var n G1B34ExecCounts
	n.Alternatives, n.Sources = len(x.Alternatives), len(x.Sources)
	for _, s := range x.Sources {
		if s.Status == G1NativeCompleted {
			n.SourcesCompleted++
			if s.NativeError && s.RoslynAccepted {
				n.NativeErrorAccepted++
			}
		}
	}
	for _, a := range x.Alternatives {
		if a.Executed {
			n.Executed++
		}
		if a.Corrected {
			n.Corrected++
		}
		switch a.Result {
		case G1ResMatch:
			n.Match++
			if a.Corrected {
				n.CorrectedMatch++
			} else if x.Schema == CSharpG1B34ExecutedSchemaR2 {
				n.SealedMatch++
			}
			switch {
			case a.Tier == G1B34TierWord:
				n.MatchStrongA++
			case a.Tier == G1B34TierCandidate:
				n.MatchCandidateA++
			case a.Tier == G1B34TierOperator:
				n.MatchOperatorA++
			case a.Group == G1ClassLiteral:
				n.MatchWeakA++
			case a.Discriminating:
				n.MatchStrongB++
			default:
				n.MatchWeakB++
			}
		case G1ResMismatch:
			n.Mismatch++
		default:
			n.NotApplicable++
		}
		for _, w := range a.Witnesses {
			n.Witnesses++
			for _, p := range w.Parts {
				n.Parts++
				switch p.Result {
				case G1ResMatch:
					n.PartMatch++
				case G1ResMismatch:
					n.PartMismatch++
				}
			}
		}
	}
	return n
}

// DecodeCSharpG1B34Executed decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1B34Executed(data []byte) (CSharpG1B34Executed, error) {
	var x CSharpG1B34Executed
	if len(data) > csharpG1B34ExecutedMax {
		return x, fail(KindResourceLimit, "G1_B34X_SIZE", CSharpG1B34ExecutedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_B34X_JSON", CSharpG1B34ExecutedPath, err)
	}
	if x.Schema != CSharpG1B34ExecutedSchema && x.Schema != CSharpG1B34ExecutedSchemaR2 || x.Basis == "" {
		return x, g1bxFault("G1_B34X_SCHEMA")
	}
	return x, nil
}

// ParseCSharpG1B34Executed decodes the asset and checks it against the sealed assets. Without in.Raw the check is offline: pins, the
// alternatives, witnesses and parts against the sealed expected asset, every result recomputed from the recorded evidence, the source
// identities against the ledger's sources, dispositions, counts and the canonical bytes. With in.Raw the asset must also equal the
// comparison re-derived from the raw native observation, byte for byte.
func ParseCSharpG1B34Executed(data []byte, in CSharpG1B34ExecutedSources) (CSharpG1B34Executed, error) {
	x, err := DecodeCSharpG1B34Executed(data)
	if err != nil {
		return x, err
	}
	c, err := g1bxContextOf(in)
	if err != nil {
		return x, err
	}
	return x, g1bxCheck(data, x, c, in)
}

// g1bxCheck is ParseCSharpG1B34Executed after the decode and the context.
func g1bxCheck(data []byte, x CSharpG1B34Executed, c g1bxContext, in CSharpG1B34ExecutedSources) error {
	if wantSchema, wantBasis := map[bool][2]string{false: {CSharpG1B34ExecutedSchema, csharpG1B34ExecutedBasis}, true: {CSharpG1B34ExecutedSchemaR2, csharpG1B34ExecutedBasisR2}}[c.r2][0], map[bool]string{false: csharpG1B34ExecutedBasis, true: csharpG1B34ExecutedBasisR2}[c.r2]; x.Schema != wantSchema {
		return g1bxFault("G1_B34X_SCHEMA")
	} else if x.Basis != wantBasis {
		return g1bxFault("G1_B34X_BYTES")
	}
	if x.Inputs.Expected != c.inputs.Expected || x.Inputs.Observation != c.inputs.Observation {
		return g1bxFault("G1_B34X_PIN")
	}
	if len(x.Sources) != len(c.ids) {
		return g1bxFault("G1_B34X_SOURCES")
	}
	completed := map[string]bool{}
	for i, s := range x.Sources {
		want := c.sources[c.ids[i]]
		if s.ID != c.ids[i] || s.SHA256 != want.SHA256 || s.Bytes != len(want.Data) || s.RoslynAccepted != c.accepted[s.ID] {
			return g1bxFault("G1_B34X_SOURCES")
		}
		if s.Status == G1NativeCompleted {
			if s.Nodes <= 0 || s.TreeDigest == "" || s.Code != "" {
				return g1bxFault("G1_B34X_SOURCES")
			}
			completed[s.ID] = true
		} else if s.Nodes != 0 || s.TreeDigest != "" || s.NativeError || s.Code == "" {
			return g1bxFault("G1_B34X_SOURCES")
		}
	}
	var sealed []G1B34Alt
	for _, a := range c.exp.Alternatives {
		if a.Status == G1ExpExpected {
			sealed = append(sealed, a)
		}
	}
	if len(x.Alternatives) != len(sealed) {
		return g1bxFault("G1_B34X_ALTERNATIVES")
	}
	for i, a := range x.Alternatives {
		s := sealed[i]
		if a.ID != s.ID || a.Row != s.Row || a.Group != s.Group || a.Basis != s.Basis || a.Discriminating != s.Discriminating || a.Tier != g1bxTier(s) || a.Corrected != c.corrected(s.ID) || len(a.Witnesses) != len(s.Witnesses) {
			return g1bxFault("G1_B34X_EXPECTATION")
		}
		for j, w := range a.Witnesses {
			sw := s.Witnesses[j]
			parts, ok := g1bxParts(s, sw)
			if !ok || w.Source != sw.Source || w.Match != sw.Match || w.Roslyn != sw.Roslyn || len(w.Parts) != len(parts) {
				return g1bxFault("G1_B34X_EXPECTATION")
			}
			for k, p := range w.Parts {
				want := parts[k]
				if p.Start != want.Start || p.End != want.End || p.Text != want.Text || !slices.Equal(p.TS, want.TS) {
					return g1bxFault("G1_B34X_EXPECTATION")
				}
				if code := g1bxCheckPart(p, completed[w.Source]); code != "" {
					return g1bxFault(code)
				}
			}
			if g1bxWitnessResult(w.Parts) != w.Result {
				return g1bxFault("G1_B34X_RESULT")
			}
		}
		if res, ex := g1bxAltResult(a.Witnesses); res != a.Result || ex != a.Executed {
			return g1bxFault("G1_B34X_RESULT")
		}
	}
	var mismatches []string
	for _, a := range x.Alternatives {
		if a.Result == G1ResMismatch {
			mismatches = append(mismatches, a.ID)
		}
	}
	if len(x.Dispositions) != len(mismatches) {
		return g1bxFault("G1_B34X_UNDISPOSED")
	}
	for i, d := range x.Dispositions {
		auth, ok := c.dispositions()[mismatches[i]]
		if d.Alternative != mismatches[i] || !ok || d.Cause != auth.Cause || d.Reason != auth.Reason || !slices.Contains([]string{G1CauseGrammar, G1CauseKindMap, G1CauseExpected, G1CauseDesign}, d.Cause) || d.Reason == "" {
			return g1bxFault("G1_B34X_UNDISPOSED")
		}
	}
	if x.Counts != g1bxCounts(x) {
		return g1bxFault("G1_B34X_COUNTS")
	}
	if x.Native.Route != "csharp" || x.Native.NativeMemory != 1<<30 || x.Native.NativeWallMS != 30000 || x.Native.BuildMemory != 4<<30 || x.Native.BuildWallMS != 120000 || len(x.Native.Grammar) == 0 || x.Inputs.Raw.SHA256 == "" {
		return g1bxFault("G1_B34X_NATIVE")
	}
	canonical, err := MarshalCSharpG1B34Executed(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return g1bxFault("G1_B34X_BYTES")
	}
	if in.Raw != nil {
		want, err := DeriveCSharpG1B34Executed(in)
		if err != nil {
			return err
		}
		if !g1eSame(x, want) {
			return g1bxFault("G1_B34X_RAW")
		}
	}
	return nil
}

// g1bxCheckPart verifies a recorded part without the tree: the evidence has exactly the span of the part, the nodes of a sealed type come
// first, and the result follows from the evidence.
func g1bxCheckPart(p G1B34ExecPart, completed bool) string {
	if !completed {
		if p.Result != G1ResNotApplicable || len(p.Found) != 0 || len(p.Near) != 0 {
			return "G1_B34X_RESULT"
		}
		return ""
	}
	if len(p.Found) > g1ExecFoundLimit || len(p.Near) > g1ExecFoundLimit {
		return "G1_B34X_EVIDENCE"
	}
	for _, f := range p.Found {
		if f.Start != p.Start || f.End != p.End || f.Type == "" {
			return "G1_B34X_EVIDENCE"
		}
	}
	for _, f := range p.Near {
		if f.Start >= p.End || f.End <= p.Start || f.End-f.Start > max(p.End-p.Start, 1) || f.Type == "" {
			return "G1_B34X_EVIDENCE"
		}
	}
	res := G1ResMismatch
	for _, f := range p.Found {
		if slices.Contains(p.TS, f.Type) {
			res = G1ResMatch
			break
		}
	}
	if res != p.Result {
		return "G1_B34X_RESULT"
	}
	// the nodes of a sealed type come first, so a cut list never hides a match; near is for mismatches
	if p.Result == G1ResMatch && !slices.Contains(p.TS, p.Found[0].Type) || p.Result != G1ResMismatch && len(p.Near) != 0 {
		return "G1_B34X_EVIDENCE"
	}
	return ""
}

// MarshalCSharpG1B34Executed writes the canonical asset: one line per source, alternative and disposition.
func MarshalCSharpG1B34Executed(x CSharpG1B34Executed) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	scalar := func(name string, v any) {
		fmt.Fprintf(&b, " %q: ", name)
		put(v)
		b.WriteString(",\n")
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
	scalar("schema", x.Schema)
	scalar("inputs", x.Inputs)
	scalar("basis", x.Basis)
	scalar("native", x.Native)
	scalar("counts", x.Counts)
	list("sources", len(x.Sources), func(i int) any { return x.Sources[i] }, false)
	list("alternatives", len(x.Alternatives), func(i int) any { return x.Alternatives[i] }, false)
	list("dispositions", len(x.Dispositions), func(i int) any { return x.Dispositions[i] }, true)
	b.WriteString("}\n")
	if strings.Contains(b.String(), "\r") {
		return nil, g1bxFault("G1_B34X_BYTES")
	}
	return b.Bytes(), werr
}
