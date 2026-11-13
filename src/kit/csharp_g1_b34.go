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

// G1 expected values of the production rows csharp-B03 (257 alternatives) and csharp-B04 (131) (#233, G1-3b PR2).
//
// The classification (csharp_g1_classify.go) sorts the alternatives into A (a literal anchor), B (a Roslyn structure) and C
// (blocked). This file turns each into an expectation, without any native output:
//
//   - A, basis spec-derived: the anchors are the keyword and punctuation tokens the production requires. The witness is the tightest
//     ordered run of those tokens in an accepted source of a linked case, one span per anchor. The expectation is that a node of
//     the anchor's types spans exactly that token (decision D-1: the node that holds the anchors is not asserted). The types of an
//     anchor are the kind map's types for the Roslyn token kind observed there.
//   - B, basis differential-oracle: the witness is the first node of the classification's Roslyn kinds (under the parent kind, if
//     the rule names one) in an accepted source of a linked case; the expectation is a node of the kind map's types for those kinds
//     with exactly that span.
//   - C: BLOCKED with the classification's reason code.
//
// Every expectation carries `discriminating`: false is a weak match (D-1 and R-5): an anchor of single punctuation characters, a
// type that other Roslyn kinds share, or a type set that holds `identifier` or five or more types. A weak match proves less.

const (
	CSharpG1B34Schema = "tsgk-csharp-g1-expected-b03b04/r1"
	CSharpG1B34Path   = "src/contracts/csharp-g1-expected-b03b04-r1.json"
	// shortcut: the asset is far below this today; raise only with a new revision.
	csharpG1B34Max = 4 << 20

	// CSharpG1B34KindMapPath is the kind map the expected types come from.
	CSharpG1B34KindMapPath = CSharpG1KindMapPathR3

	g1b34MatchSpan   = "span"
	g1b34MatchTokens = "tokens"
)

// G1B34Anchor is a keyword or punctuation token a production requires, with the node types the kind map gives its Roslyn kind.
type G1B34Anchor struct {
	Text string   `json:"text"`
	TS   []string `json:"ts"`
}

// G1B34Witness locates an alternative in a source. B: Span is the node's [start,end) and Roslyn its kind. A: Tokens are the spans
// of the anchors in order.
type G1B34Witness struct {
	Source string   `json:"source"`
	Match  string   `json:"match"`
	Span   []int    `json:"span,omitempty"`
	Roslyn string   `json:"roslyn,omitempty"`
	Tokens [][2]int `json:"tokens,omitempty"`
}

type G1B34Alt struct {
	ID             string         `json:"id"`
	Row            string         `json:"row"`
	Group          string         `json:"group"`
	Status         string         `json:"status"`
	Basis          string         `json:"basis,omitempty"`
	Discriminating bool           `json:"discriminating"`
	Form           string         `json:"form,omitempty"`
	Spec           string         `json:"spec"`
	Anchors        []G1B34Anchor  `json:"anchors,omitempty"`
	Kinds          []string       `json:"kinds,omitempty"`
	Parent         []string       `json:"parent,omitempty"`
	TS             []string       `json:"ts,omitempty"`
	Witnesses      []G1B34Witness `json:"witnesses"`
	Block          string         `json:"block,omitempty"`
	Reason         string         `json:"reason,omitempty"`
}

type G1B34Inputs struct {
	Ledger      CSharpG1Inputs        `json:"ledger_sources"`
	Observation queryregistrycore.Pin `json:"observation"`
	KindMap     queryregistrycore.Pin `json:"kind_map"`
}

type G1B34Counts struct {
	Alternatives int `json:"alternatives"`
	A            int `json:"a"`
	B            int `json:"b"`
	C            int `json:"c"`
	Expected     int `json:"expected"`
	Blocked      int `json:"blocked"`
	Strong       int `json:"discriminating"`
	Weak         int `json:"weak"`
	Witnesses    int `json:"witnesses"`
}

type CSharpG1B34 struct {
	Schema       string      `json:"schema"`
	Inputs       G1B34Inputs `json:"inputs"`
	Basis        string      `json:"basis"`
	WitnessLimit int         `json:"witness_limit"`
	Counts       G1B34Counts `json:"counts"`
	Alternatives []G1B34Alt  `json:"alternatives"`
}

