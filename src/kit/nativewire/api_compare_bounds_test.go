package nativewire

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// apiTree is a generated tree with the API observations a correct runtime would give.
type apiTree struct {
	nodes []TreeNode
	api   WireAPI
}

// genAPITree builds a preorder tree: node i>0 hangs on the right spine, so ValidateTree holds. span
// picks the byte domain (a small one makes many zero-width siblings share boundaries), fields the
// number of distinct field names, and shape biases the parent choice (0 random, 1 wide root, 2 chain).
func genAPITree(rng *rand.Rand, n, span, fields, shape int, nameLen int) apiTree {
	nodes := make([]TreeNode, n)
	nodes[0] = TreeNode{Parent: -1, Type: "root", Named: true, EndByte: uint32(span)}
	spine := []int64{0}
	for i := 1; i < n; i++ {
		var p int64
		switch shape {
		case 1:
			spine = spine[:1]
			p = 0
		case 2:
			p = spine[len(spine)-1]
		default:
			cut := 1 + rng.IntN(len(spine))
			spine = spine[:cut]
			p = spine[cut-1]
		}
		s := uint32(rng.IntN(span + 1))
		e := s
		if rng.IntN(3) > 0 {
			e = s + uint32(rng.IntN(3))
		}
		nd := TreeNode{Parent: p, Type: "t", Named: rng.IntN(2) == 0, StartByte: s, EndByte: e}
		if fields > 0 && rng.IntN(2) == 0 {
			f := fmt.Sprintf("%0*d", nameLen, rng.IntN(fields))
			nd.Field = &f
		}
		nodes[i] = nd
		spine = append(spine, int64(i))
	}
	return apiTree{nodes, trueAPI(nodes)}
}

// trueAPI computes the API rows by direct definition, quadratic where the product is not.
func trueAPI(nodes []TreeNode) WireAPI {
	n := len(nodes)
	kids := make([][]int64, n)
	depth := make([]int64, n)
	size := make([]int64, n)
	for i := n - 1; i >= 0; i-- {
		size[i]++
		if p := nodes[i].Parent; p >= 0 {
			size[p] += size[i]
		}
	}
	for i := range nodes {
		if p := nodes[i].Parent; p >= 0 {
			kids[p] = append(kids[p], int64(i))
			depth[i] = depth[p] + 1
		}
	}
	api := WireAPI{Revision: "tsgk-api/r1", ChildFields: [][]int64{}, FieldLookups: [][]int64{}, Points: [][]int64{}, Names: []string{}}
	nameOf := map[string]int64{}
	for i := range nodes {
		sib := func(step int, named bool) int64 {
			p := nodes[i].Parent
			if p < 0 {
				return -1
			}
			for k := indexOf(kids[p], int64(i)) + step; k >= 0 && k < len(kids[p]); k += step {
				if !named || nodes[kids[p][k]].Named {
					return kids[p][k]
				}
			}
			return -1
		}
		first, firstNamed, named := int64(-1), int64(-1), int64(0)
		for _, c := range kids[i] {
			if first < 0 {
				first = c
			}
			if nodes[c].Named {
				named++
				if firstNamed < 0 {
					firstNamed = c
				}
			}
		}
		api.Nodes = append(api.Nodes, []int64{nodes[i].Parent, sib(-1, false), sib(1, false), sib(-1, true), sib(1, true), first, firstNamed,
			int64(len(kids[i])), named, size[i], depth[i], int64(i), int64(i)})
		seen := map[string]bool{}
		for k, c := range kids[i] {
			f := nodes[c].Field
			if f == nil {
				continue
			}
			id, ok := nameOf[*f]
			if !ok {
				id = int64(len(api.Names))
				nameOf[*f] = id
				api.Names = append(api.Names, *f)
			}
			api.ChildFields = append(api.ChildFields, []int64{int64(i), int64(k), id})
			if !seen[*f] {
				seen[*f] = true
				api.FieldLookups = append(api.FieldLookups, []int64{int64(i), id, c})
			}
		}
	}
	return api
}

