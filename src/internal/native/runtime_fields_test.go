package native

import (
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// Visible aliases own their fields; an unaliased hidden node still inherits them.
// The last inherited entry and a later direct entry exercise both map exits.
func TestRuntimeFieldOwnership(t *testing.T) {
	b := fixtureBuild(t, "runtime_fields")
	for _, tc := range []struct {
		src, parentValue string
		withoutValue     bool
	}{
		{"ax.", "", true}, {"ax.y", "y", false},
		{"nx.", "", true}, {"nx.y", "y", false},
		{"hx.", "x", false}, {"hx.y", "x", false},
	} {
		t.Run(tc.src, func(t *testing.T) {
			r := runOracleCase(t, b, oracleContext(true, "(source_file !value) @without_value"), tc.src, nil)
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != 1 {
				t.Fatalf("field ownership execution: %s %s", r.ExecutionStatus, r.Code)
			}
			if r.Assessment != kit.AssessPass || r.Oracle.API != ClaimPass {
				t.Errorf("field ownership: %s %+v difference=%+v", r.Assessment, r.Oracle, r.Steps[0].Incremental.API.First)
			}
			var value string
			for _, n := range r.Steps[0].Incremental.Tree.Nodes {
				if n.Parent == 0 && n.Field != nil && *n.Field == "value" {
					value = tc.src[n.StartByte:n.EndByte]
					break
				}
			}
			if value != tc.parentValue {
				t.Fatalf("parent value=%q, want %q", value, tc.parentValue)
			}
			captures := streamOf(t, r.Steps[0].Incremental.Queries[0], tc.src)
			if tc.withoutValue {
				if len(captures) != 1 || captures[0] != (capText{"without_value", "source_file", tc.src}) {
					t.Fatalf("negated field captures=%v", captures)
				}
			} else if len(captures) != 0 {
				t.Fatalf("parent has value but negated field matched: %v", captures)
			}
		})
	}
}
