package kit

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

func typedNativeParity(t *testing.T, data []byte) {
	t.Helper()
	b := semanticTestBudget(t)
	var old, got nativewire.Response
	want := b.reserveJSON("native", data, 12)
	if want == nil {
		want = b.decode("native", data, &old, true)
	}
	b.work, b.logical, b.err = 0, 0, nil
	e := b.decodeNativeResponse("native", data, &got)
	if (want == nil) != (e == nil) || want != nil && (want.Kind != e.Kind || want.Code != e.Code) {
		t.Fatalf("native parity: AST=%v typed=%v input=%q", want, e, data[:min(300, len(data))])
	}
	if want == nil && !reflect.DeepEqual(old, got) {
		t.Fatal("native decoded values differ")
	}
}

func TestTypedJSONNativeDifferential(t *testing.T) {
	f := newEligibilityFixture(t)
	seed := f.files["native/response.json"]
	typedNativeParity(t, seed)
	if err := filepath.WalkDir("../testdata", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.Contains(filepath.ToSlash(path), "/responses/") && strings.HasSuffix(path, ".json") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			t.Run(filepath.ToSlash(path), func(t *testing.T) { typedNativeParity(t, data) })
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"syntax-before-shape":       []byte(`{"other":0,"id":`),
		"duplicate-before-shape":    []byte(`{"other":0,"id":1,"\u0069d":2}`),
		"trailing-value":            append(bytes.Clone(seed), []byte(` {}`)...),
		"invalid-utf8":              append(bytes.Clone(seed), 0xff),
		"primitive-null":            bytes.Replace(seed, []byte(`"complete":true`), []byte(`"complete":null`), 1),
		"negative-zero":             bytes.Replace(seed, []byte(`"source_bytes":6`), []byte(`"source_bytes":-0`), 1),
		"fraction":                  bytes.Replace(seed, []byte(`"source_bytes":6`), []byte(`"source_bytes":6.0`), 1),
		"exponent":                  bytes.Replace(seed, []byte(`"source_bytes":6`), []byte(`"source_bytes":6e0`), 1),
		"uint32-overflow":           bytes.Replace(seed, []byte(`"language_version":15`), []byte(`"language_version":4294967296`), 1),
		"integer-range":             bytes.Replace(seed, []byte(`"source_bytes":6`), []byte(`"source_bytes":9007199254740992`), 1),
		"int-overflow":              bytes.Replace(seed, []byte(`"step":0`), []byte(`"step":9223372036854775808`), 1),
		"int-fraction":              bytes.Replace(seed, []byte(`"step":0`), []byte(`"step":0.0`), 1),
		"point-length-before-null":  bytes.Replace(seed, []byte(`"start_point":[0,0]`), []byte(`"start_point":[null]`), 1),
		"point-element-null":        bytes.Replace(seed, []byte(`"start_point":[0,0]`), []byte(`"start_point":[null,2]`), 1),
		"nullable-element-rejected": bytes.Replace(seed, []byte(`"types":[`), []byte(`"types":[null,`), 1),
		"unknown-before-missing":    []byte(`{"other":0}`),
		"depth":                     []byte(strings.Repeat(`[`, jsonMaxDepth+1) + `0` + strings.Repeat(`]`, jsonMaxDepth+1)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(data, seed) {
				t.Fatal("mutation did not change fixture")
			}
			typedNativeParity(t, data)
		})
	}
	rng := rand.New(rand.NewPCG(362, 1))
	for i := 0; i < 300; i++ {
		t.Run(fmt.Sprintf("mutation-%03d", i), func(t *testing.T) {
			data := bytes.Clone(seed)
			switch i % 3 {
			case 0:
				data = data[:rng.IntN(len(data))]
			case 1:
				data[rng.IntN(len(data))] = byte(rng.Uint32())
			case 2:
				at := rng.IntN(len(data))
				data = append(append(bytes.Clone(data[:at]), data[at]), data[at:]...)
			}
			typedNativeParity(t, data)
		})
	}
}

func TestTypedJSONNativeEnvelope(t *testing.T) {
	// Conditional arms must be selected only after member-order shape validation.
	for _, data := range []string{
		`{"status":"FAILED","code":"x","parse_ms":0,"form":null,"queries":null,"api":null}`,
		`{"queries":null,"api":null,"form":null,"parse_ms":0,"code":"x","status":""}`,
		`{"status":"FAILED","code":"x","parse_ms":0,"form":null,"queries":null,"api":null,"nodes":false}`,
		`{"status":"FAILED","code":"x","parse_ms":0,"form":null,"queries":null}`,
		`{"form":"full","status":"COMPLETED","code":"","parse_ms":0,"descendant_count":0,"max_depth":0,"has_error":false,"digest":"","types":[],"fields":[],"nodes":[],"declarations":null,"queries":null,"api":null}`,
		`{"form":"summary","status":"COMPLETED","code":"","parse_ms":0,"queries":null,"api":null}`,
	} {
		b := semanticTestBudget(t)
		var old, got nativewire.WireTree
		want := b.decode("tree", []byte(data), &old, true)
		b.work, b.logical, b.err = 0, 0, nil
		e := b.decodeTyped("tree", []byte(data), &got)
		if (want == nil) != (e == nil) || want != nil && (want.Kind != e.Kind || want.Code != e.Code) || want == nil && !reflect.DeepEqual(old, got) {
			t.Fatalf("envelope parity: AST=%v typed=%v input=%s", want, e, data)
		}
	}
}

