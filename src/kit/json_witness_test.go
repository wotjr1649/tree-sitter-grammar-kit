package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	jsonv2 "encoding/json/v2"
)

func witnessTestRun(t *testing.T, metered bool) *run {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	r, e := startRun(ctx, 10*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	if metered {
		r.eligibility = &eligibilityBudget{r: r}
	}
	return r
}

func witnessASTDecode(doc string, data []byte, target any, exact bool, r *run) *Error {
	if r.eligibility != nil {
		return r.eligibility.decode(doc, data, target, exact)
	}
	value, e := decodeBounded(doc, data, MaxDocumentBytes, &valueBudget{max: 2000000, check: r.check})
	if e != nil {
		return e
	}
	if e := witnessShape(typed{doc: doc}, value, reflect.TypeOf(target).Elem(), exact, false, &valueBudget{max: 2000000, check: r.check}); e != nil {
		return e
	}
	if err := jsonv2.Unmarshal(data, target); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

func witnessBoundParity(t *testing.T, doc string, data []byte, target any, exact bool) {
	t.Helper()
	typ := reflect.TypeOf(target).Elem()
	oldValue, newValue := reflect.New(typ).Interface(), reflect.New(typ).Interface()
	oldRun, newRun := witnessTestRun(t, true), witnessTestRun(t, true)
	oldErr := oldRun.eligibility.decode(doc, data, oldValue, exact)
	newErr := decodeWitnessDocument(doc, data, newValue, exact, newRun)
	if (oldErr == nil) != (newErr == nil) || oldErr != nil && (oldErr.Kind != newErr.Kind || oldErr.Code != newErr.Code || oldErr.Code != "JSON_MISSING_FIELD" && oldErr.Path != newErr.Path) {
		t.Fatalf("bound AST/typed witness parity: AST=%v typed=%v", oldErr, newErr)
	}
	if oldErr == nil && !reflect.DeepEqual(oldValue, newValue) {
		t.Fatalf("bound AST/typed witness values differ: AST=%+v typed=%+v", oldValue, newValue)
	}
}
func witnessParity(t *testing.T, doc string, data []byte, target any, exact bool) {
	t.Helper()
	if len(data) > MaxDocumentBytes {
		t.Fatal("test document exceeds the established JSON limit")
	}
	typ := reflect.TypeOf(target).Elem()
	for _, metered := range []bool{false, true} {
		mode := "standalone"
		if metered {
			mode = "eligibility"
		}
		t.Run(mode, func(t *testing.T) {
			oldValue := reflect.New(typ).Interface()
			oldErr := witnessASTDecode(doc, data, oldValue, exact, witnessTestRun(t, metered))
			newValue := reflect.New(typ).Interface()
			newErr := decodeWitnessDocument(doc, data, newValue, exact, witnessTestRun(t, metered))
			if (oldErr == nil) != (newErr == nil) || oldErr != nil && (oldErr.Kind != newErr.Kind || oldErr.Code != newErr.Code || oldErr.Code != "JSON_MISSING_FIELD" && oldErr.Path != newErr.Path) {
				t.Fatalf("AST/typed witness parity: AST=%v typed=%v input=%q", oldErr, newErr, data[:min(len(data), 300)])
			}
			if oldErr == nil && !reflect.DeepEqual(oldValue, newValue) {
				t.Fatalf("AST/typed witness values differ: AST=%+v typed=%+v", oldValue, newValue)
			}
		})
	}
}

func queryWitnessStoredDocuments(t *testing.T) map[string][]byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/query-witness.json")
	if err != nil {
		t.Fatal(err)
	}
	var docs map[string]json.RawMessage
	if err := json.Unmarshal(data, &docs); err != nil {
		t.Fatal(err)
	}
	result := make(map[string][]byte, len(docs))
	for _, name := range []string{"raw", "projection"} {
		if len(docs[name]) == 0 {
			t.Fatalf("stored query witness has no %q document", name)
		}
		result[name] = bytes.Clone(docs[name])
	}
	return result
}

func TestTypedJSONWitnessDifferential(t *testing.T) {
	docs := queryWitnessStoredDocuments(t)
	witnessParity(t, "raw", docs["raw"], new(witnessRaw), false)
	witnessParity(t, "projection", docs["projection"], new(witnessProjection), false)

	paths, err := filepath.Glob("../testdata/query-reference/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("stored reference fixtures: paths=%v err=%v", paths, err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			witnessParity(t, "reference-response", data, new(instrumentedReference), true)
		})
	}

	req, reg, _, _ := witnessFixture(t)
	req = marshalWitness(t, req, reg, map[string]any{}, map[string]any{})
	witnessParity(t, "query-registration", req.Registration, new(QueryWitnessRegistration), true)

	capture := Capture{Match: 1, Pattern: 2, Capture: 3, Name: "x", Node: 4, Type: "identifier"}
	tuple, err := jsonv2.Marshal(QueryWitnessTuple{Query: "q", TreeRole: "incremental", Structural: []Capture{}, Expected: []Capture{capture}})
	if err != nil {
		t.Fatal(err)
	}
	witnessParity(t, "expected-captures", tuple, new(QueryWitnessTuple), true)
}

