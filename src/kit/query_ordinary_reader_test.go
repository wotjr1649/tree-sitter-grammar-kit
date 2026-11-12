package kit

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

type ordinaryReaderFixture struct {
	root         *os.Root
	run          *run
	b            *eligibilityBudget
	s            *eligibilitySession
	registration OrdinaryRegistration
	profile      OracleProfile
	manifest     []byte
	runBinding   []byte
}

func newOrdinaryReaderFixture(t *testing.T, caseIDs ...string) ordinaryReaderFixture {
	t.Helper()
	return newOrdinaryReaderFixtureEdited(t, func(string, map[string]any) {}, caseIDs...)
}

// newOrdinaryReaderFixtureEdited lets a test change the "process", "capture", "stderr" ("data")
// or "transport" document before it is serialized and pinned, so the members stay validly owned
// and pinned. A "layout" with "requeue" true first stores the case as the NOT_SENT tail of batch 0
// and answers it in batch 1, as runOracleBatchesCapture moves a not-sent tail to a new process.
func newOrdinaryReaderFixtureEdited(t *testing.T, edit func(doc string, v map[string]any), caseIDs ...string) ordinaryReaderFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "oracle"), 0700); err != nil {
		t.Fatalf("mkdir oracle: %#v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root: %#v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	t.Cleanup(cancel)
	r, runErr := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if runErr != nil {
		t.Fatalf("start run: %#v", runErr)
	}
	t.Cleanup(r.cancel)
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	op := NativeOperations()["native-query-batch"]
	caseID, queryID, query := "c1", "q1", "(source_file) @root"
	if len(caseIDs) != 0 {
		caseID = caseIDs[0]
	}
	source := []byte("select 1")
	profileBytes := []byte("executable profile pin")
	profilePin := OrdinaryPin{Path: "profile/executable.json", Bytes: uint64(len(profileBytes)), SHA256: digestHex(profileBytes)}
	planningPin := OrdinaryPin{Path: "profile/planning.json", Bytes: 1, SHA256: digestHex([]byte("p"))}
	registrationPin := OrdinaryPin{Path: "registration.json", Bytes: 1, SHA256: digestHex([]byte("r"))}
	assetsPin := OrdinaryPin{Path: "assets.json", Bytes: 1, SHA256: digestHex([]byte("a"))}
	planPin := OrdinaryPin{Path: "plan.json", Bytes: 1, SHA256: digestHex([]byte("l"))}
	queryPin := OrdinaryPin{Path: "source-witness/query-" + digestHex([]byte(query)) + ".utf8", Bytes: uint64(len(query)), SHA256: digestHex([]byte(query))}
	sourcePin := OrdinaryPin{Path: "source-witness/" + digestHex(source) + ".utf8", Bytes: uint64(len(source)), SHA256: digestHex(source)}
	operation := NativeOperations()["native-query-batch"]
	request := map[string]any{
		"protocol": nativewire.ProtocolR4, "id": caseID, "encoding": EncodingUTF8, "output": OutputTree,
		"limits":       map[string]any{"input_bytes": operation.InputBytes, "nodes": operation.Nodes, "full_nodes": operation.FullNodes, "depth": operation.Depth, "output_bytes": operation.OutputBytes, "parse_ms": operation.ParseMillis, "memory_bytes": operation.MemoryBytes, "errors": operation.Errors, "partial_nodes": operation.PartialNodes, "matches": operation.Matches, "captures": operation.Captures, "query_ms": operation.QueryMillis},
		"declarations": []any{}, "points": []any{}, "ranges": []any{}, "source": base64.StdEncoding.EncodeToString(source), "edits": []any{},
		"queries": []any{map[string]any{"id": queryID, "source": base64.StdEncoding.EncodeToString([]byte(query))}}, "api": true,
		"reuse_observation": reuseobs.Policy, "observation_run_id": "run-ordinary-test", "observation_producer_sha256": digestHex([]byte("build-identity")), "observation_history_start": 0,
	}
	requestBytes, _ := json.Marshal(request)
	requestFrame := ordinaryReaderFrame(requestBytes)
	api, predicates, symbolCount, fieldCount := "r1", "NOT_EVALUATED", uint32(1), uint32(0)
	response := map[string]any{
		"protocol": nativewire.ProtocolR4, "id": caseID, "status": "INVALID_REQUEST", "code": "INVALID_REQUEST", "complete": true,
		"source_bytes": len(source), "steps_completed": 0, "steps": []any{},
		"producer": map[string]any{"language_version": 14, "runtime_language_version": 14, "runtime_min_compatible": 14, "query": "r2", "api": api, "predicates": predicates, "symbol_count": symbolCount, "field_count": fieldCount, "reuse_observation": reusePolicyValue()},
	}
	responseBytes, _ := json.Marshal(response)
	responseFrame := ordinaryReaderFrame(responseBytes)
	process := map[string]any{"status": "COMPLETED", "reason": "EXITED", "exit_code": 2, "stdout_bytes": len(responseFrame), "stderr_bytes": 0, "stdout_truncated": false, "stderr_truncated": false, "wall_ms": 1, "cleanup": map[string]any{"verified": true, "residual_after_exit": 0, "method": "job", "scope": "process", "detail": ""}, "memory": map[string]any{}, "capabilities": map[string]any{}}
	edit("process", process)
	processBytes, _ := json.Marshal(process)
	layout := map[string]any{"requeue": false}
	edit("layout", layout)
	members := []OrdinaryMember{}
	files := map[string][]byte{}
	put := func(owner, rel, role, caseOrBatch string, data []byte) ReplayMember {
		p := filepath.Join(dir, "oracle", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatalf("mkdir member: %#v", err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatalf("write member: %#v", err)
		}
		pin := ReplayMember{Path: rel, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
		members = append(members, OrdinaryMember{Owner: owner, Path: rel, Role: role, CaseOrBatch: caseOrBatch, Pin: OrdinaryPin{Path: pin.Path, Bytes: pin.Bytes, SHA256: pin.SHA256}})
		files["oracle/"+rel] = data
		return pin
	}
	batches := []OrdinaryRunBatch{}
	if layout["requeue"] == true {
		tailOwner := "D_batch_00000"
		tailProcess, _ := json.Marshal(map[string]any{"status": "COMPLETED", "reason": "EXITED", "exit_code": 0, "stdout_bytes": 0, "stderr_bytes": 0, "stdout_truncated": false, "stderr_truncated": false, "wall_ms": 1, "cleanup": map[string]any{"verified": true, "residual_after_exit": 0, "method": "job", "scope": "process", "detail": ""}, "memory": map[string]any{}, "capabilities": map[string]any{}})
		tail := map[string]any{
			"schema": "tsgk-native-batch-capture/r1", "owner": tailOwner, "batch": 0, "build_identity": digestHex([]byte("build-identity")), "executable_sha256": digestHex([]byte("exe")), "executable_bytes": 3,
			"spec":    map[string]any{"Path": "C:/test/driver.exe", "Args": []string{}, "Dir": "C:/test", "Env": []string{}, "Stdin": nil, "Interactive": true, "StdoutBytes": operation.OutputBytes + 4, "StderrBytes": int64(65536), "Wall": int64(operation.BatchWall), "Grace": int64(timeSecond), "Memory": map[string]any{"Bytes": operation.MemoryBytes, "Hard": false}, "CgroupParent": ""},
			"process": put(tailOwner, "capture/tail-process.json", "process", tailOwner, tailProcess), "stderr": put(tailOwner, "capture/tail-stderr.bin", "stderr", tailOwner, []byte{}),
			"stdin":         map[string]any{"bytes": 0, "sha256": digestHex(nil), "retained_bytes": 0},
			"stdout":        map[string]any{"bytes": 0, "sha256": digestHex(nil), "retained_bytes": 0},
			"frames":        []any{map[string]any{"index": 0, "case_id": caseID, "state": "NOT_SENT", "status": "REQUEUED", "detail": "not sent", "wall_ms": 0, "stdin": nil, "stdout": nil}},
			"stdout_suffix": nil, "trailing_bytes": 0, "accepted": false, "code": "BATCH_CAPTURE_NOT_SENT", "capture_error": "",
		}
		edit("tail", tail)
		tailBytes, _ := json.Marshal(tail)
		batches = append(batches, OrdinaryRunBatch{Index: 0, Owner: tailOwner, Capture: put(tailOwner, "capture/tail-batch.json", "native-batch-capture", tailOwner, tailBytes), Cases: []string{caseID}})
	}
	batchIndex := len(batches)
	owner := fmt.Sprintf("D_batch_%05d", batchIndex)
	stderr := map[string]any{"data": []byte{}}
	edit("stderr", stderr)
	processPin := put(owner, "capture/process.json", "process", owner, processBytes)
	stderrPin := put(owner, "capture/stderr.bin", "stderr", owner, stderr["data"].([]byte))
	stdinPin := put(owner, "capture/request.bin", "stdin-frame", owner, requestFrame)
	stdoutPin := put(owner, "capture/response.bin", "stdout-frame", owner, responseFrame)
	span := func(offset uint64, data []byte, pin ReplayMember) map[string]any {
		return map[string]any{"offset": offset, "bytes": len(data), "sha256": digestHex(data), "complete": true, "members": []ReplayMember{pin}}
	}
	capture := map[string]any{
		"schema": "tsgk-native-batch-capture/r1", "owner": owner, "batch": batchIndex, "build_identity": digestHex([]byte("build-identity")), "executable_sha256": digestHex([]byte("exe")), "executable_bytes": 3,
		"spec":    map[string]any{"Path": "C:/test/driver.exe", "Args": []string{}, "Dir": "C:/test", "Env": []string{}, "Stdin": nil, "Interactive": true, "StdoutBytes": operation.OutputBytes + 4, "StderrBytes": int64(65536), "Wall": int64(operation.BatchWall), "Grace": int64(timeSecond), "Memory": map[string]any{"Bytes": operation.MemoryBytes, "Hard": false}, "CgroupParent": ""},
		"process": processPin, "stderr": stderrPin,
		"stdin":         map[string]any{"bytes": len(requestFrame), "sha256": digestHex(requestFrame), "retained_bytes": len(requestFrame)},
		"stdout":        map[string]any{"bytes": len(responseFrame), "sha256": digestHex(responseFrame), "retained_bytes": len(responseFrame)},
		"frames":        []any{map[string]any{"index": 0, "case_id": caseID, "state": "ANSWERED", "status": "COMPLETED", "detail": "", "wall_ms": 1, "stdin": span(0, requestFrame, stdinPin), "stdout": span(0, responseFrame, stdoutPin)}},
		"stdout_suffix": nil, "trailing_bytes": 0, "accepted": false, "code": "BATCH_CAPTURE_PROCESS_FAILED", "capture_error": "",
	}
	edit("capture", capture)
	capBytes, _ := json.Marshal(capture)
	capPin := put(owner, "capture/batch.json", "native-batch-capture", owner, capBytes)
	put("O", "source/00000-"+caseID+".bin", "source", caseID, source)
	put("O", "raw/00000-"+caseID+".json", "raw", caseID, responseBytes)
	projectionBytes := []byte(`{"id":"` + caseID + `","input":{"role":"","path":"source/` + caseID + `.bin","bytes":8,"sha256":"` + digestHex(source) + `"},"encoding":"utf8","execution_status":"FAILED","assessment":"BLOCKED","code":"INVALID_REQUEST"}`)
	projectionPin := put("O", "projection/00000-"+caseID+".json", "projection", caseID, projectionBytes)
	registrationSHA := registrationPin.SHA256
	pending := &OracleQuerySemantics{Phase: "PRODUCER_LOCAL", Claim: "BLOCKED", Code: "EXTERNAL_APPROVAL_PENDING", RegistrationSHA256: registrationSHA, Capture: &capPin, Projection: projectionPin}
	transportDoc := map[string]any{"batch": batchIndex, "frame": 0, "state": "ANSWERED", "frame_wall_ms": 1}
	edit("transport", transportDoc)
	transport, _ := json.Marshal(transportDoc)
	record := OrdinaryOracleRecord{Schema: OracleOrdinaryRecordSchema, Case: caseID, Workload: "ordinary-test", BaseProjection: projectionPin, QuerySemantics: pending, Transport: transport, StepFacts: json.RawMessage(`[]`), Complete: true}
	recordBytes, _ := json.Marshal(record)
	put("O", "records/00000-"+caseID+".json", "record", caseID, append(recordBytes, '\n'))
	batches = append(batches, OrdinaryRunBatch{Index: batchIndex, Owner: owner, Capture: capPin, Cases: []string{caseID}})
	shards := []string{ordinaryPlanShardKey("test-route", "windows-amd64", caseID)}
	manifest := OrdinaryOracleManifest{Schema: OracleOrdinaryManifestSchema, Context: OrdinaryContext, ClosureEncoding: "plain", Profile: profilePin, PlanningProfile: planningPin, Registration: registrationPin, Assets: assetsPin, Plan: planPin,
		Candidate: "candidate", SemanticCohort: "cohort", RunID: "run-ordinary-test", ProducerOwner: "O", ShardIDs: shards, FullScopeComplete: false,
		Policy: NativePolicy(op, OutputTree, nil), Operation: op, Producer: map[string]string{"profile_id": "ordinary-test", "build_identity": digestHex([]byte("build-identity")), "executable_sha256": digestHex([]byte("exe")), "platform": "windows/amd64"},
		ExecutionStatus: StatusFailed, Assessment: AssessBlocked, Cases: []string{caseID}, Members: members, Batches: batches, Complete: true}
	memberBinding := OrdinaryRunBinding{Schema: OracleOrdinaryBindingSchema, Context: OrdinaryContext, Profile: profilePin, PlanningProfile: planningPin, Registration: registrationPin, Assets: assetsPin, Plan: planPin, Candidate: manifest.Candidate, SemanticCohort: manifest.SemanticCohort, RunID: manifest.RunID, ProducerOwner: "O", ShardIDs: shards, FullScopeComplete: false, Members: members, Batches: batches}
	bindingBytes, _ := json.Marshal(memberBinding)
	manifestBytes, _ := json.Marshal(manifest)
	files["oracle/manifest.json"] = manifestBytes
	if err := os.WriteFile(filepath.Join(dir, "oracle", "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatalf("write manifest: %#v", err)
	}
	info, err := root.Stat("oracle")
	if err != nil {
		t.Fatalf("stat oracle dir: %#v", err)
	}
	snapshot := &evidenceSnapshot{graphBound: true, invalidNodes: map[string]bool{}, files: files, verifiedMembers: map[string]ReplayMember{}, observed: map[string]os.FileInfo{}, directories: map[string]os.FileInfo{"oracle": info}}
	owned, owners := map[string]eligibilityMember{}, map[string]string{}
	for _, m := range members {
		full := "oracle/" + m.Path
		node := "node-" + m.Owner
		pin := ReplayMember{Path: full, Role: m.Role, Bytes: m.Pin.Bytes, SHA256: m.Pin.SHA256}
		owned[full] = eligibilityMember{NodeID: node, Member: pin}
		owners[full] = node
		snapshot.verifiedMembers[full] = pin
		fi, err := root.Stat(full)
		if err != nil {
			t.Fatalf("stat member: %#v", err)
		}
		snapshot.observed[full] = fi
	}
	manifestReplay := ReplayMember{Path: "oracle/manifest.json", Role: "manifest", Bytes: uint64(len(manifestBytes)), SHA256: digestHex(manifestBytes)}
	owned[manifestReplay.Path] = eligibilityMember{NodeID: "node-O", Member: manifestReplay}
	owners[manifestReplay.Path] = "node-O"
	snapshot.verifiedMembers[manifestReplay.Path] = manifestReplay
	fi, err := root.Stat(manifestReplay.Path)
	if err != nil {
		t.Fatalf("stat manifest: %#v", err)
	}
	snapshot.observed[manifestReplay.Path] = fi
	ordinaryBinding := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "plain", Profile: profilePin, PlanningProfile: planningPin, Registration: registrationPin, Assets: assetsPin, Plan: planPin, Commitment: planPin, ReferenceSelection: OrdinaryPin{Path: "reference-selection.json", Bytes: 1, SHA256: digestHex([]byte("x"))}, RunBinding: OrdinaryPin{Path: "run-binding.json", Bytes: uint64(len(bindingBytes)), SHA256: digestHex(bindingBytes)}, Candidate: manifest.Candidate, SemanticCohort: manifest.SemanticCohort, RunID: manifest.RunID, ShardIDs: shards}
	ordinary := &ordinaryEligibilityScope{Binding: ordinaryBinding, ClosureEncoding: "plain", TupleRequired: 1}
	session := &eligibilitySession{r: r, b: b, snapshot: snapshot, owned: owned, owners: owners, ordinary: ordinary, bound: true, res: &QueryReferenceEligibilityResult{Report: newReport("ordinary fixture")}}
	ordinary.session = session
	profile := OracleProfile{Native: IncrementalProfile{SHA256: profilePin.SHA256, ID: "ordinary-test", Route: "test-route", Operation: "native-query-batch", Encoding: EncodingUTF8, Output: OutputTree, Declarations: &Declarations{Items: []NativeDeclaration{}}, Cases: []IncrementalCase{{ID: caseID, Encoding: EncodingUTF8, Input: NativeInput{Path: sourcePin.Path, Bytes: sourcePin.Bytes, SHA256: sourcePin.SHA256}, Edits: []Edit{}, Points: []NativePoint{}}}}, Queries: []OracleQuery{{ID: queryID, Source: query}}, API: true, OracleCases: []OracleCase{{IncrementalCase: IncrementalCase{ID: caseID, Encoding: EncodingUTF8, Input: NativeInput{Bytes: sourcePin.Bytes, SHA256: sourcePin.SHA256}, Edits: []Edit{}, Points: []NativePoint{}}}}}
	registration := OrdinaryRegistration{Context: OrdinaryContext, Route: "test-route", Set: "test-set", Platform: "windows-amd64", Subject: "windows/amd64", ClosureEncoding: "plain", Candidate: "candidate", SemanticCohort: "cohort",
		Cases: []OrdinaryCase{{ID: caseID, OriginalSelector: "selector", InitialSource: sourcePin, Edits: []Edit{}, VersionPins: []OrdinaryPin{sourcePin}, Points: []EditPoints{}}}, Queries: []OrdinaryQuery{{ID: queryID, Source: queryPin, MetadataAsset: "metadata"}}}
	return ordinaryReaderFixture{root: root, run: r, b: b, s: session, registration: registration, profile: profile, manifest: manifestBytes, runBinding: bindingBytes}
}

const timeSecond = 1000000000

func reusePolicyValue() map[string]string {
	return map[string]string{"policy": reuseobs.Policy, "runtime_manifest_sha256": reuseobs.RuntimeManifestSHA256}
}

func ordinaryReaderFrame(payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(payload)))
	copy(out[4:], payload)
	return out
}

func TestOrdinaryGuardedSetReusesSameSessionSnapshotAndBudget(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	t.Logf("manifest=%d binding=%d", len(f.manifest), len(f.runBinding))
	set, err := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
	if err != nil {
		t.Fatal(err)
	}
	if set.Assessment != AssessBlocked || !set.Complete || len(set.Findings) != 0 {
		t.Fatalf("unexpected stored-set result: %+v", set)
	}
	cache := set.Cases["c1"]
	if cache == nil || cache.raw.Status != "INVALID_REQUEST" || len(cache.versions) != 1 || len(cache.checked) != 0 || cache.ready {
		t.Fatalf("case cache did not preserve the one decoded native frame: %+v", cache)
	}
	if set.Batches["D_batch_00000"] == nil || len(set.Batches["D_batch_00000"].FrameBytes) != 1 {
		t.Fatalf("batch cache missing ordered frame: %+v", set.Batches)
	}
	if f.b.r != f.s.b.r || f.b.r != f.run {
		t.Fatal("reader detached from the eligibility run")
	}
}

func TestOrdinaryGuardedSetRejectsCrossCohortAndUnboundFallback(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	f.s.ordinary.Binding.SemanticCohort = "other-cohort"
	set, err := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
	if err != nil {
		t.Fatal(err)
	}
	if set.Assessment != AssessFail || set.FirstFailure == "" {
		t.Fatalf("cross-cohort stored set was accepted: %+v", set)
	}
	if _, err := f.b.ordinaryGuardedSet(f.root, &eligibilitySession{r: f.run, b: f.b, snapshot: f.s.snapshot}, f.registration, f.profile, f.manifest, f.runBinding); err == nil || asError(err).Code != "ORDINARY_READER_UNBOUND" {
		t.Fatalf("unbound reader had a fallback path: %v", err)
	}
}

func TestOrdinaryGuardedSetRejectsDuplicateFrameCase(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	var manifest OrdinaryOracleManifest
	if err := json.Unmarshal(f.manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Batches[0].Cases = []string{"c1", "c1"}
	mutated, _ := json.Marshal(manifest)
	// The session still owns the original bytes, so substituting a rehashed manifest is rejected.
	set, err := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, mutated, f.runBinding)
	if err == nil && (set == nil || set.Assessment != AssessFail) {
		t.Fatalf("mutable manifest substitute was accepted: %+v", set)
	}
}

func TestOrdinaryVersionPinDigestIsOrdered(t *testing.T) {
	a := []OrdinaryPin{{Path: "a", Bytes: 1, SHA256: digestHex([]byte("a"))}, {Path: "b", Bytes: 1, SHA256: digestHex([]byte("b"))}}
	if ordinaryVersionPinsDigest(a) == ordinaryVersionPinsDigest([]OrdinaryPin{a[1], a[0]}) {
		t.Fatal("source-history cache key ignored order")
	}
}

func TestOrdinaryGuardedSetRejectsProfileCaseCountBeforeIndex(t *testing.T) {
	f := newOrdinaryReaderFixture(t)
	f.profile.Native.Cases = nil
	set, e := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
	if e == nil || e.Code != "ORDINARY_CASE_COUNT" || set == nil || set.Assessment != AssessFail || len(set.Cases) != 0 {
		t.Fatalf("malformed profile case count was not rejected before indexing: error=%v", e)
	}
}

func TestOrdinaryFinalCompleteMarker(t *testing.T) {
	for _, data := range []string{`{"complete":true}`, `{"nested":{"complete":false},"complete":true}`} {
		if e := ordinaryTestBudget(t).ordinaryFinalComplete("test", []byte(data)); e != nil {
			t.Fatal(e)
		}
	}
	for _, data := range []string{`{"complete":false}`, `{"complete":true,"tail":0}`, `{"nested":{"complete":true}}`} {
		if e := ordinaryTestBudget(t).ordinaryFinalComplete("test", []byte(data)); e == nil {
			t.Fatalf("accepted nonfinal or nested completion: %s", data)
		}
	}
}

func ordinaryFindingCodes(set *ordinaryGuardedSet) []string {
	codes := []string{}
	for _, f := range set.Findings {
		codes = append(codes, f.Code)
	}
	return codes
}

// A pinned and owned transcript with more frames than its batch cases is a finding, not a panic.
func TestOrdinaryGuardedSetExtraCaptureFrameIsFinding(t *testing.T) {
	f := newOrdinaryReaderFixtureEdited(t, func(doc string, v map[string]any) {
		if doc == "capture" {
			frames := v["frames"].([]any)
			v["frames"] = append(frames, map[string]any{"index": 1, "case_id": "c2", "state": "NOT_SENT", "status": "REQUEUED", "detail": "not sent", "wall_ms": 0, "stdin": nil, "stdout": nil})
		}
	})
	var set *ordinaryGuardedSet
	var e *Error
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("an extra transcript frame panicked: %v", p)
			}
		}()
		set, e = f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
	}()
	if e != nil {
		t.Fatal(e)
	}
	if set.Assessment != AssessFail || set.FirstFailure != "ORDINARY_BATCH_CAPTURE" {
		t.Fatalf("extra frame: %s %s %v", set.Assessment, set.FirstFailure, ordinaryFindingCodes(set))
	}
}