func TestTypedJSONNativeCheckReservation(t *testing.T) {
	f := newEligibilityFixture(t)
	data := f.files["native/response.json"]
	b := semanticTestBudget(t)
	var raw nativewire.Response
	if e := b.decodeTyped("native", data, &raw); e != nil {
		t.Fatal(e)
	}
	work, logical := b.work, b.logical
	var v any
	if err := jsonv2.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	var count func(any) uint64
	count = func(v any) uint64 {
		n := uint64(1)
		switch v := v.(type) {
		case map[string]any:
			for _, x := range v {
				n += count(x)
			}
		case []any:
			for _, x := range v {
				n += count(x)
			}
		}
		return n
	}
	b.work, b.logical, b.err = 0, 0, nil
	if e := b.decodeNativeResponse("native", data, &raw); e != nil {
		t.Fatal(e)
	}
	if b.work-work != 12*count(v) || b.logical-logical != 12*uint64(len(data)) {
		t.Fatalf("Check reservation changed: work=%d logical=%d", b.work-work, b.logical-logical)
	}
	work, logical = b.work, b.logical
	for _, excess := range []uint64{0, 1} {
		b.work, b.logical, b.err = eligibilityWorkLimit-work+excess, 0, nil
		e := b.decodeNativeResponse("native", data, &raw)
		if (e == nil) != (excess == 0) {
			t.Fatalf("work boundary: %v", e)
		}
		b.work, b.logical, b.err = 0, eligibilityLogicalLimit-logical+excess, nil
		e = b.decodeNativeResponse("native", data, &raw)
		if (e == nil) != (excess == 0) {
			t.Fatalf("logical boundary: %v", e)
		}
	}
}

func TestTypedJSONNativePaths(t *testing.T) {
	for _, data := range []string{
		`{"language_version":0,"runtime_language_version":0,"runtime_min_compatible":0,"query":"","api":null}`,
		`{"start_byte":0,"old_end_byte":0,"new_end_byte":0,"start_point":[null],"old_end_point":[0,0],"new_end_point":[0,0]}`,
		`{"start_byte":0,"old_end_byte":0,"new_end_byte":0,"start_point":[null,0],"old_end_point":[0,0],"new_end_point":[0,0]}`,
	} {
		b := semanticTestBudget(t)
		var target any = &nativewire.StepEdit{}
		if strings.Contains(data, "language_version") {
			target = &nativewire.Producer{}
		}
		want := b.decode("native", []byte(data), target, true)
		got := b.decodeTyped("native", []byte(data), target)
		if want == nil || got == nil || want.Kind != got.Kind || want.Code != got.Code || want.Path != got.Path {
			t.Fatalf("native path parity: AST=%v typed=%v", want, got)
		}
	}
}

type typedArrayContext struct {
	context.Context
	checks int
	cancel context.CancelFunc
}

func (c *typedArrayContext) Err() error {
	c.checks++
	if c.checks == 5 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestTypedJSONNativeArrayInterruption(t *testing.T) {
	for _, mode := range []string{"work", "caller", "wall"} {
		t.Run(mode, func(t *testing.T) {
			b := semanticTestBudget(t)
			shape, e := b.typedJSONShape("array", reflect.TypeFor[[2]uint32]())
			if e != nil {
				t.Fatal(e)
			}
			b.work, b.logical, b.err = 0, 0, nil
			kind, code := KindResourceLimit, "ELIGIBILITY_WORK_LIMIT"
			if mode == "work" {
				b.work = eligibilityWorkLimit - 6
			} else {
				ctx, cancel := context.WithCancel(b.r.caller)
				defer cancel()
				if mode == "caller" {
					b.r.caller = &typedArrayContext{Context: ctx, cancel: cancel}
					kind, code = KindCancelled, "CANCELLED"
				} else {
					wall, stop := context.WithCancelCause(context.Background())
					defer stop(nil)
					b.r.wall = wall
					b.r.caller = &typedArrayContext{Context: ctx, cancel: func() { stop(errWall) }}
					code = "WALL_LIMIT"
				}
			}
			dec := jsontext.NewDecoder(bytes.NewBufferString(`[0,0]`))
			var values uint64
			e = b.readTypedJSON("array", dec, shape, &values)
			if e == nil || e.Kind != kind || e.Code != code {
				t.Fatalf("array interruption changed: got %v want %v/%s", e, kind, code)
			}
		})
	}
}

func FuzzTypedJSONNative(f *testing.F) {
	data, err := os.ReadFile("../testdata/query-witness.json")
	if err != nil {
		f.Fatal(err)
	}
	var fixture map[string]jsontext.Value
	if err := jsonv2.Unmarshal(data, &fixture); err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(fixture["raw"]))
	api, predicates := "tsgk-api/r1", "NOT_EVALUATED"
	count := uint32(0)
	seed, err := jsonv2.Marshal(nativewire.Response{Producer: nativewire.Producer{API: &api, Predicates: &predicates, SymbolCount: &count, FieldCount: &count, ReuseObservation: []byte(`null`)}, Steps: []nativewire.Step{{Observations: []byte(`[]`)}}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	for _, data := range []string{`null`, `{}`, `{"steps":[]}`, `{"other":0,"id":1,"\u0069d":2}`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) { typedNativeParity(t, data) })
}
