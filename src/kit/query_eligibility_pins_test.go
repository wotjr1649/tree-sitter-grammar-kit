package kit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func eligibilityPinMaterials(t *testing.T) map[string][]byte {
	t.Helper()
	root := "../contracts/evidence-pins"
	data, err := os.ReadFile(filepath.Join(root, "sets/8f0666ed6933645114fddff1cd8f30a2e55ae23e219fc33a656b0aae12473b26.json"))
	if err != nil {
		t.Fatal(err)
	}
	var set evidencePinSet
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatal(err)
	}
	materials := map[string][]byte{}
	for _, m := range set.Materials {
		if !strings.HasPrefix(m.Name, "reference/") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, "objects", m.SHA256))
		if err != nil {
			t.Fatal(err)
		}
		if digestHex(data) != m.SHA256 || uint64(len(data)) != m.Bytes {
			t.Fatal("historical object changed", m.Name)
		}
		materials[m.Name] = data
	}
	return materials
}

func eligibilityPinRequest(t *testing.T, req QueryReferenceEligibilityRequest, materials map[string][]byte) QueryReferenceEligibilityRequest {
	t.Helper()
	pins := &EvidencePinsRequest{Subject: req.Approval, Materials: materials}
	rows := []EvidencePinMaterial{}
	names := []string{}
	for name := range materials {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		role := "reference-input"
		if name == "reference/catalogue.json" {
			role = "reference-catalogue"
		} else if name == "reference/tuple.json" {
			role = "reference-tuple"
		}
		data := materials[name]
		rows = append(rows, EvidencePinMaterial{Name: name, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)})
	}
	pins.PinSet = eligibilityJSON(t, evidencePinSet{Schema: EvidencePinSetSchema, Reader: EvidencePinReader, Materials: rows})
	pinTestApprove(t, pins)
	req.Pins = pins
	return req
}

func eligibilityPinTuple(t *testing.T, materials map[string][]byte, change func(*eligibilityReferenceTuple)) {
	t.Helper()
	var tuple eligibilityReferenceTuple
	if err := json.Unmarshal(materials["reference/tuple.json"], &tuple); err != nil {
		t.Fatal(err)
	}
	change(&tuple)
	materials["reference/tuple.json"] = eligibilityJSON(t, tuple)
}

func eligibilityPinGeneration(t *testing.T, rustc, cargo string) QueryReferenceEligibilityRequest {
	t.Helper()
	f := newEligibilityFixture(t)
	materials := eligibilityPinMaterials(t)
	// Change a controller identity as a real release update would, preserving old bytes.
	materials["reference/build.py"] = append(materials["reference/build.py"], []byte("\n# synthetic generation "+rustc+"\n")...)
	f.cat.ReferenceInputs["build.py"] = digestHex(materials["reference/build.py"])
	f.cat.ControllerSHA256["build.py"] = f.cat.ReferenceInputs["build.py"]
	f.cat.UpstreamCommit = digestHex([]byte("synthetic upstream " + rustc))[:40]
	materials["reference/catalogue.json"] = eligibilityJSON(t, f.cat)
	f.reg.CatalogueSHA256 = digestHex(materials["reference/catalogue.json"])
	var identity eligibilityBuildIdentity
	if err := json.Unmarshal(f.files["build/identity.json"], &identity); err != nil {
		t.Fatal(err)
	}
	identity.Inputs = f.cat.ReferenceInputs
	identity.UpstreamCommit = f.cat.UpstreamCommit
	for name, release := range map[string]string{"rustc": rustc, "cargo": cargo} {
		identity.Versions[name] = "release: " + release + "\n"
		f.stdoutMutation("build-"+name+"-version", func([]byte) []byte { return []byte(identity.Versions[name]) })
	}
	f.replace("build/identity.json", eligibilityJSON(t, identity))
	for _, inv := range f.reg.Invocations {
		if inv.Kind == "REFERENCE_PROCESS" && inv.CohortID != "build" && inv.CatalogueID != "batch-truncated" {
			f.stdoutMutation(inv.ID, func(data []byte) []byte {
				return bytes.ReplaceAll(data, []byte(instrumentedReferenceCommit), []byte(f.cat.UpstreamCommit))
			})
		}
	}
	for i := range f.reg.Observations {
		obs := &f.reg.Observations[i]
		var receipt eligibilityReceipt
		if err := json.Unmarshal(f.files[obs.InvocationID+"/receipt.json"], &receipt); err != nil {
			t.Fatal(err)
		}
		stdout, err := base64.StdEncoding.DecodeString(receipt.StdoutBase64)
		if err != nil {
			t.Fatal(err)
		}
		obs.SHA256 = digestHex(stdout[obs.ByteOffset : obs.ByteOffset+obs.ByteLength])
	}
	var summary eligibilityControlSummary
	if err := json.Unmarshal(f.files[f.reg.Control.Summary.Member.Path], &summary); err != nil {
		t.Fatal(err)
	}
	summary.IdentitySHA256 = f.reg.Build.Identity.Member.SHA256
	f.replace(f.reg.Control.Summary.Member.Path, eligibilityJSON(t, summary))
	eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) {
		tuple.RustcRelease, tuple.CargoRelease, tuple.CatalogueSHA256 = rustc, cargo, f.reg.CatalogueSHA256
	})
	return eligibilityPinRequest(t, f.request(), materials)
}

