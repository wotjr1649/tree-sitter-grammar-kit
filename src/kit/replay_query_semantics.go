package kit

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func replayQuerySemantics(x *replayEnv) (resultErr *Error) {
	var req QuerySemanticsRequest
	for _, input := range []struct {
		role string
		data *[]byte
	}{
		{"workload-profile", &req.Profile}, {"semantics-registration", &req.Registration}, {"semantics-run-binding", &req.RunBinding},
		{"eligibility-approval", &req.EligibilityApproval}, {"eligibility-registration", &req.EligibilityRegistration},
		{"query-registration", &req.WitnessRegistration}, {"evidence-policy", &req.Evidence.Policy},
	} {
		member, e := x.oneMember(input.role)
		if e != nil {
			return e
		}
		if strings.HasPrefix(strings.ToLower(member.Path), "evidence/") {
			return fail(KindInvalidInput, "MEMBER_TRUST_DOCUMENT_INSIDE", member.Path, nil)
		}
		data, e := x.member(member.Path)
		if e != nil {
			return e
		}
		*input.data = data
	}
	scope := &semanticReplayScope{expectedContext: "full-input-utf8/r2", documentsRead: true, pins: map[string]ReplayMember{}, walked: map[string]fs.FileInfo{}, walkedDirectories: map[string]fs.FileInfo{}}
	if x.prof.Reducer == "oracle-set-r5" {
		scope.expectedContext = semanticPagedContext
	}
	for _, member := range x.prof.Members {
		inside := strings.HasPrefix(member.Path, "evidence/")
		if strings.HasPrefix(strings.ToLower(member.Path), "evidence/") && !inside || (member.Role == "semantics-evidence") != inside {
			return fail(KindInvalidInput, "MEMBER_EVIDENCE_SCOPE", member.Path, nil)
		}
		if inside {
			member.Path = strings.TrimPrefix(member.Path, "evidence/")
			scope.pins[member.Path] = member
		}
	}
	before, e := x.g.lstat("evidence")
	if e != nil {
		return e
	}
	if before == nil || kind(before) != "dir" {
		return fail(KindInvalidInput, "MEMBER_EVIDENCE_ROOT_INVALID", "evidence", nil)
	}
	if original := x.directories["evidence"]; original == nil || !os.SameFile(original, before) {
		return fail(KindIO, "SOURCE_CHANGED", "evidence", nil)
	}
	root, err := x.g.root.OpenRoot("evidence")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			if resultErr == nil {
				resultErr = fail(KindIO, "ROOT_CLOSE_FAILED", "evidence", err)
			} else {
				resultErr.Cause = errors.Join(resultErr.Cause, err)
			}
		}
	}()
	opened, err := root.Stat(".")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	after, e := x.g.lstat("evidence")
	if e != nil {
		return e
	}
	if after == nil || kind(after) != "dir" || !os.SameFile(before, opened) || !os.SameFile(before, after) {
		return fail(KindIO, "SOURCE_CHANGED", "evidence", nil)
	}
	scope.root = root
	for path, info := range x.files {
		if local, ok := strings.CutPrefix(path, "evidence/"); ok {
			scope.walked[local] = info
		}
	}
	for path, info := range x.directories {
		if local, ok := strings.CutPrefix(path, "evidence/"); ok {
			scope.walkedDirectories[local] = info
		}
	}
	res, err := verifyQuerySemanticsInRun(x.r.caller, req, x.r, scope)
	x.querySemantics, x.recomp, x.recorded = &res, Verdict{res.ExecutionStatus, res.Assessment}, res.ProducerRecorded
	x.actual = map[string]string{"workload": res.ProfileSHA256, "semantics-registration": res.RegistrationSHA256, "semantics-run-binding": res.RunBindingSHA256, "evidence-policy": digestHex(req.Evidence.Policy)}
	if scope.snapshot != nil {
		for path, pin := range scope.snapshot.verifiedMembers {
			full := "evidence/" + path
			outer, ok := x.members[full]
			outer.Path = path
			if ok && sameMemberBytes(outer, pin) {
				x.verified[full], x.consumed[full] = true, true
			} else {
				x.finding("SEMANTICS_OUTER_MEMBER", full, "nested graph member가 외부 replay 등록과 다르다")
			}
		}
		if binding := scope.binding; binding != nil {
			for _, node := range scope.snapshot.nodes {
				if node.ID != binding.OracleNode {
					continue
				}
				if x.prof.Subject.Run != node.Identities["run"] || strconv.FormatUint(x.prof.Subject.Attempt, 10) != node.Identities["attempt"] || x.prof.Subject.Platform != node.Identities["platform"] || x.prof.Subject.EvidenceMode != node.EvidenceMode {
					x.finding("SEMANTICS_SUBJECT_MISMATCH", node.ID, "subject와 Oracle producer identity가 다르다")
				}
			}
		}

	}
	if err != nil {
		return asError(err)
	}
	if x.prof.Records != nil && !slices.Equal(x.prof.Records, []string{scope.caseID}) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "등록 record 목록이 semantic workload와 다르다")
	}

	x.cons = Consumption{Expected: 1, Consumed: 1}
	gate := x.gate("query-semantics")
	switch res.Assessment {
	case AssessPass:
		gate.pass()
	case AssessFail:
		gate.fail(scope.caseID, res.Code, "producer/base/semantic 검증이 실패했다")
	default:
		gate.recorded()
	}
	return nil
}

