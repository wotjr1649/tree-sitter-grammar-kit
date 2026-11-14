package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// #427: a guarded read that keeps nothing and feeds only a hash reserves the chunk buffers it can
// really allocate, not 3x the file. Every other read keeps the full per-pass reserve.
func TestGuardedReadLogicalReserve(t *testing.T) {
	const large = uint64(16 << 20)
	chain := &streamedChain{Hash: sha256.New()}
	foreign := sha256.New()
	capped := func(passes uint64) uint64 { return passes*(readChunk+1) + readStreamFixedLogical }
	for _, c := range []struct {
		name   string
		size   uint64
		keep   uint64
		detect bool
		sinks  []sink
		want   uint64
	}{
		{"hash only, nothing kept, large", large, 0, false, nil, capped(2)},
		{"streamed chain, large", large, 0, false, []sink{chain}, capped(3)},
		{"two streamed chains, large", large, 0, false, []sink{chain, chain}, capped(4)},
		{"streamed chain, small file keeps the full reserve", 100, 0, false, []sink{chain}, 3 * 101},
		{"streamed chain, one chunk keeps the full reserve", readChunk, 0, false, []sink{chain}, 3 * (readChunk + 1)},
		{"streamed chain, empty file", 0, 0, false, []sink{chain}, 3},
		{"retained copy keeps the full reserve", large, large + 1, false, nil, 3 * (large + 1)},
		{"retained copy beside a chain", large, large + 1, false, []sink{chain}, 4 * (large + 1)},
		{"encoding detector keeps the full reserve", large, 0, true, []sink{chain}, 4 * (large + 1)},
		{"foreign sink keeps the full reserve", large, 0, false, []sink{foreign}, 3 * (large + 1)},
		{"foreign sink beside a chain", large, 0, false, []sink{chain, foreign}, 4 * (large + 1)},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, e := guardedReadLogical("input", c.size, c.keep, c.detect, c.sinks)
			if e != nil || got != c.want {
				t.Fatalf("guardedReadLogical = %d, %v; want %d", got, e, c.want)
			}
			passes := uint64(2 + len(c.sinks))
			if c.detect {
				passes++
			}
			if c.keep > 0 {
				passes++
			}
			if old := (c.size + 1) * passes; got > old {
				t.Fatalf("reserve %d exceeds the previous reserve %d", got, old)
			}
		})
	}
	if _, e := guardedReadLogical("input", ^uint64(0)-1, 0, false, nil); e == nil {
		t.Fatal("an overflowing size must be rejected, not wrapped")
	}
}

// The cap is an upper bound of the real cumulative allocation (TotalAlloc), adversarially for the
// sizes around the chunk boundary and for files many chunks long.
func TestGuardedStreamReadAllocationWithinReserve(t *testing.T) {
	for _, size := range []int{0, 1, readChunk - 1, readChunk, readChunk + 1, 2*readChunk + 37, 16 << 20} {
		for _, withChain := range []bool{false, true} {
			dir := t.TempDir()
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i*131 + 17)
			}
			if err := os.WriteFile(filepath.Join(dir, "input"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			root := semanticBudgetRoot(t, dir)
			r := semanticBudgetRun(t)
			r.eligibility = &eligibilityBudget{r: r}
			r.readCeiling = uint64(4*size + 4)
			guard := newGuard(root)
			before, err := root.Lstat("input")
			if err != nil {
				t.Fatal(err)
			}
			var sinks []sink
			var chain *streamedChain
			if withChain {
				chain = &streamedChain{Hash: sha256.New(), pins: []ReplayMember{{}}}
				sinks = []sink{chain}
			}
			want := sha256.Sum256(data)

			var m0, m1 runtime.MemStats
			runtime.ReadMemStats(&m0)
			stats, e := guard.readFileContent(r, "input", before, uint64(size)+1, uint64(4*size+4), 0, false, false, sinks...)
			runtime.ReadMemStats(&m1)
			if e != nil || stats.size != uint64(size) || stats.content != nil || stats.sha256 != hex.EncodeToString(want[:]) {
				t.Fatalf("size %d chain %v: stats=%+v err=%v", size, withChain, stats, e)
			}
			if withChain && hex.EncodeToString(chain.Sum(nil)) != stats.sha256 {
				t.Fatalf("size %d: chain hash differs from the read hash", size)
			}
			charged, alloc := r.eligibility.logical, m1.TotalAlloc-m0.TotalAlloc
			// Only the size-proportional part is reserved here; per-file structures of small reads are
			// covered by the entry and work charges, so the bound is checked where the cap applies.
			if size >= 2*readChunk {
				if alloc > charged {
					t.Fatalf("size %d chain %v: allocated %d B > reserved %d B", size, withChain, alloc, charged)
				}
				if old := uint64(size+1) * uint64(2+len(sinks)); charged >= old {
					t.Fatalf("size %d chain %v: reserve %d did not shrink below the previous %d", size, withChain, charged, old)
				}
				if limit := uint64(3*(readChunk+1) + readStreamFixedLogical); charged > limit {
					t.Fatalf("size %d chain %v: reserve %d is above the cap %d", size, withChain, charged, limit)
				}
			}
			if size < readChunk {
				if old := uint64(size+1) * uint64(2+len(sinks)); charged != old {
					t.Fatalf("size %d chain %v: small read reserve %d, want the unchanged %d", size, withChain, charged, old)
				}
			}
		}
	}
}
