package kit

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	QualificationSemanticsResultSchema  = "tsgk-qualification-result/r10"
	QualificationSemanticsPlanSchema    = "tsgk-qualification-query-semantics-plan/r1"
	QualificationSemanticsContextSchema = "tsgk-qualification-query-semantics-context/r1"
)

// QualificationQuerySemanticsRequest supplies caller documents for the small r4 scope.
type QualificationQuerySemanticsRequest struct {
	Plan                    []byte
	Targets                 []QualificationQuerySemanticsInput
	ForbiddenFileIdentities []fs.FileInfo `json:"-"`
}

// QualificationQuerySemanticsInput names one preregistered target and its external approval.
type QualificationQuerySemanticsInput struct {
	ID                   string
	Replay               ReplayRequest
	EligibilityApproval  []byte
	Context              []byte
	ExpectedRootIdentity fs.FileInfo `json:"-"`
}

// QualBaselineSummary preserves the original qualification verdict before the opt-in fold.
type QualBaselineSummary struct {
	Assessment     string            `json:"assessment"`
	Completeness   string            `json:"completeness"`
	MechanismGate  string            `json:"mechanism_gate"`
	SupportClaim   string            `json:"support_claim"`
	PlatformClaims map[string]string `json:"platform_claims"`
}

// QualSemanticDigests uses null for a caller document that was not consumed.
type QualSemanticDigests struct {
	Profile  *string `json:"profile_sha256"`
	Approval *string `json:"approval_sha256"`
	Context  *string `json:"context_sha256"`
}

// QualQuerySemanticsTarget keeps the plan denominator separate from the core's counters.
type QualQuerySemanticsTarget struct {
	ID                         string                  `json:"id"`
	Route                      string                  `json:"route"`
	Role                       string                  `json:"role"`
	Set                        string                  `json:"set"`
	Platform                   string                  `json:"platform"`
	ProbeID                    string                  `json:"probe_id"`
	State                      string                  `json:"state"`
	Assessment                 string                  `json:"assessment"`
	Code                       string                  `json:"code"`
	ExpectedProfileSHA256      *string                 `json:"expected_profile_sha256"`
	ExpectedRegistrationSHA256 *string                 `json:"expected_registration_sha256"`
	Tuple                      *QueryEligibilityCounts `json:"tuple"`
	Digests                    QualSemanticDigests     `json:"digests"`
	Core                       *QuerySemanticsResult   `json:"core"`
	BytesRead                  uint64                  `json:"bytes_read"`
}

// QualQuerySemanticsAxis is a partial probe scope; it cannot establish global support.
type QualQuerySemanticsAxis struct {
	Scope              string                     `json:"scope"`
	PlanSHA256         *string                    `json:"plan_sha256"`
	Assessment         string                     `json:"assessment"`
	Code               string                     `json:"code"`
	Targets            []QualQuerySemanticsTarget `json:"targets"`
	ProcessingComplete bool                       `json:"processing_complete"`
	FullScopeComplete  bool                       `json:"full_scope_complete"`
	MissingScopes      []string                   `json:"missing_scopes"`
	Limits             ReplayLimits               `json:"limits"`
	WorkLimit          uint64                     `json:"work_limit"`
	LogicalBytesLimit  uint64                     `json:"logical_bytes_limit"`
	BytesRead          uint64                     `json:"bytes_read"`
	Work               uint64                     `json:"work"`
	LogicalBytes       uint64                     `json:"logical_bytes"`
	Entries            uint64                     `json:"entries"`
}

type qualSemanticTuple struct {
	Case     string `json:"case"`
	Query    string `json:"query"`
	Step     uint32 `json:"step"`
	TreeRole string `json:"tree_role"`
}

type qualSemanticPlanTarget struct {
	ID                 string              `json:"id"`
	Route              string              `json:"route"`
	Role               string              `json:"role"`
	Set                string              `json:"set"`
	Platform           string              `json:"platform"`
	ProbeID            string              `json:"probe_id"`
	ProfileSHA256      string              `json:"profile_sha256"`
	RegistrationSHA256 string              `json:"registration_sha256"`
	Queries            []QualQuery         `json:"queries"`
	Required           []qualSemanticTuple `json:"required"`
}

type qualSemanticPlan struct {
	Schema          string                   `json:"schema"`
	ID              string                   `json:"id"`
	InventorySHA256 string                   `json:"inventory_sha256"`
	Context         string                   `json:"context"`
	Scope           string                   `json:"scope"`
	Targets         []qualSemanticPlanTarget `json:"targets"`
}

