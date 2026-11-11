package kit

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

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
	v, inv := ordinaryP0ForPlanTest(t)
	for _, routeName := range []string{"json", "postgresql-sql", "tsql"} {
		t.Run(routeName, func(t *testing.T) {
			plan := ordinaryPlanForTest(v, inv)
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
			planData := ordinaryJSON(t, plan)
			registryData, inventoryData, slotsData := read("../contracts/query-semantic-registry-r1.json"), read("../contracts/qualification-c1.json"), read("../contracts/query-slot-inventory-r1.json")
			for _, data := range [][]byte{pb, ab, rb, planData, registryData, inventoryData, slotsData} {
				if e := b.r.chargeRead(uint64(len(data)), OrdinaryPlanLimits().ReadBytes, "ordinary-synthetic-input"); e != nil {
					t.Fatal(e)
				}
			}
			_, e := b.ordinaryInputs(pb, ab, rb, planData, registryData, inventoryData, slotsData, ordinaryRegistrySHA)
			if e != nil && (e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT") {
				t.Fatalf("unexpected selected ordinary input error: %v logical=%d work=%d", e, b.logical, b.work)
			}
			if b.logical > OrdinaryPlanLimits().Logical || b.work > OrdinaryPlanLimits().Work {
				t.Fatalf("ordinary input validation escaped inherited cap: logical=%d work=%d", b.logical, b.work)
			}
			t.Logf("synthetic full plan + selected %s inputs: error=%v logical=%d work=%d read=%d (evidence/body excluded; never full capacity PASS)", routeName, e, b.logical, b.work, b.r.totalRead)
		})
	}
}

func TestOrdinaryReaderPlanMembership(t *testing.T) {
	v, inv := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inv)
	var registry queryregistrycore.Registry
	data, err := os.ReadFile("../contracts/query-semantic-registry-r1.json")
	if err != nil {
		t.Fatal(err)
	}
	if e := inventoryTyped(data, &registry, ordinaryTestBudget(t)); e != nil {
		t.Fatal(e)
	}
	if e := ordinaryTestBudget(t).ordinaryPlanUnion(plan, registry); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"case", "query", "source-history", "platform", "context-namespace", "id-only"} {
		t.Run(name, func(t *testing.T) {
			var changed OrdinaryShardPlan
			if e := ordinaryTestBudget(t).decodeOrdinaryTyped("plan", ordinaryJSON(t, plan), &changed); e != nil {
				t.Fatal(e)
			}
			s := &changed.Shards[0]
			switch name {
			case "case":
				s.Case += "-replacement"
			case "query":
				changed.Routes[0].NativeQueryIDs[0] += "-replacement"
			case "source-history":
				s.SourceHistory = append(s.SourceHistory, s.SourceHistory[0])
			case "platform":
				s.Subject += "-replacement"
			case "context-namespace":
				changed.Namespace = strings.Repeat("a", 64)
			case "id-only":
				s.ObligationIDs[0] = "qsem1:" + strings.Repeat("b", 64)
			}
			if e := ordinaryTestBudget(t).ordinaryPlanUnion(changed, registry); e == nil {
				t.Fatal("accepted altered plan membership")
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
