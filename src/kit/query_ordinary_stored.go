package kit

import (
	"context"
	"slices"
	"strings"
)

const (
	ordinaryReplayReducerID = "oracle-set-r9"
)

// DecodeOrdinaryQuerySemanticsResult checks the stored result's schema and internal
// accounting. It does not recompute or authenticate any evidence described by the result.
func DecodeOrdinaryQuerySemanticsResult(data []byte) (OrdinaryQuerySemanticsResult, error) {
	var result OrdinaryQuerySemanticsResult
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	defer cancel()
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		return result, e
	}
	defer r.cancel()
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	if e := r.chargeRead(uint64(len(data)), ReplayOperations()["evidence-replay"].TotalBytes, "ordinary-query-semantics-result"); e != nil {
		return result, e
	}
	if e := b.decodeOrdinaryTyped("ordinary-query-semantics-result", data, &result); e != nil {
		return OrdinaryQuerySemanticsResult{}, e
	}
	if e := reserveOrdinaryStoredValidation(result, b); e != nil {
		return OrdinaryQuerySemanticsResult{}, e
	}
	if !validOrdinaryStoredResult(result) {
		return OrdinaryQuerySemanticsResult{}, fail(KindInvalidInput, "ORDINARY_RESULT_INVALID", "ordinary-query-semantics-result", nil)
	}
	return result, nil
}

