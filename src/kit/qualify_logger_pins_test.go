package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/reuseobs"
)

func qualificationLoggerRequest(t *testing.T, label, mutation string) (QualifyRequest, map[string][]byte) {
	t.Helper()
	materials := loggerPinMaterials(t, label)
	tuple := loggerTuple(t, materials)
	f := newQfx(t)
	mutateRecord := func(set string, records []map[string]any) {
		for _, record := range records {
			producer := record["producer"].(map[string]any)
			capability := producer["reuse_observation"].(map[string]any)
			capability["runtime_manifest_sha256"] = tuple.ManifestSHA256
			producer["language_version"] = tuple.RuntimeLanguageVersion
			producer["runtime_language_version"] = tuple.RuntimeLanguageVersion
			producer["runtime_min_compatible"] = tuple.RuntimeMinCompatible
			if mutation == "producer" {
				continue
			}
			for _, value := range record["steps"].([]any) {
				step := value.(map[string]any)
				observations := step["observations"].([]reuseobs.Observation)
				for i := range observations {
					observations[i].RuntimeManifestSHA256 = tuple.ManifestSHA256
				}
			}
		}
	}
	mutateRaw := func(set string, raw map[string]any) {
		producer := raw["producer"].(map[string]any)
		producer["language_version"] = tuple.RuntimeLanguageVersion
		producer["runtime_language_version"] = tuple.RuntimeLanguageVersion
		producer["runtime_min_compatible"] = tuple.RuntimeMinCompatible
		if mutation == "raw" {
			producer["reuse_observation"].(map[string]any)["runtime_manifest_sha256"] = strings.Repeat("e", 64)
		}
		if mutation == "raw-abi" {
			producer["runtime_min_compatible"] = tuple.RuntimeMinCompatible + 1
		}
	}
	mutations := map[string]qfxMut{"windows-amd64": {record: mutateRecord, raw: mutateRaw}}
	if mutation == "tuple" {
		runtimePinTuple(t, materials, func(p *eligibilityRuntimeTuple) { p.DriverSHA256 = strings.Repeat("e", 64) })
	}
	if mutation == "projection-abi" {
		mutations["windows-amd64"] = qfxMut{record: func(set string, records []map[string]any) {
			mutateRecord(set, records)
			for _, record := range records {
				if record["id"] == "c1" {
					record["producer"].(map[string]any)["language_version"] = tuple.RuntimeLanguageVersion + 1
				}
			}
		}, raw: mutateRaw}
	}
	if mutation == "runtime-commit" {
		mutations["windows-amd64"] = qfxMut{record: mutateRecord, raw: mutateRaw, manifest: func(set string, man map[string]any) {
			if set == "s06-fxa" {
				man["producer"].(map[string]string)["runtime_commit"] = strings.Repeat("0", 40)
			}
		}}
	}
	if mutation == "producer" {
		// The declared capability changes while every observation keeps the producer's prior pin.
		mutations["windows-amd64"] = qfxMut{record: mutateRecord, raw: mutateRaw}
	}
	if mutation == "raw" || mutation == "raw-abi" {
		mutations["windows-amd64"] = qfxMut{record: mutateRecord, raw: mutateRaw}
	}
	if mutation == "unapproved" || mutation == "subject" {
		mutations["windows-amd64"] = qfxMut{record: mutateRecord, raw: mutateRaw}
	}
	hosts := f.all(t, mutations)
	request := QualifyRequest{Inventory: f.invBytes(t), Candidate: qfxCandidate}
	for _, platform := range qfxPlatforms {
		host := QualifyHost{Platform: platform.ID, Root: hosts[platform.ID]}
		if platform.ID == "windows-amd64" {
			subject, err := os.ReadFile(filepath.Join(host.Root, "run-identity.json"))
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "subject" {
				subject = append(subject, ' ')
			}
			host.RuntimePins = loggerPinRequest(t, subject, materials)
			if mutation == "unapproved" {
				host.RuntimePins.ApprovedPinSetSHA256 = strings.Repeat("0", 64)
			}
		}
		request.Hosts = append(request.Hosts, host)
	}
	return request, materials
}

