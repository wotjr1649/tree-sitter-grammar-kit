// Package queryregistrycore validates the P0 data-only obligation registry.
// It does not interpret queries or certify runtime evidence.
package queryregistrycore

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
)

const (
	Path       = "src/contracts/query-semantic-registry-r1.json"
	Schema     = "tsgk-query-semantic-registry/r1"
	Policy     = "tsgk-full-query-obligations/r1"
	MaxBytes   = 256 * 1024
	Structural = "STRUCTURAL_ONLY"
	Predicate  = "PREDICATE_REQUIRED"
	Control    = "EXPECTED_REJECTION"
)

type Pin struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

var QualificationPin = Pin{"src/contracts/qualification-c1.json", 8297072, "365154d19396001ea93f5cb7ceaf46a4f30b94e421079e7018b117165546712e"}
var SlotPin = Pin{"src/contracts/query-slot-inventory-r1.json", 60490, "bca429d4e02729db4e66ae53ad31b835ec0b95471504d9b9ef0499b8b19bc0b6"}

type Platform struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
}

func Platforms() []Platform {
	return []Platform{{"windows-amd64", "windows/amd64"}, {"linux-amd64", "linux/amd64"}}
}

type Ordinary struct {
	Route     string `json:"route"`
	Role      string `json:"role"`
	Set       string `json:"set"`
	Selection string `json:"selection"`
	Cases     string `json:"cases_ref"`
	Queries   string `json:"queries_ref"`
	Sources   string `json:"sources_ref"`
	Expected  string `json:"expected_ref"`
}

// These source-only fields are the qualification data the P0 registry expands.
type Route struct {
	Route    string
	Workload Workload
}

type Workload struct {
	Set     string
	Cases   []Case
	Queries []Query
}

type Case struct {
	ID        string
	EditCount uint32
}

type Query struct {
	ID string
}

type SourceView struct {
	Qualification []byte
	Slots         []byte
	Schema        string
	Routes        []Route
	SlotExtras    []jsontext.Value
	Large         []LargeWorkload
}

func Select(route Route, index int) Ordinary {
	p := fmt.Sprintf("qualification_inventory#/routes/%d/workload", index)
	return Ordinary{route.Route, "primary", route.Workload.Set, "ALL", p + "/cases", p + "/queries", p + "/cases/*/{input,source,edits}", "independent-expectations"}
}

type Extra struct {
	Source       string         `json:"source_ref"`
	Registration string         `json:"registration"`
	Required     *uint64        `json:"required"`
	Original     jsontext.Value `json:"original"`
}

type Asset struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	Pin    *Pin   `json:"pin"`
}

func MissingAssets() []Asset {
	var out []Asset
	for _, id := range []string{"source-witness", "independent-metadata", "independent-expectations", "full-cst-api", "ordered-captures-facts", "current-observations", "caller-approval", "stored-replay"} {
		out = append(out, Asset{id, "UNAVAILABLE", "NOT_REGISTERED_IN_P0", nil})
	}
	return out
}

func MissingScopes() []string {
	return []string{"svc-segment-range-history-codebehind", "large-node-occurrence-page-footer", "independent-assets", "current-cohort-approval-replay"}
}

type RequiredSet struct {
	Kind         string `json:"kind"`
	Scope        string `json:"scope"`
	Platform     string `json:"platform"`
	Registration string `json:"registration"`
	Required     uint64 `json:"required"`
	Digest       string `json:"coordinate_sha256"`
}

type Registry struct {
	Schema            string          `json:"schema"`
	Policy            string          `json:"policy"`
	Qualification     Pin             `json:"qualification_inventory"`
	Slots             Pin             `json:"query_slot_inventory"`
	Basis             []Pin           `json:"predicate_basis"`
	Platforms         []Platform      `json:"platforms"`
	Ordinary          []Ordinary      `json:"ordinary"`
	Probes            []Probe         `json:"predicate"`
	Large             []LargeWorkload `json:"large_workloads"`
	Extra             []Extra         `json:"extra_roles"`
	Assets            []Asset         `json:"assets"`
	MissingScopes     []string        `json:"missing_scopes"`
	ContentState      string          `json:"content_state"`
	Namespace         string          `json:"membership_namespace_sha256"`
	RequiredSets      []RequiredSet   `json:"required_sets"`
	FullScopeComplete bool            `json:"full_scope_complete"`
}

type LargeWorkload struct {
	Set       string `json:"set"`
	Case      string `json:"case"`
	Query     string `json:"query"`
	SourceRef string `json:"source_ref"`
	Kind      string `json:"kind"`
	Status    string `json:"expected_status"`
}

