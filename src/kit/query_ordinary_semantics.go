package kit

import (
	"context"
	"errors"
	"slices"
)

const OrdinaryQuerySemanticsResultSchema = "tsgk-query-semantics-result/r2"

// Caller documents are immutable for the duration of this offline call.
type OrdinaryQuerySemanticsRequest struct {
	Evidence                EvidenceRequest
	Selection               OrdinaryQuerySemanticsBinding
	Profile                 []byte
	PlanningProfile         []byte
	Registration            []byte
	AssetRegistration       []byte
	PlanCommitment          []byte
	ShardProofs             [][]byte
	Registry                []byte
	RegistrySHA256          string
	QualificationInventory  []byte
	SlotInventory           []byte
	RunBinding              []byte
	ReferenceSelection      []byte
	EligibilityApproval     []byte
	EligibilityRegistration []byte
}

// The caller's selection fixes the denominator even when external approval is absent.
// It grants no eligibility authority until the independently pinned approval matches it.
type OrdinaryQuerySemanticsBinding = ordinaryEligibilityBinding

type OrdinaryQuerySemanticsTuple struct {
	ID    string `json:"obligation_id"`
	State string `json:"state"`
	Code  string `json:"code"`
}

type OrdinaryQuerySemanticsResult struct {
	Report
	ResultSchema       string                           `json:"result_schema"`
	BindingValid       bool                             `json:"binding_valid"`
	ProducerRecorded   Verdict                          `json:"producer_recorded"`
	BaseVerified       Verdict                          `json:"base_verified"`
	Semantic           string                           `json:"query_semantics"`
	Code               string                           `json:"code"`
	SelectedShardIDs   []string                         `json:"selected_shard_ids"`
	Tuple              QueryEligibilityCounts           `json:"tuple"`
	Obligations        []OrdinaryQuerySemanticsTuple    `json:"obligations"`
	Eligibility        *QueryReferenceEligibilityResult `json:"eligibility"`
	Gates              []ReplayGate                     `json:"gates"`
	BytesRead          uint64                           `json:"bytes_read"`
	Work               uint64                           `json:"work"`
	LogicalBytes       uint64                           `json:"logical_bytes"`
	ProcessingComplete bool                             `json:"processing_complete"`
	FullScopeComplete  bool                             `json:"full_scope_complete"`
	Complete           bool                             `json:"complete"`
}

func newOrdinarySemanticsResult() OrdinaryQuerySemanticsResult {
	res := OrdinaryQuerySemanticsResult{Report: newReport("ordinary query semantics"), ResultSchema: OrdinaryQuerySemanticsResultSchema,
		ProducerRecorded: Verdict{StatusNotRun, AssessBlocked}, BaseVerified: Verdict{StatusNotRun, AssessBlocked}, Semantic: AssessBlocked,
		SelectedShardIDs: []string{}, Obligations: []OrdinaryQuerySemanticsTuple{}, Gates: []ReplayGate{}}
	res.ExecutionStatus, res.EvidenceMode, res.Assessment = StatusCompleted, ModeReplayedRaw, AssessBlocked
	return res
}

func (res *OrdinaryQuerySemanticsResult) finding(code, path, message string) {
	res.Assessment = AssessFail
	if res.Code == "" {
		res.Code = code
	}
	if len(res.Findings) < 1000 {
		res.Findings = append(res.Findings, Finding{Severity: "error", Code: code, Path: path, Message: message})
	}
}