func validSemanticReplayResult(rr ReplayResult) bool {
	if rr.Schema != ReportSchema || rr.ResultSchema != ReplaySemanticsResultSchema || !semanticReducer(rr.Reducer.ID) || (rr.Reducer.ID == "oracle-set-r4" && rr.Reducer.Revision != "r4") || (rr.Reducer.ID == "oracle-set-r5" && rr.Reducer.Revision != "r5") || rr.Operation != "evidence-replay" || rr.QueryReference != nil || len(rr.PreprocessCST) != 0 || !executionStatuses[rr.ExecutionStatus] || !assessmentValues[rr.Assessment] || !evidenceModes[rr.EvidenceMode] || len(rr.Gates) > 1 {
		return false
	}
	if rr.Assessment == AssessPass && (rr.ExecutionStatus != StatusCompleted || !rr.EvidenceValid || rr.EvidenceMode != ModeReplayedRaw) {
		return false
	}
	if rr.ExecutionStatus == StatusCompleted && !rr.EvidenceValid && rr.Assessment != AssessFail {
		return false
	}
	if rr.EvidenceValid && (rr.ExecutionStatus != StatusCompleted || slices.ContainsFunc(rr.Findings, func(f Finding) bool { return f.Severity == "error" })) {
		return false
	}
	p := rr.QuerySemantics
	recordedOnly := rr.ExecutionStatus == StatusCompleted && rr.EvidenceValid && rr.EvidenceMode == ModeRecordedNotRecomputed && rr.Assessment == AssessUnresolved && rr.Recomputed == (Verdict{StatusNotRun, AssessUnresolved})
	if !executionStatuses[rr.Subject.ExecutionStatus] || !assessmentValues[rr.Subject.Assessment] || !evidenceModes[rr.Subject.EvidenceMode] || rr.Subject.Run == "" || len(rr.Subject.Run) > 128 {
		return false
	}
	if p == nil {
		return len(rr.Gates) == 0 && (recordedOnly || !rr.EvidenceValid && rr.Assessment != AssessPass && rr.Recomputed == (Verdict{})) && rr.Consumption == (Consumption{})
	}
	if !executionStatuses[p.BaseVerified.ExecutionStatus] || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, p.BaseVerified.Assessment) {
		return false
	}
	if p.Schema != ReportSchema || p.ResultSchema != QuerySemanticsResultSchema || !executionStatuses[p.ExecutionStatus] || !assessmentValues[p.Assessment] || !evidenceModes[p.EvidenceMode] || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, p.Semantic) {
		return false
	}
	if p.ProducerFoldValid && (!executionStatuses[p.ProducerRecorded.ExecutionStatus] || !assessmentValues[p.ProducerRecorded.Assessment]) || p.ProducerRecorded.ExecutionStatus != "" && rr.Recorded != p.ProducerRecorded {
		return false
	}
	if p.ProducerFoldValid && p.ProducerRecorded == (Verdict{StatusCompleted, AssessPass}) || p.Assessment == AssessPass && (!p.ProducerFoldValid || p.ProducerRecorded != (Verdict{StatusCompleted, AssessBlocked})) {
		return false
	}
	if rr.ExecutionStatus == StatusCompleted && rr.EvidenceValid && p.ProducerFoldValid && (rr.Subject.EvidenceMode != ModeNewRun || (Verdict{rr.Subject.ExecutionStatus, rr.Subject.Assessment}) != p.ProducerRecorded) {
		return false
	}
	validReport := func(report Report) bool {
		if (p.Complete || report.ExecutionStatus == StatusCompleted) && report.Assessment != AssessFail && slices.ContainsFunc(report.Findings, func(f Finding) bool { return f.Severity == "error" }) {
			return false
		}
		return report.Schema == ReportSchema && executionStatuses[report.ExecutionStatus] && assessmentValues[report.Assessment] && evidenceModes[report.EvidenceMode] && (report.Assessment != AssessPass || report.ExecutionStatus == StatusCompleted)
	}
	if !validReport(p.Report) {
		return false
	}
	var eligibilityEvidence *EvidenceResult
	if p.Eligibility != nil {
		eligibilityEvidence = p.Eligibility.Evidence
	}
	for _, evidence := range []*EvidenceResult{p.Evidence, eligibilityEvidence} {
		if evidence != nil && (!validReport(evidence.Report) || evidence.ResultSchema != EvidenceResultSchema || evidence.Assessment == AssessPass && evidence.EvidenceMode != ModeNewRun || evidence.Assessment == AssessFail && p.Assessment != AssessFail) {
			return false
		}
	}
	if p.Eligibility != nil {
		if !validReport(p.Eligibility.Report) || p.Eligibility.ResultSchema != QueryReferenceEligibilityResultSchema {
			return false
		}
		if p.Eligibility.Assessment == AssessPass && (!p.Eligibility.Complete || p.Eligibility.Tuple.Required == 0 || p.Eligibility.Evidence == nil || p.Eligibility.Evidence.Assessment != AssessPass) {
			return false
		}
		if p.Evidence != nil && p.Eligibility.Evidence != nil && !reflect.DeepEqual(p.Evidence, p.Eligibility.Evidence) {
			return false
		}
	}
	if p.Assessment == AssessPass && (p.Evidence == nil || p.Evidence.Assessment != AssessPass || p.Evidence.ExecutionStatus != StatusCompleted || p.Evidence.EvidenceMode != ModeNewRun || p.Eligibility == nil || p.Eligibility.Evidence == nil || p.Eligibility.Evidence.Assessment != AssessPass) {
		return false
	}
	left := p.Tuple.Required
	for _, count := range []uint64{p.Tuple.Verified, p.Tuple.Failed, p.Tuple.Blocked, p.Tuple.Unprocessed} {
		if count > left {
			return false
		}
		left -= count
	}
	if left != 0 {
		return false
	}
	for _, pin := range []struct{ value, role string }{{p.ProfileSHA256, "workload"}, {p.RegistrationSHA256, "semantics-registration"}, {p.RunBindingSHA256, "semantics-run-binding"}} {
		if p.Tuple.Required > 0 && pin.value == "" || pin.value != "" && (!hex64.MatchString(pin.value) || pin.value != rr.Observed[pin.role]) {
			return false
		}
	}
	if p.Complete && (p.ExecutionStatus != StatusCompleted || p.Evidence == nil || p.Eligibility == nil || !executionStatuses[p.BaseVerified.ExecutionStatus] || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, p.BaseVerified.Assessment)) {
		return false
	}
	if p.Complete && (p.Tuple.Unprocessed != 0 || !p.ProducerFoldValid && p.Assessment != AssessFail) {
		return false
	}
	if rr.Assessment == AssessPass && (rr.ExecutionStatus != StatusCompleted || !rr.EvidenceValid || p.Assessment != AssessPass || rr.EvidenceMode != ModeReplayedRaw) {
		return false
	}
	if rr.ExecutionStatus == StatusCompleted && !recordedOnly && (rr.EvidenceValid && rr.Assessment != p.Assessment || !rr.EvidenceValid && rr.Assessment != AssessFail) {
		return false
	}
	if p.Complete && (p.Evidence.Schema != ReportSchema || p.Evidence.ResultSchema != EvidenceResultSchema || p.Evidence.ExecutionStatus != StatusCompleted || p.Eligibility.Schema != ReportSchema || p.Eligibility.ResultSchema != QueryReferenceEligibilityResultSchema || p.Eligibility.ExecutionStatus != StatusCompleted) {
		return false
	}
	if (p.Evidence != nil && p.Evidence.Assessment == AssessFail || p.Eligibility != nil && p.Eligibility.Assessment == AssessFail) && p.Assessment != AssessFail {
		return false
	}

	seenBaseGates := map[string]bool{}
	for _, gate := range p.Gates {
		finished := gate
		finished.finish()
		if gate.ID == "" || seenBaseGates[gate.ID] || gate.Recomputed < 0 || gate.NotRecomputed < 0 || gate.Failed < 0 || gate.Failed > gate.Recomputed || gate.Assessment != finished.Assessment || gate.EvidenceMode != finished.EvidenceMode {
			return false
		}
		seenBaseGates[gate.ID] = true
		if p.BaseVerified.Assessment == AssessPass && (gate.Recomputed == 0 || gate.NotRecomputed != 0 || gate.Assessment != AssessPass || gate.EvidenceMode != ModeReplayedRaw) {
			return false
		}
		if gate.Failed > 0 && (p.BaseVerified.Assessment != AssessFail || p.Assessment != AssessFail) {
			return false
		}
		if gate.Failed == 0 && (gate.Code != "" || gate.Record != "" || gate.Detail != "") {
			return false
		}
	}
	requiredBaseGates := []string{"case-binding", "case-status", "tree", "verdict", "incremental-equality", "incremental-route", "expectations", "query-equality", "api", "query-expectations", "fact-reproduction", "dynamic-sql"}
	if p.BaseVerified.Assessment == AssessPass {
		for _, id := range requiredBaseGates {
			if !seenBaseGates[id] {
				return false
			}
		}
	}
	if p.Assessment == AssessPass && (p.Code != "" || len(p.Gates) == 0) {
		return false
	}
	if p.BaseVerified.Assessment == AssessPass && p.BaseVerified.ExecutionStatus != StatusCompleted {
		return false
	}
	if p.Semantic == AssessPass && (p.Tuple.Required == 0 || p.Tuple.Verified != p.Tuple.Required || !p.BindingValid || p.Eligibility == nil || p.Eligibility.Assessment != AssessPass || !p.Eligibility.Complete || p.Eligibility.Tuple.Verified != p.Tuple.Required) {
		return false
	}
	if p.Semantic == AssessPass || p.Eligibility != nil && p.Eligibility.Assessment == AssessPass {
		if p.Eligibility.ExecutionStatus != StatusCompleted || p.Eligibility.EvidenceMode != ModeReplayedRaw {
			return false
		}
		for _, item := range []struct {
			count    QueryEligibilityCounts
			required uint64
		}{{p.Eligibility.Build, 9}, {p.Eligibility.Unit, 4}, {p.Eligibility.Control, 53}, {p.Eligibility.Assertion, 36}, {p.Eligibility.Tuple, p.Eligibility.Tuple.Required}} {
			if item.count != (QueryEligibilityCounts{Required: item.required, Verified: item.required}) {
				return false
			}
		}
	}
	if p.Eligibility != nil {
		if p.Eligibility.Build.Required != 9 || p.Eligibility.Unit.Required != 4 || p.Eligibility.Control.Required != 53 || p.Eligibility.Assertion.Required != 36 {
			return false
		}

		for _, counts := range []QueryEligibilityCounts{p.Eligibility.Build, p.Eligibility.Unit, p.Eligibility.Control, p.Eligibility.Assertion, p.Eligibility.Tuple} {
			left := counts.Required
			for _, count := range []uint64{counts.Verified, counts.Failed, counts.Blocked, counts.Unprocessed} {
				if count > left {
					return false
				}
				left -= count
			}
			if left != 0 || p.Eligibility.Complete && counts.Unprocessed != 0 {
				return false
			}
		}
	}

	if p.Assessment == AssessPass && (!p.Complete || p.Semantic != AssessPass || p.BaseVerified.Assessment != AssessPass || p.Tuple.Unprocessed != 0) {
		return false
	}
	if (p.Semantic == AssessFail || p.BaseVerified.Assessment == AssessFail || p.ProducerFoldValid && p.ProducerRecorded.Assessment == AssessFail || p.Tuple.Failed > 0) && p.Assessment != AssessFail {
		return false
	}
	if p.Assessment == AssessFail && (rr.Assessment != AssessFail || p.Code == "") {
		return false
	}
	if p.Assessment == AssessFail {
		findings := p.Findings
		if p.Eligibility != nil && p.Eligibility.Assessment == AssessFail {
			// Eligibility can retain warnings up to its cap before the first failure.
			findings = p.Eligibility.Findings
		}
		foundFailure := false
		// Findings beyond the retained prefix may only describe a later interruption.
		for _, finding := range findings[:min(len(findings), 1000)] {
			if finding.Severity == "error" {
				foundFailure = true
				if finding.Code == "" || p.Code != finding.Code {
					return false
				}
				break
			}
		}
		if !foundFailure && (p.Eligibility == nil || p.Eligibility.Assessment != AssessFail || len(findings) < 1000) {
			return false
		}
	}
	if len(rr.Gates) == 0 {
		return !p.Complete && !rr.EvidenceValid && rr.Assessment != AssessPass && rr.Recomputed == (Verdict{p.ExecutionStatus, p.Assessment}) && rr.Consumption == (Consumption{})
	}
	gate := rr.Gates[0]
	finished := gate
	finished.finish()
	if gate.ID != "query-semantics" || gate.Recomputed < 0 || gate.NotRecomputed < 0 || gate.Failed < 0 || gate.Failed > gate.Recomputed || gate.Recomputed+gate.NotRecomputed != 1 || gate.Assessment != finished.Assessment || gate.EvidenceMode != finished.EvidenceMode || !p.Complete || rr.Recorded != p.ProducerRecorded || !recordedOnly && rr.Recomputed != (Verdict{p.ExecutionStatus, p.Assessment}) || rr.Consumption != (Consumption{Expected: 1, Consumed: 1}) {
		return false
	}
	if gate.Failed == 0 && (gate.Code != "" || gate.Record != "" || gate.Detail != "") {
		return false
	}
	if gate.Failed > 0 && rr.EvidenceValid {
		return false
	}
	switch p.Assessment {
	case AssessPass:
		return gate.Recomputed == 1 && gate.Failed == 0
	case AssessFail:
		return gate.Failed == 1 && p.Code != "" && gate.Code == p.Code
	case AssessBlocked:
		return gate.NotRecomputed == 1 && gate.Failed == 0
	}
	return false
}

