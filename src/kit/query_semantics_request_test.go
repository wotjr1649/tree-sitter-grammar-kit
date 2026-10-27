package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

const semanticRequestFixture = `{"protocol":"tsgk-native/r2","id":"c1","encoding":"UTF-8","output":"tree","limits":{"input_bytes":65536,"nodes":10000,"full_nodes":10000,"depth":1024,"output_bytes":8388608,"parse_ms":10000,"memory_bytes":4294967296,"errors":100,"partial_nodes":100,"matches":10000,"captures":20000,"query_ms":4000},"declarations":[],"points":[],"ranges":[],"source":"eyJhIjpbMSwxXX0=","edits":[{"start_byte":8,"old_end_byte":9,"new_end_byte":9,"old":"MQ==","new":"Mg=="}],"queries":[{"id":"q","source":"KG51bWJlcikgQHg="}],"api":false}`

func semanticTestBudget(t *testing.T) *eligibilityBudget {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	r, e := startRun(ctx, 10*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	return b
}

func TestSemanticRequestDecode(t *testing.T) {
	b := semanticTestBudget(t)
	r, e := b.nativeRequest("request", []byte(semanticRequestFixture))
	if e != nil {
		t.Fatal(e)
	}
	if string(r.source) != `{"a":[1,1]}` || len(r.queries) != 1 || string(r.queries[0].Source) != "(number) @x" {
		t.Fatalf("decoded inputs %+v", r)
	}
	versions, points, err := b.semanticVersions(r, 65536)
	if err != nil || len(versions) != 2 || string(versions[1]) != `{"a":[1,2]}` || len(points) != 1 || points[0] != (EditPoints{Start: Point{Row: 0, Column: 8}, OldEnd: Point{Row: 0, Column: 9}, NewEnd: Point{Row: 0, Column: 9}}) {
		t.Fatalf("versions %q points %v err %v", versions, points, err)
	}
	if !bytes.Equal(versions[0], r.source) || &versions[0][0] != &r.source[0] {
		t.Fatal("initial source was copied")
	}
	r4 := strings.Replace(semanticRequestFixture, `"tsgk-native/r2"`, `"tsgk-native/r4"`, 1)
	r4 = strings.TrimSuffix(r4, "}") + `,"reuse_observation":"` + reuseobs.Policy + `","observation_run_id":"RUN_ID","observation_producer_sha256":"` + strings.Repeat("a", 64) + `","observation_history_start":0}`
	if _, e := semanticTestBudget(t).nativeRequest("request", []byte(r4)); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"reuse_observation", "observation_run_id", "observation_producer_sha256", "observation_history_start"} {
		t.Run("r4-"+key, func(t *testing.T) {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(r4), &m); err != nil {
				t.Fatal(err)
			}
			delete(m, key)
			data, _ := json.Marshal(m)
			if _, e := semanticTestBudget(t).nativeRequest("request", data); e == nil {
				t.Fatal("missing observation member accepted")
			}
		})
	}
}

func TestSemanticRequestShapeAndBase64(t *testing.T) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(semanticRequestFixture), &top); err != nil {
		t.Fatal(err)
	}
	for key := range top {
		for _, absent := range []bool{false, true} {
			t.Run(key+"-"+map[bool]string{false: "null", true: "missing"}[absent], func(t *testing.T) {
				copy := make(map[string]json.RawMessage, len(top))
				for k, v := range top {
					copy[k] = v
				}
				copy[key] = json.RawMessage("null")
				if absent {
					delete(copy, key)
				}
				data, _ := json.Marshal(copy)
				if _, e := semanticTestBudget(t).nativeRequest("request", data); e == nil {
					t.Fatal("invalid request accepted")
				}
			})
		}
	}
	for _, mutation := range []struct{ old, next string }{
		{`"ranges":[]`, `"ranges":[null]`},
		{`"ranges":[]`, `"ranges":[[{"start_byte":0,"end_byte":1,"start_point":[0],"end_point":[0,1]}]]`},
		{`"ranges":[]`, `"ranges":[[{"start_byte":0,"end_byte":1,"start_point":[0,0],"end_point":[0,4294967296]}]]`},
		{`"start_byte":8`, `"start_byte":4294967296`},
		{`"source":"eyJhIjpbMSwxXX0="`, `"source":"e31="`},
		{`"source":"eyJhIjpbMSwxXX0="`, `"source":"e30=\r\n"`},
		{`"source":"eyJhIjpbMSwxXX0="`, `"source":"e30"`},
		{`"id":"c1"`, `"id":"c1","id":"c2"`},
		{`"api":false`, `"api":false,"reuse_observation":null`},
		{`"api":false`, `"api":false,"observation_history_start":0`},
	} {
		if _, e := semanticTestBudget(t).nativeRequest("request", []byte(strings.Replace(semanticRequestFixture, mutation.old, mutation.next, 1))); e == nil {
			t.Fatalf("mutation accepted: %s", mutation.next)
		}
	}
	for _, text := range []string{"", "AA==", "//8="} {
		v := &jv{kind: '"', s: text}
		if _, e := semanticTestBudget(t).semanticBase64(typed{doc: "base64"}, v); e != nil {
			t.Fatalf("valid base64 %q: %v", text, e)
		}
	}
}

