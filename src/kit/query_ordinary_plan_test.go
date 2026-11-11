package kit

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistrycore"
)

func ordinaryP0ForPlanTest(t *testing.T) (*queryregistrycore.Validated, QualificationInventory) {
	t.Helper()
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	registry := read("../contracts/query-semantic-registry-r1.json")
	qualification := read("../contracts/qualification-c1.json")
	slots := read("../contracts/query-slot-inventory-r1.json")
	basis := map[string][]byte{
		"src/dev/query-reference/check.py":         read("../dev/query-reference/check.py"),
		"src/internal/native/query_policy_test.go": read("../internal/native/query_policy_test.go"),
	}
	inv, err := ParseQualificationInventory(qualification)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := ordinaryRegistry(OrdinaryPlanRequest{Registry: registry, Qualification: qualification, Slots: slots, RegistrySHA256: ordinaryRegistrySHA, Basis: basis})
	if err != nil {
		t.Fatal(err)
	}
	return v, inv
}

func ordinaryPlanForTest(v *queryregistrycore.Validated, inv QualificationInventory) OrdinaryShardPlan {
	plan := OrdinaryShardPlan{
		Schema:        ordinaryShardPlanSchema,
		Registry:      OrdinaryPin{Path: queryregistrycore.Path, Bytes: ordinaryRegistryBytes, SHA256: v.SHA256()},
		Namespace:     v.Namespace(),
		Witness:       OrdinaryPin{Path: "witness/p1.json", Bytes: 1, SHA256: strings.Repeat("a", 64)},
		Profile:       OrdinaryPin{Path: "profile/ordinary.json", Bytes: 1, SHA256: strings.Repeat("b", 64)},
		Assets:        OrdinaryPin{Path: "profile/assets.json", Bytes: 1, SHA256: strings.Repeat("c", 64)},
		Registrations: []OrdinaryPin{}, Candidate: "candidate", SemanticCohort: "cohort",
		Limits: OrdinaryPlanLimits(), CapacityUnknown: append([]string(nil), ordinaryCapacityUnknown...),
		Budget: OrdinaryPlanBudget{
			LogicalLimit: OrdinaryPlanLimits().Logical, FixedDocumentDecodeLogicalPerPass: ordinaryFixedDocumentDecodeLogical,
			ReferenceModel: "test estimates", SQLModel: "historical fixture", OtherRouteClosureUnknown: true,
			ReferenceRoutes: []OrdinaryReferenceRouteBudget{}, SQL: ordinarySQLFixedBudgets(),
		},
		Bindings: []OrdinaryBinding{}, Routes: []OrdinaryPlanRoute{}, Shards: []OrdinaryPlanShard{},
	}
	for _, route := range inv.Routes {
		queries := make([]string, 0, len(route.Workload.Queries))
		for _, query := range route.Workload.Queries {
			queries = append(queries, query.ID)
		}
		plan.Routes = append(plan.Routes, OrdinaryPlanRoute{Route: route.Route, Set: route.Workload.Set, NativeQueryIDs: queries, ReferenceQueryGroups: ordinaryQueryGroups(queries)})
		plan.Budget.ReferenceRoutes = append(plan.Budget.ReferenceRoutes, OrdinaryReferenceRouteBudget{
			Route: route.Route, Set: route.Workload.Set,
			Nominal: ordinaryReferenceEstimate(0, 0, 8), Dense: ordinaryReferenceEstimate(0, 0, 2),
		})
		for _, host := range []OrdinaryHost{{"windows-amd64", "windows/amd64"}, {"linux-amd64", "linux/amd64"}} {
			plan.Bindings = append(plan.Bindings, OrdinaryBinding{Route: route.Route, Platform: host.Platform, Subject: host.Subject,
				ClosureEncoding: "plain", Grammar: route.Workload.Grammar,
				Runtime: OrdinaryPin{Path: "runtime/runtime.json", Bytes: 1, SHA256: strings.Repeat("f", 64)}, Members: []OrdinaryMember{}})
		}
	}
	byKey := map[string]*OrdinaryPlanShard{}
	for _, c := range v.Coordinates() {
		if c.Kind != queryregistrycore.Structural || c.Role != "primary" {
			continue
		}
		key := ordinaryPlanShardKey(c.Route, c.Platform, c.Case)
		shard := byKey[key]
		if shard == nil {
			shard = &OrdinaryPlanShard{
				ID: key, Route: c.Route, Set: c.Set, Platform: c.Platform, Subject: c.Subject, Case: c.Case, BindingID: c.Route + "/" + c.Platform,
				SourceHistory: []OrdinaryPlanHistoryStep{{SHA256: strings.Repeat("d", 64)}}, EditPoints: []EditPoints{}, ReferenceSteps: []OrdinaryPlanReferenceStep{}, ObligationIDs: []string{},
			}
			byKey[key] = shard
		}
		shard.ObligationIDs = append(shard.ObligationIDs, c.ID(v.Namespace()))
		for uint32(len(shard.SourceHistory)-1) < c.Step {
			shard.SourceHistory = append(shard.SourceHistory, OrdinaryPlanHistoryStep{SHA256: strings.Repeat("e", 64)})
			shard.EditPoints = append(shard.EditPoints, EditPoints{})
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		shard := byKey[key]
		shard.ReferenceSteps = ordinaryReferenceSteps(uint32(len(shard.SourceHistory) - 1))
		for _, binding := range plan.Bindings {
			if binding.Route+"/"+binding.Platform == shard.BindingID {
				shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, binding.Members)
				break
			}
		}
		shard.ObligationIDSHA256 = ordinaryIDDigest(shard.ObligationIDs)
		shard.Admission = OrdinaryPlanAdmission{State: "BLOCKED", Code: "CAPACITY_UNPROVEN"}
		plan.Required += uint64(len(shard.ObligationIDs))
		plan.Blocked += uint64(len(shard.ObligationIDs))
		plan.Shards = append(plan.Shards, *shard)
	}
	return plan
}

