package native

import (
	"slices"
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

// Hard-coded ranges and captures keep the cursor/API comparison from defining
// its own expected ownership when hidden fields shadow or recovery adds extras.
func TestRuntimeFieldRecoveryAndShadow(t *testing.T) {
	b := fixtureBuild(t, "runtime_fields")
	for _, tc := range []struct {
		src, parentType   string
		withoutType       bool
		errorWithoutValue []capText
	}{
		{"sx", "", true, nil}, {"sx!", "!", false, nil},
		{"hx.yx.", "", true, nil},
		{"qa<b!", "", true, []capText{{"without_value", "ERROR", "<"}, {"without_value", "ERROR", "<"}}},
		{"qa!", "", true, nil}, {"qab!", "", true, nil},
	} {
		t.Run(tc.src, func(t *testing.T) {
			r := runOracleCase(t, b, oracleContext(true, "(source_file !type) @without_type", "(ERROR !value) @without_value"), tc.src, nil)
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != 1 {
				t.Fatalf("field recovery execution: %s %s", r.ExecutionStatus, r.Code)
			}
			if r.Assessment != kit.AssessPass || r.Oracle.API != ClaimPass {
				t.Errorf("field recovery: %s %+v difference=%+v", r.Assessment, r.Oracle, r.Steps[0].Incremental.API.First)
			}
			var value string
			for _, n := range r.Steps[0].Incremental.Tree.Nodes {
				if n.Parent == 0 && n.Field != nil && *n.Field == "type" {
					value = tc.src[n.StartByte:n.EndByte]
					break
				}
			}
			if value != tc.parentType {
				t.Fatalf("parent type=%q, want %q", value, tc.parentType)
			}
			want := []capText(nil)
			if tc.withoutType {
				want = []capText{{"without_type", "source_file", tc.src}}
			}
			for i, expected := range [][]capText{want, tc.errorWithoutValue} {
				q := r.Steps[0].Incremental.Queries[i]
				got := streamOf(t, q, tc.src)
				if q.Status != kit.StatusCompleted || q.Evaluation != EvalStructural || !slices.Equal(got, expected) {
					t.Fatalf("query %d: %s %s captures=%v, want %v", i, q.Status, q.Evaluation, got, expected)
				}
			}
		})
	}
}

// S05 locators call child_by_field_name; assert their names independently of API
// and query equality, including an unknown field and a field inside ERROR.
func TestRuntimeFieldByName(t *testing.T) {
	b := fixtureBuild(t, "runtime_fields")
	for _, tc := range []struct {
		src, node, locator string
		start, end         uint32
		status             string
	}{
		{"sx", "source_file", "field:type", 0, 0, "NAME_MISSING"},
		{"sx!", "source_file", "field:type", 2, 3, "PASS"},
		{"sx!", "source_file", "field:unknown", 0, 0, "NAME_MISSING"},
		{"hx.yx.", "source_file", "child:ERROR/field:value", 1, 2, "HAS_ERROR"},
		{"qa<b!", "source_file", "field:type", 0, 0, "NAME_MISSING"},
		{"ax.y", "source_file", "field:value", 3, 4, "PASS"},
		{"nx.y", "source_file", "field:value", 3, 4, "PASS"},
		{"hx.y", "source_file", "field:value", 1, 2, "PASS"},
	} {
		t.Run(tc.src+"/"+tc.locator, func(t *testing.T) {
			req := baseRequest(tc.src, "native-parse-edit")
			req.Declarations = []kit.NativeDeclaration{{Fact: "type_declaration", Node: tc.node, Name: tc.locator}}
			_, resp, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
			if err != nil || resp.Status != kit.StatusCompleted || len(resp.Steps) != 1 || resp.Steps[0].Incremental.Declarations == nil {
				t.Fatalf("by-name execution: %v %s %s", err, resp.Status, resp.Code)
			}
			items := *resp.Steps[0].Incremental.Declarations
			if len(items) != 1 || items[0].Status != tc.status {
				t.Fatalf("by-name declarations=%+v, want one %s", items, tc.status)
			}
			name := items[0].Name
			if tc.status == "NAME_MISSING" {
				if name != nil {
					t.Fatalf("absent field returned %+v", name)
				}
			} else if name == nil || name.StartByte != tc.start || name.EndByte != tc.end {
				t.Fatalf("by-name range=%+v, want [%d,%d)", name, tc.start, tc.end)
			}
		})
	}
}
