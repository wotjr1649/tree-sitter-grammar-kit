package querywire

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

var requested = []Source{{ID: "q", Source: []byte("(identifier) @x")}}

func fixture() Query {
	return Query{ID: "q", Status: "COMPLETED", Patterns: 2,
		CaptureNames: []string{"x"}, Predicates: []querypolicy.Predicate{}, Matches: 1,
		Types: []string{"identifier"}, Captures: [][]int64{{0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}}}
}

func encode(t *testing.T, q Query) jsontext.Value {
	t.Helper()
	raw, err := jsonv2.Marshal([]Query{q}, jsonv2.FormatNilSliceAsNull(true))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFullAndSummaryBoundaries(t *testing.T) {
	q := fixture()
	raw := encode(t, q)
	calls := 0
	got, err := DecodeFull(raw, requested, 1, 1, func(got Query, row []int64) bool {
		calls++
		return reflect.DeepEqual(got, q) && reflect.DeepEqual(row, q.Captures[0])
	})
	if err != nil || calls != 1 || !reflect.DeepEqual(got, []Query{q}) {
		t.Fatalf("full result=%+v calls=%d err=%v", got, calls, err)
	}
	if got, err := DecodeSummary(raw, requested, 1, 1); err != nil || !reflect.DeepEqual(got, []Query{q}) {
		t.Fatalf("summary wire result=%+v err=%v", got, err)
	}
	if got, err := DecodeFull(raw, requested, 1, 1, func(Query, []int64) bool { return false }); err == nil || err.Code != "CAPTURE_LINK_MISMATCH" || got != nil {
		t.Fatalf("false link adopted: %+v %v", got, err)
	}
	for _, raw := range []jsontext.Value{raw, []byte("[]"), []byte("null")} {
		if got, err := DecodeFull(raw, nil, 1, 1, nil); err == nil || err.Code != "RESPONSE_INVALID" || got != nil {
			t.Fatalf("nil full linker adopted: %+v %v", got, err)
		}
	}
	for _, raw := range []jsontext.Value{[]byte("[]"), []byte("null")} {
		if got, err := DecodeSummary(raw, nil, 1, 1); err != nil || len(got) != 0 {
			t.Fatalf("unrequested query result=%+v err=%v", got, err)
		}
	}
	q.Matches, q.Captures = 0, [][]int64{}
	if got, err := DecodeSummary(encode(t, q), requested, 1, 1); err != nil || got[0].Captures == nil {
		t.Fatalf("completed empty stream lost: %+v %v", got, err)
	}
}

func TestInvalidRowsNeverReachLinker(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Query)
	}{
		{"short", func(q *Query) { q.Captures[0] = q.Captures[0][:11] }},
		{"long", func(q *Query) { q.Captures[0] = append(q.Captures[0], 0) }},
		{"match-negative", func(q *Query) { q.Captures[0][0] = -1 }},
		{"match-count", func(q *Query) { q.Captures[0][0] = 1 }},
		{"match-order", func(q *Query) { q.Matches = 2; q.Captures[0][0] = 1 }},
		{"pattern-negative", func(q *Query) { q.Captures[0][1] = -1 }},
		{"pattern-index", func(q *Query) { q.Captures[0][1] = 2 }},
		{"capture-index", func(q *Query) { q.Captures[0][2] = 1 }},
		{"node-negative", func(q *Query) { q.Captures[0][3] = -1 }},
		{"node-index", func(q *Query) { q.Captures[0][3] = 1 }},
		{"type-index", func(q *Query) { q.Captures[0][4] = 1 }},
		{"flags-negative", func(q *Query) { q.Captures[0][5] = -1 }},
		{"flags-overflow", func(q *Query) { q.Captures[0][5] = 32 }},
		{"byte-negative", func(q *Query) { q.Captures[0][6] = -1 }},
		{"byte-order", func(q *Query) { q.Captures[0][6] = 1; q.Captures[0][7] = 0 }},
		{"source-range", func(q *Query) { q.Captures[0][7] = 2 }},
		{"point-negative", func(q *Query) { q.Captures[0][8] = -1 }},
		{"point-overflow", func(q *Query) { q.Captures[0][11] = 1 << 32 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := fixture()
			tc.edit(&q)
			calls := 0
			got, err := DecodeFull(encode(t, q), requested, 1, 1, func(Query, []int64) bool { calls++; return true })
			if err == nil || err.Code != "RESPONSE_INVALID" || got != nil || calls != 0 {
				t.Fatalf("invalid row adopted: %+v err=%v calls=%d", got, err, calls)
			}
		})
	}
}

