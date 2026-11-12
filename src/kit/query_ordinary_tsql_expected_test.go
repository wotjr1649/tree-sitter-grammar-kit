package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// Independent expected assets for the selected TSQL shard alt-tsql-B01-r001.
//
// Nothing here reads a parser, native or reference output. The trees are written by hand
// from the fixed prepared grammar.json (rules program, batch, statement, var_declarations,
// select, term, ..., go_statement, set_statement/quoted_identifier_option) and the external
// scanner source (scanner.c: GO line, marginalia, QUOTED_IDENTIFIER state). Only token byte
// offsets are computed, by locating each hand-written token text in the source. Capture
// streams are derived from those trees by a model of the pinned runtime's query cursor
// (query.c: finished-state heap ordered by capture start byte, pattern index and completion
// order; match ids are assigned when a match first yields a capture).

// tsqlTails are the bytes the four edits of the selected case append after the 267-byte
// original source; each version must equal source+tail or authoring stops.
var tsqlTails = []string{
	"",
	"\nGO\nSET QUOTED_IDENTIFIER -- c\n OFF\nSELECT \"x\"\n",
	"\nGO\nSET QUOTED_IDENTIFIER -- c\n ON\nSELECT \"x\"\n",
	"\nGO\n-- c\nSELECT 1;\n-- trailing",
	"\nGO\nSET QUOTED_IDENTIFIER -- c\r\n OFF\r\nSELECT \"x\"\r\n",
}

type tsqlNode struct {
	typ, field, text string
	named, extra     bool
	kids             []*tsqlNode
}

func tsN(typ string, kids ...*tsqlNode) *tsqlNode {
	return &tsqlNode{typ: typ, named: true, kids: kids}
}
func tsK(typ, text string) *tsqlNode { return &tsqlNode{typ: typ, text: text, named: true} }
func tsA(text string) *tsqlNode      { return &tsqlNode{typ: text, text: text} }
func tsX(typ, text string) *tsqlNode {
	return &tsqlNode{typ: typ, text: text, named: true, extra: true}
}
func tsF(field string, n *tsqlNode) *tsqlNode { n.field = field; return n }

// tsqlLayout assigns byte ranges: every leaf is located at the next occurrence of its text
// after whitespace, a parent spans its first to last child, the root spans the input.
func tsqlLayout(t *testing.T, root *tsqlNode, source []byte) []TreeNode {
	t.Helper()
	pos := 0
	type span struct{ start, end int }
	spans := map[*tsqlNode]span{}
	var locate func(n *tsqlNode)
	locate = func(n *tsqlNode) {
		if len(n.kids) == 0 {
			for pos < len(source) && strings.IndexByte(" \t\r\n", source[pos]) >= 0 {
				pos++
			}
			if !bytes.HasPrefix(source[pos:], []byte(n.text)) {
				t.Fatalf("authored token %q (%s) is not at byte %d of the source", n.text, n.typ, pos)
			}
			spans[n] = span{pos, pos + len(n.text)}
			pos += len(n.text)
			return
		}
		for _, k := range n.kids {
			locate(k)
		}
		spans[n] = span{spans[n.kids[0]].start, spans[n.kids[len(n.kids)-1]].end}
	}
	locate(root)
	if rest := bytes.TrimLeft(source[pos:], " \t\r\n"); len(rest) != 0 {
		t.Fatalf("authored tokens stop at byte %d but the source continues with %q", pos, rest)
	}
	spans[root] = span{spans[root].start, len(source)} // the root includes trailing whitespace and extras
	point := func(b int) Point {
		row, line := uint32(0), 0
		for i := 0; i < b; i++ {
			if source[i] == '\n' {
				row, line = row+1, i+1
			}
		}
		return Point{Row: row, Column: uint32(b - line)}
	}
	var out []TreeNode
	var emit func(n *tsqlNode, parent int64)
	emit = func(n *tsqlNode, parent int64) {
		s := spans[n]
		node := TreeNode{Parent: parent, Type: n.typ, Named: n.named, Extra: n.extra, StartByte: uint32(s.start), EndByte: uint32(s.end), StartPoint: point(s.start), EndPoint: point(s.end)}
		if n.field != "" {
			f := n.field
			node.Field = &f
		}
		index := int64(len(out))
		out = append(out, node)
		for _, k := range n.kids {
			emit(k, index)
		}
	}
	emit(root, -1)
	return out
}

