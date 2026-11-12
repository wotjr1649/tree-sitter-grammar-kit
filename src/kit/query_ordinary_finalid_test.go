package kit

import (
	"fmt"
	"math/bits"
	"math/rand"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// finalIDPathSets are path lists for the final-identity sort, including the adversarial shapes:
// a very long shared prefix (every comparison runs to the end of the prefix), already sorted and
// reverse order, duplicates, empty strings, one huge path and very many short ones.
func finalIDPathSets() map[string][]string {
	rnd := rand.New(rand.NewSource(7))
	prefix := strings.Repeat("oracle/very-long-directory-name/", 60)
	numbered := func(n int, prefix string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s%06d", prefix, i)
		}
		return out
	}
	shuffled := func(in []string) []string {
		out := slices.Clone(in)
		rnd.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}
	reversed := numbered(3000, prefix)
	slices.Reverse(reversed)
	short := make([]string, 100000)
	for i := range short {
		short[i] = fmt.Sprintf("%x", rnd.Intn(1<<20))
	}
	return map[string][]string{
		"empty":             nil,
		"one":               {"a"},
		"two-equal-empty":   {"", ""},
		"three":             {"b", "a", "c"},
		"duplicates":        {"x", "y", "x", "y", "x", "", "y"},
		"913-real-shape":    shuffled(numbered(913, "oracle/capture/response-000000000000000000000000/")),
		"long-prefix-rand":  shuffled(numbered(3000, prefix)),
		"long-prefix-sort":  numbered(3000, prefix),
		"long-prefix-rev":   reversed,
		"one-huge-path":     {strings.Repeat("z", 1<<20), "a", strings.Repeat("z", 1<<20-1) + "y"},
		"100000-short":      short,
		"pow2":              shuffled(numbered(1024, "d/")),
		"pow2-plus-one":     shuffled(numbered(1025, "d/")),
		"all-empty-pow2":    make([]string, 4096),
		"many-equal-prefix": shuffled(slices.Repeat([]string{prefix + "same"}, 500)),
	}
}

func finalIDPathBytes(paths []string) (n uint64) {
	for _, p := range paths {
		n += uint64(len(p) + 1)
	}
	return n
}

// The merge sort returns the order slices.Sort does, and its comparisons, compared bytes and moves
// stay within the work the final-identity charge reserves. The charge replaces the old
// 8*bits.Len(n)*pathBytes, which it never exceeds.
func TestOrdinarySortPathsStaysWithinChargedWork(t *testing.T) {
	for name, paths := range finalIDPathSets() {
		t.Run(name, func(t *testing.T) {
			want := slices.Clone(paths)
			slices.Sort(want)
			got := slices.Clone(paths)
			var cost ordinarySortCost
			withSort := refRowAlloc(func() {
				again := slices.Clone(paths)
				ordinarySortPaths(again)
			})
			cloneOnly := refRowAlloc(func() { runtime.KeepAlive(slices.Clone(paths)) })
			cost = ordinarySortPaths(got)
			if !slices.Equal(got, want) {
				t.Fatal("not sorted like slices.Sort")
			}
			n, pathBytes := len(paths), finalIDPathBytes(paths)
			work := ordinarySortWork(n, pathBytes)
			spent := cost.compares + cost.bytes + cost.moved
			if spent+1 > work {
				t.Fatalf("n=%d pathBytes=%d: sort spent %d (compares %d, bytes %d, moved %d) over the reserved %d", n, pathBytes, spent, cost.compares, cost.bytes, cost.moved, work)
			}
			if n >= 2 {
				levels := uint64(bits.Len(uint(n - 1)))
				if cost.compares > levels*uint64(n) {
					t.Fatalf("%d comparisons exceed n*ceil(log2 n) = %d", cost.compares, levels*uint64(n))
				}
			}
			// What the sort allocates beyond its input copy is at most the logical bytes charged for it.
			if charged := ordinarySortLogical(n); withSort > cloneOnly+charged {
				t.Fatalf("the sort allocated %d bytes (clone alone %d), more than the %d charged", withSort, cloneOnly, charged)
			}
			if old := 8*uint64(bits.Len(uint(n)))*pathBytes + 1; work > old {
				t.Fatalf("reserved %d, more than the previous bound %d", work, old)
			}
			t.Logf("n=%d pathBytes=%d reserved=%d (old %d) spent=%d compares=%d bytes=%d moved=%d", n, pathBytes, work, 8*uint64(bits.Len(uint(n)))*pathBytes+1, spent, cost.compares, cost.bytes, cost.moved)
		})
	}
}

