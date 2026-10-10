package native

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
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
	if lost.Assessment != kit.AssessBlocked || lost.Code != "SVC_PARTIAL_HISTORY" || lost.Process != nil || lost.Steps[1].Composite.Language.Status != "UNRESOLVED_LANGUAGE" || lost.Steps[0].Incremental == nil || lost.Steps[1].Incremental != nil || len(lost.Segments) != 1 {
		t.Fatalf("language lost %s %s", lost.Assessment, lost.Code)
	}
}

func TestSvcExplicitReferenceAndSelfRejection(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	x.SvcContext = &kit.SvcContext{DefaultLanguage: "C#", LanguageSource: "owned", CodeBehind: []kit.SvcReference{{Name: "ref.cs", Case: "ref"}}}
	for variant := 0; variant < 5; variant++ {
		self := variant == 1 || variant == 2 || variant == 4
		enc := kit.EncodingUTF8
		root := t.TempDir()
		source := []byte("z = [1, 2];")
		if self {
			source = []byte(`<%@ ServiceHost Service="S" CodeBehind="ref.cs" %>`)
		}
		if variant == 2 {
			enc = kit.EncodingCP949
			source = append([]byte{0xA1, 0xA1}, source...)
		}
		if variant == 3 {
			source = []byte("# <%@ ServiceHost Service='S' %>\nz = [1, 2];")
		}
		if variant == 4 {
			source = append([]byte("// prefix\n"), source...)
		}
		if e := os.WriteFile(filepath.Join(root, "ref.txt"), source, 0600); e != nil {
			t.Fatal(e)
		}
		c := kit.IncrementalCase{ID: "ref", Encoding: enc, Input: kit.NativeInput{Path: "ref.txt", Role: "case", SHA256: fmt.Sprintf("%x", sha256.Sum256(source)), Bytes: uint64(len(source))}, Expect: []kit.StepExpectation{{Syntax: "NO_ERROR"}}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		refs := b.runSvcReferences(ctx, x, []kit.IncrementalCase{c}, root)
		cancel()
		r := refs[c.ID]
		if self {
			if r.ExecutionStatus != kit.StatusNotRun || r.Code != "SVC_REFERENCE_INVALID" {
				t.Fatalf("self %+v", r)
			}
			continue
		}
		if r.Assessment != kit.AssessPass || r.Steps[0].Composite != nil {
			t.Fatalf("reference %s %s", r.Assessment, r.Code)
		}
		svc := []byte(`<%@ ServiceHost Service="S" CodeBehind="ref.cs" %>`)
		ctx, cancel = context.WithTimeout(context.Background(), time.Minute)
		owner := b.RunCase(ctx, x, kit.IncrementalCase{ID: "owner", Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Syntax: "NO_ERROR"}}}, svc)
		cancel()
		linkSvcReferences(&owner, refs)
		if owner.Assessment != kit.AssessPass || len(owner.References) != 1 || owner.References[0].Input != c.Input || owner.Steps[0].Composite.CodeBehind.Resolution != "PARSED" {
			t.Fatalf("owner %s %s", owner.Assessment, owner.Code)
		}
	}
}

func TestSvcSegmentsPreserveEveryExpectation(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	source := "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\nz = [1, 2];\na = f(1);"
	for _, step := range []int{0, 1} {
		c := kit.IncrementalCase{ID: "svc-duplicate-expect", Encoding: kit.EncodingUTF8, Edits: editsBy(source, edit{"%>", ""}), Expect: []kit.StepExpectation{{Step: step, Syntax: "ERROR"}, {Step: step, Syntax: "NO_ERROR"}}}
		if step == 1 {
			c.Expect[0].Syntax, c.Expect[1].Syntax = "NO_ERROR", "ERROR"
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		r := b.RunCase(ctx, x, c, []byte(source))
		cancel()
		if r.Assessment != kit.AssessFail || len(r.Expectations) != 2 || r.Expectations[0].Result != ClaimFail || r.Expectations[1].Result != ClaimPass {
			t.Fatalf("step %d: %s %+v", step, r.Assessment, r.Expectations)
		}
	}
}

func TestSvcSegmentsRetainNativeFailures(t *testing.T) {
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	source := "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\nz = [1, 2];\na = f(1);"
	for _, fault := range []string{"TSGK_FAULT_STEP1_FLAG", "TSGK_FAULT_OMIT_OLD_TREE"} {
		b := fixtureBuild(t, "plain", fault)
		c := kit.IncrementalCase{ID: "svc-segment-fault", Encoding: kit.EncodingUTF8, Edits: editsBy(source, edit{"f(1)", "f(2, 3)"}, edit{"%>", ""}), Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 2, Syntax: "ERROR"}}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		r := b.RunCase(ctx, x, c, []byte(source))
		cancel()
		if len(r.Segments) != 1 || r.Assessment != kit.AssessFail {
			t.Fatalf("%s: %s %+v", fault, r.Assessment, r.Segments)
		}
	}
}

