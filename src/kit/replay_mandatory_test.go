package kit

import (
	"encoding/json"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// mandatoryFixture is the observation fixture read by native-result-r4: c2's edit step can
// carry an error-tree route (old tree has_error, nothing reused) and an omitted old tree in
// both the recorded result and its raw response, with the recorded claims, verdicts and
// summary set as given.
func mandatoryFixture(t *testing.T, errorTree, omitOld bool, route, assessment string) *fxNative {
	t.Helper()
	f := reuseFixture(t)
	f.reducer = "native-result-r4"
	c := f.cases[1]
	steps := c["steps"].([]any)
	if errorTree {
		steps[0].(map[string]any)["incremental"] = fxTreeOut("beta", true)
		steps[1].(map[string]any)["route"] = map[string]any{"edit_has_changes": true, "edited_root_end_byte": 4, "reused_nodes": 0, "fresh_reused_nodes": 0, "proven": false}
		f.top["summary"].(map[string]any)["has_error"] = 1
	}
	if omitOld {
		observations := steps[1].(map[string]any)["observations"].([]reuseobs.Observation)
		observations[0].OldTree, observations[0].OldSourceSHA256, observations[0].Counts = false, nil, &reuseobs.Counts{NewParse: 1, Done: 1}
		var wire map[string]any
		if err := json.Unmarshal(f.extra["responses/00001-c2.json"], &wire); err != nil {
			t.Fatal(err)
		}
		wire["steps"].([]any)[1].(map[string]any)["observations"] = observations
		f.extra["responses/00001-c2.json"], _ = json.Marshal(wire)
	}
	var all []reuseobs.Step
	for _, value := range steps {
		s := value.(map[string]any)
		all = append(all, reuseobs.Step{Native: true, Edit: s["step"].(int) > 0, Observations: s["observations"].([]reuseobs.Observation)})
	}
	c["reuse_observation_claim"] = reuseobs.CaseClaim(all, true)
	c["claims"].(map[string]string)["incremental_route"] = route
	c["assessment"] = assessment
	f.top["assessment"] = assessment
	f.top["summary"].(map[string]any)["assessments"] = map[string]int{AssessPass: 1}
	if assessment == AssessPass {
		f.top["summary"].(map[string]any)["assessments"] = map[string]int{AssessPass: 2}
	} else {
		f.top["summary"].(map[string]any)["assessments"].(map[string]int)[assessment] = 1
	}
	return f
}

func writeMandatoryFixture(t *testing.T, f *fxNative, schema string) (string, []byte) {
	t.Helper()
	return writeReuseFixture(t, f, func(top map[string]any) { top["result_schema"] = schema })
}

// native-result-r4 recomputes the #217 rule: the observation claim folds into the case
// verdict, and an error-tree route is BLOCKED only with the old tree observed passed. A
// record that keeps the separate-axis reading of r3 for the same observations is refused.
func TestReplayMandatoryObservation(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		errorTree, omitOld        bool
		route, assessment, recomp string
		valid                     bool
	}{
		{"observed", false, false, claimPass, AssessPass, AssessPass, true},
		{"observed-error-tree", true, false, claimBlocked, AssessBlocked, AssessBlocked, true},
		{"omitted-old-tree", false, true, claimPass, AssessFail, AssessFail, true},
		{"omitted-old-tree-error-tree", true, true, claimFail, AssessFail, AssessFail, true},
		{"separate-axis-verdict", false, true, claimPass, AssessPass, "", false},
		{"blocked-masks-omitted-old-tree", true, true, claimBlocked, AssessFail, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, reg := writeMandatoryFixture(t, mandatoryFixture(t, tc.errorTree, tc.omitOld, tc.route, tc.assessment), "tsgk-incremental-result/r3")
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err != nil {
				t.Fatal(err)
			}
			if r.EvidenceValid != tc.valid || tc.valid && (r.Assessment != tc.recomp || gateOf(r, "reuse-observation").Failed != 0) {
				t.Fatalf("valid=%v assessment=%s findings=%v gates=%+v", r.EvidenceValid, r.Assessment, codes(r.Findings), r.Gates)
			}
		})
	}
}

// r4 reads only the r3 result and r3 only the r2 result: a revision cannot be replayed
// under the other's verdict rule.
func TestReplayMandatoryObservationRevisions(t *testing.T) {
	for _, tc := range []struct{ reducer, schema string }{
		{"native-result-r4", "tsgk-incremental-result/r2"},
		{"native-result-r3", "tsgk-incremental-result/r3"},
	} {
		t.Run(tc.reducer, func(t *testing.T) {
			f := mandatoryFixture(t, false, false, claimPass, AssessPass)
			f.reducer = tc.reducer
			root, reg := writeMandatoryFixture(t, f, tc.schema)
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err == nil && r.EvidenceValid {
				t.Fatalf("%s accepted %s", tc.reducer, tc.schema)
			}
		})
	}
}

