package kit_test

import (
	"context"
	"fmt"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// A caller owns both node-types.json snapshots (here literals; usually os.ReadFile of
// the old and new grammar's src/node-types.json). No parser, process or network is used.
func ExampleSchemaDiff() {
	before := []byte(`[{"type":"call","named":true,"fields":{"function":{"multiple":false,"required":true,"types":[{"type":"identifier","named":true}]}}},{"type":"identifier","named":true}]`)
	after := []byte(`[{"type":"call","named":true,"fields":{"function":{"multiple":false,"required":false,"types":[{"type":"identifier","named":true},{"type":"member","named":true}]}}},{"type":"identifier","named":true},{"type":"member","named":true}]`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	check, err := kit.SchemaCheck(ctx, kit.SchemaCheckRequest{Input: kit.SchemaInput{Name: "old/node-types.json", Data: before}, Limits: kit.DefaultSchemaLimits()})
	if err != nil {
		panic(err)
	}
	fmt.Println("check:", check.Assessment, check.Input.Counts.Nodes, "nodes")
	diff, err := kit.SchemaDiff(ctx, kit.SchemaDiffRequest{
		Baseline:  kit.SchemaInput{Name: "old/node-types.json", Data: before},
		Candidate: kit.SchemaInput{Name: "new/node-types.json", Data: after},
		Limits:    kit.DefaultSchemaLimits(),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("diff:", diff.Assessment)
	for _, d := range diff.Differences {
		fmt.Println(d.Code, d.Risk, d.Node.Type, d.Field, string(d.Before), "->", string(d.After))
	}
	// Output:
	// check: PASS 2 nodes
	// diff: FAIL
	// FIELD_REQUIRED_CHANGED CARDINALITY_WIDENED call function true -> false
	// FIELD_TYPE_ADDED ADDITION call function null -> {"type":"member","named":true}
	// NODE_ADDED ADDITION member  null -> {"type":"member","named":true}
}
