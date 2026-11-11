package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

type ordinaryGuardedRecord struct {
	Record     OrdinaryOracleRecord
	Transport  jsontext.Value
	StepFacts  jsontext.Value
	Projection []byte
}

type ordinaryBatchCache struct {
	Owner      string
	Index      int
	Capture    ordinaryGuardedCapture
	Frames     map[string]*ordinaryCaseCache
	FrameBytes map[int]ordinaryCapturedFrame
	Process    ordinaryGuardedProcessResult
	Accepted   bool
}

type ordinaryCapturedFrame struct{ Stdin, Stdout []byte }

type ordinaryCaseCacheKey struct {
	ProfileSHA256, Candidate, Cohort, RunID            string
	Owner                                              string
	Batch, Frame                                       int
	Case                                               string
	RequestSHA256, ResponseSHA256, SourceHistorySHA256 string
	BuildIdentity, RuntimeSHA256, PolicySHA256         string
}

// ordinaryCaseCache stores one full native case decode, shared by every selected tuple.
type ordinaryCaseCache struct {
	Key                         ordinaryCaseCacheKey
	Owner                       string
	Batch, Frame                int
	Case                        string
	RequestFrame, ResponseFrame []byte
	Source                      []byte
	request                     semanticNativeRequest
	raw                         nativewire.Response
	checked                     []nativewire.CheckedStep
	versions                    [][]byte
	points                      []EditPoints
	record                      OrdinaryOracleRecord
	projection                  []byte
	stepFacts                   []byte
	recordedBase                Verdict
	recordedBaseCode            string
	ready                       bool
	BatchAccepted               bool
}

type ordinaryGuardedSet struct {
	Manifest     OrdinaryOracleManifest
	Records      map[string]ordinaryGuardedRecord
	Batches      map[string]*ordinaryBatchCache
	Cases        map[string]*ordinaryCaseCache
	Findings     []Finding
	Assessment   string
	Code         string
	FirstFailure string
	Complete     bool
}

type ordinaryGuardedCaptureStream struct {
	Bytes         uint64 `json:"bytes"`
	SHA256        string `json:"sha256"`
	RetainedBytes uint64 `json:"retained_bytes"`
}

type ordinaryGuardedCaptureSpec struct {
	Path        string         `json:"Path"`
	Args        []string       `json:"Args"`
	Dir         string         `json:"Dir"`
	Env         []string       `json:"Env"`
	Stdin       jsontext.Value `json:"Stdin"`
	Interactive bool           `json:"Interactive"`
	StdoutBytes int64          `json:"StdoutBytes"`
	StderrBytes int64          `json:"StderrBytes"`
	Wall        time.Duration  `json:"Wall"`
	Grace       time.Duration  `json:"Grace"`
	Memory      struct {
		Bytes uint64 `json:"Bytes"`
		Hard  bool   `json:"Hard"`
	} `json:"Memory"`
	CgroupParent string `json:"CgroupParent"`
}