func TestSvcReferenceKeepsParentVerdictAndClaims(t *testing.T) {
	for _, assessment := range []string{kit.AssessFail, kit.AssessBlocked} {
		out := CaseResult{ExecutionStatus: kit.StatusCompleted, Assessment: assessment, Code: "SVC_PARTIAL_HISTORY", Claims: Claims{ClaimNotClaimed, ClaimNotClaimed, ClaimPass}, Steps: []StepResult{{Composite: &SvcComposite{CodeBehind: &kit.SvcCodeBehind{Case: "ref"}}}}}
		linkSvcReferences(&out, map[string]CaseResult{"ref": {ID: "ref", ExecutionStatus: kit.StatusCompleted, Assessment: kit.AssessPass}})
		if out.Assessment != assessment || out.Claims.Expectations != ClaimPass || len(out.References) != 1 {
			t.Fatalf("%s: %s %+v", assessment, out.Assessment, out.Claims)
		}
	}
}

func TestSvcBoundaryRecovery(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	source := "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\nz = [1, 2];\na = f(1);"
	c := kit.IncrementalCase{ID: "svc-recovery", Encoding: kit.EncodingUTF8, Edits: editsBy(source, edit{"%>", ""}, edit{"Service=\"S\" ", "Service=\"S\" %>"}), Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 1, Syntax: "ERROR"}, {Step: 2, Syntax: "NO_ERROR"}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r := b.RunCase(ctx, x, c, []byte(source))
	if r.Assessment != kit.AssessPass || len(r.Segments) != 2 || len(r.Steps) != 3 {
		t.Fatalf("%s %s %+v", r.Assessment, r.Code, r)
	}
	if r.Steps[0].Incremental == nil || r.Steps[1].Incremental != nil || r.Steps[2].Incremental == nil || !r.Steps[2].Restarted || r.Steps[2].Route != nil || r.Steps[2].Comparison != nil {
		t.Fatalf("steps %+v", r.Steps)
	}
	if r.Steps[0].Incremental.Digest != r.Steps[2].Incremental.Digest || r.Claims.IncrementalEquality != ClaimNotClaimed || r.Claims.IncrementalRoute != ClaimNotClaimed {
		t.Fatal("restoration or reuse claim")
	}
	for _, s := range r.Segments {
		if s.Process == nil || !s.Process.Cleanup.Verified {
			t.Fatal("segment cleanup")
		}
	}
}

func TestSvcDirectiveSyntaxExpectations(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	for _, s := range []string{`<%@ Page Service="S" %>`, `<%@ ServiceHost Service="S" ; %>`, `<%@ ServiceHost Service="S" Debug="banana" %>`, `<%@ ServiceHost Language="C#" Service="S" Bogus="x" %>` + "\nz = [1, 2];"} {
		for _, syntax := range []string{"ERROR", "NO_ERROR"} {
			c := kit.IncrementalCase{ID: "svc-syntax", Encoding: kit.EncodingUTF8, Expect: []kit.StepExpectation{{Step: 0, Syntax: syntax, Contains: []string{}}}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			r := b.RunCase(ctx, x, c, []byte(s))
			cancel()
			want := kit.AssessPass
			if syntax == "NO_ERROR" {
				want = kit.AssessFail
			}
			if r.Assessment != want || r.Claims.Expectations != want {
				t.Fatalf("%s %s: %s/%s %+v", s, syntax, r.Assessment, r.Code, r.Expectations)
			}
		}
	}
}

func TestSvcNativeEncodingCoordinates(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format = kit.SvcFormat
	source := "<% @ServiceHost Service=\"한😀\" Language=\" C# \" %>\r\nz = [1, 2];\na = f(1);\n"
	for _, enc := range []string{kit.EncodingUTF8, kit.EncodingUTF16LE, kit.EncodingUTF16BE, kit.EncodingCP949} {
		src := []byte(source)
		if enc == kit.EncodingUTF16LE || enc == kit.EncodingUTF16BE {
			src = utf16(enc, source)
		}
		if enc == kit.EncodingCP949 {
			src = append([]byte(`<% @ServiceHost Service="`), 0xB0, 0xA1)
			src = append(src, []byte("\" Language=\" C# \" %>\r\nz = [1, 2];\na = f(1);\n")...)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		r := b.RunCase(ctx, x, kit.IncrementalCase{ID: "svc-encoding", Encoding: enc, Expect: []kit.StepExpectation{{Syntax: "NO_ERROR", Contains: []string{"source_file"}}}}, src)
		cancel()
		if r.Assessment != kit.AssessPass || r.Steps[0].Incremental == nil {
			t.Fatalf("%s: %s %s", enc, r.Assessment, r.Code)
		}
		in := r.Steps[0].Composite.Inline
		if in.Tree.Nodes[0].StartPoint.Row != 1 || in.Tree.Input.Bytes != uint64(len(src)) || in.IncludedRanges[0].StartPoint != kit.PointAt(enc, src, int(in.IncludedRanges[0].StartByte)) {
			t.Fatalf("%s coordinates", enc)
		}
	}
}