func indexOf(list []int64, x int64) int {
	for i, v := range list {
		if v == x {
			return i
		}
	}
	return -1
}

func cloneAPI(a WireAPI) WireAPI {
	b := a
	rows := func(t [][]int64) [][]int64 {
		out := make([][]int64, len(t))
		for i := range t {
			out[i] = append([]int64(nil), t[i]...)
		}
		return out
	}
	b.Nodes, b.ChildFields, b.FieldLookups, b.Points = rows(a.Nodes), rows(a.ChildFields), rows(a.FieldLookups), rows(a.Points)
	b.Names = append([]string(nil), a.Names...)
	return b
}

// mutateAPI changes a few cells: sibling cells to a neighbouring node (to reach the zero-width
// divergence paths), other cells to anything, and the field tables.
func mutateAPI(rng *rand.Rand, nodes []TreeNode, a WireAPI, tries int) WireAPI {
	a = cloneAPI(a)
	n := int64(len(nodes))
	for ; tries > 0; tries-- {
		switch rng.IntN(8) {
		case 0, 1, 2:
			i := rng.IntN(len(a.Nodes))
			k := 1 + rng.IntN(4)
			if rng.IntN(4) == 0 {
				k = rng.IntN(13)
			}
			v := int64(rng.IntN(int(n)+2)) - 1
			if k <= 4 && rng.IntN(3) > 0 { // a node near i, usually a sibling
				v = int64(i) + int64(rng.IntN(7)) - 3
				if v < -1 || v >= n {
					v = -1
				}
			}
			a.Nodes[i][k] = v
		case 3:
			if len(a.ChildFields) > 0 {
				r := a.ChildFields[rng.IntN(len(a.ChildFields))]
				switch rng.IntN(3) {
				case 0:
					r[1] += int64(rng.IntN(3)) - 1
					if r[1] < 0 {
						r[1] = 0
					}
				case 1:
					r[2] = int64(rng.IntN(len(a.Names)))
				default:
					a.ChildFields = append(a.ChildFields, append([]int64(nil), r...))
				}
			}
		case 4:
			if len(a.FieldLookups) > 0 {
				r := a.FieldLookups[rng.IntN(len(a.FieldLookups))]
				switch rng.IntN(5) {
				case 4: // a lookup nobody asked for
					a.FieldLookups = append(a.FieldLookups, []int64{int64(rng.IntN(int(n))), int64(rng.IntN(len(a.Names))), int64(rng.IntN(int(n)))})
				case 0:
					r[2] = int64(rng.IntN(int(n)))
				case 1:
					r[1] = int64(rng.IntN(len(a.Names)))
				case 2:
					a.FieldLookups = append(a.FieldLookups, []int64{r[0], r[1], int64(rng.IntN(int(n)))})
				default:
					a.FieldLookups = append(a.FieldLookups, append([]int64(nil), r...))
				}
			}
		case 5:
			if len(a.Names) > 0 {
				if rng.IntN(2) == 0 {
					a.Names = append(a.Names, a.Names[rng.IntN(len(a.Names))]) // a duplicate name text
				} else {
					a.Names[rng.IntN(len(a.Names))] = "x"
				}
			}
		case 6:
			b := int64(rng.IntN(int(nodes[0].EndByte) + 3))
			a.Points = append(a.Points, []int64{b, int64(rng.IntN(int(n+1))) - 1, int64(rng.IntN(int(n+1))) - 1, int64(rng.IntN(int(n+1))) - 1})
		default:
			if len(a.Points) > 0 {
				r := a.Points[rng.IntN(len(a.Points))]
				r[1+rng.IntN(3)] = int64(rng.IntN(int(n+1))) - 1
			}
		}
	}
	return a
}