// The final identity of a snapshot with very many long paths charges the order at least what the
// sort did and the buffer it allocated, and keeps rejecting a changed file.
func TestOrdinaryFinalIdentityOrderChargeCoversSortAndBuffer(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	const dir = "big/dir-with-a-long-name-to-share-a-long-prefix-in-every-path"
	if err := f.root.Mkdir("big", 0700); err != nil {
		t.Fatal(err)
	}
	if err := f.root.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"big", dir} {
		info, err := f.root.Lstat(d)
		if err != nil {
			t.Fatal(err)
		}
		f.s.snapshot.directories[d] = info
	}
	for i := 0; i < 1500; i++ {
		path := fmt.Sprintf("%s/%s%05d", dir, strings.Repeat("n", 120), i)
		file, err := f.root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if f.s.snapshot.observed[path], err = f.root.Lstat(path); err != nil {
			t.Fatal(err)
		}
	}
	var paths []string
	for p := range f.s.snapshot.directories {
		paths = append(paths, p)
	}
	for p := range f.s.snapshot.observed {
		paths = append(paths, p)
	}
	prevCharge, prevVisit := ordinaryChargeHook, ordinaryVisitHook
	var orderWork, orderLogical, spent uint64
	ordinaryChargeHook = func(_ *eligibilityBudget, work, logical uint64, at string, _, _ uint64) {
		if at == "ordinary-final-identity-order" {
			orderWork, orderLogical = work, logical
		}
	}
	ordinaryVisitHook = func(kind string, visits uint64) {
		if kind == "sort" {
			spent = visits
		}
	}
	t.Cleanup(func() { ordinaryChargeHook, ordinaryVisitHook = prevCharge, prevVisit })
	if e := f.b.ordinarySnapshotIdentity(f.root, f.s); e != nil {
		t.Fatal(e)
	}
	if orderWork == 0 || spent == 0 || spent >= orderWork {
		t.Fatalf("order charged work %d for a sort that spent %d", orderWork, spent)
	}
	if want := ordinarySortWork(len(paths), finalIDPathBytes(paths)); orderWork != want {
		t.Fatalf("order charged work %d, want ordinarySortWork %d", orderWork, want)
	}
	withSort := refRowAlloc(func() { ordinarySortPaths(slices.Clone(paths)) })
	cloneOnly := refRowAlloc(func() { runtime.KeepAlive(slices.Clone(paths)) })
	if orderLogical != ordinarySortLogical(len(paths)) || withSort > cloneOnly+orderLogical {
		t.Fatalf("sort allocated %d bytes (clone alone %d), more than the %d logical charged", withSort, cloneOnly, orderLogical)
	}
	t.Logf("paths=%d reserved work=%d spent=%d logical=%d alloc(with clone)=%d clone=%d", len(paths), orderWork, spent, orderLogical, withSort, cloneOnly)
	// Still the first identity failure: a replaced file is rejected.
	victim := fmt.Sprintf("%s/%s%05d", dir, strings.Repeat("n", 120), 7)
	if err := f.root.Remove(victim); err != nil {
		t.Fatal(err)
	}
	file, err := f.root.OpenFile(victim, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if e := f.b.ordinarySnapshotIdentity(f.root, f.s); e == nil || e.Code != "SOURCE_CHANGED" {
		t.Fatalf("a replaced file was not rejected: %v", e)
	}
}

// finalIDRun is one pass over tuples that share (or do not share) the per-pass caches.
type finalIDRun struct {
	judgments []ordinaryTupleJudgment
	b         *eligibilityBudget
	visits    map[string]uint64 // visits the checks really made, per kind
	runs      map[string]int    // checks that really ran, per kind
	work      map[string]uint64 // work charged per label
	charges   map[string]int
}

