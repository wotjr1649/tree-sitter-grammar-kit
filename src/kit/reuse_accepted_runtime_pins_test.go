package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func setAcceptedRuntimeManifest(v any, manifest string) {
	switch value := v.(type) {
	case map[string]any:
		if _, ok := value["runtime_manifest_sha256"]; ok {
			value["runtime_manifest_sha256"] = manifest
		}
		if capability, ok := value["reuse_observation"].(map[string]any); ok {
			capability["runtime_manifest_sha256"] = manifest
		}
		for _, child := range value {
			setAcceptedRuntimeManifest(child, manifest)
		}
	case []any:
		for _, child := range value {
			setAcceptedRuntimeManifest(child, manifest)
		}
	}
}

func acceptedRuntimePinsFixture(t *testing.T, label string) (ReuseAcceptedRequest, map[string][]byte, eligibilityRuntimeTuple) {
	t.Helper()
	req, raw, projection := acceptedFixture(t, 0)
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	setAcceptedRuntimeManifest(raw, tuple.ManifestSHA256)
	setAcceptedRuntimeManifest(projection, tuple.ManifestSHA256)
	req.Expected.Observation.Manifest = tuple.ManifestSHA256
	setAcceptedEvidence(&req, raw, projection)
	req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimeAcceptedReader)
	return req, materials, tuple
}

func TestAcceptedRuntimePinsProductionIndependence(t *testing.T) {
	accepted, _, _ := acceptedRuntimePinsFixture(t, "A")
	want, err := JudgeReuseAccepted(accepted)
	if err != nil || want.Schema != ReuseAcceptedRuntimeSchema || want.Result != claimPass || want.RuntimePins == nil ||
		want.RuntimePins.Reader != runtimeAcceptedReader || want.RuntimePins.Assessment != AssessPass || !want.RuntimePins.Complete {
		t.Fatalf("historical accepted reader failed: %+v %v", want, err)
	}
	for _, label := range []string{"B", "C"} {
		_, materials, _ := acceptedRuntimePinsFixture(t, label)
		candidate := scopedPinRequest(t, []byte("production selection "+label), materials, runtimeAcceptedReader)
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), eligibilityJSON(t, map[string]string{
			"schema": EvidencePinSelectionSchema, "pin_set_sha256": candidate.ApprovedPinSetSHA256,
		}), candidate.PinSet); err != nil {
			t.Fatalf("select %s: %v", label, err)
		}
		got, err := JudgeReuseAccepted(accepted)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("production %s changed accepted evidence: %+v %v", label, got, err)
		}
	}
}

