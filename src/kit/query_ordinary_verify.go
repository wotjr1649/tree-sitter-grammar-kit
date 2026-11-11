package kit

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Keep ordinary null arms local: legacy witness and native wire shape rules do not change.
func ordinaryTypedNulls(s *typedJSONShape) {
	for name, f := range s.fields {
		switch s.typ {
		case reflect.TypeFor[OrdinaryAsset]():
			f.nullable = name == "pin" || name == "authoring_basis_pin" || name == "independent_review_pin"
		case reflect.TypeFor[OrdinaryShardCapacity]():
			f.nullable = name == "upper_bound"
		case reflect.TypeFor[OrdinaryCase]():
			f.nullable = name == "edits" || name == "edit_points"
		case reflect.TypeFor[OracleQuerySemantics]():
			f.nullable = name == "capture"
		}
		s.fields[name] = f
		ordinaryTypedNulls(f.shape)
	}
	if s.elem != nil {
		ordinaryTypedNulls(s.elem)
	}
}

func (b *eligibilityBudget) decodeOrdinaryTyped(doc string, data []byte, target any, options ...jsonv2.Options) *Error {
	if e := b.charge(1, uint64(len(data)), doc); e != nil {
		return e
	}
	if e := b.reserveJSONContainers(doc, data, 3, 80); e != nil {
		return e
	}
	shape, e := b.typedJSONShapeFor(doc, reflect.TypeOf(target).Elem(), false, true)
	if e != nil {
		return e
	}
	ordinaryTypedNulls(shape)
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	var values uint64
	if e := b.readTypedJSON(doc, dec, shape, &values); e != nil {
		return e
	}
	if e := b.charge(values, 0, doc); e != nil {
		return e
	}
	options = append(options, jsonv2.RejectUnknownMembers(true))
	if err := jsonv2.Unmarshal(data, target, options...); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

type ordinaryVerifiedInputs struct {
	profile      OrdinaryProfile
	assets       OrdinaryAssetRegistration
	registration OrdinaryRegistration
	plan         OrdinaryShardPlan
	registry     queryregistrycore.Registry
	route        QualRoute
}

// The fixed P0 pins permit route selection without rebuilding the full inventory AST.
func (b *eligibilityBudget) ordinaryInputs(profileData, assetData, registrationData, planData, registryData, inventoryData, slotData []byte, registrySHA string) (out ordinaryVerifiedInputs, e *Error) {
	if e = b.decodeOrdinaryTyped("ordinary-registration", registrationData, &out.registration); e != nil {
		return out, e
	}
	var err error
	if out.registration, err = validateOrdinaryRegistrationShape(out.registration); err != nil {
		return out, asError(err)
	}
	r := out.registration
	if e = b.decodeOrdinaryTyped("ordinary-profile", profileData, &out.profile); e != nil {
		return out, e
	}
	if out.profile, err = validateOrdinaryProfile(out.profile); err != nil {
		return out, asError(err)
	}
	if e = b.decodeOrdinaryTyped("ordinary-assets", assetData, &out.assets); e != nil {
		return out, e
	}
	if out.assets, err = validateOrdinaryAssets(out.assets); err != nil {
		return out, asError(err)
	}
	if e = b.charge(3, uint64(len(registryData)+len(inventoryData)+len(slotData)), "ordinary-source-pins"); e != nil {
		return out, e
	}
	if registrySHA != ordinaryRegistrySHA || queryregistrycore.FilePin(ordinaryRegistryPath, registryData) != (OrdinaryPin{Path: ordinaryRegistryPath, Bytes: ordinaryRegistryBytes, SHA256: ordinaryRegistrySHA}) || queryregistrycore.FilePin(ordinarySlotsPath, slotData) != queryregistrycore.SlotPin || r.Registry != queryregistrycore.FilePin(ordinaryRegistryPath, registryData) || r.Inventory != queryregistrycore.QualificationPin || r.Slots != queryregistrycore.SlotPin {
		return out, fail(KindInvalidInput, "REGISTRY_SOURCE_PIN", "ordinary", nil)
	}
	if e = inventoryTyped(registryData, &out.registry, b); e != nil {
		return out, e
	}
	selected, e := parseQualificationInventoryRoute(inventoryData, r.Inventory.SHA256, r.Route, b)
	if e != nil {
		return out, e
	}
	out.route = selected.Route
	if e = b.decodeOrdinaryTyped("ordinary-plan", planData, &out.plan); e != nil {
		return out, e
	}
	if out.plan, err = validateOrdinaryPlanShape(out.plan); err != nil {
		return out, asError(err)
	}
	if e = b.ordinaryPlanUnion(out.plan, out.registry); e != nil {
		return out, e
	}
	if e = b.ordinarySelectedRegistration(out, profileData, assetData, registrationData, selected.RouteIndex); e != nil {
		return out, e
	}
	return out, nil
}

// Reconstruct every ordinary coordinate from the plan, then compare each set with P0's
// independently pinned digest. No other route's source/CST is decoded for this pass.
func (b *eligibilityBudget) ordinaryPlanUnion(plan OrdinaryShardPlan, registry queryregistrycore.Registry) *Error {
	bad := func() *Error { return fail(KindInvalidInput, "ORDINARY_PLAN_MEMBERSHIP", "ordinary-plan", nil) }
	if plan.Namespace != registry.Namespace || plan.Registry.SHA256 != ordinaryRegistrySHA || len(plan.Routes) != len(registry.Ordinary) {
		return bad()
	}
	routes := map[string]OrdinaryPlanRoute{}
	for i, route := range plan.Routes {
		original := registry.Ordinary[i]
		if route.Route != original.Route || route.Set != original.Set || len(route.NativeQueryIDs) == 0 || len(route.NativeQueryIDs) > MaxQueries || routes[route.Route].Route != "" {
			return bad()
		}
		routes[route.Route] = route
	}
	coordinates := make([]OrdinaryCoordinate, 0, plan.Required)
	if e := b.charge(plan.Required+uint64(len(plan.Shards)), 1024*plan.Required, "ordinary-plan-coordinate-index"); e != nil {
		return e
	}
	for _, shard := range plan.Shards {
		route := routes[shard.Route]
		if route.Route == "" || shard.Set != route.Set || !ordinaryHost(shard.Platform, shard.Subject) || len(shard.SourceHistory) == 0 || len(shard.SourceHistory) > 5 || shard.Case == "" {
			return bad()
		}
		index := 0
		for _, q := range route.NativeQueryIDs {
			for step := range shard.SourceHistory {
				for _, role := range []string{"incremental", "fresh"} {
					if step == 0 && role == "fresh" {
						continue
					}
					c := OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: shard.Route, Platform: shard.Platform, Subject: shard.Subject, Role: "primary", Set: shard.Set, Case: shard.Case, Query: q, Step: uint32(step), TreeRole: role, Context: "qualification/r8"}
					if e := b.charge(1, 1024, "ordinary-plan-coordinate-hash"); e != nil {
						return e
					}
					if index >= len(shard.ObligationIDs) || shard.ObligationIDs[index] != c.ID(plan.Namespace) {
						return bad()
					}
					coordinates = append(coordinates, c)
					index++
				}
			}
		}
		if index != len(shard.ObligationIDs) {
			return bad()
		}
	}
	// Charge sorting and canonical strings before queryregistrycore.Sets allocates them.
	if e := b.charge(32*uint64(len(coordinates)), 2048*uint64(len(coordinates)), "ordinary-plan-set-digests"); e != nil {
		return e
	}
	want := []queryregistrycore.RequiredSet{}
	for _, set := range registry.RequiredSets {
		if set.Kind == queryregistrycore.Structural && set.Scope != "owned-plain" {
			want = append(want, set)
		}
	}
	if !reflect.DeepEqual(queryregistrycore.Sets(coordinates), want) {
		return bad()
	}
	return b.r.check()
}