// VerifyOrdinaryQuerySemantics evaluates selected ordinary shards in one guarded pass.
// PASS remains conditional on the independent caller approval and expected assets.
func VerifyOrdinaryQuerySemantics(ctx context.Context, req OrdinaryQuerySemanticsRequest) (res OrdinaryQuerySemanticsResult, resultErr error) {
	res = newOrdinarySemanticsResult()
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		failReport(&res.Report, e)
		res.Code = e.Code
		return res, e
	}
	defer r.cancel()
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	defer func() {
		ordinaryResultError(&res, resultErr, r.totalRead, b.work, b.logical)
	}()
	// Eligibility owns the approval/registration/policy reads; the other caller bytes
	// are charged here, including every independently bounded inclusion proof.
	for _, doc := range []struct {
		name string
		data []byte
	}{{"profile", req.Profile}, {"planning-profile", req.PlanningProfile}, {"ordinary-registration", req.Registration},
		{"asset-registration", req.AssetRegistration}, {"plan-commitment", req.PlanCommitment}, {"registry", req.Registry},
		{"qualification-inventory", req.QualificationInventory}, {"slot-inventory", req.SlotInventory},
		{"run-binding", req.RunBinding}, {"reference-selection", req.ReferenceSelection}} {
		if e := ordinaryCallerRead(r, doc.name, doc.data); e != nil {
			return res, e
		}
	}
	if len(req.ShardProofs) > NativeOperations()["native-query-batch"].MaxCases {
		return res, fail(KindResourceLimit, "ORDINARY_PLAN_SELECTION_LIMIT", "shard-proofs", nil)
	}
	for _, proof := range req.ShardProofs {
		if e := ordinaryCallerRead(r, "shard-proof", proof); e != nil {
			return res, e
		}
	}
	binding := req.Selection
	if len(binding.ShardIDs) > NativeOperations()["native-query-batch"].MaxCases {
		return res, fail(KindResourceLimit, "ORDINARY_PLAN_SELECTION_LIMIT", "selection", nil)
	}
	var selectionBytes uint64
	for _, pin := range []OrdinaryPin{binding.Profile, binding.PlanningProfile, binding.Registration, binding.Assets, binding.Plan, binding.Commitment, binding.ReferenceSelection, binding.RunBinding} {
		selectionBytes += uint64(len(pin.Path)+len(pin.SHA256)) + 8
	}
	for _, value := range []string{binding.Schema, binding.Context, binding.ClosureEncoding, binding.Candidate, binding.SemanticCohort, binding.RunID} {
		selectionBytes += uint64(len(value))
	}
	for _, id := range binding.ShardIDs {
		selectionBytes += uint64(len(id)) + 8
	}
	if selectionBytes > MaxDocumentBytes {
		return res, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", "selection", nil)
	}
	if e := r.chargeRead(selectionBytes, ReplayOperations()["evidence-replay"].TotalBytes, "selection"); e != nil {
		return res, e
	}
	if e := b.charge(32*selectionBytes+1, 8*selectionBytes+2048, "ordinary-selection-binding"); e != nil {
		return res, e
	}
	in, e := b.ordinaryInputs(req.PlanningProfile, req.AssetRegistration, req.Registration, req.PlanCommitment,
		req.Registry, req.QualificationInventory, req.SlotInventory, req.RegistrySHA256,
		ordinaryPlanSelection{Binding: binding, Proofs: req.ShardProofs})
	if e != nil {
		return res, e
	}
	if e := b.charge(uint64(len(in.registration.Required))+1, 512*uint64(len(in.registration.Required)+len(binding.ShardIDs)), "ordinary-coordinator-selection"); e != nil {
		return res, e
	}
	res.SelectedShardIDs = slices.Clone(binding.ShardIDs)
	res.Tuple = QueryEligibilityCounts{Required: uint64(len(in.registration.Required)), Unprocessed: uint64(len(in.registration.Required))}
	if len(req.EligibilityApproval) == 0 {
		res.Tuple.Blocked, res.Tuple.Unprocessed = res.Tuple.Required, 0
		for _, shard := range in.plan.Shards {
			for _, id := range shard.ObligationIDs {
				res.Obligations = append(res.Obligations, OrdinaryQuerySemanticsTuple{ID: id, State: ordinaryTupleBlocked, Code: "ELIGIBILITY_APPROVAL_MISSING"})
			}
		}
		res.Code = "ELIGIBILITY_APPROVAL_MISSING"
		return res, nil
	}
	profile, e := b.oracleProfile(req.Profile)
	if e != nil {
		return res, e
	}
	if e := b.charge(uint64(len(req.Profile))+1, uint64(len(req.Profile)), "ordinary-executable-profile-pin"); e != nil {
		return res, e
	}
	if !ordinaryPinMatchesBytes(binding.Profile, req.Profile) {
		return res, fail(KindInvalidInput, "ORDINARY_PROFILE_PIN", "profile", nil)
	}
	root, _, e := openRoot(req.Evidence.Root)
	if e != nil {
		return res, e
	}
	defer func() {
		if err := root.Close(); err != nil {
			resultErr = errors.Join(resultErr, fail(KindIO, "ROOT_CLOSE_FAILED", "", err))
		}
	}()
	ordinary := &ordinaryEligibilityScope{Binding: binding, ClosureEncoding: binding.ClosureEncoding, TupleRequired: res.Tuple.Required}
	scope := &eligibilityScope{r: r, root: root, ordinary: ordinary}
	eligibility, err := verifyQueryReferenceEligibilityInRun(ctx, QueryReferenceEligibilityRequest{
		Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration}, scope)
	res.Eligibility = &eligibility
	ordinaryCopyEligibilityFindings(&res, scope.session)
	if err != nil {
		return res, err
	}
	s := scope.session
	if s == nil || s.snapshot == nil || !s.snapshot.graphBound {
		return res, fail(KindInvalidInput, "ORDINARY_READER_UNBOUND", "evidence", nil)
	}
	res.Eligibility = s.res
	manifest, ok := s.owned["oracle/manifest.json"]
	if !ok {
		return res, fail(KindInvalidInput, "ORDINARY_MANIFEST_BINDING", "oracle/manifest.json", nil)
	}
	manifestBytes, ok, e := s.member(&manifest)
	if e != nil {
		return res, e
	}
	if !ok {
		res.finding("ORDINARY_MANIFEST_BINDING", manifest.Member.Path, "승인된 manifest 원문이 없다")
		return res, nil
	}
	set, e := b.ordinaryGuardedSet(root, s, in.registration, profile, manifestBytes, req.RunBinding)
	if set != nil {
		res.ProducerRecorded = Verdict{set.Manifest.ExecutionStatus, set.Manifest.Assessment}
		for _, f := range set.Findings {
			res.finding(f.Code, f.Path, f.Message)
		}
	}
	if e != nil {
		return res, e
	}
	closure, e := b.ordinaryNativeClosure(root, s, in, profile, set, res.finding)
	if e != nil {
		return res, e
	}
	res.BindingValid = closure && s.bound && set.Complete
	if e := b.ordinaryAdjudicateSet(s, in, profile, set, req.ReferenceSelection, &res); e != nil {
		return res, e
	}
	ordinaryCopyEligibilityFindings(&res, s)
	if e := b.ordinaryFinish(root, s, &res); e != nil {
		return res, e
	}
	return res, nil
}