func shapeOK(a *WireAPI, n int) bool {
	for _, r := range a.ChildFields {
		if r[0] < 0 || r[0] >= int64(n) || r[1] < 0 || r[2] < 0 || r[2] >= int64(len(a.Names)) {
			return false
		}
	}
	for _, r := range a.FieldLookups {
		if r[0] < 0 || r[0] >= int64(n) || r[1] < 0 || r[1] >= int64(len(a.Names)) {
			return false
		}
	}
	return true
}

// TestCompareAPIMatchesReference holds the result, the difference and every divergence equal to
// the previous implementation over random trees and random API tables, which reach every
// check, every positional walk (the tiny byte domain makes zero-width runs common) and every field case.
func TestCompareAPIMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(362, 1))
	kinds := map[string]int{}
	divergent := 0
	for iter := 0; iter < 30000; iter++ {
		n := 1 + rng.IntN(40)
		tree := genAPITree(rng, n, 1+rng.IntN(3), rng.IntN(5), []int{0, 1, 1, 2}[rng.IntN(4)], 1+rng.IntN(3))
		for _, b := range []int64{0, 1, 2} {
			tree.api.Points = append(tree.api.Points, []int64{b, 0, 0, firstEndingAfter(tree.nodes, b)})
		}
		api := tree.api
		if iter%3 != 0 {
			api = mutateAPI(rng, tree.nodes, api, 1+rng.IntN(6))
		}
		if !shapeOK(&api, n) {
			continue
		}
		wantDiff, wantDiv := compareAPIReference(tree.nodes, &api)
		gotDiff, gotDiv := CompareAPI(tree.nodes, &api)
		if !reflect.DeepEqual(wantDiff, gotDiff) || !reflect.DeepEqual(wantDiv, gotDiv) {
			t.Fatalf("iteration %d (n=%d): reference %+v %+v, got %+v %+v", iter, n, wantDiff, wantDiv, gotDiff, gotDiv)
		}
		if gotDiff != nil {
			kinds[strings.SplitN(gotDiff.Check, ":", 2)[0]]++
		} else {
			kinds["none"]++
		}
		if len(gotDiv) > 0 {
			divergent++
		}
	}
	for _, check := range []string{"none", "parent", "prev_sibling", "next_sibling", "prev_named_sibling", "next_named_sibling", "first_child", "first_named_child",
		"child_count", "named_child_count", "descendant_count", "cursor_depth", "child_field", "child_field_ordinal", "field_lookup", "field_lookup_extra", "descendant_for_byte", "first_child_for_byte"} {
		if kinds[check] == 0 {
			t.Errorf("no generated case ends in %q: %v", check, kinds)
		}
	}
	if divergent < 100 {
		t.Errorf("only %d cases carry a position_navigation divergence", divergent)
	}
}

func firstEndingAfter(nodes []TreeNode, b int64) int64 {
	for i := range nodes {
		if nodes[i].Parent == 0 && int64(nodes[i].EndByte) > b {
			return int64(i)
		}
	}
	return -1
}

// TestCompareAPIDivergenceRuns runs the positional walk over long zero-width runs, in every
// direction, against the previous implementation: the quadratic case that is now O(1) per query.
func TestCompareAPIDivergenceRuns(t *testing.T) {
	for _, named := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			nodes := []TreeNode{{Parent: -1, Type: "root", Named: true, EndByte: 5}}
			for i := 0; i < 60; i++ {
				nd := TreeNode{Parent: 0, Type: "z", Named: named || i%2 == 0, StartByte: 3, EndByte: 3}
				if mixed && i%7 == 3 {
					nd.StartByte, nd.EndByte, nd.Named = 3, 4, i%2 == 0
				}
				nodes = append(nodes, nd)
			}
			base := trueAPI(nodes)
			for k := 1; k <= 4; k++ {
				for i := 1; i < len(nodes); i++ {
					for _, got := range []int64{-1, int64(i), int64(i) + 1, int64(i) + 2, int64(i) + 10, int64(i) - 1, int64(i) - 2, int64(i) - 10, int64(len(nodes))} {
						if got >= int64(len(nodes)) {
							got = -1
						}
						api := cloneAPI(base)
						api.Nodes[i][k] = got
						wantDiff, wantDiv := compareAPIReference(nodes, &api)
						gotDiff, gotDiv := CompareAPI(nodes, &api)
						if !reflect.DeepEqual(wantDiff, gotDiff) || !reflect.DeepEqual(wantDiv, gotDiv) {
							t.Fatalf("named=%v mixed=%v node %d cell %d = %d: reference %+v %+v, got %+v %+v", named, mixed, i, k, got, wantDiff, wantDiv, gotDiff, gotDiv)
						}
					}
				}
			}
		}
	}
}