const csharpG1B34Basis = "Expected values for the production rows csharp-B03 (expressions and patterns) and csharp-B04 (statements), derived without any native output. An alternative of group A (a literal anchor, basis spec-derived) expects a node of the types of each anchor token, spanning exactly that token, at the tightest ordered run of the required keyword and punctuation tokens in an accepted source of a linked case; it does not assert the node that holds the anchors (decision D-1). An alternative of group B (a Roslyn structure, basis differential-oracle) expects a node of the kind map's types for the classified Roslyn kinds with exactly the span of the first such node of an accepted source of a linked case. An alternative of group C is BLOCKED with a reason code. discriminating false is a weak match: an anchor of single punctuation characters, a type that other Roslyn kinds share, or a type set that holds identifier or five or more types. The single-oracle limit applies: a differential-oracle expectation is the behavior of one Roslyn build, not the specification. The position audit and the type audit of this revision are separate assets."

func g1b34Fault(code string) error { return fail(KindInvalidInput, code, CSharpG1B34Path, nil) }

// g1b34Tightest returns the anchor tokens of the narrowest ordered run of anchors in the tokens of a source (the first of equals).
func g1b34Tightest(toks []g1Tok, anchors []string) []int {
	var best []int
	for i, t := range toks {
		if t.end == t.start || t.text != anchors[0] {
			continue
		}
		run := []int{i}
		for j := i + 1; j < len(toks) && len(run) < len(anchors); j++ {
			if toks[j].end > toks[j].start && toks[j].text == anchors[len(run)] {
				run = append(run, j)
			}
		}
		if len(run) == len(anchors) && (best == nil || toks[run[len(run)-1]].end-toks[run[0]].start < toks[best[len(best)-1]].end-toks[best[0]].start) {
			best = run
		}
	}
	return best
}

func g1b34Weak(ts []string) bool {
	return slices.Contains(ts, "identifier") || len(ts) >= 5
}

// DeriveCSharpG1B34 derives the asset from the pinned inputs: the ledger sources, the Roslyn observation and the kind map.
func DeriveCSharpG1B34(in CSharpG1ExpectedSources) (CSharpG1B34, error) {
	x, _, err := deriveCSharpG1B34(in, g1b34Picks, g1b34Overrides)
	return x, err
}