func TestEligibilityPinsProductionIndependence(t *testing.T) {
	ctx := pinTestContext(t)
	f := newEligibilityFixture(t)
	old := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	original := append([]byte(nil), old.Approval...)
	want, err := VerifyQueryReferenceEligibility(ctx, old)
	if err != nil || want.Assessment != AssessPass || !want.Complete || want.ResultSchema != QueryReferenceEligibilityPinsResultSchema || want.Build.Verified != 9 || want.Unit.Verified != 4 || want.Control.Verified != 53 || want.Assertion.Verified != 36 || want.Tuple.Verified != 3 {
		t.Fatalf("old historical proof rejected: %+v %v", want, err)
	}
	if want.Pins.SubjectSHA256 != want.ApprovalSHA256 || want.ReferenceTupleSHA256 == "" || want.ReferencePathReader == "" || want.Work <= want.Pins.Work || want.BytesRead <= want.Pins.BytesRead {
		t.Fatal("missing consumed identity/shared ledger", want)
	}
	for _, release := range []string{"1.98.2", "2.0.0"} {
		current := eligibilityPinGeneration(t, release, release)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(ctx, selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, err := VerifyQueryReferenceEligibility(ctx, current)
		if err != nil || got.Assessment != AssessPass || !got.Complete || got.CatalogueSHA256 == want.CatalogueSHA256 {
			t.Fatalf("new approved tuple rejected: %+v %v", got, err)
		}
		got, err = VerifyQueryReferenceEligibility(ctx, old)
		if err != nil || !reflect.DeepEqual(got, want) || !bytes.Equal(old.Approval, original) {
			t.Fatalf("production selection %s changed historical proof: %+v %v", release, got, err)
		}
	}
}

func TestEligibilityPinsCatalogueObligations(t *testing.T) {
	for _, mode := range []string{"build-denominator", "unit-denominator", "control-denominator", "assertion-denominator", "empty-assertion", "known-rule", "same-count-id", "request-limit", "response", "input"} {
		t.Run(mode, func(t *testing.T) {
			f := newEligibilityFixture(t)
			switch mode {
			case "build-denominator":
				f.cat.Builds = f.cat.Builds[:8]
			case "unit-denominator":
				f.cat.Units = f.cat.Units[:3]
			case "control-denominator":
				f.cat.Controls = f.cat.Controls[:52]
			case "assertion-denominator":
				f.cat.Assertions = f.cat.Assertions[:35]
			case "empty-assertion":
				f.cat.Assertions[0].RequiredInvocations = []string{}
			case "known-rule":
				f.cat.Controls[0].Rule = "invented-pass"
			case "same-count-id":
				f.cat.Controls[0].ID = "invented-control"
			case "request-limit":
				f.cat.Builds[0].MemoryBytes *= 2
			case "response":
				delete(f.cat.Responses, "response-25")
			case "input":
				for name := range f.cat.Inputs {
					delete(f.cat.Inputs, name)
					break
				}
			}
			materials := eligibilityPinMaterials(t)
			materials["reference/catalogue.json"] = eligibilityJSON(t, f.cat)
			f.reg.CatalogueSHA256 = digestHex(materials["reference/catalogue.json"])
			eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.CatalogueSHA256 = f.reg.CatalogueSHA256 })
			// Even independently approving reduced metadata cannot change reader obligations.
			req := eligibilityPinRequest(t, f.request(), materials)
			got, err := VerifyQueryReferenceEligibility(pinTestContext(t), req)
			if err != nil || got.Assessment != AssessFail || !eligibilityFinding(got, "PIN_CATALOGUE_OBLIGATIONS") {
				t.Fatalf("catalogue obligations forgery escaped guard: %+v %v", got, err)
			}
		})
	}
}

