package nativewire

import (
	"fmt"
	"slices"
	"strconv"
)

// compareAPIReference is the pre-#362-bounds CompareAPI, kept verbatim as the oracle of the differential tests.
func compareAPIReference(nodes []TreeNode, a *WireAPI) (*APIDifference, []APIDifference) {
	var divergences []APIDifference
	n := len(nodes)
	children := make([][]int64, n)
	depth := make([]int64, n)
	size := make([]int64, n)
	for i := n - 1; i >= 0; i-- {
		size[i]++
		if p := nodes[i].Parent; p >= 0 {
			size[p] += size[i]
		}
	}
	for i, nd := range nodes {
		if nd.Parent >= 0 {
			children[nd.Parent] = append(children[nd.Parent], int64(i))
			depth[i] = depth[nd.Parent] + 1
		}
	}
	diff := func(check string, node int64, got, want int64) *APIDifference {
		return &APIDifference{check, node, strconv.FormatInt(got, 10), strconv.FormatInt(want, 10)}
	}
	// positional reports whether the node API's sibling got differs from the cursor's only by
	// skipping zero-width siblings at x's boundary (x's end going forward, its start going
	// back), as the pinned runtime's position-based search does: every sibling it passed over
	// in that direction, the cursor's sibling included, is such a node, and got is null or
	// the next sibling after them. x itself is never on the walked side, so a node named as
	// its own sibling is a difference.
	positional := func(x, got int64, step int, named bool) bool {
		p := nodes[x].Parent
		if p < 0 {
			return false
		}
		b := nodes[x].EndByte
		if step < 0 {
			b = nodes[x].StartByte
		}
		sib := children[p]
		skipped := 0
		for k := slices.Index(sib, x) + step; k >= 0 && k < len(sib); k += step {
			c := sib[k]
			if named && !nodes[c].Named {
				continue
			}
			if c == got {
				return skipped > 0
			}
			if nodes[c].StartByte != b || nodes[c].EndByte != b {
				return false
			}
			skipped++
		}
		return got == -1 && skipped > 0
	}
	sibling := func(i int64, step int, named bool) int64 {
		p := nodes[i].Parent
		if p < 0 {
			return -1
		}
		sib := children[p]
		k := slices.Index(sib, i)
		for k += step; k >= 0 && k < len(sib); k += step {
			if !named || nodes[sib[k]].Named {
				return sib[k]
			}
		}
		return -1
	}
	first := func(list []int64, named bool) int64 {
		for _, c := range list {
			if !named || nodes[c].Named {
				return c
			}
		}
		return -1
	}
	for i, row := range a.Nodes {
		x := int64(i)
		named := int64(0)
		for _, c := range children[i] {
			if nodes[c].Named {
				named++
			}
		}
		want := []int64{nodes[i].Parent, sibling(x, -1, false), sibling(x, 1, false), sibling(x, -1, true), sibling(x, 1, true),
			first(children[i], false), first(children[i], true), int64(len(children[i])), named, size[i], depth[i], x, x}
		checks := []string{"parent", "prev_sibling", "next_sibling", "prev_named_sibling", "next_named_sibling", "first_child", "first_named_child",
			"child_count", "named_child_count", "descendant_count", "cursor_depth", "cursor_descendant_index", "cursor_node"}
		for k := range want {
			if row[k] == want[k] {
				continue
			}
			if k >= 1 && k <= 4 && positional(x, row[k], []int{-1, 1, -1, 1}[k-1], k >= 3) {
				divergences = append(divergences, *diff("position_navigation:"+checks[k], x, row[k], want[k]))
				continue
			}
			return diff(checks[k], x, row[k], want[k]), divergences
		}
	}
	// field name of each child: exactly the cursor fields
	got := map[[2]int64]string{}
	for _, r := range a.ChildFields {
		if r[1] >= int64(len(children[r[0]])) {
			return diff("child_field_ordinal", r[0], r[1], int64(len(children[r[0]]))), divergences
		}
		got[[2]int64{r[0], r[1]}] = a.Names[r[2]]
	}
	lookups := map[[2]int64]bool{}
	for i, list := range children {
		seen := map[string]bool{}
		for k, c := range list {
			f := nodes[c].Field
			key := [2]int64{int64(i), int64(k)}
			g, ok := got[key]
			if (f == nil) == ok || (ok && g != *f) {
				return &APIDifference{"child_field", int64(i), fmt.Sprintf("%d:%q", k, g), fmt.Sprintf("%d:%s", k, fieldTextReference(f))}, divergences
			}
			if f != nil && !seen[*f] {
				seen[*f] = true
				lookups[[2]int64{int64(i), int64(slices.Index(a.Names, *f))}] = true
				// the lookup row for this field must name this child
				found := false
				for _, r := range a.FieldLookups {
					if r[0] == int64(i) && a.Names[r[1]] == *f {
						if r[2] != c {
							return diff("field_lookup:"+*f, int64(i), r[2], c), divergences
						}
						found = true
					}
				}
				if !found {
					return diff("field_lookup:"+*f, int64(i), -1, c), divergences
				}
			}
		}
	}
	for _, r := range a.FieldLookups {
		if !lookups[[2]int64{r[0], r[1]}] {
			return diff("field_lookup_extra:"+a.Names[r[1]], r[0], r[2], -1), divergences
		}
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
		want := int64(-1)
		for _, c := range children[0] {
			if int64(nodes[c].EndByte) > b {
				want = c
				break
			}
		}
		if r[3] != want {
			return diff("first_child_for_byte", 0, r[3], want), divergences
		}
	}
	return nil, divergences
}
func fieldTextReference(f *string) string {
	if f == nil {
		return "null"
	}
	return strconv.Quote(*f)
}
