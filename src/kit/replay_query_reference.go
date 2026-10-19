package kit

import (
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
)

type referenceReplayScope struct {
	root     *os.Root
	pins     map[string]ReplayMember
	snapshot *evidenceSnapshot
}

func sameMemberBytes(a, b ReplayMember) bool {
	return a.Path == b.Path && a.Bytes == b.Bytes && a.SHA256 == b.SHA256
}

// Trust documents are independently pinned outside the nested evidence graph.
func replayQueryReferenceConnection(x *replayEnv) *Error {
	var req QueryReferenceConnectionRequest
	for _, input := range []struct {
		role string
		dst  *[]byte
	}{
		{"reference-registration", &req.Registration},
		{"query-registration", &req.WitnessRegistration},
		{"reference-policy", &req.Evidence.Policy},
	} {
		m, e := x.oneMember(input.role)
		if e != nil {
			return e
		}
		if strings.HasPrefix(strings.ToLower(m.Path), "evidence/") {
			return fail(KindInvalidInput, "MEMBER_TRUST_DOCUMENT_INSIDE", m.Path, nil)
		}
		b, e := x.member(m.Path)
		if e != nil {
			return e
		}
		*input.dst = b
	}
	scope := &referenceReplayScope{pins: map[string]ReplayMember{}, snapshot: &evidenceSnapshot{}}
	for _, m := range x.prof.Members {
		inside := strings.HasPrefix(m.Path, "evidence/")
		if strings.HasPrefix(strings.ToLower(m.Path), "evidence/") && !inside {
			return fail(KindInvalidInput, "MEMBER_EVIDENCE_SCOPE", m.Path, nil)
		}
		if (m.Role == "reference-evidence") != inside {
			return fail(KindInvalidInput, "MEMBER_EVIDENCE_SCOPE", m.Path, nil)
		}
		if inside {
			m.Path = strings.TrimPrefix(m.Path, "evidence/")
			scope.pins[m.Path] = m
		}
	}
	var reg QueryReferenceRegistration
	if e := decodeWitnessDocument("reference-registration", req.Registration, &reg, true, x.r); e != nil {
		return e
	}
	if e := validateReferenceRegistration(reg); e != nil {
		return fail(KindInvalidInput, "MEMBER_INVALID_"+e.Code, e.Path, nil)
	}
	witness, e := parseQueryWitnessRegistration(req.WitnessRegistration, x.r)
	if e != nil {
		return e
	}
	x.actual = map[string]string{"registration": digestHex(req.Registration), "witness": digestHex(req.WitnessRegistration), "evidence-policy": digestHex(req.Evidence.Policy), "source": witness.Input.SHA256, "route": witness.Route, "platform": witness.Platform, "role": witness.Role, "policy": witness.Policy}
	if x.prof.Subject.Platform != reg.Native.Identities["platform"] || x.prof.Subject.Run != reg.Native.Identities["run"] || strconv.FormatUint(x.prof.Subject.Attempt, 10) != reg.Native.Identities["attempt"] {
		x.finding("REFERENCE_SUBJECT_MISMATCH", "", "replay subject와 선택 native 실행 identity가 다르다")
	}
	if x.prof.Records != nil && !slices.Equal(x.prof.Records, []string{witness.Case}) {
		x.finding("RECORDS_WORKLOAD_MISMATCH", "", "등록 record 목록과 witness case가 다르다")
	}
	before, err := x.g.root.Lstat("evidence")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	if kind(before) != "dir" {
		return fail(KindInvalidInput, "MEMBER_EVIDENCE_ROOT_INVALID", "evidence", nil)
	}
	root, err := x.g.root.OpenRoot("evidence")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return fail(KindIO, "READ_FAILED", "evidence", err)
	}
	after, err := x.g.root.Lstat("evidence")
	if err != nil || kind(after) != "dir" || !os.SameFile(before, opened) || !os.SameFile(before, after) {
		return fail(KindIO, "SOURCE_CHANGED", "evidence", err)
	}
	scope.root = root
	scope.snapshot.walked = map[string]fs.FileInfo{}
	for p, info := range x.files {
		if rel, ok := strings.CutPrefix(p, "evidence/"); ok {
			scope.snapshot.walked[rel] = info
		}
	}
	res, resultErr := verifyQueryReferenceConnection(x.r, req, scope)
	x.queryReference = &res
	for _, n := range scope.snapshot.nodes {
		if n.ID == reg.Native.ID {
			x.recorded = Verdict{n.ExecutionStatus, n.Assessment}
			if n.EvidenceMode != x.prof.Subject.EvidenceMode {
				x.finding("REFERENCE_SUBJECT_MODE_MISMATCH", "", "subject evidence mode와 선택 native 기록이 다르다")
			}
			break
		}
	}
	for path, m := range scope.snapshot.verifiedMembers {
		full := "evidence/" + path
		outer, ok := x.members[full]
		outer.Path = path
		if ok && sameMemberBytes(outer, m) {
			x.verified[full], x.consumed[full] = true, true
		}
	}
	if resultErr != nil {
		return asError(resultErr)
	}
	x.cons = Consumption{Expected: 1, Consumed: 1}
	g := x.gate("stored-reference-connection")
	switch res.Assessment {
	case AssessPass:
		g.pass()
	case AssessFail:
		g.fail(witness.Case, "REFERENCE_CONNECTION_CONTRADICTION", "승인 source/CST/query/reference 연결 근거가 다르다")
	default:
		g.recorded()
	}
	for _, f := range res.Findings {
		if f.Severity == "error" {
			x.finding(f.Code, f.Path, f.Message)
		}
	}
	x.recomp = Verdict{res.ExecutionStatus, res.Assessment}
	return nil
}

