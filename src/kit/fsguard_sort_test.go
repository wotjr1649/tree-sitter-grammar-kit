package kit

import (
	"io/fs"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type sortTestEntry string

func (e sortTestEntry) Name() string               { return string(e) }
func (e sortTestEntry) IsDir() bool                { return false }
func (e sortTestEntry) Type() fs.FileMode          { return 0 }
func (e sortTestEntry) Info() (fs.FileInfo, error) { return nil, nil }

// The directory sort is charged ordinarySortWork before it runs: what it really did must stay below that.
func TestSortEntriesVisitsStayWithinTheChargedWork(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	prefix := strings.Repeat("p", 200)
	shapes := map[string]func(i, n int) string{
		"ascending": func(i, n int) string {
			return prefix + string(rune('a'+i%26)) + strings.Repeat("x", i%7) + "-" + strconv.Itoa(i)
		},
		"descending":  func(i, n int) string { return prefix + strconv.Itoa(n-i) },
		"random":      func(i, n int) string { return strconv.Itoa(rng.Intn(1<<30)) + "-" + strconv.Itoa(i) },
		"long-common": func(i, n int) string { return strings.Repeat("c", 4000) + strconv.Itoa(i) },
		"short-names": func(i, n int) string { return strconv.Itoa(i) },
		"one-long-rest": func(i, n int) string {
			return map[bool]string{true: strings.Repeat("z", 255), false: "a"}[i == 0] + strconv.Itoa(i)
		},
	}
	for name, shape := range shapes {
		for _, n := range []int{0, 1, 2, 3, 7, 64, 255, 256, 257, 1000} {
			entries := sortTestEntries(n, func(i int) string { return shape(i, n) })
			want := slices.Clone(entries)
			slices.SortFunc(want, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
			var fields uint64
			for _, e := range entries {
				fields += uint64(len(e.Name())) + 1
			}
			visits := sortEntries(entries)
			if !slices.EqualFunc(entries, want, func(a, b fs.DirEntry) bool { return a.Name() == b.Name() }) {
				t.Fatalf("%s n=%d: not in byte order", name, n)
			}
			if bound := ordinarySortWork(n, fields); visits > bound {
				t.Fatalf("%s n=%d: sort made %d visits for %d charged", name, n, visits, bound)
			}
		}
	}
}

func sortTestEntries(n int, name func(i int) string) []fs.DirEntry {
	out := make([]fs.DirEntry, n)
	for i := range out {
		out[i] = sortTestEntry(name(i))
	}
	return out
}
