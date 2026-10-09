package kit

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestOracleExpectationBounds(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(map[string]any, map[string]any, map[string]any)
	}{
		{"normal", "", func(f, m, q map[string]any) {}},
		{"range-overflow", "EXPECT_DYNAMIC_RANGE_INVALID", func(f, m, q map[string]any) { f["end_byte"] = uint64(1) << 32 }},
		{"range-outside", "EXPECT_DYNAMIC_RANGE_INVALID", func(f, m, q map[string]any) { f["end_byte"] = 9 }},
		{"inverted-fact", "EXPECT_DYNAMIC_RANGE_INVALID", func(f, m, q map[string]any) { f["start_byte"] = 8; f["end_byte"] = 1 }},
		{"inverted-miss", "EXPECT_DYNAMIC_RANGE_INVALID", func(f, m, q map[string]any) { m["start_byte"] = 8; m["end_byte"] = 1 }},
		{"miss-outside", "EXPECT_DYNAMIC_RANGE_INVALID", func(f, m, q map[string]any) { m["end_byte"] = 9 }},
		{"row-overflow", "EXPECT_DYNAMIC_POINT_INVALID", func(f, m, q map[string]any) { f["end_point"].(map[string]any)["row"] = uint64(1) << 32 }},
		{"column-overflow", "EXPECT_DYNAMIC_POINT_INVALID", func(f, m, q map[string]any) { f["end_point"].(map[string]any)["column"] = uint64(1) << 32 }},
		{"point-outside", "EXPECT_DYNAMIC_POINT_INVALID", func(f, m, q map[string]any) { f["end_point"].(map[string]any)["column"] = 9 }},
		{"point-inverted", "EXPECT_DYNAMIC_POINT_INVALID", func(f, m, q map[string]any) { f["start_point"].(map[string]any)["row"] = 1 }},
		{"query-overflow", "EXPECT_QUERY_OFFSET_INVALID", func(f, m, q map[string]any) { q["offset"] = uint64(1) << 32 }},
		{"query-outside", "EXPECT_QUERY_OFFSET_INVALID", func(f, m, q map[string]any) { q["offset"] = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := anchorProfile(OracleSchema, nil, nil)
			p["queries"] = []any{map[string]any{"id": "q", "source": "("}}
			p["fact_pack"] = map[string]any{"revision": "fact-queries-r1", "sha256": fxCompiler, "route": p["route"]}
			f := map[string]any{"construct": "EXEC_PAREN", "argument_kind": "LITERAL", "start_byte": 0, "end_byte": 8,
				"start_point": map[string]any{"row": 0, "column": 0}, "end_point": map[string]any{"row": 0, "column": 8}, "variable": nil, "heuristic": false}
			m := map[string]any{"start_byte": 8, "end_byte": 8}
			q := map[string]any{"type": "Syntax", "offset": 1} // EOF is a valid query error location.
			tc.mutate(f, m, q)
			c := p["cases"].([]any)[0].(map[string]any)
			c["dynamic_sql_expect"] = map[string]any{"facts": []any{f}, "known_misses": []any{m}}
			c["query_expect"] = []any{map[string]any{"query": "q", "step": 0, "status": "INVALID_QUERY", "code": "", "captures": nil, "error": q}}
			b, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseOracleProfile(b)
			if tc.code == "" {
				if err != nil || got.OracleCases[0].DynamicSQL.Facts[0].EndByte != 8 {
					t.Fatalf("valid boundary: %v", err)
				}
			} else {
				var ke *Error
				if !errors.As(err, &ke) || ke.Code != tc.code {
					t.Fatalf("got %v, want %s", err, tc.code)
				}
			}
		})
	}
}

func TestDynamicSQLCaptureBounds(t *testing.T) {
	for _, cap := range []Capture{{StartByte: 20, EndByte: 21}, {StartByte: 2, EndByte: 1}, {EndByte: ^uint32(0)}} {
		got, err := DeriveDynamicSQLChecked("tsql", EncodingUTF8, []Capture{cap}, []byte("sp_executesql"))
		var ke *Error
		if !errors.As(err, &ke) || ke.Code != "CAPTURE_RANGE_INVALID" || !reflect.DeepEqual(got, DynamicSQLFacts{}) {
			t.Fatalf("got %+v, %v", got, err)
		}
		if got := DeriveDynamicSQL("tsql", EncodingUTF8, []Capture{cap}, []byte("sp_executesql")); !reflect.DeepEqual(got, DynamicSQLFacts{}) {
			t.Fatalf("invalid legacy result: %+v", got)
		}
	}
	for _, enc := range []string{EncodingUTF8, EncodingUTF16LE, EncodingUTF16BE, EncodingCP949} {
		got, err := DeriveDynamicSQLChecked("tsql", enc, []Capture{{StartByte: 2, EndByte: 2}}, []byte("ab"))
		if err != nil || got.Mapping != "dynamic-sql-r1" || len(got.Items) != 0 {
			t.Fatalf("valid empty boundary %s: %+v, %v", enc, got, err)
		}
	}
}
