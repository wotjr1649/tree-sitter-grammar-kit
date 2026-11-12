package kit

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The incomplete Oracle set stops after real qualification/replay/eligibility admission.
func qualificationRuntimePins(t *testing.T, materials map[string][]byte) QualifyRequest {
	t.Helper()
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Note += " (synthetic historical qualification identity)"
	materials["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
	runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) {
		tuple.ManifestSHA256 = digestHex(materials["native/runtime-manifest.json"])
	})
	return qualificationPinnedFixtureWithRuntime(t, false, materials)
}

func TestQualificationRuntimePinsProductionIndependence(t *testing.T) {
	t.Parallel()
	req := qualificationRuntimePins(t, runtimePinMaterials(t))
	want, err := Qualify(pinTestContext(t), req)
	if err == nil || want.QuerySemantics == nil {
		t.Fatalf("expected partial Oracle result: %+v %v", want, err)
	}
	core := qualificationPinsTarget(t, want, "semantic-linux").Core
	if core == nil || core.Eligibility == nil || core.Eligibility.Assessment != AssessPass || core.Eligibility.RuntimePin == nil || !validPassedPinnedEligibility(core.Eligibility) {
		t.Fatalf("qualification lost approved runtime: %+v %v", core, err)
	}
	for _, current := range []QueryReferenceEligibilityRequest{runtimePinGeneration(t, 16, 14, "qual-B"), runtimePinGeneration(t, 17, 15, "qual-C")} {
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, gotErr := Qualify(pinTestContext(t), req)
		if gotErr == nil || asError(gotErr).Code != asError(err).Code || !reflect.DeepEqual(got, want) {
			t.Fatalf("production changed qualification runtime: %+v %v", got, gotErr)
		}
	}
}

func TestQualificationRuntimePinsRejectForgery(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"unapproved", "tuple-driver", "missing-driver"} {
		t.Run(mode, func(t *testing.T) {
			materials := runtimePinMaterials(t)
			if mode == "tuple-driver" {
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.DriverSHA256 = strings.Repeat("e", 64) })
			}
			if mode == "missing-driver" {
				delete(materials, "native/driver.c")
			}
			req := qualificationRuntimePins(t, materials)
			if mode == "unapproved" {
				req.QuerySemantics.Targets[0].Pins.ApprovedPinSetSHA256 = ""
			}
			got, err := Qualify(pinTestContext(t), req)
			if err == nil || got.QuerySemantics == nil {
				t.Fatalf("partial result lost: %+v %v", got, err)
			}
			core := qualificationPinsTarget(t, got, "semantic-linux").Core
			if core == nil || core.Eligibility == nil || core.Eligibility.Assessment == AssessPass {
				t.Fatalf("qualification runtime forgery accepted: %+v %v", core, err)
			}
		})
	}
}
