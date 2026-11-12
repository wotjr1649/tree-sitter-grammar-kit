package kit

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"
)

func ordinaryTestPin(path, role string, data []byte) ReplayMember {
	return ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
}

type ordinarySetFixtureFrame struct {
	Case    string
	NotSent bool
}

func ordinarySetFixture() (fstest.MapFS, OrdinaryOracleManifest, []byte) {
	return ordinarySetFixtureBatches([][]ordinarySetFixtureFrame{{{Case: "c1"}, {Case: "c2"}}})
}

// ordinarySetFixtureBatches writes one capture per batch. A NOT_SENT frame has no spans; the record
// of a case names its last answered frame, as runOracleBatchesCapture requeues a not-sent tail.
func ordinarySetFixtureBatches(layout [][]ordinarySetFixtureFrame) (fstest.MapFS, OrdinaryOracleManifest, []byte) {
	files := fstest.MapFS{}
	members := []OrdinaryMember{}
	put := func(owner, path, role, caseOrBatch string, data []byte) ReplayMember {
		files[path] = &fstest.MapFile{Data: data, Mode: 0444}
		pin := OrdinaryPin{Path: path, Bytes: uint64(len(data)), SHA256: digestHex(data)}
		members = append(members, OrdinaryMember{Owner: owner, Path: path, Role: role, CaseOrBatch: caseOrBatch, Pin: pin})
		return ordinaryTestPin(path, role, data)
	}
	batchCases := []string{}
	batches := []OrdinaryRunBatch{}
	transports := map[string]string{}
	capPins := map[string]ReplayMember{}
	rawByCase := map[string][]byte{}
	for b, layoutFrames := range layout {
		owner := fmt.Sprintf("D_batch_%05d", b)
		prefix := "capture/"
		if b > 0 {
			prefix = fmt.Sprintf("capture/%05d-", b)
		}
		process := put(owner, prefix+"process.json", "process", owner, []byte(`{"status":"COMPLETED"}`))
		stderr := put(owner, prefix+"stderr.bin", "stderr", owner, []byte{})
		frames := []any{}
		cases := []string{}
		answered := []string{}
		var stdinOffset, stdoutOffset uint64
		for i, f := range layoutFrames {
			c := f.Case
			cases = append(cases, c)
			if !slices.Contains(batchCases, c) {
				batchCases = append(batchCases, c)
			}
			if f.NotSent {
				frames = append(frames, map[string]any{"index": i, "case_id": c, "state": "NOT_SENT", "status": "REQUEUED", "detail": "not sent", "wall_ms": 0, "stdin": nil, "stdout": nil})
				continue
			}
			request := []byte(fmt.Sprintf(`{"case":"%s"}`, c))
			response := []byte(fmt.Sprintf(`{"id":"%s"}`, c))
			var inputFrame, outputFrame [4]byte
			binary.BigEndian.PutUint32(inputFrame[:], uint32(len(request)))
			binary.BigEndian.PutUint32(outputFrame[:], uint32(len(response)))
			input := append(inputFrame[:], request...)
			output := append(outputFrame[:], response...)
			rawByCase[c] = response
			inPin := put(owner, fmt.Sprintf("%sin-%d.bin", prefix, i), "stdin-frame", owner, input)
			outPin := put(owner, fmt.Sprintf("%sout-%d.bin", prefix, i), "stdout-frame", owner, output)
			frames = append(frames, map[string]any{
				"index": i, "case_id": c, "state": "ANSWERED", "status": "COMPLETED", "detail": "", "wall_ms": 1,
				"stdin":  map[string]any{"offset": stdinOffset, "bytes": len(input), "sha256": digestHex(input), "complete": true, "members": []ReplayMember{inPin}},
				"stdout": map[string]any{"offset": stdoutOffset, "bytes": len(output), "sha256": digestHex(output), "complete": true, "members": []ReplayMember{outPin}},
			})
			stdinOffset += uint64(len(input))
			stdoutOffset += uint64(len(output))
			transports[c] = fmt.Sprintf(`{"batch":%d,"frame":%d,"state":"ANSWERED","frame_wall_ms":1}`, b, i)
			answered = append(answered, c)
		}
		capDoc := map[string]any{
			"schema": "tsgk-native-batch-capture/r1", "owner": owner, "batch": b, "build_identity": "build",
			"executable_sha256": hex.EncodeToString(make([]byte, 32)), "executable_bytes": 1,
			"spec": map[string]any{}, "process": process, "stderr": stderr,
			"stdin":  map[string]any{"bytes": stdinOffset, "sha256": digestHex([]byte("requests")), "retained_bytes": stdinOffset},
			"stdout": map[string]any{"bytes": stdoutOffset, "sha256": digestHex([]byte("responses")), "retained_bytes": stdoutOffset},
			"frames": frames, "stdout_suffix": nil, "trailing_bytes": 0, "accepted": len(answered) == len(cases), "code": "", "capture_error": "",
		}
		capBytes, _ := json.Marshal(capDoc)
		capPin := put(owner, prefix+"batch.json", "native-batch-capture", owner, capBytes)
		for _, c := range answered {
			capPins[c] = capPin
		}
		batches = append(batches, OrdinaryRunBatch{Index: b, Owner: owner, Capture: capPin, Cases: cases})
	}
	projectionPins := map[string]ReplayMember{}
	shardIDs := []string{}
	for _, c := range batchCases {
		shardIDs = append(shardIDs, "shard-"+c)
		source := []byte(c + " source")
		projection := []byte(fmt.Sprintf(`{"id":%q,"execution_status":"COMPLETED","assessment":"PASS"}`, c))
		projectionPins[c] = put("O", "projection/"+c+".json", "projection", c, projection)
		put("O", "source/"+c+".bin", "source", c, source)
		put("O", "raw/"+c+".json", "raw", c, rawByCase[c])
	}
	registrationSHA := hex.EncodeToString(make([]byte, 32))
	for _, c := range batchCases {
		projection := projectionPins[c]
		capPin := capPins[c]
		pending := &OracleQuerySemantics{Phase: "PRODUCER_LOCAL", Claim: "BLOCKED", Code: "EXTERNAL_APPROVAL_PENDING", RegistrationSHA256: registrationSHA, Capture: &capPin, Projection: projection}
		record := OrdinaryOracleRecord{Schema: OracleOrdinaryRecordSchema, Case: c, Workload: "ordinary-test", BaseProjection: projection, QuerySemantics: pending, Transport: json.RawMessage(transports[c]), StepFacts: json.RawMessage(`[]`), Complete: true}
		data, _ := json.Marshal(record)
		put("O", "records/"+c+".json", "record", c, append(data, '\n'))
	}
	op := NativeOperations()["native-query-batch"]
	m := OrdinaryOracleManifest{
		Schema: OracleOrdinaryManifestSchema, Context: OrdinaryContext, ClosureEncoding: "plain",
		Profile:         OrdinaryPin{Path: "profile/executable.json", Bytes: 1, SHA256: registrationSHA},
		PlanningProfile: OrdinaryPin{Path: "profile/planning.json", Bytes: 1, SHA256: registrationSHA},
		Registration:    OrdinaryPin{Path: "registration.json", Bytes: 1, SHA256: registrationSHA},
		Assets:          OrdinaryPin{Path: "assets.json", Bytes: 1, SHA256: registrationSHA},
		Plan:            OrdinaryPin{Path: "plan.json", Bytes: 1, SHA256: registrationSHA},
		Candidate:       "candidate", SemanticCohort: "cohort", RunID: "run", ProducerOwner: "O",
		ShardIDs: shardIDs, FullScopeComplete: false,
		Policy: NativePolicy(op, OutputTree, nil), Operation: op,
		Producer: map[string]string{"profile_id": "ordinary-test"}, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked,
		Cases: batchCases, Members: members, Batches: batches, Complete: true,
	}
	data, _ := json.Marshal(m)
	files["manifest.json"] = &fstest.MapFile{Data: append(data, '\n'), Mode: 0444}
	return files, m, data
}

