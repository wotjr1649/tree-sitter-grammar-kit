package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Tree schemas produced from Session 05 (tree-and-adapter-protocol.md).
const (
	TreeSchema        = "tsgk-tree/r1"
	TreeSummarySchema = "tsgk-tree-summary/r1"
	TreeDigestScheme  = "tsgk-tree-digest/r1"
)

// Point is a 0-based row and a byte column counted from the start of the row.
type Point struct {
	Row    uint32 `json:"row"`
	Column uint32 `json:"column"`
}

// TreeNode is one node of the public ordered CST in cursor preorder. Parent is -1 for the
// root and otherwise an earlier index; Field is the parent-relative field name or nil.
type TreeNode struct {
	Parent     int64   `json:"parent"`
	Type       string  `json:"type"`
	Field      *string `json:"field"`
	Named      bool    `json:"named"`
	Extra      bool    `json:"extra"`
	IsError    bool    `json:"is_error"`
	HasError   bool    `json:"has_error"`
	IsMissing  bool    `json:"is_missing"`
	StartByte  uint32  `json:"start_byte"`
	EndByte    uint32  `json:"end_byte"`
	StartPoint Point   `json:"start_point"`
	EndPoint   Point   `json:"end_point"`
}

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
		pairs := []struct {
			name string
			l, r string
		}{
			{"parent", strconv.FormatInt(x.Parent, 10), strconv.FormatInt(y.Parent, 10)},
			{"type", strconv.Quote(x.Type), strconv.Quote(y.Type)},
			{"field", fieldText(x.Field), fieldText(y.Field)},
			{"named", strconv.FormatBool(x.Named), strconv.FormatBool(y.Named)},
			{"extra", strconv.FormatBool(x.Extra), strconv.FormatBool(y.Extra)},
			{"is_error", strconv.FormatBool(x.IsError), strconv.FormatBool(y.IsError)},
			{"has_error", strconv.FormatBool(x.HasError), strconv.FormatBool(y.HasError)},
			{"is_missing", strconv.FormatBool(x.IsMissing), strconv.FormatBool(y.IsMissing)},
			{"start_byte", strconv.FormatUint(uint64(x.StartByte), 10), strconv.FormatUint(uint64(y.StartByte), 10)},
			{"end_byte", strconv.FormatUint(uint64(x.EndByte), 10), strconv.FormatUint(uint64(y.EndByte), 10)},
			{"start_point", pointText(x.StartPoint), pointText(y.StartPoint)},
			{"end_point", pointText(x.EndPoint), pointText(y.EndPoint)},
		}
		for _, p := range pairs {
			if p.l != p.r {
				return &TreeDifference{Node: i, Field: p.name, Left: p.l, Right: p.r}
			}
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
func TreeDigest(nodes []TreeNode) string {
	h := sha256.New()
	h.Write([]byte("tsgk-tree-digest/r1\n"))
	var buf []byte
	bit := func(v bool) byte {
		if v {
			return '1'
		}
		return '0'
	}
	for i := range nodes {
		n := &nodes[i]
		field := ""
		if n.Field != nil {
			field = *n.Field
		}
		buf = strconv.AppendInt(buf[:0], n.Parent, 10)
		for _, s := range []string{n.Type, field} {
			buf = append(buf, 0)
			buf = strconv.AppendInt(buf, int64(len(s)), 10)
			buf = append(buf, ':')
			buf = append(buf, s...)
		}
		for _, f := range []bool{n.Named, n.Extra, n.IsError, n.HasError, n.IsMissing} {
			buf = append(buf, 0, bit(f))
		}
		for _, v := range []uint32{n.StartByte, n.EndByte, n.StartPoint.Row, n.StartPoint.Column, n.EndPoint.Row, n.EndPoint.Column} {
			buf = append(buf, 0)
			buf = strconv.AppendUint(buf, uint64(v), 10)
		}
		buf = append(buf, '\n')
		h.Write(buf)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ValidateTree checks a complete tree's structure: exactly one root, parent-before-child
// with contiguous preorder ancestry (each parent is the previous node or one of its
// ancestors), and ranges inside the input with start <= end. Sibling spans may overlap,
// repeat or be empty; they are not required to be disjoint or unique.
func ValidateTree(nodes []TreeNode, inputBytes uint64) error {
	if len(nodes) == 0 {
		return fail(KindInvalidInput, "TREE_EMPTY", "", nil)
	}
	stack := []int64{}
	for i := range nodes {
		n := &nodes[i]
		bad := func(code string) error {
			return fail(KindInvalidInput, code, "#/nodes/"+strconv.Itoa(i), nil)
		}
		switch {
		case i == 0 && n.Parent != -1:
			return bad("TREE_ROOT_INVALID")
		case i > 0 && (n.Parent < 0 || n.Parent >= int64(i)):
			return bad("TREE_PARENT_INVALID")
		}
		for len(stack) > 0 && stack[len(stack)-1] != n.Parent {
			stack = stack[:len(stack)-1]
		}
		if i > 0 && len(stack) == 0 {
			return bad("TREE_PREORDER_INVALID")
		}
		stack = append(stack, int64(i))
		if n.StartByte > n.EndByte || uint64(n.EndByte) > inputBytes {
			return bad("TREE_RANGE_INVALID")
		}
		if n.StartPoint.Row > n.EndPoint.Row || (n.StartPoint.Row == n.EndPoint.Row && n.StartPoint.Column > n.EndPoint.Column) {
			return bad("TREE_POINT_INVALID")
		}
	}
	return nil
}
