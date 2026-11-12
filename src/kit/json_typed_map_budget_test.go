package kit

import (
	"fmt"
	"os"
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

// mapBudgetDoc is {"m":{key: value, ...}} with n distinct keys of at least keyLen bytes.
func mapBudgetDoc(n, keyLen int, value string) []byte {
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

// mapBudgetMeasure decodes data into a fresh target and returns the allocation (GC first, worst of three runs)
// and the logical bytes the decode accounted.
func mapBudgetMeasure(t *testing.T, target any, data []byte, compact bool) (alloc, logical uint64) {
	t.Helper()
	alloc = refRowAlloc(func() {
		fresh := reflect.New(reflect.TypeOf(target).Elem()).Interface()
		b := ordinaryTestBudget(t)
		var e *Error
		if compact {
			_, e = b.decodeOrdinaryExpected("doc", data, fresh)
		} else {
			e = b.decodeTyped("doc", data, fresh)
		}
		if e != nil {
			t.Fatalf("well-formed document rejected: %v", e)
		}
		logical = b.logical
	})
	return alloc, logical
}

// A map entry allocates more than its value: the jsontext namespace of the two token walks and the Go map of the
// unmarshal (mapEntryLogical). Both the ordinary (compact) and the plain typed decode must account at least the
// allocation. Per entry the cost is linear in the entry count: the default run decodes every value type with 3-, 8-
// and 64-byte keys at 2,000 and 10,000 entries and requires both the totals and the marginal cost of the 8,000 extra
// entries ((logical10000-logical2000) / 8000 against the same difference of the allocation) to be accounted, which
// removes the fixed cost and shows the per-entry term scales; short keys use the entries they can hold, and maps
// inside slices are decoded too. The full matrix the term was derived from (100,000 entries, 20,000 and 100,000
// maps in a slice, about 70 seconds on a CI runner) needs TSGK_LONG_BUDGET_TESTS=1; the default run keeps the plain
// decode to 3-byte keys (and 64-byte keys for two values) to stay near five seconds on a CI runner. Before the term reached the plain
// decode, 1,000 to 3,500 entries of 2 to 4 byte keys fell to 0.79-0.9 of the allocation there.
func TestTypedMapEntryAccounting(t *testing.T) {
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
	const small, large = 2000, 10000
	for _, v := range values {
		for _, keyLen := range []int{3, 8, 64} {
			t.Run(fmt.Sprintf("%s/key%d", v.name, keyLen), func(t *testing.T) {
				smallDoc, largeDoc := mapBudgetDoc(small, keyLen, v.value), mapBudgetDoc(large, keyLen, v.value)
				// The plain decode is shortest of the allocation for short keys (key3); the long key is kept for the two
				// values that leave the least room there.
				modes := []bool{true}
				if keyLen == 3 || keyLen == 64 && (v.name == "int" || v.name == "nested-map-empty") {
					modes = append(modes, false)
				}
				for _, compact := range modes {
					aSmall, lSmall := mapBudgetMeasure(t, v.target, smallDoc, compact)
					aLarge, lLarge := mapBudgetMeasure(t, v.target, largeDoc, compact)
					for _, m := range []struct {
						n        int
						alloc, l uint64
					}{{small, aSmall, lSmall}, {large, aLarge, lLarge}} {
						if m.l < m.alloc {
							t.Fatalf("compact=%t %d entries: accounted logical %d below the %d bytes allocated", compact, m.n, m.l, m.alloc)
						}
					}
					marginalAlloc, marginalLogical := (aLarge-aSmall)/(large-small), (lLarge-lSmall)/(large-small)
					if marginalLogical < marginalAlloc {
						t.Fatalf("compact=%t: the %d extra entries cost %d B each but were accounted %d B each", compact, large-small, marginalAlloc, marginalLogical)
					}
					t.Logf("compact=%t: %d entries alloc %d logical %d (%.2fx); %d entries alloc %d logical %d (%.2fx)", compact,
						small, aSmall, lSmall, float64(lSmall)/float64(max(aSmall, 1)), large, aLarge, lLarge, float64(lLarge)/float64(max(aLarge, 1)))
				}
			})
		}
		// Short keys need fewer entries to stay distinct.
		t.Run(v.name+"/short-keys", func(t *testing.T) {
			for _, c := range []struct{ n, keyLen int }{{3000, 2}, {60, 1}} {
				for _, compact := range []bool{true, false} {
					if alloc, logical := mapBudgetMeasure(t, v.target, mapBudgetDoc(c.n, c.keyLen, v.value), compact); logical < alloc {
						t.Fatalf("compact=%t %d entries of %d-byte keys: accounted logical %d below the %d bytes allocated", compact, c.n, c.keyLen, logical, alloc)
					}
				}
			}
		})
	}
	inSlice := func(n int) []byte {
		return []byte(`{"m":[` + strings.TrimSuffix(strings.Repeat(`{"a":0},`, n), ",") + `]}`)
	}
	t.Run("map-in-slice", func(t *testing.T) {
		for _, compact := range []bool{true, false} {
			for _, n := range []int{small, large} {
				if alloc, logical := mapBudgetMeasure(t, new(mapBudgetInSlice), inSlice(n), compact); logical < alloc {
					t.Fatalf("compact=%t %d maps: accounted logical %d below the %d bytes allocated", compact, n, logical, alloc)
				}
			}
		}
	})
	if os.Getenv("TSGK_LONG_BUDGET_TESTS") != "1" {
		t.Log("set TSGK_LONG_BUDGET_TESTS=1 for the 100,000-entry matrix")
		return
	}
	for _, v := range values {
		for _, keyLen := range []int{3, 8, 64} {
			t.Run(fmt.Sprintf("long/%s/100000x%d", v.name, keyLen), func(t *testing.T) {
				data := mapBudgetDoc(100000, keyLen, v.value)
				for _, compact := range []bool{true, false} {
					if alloc, logical := mapBudgetMeasure(t, v.target, data, compact); logical < alloc {
						t.Fatalf("compact=%t: %d bytes accounted logical %d below the %d bytes allocated", compact, len(data), logical, alloc)
					}
				}
			})
		}
	}
	for _, n := range []int{20000, 100000} {
		t.Run(fmt.Sprintf("long/map-in-slice/%d", n), func(t *testing.T) {
			for _, compact := range []bool{true, false} {
				if alloc, logical := mapBudgetMeasure(t, new(mapBudgetInSlice), inSlice(n), compact); logical < alloc {
					t.Fatalf("compact=%t: accounted logical %d below the %d bytes allocated", compact, logical, alloc)
				}
			}
		})
	}
}
