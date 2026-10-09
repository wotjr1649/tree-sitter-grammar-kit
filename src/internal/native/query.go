package native

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Query capability and host policy revisions (tree-and-adapter-protocol.md "S06 구현").
const (
	QueryCapability = "tsgk-query/r1"
	APICapability   = "tsgk-api/r1"
	PredicatePolicy = "tsgk-predicates/r1"
)

// Query result evaluation states: STRUCTURAL (no predicates; the runtime stream is the
// result), EVALUATED (every predicate is in the host subset and was applied) and
// UNSUPPORTED (a predicate or directive outside the subset: no capture is presented).
const (
	EvalStructural  = "STRUCTURAL"
	EvalEvaluated   = "EVALUATED"
	EvalUnsupported = "UNSUPPORTED"
)

// QueryOut is one query's result on one tree as recorded. Status is the driver status
// (COMPLETED, RESOURCE_LIMIT, INVALID_QUERY, FAILED), UNSUPPORTED when the host policy cannot
// evaluate its predicates, or NOT_RUN when the tree did not complete. Captures are the
// evaluated stream; a partial stream keeps its captures with Partial set.
type QueryOut struct {
	ID           string          `json:"id"`
	SHA256       string          `json:"sha256"`
	Status       string          `json:"status"`
	Code         string          `json:"code"`
	Evaluation   string          `json:"evaluation"`
	QueryMillis  uint64          `json:"query_ms"`
	Error        *QueryError     `json:"error"`
	Patterns     uint32          `json:"patterns"`
	CaptureNames []string        `json:"capture_names"`
	Predicates   []WirePredicate `json:"predicates"`
	Matches      uint64          `json:"matches"`
	Partial      bool            `json:"partial"`
	Captures     []kit.Capture   `json:"captures"`
	structural   []kit.Capture   // the runtime stream before host evaluation
}

var supportedPredicates = map[string]bool{"eq?": true, "not-eq?": true, "any-of?": true, "not-any-of?": true}

// predicateSupport returns "" when every predicate of q is in the host subset
// tsgk-predicates/r1, else the reason: eq?/not-eq? between a single (ONE) capture and a
// string or another single capture, any-of?/not-any-of? of a single capture against one or
// more strings, compared by source bytes, and only for UTF-8 sources.
func predicateSupport(q WireQuery, encoding string) string {
	for _, p := range q.Predicates {
		op := p.Steps[0]
		if op.Kind != "string" || !supportedPredicates[op.Value] {
			return "QUERY_PREDICATE_UNSUPPORTED:" + op.Value
		}
		args := p.Steps[1:]
		single := func(s PredStep) bool { return s.Kind == "capture" && *s.Quantifier == "ONE" }
		switch op.Value {
		case "eq?", "not-eq?":
			if len(args) != 2 || !single(args[0]) || (args[1].Kind == "capture" && !single(args[1])) {
				return "QUERY_PREDICATE_UNSUPPORTED:" + op.Value
			}
		default:
			if len(args) < 2 || !single(args[0]) {
				return "QUERY_PREDICATE_UNSUPPORTED:" + op.Value
			}
			for _, a := range args[1:] {
				if a.Kind != "string" {
					return "QUERY_PREDICATE_UNSUPPORTED:" + op.Value
				}
			}
		}
		if encoding != kit.EncodingUTF8 {
			return "QUERY_PREDICATE_ENCODING"
		}
	}
	return ""
}

