package nativewire

import (
	"fmt"
	"math/bits"
	"strconv"
)

// APIDifference is the first disagreement of an API observation with the serialized tree.
type APIDifference struct {
	Check string `json:"check"`
	Node  int64  `json:"node"`
	Got   string `json:"got"`
	Want  string `json:"want"`
}

// apiCost is what one CompareAPI call did: structural visits (a node, row, child or name seen in
// one pass, or one step of a point's binary search) and string bytes hashed or compared.
type apiCost struct{ visits, bytes uint64 }

// compareAPICostHook is a test-only seam; nil in product use.
var compareAPICostHook func(visits, bytes uint64)

// CompareAPIWork is the work a budgeted caller reserves for one CompareAPI call: every pass is
// linear in the nodes and table rows (at most 16 visits each), a point costs one binary search over
// the root's children, and each field or table name is hashed and compared at most twice.
func CompareAPIWork(nodes []TreeNode, a *WireAPI) uint64 {
	rows := uint64(len(a.Nodes) + len(a.ChildFields) + len(a.FieldLookups) + len(a.Points) + len(a.Names))
	var text uint64
	for i := range nodes {
		if f := nodes[i].Field; f != nil {
			text += uint64(len(*f))
		}
	}
	for _, name := range a.Names {
		text += uint64(len(name))
	}
	return 16*(uint64(len(nodes))+rows) + uint64(len(a.Points))*uint64(bits.Len(uint(len(nodes)))) + 2*text + 1
}

// CompareAPILogical is the memory a budgeted caller reserves for one CompareAPI call: 512 bytes per node
// for its tables, 1,536 per node for the divergences (at most four per node, each about 350 bytes with
// its strings and the growth of the result slice), and per name and per field-lookup row the index maps.
func CompareAPILogical(nodes []TreeNode, a *WireAPI) uint64 {
	return 2048*uint64(len(nodes)) + 128*uint64(len(a.Names)+len(a.FieldLookups))
}

// CompareAPI checks the tsgk-api/r1 observations of a full tree against its cursor
// serialization: node API parent, siblings, first children, child/named counts, subtree
// size, cursor depth and descendant index, the field name of every child, every field
// lookup (the first child carrying the field, and no lookup without one), and the byte
// lookups of the registered points (the found node covers the byte; the first child for a
// byte is the first root child ending after it). The tree and API must have passed CheckTree.
// The pinned runtime navigates node
// siblings by byte position (lib/src/node.c ts_node__prev_sibling/ts_node__next_sibling), so
// at a zero-width node the node API may skip a sibling the cursor visits: a sibling
// difference where the node or the cursor's sibling is zero-width and the node API gives
// null or a sibling in the same direction is returned as a divergence, not a difference.
// Every other disagreement is the first difference.
//
// The work is linear in the nodes and table rows (plus one binary search per point and the field
// and name bytes); CompareAPIWork is its bound.
func CompareAPI(nodes []TreeNode, a *WireAPI) (*APIDifference, []APIDifference) {
	var c apiCost
	d, divergences := compareAPI(nodes, a, &c)
	if compareAPICostHook != nil {
		compareAPICostHook(c.visits, c.bytes)
	}
	return d, divergences
}

var apiChecks = [13]string{"parent", "prev_sibling", "next_sibling", "prev_named_sibling", "next_named_sibling", "first_child", "first_named_child",
	"child_count", "named_child_count", "descendant_count", "cursor_depth", "cursor_descendant_index", "cursor_node"}

// apiRuns are the lengths of the zero-width runs positional walks skip, per node and direction.
type apiRuns struct{ fwd, bwd, fwdNamed, bwdNamed []int32 }

type apiLookup struct {
	first, other int64 // the first row's node, and the first row naming another node
	hasOther     bool
}

