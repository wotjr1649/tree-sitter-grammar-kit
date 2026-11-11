package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

// RuntimeEvidenceResult records approved material and fixed logger interpretation.
// It does not attest execution authenticity or accepted-event/preprocess-CST support.
type RuntimeEvidenceResult struct {
	Report
	Pins         *EvidencePinsResult      `json:"pins"`
	RuntimePin   *QueryRuntimePinIdentity `json:"runtime_pin,omitempty"`
	Complete     bool                     `json:"complete"`
	BytesRead    uint64                   `json:"bytes_read"`
	Work         uint64                   `json:"work"`
	LogicalBytes uint64                   `json:"logical_bytes"`
}

func verifyLoggerRuntime(req *EvidencePinsRequest, subject []byte, b *eligibilityBudget) (res RuntimeEvidenceResult, runtime *eligibilityRuntimePins, resultErr *Error) {
	res.Report = newReport("runtime logger pins")
	res.EvidenceMode, res.Assessment = ModeReplayedRaw, AssessBlocked
	read, work, logical := b.r.totalRead, b.work, b.logical
	previous := b.runtime
	defer func() {
		b.runtime = previous
		res.BytesRead, res.Work, res.LogicalBytes = b.r.totalRead-read, b.work-work, b.logical-logical
		if resultErr != nil {
			known := res.Assessment == AssessFail
			failReport(&res.Report, resultErr)
			if known {
				res.Assessment = AssessFail
			}
			res.Complete = false
		}
	}()
	pins, err := resolveEvidencePins(*req, b)
	res.Pins, res.Assessment = &pins, pins.Assessment
	res.Findings = append(res.Findings, pins.Findings...)
	if err != nil {
		return res, nil, asError(err)
	}
	if pins.Assessment != AssessPass || !pins.Complete {
		return res, nil, nil
	}
	if e := b.charge(uint64(len(subject))+1, 0, "runtime-pin-subject"); e != nil {
		return res, nil, e
	}
	if !bytes.Equal(req.Subject, subject) {
		res.Assessment = AssessFail
		res.Findings = append(res.Findings, Finding{Code: "PIN_SUBJECT_MISMATCH", Severity: "error", Path: "runtime-pins", Message: "runtime 승인 subject가 호출자의 원본 문서와 다르다"})
		return res, nil, nil
	}
	core := QueryReferenceEligibilityResult{Report: newReport("runtime closure"), Pins: &pins}
	core.Assessment = AssessPass
	s := eligibilitySession{b: b, res: &core, bound: true}
	runtimeErr := s.runtimePins(req)
	res.Assessment, res.RuntimePin = core.Assessment, core.RuntimePin
	res.Findings = append(res.Findings, core.Findings...)
	if runtimeErr != nil {
		return res, nil, runtimeErr
	}
	if s.runtime == nil && res.Assessment == AssessPass {
		res.Assessment = AssessBlocked
		res.Findings = append(res.Findings, Finding{Code: "PIN_RUNTIME_MISSING", Severity: "warning", Path: "native/runtime-tuple.json", Message: "logger 판독에 필요한 승인 runtime tuple이 없다"})
	}
	res.Complete = res.Assessment == AssessPass && s.runtime != nil
	if res.Complete {
		runtime = s.runtime
	}
	return res, runtime, nil
}

