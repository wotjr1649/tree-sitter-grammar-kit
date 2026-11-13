package kit

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// G1-3b classification of the production alternatives of csharp-B03 and csharp-B04 (#233).
//
// Every alternative falls into one of three groups before any expected value is derived:
//
//   - A, a literal anchor (basis spec-derived): the production requires keyword or punctuation tokens, or names a nonterminal that is
//     one fixed lexeme. The anchors are looked up as an ordered run of token texts in an accepted source of a linked case. This proves
//     that the anchors occur, not the node that holds them (decision D-1): an anchor of one punctuation character is weak.
//   - B, a structure (basis differential-oracle): the construct has Roslyn node kinds that the kind map maps to a node, and an accepted
//     source of a linked case holds one (under the stated parent kind, if the rule names one).
//   - C, blocked, with a reason code of G1ClassBlockCodes.
//
// The classification is a function of G1ClassifyInput: the alternatives (production text, the ledger's required literals, the linked
// cases), the tokens and node kinds of the cases' sources, and, per Roslyn kind, only the kind map's class, map and reason. It holds no
// tree-sitter node type: the types of an alternative are derived after the classification, from the kind map, and an audit that reads
// them never sees the classification's input. It reads no native output (TestCSharpG1ExpectedSeparation).

const (
	G1ClassLiteral   = "A"
	G1ClassStructure = "B"
	G1ClassBlocked   = "C"

	G1ClassBasisSpec = "spec-derived"
	G1ClassBasisDiff = "differential-oracle"

	G1ClassBlockCategory  = "ABSTRACT_CATEGORY"
	G1ClassBlockPosition  = "POSITION_DEPENDENT"
	G1ClassBlockSemantic  = "SEMANTIC_BINDING"
	G1ClassBlockNoNode    = "NO_OWN_NODE_FOR_CONSTRUCT"
	G1ClassBlockNoWitness = "NO_WITNESS_IN_LINKED_SOURCES"
)

// G1ClassBlockCodes are the reason codes a blocked alternative may carry.
var G1ClassBlockCodes = []string{G1ClassBlockCategory, G1ClassBlockPosition, G1ClassBlockSemantic, G1ClassBlockNoNode, G1ClassBlockNoWitness}

// G1ClassifyAlt is one alternative as the classification sees it.
type G1ClassifyAlt struct {
	ID, Row, Production string
	Presence            string   // the ledger's presence status; carried to the report only
	Literals            []string // the ledger's required quoted terminals
	Cases               []string // the linked cases
}

// G1ClassifyNode is a node of a source's observation with the kind of its direct parent ("" at the root).
type G1ClassifyNode struct{ Kind, Parent string }

// G1ClassifySource is an observed source: its zero-width-free token texts in order and its nodes in preorder.
type G1ClassifySource struct {
	ID, Case string
	Accepted bool
	Tokens   []string
	Nodes    []G1ClassifyNode
}

// G1ClassifyKind is what the classification knows of a Roslyn kind: the class (token or node), the map (1:1, 1:n, none) and the reason
// of none of the kind map, and the other node kinds that share a node type with it. It carries no node type.
type G1ClassifyKind struct {
	Name, Class, Map, Reason string
	Shares                   []string
}

type G1ClassifyInput struct {
	Alts    []G1ClassifyAlt
	Sources []G1ClassifySource
	Kinds   []G1ClassifyKind
}

// G1ClassifyResult is the classification of one alternative.
type G1ClassifyResult struct {
	ID, Row, Group string
	Basis          string
	Anchors        []string // A
	Kinds          []string // B: the Roslyn kinds with a node that exhibit the construct
	Parent         []string // B: the required kind of the direct parent, if any
	Discriminating bool     // A and B; false is a weak match
	Witness        string   // A and B: the first accepted source of a linked case that exhibits it
	Block, Reason  string   // C
	Rule           string   // the key of the rule that decided it ("anchors" for an alternative with anchors)
}

type G1ClassifyCount struct {
	Key   string
	Count int
}

type G1ClassifyCounts struct {
	Alternatives  int
	A, B, C       int
	ADiscr, AWeak int
	BDiscr, BWeak int
	Rows          []G1ClassifyCount // "<row> <group>"
	Blocked       []G1ClassifyCount // by reason code
}

// G1ClassifyKindUse is a Roslyn kind that a rule of a B alternative (or one demoted for want of a node) names.
type G1ClassifyKindUse struct {
	Kind, Map, Reason string
	Alternatives      []string
	Status            string // USED (a node, the alternative is B) or NO_NODE (the kind map maps it to none)
}

type G1ClassifyReport struct {
	Results []G1ClassifyResult
	Counts  G1ClassifyCounts
	Kinds   []G1ClassifyKindUse
}

func g1clsFault(code string) error {
	return fail(KindInvalidInput, code, "src/contracts/feature-alternatives.json", nil)
}

var (
	g1clsAltSuffix = regexp.MustCompile(`\s+\[alt (\d+)/(\d+)\]$`)
	g1clsIdent     = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	g1clsWord      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	g1clsNT        = regexp.MustCompile(`[a-z_][a-z0-9_]*`)
	g1clsQuoted    = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)
)