func compareAPI(nodes []TreeNode, a *WireAPI, c *apiCost) (*APIDifference, []APIDifference) {
	var divergences []APIDifference
	n := len(nodes)
	un := uint64(n)
	size := make([]int64, n)
	c.visits += un
	for i := n - 1; i >= 0; i-- {
		size[i]++
		if p := nodes[i].Parent; p >= 0 {
			size[p] += size[i]
		}
	}
	// children lists as views of one array, in node order; pos is a node's index in its list.
	off := make([]int, n+1)
	for i := range nodes {
		if p := nodes[i].Parent; p >= 0 {
			off[p+1]++
		}
	}
	for p := 0; p < n; p++ {
		off[p+1] += off[p]
	}
	flat := make([]int64, off[n])
	children := make([][]int64, n)
	depth := make([]int64, n)
	pos := make([]int32, n)
	cnt := make([]int32, n)
	c.visits += 3 * un
	for i := range nodes {
		if p := nodes[i].Parent; p >= 0 {
			k := cnt[p]
			flat[off[p]+int(k)] = int64(i)
			pos[i] = k
			cnt[p]++
			depth[i] = depth[p] + 1
		}
	}
	// nearest named siblings and the rank among them, per node; the first named child per parent.
	prevN, nextN, firstN := make([]int64, n), make([]int64, n), make([]int64, n)
	nrank, namedCount := make([]int32, n), make([]int32, n)
	c.visits += un + 2*uint64(len(flat))
	for i := range nodes {
		if nodes[i].Parent < 0 {
			prevN[i], nextN[i] = -1, -1
		}
	}
	for p := range children {
		list := flat[off[p]:off[p+1]]
		children[p] = list
		last, rank := int64(-1), int32(0)
		for _, ch := range list {
			prevN[ch], nrank[ch] = last, rank
			if nodes[ch].Named {
				last = ch
				rank++
			}
		}
		namedCount[p] = rank
		last = -1
		for k := len(list) - 1; k >= 0; k-- {
			ch := list[k]
			nextN[ch] = last
			if nodes[ch].Named {
				last = ch
			}
		}
		firstN[p] = last
	}
	diff := func(check string, node int64, got, want int64) *APIDifference {
		return &APIDifference{check, node, strconv.FormatInt(got, 10), strconv.FormatInt(want, 10)}
	}
	zeroWidthAt := func(x int64, b uint32) bool { return nodes[x].StartByte == b && nodes[x].EndByte == b }
	var runs *apiRuns
	runLengths := func() *apiRuns {
		if runs != nil {
			return runs
		}
		runs = &apiRuns{make([]int32, n), make([]int32, n), make([]int32, n), make([]int32, n)}
		c.visits += 2 * un
		for x := n - 1; x >= 0; x-- {
			p := nodes[x].Parent
			if p < 0 {
				continue
			}
			if k := int(pos[x]) + 1; k < len(children[p]) {
				if nx := children[p][k]; zeroWidthAt(nx, nodes[x].EndByte) {
					runs.fwd[x] = 1 + runs.fwd[nx]
				}
			}
			if nn := nextN[x]; nn >= 0 && zeroWidthAt(nn, nodes[x].EndByte) {
				runs.fwdNamed[x] = 1 + runs.fwdNamed[nn]
			}
		}
		for x := range nodes {
			p := nodes[x].Parent
			if p < 0 {
				continue
			}
			if k := int(pos[x]) - 1; k >= 0 {
				if pv := children[p][k]; zeroWidthAt(pv, nodes[x].StartByte) {
					runs.bwd[x] = 1 + runs.bwd[pv]
				}
			}
			if pn := prevN[x]; pn >= 0 && zeroWidthAt(pn, nodes[x].StartByte) {
				runs.bwdNamed[x] = 1 + runs.bwdNamed[pn]
			}
		}
		return runs
	}
	// positional reports whether the node API's sibling got differs from the cursor's only by
	// skipping zero-width siblings at x's boundary (x's end going forward, its start going
	// back), as the pinned runtime's position-based search does: every sibling it passed over
	// in that direction, the cursor's sibling included, is such a node, and got is null or
	// the next sibling after them. x itself is never on the walked side, so a node named as
	// its own sibling is a difference. The candidates are ranked in walk order (named
	// siblings only when named), the run of zero-width ones after x has a precomputed length,
	// and got is judged by its rank against that run.
	positional := func(x, got int64, step int, named bool) bool {
		p := nodes[x].Parent
		if p < 0 {
			return false
		}
		r := runLengths()
		var first, total, run int64
		rank := func(ch int64) int64 { return int64(pos[ch]) }
		if named {
			rank = func(ch int64) int64 { return int64(nrank[ch]) }
			total = int64(namedCount[p])
			if step > 0 {
				first, run = int64(nrank[x]), int64(r.fwdNamed[x])
				if nodes[x].Named {
					first++
				}
			} else {
				first, run = int64(nrank[x])-1, int64(r.bwdNamed[x])
			}
		} else {
			total = int64(len(children[p]))
			first = int64(pos[x]) + int64(step)
			if step > 0 {
				run = int64(r.fwd[x])
			} else {
				run = int64(r.bwd[x])
			}
		}
		if got >= 0 && got < int64(n) && nodes[got].Parent == p && (!named || nodes[got].Named) {
			j := (rank(got) - first) * int64(step) // got's distance along the walk
			switch {
			case j < 0:
				return false
			case j < run:
				return j > 0
			case j == run:
				return run > 0
			}
			return false
		}
		after := first + int64(step)*run // the candidate after the run
		return got == -1 && (after < 0 || after >= total) && run > 0
	}
	sibling := func(i int64, step int, named bool) int64 {
		p := nodes[i].Parent
		if p < 0 {
			return -1
		}
		if named {
			if step < 0 {
				return prevN[i]
			}
			return nextN[i]
		}
		if k := int(pos[i]) + step; k >= 0 && k < len(children[p]) {
			return children[p][k]
		}
		return -1
	}
	var want [13]int64
	for i, row := range a.Nodes {
		c.visits++
		x := int64(i)
		first := int64(-1)
		if len(children[i]) > 0 {
			first = children[i][0]
		}
		want = [13]int64{nodes[i].Parent, sibling(x, -1, false), sibling(x, 1, false), sibling(x, -1, true), sibling(x, 1, true),
			first, firstN[i], int64(len(children[i])), int64(namedCount[i]), size[i], depth[i], x, x}
		for k := range want {
			if row[k] == want[k] {
				continue
			}
			if k >= 1 && k <= 4 && positional(x, row[k], []int{-1, 1, -1, 1}[k-1], k >= 3) {
				divergences = append(divergences, *diff("position_navigation:"+apiChecks[k], x, row[k], want[k]))
				continue
			}
			return diff(apiChecks[k], x, row[k], want[k]), divergences
		}
	}
	// field name of each child: exactly the cursor fields
	fieldAt := make([]int64, len(flat)) // the name index of each child slot, -1 for none
	for i := range fieldAt {
		fieldAt[i] = -1
	}
	c.visits += uint64(len(flat) + len(a.ChildFields))
	for _, r := range a.ChildFields {
		if r[1] >= int64(len(children[r[0]])) {
			return diff("child_field_ordinal", r[0], r[1], int64(len(children[r[0]]))), divergences
		}
		if r[1] >= 0 {
			fieldAt[off[r[0]]+int(r[1])] = r[2]
		}
	}
	// canon maps a name index to the first index of the same text, as slices.Index finds it.
	nameIdx := make(map[string]int32, len(a.Names))
	canon := make([]int32, len(a.Names))
	c.visits += uint64(len(a.Names) + len(a.FieldLookups))
	for j, s := range a.Names {
		c.bytes += 2 * uint64(len(s))
		if first, ok := nameIdx[s]; ok {
			canon[j] = first
		} else {
			nameIdx[s] = int32(j)
			canon[j] = int32(j)
		}
	}
	rowsOf := make(map[[2]int64]apiLookup, len(a.FieldLookups))
	for _, r := range a.FieldLookups {
		key := [2]int64{r[0], int64(canon[r[1]])}
		e, ok := rowsOf[key]
		if !ok {
			rowsOf[key] = apiLookup{first: r[2]}
		} else if !e.hasOther && r[2] != e.first {
			e.other, e.hasOther = r[2], true
			rowsOf[key] = e
		}
	}
	lookups := map[[2]int64]bool{}
	seenAt := make([]int32, len(a.Names)) // the last parent (plus one) a name was met under
	c.visits += un + uint64(len(flat))
	for i, list := range children {
		for k, ch := range list {
			f := nodes[ch].Field
			ix := fieldAt[off[i]+k]
			ok := ix >= 0
			g := ""
			if ok {
				g = a.Names[ix]
			}
			if f != nil {
				c.bytes += 2 * uint64(len(*f))
			}
			if (f == nil) == ok || (ok && g != *f) {
				return &APIDifference{"child_field", int64(i), fmt.Sprintf("%d:%q", k, g), fmt.Sprintf("%d:%s", k, fieldText(f))}, divergences
			}
			if f == nil {
				continue
			}
			fid := int64(-1)
			if first, found := nameIdx[*f]; found {
				fid = int64(first)
				if seenAt[first] == int32(i+1) {
					continue
				}
				seenAt[first] = int32(i + 1)
			}
			lookups[[2]int64{int64(i), fid}] = true
			// the lookup row for this field must name this child
			e, found := rowsOf[[2]int64{int64(i), fid}]
			switch {
			case !found:
				return diff("field_lookup:"+*f, int64(i), -1, ch), divergences
			case e.first != ch:
				return diff("field_lookup:"+*f, int64(i), e.first, ch), divergences
			case e.hasOther:
				return diff("field_lookup:"+*f, int64(i), e.other, ch), divergences
			}
		}
	}
	for _, r := range a.FieldLookups {
		if !lookups[[2]int64{r[0], r[1]}] {
			return diff("field_lookup_extra:"+a.Names[r[1]], r[0], r[2], -1), divergences
		}
	}
	if len(a.Points) == 0 {
		return nil, divergences
	}
	// rootEnd[k] is the largest end among the first k+1 root children, so the first root child
	// ending after a byte is where rootEnd first exceeds it.
	rootEnd := make([]uint32, len(children[0]))
	c.visits += uint64(len(rootEnd))
	var top uint32
	for k, ch := range children[0] {
		top = max(top, nodes[ch].EndByte)
		rootEnd[k] = top
	}
	for _, r := range a.Points {
		b := r[0]
		outsideRoot := b < int64(nodes[0].StartByte) || b > int64(nodes[0].EndByte)
		for k, check := range []string{"descendant_for_byte", "named_descendant_for_byte"} {
			x := r[1+k]
			if x < 0 || x >= int64(n) || (outsideRoot && x != 0) || (!outsideRoot && (int64(nodes[x].StartByte) > b || b > int64(nodes[x].EndByte) || (k == 1 && !nodes[x].Named && x != 0))) {
				return diff(check, x, b, -1), divergences
			}
		}
		lo, hi := 0, len(rootEnd)
		c.visits++
		for lo < hi {
			c.visits++
			if mid := int(uint(lo+hi) >> 1); int64(rootEnd[mid]) > b {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		want := int64(-1)
		if lo < len(rootEnd) {
			want = children[0][lo]
		}
		if r[3] != want {
			return diff("first_child_for_byte", 0, r[3], want), divergences
		}
	}
	return nil, divergences
}

func fieldText(f *string) string {
	if f == nil {
		return "null"
	}
	return strconv.Quote(*f)
}
