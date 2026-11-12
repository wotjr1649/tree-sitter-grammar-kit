package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func semanticHistoricalRuntime(t *testing.T, req *QuerySemanticsRequest, f *eligibilityFixture, binding *semanticRunBinding, materials map[string][]byte) {
	t.Helper()
	oldSHA := digestHex(materials["native/runtime-manifest.json"])
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Note += " (synthetic historical identity)"
	materials["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
	newSHA := digestHex(materials["native/runtime-manifest.json"])
	runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.ManifestSHA256 = newSHA })
	f.witness.Native.RuntimeManifestSHA256 = newSHA
	f.replace("native/request.json", eligibilityJSON(t, f.witness.Native))
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.ID != "native-current" {
			continue
		}
		inv.ApprovedRequest = eligibilityJSON(t, f.witness.Native)
		if inv.Request != nil {
			f.replace(inv.Request.Member.Path, eligibilityJSON(t, f.witness.Native))
		}
	}
	f.replace("native/response.json", bytes.ReplaceAll(f.files["native/response.json"], []byte(oldSHA), []byte(newSHA)))
	for _, relative := range []string{
		"raw/00000-eligibility-case.json",
		"projection/00000-eligibility-case.json",
		"records/00000-eligibility-case.json",
		"capture/stdout.bin",
	} {
		path := "oracle/" + relative
		f.replace(path, bytes.ReplaceAll(f.files[path], []byte(oldSHA), []byte(newSHA)))
	}
	capturePath, recordPath := "oracle/capture/process-capture.json", "oracle/records/00000-eligibility-case.json"
	var capture semanticProcessCapture
	if err := json.Unmarshal(f.files[capturePath], &capture); err != nil {
		t.Fatal(err)
	}
	update := func(pin *ReplayMember) {
		t.Helper()
		data := f.files["oracle/"+pin.Path]
		pin.Bytes, pin.SHA256 = uint64(len(data)), digestHex(data)
	}
	update(capture.Stdout)
	f.replace(capturePath, eligibilityJSON(t, capture))
	var local OracleQuerySemantics
	var record map[string]json.RawMessage
	if err := json.Unmarshal(f.files[recordPath], &record); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record["query_semantics"], &local); err != nil {
		t.Fatal(err)
	}
	for _, member := range binding.Members {
		switch member.Member.Role {
		case "capture":
			local.Capture = &member.Member
		case "projection":
			local.Projection = member.Member
		}
	}
	local.Capture.Path = strings.TrimPrefix(local.Capture.Path, "oracle/")
	local.Projection.Path = strings.TrimPrefix(local.Projection.Path, "oracle/")
	update(local.Capture)
	projection := local.Projection
	update(&projection)
	local.Projection = projection
	record["query_semantics"] = mustJSON(t, local)
	if string(record["complete"]) != "true" {
		t.Fatal("synthetic semantic record is not complete")
	}
	delete(record, "complete")
	data := mustJSON(t, record)
	f.replace(recordPath, append(data[:len(data)-1], []byte(`,"complete":true}`)...))
	semanticBoundReapprove(t, req, f, binding)
}

func TestSemanticRuntimePinsProductionIndependence(t *testing.T) {
	req, f, binding := semanticBoundFixture(t, 0)
	materials := runtimePinMaterials(t)
	semanticHistoricalRuntime(t, &req, f, &binding, materials)
	req.Pins = runtimePinRequest(t, f, materials).Pins
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	want, err := VerifyQuerySemantics(ctx, req)
	if err != nil || !want.Complete || want.Assessment != AssessPass || want.Semantic != AssessPass || want.Tuple.Verified != want.Tuple.Required || want.Eligibility == nil || want.Eligibility.Assessment != AssessPass || want.Eligibility.RuntimePin == nil || want.Eligibility.RuntimePin.ManifestSHA256 != digestHex(materials["native/runtime-manifest.json"]) {
		t.Fatalf("synthetic historical runtime semantic verification failed: %+v %v", want, err)
	}
	old := append([]byte(nil), req.Pins.PinSet...)
	root, profile := semanticBoundReplay(t, req, f, binding)
	var replayProfile map[string]json.RawMessage
	if err := json.Unmarshal(profile, &replayProfile); err != nil {
		t.Fatal(err)
	}
	replayProfile["reducer"] = []byte(`"oracle-set-r7"`)
	replayReq := ReplayRequest{Root: root, Profile: mustJSON(t, replayProfile), Pins: req.Pins}
	wantReplay, err := Replay(ctx, replayReq)
	if err != nil || wantReplay.Assessment != AssessPass || wantReplay.QuerySemantics == nil || !validSemanticReplayResult(wantReplay) || wantReplay.QuerySemantics.Tuple != want.Tuple || !reflect.DeepEqual(wantReplay.QuerySemantics.Eligibility.RuntimePin, want.Eligibility.RuntimePin) {
		t.Fatalf("synthetic historical runtime replay failed: %+v %v", wantReplay, err)
	}
	for _, current := range []QueryReferenceEligibilityRequest{runtimePinGeneration(t, 16, 14, "semantic-B"), runtimePinGeneration(t, 17, 15, "semantic-C")} {
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(ctx, selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := VerifyQuerySemantics(ctx, req)
		if err != nil || !reflect.DeepEqual(got, want) || !bytes.Equal(req.Pins.PinSet, old) {
			t.Fatalf("production selection changed semantic A result: %+v %v", got, err)
		}
		replayed, err := Replay(ctx, replayReq)
		if err != nil || !reflect.DeepEqual(replayed, wantReplay) {
			t.Fatalf("production selection changed replayed semantic A result: %+v %v", replayed, err)
		}
	}
}
