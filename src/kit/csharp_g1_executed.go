package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	jsonv2 "encoding/json/v2"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// G1 native comparison of the sealed lexical and type-name expectations (#233 G1-3a, native step).
//
// The sealed expected asset (csharp_g1_expected.go) and the kind map are read-only inputs. For every
// witness of every EXPECTED alternative the comparison looks at the native tree-sitter-c-sharp tree of
// the witness's source and classifies the witness, then the alternative:
//
//   - MATCH: a native node whose type is one of the alternative's sealed types has the witness's span
//     (match "span") or starts at the witness's start (match "starts-at").
//   - MISMATCH: native completed and no such node exists.
//   - NOT_APPLICABLE: the native parse of the witness's source did not complete, so there is nothing to
//     compare.
//
// An alternative is MISMATCH when any witness is, else MATCH when any witness is, else NOT_APPLICABLE;
// only an alternative whose comparison ran (it has a completed native witness) is EXECUTED. A mismatch
// is never repaired: it needs a disposition with a cause and the expectation stays as sealed.
// Nothing in this file can write an expectation, the kind map or the observation.

const (
	CSharpG1ExecutedSchema = "tsgk-csharp-g1-executed/r1"
	CSharpG1ExecutedPath   = "src/contracts/csharp-g1-executed-b01b02-r1.json"

	CSharpG1ExecutedSchemaR2 = "tsgk-csharp-g1-executed/r2"
	CSharpG1ExecutedPathR2   = "src/contracts/csharp-g1-executed-b01b02-r2.json"
	// shortcut: the asset is far below this today; raise only with a new revision.
	csharpG1ExecutedMax = 4 << 20

	G1ResMatch         = "MATCH"
	G1ResMismatch      = "MISMATCH"
	G1ResNotApplicable = "NOT_APPLICABLE"

	// causes of a disposed mismatch
	G1CauseGrammar  = "GRAMMAR_DEFECT"
	G1CauseKindMap  = "KIND_MAP_ERROR"
	G1CauseExpected = "EXPECTED_ERROR"
	G1CauseDesign   = "DESIGN_DIFFERENCE"

	g1ExecFoundLimit = 8
)