func witnessRegistrationDocument(t *testing.T, metadata any, includeQuery bool) []byte {
	t.Helper()
	var queries any = nil
	if includeQuery {
		queries = []any{map[string]any{"id": "q", "source": "query", "metadata": metadata}}
	}
	data, err := jsonv2.Marshal(map[string]any{
		"schema": QueryWitnessRegistrationSchema, "policy": "tsgk-predicates/r1", "case": "c1", "route": "owned-plain",
		"platform": "windows/amd64", "role": "owned", "encoding": EncodingUTF8,
		"input": map[string]any{"path": "case.txt", "role": "case", "bytes": 1, "sha256": strings.Repeat("a", 64)},
		"edits": []any{}, "queries": queries, "required": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestTypedJSONWitnessNullableOptionalAndPointerRules(t *testing.T) {
	metadata := map[string]any{"patterns": 1, "capture_names": []any{"x"}, "predicates": []any{}}
	tuple, err := jsonv2.Marshal(QueryWitnessTuple{Query: "q", TreeRole: "incremental", Structural: []Capture{}, Expected: nil})
	if err != nil {
		t.Fatal(err)
	}
	projectionQuery, err := jsonv2.Marshal(map[string]any{
		"id": "q", "sha256": "x", "status": StatusCompleted, "code": "", "evaluation": "EVALUATED", "query_ms": 0, "error": nil,
		"patterns": 1, "capture_names": []any{}, "predicates": []any{}, "matches": 0, "partial": false, "captures": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	rawTree, err := jsonv2.Marshal(map[string]any{
		"status": StatusCompleted, "code": "", "form": nil, "descendant_count": 0, "has_error": false, "digest": "",
		"types": nil, "fields": nil, "nodes": nil, "queries": nil,
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		doc    string
		data   []byte
		target any
		exact  bool
	}{
		{"registration-nullable-metadata-pointer", "query-registration", witnessRegistrationDocument(t, nil, true), new(QueryWitnessRegistration), true},
		{"registration-nullable-expected-anchor", "expected", tuple, new(QueryWitnessTuple), true},
		{"registration-nonnullable-metadata-array", "query-registration", witnessRegistrationDocument(t, map[string]any{"patterns": 1, "capture_names": nil, "predicates": []any{}}, true), new(QueryWitnessRegistration), true},
		{"registration-nonnullable-queries-array", "query-registration", witnessRegistrationDocument(t, metadata, false), new(QueryWitnessRegistration), true},
		{"raw-nullable-pointer-and-arrays", "raw-tree", rawTree, new(witnessRawTree), true},
		{"projection-nullable-captures", "query-projection", projectionQuery, new(witnessQueryProjection), true},
		{"projection-summary-omits-tree", "projection-tree", []byte(`{"status":"FAILED","code":"NODE_LIMIT","form":"summary","descendant_count":0,"has_error":false,"digest":"","queries":[]}`), new(witnessTreeProjection), false},
		{"projection-failed-omits-tree", "projection-tree", []byte(`{"status":"FAILED","code":"NODE_LIMIT","form":"full","descendant_count":0,"has_error":false,"digest":"","queries":[]}`), new(witnessTreeProjection), false},
		{"nullable-array-pointer-elements", "pointer-array", []byte(`[null,"x"]`), new([2]*string), true},
		{"native-operation-omitted-optionals", "operation", []byte(`{"name":"test","input_bytes":1,"nodes":1,"full_nodes":1,"depth":1,"output_bytes":1,"parse_ms":1,"memory_bytes":1,"errors":1,"partial_nodes":1,"parse_wall_ns":1,"edit_wall_ns":1,"max_edits":0,"max_cases":1,"outputs":[],"batch":false,"run_wall_ns":1}`), new(NativeOperation), true},
		{"rcinput-omitted-encoding-source", "input", []byte(`{"encoding":"UTF-8","bytes":1,"sha256":"x"}`), new(rcInput), true},
		{"rcinput-valid-encoding-source", "input", []byte(`{"encoding":"UTF-8","encoding_source":"DECLARATION","bytes":1,"sha256":"x"}`), new(rcInput), true},
		{"rcinput-invalid-encoding-source", "input", []byte(`{"encoding":"UTF-8","encoding_source":"OTHER","bytes":1,"sha256":"x"}`), new(rcInput), true},
		{"rcinput-null-encoding-source", "input", []byte(`{"encoding":"UTF-8","encoding_source":null,"bytes":1,"sha256":"x"}`), new(rcInput), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.data) == 0 {
				t.Fatal("test document is empty")
			}
			witnessParity(t, tc.doc, tc.data, tc.target, tc.exact)
		})
	}
}
func TestTypedJSONWitnessEligibilityDocuments(t *testing.T) {
	receipt, err := jsonv2.Marshal(eligibilityReceipt{Schema: "tsgk-reference-process/r1"})
	if err != nil {
		t.Fatal(err)
	}
	failure, err := jsonv2.Marshal(eligibilityFailureResponse{Schema: instrumentedReferenceSchema, Status: StatusFailed, Code: "CST_LIMIT", Complete: false})
	if err != nil {
		t.Fatal(err)
	}
	footer, err := jsonv2.Marshal(eligibilityBatchFooter{Schema: "tsgk-instrumented-query-reference-batch/r1", Status: StatusCompleted, Complete: true, Queries: []string{}, ResponsesCompleted: 0})
	if err != nil {
		t.Fatal(err)
	}
	registration := QueryWitnessRegistration{
		Schema: QueryWitnessRegistrationSchema, Policy: "tsgk-predicates/r1", Case: "c1", Route: "owned-plain", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8,
		Input: NativeInput{Path: "case.txt", Role: "case", Bytes: 1, SHA256: strings.Repeat("a", 64)}, Edits: []Edit{}, Queries: []QueryWitnessQuery{}, Required: []QueryWitnessTuple{},
	}
	witness, err := jsonv2.Marshal(eligibilityWitness{QueryWitnessRegistration: registration, Native: eligibilityNativeExpected{Points: []uint32{}}})
	if err != nil {
		t.Fatal(err)
	}
	witnessBoundParity(t, "receipt", receipt, new(eligibilityReceipt), true)
	witnessBoundParity(t, "failure-response", failure, new(eligibilityFailureResponse), true)
	witnessBoundParity(t, "batch-footer", footer, new(eligibilityBatchFooter), true)
	witnessBoundParity(t, "eligibility-witness", witness, new(eligibilityWitness), true)
}

func TestTypedJSONWitnessShapeValueCap(t *testing.T) {
	b := &eligibilityBudget{r: witnessTestRun(t, true)}
	arrayShape, e := b.typedJSONShape("scalar-array", reflect.TypeFor[[2]uint32]())
	if e != nil {
		t.Fatal(e)
	}
	var visits, values uint64
	if e := b.readTypedJSONChecked("scalar-array", jsontext.NewDecoder(bytes.NewBufferString(`[0,1]`)), arrayShape, &values, &visits); e != nil || visits != 4 {
		t.Fatalf("scalar array shape visits changed from the legacy four to %d: %v", visits, e)
	}

	shape, e := b.typedJSONShapeFor("shape-cap", reflect.TypeFor[QueryWitnessTuple](), true, true)
	if e != nil {
		t.Fatal(e)
	}
	decoder := jsontext.NewDecoder(bytes.NewBufferString(`null`))
	shapeVisits := uint64(2000000 - shape.shapeTakes)
	if e := b.takeTypedShape("shape-cap", decoder, shape, &shapeVisits, false); e != nil || shapeVisits != 2000000 {
		t.Fatalf("exact shape visit cap: visits=%d err=%v", shapeVisits, e)
	}
	if e := b.takeTypedShape("shape-cap", decoder, shape, &shapeVisits, false); e == nil || e.Kind != KindResourceLimit || e.Code != "JSON_VALUE_LIMIT" {
		t.Fatalf("shape visit cap not retained: visits=%d err=%v", shapeVisits, e)
	}
	ctx, cancel := context.WithCancel(b.r.caller)
	b.r.caller = ctx
	cancel()
	if e := b.takeTypedShape("shape-cap", decoder, shape, &shapeVisits, false); e == nil || e.Kind != KindCancelled || e.Code != "CANCELLED" {
		t.Fatalf("caller cancellation must precede the bound shape cap: %v", e)
	}
}

func TestTypedJSONReferencePointWork(t *testing.T) {
	f := newReferenceFixture(t)
	source := bytes.Repeat([]byte{'x'}, 64<<10)
	response := f.responses["reference-0.json"]
	response["input"] = map[string]any{"encoding": EncodingUTF8, "bytes": len(source), "sha256": sum(source)}
	response["tree"] = map[string]any{"nodes": []TreeNode{{Parent: -1, Type: "doc", Named: true, EndByte: uint32(len(source)), EndPoint: Point{Column: uint32(len(source))}}}}
	query := response["query"].(map[string]any)
	for _, name := range []string{"capture_names", "predicates", "match_mapping", "structural", "evaluated"} {
		query[name] = []any{}
	}
	query["matches"] = 0
	data := mustJSON(t, response)
	obs := QueryReferenceObservation{Query: "q", SourceBytes: uint64(len(source)), SourceSHA256: sum(source)}
	decode := func(r *run, versions [][]byte) *Error {
		_, code, complete, e := decodeReferenceResponseBound(data, obs, f.registration.LibrarySHA256, f.registration.Symbol, QueryWitnessQuery{}, false, versions, r)
		if e == nil && (code != "" || !complete) {
			t.Fatalf("reference control: code=%q complete=%v", code, complete)
		}
		return e
	}
	without := witnessTestRun(t, true)
	if e := decode(without, nil); e != nil {
		t.Fatal(e)
	}
	with := witnessTestRun(t, true)
	if e := decode(with, [][]byte{source}); e != nil {
		t.Fatal(e)
	}
	if got := with.eligibility.work - without.eligibility.work; got != uint64(len(source)) {
		t.Fatalf("point byte reservation: charged=%d want=%d", got, len(source))
	}
	limited := witnessTestRun(t, true)
	limited.eligibility.work = eligibilityWorkLimit - with.eligibility.work + 1
	if e := decode(limited, [][]byte{source}); e == nil || e.Kind != KindResourceLimit || e.Path != "reference-points" {
		t.Fatalf("point scan must be reserved before execution: %v", e)
	}
}

func TestTypedJSONWitnessDeterministicMutationsAndTruncation(t *testing.T) {
	docs := queryWitnessStoredDocuments(t)
	ref, err := os.ReadFile("../testdata/query-reference/eq-0.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"raw": docs["raw"], "projection": docs["projection"], "reference": ref} {
		t.Run(name+"-truncation", func(t *testing.T) {
			for i := 0; i < 8; i++ {
				cut := len(data) * i / 8
				witnessParity(t, name, data[:cut], witnessTarget(name), name == "reference")
			}
		})
	}

	seed := bytes.Clone(ref)
	rng := rand.New(rand.NewPCG(362, 4))
	for i := 0; i < 96; i++ {
		t.Run(fmt.Sprintf("reference-byte-mutation-%03d", i), func(t *testing.T) {
			data := bytes.Clone(seed)
			at := rng.IntN(len(data))
			data[at] = byte(rng.Uint32())
			witnessParity(t, "reference-response", data, new(instrumentedReference), true)
		})
	}

	for name, data := range map[string][]byte{
		"trailing-value":      append(bytes.Clone(ref), []byte(` {}`)...),
		"escaped-duplicate":   []byte(`{"schema":"x","\u0073chema":"y"}`),
		"syntax-before-shape": []byte(`{"unexpected":0,"schema":`),
		"unknown-exact":       append(bytes.TrimSuffix(bytes.Clone(ref), []byte("}")), []byte(`,"unknown":0}`)...),
	} {
		t.Run(name, func(t *testing.T) { witnessParity(t, "reference-response", data, new(instrumentedReference), true) })
	}
}

func witnessTarget(name string) any {
	switch name {
	case "raw":
		return new(witnessRaw)
	case "projection":
		return new(witnessProjection)
	default:
		return new(instrumentedReference)
	}
}

func TestTypedJSONReferenceFailedEnvelopePriority(t *testing.T) {
	for _, input := range []struct {
		name, data, wantCode, wantBoundError string
	}{
		{"failed-head-valid-code", `{"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":"CST_LIMIT","complete":false}`, "", ""},
		{"failed-exact-before-code", `{"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":"CST_LIMIT","complete":false,"unknown":0}`, "REFERENCE_RESPONSE_INVALID", "JSON_UNKNOWN_FIELD"},
		{"completed-full-is-exact", `{"schema":"tsgk-instrumented-query-reference/r1","status":"COMPLETED","code":"","complete":true,"unknown":0}`, "REFERENCE_RESPONSE_INVALID", "JSON_UNKNOWN_FIELD"},
		{"failed-syntax-before-code", `{"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":"CST_LIMIT","complete":false,`, "REFERENCE_RESPONSE_INVALID", "JSON_TRUNCATED"},
	} {
		t.Run(input.name, func(t *testing.T) {
			data := []byte(input.data)
			for _, metered := range []bool{false, true} {
				r := witnessTestRun(t, metered)
				_, code, _, e := decodeReferenceResponseBound(data, QueryReferenceObservation{}, "", "", QueryWitnessQuery{}, false, nil, r)
				if metered {
					if input.wantBoundError == "" {
						if e != nil || code != input.wantCode {
							t.Fatalf("metered FAILED-envelope ordering: code=%q err=%v", code, e)
						}
					} else if e == nil || e.Kind != KindInvalidInput || e.Code != input.wantBoundError {
						t.Fatalf("metered FAILED-envelope ordering: code=%q err=%v", code, e)
					}
				} else if e != nil || code != input.wantCode {
					t.Fatalf("standalone FAILED-envelope ordering: code=%q err=%v want %q", code, e, input.wantCode)
				}
			}
		})
	}
}
func FuzzTypedJSONWitness(f *testing.F) {
	docs, err := os.ReadFile("../testdata/query-witness.json")
	if err != nil {
		f.Fatal(err)
	}
	var witness map[string]json.RawMessage
	if err := json.Unmarshal(docs, &witness); err != nil {
		f.Fatal(err)
	}
	for kind, raw := range map[byte][]byte{'r': witness["raw"], 'p': witness["projection"]} {
		f.Add(append([]byte{kind}, raw...))
	}
	ref, err := os.ReadFile("../testdata/query-reference/eq-0.json")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(append([]byte{'f'}, ref...))
	registration, err := jsonv2.Marshal(QueryWitnessRegistration{
		Schema: QueryWitnessRegistrationSchema, Policy: "tsgk-predicates/r1", Case: "c1", Route: "owned-plain", Platform: "windows/amd64", Role: "owned", Encoding: EncodingUTF8,
		Input: NativeInput{Path: "case.txt", Role: "case", Bytes: 1, SHA256: strings.Repeat("a", 64)}, Edits: []Edit{}, Queries: []QueryWitnessQuery{}, Required: []QueryWitnessTuple{},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(append([]byte{'g'}, registration...))
	f.Add([]byte(`r{"status":"FAILED"}`))
	for _, seed := range []string{`r{}`, `p{}`, `g{}`, `f{}`, `t{"query":"q","step":0,"tree_role":"incremental","structural":null,"expected":[]}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) == 0 || len(input) > MaxDocumentBytes+1 {
			return
		}
		target := witnessTarget(string(input[:1]))
		doc := "reference-response"
		exact := true
		switch input[0] {
		case 'r':
			doc, exact = "raw", false
			target = new(witnessRaw)
		case 'p':
			doc, exact = "projection", false
			target = new(witnessProjection)
		case 'g':
			doc = "query-registration"
			target = new(QueryWitnessRegistration)
		case 't':
			doc = "expected-captures"
			target = new(QueryWitnessTuple)
		}
		witnessParity(t, doc, input[1:], target, exact)
	})
}
