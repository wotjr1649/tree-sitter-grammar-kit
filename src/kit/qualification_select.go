package kit

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"reflect"
	"unicode/utf8"
)

// A selection never masquerades as a complete inventory. Header has no routes or extra roles.
type qualificationInventorySelection struct {
	Header     QualificationInventory
	Route      QualRoute
	RouteIndex int
}

type inventoryProjection byte

const (
	inventoryDrop inventoryProjection = iota
	inventoryKeep
	inventoryRoot
	inventoryRoutes
	inventoryRoute
	inventoryWorkload
	inventoryRoles
	inventoryRole
	inventoryWorkloads
)

type inventorySpan struct{ start, end int }

type inventorySpecialSpan struct {
	inventorySpan
	name string
}

// Only the header and global ID/platform projection are copied; route spans alias guarded bytes.
type inventoryTokenIndex struct {
	data          []byte
	dec           *jsontext.Decoder
	b             *eligibilityBudget
	values        uint64
	routes        []inventorySpan
	specials      []inventorySpecialSpan
	schema        string
	schemaPresent bool
}

func inventoryChild(mode inventoryProjection, name string) inventoryProjection {
	switch mode {
	case inventoryRoot:
		switch name {
		case "routes":
			return inventoryRoutes
		case "extra_roles":
			return inventoryRoles
		default:
			return inventoryKeep
		}
	case inventoryRoute:
		switch name {
		case "route":
			return inventoryKeep
		case "workload":
			return inventoryWorkload
		}
	case inventoryWorkload:
		switch name {
		case "set", "profile", "route", "operation", "output", "symbol", "reuse_accepted":
			return inventoryKeep
		}
	case inventoryRole:
		if name == "workloads" {
			return inventoryWorkloads
		}
		return inventoryKeep
	case inventoryRoutes:
		return inventoryRoute
	case inventoryRoles:
		return inventoryRole
	case inventoryWorkloads:
		return inventoryWorkload
	}
	return inventoryDrop
}

func (s *inventoryTokenIndex) value(depth int, mode inventoryProjection) ([]byte, *Error) {
	const doc = "inventory"
	start := int(s.dec.InputOffset())
	tok, err := s.dec.ReadToken()
	if err != nil {
		return nil, jsonError(doc, "", err)
	}
	s.values++
	if s.values > 2000000 {
		return nil, fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc, nil)
	}
	if e := s.b.charge(1, 16, doc); e != nil {
		return nil, e
	}
	k := tok.Kind()
	if (k == '{' || k == '[') && depth >= jsonMaxDepth {
		return nil, fail(KindResourceLimit, "JSON_DEPTH_LIMIT", doc, nil)
	}
	project := mode != inventoryDrop && mode != inventoryKeep
	var out []byte
	if project && (k == '{' || k == '[') {
		out = append(out, byte(k))
	}
	if k == '{' || k == '[' {
		end := jsontext.Kind('}')
		if k == '[' {
			end = ']'
		}
		for s.dec.PeekKind() != end {
			if e := s.b.r.check(); e != nil {
				return nil, e
			}
			name := ""
			if k == '{' {
				key, err := s.dec.ReadToken()
				if err != nil {
					return nil, jsonError(doc, "", err)
				}
				name = key.String()
			}
			child := inventoryChild(mode, name)
			at := int(s.dec.InputOffset())
			special := -1
			if k == '{' && (name == "reuse_accepted" || name == "preprocess_cst_expect") {
				if e := s.b.charge(1, 128, "inventory-special-index"); e != nil {
					return nil, e
				}
				special = len(s.specials)
				s.specials = append(s.specials, inventorySpecialSpan{inventorySpan: inventorySpan{start: at}, name: name})
			}
			p, e := s.value(depth+1, child)
			if e != nil {
				return nil, e
			}
			if special >= 0 {
				s.specials[special].end = int(s.dec.InputOffset())
			}
			if mode == inventoryRoot && name == "schema" && len(p) > 0 && p[0] == '"' {
				s.schemaPresent = true
				if err := jsonv2.Unmarshal(p, &s.schema); err != nil {
					return nil, fail(KindInvalidInput, "JSON_MISSING_FIELD", "inventory#/schema", nil)
				}
			}
			if mode == inventoryRoutes && k == '[' {
				if e := s.b.charge(1, 64, "inventory-route-index"); e != nil {
					return nil, e
				}
				s.routes = append(s.routes, inventorySpan{at, int(s.dec.InputOffset())})
			}
			if child != inventoryDrop && project {
				key, _ := json.Marshal(name)
				if e := s.b.charge(1, 4*uint64(len(key)+len(p)+2), "inventory-projection"); e != nil {
					return nil, e
				}
				if len(out) > 1 {
					out = append(out, ',')
				}
				if k == '{' {
					out = append(out, key...)
					out = append(out, ':')
				}
				out = append(out, p...)
			}
		}
		if _, err := s.dec.ReadToken(); err != nil {
			return nil, jsonError(doc, "", err)
		}
		if project {
			return append(out, byte(end)), nil
		}
	}
	if mode != inventoryDrop {
		return bytes.TrimLeft(s.data[start:int(s.dec.InputOffset())], " \t\r\n,:"), nil
	}
	return nil, nil
}