// G1ExecFound is a native node: its type (anonymous types quoted like the kind map) and span.
type G1ExecFound struct {
	Type  string `json:"type"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// G1ExecWitness repeats the sealed witness and records the native comparison. Found are the native
// nodes in the sealed relation to the witness (same span, or same start), the nodes of a sealed
// type first, at most g1ExecFoundLimit. Near (MISMATCH only) are the native nodes that overlap the
// span and are no longer than it. Map says whether a found type is one the kind map lists for the
// witness's Roslyn kind (IN_MAP), none is (OUTSIDE_MAP), or the kind map has no type for it (NO_MAP); it
// is FRAGMENT when the witness is a part of a Roslyn token or node of that kind, not the whole one.
type G1ExecWitness struct {
	Source string        `json:"source"`
	Start  int           `json:"start"`
	End    int           `json:"end"`
	Match  string        `json:"match"`
	Roslyn string        `json:"roslyn,omitempty"`
	Result string        `json:"result"`
	Found  []G1ExecFound `json:"found"`
	Near   []G1ExecFound `json:"near,omitempty"`
	Map    string        `json:"map,omitempty"`
}

type G1ExecAlt struct {
	ID        string          `json:"id"`
	Row       string          `json:"row"`
	Basis     string          `json:"basis"`
	TS        []string        `json:"ts"`
	Executed  bool            `json:"executed"`
	Result    string          `json:"result"`
	Witnesses []G1ExecWitness `json:"witnesses"`
}

// G1ExecSource is one native-parsed source version: its identity, whether Roslyn accepted it, and the
// native status, error flag, node count and digest of the compact node list.
type G1ExecSource struct {
	ID             string `json:"id"`
	SHA256         string `json:"sha256"`
	Bytes          int    `json:"bytes"`
	RoslynAccepted bool   `json:"roslyn_accepted"`
	Status         string `json:"status"`
	Code           string `json:"code,omitempty"`
	NativeError    bool   `json:"native_error"`
	Nodes          int    `json:"nodes"`
	TreeDigest     string `json:"tree_digest"`
}

// G1ExecDisposition is the authored cause of a mismatching alternative.
type G1ExecDisposition struct {
	Alternative string `json:"alternative"`
	Cause       string `json:"cause"`
	Reason      string `json:"reason"`
}

type G1ExecInputs struct {
	Expected    queryregistrycore.Pin `json:"expected"`
	KindMap     queryregistrycore.Pin `json:"kind_map"`
	Observation queryregistrycore.Pin `json:"observation"`
	Raw         queryregistrycore.Pin `json:"raw_native"`
}

type G1ExecCounts struct {
	Alternatives         int `json:"alternatives"`
	Executed             int `json:"executed"`
	Match                int `json:"match"`
	Mismatch             int `json:"mismatch"`
	NotApplicable        int `json:"not_applicable"`
	Witnesses            int `json:"witnesses"`
	WitnessMatch         int `json:"witness_match"`
	WitnessMismatch      int `json:"witness_mismatch"`
	WitnessNotApplicable int `json:"witness_not_applicable"`
	MapInside            int `json:"map_inside"`
	MapOutside           int `json:"map_outside"`
	MapFragment          int `json:"map_fragment"`
	Sources              int `json:"sources"`
	SourcesCompleted     int `json:"sources_completed"`
	NativeErrorAccepted  int `json:"native_error_roslyn_accepted"`
	// r2 only: the alternatives a correction left as sealed that match, the corrected ones, and those that match.
	SealedMatch    int `json:"sealed_match,omitzero"`
	Corrected      int `json:"corrected,omitzero"`
	CorrectedMatch int `json:"corrected_match,omitzero"`
}

type CSharpG1Executed struct {
	Schema       string              `json:"schema"`
	Inputs       G1ExecInputs        `json:"inputs"`
	Basis        string              `json:"basis"`
	Native       G1NativeProvenance  `json:"native"`
	Counts       G1ExecCounts        `json:"counts"`
	Sources      []G1ExecSource      `json:"sources"`
	Alternatives []G1ExecAlt         `json:"alternatives"`
	Dispositions []G1ExecDisposition `json:"dispositions"`
}

const csharpG1ExecutedBasis = "Native comparison of the sealed expected asset for csharp-B01 and csharp-B02. For every EXPECTED alternative and each of its witnesses (copied unchanged from the sealed asset) the tree-sitter-c-sharp tree of the witness's source, parsed by the registered native build (provenance in native; raw_native pins the local raw observation), is searched for a node of one of the alternative's sealed types with the witness's span (match span) or start (match starts-at): MATCH, else MISMATCH, or NOT_APPLICABLE when the native parse did not complete. An alternative is MISMATCH if any witness is, MATCH if some witness is and none mismatches, NOT_APPLICABLE otherwise, and EXECUTED when a completed native witness was compared. Each mismatch has a disposition with a cause: GRAMMAR_DEFECT, KIND_MAP_ERROR, EXPECTED_ERROR or DESIGN_DIFFERENCE. Expectations, the kind map and the observation are not changed by this asset; a sealed value that the native result contradicts stays as sealed and is reported. The comparison covers the witnesses only: it does not say the grammar parses every construct of an alternative, and a MATCH is a property of the pinned Roslyn observation's spans on these sources."

// g1xRev is one revision of the executed asset: it compares the expected asset and kind map of the same revision.
type g1xRev struct {
	schema, path, expectedPath, kindMapPath, basis string
	dispositions                                   map[string]G1ExecDisposition
}

const csharpG1ExecutedBasisR2 = csharpG1ExecutedBasis + " Revision r2 compares the r2 expected asset and kind map, which correct values after the r1 native result was seen (after_native corrections: four from the native comparison itself and eleven from the audit of the differential-oracle alternatives, #233 G1AUDIT76); the r1 executed asset stays as sealed. The counts sealed_match, corrected and corrected_match keep the uncorrected comparisons and the post-native corrections apart."

var g1xRevisions = map[string]g1xRev{
	CSharpG1ExpectedSchema:   {CSharpG1ExecutedSchema, CSharpG1ExecutedPath, CSharpG1ExpectedPath, CSharpG1KindMapPath, csharpG1ExecutedBasis, g1ExecDispositions},
	CSharpG1ExpectedSchemaR2: {CSharpG1ExecutedSchemaR2, CSharpG1ExecutedPathR2, CSharpG1ExpectedPathR2, CSharpG1KindMapPathR2, csharpG1ExecutedBasisR2, g1ExecDispositionsR2},
}

func g1xFault(code string) error { return fail(KindInvalidInput, code, CSharpG1ExecutedPath, nil) }

// g1ExecDispositions are the authored causes of the mismatches, keyed by alternative. They are written
// after the first native comparison and give the evidence; the sealed assets are not changed.
var g1ExecDispositions = map[string]G1ExecDisposition{
	"csharp-B01.a335": {Cause: G1CauseExpected, Reason: "The sealed value says the conditional symbol of #define and #undef is a tree-sitter identifier, but the pinned grammar text defines preproc_define and preproc_undef with the whole symbol as one preproc_arg token (grammar.js, preproc_define and preproc_undef), and native gives preproc_arg for [8,13) of alt-csharp-B01-10#0. The sealed types were taken from the use of the same production in #if and #elif conditions, where the grammar does parse identifiers."},
	"csharp-B01.a383": {Cause: G1CauseKindMap, Reason: "The kind map maps the Roslyn token InterpolatedStringStartToken, the two characters $\", to the single type interpolation_start of the same extent. The pinned grammar text builds the start from two nodes (interpolated_string_expression: alias(interpolation_regular_start, interpolation_start) then alias(interpolation_start_quote, '\"')), and native gives interpolation_start [66,67) and \" [67,68) in alt-csharp-B01-09#0, so no node has the Roslyn token's extent. The sealed value inherits the entry. The native tree is the grammar's own representation and has no error."},
	"csharp-B01.a396": {Cause: G1CauseKindMap, Reason: "Same entry as csharp-B01.a383: the Roslyn token InterpolatedVerbatimStringStartToken ($@\") spans the interpolation_start node [141,143) and the quote node [143,144) in alt-csharp-B01-09#0, which the kind map lists as one type of the whole extent."},
	"csharp-B01.a397": {Cause: G1CauseKindMap, Reason: "Same entry as csharp-B01.a383: the Roslyn token InterpolatedVerbatimStringStartToken (@$\") spans an interpolation_start node and a quote node in alt-csharp-B01-09#0, which the kind map lists as one type of the whole extent."},
}

// g1ExecDispositionsR2 are the authored causes of the r2 mismatches. None: after the corrections the
// native result contradicts no r2 value. A mismatch without an entry is rejected.
var g1ExecDispositionsR2 = map[string]G1ExecDisposition{}

// ---- native index ----------------------------------------------------------------------------

type g1xTree struct {
	src   G1NativeSource
	types []string
}

func (t g1xTree) completed() bool { return t.src.Status == G1NativeCompleted }

func (t g1xTree) node(i int) G1ExecFound {
	n := t.src.Nodes[i]
	return G1ExecFound{Type: t.src.Types[n[0]], Start: n[2], End: n[3]}
}

// g1xCompare compares one witness with the native tree of its source.
func g1xCompare(w G1ExpWitness, ts []string, c g1xContext, t g1xTree) G1ExecWitness {
	out := G1ExecWitness{Source: w.Source, Start: w.Start, End: w.End, Match: w.Match, Roslyn: w.Roslyn, Found: []G1ExecFound{}}
	if !t.completed() {
		out.Result = G1ResNotApplicable
		return out
	}
	var hit, other []G1ExecFound
	for i := range t.src.Nodes {
		n := t.node(i)
		related := n.Start == w.Start && (w.Match == "starts-at" || n.End == w.End)
		if !related {
			continue
		}
		if slices.Contains(ts, n.Type) {
			hit = append(hit, n)
		} else {
			other = append(other, n)
		}
	}
	out.Found = append([]G1ExecFound{}, slices.Concat(hit, other)...)
	if len(out.Found) > g1ExecFoundLimit {
		out.Found = out.Found[:g1ExecFoundLimit]
	}
	if len(hit) > 0 {
		out.Result = G1ResMatch
	} else {
		out.Result = G1ResMismatch
		for i := range t.src.Nodes {
			n := t.node(i)
			if n.Start < w.End && n.End > w.Start && n.End-n.Start <= max(w.End-w.Start, 1) && len(out.Near) < g1ExecFoundLimit {
				out.Near = append(out.Near, n)
			}
		}
	}
	out.Map = g1xMap(w.Roslyn, c.whole(w), c.kinds, out.Found)
	return out
}

func g1xMap(roslyn string, whole bool, kinds map[string][]string, found []G1ExecFound) string {
	if roslyn == "" {
		return ""
	}
	if !whole {
		return "FRAGMENT"
	}
	listed := kinds[roslyn]
	if len(listed) == 0 {
		return "NO_MAP"
	}
	for _, f := range found {
		if slices.Contains(listed, f.Type) {
			return "IN_MAP"
		}
	}
	return "OUTSIDE_MAP"
}

// g1xJudgeFound recomputes a witness's result from the recorded evidence alone (the offline check).
func g1xJudgeFound(w G1ExecWitness, ts []string, completed bool) string {
	if !completed {
		return G1ResNotApplicable
	}
	for _, f := range w.Found {
		if slices.Contains(ts, f.Type) {
			return G1ResMatch
		}
	}
	return G1ResMismatch
}

// g1xAltResult aggregates witness results into the alternative's result.
func g1xAltResult(ws []G1ExecWitness) (result string, executed bool) {
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

// ---- derivation -------------------------------------------------------------------------------

// CSharpG1ExecutedSources are the inputs of the comparison. Raw is the local raw native observation;
// the offline verification runs without it.
type CSharpG1ExecutedSources struct {
	Src         CSharpG1Sources // the pinned declarations; Expected and Executed are ignored
	Expected    []byte
	KindMap     []byte
	Observation []byte
	Raw         []byte
}

type g1xContext struct {
	exp      CSharpG1Expected
	kinds    map[string][]string
	spans    map[string]bool // "<source>|<roslyn kind>|<start>|<end>" of every token and node the observation holds
	ledger   CSharpG1Ledger
	sources  map[string]G1RoslynCaseSource
	accepted map[string]bool
	ids      []string
	inputs   G1ExecInputs
	rev      g1xRev
	// corrected lists the alternatives a post-native correction changed (r2); nil in r1
	corrected map[string]bool
}

func g1xSpanKey(source, kind string, start, end int) string {
	return fmt.Sprintf("%s|%s|%d|%d", source, kind, start, end)
}

// whole reports whether the witness is a whole Roslyn token or node of its kind, not a fragment of one.
func (c g1xContext) whole(w G1ExpWitness) bool {
	return c.spans[g1xSpanKey(w.Source, w.Roslyn, w.Start, w.End)]
}

func g1xContextOf(in CSharpG1ExecutedSources) (g1xContext, error) {
	var c g1xContext
	exp, err := DecodeCSharpG1Expected(in.Expected)
	if err != nil {
		return c, err
	}
	var obs CSharpG1RoslynObserved
	if err := jsonv2.Unmarshal(in.Observation, &obs, jsonv2.RejectUnknownMembers(true)); err != nil {
		return c, fail(KindInvalidInput, "G1_EXECUTED_OBSERVATION", CSharpG1RoslynPath, err)
	}
	km, err := ParseCSharpG1KindMap(in.KindMap, obs.Kinds)
	if err != nil {
		return c, err
	}
	// the revision follows the expected asset; the kind map must be of the same revision
	rev, ok := g1xRevisions[exp.Schema]
	if !ok || km.Schema != map[string]string{CSharpG1ExpectedSchema: CSharpG1KindMapSchema, CSharpG1ExpectedSchemaR2: CSharpG1KindMapSchemaR2}[exp.Schema] {
		return c, g1xFault("G1_EXECUTED_PIN")
	}
	c.rev = rev
	if exp.Schema == CSharpG1ExpectedSchemaR2 {
		c.corrected = map[string]bool{}
		for _, k := range exp.Corrections {
			c.corrected[k.Item] = true
		}
	}
	src := in.Src
	src.Expected, src.Executed = nil, nil
	ledger, err := DeriveCSharpG1Ledger(src)
	if err != nil {
		return c, err
	}
	all, err := CSharpG1RoslynSources(src)
	if err != nil {
		return c, err
	}
	c.exp, c.ledger, c.sources = exp, ledger, map[string]G1RoslynCaseSource{}
	for _, s := range all {
		c.sources[s.ID] = s
	}
	c.kinds = map[string][]string{}
	for _, k := range km.Kinds {
		c.kinds[k.Roslyn] = k.TS
	}
	c.accepted, c.spans = map[string]bool{}, map[string]bool{}
	for _, s := range obs.Sources {
		c.accepted[s.ID] = s.Accepted != nil && *s.Accepted
		for _, list := range [][][3]int{s.Tokens, s.Nodes} {
			for _, t := range list {
				if t[0] >= 0 && t[0] < len(obs.Kinds) {
					c.spans[g1xSpanKey(s.ID, obs.Kinds[t[0]], t[1], t[2])] = true
				}
			}
		}
	}
	c.ids = CSharpG1NativeSourceIDs(ledger, exp)
	for _, id := range c.ids {
		if _, ok := c.sources[id]; !ok {
			return c, g1xFault("G1_EXECUTED_SOURCES")
		}
	}
	c.inputs = G1ExecInputs{
		Expected:    queryregistrycore.FilePin(rev.expectedPath, in.Expected),
		KindMap:     queryregistrycore.FilePin(rev.kindMapPath, in.KindMap),
		Observation: queryregistrycore.FilePin(CSharpG1RoslynPath, in.Observation),
	}
	// the expected asset pins the same kind map and observation
	if exp.Inputs.KindMap != c.inputs.KindMap || exp.Inputs.Observation != c.inputs.Observation {
		return c, g1xFault("G1_EXECUTED_PIN")
	}
	return c, nil
}

// DeriveCSharpG1Executed compares the raw native observation with the sealed assets. A mismatch
// without an authored disposition is returned as is (the verifier rejects the asset).
func DeriveCSharpG1Executed(in CSharpG1ExecutedSources) (CSharpG1Executed, error) {
	var out CSharpG1Executed
	c, err := g1xContextOf(in)
	if err != nil {
		return out, err
	}
	raw, err := DecodeCSharpG1NativeRaw(in.Raw)
	if err != nil {
		return out, err
	}
	trees := map[string]g1xTree{}
	for _, s := range raw.Sources {
		trees[s.ID] = g1xTree{src: s}
	}
	if len(raw.Sources) != len(c.ids) {
		return out, g1xFault("G1_EXECUTED_SOURCES")
	}
	for i, id := range c.ids {
		s, ok := trees[id]
		want := c.sources[id]
		if !ok || raw.Sources[i].ID != id || s.src.SHA256 != want.SHA256 || s.src.Bytes != len(want.Data) {
			return out, g1xFault("G1_EXECUTED_SOURCES")
		}
	}
	c.inputs.Raw = queryregistrycore.FilePin("csharp-g1-native-raw", in.Raw)
	out.Schema, out.Basis, out.Inputs, out.Native = c.rev.schema, c.rev.basis, c.inputs, raw.Provenance
	for _, id := range c.ids {
		s := trees[id].src
		out.Sources = append(out.Sources, G1ExecSource{ID: id, SHA256: s.SHA256, Bytes: s.Bytes, RoslynAccepted: c.accepted[id], Status: s.Status, Code: s.Code,
			NativeError: s.HasError, Nodes: len(s.Nodes), TreeDigest: s.TreeDigest})
	}
	for _, a := range c.exp.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		alt := G1ExecAlt{ID: a.ID, Row: a.Row, Basis: a.Basis, TS: a.TS, Witnesses: []G1ExecWitness{}}
		for _, w := range a.Witnesses {
			alt.Witnesses = append(alt.Witnesses, g1xCompare(w, a.TS, c, trees[w.Source]))
		}
		alt.Result, alt.Executed = g1xAltResult(alt.Witnesses)
		out.Alternatives = append(out.Alternatives, alt)
		if alt.Result == G1ResMismatch {
			d := c.rev.dispositions[a.ID]
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
	out.Counts = g1xCounts(out, c.corrected)
	return out, nil
}

func g1xCounts(x CSharpG1Executed, corrected map[string]bool) G1ExecCounts {
	var n G1ExecCounts
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
		if corrected[a.ID] {
			n.Corrected++
			if a.Result == G1ResMatch {
				n.CorrectedMatch++
			}
		}
		switch a.Result {
		case G1ResMatch:
			n.Match++
			if corrected != nil && !corrected[a.ID] {
				n.SealedMatch++
			}
		case G1ResMismatch:
			n.Mismatch++
		default:
			n.NotApplicable++
		}
		for _, w := range a.Witnesses {
			n.Witnesses++
			switch w.Result {
			case G1ResMatch:
				n.WitnessMatch++
			case G1ResMismatch:
				n.WitnessMismatch++
			default:
				n.WitnessNotApplicable++
			}
			switch w.Map {
			case "IN_MAP":
				n.MapInside++
			case "OUTSIDE_MAP":
				n.MapOutside++
			case "FRAGMENT":
				n.MapFragment++
			}
		}
	}
	return n
}

// ---- verification -----------------------------------------------------------------------------

// DecodeCSharpG1Executed decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1Executed(data []byte) (CSharpG1Executed, error) {
	var x CSharpG1Executed
	if len(data) > csharpG1ExecutedMax {
		return x, fail(KindResourceLimit, "G1_EXECUTED_SIZE", CSharpG1ExecutedPath, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_EXECUTED_JSON", CSharpG1ExecutedPath, err)
	}
	if x.Schema != CSharpG1ExecutedSchema && x.Schema != CSharpG1ExecutedSchemaR2 || x.Basis == "" {
		return x, g1xFault("G1_EXECUTED_SCHEMA")
	}
	return x, nil
}

// ParseCSharpG1Executed decodes the asset and checks it against the sealed assets. Without in.Raw the
// check is offline: pins, the alternatives and witnesses against the sealed expected asset, every
// result recomputed from the recorded evidence, the source identities against the ledger's sources,
// dispositions, counts and the canonical bytes. With in.Raw the asset must also equal the comparison
// re-derived from the raw native observation, byte for byte.
func ParseCSharpG1Executed(data []byte, in CSharpG1ExecutedSources) (CSharpG1Executed, error) {
	x, err := DecodeCSharpG1Executed(data)
	if err != nil {
		return x, err
	}
	c, err := g1xContextOf(in)
	if err != nil {
		return x, err
	}
	return x, g1xCheck(data, x, c, in)
}

// g1xCheck is ParseCSharpG1Executed after the decode and the context.
func g1xCheck(data []byte, x CSharpG1Executed, c g1xContext, in CSharpG1ExecutedSources) error {
	if x.Schema != c.rev.schema {
		return g1xFault("G1_EXECUTED_SCHEMA")
	}
	if x.Basis != c.rev.basis {
		return g1xFault("G1_EXECUTED_BYTES")
	}
	if x.Inputs.Expected != c.inputs.Expected || x.Inputs.KindMap != c.inputs.KindMap || x.Inputs.Observation != c.inputs.Observation {
		return g1xFault("G1_EXECUTED_PIN")
	}
	// the sources: exactly the needed versions, in order, with the ledger's identities
	if len(x.Sources) != len(c.ids) {
		return g1xFault("G1_EXECUTED_SOURCES")
	}
	completed := map[string]bool{}
	for i, s := range x.Sources {
		want := c.sources[c.ids[i]]
		if s.ID != c.ids[i] || s.SHA256 != want.SHA256 || s.Bytes != len(want.Data) || s.RoslynAccepted != c.accepted[s.ID] {
			return g1xFault("G1_EXECUTED_SOURCES")
		}
		if s.Status == G1NativeCompleted {
			if s.Nodes <= 0 || s.TreeDigest == "" || s.Code != "" {
				return g1xFault("G1_EXECUTED_SOURCES")
			}
			completed[s.ID] = true
		} else if s.Nodes != 0 || s.TreeDigest != "" || s.NativeError || s.Code == "" {
			return g1xFault("G1_EXECUTED_SOURCES")
		}
	}
	// the alternatives and witnesses are the sealed ones, unchanged
	var sealed []G1ExpAlt
	for _, a := range c.exp.Alternatives {
		if a.Status == G1ExpExpected {
			sealed = append(sealed, a)
		}
	}
	if len(x.Alternatives) != len(sealed) {
		return g1xFault("G1_EXECUTED_ALTERNATIVES")
	}
	for i, a := range x.Alternatives {
		s := sealed[i]
		if a.ID != s.ID || a.Row != s.Row || a.Basis != s.Basis || !slices.Equal(a.TS, s.TS) || len(a.Witnesses) != len(s.Witnesses) {
			return g1xFault("G1_EXECUTED_EXPECTATION")
		}
		for j, w := range a.Witnesses {
			sw := s.Witnesses[j]
			if w.Source != sw.Source || w.Start != sw.Start || w.End != sw.End || w.Match != sw.Match || w.Roslyn != sw.Roslyn {
				return g1xFault("G1_EXECUTED_EXPECTATION")
			}
			if code := g1xCheckWitness(w, s.TS, c, completed[w.Source]); code != "" {
				return g1xFault(code)
			}
		}
		if res, ex := g1xAltResult(a.Witnesses); res != a.Result || ex != a.Executed {
			return g1xFault("G1_EXECUTED_RESULT")
		}
	}
	// dispositions: one per mismatching alternative, authored, with a known cause
	var mismatches []string
	for _, a := range x.Alternatives {
		if a.Result == G1ResMismatch {
			mismatches = append(mismatches, a.ID)
		}
	}
	if len(x.Dispositions) != len(mismatches) {
		return g1xFault("G1_EXECUTED_UNDISPOSED")
	}
	for i, d := range x.Dispositions {
		auth, ok := c.rev.dispositions[mismatches[i]]
		if d.Alternative != mismatches[i] || !ok || d.Cause != auth.Cause || d.Reason != auth.Reason || !slices.Contains([]string{G1CauseGrammar, G1CauseKindMap, G1CauseExpected, G1CauseDesign}, d.Cause) || d.Reason == "" {
			return g1xFault("G1_EXECUTED_UNDISPOSED")
		}
	}
	if x.Counts != g1xCounts(x, c.corrected) {
		return g1xFault("G1_EXECUTED_COUNTS")
	}
	if x.Native.Route != "csharp" || x.Native.NativeMemory != 1<<30 || x.Native.NativeWallMS != 30000 || x.Native.BuildMemory != 4<<30 || x.Native.BuildWallMS != 120000 || len(x.Native.Grammar) == 0 || x.Inputs.Raw.SHA256 == "" {
		return g1xFault("G1_EXECUTED_NATIVE")
	}
	canonical, err := MarshalCSharpG1Executed(x)
	if err != nil || !bytes.Equal(canonical, data) {
		return g1xFault("G1_EXECUTED_BYTES")
	}
	if in.Raw != nil {
		want, err := DeriveCSharpG1Executed(in)
		if err != nil {
			return err
		}
		if !g1eSame(x, want) {
			return g1xFault("G1_EXECUTED_RAW")
		}
	}
	return nil
}

// g1xCheckWitness verifies a recorded witness without the tree: the evidence is in the sealed relation
// to the witness, the result follows from it, and the map label follows from the kind map.
func g1xCheckWitness(w G1ExecWitness, ts []string, c g1xContext, completed bool) string {
	if !completed {
		if w.Result != G1ResNotApplicable || len(w.Found) != 0 || len(w.Near) != 0 || w.Map != "" {
			return "G1_EXECUTED_RESULT"
		}
		return ""
	}
	if len(w.Found) > g1ExecFoundLimit || len(w.Near) > g1ExecFoundLimit {
		return "G1_EXECUTED_EVIDENCE"
	}
	for _, f := range w.Found {
		if f.Start != w.Start || w.Match == "span" && f.End != w.End || f.End < f.Start || f.Type == "" {
			return "G1_EXECUTED_EVIDENCE"
		}
	}
	for _, f := range w.Near {
		if f.Start >= w.End || f.End <= w.Start || f.End-f.Start > max(w.End-w.Start, 1) || f.Type == "" {
			return "G1_EXECUTED_EVIDENCE"
		}
	}
	if g1xJudgeFound(w, ts, true) != w.Result {
		return "G1_EXECUTED_RESULT"
	}
	// the nodes of a sealed type come first, so a cut list never hides a match; near is for mismatches
	if w.Result == G1ResMatch && !slices.Contains(ts, w.Found[0].Type) || w.Result != G1ResMismatch && len(w.Near) != 0 {
		return "G1_EXECUTED_EVIDENCE"
	}
	if w.Map != g1xMap(w.Roslyn, c.whole(G1ExpWitness{Source: w.Source, Start: w.Start, End: w.End, Roslyn: w.Roslyn}), c.kinds, w.Found) {
		return "G1_EXECUTED_RESULT"
	}
	return ""
}

// MarshalCSharpG1Executed writes the canonical asset: one line per source, alternative and disposition.
func MarshalCSharpG1Executed(x CSharpG1Executed) ([]byte, error) {
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
		return nil, g1xFault("G1_EXECUTED_BYTES")
	}
	return b.Bytes(), werr
}

// ExecutedIDs lists the alternatives of the asset whose comparison ran, in asset order.
func (x CSharpG1Executed) ExecutedIDs() []string {
	var out []string
	for _, a := range x.Alternatives {
		if a.Executed {
			out = append(out, a.ID)
		}
	}
	return out
}