func ordinaryCallerRead(r *run, name string, data []byte) *Error {
	if uint64(len(data)) > MaxDocumentBytes {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", name, nil)
	}
	return r.chargeRead(uint64(len(data)), ReplayOperations()["evidence-replay"].TotalBytes, name)
}

func ordinaryCopyEligibilityFindings(res *OrdinaryQuerySemanticsResult, s *eligibilitySession) {
	if s == nil || s.res == nil || s.res.Assessment != AssessFail {
		return
	}
	if res.Assessment != AssessFail {
		code := s.firstFailure
		if code == "" {
			code = "ELIGIBILITY_FAILED"
		}
		res.finding(code, "eligibility", "외부 approval 결속에 실패했다")
	}
}

func (b *eligibilityBudget) ordinaryAdjudicateSet(s *eligibilitySession, in ordinaryVerifiedInputs, profile OracleProfile, set *ordinaryGuardedSet, referenceSelection []byte, res *OrdinaryQuerySemanticsResult) *Error {
	if s == nil || s.b != b || s.ordinary == nil || s.ordinary.session != s || set == nil || res == nil ||
		res.Tuple.Required != uint64(len(in.registration.Required)) || res.Tuple.Unprocessed != res.Tuple.Required {
		return fail(KindInvalidInput, "ORDINARY_COORDINATOR_UNBOUND", "ordinary", nil)
	}
	if e := b.charge(64*uint64(len(in.registration.Required)+len(set.Cases)+len(in.assets.Assets)+len(in.registration.Queries)+len(in.plan.Shards)+len(profile.Native.Cases)+1),
		2048*uint64(len(in.registration.Required)+len(set.Cases)+len(in.assets.Assets)), "ordinary-coordinator-index"); e != nil {
		return e
	}
	requiredByID := map[string]OrdinaryRequired{}
	for _, required := range in.registration.Required {
		requiredByID[required.ID] = required
	}
	ordered := make([]OrdinaryRequired, 0, len(in.registration.Required))
	for _, shard := range in.plan.Shards {
		for _, id := range shard.ObligationIDs {
			required, ok := requiredByID[id]
			if !ok {
				return fail(KindInvalidInput, "ORDINARY_RUNTIME_SELECTION", id, nil)
			}
			ordered = append(ordered, required)
			delete(requiredByID, id)
		}
	}
	if len(ordered) == 0 || len(requiredByID) != 0 {
		return fail(KindInvalidInput, "ORDINARY_RUNTIME_SELECTION", "ordinary", nil)
	}
	in.registration.Required = ordered
	sources := map[string][][]byte{}
	producerFacts := map[string][]byte{}
	for _, c := range in.registration.Cases {
		if cache := set.Cases[c.ID]; cache != nil {
			if len(cache.versions) == len(c.VersionPins) {
				sources[c.ID] = cache.versions
			}
			producerFacts[c.ID] = cache.stepFacts
		}
	}
	// Base contradictions must survive an interruption in later reference/asset reads.
	facts, factErr := b.ordinaryFactContext(s, set.Manifest, profile, producerFacts)
	bases := map[string]Verdict{}
	res.BaseVerified = Verdict{StatusCompleted, AssessPass}
	for i, c := range profile.Native.Cases {
		base := ordinaryBaseResult{Verdict: Verdict{StatusNotRun, AssessBlocked}}
		var e *Error
		if cache := set.Cases[c.ID]; cache != nil {
			base, e = b.recomputeOrdinaryBase(profile, i, cache, facts)
		}
		bases[c.ID] = base.Verdict
		res.Gates = append(res.Gates, base.Gates...)
		res.BaseVerified.Assessment = foldClaims([]string{res.BaseVerified.Assessment, base.Verdict.Assessment})
		for _, f := range base.Findings {
			res.finding(f.Code, f.Path, f.Message)
		}
		if base.Verdict.Assessment == AssessFail && res.Assessment != AssessFail {
			res.finding("ORDINARY_BASE_FAILED", c.ID, "raw base 판정의 재계산이 실패했다")
		}
		if e != nil {
			return e
		}
	}
	if factErr != nil {
		return factErr
	}
	references, e := s.ordinaryReferences(in.registration, referenceSelection, sources)
	ordinaryCopyEligibilityFindings(res, s)
	if e != nil {
		return e
	}
	assets := ordinaryAssetResolver{session: s, assets: map[string]OrdinaryAsset{}, resolved: map[string][]byte{}}
	for _, asset := range in.assets.Assets {
		assets.assets[asset.ID] = asset
	}
	queries := map[string]OrdinaryQuery{}
	cases := map[string]OrdinaryCase{}
	for _, q := range in.registration.Queries {
		queries[q.ID] = q
	}
	for _, c := range in.registration.Cases {
		cases[c.ID] = c
	}
	judgments := make([]ordinaryTupleJudgment, 0, len(in.registration.Required))
	selected := make([]string, 0, len(in.registration.Required))
	for _, required := range in.registration.Required {
		coord, c := required.Coordinate, cases[required.Coordinate.Case]
		cache := set.Cases[coord.Case]
		j := &ordinaryJudgmentBuilder{}
		var tupleErr *Error
		if cache == nil || !cache.ready || int(coord.Step) >= len(cache.checked) || int(coord.Step) >= len(c.VersionPins) {
			j.block("ORDINARY_NATIVE_UNAVAILABLE", "검증된 full case frame이 없다")
		} else {
			id := ordinaryTupleIdentity{Case: coord.Case, Query: coord.Query, Step: coord.Step, TreeRole: coord.TreeRole, Source: c.VersionPins[coord.Step]}
			actual := cache.checked[coord.Step].Incremental
			if coord.TreeRole == "fresh" {
				if cache.checked[coord.Step].Fresh == nil {
					j.block("ORDINARY_NATIVE_UNAVAILABLE", "fresh full tree가 없다")
				} else {
					actual = *cache.checked[coord.Step].Fresh
				}
			}
			var data [4][]byte
			for i, a := range []struct{ id, role string }{{queries[coord.Query].MetadataAsset, "metadata"}, {required.TreeAsset, "expected-tree"}, {required.CaptureAsset, "expected-captures"}, {required.FactAsset, "expected-facts"}} {
				if a.id == "" && i == 3 {
					continue
				}
				data[i], tupleErr = assets.resolve(a.id, a.role)
				if tupleErr != nil {
					break
				}
			}
			if tupleErr == nil {
				var judgment ordinaryTupleJudgment
				judgment, tupleErr = adjudicateOrdinaryTuple(b, id, actual, cache.versions[coord.Step], data[0], data[1], data[2], data[3], facts)
				ordinaryMergeJudgment(j, judgment)
				if tupleErr == nil {
					ref := references[ordinaryReferenceKey(coord.Case, coord.Step, coord.Query)]
					if !ref.Ready {
						j.block("ORDINARY_REFERENCE_UNAVAILABLE", "승인된 reference row가 없다")
					} else {
						judgment, tupleErr = b.ordinaryReferenceExpected(id, ref.Response, data[0], data[1], data[2])
						ordinaryMergeJudgment(j, judgment)
					}
				}
			}
		}
		if tupleErr != nil && tupleErr.Kind == KindInvalidInput {
			j.fail(tupleErr.Code, "승인된 독립 자산 또는 저장 tuple 입력이 유효하지 않다")
			tupleErr = nil
		}
		judgment := j.value()
		if judgment.State == ordinaryTupleVerified && (!res.BindingValid || !s.bound || !s.res.Complete || s.res.Assessment != AssessPass || bases[coord.Case].Assessment != AssessPass) {
			j.block("ORDINARY_PREREQUISITE_INCOMPLETE", "approval·native closure 또는 재계산 base가 완료되지 않았다")
			judgment = j.value()
		}
		// An interrupted tuple is unprocessed unless a contradiction was already proved.
		if tupleErr == nil || judgment.State == ordinaryTupleFailed {
			res.ordinaryRecordTuple(required.ID, judgment)
			judgments = append(judgments, judgment)
			selected = append(selected, required.ID)
		}
		if tupleErr != nil {
			return tupleErr
		}
	}
	coverage, e := b.foldOrdinaryRuntime(in.registration, selected, judgments)
	if e != nil {
		return e
	}
	res.Tuple, res.ProcessingComplete = coverage.Tuple, coverage.ProcessingComplete
	return nil
}

