package kit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func preprocessCSTRuntimeRequest(t *testing.T) (PreprocessCSTRequest, map[string][]byte) {
	t.Helper()
	base := filepath.Join("..", "testdata", "qualify-symbols", "s05-csharp-symbols-switch")
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(base, path))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	profile, err := ParseIncrementalProfile(read("profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Platform, ObservationRunID string
		Build                      struct{ Identity string }
		Cases                      []struct {
			PreprocessCST *PreprocessCSTResult `json:"preprocess_cst"`
		}
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(read("result.json"), &top); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]any{"platform": &result.Platform, "observation_run_id": &result.ObservationRunID, "build": &result.Build, "cases": &result.Cases} {
		if err := json.Unmarshal(top[name], target); err != nil {
			t.Fatal(err)
		}
	}
	c := profile.Cases[0]
	stem := "00000-" + c.ID
	var primary PreprocessPrimary
	primaryBytes, err := json.Marshal(result.Cases[0].PreprocessCST.Primary)
	if err != nil || json.Unmarshal(primaryBytes, &primary) != nil {
		t.Fatal("primary fixture")
	}
	raw := read("primary/responses/" + stem + ".json")
	req := PreprocessCSTRequest{Case: c, Source: read("sources/" + stem + ".bin"), Preprocess: profile.Preprocess, Raw: raw, Primary: &primary,
		Operation: NativeOperations()["native-parse-edit"], Platform: result.Platform, RunID: result.ObservationRunID, ProducerSHA256: result.Build.Identity}
	allMaterials := runtimePinMaterials(t)
	materials := map[string][]byte{}
	for _, item := range runtimePinRolesR1 {
		materials[item.name] = slices.Clone(allMaterials[item.name])
	}
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Note += " (synthetic historical preprocess-CST tuple)"
	materials["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
	runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) {
		tuple.ManifestSHA256 = digestHex(materials["native/runtime-manifest.json"])
		tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible = 16, 14
	})
	var tuple eligibilityRuntimeTuple
	if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	var response nativewire.Response
	if _, err := nativewire.DecodeResponse(raw); err != nil || json.Unmarshal(raw, &response) != nil {
		t.Fatal("response fixture", err)
	}
	response.Producer.LanguageVersion = 15
	response.Producer.RuntimeLanguageVersion, response.Producer.RuntimeMinCompatible = tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible
	response.Producer.ReuseObservation = eligibilityJSON(t, map[string]string{"policy": reuseobs.Policy, "runtime_manifest_sha256": tuple.ManifestSHA256})
	for i := range response.Steps {
		var observations []reuseobs.Observation
		if err := json.Unmarshal(response.Steps[i].Observations, &observations); err != nil {
			t.Fatal(err)
		}
		for j := range observations {
			observations[j].RuntimeManifestSHA256 = tuple.ManifestSHA256
		}
		response.Steps[i].Observations = eligibilityJSON(t, observations)
	}
	req.Raw, err = json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	// Protocol r3 omits the r2 query/API members.
	req.Raw = bytes.ReplaceAll(req.Raw, []byte(`,"queries":null,"api":null`), nil)
	req.Primary.Process.StdoutBytes = int64(len(req.Raw)) + 4
	req.RuntimePins = scopedPinRequest(t, req.Raw, materials, runtimePreprocessCSTReader)
	return req, materials
}

func TestPreprocessCSTRuntimePinsProductionIndependence(t *testing.T) {
	req, _ := preprocessCSTRuntimeRequest(t)
	want := CheckPreprocessCST(req)
	if want.Schema != PreprocessCSTRuntimeSchema || want.Result != "PASS" || want.RuntimePins == nil || !validPassedScopedRuntime(want.RuntimePins, digestHex(req.Raw), runtimePreprocessCSTReader) {
		t.Fatalf("historical preprocess-CST: %+v", want)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip PreprocessCSTResult
	if err := json.Unmarshal(encoded, &roundTrip); err != nil || roundTrip.Schema != PreprocessCSTRuntimeSchema || roundTrip.RuntimePins == nil {
		t.Fatalf("runtime result roundtrip: %v %+v", err, roundTrip)
	}
	for _, generation := range []struct {
		upper, lower uint32
		label        string
	}{{16, 14, "B"}, {17, 15, "C"}} {
		candidate := runtimePinGeneration(t, generation.upper, generation.lower, "preprocess-"+generation.label)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": candidate.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, candidate.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got := CheckPreprocessCST(req)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("production %s changed historical preprocess-CST result", generation.label)
		}
	}
}