// Workload membership is known; its node/occurrence/page/footer denominator is not.
func LargeWorkloads(roles []jsontext.Value) []LargeWorkload {
	out := []LargeWorkload{}
	for i, raw := range roles {
		var role struct {
			ID        string `json:"id"`
			Workloads []struct {
				Set   string `json:"set"`
				Cases []struct {
					ID           string `json:"id"`
					ExpectStatus string `json:"expect_status"`
				} `json:"cases"`
				Queries []struct {
					ID string `json:"id"`
				} `json:"queries"`
			} `json:"workloads"`
		}
		if jsonv2.Unmarshal(raw, &role) != nil {
			return nil
		}
		if role.ID != "n461-large" {
			continue
		}
		for wi, w := range role.Workloads {
			for ci, c := range w.Cases {
				kind, status := "LARGE_WORKLOAD_POSITIVE", "COMPLETED"
				if c.ExpectStatus != "" && c.ExpectStatus != "COMPLETED" {
					kind, status = "LARGE_WORKLOAD_CONTROL", c.ExpectStatus
				}
				for _, q := range w.Queries {
					out = append(out, LargeWorkload{w.Set, c.ID, q.ID, fmt.Sprintf("qualification_inventory#/extra_roles/%d/workloads/%d/cases/%d", i, wi, ci), kind, status})
				}
			}
		}
	}
	return out
}

func Digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func FilePin(path string, data []byte) Pin { return Pin{path, uint64(len(data)), Digest(data)} }

type Fault struct {
	Kind  string
	Code  string
	Path  string
	Cause error
}

func (e *Fault) Error() string {
	if e.Cause == nil {
		return e.Code
	}
	return e.Code + ": " + e.Cause.Error()
}

func (e *Fault) Unwrap() error { return e.Cause }

func invalid(code string, cause error) error {
	kind := "INVALID_INPUT"
	if code == "REGISTRY_SIZE" || code == "REGISTRY_SHAPE_LIMIT" {
		kind = "RESOURCE_LIMIT"
	}
	return &Fault{Kind: kind, Code: code, Path: Path, Cause: cause}
}

// Strict decoding retains field presence and nullability before typed zero values can hide them.
func Parse(data []byte) (Registry, error) {
	var reg Registry
	if len(data) > MaxBytes {
		return reg, invalid("REGISTRY_SIZE", nil)
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(data))
	for values := 0; ; values++ {
		_, err := decoder.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return reg, invalid("REGISTRY_JSON", err)
		}
		if decoder.StackDepth() > 64 || values >= 100000 {
			return reg, invalid("REGISTRY_SHAPE_LIMIT", nil)
		}
	}
	if err := jsonv2.Unmarshal(data, &reg, jsonv2.RejectUnknownMembers(true)); err != nil {
		return reg, invalid("REGISTRY_JSON", err)
	}
	encoded, err := jsonv2.Marshal(reg)
	if err != nil {
		return reg, invalid("REGISTRY_JSON", err)
	}
	var original, roundtrip any
	if err := jsonv2.Unmarshal(data, &original); err != nil {
		return reg, invalid("REGISTRY_JSON", err)
	}
	if err := jsonv2.Unmarshal(encoded, &roundtrip); err != nil {
		return reg, invalid("REGISTRY_JSON", err)
	}
	if !reflect.DeepEqual(original, roundtrip) {
		return reg, invalid("REGISTRY_FIELD_SHAPE", nil)
	}
	return reg, nil
}

