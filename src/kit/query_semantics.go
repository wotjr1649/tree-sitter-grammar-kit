package kit

import (
	"context"
	"errors"
	"io/fs"
	"os"
)

const QuerySemanticsResultSchema = "tsgk-query-semantics-result/r1"

var testHookSemanticProducerFold func(*eligibilityBudget)

// Caller documents stay outside the evidence graph and its producer-local pending leaf.
type QuerySemanticsRequest struct {
	Evidence                EvidenceRequest
	Profile                 []byte
	Registration            []byte
	RunBinding              []byte
	EligibilityApproval     []byte
	EligibilityRegistration []byte
	WitnessRegistration     []byte
}

// ProducerRecorded is immutable; BaseVerified and Semantic describe this offline evaluation.
type QuerySemanticsResult struct {
	failures uint64
	Report
	ResultSchema       string                           `json:"result_schema"`
	ProfileSHA256      string                           `json:"profile_sha256"`
	RegistrationSHA256 string                           `json:"registration_sha256"`
	RunBindingSHA256   string                           `json:"run_binding_sha256"`
	BindingValid       bool                             `json:"binding_valid"`
	ProducerRecorded   Verdict                          `json:"producer_recorded"`
	ProducerFoldValid  bool                             `json:"producer_fold_valid"`
	BaseVerified       Verdict                          `json:"base_verified"`
	Semantic           string                           `json:"query_semantics"`
	Code               string                           `json:"code"`
	Tuple              QueryEligibilityCounts           `json:"tuple"`
	Evidence           *EvidenceResult                  `json:"evidence"`
	Eligibility        *QueryReferenceEligibilityResult `json:"eligibility"`
	Gates              []ReplayGate                     `json:"gates"`
	BytesRead          uint64                           `json:"bytes_read"`
	Work               uint64                           `json:"work"`
	LogicalBytes       uint64                           `json:"logical_bytes"`
	Complete           bool                             `json:"complete"`
}

// VerifyQuerySemantics verifies the registered full-input UTF-8 workload offline.
// PASS is conditional on the caller's eligibility approval, not execution authenticity.
func VerifyQuerySemantics(ctx context.Context, req QuerySemanticsRequest) (res QuerySemanticsResult, resultErr error) {
	return verifyQuerySemanticsInRun(ctx, req, nil, nil)
}

type semanticReplayScope struct {
	expectedContext   string
	documentsRead     bool
	walkedDirectories map[string]fs.FileInfo
	caseID            string
	binding           *semanticRunBinding
	root              *os.Root
	pins              map[string]ReplayMember
	walked            map[string]fs.FileInfo
	snapshot          *evidenceSnapshot
}

