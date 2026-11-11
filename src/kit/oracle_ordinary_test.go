package kit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"
)

func ordinaryTestPin(path, role string, data []byte) ReplayMember {
	return ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
}

func ordinarySetFixture() (fstest.MapFS, OrdinaryOracleManifest, []byte) {
	files := fstest.MapFS{}
	members := []OrdinaryMember{}
	put := func(owner, path, role, caseOrBatch string, data []byte) ReplayMember {
		files[path] = &fstest.MapFile{Data: data, Mode: 0444}
		pin := OrdinaryPin{Path: path, Bytes: uint64(len(data)), SHA256: digestHex(data)}
		members = append(members, OrdinaryMember{Owner: owner, Path: path, Role: role, CaseOrBatch: caseOrBatch, Pin: pin})
		return ordinaryTestPin(path, role, data)
	}
	owner := "D_batch_00000"
	process := put(owner, "capture/process.json", "process", owner, []byte(`{"status":"COMPLETED"}`))
	stderr := put(owner, "capture/stderr.bin", "stderr", owner, []byte{})
	frames := []any{}
	batchCases := []string{"c1", "c2"}
	var stdinOffset, stdoutOffset uint64
	rawByCase := map[string][]byte{}
	for i, c := range batchCases {
		request := []byte(fmt.Sprintf(`{"case":"%s"}`, c))
		response := []byte(fmt.Sprintf(`{"id":"%s"}`, c))
		var inputFrame, outputFrame [4]byte
		binary.BigEndian.PutUint32(inputFrame[:], uint32(len(request)))
		binary.BigEndian.PutUint32(outputFrame[:], uint32(len(response)))
		input := append(inputFrame[:], request...)
		output := append(outputFrame[:], response...)
		rawByCase[c] = response
		inPin := put(owner, fmt.Sprintf("capture/in-%d.bin", i), "stdin-frame", owner, input)
		outPin := put(owner, fmt.Sprintf("capture/out-%d.bin", i), "stdout-frame", owner, output)
		frames = append(frames, map[string]any{
			"index": i, "case_id": c, "state": "ANSWERED", "status": "COMPLETED", "detail": "", "wall_ms": 1,
			"stdin":  map[string]any{"offset": stdinOffset, "bytes": len(input), "sha256": digestHex(input), "complete": true, "members": []ReplayMember{inPin}},
			"stdout": map[string]any{"offset": stdoutOffset, "bytes": len(output), "sha256": digestHex(output), "complete": true, "members": []ReplayMember{outPin}},
		})
		stdinOffset += uint64(len(input))
		stdoutOffset += uint64(len(output))
	}
	streamInHash := sha256.Sum256(nil)
	_ = streamInHash
	capDoc := map[string]any{
		"schema": "tsgk-native-batch-capture/r1", "owner": owner, "batch": 0, "build_identity": "build",
		"executable_sha256": hex.EncodeToString(make([]byte, 32)), "executable_bytes": 1,
		"spec": map[string]any{}, "process": process, "stderr": stderr,
		"stdin":  map[string]any{"bytes": stdinOffset, "sha256": digestHex([]byte("requests")), "retained_bytes": stdinOffset},
		"stdout": map[string]any{"bytes": stdoutOffset, "sha256": digestHex([]byte("responses")), "retained_bytes": stdoutOffset},
		"frames": frames, "stdout_suffix": nil, "trailing_bytes": 0, "accepted": true, "code": "", "capture_error": "",
	}
	capBytes, _ := json.Marshal(capDoc)
	capPin := put(owner, "capture/batch.json", "native-batch-capture", owner, capBytes)
	projectionPins := map[string]ReplayMember{}
	for _, c := range batchCases {
		source := []byte(c + " source")
		projection := []byte(fmt.Sprintf(`{"id":%q,"execution_status":"COMPLETED","assessment":"PASS"}`, c))
		projectionPins[c] = put("O", "projection/"+c+".json", "projection", c, projection)
		put("O", "source/"+c+".bin", "source", c, source)
		put("O", "raw/"+c+".json", "raw", c, rawByCase[c])
	}
	registrationSHA := hex.EncodeToString(make([]byte, 32))
	for i, c := range batchCases {
		projection := projectionPins[c]
		pending := &OracleQuerySemantics{Phase: "PRODUCER_LOCAL", Claim: "BLOCKED", Code: "EXTERNAL_APPROVAL_PENDING", RegistrationSHA256: registrationSHA, Capture: &capPin, Projection: projection}
		record := OrdinaryOracleRecord{Schema: OracleOrdinaryRecordSchema, Case: c, Workload: "ordinary-test", BaseProjection: projection, QuerySemantics: pending, Transport: json.RawMessage(fmt.Sprintf(`{"batch":0,"frame":%d,"state":"ANSWERED","frame_wall_ms":1}`, i)), StepFacts: json.RawMessage(`[]`), Complete: true}
		data, _ := json.Marshal(record)
		put("O", "records/"+c+".json", "record", c, append(data, '\n'))
	}
	op := NativeOperations()["native-query-batch"]
	batches := []OrdinaryRunBatch{{Index: 0, Owner: owner, Capture: capPin, Cases: batchCases}}
	m := OrdinaryOracleManifest{
		Schema: OracleOrdinaryManifestSchema, Context: OrdinaryContext, ClosureEncoding: "plain",
		Profile:         OrdinaryPin{Path: "profile/executable.json", Bytes: 1, SHA256: registrationSHA},
		PlanningProfile: OrdinaryPin{Path: "profile/planning.json", Bytes: 1, SHA256: registrationSHA},
		Registration:    OrdinaryPin{Path: "registration.json", Bytes: 1, SHA256: registrationSHA},
		Assets:          OrdinaryPin{Path: "assets.json", Bytes: 1, SHA256: registrationSHA},
		Plan:            OrdinaryPin{Path: "plan.json", Bytes: 1, SHA256: registrationSHA},
		Candidate:       "candidate", SemanticCohort: "cohort", RunID: "run", ProducerOwner: "O",
		ShardIDs: []string{"shard-c1", "shard-c2"}, FullScopeComplete: false,
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
			data, _ := json.Marshal(m)
			clone["manifest.json"].Data = append(data, '\n')
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