func ordinaryMergeJudgment(j *ordinaryJudgmentBuilder, value ordinaryTupleJudgment) {
	for _, f := range value.Findings {
		if f.Severity == "error" {
			j.fail(f.Code, f.Message)
		} else {
			j.block(f.Code, f.Message)
		}
	}
	if value.State == ordinaryTupleFailed && !j.failed {
		j.fail(value.Code, "ordinary tuple 검사 실패")
	} else if value.State == ordinaryTupleBlocked && !j.blocked {
		j.block(value.Code, "ordinary tuple 검사 미완료")
	}
}

func (res *OrdinaryQuerySemanticsResult) ordinaryRecordTuple(id string, judgment ordinaryTupleJudgment) {
	res.Obligations = append(res.Obligations, OrdinaryQuerySemanticsTuple{ID: id, State: judgment.State, Code: judgment.Code})
	res.Tuple.Unprocessed--
	switch judgment.State {
	case ordinaryTupleVerified:
		res.Tuple.Verified++
	case ordinaryTupleFailed:
		res.Tuple.Failed++
		res.finding(judgment.Code, id, "독립 ordinary 의미 검사가 실패했다")
	case ordinaryTupleBlocked:
		res.Tuple.Blocked++
	}
	for _, f := range judgment.Findings {
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Severity: f.Severity, Code: f.Code, Path: id, Message: f.Message})
		}
	}
}