func TestQualificationLoggerPinsHistoricalRuntime(t *testing.T) {
	req, materials := qualificationLoggerRequest(t, "A", "valid")
	want, err := Qualify(pinTestContext(t), req)
	if err != nil || want.ResultSchema != QualificationLoggerPinsResultSchema || want.RuntimePins["windows-amd64"].Assessment != AssessPass || !want.RuntimePins["windows-amd64"].Complete {
		t.Fatalf("historical logger qualification: %+v %v", want, err)
	}
	var tuple eligibilityRuntimeTuple
	if err := json.Unmarshal(materials["native/runtime-tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	pin := want.RuntimePins["windows-amd64"]
	subject, err := os.ReadFile(filepath.Join(req.Hosts[0].Root, "run-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if pin.RuntimePin == nil || pin.RuntimePin.ManifestSHA256 != tuple.ManifestSHA256 || pin.Pins == nil || pin.Pins.SubjectSHA256 != digestHex(subject) {
		t.Fatalf("missing consumed runtime identity: %+v", pin)
	}
	if len(want.RuntimePins) != 1 {
		t.Fatalf("runtime authorization leaked to another host: %+v", want.RuntimePins)
	}
	for _, platform := range qfxPlatforms {
		cell := cellOf(want, "fxa", platform.ID)
		if cell.Mechanism != AssessPass {
			t.Fatalf("ordinary logger gate failed on %s: %s %+v", platform.ID, cell.Mechanism, cell.Set.Gates)
		}
	}
	for _, label := range []string{"B", "C"} {
		current := loggerPinRequest(t, subject, loggerPinMaterials(t, label))
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, current.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := Qualify(pinTestContext(t), req)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("production runtime %s changed historical logger result: %+v %v", label, got, err)
		}
	}
}

func TestQualificationLoggerPinsRejectsUnboundRuntime(t *testing.T) {
	for _, mode := range []string{"unapproved", "subject", "tuple", "producer", "raw", "projection-abi", "raw-abi", "runtime-commit"} {
		t.Run(mode, func(t *testing.T) {
			req, _ := qualificationLoggerRequest(t, "A", mode)
			got, err := Qualify(pinTestContext(t), req)
			if err != nil {
				t.Fatalf("qualification returned an error instead of a scoped result: %v", err)
			}
			if got.ResultSchema != QualificationLoggerPinsResultSchema {
				t.Fatalf("logger result schema %q", got.ResultSchema)
			}
			entry, ok := got.RuntimePins["windows-amd64"]
			if !ok {
				t.Fatalf("runtime result missing: %+v", got.RuntimePins)
			}
			if mode == "unapproved" || mode == "subject" || mode == "tuple" {
				if entry.Assessment == AssessPass || entry.Complete {
					t.Fatalf("runtime approval forgery passed (%s): %+v", mode, entry)
				}
				return
			}
			cell := cellOf(got, "fxa", "windows-amd64")
			gate := slices.IndexFunc(cell.Set.Gates, func(g ReplayGate) bool { return g.ID == "reuse-observation" && g.Failed > 0 })
			if cell.Mechanism != AssessFail || gate < 0 {
				t.Fatalf("logger identity forgery escaped record/raw checks (%s): %+v", mode, cell)
			}
		})
	}
}

func TestQualificationLoggerPinsLegacyAndScope(t *testing.T) {
	req, _ := qualificationLoggerRequest(t, "A", "valid")
	req.Hosts[0].RuntimePins = nil
	got, err := Qualify(pinTestContext(t), req)
	if err != nil || got.ResultSchema != QualificationResultSchema || cellOf(got, "fxa", "windows-amd64").Mechanism != AssessFail {
		t.Fatalf("legacy current-only logger boundary changed: %+v %v", got, err)
	}

	req, _ = qualificationLoggerRequest(t, "A", "valid")
	req.QuerySemantics = &QualificationQuerySemanticsRequest{}
	got, err = Qualify(pinTestContext(t), req)
	if err == nil || asError(err).Code != "LOGGER_PINS_QUALIFICATION_SCOPE" {
		t.Fatalf("runtime logger context entered query semantics: %+v %v", got, err)
	}
}