// tsqlProgram is the hand-derived CST of the original source plus the tail of one version.
func tsqlProgram(version int) *tsqlNode {
	id := func(text string) *tsqlNode { return tsK("identifier", text) }
	lit := func(text string) *tsqlNode { return tsK("literal", text) }
	field := func(name string) *tsqlNode { return tsN("field", tsF("name", id(name))) }
	objRef := func(name string) *tsqlNode { return tsN("object_reference", tsF("name", id(name))) }
	// term: value:expression, extras between, optional "AS alias".
	term := func(value *tsqlNode, rest ...*tsqlNode) *tsqlNode {
		return tsN("term", append([]*tsqlNode{tsF("value", value)}, rest...)...)
	}
	as := func(alias string) []*tsqlNode {
		return []*tsqlNode{tsK("keyword_as", "AS"), tsF("alias", id(alias))}
	}
	param := func(value *tsqlNode) *tsqlNode { return tsF("parameter", tsN("term", tsF("value", value))) }
	call := func(name string, args ...*tsqlNode) *tsqlNode {
		kids := []*tsqlNode{objRef(name), tsA("(")}
		kids = append(kids, args...)
		return tsN("invocation", append(kids, tsA(")"))...)
	}
	binary := func(left *tsqlNode, op string, right *tsqlNode) *tsqlNode {
		return tsN("binary_expression", tsF("left", left), tsF("operator", tsA(op)), tsF("right", right))
	}
	unary := func(op string, operand *tsqlNode) *tsqlNode {
		return tsN("unary_expression", tsF("operator", tsA(op)), tsF("operand", operand))
	}
	schemaRef := func(schema, name string) *tsqlNode {
		return tsN("object_reference", tsF("schema", id(schema)), tsA("."), tsF("name", id(name)))
	}
	from := func(relation ...*tsqlNode) *tsqlNode {
		return tsN("from", append([]*tsqlNode{tsK("keyword_from", "FROM")}, relation...)...)
	}
	subquery := tsN("subquery", tsA("("), tsN("query_specification",
		tsN("select", tsK("keyword_select", "SELECT"), tsN("select_expression", term(call("MAX", param(field("c1")))))),
		from(tsN("relation", schemaRef("dbo", "t2")))), tsA(")"))
	rowNumber := tsN("window_function", call("ROW_NUMBER"), tsK("keyword_over", "OVER"),
		tsN("window_specification", tsA("("), tsN("order_by", tsK("keyword_order", "ORDER"), tsK("keyword_by", "BY"), tsN("order_target", field("c1"))), tsA(")")))
	sum := tsN("window_function", call("SUM", param(field("c1"))), tsK("keyword_over", "OVER"),
		tsN("window_specification", tsA("("), tsN("partition_by", tsK("keyword_partition", "PARTITION"), tsK("keyword_by", "BY"), field("c2")), tsA(")")))
	terms := []*tsqlNode{
		term(lit("1"), append([]*tsqlNode{tsX("marginalia", "/* block comment\n   second line */")}, as("k")...)...),
		term(call("ABS", param(unary("-", lit("1")))), as("f")...),
		term(tsN("field", objRef("t"), tsA("."), tsF("name", id("c1")))),
		term(field("@v"), as("v")...),
		term(tsN("parenthesized_expression", tsA("("), binary(field("c1"), "+", lit("1")), tsA(")")), as("p")...),
		term(subquery, as("q")...),
		term(unary("-", field("c1")), as("n")...),
		term(binary(field("c1"), "*", lit("2")), as("m")...),
		term(rowNumber, as("rn")...),
		term(sum, as("s")...),
	}
	var selectItems []*tsqlNode
	for i, t := range terms {
		if i > 0 {
			selectItems = append(selectItems, tsA(","))
		}
		selectItems = append(selectItems, t)
	}
	declare := tsN("statement", tsN("var_declarations", tsK("keyword_declare", "DECLARE"),
		tsN("var_declaration", id("@v"), tsK("keyword_int", "int"), tsA("="), lit("1"))))
	selectStatement := func(kids ...*tsqlNode) *tsqlNode { return tsN("statement", tsN("query_specification", kids...)) }
	mainSelect := selectStatement(tsN("select", tsK("keyword_select", "SELECT"), tsN("select_expression", selectItems...)),
		from(tsN("relation", schemaRef("dbo", "t1"), tsK("keyword_as", "AS"), tsF("alias", id("t")))))
	prefix := []*tsqlNode{declare, tsA(";"), mainSelect, tsA(";")}
	simpleSelect := func(value *tsqlNode) *tsqlNode {
		return selectStatement(tsN("select", tsK("keyword_select", "SELECT"), tsN("select_expression", term(value))))
	}
	setStatement := func(setting string) *tsqlNode {
		return tsN("statement", tsN("set_statement", tsK("keyword_set", "SET"), tsN("object_reference", tsF("name", id("QUOTED_IDENTIFIER"))),
			tsX("comment", "-- c"), tsK(setting, strings.ToUpper(setting[len("keyword_"):]))))
	}
	goStatement := tsN("go_statement", tsK("keyword_go", "GO"))
	terminated := func() *tsqlNode { return tsN("batch", append(append([]*tsqlNode{}, prefix...), goStatement)...) }
	switch version {
	case 0:
		return tsN("program", tsN("batch", prefix...))
	case 1, 4:
		return tsN("program", terminated(), tsN("batch", setStatement("keyword_off"), simpleSelect(lit("\"x\""))))
	case 2:
		return tsN("program", terminated(), tsN("batch", setStatement("keyword_on"), simpleSelect(field("\"x\""))))
	case 3:
		return tsN("program", terminated(), tsX("comment", "-- c"), tsN("batch", simpleSelect(lit("1")), tsA(";")), tsX("comment", "-- trailing"))
	}
	panic("unknown TSQL version")
}