// evaluatePredicates applies the host subset to a complete stream: a match whose pattern
// has a failing predicate loses all its captures; the remaining captures keep their order.
// A single capture named by a predicate but absent from its match makes the query FAILED
// rather than guessing a value.
func evaluatePredicates(q WireQuery, caps []kit.Capture, src []byte) ([]kit.Capture, string) {
	byPattern := map[uint32][]WirePredicate{}
	for _, p := range q.Predicates {
		byPattern[p.Pattern] = append(byPattern[p.Pattern], p)
	}
	matches := map[uint32][]kit.Capture{}
	for _, c := range caps {
		matches[c.Match] = append(matches[c.Match], c)
	}
	keep := map[uint32]bool{}
	for id, mc := range matches {
		ok := true
		text := func(name string) ([]byte, bool) {
			for _, c := range mc {
				if c.Name == name {
					return src[c.StartByte:c.EndByte], true
				}
			}
			return nil, false
		}
		for _, p := range byPattern[mc[0].Pattern] {
			args := p.Steps[1:]
			left, found := text(args[0].Value)
			if !found {
				return nil, "QUERY_PREDICATE_CAPTURE_ABSENT"
			}
			var r bool
			switch p.Steps[0].Value {
			case "eq?", "not-eq?":
				right := []byte(args[1].Value)
				if args[1].Kind == "capture" {
					if right, found = text(args[1].Value); !found {
						return nil, "QUERY_PREDICATE_CAPTURE_ABSENT"
					}
				}
				r = bytes.Equal(left, right) == (p.Steps[0].Value == "eq?")
			default:
				in := slices.ContainsFunc(args[1:], func(a PredStep) bool { return a.Value == string(left) })
				r = in == (p.Steps[0].Value == "any-of?")
			}
			ok = ok && r
		}
		keep[id] = ok
	}
	out := []kit.Capture{}
	for _, c := range caps {
		if keep[c.Match] {
			out = append(out, c)
		}
	}
	return out, ""
}

// queryOuts converts the validated query results of one tree. sources carries each
// query's bytes for its identity; a nil tree query list (tree not completed) yields
// NOT_RUN entries so an absent result is never read as an empty one.
func queryOuts(t Tree, sources []QuerySource, encoding string, src []byte) []QueryOut {
	out := []QueryOut{}
	for i, qs := range sources {
		o := QueryOut{ID: qs.ID, SHA256: sha(qs.Source), Status: kit.StatusNotRun, Code: "TREE_NOT_COMPLETED", CaptureNames: []string{}, Predicates: []WirePredicate{}}
		if i >= len(t.Queries) {
			out = append(out, o)
			continue
		}
		q := t.Queries[i]
		o.Status, o.Code, o.QueryMillis, o.Error, o.Patterns, o.CaptureNames, o.Predicates, o.Matches, o.Partial =
			q.Status, q.Code, q.QueryMillis, q.Error, q.Patterns, q.CaptureNames, q.Predicates, q.Matches, q.Partial
		if q.Predicates == nil {
			o.Predicates = []WirePredicate{}
		}
		if q.Captures == nil {
			out = append(out, o)
			continue
		}
		caps := make([]kit.Capture, 0, len(q.Captures))
		for _, row := range q.Captures {
			caps = append(caps, captureOf(q, row))
		}
		o.Evaluation, o.Captures, o.structural = EvalStructural, caps, caps
		if len(q.Predicates) > 0 {
			reason := predicateSupport(q, encoding)
			switch {
			case reason != "":
				o.Status, o.Code, o.Evaluation, o.Captures = EvalUnsupported, reason, EvalUnsupported, nil
			case q.Partial:
				// predicates over an incomplete stream would judge incomplete matches
				o.Evaluation, o.Captures = EvalUnsupported, nil
			default:
				evaluated, code := evaluatePredicates(q, caps, src)
				if code != "" {
					o.Status, o.Code, o.Evaluation, o.Captures = kit.StatusFailed, code, EvalUnsupported, nil
				} else {
					o.Evaluation, o.Captures = EvalEvaluated, evaluated
				}
			}
		}
		out = append(out, o)
	}
	return out
}