func validOrdinaryStoredResult(p OrdinaryQuerySemanticsResult) bool {
	if p.Schema != ReportSchema || p.Command != "ordinary query semantics" || p.ResultSchema != OrdinaryQuerySemanticsResultSchema ||
		!executionStatuses[p.ExecutionStatus] || !evidenceModes[p.EvidenceMode] || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked, AssessNotAssessed}, p.Assessment) ||
		!slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, p.Semantic) || p.FullScopeComplete {
		return false
	}
	readLimit := ReplayOperations()["evidence-replay"].TotalBytes
	if p.Work > eligibilityWorkLimit || p.LogicalBytes > eligibilityLogicalLimit || p.BytesRead > readLimit &&
		(p.Complete || p.ExecutionStatus != StatusResourceLimit || p.BytesRead > readLimit+MaxDocumentBytes) {
		return false
	}
	counts := p.Tuple
	left := counts.Required
	for _, n := range []uint64{counts.Verified, counts.Failed, counts.Blocked, counts.Unprocessed} {
		if n > left {
			return false
		}
		left -= n
	}
	if counts.Required != uint64(len(p.SelectedObligationIDs)) || left != 0 || counts.Verified+counts.Failed+counts.Blocked != uint64(len(p.Obligations)) ||
		p.ProcessingComplete && counts.Unprocessed != 0 || p.Complete && counts.Unprocessed != 0 {
		return false
	}
	selected := make(map[string]bool, len(p.SelectedShardIDs))
	for _, id := range p.SelectedShardIDs {
		if id == "" || selected[id] {
			return false
		}
		selected[id] = true
	}
	if len(p.SelectedObligationIDs) > 0 && p.CoverageSHA256 != ordinaryIDDigest(p.SelectedObligationIDs) ||
		len(p.SelectedObligationIDs) == 0 && p.CoverageSHA256 != "" {
		return false
	}
	selectedObligations := make(map[string]bool, len(p.SelectedObligationIDs))
	for i, id := range p.SelectedObligationIDs {
		if !validOrdinaryObligationID(id) || selectedObligations[id] || i < len(p.Obligations) && p.Obligations[i].ID != id {
			return false
		}
		selectedObligations[id] = true
	}
	if len(p.Obligations) > len(p.SelectedObligationIDs) {
		return false
	}
	obligations := make(map[string]bool, len(p.Obligations))
	verified, failed, blocked := uint64(0), uint64(0), uint64(0)
	for _, item := range p.Obligations {
		if !validOrdinaryObligationID(item.ID) || obligations[item.ID] {
			return false
		}
		obligations[item.ID] = true
		switch item.State {
		case ordinaryTupleVerified:
			if item.Code != "" {
				return false
			}
			verified++
		case ordinaryTupleFailed:
			if item.Code == "" {
				return false
			}
			failed++
		case ordinaryTupleBlocked:
			if item.Code == "" {
				return false
			}
			blocked++
		default:
			return false
		}
	}
	if counts.Verified != verified || counts.Failed != failed || counts.Blocked != blocked {
		return false
	}
	if !executionStatuses[p.ProducerRecorded.ExecutionStatus] || !assessmentValues[p.ProducerRecorded.Assessment] ||
		!executionStatuses[p.BaseVerified.ExecutionStatus] || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, p.BaseVerified.Assessment) {
		return false
	}
	if p.Code == "" && p.Assessment == AssessFail || p.Assessment == AssessPass && p.Code != "" {
		return false
	}
	if p.Semantic == AssessPass && (!p.Complete || !p.ProcessingComplete || !p.BindingValid || counts.Required == 0 || counts.Verified != counts.Required ||
		p.BaseVerified.Assessment != AssessPass || p.Assessment != AssessPass || len(selected) == 0 || p.Code != "" || p.Eligibility == nil ||
		p.Eligibility.Assessment != AssessPass || !p.Eligibility.Complete) {
		return false
	}
	if counts.Failed > 0 && p.Semantic != AssessFail || p.Semantic == AssessFail && counts.Failed == 0 {
		return false
	}
	if p.Complete && (p.ExecutionStatus != StatusCompleted || !p.ProcessingComplete) || p.ProcessingComplete && !p.Complete || p.Assessment == AssessPass && !p.Complete {
		return false
	}
	if counts.Required > 0 && len(selected) == 0 || counts.Required == 0 && len(selected) != 0 || p.ProducerRecorded.Assessment == AssessPass ||
		(p.ProducerRecorded.Assessment == AssessFail || p.BaseVerified.Assessment == AssessFail || p.Eligibility != nil && p.Eligibility.Assessment == AssessFail) && p.Assessment != AssessFail {
		return false
	}
	if p.Eligibility != nil && !validOrdinaryNestedEligibility(*p.Eligibility, counts.Required) {
		return false
	}
	if p.Assessment == AssessPass && (p.Semantic != AssessPass || p.BaseVerified.Assessment != AssessPass) ||
		(p.BaseVerified.Assessment == AssessFail || p.Eligibility != nil && p.Eligibility.Assessment == AssessFail || slices.ContainsFunc(p.Gates, func(g ReplayGate) bool { return g.Failed > 0 })) && p.Assessment != AssessFail {
		return false
	}
	for _, gate := range p.Gates {
		finished := gate
		finished.finish()
		if gate.ID == "" || gate.Recomputed < 0 || gate.NotRecomputed < 0 || gate.Failed < 0 || gate.Failed > gate.Recomputed ||
			gate.Assessment != finished.Assessment || gate.EvidenceMode != finished.EvidenceMode || gate.Failed == 0 && (gate.Code != "" || gate.Record != "" || gate.Detail != "") || gate.Failed > 0 && (gate.Code == "" || p.Assessment != AssessFail) {
			return false
		}
	}
	if p.Assessment == AssessFail {
		foundFirst := false
		for _, f := range p.Findings {
			if f.Severity == "error" {
				foundFirst = f.Code == p.Code
				break
			}
		}
		if !foundFirst {
			if len(p.Findings) < 1000 {
				return false
			}
			failureSource := p.Tuple.Failed > 0 || p.ProducerRecorded.Assessment == AssessFail || p.BaseVerified.Assessment == AssessFail ||
				p.Eligibility != nil && p.Eligibility.Assessment == AssessFail || slices.ContainsFunc(p.Gates, func(g ReplayGate) bool { return g.Failed > 0 })
			if !failureSource {
				return false
			}
			if p.Tuple.Failed > 0 && p.ProducerRecorded.Assessment != AssessFail && p.BaseVerified.Assessment != AssessFail &&
				(p.Eligibility == nil || p.Eligibility.Assessment != AssessFail) && !slices.ContainsFunc(p.Gates, func(g ReplayGate) bool { return g.Failed > 0 }) {
				for _, obligation := range p.Obligations {
					if obligation.State == ordinaryTupleFailed {
						if obligation.Code != p.Code {
							return false
						}
						break
					}
				}
			}
		}
	}
	firstError := ""
	for _, finding := range p.Findings {
		if finding.Severity == "error" {
			firstError = finding.Code
			break
		}
	}
	if firstError != "" && firstError != p.Code || p.Assessment == AssessPass && firstError != "" ||
		firstError != "" && (p.Complete || p.ExecutionStatus == StatusCompleted) && p.Assessment != AssessFail {
		return false
	}
	if p.BaseVerified.Assessment == AssessPass {
		baseGates := map[string]bool{}
		for _, gate := range p.Gates {
			baseGates[gate.ID] = true
			if gate.Recomputed == 0 || gate.NotRecomputed != 0 || gate.Assessment != AssessPass || gate.EvidenceMode != ModeReplayedRaw {
				return false
			}
		}
		for _, id := range []string{"case-binding", "case-status", "tree", "verdict", "incremental-equality", "incremental-route", "expectations", "query-equality", "api", "query-expectations", "fact-reproduction", "dynamic-sql"} {
			if !baseGates[id] {
				return false
			}
		}
	}
	return true
}