// S reserves a strict, AST-free token pass over every byte, including unselected workloads.
func inventoryStrictIndex(data []byte, b *eligibilityBudget) (*inventoryTokenIndex, []byte, *Error) {
	const doc = "inventory"
	if uint64(len(data)) > MaxDocumentBytes {
		return nil, nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if e := b.charge(1, 3*uint64(len(data))+32768, doc); e != nil {
		return nil, nil, e
	}
	if !utf8.Valid(data) {
		return nil, nil, fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
	}
	z, e := b.significantJSONBytes(data)
	if e != nil {
		return nil, nil, e
	}
	if e := b.charge(1, 2*z, doc); e != nil {
		return nil, nil, e
	}
	s := &inventoryTokenIndex{data: data, b: b, dec: jsontext.NewDecoder(bytes.NewBuffer(data))}
	projection, e := s.value(0, inventoryRoot)
	if e != nil {
		return nil, nil, e
	}
	if _, err := s.dec.ReadToken(); err != io.EOF {
		if err != nil {
			return nil, nil, jsonError(doc, "", err)
		}
		return nil, nil, fail(KindInvalidInput, "JSON_TRAILING_VALUE", doc, nil)
	}
	return s, projection, nil
}

// T32 reserves actual typed decoding without constructing a jv tree or reflect shape maps.
func inventoryTyped(data []byte, target any, b *eligibilityBudget) *Error {
	const doc = "inventory"
	if uint64(len(data)) > MaxDocumentBytes {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	if e := b.charge(1, 5*uint64(len(data))+32768, doc); e != nil {
		return e
	}
	if !utf8.Valid(data) {
		return fail(KindInvalidInput, "JSON_INVALID_UTF8", doc, nil)
	}
	z, e := b.significantJSONBytes(data)
	if e != nil {
		return e
	}
	if e := b.charge(1, 32*z, doc); e != nil {
		return e
	}
	dec := jsontext.NewDecoder(bytes.NewBuffer(data))
	var values uint64
	for {
		kind, count := dec.StackIndex(dec.StackDepth())
		name := kind == '{' && count%2 == 0
		peek := dec.PeekKind()
		if !name && (peek == '{' || peek == '[') && dec.StackDepth() >= jsonMaxDepth {
			return fail(KindResourceLimit, "JSON_DEPTH_LIMIT", doc, nil)
		}
		tok, err := dec.ReadToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return jsonError(doc, "", err)
		}
		if !name && tok.Kind() != '}' && tok.Kind() != ']' {
			values++
			if values > 2000000 {
				return fail(KindResourceLimit, "JSON_VALUE_LIMIT", doc, nil)
			}
			if e := b.charge(4, 80, doc); e != nil {
				return e
			}
		}
	}
	if err := jsonv2.Unmarshal(data, target, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fail(KindInvalidInput, "JSON_SHAPE", doc, err)
	}
	return nil
}

// P2-C must pass bytes from a guarded read and its independently approved full-document pin.
func parseQualificationInventoryRoute(data []byte, expectedSHA256, route string, b *eligibilityBudget) (qualificationInventorySelection, *Error) {
	var result qualificationInventorySelection
	if uint64(len(data)) > MaxDocumentBytes {
		return result, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", "inventory", nil)
	}
	if e := b.charge(1, uint64(len(data)), "inventory-hash"); e != nil {
		return result, e
	}
	sha := digestHex(data)
	if !hex64.MatchString(expectedSHA256) || sha != expectedSHA256 {
		return result, fail(KindInvalidInput, "FILE_MEMBER_MISMATCH", "inventory", nil)
	}
	index, projection, e := inventoryStrictIndex(data, b)
	if e != nil {
		return result, e
	}
	if !index.schemaPresent {
		return result, fail(KindInvalidInput, "JSON_MISSING_FIELD", "inventory#/schema", nil)
	}
	if index.schema != QualificationInventorySchema && index.schema != qualificationInventoryR7 && index.schema != qualificationInventoryR6 && index.schema != qualificationInventoryR5 {
		return result, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "inventory#/schema", nil)
	}
	if e := index.checkSpecialShapes(); e != nil {
		return result, e
	}
	var inv QualificationInventory
	if e := inventoryTyped(projection, &inv, b); e != nil {
		return result, e
	}
	if e := b.charge(8*uint64(len(projection)), 4*uint64(len(projection)), "inventory-global-validation"); e != nil {
		return result, e
	}
	for _, role := range inv.ExtraRoles {
		if e := inventoryValidationProduct(b, 8, uint64(len(role.Platforms)+len(inv.Platforms)+1), uint64(len(role.Platforms)+1)); e != nil {
			return result, e
		}
	}
	if _, e := validateQualificationInventory(inv, len(inv.Routes)); e != nil {
		return result, e
	}
	selected := -1
	for i, r := range inv.Routes {
		if r.Route == route {
			selected = i
			break
		}
	}
	if selected < 0 {
		return result, fail(KindInvalidInput, "ROUTE_INVALID", "inventory#/routes", nil)
	}
	span := index.routes[selected]
	// InputOffset precedes an array separator; the selected slice must start at its value.
	raw := bytes.TrimLeft(data[span.start:span.end], " \t\r\n,")
	if e := inventoryTyped(raw, &inv.Routes[selected], b); e != nil {
		return result, e
	}
	if e := b.charge(8*uint64(len(raw))+uint64(len(projection)), 8*uint64(len(raw))+4*uint64(len(projection)), "inventory-validation"); e != nil {
		return result, e
	}
	if e := inventoryRouteValidationWork(inv.Routes[selected], b); e != nil {
		return result, e
	}
	if _, e := validateQualificationInventory(inv, selected); e != nil {
		return result, e
	}
	if e := b.r.check(); e != nil {
		return result, e
	}
	result.Route, result.RouteIndex = inv.Routes[selected], selected
	inv.SHA256, inv.Routes, inv.ExtraRoles = sha, nil, nil
	result.Header = inv
	return result, nil
}

