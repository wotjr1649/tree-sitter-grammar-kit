package kit

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// membersTextReference is the former membersText: an unstable sort, valid because member paths are unique.
func membersTextReference(ms []ReplayMember) string {
	sorted := slices.Clone(ms)
	slices.SortFunc(sorted, func(a, b ReplayMember) int { return strings.Compare(a.Path, b.Path) })
	var b strings.Builder
	b.WriteString("tsgk-replay-members/r1\n")
	for _, m := range sorted {
		fmt.Fprintf(&b, "%s\x00%s\x00%d\x00%s\n", m.Path, m.Role, m.Bytes, m.SHA256)
	}
	return b.String()
}

// The profile-member reservation must cover what membersText really does: the sort's comparisons, compared bytes
// and moves (counted by the visit hook) and the bytes it writes, with the allocation inside the logical bytes.
func TestReplayMembersChargeCoversWork(t *testing.T) {
	rng := rand.New(rand.NewPCG(362, 7))
	prefix := strings.Repeat("p", 200)
	build := func(n int, path func(int) string) []ReplayMember {
		ms := make([]ReplayMember, n)
		for i := range ms {
			ms[i] = ReplayMember{Path: path(i), Role: "role", Bytes: uint64(i), SHA256: strings.Repeat("a", 64)}
		}
		return ms
	}
	shuffled := func(ms []ReplayMember) []ReplayMember {
		rng.Shuffle(len(ms), func(i, j int) { ms[i], ms[j] = ms[j], ms[i] })
		return ms
	}
	common := func(i int) string { return fmt.Sprintf("%s%06d", prefix, i) }
	reversed := build(3000, common)
	slices.Reverse(reversed)
	cases := map[string][]ReplayMember{
		"empty":            nil,
		"one":              build(1, common),
		"three":            shuffled(build(3, common)),
		"common-prefix":    shuffled(build(3000, common)),
		"already-sorted":   build(3000, common),
		"reversed":         reversed,
		"short-paths":      shuffled(build(5000, func(i int) string { return fmt.Sprintf("%d", i) })),
		"mixed-lengths":    shuffled(build(2000, func(i int) string { return strings.Repeat("m", 1+i%97) + fmt.Sprintf("/%d", i) })),
		"deep-shared-dirs": shuffled(build(1500, func(i int) string { return strings.Repeat("d/", 40) + fmt.Sprintf("%d", i) })),
	}
	for name, members := range cases {
		t.Run(name, func(t *testing.T) {
			b := ordinaryTestBudget(t)
			if e := chargeReplayMembers(b, members); e != nil {
				t.Fatal(e)
			}
			var visits, runs uint64
			prev := ordinaryVisitHook
			ordinaryVisitHook = func(kind string, n uint64) {
				if kind == "members-sort" {
					visits, runs = visits+n, runs+1
				}
			}
			t.Cleanup(func() { ordinaryVisitHook = prev })
			var text string
			alloc := refRowAlloc(func() { text = membersText(members) })
			if text != membersTextReference(members) {
				t.Fatalf("%s: the merge sort changed the member identity text", name)
			}
			if runs == 0 {
				t.Fatalf("%s: the sort did not report its visits", name)
			}
			visits /= runs
			if done := visits + uint64(len(text)); done > b.work {
				t.Fatalf("%s: %d sort visits and %d written bytes exceed the %d work charged", name, visits, len(text), b.work)
			}
			if alloc > b.logical {
				t.Fatalf("%s: %d bytes allocated exceed the %d logical bytes charged", name, alloc, b.logical)
			}
			t.Logf("%s: members=%d visits=%d written=%d work=%d alloc=%d logical=%d", name, len(members), visits, len(text), b.work, alloc, b.logical)
		})
	}
}