// r2 admits producer failures without inventing a completed nested result.
func validReferenceReplayResult(rr ReplayResult) bool {
	if rr.Schema != ReportSchema || rr.ResultSchema != ReplayReferenceResultSchema || rr.Reducer.ID != "query-reference-connection-r1" || rr.Reducer.Revision != "r1" || rr.Operation != "evidence-replay" || !executionStatuses[rr.ExecutionStatus] || !assessmentValues[rr.Assessment] || !evidenceModes[rr.EvidenceMode] {
		return false
	}
	// r2 is labelled only after parseReplay accepted the subject
	s := rr.Subject
	if s.Run == "" || len(s.Run) > 128 || !evidenceModes[s.EvidenceMode] || !executionStatuses[s.ExecutionStatus] || !assessmentValues[s.Assessment] {
		return false
	}
	complete := len(rr.Gates) == 1
	if len(rr.Gates) > 1 {
		return false
	}
	if complete {
		g := rr.Gates[0]
		expected := g
		expected.finish()
		if expected.Assessment != g.Assessment || expected.EvidenceMode != g.EvidenceMode {
			return false
		}
		if g.ID != "stored-reference-connection" || g.Recomputed < 0 || g.NotRecomputed < 0 || g.Failed < 0 || g.Failed > g.Recomputed || g.Recomputed+g.NotRecomputed != 1 {
			return false
		}
	}
	// Replay's over-limit policy keeps only the recorded verdict of an otherwise valid result
	recordedOnly := rr.ExecutionStatus == StatusCompleted && rr.EvidenceValid && rr.EvidenceMode == ModeRecordedNotRecomputed && rr.Assessment == AssessUnresolved && rr.Recomputed == (Verdict{StatusNotRun, AssessUnresolved})
	p := rr.QueryReference
	if p == nil {
		// before the core: an error leaves no verdict, an over-limit trust document a recorded-only result
		return !complete && (recordedOnly || !rr.EvidenceValid && rr.Assessment != AssessPass && rr.Recomputed == (Verdict{})) && rr.Consumption == (Consumption{})
	}
	if p.Schema != ReportSchema || p.ResultSchema != QueryReferenceResultSchema || !executionStatuses[p.ExecutionStatus] || !assessmentValues[p.Assessment] || !evidenceModes[p.EvidenceMode] || p.Required < 0 || p.Verified < 0 || p.Verified > p.Required {
		return false
	}
	if (p.Required > 0 || p.NativeWitness != nil) && p.RegistrationSHA256 == "" {
		return false
	}
	if p.RegistrationSHA256 != "" && (!hex64.MatchString(p.RegistrationSHA256) || p.RegistrationSHA256 != rr.Observed["registration"]) {
		return false
	}
	if p.Assessment == AssessFail && rr.Assessment != AssessFail {
		return false
	}
	if p.Verified > 0 && (p.NativeWitness == nil || p.NativeWitness.ExecutionStatus != StatusCompleted) {
		return false
	}
	if n := p.NativeWitness; n != nil {
		if n.Schema != ReportSchema || n.ResultSchema != QueryWitnessResultSchema || !executionStatuses[n.ExecutionStatus] || !assessmentValues[n.Assessment] || !evidenceModes[n.EvidenceMode] || n.Required < 0 || n.Verified < 0 || n.Verified > n.Required {
			return false
		}
		if n.Required > 0 && !hex64.MatchString(n.RegistrationSHA256) {
			return false
		}
		if n.RegistrationSHA256 != "" && (!hex64.MatchString(n.RegistrationSHA256) || n.RegistrationSHA256 != rr.Observed["witness"]) {
			return false
		}
		if n.ExecutionStatus == StatusCompleted && n.Assessment == AssessFail && p.Assessment != AssessFail {
			return false
		}
		// an unfinished witness never passes, and both cores take the denominator from one registration
		if n.ExecutionStatus != StatusCompleted && n.Assessment == AssessPass || n.Required > 0 && n.Required != p.Required {
			return false
		}
		if n.ExecutionStatus == StatusCompleted && (!hex64.MatchString(n.RegistrationSHA256) || n.Required != p.Required || p.Verified > n.Verified || n.EvidenceMode != ModeReplayedRaw || !slices.Contains([]string{AssessPass, AssessFail, AssessBlocked}, n.Assessment)) {
			return false
		}
	}
	if !complete {
		return p.Assessment != AssessPass && rr.Assessment != AssessPass && rr.Recomputed == (Verdict{}) && !rr.EvidenceValid && rr.Consumption == (Consumption{})
	}
	if p.ExecutionStatus != StatusCompleted || p.EvidenceMode != ModeReplayedRaw || p.NativeWitness == nil || p.NativeWitness.ExecutionStatus != StatusCompleted || !hex64.MatchString(p.RegistrationSHA256) || rr.Recomputed != (Verdict{p.ExecutionStatus, p.Assessment}) && !recordedOnly || rr.Consumption != (Consumption{Expected: 1, Consumed: 1}) {
		return false
	}
	g := rr.Gates[0]
	switch p.Assessment {
	case AssessPass:
		if p.Required == 0 || p.Verified != p.Required || p.NativeWitness.Assessment != AssessPass || g.Recomputed != 1 || g.Failed != 0 {
			return false
		}
	case AssessFail:
		if g.Failed != 1 || rr.Assessment != AssessFail {
			return false
		}
	case AssessBlocked:
		if g.NotRecomputed != 1 || g.Failed != 0 {
			return false
		}
	default:
		return false
	}
	if rr.ExecutionStatus != StatusCompleted {
		return !rr.EvidenceValid && rr.Assessment != AssessPass
	}
	if rr.EvidenceValid { // Replay clears it whenever a gate failed
		return g.Failed == 0 && (recordedOnly || rr.Assessment == p.Assessment)
	}
	return rr.Assessment == AssessFail
}
