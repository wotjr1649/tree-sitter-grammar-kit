package kit

import (
	"context"
	"encoding/json"
	"slices"

	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ordinarycommit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func ordinaryTestBudget(t *testing.T) *eligibilityBudget {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ReplayOperations()["evidence-replay"].Wall)
	t.Cleanup(cancel)
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	b := &eligibilityBudget{r: r}
	r.eligibility = b
	return b
}

func TestOrdinaryReaderSelectedInventoryBudget(t *testing.T) {
	t.Parallel()
	v, inv := ordinaryP0ForPlanTest(t)
	for _, routeName := range []string{"json", "postgresql-sql", "tsql", "largest-shard"} {
		t.Run(routeName, func(t *testing.T) {
			plan := ordinaryPlanForTest(v, inv)
			selectedIndex := -1
			if routeName == "largest-shard" {
				for i, shard := range plan.Shards {
					if shard.Platform == "windows-amd64" && (selectedIndex < 0 || len(ordinaryJSON(t, shard)) > len(ordinaryJSON(t, plan.Shards[selectedIndex]))) {
						selectedIndex = i
					}
				}
				routeName = plan.Shards[selectedIndex].Route
			}
			routeIndex := ordinaryRouteIndex(inv, routeName)
			route := inv.Routes[routeIndex]
			p := ordinaryProfileFixture()
			p.Candidate, p.SemanticCohort, p.Bindings = plan.Candidate, plan.SemanticCohort, plan.Bindings
			pb := ordinaryJSON(t, p)
			a := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: plan.Registry, Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, Assets: []OrdinaryAsset{}}
			for _, x := range []struct{ id, role string }{{"m", "metadata"}, {"t", "expected-tree"}, {"c", "expected-captures"}, {"f", "expected-facts"}} {
				a.Assets = append(a.Assets, OrdinaryAsset{ID: x.id, Role: x.role, State: "UNAVAILABLE"})
			}
			ab := ordinaryJSON(t, a)
			plan.Profile, plan.Assets = queryregistrycore.FilePin("profile/ordinary.json", pb), queryregistrycore.FilePin("profile/assets.json", ab)
			reg := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: plan.Registry, Namespace: plan.Namespace, Inventory: queryregistrycore.QualificationPin, Slots: queryregistrycore.SlotPin, Profile: plan.Profile, Assets: plan.Assets,
				Route: route.Route, Set: route.Workload.Set, Platform: "windows-amd64", Subject: "windows/amd64", MembershipContext: "qualification/r8", Context: OrdinaryContext, ClosureEncoding: "plain", Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, PlannedMembers: []OrdinaryMember{}}
			for i, original := range route.Workload.Cases {
				index := -1
				for j := range plan.Shards {
					if plan.Shards[j].Case == original.ID && plan.Shards[j].Route == route.Route && plan.Shards[j].Platform == reg.Platform {
						index = j
						break
					}
				}
				shard := &plan.Shards[index]
				shard.SourceHistory[0] = OrdinaryPlanHistoryStep{Bytes: original.Input.Bytes, SHA256: original.Input.SHA256}
				shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, []OrdinaryMember{})
				c := OrdinaryCase{ID: original.ID, OriginalSelector: "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i), Edits: original.Edits, Points: shard.EditPoints}
				for _, version := range shard.SourceHistory {
					c.VersionPins = append(c.VersionPins, OrdinaryPin{Path: "source-witness/" + version.SHA256 + ".utf8", Bytes: version.Bytes, SHA256: version.SHA256})
				}
				c.InitialSource = c.VersionPins[0]
				reg.Cases = append(reg.Cases, c)
			}
			for _, q := range route.Workload.Queries {
				reg.Queries = append(reg.Queries, OrdinaryQuery{ID: q.ID, Source: OrdinaryPin{Path: "queries/" + q.ID, Bytes: 1, SHA256: q.SHA256}, MetadataAsset: "m", Kind: queryregistrycore.Structural})
			}
			for _, c := range v.Coordinates() {
				if c.Kind == queryregistrycore.Structural && c.Route == reg.Route && c.Platform == reg.Platform {
					required := OrdinaryRequired{ID: c.ID(reg.Namespace), Coordinate: c, TreeAsset: "t", CaptureAsset: "c"}
					if strings.HasPrefix(c.Query, "facts.") {
						required.FactAsset = "f"
					}
					reg.Required = append(reg.Required, required)
				}
			}
			rb := ordinaryJSON(t, reg)
			plan.Registrations = []OrdinaryPin{queryregistrycore.FilePin("registrations/json-windows.json", rb)}
			read := func(path string) []byte {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			b := ordinaryTestBudget(t)
			if selectedIndex < 0 {
				selectedIndex = slices.IndexFunc(plan.Shards, func(s OrdinaryPlanShard) bool { return s.Route == reg.Route && s.Platform == reg.Platform })
			}
			if t.Name() == "TestOrdinaryReaderSelectedInventoryBudget/largest-shard" {
				for _, shard := range plan.Shards {
					if len(ordinaryJSON(t, shard)) > len(ordinaryJSON(t, plan.Shards[selectedIndex])) {
						t.Fatal("selected shard is not the largest raw shard")
					}
				}
			}
			commitmentData, selection := ordinarySelectionForTest(t, plan, []int{selectedIndex})
			t.Logf("input sizes: commitment=%d proof=%d selected_shard=%d registration=%d route_cases=%d", len(commitmentData), len(selection.Proofs[0]), len(ordinaryJSON(t, plan.Shards[selectedIndex])), len(rb), len(reg.Cases))
			registryData, inventoryData, slotsData := read("../contracts/query-semantic-registry-r1.json"), read("../contracts/qualification-c1.json"), read("../contracts/query-slot-inventory-r1.json")
			for _, data := range [][]byte{pb, ab, rb, commitmentData, registryData, inventoryData, slotsData} {
				if e := b.r.chargeRead(uint64(len(data)), OrdinaryPlanLimits().ReadBytes, "ordinary-synthetic-input"); e != nil {
					t.Fatal(e)
				}
			}
			for _, proof := range selection.Proofs {
				if e := b.r.chargeRead(uint64(len(proof)), OrdinaryPlanLimits().ReadBytes, "ordinary-synthetic-proof"); e != nil {
					t.Fatal(e)
				}
			}
			out, e := b.ordinaryInputs(pb, ab, rb, commitmentData, registryData, inventoryData, slotsData, ordinaryRegistrySHA, selection)
			if e != nil {
				t.Fatal(e)
			}
			if len(out.plan.Shards) != 1 || len(out.registration.Cases) != 1 || len(out.registration.Required) != len(plan.Shards[selectedIndex].ObligationIDs) {
				t.Fatal("selection decoded unrelated cases/IDs")
			}
			if b.logical > OrdinaryPlanLimits().Logical || b.work > OrdinaryPlanLimits().Work {
				t.Fatalf("ordinary input validation escaped inherited cap: logical=%d work=%d", b.logical, b.work)
			}
			t.Logf("selected %s case=%s obligations=%d: error=%v logical=%d work=%d read=%d remaining_logical=%d remaining_work=%d remaining_read=%d (evidence/body excluded; never full capacity PASS)", routeName, plan.Shards[selectedIndex].Case, len(plan.Shards[selectedIndex].ObligationIDs), e, b.logical, b.work, b.r.totalRead, OrdinaryPlanLimits().Logical-b.logical, OrdinaryPlanLimits().Work-b.work, OrdinaryPlanLimits().ReadBytes-b.r.totalRead)
		})
	}
}

