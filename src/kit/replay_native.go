package kit

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
)

// The S05/S06 case gates, recomputed from the recorded trees with the shared comparators
// (CompareTrees, TreeDigest, ValidateTree, CompareCaptures); no new normalizer.
var nativeGates = []string{"case-binding", "case-status", "tree", "incremental-equality", "incremental-route", "expectations", "verdict", "summary"}

const (
	claimPass       = "PASS"
	claimFail       = "FAIL"
	claimBlocked    = "BLOCKED"
	claimNotClaimed = "NOT_CLAIMED"
)

type rcInput struct {
	Encoding string `json:"encoding"`
	Bytes    uint64 `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type rcDecl struct {
	Status string `json:"status"`
}

type rcTree struct {
	Status          string `json:"status"`
	Code            string `json:"code"`
	Form            string `json:"form"`
	DescendantCount uint64 `json:"descendant_count"`
	HasError        bool   `json:"has_error"`
	Digest          string `json:"digest"`
	Tree            *struct {
		Input      rcInput       `json:"input"`
		Nodes      []TreeNode    `json:"nodes"`
		Identities []IdentityRef `json:"identities"`
	} `json:"tree"`
	Summary *struct {
		Identities      []IdentityRef `json:"identities"`
		DescendantCount uint64        `json:"descendant_count"`
		Digest          struct {
			SHA256 string `json:"sha256"`
		} `json:"digest"`
		Declarations struct {
			Assessment string   `json:"assessment"`
			Items      []rcDecl `json:"items"`
		} `json:"declarations"`
	} `json:"summary"`
	Declarations *[]rcDecl `json:"declarations"`
	Queries      []struct {
		ID         string    `json:"id"`
		Status     string    `json:"status"`
		Code       string    `json:"code"`
		Evaluation string    `json:"evaluation"`
		Captures   []Capture `json:"captures"`
	} `json:"queries"`
}

type rcStep struct {
	Restarted    bool   `json:"restarted"`
	Step         int    `json:"step"`
	SourceBytes  uint64 `json:"source_bytes"`
	SourceSHA256 string `json:"source_sha256"`
	Route        *struct {
		EditHasChanges   bool   `json:"edit_has_changes"`
		ReusedNodes      uint64 `json:"reused_nodes"`
		FreshReusedNodes uint64 `json:"fresh_reused_nodes"`
		Proven           bool   `json:"proven"`
	} `json:"route"`
	Comparison *struct {
		Equal bool            `json:"equal"`
		First *TreeDifference `json:"first_difference"`
	} `json:"comparison"`
	Incremental  *rcTree `json:"incremental"`
	Fresh        *rcTree `json:"fresh"`
	QueryCompare *struct {
		Equal bool `json:"equal"`
	} `json:"query_comparison"`
	Composite *rcComposite `json:"composite"`
}

type rcComposite struct {
	Schema               string          `json:"schema"`
	Format               string          `json:"format"`
	Input                rcInput         `json:"input"`
	Language             SvcLanguage     `json:"language"`
	Directive            *SvcDirective   `json:"directive"`
	AdditionalDirectives []*SvcDirective `json:"additional_directives"`
	CodeBehind           *SvcCodeBehind  `json:"code_behind"`
	Coverage             SvcCoverage     `json:"coverage"`
	Identities           []IdentityRef   `json:"identities"`
	Inline               *struct {
		IncludedRanges []Span `json:"included_ranges"`
		Tree           *struct {
			Identities []IdentityRef `json:"identities"`
		} `json:"tree"`
	} `json:"inline"`
}

type rcExpect struct {
	Step         int            `json:"step"`
	Syntax       string         `json:"syntax"`
	Contains     []string       `json:"contains"`
	Anchors      []ExpectAnchor `json:"anchors"`
	Declarations string         `json:"declarations"`
	Result       string         `json:"result"`
}

// rcCase is the part of a recorded S05 case or S06 record the reducer reads; the rest
// (process, producer, host observations) is retained in the raw and not judged.
type rcCase struct {
	Segments        []rcSvcSegment   `json:"segments"`
	References      []rcSvcReference `json:"references"`
	Schema          string           `json:"schema"`
	Case            string           `json:"case"`
	ID              string           `json:"id"`
	Input           NativeInput      `json:"input"`
	ExecutionStatus string           `json:"execution_status"`
	Assessment      string           `json:"assessment"`
	Code            string           `json:"code"`
	Claims          struct {
		IncrementalEquality string `json:"incremental_equality"`
		IncrementalRoute    string `json:"incremental_route"`
		Expectations        string `json:"expectations"`
	} `json:"claims"`
	Oracle *struct {
		QueryEquality     string `json:"query_equality"`
		QueryExpectations string `json:"query_expectations"`
		API               string `json:"api"`
		FactReproduction  string `json:"fact_reproduction"`
		DynamicSQL        string `json:"dynamic_sql"`
	} `json:"oracle_claims"`
	Steps        []rcStep   `json:"steps"`
	Expectations []rcExpect `json:"expectations"`
	Producer     *struct {
		Query string `json:"query"`
	} `json:"producer"`
}

type rcSvcSegment struct {
	StartStep int `json:"start_step"`
	EndStep   int `json:"end_step"`
	Process   *struct {
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		Cleanup  struct {
			Verified bool `json:"verified"`
		} `json:"cleanup"`
	} `json:"process"`
}

// caseOutcome is the replayed verdict of one case; complete is false when a claim the
// verdict depends on was not recomputed.
type caseOutcome struct {
	status, assess string
	complete       bool
	hasError       bool
}

// statusAssessments are the assessments a non-completed case may carry.
var statusAssessments = map[string][]string{
	StatusNotRun:        {AssessNotAssessed, AssessBlocked},
	StatusResourceLimit: {AssessBlocked},
	StatusFailed:        {AssessNotAssessed},
	StatusCancelled:     {AssessNotAssessed},
}

func worseClaim(a, b string) string {
	rank := map[string]int{claimNotClaimed: 0, claimPass: 1, claimBlocked: 2, claimFail: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func foldClaims(claims []string) string {
	out := AssessPass
	for _, c := range claims {
		switch c {
		case claimFail:
			out = AssessFail
		case claimBlocked:
			if out == AssessPass {
				out = AssessBlocked
			}
		}
	}
	return out
}

// routeStepClaim is the S05 route rule for one edit step: PASS when proven; BLOCKED when
// the instrumented edit with changes reused no node at all (noReuse) and the old and the
// new incremental trees are full trees (the form whose has_error the tree gate recomputes
// from the nodes) and either has an error; FAIL otherwise. Only r2 reducers and qualify pass
// noReuse: under r1 every unproven route is a FAIL.
func routeStepClaim(proven, noReuse bool, old, cur *rcTree) string {
	full := func(t *rcTree) bool { return t != nil && t.Form == "full" && t.Tree != nil }
	switch {
	case proven:
		return claimPass
	case noReuse && full(old) && full(cur) && (old.HasError || cur.HasError):
		return claimBlocked
	}
	return claimFail
}

// replayCase recomputes one case. want is the workload's registration of the case (nil
// when the reducer has no per-case registration), queries says whether the workload
// registered queries.
func (x *replayEnv) replayCase(c *rcCase, idx int, want *IncrementalCase, queries bool) caseOutcome {
	x.checkSvcComposite(c, want)
	x.observeSvcReferences(c)
	name := x.name(c.ID, idx)
	out := caseOutcome{status: c.ExecutionStatus, assess: c.Assessment, complete: true}
	bind := x.gate("case-binding")
	ok := true
	if want != nil {
		ok = c.ID == want.ID && c.Input == want.Input
	}
	if ok && len(c.Steps) > 0 {
		ok = c.Steps[0].SourceSHA256 == c.Input.SHA256 && c.Steps[0].SourceBytes == c.Input.Bytes
	}
	if ok && want != nil && c.ExecutionStatus == StatusCompleted && len(c.Steps) > 0 {
		ok = len(c.Steps) == len(want.Edits)+1
	}
	bind.check(ok, name, "CASE_BINDING_MISMATCH", "case id, 입력 identity 또는 step 수가 workload 등록과 다르다")
	// an SVC composite names the run's producer and policy too (checked by checkSeen)
	for i, s := range c.Steps {
		bind.check(s.Step == i, name, "STEP_INDEX_MISMATCH", "step 순서가 index와 다르다")
		if s.Composite == nil {
			continue
		}
		ids := s.Composite.Identities
		if s.Composite.Inline != nil && s.Composite.Inline.Tree != nil {
			ids = append(slices.Clone(ids), s.Composite.Inline.Tree.Identities...)
		}
		x.seeRun(ids)
	}
	if len(c.Steps) > 0 && c.Steps[0].Incremental != nil {
		out.hasError = c.Steps[0].Incremental.HasError
	}
	st := x.gate("case-status")
	if c.ExecutionStatus != StatusCompleted {
		allowed, known := statusAssessments[c.ExecutionStatus]
		claims := []string{c.Claims.IncrementalEquality, c.Claims.IncrementalRoute, c.Claims.Expectations}
		if c.Oracle != nil {
			claims = append(claims, c.Oracle.QueryEquality, c.Oracle.QueryExpectations, c.Oracle.API, c.Oracle.FactReproduction, c.Oracle.DynamicSQL)
		}
		allNot := !slices.ContainsFunc(claims, func(s string) bool { return s != claimNotClaimed })
		st.check(known && slices.Contains(allowed, c.Assessment) && allNot, name, "STATUS_ASSESSMENT_INCONSISTENT",
			"완료되지 않은 case가 허용되지 않는 판정이나 claim을 가진다")
		return out
	}
	if (x.svcFormat == SvcFormat || x.svcFormat == SvcLegacyFormat) && len(c.Steps) > 0 && !slices.ContainsFunc(c.Steps, func(s rcStep) bool { return s.Incremental != nil || s.Composite == nil }) {
		// an SVC observation-only case: no inline tree; the verdict is the S05 rule over the
		// recorded directive observations
		st.pass()
		got, code := svcVerdict(c.Steps, want != nil && len(want.Expect) > 0)
		exps := []StepExpectation{}
		if want != nil {
			exps = want.Expect
		} else {
			for _, e := range c.Expectations {
				exps = append(exps, StepExpectation{Step: e.Step, Syntax: e.Syntax, Contains: e.Contains, Anchors: e.Anchors, Declarations: e.Declarations})
			}
		}
		if len(exps) > 0 {
			claim := claimPass
			eg := x.gate("expectations")
			eg.check(len(exps) == len(c.Expectations), name, "EXPECTATION_REGISTRATION_MISMATCH", "SVC 기대값 수가 다르다")
			for i, e := range exps {
				if e.Step < 0 || e.Step >= len(c.Steps) {
					eg.fail(name, "EXPECTATION_STEP_INVALID", "SVC step이 없다")
					claim = claimFail
					continue
				}
				r, _ := svcExpectResult(e, c.Steps[e.Step])
				claim = worseClaim(claim, r)
				if i < len(c.Expectations) {
					rec := c.Expectations[i]
					eg.check(rec.Step == e.Step && rec.Syntax == e.Syntax && slices.Equal(rec.Contains, e.Contains) && slices.Equal(rec.Anchors, e.Anchors) && rec.Declarations == e.Declarations && rec.Result == r, name, "EXPECTATION_MISMATCH", "SVC 기대값 결과가 다르다")
				}
			}
			eg.check(claim == c.Claims.Expectations, name, "CLAIM_MISMATCH", "SVC 기대값 claim이 다르다")
			got, code = claim, ""
			if claim == claimFail {
				code = "EXPECTATION_FAILED"
			}
			if claim == claimBlocked {
				code = "SVC_EXPECTATION_UNASSESSABLE"
			}
		}
		for _, r := range c.References {
			got = foldClaims([]string{got, r.Assessment})
		}
		x.gate("verdict").check(got == c.Assessment && code == c.Code, name, "VERDICT_MISMATCH", "SVC 관측 판정이 다시 계산한 값과 다르다")
		out.assess = got
		return out
	}
	completedTrees := len(c.Steps) > 0
	for k, s := range c.Steps {
		if s.Restarted && (s.Composite == nil || s.Incremental == nil || k == 0 || c.Steps[k-1].Incremental != nil) {
			st.fail(name, "SVC_RESTART_INVALID", "restart에 관측 공백이 없다")
			completedTrees = false
		}
		if (x.svcFormat == SvcFormat || x.svcFormat == SvcLegacyFormat) && s.Incremental == nil && s.Composite != nil {
			continue
		}
		if s.Incremental == nil || s.Incremental.Status != StatusCompleted || (s.Fresh != nil && s.Fresh.Status != StatusCompleted) {
			completedTrees = false
		}
	}
	if !st.check(completedTrees, name, "STATUS_TREE_INCONSISTENT", "COMPLETED case에 완료되지 않은 tree가 있다") {
		return out
	}
	// every tree names the run's producer and policy and its step's source: a tree from
	// another build, policy or input in the same set is a mixed result
	for _, s := range c.Steps {
		for _, t := range []*rcTree{s.Incremental, s.Fresh} {
			var ids []IdentityRef
			switch {
			case t == nil:
				continue
			case t.Tree != nil:
				ids = t.Tree.Identities
			case t.Summary != nil:
				ids = t.Summary.Identities
			}
			for _, id := range ids {
				switch id.Role {
				case "source":
					if id.SHA256 != s.SourceSHA256 {
						bind.fail(name, "MIXED_IDENTITY", fmt.Sprintf("step %d tree의 source identity가 step과 다르다", s.Step))
					}
				case "producer", "policy":
					x.seeRun([]IdentityRef{id})
				}
			}
		}
	}
	// tree: structure, digest and counts of every full tree; summary/record forms keep
	// only their digest and are recorded
	tg := x.gate("tree")
	for _, s := range c.Steps {
		for _, t := range []*rcTree{s.Incremental, s.Fresh} {
			if t == nil {
				continue
			}
			if t.Form != "full" || t.Tree == nil {
				ok := t.Summary == nil || (t.Summary.Digest.SHA256 == t.Digest && t.Summary.DescendantCount == t.DescendantCount)
				if !ok {
					tg.fail(name, "SUMMARY_INCONSISTENT", "summary digest 또는 node 수가 tree 기록과 다르다")
				} else {
					tg.recorded()
				}
				continue
			}
			nodes := t.Tree.Nodes
			switch {
			case ValidateTree(nodes, s.SourceBytes) != nil:
				tg.fail(name, "TREE_INVALID", fmt.Sprintf("step %d tree 구조가 규칙을 어긴다", s.Step))
			case TreeDigest(nodes) != t.Digest:
				tg.fail(name, "TREE_DIGEST_MISMATCH", fmt.Sprintf("step %d digest를 다시 계산한 값이 기록과 다르다", s.Step))
			case uint64(len(nodes)) != t.DescendantCount || nodes[0].HasError != t.HasError:
				tg.fail(name, "TREE_COUNT_MISMATCH", fmt.Sprintf("step %d node 수 또는 has_error가 nodes와 다르다", s.Step))
			case t.Tree.Input.SHA256 != s.SourceSHA256 || t.Tree.Input.Bytes != s.SourceBytes:
				tg.fail(name, "TREE_INPUT_MISMATCH", fmt.Sprintf("step %d tree 입력 identity가 step과 다르다", s.Step))
			default:
				tg.pass()
			}
		}
	}
	var claims, recorded []string // recomputed claims; recorded-only claims
	// incremental/fresh equality and route proof
	ie, ir := claimNotClaimed, claimNotClaimed
	if len(c.Steps) > 1 {
		ie, ir = claimPass, claimPass
		eq := x.gate("incremental-equality")
		for _, s := range c.Steps[1:] {
			if s.Incremental == nil && s.Composite != nil || s.Restarted {
				eq.check(s.Comparison == nil, name, "COMPARISON_UNEXPECTED", "SVC boundary에 comparison이 있다")
				continue
			}
			if s.Fresh == nil || s.Incremental.Form != "full" || s.Fresh.Form != "full" || s.Incremental.Tree == nil || s.Fresh.Tree == nil {
				eq.check(s.Comparison == nil, name, "COMPARISON_UNEXPECTED", fmt.Sprintf("step %d 비교할 full tree가 없는데 비교 기록이 있다", s.Step))
				ie = worseClaim(ie, claimBlocked)
				continue
			}
			d := CompareTrees(s.Incremental.Tree.Nodes, s.Fresh.Tree.Nodes)
			same := s.Comparison != nil && s.Comparison.Equal == (d == nil) && ((d == nil) == (s.Comparison.First == nil)) && (d == nil || *d == *s.Comparison.First)
			eq.check(same, name, "COMPARISON_MISMATCH", fmt.Sprintf("step %d incremental/fresh 비교를 다시 계산한 결과가 기록과 다르다", s.Step))
			if d != nil {
				ie = claimFail
			}
		}
		rg := x.gate("incremental-route")
		for k, s := range c.Steps[1:] {
			if s.Incremental == nil && s.Composite != nil || s.Restarted {
				rg.check(s.Route == nil, name, "ROUTE_PROOF_MISMATCH", "SVC boundary에 reuse proof가 있다")
				continue
			}
			r := s.Route
			proven := r != nil && r.EditHasChanges && r.ReusedNodes > 0 && r.FreshReusedNodes == 0
			rg.check(r == nil || r.Proven == proven, name, "ROUTE_PROOF_MISMATCH", fmt.Sprintf("step %d route 증명을 다시 계산한 값이 기록과 다르다", s.Step))
			ir = worseClaim(ir, routeStepClaim(proven, x.errorTreeRoute && r != nil && r.EditHasChanges && r.ReusedNodes == 0 && r.FreshReusedNodes == 0, c.Steps[k].Incremental, s.Incremental))
		}
		if slices.ContainsFunc(c.Steps, func(s rcStep) bool { return s.Composite != nil && s.Incremental == nil }) {
			ie, ir = claimNotClaimed, claimNotClaimed
		}
	}
	x.gate("incremental-equality").check(ie == c.Claims.IncrementalEquality, name, "CLAIM_MISMATCH", "incremental_equality claim이 다시 계산한 값과 다르다")
	x.gate("incremental-route").check(ir == c.Claims.IncrementalRoute, name, "CLAIM_MISMATCH", "incremental_route claim이 다시 계산한 값과 다르다")
	claims = append(claims, ie, ir)
	// expectations: recomputed from the workload registration, not from the record's list
	eg := x.gate("expectations")
	exps := []StepExpectation{}
	if want != nil {
		exps = want.Expect
	} else {
		for _, e := range c.Expectations {
			exps = append(exps, StepExpectation{Step: e.Step, Syntax: e.Syntax, Contains: e.Contains, Anchors: e.Anchors, Declarations: e.Declarations})
		}
	}
	ec, ecOK := claimNotClaimed, true
	if len(exps) > 0 {
		ec = claimPass
	}
	if len(c.Expectations) != len(exps) {
		eg.fail(name, "EXPECTATION_REGISTRATION_MISMATCH", "기록된 기대값 수가 workload 등록과 다르다")
		ecOK = false
	}
	for i, e := range exps {
		if e.Step >= len(c.Steps) {
			eg.fail(name, "EXPECTATION_STEP_INVALID", "기대값 step이 기록에 없다")
			ecOK = false
			continue
		}
		res, known := expectResult(e, c.Steps[e.Step].Incremental)
		if (x.svcFormat == SvcFormat || x.svcFormat == SvcLegacyFormat) && c.Steps[e.Step].Composite != nil {
			res, known = svcExpectResult(e, c.Steps[e.Step])
		}
		if !known {
			eg.recorded()
			ecOK = false
			if i < len(c.Expectations) {
				res = c.Expectations[i].Result
			}
		} else if i < len(c.Expectations) {
			r := c.Expectations[i]
			eg.check(r.Step == e.Step && r.Syntax == e.Syntax && slices.Equal(r.Contains, e.Contains) && slices.Equal(r.Anchors, e.Anchors) &&
				r.Declarations == e.Declarations && r.Result == res,
				name, "EXPECTATION_MISMATCH", fmt.Sprintf("기대값 %d을 다시 계산한 결과가 기록과 다르다", i))
		}
		switch res {
		case claimFail:
			ec = claimFail
		case claimBlocked:
			if ec == claimPass {
				ec = claimBlocked
			}
		}
	}
	if len(exps) == 0 && slices.ContainsFunc(c.Steps, func(s rcStep) bool {
		if s.Composite == nil {
			return false
		}
		e, k := (SvcObservation{Directive: s.Composite.Directive, AdditionalDirectives: s.Composite.AdditionalDirectives, Coverage: s.Composite.Coverage}).Syntax(nil)
		return e && k
	}) {
		ec = claimBlocked
	}
	if ecOK {
		eg.check(ec == c.Claims.Expectations, name, "CLAIM_MISMATCH", "expectations claim이 다시 계산한 값과 다르다")
		claims = append(claims, ec)
	} else {
		recorded = append(recorded, c.Claims.Expectations)
	}
	if c.Oracle != nil {
		qe := claimNotClaimed
		qeOK := true
		if queries && len(c.Steps) > 1 {
			qg := x.gate("query-equality")
			for _, s := range c.Steps[1:] {
				if s.Fresh == nil {
					continue
				}
				a, b := s.Incremental.Queries, s.Fresh.Queries
				if len(a) != len(b) {
					qg.fail(name, "QUERY_SET_MISMATCH", fmt.Sprintf("step %d 두 tree의 query 수가 다르다", s.Step))
					qe = claimFail
					continue
				}
				eq, structural := true, false
				for i := range a {
					switch {
					case a[i].ID != b[i].ID || a[i].Status != b[i].Status || a[i].Code != b[i].Code || a[i].Evaluation != b[i].Evaluation:
						eq = false
					case CompareCaptures(a[i].Captures, b[i].Captures) != nil:
						eq = false
					case a[i].Captures == nil:
						structural = true // the runtime structural stream is not recorded
					}
				}
				if eq && structural {
					qg.recorded()
					qeOK = false
					continue
				}
				qg.check(s.QueryCompare != nil && s.QueryCompare.Equal == eq, name, "QUERY_COMPARISON_MISMATCH", fmt.Sprintf("step %d query 비교를 다시 계산한 결과가 기록과 다르다", s.Step))
				if eq {
					qe = worseClaim(qe, claimPass)
				} else {
					qe = claimFail
				}
			}
			if slices.ContainsFunc(c.Steps, func(s rcStep) bool { return s.Composite != nil && s.Incremental == nil }) {
				qe = claimNotClaimed
			}
		}
		if qeOK {
			x.gate("query-equality").check(qe == c.Oracle.QueryEquality, name, "CLAIM_MISMATCH", "query_equality claim이 다시 계산한 값과 다르다")
			claims = append(claims, qe)
		} else {
			recorded = append(recorded, c.Oracle.QueryEquality)
		}
		for _, rc := range []struct{ id, v string }{{"api", c.Oracle.API}, {"query-expectations", c.Oracle.QueryExpectations},
			{"fact-reproduction", c.Oracle.FactReproduction}, {"dynamic-sql", c.Oracle.DynamicSQL}} {
			if rc.v != claimNotClaimed {
				x.gate(rc.id).recorded()
				recorded = append(recorded, rc.v)
			}
		}
	}
	vg := x.gate("verdict")
	for _, r := range c.References {
		claims = append(claims, r.Assessment)
	}
	vg.check(foldClaims(append(slices.Clone(claims), recorded...)) == c.Assessment, name, "VERDICT_MISMATCH", "case 판정이 claim의 가장 나쁜 값과 다르다")
	re := foldClaims(claims)
	switch {
	case len(recorded) == 0:
		out.assess = re
	case re == AssessFail:
		out.assess = AssessFail
	default:
		out.assess, out.complete = AssessUnresolved, false
	}
	return out
}

// seeRun records the producer and policy identities a case names; they are compared with
// the run's values once the whole document is read, so member order does not matter.
func (x *replayEnv) seeRun(ids []IdentityRef) {
	for _, id := range ids {
		if id.Role != "producer" && id.Role != "policy" {
			continue
		}
		if x.seen[id.Role] == nil {
			x.seen[id.Role] = map[string]bool{}
		}
		x.seen[id.Role][id.SHA256] = true
	}
}

// checkSeen compares the producer and policy every tree named with the run's values; a
// tree from another build or policy in the same document is a mixed result.
func (x *replayEnv) checkSeen(run map[string]string) {
	g := x.gate("case-binding")
	for _, role := range []string{"producer", "policy"} {
		for _, v := range sortedKeys(x.seen[role]) {
			if v != run[role] {
				g.fail(role, "MIXED_IDENTITY", "tree의 "+role+" identity가 run과 다르다")
			}
		}
	}
	x.seen = map[string]map[string]bool{}
}

// svcVerdict is the S05 observation-only rule: PASS only when every step is a directive
// without inline code and no expectation needs a tree.
func svcVerdict(steps []rcStep, expect bool) (string, string) {
	for _, s := range steps {
		d, cov := s.Composite.Directive, s.Composite.Coverage
		switch {
		case d == nil:
			return AssessBlocked, "SVC_DIRECTIVE_ABSENT"
		case d.Close == nil, cov.Inline == "UNRESOLVED":
			return AssessBlocked, "SVC_INLINE_UNRESOLVED"
		case cov.Inline == "UNSUPPORTED":
			return AssessBlocked, "SVC_INLINE_UNSUPPORTED"
		case cov.Inline == "OBSERVED":
			return AssessBlocked, "SVC_INLINE_NOT_PARSED"
		}
	}
	if expect {
		return AssessBlocked, "SVC_EXPECTATION_UNASSESSABLE"
	}
	for _, s := range steps {
		if e, k := (SvcObservation{Directive: s.Composite.Directive, AdditionalDirectives: s.Composite.AdditionalDirectives, Coverage: s.Composite.Coverage}).Syntax(nil); e && k {
			return AssessBlocked, "SVC_DIRECTIVE_DIAGNOSTICS"
		}
	}
	return AssessPass, ""
}

func svcExpectResult(e StepExpectation, s rcStep) (string, bool) {
	o := SvcObservation{Directive: s.Composite.Directive, AdditionalDirectives: s.Composite.AdditionalDirectives, Coverage: s.Composite.Coverage}
	var inline *bool
	result := claimPass
	if s.Incremental != nil {
		inline = &s.Incremental.HasError
		copy := e
		copy.Syntax = "ANY"
		var known bool
		result, known = expectResult(copy, s.Incremental)
		if !known {
			return result, false
		}
	} else if len(e.Contains) > 0 || len(e.Anchors) > 0 || e.Declarations != "" {
		result = claimBlocked
	}
	hasError, known := o.Syntax(inline)
	if e.Syntax != "ANY" && !known {
		return claimBlocked, true
	}
	if e.Syntax == "NO_ERROR" && hasError || e.Syntax == "ERROR" && !hasError && known {
		return claimFail, true
	}
	return result, true
}

// expectResult recomputes one expectation as S05 evaluate does; known is false when the
// record lacks what the check needs.
func expectResult(e StepExpectation, t *rcTree) (string, bool) {
	if t == nil {
		return "", false
	}
	if (e.Syntax == "NO_ERROR" && t.HasError) || (e.Syntax == "ERROR" && !t.HasError) {
		return claimFail, true
	}
	if len(e.Contains) > 0 {
		if t.Form != "full" {
			return claimBlocked, true
		}
		if t.Tree == nil {
			return "", false
		}
		seen := map[string]bool{}
		for _, n := range t.Tree.Nodes {
			if n.Named {
				seen[n.Type] = true
			}
		}
		for _, ty := range e.Contains {
			if !seen[ty] {
				return claimFail, true
			}
		}
	}
	if len(e.Anchors) > 0 {
		if t.Form != "full" {
			return claimBlocked, true
		}
		if t.Tree == nil {
			return "", false
		}
		for _, a := range e.Anchors {
			if !anchorFound(t.Tree.Nodes, a) {
				return claimFail, true
			}
		}
	}
	if e.Declarations != "" {
		var items *[]rcDecl
		switch {
		case t.Form == "full":
			items = t.Declarations // r1 full trees do not record the items
		case t.Summary != nil && t.Summary.Declarations.Assessment != AssessNotAssessed:
			items = &t.Summary.Declarations.Items
		case t.Summary != nil:
			return claimBlocked, true
		}
		if items == nil {
			return "", false
		}
		got := AssessNotApplicable
		if len(*items) > 0 {
			got = AssessPass
			for _, it := range *items {
				if it.Status != "PASS" {
					got = AssessFail
				}
			}
		}
		if got != e.Declarations {
			return claimFail, true
		}
	}
	return claimPass, true
}

// aggregateCases folds case outcomes into the run verdict (S05 aggregate: worst assessment,
// FAIL > BLOCKED > NOT_ASSESSED > PASS); an unresolved case keeps a non-failed run unresolved.
func aggregateCases(outs []caseOutcome) Verdict {
	status, assess := StatusCompleted, AssessPass
	rank := map[string]int{AssessPass: 0, AssessNotAssessed: 1, AssessBlocked: 2, AssessFail: 3}
	unresolved := false
	for _, c := range outs {
		switch c.status {
		case StatusCompleted:
		case StatusCancelled:
			status = StatusCancelled
		case StatusResourceLimit:
			if status == StatusCompleted {
				status = StatusResourceLimit
			}
		default:
			if status == StatusCompleted || status == StatusResourceLimit {
				status = StatusFailed
			}
		}
		if c.assess == AssessUnresolved {
			unresolved = true
			continue
		}
		if rank[c.assess] > rank[assess] {
			assess = c.assess
		}
	}
	if len(outs) == 0 {
		return Verdict{StatusNotRun, AssessNotAssessed}
	}
	if unresolved && assess != AssessFail {
		assess = AssessUnresolved
	}
	return Verdict{status, assess}
}

// consume accounts the records actually seen against the expected ordered list.
type consumer struct {
	x      *replayEnv
	want   []string
	index  map[string]int
	seen   map[string]int
	next   int
	redact bool
}

func newConsumer(x *replayEnv, want []string) *consumer {
	c := &consumer{x: x, want: want, index: map[string]int{}, seen: map[string]int{}}
	for i, w := range want {
		if _, dup := c.index[w]; dup {
			x.cons.Duplicate++
			c.first(fmt.Sprintf("expected %s", x.name(w, i)))
			continue
		}
		c.index[w] = i
	}
	x.cons.Expected += len(c.index)
	return c
}

func (c *consumer) first(s string) {
	if c.x.cons.First == "" {
		c.x.cons.First = s
	}
}

// take consumes one record; it returns false for an unused or duplicate record.
func (c *consumer) take(id string, ordinal int) bool {
	i, ok := c.index[id]
	if !ok {
		c.x.cons.Unused++
		c.first("unused " + c.x.name(id, ordinal))
		return false
	}
	if c.seen[id]++; c.seen[id] > 1 {
		c.x.cons.Duplicate++
		c.first("duplicate " + c.x.name(id, ordinal))
		return false
	}
	if i < c.next { // arrives after a record registered later
		c.x.cons.OutOfOrder++
		c.first("out of order " + c.x.name(id, ordinal))
	}
	c.next = max(c.next, i+1)
	c.x.cons.Consumed++
	return true
}

func (c *consumer) close() {
	for i, w := range c.want {
		if c.seen[w] == 0 {
			c.x.cons.Missing++
			c.first("missing " + c.x.name(w, i))
		}
	}
}

// streamTop reads a top-level JSON object member by member; each member value is bounded
// by RecordBytes, except arrayKey whose elements are handed one at a time to each.
func (x *replayEnv) streamTop(m *memberReader, arrayKey string, member func(name string, raw jsontext.Value) *Error, each func(raw jsontext.Value, i int) *Error) *Error {
	dec := jsontext.NewDecoder(m)
	bound := func() { m.limit = dec.InputOffset() + int64(x.lim.RecordBytes) + 2*readChunk }
	jerr := func(err error) *Error {
		if m.err != nil {
			return m.err
		}
		if errors.Is(err, errRecordLimit) {
			return fail(KindResourceLimit, "RECORD_BYTES_LIMIT", m.path, nil)
		}
		return jsonError(m.path, "", err)
	}
	bound()
	tok, err := dec.ReadToken()
	if err != nil {
		return jerr(err)
	}
	if tok.Kind() != '{' {
		return fail(KindInvalidInput, "JSON_TYPE", m.path, nil)
	}
	for dec.PeekKind() != '}' {
		bound()
		nt, err := dec.ReadToken()
		if err != nil {
			return jerr(err)
		}
		name := nt.String()
		if name == arrayKey {
			if dec.PeekKind() == 'n' {
				if _, err := dec.ReadToken(); err != nil {
					return jerr(err)
				}
				continue
			}
			if dec.PeekKind() != '[' {
				return fail(KindInvalidInput, "JSON_TYPE", m.path+"#/"+name, nil)
			}
			if _, err := dec.ReadToken(); err != nil {
				return jerr(err)
			}
			for i := 0; dec.PeekKind() != ']'; i++ {
				if uint64(i) >= x.lim.Records {
					return fail(KindResourceLimit, "RECORD_COUNT_LIMIT", m.path, nil)
				}
				bound()
				raw, err := dec.ReadValue()
				if err != nil {
					return jerr(err)
				}
				if uint64(len(raw)) > x.lim.RecordBytes {
					return fail(KindResourceLimit, "RECORD_BYTES_LIMIT", m.path, nil)
				}
				if e := each(raw, i); e != nil {
					return e
				}
			}
			bound()
			if _, err := dec.ReadToken(); err != nil {
				return jerr(err)
			}
			continue
		}
		raw, err := dec.ReadValue()
		if err != nil {
			return jerr(err)
		}
		if e := member(name, raw); e != nil {
			return e
		}
	}
	bound()
	if _, err := dec.ReadToken(); err != nil {
		return jerr(err)
	}
	m.limit = 0
	if _, err := dec.ReadToken(); err != io.EOF {
		if err == nil {
			return fail(KindInvalidInput, "JSON_TRAILING_VALUE", m.path, nil)
		}
		return jerr(err)
	}
	return m.finish()
}

func decodeRecord(path string, raw []byte, v any) *Error {
	if err := jsonv2.Unmarshal(raw, v); err != nil {
		return fail(KindInvalidInput, "RECORD_INVALID", path, err)
	}
	return nil
}

// membersByRole returns the profile members of one role in path order.
func (x *replayEnv) membersByRole(role string) []ReplayMember {
	var out []ReplayMember
	for _, m := range x.prof.Members {
		if m.Role == role {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b ReplayMember) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	return out
}

func (x *replayEnv) oneMember(role string) (ReplayMember, *Error) {
	ms := x.membersByRole(role)
	if len(ms) != 1 {
		return ReplayMember{}, fail(KindInvalidInput, "MEMBER_ROLE_COUNT", role, nil)
	}
	return ms[0], nil
}

// nativeResult is the top of a tsgk-incremental-result/r1 document.
type nativeResult struct {
	Schema          string        `json:"schema"`
	ResultSchema    string        `json:"result_schema"`
	ExecutionStatus string        `json:"execution_status"`
	Assessment      string        `json:"assessment"`
	Identities      []IdentityRef `json:"identities"`
	Route           string        `json:"route"`
	Operation       struct {
		Name string `json:"name"`
	} `json:"operation"`
	Build *struct {
		Identity         string `json:"identity"`
		ExecutableSHA256 string `json:"executable_sha256"`
		RuntimeCommit    string `json:"runtime_commit"`
		Compiler         struct {
			SHA256 string `json:"sha256"`
		} `json:"compiler"`
	} `json:"build"`
	Summary struct {
		Cases       int            `json:"cases"`
		Statuses    map[string]int `json:"execution_statuses"`
		Assessments map[string]int `json:"assessments"`
		Codes       map[string]int `json:"codes"`
		HasError    int            `json:"has_error"`
	} `json:"summary"`
	Platform      string `json:"platform"`
	ProfileSHA256 string `json:"profile_sha256"`
}

var responseName = regexp.MustCompile(`^responses/([0-9]{5})-(.+)\.json$`)

// replayResultFile replays one S05 result document against its workload profile. prefix
// namespaces the identity roles (private corpus routes); it returns the outcomes and the
// per-case projection used by the private corpus reducer.
func (x *replayEnv) replayResultFile(resultPath string, prof IncrementalProfile, prefix string, onCase func(c *rcCase, o caseOutcome, i int)) (nativeResult, []caseOutcome, *Error) {
	var top nativeResult
	want := map[string]*IncrementalCase{}
	ids := make([]string, 0, len(prof.Cases))
	for i := range prof.Cases {
		want[prof.Cases[i].ID] = &prof.Cases[i]
		ids = append(ids, prof.Cases[i].ID)
	}
	cons := newConsumer(x, ids)
	x.seen = map[string]map[string]bool{}
	defer func() {
		run := map[string]string{}
		if top.Build != nil {
			run["producer"] = top.Build.Identity
		}
		for _, id := range top.Identities {
			if id.Role == "policy" {
				run["policy"] = id.SHA256
			}
		}
		x.checkSeen(run)
	}()
	x.startSvcReferences(prof.SvcContext)
	x.svcFormat = prof.Format
	defer x.checkSvcReferences()
	m, e := x.stream(resultPath)
	if e != nil {
		return top, nil, e
	}
	var outs []caseOutcome
	sum := struct {
		statuses, assessments, codes map[string]int
		hasError, cases              int
	}{map[string]int{}, map[string]int{}, map[string]int{}, 0, 0}
	e = x.streamTop(m, "cases", func(name string, raw jsontext.Value) *Error {
		switch name {
		case "schema", "result_schema", "execution_status", "assessment", "identities", "route", "operation", "build", "summary", "platform", "profile_sha256":
			wrapped := append(append([]byte(`{"`+name+`":`), raw...), '}')
			return decodeRecord(resultPath, wrapped, &top)
		}
		return nil
	}, func(raw jsontext.Value, i int) *Error {
		var c rcCase
		if e := decodeRecord(resultPath, raw, &c); e != nil {
			return e
		}
		if !cons.take(c.ID, i) {
			return nil
		}
		o := x.replayCase(&c, i, want[c.ID], false)
		outs = append(outs, o)
		sum.cases++
		sum.statuses[c.ExecutionStatus]++
		sum.assessments[c.Assessment]++
		if c.Code != "" {
			sum.codes[c.Code]++
		}
		if o.hasError {
			sum.hasError++
		}
		if onCase != nil {
			onCase(&c, o, i)
		}
		return x.r.check()
	})
	cons.close()
	if e != nil {
		return top, outs, e
	}
	if top.Schema != ReportSchema || top.ResultSchema != "tsgk-incremental-result/r1" {
		return top, outs, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", resultPath, nil)
	}
	sg := x.gate("summary")
	s := top.Summary
	sg.check(s.Cases == sum.cases && maps.Equal(s.Statuses, sum.statuses) && maps.Equal(s.Assessments, sum.assessments) && maps.Equal(s.Codes, sum.codes) && s.HasError == sum.hasError,
		resultPath, "SUMMARY_MISMATCH", "result summary 개수가 case에서 다시 센 값과 다르다")
	// the run verdict must be the S05 aggregate of the recorded case verdicts
	sg.check(aggregateRecorded(sum.statuses, sum.assessments, sum.cases) == Verdict{top.ExecutionStatus, top.Assessment}, resultPath, "RUN_VERDICT_MISMATCH", "run 판정이 case 판정의 집계와 다르다")
	pol := ""
	for _, id := range top.Identities {
		if id.Role == "policy" {
			pol = id.SHA256
		}
	}
	x.actual[prefix+"workload"] = top.ProfileSHA256
	x.actual[prefix+"policy"] = pol
	x.actual[prefix+"operation"] = top.Operation.Name
	x.actual[prefix+"platform"] = top.Platform
	if top.Build != nil {
		x.actual[prefix+"producer"] = top.Build.Identity
		x.actual[prefix+"executable"] = top.Build.ExecutableSHA256
		x.actual[prefix+"compiler"] = top.Build.Compiler.SHA256
		x.actual[prefix+"runtime"] = top.Build.RuntimeCommit
	}
	if top.ProfileSHA256 != prof.SHA256 {
		x.finding("WORKLOAD_MISMATCH", resultPath, "result가 결속한 workload profile이 등록 profile과 다르다")
	}
	return top, outs, nil
}

// aggregateRecorded is the S05 run aggregate over recorded case counts.
func aggregateRecorded(statuses, assessments map[string]int, n int) Verdict {
	var outs []caseOutcome
	for s, k := range statuses {
		for range k {
			outs = append(outs, caseOutcome{status: s, assess: AssessPass})
		}
	}
	v := aggregateCases(outs)
	rank := map[string]int{AssessPass: 0, AssessNotAssessed: 1, AssessBlocked: 2, AssessFail: 3}
	a := AssessPass
	for s, k := range assessments {
		if k > 0 && rank[s] > rank[a] {
			a = s
		}
	}
	if n == 0 {
		return Verdict{StatusNotRun, AssessNotAssessed}
	}
	return Verdict{v.ExecutionStatus, a}
}

// replayNativeResult: an S05 result.json, its workload profile and its responses.
func replayNativeResult(x *replayEnv) *Error {
	pm, e := x.oneMember("workload-profile")
	if e != nil {
		return e
	}
	rm, e := x.oneMember("result")
	if e != nil {
		return e
	}
	data, e := x.member(pm.Path)
	if e != nil {
		return e
	}
	prof, e := parseIncremental(data)
	if e != nil {
		return e
	}
	prof.SHA256 = digestHex(data)
	if x.prof.Records != nil && !slices.Equal(x.prof.Records, caseIDs(prof.Cases)) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "등록 record 목록이 workload profile의 case와 다르다")
	}
	top, outs, e := x.replayResultFile(rm.Path, prof, "", nil)
	if e != nil {
		return e
	}
	x.recorded = Verdict{top.ExecutionStatus, top.Assessment}
	x.recomp = aggregateCases(outs)
	// responses: one raw member per case position, never parsed by the offline API
	rg := x.gate("responses")
	for _, m := range x.membersByRole("response") {
		sm := responseName.FindStringSubmatch(m.Path)
		ok := sm != nil
		if ok {
			n, _ := strconv.Atoi(sm[1])
			ok = n < len(prof.Cases) && prof.Cases[n].ID == sm[2]
		}
		rg.check(ok, m.Path, "RESPONSE_UNBOUND", "response 이름이 workload case 순번·id와 맞지 않는다")
		x.consumed[m.Path] = ok
	}
	return nil
}

func caseIDs(cs []IncrementalCase) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

// replayOracleSet: an S06 record set, checked with the shared record checks, then each
// record replayed with the S05 gates and the query comparison.
func replayOracleSet(x *replayEnv) *Error {
	pm, e := x.oneMember("workload-profile")
	if e != nil {
		return e
	}
	mm, e := x.oneMember("manifest")
	if e != nil {
		return e
	}
	data, e := x.member(pm.Path)
	if e != nil {
		return e
	}
	prof, e := parseOracle(data)
	if e != nil {
		return e
	}
	mdata, e := x.member(mm.Path)
	if e != nil {
		return e
	}
	var man OracleManifest
	if e := decodeOracleManifest(mdata, &man); e != nil {
		return fail(KindInvalidInput, "MEMBER_INVALID_"+e.Code, mm.Path, nil)
	}
	ids := caseIDs(prof.Native.Cases)
	if x.prof.Records != nil && !slices.Equal(x.prof.Records, ids) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "등록 record 목록이 workload profile의 case와 다르다")
	}
	sg := x.gate("summary")
	sg.check(slices.Equal(man.Cases, ids), mm.Path, "MANIFEST_CASES_MISMATCH", "manifest 사례 목록이 workload profile과 다르다")
	cons := newConsumer(x, ids)
	want := map[string]*IncrementalCase{}
	for i := range prof.Native.Cases {
		want[prof.Native.Cases[i].ID] = &prof.Native.Cases[i]
	}
	x.seen = map[string]map[string]bool{}
	defer x.checkSeen(map[string]string{"producer": man.Producer["build_identity"], "policy": man.Policy.SHA256})
	x.startSvcReferences(prof.Native.SvcContext)
	x.svcFormat = prof.Native.Format
	defer x.checkSvcReferences()
	var outs []caseOutcome
	statuses, assessments := map[string]int{}, map[string]int{}
	records := 0
	for i, mem := range man.Members {
		pmem, registered := x.members[mem.Path]
		if !registered || pmem.SHA256 != mem.SHA256 || pmem.Bytes != mem.Bytes {
			x.finding("MANIFEST_MEMBER_UNREGISTERED", mem.Path, "manifest member가 등록 inventory와 다르다")
			continue
		}
		if mem.Role != "record" {
			// the raw driver response is bound by its hash; the offline API does not decode it
			if e := x.verifyFile(mem.Path, mem.Bytes, mem.SHA256); e != nil {
				return e
			}
			x.consumed[mem.Path] = true
			continue
		}
		b, e := x.member(mem.Path)
		if e != nil {
			return e
		}
		records++
		if code := checkRecord(b, mem); code != "" {
			x.finding(code, mem.Path, "record가 완결되지 않았거나 member identity와 다르다")
			continue
		}
		var c rcCase
		if e := decodeRecord(mem.Path, b, &c); e != nil {
			return e
		}
		if !cons.take(c.ID, i) {
			continue
		}
		o := x.replayCase(&c, i, want[c.ID], len(prof.Queries) > 0)
		outs = append(outs, o)
		statuses[c.ExecutionStatus]++
		assessments[c.Assessment]++
		if e := x.r.check(); e != nil {
			return e
		}
	}
	cons.close()
	sg.check(records == man.Records, mm.Path, "RECORD_COUNT_MISMATCH", "record 수가 manifest와 다르다")
	sg.check(aggregateRecorded(statuses, assessments, records) == Verdict{man.ExecutionStatus, man.Assessment}, mm.Path, "RUN_VERDICT_MISMATCH", "set 판정이 record 판정의 집계와 다르다")
	x.recorded = Verdict{man.ExecutionStatus, man.Assessment}
	x.recomp = aggregateCases(outs)
	if man.Workload["profile_sha256"] != digestHex(data) {
		x.finding("WORKLOAD_MISMATCH", mm.Path, "set이 결속한 workload profile이 등록 profile과 다르다")
	}
	qs := ""
	for _, q := range man.Queries {
		qs += q.Role + "=" + q.SHA256 + ";"
	}
	fp := ""
	if man.FactPack != nil {
		fp = man.FactPack.Revision + "=" + man.FactPack.SHA256
	}
	for k, v := range map[string]string{"workload": man.Workload["profile_sha256"], "operation": man.Workload["operation"], "producer": man.Producer["build_identity"],
		"executable": man.Producer["executable_sha256"], "compiler": man.Producer["compiler_sha256"], "runtime": man.Producer["runtime_commit"],
		"platform": man.Producer["platform"], "policy": man.Policy.SHA256, "protocol": man.Protocol, "comparators": joinList(man.Comparators), "queries": qs, "fact_pack": fp} {
		x.actual[k] = v
	}
	return nil
}

func joinList(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