func TestOrdinaryOracleSetOwnersFramesAndPins(t *testing.T) {
	files, manifest, _ := ordinarySetFixture()
	if report := VerifyOracleSet(files); !report.Valid || report.Records != 2 || report.Assessment != AssessBlocked {
		t.Fatalf("valid producer-local ordinary set rejected: %+v", report)
	}

	var raw map[string]any
	manifestBytes := files["manifest.json"].Data
	if err := json.Unmarshal(manifestBytes, &raw); err != nil {
		t.Fatal(err)
	}
	mutate := func(name string, fn func(fstest.MapFS, map[string]any)) {
		t.Run(name, func(t *testing.T) {
			clone := fstest.MapFS{}
			for k, v := range files {
				clone[k] = &fstest.MapFile{Data: append([]byte(nil), v.Data...), Mode: v.Mode}
			}
			var m map[string]any
			_ = json.Unmarshal(manifestBytes, &m)
			fn(clone, m)
			delete(m, "complete")
			data, _ := json.Marshal(m)
			clone["manifest.json"].Data = append(data[:len(data)-1], []byte(",\"complete\":true}\n")...)
			if report := VerifyOracleSet(clone); report.Valid {
				t.Fatalf("accepted mutation; manifest fixture: %+v", manifest)
			}
		})
	}
	mutate("process-owner-copied-per-case", func(_ fstest.MapFS, m map[string]any) {
		members := m["members"].([]any)
		for _, value := range members {
			member := value.(map[string]any)
			if member["role"] == "process" {
				member["owner"], member["case_or_batch"] = "D_batch_00001", "D_batch_00001"
			}
		}
	})
	mutate("selected-scope-cannot-be-promoted", func(_ fstest.MapFS, m map[string]any) { m["full_scope_complete"] = true })
	mutate("declared-record-deleted", func(clone fstest.MapFS, m map[string]any) {
		for _, value := range m["members"].([]any) {
			member := value.(map[string]any)
			if member["role"] == "record" {
				delete(clone, member["path"].(string))
				return
			}
		}
		t.Fatal("fixture has no declared record")
	})
	mutate("span-allocation-is-bounded", func(clone fstest.MapFS, m map[string]any) {
		members := m["members"].([]any)
		for _, value := range members {
			member := value.(map[string]any)
			if member["role"] != "native-batch-capture" {
				continue
			}
			var capture map[string]any
			_ = json.Unmarshal(clone[member["path"].(string)].Data, &capture)
			frame := capture["frames"].([]any)[0].(map[string]any)
			frame["stdout"].(map[string]any)["bytes"] = float64(^uint64(0))
			data, _ := json.Marshal(capture)
			clone[member["path"].(string)].Data = data
			pin := member["pin"].(map[string]any)
			pin["bytes"], pin["sha256"] = float64(len(data)), digestHex(data)
			batch := m["batches"].([]any)[0].(map[string]any)
			batch["capture"].(map[string]any)["bytes"], batch["capture"].(map[string]any)["sha256"] = float64(len(data)), digestHex(data)
		}
	})
	mutate("raw-must-match-captured-response", func(clone fstest.MapFS, m map[string]any) {
		members := m["members"].([]any)
		for _, value := range members {
			member := value.(map[string]any)
			if member["role"] == "raw" && member["case_or_batch"] == "c1" {
				body := []byte(`{"id":"wrong"}`)
				clone[member["path"].(string)].Data = body
				pin := member["pin"].(map[string]any)
				pin["bytes"], pin["sha256"] = float64(len(body)), digestHex(body)
			}
		}
	})
}

