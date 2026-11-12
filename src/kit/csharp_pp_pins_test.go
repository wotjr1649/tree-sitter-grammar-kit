package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ppexpect"
)

func ppPinsFixture(t *testing.T) ([]byte, []byte, ppexpect.Envelope) {
	t.Helper()
	dir := filepath.Join("..", "testdata", "csharp-pp")
	cases, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	observed, err := os.ReadFile(filepath.Join(dir, "roslyn-observed.json"))
	if err != nil {
		t.Fatal(err)
	}
	pins, err := ppexpect.ReadEnvelope(filepath.Join(dir, "roslyn-pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cases, observed, pins
}

func ppApproveFixture(t *testing.T, cases, observed []byte, tuple ppexpect.Tuple) ppexpect.Envelope {
	t.Helper()
	tuple.ObservationSHA256, tuple.CasesSHA256 = digestHex(observed), digestHex(cases)
	pin := eligibilityJSON(t, tuple)
	materials := []EvidencePinMaterial{}
	for _, item := range []struct {
		name, role string
		data       []byte
	}{
		{"roslyn/tuple.json", "roslyn-tuple", pin}, {"roslyn/observed.json", "roslyn-observation", observed}, {"roslyn/cases.json", "roslyn-cases", cases},
	} {
		materials = append(materials, EvidencePinMaterial{Name: item.name, Role: item.role, Bytes: uint64(len(item.data)), SHA256: digestHex(item.data)})
	}
	req := EvidencePinsRequest{Subject: observed, PinSet: eligibilityJSON(t, evidencePinSet{Schema: EvidencePinSetSchema, Reader: EvidencePinReader, Materials: materials})}
	pinTestApprove(t, &req)
	return ppexpect.Envelope{Approval: req.Approval, ApprovedApprovalSHA256: req.ApprovedApprovalSHA256, PinSet: req.PinSet, ApprovedPinSetSHA256: req.ApprovedPinSetSHA256, Tuple: pin}
}

func ppNewFixture(t *testing.T, cases, observed []byte, identity string) ([]byte, ppexpect.Envelope) {
	t.Helper()
	var d ppexpect.Document
	if err := json.Unmarshal(observed, &d); err != nil {
		t.Fatal(err)
	}
	d.Schema, d.Roslyn = ppexpect.ProvenanceSchema, "assembly-"+identity
	d.Provenance = &ppexpect.Provenance{SDK: "sdk-" + identity, OracleSHA256: strings.Repeat("1", 64), CodeAnalysisSHA256: strings.Repeat("2", 64), CSharpSHA256: strings.Repeat("3", 64)}
	raw := eligibilityJSON(t, d)
	return raw, ppApproveFixture(t, cases, raw, ppexpect.Tuple{Schema: "tsgk-roslyn-pin-tuple/r1", Reader: "tsgk-roslyn-selection/r1", ObservationSchema: d.Schema, Roslyn: d.Roslyn, LanguageVersion: d.LanguageVersion, Scope: "sdk-dll-provenance", Provenance: d.Provenance})
}

func TestRoslynPinsProductionIndependence(t *testing.T) {
	cases, legacy, a := ppPinsFixture(t)
	want, err := ppApprovedObservation(cases, legacy, a)
	if err != nil || want.Schema != ppexpect.ObservedSchema || want.Provenance != nil {
		t.Fatalf("legacy: %+v %v", want, err)
	}
	for _, generation := range []string{"B", "C"} {
		raw, candidate := ppNewFixture(t, cases, legacy, generation)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": candidate.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, candidate.PinSet); err != nil {
			t.Fatal(err)
		}
		current, err := ppApprovedObservation(cases, raw, candidate)
		if err != nil || len(current.Cases) != len(want.Cases) {
			t.Fatalf("new observation: %v", err)
		}
		got, err := ppApprovedObservation(cases, legacy, a)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("production %s changed historical expectation: %v", generation, err)
		}
	}
}

func TestRoslynPinsRejectForgery(t *testing.T) {
	cases, legacy, a := ppPinsFixture(t)
	for _, name := range []string{"unapproved-set", "unapproved-approval", "raw", "metadata", "sdk", "dll", "missing-provenance", "legacy-provenance", "coverage", "reader", "scope", "tuple-content", "material-role", "materials-shrink"} {
		t.Run(name, func(t *testing.T) {
			raw, pins := ppNewFixture(t, cases, legacy, "new")
			var tuple ppexpect.Tuple
			if err := json.Unmarshal(pins.Tuple, &tuple); err != nil {
				t.Fatal(err)
			}
			var d ppexpect.Document
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "unapproved-set":
				pins.ApprovedPinSetSHA256 = a.ApprovedPinSetSHA256
			case "unapproved-approval":
				pins.ApprovedApprovalSHA256 = a.ApprovedApprovalSHA256
			case "raw":
				raw = append(raw, '\n')
			case "metadata":
				d.Roslyn = "forged"
				raw = eligibilityJSON(t, d)
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "sdk":
				d.Provenance.SDK = "forged"
				raw = eligibilityJSON(t, d)
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "dll":
				d.Provenance.CSharpSHA256 = strings.Repeat("4", 64)
				raw = eligibilityJSON(t, d)
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "missing-provenance":
				d.Provenance = nil
				raw = eligibilityJSON(t, d)
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "legacy-provenance":
				raw = legacy
				tuple.ObservationSchema = ppexpect.ObservedSchema
				var old ppexpect.Document
				if err := json.Unmarshal(legacy, &old); err != nil {
					t.Fatal(err)
				}
				tuple.Roslyn = old.Roslyn
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "coverage":
				d.Cases = d.Cases[:len(d.Cases)-1]
				raw = eligibilityJSON(t, d)
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "reader":
				tuple.Reader = "unknown"
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "scope":
				tuple.Scope = "unknown"
				pins = ppApproveFixture(t, cases, raw, tuple)
			case "tuple-content":
				tuple.ObservationSHA256 = strings.Repeat("5", 64)
				pins.Tuple = eligibilityJSON(t, tuple)
			case "material-role", "materials-shrink":
				var set evidencePinSet
				if err := json.Unmarshal(pins.PinSet, &set); err != nil {
					t.Fatal(err)
				}
				if name == "material-role" {
					set.Materials[0].Role = "reference-input"
				} else {
					set.Materials = set.Materials[:2]
				}
				req := EvidencePinsRequest{Subject: raw, PinSet: eligibilityJSON(t, set)}
				pinTestApprove(t, &req)
				pins.PinSet, pins.ApprovedPinSetSHA256, pins.Approval, pins.ApprovedApprovalSHA256 = req.PinSet, req.ApprovedPinSetSHA256, req.Approval, req.ApprovedApprovalSHA256
			}
			if got, err := ppApprovedObservation(cases, raw, pins); err == nil {
				t.Fatalf("Roslyn forgery accepted: %s %+v", name, got)
			} else if (name == "missing-provenance" || name == "legacy-provenance") && !strings.Contains(err.Error(), "BLOCKED") {
				t.Fatalf("missing provenance did not remain BLOCKED: %v", err)
			}
		})
	}
}

func TestRoslynEnvelopeStrict(t *testing.T) {
	_, _, pins := ppPinsFixture(t)
	raw := eligibilityJSON(t, pins)
	for _, data := range [][]byte{append([]byte(`{"unknown":0,`), raw[1:]...), append([]byte(`{"tuple_base64":"",`), raw[1:]...), []byte(`null`)} {
		path := filepath.Join(t.TempDir(), "pins.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ppexpect.ReadEnvelope(path); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
}