// Canonical is UTF-8 JSON with fixed struct/array order, sorted map keys and no newline.
func Canonical(value any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(value); err != nil {
		panic(err)
	} // All callers use JSON data types.
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

func Namespace(reg Registry) string {
	// Expected output and derived digests are deliberately outside the membership preimage.
	probes := make([]any, 0, len(reg.Probes))
	for _, p := range reg.Probes {
		probes = append(probes, []any{p.ID, p.Kind, p.Source, p.Query, p.Encoding, p.Basis, p.PolicyInput})
	}
	extras := make([]any, 0, len(reg.Extra))
	for _, e := range reg.Extra {
		var original any
		if err := jsonv2.Unmarshal(e.Original, &original); err != nil {
			return ""
		}
		extras = append(extras, []any{e.Source, e.Registration, e.Required, original})
	}
	large := make([]any, 0, len(reg.Large))
	for _, w := range reg.Large {
		large = append(large, []any{w.Set, w.Case, w.Query, w.SourceRef, w.Kind})
	}
	return Digest(Canonical([]any{"tsgk-query-membership/r1", reg.Policy, reg.Qualification, reg.Slots, reg.Basis, reg.Platforms, reg.Ordinary, probes, extras, large}))
}

type Coordinate struct {
	Kind     string `json:"kind"`
	Route    string `json:"route"`
	Platform string `json:"platform"`
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	Set      string `json:"set"`
	Case     string `json:"case"`
	Query    string `json:"query"`
	Step     uint32 `json:"step"`
	TreeRole string `json:"tree_role"`
	Context  string `json:"context"`
}

func (c Coordinate) Canonical() []byte {
	if b, ok := appendCoordinate(append(make([]byte, 0, 192), '['), c); ok {
		return append(b, ']')
	}
	return c.slowCanonical()
}

func (c Coordinate) slowCanonical() []byte {
	return Canonical([]any{c.Kind, c.Route, c.Platform, c.Subject, c.Role, c.Set, c.Case, c.Query, c.Step, c.TreeRole, c.Context})
}

func (c Coordinate) ID(namespace string) string {
	b := append(make([]byte, 0, 256), `["tsgk-query-obligation-id/r1",`...)
	b, ok := appendPlainJSONString(b, namespace)
	if ok {
		if b, ok = appendCoordinate(append(b, ','), c); ok {
			return "qsem1:" + Digest(append(b, ']'))
		}
	}
	return c.slowID(namespace)
}

func (c Coordinate) slowID(namespace string) string {
	return "qsem1:" + Digest(Canonical([]any{"tsgk-query-obligation-id/r1", namespace, c.Kind, c.Route, c.Platform, c.Subject, c.Role, c.Set, c.Case, c.Query, c.Step, c.TreeRole, c.Context}))
}

// appendCoordinate writes the eleven Canonical elements without reflection. It handles only strings of
// printable ASCII without quote or backslash, where JSON encoding is the identity; it reports false
// for anything else and the caller falls back to Canonical, so the bytes never differ.
func appendCoordinate(b []byte, c Coordinate) ([]byte, bool) {
	ok := true
	str := func(s string, comma bool) {
		if comma {
			b = append(b, ',')
		}
		if ok {
			b, ok = appendPlainJSONString(b, s)
		}
	}
	str(c.Kind, false)
	for _, s := range [...]string{c.Route, c.Platform, c.Subject, c.Role, c.Set, c.Case, c.Query} {
		str(s, true)
	}
	b = strconv.AppendUint(append(b, ','), uint64(c.Step), 10)
	str(c.TreeRole, true)
	str(c.Context, true)
	return b, ok
}

func appendPlainJSONString(b []byte, s string) ([]byte, bool) {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		if ch := s[i]; ch < 0x20 || ch > 0x7e || ch == '"' || ch == '\\' {
			return b, false
		}
	}
	return append(append(b, s...), '"'), true
}

// Expand visits the original pinned inventory, never a result or a filtered expectation list.
func Expand(inv []Route, platforms []Platform, probes []Probe, large []LargeWorkload) []Coordinate {
	var out []Coordinate
	for _, route := range inv {
		w := route.Workload
		for _, p := range platforms {
			for _, c := range w.Cases {
				for _, q := range w.Queries {
					for step := uint64(0); step <= uint64(c.EditCount); step++ {
						out = append(out, Coordinate{Structural, route.Route, p.ID, p.Subject, "primary", w.Set, c.ID, q.ID, uint32(step), "incremental", "qualification/r8"})
						if step != 0 {
							out = append(out, Coordinate{Structural, route.Route, p.ID, p.Subject, "primary", w.Set, c.ID, q.ID, uint32(step), "fresh", "qualification/r8"})
						}
					}
				}
			}
		}
	}
	for _, probe := range probes {
		for _, p := range platforms {
			out = append(out, Coordinate{probe.Kind, "owned-plain", p.ID, p.Subject, "predicate-probe", "predicate/r1", probe.ID, "q0", 0, "incremental", probe.Basis})
		}
	}
	for _, workload := range large {
		out = append(out, Coordinate{workload.Kind, "csharp", "windows-amd64", "windows/amd64", "n461-large", workload.Set, workload.Case, workload.Query, 0, "incremental", "qualification/r8-large-subledger-unregistered"})
	}
	return out
}

func setKey(c Coordinate) string { return c.Kind + "/" + c.Route + "/" + c.Platform }

func Sets(coordinates []Coordinate) []RequiredSet {
	groups := map[string][]string{}
	first := map[string]Coordinate{}
	for _, c := range coordinates {
		key := setKey(c)
		groups[key] = append(groups[key], string(c.Canonical()))
		first[key] = c
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]RequiredSet, 0, len(keys))
	for _, key := range keys {
		rows := groups[key]
		sort.Strings(rows)
		h := sha256.New()
		for _, row := range rows {
			var size [8]byte
			binary.BigEndian.PutUint64(size[:], uint64(len(row)))
			h.Write(size[:])
			h.Write([]byte(row))
		}
		c := first[key]
		out = append(out, RequiredSet{c.Kind, c.Route, c.Platform, "REGISTERED", uint64(len(rows)), hex.EncodeToString(h.Sum(nil))})
	}
	return out
}