func TestOrdinaryPlanExactP0UnionAndFixedCaps(t *testing.T) {
	v, inv := ordinaryP0ForPlanTest(t)
	plan := ordinaryPlanForTest(v, inv)
	if err := validateOrdinaryShardPlan(plan, v); err != nil {
		t.Fatal("complete ordinary plan rejected:", err)
	}
	if plan.Required != 106208 || plan.Admitted != 0 || plan.Blocked != 106208 {
		t.Fatalf("ordinary denominator or capacity state changed: required=%d admitted=%d blocked=%d", plan.Required, plan.Admitted, plan.Blocked)
	}
	raw, err := MarshalOrdinaryShardPlan(plan)
	if err != nil {
		t.Fatal("full plan must fit the existing 16 MiB file bound:", err)
	}
	decoded, err := ParseOrdinaryShardPlan(raw)
	if err != nil {
		t.Fatal("strict plan round trip failed:", err)
	}
	if err := validateOrdinaryShardPlan(decoded, v); err != nil {
		t.Fatal("round-tripped plan rejected:", err)
	}

	omitted := plan
	omitted.Shards = append([]OrdinaryPlanShard(nil), plan.Shards...)
	omitted.Shards[0].ObligationIDs = append([]string(nil), plan.Shards[0].ObligationIDs[1:]...)
	omitted.Shards[0].ObligationIDSHA256 = ordinaryIDDigest(omitted.Shards[0].ObligationIDs)
	omitted.Required--
	omitted.Blocked--
	if err := validateOrdinaryShardPlan(omitted, v); err == nil {
		t.Fatal("plan with one omitted required ID accepted")
	}

	raised := plan
	raised.Limits.Logical++
	if err := validateOrdinaryShardPlan(raised, v); err == nil {
		t.Fatal("plan with raised logical cap accepted")
	}
	noFixedDoc := plan
	noFixedDoc.Budget.FixedDocumentDecodeLogicalPerPass = 0
	if err := validateOrdinaryShardPlan(noFixedDoc, v); err == nil {
		t.Fatal("plan omitting the fixed document decode charge accepted")
	}
	noShardFixedDoc := plan
	noShardFixedDoc.Shards = append([]OrdinaryPlanShard(nil), plan.Shards...)
	noShardFixedDoc.Shards[0].Capacity.FixedDocumentDecodeLogical = 0
	if err := validateOrdinaryShardPlan(noShardFixedDoc, v); err == nil {
		t.Fatal("shard omitting the per-pass fixed document decode charge accepted")
	}
	changedSQL := plan
	changedSQL.Budget.SQL = append([]OrdinarySQLBudget(nil), plan.Budget.SQL...)
	changedSQL.Budget.SQL[0].RemainingLogical++
	if err := validateOrdinaryShardPlan(changedSQL, v); err == nil {
		t.Fatal("mutated SQL fixed-cost margin accepted")
	}
	changedRoute := plan
	changedRoute.Routes = append([]OrdinaryPlanRoute(nil), plan.Routes...)
	changedRoute.Routes[0].NativeQueryIDs = append([]string(nil), plan.Routes[0].NativeQueryIDs...)
	changedRoute.Routes[0].NativeQueryIDs[0] = "wrong-query"
	if err := validateOrdinaryShardPlan(changedRoute, v); err == nil {
		t.Fatal("changed route native query catalogue accepted")
	}
	overlapped := plan
	overlapped.Shards = append([]OrdinaryPlanShard(nil), plan.Shards...)
	overlapped.Shards[0].ObligationIDs = append([]string(nil), plan.Shards[0].ObligationIDs...)
	overlapped.Shards[0].ObligationIDs[0] = overlapped.Shards[0].ObligationIDs[1]
	overlapped.Shards[0].ObligationIDSHA256 = ordinaryIDDigest(overlapped.Shards[0].ObligationIDs)
	if err := validateOrdinaryShardPlan(overlapped, v); err == nil {
		t.Fatal("same-count overlapped ID accepted")
	}
	missingHost := plan
	missingHost.Bindings = append([]OrdinaryBinding(nil), plan.Bindings...)
	missingHost.Bindings[0].Route = "unregistered-route"
	if err := validateOrdinaryShardPlan(missingHost, v); err == nil {
		t.Fatal("same-count missing route/host binding accepted")
	}
	empty := plan
	empty.Shards = []OrdinaryPlanShard{}
	empty.Required = 0
	empty.Blocked = 0
	if err := validateOrdinaryShardPlan(empty, v); err == nil {
		t.Fatal("empty ordinary plan accepted")
	}
}
