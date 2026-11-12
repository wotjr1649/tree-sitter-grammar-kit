package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pinnedSemanticReplayFixture(t *testing.T) (ReplayRequest, QuerySemanticsRequest) {
	t.Helper()
	f := newEligibilityFixture(t)
	eligibility := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	semantic := semanticPinsRequest(t, eligibility)
	return pinnedSemanticReplayFor(t, semantic), semantic
}

func pinnedSemanticReplayFor(t *testing.T, semantic QuerySemanticsRequest) ReplayRequest {
	t.Helper()
	var approval eligibilityApproval
	if err := json.Unmarshal(semantic.EligibilityApproval, &approval); err != nil {
		t.Fatal(err)
	}
	var registration semanticRegistration
	if err := json.Unmarshal(semantic.Registration, &registration); err != nil || len(registration.Profile.Cases) == 0 {
		t.Fatalf("invalid semantic fixture registration: %v", err)
	}
	root := t.TempDir()
	add := func(path, role string, data []byte) ReplayMember {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(path))), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0600); err != nil {
			t.Fatal(err)
		}
		return ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: digestHex(data)}
	}
	members := []ReplayMember{
		add("profile.json", "workload-profile", semantic.Profile),
		add("registration.json", "semantics-registration", semantic.Registration),
		add("run-binding.json", "semantics-run-binding", semantic.RunBinding),
		add("approval.json", "eligibility-approval", semantic.EligibilityApproval),
		add("eligibility-registration.json", "eligibility-registration", semantic.EligibilityRegistration),
		add("witness.json", "query-registration", semantic.WitnessRegistration),
		add("policy.json", "evidence-policy", semantic.Evidence.Policy),
	}
	if err := filepath.WalkDir(semantic.Evidence.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			rel, err := filepath.Rel(semantic.Evidence.Root, path)
			if err != nil {
				return err
			}
			if rel != "." {
				return os.MkdirAll(filepath.Join(root, "evidence", rel), 0700)
			}
			return nil
		}
		rel, err := filepath.Rel(semantic.Evidence.Root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		members = append(members, add("evidence/"+filepath.ToSlash(rel), "semantics-evidence", data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	profile := ReplayProfile{ID: "pinned-semantic-replay", Reducer: "oracle-set-r7", Operation: "evidence-replay",
		Subject: ReplaySubject{Run: approval.Candidate.Run, Attempt: approval.Candidate.Attempt, Platform: approval.Candidate.Platform, Commit: approval.Candidate.SourceRevision,
			EvidenceMode: ModeNewRun, ExecutionStatus: StatusCompleted, Assessment: AssessBlocked},
		Identities: map[string]string{"workload": digestHex(semantic.Profile), "semantics-registration": digestHex(semantic.Registration),
			"semantics-run-binding": digestHex(semantic.RunBinding), "evidence-policy": digestHex(semantic.Evidence.Policy)}, Members: members}
	profileBytes := mustJSON(t, map[string]any{"schema": ReplaySchema, "id": profile.ID, "reducer": profile.Reducer, "operation": profile.Operation,
		"subject": profile.Subject, "identities": profile.Identities, "records": []string{registration.Profile.Cases[0].ID}, "members": profile.Members})
	return ReplayRequest{Root: root, Profile: profileBytes, Pins: semantic.Pins}
}

func mustSemanticCase(t *testing.T, req ReplayRequest) string {
	t.Helper()
	var replay ReplayProfile
	if err := json.Unmarshal(req.Profile, &replay); err != nil || len(replay.Records) != 1 {
		t.Fatalf("invalid replay fixture records: %v", err)
	}
	return replay.Records[0]
}

func runPinnedSemanticReplay(t *testing.T, req ReplayRequest) (ReplayResult, error) {
	t.Helper()
	return Replay(pinTestContext(t), req)
}

func TestReplaySemanticPinsForwardingAndProductionIndependence(t *testing.T) {
	req, _ := pinnedSemanticReplayFixture(t)
	want, err := runPinnedSemanticReplay(t, req)
	if err == nil || want.ResultSchema != ReplaySemanticsPinsResultSchema || want.Reducer.ID != "oracle-set-r7" || want.QuerySemantics == nil ||
		want.QuerySemantics.ResultSchema != QuerySemanticsPinsResultSchema || want.QuerySemantics.Eligibility == nil ||
		want.QuerySemantics.Eligibility.ResultSchema != QueryReferenceEligibilityPinsResultSchema || !validEvidencePinsResult(want.QuerySemantics.Eligibility.Pins) {
		t.Fatalf("replay lost caller pin context: %+v %v", want, err)
	}
	for _, release := range []string{"1.98.2", "2.0.0"} {
		current := eligibilityPinGeneration(t, release, release)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, err := SelectEvidenceProductionPin(pinTestContext(t), selection, current.Pins.PinSet); err != nil {
			t.Fatal(err)
		}
		got, gotErr := runPinnedSemanticReplay(t, req)
		if gotErr == nil || asError(gotErr).Code != asError(err).Code || !reflect.DeepEqual(got, want) {
			t.Fatalf("production selection changed old replay result: %+v %v", got, gotErr)
		}
	}
}

func TestReplaySemanticPinsRejectUnapprovedAndReducedCatalogue(t *testing.T) {
	t.Run("unapproved", func(t *testing.T) {
		req, _ := pinnedSemanticReplayFixture(t)
		req.Pins.ApprovedPinSetSHA256 = ""
		got, err := runPinnedSemanticReplay(t, req)
		if err == nil || got.ResultSchema != ReplaySemanticsPinsResultSchema || got.QuerySemantics == nil || got.QuerySemantics.Eligibility == nil ||
			!eligibilityFinding(*got.QuerySemantics.Eligibility, "PIN_APPROVAL_MISSING") || !validEvidencePinsResult(got.QuerySemantics.Eligibility.Pins) || got.QuerySemantics.Eligibility.Pins.Assessment == AssessPass {
			t.Fatalf("replay accepted unapproved pins: %+v %v", got, err)
		}
	})
	t.Run("reduced-catalogue", func(t *testing.T) {
		f := newEligibilityFixture(t)
		materials := eligibilityPinMaterials(t)
		f.cat.Controls = f.cat.Controls[:52]
		materials["reference/catalogue.json"] = eligibilityJSON(t, f.cat)
		f.reg.CatalogueSHA256 = digestHex(materials["reference/catalogue.json"])
		eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.CatalogueSHA256 = f.reg.CatalogueSHA256 })
		eligibility := eligibilityPinRequest(t, f.request(), materials)
		semantic := semanticPinsRequest(t, eligibility)
		req := pinnedSemanticReplayFor(t, semantic)
		got, err := runPinnedSemanticReplay(t, req)
		if err == nil || got.QuerySemantics == nil || got.QuerySemantics.Eligibility == nil ||
			!eligibilityFinding(*got.QuerySemantics.Eligibility, "PIN_CATALOGUE_OBLIGATIONS") || got.QuerySemantics.Eligibility.Assessment == AssessPass {
			t.Fatalf("replay accepted reduced catalogue obligations: %+v %v", got, err)
		}
	})
}

