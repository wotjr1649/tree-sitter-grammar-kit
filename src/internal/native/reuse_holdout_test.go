package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// TestNativeReuseHoldouts runs every reuse holdout of src/testdata/native/reuse-holdouts.json
// (#217: the historical error-tree route BLOCKED originals, kept outside the registered
// corpus and its support claim) on the registered grammar. The pinned disposition is
// checked per edit step: with the old tree passed the route stays BLOCKED on exactly the
// recorded steps, every parse is observed in full, and the logger accepted reuse on each
// BLOCKED step (the public node identity shows none); with the old tree omitted the same
// history is a kit FAIL from its own first step. Any other outcome changes the disposition.
func TestNativeReuseHoldouts(t *testing.T) {
	if os.Getenv("TSGK_BENCH_PREPARED") == "" {
		// the owned suite has no registered closure; the native routes job checks this ran
		t.Skip("registered prepared closure not configured")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native", "reuse-holdouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Holdouts []struct {
			ID, Route  string
			SourceUTF8 string `json:"source_utf8"`
			Edits      []struct {
				StartByte  uint32 `json:"start_byte"`
				OldEndByte uint32 `json:"old_end_byte"`
				NewEndByte uint32 `json:"new_end_byte"`
				Old        string `json:"old_utf8"`
				New        string `json:"new_utf8"`
			}
			BlockedSteps []int `json:"blocked_steps"`
			OmitOldFirst int   `json:"omit_old_tree_first_step"`
		}
	}
	if err := json.Unmarshal(data, &f); err != nil || len(f.Holdouts) == 0 {
		t.Fatalf("reuse holdouts: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	completed := map[bool]int{}
	for _, omit := range []bool{false, true} {
		var defines []string
		if omit {
			defines = []string{"TSGK_FAULT_OMIT_OLD_TREE"}
		}
		for _, h := range f.Holdouts {
			if len(h.BlockedSteps) == 0 {
				t.Fatalf("%s: no blocked step", h.ID)
			}
			edits := []kit.Edit{}
			for _, e := range h.Edits {
				edits = append(edits, kit.Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, Old: []byte(e.Old), New: []byte(e.New)})
			}
			x := testContext("native-parse-edit")
			x.Route, x.Protocol, x.ObservationRunID = h.Route, ProtocolR3, "reuse-holdout-run"
			r := registeredBuild(t, h.Route, defines...).RunCase(ctx, x, kit.IncrementalCase{ID: h.ID, Encoding: kit.EncodingUTF8, Edits: edits}, []byte(h.SourceUTF8))
			if r.ExecutionStatus != "COMPLETED" || r.Claims.IncrementalEquality != ClaimPass || len(r.Steps) != len(edits)+1 {
				t.Fatalf("%s omit_old=%t: status=%s code=%s claims=%+v", h.ID, omit, r.ExecutionStatus, r.Code, r.Claims)
			}
			for _, o := range r.Steps[0].Observations {
				if o.Status != "COMPLETE" || o.OldTree || o.Counts.NewParse != 1 {
					t.Fatalf("%s omit_old=%t initial parse collector: %+v", h.ID, omit, o)
				}
			}
			if len(r.Steps[0].Observations) == 0 {
				t.Fatalf("%s omit_old=%t: initial parse not observed", h.ID, omit)
			}
			var blocked []int
			for k, s := range r.Steps[1:] {
				if len(s.Observations) != 2 {
					t.Fatalf("%s omit_old=%t step %d: %d observations", h.ID, omit, s.Step, len(s.Observations))
				}
				inc, fresh := s.Observations[0], s.Observations[1]
				if inc.Status != "COMPLETE" || fresh.Status != "COMPLETE" || fresh.OldTree || fresh.Counts.NewParse != 1 || fresh.Counts.Accepted != 0 {
					t.Fatalf("%s omit_old=%t step %d collector: %+v %+v", h.ID, omit, s.Step, inc, fresh)
				}
				if omit && (inc.OldTree || inc.Counts.NewParse != 1 || inc.Counts.ParseAfterEdit != 0 || inc.Counts.Accepted != 0) {
					t.Fatalf("%s step %d: omitted old tree observed as %+v", h.ID, s.Step, inc)
				}
				if !omit && (!inc.OldTree || inc.Counts.ParseAfterEdit != 1) {
					t.Fatalf("%s step %d: old tree not observed: %+v", h.ID, s.Step, inc)
				}
				if stepRoute(r.Steps[k], s) == ClaimBlocked {
					blocked = append(blocked, s.Step)
					if !omit && (inc.Counts.Accepted == 0 || s.Route.ReusedNodes != 0) {
						t.Fatalf("%s step %d: BLOCKED with accepted=%d reused=%d", h.ID, s.Step, inc.Counts.Accepted, s.Route.ReusedNodes)
					}
				}
			}
			if omit {
				// the omitted old tree is a kit FAIL, not hidden behind the error-tree BLOCKED
				if r.Assessment != kit.AssessFail || r.Claims.IncrementalRoute != ClaimFail || r.ReuseObservationClaim != ClaimFail ||
					r.Code != oldTreeUnverifiedPrefix+strconv.Itoa(h.OmitOldFirst) {
					t.Fatalf("%s old-tree omission: assessment=%s route=%s observation=%s code=%s", h.ID, r.Assessment, r.Claims.IncrementalRoute, r.ReuseObservationClaim, r.Code)
				}
			} else if r.Assessment != kit.AssessBlocked || r.Claims.IncrementalRoute != ClaimBlocked || r.ReuseObservationClaim != ClaimPass ||
				!slices.Equal(blocked, h.BlockedSteps) || r.Code != routeUnobservablePrefix+strconv.Itoa(h.BlockedSteps[0]) {
				t.Fatalf("%s disposition changed: assessment=%s route=%s observation=%s code=%s blocked_steps=%v", h.ID, r.Assessment, r.Claims.IncrementalRoute, r.ReuseObservationClaim, r.Code, blocked)
			}
			completed[omit]++
			t.Logf("%s omit_old=%t assessment=%s code=%s blocked_steps=%v", h.ID, omit, r.Assessment, r.Code, blocked)
		}
	}
	t.Logf("reuse holdouts=%d completed=%d omit_old_completed=%d", len(f.Holdouts), completed[false], completed[true])
}