// compareQueries is the incremental/fresh query comparison of one edit step: the same
// status, code and evaluated capture stream for every query. Node indices are compared too:
// equal trees give equal preorder indices.
func compareQueries(a, b []QueryOut) (bool, string) {
	for i := range a {
		x, y := a[i], b[i]
		if x.Status != y.Status || x.Code != y.Code || x.Evaluation != y.Evaluation {
			return false, fmt.Sprintf("query %s status %s/%s %s/%s", x.ID, x.Status, y.Status, x.Code, y.Code)
		}
		if d := kit.CompareCaptures(x.Captures, y.Captures); d != nil {
			return false, fmt.Sprintf("query %s capture %d %s %s != %s", x.ID, d.Index, d.Field, d.Left, d.Right)
		}
		// a query without an evaluated stream (UNSUPPORTED predicates) still has the runtime's
		// structural stream on both trees
		if d := kit.CompareCaptures(x.structural, y.structural); d != nil {
			return false, fmt.Sprintf("query %s structural capture %d %s %s != %s", x.ID, d.Index, d.Field, d.Left, d.Right)
		}
	}
	return true, ""
}

// APIDifference is the first disagreement of an API observation with the serialized tree.
type APIDifference struct {
	Check string `json:"check"`
	Node  int64  `json:"node"`
	Got   string `json:"got"`
	Want  string `json:"want"`
}

