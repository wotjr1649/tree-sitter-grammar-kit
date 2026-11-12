package kit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type mapBudgetStruct struct {
	A string `json:"a"`
	B int64  `json:"b"`
}

type (
	mapBudgetInt struct {
		M map[string]int `json:"m"`
	}
	mapBudgetBool struct {
		M map[string]bool `json:"m"`
	}
	mapBudgetStr struct {
		M map[string]string `json:"m"`
	}
	mapBudgetSlice struct {
		M map[string][]string `json:"m"`
	}
	mapBudgetStruct1 struct {
		M map[string]mapBudgetStruct `json:"m"`
	}
	mapBudgetPtr struct {
		M map[string]*mapBudgetStruct `json:"m"`
	}
	mapBudgetNested struct {
		M map[string]map[string]int `json:"m"`
	}
	mapBudgetInts struct {
		M map[string][]int64 `json:"m"`
	}
	mapBudgetSliceMaps struct {
		M map[string][]map[string]int `json:"m"`
	}
	mapBudgetTriple struct {
		M map[string]map[string]map[string]int `json:"m"`
	}
	mapBudgetHolder struct {
		M map[string]struct {
			M map[string]int `json:"m"`
		} `json:"m"`
	}
	mapBudgetInSlice struct {
		M []map[string]int `json:"m"`
	}
)

// mapBudgetKey is the i-th distinct key: base 62, at least keyLen bytes (0: the shortest distinct key).
func mapBudgetKey(i, keyLen int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	k := []byte{digits[i%62]}
	for i /= 62; i > 0; i /= 62 {
		k = append(k, digits[i%62])
	}
	if len(k) < keyLen {
		k = append(k, strings.Repeat("_", keyLen-len(k))...)
	}
	return string(k)
}

// A map entry allocates more than its value: the jsontext namespace of the two token walks and the Go map of the
// unmarshal (compactMapEntryLogical). The ordinary decode must account at least the allocation for the worst
// inputs measured: 100,000 entries with 3-, 8- and 64-byte keys and values of different sizes, short keys with fewer
// entries, and maps inside slices and maps. The plain (non-compact) decode is checked as a floor.
func TestCompactMapEntryAccounting(t *testing.T) {
	obj := func(n, keyLen int, value string) []byte {
		var sb strings.Builder
		sb.WriteString(`{"m":{`)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `"%s":%s`, mapBudgetKey(i, keyLen), value)
		}
		sb.WriteString(`}}`)
		return []byte(sb.String())
	}
	type tc struct {
		name   string
		target any
		value  string
	}
	values := []tc{
		{"int", new(mapBudgetInt), "0"},
		{"bool", new(mapBudgetBool), "true"},
		{"empty-string", new(mapBudgetStr), `""`},
		{"string-16", new(mapBudgetStr), `"` + strings.Repeat("v", 16) + `"`},
		{"empty-slice", new(mapBudgetSlice), `[]`},
		{"slice-2", new(mapBudgetSlice), `["",""]`},
		{"struct", new(mapBudgetStruct1), `{"a":"","b":0}`},
		{"pointer", new(mapBudgetPtr), `{"a":"","b":0}`},
		{"nested-map-1", new(mapBudgetNested), `{"a":0}`},
		{"nested-map-empty", new(mapBudgetNested), `{}`},
		{"int-slice-1", new(mapBudgetInts), `[0]`},
		{"slice-of-empty-maps", new(mapBudgetSliceMaps), `[{}]`},
		{"triple-nested-map", new(mapBudgetTriple), `{"a":{}}`},
		{"struct-with-empty-map", new(mapBudgetHolder), `{"m":{}}`},
	}
	type input struct {
		name   string
		target any
		data   []byte
	}
	var inputs []input
	for _, v := range values {
		for _, keyLen := range []int{3, 8, 64} {
			inputs = append(inputs, input{fmt.Sprintf("%s/100000x%d", v.name, keyLen), v.target, obj(100000, keyLen, v.value)})
		}
		// Short keys need fewer entries to stay distinct.
		inputs = append(inputs, input{v.name + "/3000x2", v.target, obj(3000, 2, v.value)}, input{v.name + "/60x1", v.target, obj(60, 1, v.value)})
	}
	for _, n := range []int{20000, 100000} {
		var sb strings.Builder
		sb.WriteString(`{"m":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(`{"a":0}`)
		}
		sb.WriteString(`]}`)
		inputs = append(inputs, input{fmt.Sprintf("map-in-slice/%d", n), new(mapBudgetInSlice), []byte(sb.String())})
	}
	for _, in := range inputs {
		for _, compact := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/compact=%t", in.name, compact), func(t *testing.T) {
				var logical uint64
				var e *Error
				alloc := refRowAlloc(func() {
					target := reflect.New(reflect.TypeOf(in.target).Elem()).Interface()
					b := ordinaryTestBudget(t)
					if compact {
						_, e = b.decodeOrdinaryExpected("doc", in.data, target)
					} else {
						e = b.decodeTyped("doc", in.data, target)
					}
					logical = b.logical
				})
				if e != nil {
					t.Fatalf("well-formed document rejected: %v", e)
				}
				if logical < alloc {
					t.Fatalf("%d bytes accounted logical %d below the %d bytes allocated", len(in.data), logical, alloc)
				}
				t.Logf("bytes=%d alloc=%d logical=%d (%.2fx)", len(in.data), alloc, logical, float64(logical)/float64(max(alloc, 1)))
			})
		}
	}
}
