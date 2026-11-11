package kit

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
)

func typedSnapshotParity(t *testing.T, data []byte) {
	t.Helper()
	b := semanticTestBudget(t)
	var old, got map[string]eligibilitySnapshotRow
	wantErr := b.decode("snapshot", data, &old, true)
	b.work, b.logical, b.err = 0, 0, nil
	gotErr := b.decodeTyped("snapshot", data, &got)
	if (wantErr == nil) != (gotErr == nil) || wantErr != nil && (wantErr.Kind != gotErr.Kind || wantErr.Code != gotErr.Code) {
		t.Fatalf("parity: AST=%v typed=%v input=%q", wantErr, gotErr, data[:min(len(data), 300)])
	}
	if wantErr == nil && !reflect.DeepEqual(old, got) {
		t.Fatalf("values: AST=%+v typed=%+v", old, got)
	}
}

func TestTypedJSONSnapshotDifferential(t *testing.T) {
	f := newEligibilityFixture(t)
	for _, pin := range []eligibilityMember{f.reg.Build.SourceSnapshot, f.reg.Build.VendorSnapshot} {
		t.Run(pin.Member.Role, func(t *testing.T) { typedSnapshotParity(t, f.files[pin.Member.Path]) })
	}
	cases := map[string]string{
		"empty": `{}`, "empty-input": ``, "null": `null`, "array": `[]`,
		"normal":            `{"한글/~/a":{"bytes":9007199254740991,"sha256":"한글\\path\u0061"}}`,
		"reordered":         `{"a":{"sha256":"x","bytes":0}}`,
		"duplicate":         `{"a":{"bytes":1,"bytes":2,"sha256":"x"}}`,
		"escaped-duplicate": `{"a":{"bytes":1,"\u0062ytes":2,"sha256":"x"}}`,
		"map-duplicate":     `{"a":{"bytes":1,"sha256":"x"},"\u0061":{}}`,
		"unknown":           `{"a":{"bytes":1,"sha256":"x","other":0}}`,
		"case":              `{"a":{"Bytes":1,"sha256":"x"}}`,
		"missing":           `{"a":{"bytes":1}}`,
		"row-null":          `{"a":null}`, "bytes-null": `{"a":{"bytes":null,"sha256":"x"}}`,
		"string-null":            `{"a":{"bytes":1,"sha256":null}}`,
		"bytes-string":           `{"a":{"bytes":"1","sha256":"x"}}`,
		"string-number":          `{"a":{"bytes":1,"sha256":2}}`,
		"negative":               `{"a":{"bytes":-1,"sha256":"x"}}`,
		"negative-zero":          `{"a":{"bytes":-0,"sha256":"x"}}`,
		"fraction":               `{"a":{"bytes":1.0,"sha256":"x"}}`,
		"exponent":               `{"a":{"bytes":1e0,"sha256":"x"}}`,
		"range":                  `{"a":{"bytes":9007199254740992,"sha256":"x"}}`,
		"leading-zero":           `{"a":{"bytes":01,"sha256":"x"}}`,
		"syntax-before-shape":    `{"a":{"other":0},"b":`,
		"duplicate-before-shape": `{"a":{"other":0},"a":{}}`,
		"surrogate":              `{"a":{"bytes":1,"sha256":"\ud800"}}`,
		"bad-escape":             `{"a":{"bytes":1,"sha256":"\x"}}`,
		"trailing-value":         `{} {}`, "trailing-garbage": `{} x`,
		"depth-at-limit": strings.Repeat(`[`, jsonMaxDepth) + `0` + strings.Repeat(`]`, jsonMaxDepth),
		"depth-over":     strings.Repeat(`[`, jsonMaxDepth+1) + `0` + strings.Repeat(`]`, jsonMaxDepth+1),
		"invalid-utf8":   string([]byte{'"', 0xff, '"'}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) { typedSnapshotParity(t, []byte(data)) })
	}
	seed := []byte(`{"first":{"bytes":12,"sha256":"abc"},"second":{"bytes":0,"sha256":"def"}}`)
	rng := rand.New(rand.NewPCG(362, 3))
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
			typedSnapshotParity(t, data)
		})
	}
}

