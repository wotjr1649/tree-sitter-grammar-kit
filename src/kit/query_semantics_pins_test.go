package kit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The incomplete Oracle set deliberately stops after the real nested eligibility pass.
func semanticPinsRequest(t *testing.T, eligibility QueryReferenceEligibilityRequest) QuerySemanticsRequest {
	t.Helper()
	reg, profile := semanticRegistrationFixture(t)
	registration, profile := semanticEncodeRegistration(t, reg, profile)
	if err := os.MkdirAll(filepath.Join(eligibility.Evidence.Root, "oracle"), 0700); err != nil {
		t.Fatal(err)
	}
	return QuerySemanticsRequest{Evidence: eligibility.Evidence, Profile: semanticProfileBytes(t, profile), Registration: registration,
		EligibilityApproval: eligibility.Approval, EligibilityRegistration: eligibility.Registration, WitnessRegistration: eligibility.WitnessRegistration, Pins: eligibility.Pins}
}

func TestSemanticPinsForwarding(t *testing.T) {
	f := newEligibilityFixture(t)
	eligibility := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	req := semanticPinsRequest(t, eligibility)
	ctx := pinTestContext(t)
	want, err := VerifyQuerySemantics(ctx, req)
	if err == nil || want.Complete || want.ResultSchema != QuerySemanticsPinsResultSchema || want.Eligibility == nil || want.Eligibility.ResultSchema != QueryReferenceEligibilityPinsResultSchema || want.Eligibility.Assessment != AssessPass || want.Eligibility.Pins == nil || want.Eligibility.Pins.Assessment != AssessPass {
		t.Fatalf("semantic pin forwarding missing before incomplete Oracle set: %+v %v", want, err)
	}
	if want.Work <= want.Eligibility.Pins.Work || want.BytesRead <= want.Eligibility.Pins.BytesRead {
		t.Fatal("nested pin resolver did not use the semantic ledger")
	}
	original := append([]byte(nil), req.EligibilityApproval...)
	for _, release := range []string{"1.98.2", "2.0.0"} {
		current := eligibilityPinGeneration(t, release, release)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(ctx, selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, gotErr := VerifyQuerySemantics(ctx, req)
		if gotErr == nil || asError(gotErr).Code != asError(err).Code || !reflect.DeepEqual(got, want) || !bytes.Equal(original, req.EligibilityApproval) {
			t.Fatalf("production selection changed semantic historical result: %+v %v", got, gotErr)
		}
	}
	legacy := req
	legacy.Pins = nil
	got, err := VerifyQuerySemantics(ctx, legacy)
	if err == nil || got.ResultSchema != QuerySemanticsResultSchema || got.Eligibility == nil || got.Eligibility.ResultSchema != QueryReferenceEligibilityResultSchema || got.Eligibility.Pins != nil {
		t.Fatalf("legacy semantic result changed: %+v %v", got, err)
	}
}

func TestSemanticPinsRejectForgery(t *testing.T) {
	for _, mode := range []string{"unapproved", "forged", "reduced-catalogue"} {
		t.Run(mode, func(t *testing.T) {
			f := newEligibilityFixture(t)
			materials := eligibilityPinMaterials(t)
			if mode == "reduced-catalogue" {
				f.cat.Controls = f.cat.Controls[:52]
				materials["reference/catalogue.json"] = eligibilityJSON(t, f.cat)
				f.reg.CatalogueSHA256 = digestHex(materials["reference/catalogue.json"])
				eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.CatalogueSHA256 = f.reg.CatalogueSHA256 })
			}
			eligibility := eligibilityPinRequest(t, f.request(), materials)
			code := "PIN_CATALOGUE_OBLIGATIONS"
			if mode == "unapproved" {
				eligibility.Pins.ApprovedPinSetSHA256 = ""
				code = "PIN_APPROVAL_MISSING"
			} else if mode == "forged" {
				eligibility.Pins.ApprovedPinSetSHA256 = strings.Repeat("0", 64)
				code = "PIN_APPROVAL_MISMATCH"
			}
			got, err := VerifyQuerySemantics(pinTestContext(t), semanticPinsRequest(t, eligibility))
			if err == nil || got.Complete || got.ResultSchema != QuerySemanticsPinsResultSchema || got.Eligibility == nil || got.Eligibility.ResultSchema != QueryReferenceEligibilityPinsResultSchema || got.Eligibility.Assessment == AssessPass || !eligibilityFinding(*got.Eligibility, code) {
				t.Fatalf("semantic pin guard escaped: %+v %v", got, err)
			}
		})
	}
}

func TestSemanticPinsEarlySchema(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := VerifyQuerySemantics(ctx, QuerySemanticsRequest{Pins: &EvidencePinsRequest{}})
	if err == nil || got.ResultSchema != QuerySemanticsPinsResultSchema || got.Complete {
		t.Fatalf("early opt-in schema lost: %+v %v", got, err)
	}
}