// deriveCSharpG1B34 is the derivation with the corrections of the audits as arguments (a test derives without them to show what they
// change) and also returns the text of the sources, which the audits compare with.
func deriveCSharpG1B34(in CSharpG1ExpectedSources, picks map[string]g1b34Pick, overrides map[string]struct{ code, reason string }) (CSharpG1B34, map[string][]byte, error) {
	var out CSharpG1B34
	_, c, ledger, err := g1ExpContext(in)
	if err != nil {
		return out, nil, err
	}
	texts := map[string][]byte{}
	for _, s := range c.order {
		texts[s.id] = s.data
	}
	_, km3, err := g1ParseKindMapAny(in.KindMap, c.obs.Kinds)
	if err != nil || km3 == nil {
		return out, nil, g1b34Fault("G1_B34_KINDMAP")
	}
	view, err := g1ClassifyViewOf(in, c, ledger)
	if err != nil {
		return out, nil, err
	}
	rep, err := ClassifyCSharpG1(view)
	if err != nil {
		return out, nil, err
	}
	recs := map[string]g1RuleSrc{}
	var alts g1ExpAltFile
	if err := json.Unmarshal(in.Src.Alternatives, &alts); err != nil {
		return out, nil, g1b34Fault("G1_B34_ALTERNATIVES")
	}
	for _, row := range g1ClassifyRows {
		for _, a := range alts.Routes[g1Route][row].Alternatives {
			recs[a.ID] = g1RuleSrc{ID: a.ID, Row: row, Production: a.Production, Kind: a.Kind, Ref: a.Ref}
		}
	}
	out.Schema, out.Basis, out.WitnessLimit = CSharpG1B34Schema, csharpG1B34Basis, g1ExpWitnessLimit
	out.Inputs = G1B34Inputs{Ledger: ledger.Inputs, Observation: queryregistrycore.FilePin(CSharpG1RoslynPathR3, in.Observation), KindMap: queryregistrycore.FilePin(CSharpG1B34KindMapPath, in.KindMap)}
	links := map[string][]G1Link{}
	for _, a := range ledger.Alternatives {
		links[a.ID] = a.Links
	}
	for _, r := range rep.Results {
		rec, ok := recs[r.ID]
		if !ok {
			return out, nil, g1b34Fault("G1_B34_ALTERNATIVES")
		}
		alt := G1B34Alt{ID: r.ID, Row: r.Row, Group: r.Group, Spec: g1SpecLabel(rec.Ref), Witnesses: []G1B34Witness{}}
		if r.Group == G1ClassBlocked {
			alt.Status, alt.Block, alt.Reason = G1ExpBlocked, r.Block, r.Reason
			out.Alternatives = append(out.Alternatives, alt)
			continue
		}
		if o, ok := overrides[r.ID]; ok {
			alt.Status, alt.Block, alt.Reason = G1ExpBlocked, o.code, o.reason
			alt.Group = G1ClassBlocked
			out.Alternatives = append(out.Alternatives, alt)
			continue
		}
		alt.Status, alt.Basis, alt.Discriminating = G1ExpExpected, r.Basis, r.Discriminating
		var sources []*g1Src
		seen := map[string]bool{}
		for _, l := range links[r.ID] {
			if !seen[l.Case] {
				seen[l.Case] = true
				sources = append(sources, c.byCase[l.Case]...)
			}
		}
		sources = slices.DeleteFunc(sources, func(s *g1Src) bool { return !s.accepted })
		switch r.Group {
		case G1ClassLiteral:
			i := slices.IndexFunc(sources, func(s *g1Src) bool { return s.id == r.Witness })
			if i < 0 {
				return out, nil, g1b34Fault("G1_B34_WITNESS")
			}
			s := sources[i]
			run := g1b34Tightest(s.toks, r.Anchors)
			if run == nil {
				return out, nil, g1b34Fault("G1_B34_WITNESS")
			}
			w := G1B34Witness{Source: s.id, Match: g1b34MatchTokens}
			for k, ti := range run {
				t := s.toks[ti]
				ts := slices.Clone(c.kmIdx[t.kind].TS)
				if len(ts) == 0 {
					return out, nil, g1b34Fault("G1_B34_ANCHOR_TYPE")
				}
				alt.Anchors = append(alt.Anchors, G1B34Anchor{Text: r.Anchors[k], TS: ts})
				alt.Discriminating = alt.Discriminating && !g1b34Weak(ts)
				w.Tokens = append(w.Tokens, [2]int{t.start, t.end})
			}
			alt.Form = "anchors " + strings.Join(r.Anchors, " ")
			alt.Witnesses = append(alt.Witnesses, w)
		default:
			pick := picks[r.ID]
			var s *g1Src
			ni := -1
			for _, cand := range sources {
				if ni = slices.IndexFunc(cand.nodes, func(n g1Node) bool { return pick.accepts(cand, n, r.Kinds, r.Parent) }); ni >= 0 {
					s = cand
					break
				}
			}
			if ni < 0 {
				alt.Status, alt.Basis, alt.Discriminating = G1ExpBlocked, "", false
				alt.Block, alt.Reason = G1ClassBlockNoWitness, "no node of the construct (kinds "+strings.Join(r.Kinds, ",")+g1clsUnder(r.Parent)+") passes the specification extent of the alternative in an accepted source of a linked case"
				alt.Group = G1ClassBlocked
				out.Alternatives = append(out.Alternatives, alt)
				continue
			}
			n := s.nodes[ni]
			parent := ""
			if n.parent >= 0 {
				parent = s.nodes[n.parent].kind
			}
			alt.TS = km3.TypesOf(n.kind, parent)
			slices.Sort(alt.TS)
			if len(alt.TS) == 0 {
				return out, nil, g1b34Fault("G1_B34_NODE_TYPE")
			}
			alt.Kinds, alt.Parent = slices.Clone(r.Kinds), slices.Clone(r.Parent)
			alt.Form = "node " + strings.Join(r.Kinds, ",") + g1clsUnder(r.Parent)
			alt.Discriminating = alt.Discriminating && !g1b34Weak(alt.TS)
			alt.Witnesses = append(alt.Witnesses, G1B34Witness{Source: s.id, Match: g1b34MatchSpan, Span: []int{n.start, n.end}, Roslyn: n.kind})
		}
		out.Alternatives = append(out.Alternatives, alt)
	}
	out.Counts = g1b34Counts(out.Alternatives)
	return out, texts, nil
}