func TestAcceptedRuntimePinsRejectForgery(t *testing.T) {
	for _, mode := range []string{"reader-blocked", "subject-fail", "cancelled"} {
		t.Run("target-denominator/"+mode, func(t *testing.T) {
			req, materials, _ := acceptedRuntimePinsFixture(t, "A")
			h := req.Ledger.original(req.Case.ID)
			switch mode {
			case "reader-blocked":
				req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimePreprocessCSTReader)
			case "subject-fail":
				req.Raw = append(req.Raw, ' ')
			case "cancelled":
				req.Expected.Check = func() error { return &Error{Kind: KindCancelled, Code: "CANCELLED"} }
			}
			got, err := JudgeReuseAccepted(req)
			if mode == "cancelled" && (err == nil || asError(err).Kind != KindCancelled) || mode != "cancelled" && err != nil || got.HistorySHA256 != h.historySHA256() || len(got.Steps) != len(h.BlockedSteps) {
				t.Fatalf("runtime interruption shrank accepted targets: %+v %v", got, err)
			}
			for i, k := range h.BlockedSteps {
				if got.Steps[i].Step != k || got.Steps[i].Result != claimBlocked {
					t.Fatalf("target %d changed: %+v", k, got.Steps)
				}
			}
		})
	}
	t.Run("expected-manifest", func(t *testing.T) {
		req, _, _ := acceptedRuntimePinsFixture(t, "A")
		req.Expected.Observation.Manifest = digestHex([]byte("different manifest"))
		before := *req.Expected.Observation
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result != claimFail || got.Code != "REUSE_ACCEPTED_REQUEST_MISMATCH" || got.RuntimePins == nil ||
			!reflect.DeepEqual(*req.Expected.Observation, before) {
			t.Fatalf("expected manifest forgery accepted or input mutated: %+v %v", got, err)
		}
	})
	t.Run("runtime-material", func(t *testing.T) {
		req, _, _ := acceptedRuntimePinsFixture(t, "A")
		pins := *req.RuntimePins
		pins.Materials = clonePinMaterials(pins.Materials)
		req.RuntimePins = &pins
		req.RuntimePins.Materials["native/runtime-tuple.json"] = append(req.RuntimePins.Materials["native/runtime-tuple.json"], '\n')
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result == claimPass || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail {
			t.Fatalf("tuple forgery accepted: %+v %v", got, err)
		}
	})
	t.Run("tuple-driver", func(t *testing.T) {
		req, materials, _ := acceptedRuntimePinsFixture(t, "A")
		materials = clonePinMaterials(materials)
		var tuple eligibilityRuntimeTuple
		if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
			t.Fatal(err)
		}
		tuple.DriverSHA256 = digestHex([]byte("unapproved historical driver"))
		materials["native/runtime-tuple.json"] = eligibilityJSON(t, tuple)
		req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimeAcceptedReader)
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result == claimPass || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail {
			t.Fatalf("tuple-driver forgery accepted: %+v %v", got, err)
		}
	})
	t.Run("known-failure-before-interruption", func(t *testing.T) {
		req, materials, _ := acceptedRuntimePinsFixture(t, "A")
		materials = clonePinMaterials(materials)
		var tuple eligibilityRuntimeTuple
		if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
			t.Fatal(err)
		}
		tuple.DriverSHA256 = digestHex([]byte("unapproved historical driver"))
		materials["native/runtime-tuple.json"] = eligibilityJSON(t, tuple)
		req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimeAcceptedReader)
		calls := 0
		req.Expected.Check = func() error {
			calls++
			if calls == 2 {
				return context.Canceled
			}
			return nil
		}
		got, err := JudgeReuseAccepted(req)
		if err != context.Canceled || got.Result != claimFail || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail {
			t.Fatalf("confirmed pin failure was lost to interruption: %+v %v", got, err)
		}
	})
	t.Run("subject", func(t *testing.T) {
		req, _, _ := acceptedRuntimePinsFixture(t, "A")
		req.Raw = append([]byte(nil), req.Raw...)
		req.Raw[0] ^= 1
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result == claimPass || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail {
			t.Fatalf("raw subject forgery accepted: %+v %v", got, err)
		}
	})
	t.Run("approved-subject-other-manifest", func(t *testing.T) {
		req, materials, tuple := acceptedRuntimePinsFixture(t, "A")
		foreign := digestHex([]byte("different runtime manifest"))
		req.Raw = bytes.ReplaceAll(req.Raw, []byte(tuple.ManifestSHA256), []byte(foreign))
		req.Projection = bytes.ReplaceAll(req.Projection, []byte(tuple.ManifestSHA256), []byte(foreign))
		req.Expected.Observation.Manifest = foreign
		req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimeAcceptedReader)
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result == claimPass || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessPass {
			t.Fatalf("approved runtime tuple did not reject other-manifest observations: %+v %v", got, err)
		}
	})
	t.Run("reader-scope", func(t *testing.T) {
		req, materials, _ := acceptedRuntimePinsFixture(t, "A")
		req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimePreprocessCSTReader)
		got, err := JudgeReuseAccepted(req)
		if err != nil || got.Result != claimBlocked || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessBlocked {
			t.Fatalf("missing accepted reader scope was not blocked: %+v %v", got, err)
		}
	})
}
