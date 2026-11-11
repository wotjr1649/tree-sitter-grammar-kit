package kit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistrycore"
)

const ordinaryShardPlanSchema = "tsgk-query-ordinary-shard-plan/r1"

var ordinaryCapacityUnknown = []string{
	"independent-metadata-body",
	"independent-full-cst-body-and-output-cap",
	"independent-capture-body-and-output-cap",
	"independent-fact-body-and-output-cap",
	"current-closure-and-build-control-identity",
	"native-batch-capture-output-cap",
	"api-fact-and-shape-work-upper-bound",
	"reader-work-logical-entry-and-wall-upper-bound",
	"independent-expected-output-capacity-bound",
}

type OrdinaryPlanRequest struct {
	Registry       []byte
	Qualification  []byte
	Slots          []byte
	Witness        []byte
	Profile        []byte
	Assets         []byte
	RegistrySHA256 string
	Basis          map[string][]byte
	Registrations  [][]byte
	Bodies         map[string][]byte
}

type OrdinaryPlanHistoryStep struct {
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

type OrdinaryPlanRoute struct {
	Route                string     `json:"route"`
	Set                  string     `json:"set"`
	NativeQueryIDs       []string   `json:"native_query_ids"`
	ReferenceQueryGroups [][]string `json:"reference_query_groups"`
}

type OrdinaryPlanReferenceStep struct {
	Step      uint32   `json:"step"`
	TreeRoles []string `json:"tree_roles"`
}

type OrdinaryPlanAdmission struct {
	State string `json:"state"`
	Code  string `json:"code"`
}

type OrdinaryShardCapacity struct {
	ClosurePasses              uint32          `json:"closure_passes"`
	FixedDocumentDecodeLogical uint64          `json:"fixed_document_decode_logical"`
	SourceHistoryBytes         uint64          `json:"source_history_bytes"`
	DeclaredMemberBytes        uint64          `json:"declared_member_bytes"`
	UpperBound                 *OrdinaryLimits `json:"upper_bound"`
}

type OrdinaryReferenceRouteBudget struct {
	Route       string                    `json:"route"`
	Set         string                    `json:"set"`
	NominalCase string                    `json:"nominal_max_case"`
	NominalStep uint32                    `json:"nominal_max_step"`
	Nominal     OrdinaryReferenceEstimate `json:"nominal"`
	DenseCase   string                    `json:"dense_max_case"`
	DenseStep   uint32                    `json:"dense_max_step"`
	Dense       OrdinaryReferenceEstimate `json:"dense"`
}

type OrdinarySQLBudget struct {
	Route                      string `json:"route"`
	HistoricalCoreLogical      uint64 `json:"historical_core_logical"`
	SetLayerLogical            uint64 `json:"set_layer_logical"`
	FixedDocumentDecodeLogical uint64 `json:"fixed_document_decode_logical"`
	KnownSubtotalLogical       uint64 `json:"known_subtotal_logical"`
	RemainingLogical           int64  `json:"remaining_logical"`
}

type OrdinaryPlanBudget struct {
	LogicalLimit                      uint64                         `json:"logical_limit"`
	FixedDocumentDecodeLogicalPerPass uint64                         `json:"fixed_document_decode_logical_per_pass"`
	ReferenceModel                    string                         `json:"reference_model_note"`
	SQLModel                          string                         `json:"sql_model_note"`
	OtherRouteClosureUnknown          bool                           `json:"other_route_closure_unknown"`
	StructuralSavingsApplied          bool                           `json:"structural_savings_applied"`
	ReferenceRoutes                   []OrdinaryReferenceRouteBudget `json:"reference_routes"`
	SQL                               []OrdinarySQLBudget            `json:"sql_fixed_budgets"`
}

type OrdinaryPlanShard struct {
	ID                 string                      `json:"id"`
	Route              string                      `json:"route"`
	Set                string                      `json:"set"`
	Platform           string                      `json:"platform_id"`
	Subject            string                      `json:"subject_platform"`
	Case               string                      `json:"case_id"`
	BindingID          string                      `json:"binding_id"`
	SourceHistory      []OrdinaryPlanHistoryStep   `json:"source_history"`
	EditPoints         []EditPoints                `json:"edit_points"`
	ReferenceSteps     []OrdinaryPlanReferenceStep `json:"reference_steps"`
	ObligationIDs      []string                    `json:"obligation_ids"`
	ObligationIDSHA256 string                      `json:"obligation_ids_sha256"`
	Capacity           OrdinaryShardCapacity       `json:"capacity"`
	Admission          OrdinaryPlanAdmission       `json:"admission"`
}

type OrdinaryShardPlan struct {
	Schema            string              `json:"schema"`
	Registry          OrdinaryPin         `json:"registry_pin"`
	Namespace         string              `json:"membership_namespace"`
	Witness           OrdinaryPin         `json:"source_witness_pin"`
	Profile           OrdinaryPin         `json:"profile_pin"`
	Assets            OrdinaryPin         `json:"asset_registration_pin"`
	Registrations     []OrdinaryPin       `json:"registration_pins"`
	Candidate         string              `json:"candidate"`
	SemanticCohort    string              `json:"semantic_cohort"`
	Limits            OrdinaryLimits      `json:"limits"`
	CapacityUnknown   []string            `json:"capacity_unknown"`
	Bindings          []OrdinaryBinding   `json:"profile_bindings"`
	Routes            []OrdinaryPlanRoute `json:"routes"`
	Budget            OrdinaryPlanBudget  `json:"budget"`
	Shards            []OrdinaryPlanShard `json:"shards"`
	Required          uint64              `json:"required"`
	Admitted          uint64              `json:"admitted"`
	Blocked           uint64              `json:"blocked"`
	FullScopeComplete bool                `json:"full_scope_complete"`
}

// BuildOrdinaryShardPlan uses only pinned source documents and declared bounds. P1 source
// witness bytes reconstruct edits but never count as independent expected output.
func BuildOrdinaryShardPlan(req OrdinaryPlanRequest) (OrdinaryShardPlan, error) {
	var out OrdinaryShardPlan
	v, inv, err := ordinaryRegistry(req)
	if err != nil {
		return out, err
	}
	profile, err := ParseOrdinaryProfile(req.Profile)
	if err != nil {
		return out, err
	}
	assets, err := ParseOrdinaryAssets(req.Assets)
	if err != nil {
		return out, err
	}
	witness, err := parseOrdinarySourceExport(req.Witness, inv, v.SHA256())
	if err != nil {
		return out, err
	}
	if len(profile.Bindings) != len(inv.Routes)*len(profile.Hosts) {
		return out, ordinaryBad("ORDINARY_PLAN_BINDING_GAP")
	}
	bindingByKey := make(map[string]OrdinaryBinding, len(profile.Bindings))
	for _, binding := range profile.Bindings {
		key := binding.Route + "/" + binding.Platform
		if ordinaryRouteIndex(inv, binding.Route) < 0 {
			return out, ordinaryBad("ORDINARY_PLAN_BINDING_GAP")
		}
		if _, exists := bindingByKey[key]; exists {
			return out, ordinaryBad("ORDINARY_PLAN_BINDING_DUPLICATE")
		}
		bindingByKey[key] = binding
	}
	for _, route := range inv.Routes {
		for _, host := range profile.Hosts {
			binding, ok := bindingByKey[route.Route+"/"+host.Platform]
			if !ok || binding.Subject != host.Subject || !reflect.DeepEqual(binding.Grammar, route.Workload.Grammar) {
				return out, ordinaryBad("ORDINARY_PLAN_BINDING_GAP")
			}
		}
	}
	if assets.Registry != (OrdinaryPin{Path: queryregistrycore.Path, Bytes: uint64(len(req.Registry)), SHA256: v.SHA256()}) || assets.Candidate != profile.Candidate || assets.SemanticCohort != profile.SemanticCohort {
		return out, ordinaryBad("ORDINARY_PLAN_ASSET_BINDING")
	}
	for _, asset := range assets.Assets {
		if asset.State != "PINNED" {
			continue
		}
		for _, pin := range []*OrdinaryPin{asset.Pin, asset.AuthoringBasis, asset.IndependentReview} {
			if err := ordinaryCheckBody(*pin, req.Bodies); err != nil {
				return out, err
			}
		}
		if err := ordinaryValidateAssetBody(asset, req.Bodies[asset.Pin.Path]); err != nil {
			return out, err
		}
	}
	for _, binding := range profile.Bindings {
		for _, member := range binding.Members {
			if err := ordinaryCheckBody(member.Pin, req.Bodies); err != nil {
				return out, err
			}
			if err := ordinaryValidateMemberBody(member, req.Bodies[member.Pin.Path]); err != nil {
				return out, err
			}
		}
	}

	registrations := make(map[string]OrdinaryRegistration, len(req.Registrations))
	regPins := make([]OrdinaryPin, 0, len(req.Registrations))
	for _, raw := range req.Registrations {
		r, e := ParseOrdinaryRegistration(raw)
		if e != nil {
			return out, e
		}
		if e = validateOrdinaryRegistration(r, profile, assets, witness, inv, req.Profile, req.Assets, v); e != nil {
			return out, e
		}
		for _, query := range r.Queries {
			if e = ordinaryCheckBody(query.Source, req.Bodies); e != nil {
				return out, e
			}
		}
		key := r.Route + "/" + r.Platform
		if _, exists := registrations[key]; exists {
			return out, ordinaryBad("ORDINARY_PLAN_REGISTRATION_DUPLICATE")
		}
		registrations[key] = r
		regPins = append(regPins, OrdinaryPin{Path: "registrations/" + key + ".json", Bytes: uint64(len(raw)), SHA256: queryregistrycore.Digest(raw)})
	}
	sort.Slice(regPins, func(i, j int) bool { return regPins[i].Path < regPins[j].Path })

	profilePin := OrdinaryPin{Path: "profile/tsgk-oracle-r4.json", Bytes: uint64(len(req.Profile)), SHA256: queryregistrycore.Digest(req.Profile)}
	assetPin := OrdinaryPin{Path: "profile/ordinary-assets-r1.json", Bytes: uint64(len(req.Assets)), SHA256: queryregistrycore.Digest(req.Assets)}
	witnessPin := OrdinaryPin{Path: "source-witness/query-source-export-r1.json", Bytes: uint64(len(req.Witness)), SHA256: queryregistrycore.Digest(req.Witness)}
	routes := make([]OrdinaryPlanRoute, 0, len(inv.Routes))
	for _, route := range inv.Routes {
		queries := make([]string, 0, len(route.Workload.Queries))
		for _, query := range route.Workload.Queries {
			queries = append(queries, query.ID)
		}
		routes = append(routes, OrdinaryPlanRoute{Route: route.Route, Set: route.Workload.Set, NativeQueryIDs: queries, ReferenceQueryGroups: ordinaryQueryGroups(queries)})
	}
	out = OrdinaryShardPlan{
		Schema:    ordinaryShardPlanSchema,
		Registry:  OrdinaryPin{Path: queryregistrycore.Path, Bytes: uint64(len(req.Registry)), SHA256: v.SHA256()},
		Namespace: v.Namespace(), Witness: witnessPin, Profile: profilePin, Assets: assetPin,
		Registrations: regPins, Candidate: profile.Candidate, SemanticCohort: profile.SemanticCohort,
		Limits: OrdinaryPlanLimits(), CapacityUnknown: append([]string(nil), ordinaryCapacityUnknown...),
		Bindings: append([]OrdinaryBinding{}, profile.Bindings...), Routes: routes,
		Budget: ordinaryPlanBudget(inv, witness), Shards: []OrdinaryPlanShard{}, FullScopeComplete: false,
	}
	versions := map[string]ordinarySourceVersion{}
	for _, route := range witness.Routes {
		for _, version := range route.Versions {
			versions[version.SHA256] = version
		}
	}
	shards := map[string]*OrdinaryPlanShard{}
	coordinates := v.Coordinates()
	for _, c := range coordinates {
		if c.Kind != queryregistrycore.Structural || c.Role != "primary" {
			continue
		}
		key := ordinaryPlanShardKey(c.Route, c.Platform, c.Case)
		shard := shards[key]
		if shard == nil {
			shard = &OrdinaryPlanShard{ID: key, Route: c.Route, Set: c.Set, Platform: c.Platform, Subject: c.Subject, Case: c.Case, BindingID: c.Route + "/" + c.Platform, ObligationIDs: []string{}}
			route, caseIndex, ok := ordinarySourceCaseAt(witness, c.Route, c.Case)
			if !ok {
				return OrdinaryShardPlan{}, ordinaryBad("ORDINARY_PLAN_SOURCE_CASE")
			}
			steps := routeCaseSteps(route, caseIndex)
			shard.SourceHistory = make([]OrdinaryPlanHistoryStep, len(steps))
			for i, sha := range steps {
				version, exists := versions[sha]
				if !exists {
					return OrdinaryShardPlan{}, ordinaryBad("ORDINARY_PLAN_SOURCE_VERSION")
				}
				shard.SourceHistory[i] = OrdinaryPlanHistoryStep{SHA256: sha, Bytes: version.Bytes}
			}
			shard.EditPoints = append([]EditPoints{}, route.Cases[caseIndex].Points...)
			shard.ReferenceSteps = ordinaryReferenceSteps(uint32(len(shard.SourceHistory) - 1))
			shards[key] = shard
		}
		shard.ObligationIDs = append(shard.ObligationIDs, c.ID(v.Namespace()))
	}
	keys := make([]string, 0, len(shards))
	for key := range shards {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		shard := shards[key]
		shard.ObligationIDSHA256 = ordinaryIDDigest(shard.ObligationIDs)
		shard.Capacity = ordinaryShardCapacity(shard.SourceHistory, bindingByKey[shard.BindingID].Members)
		shard.Admission = OrdinaryPlanAdmission{State: "BLOCKED", Code: "CAPACITY_UNPROVEN"}
		out.Required += uint64(len(shard.ObligationIDs))
		out.Blocked += uint64(len(shard.ObligationIDs))
		out.Shards = append(out.Shards, *shard)
	}
	if out.Required != 106208 {
		return OrdinaryShardPlan{}, ordinaryBad("ORDINARY_PLAN_DENOMINATOR")
	}
	if err := validateOrdinaryShardPlan(out, v); err != nil {
		return OrdinaryShardPlan{}, err
	}
	return out, nil
}

func ordinaryCheckBody(pin OrdinaryPin, bodies map[string][]byte) error {
	body, ok := bodies[pin.Path]
	if !ok {
		return ordinaryBad("ORDINARY_PLAN_DECLARED_BODY_MISSING")
	}
	if uint64(len(body)) != pin.Bytes || queryregistrycore.Digest(body) != pin.SHA256 {
		return ordinaryBad("ORDINARY_PLAN_PINNED_BODY_MISMATCH")
	}
	return nil
}

func ordinaryPlanShardKey(route, platform, caseID string) string {
	return route + "/" + platform + "/" + caseID
}

func ordinarySourceCaseAt(w ordinarySourceExport, routeName, caseID string) (ordinarySourceRoute, int, bool) {
	for _, route := range w.Routes {
		if route.Route != routeName {
			continue
		}
		for i, c := range route.Cases {
			if c.ID == caseID {
				return route, i, true
			}
		}
	}
	return ordinarySourceRoute{}, 0, false
}

func routeCaseSteps(route ordinarySourceRoute, index int) []string {
	if index < 0 || index >= len(route.Cases) {
		return nil
	}
	return route.Cases[index].Steps
}

func ordinaryRouteIndex(inv QualificationInventory, route string) int {
	for i := range inv.Routes {
		if inv.Routes[i].Route == route {
			return i
		}
	}
	return -1
}

func ordinaryQueryGroups(queries []string) [][]string {
	out := make([][]string, 0, (len(queries)+14)/15)
	for start := 0; start < len(queries); start += 15 {
		end := min(start+15, len(queries))
		out = append(out, append([]string{}, queries[start:end]...))
	}
	return out
}

func ordinaryReferenceSteps(lastStep uint32) []OrdinaryPlanReferenceStep {
	out := make([]OrdinaryPlanReferenceStep, 0, lastStep+1)
	for step := uint32(0); step <= lastStep; step++ {
		roles := []string{"incremental"}
		if step > 0 {
			roles = append(roles, "fresh")
		}
		out = append(out, OrdinaryPlanReferenceStep{Step: step, TreeRoles: roles})
	}
	return out
}

func ordinaryIDDigest(ids []string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("tsgk-ordinary-shard-members/r1\x00"))
	var size [8]byte
	for _, id := range ids {
		binary.BigEndian.PutUint64(size[:], uint64(len(id)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(id))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ordinaryShardCapacity(history []OrdinaryPlanHistoryStep, members []OrdinaryMember) OrdinaryShardCapacity {
	capacity := OrdinaryShardCapacity{ClosurePasses: 1, FixedDocumentDecodeLogical: ordinaryFixedDocumentDecodeLogical}
	for _, step := range history {
		capacity.SourceHistoryBytes += step.Bytes
	}
	for _, member := range members {
		capacity.DeclaredMemberBytes += member.Pin.Bytes
	}
	return capacity
}

func ordinarySQLFixedBudgets() []OrdinarySQLBudget {
	out := make([]OrdinarySQLBudget, 0, 2)
	for _, value := range []struct {
		route string
		core  uint64
		set   uint64
	}{
		{"postgresql-sql", 1017472466, 141748},
		{"tsql", 1061314324, 202898},
	} {
		subtotal := value.core + value.set + ordinaryFixedDocumentDecodeLogical
		out = append(out, OrdinarySQLBudget{
			Route: value.route, HistoricalCoreLogical: value.core, SetLayerLogical: value.set,
			FixedDocumentDecodeLogical: ordinaryFixedDocumentDecodeLogical,
			KnownSubtotalLogical:       subtotal, RemainingLogical: int64(OrdinaryPlanLimits().Logical) - int64(subtotal),
		})
	}
	return out
}

func ordinaryPlanBudget(inv QualificationInventory, witness ordinarySourceExport) OrdinaryPlanBudget {
	budget := OrdinaryPlanBudget{
		LogicalLimit:                      OrdinaryPlanLimits().Logical,
		FixedDocumentDecodeLogicalPerPass: ordinaryFixedDocumentDecodeLogical,
		ReferenceModel:                    "Per-route maximum single source step; versions_text_bytes equals that step source. Density rows are estimates, never admission upper bounds.",
		SQLModel:                          "Historical after-reuse #366 core anchors plus known set layer and fixed inventory/registry decode; not a current measured closure. No K0-K4 or B structural savings applied.",
		OtherRouteClosureUnknown:          true,
		StructuralSavingsApplied:          false,
		ReferenceRoutes:                   make([]OrdinaryReferenceRouteBudget, 0, len(inv.Routes)),
		SQL:                               ordinarySQLFixedBudgets(),
	}
	for ri, route := range inv.Routes {
		var row OrdinaryReferenceRouteBudget
		row.Route, row.Set = route.Route, route.Workload.Set
		if ri >= len(witness.Routes) || witness.Routes[ri].Route != route.Route {
			continue
		}
		sourceSizes := make(map[string]uint64, len(witness.Routes[ri].Versions))
		for _, version := range witness.Routes[ri].Versions {
			sourceSizes[version.SHA256] = version.Bytes
		}
		for _, c := range witness.Routes[ri].Cases {
			for step, sha := range c.Steps {
				size, ok := sourceSizes[sha]
				if !ok {
					continue
				}
				nominal := ordinaryReferenceEstimate(size, size, 8)
				if row.Nominal.TotalLogical == 0 || nominal.TotalLogical > row.Nominal.TotalLogical {
					row.NominalCase, row.NominalStep, row.Nominal = c.ID, uint32(step), nominal
				}
				dense := ordinaryReferenceEstimate(size, size, 2)
				if row.Dense.TotalLogical == 0 || dense.TotalLogical > row.Dense.TotalLogical {
					row.DenseCase, row.DenseStep, row.Dense = c.ID, uint32(step), dense
				}
			}
		}
		budget.ReferenceRoutes = append(budget.ReferenceRoutes, row)
	}
	return budget
}

// ValidateOrdinaryShardPlan validates shard metadata, fixed caps and exact P0 ordinary ID union.
func validateOrdinaryShardPlan(plan OrdinaryShardPlan, v *queryregistrycore.Validated) error {
	if v == nil || plan.Schema != ordinaryShardPlanSchema || plan.Limits != OrdinaryPlanLimits() || plan.FullScopeComplete || plan.Namespace != v.Namespace() ||
		plan.Registry != (OrdinaryPin{Path: queryregistrycore.Path, Bytes: ordinaryRegistryBytes, SHA256: v.SHA256()}) ||
		plan.Required != uint64(len(planIDs(plan.Shards))) || plan.Admitted != 0 || plan.Blocked != plan.Required || plan.Shards == nil ||
		!reflect.DeepEqual(plan.CapacityUnknown, ordinaryCapacityUnknown) || plan.Budget.LogicalLimit != OrdinaryPlanLimits().Logical ||
		plan.Budget.FixedDocumentDecodeLogicalPerPass != ordinaryFixedDocumentDecodeLogical || plan.Budget.SQLModel == "" || !plan.Budget.OtherRouteClosureUnknown || plan.Budget.StructuralSavingsApplied ||
		!reflect.DeepEqual(plan.Budget.SQL, ordinarySQLFixedBudgets()) {
		return ordinaryBad("ORDINARY_PLAN_HEADER")
	}
	want := make(map[string]OrdinaryCoordinate)
	idsByShard := map[string][]string{}
	queriesByRoute := map[string][]string{}
	setsByRoute := map[string]string{}
	stepsByShard := map[string]uint32{}
	shardKeySet := map[string]bool{}
	routeOrder := []string{}
	for _, c := range v.Coordinates() {
		if c.Kind == queryregistrycore.Structural && c.Role == "primary" {
			id := c.ID(v.Namespace())
			want[id] = c
			key := ordinaryPlanShardKey(c.Route, c.Platform, c.Case)
			idsByShard[key] = append(idsByShard[key], id)
			if c.Step > stepsByShard[key] {
				stepsByShard[key] = c.Step
			}
			if !shardKeySet[key] {
				shardKeySet[key] = true
			}
			if _, exists := setsByRoute[c.Route]; !exists {
				routeOrder = append(routeOrder, c.Route)
				setsByRoute[c.Route] = c.Set
			}
			found := false
			for _, q := range queriesByRoute[c.Route] {
				if q == c.Query {
					found = true
					break
				}
			}
			if !found {
				queriesByRoute[c.Route] = append(queriesByRoute[c.Route], c.Query)
			}
		}
	}
	if len(want) != 106208 || plan.Required != uint64(len(want)) || len(plan.Routes) != len(routeOrder) || len(plan.Shards) != len(shardKeySet) {
		return ordinaryBad("ORDINARY_PLAN_DENOMINATOR")
	}
	routeTable := make(map[string]OrdinaryPlanRoute, len(plan.Routes))
	for i, route := range plan.Routes {
		if route.Route != routeOrder[i] || route.Set != setsByRoute[route.Route] || !reflect.DeepEqual(route.NativeQueryIDs, queriesByRoute[route.Route]) ||
			!reflect.DeepEqual(route.ReferenceQueryGroups, ordinaryQueryGroups(queriesByRoute[route.Route])) {
			return ordinaryBad("ORDINARY_PLAN_ROUTE")
		}
		routeTable[route.Route] = route
	}
	bindings := make(map[string]OrdinaryBinding, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		key := binding.Route + "/" + binding.Platform
		if _, exists := bindings[key]; exists || ordinaryRouteIndexForPlan(routeOrder, binding.Route) < 0 || !ordinaryHost(binding.Platform, binding.Subject) {
			return ordinaryBad("ORDINARY_PLAN_BINDING")
		}
		bindings[key] = binding
	}
	if len(bindings) != len(routeOrder)*2 {
		return ordinaryBad("ORDINARY_PLAN_BINDING")
	}
	if len(plan.Budget.ReferenceRoutes) != len(routeOrder) || plan.Budget.ReferenceModel == "" {
		return ordinaryBad("ORDINARY_PLAN_BUDGET")
	}
	for i, route := range routeOrder {
		budget := plan.Budget.ReferenceRoutes[i]
		if budget.Route != route || budget.Set != setsByRoute[route] || budget.Nominal.Density != 8 || budget.Dense.Density != 2 {
			return ordinaryBad("ORDINARY_PLAN_BUDGET")
		}
		for _, platform := range []string{"windows-amd64", "linux-amd64"} {
			if _, ok := bindings[route+"/"+platform]; !ok {
				return ordinaryBad("ORDINARY_PLAN_BINDING")
			}
		}
	}
	seen := make(map[string]bool, len(want))
	shardKeys := make(map[string]bool, len(plan.Shards))
	for _, shard := range plan.Shards {
		key := ordinaryPlanShardKey(shard.Route, shard.Platform, shard.Case)
		binding, hasBinding := bindings[shard.BindingID]
		if shard.ID != key || shardKeys[key] || shard.Set != setsByRoute[shard.Route] || !ordinaryHost(shard.Platform, shard.Subject) || !hasBinding || shard.BindingID != shard.Route+"/"+shard.Platform || binding.Subject != shard.Subject ||
			shard.ObligationIDs == nil || shard.Admission.State != "BLOCKED" || shard.Admission.Code != "CAPACITY_UNPROVEN" ||
			len(shard.SourceHistory) == 0 || shard.EditPoints == nil || shard.ReferenceSteps == nil || routeTable[shard.Route].Route == "" {
			return ordinaryBad("ORDINARY_PLAN_SHARD")
		}
		shardKeys[key] = true
		for _, step := range shard.SourceHistory {
			if !hex64.MatchString(step.SHA256) || step.Bytes > 65536 {
				return ordinaryBad("ORDINARY_PLAN_HISTORY")
			}
		}
		if len(shard.SourceHistory) != int(stepsByShard[key])+1 || len(shard.EditPoints) != len(shard.SourceHistory)-1 ||
			!reflect.DeepEqual(shard.ReferenceSteps, ordinaryReferenceSteps(stepsByShard[key])) {
			return ordinaryBad("ORDINARY_PLAN_CASE_HISTORY")
		}
		if shard.Capacity != ordinaryShardCapacity(shard.SourceHistory, binding.Members) {
			return ordinaryBad("ORDINARY_PLAN_CAPACITY")
		}
		for _, id := range shard.ObligationIDs {
			c, ok := want[id]
			if !ok || seen[id] || c.Route != shard.Route || c.Set != shard.Set || c.Platform != shard.Platform || c.Subject != shard.Subject || c.Case != shard.Case || c.ID(v.Namespace()) != id {
				return ordinaryBad("ORDINARY_PLAN_COORDINATE")
			}
			seen[id] = true
		}
		if !reflect.DeepEqual(shard.ObligationIDs, idsByShard[key]) {
			return ordinaryBad("ORDINARY_PLAN_SHARD_ID_ORDER")
		}
		if ordinaryIDDigest(shard.ObligationIDs) != shard.ObligationIDSHA256 {
			return ordinaryBad("ORDINARY_PLAN_SET_DIGEST")
		}
	}
	if len(seen) != len(want) {
		return ordinaryBad("ORDINARY_PLAN_GAP")
	}
	return nil
}

func ordinaryRouteIndexForPlan(routes []string, route string) int {
	for i, candidate := range routes {
		if candidate == route {
			return i
		}
	}
	return -1
}

func planIDs(shards []OrdinaryPlanShard) []string {
	var ids []string
	for _, shard := range shards {
		ids = append(ids, shard.ObligationIDs...)
	}
	return ids
}

// ParseOrdinaryShardPlan is a strict, bounded decoder for stored source-only plans.
func ParseOrdinaryShardPlan(data []byte) (OrdinaryShardPlan, error) {
	var plan OrdinaryShardPlan
	if err := ordinaryDecode(data, &plan); err != nil {
		return plan, err
	}
	if plan.Schema != ordinaryShardPlanSchema || !ordinaryPin(plan.Registry) || !ordinaryPin(plan.Witness) || !ordinaryPin(plan.Profile) || !ordinaryPin(plan.Assets) ||
		plan.Namespace == "" || !hex64.MatchString(plan.Namespace) || plan.Candidate == "" || plan.SemanticCohort == "" || plan.Registrations == nil || plan.Shards == nil || plan.Bindings == nil || plan.Routes == nil ||
		!reflect.DeepEqual(plan.CapacityUnknown, ordinaryCapacityUnknown) || plan.Budget.LogicalLimit != OrdinaryPlanLimits().Logical ||
		plan.Budget.FixedDocumentDecodeLogicalPerPass != ordinaryFixedDocumentDecodeLogical || plan.Budget.ReferenceModel == "" || plan.Budget.SQLModel == "" || !plan.Budget.OtherRouteClosureUnknown || plan.Budget.StructuralSavingsApplied ||
		!reflect.DeepEqual(plan.Budget.SQL, ordinarySQLFixedBudgets()) || len(plan.Budget.ReferenceRoutes) != len(plan.Routes) {
		return plan, ordinaryBad("ORDINARY_PLAN_SCHEMA")
	}
	if plan.Registry != (OrdinaryPin{Path: queryregistrycore.Path, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA}) ||
		plan.Limits != OrdinaryPlanLimits() || plan.Admitted != 0 || plan.Blocked != plan.Required || plan.FullScopeComplete || plan.Required != 106208 || plan.Required != uint64(len(planIDs(plan.Shards))) ||
		len(plan.Bindings) != len(plan.Routes)*2 {
		return plan, ordinaryBad("ORDINARY_PLAN_HEADER")
	}
	for i, route := range plan.Routes {
		if route.Route == "" || route.Set == "" || route.NativeQueryIDs == nil || !reflect.DeepEqual(route.ReferenceQueryGroups, ordinaryQueryGroups(route.NativeQueryIDs)) ||
			plan.Budget.ReferenceRoutes[i].Route != route.Route || plan.Budget.ReferenceRoutes[i].Set != route.Set ||
			plan.Budget.ReferenceRoutes[i].Nominal.Density != 8 || plan.Budget.ReferenceRoutes[i].Dense.Density != 2 {
			return plan, ordinaryBad("ORDINARY_PLAN_ROUTE")
		}
	}
	bindings := make(map[string]OrdinaryBinding, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		key := binding.Route + "/" + binding.Platform
		if _, exists := bindings[key]; exists || !ordinaryHost(binding.Platform, binding.Subject) || !ordinaryClosure(binding.ClosureEncoding) || binding.Members == nil || ordinaryMembers(binding.Members, binding.ClosureEncoding) != nil {
			return plan, ordinaryBad("ORDINARY_PLAN_BINDING")
		}
		bindings[key] = binding
	}
	for _, route := range plan.Routes {
		for _, host := range []string{"windows-amd64", "linux-amd64"} {
			if _, ok := bindings[route.Route+"/"+host]; !ok {
				return plan, ordinaryBad("ORDINARY_PLAN_BINDING")
			}
		}
	}
	seenIDs := make(map[string]bool, plan.Required)
	seenShards := make(map[string]bool, len(plan.Shards))
	for _, shard := range plan.Shards {
		binding, ok := bindings[shard.BindingID]
		if shard.ID != ordinaryPlanShardKey(shard.Route, shard.Platform, shard.Case) || seenShards[shard.ID] || !ok ||
			shard.BindingID != shard.Route+"/"+shard.Platform || shard.Admission.State != "BLOCKED" || shard.Admission.Code != "CAPACITY_UNPROVEN" ||
			shard.ObligationIDs == nil || shard.SourceHistory == nil || shard.EditPoints == nil || shard.ReferenceSteps == nil ||
			shard.Capacity != ordinaryShardCapacity(shard.SourceHistory, binding.Members) || ordinaryIDDigest(shard.ObligationIDs) != shard.ObligationIDSHA256 {
			return plan, ordinaryBad("ORDINARY_PLAN_SHARD")
		}
		if len(shard.SourceHistory) != len(shard.EditPoints)+1 || !reflect.DeepEqual(shard.ReferenceSteps, ordinaryReferenceSteps(uint32(len(shard.SourceHistory)-1))) {
			return plan, ordinaryBad("ORDINARY_PLAN_CASE_HISTORY")
		}
		seenShards[shard.ID] = true
		for _, id := range shard.ObligationIDs {
			if len(id) != len("qsem1:")+64 || !strings.HasPrefix(id, "qsem1:") || seenIDs[id] {
				return plan, ordinaryBad("ORDINARY_PLAN_ID")
			}
			seenIDs[id] = true
		}
	}
	return plan, nil
}

func MarshalOrdinaryShardPlan(plan OrdinaryShardPlan) ([]byte, error) {
	if plan.Shards == nil {
		return nil, ordinaryBad("ORDINARY_PLAN_SHARDS")
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, ordinaryBad("ORDINARY_PLAN_JSON")
	}
	if len(data)+1 > 16<<20 {
		return nil, ordinaryBad("ORDINARY_PLAN_SIZE")
	}
	return append(data, '\n'), nil
}

// ValidateOrdinaryShardPlan rebuilds the source-only plan from caller-pinned inputs and
// compares every history, query group, member, ID, pin and cap before accepting stored bytes.
func ValidateOrdinaryShardPlan(plan OrdinaryShardPlan, req OrdinaryPlanRequest) error {
	expected, err := BuildOrdinaryShardPlan(req)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, expected) {
		return ordinaryBad("ORDINARY_PLAN_MISMATCH")
	}
	return nil
}
