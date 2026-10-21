package kit

import (
	"strconv"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

// Tree schemas produced from Session 05 (tree-and-adapter-protocol.md).
const (
	TreeSchema        = "tsgk-tree/r1"
	TreeSummarySchema = "tsgk-tree-summary/r1"
	TreeDigestScheme  = "tsgk-tree-digest/r1"
)

// Point is a 0-based row and a byte column counted from the start of the row.
type Point = nativewire.Point

// TreeNode is one node of the public ordered CST in cursor preorder. Parent is -1 for the
// root and otherwise an earlier index; Field is the parent-relative field name or nil.
type TreeNode = nativewire.TreeNode

// TreeDifference is the first difference CompareTrees finds: the preorder node index, the
// public field name ("node_count" when one tree is a prefix of the other) and both values.
type TreeDifference struct {
	Node  int    `json:"node"`
	Field string `json:"field"`
	Left  string `json:"left"`
	Right string `json:"right"`
}

func fieldText(f *string) string {
	if f == nil {
		return "null"
	}
	return strconv.Quote(*f)
}

func pointText(p Point) string {
	return "[" + strconv.FormatUint(uint64(p.Row), 10) + "," + strconv.FormatUint(uint64(p.Column), 10) + "]"
}

// CompareTrees is the single semantic tree comparator: every public field of every node in
// preorder, with no sorting, deduplication or span adjustment. It returns nil when the two
// node sequences are equal.
func CompareTrees(a, b []TreeNode) *TreeDifference {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, y := &a[i], &b[i]
		// Compare values first; format only the first difference in contract order.
		switch {
		case x.Parent != y.Parent:
			return &TreeDifference{i, "parent", strconv.FormatInt(x.Parent, 10), strconv.FormatInt(y.Parent, 10)}
		case x.Type != y.Type:
			return &TreeDifference{i, "type", strconv.Quote(x.Type), strconv.Quote(y.Type)}
		case (x.Field == nil) != (y.Field == nil) || (x.Field != nil && y.Field != nil && *x.Field != *y.Field):
			return &TreeDifference{i, "field", fieldText(x.Field), fieldText(y.Field)}
		case x.Named != y.Named:
			return &TreeDifference{i, "named", strconv.FormatBool(x.Named), strconv.FormatBool(y.Named)}
		case x.Extra != y.Extra:
			return &TreeDifference{i, "extra", strconv.FormatBool(x.Extra), strconv.FormatBool(y.Extra)}
		case x.IsError != y.IsError:
			return &TreeDifference{i, "is_error", strconv.FormatBool(x.IsError), strconv.FormatBool(y.IsError)}
		case x.HasError != y.HasError:
			return &TreeDifference{i, "has_error", strconv.FormatBool(x.HasError), strconv.FormatBool(y.HasError)}
		case x.IsMissing != y.IsMissing:
			return &TreeDifference{i, "is_missing", strconv.FormatBool(x.IsMissing), strconv.FormatBool(y.IsMissing)}
		case x.StartByte != y.StartByte:
			return &TreeDifference{i, "start_byte", strconv.FormatUint(uint64(x.StartByte), 10), strconv.FormatUint(uint64(y.StartByte), 10)}
		case x.EndByte != y.EndByte:
			return &TreeDifference{i, "end_byte", strconv.FormatUint(uint64(x.EndByte), 10), strconv.FormatUint(uint64(y.EndByte), 10)}
		case x.StartPoint != y.StartPoint:
			return &TreeDifference{i, "start_point", pointText(x.StartPoint), pointText(y.StartPoint)}
		case x.EndPoint != y.EndPoint:
			return &TreeDifference{i, "end_point", pointText(x.EndPoint), pointText(y.EndPoint)}
		}
	}
	if len(a) != len(b) {
		n := min(len(a), len(b))
		return &TreeDifference{Node: n, Field: "node_count", Left: strconv.Itoa(len(a)), Right: strconv.Itoa(len(b))}
	}
	return nil
}

// TreeDigest returns the tsgk-tree-digest/r1 sha256 of a preorder node sequence. A full
// tree and a summary of the same bytes, grammar and producer have the same digest.
func TreeDigest(nodes []TreeNode) string { return nativewire.TreeDigest(nodes) }

// ValidateTree checks a complete tree's structure: exactly one root, parent-before-child
// with contiguous preorder ancestry (each parent is the previous node or one of its
// ancestors), and ranges inside the input with start <= end. Sibling spans may overlap,
// repeat or be empty; they are not required to be disjoint or unique.
func ValidateTree(nodes []TreeNode, inputBytes uint64) error {
	if e := nativewire.ValidateTree(nodes, inputBytes); e != nil {
		return fail(KindInvalidInput, e.Code, e.Path, nil)
	}
	return nil
}