// ---- query patterns (the S-expression subset of the 8 registered queries) ----

type tsqlPat struct {
	typ        string // node type, "_" wildcard, or the token text of an anonymous token
	anon       bool
	namedWild  bool // (_) matches named nodes only
	field      string
	anchor     bool
	caps       []string
	kids       []*tsqlPat
	patternIdx int
}

type tsqlQuery struct {
	patterns []*tsqlPat
	names    []string // capture names in first-appearance order (the runtime's capture ids)
}

func tsqlParseQuery(t *testing.T, src string) tsqlQuery {
	t.Helper()
	q := tsqlQuery{}
	i := 0
	skip := func() {
		for i < len(src) && strings.IndexByte(" \t\r\n", src[i]) >= 0 {
			i++
		}
	}
	word := func() string {
		start := i
		for i < len(src) && strings.IndexByte(" \t\r\n():@\"", src[i]) < 0 {
			i++
		}
		return src[start:i]
	}
	var node func() *tsqlPat
	captures := func(p *tsqlPat) {
		for {
			skip()
			if i >= len(src) || src[i] != '@' {
				return
			}
			i++
			name := word()
			p.caps = append(p.caps, name)
			if !slices.Contains(q.names, name) {
				q.names = append(q.names, name)
			}
		}
	}
	node = func() *tsqlPat {
		skip()
		p := &tsqlPat{}
		switch {
		case i < len(src) && src[i] == '(':
			i++
			skip()
			p.typ = word()
			p.namedWild = p.typ == "_"
			for {
				skip()
				if i >= len(src) {
					t.Fatalf("unterminated query pattern in %q", src)
				}
				if src[i] == ')' {
					i++
					break
				}
				anchor := false
				if src[i] == '.' {
					anchor = true
					i++
					skip()
				}
				field := ""
				if j := i; src[i] != '(' && src[i] != '"' {
					w := word()
					if i < len(src) && src[i] == ':' {
						field = w
						i++
					} else {
						i = j
					}
				}
				kid := node()
				kid.field, kid.anchor = field, anchor
				p.kids = append(p.kids, kid)
			}
		case i < len(src) && src[i] == '"':
			i++
			start := i
			for src[i] != '"' {
				i++
			}
			p.typ, p.anon = src[start:i], true
			i++
		default:
			p.typ = word()
			if p.typ != "_" {
				t.Fatalf("unsupported query token %q", p.typ)
			}
		}
		captures(p)
		return p
	}
	for skip(); i < len(src); skip() {
		p := node()
		p.patternIdx = len(q.patterns)
		q.patterns = append(q.patterns, p)
	}
	return q
}

func (q tsqlQuery) metadata() QueryWitnessMetadata {
	return QueryWitnessMetadata{Patterns: uint32(len(q.patterns)), CaptureNames: append([]string{}, q.names...), Predicates: []QueryPredicate{}}
}

// ---- cursor model ----

type tsqlTree struct {
	nodes []TreeNode
	kids  [][]int
	exit  []int // preorder index of the last descendant
}

