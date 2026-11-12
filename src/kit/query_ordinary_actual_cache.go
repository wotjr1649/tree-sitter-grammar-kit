package kit

import (
	"fmt"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// ordinaryActualKey names one checked native tree. cache.checked returns the same node table (and API
// table) for every query of one (case, step, tree role) for the whole pass, and nothing writes through
// it. The pointers keep the arrays alive, so an address is never reused for another tree while the
// pass runs.
type ordinaryActualKey struct {
	nodes *TreeNode
	count int
	api   *nativewire.WireAPI
}

// ordinaryActualEntry holds the query-independent outcomes of one actual tree, each bound to what it
// was computed against: the source pin (structure and points), or the shared expected tree (comparison).
type ordinaryActualEntry struct {
	tree    map[OrdinaryPin]ordinaryActualTreeCheck
	api     *ordinaryTreeCheck
	compare map[*ordinaryExpectedTree]ordinaryTreeCompare
}

// ordinaryActualTreeCheck is the structure outcome of an actual tree against its bound source.
type ordinaryActualTreeCheck struct {
	invalid string // ValidateTree code, "" when valid
	points  bool   // the node points do not match the source
}

// ordinaryTreeCompare is the outcome of comparing an actual tree with an expected tree.
type ordinaryTreeCompare struct {
	changed, fieldOnly bool
	detail             string // the first difference, when not field-only
}

// ordinaryActualEntry returns the shared entry of the actual tree, or nil without a share. Each call
// is one priced lookup; a new entry is priced like the expected-asset entries.
func (b *eligibilityBudget) ordinaryActualEntry(actual nativewire.Tree) (*ordinaryActualEntry, *Error) {
	share := b.decoded
	if share == nil || len(actual.Nodes) == 0 {
		return nil, nil
	}
	if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-actual-cache-lookup"); e != nil {
		return nil, e
	}
	key := ordinaryActualKey{nodes: &actual.Nodes[0], count: len(actual.Nodes), api: actual.API}
	if hit, ok := share.actual[key]; ok {
		return hit, nil
	}
	if e := b.charge(1, ordinaryDecodedEntryLogical, "ordinary-actual-cache"); e != nil {
		return nil, e
	}
	entry := &ordinaryActualEntry{}
	share.actual[key] = entry
	return entry, nil
}

// ordinaryActualTreeCheck validates an actual tree and its points against the tuple's source. The
// outcome depends only on (tree, source pin): the source bytes were hashed against the pin for this
// tuple, so with a share it is computed once per pair and later uses pay only the lookup.
func (b *eligibilityBudget) ordinaryActualTreeCheck(entry *ordinaryActualEntry, nodes []TreeNode, pin OrdinaryPin, source []byte) (ordinaryActualTreeCheck, *Error) {
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-actual-cache-lookup"); e != nil {
			return ordinaryActualTreeCheck{}, e
		}
		if got, ok := entry.tree[pin]; ok {
			return got, nil
		}
	}
	count := uint64(len(nodes))
	logical, e := eligibilityProduct("ordinary-actual-tree-check", count, 512)
	if e != nil {
		return ordinaryActualTreeCheck{}, e
	}
	work, e := eligibilityProduct("ordinary-actual-tree-check", count, 16)
	if e != nil {
		return ordinaryActualTreeCheck{}, e
	}
	if e := b.charge(work+uint64(len(source))+1, logical, "ordinary-actual-tree-check"); e != nil {
		return ordinaryActualTreeCheck{}, e
	}
	ordinaryVisitObserved("tree-check", count+uint64(len(source)))
	var got ordinaryActualTreeCheck
	if err := nativewire.ValidateTree(nodes, uint64(len(source))); err != nil {
		got.invalid = err.Code
	}
	got.points = !ordinaryTreePointsValid(nodes, source)
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedEntryLogical+uint64(len(got.invalid)), "ordinary-actual-cache"); e != nil {
			return ordinaryActualTreeCheck{}, e
		}
		if entry.tree == nil {
			entry.tree = map[OrdinaryPin]ordinaryActualTreeCheck{}
		}
		entry.tree[pin] = got
	}
	return got, nil
}

// ordinaryActualAPICheck is the API-table outcome of an actual tree (its shape, then the comparison
// with the node table); code is "" when the table is valid and agrees. actual.API must not be nil.
func (b *eligibilityBudget) ordinaryActualAPICheck(entry *ordinaryActualEntry, actual nativewire.Tree) (ordinaryTreeCheck, *Error) {
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-actual-cache-lookup"); e != nil {
			return ordinaryTreeCheck{}, e
		}
		if entry.api != nil {
			return *entry.api, nil
		}
	}
	api := actual.API
	apiRows := uint64(len(api.Nodes) + len(api.ChildFields) + len(api.FieldLookups) + len(api.Points) + len(api.Names))
	work, logical := nativewire.CompareAPIWork(actual.Nodes, api), nativewire.CompareAPILogical(actual.Nodes, api)
	// The shape walk and the comparison are priced together, before either runs.
	if e := b.charge(work, logical, "ordinary-api-compare"); e != nil {
		return ordinaryTreeCheck{}, e
	}
	ordinaryVisitObserved("api-compare", uint64(len(actual.Nodes))+apiRows)
	var got ordinaryTreeCheck
	if !ordinaryAPIShapeValid(api, len(actual.Nodes)) {
		got = ordinaryTreeCheck{"ORDINARY_API_INVALID", "API table의 checked shape가 유효하지 않다"}
	} else if difference, _ := nativewire.CompareAPI(actual.Nodes, api); difference != nil {
		got = ordinaryTreeCheck{"ORDINARY_API_MISMATCH", fmt.Sprintf("API %s node %d %s != %s", difference.Check, difference.Node, difference.Got, difference.Want)}
	}
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedEntryLogical+uint64(len(got.detail)), "ordinary-actual-cache"); e != nil {
			return ordinaryTreeCheck{}, e
		}
		entry.api = &got
	}
	return got, nil
}

// ordinaryActualTreeCompare compares an actual tree with the expected tree the tuple was bound to. The
// outcome depends only on the two node tables, so with a share it is computed once per pair.
func (b *eligibilityBudget) ordinaryActualTreeCompare(entry *ordinaryActualEntry, actual []TreeNode, expected *ordinaryExpectedTree) (ordinaryTreeCompare, *Error) {
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedLookupLogical, "ordinary-actual-cache-lookup"); e != nil {
			return ordinaryTreeCompare{}, e
		}
		if got, ok := entry.compare[expected]; ok {
			return got, nil
		}
	}
	if e := b.charge(16*uint64(len(expected.Tree))+1, 0, "ordinary-tree-compare"); e != nil {
		return ordinaryTreeCompare{}, e
	}
	ordinaryVisitObserved("tree-compare", uint64(len(expected.Tree)))
	var got ordinaryTreeCompare
	if fieldOnly, changed := ordinaryTreeFieldOnlyDifference(actual, expected.Tree); changed {
		got.changed, got.fieldOnly = true, fieldOnly
		if !fieldOnly {
			d := CompareTrees(actual, expected.Tree)
			got.detail = fmt.Sprintf("tree node %d %s %s != %s", d.Node, d.Field, d.Left, d.Right)
		}
	}
	if entry != nil {
		if e := b.charge(1, ordinaryDecodedEntryLogical+uint64(len(got.detail)), "ordinary-actual-cache"); e != nil {
			return ordinaryTreeCompare{}, e
		}
		if entry.compare == nil {
			entry.compare = map[*ordinaryExpectedTree]ordinaryTreeCompare{}
		}
		entry.compare[expected] = got
	}
	return got, nil
}