type qualSemanticContext struct {
	Schema           string `json:"schema"`
	PlanSHA256       string `json:"plan_sha256"`
	TargetID         string `json:"target_id"`
	Repository       string `json:"repository"`
	Workflow         string `json:"workflow"`
	WorkflowPath     string `json:"workflow_path"`
	WorkflowRevision string `json:"workflow_revision"`
	RunID            string `json:"run_id"`
	Attempt          uint64 `json:"attempt"`
	Event            string `json:"event"`
	HeadSHA          string `json:"head_sha"`
	Checkout         string `json:"checkout"`
	ArtifactID       string `json:"artifact_id"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	GraphSHA256      string `json:"graph_sha256"`
}

const qualSemanticScope = "small-full-input-predicate-probes/r1"

// Charge each comparison before doing it, so two targets share the actual sorting work.
func qualificationMembersText(members []ReplayMember, b *eligibilityBudget) (string, *Error) {
	fields := uint64(0)
	for _, member := range members {
		if e := b.charge(1, 0, "replay-profile-members"); e != nil {
			return "", e
		}
		fields += uint64(len(member.Path) + len(member.Role) + len(member.SHA256) + 32)
	}
	levels := uint64(1)
	for n := len(members); n > 1; n = (n + 1) / 2 {
		levels++
	}
	// Reserve two member slices, builder growth plus hash input, and fixed header/digest space.
	if e := b.charge(8*(fields+32)+uint64(len(members))*(levels+2), 3*fields+128*uint64(len(members))+256, "replay-profile-members"); e != nil {
		return "", e
	}
	sorted := slices.Clone(members)
	temp := make([]ReplayMember, len(members))
	for width := 1; width < len(sorted); width *= 2 {
		for lo := 0; lo < len(sorted); lo += 2 * width {
			mid, hi := min(lo+width, len(sorted)), min(lo+2*width, len(sorted))
			i, j := lo, mid
			for k := lo; k < hi; k++ {
				left := j == hi
				if i < mid && j < hi {
					if e := b.charge(uint64(len(sorted[i].Path)+len(sorted[j].Path))+1, 0, "replay-profile-members-compare"); e != nil {
						return "", e
					}
					left = strings.Compare(sorted[i].Path, sorted[j].Path) <= 0
				}
				if left {
					temp[k], i = sorted[i], i+1
				} else {
					temp[k], j = sorted[j], j+1
				}
			}
		}
		sorted, temp = temp, sorted
	}
	var text strings.Builder
	text.WriteString("tsgk-replay-members/r1\n")
	for _, member := range sorted {
		if e := b.r.check(); e != nil {
			return "", e
		}
		fmt.Fprintf(&text, "%s\x00%s\x00%d\x00%s\n", member.Path, member.Role, member.Bytes, member.SHA256)
	}
	return text.String(), nil
}

type qualSemanticRun struct {
	q         *qualifier
	r         *run
	lim       ReplayLimits
	axis      *QualQuerySemanticsAxis
	planSHA   string
	planBytes uint64
	targets   []*qualSemanticTargetRun
	roots     []*os.Root
}

type qualSemanticTargetRun struct {
	owner        *qualSemanticRun
	plan         qualSemanticPlanTarget
	input        QualificationQuerySemanticsInput
	row          *QualQuerySemanticsTarget
	host         *qhost
	profile      *ReplayProfile
	replay       *ReplayResult
	context      qualSemanticContext
	root         *os.Root
	rootInfo     fs.FileInfo
	directories  map[string]fs.FileInfo
	blockedRoots []fs.FileInfo
	blocked      bool
	registered   bool
	tuplesMatch  bool
}

func newQualSemanticsAxis() *QualQuerySemanticsAxis {
	lim := ReplayOperations()["evidence-replay"]
	if testHookReplayLimits != nil {
		lim = testHookReplayLimits(lim)
	}
	return &QualQuerySemanticsAxis{Scope: qualSemanticScope, Assessment: AssessBlocked,
		Targets: []QualQuerySemanticsTarget{
			{Platform: "windows-amd64", State: "UNREGISTERED", Assessment: AssessBlocked, Code: "SEMANTICS_TARGET_UNREGISTERED"},
			{Platform: "linux-amd64", State: "UNREGISTERED", Assessment: AssessBlocked, Code: "SEMANTICS_TARGET_UNREGISTERED"},
		}, MissingScopes: []string{"ordinary-query-workloads", "svc-query-workloads", "large-query-workloads"},
		Limits: lim, WorkLimit: eligibilityWorkLimit, LogicalBytesLimit: eligibilityLogicalLimit}
}

func semanticDigest(data []byte) *string { s := digestHex(data); return &s }

func (s *qualSemanticRun) document(name string, data []byte) *Error {
	if e := s.r.check(); e != nil {
		return e
	}
	if uint64(len(data)) > s.lim.FileBytes {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", name, nil)
	}
	left, e := s.r.remainingRead(s.lim.TotalBytes, name)
	if e != nil {
		return e
	}
	if uint64(len(data)) > left {
		return fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", name, nil)
	}
	// Reserve caller-document hashing before consuming its bytes.
	if e := s.r.eligibility.charge(uint64(len(data))+1, 256, name); e != nil {
		return e
	}
	return s.r.chargeRead(uint64(len(data)), s.lim.TotalBytes, name)
}

func (t *qualSemanticTargetRun) failure(code string) {
	wasFailure := t.row.Assessment == AssessFail
	t.row.Assessment, t.row.State = AssessFail, "REJECTED"
	if !wasFailure || t.row.Code == "" {
		t.row.Code = code
	}
	t.owner.axis.Assessment = AssessFail
	if t.owner.axis.Code == "" {
		t.owner.axis.Code = code
	}
}

func (t *qualSemanticTargetRun) finishCounts() {
	if t.row.Tuple == nil {
		return
	}
	n := t.row.Tuple.Required
	if t.row.Core == nil || !t.tuplesMatch {
		*t.row.Tuple = QueryEligibilityCounts{Required: n}
		if t.row.State == "INTERRUPTED" || t.row.State == "NOT_RUN" {
			t.row.Tuple.Unprocessed = n
		} else if t.row.Assessment == AssessFail {
			t.row.Tuple.Failed = n
		} else {
			t.row.Tuple.Blocked = n
		}
	}
}

func (q *qualifier) querySemantics(ctx context.Context, req QualificationQuerySemanticsRequest, axis *QualQuerySemanticsAxis) (resultErr *Error) {
	r, e := startRun(ctx, axis.Limits.Wall)
	if e != nil {
		return e
	}
	r.parent, r.readCeiling = q.r, axis.Limits.TotalBytes
	r.forbiddenFiles = q.r.forbiddenFiles
	r.eligibility = &eligibilityBudget{r: r}
	r.semanticEntries = newSemanticEntryBudget(axis.Limits.Files)
	s := &qualSemanticRun{q: q, r: r, lim: axis.Limits, axis: axis}
	q.semantic = s
	defer func() {
		for _, root := range s.roots {
			if err := root.Close(); err != nil && resultErr == nil {
				resultErr = fail(KindIO, "ROOT_CLOSE_FAILED", "query-semantics", err)
			}
		}
		s.roots = nil
		q.restoreQuerySemanticsStats()
	}()
	if len(q.hosts) > 3 || len(req.Targets) > 2 {
		return fail(KindInvalidInput, "SEMANTICS_INPUT_LIMIT", "query-semantics", nil)
	}
	if req.Plan == nil {
		return fail(KindInvalidInput, "SEMANTICS_PLAN_REQUIRED", "semantic-plan", nil)
	}
	start := r.totalRead
	if e := s.document("semantic-plan", req.Plan); e != nil {
		return e
	}
	if e := q.r.chargeRead(r.totalRead-start, q.lim.TotalBytes, "semantic-plan"); e != nil {
		return e
	}
	axis.PlanSHA256 = semanticDigest(req.Plan)
	s.planSHA = *axis.PlanSHA256
	s.planBytes = r.totalRead - start
	var plan qualSemanticPlan
	if e := r.eligibility.decode("semantic-plan", req.Plan, &plan, true); e != nil {
		return e
	}
	if plan.Schema != QualificationSemanticsPlanSchema || !validID(plan.ID) || plan.Context != "full-input-utf8/r2" || plan.Scope != qualSemanticScope || !hex64.MatchString(plan.InventorySHA256) || len(plan.Targets) > 2 {
		return fail(KindInvalidInput, "SEMANTICS_PLAN_INVALID", "semantic-plan", nil)
	}
	inputs := map[string]QualificationQuerySemanticsInput{}
	for _, input := range req.Targets {
		if !validID(input.ID) {
			return fail(KindInvalidInput, "SEMANTICS_INPUT_ID", "query-semantics", nil)
		}
		if _, ok := inputs[input.ID]; ok {
			return fail(KindInvalidInput, "SEMANTICS_INPUT_DUPLICATE", input.ID, nil)
		}
		inputs[input.ID] = input
	}
	ids, platforms := map[string]bool{}, map[string]bool{}
	for _, p := range plan.Targets {
		if e := r.eligibility.charge(1, 0, "semantic-plan-target"); e != nil {
			return e
		}
		idx := slices.IndexFunc(axis.Targets, func(row QualQuerySemanticsTarget) bool { return row.Platform == p.Platform })
		if idx < 0 || platforms[p.Platform] || !validID(p.ID) || ids[p.ID] || !validID(p.Route) || !validID(p.Role) || !validID(p.Set) || !validID(p.ProbeID) || !hex64.MatchString(p.ProfileSHA256) || !hex64.MatchString(p.RegistrationSHA256) {
			return fail(KindInvalidInput, "SEMANTICS_PLAN_TARGET_INVALID", "semantic-plan", nil)
		}
		ids[p.ID], platforms[p.Platform] = true, true
		queries := map[string]bool{}
		for _, query := range p.Queries {
			if !validID(query.ID) || queries[query.ID] || !hex64.MatchString(query.SHA256) {
				return fail(KindInvalidInput, "SEMANTICS_PLAN_QUERY_INVALID", p.ID, nil)
			}
			queries[query.ID] = true
		}
		tuples := map[qualSemanticTuple]bool{}
		for _, tuple := range p.Required {
			if e := r.eligibility.charge(1, 0, "semantic-plan-tuple"); e != nil {
				return e
			}
			if !validID(tuple.Case) || !queries[tuple.Query] || !slices.Contains([]string{"incremental", "fresh"}, tuple.TreeRole) || tuples[tuple] {
				return fail(KindInvalidInput, "SEMANTICS_PLAN_TUPLE_INVALID", p.ID, nil)
			}
			tuples[tuple] = true
		}
		row := &axis.Targets[idx]
		row.ID, row.Route, row.Role, row.Set, row.ProbeID = p.ID, p.Route, p.Role, p.Set, p.ProbeID
		row.ExpectedProfileSHA256, row.ExpectedRegistrationSHA256 = &p.ProfileSHA256, &p.RegistrationSHA256
		row.State, row.Code = "NOT_RUN", ""
		if len(p.Required) > 0 && len(p.Queries) > 0 {
			row.Tuple = &QueryEligibilityCounts{Required: uint64(len(p.Required)), Unprocessed: uint64(len(p.Required))}
		}
		t := &qualSemanticTargetRun{owner: s, plan: p, row: row, host: q.hosts[p.Platform], input: inputs[p.ID]}
		s.targets = append(s.targets, t)
		if t.host != nil && (p.Platform == "windows-amd64" && t.host.platform.pair() != "windows/amd64" || p.Platform == "linux-amd64" && t.host.platform.pair() != "linux/amd64") {
			t.failure("SEMANTICS_PLATFORM_PAIR_MISMATCH")
		}
		if plan.InventorySHA256 != q.inv.SHA256 || !q.semanticWorkload(p) {
			t.failure("SEMANTICS_PLAN_INVENTORY_MISMATCH")
		}
	}
	for id := range inputs {
		if !ids[id] {
			return fail(KindInvalidInput, "SEMANTICS_INPUT_UNREGISTERED", id, nil)
		}
	}
	for _, t := range s.targets {
		if e := r.check(); e != nil {
			return e
		}
		if t.row.Tuple == nil {
			t.row.State, t.row.Code = "UNREGISTERED", "SEMANTICS_TARGET_UNREGISTERED"
			continue
		}
		if t.row.Assessment == AssessFail {
			t.finishCounts()
			continue
		}
		if t.host == nil || t.host.id == nil || len(t.host.rejected) > 0 {
			t.row.State, t.row.Code = "INPUT_MISSING", "SEMANTICS_HOST_MISSING"
			t.finishCounts()
			continue
		}
		if t.input.Replay.Profile == nil || t.input.Replay.Root == "" {
			t.row.State, t.row.Code = "INPUT_MISSING", "SEMANTICS_INPUT_MISSING"
			t.finishCounts()
			continue
		}
		if e := s.target(t); e != nil {
			return e
		}
	}
	axis.ProcessingComplete = true
	axis.Assessment = AssessPass
	for _, row := range axis.Targets {
		if row.Assessment == AssessFail {
			axis.Assessment = AssessFail
			break
		}
		if row.Assessment != AssessPass {
			axis.Assessment = AssessBlocked
		}
	}
	return nil
}

func (q *qualifier) semanticWorkload(p qualSemanticPlanTarget) bool {
	if p.Role == "primary" {
		return slices.ContainsFunc(q.inv.Routes, func(r QualRoute) bool {
			return r.Route == p.Route && r.Workload.Set == p.Set && r.Workload.Route == p.Route
		})
	}
	for _, role := range q.inv.ExtraRoles {
		if role.ID == p.Role && role.Status == "EXECUTED" && slices.Contains(role.Platforms, p.Platform) {
			return slices.ContainsFunc(role.Workloads, func(w QualWorkload) bool { return w.Set == p.Set && w.Route == p.Route })
		}
	}
	return false
}

func (s *qualSemanticRun) target(t *qualSemanticTargetRun) (resultErr *Error) {
	start := s.r.totalRead
	if t.host.x.r.totalRead > s.q.lim.TotalBytes {
		return fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", t.plan.ID, nil)
	}
	left := s.q.lim.TotalBytes - t.host.x.r.totalRead
	if left > s.lim.TotalBytes-s.r.totalRead {
		left = s.lim.TotalBytes - s.r.totalRead
	}
	s.r.readCeiling = s.r.totalRead + left
	defer func() {
		delta := s.r.totalRead - start
		t.row.BytesRead = delta
		if e := t.host.x.r.chargeRead(delta, s.q.lim.TotalBytes, t.plan.ID); e != nil && resultErr == nil {
			resultErr = e
		}
		if resultErr != nil {
			t.row.State = "INTERRUPTED"
			if t.row.Code == "" {
				t.row.Code = resultErr.Code
			}
		}
		t.finishCounts()
		s.r.readCeiling = s.lim.TotalBytes
	}()
	if e := s.document("semantic-replay-profile", t.input.Replay.Profile); e != nil {
		return e
	}
	t.row.Digests.Profile = semanticDigest(t.input.Replay.Profile)
	profile, e := parseQualificationReplay(t.input.Replay.Profile, s.r)
	if e != nil {
		return e
	}
	t.profile = &profile
	id := t.host.id
	attempt, err := strconv.ParseUint(id.RunAttempt, 10, 64)
	if err != nil || attempt == 0 {
		t.failure("SEMANTICS_HOST_ATTEMPT_INVALID")
	}
	if profile.Subject.Commit != s.q.candidate || profile.Subject.Run != id.RunID || profile.Subject.Attempt != attempt || profile.Subject.Platform != t.host.platform.pair() || profile.Subject.EvidenceMode != ModeNewRun {
		t.failure("SEMANTICS_SUBJECT_CANDIDATE_MISMATCH")
	}
	for _, doc := range []struct {
		name string
		data []byte
		pin  **string
	}{{"semantic-approval", t.input.EligibilityApproval, &t.row.Digests.Approval}, {"semantic-context", t.input.Context, &t.row.Digests.Context}} {
		if doc.data == nil {
			continue
		}
		if e := s.document(doc.name, doc.data); e != nil {
			return e
		}
		*doc.pin = semanticDigest(doc.data)
	}
	if t.input.Context != nil {
		if e := s.r.eligibility.decode("semantic-context", t.input.Context, &t.context, true); e != nil {
			return e
		}
		c := t.context
		if c.Schema != QualificationSemanticsContextSchema || !hex64.MatchString(c.PlanSHA256) || !validID(c.TargetID) || !portable(c.WorkflowPath, false) || !hex40.MatchString(c.WorkflowRevision) || !eligibilityString(c.ArtifactID) || !hex64.MatchString(c.ArtifactSHA256) || !hex64.MatchString(c.GraphSHA256) {
			return fail(KindInvalidInput, "SEMANTICS_CONTEXT_INVALID", t.plan.ID, nil)
		}
		if c.PlanSHA256 != s.planSHA || c.TargetID != t.plan.ID || c.Repository != id.Repository || c.Workflow != id.Workflow || c.RunID != id.RunID || c.Attempt != attempt || c.Event != id.Event || c.HeadSHA != id.HeadSHA || c.Checkout != s.q.candidate || c.Checkout != id.Checkout {
			t.failure("SEMANTICS_CONTEXT_COHORT_MISMATCH")
		}
	}
	if t.row.Assessment == AssessFail {
		return nil
	}
	if t.input.EligibilityApproval == nil || t.input.Context == nil {
		t.row.State, t.row.Code = "INPUT_MISSING", "SEMANTICS_CALLER_DOCUMENT_MISSING"
		return nil
	}
	if e := t.checkApproval(); e != nil {
		return e
	}
	if t.row.Assessment == AssessFail {
		return nil
	}
	if t.blocked {
		t.row.State = "NOT_ELIGIBLE"
		return nil
	}
	if e := t.openRoot(); e != nil {
		return e
	}
	rr, replayErr := replayShared(s.r.caller, t.input.Replay, &replayBudget{semantic: t})
	t.replay = &rr
	t.row.Core = rr.QuerySemantics
	if t.row.Core != nil {
		if t.row.Core.Assessment == AssessFail {
			t.failure(t.row.Core.Code)
		}
		if t.registered && t.row.Core.Tuple.Required != t.row.Tuple.Required {
			t.failure("SEMANTICS_PLAN_DENOMINATOR_MISMATCH")
		} else if t.tuplesMatch {
			*t.row.Tuple = t.row.Core.Tuple
		}
	}
	if rr.Assessment == AssessFail && t.row.Assessment != AssessFail {
		code := "SEMANTICS_REPLAY_FAILED"
		for _, f := range rr.Findings {
			if f.Severity == "error" {
				code = f.Code
				break
			}
		}
		t.failure(code)
	}
	if replayErr != nil {
		return asError(replayErr)
	}
	if t.row.Assessment == AssessFail {
		return nil
	}
	t.row.State = "REPLAYED"
	core := t.row.Core
	if !t.blocked && core != nil && core.Complete && core.Assessment == AssessPass && core.Semantic == AssessPass && core.BindingValid && core.BaseVerified.Assessment == AssessPass && core.Eligibility != nil && core.Eligibility.Complete && core.Eligibility.Assessment == AssessPass && rr.EvidenceValid && rr.ExecutionStatus == StatusCompleted && t.row.Tuple.Verified == t.row.Tuple.Required && t.row.Tuple.Failed == 0 && t.row.Tuple.Blocked == 0 && t.row.Tuple.Unprocessed == 0 {
		t.row.Assessment, t.row.Code = AssessPass, ""
	} else {
		t.row.Assessment = AssessBlocked
		if t.row.Code == "" {
			t.row.Code = "SEMANTICS_PROBE_INCOMPLETE"
		}
	}
	return nil
}

func parseQualificationReplay(data []byte, r *run) (ReplayProfile, *Error) {
	if e := r.eligibility.charge(2*uint64(len(data))+1, 2*uint64(len(data))+256, "qualification-replay-selector"); e != nil {
		return ReplayProfile{}, e
	}
	selected, e := selectJSONRevision(data, r, "replay", "reducer", "oracle-set-r4", nil, "oracle-set-r5")
	if e != nil {
		return ReplayProfile{}, e
	}
	if selected != "oracle-set-r4" {
		return ReplayProfile{}, fail(KindUnsupported, "SEMANTICS_REDUCER_UNSUPPORTED", "replay", nil)
	}
	return parseSemanticReplayInRun(data, r)
}

func parseSemanticReplayInRun(data []byte, r *run) (ReplayProfile, *Error) {
	b := r.eligibility
	if e := b.reserveJSON("replay", data, 3); e != nil {
		return ReplayProfile{}, e
	}
	v, e := decodeBounded("replay", data, MaxDocumentBytes, &valueBudget{max: 2000000, check: r.check, charge: func() *Error { return b.charge(1, 0, "replay-profile") }})
	if e != nil {
		return ReplayProfile{}, e
	}
	return parseReplayValue(data, v, typed{doc: "replay", check: r.check})
}

func (t *qualSemanticTargetRun) checkApproval() *Error {
	var approval eligibilityApproval
	if e := t.owner.r.eligibility.decode("semantic-external-approval", t.input.EligibilityApproval, &approval, true); e != nil {
		return e
	}
	if approval.Schema != QueryReferenceEligibilityApprovalSchema {
		return fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "semantic-external-approval", nil)
	}
	c := approval.Candidate
	if c.SourceRevision != t.owner.q.candidate || c.Platform != t.host.platform.pair() || c.Run != t.context.RunID || c.Attempt != t.context.Attempt {
		t.failure("SEMANTICS_APPROVAL_CANDIDATE_MISMATCH")
	}
	var current *eligibilityCohort
	for i := range approval.Cohorts {
		if approval.Cohorts[i].Purpose != "current" {
			continue
		}
		if current != nil {
			return fail(KindInvalidInput, "SEMANTICS_APPROVAL_CURRENT_DUPLICATE", t.plan.ID, nil)
		}
		current = &approval.Cohorts[i]
	}
	if current == nil {
		t.blocked = true
		if t.row.Assessment != AssessFail {
			t.row.Code = "SEMANTICS_CURRENT_COHORT_MISSING"
		}
		return nil
	}
	if current.SourceRevision != c.SourceRevision || current.Platform != c.Platform || current.Run != c.Run || current.Attempt != c.Attempt {
		t.failure("SEMANTICS_APPROVAL_CURRENT_MISMATCH")
	}
	var provenance *eligibilityProvenance
	for i := range approval.Provenance {
		if approval.Provenance[i].ID != current.ProvenanceID {
			continue
		}
		if provenance != nil {
			return fail(KindInvalidInput, "SEMANTICS_PROVENANCE_DUPLICATE", t.plan.ID, nil)
		}
		provenance = &approval.Provenance[i]
	}
	if provenance == nil {
		t.blocked = true
		if t.row.Assessment != AssessFail {
			t.row.Code = "SEMANTICS_CURRENT_PROVENANCE_MISSING"
		}
		return nil
	}
	p := *provenance
	if p.SourceRevision != t.owner.q.candidate {
		t.failure("SEMANTICS_CURRENT_SOURCE_MISMATCH")
	}
	if p.Kind == "approved-local" {
		if p.Repository != "" || p.WorkflowPath != "" || p.WorkflowRevision != "" || p.RunID != "" || p.Attempt != 0 || p.ArtifactID != "" || p.ArtifactSHA256 != "" {
			t.failure("SEMANTICS_CURRENT_PROVENANCE_MISMATCH")
		}
		t.blocked = true
		if t.row.Assessment != AssessFail {
			t.row.Code = "SEMANTICS_CURRENT_CI_REQUIRED"
		}
		return nil
	}
	if p.Kind != "reviewed-ci" {
		return fail(KindInvalidInput, "SEMANTICS_PROVENANCE_KIND", t.plan.ID, nil)
	}
	x := t.context
	if p.Repository != x.Repository || p.WorkflowPath != x.WorkflowPath || p.WorkflowRevision != x.WorkflowRevision || p.RunID != x.RunID || p.Attempt != x.Attempt || p.ArtifactID != x.ArtifactID || p.ArtifactSHA256 != x.ArtifactSHA256 || p.ReviewedContextSHA256 != *t.row.Digests.Context {
		t.failure("SEMANTICS_CURRENT_PROVENANCE_MISMATCH")
	}
	return nil
}

func (t *qualSemanticTargetRun) registration(req QuerySemanticsRequest, profile OracleProfile, reg semanticRegistration, add func(string, string, string)) *Error {
	t.registered = true
	bad := func(code string) {
		t.failure(code)
		add(code, t.plan.ID, "qualification 사전 plan/caller 원문과 guarded 문서가 다르다")
	}
	if e := t.owner.r.eligibility.charge(uint64(len(req.Profile)+len(req.Registration)+len(req.EligibilityApproval))+1, 0, "qualification-semantic-documents"); e != nil {
		return e
	}
	if digestHex(req.Profile) != t.plan.ProfileSHA256 || digestHex(req.Registration) != t.plan.RegistrationSHA256 {
		bad("SEMANTICS_PLAN_DIGEST_MISMATCH")
	}
	if !bytes.Equal(req.EligibilityApproval, t.input.EligibilityApproval) {
		bad("SEMANTICS_CALLER_APPROVAL_MISMATCH")
	}
	if reg.Route != t.plan.Route || profile.Native.Route != t.plan.Route || reg.Platform != t.host.platform.pair() || reg.Context != "full-input-utf8/r2" {
		bad("SEMANTICS_PLAN_ROUTE_MISMATCH")
	}
	queries := map[string]string{}
	for _, q := range reg.Queries {
		if e := t.owner.r.eligibility.charge(uint64(len(q.Source))+1, 0, "qualification-semantic-query"); e != nil {
			return e
		}
		if queries[q.ID] != "" {
			bad("SEMANTICS_PLAN_QUERY_MISMATCH")
		}
		queries[q.ID] = digestHex([]byte(q.Source))
		if q.Metadata == nil || len(q.Metadata.Predicates) == 0 {
			bad("SEMANTICS_PREDICATE_PROBE_REQUIRED")
		}
	}
	if len(queries) != len(t.plan.Queries) {
		bad("SEMANTICS_PLAN_QUERY_MISMATCH")
	}
	for _, q := range t.plan.Queries {
		if queries[q.ID] != q.SHA256 {
			bad("SEMANTICS_PLAN_QUERY_MISMATCH")
		}
	}
	tuples := map[qualSemanticTuple]bool{}
	for _, tuple := range reg.Required {
		tuples[qualSemanticTuple{Case: reg.Case, Query: tuple.Query, Step: tuple.Step, TreeRole: tuple.TreeRole}] = true
	}
	t.tuplesMatch = len(tuples) == len(t.plan.Required)
	if !t.tuplesMatch {
		bad("SEMANTICS_PLAN_DENOMINATOR_MISMATCH")
	}
	for _, tuple := range t.plan.Required {
		if !tuples[tuple] {
			t.tuplesMatch = false
			bad("SEMANTICS_PLAN_TUPLE_MISMATCH")
		}
	}
	return nil
}

func (t *qualSemanticTargetRun) snapshot(snapshot *evidenceSnapshot, binding semanticRunBinding, session *eligibilitySession, current *QuerySemanticsResult, add func(string, string, string)) *Error {
	if current.Assessment == AssessFail && current.Code != "" {
		t.failure(current.Code)
	}
	bad := func(code string) {
		t.failure(code)
		add(code, t.plan.ID, "현재 host/caller context와 guarded Oracle 근거가 다르다")
	}
	if snapshot == nil || session == nil {
		return fail(KindInvalidInput, "SEMANTICS_SNAPSHOT_MISSING", t.plan.ID, nil)
	}
	if t.context.GraphSHA256 != snapshot.verifiedMembers["evidence.json"].SHA256 || t.context.GraphSHA256 != session.reg.Graph.SHA256 {
		bad("SEMANTICS_CONTEXT_GRAPH_MISMATCH")
	}
	found := false
	for _, node := range snapshot.nodes {
		if e := t.owner.r.eligibility.charge(1, 0, "qualification-semantic-node"); e != nil {
			return e
		}
		if node.ID != binding.OracleNode {
			continue
		}
		found = true
		if node.Identities["source"] != t.owner.q.candidate || node.Identities["source"] != session.approval.Candidate.SourceRevision {
			bad("SEMANTICS_ORACLE_SOURCE_MISMATCH")
		}
		if node.Identities["run"] != t.context.RunID || node.Identities["attempt"] != strconv.FormatUint(t.context.Attempt, 10) || node.Identities["platform"] != t.host.platform.pair() || node.EvidenceMode != ModeNewRun {
			bad("SEMANTICS_ORACLE_COHORT_MISMATCH")
		}
	}
	if !found {
		bad("SEMANTICS_ORACLE_NODE_MISSING")
	}
	return nil
}

func semanticPathOverlap(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && (rel == "." || rel != ".." && !bytes.HasPrefix([]byte(rel), []byte(".."+string(filepath.Separator))))
}

func (t *qualSemanticTargetRun) openRoot() *Error {
	root, resolved, e := openRoot(t.input.Replay.Root)
	if e != nil {
		return e
	}
	t.owner.roots = append(t.owner.roots, root)
	t.root = root
	t.rootInfo, e = openedRootInfo(root, t.plan.ID)
	if e != nil {
		return e
	}
	if expected := t.input.ExpectedRootIdentity; expected != nil && !os.SameFile(expected, t.rootInfo) {
		return fail(KindIO, "SOURCE_CHANGED", t.plan.ID, nil)
	}
	t.blockedRoots = append(t.blockedRoots, t.rootInfo)
	check := func(other *os.Root, dirs map[string]fs.FileInfo) *Error {
		if e := t.owner.r.eligibility.charge(1, 256, "semantic-root-admission"); e != nil {
			return e
		}
		info, e := openedRootInfo(other, t.plan.ID)
		if e != nil {
			return e
		}
		t.blockedRoots = append(t.blockedRoots, info)
		if os.SameFile(t.rootInfo, info) || semanticPathOverlap(resolved, other.Name()) || semanticPathOverlap(other.Name(), resolved) {
			return fail(KindInvalidInput, "SEMANTIC_ROOT_OVERLAP", t.plan.ID, nil)
		}
		for name := range dirs {
			if e := t.owner.r.eligibility.charge(128, 128*uint64(len(name)+1), "semantic-root-admission"); e != nil {
				return e
			}
			child, err := other.OpenRoot(name)
			if err != nil {
				return fail(KindIO, "ROOT_UNREADABLE", t.plan.ID, err)
			}
			childInfo, e := openedRootInfo(child, t.plan.ID)
			closeErr := child.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return fail(KindIO, "ROOT_CLOSE_FAILED", t.plan.ID, closeErr)
			}
			if !os.SameFile(dirs[name], childInfo) {
				return fail(KindIO, "SOURCE_CHANGED", name, nil)
			}
			t.blockedRoots = append(t.blockedRoots, childInfo)
			if os.SameFile(t.rootInfo, childInfo) {
				return fail(KindInvalidInput, "SEMANTIC_ROOT_OVERLAP", t.plan.ID, nil)
			}
		}
		return nil
	}
	for _, h := range t.owner.q.hosts {
		if e := check(h.x.g.root, h.x.directories); e != nil {
			return e
		}
	}
	for _, prior := range t.owner.targets {
		if prior == t {
			break
		}
		if prior.root != nil {
			if e := check(prior.root, prior.directories); e != nil {
				return e
			}
		}
	}
	return t.owner.r.semanticEntries.begin(root, &t.host.entries, t.owner.q.lim.Files)
}

func (t *qualSemanticTargetRun) directory(root *os.Root, name string) *Error {
	child, err := root.OpenRoot(name)
	if err != nil {
		return fail(KindIO, "ROOT_UNREADABLE", name, err)
	}
	info, e := openedRootInfo(child, name)
	closeErr := child.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return fail(KindIO, "ROOT_CLOSE_FAILED", name, closeErr)
	}
	for _, blocked := range t.blockedRoots {
		if e := t.owner.r.eligibility.charge(1, 0, "semantic-root-comparison"); e != nil {
			return e
		}
		if os.SameFile(info, blocked) {
			return fail(KindInvalidInput, "SEMANTIC_ROOT_OVERLAP", name, nil)
		}
	}
	if name != "." {
		if _, e := t.owner.r.semanticEntries.record(root, name, info); e != nil {
			return e
		}
	}
	t.blockedRoots = append(t.blockedRoots, info)
	return nil
}

func (q *qualifier) restoreQuerySemanticsStats() {
	if q.semantic == nil {
		return
	}
	s := q.semantic
	s.axis.BytesRead, s.axis.Work, s.axis.LogicalBytes = s.r.totalRead, s.r.eligibility.work, s.r.eligibility.logical
	s.axis.Entries = s.r.semanticEntries.used
}

func knownQualificationFailure(res QualificationResult, findings []Finding) bool {
	if res.Assessment == AssessFail || len(findings) > 0 {
		return true
	}
	for _, c := range res.Cells {
		if c.Status == CellFail {
			return true
		}
	}
	for _, r := range res.ExtraRoles {
		if r.Status == CellFail || r.PreprocessCST != nil && r.PreprocessCST.Result == AssessFail {
			return true
		}
	}
	return false
}

func (q *qualifier) restoreQuerySemantics(res *QualificationResult) {
	q.restoreQuerySemanticsStats()
	if q.semantic != nil {
		for _, t := range q.semantic.targets {
			t.finishCounts()
		}
	}
	if res.Baseline != nil {
		baseline := *res
		baseline.Assessment = res.Baseline.Assessment
		if knownQualificationFailure(baseline, q.findings) {
			res.Baseline.Assessment = AssessFail
		}
		for _, h := range q.hosts {
			if len(h.rejected) != 0 {
				res.Baseline.Assessment = AssessFail
				for _, finding := range h.rejected {
					if len(res.Findings) < 1000 {
						res.Findings = append(res.Findings, finding)
					}
				}
			}
		}
		if res.Baseline.Assessment == AssessFail {
			res.Assessment = AssessFail
		}
	}
}

func (q *qualifier) foldQuerySemantics(res *QualificationResult) {
	q.restoreQuerySemanticsStats()
	claims := map[string]string{}
	for p, v := range res.PlatformClaims {
		claims[p] = v
	}
	res.Baseline = &QualBaselineSummary{Assessment: res.Assessment, Completeness: res.Completeness, MechanismGate: res.MechanismGate, SupportClaim: res.SupportClaim, PlatformClaims: claims}
	res.Assessment = AssessBlocked
	if res.Baseline.Assessment == AssessFail || res.QuerySemantics.Assessment == AssessFail {
		res.Assessment = AssessFail
	}
	res.SupportClaim = "BLOCKED"
	for p := range res.PlatformClaims {
		res.PlatformClaims[p] = "BLOCKED"
	}
	res.Explanation = append(res.Explanation, "small predicate probe 축은 부분 semantic scope다. Ordinary/SVC/large 전체 근거는 아직 등록되지 않았다")
}

func validQualificationSemantics(res QualificationResult, s *qualSemanticRun) bool {
	a := res.QuerySemantics
	if s == nil || res.ResultSchema != QualificationSemanticsResultSchema || a == nil || res.Baseline == nil || a.Scope != qualSemanticScope || a.FullScopeComplete || res.SupportClaim != "BLOCKED" || len(a.Targets) != 2 || !slices.Equal(a.MissingScopes, []string{"ordinary-query-workloads", "svc-query-workloads", "large-query-workloads"}) || a.PlanSHA256 == nil || *a.PlanSHA256 != s.planSHA {
		return false
	}
	if a.BytesRead > a.Limits.TotalBytes || a.Work > a.WorkLimit || a.LogicalBytes > a.LogicalBytesLimit || a.Entries > a.Limits.Files || res.BytesRead < a.BytesRead {
		return false
	}
	if a.Limits != s.lim || a.WorkLimit != eligibilityWorkLimit || a.LogicalBytesLimit != eligibilityLogicalLimit || a.BytesRead != s.r.totalRead || a.Work != s.r.eligibility.work || a.LogicalBytes != s.r.eligibility.logical || a.Entries != s.r.semanticEntries.used {
		return false
	}
	if len(res.PlatformClaims) != len(res.Baseline.PlatformClaims) {
		return false
	}
	for platform := range res.Baseline.PlatformClaims {
		if res.PlatformClaims[platform] != "BLOCKED" {
			return false
		}
	}
	seen, ids := map[string]bool{}, map[string]bool{}
	scope := AssessPass
	read := s.planBytes
	for _, row := range a.Targets {
		if !slices.Contains([]string{"windows-amd64", "linux-amd64"}, row.Platform) || seen[row.Platform] {
			return false
		}
		seen[row.Platform] = true
		var planned *qualSemanticPlanTarget
		var actual *qualSemanticTargetRun
		for _, t := range s.targets {
			if t.plan.Platform == row.Platform {
				planned = &t.plan
				actual = t
				break
			}
		}
		if planned != nil {
			if ids[row.ID] || row.ID != planned.ID || row.Route != planned.Route || row.Role != planned.Role || row.Set != planned.Set || row.ProbeID != planned.ProbeID || row.ExpectedProfileSHA256 == nil || *row.ExpectedProfileSHA256 != planned.ProfileSHA256 || row.ExpectedRegistrationSHA256 == nil || *row.ExpectedRegistrationSHA256 != planned.RegistrationSHA256 {
				return false
			}
			ids[row.ID] = true
			if len(planned.Required) > 0 && len(planned.Queries) > 0 && (row.Tuple == nil || row.Tuple.Required != uint64(len(planned.Required))) {
				return false
			}
		} else if row.ID != "" || row.Tuple != nil {
			return false
		}
		if row.BytesRead > ^uint64(0)-read {
			return false
		}
		read += row.BytesRead
		if row.Assessment == AssessFail {
			scope = AssessFail
		} else if scope != AssessFail && row.Assessment != AssessPass {
			scope = AssessBlocked
		}
		if row.Tuple == nil {
			if row.Assessment == AssessPass || row.State != "UNREGISTERED" || row.Core != nil {
				return false
			}
			continue
		}
		c := row.Tuple
		sum := uint64(0)
		for _, n := range []uint64{c.Verified, c.Failed, c.Blocked, c.Unprocessed} {
			if n > c.Required-sum {
				return false
			}
			sum += n
		}
		if sum != c.Required || c.Required == 0 || row.ExpectedProfileSHA256 == nil || row.ExpectedRegistrationSHA256 == nil {
			return false
		}
		if row.Core != nil && row.Core.Assessment == AssessFail && row.Assessment != AssessFail {
			return false
		}
		if core := row.Core; core != nil {
			if actual == nil || actual.replay == nil {
				return false
			}
			replay := *actual.replay
			replay.QuerySemantics = core
			if !validSemanticReplayResult(replay) {
				return false
			}
			if core.Complete != (core.ExecutionStatus == StatusCompleted) || !validSemanticCompletion(core) {
				return false
			}
			sum := uint64(0)
			for _, n := range []uint64{core.Tuple.Verified, core.Tuple.Failed, core.Tuple.Blocked, core.Tuple.Unprocessed} {
				if n > core.Tuple.Required-sum {
					return false
				}
				sum += n
			}
			if sum != core.Tuple.Required {
				return false
			}
		}
		if row.Assessment == AssessPass {
			if row.Core == nil || row.Core.ExecutionStatus != StatusCompleted || row.Core.BaseVerified.ExecutionStatus != StatusCompleted || row.Core.Eligibility == nil || row.Core.Eligibility.ExecutionStatus != StatusCompleted {
				return false
			}
			if row.Core != nil && (row.Core.ProfileSHA256 != *row.ExpectedProfileSHA256 || row.Core.RegistrationSHA256 != *row.ExpectedRegistrationSHA256) {
				return false
			}
			if row.State != "REPLAYED" || row.Core == nil || row.Core.Tuple != *row.Tuple || !row.Core.Complete || row.Core.Assessment != AssessPass || !row.Core.BindingValid || row.Core.BaseVerified.Assessment != AssessPass || row.Core.Eligibility == nil || !row.Core.Eligibility.Complete || row.Core.Eligibility.Assessment != AssessPass || row.Core.Semantic != AssessPass || c.Verified != c.Required || c.Failed != 0 || c.Blocked != 0 || c.Unprocessed != 0 || row.Digests.Profile == nil || row.Digests.Approval == nil || row.Digests.Context == nil {
				return false
			}
		}
	}
	if a.Assessment != scope || read != a.BytesRead || scope == AssessFail && a.Code == "" {
		return false
	}
	if res.ExecutionStatus == StatusCompleted {
		want := AssessBlocked
		if res.Baseline.Assessment == AssessFail || a.Assessment == AssessFail {
			want = AssessFail
		}
		if res.Assessment != want || !a.ProcessingComplete {
			return false
		}
	}
	return true
}