func finalIDExecute(t *testing.T, share bool, tuples []expSeq) finalIDRun {
	t.Helper()
	b := ordinaryTestBudget(t)
	if share {
		b.decoded = newOrdinaryDecodedAssets()
	}
	run := finalIDRun{b: b, visits: map[string]uint64{}, runs: map[string]int{}, work: map[string]uint64{}, charges: map[string]int{}}
	prevCharge, prevVisit := ordinaryChargeHook, ordinaryVisitHook
	ordinaryChargeHook = func(_ *eligibilityBudget, work, _ uint64, at string, _, _ uint64) {
		run.work[at] += work
		run.charges[at]++
	}
	ordinaryVisitHook = func(kind string, visits uint64) {
		run.visits[kind] += visits
		run.runs[kind]++
	}
	defer func() { ordinaryChargeHook, ordinaryVisitHook = prevCharge, prevVisit }()
	for _, c := range tuples {
		c.skipReference = true
		run.judgments = append(run.judgments, expJudge(t, b, c))
	}
	return run
}

// finalIDTuples are tuples that read one actual tree (the same node table) with different queries,
// plus tuples over other actual trees: an API table that disagrees, an invalid node table.
func finalIDTuples(t *testing.T, queries int) []expSeq {
	t.Helper()
	tuples := expTuples(t)
	for i := 0; i < queries; i++ {
		c := tuples[0]
		c.id.Query = fmt.Sprintf("extra-%d", i)
		c.actual.Queries = slices.Clone(c.actual.Queries)
		c.actual.Queries[0].ID = c.id.Query
		tuples = append(tuples, c)
	}
	base := tuples[0]
	apiBad := base
	apiBad.actual.API = &nativewire.WireAPI{Revision: base.actual.API.Revision, Names: []string{}, ChildFields: [][]int64{}, FieldLookups: [][]int64{}, Points: [][]int64{},
		Nodes: [][]int64{slices.Clone(base.actual.API.Nodes[0]), slices.Clone(base.actual.API.Nodes[1]), slices.Clone(base.actual.API.Nodes[2])}}
	apiBad.actual.API.Nodes[1][0] = -1
	invalid := base
	invalid.actual.Nodes = slices.Clone(base.actual.Nodes)
	invalid.actual.Nodes[1].EndByte = 99
	invalid.actual.API = nil
	shape := base
	shape.actual.API = &nativewire.WireAPI{Revision: "other", Nodes: base.actual.API.Nodes, Names: []string{}, ChildFields: [][]int64{}, FieldLookups: [][]int64{}, Points: [][]int64{}}
	return append(tuples, apiBad, invalid, shape, apiBad, invalid, shape, base)
}