func ordinarySelectionForTest(t *testing.T, plan OrdinaryShardPlan, indices []int) ([]byte, ordinaryPlanSelection) {
	t.Helper()
	c, proofs, err := ordinarycommit.Build(ordinaryJSON(t, plan))
	if err != nil {
		t.Fatal(err)
	}
	data := ordinaryJSON(t, c)
	pin := OrdinaryPin{Path: "test/pin.json", Bytes: 1, SHA256: strings.Repeat("a", 64)}
	binding := ordinaryEligibilityBinding{Schema: ordinaryEligibilityBindingSchema, Context: OrdinaryContext, ClosureEncoding: "plain", Profile: pin, PlanningProfile: plan.Profile, Registration: pin, Assets: plan.Assets, Plan: OrdinaryPin{Path: c.Plan.Path, Bytes: c.Plan.Bytes, SHA256: c.Plan.SHA256}, Commitment: queryregistrycore.FilePin("plan/commitment.json", data), ReferenceSelection: pin, RunBinding: pin, Candidate: plan.Candidate, SemanticCohort: plan.SemanticCohort, RunID: "run", ShardIDs: []string{}}
	if len(plan.Registrations) > 0 {
		binding.Registration = plan.Registrations[0]
	}
	out := ordinaryPlanSelection{Binding: binding}
	for _, i := range indices {
		out.Binding.ShardIDs = append(out.Binding.ShardIDs, plan.Shards[i].ID)
		out.Proofs = append(out.Proofs, ordinaryJSON(t, proofs[i]))
	}
	return data, out
}