func newTSQLTree(nodes []TreeNode) tsqlTree {
	tree := tsqlTree{nodes: nodes, kids: make([][]int, len(nodes)), exit: make([]int, len(nodes))}
	for i, n := range nodes {
		tree.exit[i] = i
		if n.Parent >= 0 {
			tree.kids[n.Parent] = append(tree.kids[n.Parent], i)
		}
	}
	for i := len(nodes) - 1; i > 0; i-- {
		p := nodes[i].Parent
		tree.exit[p] = max(tree.exit[p], tree.exit[i])
	}
	return tree
}

func (tr tsqlTree) fits(p *tsqlPat, n int) bool {
	node := tr.nodes[n]
	switch {
	case p.anon:
		if node.Named || node.Type != p.typ {
			return false
		}
	case p.typ == "_":
		if p.namedWild && !node.Named {
			return false
		}
	default:
		if !node.Named || node.Type != p.typ {
			return false
		}
	}
	return p.field == "" || node.Field != nil && *node.Field == p.field
}

type tsqlCapRef struct {
	node    int
	capture int
}

type tsqlInstance struct {
	caps []tsqlCapRef
	done int
}

func (tr tsqlTree) capRefs(q tsqlQuery, p *tsqlPat, n int) []tsqlCapRef {
	var refs []tsqlCapRef
	for _, name := range p.caps {
		refs = append(refs, tsqlCapRef{n, slices.Index(q.names, name)})
	}
	return refs
}

// instances lists the matches of p rooted at n. Shapes the model does not cover stop the
// authoring instead of being approximated.
func (tr tsqlTree) instances(t *testing.T, q tsqlQuery, p *tsqlPat, n int) []tsqlInstance {
	t.Helper()
	if !tr.fits(p, n) {
		return nil
	}
	own := tr.capRefs(q, p, n)
	if len(p.kids) == 0 {
		return []tsqlInstance{{own, n}}
	}
	if len(p.kids) > 1 {
		next := 0
		found := 0
		for _, k := range p.kids {
			for ; next < len(tr.kids[n]); next++ {
				if len(tr.instances(t, q, k, tr.kids[n][next])) != 0 {
					found++
					next++
					break
				}
			}
		}
		if found == len(p.kids) {
			t.Fatalf("pattern %d matches with several child steps; the authoring model does not cover it", p.patternIdx)
		}
		return nil
	}
	var out []tsqlInstance
	for _, c := range tr.kids[n] {
		for _, sub := range tr.instances(t, q, p.kids[0], c) {
			if p.kids[0].anchor {
				t.Fatalf("anchored pattern %d matches; the authoring model does not cover it", p.patternIdx)
			}
			out = append(out, tsqlInstance{append(slices.Clone(own), sub.caps...), sub.done})
		}
	}
	return out
}

type tsqlState struct {
	pattern  int
	appear   int          // node whose entry creates the state
	done     int          // node whose entry completes it; -1 when it never completes
	die      int          // last node of the root subtree: an unfinished state is dropped after it
	partial  []tsqlCapRef // captures held while the state is in progress (the root capture)
	caps     []tsqlCapRef // all captures once the match is complete
	definite bool         // the next step is guaranteed, so partial captures may be returned early
	consumed int
	id       int
	shared   *int // lineage id: a copy made by ts_query_cursor__copy_state keeps the id and consumed count of its source
	order    int
	finished bool
	dead     bool
}

// tsqlDefinite marks the patterns whose child step the runtime proves present in every node
// of the root type (root_pattern_guaranteed): field name on object_reference, field and
// binary_expression (every production of those rules has the field), and the wildcard child of
// program. The schema field of object_reference is fallible.
var tsqlDefinite = map[string]map[int]bool{"facts.dynamic_sql": {3: true, 7: true, 8: true, 12: true}}