func ExtraRoles(originals []jsontext.Value) []Extra {
	out := make([]Extra, 0, len(originals))
	for i, raw := range originals {
		var role struct {
			ID string `json:"id"`
		}
		if err := jsonv2.Unmarshal(raw, &role); err != nil {
			return nil
		}
		state := "PRESERVED"
		if role.ID == "n461-svc" || role.ID == "n461-large" {
			state = "UNREGISTERED"
		}
		out = append(out, Extra{fmt.Sprintf("query_slot_inventory#/extra_roles/%d", i), state, nil, raw})
	}
	return out
}

type Validated struct {
	registry    Registry
	sha         string
	coordinates []Coordinate
	byID        map[string]Coordinate
}

func (v *Validated) Coordinates() []Coordinate { return append([]Coordinate(nil), v.coordinates...) }
func (v *Validated) SHA256() string            { return v.sha }
func (v *Validated) Namespace() string         { return v.registry.Namespace }

func preflight(data []byte, callerSHA string) (Registry, error) {
	if len(data) > MaxBytes {
		return Registry{}, invalid("REGISTRY_SIZE", nil)
	}
	if len(callerSHA) != 64 || Digest(data) != callerSHA {
		return Registry{}, invalid("REGISTRY_CALLER_PIN", nil)
	}
	return Parse(data)
}

// Preflight preserves the registry-before-source error order for adapters that must parse
// another pinned source before calling Validate.
func Preflight(data []byte, callerSHA string) error {
	_, err := preflight(data, callerSHA)
	return err
}

// Validate is read-only. The caller must select the registry hash before observations exist.
func Validate(data []byte, callerSHA string, source SourceView, basis map[string][]byte) (*Validated, error) {
	reg, err := preflight(data, callerSHA)
	if err != nil {
		return nil, err
	}
	if FilePin(QualificationPin.Path, source.Qualification) != QualificationPin || FilePin(SlotPin.Path, source.Slots) != SlotPin {
		return nil, invalid("REGISTRY_SOURCE_PIN", nil)
	}
	if source.Schema != "tsgk-qualification-inventory/r8" {
		return nil, invalid("REGISTRY_SOURCE_SCHEMA", nil)
	}
	if reg.Schema != Schema || reg.Policy != Policy || reg.Qualification != QualificationPin || reg.Slots != SlotPin ||
		!reflect.DeepEqual(reg.Platforms, Platforms()) || reg.ContentState != "INCOMPLETE" || reg.FullScopeComplete ||
		!reflect.DeepEqual(reg.Assets, MissingAssets()) || !reflect.DeepEqual(reg.MissingScopes, MissingScopes()) ||
		!bytes.Equal(Canonical(reg.Extra), Canonical(ExtraRoles(source.SlotExtras))) || !reflect.DeepEqual(reg.Probes, Roster()) || !reflect.DeepEqual(reg.Large, source.Large) {
		return nil, invalid("REGISTRY_MEMBERSHIP", nil)
	}
	paths := []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"}
	if len(reg.Basis) != len(paths) {
		return nil, invalid("REGISTRY_BASIS", nil)
	}
	for i, path := range paths {
		raw, ok := basis[path]
		if !ok || reg.Basis[i] != FilePin(path, raw) {
			return nil, invalid("REGISTRY_BASIS_PIN", nil)
		}
	}
	if len(reg.Ordinary) != len(source.Routes) {
		return nil, invalid("REGISTRY_ORDINARY", nil)
	}
	for i, route := range source.Routes {
		if reg.Ordinary[i] != Select(route, i) {
			return nil, invalid("REGISTRY_SELECTION", nil)
		}
	}
	if reg.Namespace != Namespace(reg) {
		return nil, invalid("REGISTRY_NAMESPACE", nil)
	}
	coordinates := Expand(source.Routes, Platforms(), Roster(), source.Large)
	if !reflect.DeepEqual(reg.RequiredSets, Sets(coordinates)) {
		return nil, invalid("REGISTRY_REQUIRED_SETS", nil)
	}
	v := &Validated{reg, callerSHA, coordinates, make(map[string]Coordinate, len(coordinates))}
	for _, c := range coordinates {
		id := c.ID(reg.Namespace)
		if _, exists := v.byID[id]; exists {
			return nil, invalid("REGISTRY_DUPLICATE_COORDINATE", nil)
		}
		v.byID[id] = c
	}
	return v, nil
}
