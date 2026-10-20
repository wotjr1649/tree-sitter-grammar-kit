package native

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestNativeReuseJSONKeepsBlockedRoutes(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		t.Skip("registered JSON prepared closure not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, omit := range []bool{false, true} {
		var defines []string
		if omit {
			defines = []string{"TSGK_FAULT_OMIT_OLD_TREE"}
		}
		b := benchmarkJSONBuild(t, defines...)
		for _, tc := range []struct {
			id, source string
			edit       kit.Edit
		}{
			{"repair-close", "[1", kit.Edit{StartByte: 2, OldEndByte: 2, NewEndByte: 3, New: []byte("]")}},
			{"break-close", "[1]", kit.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 2, Old: []byte("]")}},
			{"repair-colon", "{\"a\" 1}", kit.Edit{StartByte: 4, OldEndByte: 5, NewEndByte: 5, Old: []byte(" "), New: []byte(":")}},
		} {
			x := testContext("native-parse-edit")
			x.Protocol, x.ObservationRunID = ProtocolR3, "json-reuse-run"
			r := b.RunCase(ctx, x, kit.IncrementalCase{ID: tc.id, Encoding: kit.EncodingUTF8, Edits: []kit.Edit{tc.edit}}, []byte(tc.source))
			if r.Claims.IncrementalEquality != ClaimPass || len(r.Steps) != 2 {
				t.Fatalf("JSON %s omit=%t: status=%s code=%s claims=%+v", tc.id, omit, r.ExecutionStatus, r.Code, r.Claims)
			}
			inc, fresh := r.Steps[1].Observations[0], r.Steps[1].Observations[1]
			if inc.Status != "COMPLETE" || fresh.Status != "COMPLETE" || fresh.Counts.Accepted != 0 {
				t.Fatalf("JSON %s collector: %+v %+v", tc.id, inc, fresh)
			}
			if omit {
				// #217: the omitted old tree is a kit FAIL, not hidden behind the error-tree BLOCKED
				if r.ReuseObservationClaim != ClaimFail || inc.OldTree || inc.Counts.NewParse != 1 || inc.Counts.Accepted != 0 ||
					r.Assessment != kit.AssessFail || r.Claims.IncrementalRoute != ClaimFail || r.Code != "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1" {
					t.Fatalf("JSON old-tree omission undetected: assessment=%s route=%s code=%s", r.Assessment, r.Claims.IncrementalRoute, r.Code)
				}
			} else if r.Assessment != kit.AssessBlocked || r.Claims.IncrementalRoute != ClaimBlocked || r.ReuseObservationClaim != ClaimPass || !inc.OldTree || inc.Counts.ParseAfterEdit != 1 || inc.Counts.Accepted == 0 || r.Steps[1].Route.ReusedNodes != 0 {
				t.Fatalf("JSON %s existing BLOCKED or separate logger evidence changed", tc.id)
			}
			t.Logf("JSON %s omit_old=%t public_route=%s logger_claim=%s accept_events=%d", tc.id, omit, r.Claims.IncrementalRoute, r.ReuseObservationClaim, inc.Counts.Accepted)
		}
	}
}