type ordinaryGuardedProcessResult struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	ExitCode        int    `json:"exit_code"`
	StdoutBytes     int64  `json:"stdout_bytes"`
	StderrBytes     int64  `json:"stderr_bytes"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	WallMillis      uint64 `json:"wall_ms"`
	Cleanup         struct {
		Method            string `json:"method"`
		Scope             string `json:"scope"`
		Verified          bool   `json:"verified"`
		ResidualAfterExit int    `json:"residual_after_exit"`
		Detail            string `json:"detail,omitempty"`
	} `json:"cleanup"`
	Memory       jsontext.Value `json:"memory"`
	Capabilities jsontext.Value `json:"capabilities"`
}

type ordinaryGuardedFrame struct {
	Index      int            `json:"index"`
	Case       string         `json:"case_id"`
	State      string         `json:"state"`
	Status     string         `json:"status"`
	Detail     string         `json:"detail"`
	WallMillis int64          `json:"wall_ms"`
	Stdin      jsontext.Value `json:"stdin"`
	Stdout     jsontext.Value `json:"stdout"`
}

type ordinaryGuardedCapture struct {
	Schema           string                       `json:"schema"`
	Owner            string                       `json:"owner"`
	Batch            int                          `json:"batch"`
	BuildIdentity    string                       `json:"build_identity"`
	ExecutableSHA256 string                       `json:"executable_sha256"`
	ExecutableBytes  int64                        `json:"executable_bytes"`
	Spec             ordinaryGuardedCaptureSpec   `json:"spec"`
	Process          ReplayMember                 `json:"process"`
	Stderr           ReplayMember                 `json:"stderr"`
	Stdin            ordinaryGuardedCaptureStream `json:"stdin"`
	Stdout           ordinaryGuardedCaptureStream `json:"stdout"`
	Frames           []ordinaryGuardedFrame       `json:"frames"`
	Suffix           jsontext.Value               `json:"stdout_suffix"`
	TrailingBytes    int64                        `json:"trailing_bytes"`
	Accepted         bool                         `json:"accepted"`
	Code             string                       `json:"code"`
	CaptureError     string                       `json:"capture_error"`
}

type ordinaryGuardedSpan struct {
	Offset   uint64         `json:"offset"`
	Bytes    uint64         `json:"bytes"`
	SHA256   string         `json:"sha256"`
	Complete bool           `json:"complete"`
	Members  []ReplayMember `json:"members"`
}

type ordinaryRecordWire struct {
	Schema         string                `json:"schema"`
	Case           string                `json:"case"`
	Workload       string                `json:"workload"`
	BaseProjection ReplayMember          `json:"base_projection"`
	QuerySemantics *OracleQuerySemantics `json:"query_semantics"`
	Transport      jsontext.Value        `json:"transport"`
	StepFacts      jsontext.Value        `json:"step_facts"`
	Complete       bool                  `json:"complete"`
}

type ordinaryTransport struct {
	Batch           int    `json:"batch"`
	Frame           int    `json:"frame"`
	State           string `json:"state"`
	FrameWallMillis uint64 `json:"frame_wall_ms"`
}

type ordinaryProjectionCase struct {
	ID              string      `json:"id"`
	Input           NativeInput `json:"input"`
	Encoding        string      `json:"encoding"`
	ExecutionStatus string      `json:"execution_status"`
	Assessment      string      `json:"assessment"`
	Code            string      `json:"code"`
}

// ordinaryGuardedSet reuses the eligibility pass's immutable member authority, physical root,
// and budget. It never opens a member through a new filesystem reader or ledger.
func (b *eligibilityBudget) ordinaryGuardedSet(root *os.Root, s *eligibilitySession, registration OrdinaryRegistration, profile OracleProfile, manifestBytes, runBindingBytes []byte) (*ordinaryGuardedSet, *Error) {
	out := &ordinaryGuardedSet{Records: map[string]ordinaryGuardedRecord{}, Batches: map[string]*ordinaryBatchCache{}, Cases: map[string]*ordinaryCaseCache{}, Findings: []Finding{}, Assessment: AssessBlocked}
	if b == nil || b.r == nil || s == nil || s.res == nil || s.b != b || s.r != b.r || s.ordinary == nil || s.ordinary.session != s || root == nil || s.snapshot == nil || !s.snapshot.graphBound {
		return nil, fail(KindInvalidInput, "ORDINARY_READER_UNBOUND", "oracle", nil)
	}
	pagingValid := false
	if s.snapshot.paging != nil {
		var e *Error
		pagingValid, e = s.snapshot.paging.valid(s)
		if e != nil {
			return nil, e
		}
		if !pagingValid || s.snapshot.paging.root != root || s.snapshot.paging.b != b {
			return nil, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", "oracle", nil)
		}
	}
	if s.ordinary.ClosureEncoding != "plain" && !pagingValid {
		return nil, fail(KindInvalidInput, "ELIGIBILITY_PAGING_IDENTITY", "oracle", nil)
	}
	if e := b.charge(1, 0, "ordinary-reader-start"); e != nil {
		return nil, e
	}
	globalInvalid := false
	recordFinding := func(code, at, message string) {
		if code == "" {
			code = "ORDINARY_SET_INVALID"
		}
		if out.Code == "" {
			out.Code = code
		}
		if out.FirstFailure == "" {
			out.FirstFailure = code
		}
		out.Assessment = AssessFail
		if len(out.Findings) < 1000 {
			out.Findings = append(out.Findings, Finding{Severity: "error", Code: code, Path: at, Message: message})
		}
	}
	add := func(code, at, message string) {
		if code != "ORDINARY_PRODUCER_FAILED" && !strings.HasPrefix(code, "ORDINARY_BASE_") {
			globalInvalid = true
		}
		recordFinding(code, at, message)
	}
	addCase := func(code, at, message string) { recordFinding(code, at, message) }
	stop := func(err *Error) (*ordinaryGuardedSet, *Error) {
		if err != nil && err.Kind == KindInvalidInput {
			globalInvalid = true
			recordFinding(err.Code, err.Path, "ordinary 저장 입력의 결속 또는 형식이 유효하지 않다")
		}
		if globalInvalid {
			for _, cache := range out.Cases {
				cache.ready = false
			}
		}
		if err != nil && out.Assessment == AssessFail {
			out.Complete = false
		}
		if err == nil {
			return out, nil
		}
		return out, asError(err)
	}
	physical, openErr := root.OpenRoot("oracle")
	if openErr != nil {
		return stop(fail(KindIO, "ROOT_UNREADABLE", "oracle", openErr))
	}
	defer func() { _ = physical.Close() }()
	if b.r.semanticEntries != nil {
		if e := b.r.semanticEntries.bindChild(root, "oracle", physical); e != nil {
			return stop(e)
		}
	}
	if e := b.semanticSnapshotIdentity(root, physical, s.snapshot); e != nil {
		return stop(e)
	}
	manifestPin := eligibilityMember{NodeID: s.owners["oracle/manifest.json"], Member: ReplayMember{Path: "oracle/manifest.json", Role: "manifest", Bytes: uint64(len(manifestBytes)), SHA256: digestHex(manifestBytes)}}
	manifestOwned, manifestOK, e := s.member(&manifestPin)
	if e != nil {
		return stop(e)
	}
	if !manifestOK || !bytes.Equal(manifestOwned, manifestBytes) {
		add("ORDINARY_MANIFEST_BINDING", "oracle/manifest.json", "manifest가 같은 eligibility snapshot에 보유된 원문과 다르다")
		return stop(nil)
	}
	if e = b.ordinaryFinalComplete("ordinary-manifest", manifestBytes); e != nil {
		return stop(e)
	}
	var manifest OrdinaryOracleManifest
	if e = b.decodeOrdinaryTyped("ordinary-manifest", manifestBytes, &manifest, jsonv1.FormatDurationAsNano(true)); e != nil {
		return stop(e)
	}
	if manifest, err := validateOrdinaryManifestShape(manifest); err != nil {
		add(asError(err).Code, "oracle/manifest.json", "r8 manifest 구조가 유효하지 않다")
		return stop(nil)
	} else {
		out.Manifest = manifest
	}
	ordinary := s.ordinary.Binding
	if ordinary.ClosureEncoding != s.ordinary.ClosureEncoding || manifest.ClosureEncoding != s.ordinary.ClosureEncoding || manifest.Context != ordinary.Context ||
		manifest.Profile != ordinary.Profile || manifest.PlanningProfile != ordinary.PlanningProfile || manifest.Registration != ordinary.Registration || manifest.Assets != ordinary.Assets || manifest.Plan != ordinary.Plan ||
		manifest.Candidate != ordinary.Candidate || manifest.SemanticCohort != ordinary.SemanticCohort || manifest.RunID != ordinary.RunID || !slices.Equal(manifest.ShardIDs, ordinary.ShardIDs) ||
		profile.Native.SHA256 != manifest.Profile.SHA256 || profile.Native.ID != manifest.Producer["profile_id"] || profile.Native.Route != registration.Route || profile.Native.Operation != "native-query-batch" || profile.Native.Encoding != EncodingUTF8 || profile.Native.Output != OutputTree || !profile.API {
		add("ORDINARY_MANIFEST_BINDING", "oracle/manifest.json", "profile·closure·cohort·run 또는 선택 범위가 caller 승인과 다르다")
	}
	if manifest.RunID == "" || manifest.Producer["build_identity"] == "" || manifest.Producer["executable_sha256"] == "" || manifest.Producer["platform"] != registration.Subject || manifest.Assessment == AssessPass {
		add("ORDINARY_MANIFEST_PRODUCER", "oracle/manifest.json", "producer identity 또는 producer-local 판정이 유효하지 않다")
	}
	if manifest.Assessment == AssessFail {
		add("ORDINARY_PRODUCER_FAILED", "oracle/manifest.json", "manifest에 저장된 producer FAIL을 보존한다")
	}
	if uint64(len(manifestBytes)) > OrdinaryPlanLimits().FileBytes || len(manifest.Members)+1 > int(OrdinaryPlanLimits().Entries) {
		return stop(fail(KindResourceLimit, "ORDINARY_SET_LIMIT", "oracle/manifest.json", nil))
	}
	if !ordinaryPinMatchesBytes(ordinary.RunBinding, runBindingBytes) {
		add("ORDINARY_RUN_BINDING_PIN", "run-binding", "caller run binding bytes가 승인된 pin과 다르다")
	}
	var runBinding OrdinaryRunBinding
	if e = b.decodeOrdinaryTyped("ordinary-run-binding", runBindingBytes, &runBinding); e != nil {
		return stop(e)
	}
	if runBinding, err := validateOrdinaryRunBindingShape(runBinding); err != nil {
		add(asError(err).Code, "run-binding", "r2 caller binding 구조가 유효하지 않다")
	} else if !ordinaryRunBindingMatchesManifest(runBinding, manifest) || !ordinaryRunBindingMatchesEligibility(runBinding, ordinary) {
		add("ORDINARY_BINDING_MISMATCH", "run-binding", "caller r2 binding과 r8 manifest의 전체 member·batch 소유권이 다르다")
	}
	var memberPathBytes uint64
	for _, member := range manifest.Members {
		memberPathBytes += uint64(len(member.Path))
	}
	memberWork, memberLogical := ordinaryMemberIndexReservation(uint64(len(manifest.Members)), memberPathBytes)
	if e := b.charge(memberWork, memberLogical, "ordinary-member-case-index"); e != nil {
		return stop(e)
	}
	memberByPath := make(map[string]OrdinaryMember, len(manifest.Members))
	listed := map[string]bool{"manifest.json": true}
	folded := map[string]bool{"manifest.json": true}
	var total uint64 = uint64(len(manifestBytes))
	for _, m := range manifest.Members {
		key := strings.ToLower(m.Path)
		if !ordinaryPath(m.Path) || m.Pin.Path != m.Path || !ordinaryPin(m.Pin) || folded[key] || !ordinaryMemberOwnerRole(m) || !ordinaryMemberRole(m.Role, manifest.ClosureEncoding) || m.Pin.Bytes > OrdinaryPlanLimits().FileBytes || total > OrdinaryPlanLimits().ReadBytes-m.Pin.Bytes {
			add("ORDINARY_MEMBER_INVALID", m.Path, "r8 member 경로·owner·역할·pin 또는 한도가 다르다")
			continue
		}
		folded[key], listed[m.Path] = true, true
		memberByPath[m.Path] = m
		total += m.Pin.Bytes
		path := "oracle/" + m.Path
		owned, ok := s.owned[path]
		if !ok || owned.NodeID != s.owners[path] || owned.Member != (ReplayMember{Path: path, Role: m.Role, Bytes: m.Pin.Bytes, SHA256: m.Pin.SHA256}) || s.snapshot.verifiedMembers[path] != owned.Member || s.snapshot.invalidNodes[owned.NodeID] {
			add("ORDINARY_MEMBER_OWNER", path, "r8 member가 같은 graph snapshot의 유일한 owner pin에 결속되지 않았다")
			continue
		}
		if _, e := s.checkMemberPin(&owned, false); e != nil {
			return stop(e)
		}
	}
	if total > OrdinaryPlanLimits().ReadBytes {
		return stop(fail(KindResourceLimit, "ORDINARY_SET_LIMIT", "oracle/manifest.json", nil))
	}
	if e = ordinaryCheckManifestMembers(manifest, registration, profile, memberByPath, add); e != nil {
		return stop(e)
	}
	// Build transcript and member indexes before decoding any case so frame order and owners
	// are fixed independently of map iteration order.
	caseBatch := map[string]ordinaryFrameLocation{}
	for i, batch := range manifest.Batches {
		if e := b.charge(1+uint64(len(batch.Cases)), 512+256*uint64(len(batch.Cases)), "ordinary-batch-index"); e != nil {
			return stop(e)
		}
		owner := fmt.Sprintf("D_batch_%05d", i)
		if batch.Index != i || batch.Owner != owner || batch.Cases == nil || len(batch.Cases) > manifest.Operation.BatchFiles || batch.Capture.Role != "native-batch-capture" || !ordinaryReplayPinValid(batch.Capture) {
			add("ORDINARY_BATCH_INVALID", batch.Capture.Path, "batch index·owner·case 목록·capture pin이 유효하지 않다")
			continue
		}
		capMember, ok := memberByPath[batch.Capture.Path]
		if !ok || capMember.Owner != owner || capMember.CaseOrBatch != owner || capMember.Role != batch.Capture.Role || !ordinaryReplayPinMatches(capMember, batch.Capture) {
			add("ORDINARY_BATCH_CAPTURE", batch.Capture.Path, "transcript member의 batch owner 또는 pin이 다르다")
			continue
		}
		captureBytes, ok, e := b.ordinaryMemberBytes(s, capMember)
		if e != nil {
			return stop(e)
		}
		if !ok {
			add("ORDINARY_BATCH_CAPTURE", capMember.Path, "검증된 transcript bytes가 없다")
			continue
		}
		var capture ordinaryGuardedCapture
		if e := b.decodeOrdinaryTyped("oracle/"+capMember.Path, captureBytes, &capture, jsonv1.FormatDurationAsNano(true)); e != nil {
			return stop(e)
		}
		batchCache := &ordinaryBatchCache{Owner: owner, Index: i, Capture: capture, Frames: map[string]*ordinaryCaseCache{}, FrameBytes: map[int]ordinaryCapturedFrame{}, Accepted: capture.Accepted}
		out.Batches[owner] = batchCache
		if capture.Schema != "tsgk-native-batch-capture/r1" || capture.Owner != owner || capture.Batch != i || capture.Process.Path == "" || capture.Stderr.Path == "" || len(capture.Frames) != len(batch.Cases) || capture.BuildIdentity != manifest.Producer["build_identity"] || capture.ExecutableSHA256 != manifest.Producer["executable_sha256"] || capture.ExecutableBytes <= 0 {
			add("ORDINARY_BATCH_CAPTURE", capMember.Path, "transcript schema·build·executable·frame cardinality가 다르다")
		}
		if capture.Process.Role != "process" || capture.Stderr.Role != "stderr" || !ordinaryCaptureMember(capture.Process, owner, memberByPath) || !ordinaryCaptureMember(capture.Stderr, owner, memberByPath) {
			add("ORDINARY_BATCH_OWNER", capMember.Path, "process 또는 stderr owner가 batch capture와 다르다")
		}
		processMember, processOK, e := b.ordinaryMemberBytes(s, memberByPath[capture.Process.Path])
		if e != nil {
			return stop(e)
		}
		var process ordinaryGuardedProcessResult
		if !processOK {
			add("ORDINARY_BATCH_PROCESS", capture.Process.Path, "동일 process owner의 typed process result가 없다")
		} else if e := b.decodeOrdinaryTyped("oracle/"+capture.Process.Path, processMember, &process); e != nil {
			return stop(e)
		}
		batchCache.Process = process
		if process.StdoutBytes < 0 || process.StderrBytes < 0 || uint64(process.StdoutBytes) != capture.Stdout.Bytes || uint64(process.StderrBytes) != capture.Stderr.Bytes {
			batchCache.Accepted = false
			add("ORDINARY_BATCH_PROCESS_STREAM", capture.Process.Path, "process stream count가 transcript와 다르다")
		}
		if process.Status != "COMPLETED" || process.ExitCode != 0 || !process.Cleanup.Verified || process.Cleanup.ResidualAfterExit != 0 || process.StdoutTruncated || process.StderrTruncated {
			batchCache.Accepted = false
		}
		if capture.Spec.Path == "" || !capture.Spec.Interactive || len(capture.Spec.Stdin) != 0 && string(capture.Spec.Stdin) != "null" || capture.Spec.StdoutBytes <= 0 || capture.Spec.StderrBytes <= 0 || uint64(capture.Spec.StdoutBytes) != manifest.Operation.OutputBytes+4 || capture.Spec.Wall <= 0 || capture.TrailingBytes != 0 || capture.CaptureError != "" {
			batchCache.Accepted = false
			add("ORDINARY_BATCH_SPEC", capMember.Path, "interactive process spec·capture 상태가 일반 batch 정책과 다르다")
		}
		if capture.Accepted && (!processOK || process.Status != "COMPLETED" || process.ExitCode != 0 || !process.Cleanup.Verified || capture.TrailingBytes != 0 || capture.CaptureError != "" || capture.Stdin.Bytes != capture.Stdin.RetainedBytes || capture.Stdout.Bytes != capture.Stdout.RetainedBytes) {
			add("ORDINARY_BATCH_ACCEPTED", capMember.Path, "accepted marker와 process·stream 종료 증거가 다르다")
			batchCache.Accepted = false
		}
		for j, frame := range capture.Frames {
			if frame.Index != j || frame.Case != batch.Cases[j] || caseBatch[frame.Case].owner != "" || !containsString(manifest.Cases, frame.Case) {
				add("ORDINARY_BATCH_FRAME", capMember.Path, "frame 순서 또는 case 소유권이 중복·변조됐다")
				continue
			}
			caseBatch[frame.Case] = ordinaryFrameLocation{owner: owner, batch: i, frame: j, capture: &capture, batchCache: batchCache}
			if frame.State != "ANSWERED" && frame.State != "SENT" && frame.State != "NOT_SENT" {
				add("ORDINARY_BATCH_FRAME", capMember.Path, "frame 상태가 허용되지 않는다")
			}
			if frame.State == "ANSWERED" && frame.Stdout.Kind() == 'n' || frame.State == "NOT_SENT" && (frame.Stdin.Kind() != 'n' || frame.Stdout.Kind() != 'n') {
				add("ORDINARY_BATCH_FRAME", capMember.Path, "frame state와 request/response span이 모순된다")
			}
		}
		if e := b.ordinaryValidateBatchStreams(s, capture, capMember, memberByPath, batchCache, add); e != nil {
			return stop(e)
		}
	}
	for i, c := range manifest.Cases {
		if i >= len(profile.Native.Cases) {
			add("ORDINARY_PROFILE_CASES", c, "manifest case 수가 caller profile보다 많다")
			return stop(nil)
		}
		location, ok := caseBatch[c]
		if !ok {
			add("ORDINARY_BATCH_CASE", c, "등록 case에 native batch frame이 없다")
			continue
		}
		profileCase := profile.Native.Cases[i]
		registrationCaseIndex := slices.IndexFunc(registration.Cases, func(v OrdinaryCase) bool { return v.ID == c })
		if registrationCaseIndex < 0 {
			add("ORDINARY_CASE_BINDING", c, "case가 ordinary registration에 없다")
			continue
		}
		cache, e := b.ordinaryCase(s, registration, profile, profileCase, registration.Cases[registrationCaseIndex], location, memberByPath, manifest, addCase)
		if e != nil {
			return stop(e)
		}
		if cache != nil {
			out.Cases[c] = cache
			location.batchCache.Frames[c] = cache
		}
	}
	for _, m := range manifest.Members {
		if m.Role != "record" {
			continue
		}
		recordBytes, ok, e := b.ordinaryMemberBytes(s, m)
		if e != nil {
			return stop(e)
		}
		if !ok {
			add("ORDINARY_RECORD_INVALID", m.Path, "record bytes가 검증된 snapshot에 없다")
			continue
		}
		if e := b.ordinaryFinalComplete("oracle/"+m.Path, recordBytes); e != nil {
			return stop(e)
		}
		var wire ordinaryRecordWire
		if e := b.decodeOrdinaryTyped("oracle/"+m.Path, recordBytes, &wire); e != nil {
			return stop(e)
		}
		record := OrdinaryOracleRecord{Schema: wire.Schema, Case: wire.Case, Workload: wire.Workload, BaseProjection: wire.BaseProjection, QuerySemantics: wire.QuerySemantics, Transport: append([]byte(nil), wire.Transport...), StepFacts: append([]byte(nil), wire.StepFacts...), Complete: wire.Complete}
		if _, err := validateOrdinaryRecordShape(record); err != nil || record.Case != m.CaseOrBatch || m.Owner != "O" || record.Workload != profile.Native.ID || record.QuerySemantics.RegistrationSHA256 != manifest.Registration.SHA256 || record.QuerySemantics.Projection != record.BaseProjection {
			add("ORDINARY_RECORD_BINDING", m.Path, "r5 record schema·case·workload·registration·projection이 다르다")
			continue
		}
		projectionMember, exists := memberByPath[record.BaseProjection.Path]
		if !exists || projectionMember.Owner != "O" || projectionMember.CaseOrBatch != record.Case || projectionMember.Role != "projection" || !ordinaryReplayPinMatches(projectionMember, record.BaseProjection) {
			add("ORDINARY_RECORD_PROJECTION", m.Path, "r5 base projection owner 또는 pin이 다르다")
			continue
		}
		projection, ok, e := b.ordinaryMemberBytes(s, projectionMember)
		if e != nil {
			return stop(e)
		}
		if !ok {
			add("ORDINARY_RECORD_PROJECTION", projectionMember.Path, "base projection bytes가 없다")
			continue
		}
		var base ordinaryProjectionCase
		if e := b.decodeTypedDocument("ordinary-base-verdict", projection, &base, 3, false, false, false); e != nil {
			return stop(e)
		}
		if base.Assessment == AssessFail {
			code := base.Code
			if code == "" {
				code = "ORDINARY_BASE_RECORDED_FAILED"
			}
			recordFinding(code, record.Case, "원래 base projection의 FAIL을 보존한다")
		}
		if err := b.ordinaryRecordTransport(record, locationOrEmpty(caseBatch, record.Case), memberByPath, add); err != nil {
			return stop(err)
		}
		if cache := out.Cases[record.Case]; cache != nil {
			cache.record, cache.projection = record, projection
			cache.recordedBase, cache.recordedBaseCode = Verdict{base.ExecutionStatus, base.Assessment}, base.Code
			cache.stepFacts = append([]byte(nil), wire.StepFacts...)
		}
		out.Records[record.Case] = ordinaryGuardedRecord{Record: record, Transport: wire.Transport, StepFacts: wire.StepFacts, Projection: projection}
	}
	if e := b.walkSemanticSet(physical.FS(), listed, add); e != nil {
		return stop(e)
	}
	if e := b.semanticSnapshotIdentity(root, physical, s.snapshot); e != nil {
		return stop(e)
	}
	if out.Assessment != AssessFail {
		out.Assessment = AssessBlocked
	}
	out.Complete = true
	if e := b.r.check(); e != nil {
		return stop(e)
	}
	return stop(nil)
}

type ordinaryFrameLocation struct {
	owner        string
	batch, frame int
	capture      *ordinaryGuardedCapture
	batchCache   *ordinaryBatchCache
}

func ordinaryPinMatchesBytes(pin OrdinaryPin, data []byte) bool {
	return ordinaryPin(pin) && pin.Bytes == uint64(len(data)) && pin.SHA256 == digestHex(data)
}

func ordinaryRunBindingMatchesManifest(b OrdinaryRunBinding, m OrdinaryOracleManifest) bool {
	return b.Context == m.Context && b.Profile == m.Profile && b.PlanningProfile == m.PlanningProfile && b.Registration == m.Registration && b.Assets == m.Assets && b.Plan == m.Plan && b.Candidate == m.Candidate && b.SemanticCohort == m.SemanticCohort && b.RunID == m.RunID && b.ProducerOwner == m.ProducerOwner && b.FullScopeComplete == m.FullScopeComplete && slices.Equal(b.ShardIDs, m.ShardIDs) && reflect.DeepEqual(b.Members, m.Members) && reflect.DeepEqual(b.Batches, m.Batches)
}

func ordinaryRunBindingMatchesEligibility(b OrdinaryRunBinding, e ordinaryEligibilityBinding) bool {
	return b.Context == e.Context && b.Profile == e.Profile && b.PlanningProfile == e.PlanningProfile && b.Registration == e.Registration && b.Assets == e.Assets && b.Plan == e.Plan && b.Candidate == e.Candidate && b.SemanticCohort == e.SemanticCohort && b.RunID == e.RunID && b.ProducerOwner == "O" && slices.Equal(b.ShardIDs, e.ShardIDs)
}

func (b *eligibilityBudget) ordinaryFinalComplete(doc string, data []byte) *Error {
	if len(data) == 0 || len(data) > MaxDocumentBytes {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if e := b.charge(uint64(len(data)), uint64(len(data))+32768, doc+"/complete-marker"); e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	last, complete := "", false
	for {
		kind, count := dec.StackIndex(dec.StackDepth())
		depth := dec.StackDepth()
		tok, err := dec.ReadToken()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fail(KindInvalidInput, "JSON_SYNTAX", doc, err)
		}
		if depth == 1 && kind == '{' && count%2 == 0 && tok.Kind() == '"' {
			last, complete = tok.String(), false
		} else if depth == 1 && kind == '{' && count%2 == 1 && tok.Kind() != '}' && tok.Kind() != ']' {
			complete = last == "complete" && tok.Kind() == 't'
		}
		if e := b.r.check(); e != nil {
			return e
		}
	}
	if last != "complete" || !complete {
		return fail(KindInvalidInput, "SET_INCOMPLETE", doc, nil)
	}
	return nil
}

func (b *eligibilityBudget) ordinaryMemberBytes(s *eligibilitySession, m OrdinaryMember) ([]byte, bool, *Error) {
	path := "oracle/" + m.Path
	pin, ok := s.owned[path]
	if !ok || pin.NodeID != s.owners[path] || pin.Member != (ReplayMember{Path: path, Role: m.Role, Bytes: m.Pin.Bytes, SHA256: m.Pin.SHA256}) {
		return nil, false, nil
	}
	return s.member(&pin)
}

func ordinaryCheckManifestMembers(m OrdinaryOracleManifest, registration OrdinaryRegistration, profile OracleProfile, members map[string]OrdinaryMember, add func(string, string, string)) *Error {
	if len(m.Cases) == 0 || len(m.Cases) != len(profile.Native.Cases) || len(m.Cases) != len(m.ShardIDs) {
		return fail(KindInvalidInput, "ORDINARY_CASE_COUNT", "manifest.json", nil)
	}
	cases := map[string]bool{}
	for i, c := range m.Cases {
		if c == "" || cases[c] || profile.Native.Cases[i].ID != c || m.ShardIDs[i] != ordinaryPlanShardKey(registration.Route, registration.Platform, c) {
			add("ORDINARY_CASE_ORDER", c, "manifest case와 profile·shard 순서가 다르다")
			return fail(KindInvalidInput, "ORDINARY_CASE_ORDER", "manifest.json", nil)
		}
		cases[c] = true
	}
	caseRoles, batchRoles := map[string]map[string]int{}, map[string]map[string]int{}
	for _, member := range m.Members {
		if member.Owner == "O" && member.CaseOrBatch != "run" {
			if !cases[member.CaseOrBatch] {
				add("ORDINARY_CASE_MEMBER_OWNER", member.Path, "case member가 등록 case를 소유하지 않는다")
			}
			if caseRoles[member.CaseOrBatch] == nil {
				caseRoles[member.CaseOrBatch] = map[string]int{}
			}
			caseRoles[member.CaseOrBatch][member.Role]++
		}
		if strings.HasPrefix(member.Owner, "D_batch_") {
			if batchRoles[member.Owner] == nil {
				batchRoles[member.Owner] = map[string]int{}
			}
			batchRoles[member.Owner][member.Role]++
		}
		if member.Owner == "O" && member.CaseOrBatch == "run" && member.Role != "fact-pack" && member.Role != "fact-mapping" {
			add("ORDINARY_RUN_MEMBER_ROLE", member.Path, "run owner가 아닌 fact pack/mapping role이 사용됐다")
		}
		if member.Role == "raw" || member.Role == "projection" || member.Role == "record" || member.Role == "source" || member.Role == "source-witness" || member.Role == "expected-tree" || member.Role == "expected-captures" || member.Role == "expected-facts" || member.Role == "metadata" {
			if member.Owner != "O" || !cases[member.CaseOrBatch] {
				add("ORDINARY_CASE_MEMBER_OWNER", member.Path, "ordinary case 자산 owner가 다르다")
			}
		}
	}
	for _, c := range m.Cases {
		roles := caseRoles[c]
		if roles["source"]+roles["source-witness"] != 1 || roles["projection"] != 1 || roles["record"] != 1 || roles["raw"] > 1 {
			add("ORDINARY_CASE_MEMBER_CARDINALITY", c, "case source/projection/record/raw member 수가 다르다")
		}
	}
	for owner, roles := range batchRoles {
		if roles["native-batch-capture"] != 1 || roles["process"] != 1 || roles["stderr"] != 1 {
			add("ORDINARY_BATCH_OWNER_CARDINALITY", owner, "batch process owner member 수가 다르다")
		}
	}
	for _, query := range registration.Queries {
		if !ordinaryPin(query.Source) {
			add("ORDINARY_QUERY_BINDING", query.ID, "등록 query source pin이 유효하지 않다")
		}
	}
	if profile.Native.Operation != "native-query-batch" || !profile.API || len(profile.Queries) != len(registration.Queries) {
		add("ORDINARY_PROFILE_BINDING", "profile", "executable profile이 batch/API/query 계약과 다르다")
	}
	for i, q := range registration.Queries {
		if i >= len(profile.Queries) || profile.Queries[i].ID != q.ID || q.Source.SHA256 != digestHex([]byte(profile.Queries[i].Source)) || q.Source.Bytes != uint64(len(profile.Queries[i].Source)) {
			add("ORDINARY_QUERY_BINDING", q.ID, "query 순서 또는 원문 pin이 caller profile과 다르다")
		}
	}
	return nil
}

func (b *eligibilityBudget) ordinaryValidateBatchStreams(s *eligibilitySession, capture ordinaryGuardedCapture, captureMember OrdinaryMember, members map[string]OrdinaryMember, batch *ordinaryBatchCache, add func(string, string, string)) *Error {
	stdinHash, stdoutHash := sha256.New(), sha256.New()
	stdinOffset, stdoutOffset := uint64(0), uint64(0)
	for i, frame := range capture.Frames {
		var data ordinaryCapturedFrame
		if frame.Stdin.Kind() != 'n' {
			frameBytes, span, e := b.ordinaryReadFrameSpan(s, frame.Stdin, capture.Owner, members, false)
			if e != nil {
				return e
			}
			if span.Offset != stdinOffset {
				add("ORDINARY_BATCH_FRAME_OFFSET", captureMember.Path, "stdin span offset가 앞선 frame과 연속되지 않는다")
			}
			stdinOffset += span.Bytes
			_, _ = stdinHash.Write(frameBytes)
			data.Stdin = frameBytes
			if frame.State == "ANSWERED" && !span.Complete || frame.State == "SENT" && span.Complete {
				add("ORDINARY_BATCH_FRAME_STATE", captureMember.Path, "stdin 완료 상태가 frame state와 다르다")
			}
		} else if frame.State == "ANSWERED" || frame.State == "SENT" {
			add("ORDINARY_BATCH_FRAME_STATE", captureMember.Path, "보낸 frame의 stdin bytes가 없다")
		}
		if frame.Stdout.Kind() != 'n' {
			frameBytes, span, e := b.ordinaryReadFrameSpan(s, frame.Stdout, capture.Owner, members, true)
			if e != nil {
				return e
			}
			if span.Offset != stdoutOffset {
				add("ORDINARY_BATCH_FRAME_OFFSET", captureMember.Path, "stdout span offset가 앞선 frame과 연속되지 않는다")
			}
			stdoutOffset += span.Bytes
			_, _ = stdoutHash.Write(frameBytes)
			data.Stdout = frameBytes
			if frame.State == "ANSWERED" && !span.Complete {
				add("ORDINARY_BATCH_FRAME_STATE", captureMember.Path, "answered frame의 stdout이 불완전하다")
			}
		} else if frame.State == "ANSWERED" {
			add("ORDINARY_BATCH_FRAME_STATE", captureMember.Path, "answered frame의 stdout bytes가 없다")
		}
		batch.FrameBytes[i] = data
	}
	if capture.Stdin.RetainedBytes != stdinOffset || capture.Stdin.RetainedBytes > capture.Stdin.Bytes {
		batch.Accepted = false
		add("ORDINARY_BATCH_STDIN_STREAM", captureMember.Path, "stdin retained 길이가 실제 frame 합계와 다르다")
	} else if capture.Stdin.Bytes == capture.Stdin.RetainedBytes && hex.EncodeToString(stdinHash.Sum(nil)) != capture.Stdin.SHA256 {
		batch.Accepted = false
		add("ORDINARY_BATCH_STDIN_STREAM", captureMember.Path, "완전 보존된 stdin stream hash가 다르다")
	} else if capture.Stdin.Bytes != capture.Stdin.RetainedBytes {
		batch.Accepted = false
	}
	if capture.Suffix.Kind() != 'n' {
		suffixBytes, suffix, e := b.ordinaryReadFrameSpan(s, capture.Suffix, capture.Owner, members, true)
		if e != nil {
			return e
		}
		if suffix.Offset != stdoutOffset || suffix.Complete {
			add("ORDINARY_BATCH_SUFFIX", captureMember.Path, "stdout suffix offset 또는 상태가 다르다")
		}
		stdoutOffset += suffix.Bytes
		_, _ = stdoutHash.Write(suffixBytes)
	}
	if capture.Stdout.RetainedBytes != stdoutOffset || capture.Stdout.RetainedBytes > capture.Stdout.Bytes {
		batch.Accepted = false
		add("ORDINARY_BATCH_STDOUT_STREAM", captureMember.Path, "stdout retained 길이가 frame·suffix 합계와 다르다")
	} else if capture.Stdout.Bytes == capture.Stdout.RetainedBytes && hex.EncodeToString(stdoutHash.Sum(nil)) != capture.Stdout.SHA256 {
		batch.Accepted = false
		add("ORDINARY_BATCH_STDOUT_STREAM", captureMember.Path, "완전 보존된 stdout stream hash가 다르다")
	} else if capture.Stdout.Bytes != capture.Stdout.RetainedBytes {
		batch.Accepted = false
	}
	if capture.Accepted && (capture.Stdout.Bytes != uint64(sumFrameStdout(batch.FrameBytes)) || capture.TrailingBytes != 0 || capture.Suffix.Kind() != 'n') {
		batch.Accepted = false
		add("ORDINARY_BATCH_ACCEPTED", captureMember.Path, "accepted stream에 trailing/suffix bytes가 있다")
	}
	return nil
}

func sumFrameStdout(frames map[int]ordinaryCapturedFrame) int {
	n := 0
	for _, frame := range frames {
		n += len(frame.Stdout)
	}
	return n
}

func (b *eligibilityBudget) ordinaryCase(s *eligibilitySession, registration OrdinaryRegistration, profile OracleProfile, profileCase IncrementalCase, registered OrdinaryCase, loc ordinaryFrameLocation, members map[string]OrdinaryMember, manifest OrdinaryOracleManifest, add func(string, string, string)) (*ordinaryCaseCache, *Error) {
	cache := &ordinaryCaseCache{Owner: loc.owner, Batch: loc.batch, Frame: loc.frame, Case: profileCase.ID, BatchAccepted: loc.batchCache.Accepted}
	caseValid := true
	caseAdd := func(code, at, message string) { caseValid = false; add(code, at, message) }
	if len(loc.capture.Frames) <= loc.frame {
		caseAdd("ORDINARY_BATCH_FRAME", profileCase.ID, "case frame이 transcript에 없다")
		return cache, nil
	}
	frame := loc.capture.Frames[loc.frame]
	if frame.State != "ANSWERED" {
		return cache, nil
	}
	if len(registered.VersionPins) == 0 {
		caseAdd("ORDINARY_SOURCE_HISTORY", profileCase.ID, "registration에 source history가 없다")
		return cache, nil
	}
	sourceMembers := make([]OrdinaryMember, 0, 1)
	rawMembers := make([]OrdinaryMember, 0, 1)
	for _, member := range manifest.Members {
		if member.CaseOrBatch == profileCase.ID && member.Owner == "O" {
			switch member.Role {
			case "source", "source-witness":
				sourceMembers = append(sourceMembers, member)
			case "raw":
				rawMembers = append(rawMembers, member)
			}
		}
	}
	if len(sourceMembers) != 1 || len(rawMembers) != 1 {
		caseAdd("ORDINARY_CASE_MEMBER_CARDINALITY", profileCase.ID, "answered case에 source/raw member가 각각 하나 필요하다")
		return cache, nil
	}
	source, ok, e := b.ordinaryMemberBytes(s, sourceMembers[0])
	if e != nil {
		return nil, e
	}
	if !ok {
		caseAdd("ORDINARY_SOURCE_MISSING", profileCase.ID, "source bytes가 verified snapshot에 없다")
		return cache, nil
	}
	if uint64(len(source)) != registered.VersionPins[0].Bytes || digestHex(source) != registered.VersionPins[0].SHA256 || sourceMembers[0].Pin.Bytes != registered.VersionPins[0].Bytes || sourceMembers[0].Pin.SHA256 != registered.VersionPins[0].SHA256 || profileCase.Input.Bytes != registered.VersionPins[0].Bytes || profileCase.Input.SHA256 != registered.VersionPins[0].SHA256 {
		caseAdd("ORDINARY_SOURCE_BINDING", sourceMembers[0].Path, "source 원문이 source history·profile pin에 다르다")
	}
	cache.Source = source
	frameBytes, ok := loc.batchCache.FrameBytes[loc.frame]
	if !ok {
		caseAdd("ORDINARY_BATCH_FRAME", loc.owner, "검증한 frame cache가 없다")
		return cache, nil
	}
	stdin, stdout := frameBytes.Stdin, frameBytes.Stdout
	if len(stdin) < 4 || binary.BigEndian.Uint32(stdin[:4]) != uint32(len(stdin)-4) || len(stdout) < 4 || binary.BigEndian.Uint32(stdout[:4]) != uint32(len(stdout)-4) {
		caseAdd("ORDINARY_BATCH_FRAME_LENGTH", loc.capture.Owner, "request 또는 response frame header가 payload length와 다르다")
		return cache, nil
	}
	cache.RequestFrame, cache.ResponseFrame = stdin, stdout
	request, e := b.nativeRequestFrame("oracle/"+loc.capture.Owner+"/stdin-frame", stdin)
	if e != nil {
		if ordinaryInterruption(e) {
			return nil, e
		}
		caseAdd(e.Code, loc.capture.Owner, "actual request frame이 strict request schema를 만족하지 않는다")
		return cache, nil
	}
	cache.request = request
	rawData, ok, e := b.ordinaryMemberBytes(s, rawMembers[0])
	if e != nil {
		return nil, e
	}
	if !ok {
		caseAdd("ORDINARY_RAW_MISSING", rawMembers[0].Path, "native raw response bytes가 없다")
		return cache, nil
	}
	if !bytes.Equal(request.source, source) {
		caseAdd("ORDINARY_ACTUAL_REQUEST_SOURCE", profileCase.ID, "request의 base64 source가 source member와 다르다")
	}
	if !bytes.Equal(rawData, stdout[4:]) || rawMembers[0].Pin.Bytes != uint64(len(stdout)-4) || rawMembers[0].Pin.SHA256 != digestHex(stdout[4:]) {
		caseAdd("ORDINARY_BATCH_RAW_BINDING", rawMembers[0].Path, "raw member가 actual stdout frame payload와 다르다")
	}
	if e := b.decodeNativeResponse("oracle/"+rawMembers[0].Path, rawData, &cache.raw); e != nil {
		if ordinaryInterruption(e) {
			return nil, e
		}
		caseAdd(e.Code, rawMembers[0].Path, "native raw response가 strict typed schema를 만족하지 않는다")
		return cache, nil
	}
	versions, points, err := b.semanticVersions(request, NativeOperations()["native-query-batch"].InputBytes)
	if err != nil {
		if ordinaryInterruption(asError(err)) {
			return nil, asError(err)
		}
		caseAdd(asError(err).Code, profileCase.ID, "request history를 pinned source로 재구성할 수 없다")
		return cache, nil
	}
	cache.versions, cache.points = versions, points
	if e := s.nativeReservation(cache.raw, versions); e != nil {
		return nil, e
	}
	if !semanticEditsEqual(request.edits, registered.Edits) || !semanticEditsEqual(profileCase.Edits, registered.Edits) || !slices.Equal(points, registered.Points) || !slices.Equal(request.Points, profileCase.Points) || len(versions) != len(registered.VersionPins) {
		caseAdd("ORDINARY_HISTORY_BINDING", profileCase.ID, "request·profile·registration의 edit history 또는 points가 다르다")
	}
	for i, version := range versions {
		if i >= len(registered.VersionPins) || uint64(len(version)) != registered.VersionPins[i].Bytes || digestHex(version) != registered.VersionPins[i].SHA256 {
			caseAdd("ORDINARY_SOURCE_HISTORY", profileCase.ID, "재구성한 version이 pinned history와 다르다")
			break
		}
	}
	var declarations []NativeDeclaration
	if profile.Native.Declarations != nil {
		declarations = profile.Native.Declarations.Items
	}
	if request.ID != profileCase.ID || request.Encoding != EncodingUTF8 || request.Output != OutputTree || request.API != profile.API || len(request.Ranges) != 0 || request.ReuseObservation != reuseobs.Policy || request.ObservationRunID != manifest.RunID || request.ObservationProducerSHA256 != loc.capture.BuildIdentity || request.Protocol != nativewire.ProtocolR4 || len(request.Queries) != len(profile.Queries) || !slices.Equal(request.Declarations, declarations) || !ordinaryRequestLimits(request.Limits, manifest.Operation) {
		caseAdd("ORDINARY_ACTUAL_REQUEST", profileCase.ID, "request가 선택된 executable profile·batch policy·runtime에 다르다")
	}
	for i, query := range profile.Queries {
		if i >= len(request.queries) || request.queries[i].ID != query.ID || !bytes.Equal(request.queries[i].Source, []byte(query.Source)) || i >= len(registration.Queries) || registration.Queries[i].ID != query.ID || registration.Queries[i].Source.SHA256 != digestHex([]byte(query.Source)) || registration.Queries[i].Source.Bytes != uint64(len(query.Source)) {
			caseAdd("ORDINARY_ACTUAL_QUERIES", profileCase.ID, "request query 순서 또는 원문 pin이 다르다")
			break
		}
	}
	process := loc.capture.Process
	var proc ordinaryGuardedProcessResult
	procBytes, processOK, e := b.ordinaryMemberBytes(s, members[process.Path])
	if e != nil {
		return nil, e
	}
	if processOK {
		if e := b.decodeOrdinaryTyped("oracle/"+process.Path, procBytes, &proc); e != nil {
			return nil, e
		}
	} else {
		caseAdd("ORDINARY_BATCH_PROCESS", process.Path, "verified process result가 없다")
		return cache, nil
	}
	editPoints := make([]uint32, len(request.Points))
	for i, p := range request.Points {
		editPoints[i] = p.Byte
	}
	expect := nativewire.Expect{Check: func() error {
		if e := b.r.check(); e != nil {
			return e
		}
		return nil
	}, Revision: request.Protocol, ID: request.ID, Nodes: request.Limits.Nodes, Queries: request.queries, API: request.API, SourceBytes: uint64(len(source)), EditPoints: points, Exit: &proc.ExitCode,
		Observation: &nativewire.Binding{Manifest: reuseobs.RuntimeManifestSHA256, RunID: request.ObservationRunID, ProducerSHA256: request.ObservationProducerSHA256, HistoryStart: request.ObservationHistoryStart}}
	expect.Points = editPoints
	for _, edit := range request.edits {
		expect.Edits = append(expect.Edits, nativewire.EditSpan{StartByte: edit.StartByte, OldEndByte: edit.OldEndByte, NewEndByte: edit.NewEndByte})
	}
	for _, version := range versions {
		expect.Sources = append(expect.Sources, nativewire.SourceID{Bytes: uint64(len(version)), SHA256: digestHex(version)})
	}
	checked, checkErr := nativewire.Check(cache.raw, expect)
	if wireErr, ok := checkErr.(*Error); ok {
		return nil, wireErr
	}
	if checkErr != nil {
		caseAdd("ORDINARY_NATIVE_INVALID", rawMembers[0].Path, checkErr.Error())
	}
	cache.checked = checked.Steps
	cache.ready = caseValid && checkErr == nil && cache.raw.Status == StatusCompleted && len(cache.checked) == len(versions) && loc.batchCache.Accepted && proc.Status == "COMPLETED" && proc.ExitCode == 0 && proc.Cleanup.Verified && s.bound && s.res != nil && s.res.Assessment != AssessFail
	historyHash := ordinaryVersionPinsDigest(registered.VersionPins)
	cache.Key = ordinaryCaseCacheKey{ProfileSHA256: manifest.Profile.SHA256, Candidate: manifest.Candidate, Cohort: manifest.SemanticCohort, RunID: manifest.RunID, Owner: loc.owner, Batch: loc.batch, Frame: loc.frame, Case: profileCase.ID,
		RequestSHA256: digestHex(stdin), ResponseSHA256: digestHex(stdout), SourceHistorySHA256: historyHash, BuildIdentity: loc.capture.BuildIdentity, RuntimeSHA256: profile.Native.Compiler.SHA256, PolicySHA256: manifest.Policy.SHA256}
	return cache, nil
}

func ordinaryInterruption(e *Error) bool {
	return e != nil && e.Kind != KindInvalidInput && e.Kind != KindUnsupported
}

func (b *eligibilityBudget) ordinaryReadFrameSpan(s *eligibilitySession, raw jsontext.Value, owner string, members map[string]OrdinaryMember, stdout bool) ([]byte, *ordinaryGuardedSpan, *Error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var span ordinaryGuardedSpan
	if e := b.decodeOrdinaryTyped("ordinary-frame-span", raw, &span); e != nil {
		return nil, nil, e
	}
	if span.Bytes > OrdinaryPlanLimits().FileBytes || span.Bytes > uint64(^uint(0)>>1) || span.Members == nil {
		return nil, &span, fail(KindResourceLimit, "ORDINARY_FRAME_LIMIT", owner, nil)
	}
	var sum uint64
	for _, pin := range span.Members {
		if pin.Bytes > OrdinaryPlanLimits().FileBytes || sum > span.Bytes-pin.Bytes {
			return nil, &span, fail(KindInvalidInput, "ORDINARY_FRAME_SPAN", pin.Path, nil)
		}
		sum += pin.Bytes
	}
	if sum != span.Bytes {
		return nil, &span, fail(KindInvalidInput, "ORDINARY_FRAME_SPAN", owner, nil)
	}
	if e := b.charge(span.Bytes+1, 2*span.Bytes+32768, "ordinary-frame-span"); e != nil {
		return nil, &span, e
	}
	out := make([]byte, 0, int(span.Bytes))
	for _, pin := range span.Members {
		if !ordinaryCaptureMember(pin, owner, members) || stdout && pin.Role != "stdout-frame" || !stdout && pin.Role != "stdin-frame" {
			return nil, &span, fail(KindInvalidInput, "ORDINARY_FRAME_OWNER", pin.Path, nil)
		}
		member := members[pin.Path]
		data, ok, e := b.ordinaryMemberBytes(s, member)
		if e != nil {
			return nil, &span, e
		}
		if !ok || uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			return nil, &span, fail(KindInvalidInput, "ORDINARY_FRAME_PIN", pin.Path, nil)
		}
		out = append(out, data...)
	}
	if uint64(len(out)) != span.Bytes || digestHex(out) != span.SHA256 {
		return nil, &span, fail(KindInvalidInput, "ORDINARY_FRAME_HASH", owner, nil)
	}
	return out, &span, nil
}

func (b *eligibilityBudget) ordinaryRecordTransport(r OrdinaryOracleRecord, loc ordinaryFrameLocation, members map[string]OrdinaryMember, add func(string, string, string)) *Error {
	if string(r.Transport) == "null" {
		add("ORDINARY_RECORD_TRANSPORT", r.Case, "선택 ordinary case의 record transport가 null이다")
		return nil
	}
	var t ordinaryTransport
	if e := b.decodeOrdinaryTyped("ordinary-record-transport", r.Transport, &t); e != nil {
		return e
	}
	if loc.capture == nil || loc.frame < 0 || loc.frame >= len(loc.capture.Frames) || t.Batch != loc.batch || t.Frame != loc.frame || t.State != loc.capture.Frames[loc.frame].State || t.State == "NOT_SENT" {
		add("ORDINARY_RECORD_TRANSPORT", r.Case, "record transport가 batch/frame/state와 다르다")
		return nil
	}
	if r.QuerySemantics == nil || r.QuerySemantics.Capture == nil {
		add("ORDINARY_RECORD_CAPTURE", r.Case, "r5 record에 pinned transcript가 없다")
		return nil
	}
	capMember, ok := members[r.QuerySemantics.Capture.Path]
	if !ok || capMember.Owner != loc.owner || capMember.Role != "native-batch-capture" || !ordinaryReplayPinMatches(capMember, *r.QuerySemantics.Capture) {
		add("ORDINARY_RECORD_CAPTURE", r.Case, "r5 capture pin이 같은 batch owner가 아니다")
	}
	return nil
}

func locationOrEmpty(m map[string]ordinaryFrameLocation, c string) ordinaryFrameLocation { return m[c] }

func ordinaryRequestLimits(got semanticRequestLimits, op NativeOperation) bool {
	want := semanticRequestLimits{InputBytes: op.InputBytes, Nodes: op.Nodes, FullNodes: op.FullNodes, Depth: op.Depth, OutputBytes: op.OutputBytes, ParseMillis: op.ParseMillis, MemoryBytes: op.MemoryBytes, Errors: op.Errors, PartialNodes: op.PartialNodes, Matches: op.Matches, Captures: op.Captures, QueryMillis: op.QueryMillis}
	return got == want
}

func ordinaryVersionPinsDigest(pins []OrdinaryPin) string {
	h := sha256.New()
	var n [8]byte
	for _, pin := range pins {
		binary.BigEndian.PutUint64(n[:], uint64(len(pin.Path)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(pin.Path))
		binary.BigEndian.PutUint64(n[:], pin.Bytes)
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(pin.SHA256))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func containsString(xs []string, x string) bool { return slices.Contains(xs, x) }

// ordinaryMemberIndexReservation prices the exact-path, folded-path and member maps built from n manifest
// members whose paths total p bytes. It keeps both merged parents' terms: the case-index entries
// (2n+2 work, 128n+256 logical) and the lowered-key member index (n+4p+1 work, 1024(n+1)+4p logical),
// plus constant per-member map-operation visits.
func ordinaryMemberIndexReservation(n, p uint64) (work, logical uint64) {
	return 6*n + 4*p + 8, 1024*(n+1) + 128*n + 256 + 4*p
}