type apiCase struct {
	name string
	tree apiTree
}

// adversarialAPICases are inputs that made the previous implementation super-linear: a node with
// very many siblings (named and unnamed), a deep chain, very many fields on one node, very long
// field names, API tables much larger or smaller than the tree, and many points.
func adversarialAPICases() []apiCase {
	rng := rand.New(rand.NewPCG(362, 2))
	var cases []apiCase
	add := func(name string, tree apiTree) { cases = append(cases, apiCase{name, tree}) }

	wide := genAPITree(rng, 60001, 1000, 0, 1, 1)
	add("wide-root-60000-siblings", wide)

	unnamed := genAPITree(rng, 30001, 4, 0, 1, 1)
	for i := 1; i < len(unnamed.nodes)-1; i++ {
		unnamed.nodes[i].Named = false
	}
	unnamed.api = trueAPI(unnamed.nodes)
	add("wide-root-unnamed-siblings", unnamed)

	zero := genAPITree(rng, 30001, 1, 0, 1, 1)
	for i := 1; i < len(zero.nodes); i++ {
		zero.nodes[i].StartByte, zero.nodes[i].EndByte, zero.nodes[i].Named = 1, 1, true
	}
	zero.api = trueAPI(zero.nodes)
	// every sibling cell names the sibling one further along: a zero-width skip at every node, in all four walks
	for i := 1; i < len(zero.nodes); i++ {
		for k := 1; k <= 4; k++ {
			v, step := zero.api.Nodes[i][k], int64(1)
			if k%2 == 1 {
				step = -1
			}
			if v >= 1 && v+step >= 1 && v+step < int64(len(zero.nodes)) {
				zero.api.Nodes[i][k] = v + step
			}
		}
	}
	add("wide-root-all-zero-width-all-divergent", zero)

	add("chain-depth-40000", genAPITree(rng, 40001, 100, 3, 2, 2))
	add("random-tree", genAPITree(rng, 50001, 50, 8, 0, 3))

	manyFields := genAPITree(rng, 40001, 50, 20000, 1, 6)
	add("root-with-20000-distinct-fields", manyFields)

	longNames := genAPITree(rng, 2001, 50, 40, 1, 1)
	for i := range longNames.nodes {
		if f := longNames.nodes[i].Field; f != nil {
			s := strings.Repeat("f", 4000) + *f
			longNames.nodes[i].Field = &s
		}
	}
	longNames.api = trueAPI(longNames.nodes)
	add("fields-of-4000-bytes", longNames)

	manyRows := genAPITree(rng, 500, 50, 10, 1, 2)
	for i := 0; i < 40000; i++ {
		manyRows.api.ChildFields = append(manyRows.api.ChildFields, append([]int64(nil), manyRows.api.ChildFields[i%len(manyRows.api.ChildFields)]...))
		manyRows.api.FieldLookups = append(manyRows.api.FieldLookups, append([]int64(nil), manyRows.api.FieldLookups[i%len(manyRows.api.FieldLookups)]...))
		manyRows.api.Names = append(manyRows.api.Names, manyRows.api.Names[i%len(manyRows.api.Names)])
	}
	add("tables-much-larger-than-the-tree", manyRows)

	fewRows := genAPITree(rng, 30000, 50, 0, 1, 1)
	fewRows.api.Nodes = fewRows.api.Nodes[:10]
	fewRows.api.ChildFields, fewRows.api.FieldLookups, fewRows.api.Names = [][]int64{}, [][]int64{}, []string{}
	add("tables-much-smaller-than-the-tree", fewRows)

	points := genAPITree(rng, 30001, 30000, 0, 1, 1)
	for b := int64(0); b < 30000; b++ {
		points.api.Points = append(points.api.Points, []int64{b, 0, 0, firstEndingAfter(points.nodes, b)})
	}
	add("30000-points-over-30000-root-children", points)
	return cases
}

