package native

import (
	"context"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestNativeReuseSvcBoundaries(t *testing.T) {
	b := fixtureBuild(t, "plain")
	x := testContext("native-parse-edit")
	x.Format, x.Protocol, x.ObservationRunID = kit.SvcFormat, ProtocolR3, "svc-reuse-run"
	source := "<%@ ServiceHost Language=\"C#\" Service=\"S\" %>\nz = [1, 2];\na = f(1);"
	c := kit.IncrementalCase{ID: "svc-reuse", Encoding: kit.EncodingUTF8, Edits: editsBy(source, edit{"%>", ""}, edit{"Service=\"S\" ", "Service=\"S\" %>"}), Expect: []kit.StepExpectation{{Step: 0, Syntax: "NO_ERROR"}, {Step: 1, Syntax: "ERROR"}, {Step: 2, Syntax: "NO_ERROR"}}}
	r := b.RunCase(context.Background(), x, c, []byte(source))
	if r.Assessment != kit.AssessPass || r.ReuseObservationClaim != ClaimPass || len(r.Steps) != 3 || len(r.Segments) != 2 {
		t.Fatalf("svc: %+v", r)
	}
	for _, k := range []int{0, 2} {
		o := r.Steps[k].Observations
		if len(o) != 1 || o[0].Step != 0 || o[0].HistoryStart != uint64(k) || o[0].Role != "initial" || o[0].OldTree || o[0].Status != "COMPLETE" {
			t.Fatalf("segment %d: %+v", k, o)
		}
	}
	for _, o := range r.Steps[1].Observations {
		if o.Status != "UNAVAILABLE" || o.Reason != "NO_NATIVE_PARSE" || o.Events != 0 {
			t.Fatalf("host-only: %+v", o)
		}
	}
	for _, segment := range r.Segments {
		if len(segment.Raw) == 0 {
			t.Fatal("segment raw response not retained")
		}
	}
	if r.Claims.IncrementalEquality != ClaimNotClaimed || r.Claims.IncrementalRoute != ClaimNotClaimed {
		t.Fatal("whole-history claims promoted")
	}
	noInline := b.RunCase(context.Background(), x, kit.IncrementalCase{ID: "svc-host-only", Encoding: kit.EncodingUTF8}, []byte(`<%@ ServiceHost Language="C#" Service="S" %>`))
	if noInline.ReuseObservationClaim != ClaimNotClaimed || len(noInline.Steps) != 1 || noInline.Steps[0].Observations[0].Reason != "NO_NATIVE_PARSE" || noInline.Process != nil {
		t.Fatalf("no native: %+v", noInline)
	}
}
