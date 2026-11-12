package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func runtimePinArchive(t *testing.T, setSHA string) map[string][]byte {
	t.Helper()
	root := "../contracts/evidence-pins"
	data, err := os.ReadFile(filepath.Join(root, "sets", setSHA+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var set evidencePinSet
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	materials := map[string][]byte{}
	for _, m := range set.Materials {
		data, err := os.ReadFile(filepath.Join(root, "objects", m.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if digestHex(data) != m.SHA256 || uint64(len(data)) != m.Bytes {
			t.Fatal("historical material changed", m.Name)
		}
		materials[m.Name] = data
	}
	return materials
}

func runtimePinMaterials(t *testing.T) map[string][]byte {
	return runtimePinArchive(t, "6cd6a117001a1d90ab174e65df6fd735773e6af682c5ede43978cdfedb50f408")
}

func runtimePinRequest(t *testing.T, f *eligibilityFixture, materials map[string][]byte) QueryReferenceEligibilityRequest {
	t.Helper()
	return runtimePinRequestFor(t, f.request(), materials)
}

func runtimePinRequestFor(t *testing.T, input QueryReferenceEligibilityRequest, materials map[string][]byte) QueryReferenceEligibilityRequest {
	t.Helper()
	req := eligibilityPinRequest(t, input, materials)
	var set evidencePinSet
	if err := json.Unmarshal(req.Pins.PinSet, &set); err != nil {
		t.Fatal(err)
	}
	for i := range set.Materials {
		m := &set.Materials[i]
		switch m.Name {
		case "native/runtime-tuple.json":
			m.Role = "runtime-tuple"
		case "native/runtime-manifest.json":
			m.Role = "runtime-manifest"
		case "native/driver.c":
			m.Role = "driver"
		case "native/cp949_table.h":
			m.Role = "cp949"
		case "native/shim-template.c":
			m.Role = "shim-template"
		default:
			if strings.HasPrefix(m.Name, "native/runtime-") {
				m.Role = "runtime-patch"
			}
		}
	}
	req.Pins.PinSet = eligibilityJSON(t, set)
	pinTestApprove(t, req.Pins)
	return req
}

func runtimePinTuple(t *testing.T, materials map[string][]byte, change func(*eligibilityRuntimeTuple)) {
	t.Helper()
	var tuple eligibilityRuntimeTuple
	if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	change(&tuple)
	materials["native/runtime-tuple.json"] = eligibilityJSON(t, tuple)
}

// Rebind synthetic stored evidence; no runtime or driver is executed.
func runtimePinRebind(t *testing.T, f *eligibilityFixture, materials map[string][]byte) {
	t.Helper()
	var tuple eligibilityRuntimeTuple
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	rows := map[string]eligibilitySnapshotRow{}
	for _, row := range manifest.Files {
		rows["runtime "+row.Path] = eligibilitySnapshotRow{Bytes: row.Bytes, SHA256: row.SHA256}
	}
	rows["driver driver/driver.c"] = eligibilitySnapshotRow{Bytes: uint64(len(materials["native/driver.c"])), SHA256: tuple.DriverSHA256}
	rows["driver driver/cp949_table.h"] = eligibilitySnapshotRow{Bytes: uint64(len(materials["native/cp949_table.h"])), SHA256: tuple.CP949SHA256}
	shim := []byte(fmt.Sprintf(string(materials["native/shim-template.c"]), f.reg.Native.Symbol, f.reg.Native.Symbol))
	rows["driver driver/shim.c"] = eligibilitySnapshotRow{Bytes: uint64(len(shim)), SHA256: digestHex(shim)}
	f.nativeManifestMutation(func(data string) string {
		lines := strings.Split(data, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "runtime=") {
				lines[i] = "runtime=" + manifest.Commit
			}
			for key, row := range rows {
				if strings.HasPrefix(line, "file="+key+" ") {
					lines[i] = fmt.Sprintf("file=%s %s %d", key, row.SHA256, row.Bytes)
				}
			}
		}
		return strings.Join(lines, "\n")
	})
	f.witness.Native.RuntimeManifestSHA256 = tuple.ManifestSHA256
	f.witness.Native.ProducerABI.RuntimeLanguageVersion, f.witness.Native.ProducerABI.RuntimeMinCompatible = tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible
	f.replace("native/request.json", eligibilityJSON(t, f.witness.Native))
	for i := range f.reg.Invocations {
		if f.reg.Invocations[i].ID == "native-current" {
			f.reg.Invocations[i].ApprovedRequest = eligibilityJSON(t, f.witness.Native)
		}
	}
	var raw nativewire.Response
	if err := json.Unmarshal(f.files["native/response.json"], &raw); err != nil {
		t.Fatal(err)
	}
	raw.Producer.RuntimeLanguageVersion, raw.Producer.RuntimeMinCompatible = tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible
	raw.Producer.ReuseObservation = eligibilityJSON(t, map[string]string{"policy": reuseobs.Policy, "runtime_manifest_sha256": tuple.ManifestSHA256})
	for i := range raw.Steps {
		var obs []reuseobs.Observation
		if err := json.Unmarshal(raw.Steps[i].Observations, &obs); err != nil {
			t.Fatal(err)
		}
		for j := range obs {
			obs[j].RuntimeManifestSHA256 = tuple.ManifestSHA256
		}
		raw.Steps[i].Observations = eligibilityJSON(t, obs)
	}
	f.replace("native/response.json", eligibilityJSON(t, raw))
	var process eligibilityExecution
	if err := json.Unmarshal(f.files["native/process.json"], &process); err != nil {
		t.Fatal(err)
	}
	process.StdoutBytes = uint64(len(f.files["native/response.json"])) + 4
	f.replace("native/process.json", eligibilityJSON(t, process))
}

func runtimePinGeneration(t *testing.T, upper, lower uint32, label string) QueryReferenceEligibilityRequest {
	t.Helper()
	f, materials := newEligibilityFixture(t), runtimePinMaterials(t)
	var manifest eligibilityRuntimeManifest
	if err := json.Unmarshal(materials["native/runtime-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Commit, manifest.Note = digestHex([]byte(label))[:40], "synthetic runtime "+label
	materials["native/runtime-manifest.json"] = eligibilityJSON(t, manifest)
	materials["native/driver.c"] = append(materials["native/driver.c"], []byte("\n/* synthetic "+label+" */\n")...)
	runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) {
		tuple.ManifestSHA256, tuple.DriverSHA256 = digestHex(materials["native/runtime-manifest.json"]), digestHex(materials["native/driver.c"])
		tuple.RuntimeLanguageVersion, tuple.RuntimeMinCompatible = upper, lower
	})
	runtimePinRebind(t, f, materials)
	return runtimePinRequest(t, f, materials)
}

func TestRuntimePinsProductionIndependence(t *testing.T) {
	f := newEligibilityFixture(t)
	materials := runtimePinArchive(t, "d301defb285c098a95bb5d6e4e7e01209eee545761969bca14cdd54994416801")
	runtimePinRebind(t, f, materials)
	old := runtimePinRequest(t, f, materials)
	original := append([]byte(nil), old.Approval...)
	want, err := VerifyQueryReferenceEligibility(pinTestContext(t), old)
	if err != nil || want.Assessment != AssessPass || !want.Complete || want.Tuple.Verified != 3 || want.RuntimePin == nil || want.RuntimePin.ManifestSHA256 != "affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080" {
		t.Fatalf("historical runtime closure rejected: %+v %v", want, err)
	}
	for _, current := range []QueryReferenceEligibilityRequest{runtimePinGeneration(t, 16, 14, "B"), runtimePinGeneration(t, 17, 15, "C")} {
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := VerifyQueryReferenceEligibility(pinTestContext(t), current)
		if err != nil || got.Assessment != AssessPass || !got.Complete || got.RuntimePin == nil {
			t.Fatalf("approved new runtime rejected: %+v %v", got, err)
		}
		got, err = VerifyQueryReferenceEligibility(pinTestContext(t), old)
		if err != nil || !reflect.DeepEqual(got, want) || !bytes.Equal(original, old.Approval) {
			t.Fatalf("production changed historical runtime: %+v %v", got, err)
		}
	}
	legacy := old
	legacy.Pins = nil
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), legacy)
	if err != nil || got.Assessment != AssessBlocked || !eligibilityFinding(got, "ELIGIBILITY_NATIVE_HISTORICAL_RUNTIME") || got.RuntimePin != nil || got.ResultSchema != QueryReferenceEligibilityResultSchema {
		t.Fatalf("legacy runtime boundary changed: %+v %v", got, err)
	}
}

func TestRuntimePinsRejectForgery(t *testing.T) {
	for _, mode := range []string{"tuple-driver", "tuple-manifest", "abi", "marker", "capability", "build-driver", "build-runtime", "missing-driver", "missing-patch", "reduced-files", "duplicate-files", "reduced-patches", "shim", "patch", "reader", "unapproved-set"} {
		t.Run(mode, func(t *testing.T) {
			f, materials := newEligibilityFixture(t), runtimePinMaterials(t)
			want, code := AssessFail, ""
			switch mode {
			case "tuple-driver":
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.DriverSHA256 = strings.Repeat("e", 64) })
				runtimePinRebind(t, f, materials)
				code = "PIN_RUNTIME_BINDING"
			case "tuple-manifest":
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.ManifestSHA256 = strings.Repeat("e", 64) })
				code = "PIN_RUNTIME_BINDING"
			case "abi":
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.RuntimeLanguageVersion = 16 })
				code = "ELIGIBILITY_NATIVE_RUNTIME_ABI"
			case "marker", "capability":
				var raw nativewire.Response
				if err := json.Unmarshal(f.files["native/response.json"], &raw); err != nil {
					t.Fatal(err)
				}
				if mode == "marker" {
					raw.Steps[0].Observations = bytes.ReplaceAll(raw.Steps[0].Observations, []byte(reuseobs.RuntimeManifestSHA256), []byte(strings.Repeat("e", 64)))
				} else {
					raw.Producer.ReuseObservation = bytes.ReplaceAll(raw.Producer.ReuseObservation, []byte(reuseobs.RuntimeManifestSHA256), []byte(strings.Repeat("e", 64)))
				}
				f.replace("native/response.json", eligibilityJSON(t, raw))
			case "build-driver":
				f.nativeManifestMutation(func(text string) string {
					return strings.ReplaceAll(text, digestHex(materials["native/driver.c"]), strings.Repeat("e", 64))
				})
				code = "ELIGIBILITY_NATIVE_RUNTIME_DRIVER"
			case "build-runtime":
				f.nativeManifestMutation(func(text string) string { return strings.Replace(text, "runtime=", "runtime=bad", 1) })
				code = "ELIGIBILITY_NATIVE_BUILD_HEADER"
			case "missing-driver":
				delete(materials, "native/driver.c")
				want, code = AssessBlocked, "PIN_RUNTIME_MISSING"
			case "missing-patch":
				delete(materials, "native/runtime-navigation.patch.json")
				want, code = AssessBlocked, "PIN_RUNTIME_MISSING"
			case "reduced-files", "duplicate-files", "reduced-patches", "patch":
				var m eligibilityRuntimeManifest
				if err := json.Unmarshal(materials["native/runtime-manifest.json"], &m); err != nil {
					t.Fatal(err)
				}
				if mode == "reduced-files" {
					m.Files = m.Files[:len(m.Files)-1]
				} else if mode == "duplicate-files" {
					m.Files[1] = m.Files[0]
				} else if mode == "reduced-patches" {
					m.Patches = m.Patches[:2]
				} else {
					m.Patches[0].SHA256 = strings.Repeat("e", 64)
				}
				materials["native/runtime-manifest.json"] = eligibilityJSON(t, m)
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) {
					tuple.ManifestSHA256 = digestHex(materials["native/runtime-manifest.json"])
				})
				code = "PIN_RUNTIME_OBLIGATIONS"
				if mode == "patch" {
					code = "PIN_RUNTIME_PATCH"
				}
			case "shim":
				materials["native/shim-template.c"] = []byte("fabricated shim")
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) {
					tuple.ShimTemplateSHA256 = digestHex(materials["native/shim-template.c"])
				})
				code = "PIN_RUNTIME_OBLIGATIONS"
			case "reader":
				runtimePinTuple(t, materials, func(tuple *eligibilityRuntimeTuple) { tuple.LoggerReader = "unknown/r1" })
			}
			req := runtimePinRequest(t, f, materials)
			if mode == "unapproved-set" {
				req.Pins.ApprovedPinSetSHA256 = strings.Repeat("e", 64)
				code = "PIN_APPROVAL_MISMATCH"
			}
			got, err := VerifyQueryReferenceEligibility(pinTestContext(t), req)
			if mode == "reader" {
				if err == nil || asError(err).Kind != KindUnsupported {
					t.Fatalf("unsupported reader accepted: %+v %v", got, err)
				}
				return
			}
			if err != nil || got.Assessment != want || code != "" && !eligibilityFinding(got, code) {
				t.Fatalf("runtime forgery accepted (%s): %+v %v", mode, got, err)
			}
		})
	}
}