// Sharing the query-independent checks changes the ledger, never a judgment: states, codes,
// findings and their order equal those of a pass without it. The checks run once per actual tree
// (per source pin, per expected tree), and what they charged covers what they visited.
func TestOrdinaryActualShareKeepsJudgmentsAndRunsOncePerTree(t *testing.T) {
	tuples := finalIDTuples(t, 24)
	plain := finalIDExecute(t, false, tuples)
	shared := finalIDExecute(t, true, tuples)
	if !reflect.DeepEqual(plain.judgments, shared.judgments) {
		for i := range plain.judgments {
			if !reflect.DeepEqual(plain.judgments[i], shared.judgments[i]) {
				t.Errorf("tuple %d: plain %+v shared %+v", i, plain.judgments[i], shared.judgments[i])
			}
		}
		t.Fatal("sharing changed a judgment")
	}
	n := len(tuples)
	if shared.judgments[n-1].State != ordinaryTupleVerified {
		t.Fatalf("the unchanged actual tree must verify: %+v", shared.judgments[n-1])
	}
	// The appended tuples are apiBad, invalid, shape, apiBad, invalid, shape, base.
	wantCodes := map[int]string{n - 7: "ORDINARY_API_MISMATCH", n - 4: "ORDINARY_API_MISMATCH", n - 6: "ORDINARY_ACTUAL_TREE_INVALID", n - 3: "ORDINARY_ACTUAL_TREE_INVALID", n - 5: "ORDINARY_API_INVALID", n - 2: "ORDINARY_API_INVALID"}
	for i, want := range wantCodes {
		found := false
		for _, f := range shared.judgments[i].Findings {
			found = found || f.Code == want
		}
		if !found {
			t.Errorf("tuple %d lost its finding %s: %+v", i, want, shared.judgments[i])
		}
	}
	// Four actual trees (base, API mismatch, invalid-shape API, invalid node table) over one source pin. The
	// verified expected tree meets all four, the tampered one only the base tree.
	for kind, want := range map[string]int{"tree-check": 4, "api-compare": 3, "tree-compare": 5} {
		if got := shared.runs[kind]; got != want {
			t.Errorf("%s ran %d times, want %d (once per actual tree, source pin and expected tree)", kind, got, want)
		}
	}
	if plain.runs["tree-check"] != n || plain.runs["api-compare"] != n-2 {
		t.Errorf("without a share every tuple checks its tree: %v", plain.runs)
	}
	// The charged work covers the visits, with and without a share.
	for _, run := range []finalIDRun{plain, shared} {
		for kind, label := range map[string]string{"tree-check": "ordinary-actual-tree-check", "api-compare": "ordinary-api-compare", "tree-compare": "ordinary-tree-compare"} {
			if run.work[label] < run.visits[kind] {
				t.Errorf("%s charged work %d below the %d visits made", label, run.work[label], run.visits[kind])
			}
		}
	}
	if shared.b.work >= plain.b.work || shared.b.logical >= plain.b.logical {
		t.Fatalf("sharing did not reduce the ledger: work %d/%d logical %d/%d", shared.b.work, plain.b.work, shared.b.logical, plain.b.logical)
	}
	// Every use pays its lookups, one work unit each: the tree entry and the structure check of every tuple, the
	// API check and the comparison of every tuple that reaches them (the plain pass charges the checks per use).
	wantLookups := uint64(2*n) + uint64(plain.charges["ordinary-api-compare"]) + uint64(plain.charges["ordinary-tree-compare"])
	if got := shared.work["ordinary-actual-cache-lookup"]; got != wantLookups || plain.work["ordinary-actual-cache-lookup"] != 0 {
		t.Errorf("%d lookup work charged for %d tuples, want %d (plain %d)", got, n, wantLookups, plain.work["ordinary-actual-cache-lookup"])
	}
}

// A cached outcome is bound to the source pin it was computed against: the same node table read
// with another source (same length, other line layout) is checked again and fails alone.
func TestOrdinaryActualShareRebindsToTheTupleSource(t *testing.T) {
	nodes, a := refRowTree(64, "t")
	b2 := slices.Clone(a)
	b2[10] = '\n' // the node points of a one-line tree no longer match this source
	pin := func(src []byte) OrdinaryPin {
		return OrdinaryPin{Path: "source-witness/" + digestHex(src) + ".utf8", Bytes: uint64(len(src)), SHA256: digestHex(src)}
	}
	actual := nativewire.Tree{Status: StatusCompleted, Form: "full", Nodes: nodes, Queries: []nativewire.WireQuery{{ID: "q"}}}
	tuple := func(src []byte, query string) expSeq {
		c := expSeq{id: ordinaryTupleIdentity{Case: "c", Query: query, TreeRole: "incremental", Source: pin(src)}, actual: actual, source: src, skipReference: true}
		return c
	}
	for _, order := range [][]string{{"a", "b", "a"}, {"b", "a", "b"}} {
		var tuples []expSeq
		for i, which := range order {
			src := a
			if which == "b" {
				src = b2
			}
			tuples = append(tuples, tuple(src, fmt.Sprintf("q%d", i)))
		}
		plain := finalIDExecute(t, false, tuples)
		shared := finalIDExecute(t, true, tuples)
		if !reflect.DeepEqual(plain.judgments, shared.judgments) {
			t.Fatalf("order %v: sharing changed the judgments\nplain  %+v\nshared %+v", order, plain.judgments, shared.judgments)
		}
		for i, which := range order {
			points := false
			for _, f := range shared.judgments[i].Findings {
				points = points || f.Code == "ORDINARY_ACTUAL_TREE_POINTS"
			}
			if points != (which == "b") {
				t.Fatalf("order %v tuple %d (source %s): points finding=%v", order, i, which, points)
			}
		}
		if shared.runs["tree-check"] != 2 {
			t.Fatalf("order %v: %d tree checks, want one per (tree, source pin)", order, shared.runs["tree-check"])
		}
	}
}

