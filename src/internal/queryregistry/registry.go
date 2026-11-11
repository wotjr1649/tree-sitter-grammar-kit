// Package queryregistry adapts the qualification inventory to the dependency-free P0 core.
// It does not interpret queries or certify runtime evidence.
package queryregistry

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistrycore"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	Path       = queryregistrycore.Path
	Schema     = queryregistrycore.Schema
	Policy     = queryregistrycore.Policy
	MaxBytes   = queryregistrycore.MaxBytes
	Structural = queryregistrycore.Structural
	Predicate  = queryregistrycore.Predicate
	Control    = queryregistrycore.Control
)

type Pin = queryregistrycore.Pin
type Platform = queryregistrycore.Platform
type Ordinary = queryregistrycore.Ordinary
type Extra = queryregistrycore.Extra
type Asset = queryregistrycore.Asset
type RequiredSet = queryregistrycore.RequiredSet
type Registry = queryregistrycore.Registry
type LargeWorkload = queryregistrycore.LargeWorkload
type Coordinate = queryregistrycore.Coordinate
type Probe = queryregistrycore.Probe
type PolicyInput = queryregistrycore.PolicyInput
type Subject = queryregistrycore.Subject
type Judgment = queryregistrycore.Judgment
type Shard = queryregistrycore.Shard
type Counter = queryregistrycore.Counter
type Coverage = queryregistrycore.Coverage

var QualificationPin = queryregistrycore.QualificationPin
var SlotPin = queryregistrycore.SlotPin

type Validated struct{ inner *queryregistrycore.Validated }

func Platforms() []Platform                { return queryregistrycore.Platforms() }
func MissingAssets() []Asset               { return queryregistrycore.MissingAssets() }
func MissingScopes() []string              { return queryregistrycore.MissingScopes() }
func Digest(data []byte) string            { return queryregistrycore.Digest(data) }
func FilePin(path string, data []byte) Pin { return queryregistrycore.FilePin(path, data) }
func Parse(data []byte) (Registry, error) {
	reg, err := queryregistrycore.Parse(data)
	return reg, fromCoreError(err)
}
func Canonical(value any) []byte                    { return queryregistrycore.Canonical(value) }
func Namespace(reg Registry) string                 { return queryregistrycore.Namespace(reg) }
func Roster() []Probe                               { return queryregistrycore.Roster() }
func ExtraRoles(originals []jsontext.Value) []Extra { return queryregistrycore.ExtraRoles(originals) }
func Sets(coordinates []Coordinate) []RequiredSet   { return queryregistrycore.Sets(coordinates) }

func Select(route kit.QualRoute, index int) Ordinary {
	return queryregistrycore.Select(routeView(route), index)
}

func LargeWorkloads(inv kit.QualificationInventory) []LargeWorkload {
	roles := make([]jsontext.Value, 0, len(inv.ExtraRoles))
	for _, role := range inv.ExtraRoles {
		raw, err := jsonv2.Marshal(role)
		if err != nil {
			return nil
		}
		roles = append(roles, raw)
	}
	return queryregistrycore.LargeWorkloads(roles)
}

func Expand(inv kit.QualificationInventory, platforms []Platform, probes []Probe) []Coordinate {
	view := routesView(inv)
	return queryregistrycore.Expand(view, platforms, probes, LargeWorkloads(inv))
}

// Sources checks the pinned source bytes before invoking the owning qualification parser.
func Sources(qualification, slots []byte) (kit.QualificationInventory, []jsontext.Value, error) {
	var inv kit.QualificationInventory
	if uint64(len(qualification)) != QualificationPin.Bytes || uint64(len(slots)) != SlotPin.Bytes || FilePin(QualificationPin.Path, qualification) != QualificationPin || FilePin(SlotPin.Path, slots) != SlotPin {
		return inv, nil, &kit.Error{Kind: kit.KindInvalidInput, Code: "REGISTRY_SOURCE_PIN", Path: Path}
	}
	var err error
	inv, err = kit.ParseQualificationInventory(qualification)
	if err != nil {
		return inv, nil, err
	}
	if inv.Schema != "tsgk-qualification-inventory/r8" {
		return inv, nil, &kit.Error{Kind: kit.KindInvalidInput, Code: "REGISTRY_SOURCE_SCHEMA", Path: Path}
	}
	var slot struct {
		Extra []jsontext.Value `json:"extra_roles"`
	}
	if err := jsonv2.Unmarshal(slots, &slot); err != nil {
		return inv, nil, err
	}
	return inv, slot.Extra, nil
}

// SourceView converts the parsed qualification source to the small projection consumed by the
// core. Callers must use the owning parser and preserve the original pinned bytes.
func SourceView(inv kit.QualificationInventory, qualification, slots []byte, slotExtras []jsontext.Value) queryregistrycore.SourceView {
	return queryregistrycore.SourceView{
		Qualification: qualification,
		Slots:         slots,
		Schema:        inv.Schema,
		Routes:        routesView(inv),
		SlotExtras:    slotExtras,
		Large:         LargeWorkloads(inv),
	}
}

func routesView(inv kit.QualificationInventory) []queryregistrycore.Route {
	out := make([]queryregistrycore.Route, 0, len(inv.Routes))
	for _, route := range inv.Routes {
		out = append(out, routeView(route))
	}
	return out
}

func routeView(route kit.QualRoute) queryregistrycore.Route {
	w := queryregistrycore.Workload{Set: route.Workload.Set,
		Cases:   make([]queryregistrycore.Case, 0, len(route.Workload.Cases)),
		Queries: make([]queryregistrycore.Query, 0, len(route.Workload.Queries)),
	}
	for _, c := range route.Workload.Cases {
		w.Cases = append(w.Cases, queryregistrycore.Case{ID: c.ID, EditCount: uint32(len(c.Edits))})
	}
	for _, q := range route.Workload.Queries {
		w.Queries = append(w.Queries, queryregistrycore.Query{ID: q.ID})
	}
	return queryregistrycore.Route{Route: route.Route, Workload: w}
}

func Validate(data []byte, callerSHA string, qualification, slots []byte, basis map[string][]byte) (*Validated, error) {
	inv, extras, err := Sources(qualification, slots)
	if err != nil {
		if preflightErr := queryregistrycore.Preflight(data, callerSHA); preflightErr != nil {
			return nil, fromCoreError(preflightErr)
		}
		return nil, err
	}
	inner, err := queryregistrycore.Validate(data, callerSHA, SourceView(inv, qualification, slots, extras), basis)
	if err != nil {
		return nil, fromCoreError(err)
	}
	return &Validated{inner: inner}, nil
}

func (v *Validated) Coordinates() []Coordinate { return v.inner.Coordinates() }
func (v *Validated) SHA256() string            { return v.inner.SHA256() }
func (v *Validated) Namespace() string         { return v.inner.Namespace() }
func (v *Validated) VerifyExact(shards []Shard) error {
	return fromCoreError(v.inner.VerifyExact(shards))
}
func (v *Validated) Fold(shards []Shard, missingInput bool) (Coverage, error) {
	coverage, err := v.inner.Fold(shards, missingInput)
	return coverage, fromCoreError(err)
}

func fromCoreError(err error) error {
	if err == nil {
		return nil
	}
	var fault *queryregistrycore.Fault
	if errors.As(err, &fault) {
		kind := kit.KindInvalidInput
		if fault.Kind == "RESOURCE_LIMIT" {
			kind = kit.KindResourceLimit
		}
		return &kit.Error{Kind: kind, Code: fault.Code, Path: fault.Path, Cause: fault.Cause}
	}
	return err
}
