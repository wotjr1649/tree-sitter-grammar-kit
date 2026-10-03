package native

import (
	"context"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// N461-SVC-INLINE/BOUNDARY/ENCODING (mechanism, owned grammar standing in for the inline
// language): the inline segment is parsed through included ranges in original
// coordinates, every edit step recomputes the directive and range, and incremental and
// fresh trees still agree; a step without C# inline code runs no driver at all.
func TestSvcComposite(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	src := "<%@ ServiceHost Language=\"C#\" Service=\"App.Svc\" %>\r\nz = [1, 2];\r\na = f(1);\r\n"
	run := func(id, s string, edits ...edit) CaseResult {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return b.RunCase(ctx, x, kit.IncrementalCase{ID: id, Encoding: kit.EncodingUTF8, Edits: editsBy(s, edits...)}, []byte(s))
	}
	r := run("svc-inline", src, edit{"f(1)", "f(2, 3)"}, edit{"Service=\"App.Svc\"", "Service=\"App.Svc2\" Debug=\"true\""})
	if r.Assessment != kit.AssessPass || len(r.Steps) != 3 {
		t.Fatalf("%s %s %+v", r.Assessment, r.Code, r.Claims)
	}
	for _, s := range r.Steps {
		c := s.Composite
		if c == nil || c.Schema != kit.SvcCompositeSchema || c.Inline == nil || c.Inline.Tree == nil || c.Coverage.Inline != "OBSERVED" {
			t.Fatalf("step %d composite %+v", s.Step, c)
		}
		root := c.Inline.Tree.Nodes[0]
		g := c.Inline.IncludedRanges[0]
		if root.StartByte < g.StartByte || root.EndByte > g.EndByte || root.StartPoint.Row != 1 { // original coordinates, after the directive line
			t.Fatalf("step %d inline root %+v range %+v", s.Step, root, c.Inline.IncludedRanges[0])
		}
	}
	// the directive edit moved the inline range: recomputed, not carried over
	if r.Steps[2].Composite.Inline.IncludedRanges[0].StartByte == r.Steps[1].Composite.Inline.IncludedRanges[0].StartByte {
		t.Fatal("included range not recomputed after a directive edit")
	}
	only := run("svc-directive-only", "<%@ ServiceHost Service=\"App.Svc\" CodeBehind=\"Svc.svc.cs\" %>\r\n")
	if only.Assessment != kit.AssessPass || only.Process != nil || only.Steps[0].Composite.Inline != nil || only.Steps[0].Composite.Coverage.CodeBehind != "OBSERVED" {
		t.Fatalf("directive-only %s %s", only.Assessment, only.Code)
	}
	// R1 M-3: inline code that is not parsed as C# is never PASS.
	for code, s := range map[string]string{
		"SVC_INLINE_UNRESOLVED":  "<%@ ServiceHost Service=\"S\" %>\nclass Svc { }\n",
		"SVC_INLINE_UNSUPPORTED": "<%@ ServiceHost Language=\"VB\" Service=\"S\" %>\nClass Svc\n",
		"SVC_DIRECTIVE_ABSENT":   "class Svc { }\n",
	} {
		if r := run("svc-"+code, s); r.Assessment != kit.AssessBlocked || r.Code != code || r.Steps[0].Incremental != nil {
			t.Fatalf("%s: %s %s", code, r.Assessment, r.Code)
		}
	}
	// R2 M-1 / n-2: a directive without %> (inline boundary unknown) and a damaged directive.
	if r := run("svc-no-terminator", "<%@ ServiceHost Language=\"C#\" Service=\"S\"\nusing System;\npublic class Svc { }\n"); r.Assessment != kit.AssessBlocked || r.Code != "SVC_INLINE_UNRESOLVED" ||
		r.Steps[0].Composite.Coverage.Inline != "UNRESOLVED" {
		t.Fatalf("no terminator: %s %s %+v", r.Assessment, r.Code, r.Steps[0].Composite.Coverage)
	}
	if r := run("svc-diagnostics", "<%@ ServiceHost Service=\"S\" Bogus=\"x\" %>\n"); r.Assessment != kit.AssessBlocked || r.Code != "SVC_DIRECTIVE_DIAGNOSTICS" {
		t.Fatalf("diagnostics: %s %s", r.Assessment, r.Code)
	}
	// an edit that removes the Language attribute leaves a step without parseable inline code
	lost := run("svc-language-lost", src, edit{"Language=\"C#\" ", ""})
	if lost.Assessment != kit.AssessBlocked || lost.Code != "SVC_INLINE_NOT_PARSED" || lost.Process != nil || lost.Steps[1].Composite.Language.Status != "UNRESOLVED_LANGUAGE" {
		t.Fatalf("language lost %s %s", lost.Assessment, lost.Code)
	}
}
