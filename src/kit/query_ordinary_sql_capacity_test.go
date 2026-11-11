package kit

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func ordinarySQLCapacityRequest(t *testing.T, routeName string) (OrdinaryQuerySemanticsRequest, string, string) {
	t.Helper()
	v, inventory := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inventory)
	routeIndex := ordinaryRouteIndex(inventory, routeName)
	route := inventory.Routes[routeIndex]
	p := ordinaryProfileFixture()
	p.Candidate, p.SemanticCohort, p.Bindings = plan.Candidate, plan.SemanticCohort, plan.Bindings
	planningProfile := ordinaryJSON(t, p)
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: plan.Registry, Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, Assets: []OrdinaryAsset{}}
	for _, asset := range []struct{ id, role string }{{"m", "metadata"}, {"t", "expected-tree"}, {"c", "expected-captures"}, {"f", "expected-facts"}} {
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: asset.id, Role: asset.role, State: "UNAVAILABLE"})
	}
	assetData := ordinaryJSON(t, assets)
	plan.Profile = queryregistrycore.FilePin("profile/ordinary.json", planningProfile)
	plan.Assets = queryregistrycore.FilePin("profile/assets.json", assetData)
	registration := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: plan.Registry, Namespace: plan.Namespace,
		Inventory: queryregistrycore.QualificationPin, Slots: queryregistrycore.SlotPin, Profile: plan.Profile, Assets: plan.Assets,
		Route: route.Route, Set: route.Workload.Set, Platform: "windows-amd64", Subject: "windows/amd64", MembershipContext: "qualification/r8",
		Context: OrdinaryContext, ClosureEncoding: "plain", Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, PlannedMembers: []OrdinaryMember{}}
	selectedIndex := -1
	for i, shard := range plan.Shards {
		if shard.Route == route.Route && shard.Platform == registration.Platform {
			selectedIndex = i
			break
		}
	}
	if selectedIndex < 0 {
		t.Fatalf("no selected shard for %s", routeName)
	}
	for i, original := range route.Workload.Cases {
		index := slices.IndexFunc(plan.Shards, func(shard OrdinaryPlanShard) bool {
			return shard.Case == original.ID && shard.Route == route.Route && shard.Platform == registration.Platform
		})
		if index < 0 {
			t.Fatalf("no shard for %s case %s", routeName, original.ID)
		}
		shard := &plan.Shards[index]
		shard.SourceHistory[0] = OrdinaryPlanHistoryStep{Bytes: original.Input.Bytes, SHA256: original.Input.SHA256}
		shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, []OrdinaryMember{})
		c := OrdinaryCase{ID: original.ID, OriginalSelector: "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i),
			Edits: original.Edits, Points: shard.EditPoints}
		for _, version := range shard.SourceHistory {
			c.VersionPins = append(c.VersionPins, OrdinaryPin{Path: "source-witness/" + version.SHA256 + ".utf8", Bytes: version.Bytes, SHA256: version.SHA256})
		}
		c.InitialSource = c.VersionPins[0]
		registration.Cases = append(registration.Cases, c)
	}
	for _, query := range route.Workload.Queries {
		registration.Queries = append(registration.Queries, OrdinaryQuery{ID: query.ID,
			Source: OrdinaryPin{Path: "queries/" + query.ID, Bytes: 1, SHA256: query.SHA256}, MetadataAsset: "m", Kind: queryregistrycore.Structural})
	}
	for _, coord := range v.Coordinates() {
		if coord.Kind != queryregistrycore.Structural || coord.Route != route.Route || coord.Platform != registration.Platform {
			continue
		}
		required := OrdinaryRequired{ID: coord.ID(registration.Namespace), Coordinate: coord, TreeAsset: "t", CaptureAsset: "c"}
		if strings.HasPrefix(coord.Query, "facts.") {
			required.FactAsset = "f"
		}
		registration.Required = append(registration.Required, required)
	}
	registrationData := ordinaryJSON(t, registration)
	plan.Registrations = []OrdinaryPin{queryregistrycore.FilePin("registrations/"+routeName+"-windows.json", registrationData)}
	commitmentData, selection := ordinarySelectionForTest(t, plan, []int{selectedIndex})
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	registry, qualification, slots := read("../contracts/query-semantic-registry-r1.json"), read("../contracts/qualification-c1.json"), read("../contracts/query-slot-inventory-r1.json")
	request := OrdinaryQuerySemanticsRequest{Selection: OrdinaryQuerySemanticsBinding(selection.Binding), PlanningProfile: planningProfile, Registration: registrationData,
		AssetRegistration: assetData, PlanCommitment: commitmentData, ShardProofs: selection.Proofs, Registry: registry, RegistrySHA256: ordinaryRegistrySHA,
		QualificationInventory: qualification, SlotInventory: slots}
	return request, plan.Shards[selectedIndex].Case, plan.Shards[selectedIndex].ID
}

func TestOrdinarySQLPassCapacityBeforeExternalApproval(t *testing.T) {
	for _, route := range []string{"postgresql-sql", "tsql"} {
		t.Run(route, func(t *testing.T) {
			req, caseID, shardID := ordinarySQLCapacityRequest(t, route)
			ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
			defer cancel()
			res, err := VerifyOrdinaryQuerySemantics(ctx, req)
			if err != nil {
				t.Fatalf("caller admission unexpectedly failed: %v; result=%+v", err, res)
			}
			if res.Code != "ELIGIBILITY_APPROVAL_MISSING" || res.Semantic != AssessBlocked || res.Tuple.Required == 0 ||
				res.Tuple.Blocked != res.Tuple.Required || res.Tuple.Unprocessed != 0 || res.Complete {
				t.Fatalf("missing external approval did not preserve selected denominator: %+v", res)
			}
			limits := OrdinaryPlanLimits()
			if res.LogicalBytes > limits.Logical || res.Work > limits.Work || res.BytesRead > limits.ReadBytes {
				t.Fatalf("selected caller pass exceeded an established cap: logical=%d work=%d read=%d", res.LogicalBytes, res.Work, res.BytesRead)
			}
			t.Logf("route=%s case=%s shard=%s required=%d stage=caller-admission-then-missing-approval logical=%d work=%d read=%d remaining_logical=%d remaining_work=%d remaining_read=%d; native/eligibility/evidence/expected/reference closure excluded",
				route, caseID, shardID, res.Tuple.Required, res.LogicalBytes, res.Work, res.BytesRead,
				limits.Logical-res.LogicalBytes, limits.Work-res.Work, limits.ReadBytes-res.BytesRead)
		})
	}
}