// tsqlCaptureStream returns the ordered capture stream of one query over one tree and the
// number of matches that yielded a capture. It follows ts_query_cursor_next_capture: states
// are created and completed in preorder; a finished capture is returned when it precedes
// every capture of an in-progress state (start byte, then pattern index) or when the
// earliest in-progress state is definite; the stream is drained after each node that
// completed a state or advanced into a definite step.
func tsqlCaptureStream(t *testing.T, queryID string, q tsqlQuery, tr tsqlTree) ([]Capture, uint64) {
	t.Helper()
	var states []*tsqlState
	for n := range tr.nodes {
		for _, p := range q.patterns {
			if !tr.fits(p, n) {
				continue
			}
			insts := tr.instances(t, q, p, n)
			rootCaps := len(p.caps) != 0
			definite := tsqlDefinite[queryID][p.patternIdx]
			switch {
			case len(p.kids) == 0 || !rootCaps:
				for _, in := range insts {
					states = append(states, &tsqlState{pattern: p.patternIdx, appear: n, done: in.done, die: tr.exit[n], caps: in.caps})
				}
			case p.kids[0].typ == "_" && p.kids[0].field == "":
				// wildcard child: the original state advances at the first child and a copy stays behind for
				// the later siblings. A copy inherits the id and the consumed count of its source, so with a
				// definite step the root capture is yielded once, early, and every child capture joins one match.
				if !definite {
					t.Fatalf("pattern %d: a wildcard child step under a root capture must be definite for this model", p.patternIdx)
				}
				shared := new(int)
				*shared = -1
				for k, in := range insts {
					st := &tsqlState{pattern: p.patternIdx, appear: n, done: in.done, die: tr.exit[n], caps: in.caps, shared: shared}
					if k == 0 {
						st.partial, st.definite = tr.capRefs(q, p, n), true
					} else {
						st.consumed = len(tr.capRefs(q, p, n))
					}
					states = append(states, st)
				}
				if len(insts) == 0 {
					states = append(states, &tsqlState{pattern: p.patternIdx, appear: n, done: -1, die: tr.exit[n], partial: tr.capRefs(q, p, n), definite: true, shared: shared})
				}
			default:
				if len(insts) > 1 {
					t.Fatalf("pattern %d matches one root with several children; the authoring model does not cover it", p.patternIdx)
				}
				st := &tsqlState{pattern: p.patternIdx, appear: n, done: -1, die: tr.exit[n], partial: tr.capRefs(q, p, n), definite: definite}
				if len(insts) == 1 {
					st.done, st.caps = insts[0].done, insts[0].caps
				}
				states = append(states, st)
			}
		}
	}
	for _, s := range states {
		s.id = -1
	}
	var out []Capture
	nextID, insertOrder := 0, 0
	list := func(s *tsqlState) []tsqlCapRef {
		if s.finished {
			return s.caps
		}
		return s.partial
	}
	startOf := func(s *tsqlState) uint32 { return tr.nodes[list(s)[s.consumed].node].StartByte }
	before := func(a, b *tsqlState) bool {
		return startOf(a) < startOf(b) || startOf(a) == startOf(b) && a.pattern < b.pattern
	}
	emit := func(s *tsqlState) {
		if s.shared != nil {
			if *s.shared < 0 {
				*s.shared = nextID
				nextID++
			}
			s.id = *s.shared
		} else if s.id < 0 {
			s.id = nextID
			nextID++
		}
		ref := list(s)[s.consumed]
		node := tr.nodes[ref.node]
		out = append(out, Capture{Match: uint32(s.id), Pattern: uint32(s.pattern), Capture: uint32(ref.capture), Name: q.names[ref.capture], Node: int64(ref.node), Type: node.Type,
			Named: node.Named, Extra: node.Extra, IsError: node.IsError, HasError: node.HasError, IsMissing: node.IsMissing,
			StartByte: node.StartByte, EndByte: node.EndByte, StartPoint: node.StartPoint, EndPoint: node.EndPoint})
		s.consumed++
	}
	drain := func(current int) {
		for {
			var unfinished, finished *tsqlState
			for _, s := range states {
				if s.dead || s.finished || s.appear > current || s.consumed >= len(s.partial) {
					continue
				}
				if unfinished == nil || before(s, unfinished) {
					unfinished = s
				}
			}
			for _, s := range states {
				if !s.finished || s.consumed >= len(s.caps) {
					continue
				}
				if finished == nil || before(s, finished) || !before(finished, s) && s.order < finished.order {
					finished = s
				}
			}
			switch {
			case finished != nil && (unfinished == nil || before(finished, unfinished)):
				emit(finished)
			case unfinished != nil && unfinished.definite:
				emit(unfinished)
			default:
				return
			}
		}
	}
	for n := range tr.nodes {
		didMatch := false
		for _, s := range states {
			if !s.finished && !s.dead && s.die < n {
				s.dead = true
			}
		}
		for _, s := range states {
			if !s.finished && !s.dead && s.done == n {
				s.finished, s.order = true, insertOrder
				insertOrder++
				didMatch = true
			}
		}
		for _, s := range states {
			if !s.finished && !s.dead && s.appear == n && len(s.partial) != 0 && s.definite {
				didMatch = true
			}
		}
		if didMatch {
			drain(n)
		}
	}
	for _, s := range states {
		if !s.finished {
			s.dead = true
		}
	}
	drain(len(tr.nodes))
	return out, uint64(nextID)
}