// A case that ended early keeps its unfinished last step in the raw response, which
// steps_completed omits; r4 accepts the short count only when that step is unfinished in
// both the record and the raw response (Codex A217B/A217D P2), never for a completed case or
// an early case whose trees all completed.
func TestReplayMandatoryObservationEarlyEnd(t *testing.T) {
	for _, tc := range []struct {
		name              string
		early, unfinished bool
		forge             string // r3 carries no queries: a query limit forged on the "record", "raw" or "both" trees
		valid             bool
	}{{"resource-limit", true, true, "", true}, {"completed-short-count", false, false, "", false}, {"early-trees-complete", true, false, "", false},
		{"r3-query-limit", true, false, "both", false}, {"r3-raw-query-limit", true, true, "raw", false}, {"r3-record-query-limit", true, true, "record", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := mandatoryFixture(t, false, false, claimPass, AssessPass)
			c := f.cases[1]
			step := c["steps"].([]any)[1].(map[string]any)
			observations := step["observations"].([]reuseobs.Observation)
			var wire map[string]any
			if err := json.Unmarshal(f.extra["responses/00001-c2.json"], &wire); err != nil {
				t.Fatal(err)
			}
			wire["steps_completed"], wire["status"] = 1, StatusResourceLimit
			if tc.unfinished {
				observations[0].Status, observations[0].Reason, observations[0].Counts.Done = "UNAVAILABLE", "PARSE_INCOMPLETE", 0
				observations[1].Status, observations[1].Reason, observations[1].Events, observations[1].Bytes, observations[1].Counts = "UNAVAILABLE", "PARSE_NOT_RUN", 0, 0, &reuseobs.Counts{}
				raw := wire["steps"].([]any)[1].(map[string]any)
				raw["observations"], raw["incremental"] = observations, map[string]any{"status": StatusResourceLimit}
				step["incremental"].(map[string]any)["status"], step["fresh"] = StatusResourceLimit, nil
				step["route"], step["comparison"] = nil, nil
			}
			limited := []any{map[string]any{"id": "q", "status": StatusResourceLimit}}
			if tc.forge == "record" || tc.forge == "both" { // a completed recorded tree that only a query opens
				step["incremental"].(map[string]any)["status"], step["incremental"].(map[string]any)["queries"] = StatusCompleted, limited
			}
			if tc.forge == "raw" || tc.forge == "both" {
				wire["steps"].([]any)[1].(map[string]any)["incremental"] = map[string]any{"status": StatusCompleted, "queries": limited}
			}
			if tc.early {
				c["execution_status"], c["assessment"], c["code"] = StatusResourceLimit, AssessBlocked, "PARSE_TIME_LIMIT"
				c["response_status"], c["response_code"], wire["code"] = StatusResourceLimit, "PARSE_TIME_LIMIT", "PARSE_TIME_LIMIT"
				c["claims"] = map[string]string{"incremental_equality": claimNotClaimed, "incremental_route": claimNotClaimed, "expectations": claimNotClaimed}
				all := []reuseobs.Step{{Native: true, Observations: c["steps"].([]any)[0].(map[string]any)["observations"].([]reuseobs.Observation)}, {Native: true, Edit: true, Observations: observations}}
				c["reuse_observation_claim"] = reuseobs.CaseClaim(all, false)
				f.top["execution_status"], f.top["assessment"] = StatusResourceLimit, AssessBlocked
				f.top["summary"] = map[string]any{"cases": 2, "execution_statuses": map[string]int{StatusCompleted: 1, StatusResourceLimit: 1}, "assessments": map[string]int{AssessPass: 1, AssessBlocked: 1},
					"codes": map[string]int{"PARSE_TIME_LIMIT": 1}, "has_error": 0, "batches": 0, "requeued": 0}
			}
			f.extra["responses/00001-c2.json"], _ = json.Marshal(wire)
			root, reg := writeMandatoryFixture(t, f, "tsgk-incremental-result/r3")
			r, err := Replay(ctxT(t), ReplayRequest{Root: root, Profile: reg})
			if err != nil {
				t.Fatal(err)
			}
			if r.EvidenceValid != tc.valid || tc.valid && gateOf(r, "reuse-observation").Failed != 0 {
				t.Fatalf("valid=%v assessment=%s findings=%v gates=%+v", r.EvidenceValid, r.Assessment, codes(r.Findings), r.Gates)
			}
		})
	}
}

// The short count needs the response's noncomplete status in both the recorded and the raw
// last step: open is "" for a completed case, an earlier segment or a recorded step whose
// trees completed (Codex A217C/A217D P2).
func TestStepsCounted(t *testing.T) {
	const rl = StatusResourceLimit
	for _, tc := range []struct {
		name                  string
		completed             uint64
		count                 int
		status, open, rawOpen string
		want                  bool
	}{
		{"all-counted", 2, 2, StatusCompleted, "", "", true},
		{"last-unfinished", 1, 2, rl, rl, rl, true},
		{"not-allowed", 1, 2, rl, "", rl, false},
		{"raw-trees-complete", 1, 2, rl, rl, "", false},
		{"other-status", 1, 2, rl, StatusFailed, rl, false},
		{"completed-status", 1, 2, StatusCompleted, StatusCompleted, StatusCompleted, false},
		{"missing-status", 1, 2, "", "", "", false},
		{"two-short", 0, 2, rl, rl, rl, false},
		{"empty", 0, 0, rl, "", "", true},
	} {
		if got := stepsCounted(tc.completed, tc.count, tc.status, tc.open, tc.rawOpen); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
