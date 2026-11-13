package kit

import (
	"encoding/json"
	"slices"
)

// The view of the pinned inputs for the G1-3b classification: the one place where the classification's input is read from the
// ledger, the observation and the kind map. It is on the derivation side and in the closure's roots; ClassifyCSharpG1 does not call it.
// The node types of the kind map enter here only as the names of the other Roslyn kinds that share one (Shares), never by name.

// g1ClassifyRows are the rows the G1-3b classification covers.
var g1ClassifyRows = []string{"csharp-B03", "csharp-B04"}

// G1ClassifyView builds the classification input from the pinned declarations, the Roslyn observation (r3) and the kind map (r2).
func G1ClassifyView(in CSharpG1ExpectedSources) (G1ClassifyInput, error) {
	_, c, ledger, err := g1ExpContext(in)
	if err != nil {
		return G1ClassifyInput{}, err
	}
	return g1ClassifyViewOf(in, c, ledger)
}

// g1ClassifyViewOf builds the input from a context that the caller derived (the derivation of the expected asset shares it).
func g1ClassifyViewOf(in CSharpG1ExpectedSources, c *g1Ctx, ledger CSharpG1Ledger) (G1ClassifyInput, error) {
	var out G1ClassifyInput
	var alts g1ExpAltFile
	if err := json.Unmarshal(in.Src.Alternatives, &alts); err != nil {
		return out, g1eFault("G1_EXPECTED_ALTERNATIVES")
	}
	led := map[string]G1Alternative{}
	for _, a := range ledger.Alternatives {
		led[a.ID] = a
	}
	linked := map[string]bool{}
	for _, row := range g1ClassifyRows {
		for _, a := range alts.Routes[g1Route][row].Alternatives {
			l, ok := led[a.ID]
			if !ok {
				return out, g1eFault("G1_EXPECTED_ALTERNATIVES")
			}
			alt := G1ClassifyAlt{ID: a.ID, Row: row, Production: a.Production, Presence: l.Presence.Status, Literals: slices.Clone(l.Presence.Literals)}
			for _, k := range l.Links {
				alt.Cases = append(alt.Cases, k.Case)
				linked[k.Case] = true
			}
			out.Alts = append(out.Alts, alt)
		}
	}
	for _, s := range c.order {
		if !linked[s.caseID] {
			continue
		}
		cs := G1ClassifySource{ID: s.id, Case: s.caseID, Accepted: s.accepted}
		for _, t := range s.toks {
			if t.end > t.start {
				cs.Tokens = append(cs.Tokens, t.text)
			}
		}
		for _, n := range s.nodes {
			parent := ""
			if n.parent >= 0 {
				parent = s.nodes[n.parent].kind
			}
			cs.Nodes = append(cs.Nodes, G1ClassifyNode{Kind: n.kind, Parent: parent})
		}
		out.Sources = append(out.Sources, cs)
	}
	for _, k := range c.km.Kinds {
		kind := G1ClassifyKind{Name: k.Roslyn, Class: k.Class, Map: k.Map, Reason: k.Reason}
		if k.Class == "node" {
			for _, o := range c.km.Kinds {
				if o.Class == "node" && o.Roslyn != k.Roslyn && slices.ContainsFunc(o.TS, func(t string) bool { return slices.Contains(k.TS, t) }) {
					kind.Shares = append(kind.Shares, o.Roslyn)
				}
			}
		}
		out.Kinds = append(out.Kinds, kind)
	}
	return out, nil
}
