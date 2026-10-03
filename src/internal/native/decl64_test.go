package native

import (
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// R1 M-1: with the maximum 64 declarations, only real matches produce items (the 64th
// mapping entry is not confused with a computed flag).
func TestSixtyFourDeclarations(t *testing.T) {
	b := fixtureBuild(t, "plain")
	req := baseRequest("a = 1;\nb = 2;\n", "native-parse-edit")
	for i := 0; i < 63; i++ {
		req.Declarations = append(req.Declarations, kit.NativeDeclaration{Fact: "type_declaration", Node: "no_such_node", Name: "node"})
	}
	req.Declarations = append(req.Declarations, kit.NativeDeclaration{Fact: "type_declaration", Node: "assignment", Name: "field:name"})
	_, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
	if err != nil || resp.Status != kit.StatusCompleted {
		t.Fatalf("%v %s %s", err, resp.Status, resp.Code)
	}
	items := *resp.Steps[0].Incremental.Declarations
	if len(items) != 2 || items[0].NodeType != "assignment" || items[0].Status != "PASS" {
		t.Fatalf("declaration items %+v", items)
	}
}