// g1clsSplit returns the head, the right side without the alternative suffix and the 1-based index of the alternative (1 when the
// production has no suffix).
func g1clsSplit(p string) (head, rhs string, idx int, ok bool) {
	head, rhs, ok = strings.Cut(p, ": ")
	if !ok {
		return "", "", 0, false
	}
	idx = 1
	if m := g1clsAltSuffix.FindStringSubmatch(rhs); m != nil {
		idx, _ = strconv.Atoi(m[1])
		rhs = rhs[:len(rhs)-len(m[0])]
	}
	return strings.TrimPrefix(head, "fragment "), strings.TrimSpace(rhs), idx, true
}

// g1clsNonterminals lists the nonterminal names of a right side in order, leaving out the quoted terminals.
func g1clsNonterminals(rhs string) []string {
	return g1clsNT.FindAllString(g1clsQuoted.ReplaceAllString(rhs, " "), -1)
}

// g1clsLookup finds the rule of an alternative: head#i, head>target (a pass-through), head, >target.
func g1clsLookup(head, rhs string, idx int) (g1clsRule, string, bool) {
	keys := []string{head + "#" + strconv.Itoa(idx)}
	if g1clsIdent.MatchString(rhs) {
		keys = append(keys, head+">"+rhs, head, ">"+rhs)
	} else {
		keys = append(keys, head)
	}
	for _, k := range keys {
		if r, ok := g1clsRules[k]; ok {
			return r, k, true
		}
	}
	return g1clsRule{}, "", false
}

// g1clsAnchors are the anchors of an alternative: its required literals, or the lexemes of the nonterminals that are one fixed lexeme.
func g1clsAnchors(a G1ClassifyAlt, rhs string) []string {
	if len(a.Literals) > 0 {
		return a.Literals
	}
	var out []string
	for _, nt := range g1clsNonterminals(rhs) {
		if w, ok := g1clsLexemes[nt]; ok {
			out = append(out, w)
		}
	}
	return out
}

func g1clsHasRun(tokens, anchors []string) bool {
	at := 0
	for _, t := range tokens {
		if t == anchors[at] {
			if at++; at == len(anchors) {
				return true
			}
		}
	}
	return false
}

// g1clsAnchorDiscriminates: an anchor that is a word or longer than one character names its construct; a bracket, comma, dot, colon,
// question mark, equals sign or other single character occurs in most constructs.
func g1clsAnchorDiscriminates(anchors []string) bool {
	return slices.ContainsFunc(anchors, func(s string) bool { return len(s) > 1 || g1clsWord.MatchString(s) })
}

func g1clsHasNode(s G1ClassifySource, kinds, parent []string) bool {
	return slices.ContainsFunc(s.Nodes, func(n G1ClassifyNode) bool {
		return slices.Contains(kinds, n.Kind) && (len(parent) == 0 || slices.Contains(parent, n.Parent))
	})
}

func (r *G1ClassifyResult) block(code, reason string) {
	r.Group, r.Block, r.Reason = G1ClassBlocked, code, reason
}