func TestOrdinarySelectedPlanProofAndCoordinate(t *testing.T) {
	t.Parallel()
	v, inv := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inv)
	data, selection := ordinarySelectionForTest(t, plan, []int{0, 1})
	if _, e := ordinaryTestBudget(t).ordinarySelectedPlan(data, selection); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"foreign-plan", "coordinate", "reorder", "omit", "duplicate", "caller-pin", "full-plan-pin", "header-pin"} {
		t.Run(name, func(t *testing.T) {
			raw := append([]byte(nil), data...)
			s := selection
			s.Proofs = append([][]byte(nil), selection.Proofs...)
			s.Binding.ShardIDs = append([]string(nil), selection.Binding.ShardIDs...)
			switch name {
			case "foreign-plan":
				var changed OrdinaryShardPlan
				if err := json.Unmarshal(ordinaryJSON(t, plan), &changed); err != nil {
					t.Fatal(err)
				}
				changed.Witness.SHA256 = strings.Repeat("f", 64)
				_, other := ordinarySelectionForTest(t, changed, []int{0})
				s.Proofs[0] = other.Proofs[0]
			case "coordinate":
				var changed OrdinaryShardPlan
				if err := json.Unmarshal(ordinaryJSON(t, plan), &changed); err != nil {
					t.Fatal(err)
				}
				changed.Shards[0].ObligationIDs[0] = "qsem1:" + strings.Repeat("a", 64)
				changed.Shards[0].ObligationIDSHA256 = ordinaryIDDigest(changed.Shards[0].ObligationIDs)
				raw, s = ordinarySelectionForTest(t, changed, []int{0, 1})
			case "reorder":
				s.Proofs[0], s.Proofs[1] = s.Proofs[1], s.Proofs[0]
				s.Binding.ShardIDs[0], s.Binding.ShardIDs[1] = s.Binding.ShardIDs[1], s.Binding.ShardIDs[0]
			case "omit":
				s.Proofs = s.Proofs[:1]
			case "duplicate":
				s.Proofs[1] = s.Proofs[0]
			case "caller-pin":
				s.Binding.Commitment.SHA256 = strings.Repeat("f", 64)
			case "full-plan-pin":
				s.Binding.Plan.Bytes++
			case "header-pin":
				s.Binding.PlanningProfile.Bytes++
			}
			if _, e := ordinaryTestBudget(t).ordinarySelectedPlan(raw, s); e == nil {
				t.Fatal("accepted altered selected plan")
			}
		})
	}
}

func TestOrdinaryReaderTypedShapes(t *testing.T) {
	profile := ordinaryProfileFixture()
	var decoded OrdinaryProfile
	if e := ordinaryTestBudget(t).decodeOrdinaryTyped("profile", ordinaryJSON(t, profile), &decoded); e != nil {
		t.Fatal(e)
	}
	for _, data := range []string{
		strings.Replace(string(ordinaryJSON(t, profile)), `"bindings":[]`, `"bindings":null`, 1),
		strings.Replace(string(ordinaryJSON(t, profile)), `"context":`, `"extra":true,"context":`, 1),
		strings.Replace(string(ordinaryJSON(t, profile)), `"context":`, `"context":"duplicate","context":`, 1),
	} {
		if e := ordinaryTestBudget(t).decodeOrdinaryTyped("profile", []byte(data), &decoded); e == nil {
			t.Fatal("accepted malformed ordinary profile")
		}
	}
	asset := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA}, Candidate: "candidate", SemanticCohort: "cohort", Assets: []OrdinaryAsset{{ID: "metadata", Role: "metadata", State: "UNAVAILABLE"}}}
	var assets OrdinaryAssetRegistration
	if e := ordinaryTestBudget(t).decodeOrdinaryTyped("assets", ordinaryJSON(t, asset), &assets); e != nil {
		t.Fatal(e)
	}
	if _, err := validateOrdinaryAssets(assets); err != nil {
		t.Fatal(err)
	}
}
