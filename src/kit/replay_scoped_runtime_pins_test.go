package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func scopedPinRequest(t *testing.T, subject []byte, materials map[string][]byte, reader string) *EvidencePinsRequest {
	t.Helper()
	tuple, ok := materials["native/runtime-tuple.json"]
	if !ok {
		t.Fatal("runtime tuple missing")
	}
	var scopes runtimeReaders
	scopes.Schema = "tsgk-runtime-readers/r1"
	scopes.TupleSHA256 = digestHex(tuple)
	switch reader {
	case runtimeAcceptedReader:
		scopes.AcceptedReader = reader
	case runtimePreprocessCSTReader:
		scopes.PreprocessCSTReader = reader
	default:
		t.Fatalf("unknown test reader %q", reader)
	}
	materials = clonePinMaterials(materials)
	materials[runtimeReadersName] = eligibilityJSON(t, scopes)
	return scopedPinRequestWithMaterials(t, subject, materials)
}

func scopedPinRequestWithMaterials(t *testing.T, subject []byte, materials map[string][]byte) *EvidencePinsRequest {
	t.Helper()
	set := evidencePinSet{Schema: EvidencePinSetSchema, Reader: EvidencePinReader}
	for _, row := range runtimePinRolesR1 {
		if data, ok := materials[row.name]; ok {
			set.Materials = append(set.Materials, EvidencePinMaterial{Name: row.name, Role: row.role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
		}
	}
	if data, ok := materials[runtimeReadersName]; ok {
		set.Materials = append(set.Materials, EvidencePinMaterial{Name: runtimeReadersName, Role: "runtime-readers", Bytes: uint64(len(data)), SHA256: digestHex(data)})
	}
	req := &EvidencePinsRequest{Subject: append([]byte(nil), subject...), Materials: materials, PinSet: eligibilityJSON(t, set)}
	pinTestApprove(t, req)
	return req
}

func clonePinMaterials(materials map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(materials))
	for name, data := range materials {
		clone[name] = append([]byte(nil), data...)
	}
	return clone
}

func setScopedManifest(v any, manifest string) {
	switch value := v.(type) {
	case map[string]any:
		if _, ok := value["runtime_manifest_sha256"]; ok {
			value["runtime_manifest_sha256"] = manifest
		}
		if capability, ok := value["reuse_observation"].(map[string]any); ok {
			capability["runtime_manifest_sha256"] = manifest
		}
		for _, child := range value {
			setScopedManifest(child, manifest)
		}
	case []any:
		for _, child := range value {
			setScopedManifest(child, manifest)
		}
	}
}

func scopedAcceptedReplay(t *testing.T, label string) ReplayRequest {
	t.Helper()
	root, registrationBytes := acceptedReplayFixture(t, nil)
	var profile map[string]any
	if err := json.Unmarshal(registrationBytes, &profile); err != nil {
		t.Fatal(err)
	}
	profile["reducer"] = "native-result-r11"
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	for _, member := range profile["members"].([]any) {
		m := member.(map[string]any)
		path := filepath.Join(root, filepath.FromSlash(m["path"].(string)))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte(reuseobs.RuntimeManifestSHA256), []byte(tuple.ManifestSHA256))
		data = bytes.ReplaceAll(data, []byte(fxRuntime), []byte(tupleManifestCommit(t, materials)))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		m["bytes"], m["sha256"] = len(data), digestHex(data)
	}
	registrationBytes, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	return ReplayRequest{Root: root, Profile: registrationBytes, RuntimePins: scopedPinRequest(t, registrationBytes, materials, runtimeAcceptedReader)}
}

func tupleManifestCommit(t *testing.T, materials map[string][]byte) string {
	t.Helper()
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest.Commit
}