func decodeSemanticReplayResult(data []byte, r *run) (ReplayResult, *Error) {
	var result ReplayResult
	var b *eligibilityBudget
	var check func() *Error
	if r != nil {
		b = r.eligibility
		if b == nil {
			if r.storedSemantics == nil {
				r.storedSemantics = &eligibilityBudget{r: r}
			}
			b = r.storedSemantics
		}
		check = r.check
	}
	if e := b.reserveJSON("replay-result", data, 3); e != nil {
		return result, e
	}
	budget := &valueBudget{max: 2000000, check: check, charge: func() *Error { return b.charge(1, 0, "stored-semantic-result") }}
	v, e := decodeBounded("replay-result", data, MaxDocumentBytes, budget)
	if e != nil {
		return result, e
	}
	shape := &valueBudget{max: 2000000, check: check, charge: func() *Error { return b.charge(1, 0, "stored-semantic-result-shape") }, reserve: func(n uint64) *Error { return b.charge(0, n, "stored-semantic-result-shape") }}
	if e := eligibilityShape(typed{doc: "replay-result", check: check}, v, reflect.TypeFor[ReplayResult](), true, shape); e != nil {
		return result, e
	}
	if e := decodeTyped("replay-result", data, &result); e != nil {
		return result, e
	}
	return result, nil
}