// SENT means the response did not complete; the request may have been written whole (a frame
// watchdog or memory limit while waiting), in part, or not at all (a failed first write leaves no
// stdin span). ANSWERED still needs the whole request.
func TestOrdinaryGuardedSetSentFrameStdinCompletion(t *testing.T) {
	for _, c := range []struct {
		state, stdin string
		ok           bool
	}{{"SENT", "complete", true}, {"SENT", "partial", true}, {"SENT", "null", true}, {"ANSWERED", "partial", false}, {"ANSWERED", "null", false}} {
		f := newOrdinaryReaderFixtureEdited(t, func(doc string, v map[string]any) {
			switch doc {
			case "process":
				if c.state == "SENT" {
					v["stdout_bytes"] = 0
				}
			case "capture":
				frame := v["frames"].([]any)[0].(map[string]any)
				frame["state"] = c.state
				frame["stdin"].(map[string]any)["complete"] = c.stdin == "complete"
				if c.stdin == "null" {
					frame["stdin"] = nil
					v["stdin"] = map[string]any{"bytes": 0, "sha256": digestHex(nil), "retained_bytes": 0}
				}
				if c.state == "SENT" {
					frame["status"], frame["detail"], frame["stdout"] = "RESOURCE_LIMIT", "FRAME_TIME_LIMIT", nil
					v["stdout"] = map[string]any{"bytes": 0, "sha256": digestHex(nil), "retained_bytes": 0}
				}
			case "transport":
				v["state"] = c.state
			}
		})
		set, e := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
		if e != nil {
			t.Fatal(e)
		}
		stateFinding := slices.Contains(ordinaryFindingCodes(set), "ORDINARY_BATCH_FRAME_STATE")
		if c.ok && (set.Assessment != AssessBlocked || len(set.Findings) != 0) || !c.ok && (!stateFinding || set.Assessment != AssessFail) {
			t.Errorf("%s with %s stdin: %s %v", c.state, c.stdin, set.Assessment, ordinaryFindingCodes(set))
		}
	}
}