func TestRuntimePinsSharedAuthority(t *testing.T) {
	b := semanticTestBudget(t)
	f := newEligibilityFixture(t)
	req := runtimePinRequest(t, f, runtimePinMaterials(t))
	scope := &eligibilityScope{r: b.r, wanted: map[string]bool{}}
	got, err := verifyQueryReferenceEligibilityInRun(b.r.caller, req, scope)
	if err != nil || got.Assessment != AssessPass || scope.session.runtime == nil || b.runtime != nil {
		t.Fatalf("runtime authority escaped subject: %+v %v", got, err)
	}
	if err := b.charge(1, 1, "after-runtime"); err != nil {
		t.Fatal(err)
	}
	legacy := newEligibilityFixture(t).request()
	got, err = verifyQueryReferenceEligibilityInRun(b.r.caller, legacy, &eligibilityScope{r: b.r, wanted: map[string]bool{}})
	if err != nil || got.Assessment != AssessPass || got.RuntimePin != nil || b.runtime != nil {
		t.Fatalf("shared budget leaked runtime authority: %+v %v", got, err)
	}
}

func TestRuntimePinsStoredIdentity(t *testing.T) {
	f := newEligibilityFixture(t)
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), runtimePinRequest(t, f, runtimePinMaterials(t)))
	if err != nil || !validPassedPinnedEligibility(&got) {
		t.Fatalf("stored runtime result rejected: %+v %v", got, err)
	}
	for _, mode := range []string{"absent", "manifest", "tuple", "abi", "reader"} {
		var changed QueryReferenceEligibilityResult
		if err := json.Unmarshal(eligibilityJSON(t, got), &changed); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "absent":
			changed.RuntimePin = nil
		case "manifest":
			changed.RuntimePin.ManifestSHA256 = strings.Repeat("e", 64)
		case "tuple":
			changed.RuntimePin.TupleSHA256 = strings.Repeat("e", 64)
		case "abi":
			changed.RuntimePin.RuntimeMinCompatible = changed.RuntimePin.RuntimeLanguageVersion + 1
		case "reader":
			changed.RuntimePin.Reader = "unknown/r1"
		}
		if validPassedPinnedEligibility(&changed) {
			t.Fatal("stored runtime identity forgery accepted", mode)
		}
	}
}