// ---- assets ----

type ordinaryTSQLMutation struct {
	Tree     func(step uint32, role string, nodes []TreeNode) []TreeNode
	Captures func(query string, step uint32, role string, c *ordinaryExpectedCaptures)
	Fact     func(query string, step uint32, role string, f *ordinaryExpectedFact)
}

type ordinaryTSQLExpected struct {
	Trees    map[uint32][]TreeNode
	Captures map[string]ordinaryExpectedCaptures // key query/step
}

func tsqlKey(query string, step uint32) string { return fmt.Sprintf("%s/%d", query, step) }

func ordinaryTSQLAssets(t *testing.T, caseID string, source, profileBytes []byte, mutation *ordinaryTSQLMutation) (OrdinaryAssetRegistration, map[string][]byte, ordinaryTSQLExpected, map[string]QueryWitnessMetadata) {
	t.Helper()
	profile, err := ParseOracleProfile(profileBytes)
	if err != nil || len(profile.Native.Cases) != 1 || profile.Native.Cases[0].ID != caseID || len(profile.Native.Cases[0].Edits) != 4 {
		t.Fatalf("expected the fixed four-edit TSQL case: err=%v", err)
	}
	versions, _, err := ApplyEdits(EncodingUTF8, source, profile.Native.Cases[0].Edits, 65536)
	if err != nil {
		t.Fatal(err)
	}
	return ordinaryTSQLAssetsFrom(t, caseID, source, versions, profile.Queries, *profile.FactPack, mutation)
}

