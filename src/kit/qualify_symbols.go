package kit

import (
	"cmp"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	jsonv2 "encoding/json/v2"
)

// evalSymbolSet verifies and judges one symbol-mode workload (#218) on one host: the S05 run
// the host's producer wrote under incremental/<set>/ (profile.json, result.json, responses/
// and the case-source witnesses under sources/). The host profile must be the registered
// workload; the inventory revision selects its replay reducer under the identities it
// fixes (workload, operation, platform, the policy recomputed with the operation's own walls,
// the pinned runtime); and every case must end with its registered outcome and the selection
// derived from the pinned Roslyn observation alone. Like the route cells it trusts the
// reviewed workflow and producer: the producer's own build reports (producer, executable,
// compiler) are checked for consistency only, and that the driver received the active ranges
// is not recomputed from stored evidence.
func (q *qualifier) evalSymbolSet(w QualWorkload, p QualPlatform) (*qset, *Error) {
	s := newQSet(w, p)
	h := q.hosts[p.ID]
	if h == nil {
		return s, nil
	}
	dir := "incremental/" + w.Set + "/"
	profPath, resPath := dir+"profile.json", dir+"result.json"
	if h.files[profPath] == nil {
		return s, nil
	}
	markUsed(h, dir, "") // its files are this set's; the replay rejects any it does not register
	if h.id == nil || len(h.rejected) > 0 {
		s.Evidence, s.Mechanism = "REJECTED", AssessFail
		s.Findings = append(s.Findings, h.rejected...)
		if h.id == nil {
			s.Findings = append(s.Findings, Finding{Code: "RUN_IDENTITY_MISSING", Severity: "error", Path: p.ID, Message: "host 실행 identity가 없어 칸을 채울 수 없다"})
		}
		return s, nil
	}
	s.Evidence = ModeNewRun
	over := false
	add := func(code, path, msg string) {
		if len(s.Findings) < 200 {
			s.Findings = append(s.Findings, Finding{Code: code, Severity: "error", Path: p.ID + "/" + path, Message: msg})
		}
	}
	done := func() (*qset, *Error) {
		s.Mechanism = AssessPass
		for _, g := range s.Gates {
			if g.Failed > 0 {
				s.Mechanism = AssessFail
			}
		}
		switch {
		case len(s.Findings) > 0:
			s.Mechanism = AssessFail
		case over:
			s.Mechanism = AssessBlocked // not every case was recomputed: never a pass
		}
		return s, nil
	}
	stop := func(e *Error) bool {
		return e != nil && (e.Kind == KindCancelled || e.Code == "WALL_LIMIT" || e.Code == "TOTAL_BYTES_LIMIT" || e.Kind == KindIO)
	}
	// a file beyond the host file limit is not read: the set stays unverified, as a route
	// set's member over the limits does (BLOCKED, never a pass or a failure)
	tooLarge := func(e *Error) bool { return e != nil && e.Code == "FILE_BYTES_LIMIT" }
	// the registration: the host profile is the registered workload, field by field
	pdata, e := h.readWalked(profPath)
	if stop(e) {
		return nil, e
	}
	if tooLarge(e) {
		over = true
		return done()
	}
	if e != nil {
		add("PROFILE_MISSING", profPath, "set의 workload profile을 읽지 못했다")
		return done()
	}
	s.Profile = digestHex(pdata)
	prof, pe := parseIncremental(pdata)
	if pe != nil {
		add("PROFILE_INVALID_"+pe.Code, profPath, "workload profile이 계약에 맞지 않는다")
		return done()
	}
	if d := registrationDiff(OracleProfile{Native: prof, OracleCases: make([]OracleCase, len(prof.Cases))}, w); d != "" {
		add("REGISTRATION_MISMATCH", profPath, "host workload가 등록 inventory와 다르다: "+d)
		return done()
	}
	rdata, e := h.readWalked(resPath)
	if stop(e) {
		return nil, e
	}
	var res struct {
		ExecutionStatus string       `json:"execution_status"`
		Assessment      string       `json:"assessment"`
		Build           *nativeBuild `json:"build"`
		Cases           []rcCase     `json:"cases"`
	}
	if tooLarge(e) {
		over = true
		return done()
	}
	if e != nil || jsonv2.Unmarshal(rdata, &res) != nil || res.Build == nil {
		add("RESULT_INVALID", resPath, "S05 result를 읽지 못했다")
		return done()
	}
	// the members: every file of the set by its place; the replay binds each to its content
	var members []ReplayMember
	for _, f := range sortedKeys(h.files) {
		rel, ok := strings.CutPrefix(f, dir)
		if !ok {
			continue
		}
		role := map[string]string{"profile.json": "workload-profile", "result.json": "result"}[rel]
		switch {
		case role != "":
		case strings.HasPrefix(rel, "responses/"):
			role = "response"
		case strings.HasPrefix(rel, "sources/"):
			role = "case-source"
		default:
			continue // not a member: the replay rejects it (MEMBER_UNLISTED)
		}
		data, e := h.readWalked(f)
		if stop(e) {
			return nil, e
		}
		if tooLarge(e) {
			over = true
			return done()
		}
		if e != nil {
			add(e.Code, f, "symbol set 파일을 읽지 못했다")
			continue
		}
		members = append(members, ReplayMember{Path: rel, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
	}
	if len(s.Findings) > 0 {
		return done()
	}
	attempt, err := strconv.ParseUint(h.id.RunAttempt, 10, 64)
	if err != nil {
		add("RUN_IDENTITY_INVALID", "run-identity.json", "run attempt가 수가 아니다")
		return done()
	}
	selection := q.inv.Schema == QualificationInventorySchema
	reducer := "native-result-r5"
	if selection {
		reducer = "native-result-r6"
	}
	policy := nativePolicy(NativeOperations()[w.Operation], w.Output, w.Preprocess, selection).SHA256
	reg, _ := json.Marshal(map[string]any{"schema": ReplaySchema, "id": "qualification-" + w.Set, "reducer": reducer, "operation": "evidence-replay",
		"subject": map[string]any{"run": h.id.RunID, "attempt": attempt, "platform": p.pair(), "commit": q.candidate, "evidence_mode": ModeNewRun,
			"execution_status": res.ExecutionStatus, "assessment": res.Assessment},
		"identities": map[string]string{"workload": s.Profile, "policy": policy, "operation": w.Operation, "platform": p.pair(), "runtime": w.RuntimeCommit,
			"producer": res.Build.Identity, "executable": res.Build.ExecutableSHA256, "compiler": prof.Compiler.SHA256},
		"records": nil, "members": members})
	if h.records += uint64(len(res.Cases)); h.records > uint64(q.lim.Records) {
		return nil, fail(KindResourceLimit, "RECORDS_LIMIT", resPath, nil)
	}
	// the replay shares the qualification's caller, wall and the host's byte budget: the
	// qualification wall ends the replay, and a stopped replay reports the qualification's
	// own cause (the caller, or its wall as WALL_LIMIT), not the replay's view of it
	ctx, cancel := context.WithCancel(q.r.caller)
	stopWall := context.AfterFunc(q.r.wall, cancel)
	rr, rerr := replayShared(ctx, ReplayRequest{Root: filepath.Join(h.root, "incremental", w.Set), Profile: reg}, &replayBudget{r: h.x.r, total: q.lim.TotalBytes})
	stopWall()
	cancel()
	if rerr != nil && stop(asError(rerr)) {
		if e := q.r.check(); e != nil {
			return nil, e
		}
		return nil, asError(rerr)
	}
	if e := q.r.check(); e != nil {
		return nil, e
	}
	s.Gates = append(s.Gates, rr.Gates...)
	s.Identities = map[string]string{"runtime": rr.Observed["runtime"], "policy": rr.Observed["policy"], "operation": rr.Observed["operation"], "workload-route": w.Route}
	s.Host = map[string]string{"build_identity": res.Build.Identity, "executable_sha256": res.Build.ExecutableSHA256, "compiler_sha256": res.Build.Compiler.SHA256, "platform": p.pair()}
	switch {
	case rerr != nil:
		add("REPLAY_FAILED", dir, reducer+" replay가 끝나지 않았다: "+rerr.Error())
		return done()
	case !rr.EvidenceValid:
		for _, f := range rr.Findings {
			if f.Severity == "error" {
				add(f.Code, dir+f.Path, f.Message)
			}
		}
		add("REPLAY_INVALID", dir, reducer+" replay가 근거를 받지 않았다")
		return done()
	case rr.EvidenceMode != ModeReplayedRaw || rr.Recomputed != rr.Recorded || rr.Recomputed.Assessment == AssessUnresolved:
		over = true // valid, but not every case was recomputed
	}
	// the registered outcome and the Roslyn-derived selection of every case, in order
	if len(res.Cases) != len(w.Cases) {
		add("CASE_SET_MISMATCH", resPath, "result 사례 수가 등록 workload와 다르다")
		return done()
	}
	for i, c := range res.Cases {
		qc := w.Cases[i]
		v := claimPass
		if c.ID != qc.ID || c.ExecutionStatus != cmp.Or(qc.ExpectStatus, StatusCompleted) || c.Assessment != cmp.Or(qc.ExpectAssessment, AssessPass) || c.Code != qc.ExpectCode {
			v = claimFail
		}
		if c.Preprocess == nil || !reflect.DeepEqual(registeredSelection(*c.Preprocess), qc.PreprocessExpect) {
			v = claimFail // a registered check: it fails the row's checks, not the kit axis
		}
		s.checks[qc.ID] = v
		var digests []string
		for _, st := range c.Steps {
			if st.Incremental != nil {
				digests = append(digests, st.Incremental.Digest)
			}
		}
		sum, _ := json.Marshal(map[string]any{"status": c.ExecutionStatus, "assessment": c.Assessment, "code": c.Code, "preprocess": c.Preprocess, "steps": digests})
		s.summaries[qc.ID] = string(sum)
		s.Records++
	}
	return done()
}

// registeredSelection is the registered form of recorded preprocess steps: their byte ranges
// without points, the step index in order.
func registeredSelection(steps []PreprocessStep) []QualPreprocessStep {
	bytes := func(spans []Span) []ByteRange {
		out := []ByteRange{}
		for _, sp := range spans {
			out = append(out, ByteRange{sp.StartByte, sp.EndByte})
		}
		return out
	}
	out := []QualPreprocessStep{}
	for i, st := range steps {
		d := st.Directives
		if d == nil {
			d = []PreprocessDirective{}
		}
		out = append(out, QualPreprocessStep{Step: i, Status: st.Status, Directives: d, Active: bytes(st.Active), Inactive: bytes(st.Inactive)})
	}
	return out
}

// registeredSelectionBlock admits only the explicit disjoint-selection outcome of r6;
// historical inventories keep their all-OK syntax-only registration rule.
func registeredSelectionBlock(c QualCase) bool {
	suffix, ok := strings.CutPrefix(c.ExpectCode, SelectionRoutePrefix)
	step, err := strconv.Atoi(suffix)
	if !ok || err != nil || step < 1 || step > len(c.Edits) || suffix != strconv.Itoa(step) {
		return false
	}
	observation := func(s QualPreprocessStep) PreprocessStep {
		o := PreprocessStep{Status: s.Status}
		for _, r := range s.Active {
			o.Active = append(o.Active, Span{StartByte: r.Start, EndByte: r.End})
		}
		for _, r := range s.Inactive {
			o.Inactive = append(o.Inactive, Span{StartByte: r.Start, EndByte: r.End})
		}
		return o
	}
	size := int64(c.Input.Bytes)
	for i, e := range c.Edits {
		next := size + int64(e.NewEndByte) - int64(e.OldEndByte)
		if next < 0 {
			return false
		}
		if i+1 == step {
			return PreprocessSelectionDisjoint(observation(c.PreprocessExpect[i]), observation(c.PreprocessExpect[i+1]), e, uint64(size), uint64(next))
		}
		size = next
	}
	return false
}