func TestSemanticRequestReservations(t *testing.T) {
	b := semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit
	if value, e := b.semanticBase64(typed{doc: "base64"}, &jv{kind: '"', s: "!!!!"}); value != nil || e == nil || e.Kind != KindResourceLimit {
		t.Fatalf("decode preceded reservation: %q %v", value, e)
	}
	b = semanticTestBudget(t)
	if _, e := b.semanticBase64(typed{doc: "base64"}, &jv{kind: '"', s: "!!!!"}); e == nil || e.Kind != KindInvalidInput || b.work == 0 || b.logical == 0 {
		t.Fatalf("malformed prefix uncharged: %v work%d bytes%d", e, b.work, b.logical)
	}
	if _, _, e := semanticBase64Cost("overflow", math.MaxUint64); e == nil || e.Kind != KindResourceLimit {
		t.Fatalf("overflow: %v", e)
	}
	r := semanticNativeRequest{Encoding: EncodingUTF8, source: []byte("a"), edits: []Edit{
		{0, 1, 1, []byte("a"), []byte("b")}, {0, 1, 1, []byte("b"), []byte("c")}, {0, 1, 1, []byte("c"), []byte("d")},
	}}
	versions, _, err := semanticTestBudget(t).semanticVersions(r, 1)
	if err != nil || len(versions) != 4 || string(versions[3]) != "d" {
		t.Fatalf("individual cap became cumulative: %q %v", versions, err)
	}
	b = semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit
	if versions, _, err := b.semanticVersions(r, 1); versions != nil || !errors.As(err, new(*Error)) || asError(err).Kind != KindResourceLimit {
		t.Fatalf("versions allocated before reservation: %q %v", versions, err)
	}
	r.edits[0].Old = []byte("wrong")
	if _, _, err := semanticTestBudget(t).semanticVersions(r, 1); err == nil || asError(err).Code != "EDIT_OLD_MISMATCH" {
		t.Fatalf("ApplyEdits error changed: %v", err)
	}
}

func TestSemanticRequestQueryArrayReservation(t *testing.T) {
	var input semanticNativeRequest
	if err := json.Unmarshal([]byte(semanticRequestFixture), &input); err != nil {
		t.Fatal(err)
	}
	input.Source, input.Edits = "", []semanticRequestEdit{}
	input.Queries = make([]semanticRequestQuery, 1000)
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	pre := semanticTestBudget(t)
	var decoded semanticNativeRequest
	if _, e := pre.decodeValue("request", data, &decoded, true); e != nil {
		t.Fatal(e)
	}
	b := semanticTestBudget(t)
	b.logical = eligibilityLogicalLimit - pre.logical - 36*uint64(len(input.Queries))
	if request, e := b.nativeRequest("request", data); e == nil || e.Kind != KindResourceLimit || request.queries != nil {
		t.Fatalf("query array allocated before its full reservation: %v", e)
	}
}

func TestSemanticVersionsCancellation(t *testing.T) {
	b := semanticTestBudget(t)
	ctx, cancel := context.WithCancel(b.r.caller)
	b.r.caller = ctx
	cancel()
	request := semanticNativeRequest{Encoding: EncodingUTF8, source: []byte("a"), edits: make([]Edit, 1000)}
	if versions, _, err := b.semanticVersions(request, 1); versions != nil || err == nil || asError(err).Kind != KindCancelled || b.work != 0 {
		t.Fatalf("cancelled reservation work %d versions %q error %v", b.work, versions, err)
	}
}