// A not-sent tail answered by a later process belongs to that later frame.
func TestOrdinaryGuardedSetRequeuedNotSentTail(t *testing.T) {
	for _, tailState := range []string{"NOT_SENT", "SENT"} {
		f := newOrdinaryReaderFixtureEdited(t, func(doc string, v map[string]any) {
			switch doc {
			case "layout":
				v["requeue"] = true
			case "tail":
				v["frames"].([]any)[0].(map[string]any)["state"] = tailState
			}
		})
		set, e := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
		if e != nil {
			t.Fatal(e)
		}
		cache := set.Cases["c1"]
		if tailState == "NOT_SENT" && (set.Assessment != AssessBlocked || len(set.Findings) != 0 || cache == nil || cache.Owner != "D_batch_00001" || cache.Batch != 1 || cache.raw.Status != "INVALID_REQUEST") {
			t.Errorf("requeued tail: %s %v", set.Assessment, ordinaryFindingCodes(set))
		}
		// a case sent by one process is never sent again
		if tailState == "SENT" && (set.Assessment != AssessFail || !slices.Contains(ordinaryFindingCodes(set), "ORDINARY_BATCH_FRAME")) {
			t.Errorf("case sent twice: %s %v", set.Assessment, ordinaryFindingCodes(set))
		}
	}
}

