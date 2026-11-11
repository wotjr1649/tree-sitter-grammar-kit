package native

import (
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

// Query capability and host policy revisions (tree-and-adapter-protocol.md "S06 구현").
const (
	QueryCapability = "tsgk-query/r1"
	APICapability   = "tsgk-api/r1"
	PredicatePolicy = querypolicy.Revision
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

func predicateSupport(q WireQuery, encoding string) string {
	return querypolicy.Support(q.Patterns, q.Predicates, encoding)
}

func evaluatePredicates(q WireQuery, caps []kit.Capture, src []byte) ([]kit.Capture, string) {
	views := make([]querypolicy.Capture, len(caps))
	for i, c := range caps {
		views[i] = querypolicy.Capture{Match: c.Match, Pattern: c.Pattern, Name: c.Name, StartByte: c.StartByte, EndByte: c.EndByte}
	}
	indices, code := querypolicy.Filter(q.Patterns, q.Predicates, views, kit.EncodingUTF8, src)
	if code != "" {
		return nil, code
	}
	out := make([]kit.Capture, 0, len(indices))
	for _, i := range indices {
		out = append(out, caps[i])
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

type APIDifference = nativewire.APIDifference

func compareAPI(nodes []kit.TreeNode, a *WireAPI) (*APIDifference, []APIDifference) {
	return nativewire.CompareAPI(nodes, a)
}