func TestEligibilityPinsForgery(t *testing.T) {
	for _, mode := range []string{"unapproved-set", "self-rehash", "sidecar-reuse", "wrong-subject", "catalogue-substitution", "mixed-rustc", "mixed-cargo", "duplicate-release", "release-suffix", "tuple-catalogue", "input-binding", "receipt-binding", "tool-binding", "source-binding", "vendor-binding", "response-upstream", "unsupported-reader", "unknown-tuple", "missing-material", "missing-role", "missing-approval"} {
		t.Run(mode, func(t *testing.T) {
			f := newEligibilityFixture(t)
			materials := eligibilityPinMaterials(t)
			code, assess, typed := "", AssessFail, false
			switch mode {
			case "mixed-rustc", "mixed-cargo", "duplicate-release", "release-suffix":
				eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) {
					if mode == "mixed-cargo" {
						tuple.CargoRelease = "2.0.0"
					} else {
						tuple.RustcRelease = "2.0.0"
					}
				})
				if mode == "duplicate-release" || mode == "release-suffix" {
					var identity eligibilityBuildIdentity
					if err := json.Unmarshal(f.files["build/identity.json"], &identity); err != nil {
						t.Fatal(err)
					}
					text := "release: 2.0.0\nrelease: 2.0.0\n"
					if mode == "release-suffix" {
						text = "release: 2.0.0-nightly\n"
					}
					identity.Versions["rustc"] = text
					f.stdoutMutation("build-rustc-version", func([]byte) []byte { return []byte(text) })
					f.replace("build/identity.json", eligibilityJSON(t, identity))
				}
				code = "ELIGIBILITY_TOOL_VERSION"
			case "tuple-catalogue":
				eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.CatalogueSHA256 = strings.Repeat("a", 64) })
				code = "PIN_REFERENCE_CATALOGUE"
			case "input-binding":
				materials["reference/build.py"] = []byte("unrelated controller")
				code = "PIN_REFERENCE_INPUT"
			case "receipt-binding":
				f.stdoutMutation("build-rustc-version", func([]byte) []byte { return []byte("release: 2.0.0\n") })
				code = "ELIGIBILITY_VERSION_RECEIPT"
			case "tool-binding":
				f.reg.Tools[0].SubjectSHA256 = strings.Repeat("a", 64)
				code = "ELIGIBILITY_TOOL_BINDING"
			case "source-binding":
				var rows map[string]eligibilitySnapshotRow
				if err := json.Unmarshal(f.files["build/source-snapshot.json"], &rows); err != nil {
					t.Fatal(err)
				}
				delete(rows, "Cargo.lock")
				f.replace("build/source-snapshot.json", eligibilityJSON(t, rows))
				code = "ELIGIBILITY_SOURCE_RELATION"
			case "vendor-binding":
				f.replace("build/vendor-snapshot.json", []byte(`{}`))
				code = "ELIGIBILITY_VENDOR_EMPTY"
			case "response-upstream":
				f.stdoutMutation("control-structural-0", func(data []byte) []byte {
					return bytes.ReplaceAll(data, []byte(instrumentedReferenceCommit), []byte(strings.Repeat("b", 40)))
				})
				code = "ELIGIBILITY_MANUAL_RESPONSE"
			case "unsupported-reader":
				eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.PathReader = "unknown/r1" })
				typed = true
			case "unknown-tuple":
				materials["reference/tuple.json"] = append(bytes.TrimSpace(materials["reference/tuple.json"])[:len(bytes.TrimSpace(materials["reference/tuple.json"]))-1], []byte(`,"extra":true}`)...)
				typed = true
			case "missing-role":
				delete(materials, "reference/tuple.json")
				code, assess = "PIN_REFERENCE_MISSING", AssessBlocked
			}
			req := eligibilityPinRequest(t, f.request(), materials)
			switch mode {
			case "unapproved-set":
				req.Pins.ApprovedPinSetSHA256 = strings.Repeat("a", 64)
				code = "PIN_APPROVAL_MISMATCH"
			case "self-rehash":
				approved := req.Pins.ApprovedApprovalSHA256
				req.Pins.PinSet = append(req.Pins.PinSet, ' ')
				pinTestApprove(t, req.Pins)
				req.Pins.ApprovedApprovalSHA256 = approved
				code = "PIN_APPROVAL_MISMATCH"
			case "sidecar-reuse":
				req.Approval = append(req.Approval, ' ')
				code = "PIN_ELIGIBILITY_SUBJECT"
			case "wrong-subject":
				req.Pins.Subject = req.Registration
				pinTestApprove(t, req.Pins)
				code = "PIN_ELIGIBILITY_SUBJECT"
			case "catalogue-substitution":
				req.Pins.Materials["reference/catalogue.json"] = append(req.Pins.Materials["reference/catalogue.json"], ' ')
				code = "PIN_MATERIAL_MISMATCH"
			case "missing-material":
				delete(req.Pins.Materials, "reference/catalogue.json")
				code, assess = "PIN_MATERIAL_MISSING", AssessBlocked
			case "missing-approval":
				req.Pins.Approval = nil
				code, assess = "PIN_APPROVAL_MISSING", AssessBlocked
			}
			got, err := VerifyQueryReferenceEligibility(pinTestContext(t), req)
			if typed {
				if err == nil || got.Complete {
					t.Fatalf("unsupported/malformed pin accepted: %+v %v", got, err)
				}
			} else if err != nil || got.Assessment != assess || !eligibilityFinding(got, code) {
				t.Fatalf("forgery %s accepted: %+v %v", mode, got, err)
			}
			eligibilityAssertCounts(t, got)
		})
	}
}