func validPassedLoggerRuntime(p *RuntimeEvidenceResult, subject string) bool {
	if p == nil || p.Schema != ReportSchema || p.ExecutionStatus != StatusCompleted || p.EvidenceMode != ModeReplayedRaw || p.Assessment != AssessPass || !p.Complete ||
		!validEvidencePinsResult(p.Pins) || p.Pins.Assessment != AssessPass || p.Pins.SubjectSHA256 != subject || p.RuntimePin == nil ||
		p.BytesRead > ReplayOperations()["evidence-replay"].TotalBytes || p.Work > eligibilityWorkLimit || p.LogicalBytes > eligibilityLogicalLimit {
		return false
	}
	r := p.RuntimePin
	if !hex64.MatchString(r.TupleSHA256) || !hex64.MatchString(r.ManifestSHA256) || r.Reader != runtimeClosureReader || r.LoggerReader != reuseobs.RuntimeLoggerReader || r.RuntimeMinCompatible == 0 || r.RuntimeMinCompatible > r.RuntimeLanguageVersion {
		return false
	}
	for _, required := range runtimePinRolesR1 {
		found := 0
		for _, material := range p.Pins.Materials {
			if material.Name != required.name {
				continue
			}
			if material.Role != required.role || material.Bytes == 0 || required.role == "runtime-tuple" && material.SHA256 != r.TupleSHA256 || required.role == "runtime-manifest" && material.SHA256 != r.ManifestSHA256 {
				return false
			}
			found++
		}
		if found != 1 {
			return false
		}
	}
	return true
}

func loggerPinsReducer(id string) bool { return id == "native-result-r9" || id == "native-result-r10" }

func validLoggerReplayResult(rr ReplayResult) bool {
	if rr.Schema != ReportSchema || rr.ResultSchema != ReplayLoggerPinsResultSchema || !loggerPinsReducer(rr.Reducer.ID) || rr.Reducer.Revision != strings.TrimPrefix(rr.Reducer.ID, "native-result-") || rr.Operation != "evidence-replay" ||
		!executionStatuses[rr.ExecutionStatus] || !assessmentValues[rr.Assessment] || !evidenceModes[rr.EvidenceMode] || rr.QuerySemantics != nil || rr.QueryReference != nil || len(rr.PreprocessCST) != 0 {
		return false
	}
	s := rr.Subject
	if s.Run == "" || len(s.Run) > 128 || !executionStatuses[s.ExecutionStatus] || !assessmentValues[s.Assessment] || !evidenceModes[s.EvidenceMode] {
		return false
	}
	if rr.ExecutionStatus == StatusCompleted && rr.EvidenceValid && rr.Recorded != (Verdict{s.ExecutionStatus, s.Assessment}) {
		return false
	}
	var subject string
	for _, id := range rr.Identities {
		if id.Role == "replay-profile" {
			subject = id.SHA256
		}
	}
	if rr.RuntimePins != nil && rr.RuntimePins.Assessment == AssessPass && !validPassedLoggerRuntime(rr.RuntimePins, subject) {
		return false
	}
	if rr.Assessment != AssessPass && !rr.EvidenceValid {
		return true
	}
	if rr.ExecutionStatus != StatusCompleted || !validPassedLoggerRuntime(rr.RuntimePins, subject) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range rr.Gates {
		fold := g
		fold.finish()
		if seen[g.ID] || g.Recomputed < 0 || g.NotRecomputed < 0 || g.Failed < 0 || g.Failed > g.Recomputed || fold.Assessment != g.Assessment || fold.EvidenceMode != g.EvidenceMode {
			return false
		}
		seen[g.ID] = true
		if rr.Assessment == AssessPass && g.Failed > 0 {
			return false
		}
	}
	if rr.Assessment == AssessPass {
		c := rr.Consumption
		return rr.EvidenceValid && seen["reuse-observation"] && seen["verdict"] && rr.Recomputed == (Verdict{StatusCompleted, AssessPass}) && c.Expected > 0 && c.Expected == c.Consumed && c.Missing == 0 && c.Duplicate == 0 && c.Unused == 0 && c.OutOfOrder == 0
	}
	return rr.EvidenceValid
}

func (p *eligibilityRuntimePins) matchesABI(languageRaw, upperRaw, lowerRaw jsontext.Value) bool {
	var language, upper, lower uint32
	if jsonv2.Unmarshal(languageRaw, &language) != nil || jsonv2.Unmarshal(upperRaw, &upper) != nil || jsonv2.Unmarshal(lowerRaw, &lower) != nil {
		return false
	}
	return upper == p.tuple.RuntimeLanguageVersion && lower == p.tuple.RuntimeMinCompatible && language >= lower && language <= upper
}
