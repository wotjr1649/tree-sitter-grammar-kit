package kit

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
)

// #427: expectResult checks every anchor of an expectation against the whole tree. With several anchors it now
// indexes the named nodes once. The verdict must not change, and the ordinary base reservation must cover the
// steps and the allocation of the indexed path.

func anchorTestNodes(rng *rand.Rand, n int, types int) []TreeNode {
	nodes := make([]TreeNode, n)
	for i := range nodes {
		start := uint32(rng.Intn(40))
		nodes[i] = TreeNode{Parent: int64(i) - 1, Type: fmt.Sprintf("t%d", rng.Intn(types)), Named: rng.Intn(3) != 0,
			StartByte: start, EndByte: start + uint32(rng.Intn(6))}
	}
	return nodes
}

func anchorTestReference(nodes []TreeNode, anchors []ExpectAnchor) bool {
	for _, a := range anchors {
		if !anchorFound(nodes, a) {
			return false
		}
	}
	return true
}

func TestAnchorsFoundMatchesPerAnchorScan(t *testing.T) {
	rng := rand.New(rand.NewSource(427))
	for round := 0; round < 6000; round++ {
		nodes := anchorTestNodes(rng, rng.Intn(60), 1+rng.Intn(8))
		var anchors []ExpectAnchor
		for i, n := 0, []int{0, 1, 2, 3, 5, 12, 64}[rng.Intn(7)]; i < n; i++ {
			if len(nodes) > 0 && rng.Intn(3) != 0 {
				pick := nodes[rng.Intn(len(nodes))] // an existing node: named or not, so unnamed ones must not satisfy
				anchors = append(anchors, ExpectAnchor{Type: pick.Type, StartByte: pick.StartByte, EndByte: pick.EndByte})
			} else {
				start := uint32(rng.Intn(45))
				anchors = append(anchors, ExpectAnchor{Type: fmt.Sprintf("t%d", rng.Intn(9)), StartByte: start, EndByte: start + uint32(rng.Intn(7))})
			}
			if rng.Intn(8) == 0 {
				anchors = append(anchors, anchors[len(anchors)-1]) // duplicates are allowed
			}
		}
		if got, want := anchorsFound(nodes, anchors), anchorTestReference(nodes, anchors); got != want {
			t.Fatalf("round %d: anchorsFound=%v, per-anchor scan=%v\nnodes=%+v\nanchors=%+v", round, got, want, nodes, anchors)
		}
	}
}

func TestExpectResultAnchorsKeepTheirVerdictAtEveryCount(t *testing.T) {
	nodes := []TreeNode{{Parent: -1, Type: "root", Named: true, StartByte: 0, EndByte: 20}, {Parent: 0, Type: "call", Named: true, StartByte: 0, EndByte: 7},
		{Parent: 0, Type: "call", Named: true, StartByte: 9, EndByte: 16}, {Parent: 0, Type: "call", Named: false, StartByte: 9, EndByte: 13}}
	tree := &rcTree{Form: "full", Tree: &rcFullTree{Nodes: nodes}}
	for name, c := range map[string]struct {
		anchors []ExpectAnchor
		want    string
	}{
		"one found":         {[]ExpectAnchor{{"call", 0, 7}}, claimPass},
		"two found":         {[]ExpectAnchor{{"call", 0, 7}, {"call", 9, 16}}, claimPass},
		"two, first absent": {[]ExpectAnchor{{"call", 1, 7}, {"call", 9, 16}}, claimFail},
		"two, last absent":  {[]ExpectAnchor{{"call", 0, 7}, {"call", 1, 7}}, claimFail},
		"two, unnamed only": {[]ExpectAnchor{{"call", 0, 7}, {"call", 9, 13}}, claimFail},
		"duplicates":        {[]ExpectAnchor{{"call", 0, 7}, {"call", 0, 7}, {"call", 0, 7}}, claimPass},
	} {
		if got, known := expectResult(StepExpectation{Syntax: "ANY", Anchors: c.anchors}, tree); !known || got != c.want {
			t.Fatalf("%s: got %q known=%v, want %q", name, got, known, c.want)
		}
	}
}

// anchorSteps runs anchorsFound and returns the steps it reported.
func anchorSteps(nodes []TreeNode, anchors []ExpectAnchor) uint64 {
	var steps uint64
	previous := expectStepHook
	expectStepHook = func(n uint64) { steps += n }
	defer func() { expectStepHook = previous }()
	anchorsFound(nodes, anchors)
	return steps
}

func anchorTestCase(nodeCount, anchorCount int) ([]TreeNode, []ExpectAnchor) {
	nodes := make([]TreeNode, nodeCount)
	for i := range nodes {
		nodes[i] = TreeNode{Parent: int64(i) - 1, Type: fmt.Sprintf("type%d", i%97), Named: true, StartByte: uint32(i), EndByte: uint32(i) + 1}
	}
	anchors := make([]ExpectAnchor, anchorCount)
	for i := range anchors {
		n := nodes[(i*7919)%nodeCount]
		anchors[i] = ExpectAnchor{Type: n.Type, StartByte: n.StartByte, EndByte: n.EndByte}
	}
	return nodes, anchors
}