func reserveOrdinaryStoredValidation(p OrdinaryQuerySemanticsResult, b *eligibilityBudget) *Error {
	if b == nil {
		return nil
	}
	visits := uint64(len(p.SelectedShardIDs) + len(p.SelectedObligationIDs) + len(p.Obligations) + len(p.Findings) + len(p.Gates) + 1)
	var text uint64
	for _, id := range p.SelectedShardIDs {
		text += uint64(len(id))
	}
	var digestBytes uint64 = uint64(len("tsgk-ordinary-shard-members/r1\x00"))
	for _, id := range p.SelectedObligationIDs {
		text += uint64(len(id))
		digestBytes += 8 + uint64(len(id))
	}
	for _, item := range p.Obligations {
		text += uint64(len(item.ID) + len(item.State) + len(item.Code))
	}
	for _, finding := range p.Findings {
		text += uint64(len(finding.Code) + len(finding.Path) + len(finding.Message) + len(finding.Severity))
	}
	work, e := eligibilityProduct("ordinary-stored-validation", 32, visits+text)
	if e != nil {
		return e
	}
	if e := b.charge(work, 128*visits+4*text, "ordinary-stored-validation"); e != nil {
		return e
	}
	if len(p.SelectedObligationIDs) != 0 {
		work, e := eligibilityProduct("ordinary-stored-coverage-digest", 32, digestBytes)
		if e != nil {
			return e
		}
		if e := b.charge(work, digestBytes+32, "ordinary-stored-coverage-digest"); e != nil {
			return e
		}
	}
	return nil
}

func validOrdinaryObligationID(id string) bool {
	return strings.HasPrefix(id, "qsem1:") && hex64.MatchString(strings.TrimPrefix(id, "qsem1:"))
}

func validOrdinaryNestedEligibility(p QueryReferenceEligibilityResult, ordinaryRequired uint64) bool {
	if p.Schema != ReportSchema || p.ResultSchema != QueryReferenceEligibilityResultSchema || !executionStatuses[p.ExecutionStatus] || !assessmentValues[p.Assessment] || !evidenceModes[p.EvidenceMode] {
		return false
	}
	// Ordinary eligibility verifies admission gates; the ordinary adjudicator owns tuple judgments.
	if p.Tuple != (QueryEligibilityCounts{Required: ordinaryRequired, Unprocessed: ordinaryRequired}) {
		return false
	}
	for _, counts := range []QueryEligibilityCounts{p.Build, p.Unit, p.Control, p.Assertion} {
		left := counts.Required
		for _, n := range []uint64{counts.Verified, counts.Failed, counts.Blocked, counts.Unprocessed} {
			if n > left {
				return false
			}
			left -= n
		}
		if left != 0 || p.Complete && counts.Unprocessed != 0 {
			return false
		}
	}
	if p.Assessment == AssessPass && (!p.Complete || p.Tuple.Required == 0 ||
		p.Build != (QueryEligibilityCounts{Required: 9, Verified: 9}) || p.Unit != (QueryEligibilityCounts{Required: 4, Verified: 4}) ||
		p.Control != (QueryEligibilityCounts{Required: 53, Verified: 53}) || p.Assertion != (QueryEligibilityCounts{Required: 36, Verified: 36}) ||
		p.Evidence == nil || p.Evidence.Assessment != AssessPass || p.Evidence.ResultSchema != EvidenceResultSchema || p.Evidence.ExecutionStatus != StatusCompleted) {
		return false
	}
	if p.Evidence != nil && (p.Evidence.Schema != ReportSchema || p.Evidence.ResultSchema != EvidenceResultSchema ||
		!executionStatuses[p.Evidence.ExecutionStatus] || !assessmentValues[p.Evidence.Assessment] || !evidenceModes[p.Evidence.EvidenceMode] ||
		p.Evidence.Assessment == AssessPass && (p.Evidence.ExecutionStatus != StatusCompleted || p.Evidence.EvidenceMode != ModeNewRun) ||
		p.Evidence.Assessment == AssessFail && p.Assessment != AssessFail) {
		return false
	}
	if p.Assessment == AssessPass && slices.ContainsFunc(p.Findings, func(f Finding) bool { return f.Severity == "error" }) ||
		p.Assessment != AssessFail && slices.ContainsFunc(p.Findings, func(f Finding) bool { return f.Severity == "error" }) && (p.Complete || p.ExecutionStatus == StatusCompleted) {
		return false
	}
	return !p.Complete || p.ExecutionStatus == StatusCompleted
}

