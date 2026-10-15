package native

import (
	"context"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestNativeReuseObservations(t *testing.T) {
	for _, tc := range []struct{ name, define, status, reason string }{
		{"normal", "", "COMPLETE", ""},
		{"omit-old", "TSGK_FAULT_OMIT_OLD_TREE", "COMPLETE", ""},
		{"missing", "TSGK_FAULT_REUSE_MISSING", "UNAVAILABLE", "LOGGER_MISSING"},
		{"unknown", "TSGK_FAULT_REUSE_UNKNOWN", "UNAVAILABLE", "FORMAT_UNKNOWN"},
		{"empty-symbol", "TSGK_FAULT_REUSE_EMPTY_SYMBOL", "UNAVAILABLE", "FORMAT_UNKNOWN"},
		{"after-done", "TSGK_FAULT_REUSE_AFTER_DONE", "UNAVAILABLE", "FORMAT_UNKNOWN"},
		{"overflow", "TSGK_FAULT_REUSE_OVERFLOW", "OVERFLOW", "COUNTER_OVERFLOW"},
		{"event-limit", "TSGK_FAULT_REUSE_EVENT_LIMIT", "TRUNCATED", "EVENT_LIMIT"},
		{"byte-limit", "TSGK_FAULT_REUSE_BYTE_LIMIT", "TRUNCATED", "BYTE_LIMIT"},
		{"message-limit", "TSGK_FAULT_REUSE_MESSAGE_LIMIT", "TRUNCATED", "MESSAGE_LIMIT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // The sequential parent keeps timing-sensitive tests separate.
			source := "a = 1; b = 2;"
			req := baseRequest(source, "native-parse-edit")
			req.Protocol = ProtocolR3
			req.ObservationRunID = "native-reuse-test"
			req.Edits = editsBy(source, edit{"1", "10"})
			versions, points, err := kit.ApplyEdits(req.Encoding, req.Source, req.Edits, req.Limits.InputBytes)
			if err != nil {
				t.Fatal(err)
			}
			var defines []string
			if tc.define != "" {
				defines = []string{tc.define}
			}
			b := fixtureBuild(t, "plain", defines...)
			req.ObservationProducerSHA256 = b.Identity
			run, response, err := rawExec(t, b, Frame(req.Encode()), "native-parse-edit")
			if err != nil {
				t.Fatal(err)
			}
			checked, err := Check(response, req, versions, points, run.ExitCode)
			if err != nil {
				t.Fatal(err)
			}
			if len(checked.Steps) != 2 {
				t.Fatal("missing parse steps")
			}
			for step, cs := range checked.Steps {
				for _, o := range cs.Observations {
					if o.Status != tc.status || o.Reason != tc.reason {
						t.Fatalf("step %d %s: %s/%s", step, o.Role, o.Status, o.Reason)
					}
				}
			}
			edit := checked.Steps[1]
			if edit.Fresh == nil || edit.Incremental.Form != "full" || edit.Fresh.Form != "full" {
				t.Fatal("the logger observation displaced full CST evidence")
			}
			if diff := kit.CompareTrees(edit.Incremental.Nodes, edit.Fresh.Nodes); diff != nil {
				t.Fatal(diff)
			}
			initial, inc, fresh := checked.Steps[0].Observations[0], edit.Observations[0], edit.Observations[1]
			if initial.OldTree || fresh.OldTree {
				t.Fatal("initial/fresh passed an old tree")
			}
			if tc.name == "normal" {
				if !inc.OldTree || inc.Counts.ParseAfterEdit != 1 || inc.Counts.Accepted == 0 ||
					fresh.Counts.NewParse != 1 || fresh.Counts.Accepted != 0 || initial.Counts.Accepted != 0 {
					t.Fatal("logger branch, reuse or independent payload reset")
				}
			}
			if tc.name == "omit-old" && (inc.OldTree || inc.Counts.NewParse != 1 || inc.Counts.Accepted != 0) {
				t.Fatal("omit-old became incremental evidence")
			}
			x := testContext("native-parse-edit")
			x.Protocol, x.ObservationRunID = ProtocolR3, req.ObservationRunID
			r := b.RunCase(context.Background(), x, kit.IncrementalCase{ID: req.ID, Encoding: req.Encoding, Edits: req.Edits}, req.Source)
			want := ClaimBlocked
			if tc.name == "normal" {
				want = ClaimPass
			}
			if tc.name == "omit-old" {
				want = ClaimFail
			}
			if r.ReuseObservationClaim != want || len(r.Steps) != 2 || r.Claims.IncrementalEquality != ClaimPass {
				t.Fatalf("host projection/claim: %+v", r)
			}
		})
	}
}