// Adversarial: a large node table read by many queries is checked once; the ledger charged for the
// shared pass covers the heap it allocated and the nodes it visited.
func TestOrdinaryActualShareLargeTreeManyQueries(t *testing.T) {
	const nodeCount, queries = 20000, 64
	nodes, source := refRowTree(nodeCount, "t")
	pin := OrdinaryPin{Path: "source-witness/" + digestHex(source) + ".utf8", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	var tuples []expSeq
	for i := 0; i < queries; i++ {
		q := fmt.Sprintf("q%d", i)
		tuples = append(tuples, expSeq{id: ordinaryTupleIdentity{Case: "c", Query: q, TreeRole: "incremental", Source: pin}, source: source, skipReference: true,
			actual: nativewire.Tree{Status: StatusCompleted, Form: "full", Nodes: nodes, Queries: []nativewire.WireQuery{{ID: q}}}})
	}
	plain := finalIDExecute(t, false, tuples)
	var shared finalIDRun
	alloc := refRowAlloc(func() { shared = finalIDExecute(t, true, tuples) })
	if shared.runs["tree-check"] != 1 || plain.runs["tree-check"] != queries {
		t.Fatalf("tree checks: shared %d plain %d", shared.runs["tree-check"], plain.runs["tree-check"])
	}
	if shared.visits["tree-check"] != uint64(nodeCount+len(source)) || plain.visits["tree-check"] != queries*uint64(nodeCount+len(source)) {
		t.Fatalf("visits shared %d plain %d", shared.visits["tree-check"], plain.visits["tree-check"])
	}
	if shared.work["ordinary-actual-tree-check"] < shared.visits["tree-check"] {
		t.Fatalf("charged work %d below %d visits", shared.work["ordinary-actual-tree-check"], shared.visits["tree-check"])
	}
	if shared.b.logical < alloc {
		t.Fatalf("shared pass accounted logical %d below the %d bytes allocated", shared.b.logical, alloc)
	}
	if !reflect.DeepEqual(plain.judgments, shared.judgments) {
		t.Fatal("sharing changed a judgment")
	}
	t.Logf("work shared %d plain %d; logical shared %d plain %d; alloc %d", shared.b.work, plain.b.work, shared.b.logical, plain.b.logical, alloc)
}

// The shared outcomes are never written: after the pass each equals a fresh check, and the node
// table they were computed from is unchanged.
func TestOrdinaryActualShareValuesStayUnchanged(t *testing.T) {
	tuples := finalIDTuples(t, 4)
	before := make([][]TreeNode, len(tuples))
	for i, c := range tuples {
		before[i] = slices.Clone(c.actual.Nodes)
	}
	shared := finalIDExecute(t, true, tuples)
	checked := 0
	for key, entry := range shared.b.decoded.actual {
		for pin, got := range entry.tree {
			c := tuples[slices.IndexFunc(tuples, func(c expSeq) bool { return len(c.actual.Nodes) > 0 && &c.actual.Nodes[0] == key.nodes })]
			fresh, e := ordinaryTestBudget(t).ordinaryActualTreeCheck(nil, c.actual.Nodes, pin, c.source)
			if e != nil || fresh != got {
				t.Fatalf("shared tree check %+v differs from a fresh one %+v (%v)", got, fresh, e)
			}
			checked++
		}
		if entry.api != nil {
			c := tuples[slices.IndexFunc(tuples, func(c expSeq) bool {
				return len(c.actual.Nodes) > 0 && &c.actual.Nodes[0] == key.nodes && c.actual.API == key.api
			})]
			fresh, e := ordinaryTestBudget(t).ordinaryActualAPICheck(nil, c.actual)
			if e != nil || fresh != *entry.api {
				t.Fatalf("shared API check %+v differs from a fresh one %+v (%v)", *entry.api, fresh, e)
			}
			checked++
		}
	}
	for i, c := range tuples {
		if !slices.EqualFunc(before[i], c.actual.Nodes, func(a, b TreeNode) bool { return reflect.DeepEqual(a, b) }) {
			t.Fatalf("tuple %d: the shared node table changed during the pass", i)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d shared outcomes checked", checked)
	}
}