func scopedCSTReplay(t *testing.T, label string) ReplayRequest {
	t.Helper()
	source := filepath.Join("..", "testdata", "qualify-symbols", "s05-csharp-symbols-switch")
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	paths, err := testTreeFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	profileBytes, err := os.ReadFile(filepath.Join(root, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	var originalResult nativeResult
	resultData, err := os.ReadFile(filepath.Join(root, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(resultData, &originalResult); err != nil {
		t.Fatal(err)
	}
	var members []any
	for _, path := range paths {
		name, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		rel := filepath.ToSlash(name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte(reuseobs.RuntimeManifestSHA256), []byte(tuple.ManifestSHA256))
		data = bytes.ReplaceAll(data, []byte(originalResult.Build.RuntimeCommit), []byte(tupleManifestCommit(t, materials)))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		role := "retained"
		switch {
		case rel == "profile.json":
			role = "workload-profile"
		case rel == "result.json":
			role = "result"
		case strings.HasPrefix(rel, "responses/"):
			role = "response"
		case strings.HasPrefix(rel, "primary/responses/"):
			role = "primary-response"
		case strings.HasPrefix(rel, "sources/"):
			role = "case-source"
		}
		members = append(members, map[string]any{"path": rel, "role": role, "bytes": len(data), "sha256": digestHex(data)})
	}
	resultBytes, err := os.ReadFile(filepath.Join(root, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result nativeResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{"workload": digestHex(profileBytes), "policy": "", "operation": result.Operation.Name, "platform": result.Platform}
	for _, id := range result.Identities {
		if id.Role == "policy" {
			ids["policy"] = id.SHA256
		}
	}
	ids["producer"], ids["executable"], ids["compiler"], ids["runtime"] = result.Build.Identity, result.Build.ExecutableSHA256, result.Build.Compiler.SHA256, result.Build.RuntimeCommit
	ids["runtime"] = tupleManifestCommit(t, materials)
	records := registration("native-result-r12", "evidence-replay", result.Assessment, ids, nil, members)
	registrationBytes, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	return ReplayRequest{Root: root, Profile: registrationBytes, RuntimePins: scopedPinRequest(t, registrationBytes, materials, runtimePreprocessCSTReader)}
}

func scopedOracleAcceptedReplay(t *testing.T, label string) ReplayRequest {
	t.Helper()
	accepted, raw, projection := acceptedFixture(t, 12)
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	op := NativeOperations()["native-query"]
	accepted.Policy = NativePolicy(op, OutputTree, nil)
	accepted.Expected.Revision = nativewire.ProtocolR4
	accepted.Expected.Observation.Manifest = tuple.ManifestSHA256
	raw["protocol"] = nativewire.ProtocolR4
	setScopedManifest(raw, tuple.ManifestSHA256)
	setScopedManifest(projection, tuple.ManifestSHA256)
	setOracleR4Producer(t, raw["producer"].(map[string]any))
	setOracleR4Producer(t, projection["producer"].(map[string]any))
	for _, value := range raw["steps"].([]any) {
		step := value.(map[string]any)
		for _, slot := range []string{"incremental", "fresh"} {
			if tree, ok := step[slot].(map[string]any); ok {
				tree["queries"], tree["api"] = nil, nil
			}
		}
	}
	for _, value := range projection["steps"].([]any) {
		step := value.(map[string]any)
		for _, slot := range []string{"incremental", "fresh"} {
			if tree, ok := step[slot].(map[string]any); ok {
				identityTree := tree["tree"].(map[string]any)
				identityTree["input"].(map[string]any)["encoding_source"] = SourceDeclaration
				for _, value := range identityTree["identities"].([]any) {
					identity := value.(map[string]any)
					if identity["role"] == "policy" {
						identity["schema"], identity["sha256"] = accepted.Policy.Schema, accepted.Policy.SHA256
					}
				}
			}
		}
	}
	projection["reuse_observation_claim"] = claimPass
	projection["reuse_accepted"] = map[string]any{}
	projection["claims"] = map[string]string{"incremental_equality": claimPass, "incremental_route": claimPass, "expectations": claimNotClaimed}
	projection["expectations"] = []any{}
	projection["assessment"], projection["code"] = AssessPass, ""
	caseInput := accepted.Case.Input
	caseInput.Role = "case"
	accepted.Case.Input = caseInput
	projection["input"] = caseInput
	setAcceptedEvidence(&accepted, raw, projection)
	accepted.RuntimePins = scopedPinRequest(t, accepted.Raw, materials, runtimeAcceptedReader)
	proof, err := JudgeReuseAccepted(accepted)
	if err != nil || proof.Result != claimPass {
		t.Fatalf("oracle accepted proof fixture: %+v %v", proof, err)
	}
	proof.Schema, proof.RuntimePins = ReuseAcceptedSchema, nil
	projection["reuse_accepted"] = proof
	setAcceptedEvidence(&accepted, raw, projection)
	accepted.RuntimePins = scopedPinRequest(t, accepted.Raw, materials, runtimeAcceptedReader)

	profile := newFxNative().profile
	profile["schema"], profile["id"], profile["route"], profile["operation"] = OracleSchema, "accepted-oracle", accepted.Route, "native-query"
	profile["output"], profile["encoding"] = OutputTree, EncodingUTF8
	profile["cases"] = []any{map[string]any{"id": accepted.Case.ID, "input": caseInput, "edits": accepted.Case.Edits, "points": []any{}, "expect": []any{}, "query_expect": []any{}, "dynamic_sql_expect": nil}}
	profile["queries"], profile["fact_pack"], profile["api"] = []OracleQuery{}, nil, false
	profileBytes, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	ledgerBytes, _ := acceptedLedgerFixture(t)
	caseID := accepted.Case.ID
	stem := "00000-" + caseID
	recordBody, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	record := append([]byte(`{"schema":"`+OracleAcceptedRecordSchema+`","case":"`+caseID+`","workload":"accepted-oracle",`), recordBody[1:len(recordBody)-1]...)
	record = append(record, []byte(`,"complete":true}`)...)
	manifest := OracleManifest{Schema: OracleAcceptedManifestSchema,
		Operation: &op, ReuseAcceptedPolicy: ReuseAcceptedSchema, ReuseAcceptedLedgerSHA256: ReuseAcceptedLedgerSHA256,
		Workload: map[string]string{"profile_id": "accepted-oracle", "profile_sha256": digestHex(profileBytes), "route": accepted.Route, "operation": "native-query", "output": OutputTree},
		Producer: map[string]string{"build_identity": accepted.Expected.Observation.ProducerSHA256, "executable_sha256": fxExe, "compiler_sha256": fxCompiler, "compiler_version": "cc 1", "runtime_commit": tupleManifestCommit(t, materials), "platform": accepted.Platform, "observation_run_id": accepted.Expected.Observation.RunID},
		Policy:   accepted.Policy, Protocol: nativewire.ProtocolR4,
		Comparators: []string{"tsgk-tree-digest/r1", "tsgk-capture-compare/r1", "tsgk-api/r1", "tsgk-predicates/r1"}, Queries: []IdentityRef{},
		ExecutionStatus: StatusCompleted, Assessment: AssessPass, Records: 1, Cases: []string{caseID}, Complete: true}
	files := map[string][]byte{"workload.json": profileBytes, "manifest.json": nil, "records/" + stem + ".json": record, "raw/" + stem + ".json": accepted.Raw,
		"source/" + stem + ".bin": accepted.Source, "reuse-ledger.json": ledgerBytes}
	input := accepted.Case.Input
	manifest.Members = []OracleMember{
		{Path: "raw/" + stem + ".json", Role: "raw", Case: caseID, Bytes: uint64(len(files["raw/"+stem+".json"])), SHA256: digestHex(files["raw/"+stem+".json"]), InputBytes: input.Bytes, InputSHA256: input.SHA256, ExecutionStatus: StatusCompleted},
		{Path: "records/" + stem + ".json", Role: "record", Case: caseID, Bytes: uint64(len(record)), SHA256: digestHex(record), InputBytes: input.Bytes, InputSHA256: input.SHA256, ExecutionStatus: StatusCompleted},
		{Path: "source/" + stem + ".bin", Role: "source", Case: caseID, Bytes: input.Bytes, SHA256: input.SHA256, InputBytes: input.Bytes, InputSHA256: input.SHA256, ExecutionStatus: StatusCompleted},
		{Path: "reuse-ledger.json", Role: "reuse-ledger", Bytes: uint64(len(ledgerBytes)), SHA256: digestHex(ledgerBytes), InputBytes: uint64(len(ledgerBytes)), InputSHA256: digestHex(ledgerBytes), ExecutionStatus: StatusCompleted},
	}
	files["manifest.json"], err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	members := writeSet(t, root, files, func(path string) string {
		switch {
		case path == "manifest.json":
			return "manifest"
		case path == "workload.json":
			return "workload-profile"
		case path == "reuse-ledger.json":
			return "reuse-ledger"
		case strings.HasPrefix(path, "records/"):
			return "record"
		case strings.HasPrefix(path, "raw/"):
			return "raw"
		case strings.HasPrefix(path, "source/"):
			return "case-source"
		default:
			return "retained"
		}
	})
	queries := ""
	comparators := strings.Join(manifest.Comparators, ",")
	ids := map[string]string{"workload": digestHex(profileBytes), "operation": "native-query", "producer": accepted.Expected.Observation.ProducerSHA256,
		"executable": fxExe, "compiler": fxCompiler, "runtime": tupleManifestCommit(t, materials), "platform": accepted.Platform, "policy": accepted.Policy.SHA256,
		"protocol": nativewire.ProtocolR4, "comparators": comparators, "queries": queries, "fact_pack": ""}
	registrationBytes, err := json.Marshal(registration("oracle-set-r8", "evidence-replay", AssessPass, ids, nil, members))
	if err != nil {
		t.Fatal(err)
	}
	return ReplayRequest{Root: root, Profile: registrationBytes, RuntimePins: scopedPinRequest(t, registrationBytes, materials, runtimeAcceptedReader)}
}

func setOracleR4Producer(t *testing.T, producer map[string]any) {
	t.Helper()
	producer["query"], producer["api"], producer["predicates"] = "tsgk-query/r1", "tsgk-api/r1", "NOT_EVALUATED"
	producer["symbol_count"], producer["field_count"] = uint32(25), uint32(3)
}

func testTreeFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func TestReplayScopedRuntimePinsAcceptedProductionIndependence(t *testing.T) {
	req := scopedAcceptedReplay(t, "accepted-A")
	want, err := Replay(pinTestContext(t), req)
	if err != nil || !want.EvidenceValid || want.ResultSchema != ReplayScopedRuntimeResultSchema || !validPassedScopedRuntime(want.RuntimePins, digestHex(req.Profile), runtimeAcceptedReader) || !validScopedRuntimeReplayResult(want) {
		t.Fatalf("historical accepted replay: err=%v evidence=%v schema=%s assessment=%s recorded=%+v recomputed=%+v subject=%+v gates=%+v runtime=%+v logger-valid=%v scoped-valid=%v envelope-valid=%v", err, want.EvidenceValid, want.ResultSchema, want.Assessment, want.Recorded, want.Recomputed, want.Subject, want.Gates, want.RuntimePins, validPassedLoggerRuntime(want.RuntimePins, digestHex(req.Profile)), validPassedScopedRuntime(want.RuntimePins, digestHex(req.Profile), runtimeAcceptedReader), validScopedRuntimeReplayResult(want))
	}
	if gateOf(want, "reuse-accepted").Failed != 0 || want.Recomputed.Assessment != AssessPass {
		t.Fatalf("accepted gate: %+v", want.Gates)
	}
	checkScopedStoredReplay(t, want, runtimeAcceptedReader)
	for _, label := range []string{"accepted-B", "accepted-C"} {
		production := scopedPinRequest(t, req.Profile, loggerPinMaterials(t, label), runtimeAcceptedReader)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": production.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, production.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := Replay(pinTestContext(t), req)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("accepted production %s changed A: %+v %v", label, got, err)
		}
	}
}

func TestReplayScopedRuntimePinsCSTProductionIndependence(t *testing.T) {
	req := scopedCSTReplay(t, "cst-A")
	want, err := Replay(pinTestContext(t), req)
	if err != nil || want.ResultSchema != ReplayScopedRuntimeResultSchema || !want.EvidenceValid || !validPassedScopedRuntime(want.RuntimePins, digestHex(req.Profile), runtimePreprocessCSTReader) || !validScopedRuntimeReplayResult(want) || want.Assessment != AssessBlocked || want.Recomputed != want.Recorded {
		t.Fatalf("historical preprocess-CST replay: err=%v schema=%s runtime=%+v logger-valid=%v scoped-valid=%v", err, want.ResultSchema, want.RuntimePins, validPassedLoggerRuntime(want.RuntimePins, digestHex(req.Profile)), validPassedScopedRuntime(want.RuntimePins, digestHex(req.Profile), runtimePreprocessCSTReader))
	}
	if len(want.PreprocessCST) != 1 || want.PreprocessCST[0].Result.Result != "PASS" {
		t.Fatalf("preprocess-CST gate: %+v", want.PreprocessCST)
	}
	if !want.EvidenceValid || !validScopedRuntimeReplayResult(want) {
		t.Fatalf("stored preprocess-CST replay invariant: evidence=%v valid=%v assessment=%s recomputed=%+v", want.EvidenceValid, validScopedRuntimeReplayResult(want), want.Assessment, want.Recomputed)
	}
	checkScopedStoredReplay(t, want, runtimePreprocessCSTReader)
	for _, label := range []string{"cst-B", "cst-C"} {
		production := scopedPinRequest(t, req.Profile, loggerPinMaterials(t, label), runtimePreprocessCSTReader)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": production.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, production.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := Replay(pinTestContext(t), req)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("CST production %s changed A: %+v %v", label, got, err)
		}
	}
}

func TestReplayScopedRuntimePinsOracleAcceptedProductionIndependence(t *testing.T) {
	req := scopedOracleAcceptedReplay(t, "oracle-accepted-A")
	want, err := Replay(pinTestContext(t), req)
	if err != nil || want.ResultSchema != ReplayScopedRuntimeResultSchema || !validPassedScopedRuntime(want.RuntimePins, digestHex(req.Profile), runtimeAcceptedReader) || !want.EvidenceValid || !validScopedRuntimeReplayResult(want) {
		t.Fatalf("historical oracle accepted replay: err=%v evidence=%v assessment=%s schema=%s findings=%+v gates=%+v runtime=%+v", err, want.EvidenceValid, want.Assessment, want.ResultSchema, want.Findings, want.Gates, want.RuntimePins)
	}
	if gateOf(want, "reuse-accepted").Failed != 0 {
		t.Fatalf("oracle accepted gate: %+v", want.Gates)
	}
	checkScopedStoredReplay(t, want, runtimeAcceptedReader)
	for _, label := range []string{"oracle-accepted-B", "oracle-accepted-C"} {
		production := scopedPinRequest(t, req.Profile, loggerPinMaterials(t, label), runtimeAcceptedReader)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": production.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, production.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := Replay(pinTestContext(t), req)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("oracle accepted production %s changed A: %+v %v", label, got, err)
		}
	}
}

func checkScopedStoredReplay(t *testing.T, result ReplayResult, reader string) {
	t.Helper()
	if !validScopedRuntimeReplayResult(result) {
		t.Fatalf("scoped saved replay result invalid: %s %+v", reader, result.RuntimePins)
	}
	for _, id := range result.Identities {
		if id.Role == "replay-profile" && result.RuntimePins.Pins.SubjectSHA256 != id.SHA256 {
			t.Fatalf("saved scope subject changed: %s", reader)
		}
	}
	pass := result
	if reader == runtimePreprocessCSTReader && pass.Assessment != AssessPass {
		pass.Subject.Assessment = AssessPass
		pass.Recorded = Verdict{pass.Subject.ExecutionStatus, AssessPass}
		pass.Recomputed = Verdict{StatusCompleted, AssessPass}
		pass.Assessment = AssessPass
		if !validScopedRuntimeReplayResult(pass) {
			t.Fatalf("synthetic stored PASS envelope rejected before CST mutations: %+v", pass.Gates)
		}
	}
	for _, gateID := range map[string][]string{
		runtimeAcceptedReader:      {"reuse-accepted"},
		runtimePreprocessCSTReader: {"preprocess", "preprocess-cst"},
	}[reader] {
		bad := pass
		bad.Gates = append([]ReplayGate(nil), pass.Gates...)
		index := -1
		for i := range bad.Gates {
			if bad.Gates[i].ID == gateID {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatalf("stored scoped gate missing: %s/%s", reader, gateID)
		}
		bad.Gates[index].Recomputed, bad.Gates[index].NotRecomputed, bad.Gates[index].Failed = 0, 1, 0
		bad.Gates[index].Assessment, bad.Gates[index].EvidenceMode = AssessBlocked, ModeRecordedNotRecomputed
		bad.Gates[index].finish()
		if validScopedRuntimeReplayResult(bad) {
			t.Fatalf("saved PASS accepted a blocked scoped gate: %s/%s", reader, gateID)
		}
	}
	if reader == runtimePreprocessCSTReader {
		badCount := pass
		badCount.Consumption.Expected++
		badCount.Consumption.Consumed++
		if validScopedRuntimeReplayResult(badCount) {
			t.Fatal("saved PASS accepted a shortened CST case list")
		}
		mutations := []struct {
			name string
			edit func(*PreprocessCSTCase)
		}{
			{"schema", func(c *PreprocessCSTCase) { c.Result.Schema = "tsgk-preprocess-cst/r99" }},
			{"result", func(c *PreprocessCSTCase) {
				c.Result.Result, c.Result.Code = "UNVERIFIED", "PREPROCESS_CST_PRIMARY_UNVERIFIED"
			}},
			{"runtime-pins", func(c *PreprocessCSTCase) { c.Result.RuntimePins = pass.RuntimePins }},
		}
		for _, mutation := range mutations {
			bad := pass
			bad.PreprocessCST = append([]PreprocessCSTCase(nil), pass.PreprocessCST...)
			mutation.edit(&bad.PreprocessCST[0])
			if validScopedRuntimeReplayResult(bad) {
				t.Fatalf("saved PASS accepted CST %s mutation", mutation.name)
			}
		}
		bad := pass
		bad.PreprocessCST = append([]PreprocessCSTCase(nil), pass.PreprocessCST...)
		bad.PreprocessCST = append(bad.PreprocessCST, bad.PreprocessCST[0])
		if validScopedRuntimeReplayResult(bad) {
			t.Fatal("saved PASS accepted duplicate CST case ID")
		}
	}
	bad := pass
	pin := *pass.RuntimePins
	pins := *pin.Pins
	pins.Materials = append([]EvidencePinMaterial(nil), pins.Materials...)
	pins.Materials = slices.DeleteFunc(pins.Materials, func(m EvidencePinMaterial) bool { return m.Name == runtimeReadersName })
	pins.Required--
	pins.Verified--
	pin.Pins = &pins
	bad.RuntimePins = &pin
	if validScopedRuntimeReplayResult(bad) {
		t.Fatalf("saved %s scope allowed required descriptor shrink", reader)
	}
	profileSHA := ""
	for _, id := range pass.Identities {
		if id.Role == "replay-profile" {
			profileSHA = id.SHA256
		}
	}
	node := &EvidenceNode{ID: "replay", EvidenceMode: pass.EvidenceMode, Assessment: pass.Assessment, Files: []ReplayMember{{Path: "result.json", Role: "replay-result"}}}
	subject := &EvidenceNode{ExecutionStatus: pass.Subject.ExecutionStatus, Assessment: pass.Subject.Assessment, EvidenceMode: pass.Subject.EvidenceMode,
		Identities: map[string]string{"run": pass.Subject.Run, "attempt": "1", "platform": pass.Subject.Platform}}
	for _, field := range []string{"status", "assessment", "mode"} {
		forged := pass
		switch field {
		case "status":
			forged.Subject.ExecutionStatus = StatusFailed
		case "assessment":
			forged.Subject.Assessment = AssessBlocked
		case "mode":
			forged.Subject.EvidenceMode = ModeCarriedForward
		}
		forged.Recorded = Verdict{forged.Subject.ExecutionStatus, forged.Subject.Assessment}
		checker := &replayEnv{}
		if err := checker.checkReplayFile(node, subject, map[string][]byte{"result.json": eligibilityJSON(t, forged)}, func(code, path, message string) { checker.finding(code, path, message) }); err != nil || len(checker.findings) == 0 {
			t.Fatalf("saved %s graph subject forgery accepted (%s, %s): %v", reader, field, profileSHA, err)
		}
	}
}

func TestReplayScopedRuntimePinsRejectsScopeForgery(t *testing.T) {
	for _, reader := range []string{runtimeAcceptedReader, runtimePreprocessCSTReader} {
		t.Run(reader, func(t *testing.T) {
			req := scopedAcceptedReplay(t, "scope-forgery")
			if reader == runtimePreprocessCSTReader {
				req = scopedCSTReplay(t, "scope-forgery")
			}
			var scopes runtimeReaders
			if err := json.Unmarshal(req.RuntimePins.Materials[runtimeReadersName], &scopes); err != nil {
				t.Fatal(err)
			}
			scopes.TupleSHA256 = strings.Repeat("0", 64)
			materials := clonePinMaterials(req.RuntimePins.Materials)
			materials[runtimeReadersName] = eligibilityJSON(t, scopes)
			req.RuntimePins = scopedPinRequestWithMaterials(t, req.Profile, materials)
			got, err := Replay(pinTestContext(t), req)
			if err != nil || got.EvidenceValid || got.RuntimePins == nil || got.RuntimePins.Assessment == AssessPass {
				t.Fatalf("reader tuple substitution accepted: %+v %v", got, err)
			}
		})
	}
}

func TestReplayScopedRuntimePinsRejectsUnboundInputs(t *testing.T) {
	for _, reader := range []string{runtimeAcceptedReader, runtimePreprocessCSTReader} {
		for _, mode := range []string{"missing-reader", "unsupported-reader", "tuple-driver", "unapproved", "subject"} {
			t.Run(reader+"/"+mode, func(t *testing.T) {
				var req ReplayRequest
				if reader == runtimeAcceptedReader {
					req = scopedAcceptedReplay(t, mode)
				} else {
					req = scopedCSTReplay(t, mode)
				}
				materials := clonePinMaterials(req.RuntimePins.Materials)
				switch mode {
				case "missing-reader":
					delete(materials, runtimeReadersName)
					req.RuntimePins = scopedPinRequestWithMaterials(t, req.Profile, materials)
				case "unsupported-reader":
					var scopes runtimeReaders
					if err := json.Unmarshal(materials[runtimeReadersName], &scopes); err != nil {
						t.Fatal(err)
					}
					if reader == runtimeAcceptedReader {
						scopes.AcceptedReader = "tsgk-runtime-accepted/r99"
					} else {
						scopes.PreprocessCSTReader = "tsgk-runtime-preprocess-cst/r99"
					}
					materials[runtimeReadersName] = eligibilityJSON(t, scopes)
					req.RuntimePins = scopedPinRequestWithMaterials(t, req.Profile, materials)
				case "tuple-driver":
					runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.DriverSHA256 = strings.Repeat("e", 64) })
					req.RuntimePins = scopedPinRequest(t, req.Profile, materials, reader)
				case "unapproved":
					req.RuntimePins.ApprovedPinSetSHA256 = ""
				case "subject":
					req.RuntimePins.Subject = []byte("different replay profile")
					pinTestApprove(t, req.RuntimePins)
				}
				got, err := Replay(pinTestContext(t), req)
				if mode == "unsupported-reader" {
					if err == nil || asError(err).Code != "PIN_RUNTIME_READER_UNSUPPORTED" {
						t.Fatalf("unsupported reader was not rejected: %+v %v", got.RuntimePins, err)
					}
					return
				}
				if err != nil || got.Assessment == AssessPass && got.EvidenceValid {
					t.Fatalf("unbound %s accepted: %+v %v", mode, got.RuntimePins, err)
				}
			})
		}
	}
}
