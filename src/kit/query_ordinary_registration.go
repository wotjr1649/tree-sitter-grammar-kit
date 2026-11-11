package kit

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistrycore"
)

// Parse the existing qualification contract before constructing the dependency-free view.
func ordinaryRegistry(req OrdinaryPlanRequest) (*queryregistrycore.Validated, QualificationInventory, error) {
	var inv QualificationInventory
	if queryregistrycore.FilePin(queryregistrycore.QualificationPin.Path, req.Qualification) != queryregistrycore.QualificationPin || queryregistrycore.FilePin(queryregistrycore.SlotPin.Path, req.Slots) != queryregistrycore.SlotPin {
		return nil, inv, ordinaryBad("REGISTRY_SOURCE_PIN")
	}
	var err error
	inv, err = ParseQualificationInventory(req.Qualification)
	if err != nil {
		return nil, inv, err
	}
	var slots struct {
		Extra []jsontext.Value `json:"extra_roles"`
	}
	if err = jsonv2.Unmarshal(req.Slots, &slots); err != nil {
		return nil, inv, err
	}
	view := queryregistrycore.SourceView{Qualification: req.Qualification, Slots: req.Slots, Schema: inv.Schema, SlotExtras: slots.Extra}
	for _, r := range inv.Routes {
		route := queryregistrycore.Route{Route: r.Route, Workload: queryregistrycore.Workload{Set: r.Workload.Set}}
		for _, c := range r.Workload.Cases {
			route.Workload.Cases = append(route.Workload.Cases, queryregistrycore.Case{ID: c.ID, EditCount: uint32(len(c.Edits))})
		}
		for _, q := range r.Workload.Queries {
			route.Workload.Queries = append(route.Workload.Queries, queryregistrycore.Query{ID: q.ID})
		}
		view.Routes = append(view.Routes, route)
	}
	roles := make([]jsontext.Value, 0, len(inv.ExtraRoles))
	for _, role := range inv.ExtraRoles {
		raw, err := jsonv2.Marshal(role)
		if err != nil {
			return nil, inv, err
		}
		roles = append(roles, raw)
	}
	view.Large = queryregistrycore.LargeWorkloads(roles)
	v, err := queryregistrycore.Validate(req.Registry, req.RegistrySHA256, view, req.Basis)
	if e, ok := err.(*queryregistrycore.Fault); ok {
		return nil, inv, fail(e.Kind, e.Code, e.Path, e.Cause)
	}
	return v, inv, err
}

func validateOrdinaryRegistration(r OrdinaryRegistration, p OrdinaryProfile, a OrdinaryAssetRegistration, w ordinarySourceExport, inv QualificationInventory, profileBytes, assetBytes []byte, v *queryregistrycore.Validated) error {
	if r.Registry != w.Registry || r.Namespace != v.Namespace() || r.Inventory != w.Qualification || r.Slots != w.Slots || r.Profile != queryregistrycore.FilePin(r.Profile.Path, profileBytes) || r.Assets != queryregistrycore.FilePin(r.Assets.Path, assetBytes) || r.Candidate != p.Candidate || r.SemanticCohort != p.SemanticCohort || a.Registry != r.Registry || a.Candidate != p.Candidate || a.SemanticCohort != p.SemanticCohort {
		return ordinaryBad("ORDINARY_REGISTRATION_BINDING")
	}
	var binding *OrdinaryBinding
	for i := range p.Bindings {
		b := &p.Bindings[i]
		if b.Route == r.Route && b.Platform == r.Platform {
			binding = b
			break
		}
	}
	if binding == nil || binding.Subject != r.Subject || binding.ClosureEncoding != r.ClosureEncoding || !reflect.DeepEqual(binding.Members, r.PlannedMembers) {
		return ordinaryBad("ORDINARY_REGISTRATION_BINDING")
	}
	var route *QualRoute
	for i := range inv.Routes {
		if inv.Routes[i].Route == r.Route {
			route = &inv.Routes[i]
			break
		}
	}
	if route == nil || r.Set != route.Workload.Set || !reflect.DeepEqual(binding.Grammar, route.Workload.Grammar) {
		return ordinaryBad("ORDINARY_GRAMMAR_BINDING")
	}
	var source *ordinarySourceRoute
	for i := range w.Routes {
		if w.Routes[i].Route == r.Route {
			source = &w.Routes[i]
			break
		}
	}
	if source == nil || len(r.Cases) != len(source.Cases) || len(r.Queries) != len(source.Queries) {
		return ordinaryBad("ORDINARY_WORKLOAD_BINDING")
	}
	versions := map[string]uint64{}
	for _, version := range source.Versions {
		versions[version.SHA256] = version.Bytes
	}
	for i, c := range r.Cases {
		s := source.Cases[i]
		original := route.Workload.Cases[i]
		if c.ID != s.ID || c.OriginalSelector != s.Selector || !semanticEditsEqual(c.Edits, original.Edits) || !reflect.DeepEqual(c.Points, s.Points) || len(c.VersionPins) != len(s.Steps) {
			return ordinaryBad("ORDINARY_HISTORY_BINDING")
		}
		for step, pin := range c.VersionPins {
			if !ordinaryPin(pin) || pin.Path != "source-witness/"+s.Steps[step]+".utf8" || pin.SHA256 != s.Steps[step] || pin.Bytes != versions[s.Steps[step]] {
				return ordinaryBad("ORDINARY_SOURCE_BINDING")
			}
		}
		if c.InitialSource != c.VersionPins[0] {
			return ordinaryBad("ORDINARY_SOURCE_BINDING")
		}
	}
	assets := map[string]OrdinaryAsset{}
	for _, asset := range a.Assets {
		assets[asset.ID] = asset
	}
	role := func(id, want string) bool { asset, ok := assets[id]; return ok && asset.Role == want }
	for i, q := range r.Queries {
		if q.ID != source.Queries[i].ID || !ordinaryPin(q.Source) || q.Source.SHA256 != source.Queries[i].SHA256 || q.Kind != queryregistrycore.Structural || !role(q.MetadataAsset, "metadata") {
			return ordinaryBad("ORDINARY_QUERY_BINDING")
		}
	}
	expected := map[string]OrdinaryCoordinate{}
	for _, c := range v.Coordinates() {
		if c.Kind == queryregistrycore.Structural && c.Role == "primary" && c.Route == r.Route && c.Platform == r.Platform {
			expected[c.ID(v.Namespace())] = c
		}
	}
	if len(expected) != len(r.Required) {
		return ordinaryBad("ORDINARY_REQUIRED_GAP")
	}
	seen := map[string]bool{}
	for _, required := range r.Required {
		c, ok := expected[required.ID]
		if !ok || c != required.Coordinate || c.ID(r.Namespace) != required.ID || seen[required.ID] {
			return ordinaryBad("ORDINARY_REQUIRED_COORDINATE")
		}
		seen[required.ID] = true
		if !role(required.TreeAsset, "expected-tree") || !role(required.CaptureAsset, "expected-captures") || strings.HasPrefix(c.Query, "facts.") && !role(required.FactAsset, "expected-facts") || required.FactAsset != "" && !role(required.FactAsset, "expected-facts") {
			return ordinaryBad("ORDINARY_REQUIRED_ASSET")
		}
	}
	return nil
}