// compareAPI checks the tsgk-api/r1 observations of a full tree against its cursor
// serialization: node API parent, siblings, first children, child/named counts, subtree
// size, cursor depth and descendant index, the field name of every child, every field
// lookup (the first child carrying the field, and no lookup without one), and the byte
// lookups of the registered points (the found node covers the byte; the first child for a
// byte is the first root child ending after it). The pinned runtime navigates node
// siblings by byte position (lib/src/node.c ts_node__prev_sibling/ts_node__next_sibling), so
// at a zero-width node the node API may skip a sibling the cursor visits: a sibling
// difference where the node or the cursor's sibling is zero-width and the node API gives
// null or a sibling in the same direction is returned as a divergence, not a difference.
// Every other disagreement is the first difference.
func compareAPI(nodes []kit.TreeNode, a *WireAPI) (*APIDifference, []APIDifference) {
	var divergences []APIDifference
	n := len(nodes)
	children := make([][]int64, n)
	depth := make([]int64, n)
	size := make([]int64, n)
	for i := n - 1; i >= 0; i-- {
		size[i]++
		if p := nodes[i].Parent; p >= 0 {
			size[p] += size[i]
		}
	}
	for i, nd := range nodes {
		if nd.Parent >= 0 {
			children[nd.Parent] = append(children[nd.Parent], int64(i))
			depth[i] = depth[nd.Parent] + 1
		}
	}
	diff := func(check string, node int64, got, want int64) *APIDifference {
		return &APIDifference{check, node, strconv.FormatInt(got, 10), strconv.FormatInt(want, 10)}
	}
	// positional reports whether the node API's sibling got differs from the cursor's only by
	// skipping zero-width siblings at x's boundary (x's end going forward, its start going
	// back), as the pinned runtime's position-based search does: every sibling it passed over
	// in that direction, the cursor's sibling included, is such a node, and got is null or
	// the next sibling after them. x itself is never on the walked side, so a node named as
	// its own sibling is a difference.
	positional := func(x, got int64, step int, named bool) bool {
		p := nodes[x].Parent
		if p < 0 {
			return false
		}
		b := nodes[x].EndByte
		if step < 0 {
			b = nodes[x].StartByte
		}
		sib := children[p]
		skipped := 0
		for k := slices.Index(sib, x) + step; k >= 0 && k < len(sib); k += step {
			c := sib[k]
			if named && !nodes[c].Named {
				continue
			}
			if c == got {
				return skipped > 0
			}
			if nodes[c].StartByte != b || nodes[c].EndByte != b {
				return false
			}
			skipped++
		}
		return got == -1 && skipped > 0
	}
	sibling := func(i int64, step int, named bool) int64 {
		p := nodes[i].Parent
		if p < 0 {
			return -1
		}
		sib := children[p]
		k := slices.Index(sib, i)
		for k += step; k >= 0 && k < len(sib); k += step {
			if !named || nodes[sib[k]].Named {
				return sib[k]
			}
		}
		return -1
	}
	first := func(list []int64, named bool) int64 {
		for _, c := range list {
			if !named || nodes[c].Named {
				return c
			}
		}
		return -1
	}
	for i, row := range a.Nodes {
		x := int64(i)
		named := int64(0)
		for _, c := range children[i] {
			if nodes[c].Named {
				named++
			}
		}
		want := []int64{nodes[i].Parent, sibling(x, -1, false), sibling(x, 1, false), sibling(x, -1, true), sibling(x, 1, true),
			first(children[i], false), first(children[i], true), int64(len(children[i])), named, size[i], depth[i], x, x}
		checks := []string{"parent", "prev_sibling", "next_sibling", "prev_named_sibling", "next_named_sibling", "first_child", "first_named_child",
			"child_count", "named_child_count", "descendant_count", "cursor_depth", "cursor_descendant_index", "cursor_node"}
		for k := range want {
			if row[k] == want[k] {
				continue
			}
			if k >= 1 && k <= 4 && positional(x, row[k], []int{-1, 1, -1, 1}[k-1], k >= 3) {
				divergences = append(divergences, *diff("position_navigation:"+checks[k], x, row[k], want[k]))
				continue
			}
			return diff(checks[k], x, row[k], want[k]), divergences
		}
	}
	// field name of each child: exactly the cursor fields
	got := map[[2]int64]string{}
	for _, r := range a.ChildFields {
		if r[1] >= int64(len(children[r[0]])) {
			return diff("child_field_ordinal", r[0], r[1], int64(len(children[r[0]]))), divergences
		}
		got[[2]int64{r[0], r[1]}] = a.Names[r[2]]
	}
	lookups := map[[2]int64]bool{}
	for i, list := range children {
		seen := map[string]bool{}
		for k, c := range list {
			f := nodes[c].Field
			key := [2]int64{int64(i), int64(k)}
			g, ok := got[key]
			if (f == nil) == ok || (ok && g != *f) {
				return &APIDifference{"child_field", int64(i), fmt.Sprintf("%d:%q", k, g), fmt.Sprintf("%d:%s", k, fieldText(f))}, divergences
			}
			if f != nil && !seen[*f] {
				seen[*f] = true
				lookups[[2]int64{int64(i), int64(slices.Index(a.Names, *f))}] = true
				// the lookup row for this field must name this child
				found := false
				for _, r := range a.FieldLookups {
					if r[0] == int64(i) && a.Names[r[1]] == *f {
						if r[2] != c {
							return diff("field_lookup:"+*f, int64(i), r[2], c), divergences
						}
						found = true
					}
				}
				if !found {
					return diff("field_lookup:"+*f, int64(i), -1, c), divergences
				}
			}
		}
	}
	for _, r := range a.FieldLookups {
		if !lookups[[2]int64{r[0], r[1]}] {
			return diff("field_lookup_extra:"+a.Names[r[1]], r[0], r[2], -1), divergences
		}
	}
	for _, r := range a.Points {
		b := r[0]
		outsideRoot := b < int64(nodes[0].StartByte) || b > int64(nodes[0].EndByte)
		for k, check := range []string{"descendant_for_byte", "named_descendant_for_byte"} {
			x := r[1+k]
			if x < 0 || x >= int64(n) || (outsideRoot && x != 0) || (!outsideRoot && (int64(nodes[x].StartByte) > b || b > int64(nodes[x].EndByte) || (k == 1 && !nodes[x].Named && x != 0))) {
				return diff(check, x, b, -1), divergences
			}
		}
		want := int64(-1)
		for _, c := range children[0] {
			if int64(nodes[c].EndByte) > b {
				want = c
				break
			}
		}
		if r[3] != want {
			return diff("first_child_for_byte", 0, r[3], want), divergences
		}
	}
	return nil, divergences
}

func fieldText(f *string) string {
	if f == nil {
		return "null"
	}
	return strconv.Quote(*f)
}
