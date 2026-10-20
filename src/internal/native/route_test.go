package native

import (
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// routeCase is a completed case with two edit steps whose incremental trees carry the given
// has_error values (step 0, 1, 2) and whose edit steps carry the given route observations;
// both comparisons are equal.
func routeCase(hasError [3]bool, routes [2]*Route) *CaseResult {
	out := &CaseResult{ExecutionStatus: kit.StatusCompleted}
	for k := range 3 {
		s := StepResult{Step: k, Incremental: &TreeOut{Status: kit.StatusCompleted, Form: "full", HasError: hasError[k]}}
		if k > 0 {
			s.Comparison = &Comparison{Equal: true}
			if r := routes[k-1]; r != nil {
				ro := RouteOut{Route: *r}
				ro.Proven = ro.EditHasChanges && ro.ReusedNodes > 0 && ro.FreshReusedNodes == 0
				s.Route = &ro
			}
		}
		out.Steps = append(out.Steps, s)
	}
	return out
}

var (
	reused    = &Route{EditHasChanges: true, ReusedNodes: 3}
	notReused = &Route{EditHasChanges: true}
)

// An edit step that reuses no node is not a route proof. When its old or new tree has an
// error the route cannot be observed (tree-sitter reuses nothing around an ERROR), so the
// claim is BLOCKED with its own code; on clean trees the same observation is a FAIL. Every
// other unproven route (no instrumentation, no change, a fresh parse that reused nodes)
// stays a FAIL whatever the trees hold.
func TestIncrementalRouteOnErrorTree(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hasError [3]bool
		routes   [2]*Route
		claim    string
		code     string // case code (judgeIncremental's or the blocked one)
		blocked  string
	}{
		{"proven", [3]bool{}, [2]*Route{reused, reused}, ClaimPass, "", ""},
		{"clean-no-reuse", [3]bool{}, [2]*Route{reused, notReused}, ClaimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_2", ""},
		{"old-tree-error", [3]bool{true, false, false}, [2]*Route{notReused, reused}, ClaimBlocked, "", "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"},
		{"new-tree-error", [3]bool{false, false, true}, [2]*Route{reused, notReused}, ClaimBlocked, "", "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_2"},
		{"every-step-error", [3]bool{true, true, true}, [2]*Route{notReused, notReused}, ClaimBlocked, "", "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"},
		{"blocked-then-fail", [3]bool{true, false, false}, [2]*Route{notReused, notReused}, ClaimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_2", "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"},
		{"error-tree-no-route", [3]bool{true, true, true}, [2]*Route{reused, nil}, ClaimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_2", ""},
		{"error-tree-no-change", [3]bool{true, true, true}, [2]*Route{reused, {}}, ClaimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_2", ""},
		{"error-tree-fresh-reused", [3]bool{true, true, true}, [2]*Route{reused, {EditHasChanges: true, FreshReusedNodes: 1}}, ClaimFail, "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_2", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := routeCase(tc.hasError, tc.routes)
			blocked := judgeIncremental(out, false)
			if out.Claims.IncrementalRoute != tc.claim || out.Code != tc.code || blocked != tc.blocked || out.Claims.IncrementalEquality != ClaimPass {
				t.Fatalf("claims %+v code %q blocked %q, want route %s code %q blocked %q", out.Claims, out.Code, blocked, tc.claim, tc.code, tc.blocked)
			}
		})
	}
}

// The error-tree exception reads has_error only from full trees, the form whose has_error
// replay recomputes from the nodes: when the old or the new incremental tree of the step
// is a summary or record form, an unproven route stays FAIL.
func TestIncrementalRouteNeedsFullTrees(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hasError [3]bool
		summary  int // the step whose incremental tree is not full
	}{
		{"old-error-tree-summary", [3]bool{true, false, false}, 0},
		{"old-error-new-summary", [3]bool{true, false, false}, 1},
		{"new-error-tree-summary", [3]bool{false, true, false}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := routeCase(tc.hasError, [2]*Route{notReused, reused})
			out.Steps[tc.summary].Incremental.Form = "summary"
			blocked := judgeIncremental(out, false)
			if out.Claims.IncrementalRoute != ClaimFail || out.Code != "INCREMENTAL_ROUTE_NOT_OBSERVED_STEP_1" || blocked != "" {
				t.Fatalf("claims %+v code %q blocked %q, want route FAIL", out.Claims, out.Code, blocked)
			}
		})
	}
}

// A BLOCKED route makes the case BLOCKED, never FAIL, and its code is the case code only
// when no claim fails: an expectation FAIL keeps EXPECTATION_FAILED, an oracle claim FAIL
// keeps ORACLE_CLAIM_FAILED.
func TestBlockedRouteCode(t *testing.T) {
	const code = "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"
	out := routeCase([3]bool{true, true, true}, [2]*Route{notReused, notReused})
	out.Claims.Expectations = ClaimNotClaimed
	finishCase(out, judgeIncremental(out, false))
	if out.Assessment != kit.AssessBlocked || out.Code != code {
		t.Fatalf("assessment %s code %q", out.Assessment, out.Code)
	}
	out = routeCase([3]bool{true, true, true}, [2]*Route{notReused, notReused})
	out.Claims.Expectations = ClaimFail
	blocked := judgeIncremental(out, false)
	out.Code = "EXPECTATION_FAILED"
	finishCase(out, blocked)
	if out.Assessment != kit.AssessFail || out.Code != "EXPECTATION_FAILED" {
		t.Fatalf("assessment %s code %q", out.Assessment, out.Code)
	}
	// an r2 case whose route is blocked and whose query expectation fails
	out = routeCase([3]bool{true, true, true}, [2]*Route{notReused, notReused})
	out.Oracle = &OracleClaims{ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed, ClaimNotClaimed}
	out.Claims.Expectations = ClaimNotClaimed
	out.Steps[0].Incremental.Queries = []QueryOut{{ID: "q", Status: kit.StatusCompleted}}
	finishCase(out, judgeIncremental(out, false))
	if out.Code != code {
		t.Fatalf("code %q before the oracle judgement", out.Code)
	}
	judgeOracle(out, kit.OracleCase{QueryExpect: []kit.QueryExpectation{{Query: "q", Step: 0, Status: kit.StatusFailed}}}, kit.OracleProfile{}, nil, Context{}, nil)
	if out.Oracle.QueryExpectations != ClaimFail || out.Assessment != kit.AssessFail || out.Code != "ORACLE_CLAIM_FAILED" {
		t.Fatalf("oracle %+v assessment %s code %q", *out.Oracle, out.Assessment, out.Code)
	}
}

// The incremental_equality claim folds the worst step (FAIL > BLOCKED > PASS), as replay
// recomputes it: a later step without a comparison never masks an earlier FAIL, and a
// FAIL after a BLOCKED step is a FAIL.
func TestIncrementalEqualityFold(t *testing.T) {
	for _, tc := range []struct {
		name  string
		comps [2]*Comparison
		claim string
		code  string
	}{
		{"fail-then-blocked", [2]*Comparison{{Equal: false}, nil}, ClaimFail, "INCREMENTAL_FRESH_MISMATCH_STEP_1"},
		{"blocked-then-fail", [2]*Comparison{nil, {Equal: false}}, ClaimFail, "INCREMENTAL_FRESH_MISMATCH_STEP_2"},
		{"pass-then-blocked", [2]*Comparison{{Equal: true}, nil}, ClaimBlocked, ""},
		{"pass", [2]*Comparison{{Equal: true}, {Equal: true}}, ClaimPass, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := routeCase([3]bool{}, [2]*Route{reused, reused})
			out.Steps[1].Comparison, out.Steps[2].Comparison = tc.comps[0], tc.comps[1]
			judgeIncremental(out, false)
			if out.Claims.IncrementalEquality != tc.claim || out.Code != tc.code || out.Claims.IncrementalRoute != ClaimPass {
				t.Fatalf("claims %+v code %q, want equality %s code %q", out.Claims, out.Code, tc.claim, tc.code)
			}
		})
	}
}

