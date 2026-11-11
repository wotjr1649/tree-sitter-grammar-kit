package kit

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// memberIndexModel counts the operations ordinaryGuardedSet performs to build its three member maps.
// Unit: one visit per member and per map operation, one per byte of a byte loop (ToLower) and one per
// 8-byte word of a map key hash. The model does not read ordinaryMemberIndexReservation.
func memberIndexModel(paths []string) (work, logical uint64) {
	work, logical = 2+2*2, 2*(16+1)*2 // the two manifest.json seed entries
	for _, p := range paths {
		n := uint64(len(p))
		words := (n + 7) / 8
		work += 1 + 4 + n + 4*words // visit, four map operations, ToLower, four key hashes
		// folded and listed entries (string header + bool), the member entry (string header + value),
		// each doubled for map load factor, plus the lowered key bytes.
		logical += 2*2*(16+1) + 2*(16+uint64(unsafe.Sizeof(OrdinaryMember{}))) + n
	}
	return work, logical
}

func TestOrdinaryMemberIndexReservationCoversMaps(t *testing.T) {
	for _, tc := range []struct{ count, length int }{{1, 1}, {1, 4096}, {16, 12}, {1000, 40}, {10000, 6}} {
		paths := make([]string, tc.count)
		var pathBytes uint64
		for i := range paths {
			num := strconv.Itoa(i)
			paths[i] = "Q" + strings.Repeat("x", max(tc.length-1-len(num), 0)) + num // upper case so ToLower allocates
			pathBytes += uint64(len(paths[i]))
		}
		work, logical := ordinaryMemberIndexReservation(uint64(len(paths)), pathBytes)
		modelWork, modelLogical := memberIndexModel(paths)
		if work < modelWork || logical < modelLogical {
			t.Fatalf("member index reservation below modeled traversal: reserved %d/%d model %d/%d", work, logical, modelWork, modelLogical)
		}
		if tc.count < 1000 {
			continue
		}
		// Measured allocation of the same three maps, including the lowered key copies.
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		memberByPath := make(map[string]OrdinaryMember, len(paths))
		listed := map[string]bool{"manifest.json": true}
		folded := map[string]bool{"manifest.json": true}
		for _, p := range paths {
			folded[strings.ToLower(p)], listed[p] = true, true
			memberByPath[p] = OrdinaryMember{Path: p}
		}
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(memberByPath)
		runtime.KeepAlive(listed)
		runtime.KeepAlive(folded)
		if actual := after.TotalAlloc - before.TotalAlloc; actual > logical {
			t.Fatalf("member index logical reservation %d below measured allocation %d for %d members", logical, actual, tc.count)
		}
	}
}