func TestEligibilityPinsSharedBudget(t *testing.T) {
	f := newEligibilityFixture(t)
	req := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	r, e := startRun(pinTestContext(t), ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	defer r.cancel()
	b := &eligibilityBudget{r: r, work: eligibilityWorkLimit - 1}
	res := QueryReferenceEligibilityResult{Report: newReport("query reference eligibility")}
	s := &eligibilitySession{r: r, b: b, res: &res, bound: true}
	err := s.referencePins(req)
	if err == nil || err.Code != "ELIGIBILITY_WORK_LIMIT" || b.work != eligibilityWorkLimit || res.Pins == nil || res.Pins.Complete {
		t.Fatalf("pin resolver reset inherited budget: %+v %v work=%d", res, err, b.work)
	}
}

func TestEligibilityPinsReaderIdentity(t *testing.T) {
	if digestHex(eligibilityObligationsR1) != "282d67dac6df7e8cd862cb596f2d5e7bc28bd1a94b6f176791dd8b8e9058d018" {
		t.Fatal("historical reader obligations changed; review a new reader instead")
	}
}

func TestEligibilityPinsEarlyResults(t *testing.T) {
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), QueryReferenceEligibilityRequest{Pins: &EvidencePinsRequest{}})
	if err != nil || got.ResultSchema != QueryReferenceEligibilityPinsResultSchema || got.Assessment != AssessBlocked || got.Complete {
		t.Fatalf("missing eligibility approval lost opt-in schema: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(pinTestContext(t))
	cancel()
	pins, err := ResolveEvidencePins(ctx, EvidencePinsRequest{})
	if err == nil || pins.ResultSchema != EvidencePinResultSchema || pins.Reader != EvidencePinReader || pins.Scope != EvidencePinScope || pins.Materials == nil {
		t.Fatalf("early error lost legacy pin result identity: %+v %v", pins, err)
	}
	got, err = VerifyQueryReferenceEligibility(ctx, QueryReferenceEligibilityRequest{Pins: &EvidencePinsRequest{}})
	if err == nil || got.ResultSchema != QueryReferenceEligibilityPinsResultSchema {
		t.Fatalf("early error lost eligibility opt-in schema: %+v %v", got, err)
	}
}

func TestEligibilityPinsTruncatedFindingsPreserveFailure(t *testing.T) {
	f := newEligibilityFixture(t)
	req := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	var set evidencePinSet
	if err := json.Unmarshal(req.Pins.PinSet, &set); err != nil {
		t.Fatal(err)
	}
	missing := []EvidencePinMaterial{}
	for i := 0; i < 1000; i++ {
		missing = append(missing, EvidencePinMaterial{Name: fmt.Sprintf("missing/%04d", i), Role: "reference-input", SHA256: digestHex(nil)})
	}
	set.Materials = append(missing, set.Materials...)
	req.Pins.PinSet = eligibilityJSON(t, set)
	pinTestApprove(t, req.Pins)
	req.Pins.Materials["reference/build.py"] = []byte("tampered after independent approval")
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), req)
	if err != nil || got.Assessment != AssessFail || got.Pins.Assessment != AssessFail || len(got.Findings) != 1000 {
		t.Fatalf("truncated nested findings hid FAIL: outer=%s inner=%s findings=%d err=%v", got.Assessment, got.Pins.Assessment, len(got.Findings), err)
	}
}