func inventoryValidationProduct(b *eligibilityBudget, factors ...uint64) *Error {
	work, e := eligibilityProduct("inventory-validation-product", factors...)
	if e != nil {
		return e
	}
	return b.charge(work, 0, "inventory-validation-product")
}

// Coverage and duplicate/platform scans can revisit the same retained values quadratically.
func inventoryRouteValidationWork(route QualRoute, b *eligibilityBudget) *Error {
	var errorBytes, kinds uint64
	for _, name := range route.ErrorNodes {
		errorBytes += uint64(len(name)) + 1
	}
	if e := inventoryValidationProduct(b, errorBytes+1, uint64(len(route.ErrorNodes)+1)); e != nil {
		return e
	}
	for _, req := range route.Requirements {
		kinds += uint64(len(req.Kinds))
	}
	for _, c := range route.Workload.Cases {
		visits := uint64(len(c.Expect) + len(c.QueryExpect) + 1)
		for _, expect := range c.Expect {
			for _, name := range expect.Contains {
				visits += (uint64(len(name)) + 1) * uint64(len(route.ErrorNodes)+1)
			}
		}
		var coversBytes, maxClaims uint64
		for row, claimed := range c.Covers {
			coversBytes += uint64(len(row)) + 1
			maxClaims = max(maxClaims, uint64(len(claimed)))
			if e := inventoryValidationProduct(b, uint64(len(claimed)+1), kinds+2*visits+1); e != nil {
				return e
			}
		}
		if e := inventoryValidationProduct(b, uint64(len(c.Alternatives)), maxClaims+1); e != nil {
			return e
		}
		if e := inventoryValidationProduct(b, coversBytes+1, uint64(len(c.Covers)+1)); e != nil {
			return e
		}
	}
	return nil
}

// The legacy parser checks these arms globally, before typed decoding, in preorder.
func (s *inventoryTokenIndex) checkSpecialShapes() *Error {
	for _, span := range s.specials {
		raw := bytes.TrimLeft(s.data[span.start:span.end], " \t\r\n,:")
		if e := s.b.reserveJSON("inventory", raw, 3); e != nil {
			return e
		}
		v, e := decodeStrict("inventory", raw, MaxDocumentBytes)
		if e != nil {
			return e
		}
		if span.name == "reuse_accepted" {
			if s.schema != QualificationInventorySchema {
				return fail(KindInvalidInput, "WORKLOAD_INVALID", "inventory", nil)
			}
			shape := &valueBudget{max: 2000000, check: s.b.r.check, charge: func() *Error { return s.b.charge(1, 0, "inventory-special-shape") }, reserve: func(n uint64) *Error { return s.b.charge(0, n, "inventory-special-shape") }}
			if e := witnessShape(typed{doc: "inventory"}, v, reflect.TypeFor[QualReuseAcceptedRegistration](), true, false, shape); e != nil {
				return e
			}
		} else if !qualificationHasCST(s.schema) || !preprocessCSTShape(v, reflect.TypeFor[[]PreprocessCSTStep]()) {
			return fail(KindInvalidInput, "CASE_INVALID", "inventory", nil)
		}
	}
	return nil
}