func TestOrdinaryOracleSetRequiresProducerPendingAfterRecordRehash(t *testing.T) {
	files, manifest, _ := ordinarySetFixture()
	changed := false
	for i, member := range manifest.Members {
		if member.Role != "record" {
			continue
		}
		var record OrdinaryOracleRecord
		if err := json.Unmarshal(files[member.Path].Data, &record); err != nil {
			t.Fatal(err)
		}
		record.QuerySemantics = nil
		data := ordinaryJSON(t, record)
		files[member.Path].Data = data
		manifest.Members[i].Pin.Bytes, manifest.Members[i].Pin.SHA256 = uint64(len(data)), digestHex(data)
		changed = true
		break
	}
	if !changed {
		t.Fatal("fixture has no ordinary record")
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"].Data = append(manifestBytes, '\n')
	if result := VerifyOracleSet(files); result.Valid {
		t.Fatal("rehashed record without producer-local pending was accepted")
	}
}

func TestOrdinaryRunBindingMatchesCallerOutput(t *testing.T) {
	_, manifest, _ := ordinarySetFixture()
	binding := OrdinaryRunBinding{
		Schema: OracleOrdinaryBindingSchema, Context: manifest.Context, Profile: manifest.Profile,
		PlanningProfile: manifest.PlanningProfile, Registration: manifest.Registration, Assets: manifest.Assets, Plan: manifest.Plan,
		Candidate: manifest.Candidate, SemanticCohort: manifest.SemanticCohort, RunID: manifest.RunID,
		ProducerOwner: manifest.ProducerOwner, ShardIDs: append([]string(nil), manifest.ShardIDs...),
		Members: append([]OrdinaryMember(nil), manifest.Members...), Batches: append([]OrdinaryRunBatch(nil), manifest.Batches...),
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateOrdinaryRunBinding(data, manifest); err != nil {
		t.Fatal("valid caller binding rejected:", err)
	}
	binding.Members[0].Owner = "D_batch_00001"
	mutated, _ := json.Marshal(binding)
	if err := ValidateOrdinaryRunBinding(mutated, manifest); err == nil {
		t.Fatal("member owner mutation accepted")
	}
}

// runOracleBatchesCapture moves a NOT_SENT tail to a new process. Only a SENT or ANSWERED frame
// owns a case; the earlier NOT_SENT frame stays as a diagnostic.
func TestOrdinaryOracleSetBatchCaseOwnership(t *testing.T) {
	sent := func(c string) ordinarySetFixtureFrame { return ordinarySetFixtureFrame{Case: c} }
	notSent := func(c string) ordinarySetFixtureFrame { return ordinarySetFixtureFrame{Case: c, NotSent: true} }
	for _, c := range []struct {
		name   string
		layout [][]ordinarySetFixtureFrame
		valid  bool
	}{
		{"requeued-tail", [][]ordinarySetFixtureFrame{{sent("c1"), notSent("c2")}, {sent("c2")}}, true},
		{"requeued-twice", [][]ordinarySetFixtureFrame{{sent("c1"), notSent("c2")}, {notSent("c2")}, {sent("c2")}}, true},
		{"answered-twice", [][]ordinarySetFixtureFrame{{sent("c1"), sent("c2")}, {sent("c2")}}, false},
		{"not-sent-after-owner", [][]ordinarySetFixtureFrame{{sent("c1"), sent("c2")}, {notSent("c2")}}, false},
		{"not-sent-twice-in-one-batch", [][]ordinarySetFixtureFrame{{sent("c1"), notSent("c2"), notSent("c2")}, {sent("c2")}}, false},
	} {
		files, _, _ := ordinarySetFixtureBatches(c.layout)
		report := VerifyOracleSet(files)
		if report.Valid != c.valid || !c.valid && !slices.ContainsFunc(report.Findings, func(f Finding) bool { return f.Code == "ORDINARY_BATCH_CASE" }) {
			t.Errorf("%s: valid=%v findings=%+v", c.name, report.Valid, report.Findings)
		}
	}
}
