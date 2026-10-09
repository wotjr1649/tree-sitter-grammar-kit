package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func field(s string) *string { return &s }

// two same-span zero-width siblings and a duplicated parent/child range
func sampleTree() []TreeNode {
	return []TreeNode{
		{Parent: -1, Type: "source_file", Named: true, StartByte: 0, EndByte: 4, EndPoint: Point{0, 4}},
		{Parent: 0, Type: "assignment", Named: true, HasError: true, StartByte: 0, EndByte: 4, EndPoint: Point{0, 4}},
		{Parent: 1, Type: "identifier", Field: field("name"), Named: true, StartByte: 0, EndByte: 1, EndPoint: Point{0, 1}},
		{Parent: 1, Type: ")", IsMissing: true, StartByte: 4, EndByte: 4, StartPoint: Point{0, 4}, EndPoint: Point{0, 4}},
		{Parent: 1, Type: ";", IsMissing: true, StartByte: 4, EndByte: 4, StartPoint: Point{0, 4}, EndPoint: Point{0, 4}},
	}
}

// S05-A05/A12: the comparator keeps order, duplicates and every field; reordering two
// same-span siblings, dropping one, or changing a flag or a field name is a difference.
func TestCompareTrees(t *testing.T) {
	a := sampleTree()
	if err := ValidateTree(a, 4); err != nil {
		t.Fatalf("same-span siblings and duplicated ranges are valid: %v", err)
	}
	if d := CompareTrees(a, sampleTree()); d != nil {
		t.Fatalf("equal trees differ: %+v", d)
	}
	swapped := sampleTree()
	swapped[3], swapped[4] = swapped[4], swapped[3]
	if d := CompareTrees(a, swapped); d == nil || d.Node != 3 || d.Field != "type" {
		t.Fatalf("sibling order: %+v", d)
	}
	if d := CompareTrees(a, sampleTree()[:4]); d == nil || d.Field != "node_count" || d.Node != 4 {
		t.Fatalf("dropped duplicate: %+v", d)
	}
	for name, mutate := range map[string]func(*TreeNode){
		"extra":       func(n *TreeNode) { n.Extra = true },
		"is_error":    func(n *TreeNode) { n.IsError = true },
		"has_error":   func(n *TreeNode) { n.HasError = !n.HasError },
		"is_missing":  func(n *TreeNode) { n.IsMissing = !n.IsMissing },
		"named":       func(n *TreeNode) { n.Named = !n.Named },
		"field":       func(n *TreeNode) { n.Field = field("other") },
		"start_byte":  func(n *TreeNode) { n.StartByte++ },
		"end_point":   func(n *TreeNode) { n.EndPoint.Column++ },
		"start_point": func(n *TreeNode) { n.StartPoint.Row++ },
	} {
		b := sampleTree()
		mutate(&b[2])
		if d := CompareTrees(a, b); d == nil || d.Node != 2 || d.Field != name {
			t.Errorf("%s: %+v", name, d)
		}
	}
	nilField := sampleTree()
	nilField[2].Field = nil
	if d := CompareTrees(a, nilField); d == nil || d.Field != "field" || d.Right != "null" {
		t.Fatalf("absent field must differ from a name: %+v", d)
	}
}