func TestReplaySemanticPinsReducerAdmission(t *testing.T) {
	req, _ := pinnedSemanticReplayFixture(t)
	var profile ReplayProfile
	if err := json.Unmarshal(req.Profile, &profile); err != nil {
		t.Fatal(err)
	}
	profile.Reducer = "oracle-set-r4"
	profileBytes := mustJSON(t, map[string]any{"schema": ReplaySchema, "id": profile.ID, "reducer": profile.Reducer, "operation": profile.Operation,
		"subject": profile.Subject, "identities": profile.Identities, "records": []string{mustSemanticCase(t, req)}, "members": profile.Members})
	_, err := Replay(pinTestContext(t), ReplayRequest{Root: req.Root, Profile: profileBytes, Pins: req.Pins})
	if err == nil || !strings.Contains(err.Error(), "REPLAY_PINS_REDUCER_MISMATCH") {
		t.Fatalf("pin context crossed reducer revision: %v", err)
	}
	profile.Reducer = "oracle-set-r7"
	profileBytes = mustJSON(t, map[string]any{"schema": ReplaySchema, "id": profile.ID, "reducer": profile.Reducer, "operation": profile.Operation,
		"subject": profile.Subject, "identities": profile.Identities, "records": []string{mustSemanticCase(t, req)}, "members": profile.Members})
	_, err = Replay(pinTestContext(t), ReplayRequest{Root: req.Root, Profile: profileBytes})
	if err == nil || !strings.Contains(err.Error(), "REPLAY_PINS_REDUCER_MISMATCH") {
		t.Fatalf("pin reducer accepted absent caller context: %v", err)
	}
}

func TestStoredPinnedEligibilityCannotClaimUnverifiedPass(t *testing.T) {
	f := newEligibilityFixture(t)
	req := eligibilityPinRequest(t, f.request(), eligibilityPinMaterials(t))
	got, err := VerifyQueryReferenceEligibility(pinTestContext(t), req)
	if err != nil || got.Assessment != AssessPass || !validPassedPinnedEligibility(&got) {
		t.Fatalf("fixture is not a valid pinned eligibility pass: %+v %v", got, err)
	}
	blocked := got
	blocked.Pins = new(EvidencePinsResult)
	*blocked.Pins = *got.Pins
	blocked.Pins.Assessment = AssessBlocked
	if validPassedPinnedEligibility(&blocked) {
		t.Fatal("eligibility PASS accepted a blocked pin-material result")
	}
	wrongCatalogue := got
	wrongCatalogue.Pins = new(EvidencePinsResult)
	*wrongCatalogue.Pins = *got.Pins
	wrongCatalogue.Pins.Materials = append([]EvidencePinMaterial(nil), got.Pins.Materials...)
	for i := range wrongCatalogue.Pins.Materials {
		if wrongCatalogue.Pins.Materials[i].Name == "reference/catalogue.json" {
			wrongCatalogue.Pins.Materials[i].SHA256 = strings.Repeat("0", 64)
		}
	}
	if validPassedPinnedEligibility(&wrongCatalogue) {
		t.Fatal("eligibility PASS accepted a mismatched historical catalogue descriptor")
	}
	wrongSubject := got
	wrongSubject.Pins = new(EvidencePinsResult)
	*wrongSubject.Pins = *got.Pins
	wrongSubject.Pins.SubjectSHA256 = strings.Repeat("0", 64)
	if validPassedPinnedEligibility(&wrongSubject) {
		t.Fatal("eligibility PASS accepted an unrelated pin subject")
	}
}