// The reservation covers 64 x the real steps and shrinks from nodes x anchors to nodes + anchors.
func TestOrdinaryBaseReplayAnchorReservationCoversSteps(t *testing.T) {
	for _, c := range []struct{ nodes, anchors int }{{2000, 1}, {2000, 2}, {2000, 40}, {5000, 64}, {50, 64}} {
		nodes, anchors := anchorTestCase(c.nodes, c.anchors)
		steps := anchorSteps(nodes, anchors)
		if c.anchors >= anchorIndexMin && steps != uint64(c.nodes+c.anchors) {
			t.Fatalf("%v: indexed path took %d steps, want nodes+anchors = %d", c, steps, c.nodes+c.anchors)
		}
		work := func(expect []StepExpectation) uint64 {
			b := semanticTestBudget(t)
			cs := &rcCase{Steps: []rcStep{{Incremental: &rcTree{Form: "full", Tree: &rcFullTree{Nodes: nodes}}}}}
			if e := b.reserveOrdinaryBaseReplay(cs, &IncrementalCase{Expect: expect}, 1); e != nil {
				t.Fatal(e)
			}
			return b.work
		}
		with := work([]StepExpectation{{Step: 0, Syntax: "ANY", Anchors: anchors}})
		without := work([]StepExpectation{{Step: 0, Syntax: "ANY"}})
		if with-without < 64*steps {
			t.Fatalf("%v: reserved %d work for %d real steps", c, with-without, steps)
		}
		if old := uint64(c.nodes) * uint64(c.anchors) * 64; c.anchors >= 8 && with-without > old/4 {
			t.Fatalf("%v: reservation %d did not shrink well below the scan bound %d", c, with-without, old)
		}
	}
}

// The index is the only allocation proportional to the tree; the reservation charges 256 B per node for it.
func TestAnchorsFoundAllocationWithinReservation(t *testing.T) {
	for _, n := range []int{10, 1000, 60000} {
		nodes, anchors := anchorTestCase(n, 5)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		anchorsFound(nodes, anchors)
		runtime.ReadMemStats(&after)
		if alloc, charged := after.TotalAlloc-before.TotalAlloc, uint64(256*n); alloc > charged {
			t.Fatalf("%d nodes: allocated %d B > reserved %d B", n, alloc, charged)
		}
	}
}

// A shared cache must not change any expectation's verdict, whatever the order, repetition or mix of Contains,
// anchors and trees within a case.
func TestExpectCacheKeepsVerdicts(t *testing.T) {
	rng := rand.New(rand.NewSource(4271))
	for round := 0; round < 1500; round++ {
		trees := make([]*rcTree, 1+rng.Intn(3))
		for i := range trees {
			trees[i] = &rcTree{Form: []string{"full", "full", "full", "summary"}[rng.Intn(4)], Tree: &rcFullTree{Nodes: anchorTestNodes(rng, rng.Intn(40), 1+rng.Intn(6))}}
			if rng.Intn(10) == 0 {
				trees[i].Tree = nil
			}
		}
		var cache expectCache
		for i := 0; i < 1+rng.Intn(12); i++ {
			tree := trees[rng.Intn(len(trees))]
			e := StepExpectation{Syntax: "ANY"}
			for j, n := 0, rng.Intn(4); j < n; j++ {
				e.Contains = append(e.Contains, fmt.Sprintf("t%d", rng.Intn(8)))
			}
			for j, n := 0, []int{0, 1, 2, 3, 6}[rng.Intn(5)]; j < n; j++ {
				start := uint32(rng.Intn(45))
				e.Anchors = append(e.Anchors, ExpectAnchor{Type: fmt.Sprintf("t%d", rng.Intn(8)), StartByte: start, EndByte: start + uint32(rng.Intn(7))})
			}
			wantResult, wantKnown := expectResult(e, tree)
			if got, known := expectResultCached(e, tree, &cache); got != wantResult || known != wantKnown {
				t.Fatalf("round %d expectation %d: cached (%q, %v), uncached (%q, %v)", round, i, got, known, wantResult, wantKnown)
			}
		}
	}
}

// The ordinary reservation charges a step tree's type set and anchor index once per case, as the evaluator
// builds them: the work covers 64 x the real steps, and many expectations no longer multiply the tree.
func TestOrdinaryBaseReplayReservesSharedSetsOnce(t *testing.T) {
	nodes, anchors := anchorTestCase(3000, 6)
	expectation := func() StepExpectation {
		return StepExpectation{Step: 0, Syntax: "ANY", Contains: []string{"type1", "type2", "type3"}, Anchors: anchors[:4]}
	}
	reserved := func(count int) (work, logical uint64) {
		expect := make([]StepExpectation, count)
		for i := range expect {
			expect[i] = expectation()
		}
		b := semanticTestBudget(t)
		tree := &rcTree{Form: "full", Tree: &rcFullTree{Nodes: nodes}}
		cs := &rcCase{Steps: []rcStep{{Incremental: tree}}}
		if e := b.reserveOrdinaryBaseReplay(cs, &IncrementalCase{Expect: expect}, 1); e != nil {
			t.Fatal(e)
		}
		return b.work, b.logical
	}
	realSteps := func(count int) uint64 {
		var steps uint64
		previous := expectStepHook
		expectStepHook = func(n uint64) { steps += n }
		defer func() { expectStepHook = previous }()
		tree := &rcTree{Form: "full", Tree: &rcFullTree{Nodes: nodes}}
		var cache expectCache
		for i := 0; i < count; i++ {
			if got, known := expectResultCached(expectation(), tree, &cache); !known || got != claimPass {
				t.Fatalf("expectation %d: %q %v", i, got, known)
			}
		}
		return steps
	}
	w1, l1 := reserved(1)
	w1base, _ := reserved(0)
	for _, count := range []int{1, 5, 40} {
		work, logical := reserved(count)
		if steps := realSteps(count); work-w1base < 64*steps {
			t.Fatalf("%d expectations: reserved %d work for %d real steps", count, work-w1base, steps)
		}
		if count > 1 && (work-w1base > 3*(w1-w1base) || logical > l1+uint64(count)*8192) {
			t.Fatalf("%d expectations: work %d logical %d did not stay near the single-expectation %d / %d", count, work-w1base, logical, w1-w1base, l1)
		}
	}
}
