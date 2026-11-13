package kit

import (
	"regexp"
	"slices"
)

// The authored corrections of the G1-3b derivation that the position audit demanded before the seal (#233). The classification
// (csharp_g1_classify_rules.go) names the Roslyn kinds of a construct; some constructs are narrower than their Roslyn kind. The
// position audit (csharp_g1_b34_audit.go) lists, from the specification text, the instances of each B alternative and compared them
// with the first node of the classified kinds: the alternatives below did not match and are fixed here, so that the sealed
// expectation is an instance of the alternative. Each entry names the alternative's audit row, whose `fixed` text states the finding.

// g1b34Pick narrows which node of the classified kinds is the witness: the direct parent kind, a pattern the text of the node must
// match (Text) or must not (NotText), and a pattern the text of the parent must not match (ParentNotText).
type g1b34Pick struct {
	Parent        []string
	Text, NotText string
	ParentNotText string
}

var g1b34Picks = map[string]g1b34Pick{
	// the arguments of an argument list; a tuple element is a Roslyn Argument too, but its form is tuple_element
	"csharp-B03.a014": {Parent: []string{"ArgumentList", "BracketedArgumentList"}},
	// base[0] is base_access (12.8.15), not element_access
	"csharp-B03.a025": {NotText: `^base\[`},
	// a?.b is a null conditional member access; the element access form is a?[...]
	"csharp-B03.a026": {Text: `^[A-Za-z_]\w*\?\[`},
	// a block as a statement; the body of a method is method_body, not an embedded statement
	"csharp-B04.a004": {Parent: []string{"Block"}},
	// var and ref declarations are other productions
	"csharp-B04.a031": {NotText: `^(?:var|ref)\b`},
	"csharp-B04.a033": {ParentNotText: `^(?:var|ref)\b`},
}

var g1b34PickRe = map[string]*regexp.Regexp{}

func g1b34Re(p string) *regexp.Regexp {
	if p == "" {
		return nil
	}
	if r, ok := g1b34PickRe[p]; ok {
		return r
	}
	r := regexp.MustCompile(p)
	g1b34PickRe[p] = r
	return r
}

// accepts reports whether the node n of s is a node of the classified kinds (under the classified parent kinds) that the pick allows.
func (p g1b34Pick) accepts(s *g1Src, n g1Node, kinds, parent []string) bool {
	if !slices.Contains(kinds, n.kind) {
		return false
	}
	parents := parent
	if len(p.Parent) > 0 {
		parents = p.Parent
	}
	pk, ptext := "", ""
	if n.parent >= 0 {
		pk, ptext = s.nodes[n.parent].kind, s.nodeText(s.nodes[n.parent])
	}
	if len(parents) > 0 && !slices.Contains(parents, pk) {
		return false
	}
	text := s.nodeText(n)
	if r := g1b34Re(p.Text); r != nil && !r.MatchString(text) {
		return false
	}
	if r := g1b34Re(p.NotText); r != nil && r.MatchString(text) {
		return false
	}
	if r := g1b34Re(p.ParentNotText); r != nil && r.MatchString(ptext) {
		return false
	}
	return true
}

// g1b34Overrides are the alternatives the position audit moved from B to blocked: no node has the extent of the production.
var g1b34Overrides = map[string]struct{ code, reason string }{
	"csharp-B03.a028": {G1ClassBlockNoNode, "base_access is base with its member or element access (12.8.15); Roslyn's BaseExpression is the keyword alone, so no node of the construct has the extent of the production"},
	"csharp-B03.a209": {G1ClassBlockNoNode, "join_into_clause is a whole join clause with its into part (12.23.1); Roslyn's JoinIntoClause and the grammar's join_into_clause are the into part alone, so no node has the extent of the production"},
}