func TestValidateTree(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func([]TreeNode) []TreeNode
		code   string
	}{
		"root-field":     {func(n []TreeNode) []TreeNode { f := "name"; n[0].Field = &f; return n }, "TREE_FIELD_INVALID"},
		"empty-field":    {func(n []TreeNode) []TreeNode { f := ""; n[2].Field = &f; return n }, "TREE_FIELD_INVALID"},
		"empty":          {func(n []TreeNode) []TreeNode { return nil }, "TREE_EMPTY"},
		"two-roots":      {func(n []TreeNode) []TreeNode { n[1].Parent = -1; return n }, "TREE_PARENT_INVALID"},
		"forward-parent": {func(n []TreeNode) []TreeNode { n[2].Parent = 3; return n }, "TREE_PARENT_INVALID"},
		"range":          {func(n []TreeNode) []TreeNode { n[2].EndByte = 9; return n }, "TREE_RANGE_INVALID"},
		"inverted":       {func(n []TreeNode) []TreeNode { n[2].StartByte = 2; return n }, "TREE_RANGE_INVALID"},
		"point":          {func(n []TreeNode) []TreeNode { n[2].StartPoint = Point{1, 0}; return n }, "TREE_POINT_INVALID"},
		"root-not-first": {func(n []TreeNode) []TreeNode { n[0].Parent = 0; return n }, "TREE_ROOT_INVALID"},
	} {
		err := ValidateTree(tc.mutate(sampleTree()), 4)
		var ke *Error
		if tc.code == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if !errors.As(err, &ke) || ke.Code != tc.code {
			t.Errorf("%s: %v, want %s", name, err, tc.code)
		}
	}
	// A parent that is not on the current ancestor chain breaks contiguous preorder.
	bad := sampleTree()
	bad = append(bad, TreeNode{Parent: 2, Type: "x", StartByte: 0, EndByte: 1, EndPoint: Point{0, 1}})
	var ke *Error
	if err := ValidateTree(bad, 4); !errors.As(err, &ke) || ke.Code != "TREE_PREORDER_INVALID" {
		t.Fatalf("non-contiguous ancestry: %v", err)
	}
}

// The digest preimage is spelled out here independently of TreeDigest.
func TestTreeDigestVector(t *testing.T) {
	nodes := sampleTree()[:3]
	pre := "tsgk-tree-digest/r1\n" +
		"-1\x0011:source_file\x000:\x001\x000\x000\x000\x000\x000\x004\x000\x000\x000\x004\n" +
		"0\x0010:assignment\x000:\x001\x000\x000\x001\x000\x000\x004\x000\x000\x000\x004\n" +
		"1\x0010:identifier\x004:name\x001\x000\x000\x000\x000\x000\x001\x000\x000\x000\x001\n"
	sum := sha256.Sum256([]byte(pre))
	if got := TreeDigest(nodes); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest %s", got)
	}
}

// S05-A16: the embedded table is the pinned index-euc-kr and the driver header is its
// mechanical transformation.
func TestCP949Table(t *testing.T) {
	sum := sha256.Sum256(euckrIndex)
	if hex.EncodeToString(sum[:]) != EUCKRIndexSHA256 || len(euckrIndex) != 671621 || !strings.Contains(string(euckrIndex), "# Identifier: "+EUCKRIdentifier+"\n") {
		t.Fatal("embedded index-euc-kr identity changed")
	}
	if r, ok := CP949Rune(0xC7, 0xD1); !ok || r != '한' {
		t.Fatalf("C7D1 -> %U %v", r, ok)
	}
	for _, p := range [][2]byte{{0xC9, 0xA1}, {0xFE, 0xA1}, {0x81, 0x40}, {0x80, 0x41}, {0xA1, 0xFF}} {
		if CP949Pair(p[0], p[1]) {
			t.Errorf("%X%X must not be a mapped pair", p[0], p[1])
		}
	}
	notice, err := os.ReadFile(filepath.Join("data", "NOTICE-index-euc-kr.md"))
	if err != nil || !strings.Contains(string(notice), "CC BY 4.0") && !strings.Contains(string(notice), "Creative Commons Attribution") || !strings.Contains(string(notice), "BSD 3-Clause") {
		t.Fatalf("licence notice missing: %v", err)
	}
}

func TestCP949TableHeader(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "drivers", "native-c", "cp949_table.h"))
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "static const uint16_t tsgk_euc_kr[23940] = {\n")
	if !ok {
		t.Fatal("header declaration missing")
	}
	var b strings.Builder
	tab := euckrTable()
	for i := 0; i < euckrPointers; i += 16 {
		for k := i; k < i+16 && k < euckrPointers; k++ {
			if k > i {
				b.WriteByte(',')
			}
			b.WriteString("0x" + hex4(tab[k]))
		}
		b.WriteString(",\n")
	}
	b.WriteString("};\n")
	if body != b.String() {
		t.Fatal("cp949_table.h does not match the pinned index")
	}
}

func hex4(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>12], digits[v>>8&15], digits[v>>4&15], digits[v&15]})
}

