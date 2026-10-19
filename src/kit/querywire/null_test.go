package querywire

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querypolicy"
)

func TestNullPrimitiveIsNotZeroValue(t *testing.T) {
	base := encode(t, fixture())
	for _, field := range []string{"code", "query_ms", "partial"} {
		t.Run(field, func(t *testing.T) {
			var objects []map[string]jsontext.Value
			if err := jsonv2.Unmarshal(base, &objects); err != nil {
				t.Fatal(err)
			}
			objects[0][field] = jsontext.Value("null")
			raw, err := jsonv2.Marshal(objects)
			if err != nil {
				t.Fatal(err)
			}
			assertNullRejected(t, raw)
		})
	}
	for column := range 12 {
		t.Run(fmt.Sprintf("capture-column-%d", column), func(t *testing.T) {
			var objects []map[string]jsontext.Value
			if err := jsonv2.Unmarshal(base, &objects); err != nil {
				t.Fatal(err)
			}
			var rows [][]jsontext.Value
			if err := jsonv2.Unmarshal(objects[0]["captures"], &rows); err != nil {
				t.Fatal(err)
			}
			rows[0][column] = jsontext.Value("null")
			var err error
			objects[0]["captures"], err = jsonv2.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := jsonv2.Marshal(objects)
			if err != nil {
				t.Fatal(err)
			}
			assertNullRejected(t, raw)
		})
	}
	for _, field := range []string{"capture_names", "types"} {
		t.Run(field, func(t *testing.T) {
			from := []byte(`"` + field + `":["`)
			var objects []map[string]jsontext.Value
			if !bytes.Contains(base, from) {
				t.Fatal("missing fixture field")
			}
			if err := jsonv2.Unmarshal(base, &objects); err != nil {
				t.Fatal(err)
			}
			objects[0][field] = jsontext.Value("[null]")
			raw, err := jsonv2.Marshal(objects)
			if err != nil {
				t.Fatal(err)
			}
			assertNullRejected(t, raw)
		})
	}
}

func TestNullNestedPrimitiveIsNotZeroValue(t *testing.T) {
	q := fixture()
	one := "ONE"
	q.Predicates = []querypolicy.Predicate{{Steps: []querypolicy.Step{{Kind: "string", Value: "eq?"}, {Kind: "capture", Value: "x", Quantifier: &one}, {Kind: "string", Value: ""}}}}
	base := encode(t, q)
	for _, tc := range []struct{ name, from, to string }{
		{"predicate-pattern", `"pattern":0`, `"pattern":null`},
		{"predicate-kind", `"kind":"string"`, `"kind":null`},
		{"predicate-literal", `"value":""`, `"value":null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bytes.Replace(base, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(raw, base) {
				t.Fatal("mutation did not change fixture")
			}
			assertNullRejected(t, raw)
		})
	}
	// null error and string-step quantifiers are allowed; an empty string stays a string.
	if got, err := DecodeSummary(base, requested, 1, 1); err != nil || len(got) != 1 {
		t.Fatalf("valid nullable members lost: %+v %v", got, err)
	}
	q.Status, q.Code, q.Captures, q.Patterns = "INVALID_QUERY", "QUERY_SYNTAX", nil, 0
	q.Error = &CompileError{Type: "SYNTAX", Point: [2]uint32{0, 0}}
	q.Predicates = nil
	base = encode(t, q)
	if got, err := DecodeSummary(base, requested, 1, 1); err != nil || len(got) != 1 {
		t.Fatalf("valid rejection/null stream lost: %+v %v", got, err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"error-type", `"type":"SYNTAX"`, `"type":null`},
		{"error-offset", `"offset":0`, `"offset":null`},
		{"error-point", `"point":[0,0]`, `"point":null`},
		{"error-point-element", `"point":[0,0]`, `"point":[null,0]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bytes.Replace(base, []byte(tc.from), []byte(tc.to), 1)
			if bytes.Equal(raw, base) {
				t.Fatal("mutation did not change fixture")
			}
			assertNullRejected(t, raw)
		})
	}
}

func TestPaddedNullQueryList(t *testing.T) {
	for _, raw := range []jsontext.Value{[]byte("null"), []byte(" \nnull\t\r")} {
		if got, err := DecodeSummary(raw, nil, 1, 1); err != nil || len(got) != 0 {
			t.Fatalf("empty request lost nullable list: %+v %v", got, err)
		}
		if got, err := DecodeSummary(raw, requested, 1, 1); err == nil || got != nil {
			t.Fatalf("requested query disappeared: %+v %v", got, err)
		}
		if got, err := DecodeFull(raw, nil, 1, 1, nil); err == nil || got != nil {
			t.Fatalf("nil full linker accepted: %+v %v", got, err)
		}
		calls := 0
		if got, err := DecodeFull(raw, nil, 1, 1, func(Query, []int64) bool { calls++; return true }); err != nil || len(got) != 0 || calls != 0 {
			t.Fatalf("empty full request: %+v %v calls=%d", got, err, calls)
		}
	}
	for _, raw := range []jsontext.Value{[]byte("\vnull"), []byte("null0"), []byte("null null")} {
		if got, err := DecodeSummary(raw, nil, 1, 1); err == nil || got != nil {
			t.Fatalf("invalid JSON read as null: %+v %v", got, err)
		}
	}
}

func TestNullCaptureQuantifierInRejectedQueries(t *testing.T) {
	for _, status := range []string{"FAILED", "INVALID_QUERY"} {
		t.Run(status, func(t *testing.T) {
			q := fixture()
			q.Status = status
			q.Captures = nil
			q.Predicates = []querypolicy.Predicate{{Steps: []querypolicy.Step{{Kind: "capture", Value: "x"}}}}
			if status == "FAILED" {
				q.Code, q.Types = "CAPTURE_NODE_UNMAPPED", []string{}
			} else {
				q.Code = "QUERY_SYNTAX"
				q.Patterns = 0
				q.Error = &CompileError{Type: "SYNTAX"}
			}
			if got, err := DecodeSummary(encode(t, q), requested, 1, 1); err == nil || err.Code != "RESPONSE_INVALID" || got != nil {
				t.Fatalf("capture quantifier null adopted: %+v %v", got, err)
			}
			q.Predicates[0].Steps[0].Kind = "string"
			got, err := DecodeSummary(encode(t, q), requested, 1, 1)
			// an invalid query has no pattern, so the producer emits no predicate for it
			if status == "INVALID_QUERY" && (err == nil || err.Code != "RESPONSE_INVALID") || status == "FAILED" && (err != nil || len(got) != 1) {
				t.Fatalf("string quantifier null: %+v %v", got, err)
			}
		})
	}
}

func assertNullRejected(t *testing.T, raw jsontext.Value) {
	t.Helper()
	if got, err := DecodeSummary(raw, requested, 1, 1); err == nil || err.Code != "RESPONSE_MALFORMED" || got != nil {
		t.Fatalf("null primitive adopted as a zero value: %+v err=%v", got, err)
	}
	calls := 0
	if got, err := DecodeFull(raw, requested, 1, 1, func(Query, []int64) bool { calls++; return true }); err == nil || err.Code != "RESPONSE_MALFORMED" || got != nil || calls != 0 {
		t.Fatalf("null primitive reached full adoption/linking: %+v err=%v calls=%d", got, err, calls)
	}
}