func (b *eligibilityBudget) ordinarySelectedRegistration(in ordinaryVerifiedInputs, profileData, assetData, registrationData []byte, routeIndex int) *Error {
	r, p, a, plan, route := in.registration, in.profile, in.assets, in.plan, in.route
	bad := func(code string) *Error { return fail(KindInvalidInput, code, "ordinary-registration", nil) }
	registered := slices.ContainsFunc(plan.Registrations, func(pin OrdinaryPin) bool { return pin == queryregistrycore.FilePin(pin.Path, registrationData) })
	if !registered || r.Namespace != plan.Namespace || r.Registry != plan.Registry || r.Profile != queryregistrycore.FilePin(r.Profile.Path, profileData) || r.Assets != queryregistrycore.FilePin(r.Assets.Path, assetData) || r.Candidate != p.Candidate || r.SemanticCohort != p.SemanticCohort || a.Registry != r.Registry || a.Candidate != r.Candidate || a.SemanticCohort != r.SemanticCohort || plan.Profile != r.Profile || plan.Assets != r.Assets || plan.Candidate != r.Candidate || plan.SemanticCohort != r.SemanticCohort {
		return bad("ORDINARY_REGISTRATION_BINDING")
	}
	var binding *OrdinaryBinding
	for i := range p.Bindings {
		if p.Bindings[i].Route == r.Route && p.Bindings[i].Platform == r.Platform {
			binding = &p.Bindings[i]
		}
	}
	if binding == nil || binding.Subject != r.Subject || binding.ClosureEncoding != r.ClosureEncoding || !reflect.DeepEqual(binding.Grammar, route.Workload.Grammar) || !reflect.DeepEqual(binding.Members, r.PlannedMembers) || len(r.Cases) != len(route.Workload.Cases) || len(r.Queries) != len(route.Workload.Queries) || r.Set != route.Workload.Set {
		return bad("ORDINARY_WORKLOAD_BINDING")
	}
	foundBinding := false
	for _, v := range plan.Bindings {
		if v.Route == r.Route && v.Platform == r.Platform {
			foundBinding = reflect.DeepEqual(*binding, v)
		}
	}
	if !foundBinding {
		return bad("ORDINARY_PLAN_BINDING")
	}
	assets := map[string]string{}
	for _, asset := range a.Assets {
		assets[asset.ID] = asset.Role
	}
	for i, q := range r.Queries {
		original := route.Workload.Queries[i]
		if q.ID != original.ID || q.Source.SHA256 != original.SHA256 || assets[q.MetadataAsset] != "metadata" {
			return bad("ORDINARY_QUERY_BINDING")
		}
	}
	shards := map[string]OrdinaryPlanShard{}
	for _, shard := range plan.Shards {
		if shard.Route == r.Route && shard.Platform == r.Platform {
			shards[shard.Case] = shard
		}
	}
	if e := b.charge(uint64(len(r.Required)+len(r.Cases)+len(r.Queries)), 512*uint64(len(r.Required)), "ordinary-registration-index"); e != nil {
		return e
	}
	ids := map[string]OrdinaryCoordinate{}
	for i, c := range r.Cases {
		original, shard := route.Workload.Cases[i], shards[c.ID]
		selector := "qualification_inventory#/routes/" + strconv.Itoa(routeIndex) + "/workload/cases/" + strconv.Itoa(i)
		if c.ID != original.ID || c.OriginalSelector != selector || !semanticEditsEqual(c.Edits, original.Edits) || c.InitialSource.Bytes != original.Input.Bytes || c.InitialSource.SHA256 != original.Input.SHA256 || len(c.VersionPins) != len(shard.SourceHistory) || !reflect.DeepEqual(c.Points, shard.EditPoints) {
			return bad("ORDINARY_HISTORY_BINDING")
		}
		for step, pin := range c.VersionPins {
			if pin.Bytes != shard.SourceHistory[step].Bytes || pin.SHA256 != shard.SourceHistory[step].SHA256 {
				return bad("ORDINARY_SOURCE_BINDING")
			}
		}
		for _, q := range r.Queries {
			for step := range c.VersionPins {
				for _, role := range []string{"incremental", "fresh"} {
					if step == 0 && role == "fresh" {
						continue
					}
					coord := OrdinaryCoordinate{Kind: queryregistrycore.Structural, Route: r.Route, Platform: r.Platform, Subject: r.Subject, Role: "primary", Set: r.Set, Case: c.ID, Query: q.ID, Step: uint32(step), TreeRole: role, Context: r.MembershipContext}
					ids[coord.ID(r.Namespace)] = coord
				}
			}
		}
	}
	if len(ids) != len(r.Required) {
		return bad("ORDINARY_REQUIRED_GAP")
	}
	for _, required := range r.Required {
		if ids[required.ID] != required.Coordinate || assets[required.TreeAsset] != "expected-tree" || assets[required.CaptureAsset] != "expected-captures" || strings.HasPrefix(required.Coordinate.Query, "facts.") && assets[required.FactAsset] != "expected-facts" || required.FactAsset != "" && assets[required.FactAsset] != "expected-facts" {
			return bad("ORDINARY_REQUIRED_ASSET")
		}
		delete(ids, required.ID)
	}
	return b.r.check()
}
