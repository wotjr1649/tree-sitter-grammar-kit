package kit

import (
	"reflect"
	"strings"
	"testing"
)

func ordinaryRegistrationSelectionFixture() (OrdinaryRegistration, OrdinaryPlanShard) {
	registration := OrdinaryRegistration{
		Schema:    "tsgk-query-semantics-registration/r2",
		Registry:  OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA},
		Namespace: strings.Repeat("a", 64),
		Inventory: OrdinaryPin{Path: "contracts/inventory.json", Bytes: 1, SHA256: strings.Repeat("b", 64)},
		Slots:     OrdinaryPin{Path: "contracts/slots.json", Bytes: 1, SHA256: strings.Repeat("c", 64)},
		Profile:   OrdinaryPin{Path: "profile/ordinary.json", Bytes: 1, SHA256: strings.Repeat("d", 64)},
		Assets:    OrdinaryPin{Path: "profile/assets.json", Bytes: 1, SHA256: strings.Repeat("e", 64)},
		Route:     "json", Set: "set_json", Platform: "windows-amd64", Subject: "windows/amd64",
		MembershipContext: "qualification/r8", Context: OrdinaryContext, ClosureEncoding: "plain",
		Candidate: "candidate", SemanticCohort: "cohort", Cases: []OrdinaryCase{}, Queries: []OrdinaryQuery{}, Required: []OrdinaryRequired{}, PlannedMembers: []OrdinaryMember{},
	}
	var selected OrdinaryPlanShard
	for _, caseID := range []string{"case-selected", "case-other"} {
		sha := strings.Repeat("f", 64)
		pin := OrdinaryPin{Path: "source-witness/" + sha + ".utf8", Bytes: 1, SHA256: sha}
		registration.Cases = append(registration.Cases, OrdinaryCase{
			ID: caseID, OriginalSelector: "qualification_inventory#/routes/0/workload/cases/0",
			InitialSource: pin, Edits: []Edit{}, VersionPins: []OrdinaryPin{pin}, Points: []EditPoints{},
		})
		coord := OrdinaryCoordinate{Kind: "STRUCTURAL_ONLY", Route: registration.Route, Platform: registration.Platform, Subject: registration.Subject,
			Role: "primary", Set: registration.Set, Case: caseID, Query: "q1", Step: 0, TreeRole: "incremental", Context: registration.MembershipContext}
		registration.Required = append(registration.Required, OrdinaryRequired{ID: coord.ID(registration.Namespace), Coordinate: coord, TreeAsset: "tree", CaptureAsset: "captures"})
		if caseID == "case-selected" {
			selected = OrdinaryPlanShard{Route: registration.Route, Set: registration.Set, Platform: registration.Platform, Subject: registration.Subject,
				Case: caseID, SourceHistory: []OrdinaryPlanHistoryStep{{SHA256: sha, Bytes: 1}}, ObligationIDs: []string{coord.ID(registration.Namespace)}}
		}
	}
	registration.Queries = []OrdinaryQuery{{ID: "q1", Source: OrdinaryPin{Path: "queries/q1.scm", Bytes: 1, SHA256: strings.Repeat("1", 64)}, MetadataAsset: "metadata", Kind: "STRUCTURAL_ONLY"}}
	return registration, selected
}

func TestOrdinaryRegistrationSelectionDecodesOnlySelectedRows(t *testing.T) {
	registration, shard := ordinaryRegistrationSelectionFixture()
	data := ordinaryJSON(t, registration)
	b := ordinaryTestBudget(t)
	got, e := b.decodeOrdinaryRegistrationSelection(data, []OrdinaryPlanShard{shard})
	if e != nil {
		t.Fatal(e)
	}
	if got.TotalCases != 2 || got.TotalQueries != 1 || !reflect.DeepEqual(got.CaseIndices, []int{0}) || len(got.Registration.Cases) != 1 || len(got.Registration.Queries) != 1 || len(got.Registration.Required) != 1 {
		t.Fatalf("unexpected selected projection: indices=%v cases=%d queries=%d required=%d", got.CaseIndices, len(got.Registration.Cases), len(got.Registration.Queries), len(got.Registration.Required))
	}

}

func TestOrdinaryRegistrationSelectionRejectsBrokenGlobalOrSelectedTuples(t *testing.T) {
	registration, shard := ordinaryRegistrationSelectionFixture()
	valid := ordinaryJSON(t, registration)
	if _, e := ordinaryTestBudget(t).decodeOrdinaryRegistrationSelection(valid, []OrdinaryPlanShard{shard}); e != nil {
		t.Fatal("baseline rejected:", e)
	}
	mutations := map[string]func(*OrdinaryRegistration){
		"duplicate-case-id":               func(r *OrdinaryRegistration) { r.Cases[1].ID = r.Cases[0].ID },
		"missing-global-tuple":            func(r *OrdinaryRegistration) { r.Required = r.Required[:1] },
		"selected-id-replacement":         func(r *OrdinaryRegistration) { r.Required[0].ID = "qsem1:" + strings.Repeat("9", 64) },
		"selected-coordinate-replacement": func(r *OrdinaryRegistration) { r.Required[0].Coordinate.Step = 1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := registration
			changed.Cases = append([]OrdinaryCase(nil), registration.Cases...)
			changed.Required = append([]OrdinaryRequired(nil), registration.Required...)
			mutate(&changed)
			if _, e := ordinaryTestBudget(t).decodeOrdinaryRegistrationSelection(ordinaryJSON(t, changed), []OrdinaryPlanShard{shard}); e == nil {
				t.Fatal("accepted changed registration tuple")
			}
		})
	}
	duplicateKey := strings.Replace(string(valid), `"context":"`+OrdinaryContext+`"`, `"context":"wrong","context":"`+OrdinaryContext+`"`, 1)
	if _, e := ordinaryTestBudget(t).decodeOrdinaryRegistrationSelection([]byte(duplicateKey), []OrdinaryPlanShard{shard}); e == nil {
		t.Fatal("accepted duplicate member name")
	}
}