// S05-A07/A17: edit validation, boundaries and byte points (no Unicode scalar counting).
func TestApplyEdits(t *testing.T) {
	src := []byte("a\r\n\xED\x95\x9C\xFFb\nc")
	v, p, err := ApplyEdits(EncodingUTF8, src, []Edit{{StartByte: 6, OldEndByte: 7, NewEndByte: 8, Old: []byte{0xFF}, New: []byte("xy")}}, 64)
	if err != nil || string(v[1]) != "a\r\n\xED\x95\x9Cxyb\nc" {
		t.Fatalf("%v %q", err, v)
	}
	if p[0].Start != (Point{1, 3}) || p[0].OldEnd != (Point{1, 4}) || p[0].NewEnd != (Point{1, 5}) {
		t.Fatalf("points %+v", p[0])
	}
	if got := PointAt(EncodingUTF8, v[1], len(v[1])); got != (Point{2, 1}) {
		t.Fatalf("eof point %+v", got)
	}
	le := []byte{'a', 0, '\n', 0, 'b', 0}
	if got := PointAt(EncodingUTF16LE, le, 4); got != (Point{1, 0}) {
		t.Fatalf("utf16 point %+v", got)
	}
	for name, tc := range map[string]struct {
		enc  string
		src  []byte
		e    Edit
		code string
	}{
		"range":         {EncodingUTF8, src, Edit{StartByte: 3, OldEndByte: 99}, "EDIT_RANGE"},
		"inverted":      {EncodingUTF8, src, Edit{StartByte: 4, OldEndByte: 3}, "EDIT_RANGE"},
		"old":           {EncodingUTF8, src, Edit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, Old: []byte("z"), New: []byte("q")}, "EDIT_OLD_MISMATCH"},
		"length":        {EncodingUTF8, src, Edit{StartByte: 0, OldEndByte: 1, NewEndByte: 3, Old: []byte("a"), New: []byte("q")}, "EDIT_LENGTH_INCONSISTENT"},
		"empty":         {EncodingUTF8, src, Edit{StartByte: 1, OldEndByte: 1, NewEndByte: 1}, "EDIT_EMPTY"},
		"split-utf8":    {EncodingUTF8, src, Edit{StartByte: 4, OldEndByte: 4, NewEndByte: 5, New: []byte("x")}, "EDIT_SPLITS_CHARACTER"},
		"too-large":     {EncodingUTF8, src, Edit{StartByte: 0, OldEndByte: 0, NewEndByte: 60, New: make([]byte, 60)}, "INPUT_TOO_LARGE"},
		"odd-utf16":     {EncodingUTF16LE, le, Edit{StartByte: 1, OldEndByte: 1, NewEndByte: 3, New: []byte{'x', 0}}, "EDIT_ODD_UTF16"},
		"utf16-nul":     {EncodingUTF16LE, le, Edit{StartByte: 0, OldEndByte: 2, NewEndByte: 2, Old: []byte{'a', 0}, New: []byte{0, 0}}, "EDIT_ENCODING_INVALID"},
		"split-cp949":   {EncodingCP949, []byte("a\xC7\xD1"), Edit{StartByte: 2, OldEndByte: 2, NewEndByte: 3, New: []byte("x")}, "EDIT_SPLITS_CHARACTER"},
		"cp949-invalid": {EncodingCP949, []byte("a\xC7\xD1"), Edit{StartByte: 0, OldEndByte: 1, NewEndByte: 2, Old: []byte("a"), New: []byte{0xFE, 0xA1}}, "EDIT_ENCODING_INVALID"},
	} {
		if _, _, err := ApplyEdits(tc.enc, tc.src, []Edit{tc.e}, 64); !hasErrCode(err, tc.code) {
			t.Errorf("%s: %v, want %s", name, err, tc.code)
		}
	}
	if _, _, err := ApplyEdits(EncodingUTF16BE, []byte{0xD8, 0x3D}, nil, 64); !hasErrCode(err, "SOURCE_ENCODING_INVALID") {
		t.Fatalf("lone surrogate source: %v", err)
	}
}

func hasErrCode(err error, code string) bool {
	var ke *Error
	return errors.As(err, &ke) && ke.Code == code
}
