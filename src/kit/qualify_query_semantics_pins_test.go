package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func qualificationPinnedFixture(t *testing.T, reducedCatalogue bool) QualifyRequest {
	return qualificationPinnedFixtureWithRuntime(t, reducedCatalogue, nil)
}

func qualificationPinnedFixtureWithRuntime(t *testing.T, reducedCatalogue bool, runtimeMaterials map[string][]byte) QualifyRequest {
	t.Helper()
	const platform = "linux/amd64"
	f := newEligibilityFixture(t)
	if reducedCatalogue {
		f.cat.Controls = f.cat.Controls[:52]
		f.reg.CatalogueSHA256 = digestHex(eligibilityJSON(t, f.cat))
	}
	for i := range f.reg.Invocations {
		inv := &f.reg.Invocations[i]
		if inv.CohortID != "current" {
			continue
		}
		node := f.nodes[inv.ID]
		node.Identities["source"] = qfxCandidate
		f.nodes[inv.ID] = node
		inv.Node.Identities = node.Identities
	}
	if runtimeMaterials != nil {
		runtimePinRebind(t, f, runtimeMaterials)
	}
	eligibility := f.request()
	var approval eligibilityApproval
	if err := json.Unmarshal(eligibility.Approval, &approval); err != nil {
		t.Fatal(err)
	}
	approval.Candidate = eligibilityCandidate{SourceRevision: qfxCandidate, Platform: platform, Run: "500", Attempt: 1}
	var currentID string
	for i := range approval.Cohorts {
		if approval.Cohorts[i].Purpose == "current" {
			c := &approval.Cohorts[i]
			c.SourceRevision, c.Platform, c.Run, c.Attempt = qfxCandidate, platform, "500", 1
			currentID = c.ProvenanceID
		}
	}
	// Only the external CI provenance changes; registration, graph and witness bytes stay fixed.
	for i := range approval.Provenance {
		p := &approval.Provenance[i]
		if p.ID == currentID {
			p.Kind, p.SourceRevision = "reviewed-ci", qfxCandidate
			p.Repository, p.WorkflowPath, p.WorkflowRevision = "o/r", ".github/workflows/foundation.yml", strings.Repeat("d", 40)
			p.RunID, p.Attempt, p.ArtifactID = "500", 1, "synthetic-qualification-artifact"
			p.ArtifactSHA256, p.SessionID = digestHex([]byte(p.ArtifactID)), ""
		}
	}
	eligibility.Approval = eligibilityJSON(t, approval)
	materials := eligibilityPinMaterials(t)
	if reducedCatalogue {
		materials["reference/catalogue.json"] = eligibilityJSON(t, f.cat)
		eligibilityPinTuple(t, materials, func(tuple *eligibilityReferenceTuple) { tuple.CatalogueSHA256 = f.reg.CatalogueSHA256 })
	}
	if runtimeMaterials != nil {
		eligibility = runtimePinRequestFor(t, eligibility, runtimeMaterials)
	} else {
		eligibility = eligibilityPinRequest(t, eligibility, materials)
	}
	semantic := semanticPinsRequest(t, eligibility)
	var parsed semanticRegistration
	if err := json.Unmarshal(semantic.Registration, &parsed); err != nil {
		t.Fatal(err)
	}
	fx := newQfx(t)
	fx.inv.Platforms = []QualPlatform{{ID: "linux-amd64", GOOS: "linux", GOARCH: "amd64"}}
	workload := qfxWorkload("s06-json", parsed.Route, qfxCases(parsed.Route))
	fx.inv.Routes = []QualRoute{{Route: parsed.Route, Workload: workload,
		Requirements: []QualRequirement{{Row: parsed.Route + "-B01", Kinds: []string{"P", "E", "Q"}, AlternativesStatus: AlternativesComplete, Alternatives: []string{parsed.Route + "-B01.a01"}}}}}
	fx.inv.Cells, fx.inv.ExtraRoles = 1, []QualRole{}
	host := fx.host(t, fx.inv.Platforms[0], qfxMut{})
	inventory := fx.invBytes(t)

	plan := qualSemanticPlan{Schema: QualificationSemanticsPlanSchema, ID: "pin-qualification-probe", InventorySHA256: digestHex(inventory),
		Context: "full-input-utf8/r2", Scope: qualSemanticScope}
	target := qualSemanticPlanTarget{ID: "semantic-linux", Route: parsed.Route, Role: "primary", Set: workload.Set, Platform: "linux-amd64",
		ProbeID: parsed.Queries[0].ID, ProfileSHA256: digestHex(semantic.Profile), RegistrationSHA256: digestHex(semantic.Registration),
		Queries: []QualQuery{}, Required: []qualSemanticTuple{}}
	for _, q := range parsed.Queries {
		target.Queries = append(target.Queries, QualQuery{ID: q.ID, SHA256: digestHex([]byte(q.Source))})
	}
	for _, tuple := range parsed.Required {
		target.Required = append(target.Required, qualSemanticTuple{Case: parsed.Case, Query: tuple.Query, Step: tuple.Step, TreeRole: tuple.TreeRole})
	}
	plan.Targets = []qualSemanticPlanTarget{target}
	planBytes := eligibilityJSON(t, plan)
	graph, err := os.ReadFile(filepath.Join(semantic.Evidence.Root, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	qualContext := qualSemanticContext{Schema: QualificationSemanticsContextSchema, PlanSHA256: digestHex(planBytes), TargetID: target.ID,
		Repository: "o/r", Workflow: "Foundation", WorkflowPath: ".github/workflows/foundation.yml", WorkflowRevision: strings.Repeat("d", 40),
		RunID: "500", Attempt: 1, Event: "pull_request", HeadSHA: qualificationSemanticHead, Checkout: qfxCandidate,
		ArtifactID: "synthetic-qualification-artifact", ArtifactSHA256: digestHex([]byte("synthetic-qualification-artifact")), GraphSHA256: digestHex(graph)}
	contextBytes := eligibilityJSON(t, qualContext)
	for i := range approval.Provenance {
		if approval.Provenance[i].ID == currentID {
			approval.Provenance[i].ReviewedContextSHA256 = digestHex(contextBytes)
		}
	}
	semantic.EligibilityApproval = eligibilityJSON(t, approval)
	semantic.Pins.Subject = slices.Clone(semantic.EligibilityApproval)
	var sidecar evidencePinApproval
	if err := json.Unmarshal(semantic.Pins.Approval, &sidecar); err != nil {
		t.Fatal(err)
	}
	sidecar.SubjectSHA256 = digestHex(semantic.Pins.Subject)
	semantic.Pins.Approval = eligibilityJSON(t, sidecar)
	semantic.Pins.ApprovedApprovalSHA256 = digestHex(semantic.Pins.Approval)

	replay := pinnedSemanticReplayFor(t, semantic)
	replay.Pins = nil // QualificationQuerySemanticsInput is the only source of Pins in this test.
	qualInput := QualificationQuerySemanticsInput{ID: target.ID, Replay: replay, EligibilityApproval: slices.Clone(semantic.EligibilityApproval),
		Pins: semantic.Pins, Context: contextBytes}
	return QualifyRequest{Inventory: inventory, Candidate: qfxCandidate, Hosts: []QualifyHost{{Platform: "linux-amd64", Root: host}},
		QuerySemantics: &QualificationQuerySemanticsRequest{Plan: planBytes, Targets: []QualificationQuerySemanticsInput{qualInput}}}
}

func TestQualificationPinsForwardingAndProductionIndependence(t *testing.T) {
	t.Parallel()
	req := qualificationPinnedFixture(t, false)
	ctx := pinTestContext(t)
	want, err := Qualify(ctx, req)
	if err == nil || want.ResultSchema != QualificationSemanticsPinsResultSchema || want.QuerySemantics == nil {
		t.Fatalf("qualification did not produce the opt-in partial result: %+v %v", want, err)
	}
	row := qualificationPinsTarget(t, want, "semantic-linux")
	core := row.Core
	if core == nil || core.ResultSchema != QuerySemanticsPinsResultSchema || core.Eligibility == nil ||
		core.Eligibility.ResultSchema != QueryReferenceEligibilityPinsResultSchema || core.Eligibility.Assessment != AssessPass ||
		!validEvidencePinsResult(core.Eligibility.Pins) || core.Eligibility.Pins.Assessment != AssessPass {
		t.Fatalf("qualification failed to forward caller Pins through replay to eligibility: %+v %v", row, err)
	}
	for _, release := range []string{"1.98.2", "2.0.0"} {
		current := eligibilityPinGeneration(t, release, release)
		selection := eligibilityJSON(t, map[string]string{"schema": EvidencePinSelectionSchema, "pin_set_sha256": current.Pins.ApprovedPinSetSHA256})
		if _, selectErr := SelectEvidenceProductionPin(ctx, selection, current.Pins.PinSet); selectErr != nil {
			t.Fatal(selectErr)
		}
		got, gotErr := Qualify(ctx, req)
		if gotErr == nil || asError(gotErr).Code != asError(err).Code || !reflect.DeepEqual(got, want) {
			t.Fatalf("production selection %s changed historical qualification: %+v %v", release, got, gotErr)
		}
	}
}

func TestQualificationPinsRejectUnapprovedAndReducedCatalogue(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"unapproved", "reduced-catalogue"} {
		t.Run(mode, func(t *testing.T) {
			req := qualificationPinnedFixture(t, mode == "reduced-catalogue")
			pins := req.QuerySemantics.Targets[0].Pins
			if mode == "unapproved" {
				pins.ApprovedPinSetSHA256 = ""
			}
			got, err := Qualify(pinTestContext(t), req)
			if err == nil || got.ResultSchema != QualificationSemanticsPinsResultSchema || got.QuerySemantics == nil {
				t.Fatalf("qualification pin rejection lost partial result: %+v %v", got, err)
			}
			row := qualificationPinsTarget(t, got, "semantic-linux")
			core := row.Core
			if core == nil || core.Eligibility == nil || core.Eligibility.Assessment == AssessPass || mode == "unapproved" && !eligibilityFinding(*core.Eligibility, "PIN_APPROVAL_MISSING") || mode == "reduced-catalogue" && !eligibilityFinding(*core.Eligibility, "PIN_CATALOGUE_OBLIGATIONS") {
				t.Fatalf("qualification accepted %s historical pin input: %+v", mode, row)
			}
		})
	}
}

func qualificationPinsTarget(t *testing.T, result QualificationResult, id string) QualQuerySemanticsTarget {
	t.Helper()
	for _, target := range result.QuerySemantics.Targets {
		if target.ID == id {
			return target
		}
	}
	t.Fatalf("qualification result omitted target %q: %+v", id, result.QuerySemantics)
	return QualQuerySemanticsTarget{}
}