// Batch responses have no per-frame exit: a nonfatal response before a clean EOF exit 0, or a
// response before a later fatal frame's exit 3, is checked against the batch process separately.
func TestOrdinaryGuardedSetBatchExitIsNotFrameExit(t *testing.T) {
	for _, exit := range []int{0, 2, 3} {
		f := newOrdinaryReaderFixtureEdited(t, func(doc string, v map[string]any) {
			if doc == "process" {
				v["exit_code"] = exit
			}
		})
		set, e := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
		if e != nil {
			t.Fatal(e)
		}
		cache := set.Cases["c1"]
		if set.Assessment != AssessBlocked || len(set.Findings) != 0 || cache == nil || cache.ready || set.Batches["D_batch_00000"].Accepted {
			t.Errorf("exit %d: %s %v", exit, set.Assessment, ordinaryFindingCodes(set))
		}
	}
}

// The runner counts all stderr bytes but keeps only the spec cap; a truncated stream is not accepted.
func TestOrdinaryGuardedSetTruncatedStderr(t *testing.T) {
	const capBytes = 65536
	for _, c := range []struct {
		name               string
		observed, retained int
		truncated, ok      bool
	}{
		{"exact", 7, 7, false, true},
		{"truncated", capBytes + 1, capBytes, true, true},
		{"truncated-short-prefix", capBytes + 1, capBytes - 1, true, false},
		{"truncated-within-cap", capBytes, capBytes, true, false},
		{"untruncated-count-mismatch", capBytes + 1, capBytes, false, false},
	} {
		f := newOrdinaryReaderFixtureEdited(t, func(doc string, v map[string]any) {
			switch doc {
			case "process":
				v["stderr_bytes"], v["stderr_truncated"] = c.observed, c.truncated
			case "stderr":
				v["data"] = make([]byte, c.retained)
			}
		})
		set, e := f.b.ordinaryGuardedSet(f.root, f.s, f.registration, f.profile, f.manifest, f.runBinding)
		if e != nil {
			t.Fatal(e)
		}
		streamFinding := slices.Contains(ordinaryFindingCodes(set), "ORDINARY_BATCH_PROCESS_STREAM")
		if c.ok && (set.Assessment != AssessBlocked || len(set.Findings) != 0) || !c.ok && (!streamFinding || set.Assessment != AssessFail) || set.Batches["D_batch_00000"].Accepted {
			t.Errorf("%s: %s %v", c.name, set.Assessment, ordinaryFindingCodes(set))
		}
	}
}