func verifyQuerySemanticsInRun(ctx context.Context, req QuerySemanticsRequest, r *run, replay *semanticReplayScope) (res QuerySemanticsResult, resultErr error) {
	res = QuerySemanticsResult{Report: newReport("query semantics"), ResultSchema: QuerySemanticsResultSchema,
		Semantic: AssessBlocked, BaseVerified: Verdict{StatusNotRun, AssessBlocked}, Gates: []ReplayGate{}}
	res.ExecutionStatus, res.EvidenceMode, res.Assessment = StatusCompleted, ModeReplayedRaw, AssessBlocked
	var e *Error
	if r == nil {
		r, e = startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
		if e != nil {
			failReport(&res.Report, e)
			return res, e
		}
		defer r.cancel()
	}
	b := r.eligibility
	if b == nil {
		b = &eligibilityBudget{r: r}
		r.eligibility = b
	}
	defer func() {
		res.BytesRead, res.Work, res.LogicalBytes = r.totalRead, b.work, b.logical
		if resultErr != nil {
			known, code := res.Assessment == AssessFail, res.Code
			failReport(&res.Report, asError(resultErr))
			res.Complete = false
			if known {
				res.Assessment, res.Code = AssessFail, code
			} else if res.Code == "" {
				res.Code = asError(resultErr).Code
			}
		}
	}()
	add := func(code, at, message string) {
		res.failures++
		res.Assessment = AssessFail
		if res.Code == "" {
			res.Code = code
		}
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: "error", Code: code, Path: at, Message: message})
		}
	}
	if r.totalRead > ReplayOperations()["evidence-replay"].TotalBytes {
		return res, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", "", nil)
	}
	// Fix the denominator before consulting external eligibility or its ABSENT path.
	for _, doc := range []struct {
		name string
		data []byte
	}{{"oracle", req.Profile}, {"semantics-registration", req.Registration}, {"run-binding", req.RunBinding}} {
		if uint64(len(doc.data)) > MaxDocumentBytes {
			return res, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc.name, nil)
		}
		if replay != nil && replay.documentsRead {
			continue
		}
		if uint64(len(doc.data)) > ReplayOperations()["evidence-replay"].TotalBytes-r.totalRead {
			return res, fail(KindResourceLimit, "TOTAL_BYTES_LIMIT", doc.name, nil)
		}
		r.totalRead += uint64(len(doc.data))
	}
	profile, e := b.oracleProfile(req.Profile)
	if e != nil {
		return res, e
	}
	reg, e := b.semanticsRegistration(req.Registration, profile)
	if e != nil {
		return res, e
	}
	res.ProfileSHA256, res.RegistrationSHA256, res.RunBindingSHA256 = digestHex(req.Profile), digestHex(req.Registration), digestHex(req.RunBinding)
	if replay != nil {
		replay.caseID = reg.Case
		if replay.expectedContext != "" && replay.expectedContext != reg.Context {
			return res, fail(KindInvalidInput, "SEMANTICS_REDUCER_CONTEXT", "semantics-registration", nil)
		}
	}
	res.Tuple.Required = uint64(len(reg.Required))
	res.Tuple.Unprocessed = res.Tuple.Required
	var root *os.Root
	if replay != nil {
		root = replay.root
	}
	if root == nil {
		root, _, e = openRoot(req.Evidence.Root)
		if e != nil {
			return res, e
		}
		defer func() {
			if err := root.Close(); err != nil {
				resultErr = errors.Join(resultErr, fail(KindIO, "ROOT_CLOSE_FAILED", "", err))
			}
		}()
	}
	if info, e := newGuard(root).lstat("oracle"); e != nil || info == nil || kind(info) != "dir" {
		if e != nil {
			return res, e
		}
		return res, fail(KindInvalidInput, "SEMANTICS_ROOT_INVALID", "oracle", nil)
	}
	child, err := root.OpenRoot("oracle")
	if err != nil {
		return res, fail(KindIO, "ROOT_UNREADABLE", "oracle", err)
	}
	defer func() {
		if err := child.Close(); err != nil {
			resultErr = errors.Join(resultErr, fail(KindIO, "ROOT_CLOSE_FAILED", "oracle", err))
		}
	}()
	wanted := map[string]bool{}
	for _, member := range reg.PlannedMembers {
		wanted["oracle/"+member.Path] = true
	}
	cache := &semanticNativeCache{}
	scope := &eligibilityScope{r: r, root: root, wanted: wanted, native: cache}
	if replay != nil {
		scope.walked, scope.walkedDirectories = replay.walked, replay.walkedDirectories
		scope.documentsRead = replay.documentsRead
	}
	eligibility, eligibilityErr := verifyQueryReferenceEligibilityInRun(ctx, QueryReferenceEligibilityRequest{
		Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration}, scope)
	res.Eligibility = &eligibility
	if replay != nil && scope.session != nil {
		replay.snapshot = scope.session.snapshot
	}
	if eligibility.Assessment == AssessFail {
		res.Assessment = AssessFail
		if res.Code == "" {
			res.Code = scope.session.firstFailure
			if res.Code == "" {
				res.Code = "ELIGIBILITY_FAILED"
			}
		}
		for _, finding := range eligibility.Findings {
			if finding.Severity == "error" {
				add(finding.Code, finding.Path, finding.Message)
			}
		}
	}
	if eligibilityErr != nil {
		return res, eligibilityErr
	}
	snapshot := scope.session.snapshot
	if len(req.EligibilityApproval) == 0 {
		snapshot = &evidenceSnapshot{wanted: wanted}
		if replay != nil {
			snapshot.walked, snapshot.approved = replay.walked, replay.pins
			snapshot.walkedDirectories = replay.walkedDirectories
			replay.snapshot = snapshot
		}
		evidence, err := verifyEvidenceInRun(ctx, req.Evidence, snapshot, r, root)
		res.Evidence = &evidence
		if evidence.Assessment == AssessFail {
			add("SEMANTICS_EVIDENCE_FAILED", "evidence.json", "독립 policy에 대한 evidence 검증이 실패했다")
		}
		if err != nil {
			return res, err
		}
	} else {
		res.Evidence = eligibility.Evidence
	}
	manifest, files, state, e := b.semanticSnapshot(root, child, snapshot, reg, add)
	res.ProducerRecorded = Verdict{manifest.ExecutionStatus, manifest.Assessment}
	res.ProducerFoldValid = state.producerVerified
	if e != nil {
		return res, e
	}
	var binding semanticRunBinding
	if replay != nil {
		replay.binding = &binding
	}
	res.BindingValid, e = b.semanticRunBinding(&binding, req, profile, reg, manifest, files, snapshot, scope.session, state, add)
	if e != nil {
		return res, e
	}
	if state.local == nil || state.projection == nil {
		return res, fail(KindInvalidInput, "SEMANTICS_PROJECTION_MISSING", "oracle", nil)
	}
	if e := b.semanticNative(req, profile, reg, manifest, files, state, scope.session, cache, &res, add); e != nil {
		return res, e
	}
	if e := b.semanticBase(profile, files[state.local.Projection.Path], cache, &res, add); e != nil {
		return res, e
	}
	if e := b.semanticSnapshotIdentity(root, child, snapshot); e != nil {
		return res, e
	}
	if res.BindingValid && res.Tuple.Required > 0 && res.Tuple.Verified == res.Tuple.Required && eligibility.Complete && eligibility.Assessment == AssessPass {
		res.Semantic = AssessPass
	}
	if res.Tuple.Failed > 0 {
		res.Semantic = AssessFail
	}
	if res.Assessment != AssessFail {
		res.Assessment = foldClaims([]string{res.Semantic, res.BaseVerified.Assessment})
		if res.Assessment == AssessBlocked && res.Code == "" {
			res.Code = "SEMANTICS_INCOMPLETE"
		}
	}
	res.Complete = true
	if e := r.check(); e != nil {
		return res, e
	}
	if e := sealOutput(res, ReplayOperations()["evidence-replay"].OutputBytes); e != nil {
		return res, e
	}
	return res, nil
}