func TestTypedJSONPaths(t *testing.T) {
	for _, data := range []string{`{"a/~":{"bytes":1}}`, `{"a/~":{"sha256":"x"}}`, `{"a/~":{"bytes":1,"sha256":"x","bad":0}}`, `{"a/~":{"bytes":1.0,"sha256":"x"}}`} {
		b := semanticTestBudget(t)
		var rows map[string]eligibilitySnapshotRow
		old := b.decode("snapshot", []byte(data), &rows, true)
		got := b.decodeTyped("snapshot", []byte(data), &rows)
		if old == nil || got == nil || old.Path != got.Path {
			t.Fatalf("path parity: %v / %v", old, got)
		}
	}
}

func TestTypedJSONFixedRows(t *testing.T) {
	type row struct {
		Name string `json:"name"`
		Size uint64 `json:"size,omitempty"`
	}
	for _, data := range []string{`[]`, `[{"name":"a"},{"name":"b","size":2}]`, `[{"name":"a","size":null}]`, `[{"size":1}]`, `[null]`} {
		b := semanticTestBudget(t)
		var old, got []row
		oldErr := b.decode("rows", []byte(data), &old, true)
		gotErr := b.decodeTyped("rows", []byte(data), &got)
		if (oldErr == nil) != (gotErr == nil) || oldErr != nil && (oldErr.Kind != gotErr.Kind || oldErr.Code != gotErr.Code) || oldErr == nil && !reflect.DeepEqual(old, got) {
			t.Fatalf("fixed rows parity: %s AST=%v typed=%v", data, oldErr, gotErr)
		}
	}
}

func TestTypedJSONLimits(t *testing.T) {
	t.Run("document-at-limit", func(t *testing.T) {
		data := bytes.Repeat([]byte{' '}, int(MaxDocumentBytes))
		copy(data, `{}`)
		typedSnapshotParity(t, data)
	})
	t.Run("document-over", func(t *testing.T) { typedSnapshotParity(t, bytes.Repeat([]byte{' '}, int(MaxDocumentBytes)+1)) })
	t.Run("values-over", func(t *testing.T) {
		data := []byte(`[` + strings.Repeat(`0,`, 1999999) + `0]`)
		b := semanticTestBudget(t)
		var rows map[string]eligibilitySnapshotRow
		old := b.decode("snapshot", data, &rows, true)
		b.work, b.logical, b.err = 0, 0, nil
		got := b.decodeTyped("snapshot", data, &rows)
		if old == nil || got == nil || old.Code != "JSON_VALUE_LIMIT" || got.Code != old.Code {
			t.Fatalf("value cap: %v / %v", old, got)
		}
	})
	t.Run("ledger", func(t *testing.T) {
		data := []byte(`{"a":{"bytes":1,"sha256":"x"}}`)
		b := semanticTestBudget(t)
		var rows map[string]eligibilitySnapshotRow
		if e := b.decodeTyped("snapshot", data, &rows); e != nil {
			t.Fatal(e)
		}
		work, logical := b.work, b.logical
		for _, excess := range []uint64{0, 1} {
			b.work, b.logical = eligibilityWorkLimit-work+excess, 0
			e := b.decodeTyped("snapshot", data, &rows)
			if (e == nil) != (excess == 0) || e != nil && e.Code != "ELIGIBILITY_WORK_LIMIT" {
				t.Fatalf("work boundary: %v", e)
			}
			b.work, b.logical = 0, eligibilityLogicalLimit-logical+excess
			e = b.decodeTyped("snapshot", data, &rows)
			if (e == nil) != (excess == 0) || e != nil && e.Code != "ELIGIBILITY_WORK_LIMIT" {
				t.Fatalf("logical boundary: %v", e)
			}
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		r, e := startRun(ctx, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer r.cancel()
		cancel()
		b := &eligibilityBudget{r: r}
		var rows map[string]eligibilitySnapshotRow
		if e := b.decodeTyped("snapshot", []byte(`{}`), &rows); e == nil || e.Kind != KindCancelled {
			t.Fatalf("cancel: %v", e)
		}
	})
}

func FuzzTypedJSONSnapshot(f *testing.F) {
	for _, seed := range []string{`{}`, `{"a":{"bytes":1,"sha256":"x"}}`, `{"a":{"bytes":1,"bytes":2,"sha256":"x"}}`, `{"a":{"other":1}}`, `null`, `{} []`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) { typedSnapshotParity(t, data) })
}