func g1b34Counts(alts []G1B34Alt) G1B34Counts {
	var c G1B34Counts
	for _, a := range alts {
		c.Alternatives++
		switch a.Group {
		case G1ClassLiteral:
			c.A++
		case G1ClassStructure:
			c.B++
		default:
			c.C++
		}
		if a.Status == G1ExpExpected {
			c.Expected++
			c.Witnesses += len(a.Witnesses)
			if a.Discriminating {
				c.Strong++
			} else {
				c.Weak++
			}
		} else {
			c.Blocked++
		}
	}
	return c
}

// MarshalCSharpG1B34 writes the canonical asset: one line per alternative.
func MarshalCSharpG1B34(x CSharpG1B34) ([]byte, error) {
	var b bytes.Buffer
	var werr error
	put := func(v any) {
		d, err := json.Marshal(v)
		if err != nil {
			werr = err
		}
		b.Write(d)
	}
	b.WriteString("{\n")
	for _, f := range []struct {
		name string
		v    any
	}{{"schema", x.Schema}, {"inputs", x.Inputs}, {"basis", x.Basis}, {"witness_limit", x.WitnessLimit}, {"counts", x.Counts}} {
		fmt.Fprintf(&b, " %q: ", f.name)
		put(f.v)
		b.WriteString(",\n")
	}
	b.WriteString(" \"alternatives\": [\n")
	for i, a := range x.Alternatives {
		b.WriteString("  ")
		put(a)
		if i < len(x.Alternatives)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(" ]\n}\n")
	return b.Bytes(), werr
}

// DecodeCSharpG1B34 decodes the asset strictly (size bound, no unknown member, schema).
func DecodeCSharpG1B34(data []byte) (CSharpG1B34, error) {
	var x CSharpG1B34
	if len(data) > csharpG1B34Max {
		return x, fail(KindResourceLimit, "G1_B34_SIZE", CSharpG1B34Path, nil)
	}
	if err := jsonv2.Unmarshal(data, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
		return x, fail(KindInvalidInput, "G1_B34_JSON", CSharpG1B34Path, err)
	}
	if x.Schema != CSharpG1B34Schema || x.Basis == "" {
		return x, g1b34Fault("G1_B34_SCHEMA")
	}
	return x, nil
}

// ParseCSharpG1B34 decodes the asset and requires it to equal the derivation from the pinned inputs, member by member.
func ParseCSharpG1B34(data []byte, in CSharpG1ExpectedSources) (CSharpG1B34, error) {
	x, err := DecodeCSharpG1B34(data)
	if err != nil {
		return x, err
	}
	want, err := DeriveCSharpG1B34(in)
	if err != nil {
		return x, err
	}
	return x, CheckCSharpG1B34(data, x, want)
}

// CheckCSharpG1B34 compares a decoded asset with the derivation.
func CheckCSharpG1B34(data []byte, x, want CSharpG1B34) error {
	if !g1eSame(x.Inputs, want.Inputs) {
		return g1b34Fault("G1_B34_PIN")
	}
	if !g1eSame(x.Alternatives, want.Alternatives) {
		return g1b34Fault("G1_B34_ALTERNATIVES")
	}
	if x.Counts != want.Counts || x.WitnessLimit != want.WitnessLimit || x.Basis != want.Basis {
		return g1b34Fault("G1_B34_COUNTS")
	}
	canonical, err := MarshalCSharpG1B34(want)
	if err != nil || !bytes.Equal(canonical, data) {
		return g1b34Fault("G1_B34_BYTES")
	}
	return nil
}

// ExpectedIDs lists the alternatives whose status is EXPECTED.
func (x CSharpG1B34) ExpectedIDs() []string {
	var out []string
	for _, a := range x.Alternatives {
		if a.Status == G1ExpExpected {
			out = append(out, a.ID)
		}
	}
	return out
}
