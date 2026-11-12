package native

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func svcScopeAssets(t *testing.T) (e kit.SvcExpected, inventory []byte) {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	inventory = read("contracts/qualification-c1.json")
	e, err := kit.ParseSvcExpected(read("contracts/query-svc-expected-r1.json"), read("contracts/query-semantic-registry-r1.json"), inventory, read("testdata/native/n461/svc.json"))
	if err != nil {
		t.Fatal(err)
	}
	return e, inventory
}

// The owned native producer records each step's composite from the same observation path the
// replay re-checks. Run all 68 SVC cases through it (the owned plain grammar stands in for the
// inline C# language, as in TestSvcComposite) and compare the recorded segment, range, point,
// history and CodeBehind facts with the independently derived expected values. The inline C#
// tree and the two queries need the real C# grammar and are not judged here.
func TestNativeSvcScopeOwnedProducer(t *testing.T) {
	b := fixtureBuild(t, "plain")
	e, inventory := svcScopeAssets(t)
	parsed, err := kit.ParseQualificationInventory(inventory)
	if err != nil {
		t.Fatal(err)
	}
	var workload kit.QualWorkload
	for _, role := range parsed.ExtraRoles {
		if role.ID == "n461-svc" {
			workload = role.Workloads[0]
		}
	}
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	x.SvcContext = &e.Workload.Context
	ran, steps := 0, 0
	for i, c := range e.Cases {
		if c.Reference {
			continue
		}
		q := workload.Cases[i]
		t.Run(c.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			r := b.RunCase(ctx, x, kit.IncrementalCase{ID: c.ID, Encoding: kit.EncodingUTF8, Edits: q.Edits}, []byte(*q.Source))
			ran++
			if r.ExecutionStatus != kit.StatusCompleted || len(r.Steps) != len(c.Steps) {
				t.Fatalf("%s: %s %s steps %d/%d", c.ID, r.ExecutionStatus, r.Code, len(r.Steps), len(c.Steps))
			}
			for k, s := range r.Steps {
				steps++
				w := c.Steps[k]
				comp := s.Composite
				if comp == nil || s.Step != k || s.SourceBytes != w.Bytes || s.SourceSHA256 != w.SHA256 {
					t.Fatalf("%s step %d: history %+v want %d/%s", c.ID, k, s, w.Bytes, w.SHA256)
				}
				if k > 0 {
					ex := c.Edits[k-1]
					if s.Edit == nil || s.Edit.StartByte != ex.StartByte || s.Edit.OldEndByte != ex.OldEndByte || s.Edit.NewEndByte != ex.NewEndByte ||
						s.Edit.StartPoint != ex.StartPoint || s.Edit.OldEndPoint != ex.OldEndPoint || s.Edit.NewEndPoint != ex.NewEndPoint {
						t.Fatalf("%s step %d: edit %+v want %+v", c.ID, k, s.Edit, ex)
					}
				} else if s.Edit != nil {
					t.Fatalf("%s: step 0 has an edit", c.ID)
				}
				o := w.Observation
				var ranges []kit.Span
				if comp.Inline != nil {
					ranges = comp.Inline.IncludedRanges
				}
				if !reflect.DeepEqual(comp.Directive, o.Directive) || !reflect.DeepEqual(comp.AdditionalDirectives, o.AdditionalDirectives) || !reflect.DeepEqual(comp.Language, o.Language) ||
					!reflect.DeepEqual(comp.CodeBehind, o.CodeBehind) || comp.Coverage != o.Coverage || !reflect.DeepEqual(ranges, o.IncludedRanges) {
					t.Fatalf("%s step %d: composite differs from the independent observation", c.ID, k)
				}
				// the inline tree, when there is one, is parsed in original coordinates inside the range
				if o.IncludedRanges != nil && (comp.Inline == nil || comp.Inline.Tree == nil || comp.Inline.Tree.Nodes[0].StartByte < o.IncludedRanges[0].StartByte || comp.Inline.Tree.Nodes[0].EndByte > o.IncludedRanges[0].EndByte) {
					t.Fatalf("%s step %d: inline tree outside the included range", c.ID, k)
				}
				if (o.IncludedRanges == nil) != (s.Incremental == nil) {
					t.Fatalf("%s step %d: tree presence differs from the included range", c.ID, k)
				}
			}
			if c.Negative != nil {
				// the existing unsupported-language negative: BLOCKED, exact code, no process, no tree
				if r.Assessment != c.Negative.Assessment || r.Code != c.Negative.Code || r.Process != nil || r.Steps[0].Incremental != nil || r.Steps[0].Composite.Inline != nil {
					t.Fatalf("%s negative: %s %s process=%v", c.ID, r.Assessment, r.Code, r.Process != nil)
				}
				if r.Assessment == kit.AssessPass {
					t.Fatalf("%s negative became a pass", c.ID)
				}
			}
		})
	}
	if ran != 68 || steps != 109 {
		t.Fatalf("ran %d cases / %d steps, want 68 / 109", ran, steps)
	}
}