// observedPair is the incremental and fresh observation of an edit step; oldTree false is
// the omitted old-tree fault.
func observedPair(step int, oldTree bool, status string) []reuseobs.Observation {
	base := reuseobs.Observation{Schema: reuseobs.Schema, Policy: reuseobs.Policy, RuntimeManifestSHA256: reuseobs.RuntimeManifestSHA256,
		RequestID: "case", RunID: "run", ProducerSHA256: sha([]byte("producer")), Step: uint64(step), SourceSHA256: sha([]byte("new")), SourceBytes: 3,
		Status: "COMPLETE", Events: 2, Bytes: 20, Counts: &reuseobs.Counts{NewParse: 1, Done: 1}}
	inc, fresh := base, base
	inc.Role, fresh.Role = "incremental", "fresh"
	if oldTree {
		old := sha([]byte("old"))
		inc.OldTree, inc.OldSourceSHA256, inc.Counts = true, &old, &reuseobs.Counts{ParseAfterEdit: 1, Done: 1}
	}
	if status == "TRUNCATED" { // a capped prefix without the start event
		inc.Status, inc.Reason, inc.Counts = "TRUNCATED", "MESSAGE_LIMIT", &reuseobs.Counts{Done: 1}
	}
	return []reuseobs.Observation{inc, fresh}
}