func TestRuntimePinsReaderIdentity(t *testing.T) {
	if digestHex(runtimeObligationsR1) != "43e1e2f828832935f595280bcca86c5a9a384717be7b1995be6560dd8a7d20fd" {
		t.Fatal("historical runtime reader obligations changed")
	}
}

func TestRuntimePinsStoredClosureCannotShrink(t *testing.T) {
	f := newEligibilityFixture(t)
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), runtimePinRequest(t, f, runtimePinMaterials(t)))
	if err != nil || !validPassedPinnedEligibility(&got) {
		t.Fatalf("stored runtime result rejected: %+v %v", got, err)
	}
	for _, role := range runtimePinRolesR1 {
		var changed QueryReferenceEligibilityResult
		if err := json.Unmarshal(eligibilityJSON(t, got), &changed); err != nil {
			t.Fatal(err)
		}
		materials := []EvidencePinMaterial{}
		for _, m := range changed.Pins.Materials {
			if m.Name != role.name {
				materials = append(materials, m)
			}
		}
		changed.Pins.Materials = materials
		changed.Pins.Required, changed.Pins.Verified = uint64(len(materials)), uint64(len(materials))
		if validPassedPinnedEligibility(&changed) {
			t.Fatal("stored closure shrink accepted", role.name)
		}
	}
}