func TestPreprocessCSTRuntimePinsRejectsForgery(t *testing.T) {
	t.Run("tuple driver", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		req.RuntimePins.Materials["native/driver.c"] = append(req.RuntimePins.Materials["native/driver.c"], '\n')
		// The caller reapproves the altered material set; the tuple still binds the original driver.
		req.RuntimePins = scopedPinRequest(t, req.Raw, req.RuntimePins.Materials, runtimePreprocessCSTReader)
		got := CheckPreprocessCST(req)
		if got.Result == "PASS" || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessFail {
			t.Fatalf("tuple/driver forgery accepted: %+v", got)
		}
	})
	t.Run("raw ABI", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		var response nativewire.Response
		if err := json.Unmarshal(req.Raw, &response); err != nil {
			t.Fatal(err)
		}
		response.Producer.RuntimeMinCompatible--
		badRaw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		badRaw = bytes.ReplaceAll(badRaw, []byte(`,"queries":null,"api":null`), nil)
		req.Raw = badRaw
		req.Primary.Process.StdoutBytes = int64(len(badRaw)) + 4
		req.RuntimePins = scopedPinRequest(t, badRaw, req.RuntimePins.Materials, runtimePreprocessCSTReader)
		got := CheckPreprocessCST(req)
		if got.Result != "FAIL" || got.Code != "PREPROCESS_CST_PRIMARY_INVALID" || got.RuntimePins.Assessment != AssessPass {
			t.Fatalf("raw ABI mismatch accepted: %+v", got)
		}
	})
	t.Run("source identity", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		req.Source = slices.Clone(req.Source)
		req.Source[0] ^= 1
		got := CheckPreprocessCST(req)
		if got.Result != "FAIL" || got.Code != "PREPROCESS_CST_PRIMARY_INVALID" {
			t.Fatalf("source binding mismatch accepted: %+v", got)
		}
	})
	t.Run("fresh equality", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		var response nativewire.Response
		if err := json.Unmarshal(req.Raw, &response); err != nil {
			t.Fatal(err)
		}
		fresh := response.Steps[1].Fresh
		fresh.Types[fresh.Nodes[0][1]] = "different_fresh_root"
		fresh.Digest = nativewire.TreeDigest(func() []TreeNode {
			nodes, err := nativewire.Nodes(fresh.Nodes, fresh.Types, fresh.Fields)
			if err != nil {
				t.Fatal(err)
			}
			return nodes
		}())
		badRaw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		badRaw = bytes.ReplaceAll(badRaw, []byte(`,"queries":null,"api":null`), nil)
		req.Raw = badRaw
		req.Primary.Process.StdoutBytes = int64(len(badRaw)) + 4
		req.RuntimePins = scopedPinRequest(t, badRaw, req.RuntimePins.Materials, runtimePreprocessCSTReader)
		got := CheckPreprocessCST(req)
		if got.Result != "FAIL" || got.Code != "PREPROCESS_CST_EQUALITY_FAILED" {
			t.Fatalf("fresh tree mismatch accepted: %+v", got)
		}
	})
	t.Run("unapproved reader", func(t *testing.T) {
		req, _ := preprocessCSTRuntimeRequest(t)
		materials := clonePinMaterials(req.RuntimePins.Materials)
		var readers runtimeReaders
		if err := json.Unmarshal(materials[runtimeReadersName], &readers); err != nil {
			t.Fatal(err)
		}
		readers.PreprocessCSTReader = ""
		materials[runtimeReadersName] = eligibilityJSON(t, readers)
		req.RuntimePins = scopedPinRequestWithMaterials(t, req.Raw, materials)
		got := CheckPreprocessCST(req)
		if got.Result == "PASS" || got.RuntimePins == nil || got.RuntimePins.Assessment != AssessBlocked {
			t.Fatalf("unapproved reader accepted: %+v", got)
		}
	})
}

func TestCheckPreprocessCSTLegacySchemaUnchanged(t *testing.T) {
	req, _ := preprocessCSTRuntimeRequest(t)
	req.RuntimePins = nil
	got := CheckPreprocessCST(req)
	if got.Schema != PreprocessCSTSchema {
		t.Fatalf("nil opt-in changed schema: %s", got.Schema)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"runtime_pins"`)) {
		t.Fatal("nil opt-in added runtime_pins to legacy output")
	}
	if _, err := decodePreprocessCST(encoded); err != nil {
		t.Fatal(err)
	}
	r2 := bytes.Replace(encoded, []byte(`"schema":"`+PreprocessCSTSchema+`"`), []byte(`"schema":"`+PreprocessCSTRuntimeSchema+`"`), 1)
	runtimePins := append(bytes.TrimSuffix(bytes.Clone(encoded), []byte("}")), []byte(`,"runtime_pins":null}`)...)
	for name, bad := range map[string][]byte{"r2 schema": r2, "r1 with runtime pin": runtimePins} {
		if _, err := decodePreprocessCST(bad); err == nil {
			t.Fatalf("legacy projection reader accepted %s", name)
		}
	}
}