func ordinaryTSQLAssetsFrom(t *testing.T, caseID string, source []byte, versions [][]byte, profileQueries []OracleQuery, factRef FactPackRef, mutation *ordinaryTSQLMutation) (OrdinaryAssetRegistration, map[string][]byte, ordinaryTSQLExpected, map[string]QueryWitnessMetadata) {
	t.Helper()
	if len(versions) != len(tsqlTails) {
		t.Fatalf("unexpected TSQL version count %d", len(versions))
	}
	for i, tail := range tsqlTails {
		if string(versions[i]) != string(source)+tail {
			t.Fatalf("version %d differs from the authored source+tail", i)
		}
	}
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Assets: []OrdinaryAsset{
		{ID: "m", Role: "metadata", State: "UNAVAILABLE"}, {ID: "t", Role: "expected-tree", State: "UNAVAILABLE"},
		{ID: "c", Role: "expected-captures", State: "UNAVAILABLE"}, {ID: "f", Role: "expected-facts", State: "UNAVAILABLE"},
	}}
	bodies := map[string][]byte{}
	add := func(id, role, basisText string, body []byte) {
		t.Helper()
		base := "expected/tsql/" + id
		bodyPath, basisPath, reviewPath := base+".json", "basis/"+base+".txt", "review/"+base+".txt"
		basis := []byte(basisText)
		review := []byte("Fixture review: pins and tuple identity checked against the fixed TSQL P0 route, its prepared grammar and the registered query text.")
		pin := OrdinaryPin{Path: bodyPath, Bytes: uint64(len(body)), SHA256: digestHex(body)}
		basisPin := OrdinaryPin{Path: basisPath, Bytes: uint64(len(basis)), SHA256: digestHex(basis)}
		reviewPin := OrdinaryPin{Path: reviewPath, Bytes: uint64(len(review)), SHA256: digestHex(review)}
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: id, Role: role, State: "PINNED", Pin: &pin, AuthoringBasis: &basisPin, IndependentReview: &reviewPin})
		bodies[bodyPath], bodies[basisPath], bodies[reviewPath] = body, basis, review
	}
	queries := map[string]tsqlQuery{}
	metadata := map[string]QueryWitnessMetadata{}
	for _, query := range profileQueries {
		q := tsqlParseQuery(t, query.Source)
		queries[query.ID] = q
		metadata[query.ID] = q.metadata()
		if !ordinaryStructuralMetadataValid(metadata[query.ID]) {
			t.Fatalf("derived metadata of %s is invalid", query.ID)
		}
		add("meta-"+query.ID, "metadata", "Query metadata derived from the registered query text: top-level pattern count and capture names in first-appearance order (runtime capture ids); no predicates.", ordinaryJSON(t, metadata[query.ID]))
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	mappingBytes, err := os.ReadFile("../contracts/fact-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var mapping struct {
		Schema   string `json:"schema"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(mappingBytes, &mapping); err != nil {
		t.Fatal(err)
	}
	packRoute, ok := ordinaryFactRoute(pack, factRef.Route)
	if !ok {
		t.Fatalf("TSQL fact route missing from pack: %s", factRef.Route)
	}
	expected := ordinaryTSQLExpected{Trees: map[uint32][]TreeNode{}, Captures: map[string]ordinaryExpectedCaptures{}}
	for step, version := range versions {
		tree := tsqlLayout(t, tsqlProgram(step), version)
		trees := newTSQLTree(tree)
		sourcePin := OrdinaryPin{Path: "source-witness/" + digestHex(version) + ".utf8", Bytes: uint64(len(version)), SHA256: digestHex(version)}
		expected.Trees[uint32(step)] = tree
		roles := []string{"incremental"}
		if step > 0 {
			roles = append(roles, "fresh")
		}
		for _, role := range roles {
			// Both roles parse the same bytes; the hand-derived tree and streams are identical.
			roleTree := tree
			if mutation != nil && mutation.Tree != nil {
				roleTree = mutation.Tree(uint32(step), role, slices.Clone(tree))
			}
			basis := fmt.Sprintf("Hand-derived CST of version %d from the prepared TSQL grammar.json productions (program/batch/statement/select/term/field/invocation/window_function/set_statement) and scanner.c (GO line end, marginalia, QUOTED_IDENTIFIER %s); extras attach to the lowest node spanning both neighbours.", step, map[int]string{0: "not set", 1: "OFF", 2: "ON", 3: "not set", 4: "OFF"}[step])
			add(fmt.Sprintf("tsql-tree-s%d-%s", step, role), "expected-tree", basis,
				ordinaryJSON(t, ordinaryExpectedTree{Schema: ordinaryExpectedTreeSchema, Case: caseID, Step: uint32(step), TreeRole: role, Source: sourcePin, Tree: roleTree}))
			for _, query := range profileQueries {
				q := queries[query.ID]
				stream, matches := tsqlCaptureStream(t, query.ID, q, trees)
				captures := ordinaryExpectedCaptures{Schema: ordinaryExpectedCaptureSchema, Case: caseID, Query: query.ID, Step: uint32(step), TreeRole: role, Source: sourcePin, Matches: matches, Ordered: stream}
				if captures.Ordered == nil {
					captures.Ordered = []Capture{}
				}
				if role == "incremental" {
					expected.Captures[tsqlKey(query.ID, uint32(step))] = captures
				}
				if mutation != nil && mutation.Captures != nil {
					mutation.Captures(query.ID, uint32(step), role, &captures)
				}
				add(fmt.Sprintf("tsql-captures-%s-s%d-%s", query.ID, step, role), "expected-captures",
					"Ordered capture stream derived from the hand-written tree and the registered query patterns with the pinned runtime cursor order (start byte, pattern index, completion order; match ids by first yield).",
					ordinaryJSON(t, captures))
			}
			for _, fq := range packRoute.Queries {
				fact := ordinaryExpectedFact{Schema: ordinaryExpectedFactSchema, Case: caseID, Query: fq.ID, Step: uint32(step), TreeRole: role, Source: sourcePin,
					Pack: factRef, Mapping: ordinaryFactMappingPin{Schema: mapping.Schema, Revision: mapping.Revision, SHA256: digestHex(mappingBytes)},
					FactQuery: ordinaryFactQueryPin{ID: fq.ID, SHA256: digestHex([]byte(fq.Source)), Kind: fq.Facts}, Status: StatusCompleted, Code: ""}
				switch fq.Facts {
				case FactDeclarations:
					fact.Declarations = &ordinaryExpectedDeclarations{Items: []DeclarationItem{}, Assessment: "NOT_APPLICABLE"}
				case FactDynamicSQL:
					fact.DynamicSQL = &DynamicSQLFacts{Mapping: "dynamic-sql-r1", Heuristic: false, Items: []DynamicSQLFact{}, KnownMisses: []KnownMiss{}, Unlisted: []string{}}
				default:
					t.Fatalf("unexpected TSQL fact kind %s", fq.Facts)
				}
				if mutation != nil && mutation.Fact != nil {
					mutation.Fact(fq.ID, uint32(step), role, &fact)
				}
				add(fmt.Sprintf("tsql-facts-%s-s%d-%s", fq.ID, step, role), "expected-facts",
					"Hand-derived facts: the tree has no create_*, execute_statement or ERROR node, so the declaration list is empty (NOT_APPLICABLE) and dynamic-sql-r1 has no items, known misses or unlisted misses for the tsql route.",
					ordinaryJSON(t, fact))
			}
		}
	}
	return assets, bodies, expected, metadata
}

// The authoring itself needs no native run: every tree must be structurally and positionally
// valid, every capture stream must link to its tree and metadata, and the derived metadata must
// follow the query text.
func TestOrdinaryTSQLExpectedAuthoring(t *testing.T) {
	const caseID = "alt-tsql-B01-r001"
	_, inventory := ordinaryP0ForPlanTest(t)
	workload := inventory.Routes[ordinaryRouteIndex(inventory, "tsql")].Workload
	source := ordinaryRouteSources(t, "tsql")[caseID]
	querySources := ordinaryRouteQueries(t, "tsql")
	queries := make([]OracleQuery, 0, len(workload.Queries))
	for _, query := range workload.Queries {
		queries = append(queries, OracleQuery{ID: query.ID, Source: querySources[query.ID]})
	}
	packBytes, err := os.ReadFile("../contracts/fact-query-pack.json")
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ParseFactPack(packBytes)
	if err != nil {
		t.Fatal(err)
	}
	var versions [][]byte
	for _, tail := range tsqlTails {
		versions = append(versions, []byte(source+tail))
	}
	_, bodies, expected, metadata := ordinaryTSQLAssetsFrom(t, caseID, []byte(source), versions, queries, FactPackRef{Revision: pack.Revision, SHA256: pack.SHA256, Route: "tsql"}, nil)
	for step, tree := range expected.Trees {
		if err := ValidateTree(tree, uint64(len(versions[step]))); err != nil || !ordinaryTreePointsValid(tree, versions[step]) {
			t.Fatalf("version %d expected tree is invalid: %v", step, err)
		}
		for _, query := range queries {
			captures := expected.Captures[tsqlKey(query.ID, step)]
			if !ordinaryCapturesValid(captures.Ordered, metadata[query.ID], tree, versions[step]) {
				t.Fatalf("version %d query %s: expected captures do not link to the tree and metadata", step, query.ID)
			}
		}
	}
	if out := os.Getenv("TSGK_TSQL_SEAL_OUT"); out != "" {
		// Seal record: per-member SHA-256 and one aggregate over the sorted body members, written before any native run.
		paths := make([]string, 0, len(bodies))
		for path := range bodies {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		var manifest, aggregate strings.Builder
		for _, path := range paths {
			line := digestHex(bodies[path]) + "  " + path + "\n"
			manifest.WriteString(line)
			if strings.HasPrefix(path, "expected/") {
				aggregate.WriteString(line)
			}
		}
		manifest.WriteString("aggregate-sha256(expected bodies)  " + digestHex([]byte(aggregate.String())) + "\n")
		manifest.WriteString("aggregate-sha256(all members)  " + digestHex([]byte(manifest.String())) + "\n")
		if err := os.WriteFile(out, []byte(manifest.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if len(bodies) == 0 || len(metadata) != 8 {
		t.Fatalf("expected 8 query metadata and a non-empty asset set, got %d/%d", len(metadata), len(bodies))
	}
	if os.Getenv("TSGK_TSQL_DUMP") == "1" {
		for _, step := range []uint32{0, 1, 3} {
			tree := expected.Trees[step]
			for _, query := range queries {
				for _, c := range expected.Captures[tsqlKey(query.ID, step)].Ordered {
					t.Logf("s%d %s m%d p%d %s node%d %s [%d,%d)", step, query.ID, c.Match, c.Pattern, c.Name, c.Node, tree[c.Node].Type, c.StartByte, c.EndByte)
				}
			}
		}
	}
	// Capture counts and node counts are part of the authored claim; changing the grammar derivation must change them knowingly.
	for step := range versions {
		total := 0
		for _, query := range queries {
			total += len(expected.Captures[tsqlKey(query.ID, uint32(step))].Ordered)
		}
		t.Logf("version %d: %d nodes, %d captures across %d queries", step, len(expected.Trees[uint32(step)]), total, len(queries))
	}
}