// With mandatory observation an error-tree step stays BLOCKED only when the old tree was
// observed passed; an omitted old tree or a missing observation fails the route with its own
// code, a collection cap keeps it BLOCKED, and a proven route is judged by the claim fold.
func TestErrorTreeRouteNeedsObservedOldTree(t *testing.T) {
	const blockedCode = "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1"
	for _, tc := range []struct {
		name         string
		routes       [2]*Route
		observations []reuseobs.Observation
		claim, code  string
		blocked      string
	}{
		{"observed", [2]*Route{notReused, reused}, observedPair(1, true, "COMPLETE"), ClaimBlocked, "", blockedCode},
		{"omitted-old-tree", [2]*Route{notReused, reused}, observedPair(1, false, "COMPLETE"), ClaimFail, "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1", ""},
		{"missing", [2]*Route{notReused, reused}, nil, ClaimFail, "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1", ""},
		{"truncated", [2]*Route{notReused, reused}, observedPair(1, true, "TRUNCATED"), ClaimBlocked, "", blockedCode},
		{"proven", [2]*Route{reused, reused}, observedPair(1, false, "COMPLETE"), ClaimPass, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := routeCase([3]bool{true, true, false}, tc.routes)
			out.Steps[1].Observations = tc.observations
			out.Steps[2].Observations = observedPair(2, true, "COMPLETE")
			blocked := judgeIncremental(out, true)
			if out.Claims.IncrementalRoute != tc.claim || out.Code != tc.code || blocked != tc.blocked {
				t.Fatalf("claims %+v code %q blocked %q, want route %s code %q blocked %q", out.Claims, out.Code, blocked, tc.claim, tc.code, tc.blocked)
			}
		})
	}
	if got := svcSegmentCode("INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_1", 2); got != "INCREMENTAL_ROUTE_OLD_TREE_UNVERIFIED_STEP_3" {
		t.Fatalf("segment code %s", got)
	}
}

// The mandatory observation claim folds into a completed case last: a FAIL turns a BLOCKED
// case FAIL with the observation code, never weakens an earlier FAIL or its code, and a
// case that ended early keeps its terminal verdict.
func TestObservationClaimFold(t *testing.T) {
	x := Context{Protocol: ProtocolR3}
	for _, tc := range []struct {
		name, status, assessment, code string
		oldTree                        bool
		wantAssessment, wantCode       string
	}{
		{"blocked-to-fail", kit.StatusCompleted, kit.AssessBlocked, "INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1", false, kit.AssessFail, "REUSE_OBSERVATION_FAILED"},
		{"pass-to-fail", kit.StatusCompleted, kit.AssessPass, "", false, kit.AssessFail, "REUSE_OBSERVATION_FAILED"},
		{"fail-kept", kit.StatusCompleted, kit.AssessFail, "EXPECTATION_FAILED", false, kit.AssessFail, "EXPECTATION_FAILED"},
		{"observed", kit.StatusCompleted, kit.AssessPass, "", true, kit.AssessPass, ""},
		{"ended-early", kit.StatusResourceLimit, kit.AssessBlocked, "PARSE_TIME_LIMIT", false, kit.AssessBlocked, "PARSE_TIME_LIMIT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial := observedPair(0, false, "COMPLETE")[1]
			initial.Role = "initial"
			c := CaseResult{ID: "case", ExecutionStatus: tc.status, Assessment: tc.assessment, Code: tc.code, Steps: []StepResult{
				{Incremental: &TreeOut{}, Observations: []reuseobs.Observation{initial}},
				{Step: 1, Incremental: &TreeOut{}, Observations: observedPair(1, tc.oldTree, "COMPLETE")}}}
			x.observeCase(&c)
			x.observeCase(&c) // every return path may observe again
			if c.Assessment != tc.wantAssessment || c.Code != tc.wantCode || c.ReuseObservationClaim != map[bool]string{true: ClaimPass, false: ClaimFail}[tc.oldTree] {
				t.Fatalf("assessment %s code %q claim %s", c.Assessment, c.Code, c.ReuseObservationClaim)
			}
		})
	}
}