// sharedAdversarialAPICases generates the seeded cases once; both tests below only read them
// (a mismatched variant works on a clone).
var sharedAdversarialAPICases = sync.OnceValue(adversarialAPICases)

// TestCompareAPIStaysWithinCompareAPIWork holds the visits and string bytes CompareAPI really
// performs, and the memory it allocates, within what CompareAPIWork and CompareAPILogical reserve.
func TestCompareAPIStaysWithinCompareAPIWork(t *testing.T) {
	var visits, text uint64
	compareAPICostHook = func(v, b uint64) { visits, text = v, b }
	defer func() { compareAPICostHook = nil }()
	for _, tc := range sharedAdversarialAPICases() {
		nodes, api := tc.tree.nodes, tc.tree.api
		for _, variant := range []string{"valid", "mismatched"} {
			a := api
			if variant == "mismatched" { // the first difference comes last: change only the final point
				a = cloneAPI(api)
				if len(a.Points) > 0 {
					a.Points[len(a.Points)-1][3] = -2
				} else {
					a.Nodes[len(a.Nodes)-1][12] = -2
				}
			}
			work, logical := CompareAPIWork(nodes, &a), CompareAPILogical(nodes, &a)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			d, div := CompareAPI(nodes, &a)
			runtime.ReadMemStats(&after)
			alloc := after.TotalAlloc - before.TotalAlloc
			if variant == "valid" && tc.name != "wide-root-all-zero-width-all-divergent" && (d != nil || len(div) != 0) {
				t.Fatalf("%s: valid tree reports %+v %+v", tc.name, d, div)
			}
			if visits+text > work {
				t.Errorf("%s/%s: visits %d + bytes %d exceed the reserved work %d", tc.name, variant, visits, text, work)
			}
			if alloc > logical {
				t.Errorf("%s/%s: allocated %d bytes, reserved %d", tc.name, variant, alloc, logical)
			}
			t.Logf("%s/%s: nodes %d visits %d bytes %d reserved work %d; alloc %d reserved %d", tc.name, variant, len(nodes), visits, text, work, alloc, logical)
		}
	}
}

// TestCompareAPIAdversarialMatchesReference checks the large adversarial cases whose reference
// run is still affordable.
func TestCompareAPIAdversarialMatchesReference(t *testing.T) {
	for _, tc := range sharedAdversarialAPICases() {
		if len(tc.tree.nodes) > 31000 || tc.name == "wide-root-all-zero-width-all-divergent" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			// the cases are read-only and the cost hook is only set by a sequential test
			t.Parallel()
			wantDiff, wantDiv := compareAPIReference(tc.tree.nodes, &tc.tree.api)
			gotDiff, gotDiv := CompareAPI(tc.tree.nodes, &tc.tree.api)
			if !reflect.DeepEqual(wantDiff, gotDiff) || !reflect.DeepEqual(wantDiv, gotDiv) {
				t.Fatalf("%s: reference %+v %+v, got %+v %+v", tc.name, wantDiff, wantDiv, gotDiff, gotDiv)
			}
		})
	}
}
