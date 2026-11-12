package nativewire

import "testing"

func apiCompareFixture() ([]TreeNode, WireAPI) {
	body := "body"
	nodes := []TreeNode{
		{Parent: -1, Type: "root", Named: true, StartByte: 0, EndByte: 1},
		{Parent: 0, Type: "missing_a", IsMissing: true},
		{Parent: 0, Type: "missing_b", IsMissing: true},
		{Parent: 0, Type: "body", Field: &body, Named: true, StartByte: 0, EndByte: 1},
	}
	api := WireAPI{
		Revision: "tsgk-api/r1",
		Nodes: [][]int64{
			{-1, -1, -1, -1, -1, 1, 3, 3, 1, 4, 0, 0, 0},
			{0, -1, 2, -1, 3, -1, -1, 0, 0, 1, 1, 1, 1},
			{0, 1, 3, -1, 3, -1, -1, 0, 0, 1, 1, 2, 2},
			{0, 2, -1, -1, -1, -1, -1, 0, 0, 1, 1, 3, 3},
		},
		ChildFields:  [][]int64{{0, 2, 0}},
		FieldLookups: [][]int64{{0, 0, 3}},
		Points:       [][]int64{{0, 0, 0, 3}},
		Names:        []string{body},
	}
	return nodes, api
}

func TestCompareAPIZeroWidthFieldsAndPoints(t *testing.T) {
	nodes, api := apiCompareFixture()
	if diff, divergences := CompareAPI(nodes, &api); diff != nil || len(divergences) != 0 {
		t.Fatalf("valid API: diff=%+v divergences=%+v", diff, divergences)
	}

	zeroWidth := api
	zeroWidth.Nodes = make([][]int64, len(api.Nodes))
	for i := range api.Nodes {
		zeroWidth.Nodes[i] = append([]int64(nil), api.Nodes[i]...)
	}
	zeroWidth.Nodes[1][2] = 3 // The node API skips the zero-width sibling at index 2.
	if diff, divergences := CompareAPI(nodes, &zeroWidth); diff != nil || len(divergences) != 1 || divergences[0].Check != "position_navigation:next_sibling" {
		t.Fatalf("zero-width navigation: diff=%+v divergences=%+v", diff, divergences)
	}

	badField := api
	badField.Names = []string{"other"}
	if diff, _ := CompareAPI(nodes, &badField); diff == nil || diff.Check != "child_field" {
		t.Fatalf("field mutation: %+v", diff)
	}

	badPoint := api
	badPoint.Points = [][]int64{{0, 0, 0, 2}}
	if diff, _ := CompareAPI(nodes, &badPoint); diff == nil || diff.Check != "first_child_for_byte" {
		t.Fatalf("point mutation: %+v", diff)
	}
}