func decodeOrdinaryReplayResult(data []byte, r *run) (ReplayResult, *Error) {
	var result ReplayResult
	if r == nil {
		return result, fail(KindInvalidInput, "ORDINARY_REPLAY_RUN_REQUIRED", "replay-result", nil)
	}
	b := ordinaryStoredBudget(r)
	var e *Error
	result, e = decodeSemanticReplayResult(data, r)
	if e != nil {
		return result, e
	}
	if result.OrdinaryQuerySemantics != nil {
		if e := reserveOrdinaryStoredValidation(*result.OrdinaryQuerySemantics, b); e != nil {
			return ReplayResult{}, e
		}
	}
	if !validOrdinaryReplayResult(result) {
		return ReplayResult{}, fail(KindInvalidInput, "ORDINARY_REPLAY_RESULT_INVALID", "replay-result", nil)
	}
	return result, nil
}

func ordinaryStoredBudget(r *run) *eligibilityBudget {
	if r.eligibility != nil {
		return r.eligibility
	}
	if r.storedSemantics == nil {
		r.storedSemantics = &eligibilityBudget{r: r}
	}
	return r.storedSemantics
}

func validOrdinaryReplayResult(rr ReplayResult) bool {
	p := rr.OrdinaryQuerySemantics
	if rr.Schema != ReportSchema || rr.ResultSchema != ReplayOrdinaryResultSchema || rr.Reducer.ID != ordinaryReplayReducerID || rr.Reducer.Revision != "r9" ||
		rr.Operation != "evidence-replay" || rr.QueryReference != nil || rr.QuerySemantics != nil ||
		!executionStatuses[rr.ExecutionStatus] || !assessmentValues[rr.Assessment] || !evidenceModes[rr.EvidenceMode] ||
		!executionStatuses[rr.Recorded.ExecutionStatus] || !assessmentValues[rr.Recorded.Assessment] ||
		!executionStatuses[rr.Subject.ExecutionStatus] || !assessmentValues[rr.Subject.Assessment] || !evidenceModes[rr.Subject.EvidenceMode] ||
		rr.Subject.Run == "" || len(rr.Subject.Run) > 128 || len(rr.Gates) > 1 || len(rr.PreprocessCST) != 0 {
		return false
	}
	if p == nil {
		return len(rr.Gates) == 0 && rr.Consumption == (Consumption{}) && !rr.EvidenceValid && rr.Assessment != AssessPass && rr.Recomputed == (Verdict{})
	}
	if !validOrdinaryStoredResult(*p) {
		return false
	}
	if rr.Recomputed != (Verdict{p.ExecutionStatus, p.Assessment}) || rr.BytesRead != p.BytesRead {
		return false
	}
	if p.ProducerRecorded.ExecutionStatus != StatusNotRun && rr.Recorded != p.ProducerRecorded {
		return false
	}
	if rr.Assessment == AssessPass && (!rr.EvidenceValid || rr.EvidenceMode != ModeReplayedRaw || p.Assessment != AssessPass || !p.Complete || !p.ProcessingComplete || p.FullScopeComplete) {
		return false
	}
	if rr.Assessment == AssessPass && (rr.ExecutionStatus != StatusCompleted || !rr.EvidenceValid) ||
		rr.EvidenceValid && (rr.Assessment == AssessFail || rr.ExecutionStatus != StatusCompleted || slices.ContainsFunc(rr.Findings, func(f Finding) bool { return f.Severity == "error" })) {
		return false
	}
	if rr.Consumption != (Consumption{}) && rr.Consumption != (Consumption{Expected: len(p.SelectedObligationIDs), Consumed: len(p.Obligations)}) {
		return false
	}
	if len(rr.Gates) == 0 {
		return rr.Consumption == (Consumption{}) && !rr.EvidenceValid && rr.Assessment != AssessPass
	}
	if rr.Consumption != (Consumption{Expected: len(p.SelectedObligationIDs), Consumed: len(p.Obligations)}) {
		return false
	}
	g := rr.Gates[0]
	finished := g
	finished.finish()
	if g.ID != "ordinary-query-semantics" || g.Recomputed+g.NotRecomputed != 1 || g.Failed > g.Recomputed || g.Assessment != finished.Assessment || g.EvidenceMode != finished.EvidenceMode ||
		g.Failed == 0 && (g.Code != "" || g.Record != "" || g.Detail != "") || g.Failed > 0 && (g.Code == "" || rr.EvidenceValid) {
		return false
	}
	switch p.Assessment {
	case AssessPass:
		return g.Recomputed == 1 && g.Failed == 0 && (!rr.EvidenceValid || rr.EvidenceMode == ModeReplayedRaw && rr.Assessment == AssessPass)
	case AssessFail:
		return g.Recomputed == 1 && g.Failed == 1 && g.Code == p.Code && (!rr.EvidenceValid || rr.Assessment == AssessFail) ||
			g.Failed == 0 && rr.Assessment != AssessPass && !rr.EvidenceValid
	case AssessBlocked:
		return g.NotRecomputed == 1 && g.Failed == 0 && (!rr.EvidenceValid || rr.Assessment == AssessBlocked)
	default:
		return false
	}
}