func TestStrictJSONAndRequiredMembers(t *testing.T) {
	q := fixture()
	one := "ONE"
	q.Predicates = []querypolicy.Predicate{{Steps: []querypolicy.Step{{Kind: "string", Value: "eq?"}, {Kind: "capture", Value: "x", Quantifier: &one}, {Kind: "string", Value: "a"}}}}
	base := encode(t, q)
	for _, tc := range []struct {
		name, from, to, code string
	}{
		{"missing-list", `"predicates":`, `"other":`, "RESPONSE_INVALID"},
		{"missing-pattern", `"pattern":0,`, "", "RESPONSE_INVALID"},
		{"missing-steps", `"steps":`, `"other":`, "RESPONSE_INVALID"},
		{"missing-kind", `"kind":"capture",`, "", "RESPONSE_INVALID"},
		{"missing-value", `"value":"x",`, "", "RESPONSE_INVALID"},
		{"missing-quantifier", `,"quantifier":"ONE"`, "", "RESPONSE_INVALID"},
		{"null-captures", `"captures":[[0,0,0,0,0,1,0,1,0,0,0,1]]`, `"captures":null`, "RESPONSE_INVALID"},
		{"null-predicates", `"predicates":[`, `"predicates":null,"extra":[`, "RESPONSE_MALFORMED"},
		{"unknown", `"query_ms":0`, `"query_ms":0,"unknown":0`, "RESPONSE_MALFORMED"},
		{"nested-unknown", `"value":"x"`, `"value":"x","unknown":0`, "RESPONSE_MALFORMED"},
		{"duplicate", `"id":"q"`, `"id":"q","id":"q"`, "RESPONSE_MALFORMED"},
		{"nested-duplicate", `"kind":"capture"`, `"kind":"capture","kind":"capture"`, "RESPONSE_MALFORMED"},
		{"wrong-type", `"patterns":2`, `"patterns":"2"`, "RESPONSE_MALFORMED"},
		{"pattern-overflow", `"patterns":2`, `"patterns":4294967296`, "RESPONSE_MALFORMED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bytes.Replace(base, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(raw, base) {
				t.Fatal("mutation did not change fixture")
			}
			if got, err := DecodeSummary(raw, requested, 1, 1); err == nil || err.Code != tc.code || got != nil {
				t.Fatalf("mutated JSON adopted: %+v err=%v", got, err)
			}
		})
	}
	trailing := append(bytes.Clone(base), '0')
	if got, err := DecodeSummary(trailing, requested, 1, 1); err == nil || err.Code != "RESPONSE_MALFORMED" || got != nil {
		t.Fatalf("trailing top-level data adopted: %+v %v", got, err)
	}
	for _, raw := range []jsontext.Value{nil, []byte("null"), []byte("[]"), []byte("{}"), []byte("[null]")} {
		if got, err := DecodeSummary(raw, requested, 1, 1); err == nil || got != nil {
			t.Fatalf("missing query adopted: %+v %v", got, err)
		}
	}
}

func TestStatusAndStreamContracts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Query)
		pass bool
	}{
		{"id", func(q *Query) { q.ID = "other" }, false},
		{"count", func(q *Query) { q.Matches = 2 }, false},
		{"completed-partial", func(q *Query) { q.Partial = true }, false},
		{"completed-code", func(q *Query) { q.Code = "CAPTURE_LIMIT" }, false},
		{"null-predicates", func(q *Query) { q.Predicates = nil }, false},
		{"null-types", func(q *Query) { q.Types = nil }, false},
		{"null-names", func(q *Query) { q.CaptureNames = nil }, false},
		{"host-status", func(q *Query) { q.Status = "UNSUPPORTED" }, false},
		{"limit", func(q *Query) { q.Status = "RESOURCE_LIMIT"; q.Code = "CAPTURE_LIMIT"; q.Partial = true }, true},
		{"limit-complete", func(q *Query) { q.Status = "RESOURCE_LIMIT"; q.Code = "CAPTURE_LIMIT" }, false},
		{"limit-code", func(q *Query) { q.Status = "RESOURCE_LIMIT"; q.Code = "OTHER"; q.Partial = true }, false},
		{"failed", func(q *Query) { q.Status = "FAILED"; q.Code = "CAPTURE_NODE_UNMAPPED"; q.Captures = nil }, true},
		{"failed-stream", func(q *Query) { q.Status = "FAILED"; q.Code = "CAPTURE_NODE_UNMAPPED" }, false},
		{"invalid", func(q *Query) {
			q.Status = "INVALID_QUERY"
			q.Code = "QUERY_SYNTAX"
			q.Error = &CompileError{Type: "SYNTAX", Offset: 1}
			q.Patterns = 0
			q.Captures = nil
		}, true},
		{"invalid-offset", func(q *Query) {
			q.Status = "INVALID_QUERY"
			q.Code = "QUERY_SYNTAX"
			q.Error = &CompileError{Type: "SYNTAX", Offset: 999}
			q.Patterns = 0
			q.Captures = nil
		}, false},
		{"pattern-of-match", func(q *Query) { q.Captures = append(q.Captures, []int64{0, 1, 0, 0, 0, 1, 0, 1, 0, 0, 0, 1}) }, false},
		{"interleaved-and-duplicate", func(q *Query) {
			a := q.Captures[0]
			b := append([]int64(nil), a...)
			b[0] = 1
			q.Matches = 2
			q.Captures = [][]int64{a, b, a, a}
		}, true},
		{"unsupported-wire-predicate", func(q *Query) {
			q.Predicates = []querypolicy.Predicate{{Steps: []querypolicy.Step{{Kind: "string", Value: "match?"}}}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := fixture()
			tc.edit(&q)
			got, err := DecodeSummary(encode(t, q), requested, 1, 1)
			if tc.pass {
				if err != nil || !reflect.DeepEqual(got, []Query{q}) {
					t.Fatalf("valid stream changed: %+v %v", got, err)
				}
			} else if err == nil || got != nil {
				t.Fatalf("invalid stream adopted: %+v %v", got, err)
			}
		})
	}
}