// ClassifyCSharpG1 classifies every alternative of the input. It fails on an alternative without a rule, a rule that names a kind
// the kind map lacks or a token kind, and an input whose kinds repeat; it never guesses.
func ClassifyCSharpG1(in G1ClassifyInput) (G1ClassifyReport, error) {
	var rep G1ClassifyReport
	kinds := map[string]G1ClassifyKind{}
	for _, k := range in.Kinds {
		if _, dup := kinds[k.Name]; dup {
			return rep, g1clsFault("G1_CLASSIFY_KINDS")
		}
		kinds[k.Name] = k
	}
	byCase := map[string][]int{}
	for i, s := range in.Sources {
		byCase[s.Case] = append(byCase[s.Case], i)
	}
	uses := map[string]*G1ClassifyKindUse{}
	seen := map[string]bool{}
	var missing []string
	for _, a := range in.Alts {
		if seen[a.ID] {
			return rep, g1clsFault("G1_CLASSIFY_ALTERNATIVES")
		}
		seen[a.ID] = true
		res := G1ClassifyResult{ID: a.ID, Row: a.Row}
		head, rhs, idx, ok := g1clsSplit(a.Production)
		if !ok {
			return rep, g1clsFault("G1_CLASSIFY_PRODUCTION")
		}
		// the sources a linked, accepted case offers, in input order
		var sources []G1ClassifySource
		for _, c := range a.Cases {
			for _, i := range byCase[c] {
				if in.Sources[i].Accepted {
					sources = append(sources, in.Sources[i])
				}
			}
		}
		if anchors := g1clsAnchors(a, rhs); len(anchors) > 0 {
			res.Rule = "anchors"
			res.Group, res.Basis, res.Anchors, res.Discriminating = G1ClassLiteral, G1ClassBasisSpec, slices.Clone(anchors), g1clsAnchorDiscriminates(anchors)
			if i := slices.IndexFunc(sources, func(s G1ClassifySource) bool { return g1clsHasRun(s.Tokens, anchors) }); i >= 0 {
				res.Witness = sources[i].ID
			} else {
				res.Group, res.Basis, res.Anchors, res.Discriminating = "", "", nil, false
				res.block(G1ClassBlockNoWitness, "no accepted source of a linked case holds the anchors "+strings.Join(anchors, " ")+" in order")
			}
			rep.Results = append(rep.Results, res)
			continue
		}
		rule, key, found := g1clsLookup(head, rhs, idx)
		if !found {
			missing = append(missing, a.ID)
			continue
		}
		res.Rule = key
		if rule.block != "" {
			res.block(rule.block, rule.why)
			rep.Results = append(rep.Results, res)
			continue
		}
		// a kind of the rule with a node; a kind without one is the kind map's disposition of the construct
		var withNode, without []string
		for _, name := range rule.kinds {
			k, ok := kinds[name]
			if !ok || k.Class != "node" {
				return rep, g1clsFault("G1_CLASSIFY_KIND:" + key + ":" + name)
			}
			u := uses[name]
			if u == nil {
				u = &G1ClassifyKindUse{Kind: name, Map: k.Map, Reason: k.Reason, Status: "USED"}
				if k.Map == "none" {
					u.Status = "NO_NODE"
				}
				uses[name] = u
			}
			u.Alternatives = append(u.Alternatives, a.ID)
			if k.Map == "none" {
				without = append(without, name+" ("+k.Reason+")")
			} else {
				withNode = append(withNode, name)
			}
		}
		if len(withNode) == 0 {
			res.block(G1ClassBlockNoNode, "the kind map gives no node to "+strings.Join(without, ", "))
			rep.Results = append(rep.Results, res)
			continue
		}
		i := slices.IndexFunc(sources, func(s G1ClassifySource) bool { return g1clsHasNode(s, withNode, rule.parent) })
		if i < 0 {
			res.block(G1ClassBlockNoWitness, "no accepted source of a linked case holds a node of "+strings.Join(withNode, ", ")+g1clsUnder(rule.parent))
			rep.Results = append(rep.Results, res)
			continue
		}
		res.Group, res.Basis, res.Kinds, res.Parent, res.Witness = G1ClassStructure, G1ClassBasisDiff, withNode, slices.Clone(rule.parent), sources[i].ID
		res.Discriminating = true
		for _, name := range withNode {
			k := kinds[name]
			// a node type that only the kinds of this construct share names it; one shared with another kind is a weak match
			if k.Map != "1:1" || slices.ContainsFunc(k.Shares, func(o string) bool { return !slices.Contains(withNode, o) }) {
				res.Discriminating = false
			}
		}
		rep.Results = append(rep.Results, res)
	}
	if len(missing) > 0 {
		return rep, g1clsFault("G1_CLASSIFY_RULE:" + strings.Join(missing, ","))
	}
	g1clsCount(&rep, uses)
	return rep, nil
}

func g1clsUnder(parent []string) string {
	if len(parent) == 0 {
		return ""
	}
	return " under " + strings.Join(parent, ", ")
}

func g1clsCount(rep *G1ClassifyReport, uses map[string]*G1ClassifyKindUse) {
	c := &rep.Counts
	rows := map[string]int{}
	blocked := map[string]int{}
	for _, r := range rep.Results {
		c.Alternatives++
		rows[r.Row+" "+r.Group]++
		switch r.Group {
		case G1ClassLiteral:
			c.A++
			if r.Discriminating {
				c.ADiscr++
			} else {
				c.AWeak++
			}
		case G1ClassStructure:
			c.B++
			if r.Discriminating {
				c.BDiscr++
			} else {
				c.BWeak++
			}
		default:
			c.C++
			blocked[r.Block]++
		}
	}
	for _, k := range g1clsKeys(rows) {
		c.Rows = append(c.Rows, G1ClassifyCount{k, rows[k]})
	}
	for _, k := range g1clsKeys(blocked) {
		c.Blocked = append(c.Blocked, G1ClassifyCount{k, blocked[k]})
	}
	for _, k := range g1clsKeys(uses) {
		u := *uses[k]
		u.Alternatives = slices.Compact(u.Alternatives)
		rep.Kinds = append(rep.Kinds, u)
	}
}

// String is the one-line form of a result for the dry-run listing.
func (r G1ClassifyResult) String() string {
	switch r.Group {
	case G1ClassLiteral:
		return fmt.Sprintf("%s\tA\tdiscriminating=%t\tanchors=%s\twitness=%s", r.ID, r.Discriminating, strings.Join(r.Anchors, " "), r.Witness)
	case G1ClassStructure:
		return fmt.Sprintf("%s\tB\tdiscriminating=%t\tkinds=%s%s\twitness=%s", r.ID, r.Discriminating, strings.Join(r.Kinds, ","), g1clsUnder(r.Parent), r.Witness)
	}
	return fmt.Sprintf("%s\tC\t%s\t%s", r.ID, r.Block, r.Reason)
}

// g1clsKeys returns the keys of m in order (a result must not depend on the iteration order of a map).
func g1clsKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
